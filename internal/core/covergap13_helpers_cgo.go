//go:build cgo && unix

package core

import "unsafe"

/*
#include "cdlbridge.h"
*/
import "C"

// Test-only helpers: _test.go files cannot import "C".

func testStateOfNil() {
	_ = stateOf(nil)
}

func testStateOfUnknown() {
	var word uintptr = 0xdeadbeef
	_ = stateOf((*C.lua_State)(unsafe.Pointer(&word)))
}

func testLuaTocfunction(L *State, idx int) {
	_ = lua_tocfunction(L.statePtr(), C.int(idx))
}

func testLuaTothread(L *State, idx int) {
	_ = lua_tothread(L.statePtr(), C.int(idx))
}

func testLuaClosethread(co, from *State) int {
	var fp *C.lua_State
	if from != nil {
		fp = from.statePtr()
	}
	return int(lua_closethread(co.statePtr(), fp))
}

func testBufNilOps() {
	_ = bufOf(nil)
	bufLeave(nil, nil)
	_ = luaL_prepbuffsize(nil, 1)
	luaL_addchar_(nil, 'x')
	luaL_addstring(nil, nil)
	luaL_addvalue(nil)
	luaL_buffinit(nil, nil)
}

func testBoolChar() {
	_ = boolChar(true)
	_ = boolChar(false)
}

func testPtrOf(v uintptr) { _ = ptrOf(v) }

func testPushedCStr(L *State) {
	_ = pushedCStr(L)
}

func testRegisterCgoBuf(L *State) int {
	buf := L.NewBuffer()
	buf.cblock = true
	buf.room(0)
	buf.L = L
	cgoBufMu.Lock()
	cgoBufNext++
	id := cgoBufNext
	cgoBufs[id] = buf
	cgoBufMu.Unlock()
	return id
}
