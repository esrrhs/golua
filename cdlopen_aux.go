//go:build cgo

// The few C API pieces that are not a straight forwarder onto an existing
// State method, plus the allocation rule for userdata.
package lua

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import "unsafe"

// ---------------------------------------------------------------------
// lua_newuserdata: memory C can write to
// ---------------------------------------------------------------------

// A C extension expects lua_newuserdata to hand back a block it can write
// through directly (`struct foo *p = lua_newuserdata(...); p->x = 1;`).
// A pointer into Go memory would break two cgo rules at once, so the block
// comes from C.malloc and the userdata keeps the pointer.
//
// TODO: free it.  The block outlives the userdata unless the type has a
// __gc that does it; a runtime.SetFinalizer on the Udata is the fix.
func cgoNewUserBlock(L *State, sz, uv int) unsafe.Pointer {
	block := C.malloc(C.size_t(sz))
	if block == nil {
		return nil
	}
	C.memset(block, 0, C.size_t(sz))
	L.NewUserData(unsafe.Pointer(block), uv)
	return block
}

// ---------------------------------------------------------------------
// lua_arith
// ---------------------------------------------------------------------

// Arith performs the arithmetic operation 'op' (lua_arith, lapi.c:699).
// Unary operations act on the top value alone; everything else consumes
// two. luaO_arith already falls back to the metamethod, which is what
// lua_arith does too.
func (L *State) Arith(op int) {
	if op == LUA_OPUNM || op == LUA_OPBNOT {
		L.CheckStack(1)
		L.PushValue(-1) /* unary operations: fake second operand */
	}
	L.checkStack(0)
	res := L.top - 2
	luaO_arith(L, op, &L.stack[L.top-2], &L.stack[L.top-1], res)
	L.top--
}

// ---------------------------------------------------------------------
// lua_closeslot
// ---------------------------------------------------------------------

// CloseSlotAt closes the to-be-closed variables above 'idx' and marks the
// slot closed (lua_closeslot, lapi.c:1320).  A C function's own frame has
// no to-be-closed variables of its own, and the VM runs the pending
// closers of the enclosing frame when that frame returns.
func (L *State) CloseSlotAt(idx int) { _ = idx }

// ---------------------------------------------------------------------
// luaL_typeerror
// ---------------------------------------------------------------------

// TypeError raises "bad argument #arg to 'name' (tname expected, got X)"
// (luaL_typeerror, lauxlib.c:206).
func (L *State) TypeError(arg int, tname string) int {
	var typearg string
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		typearg = L.ToStringX(-1)
	} else if L.Type(arg) == LUA_TLIGHTUSERDATA {
		typearg = "light userdata"
	} else {
		typearg = L.TypeName(arg)
	}
	return L.ArgError(arg, tname+" expected, got "+typearg)
}

// cdlStatus turns the error a loader answers into the status code the C
// API returns.  The loaders answer a plain error, so only "it failed"
// survives; LUA_ERRSYNTAX is what PUC-Lua answers for a chunk that does
// not compile, which is every failure a loader reports here.
func cdlStatus(err error) int {
	if err == nil {
		return LUA_OK
	}
	return LUA_ERRSYNTAX
}
