//go:build cgo && unix

package core

import (
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"

	"github.com/esrrhs/golua/internal/goid"
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
type cgoGEntry struct {
	id    int64
	depth int
}

const stateCacheMask = 255

var (
	cgoMu         sync.Mutex
	cgoActive     = make([]cgoGEntry, 0, 16) // active goroutine id -> enter depth
	cgoParked     = map[int64][]int{}        // stack of depths stashed by nested parks
	cgoDefer      []unsafe.Pointer
	cgoDeferSpare []unsafe.Pointer

	// stateActiveCache caches active *State pointers only while a cgo call
	// is on the stack, avoiding stateRegMu and weak.Pointer.Value() on
	// every lua_* C API call while never pinning idle states/coroutines.
	stateActiveCache [stateCacheMask + 1]atomic.Pointer[State]
	stateSlowDirty   atomic.Bool

	// cgoFreed counts blocks actually returned to libc.
	cgoFreed atomic.Uint64
	// cstrDetached counts Lua-string copies whose owning TString has
	// been collected. The copy itself stays readable until cgoFree runs.
	cstrDetached atomic.Uint64
	// cstateLive is how many lua_State handles are still allocated.
	cstateLive atomic.Int64
)

func clearStateActiveCache() {
	stateSlowDirty.Store(false)
	for i := range stateActiveCache {
		if stateActiveCache[i].Load() != nil {
			stateActiveCache[i].Store(nil)
		}
	}
}

// cgoOnQuiet runs when the last active (non-parked) cgo call finishes.
// It is not run on park: a suspended frame may still need its lua_Debug
// entries in cgoArs after resume.
var cgoOnQuiet func()

func cgoGoid() int64 {
	return int64(goid.Get())
}

func cgoBusyLocked() bool { return len(cgoActive) > 0 }

func cgoIsBusy() bool {
	cgoMu.Lock()
	busy := len(cgoActive) > 0
	cgoMu.Unlock()
	return busy
}

func cgoEnter() {
	id := cgoGoid()
	cgoMu.Lock()
	for i := range cgoActive {
		if cgoActive[i].id == id {
			cgoActive[i].depth++
			cgoMu.Unlock()
			return
		}
	}
	cgoActive = append(cgoActive, cgoGEntry{id: id, depth: 1})
	cgoMu.Unlock()
}

func cgoFlushBatch(batch []unsafe.Pointer) {
	for _, p := range batch {
		C.free(p)
		cgoFreed.Add(1)
	}
	clear(batch)
	cgoMu.Lock()
	if cgoDeferSpare == nil {
		cgoDeferSpare = batch[:0]
	}
	cgoMu.Unlock()
}

func cgoLeave() {
	id := cgoGoid()
	cgoMu.Lock()
	if len(cgoActive) == 1 && cgoActive[0].id == id && cgoActive[0].depth == 1 {
		cgoActive = cgoActive[:0]
		if stateSlowDirty.Load() {
			clearStateActiveCache()
		}
		if len(cgoDefer) == 0 {
			onQuiet := len(cgoParked) == 0 && cgoArDirty.Load()
			cgoMu.Unlock()
			if onQuiet && cgoOnQuiet != nil {
				cgoOnQuiet()
			}
			return
		}
	} else {
		for i := len(cgoActive) - 1; i >= 0; i-- {
			if cgoActive[i].id == id {
				if cgoActive[i].depth > 1 {
					cgoActive[i].depth--
				} else {
					last := len(cgoActive) - 1
					cgoActive[i] = cgoActive[last]
					cgoActive = cgoActive[:last]
				}
				break
			}
		}
	}
	var batch []unsafe.Pointer
	quiet := !cgoBusyLocked()
	if quiet {
		if len(cgoDefer) > 0 {
			batch = cgoDefer
			cgoDefer = cgoDeferSpare
			cgoDeferSpare = nil
		}
		if stateSlowDirty.Load() {
			clearStateActiveCache()
		}
	}
	/* OnQuiet clears cgoArs. A parked goroutine may still need those
	   entries after resume, so wait until nothing is parked either. */
	onQuiet := quiet && len(cgoParked) == 0
	cgoMu.Unlock()
	if len(batch) > 0 {
		cgoFlushBatch(batch)
	}
	if onQuiet && cgoOnQuiet != nil {
		cgoOnQuiet()
	}
}

// cgoPark steps this goroutine out of every nested cgo enter across a
// yield. Holding a lua_tolstring pointer across a yield is already
// undefined in PUC-Lua; other goroutines must be free to reclaim memory.
// Depths are pushed so a continuation that yields again does not erase
// the outer park (that would leave the outer C frame with depth 0).
func cgoPark() {
	id := cgoGoid()
	cgoMu.Lock()
	n := 0
	for i := len(cgoActive) - 1; i >= 0; i-- {
		if cgoActive[i].id == id {
			n = cgoActive[i].depth
			last := len(cgoActive) - 1
			cgoActive[i] = cgoActive[last]
			cgoActive = cgoActive[:last]
			break
		}
	}
	if n > 0 {
		cgoParked[id] = append(cgoParked[id], n)
	}
	var batch []unsafe.Pointer
	quiet := !cgoBusyLocked()
	if quiet {
		if len(cgoDefer) > 0 {
			batch = cgoDefer
			cgoDefer = cgoDeferSpare
			cgoDeferSpare = nil
		}
		clearStateActiveCache()
	} else {
		stateSlowDirty.Store(true)
	}
	cgoMu.Unlock()
	if len(batch) > 0 {
		cgoFlushBatch(batch)
	}
}

// cgoUnpark restores the most recent park for this goroutine.
func cgoUnpark() {
	id := cgoGoid()
	cgoMu.Lock()
	stack := cgoParked[id]
	if n := len(stack); n > 0 {
		depth := stack[n-1]
		if n == 1 {
			delete(cgoParked, id)
		} else {
			cgoParked[id] = stack[:n-1]
		}
		found := false
		for i := range cgoActive {
			if cgoActive[i].id == id {
				cgoActive[i].depth = depth
				found = true
				break
			}
		}
		if !found {
			cgoActive = append(cgoActive, cgoGEntry{id: id, depth: depth})
		}
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
		slot := b.id & stateCacheMask
		if L := stateByID(b.id); L != nil {
			stateActiveCache[slot].CompareAndSwap(L, nil)
			atomic.StorePointer(&L.cstate, nil)
			L.cstateBase = nil
			L.cstateTok = nil
		} else {
			stateActiveCache[slot].Store(nil)
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

// testCgoParkStackKeepsOuterDepth parks twice (outer yield + continuation
// yield), unparks the inner nest, and reports whether a free is still
// deferred while the outer C frame would still be live.
func testCgoParkStackKeepsOuterDepth() (heldAfterInner, freedAfterOuter bool) {
	cgoEnter() // outer C function
	cgoPark()  // outer yield
	cgoEnter() // continuation
	cgoPark()  // continuation yields again
	cgoUnpark()
	cgoLeave() // continuation returns
	/* outer unpark must restore depth; without a park stack it would not */
	cgoUnpark()
	p := C.malloc(1)
	if p == nil {
		cgoLeave()
		return false, false
	}
	base := cgoFreed.Load()
	cgoFree(p)
	heldAfterInner = cgoFreed.Load() == base
	cgoLeave()
	freedAfterOuter = cgoFreed.Load() > base
	return heldAfterInner, freedAfterOuter
}

// testCgoArsSurviveQuietWhileParked parks one goroutine, lets another
// go cgo-quiet, and reports whether a cgoArs entry survived.
func testCgoArsSurviveQuietWhileParked() bool {
	const key = uintptr(0xc9a15)
	ar := &dbAr{name: "parked-ar"}
	cgoArMu.Lock()
	cgoArs[key] = ar
	cgoArDirty.Store(true)
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

// testWarnfSeesUD installs a C warnf with ud and emits a warning.
// It reports whether the handler received that same ud pointer.
func testWarnfSeesUD(L *State) bool {
	var marker int
	ud := unsafe.Pointer(&marker)
	C.cdl_test_install_warnf(L.statePtr(), ud)
	L.Warning("warn-ud-probe", false)
	return C.cdl_test_warn_ud_seen() == ud
}

// testCgoParkedCount is how many goroutines currently have a parked depth.
func testCgoParkedCount() int {
	cgoMu.Lock()
	n := len(cgoParked)
	cgoMu.Unlock()
	return n
}

// testLuaResume is lua_resume for tests that cannot import "C".
// nres is lua_gettop(L) after resume (5.3 has no out-param).
func testLuaResume(L, from *State, nargs int) (status, nres int) {
	var f *C.lua_State
	if from != nil {
		f = from.statePtr()
	}
	st := lua_resume(L.statePtr(), f, C.int(nargs))
	return int(st), L.GetTop()
}
