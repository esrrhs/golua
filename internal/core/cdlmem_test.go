//go:build cgo && unix

package core

import (
	"runtime"
	"testing"
	"time"
)

// waitUntil polls cond until it is true. Finalizers run on their own
// goroutine, after runtime.GC has returned.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 50; i++ {
		if cond() {
			return
		}
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// The pointer lua_pushstring returns is the internal copy. The caller's
// buffer may be overwritten the moment the call returns.
func TestPushStringReturnsInternalCopy(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	got, same := testPushStringCopy(L, "hello")
	if got != "hello" || !same {
		t.Fatalf("pushstring returned %q, same as tostring=%v", got, same)
	}
	if testPushLStringCopy(L, "world") != "world" {
		t.Fatal("pushlstring returned the caller's buffer")
	}
}

// lua_typename's pointer is permanent. A .so stores it, and it must be
// the same address every call -- a fresh malloc each time both leaks
// and dangles if anyone frees the "static" string PUC-Lua would return.
func TestTypeNameIsStable(t *testing.T) {
	id, text := testTypeNameID(LUA_TSTRING)
	if text != "string" {
		t.Fatalf("typename %q", text)
	}
	for i := 0; i < 1000; i++ {
		if got, _ := testTypeNameID(LUA_TSTRING); got != id {
			t.Fatal("lua_typename allocated a new string")
		}
	}
	if testKeepCStrID("same-debug-name") != testKeepCStrID("same-debug-name") {
		t.Fatal("debug name was allocated twice")
	}
}

// lua_pushfstring used to leak the formatted scratch buffer, and used to
// hand C a pointer into a 4-deep ring. The scratch is freed before the
// call returns, and the pointer C gets is the Lua string itself.
func TestPushFstringDoesNotLeak(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	live, first := testFStringRound(L, 32)
	if live != 0 {
		t.Fatalf("after pushfstring, %d scratch buffers still live", live)
	}
	if first != "i=0" {
		t.Fatalf("first pushfstring result %q", first)
	}
}

// A tolstring pointer stays readable across a garbage collection that
// has already detached the string, for as long as a cgo call is in
// progress. Freeing it from the finalizer is the use-after-free.
func TestTolstringNotFreedWhileCRuns(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	const want = "tolstring-sentinel-value"
	L.PushString(want)
	p := tsCStr(tsvalue(index2value(L, -1)))
	L.Pop(1)

	beforeDet := cstrDetached.Load()
	beforeFree := cgoFreed.Load()
	held := func() bool {
		cgoEnter()
		defer cgoLeave()
		L.fullCollect()
		waitUntil(t, "string finalizer", func() bool {
			return cstrDetached.Load() > beforeDet
		})
		if cgoFreed.Load() != beforeFree {
			t.Fatal("C string was freed while a cgo call was in progress")
		}
		if cGoString(p) != want {
			t.Fatalf("detached string reads %q", cGoString(p))
		}
		return true
	}()
	if held && cgoFreed.Load() <= beforeFree {
		t.Fatal("C string was not freed after the cgo call returned")
	}
}

// lua_newuserdata's block is not reclaimed on the finalizer's goroutine
// while C is still on the stack. It is reclaimed once that call returns.
func TestUserdataBlockSurvivesUntilCReturns(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	block := cgoNewUserBlock(L, 4, 0)
	*(*byte)(block) = 7
	beforeUser := cdlUserFrees.Load()
	beforeFree := cgoFreed.Load()
	L.Pop(1)
	held := func() bool {
		cgoEnter()
		defer cgoLeave()
		L.fullCollect()
		waitUntil(t, "userdata finalizer", func() bool {
			return cdlUserFrees.Load() > beforeUser
		})
		if cgoFreed.Load() != beforeFree {
			t.Fatal("userdata block was freed while a cgo call was in progress")
		}
		if *(*byte)(block) != 7 {
			t.Fatal("userdata block was overwritten while C could still read it")
		}
		return true
	}()
	if held && cgoFreed.Load() <= beforeFree {
		t.Fatal("userdata block was not freed after the cgo call returned")
	}
}

// __gc may store the userdata and keep it. The C block has to stay with
// the object; freeing it at the end of the metamethod dangles the pointer
// the object still owns.
func TestUserdataResurrectKeepsBlock(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	block := cgoNewUserBlock(L, 4, 0)
	*(*byte)(block) = 0xAB
	L.NewTable()
	L.PushGoFunc(func(L *State) int {
		L.PushValue(1)
		L.SetField(LUA_REGISTRYINDEX, "hold-ud")
		return 0
	})
	L.SetField(-2, "__gc")
	L.SetMetaTable(-2)
	L.Pop(1)

	before := cdlUserFrees.Load()
	L.fullCollect()
	// The metamethod has run and the object is in the registry. A few
	// more collections must not free a block that object still owns.
	for i := 0; i < 5; i++ {
		runtime.GC()
		L.fullCollect()
	}
	if cdlUserFrees.Load() != before {
		t.Fatalf("resurrected userdata lost its block (%d frees)", cdlUserFrees.Load()-before)
	}
	if *(*byte)(block) != 0xAB {
		t.Fatal("resurrected block was reused")
	}

	L.PushNil()
	L.SetField(LUA_REGISTRYINDEX, "hold-ud")
	L.fullCollect()
	waitUntil(t, "userdata block free after drop", func() bool {
		return cdlUserFrees.Load() > before
	})
}

// luaL_pushresult gives the buffer's block back. Leaving it in the
// global table pinned the buffer, and the block, for the life of the
// process.
func TestBufferReleasedOnPushResult(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	before, after, freedWhileIn, freedAfter, still := testBufferPushResult(L)
	if after != before {
		t.Fatalf("buffer still registered after pushresult: %d -> %d", before, after)
	}
	if freedWhileIn {
		t.Fatal("buffer block freed before the cgo call returned")
	}
	if !still {
		t.Fatal("buffer block was dropped while C could still see it")
	}
	if !freedAfter {
		t.Fatal("buffer block was not freed after the cgo call returned")
	}
}

// A buffer that is abandoned (its box falls off the stack) is released
// when that userdata is collected, not kept by the registry forever.
func TestAbandonedBufferIsCollected(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	before := cgoBufCount()
	testAbandonBuffer(L)
	L.Pop(1) // the box
	L.fullCollect()
	waitUntil(t, "abandoned buffer release", func() bool {
		return cgoBufCount() == before
	})
}

// Every thread C has been shown gets a malloc'd handle. Close of the
// main state gives them all back; a cgo.Handle would have pinned the
// coroutine, and the block, for the rest of the process.
func TestCStateBlocksReleasedOnClose(t *testing.T) {
	before := cstateLive.Load()
	L := NewState()
	_ = L.statePtr()
	th := L.NewThread()
	_ = th.statePtr()
	if got := cstateLive.Load(); got != before+2 {
		t.Fatalf("live handles %d, want %d", got, before+2)
	}
	L.Close()
	if got := cstateLive.Load(); got != before {
		t.Fatalf("live handles after Close %d, want %d", got, before)
	}
}
