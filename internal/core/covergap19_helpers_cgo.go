//go:build cgo && unix

package core

import "unsafe"

/*
#include "cdlbridge.h"
*/
import "C"

func testCMallocNilStatePtr(L *State) {
	old := cMallocFn
	cMallocFn = func(n C.size_t) unsafe.Pointer { return nil }
	defer func() { cMallocFn = old }()
	defer func() { _ = recover() }()
	_ = L.statePtr()
}

func testCMallocNilPtrOf() {
	old := cMallocFn
	cMallocFn = func(n C.size_t) unsafe.Pointer { return nil }
	defer func() { cMallocFn = old }()
	defer func() { _ = recover() }()
	_ = ptrOf(0xbadc0de19)
}

func testLuaGetstackNil(L *State) {
	_ = lua_getstack(L.statePtr(), 0, nil)
}

func testLuaGetstackMiss(L *State) {
	var ar C.lua_Debug
	_ = lua_getstack(L.statePtr(), 99, &ar)
}

func testLuaGetinfoNil(L *State) {
	what := C.CString("n")
	defer C.free(unsafe.Pointer(what))
	_ = lua_getinfo(L.statePtr(), what, nil)
}

func testLuaLocalNil(L *State) {
	_ = lua_getlocal(L.statePtr(), nil, 1)
	_ = lua_setlocal(L.statePtr(), nil, 1)
}

func testLuaUpvalueMiss(L *State) {
	_ = lua_upvalueid(L.statePtr(), 1, 1)
	_ = lua_getupvalue(L.statePtr(), 1, 99)
	_ = lua_setupvalue(L.statePtr(), 1, 99)
}

func testLuaLNilBuf(L *State) {
	a := C.CString("a")
	b := C.CString("b")
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(b))
	luaL_addgsub(nil, a, a, b)
	_ = luaL_buffinitsize(L.statePtr(), nil, 8)
}

func testCdlPushEmpty(L *State) {
	cdl_pushlstring_cb(L.statePtr(), nil, 0)
}

func testCFuncMigrate(L *State) {
	f := cFunc(unsafe.Pointer(uintptr(1)))
	co := L.NewThread()
	co.co.sync = true
	co.co.live = false
	func() {
		defer func() { _ = recover() }()
		_ = f(co) /* sync → migrateAbort before cgo */
	}()
}
