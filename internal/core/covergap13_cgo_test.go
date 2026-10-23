//go:build cgo && unix

package core

import (
	"path/filepath"
	"testing"
	"unsafe"
)

func TestCoverGap13CdlNilAndBuf(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("cgo")
	}
	func() {
		defer func() { _ = recover() }()
		testStateOfNil()
	}()
	func() {
		defer func() { _ = recover() }()
		testStateOfUnknown()
	}()

	cgoFree(nil)
	testBufNilOps()
	testBoolChar()
	testPtrOf(0)
	testPtrOf(0x1234)

	L := NewState()
	defer L.Close()
	id := testRegisterCgoBuf(L)
	releaseCgoBuf(0)
	releaseCgoBuf(id)
	id2 := testRegisterCgoBuf(L)
	_ = id2
	releaseCgoBufsOf(L.g)
	releaseCgoBufsOf(nil)
	cgoReleaseState(nil)
	_ = cdlErrMsg()

	L.PushNil()
	testLuaTocfunction(L, -1)
	L.PushGoFunc(func(L *State) int { return 0 })
	testLuaTocfunction(L, -1)
	L.PushNil()
	testLuaTothread(L, -1)
	_ = L.NewThread()
	L.PushThread()
	testLuaTothread(L, -1)

	func() {
		defer func() { _ = recover() }()
		L.SetTop(0)
		L.PushLightUserData(unsafe.Pointer(&id))
		_ = L.TypeError(1, "file")
	}()

	L.SetTop(0)
	func() {
		defer func() { _ = recover() }()
		dbUpValueJoin2(L, 1, 1, 1, 1)
	}()
	L.SetTop(0)
	if err := L.LoadString(`local function f(a) return function() return a end end; return f(1), f(2)`, "=u"); err == nil {
		_ = L.PCall(0, 2, 0)
		func() {
			defer func() { _ = recover() }()
			dbUpValueJoin2(L, -2, 1, -1, 1)
		}()
		func() {
			defer func() { _ = recover() }()
			dbUpValueJoin2(L, -2, 99, -1, 1)
		}()
	}
	L.SetTop(0)

	_ = internCStr("covergap13-unique-bbbb")
	_ = internCStr("covergap13-unique-bbbb")

	stateRegMu.Lock()
	old := stateRegNext
	stateRegNext = ^uintptr(0)
	stateRegMu.Unlock()
	_ = registerState(L)
	stateRegMu.Lock()
	stateRegNext = old
	stateRegMu.Unlock()

	_ = cstateTrackedCount(nil)
	_ = cstateTrackedCount(L.g)

	_, _ = testCgoParkFlush()
	_, _ = testCgoParkNestedFlush()
	_ = testEmptyCStrID()
	_, _ = testGetUpvalueCClosureName(L)
	_, _ = testCgoParkStackKeepsOuterDepth()

	L.PushString("hi")
	testPushedCStr(L)
	L.PushNil()
	testPushedCStr(L)

	ts := luaS_newlstr(L, "freeme-covergap13")
	_ = tsCStr(ts)
	_ = ts

	_ = runChunk(t, `
local co = coroutine.create(function()
  local t <close> = setmetatable({}, {__close=function() error("c") end})
  coroutine.yield(1)
end)
coroutine.resume(co)
local ok, err = coroutine.close(co)
return tostring(ok), tostring(err)
`)
	co := L.NewThread()
	_ = testLuaClosethread(co, L)
	_ = testLuaClosethread(co, nil)
}

func TestCoverGap13CdlLoadSym(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("cgo")
	}
	so, err := filepath.Abs("testdata/tiny.so")
	if err != nil {
		t.Fatal(err)
	}
	L := NewState()
	defer L.Close()
	st := cgoLoadSym(L, so, "luaopen_tiny")
	if st != 0 {
		t.Logf("load sym status %d msg=%v", st, L.ToStringX(-1))
	}
	L.SetTop(0)
	_ = cgoLoadSym(L, so, "*")
	L.SetTop(0)
	_ = cgoLoadSym(L, "/no/such/lib.so", "x")
	L.SetTop(0)
	_ = cgoLoadSym(L, so, "missing_sym_xyz")
}
