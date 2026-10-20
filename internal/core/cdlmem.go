//go:build cgo && unix

package core

import (
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
var (
	cgoMu    sync.Mutex
	cgoDepth int
	cgoDefer []unsafe.Pointer

	// cgoFreed counts blocks actually returned to libc.
	cgoFreed atomic.Uint64
	// cstrDetached counts Lua-string copies whose owning TString has
	// been collected. The copy itself stays readable until cgoFree runs.
	cstrDetached atomic.Uint64
	// cstateLive is how many lua_State handles are still allocated.
	cstateLive atomic.Int64
)

// cgoOnQuiet runs on the transition back to "no cgo call in progress".
var cgoOnQuiet func()

func cgoEnter() {
	cgoMu.Lock()
	cgoDepth++
	cgoMu.Unlock()
}

func cgoLeave() {
	cgoMu.Lock()
	if cgoDepth > 0 {
		cgoDepth--
	}
	var batch []unsafe.Pointer
	quiet := cgoDepth == 0
	if quiet {
		batch = cgoDefer
		cgoDefer = nil
	}
	cgoMu.Unlock()
	for _, p := range batch {
		C.free(p)
		cgoFreed.Add(1)
	}
	if quiet && cgoOnQuiet != nil {
		cgoOnQuiet()
	}
}

// cgoFree releases p now when no cgo call is in progress, and otherwise
// queues it until the outermost call returns.
func cgoFree(p unsafe.Pointer) {
	if p == nil {
		return
	}
	cgoMu.Lock()
	if cgoDepth == 0 {
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
func internCStr(s string) *C.char {
	if s == "" {
		return nil
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

var cStrCache sync.Map // string -> *C.char

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

func (g *globalState) releaseCStates() {
	g.cstateMu.Lock()
	all := g.cstates
	g.cstates = nil
	g.cstateMu.Unlock()
	for _, b := range all {
		b.release()
	}
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
		unregisterState(b.id)
		if b.base != nil {
			base := b.base
			b.base = nil
			cgoFree(base)
		}
		cstateLive.Add(-1)
	})
}
