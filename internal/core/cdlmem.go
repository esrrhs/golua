//go:build cgo && unix

package core

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

/*
#include <stdlib.h>
#include "cdlbridge.h"
*/
import "C"

// Memory C is holding must not be freed while a cgo call into a shared
// object is on some goroutine's stack. Go's collector runs during that
// call and does not see pointers that live only in C, so a finalizer
// that called C.free directly would pull the block out from under the
// .so. cgoFree postpones the free until every such call has returned.
//
// Depth is per goroutine: a yield parks that goroutine's whole nest of
// enters (outer cFunc + inner cFunc via lua_callk), so other states are
// not stuck behind a suspended frame. Concurrent cgo on another
// goroutine still holds its own depth and still defers frees.
var (
	cgoMu     sync.Mutex
	cgoByG    = map[int64]int{} // goroutine id -> enter depth
	cgoParked = map[int64]int{} // depth stashed by cgoPark for cgoUnpark
	cgoDefer  []unsafe.Pointer

	// cgoFreed counts blocks actually returned to libc.
	cgoFreed atomic.Uint64
	// cstrDetached counts Lua-string copies whose owning TString has
	// been collected. The copy itself stays readable until cgoFree runs.
	cstrDetached atomic.Uint64
	// cstateLive is how many lua_State handles are still allocated.
	cstateLive atomic.Int64
)

// cgoOnQuiet runs when the last active (non-parked) cgo call finishes.
// It is not run on park: a suspended frame may still need its lua_Debug
// entries in cgoArs after resume.
var cgoOnQuiet func()

func cgoGoid() int64 {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	// "goroutine N ["
	const p = len("goroutine ")
	if n <= p {
		return 0
	}
	id := int64(0)
	for i := p; i < n && buf[i] >= '0' && buf[i] <= '9'; i++ {
		id = id*10 + int64(buf[i]-'0')
	}
	return id
}

func cgoBusyLocked() bool { return len(cgoByG) > 0 }

func cgoEnter() {
	id := cgoGoid()
	cgoMu.Lock()
	cgoByG[id]++
	cgoMu.Unlock()
}

func cgoLeave() {
	id := cgoGoid()
	cgoMu.Lock()
	if d := cgoByG[id]; d > 1 {
		cgoByG[id] = d - 1
	} else {
		delete(cgoByG, id)
	}
	var batch []unsafe.Pointer
	quiet := !cgoBusyLocked()
	if quiet {
		batch = cgoDefer
		cgoDefer = nil
	}
	/* OnQuiet clears cgoArs. A parked goroutine may still need those
	   entries after resume, so wait until nothing is parked either. */
	onQuiet := quiet && len(cgoParked) == 0
	cgoMu.Unlock()
	for _, p := range batch {
		C.free(p)
		cgoFreed.Add(1)
	}
	if onQuiet && cgoOnQuiet != nil {
		cgoOnQuiet()
	}
}

// cgoPark steps this goroutine out of every nested cgo enter across a
// yield. Holding a lua_tolstring pointer across a yield is already
// undefined in PUC-Lua; other goroutines must be free to reclaim memory.
func cgoPark() {
	id := cgoGoid()
	cgoMu.Lock()
	n := cgoByG[id]
	delete(cgoByG, id)
	if n > 0 {
		cgoParked[id] = n
	}
	var batch []unsafe.Pointer
	quiet := !cgoBusyLocked()
	if quiet {
		batch = cgoDefer
		cgoDefer = nil
	}
	cgoMu.Unlock()
	for _, p := range batch {
		C.free(p)
		cgoFreed.Add(1)
	}
}

// cgoUnpark restores the enters that cgoPark stashed for this goroutine.
func cgoUnpark() {
	id := cgoGoid()
	cgoMu.Lock()
	n := cgoParked[id]
	delete(cgoParked, id)
	if n > 0 {
		cgoByG[id] = n
	}
	cgoMu.Unlock()
}

// cgoFree releases p now when no cgo call is in progress, and otherwise
// queues it until every active call has returned or parked.
func cgoFree(p unsafe.Pointer) {
	if p == nil {
		return
	}
	cgoMu.Lock()
	if !cgoBusyLocked() {
		cgoMu.Unlock()
		C.free(p)
		cgoFreed.Add(1)
		return
	}
	cgoDefer = append(cgoDefer, p)
	cgoMu.Unlock()
}

// internCStr returns a stable NUL-terminated copy of s. lua_typename and
// the debug API hand the pointer to C, which may keep it, so it is not
// freed. Equal strings share one copy: a fresh malloc per call grows
// without bound under a line hook.
//
// The empty string is a single immortal "" pointer: lua_getupvalue must
// return non-NULL for a C closure's nameless upvalue, as PUC-Lua does.
func internCStr(s string) *C.char {
	if s == "" {
		return cStrEmpty
	}
	if v, ok := cStrCache.Load(s); ok {
		return v.(*C.char)
	}
	cs := C.CString(s)
	if actual, loaded := cStrCache.LoadOrStore(s, cs); loaded {
		C.free(unsafe.Pointer(cs))
		return actual.(*C.char)
	}
	return cs
}

var (
	cStrCache sync.Map // string -> *C.char
	cStrEmpty = C.CString("")
)

// ---------------------------------------------------------------------
// lua_State handles
// ---------------------------------------------------------------------

// A lua_State* C holds is the address of a malloc'd word, not a Go
// pointer. The word stores an integer naming the state. A cgo.Handle
// would pin the state for as long as the block lived, so a coroutine C
// had merely looked at could never be collected. The registry is weak:
// once nothing in Go refers to the state, it can be collected and the
// block freed.
var (
	stateRegMu   sync.Mutex
	stateReg     = map[uintptr]weak.Pointer[State]{}
	stateRegNext uintptr
)

func registerState(L *State) uintptr {
	stateRegMu.Lock()
	defer stateRegMu.Unlock()
	stateRegNext++
	if stateRegNext == 0 {
		stateRegNext++
	}
	id := stateRegNext
	stateReg[id] = weak.Make(L)
	return id
}

func unregisterState(id uintptr) {
	stateRegMu.Lock()
	delete(stateReg, id)
	stateRegMu.Unlock()
}

func stateByID(id uintptr) *State {
	stateRegMu.Lock()
	w, ok := stateReg[id]
	stateRegMu.Unlock()
	if !ok {
		return nil
	}
	return w.Value()
}

func (g *globalState) trackCState(b *cstateBlock) {
	g.cstateMu.Lock()
	g.cstates = append(g.cstates, b)
	g.cstateMu.Unlock()
}

func (g *globalState) untrackCState(b *cstateBlock) {
	if g == nil || b == nil {
		return
	}
	g.cstateMu.Lock()
	for i, x := range g.cstates {
		if x == b {
			g.cstates[i] = g.cstates[len(g.cstates)-1]
			g.cstates[len(g.cstates)-1] = nil
			g.cstates = g.cstates[:len(g.cstates)-1]
			break
		}
	}
	g.cstateMu.Unlock()
}

func (g *globalState) releaseCStates() {
	g.cstateMu.Lock()
	all := g.cstates
	g.cstates = nil
	g.cstateMu.Unlock()
	for _, b := range all {
		b.release()
	}
}

func cstateTrackedCount(g *globalState) int {
	if g == nil {
		return 0
	}
	g.cstateMu.Lock()
	n := len(g.cstates)
	g.cstateMu.Unlock()
	return n
}

// ---------------------------------------------------------------------
// helpers the regression tests call. A _test.go file cannot import "C".
// ---------------------------------------------------------------------

// testPushStringCopy pushes s and reports what the pointer handed back
// to C reads after the caller's own buffer has been overwritten.
func testPushStringCopy(L *State, s string) (got string, sameAsToString bool) {
	raw := C.CString(s)
	defer C.free(unsafe.Pointer(raw))
	p := L.statePtr()
	ret := lua_pushstring(p, raw)
	if len(s) > 0 {
		*raw = *raw + 1
	}
	got = C.GoString(ret)
	sameAsToString = lua_tostring(p, -1) == ret
	return got, sameAsToString
}

// testPushLStringCopy is testPushStringCopy for an explicit length.
func testPushLStringCopy(L *State, s string) string {
	raw := C.CString(s)
	defer C.free(unsafe.Pointer(raw))
	ret := lua_pushlstring(L.statePtr(), raw, C.size_t(len(s)))
	if len(s) > 0 {
		*raw = *raw + 1
	}
	return C.GoString(ret)
}

// testTypeNameID is the address lua_typename returns for tp, and the text.
func testTypeNameID(tp int) (uintptr, string) {
	p := lua_typename(nil, C.int(tp))
	return uintptr(unsafe.Pointer(p)), C.GoString(p)
}

// testKeepCStrID is the address keepCStr returns for s.
func testKeepCStrID(s string) uintptr {
	return uintptr(unsafe.Pointer(keepCStr(s)))
}

// testFStringRound pushes n formatted strings and pops each one. It
// reports how many scratch buffers were still live afterwards, and the
// text of the first result as it read back before that value was popped.
func testFStringRound(L *State, n int) (live int, first string) {
	p := L.statePtr()
	for i := 0; i < n; i++ {
		s := C.cdl_test_one_fstring(p, C.int(i))
		if i == 0 {
			first = C.GoString(s)
		}
		L.Pop(1)
	}
	return int(C.cdl_out_live_count()), first
}

func cGoString(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	return C.GoString((*C.char)(p))
}

// testAbandonBuffer starts a buffer and leaves its box on the stack.
func testAbandonBuffer(L *State) {
	var b C.luaL_Buffer
	luaL_buffinit(L.statePtr(), &b)
}

// testBufferPushResult builds a one-byte buffer and finishes it while a
// cgo call is notionally in progress. before/after are registered
// buffers, freed is how many blocks C.free saw across the finish, and
// still is whether the byte was readable until the call returned.
func testBufferPushResult(L *State) (before, after int, freedWhileIn, freedAfter bool, still bool) {
	p := L.statePtr()
	before = cgoBufCount()
	var b C.luaL_Buffer
	luaL_buffinit(p, &b)
	cs := C.CString("Z")
	defer C.free(unsafe.Pointer(cs))
	luaL_addstring(&b, cs)
	base := cgoFreed.Load()
	func() {
		cgoEnter()
		defer cgoLeave()
		luaL_pushresult(&b)
		after = cgoBufCount()
		freedWhileIn = cgoFreed.Load() != base
		still = b.b != nil && *b.b == 'Z'
	}()
	freedAfter = cgoFreed.Load() > base
	return before, after, freedWhileIn, freedAfter, still
}

func (b *cstateBlock) release() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		if L := stateByID(b.id); L != nil {
			L.cstate = nil
			L.cstateBase = nil
			L.cstateTok = nil
		}
		if b.g != nil {
			b.g.untrackCState(b)
		}
		unregisterState(b.id)
		if b.base != nil {
			base := b.base
			b.base = nil
			cgoFree(base)
		}
		cstateLive.Add(-1)
	})
}

// testCgoParkFlush enters a cgo call, queues a free, parks (as a yield
// would), and reports whether the free happened before and after park.
func testCgoParkFlush() (heldDuringEnter, freedOnPark bool) {
	cgoEnter()
	p := C.malloc(1)
	if p == nil {
		cgoLeave()
		return false, false
	}
	base := cgoFreed.Load()
	cgoFree(p)
	heldDuringEnter = cgoFreed.Load() == base
	cgoPark()
	freedOnPark = cgoFreed.Load() > base
	cgoUnpark()
	cgoLeave()
	return heldDuringEnter, freedOnPark
}

// testCgoParkNestedFlush is testCgoParkFlush with two nested enters, the
// shape of an outer C function that lua_callk's into another that yields.
func testCgoParkNestedFlush() (heldDuringEnter, freedOnPark bool) {
	cgoEnter()
	cgoEnter()
	p := C.malloc(1)
	if p == nil {
		cgoLeave()
		cgoLeave()
		return false, false
	}
	base := cgoFreed.Load()
	cgoFree(p)
	heldDuringEnter = cgoFreed.Load() == base
	cgoPark()
	freedOnPark = cgoFreed.Load() > base
	cgoUnpark()
	cgoLeave()
	cgoLeave()
	return heldDuringEnter, freedOnPark
}

// testEmptyCStrID is the address keepCStr returns for "".
func testEmptyCStrID() uintptr {
	p := keepCStr("")
	if p == nil {
		return 0
	}
	return uintptr(unsafe.Pointer(p))
}

// testGetUpvalueCClosureName calls lua_getupvalue on a one-upvalue C
// closure and reports the name pointer's text ("" for C closures).
func testGetUpvalueCClosureName(L *State) (name string, nonNil bool) {
	cl := luaF_newCclosure(L, 1)
	cl.f = func(L *State) int { return 0 }
	setivalue(&cl.upvalue[0], 1)
	L.checkStack(1)
	setclCvalue(&L.stack[L.top], cl)
	L.top++
	p := lua_getupvalue(L.statePtr(), -1, 1)
	if p == nil {
		return "", false
	}
	return C.GoString(p), true
}

// testCgoArsSurviveQuietWhileParked parks one goroutine, lets another
// go cgo-quiet, and reports whether a cgoArs entry survived.
func testCgoArsSurviveQuietWhileParked() bool {
	const key = uintptr(0xc9a15)
	ar := &dbAr{name: "parked-ar"}
	cgoArMu.Lock()
	cgoArs[key] = ar
	cgoArMu.Unlock()

	parked := make(chan struct{})
	release := make(chan struct{})
	go func() {
		cgoEnter()
		cgoPark()
		close(parked)
		<-release
		cgoUnpark()
		cgoLeave()
	}()
	<-parked
	/* This leave is quiet for active cgo, but must not OnQuiet-wipe
	   cgoArs while the other goroutine is parked. */
	cgoEnter()
	cgoLeave()
	cgoArMu.Lock()
	_, ok := cgoArs[key]
	delete(cgoArs, key)
	cgoArMu.Unlock()
	close(release)
	return ok
}

// testErrorDoesNotLeak raises n formatted luaL_error calls under pcall
// and answers how many of their scratch buffers were still live after.
func testErrorDoesNotLeak(L *State, n int) int {
	return int(C.cdl_test_error_round(L.statePtr(), C.int(n)))
}

func testHoldPanicPtr(L *State) {
	panicMu.Lock()
	/* any non-nil C function pointer; the test only cares that Close deletes the entry */
	panicCPtr[L.g] = unsafe.Pointer(C.cdl_allocfn())
	panicMu.Unlock()
}

func panicCPtrHolds(g *globalState) bool {
	panicMu.Lock()
	_, ok := panicCPtr[g]
	panicMu.Unlock()
	return ok
}
