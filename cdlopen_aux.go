//go:build cgo

// The few C API pieces that are not a straight forwarder onto an existing
// State method, plus the allocation rule for userdata.
package lua

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

// ---------------------------------------------------------------------
// lua_newuserdata: memory C can write to
// ---------------------------------------------------------------------

// A C extension expects lua_newuserdata to hand back a block it can write
// through directly (`struct foo *p = lua_newuserdata(...); p->x = 1;`).
// A pointer into Go memory would break two cgo rules at once, so the block
// comes from C.malloc and the userdata keeps the pointer.
//
// The block is freed when the userdata is: C.malloc is not memory the Go
// collector knows about, so without this an extension that makes
// userdata in a loop grows without bound. The finalizer is on the
// Udata, which is a Go object, so it runs on the collector's own
// schedule -- a type with a '__gc' still gets that first, because the
// port holds those until it runs a cycle.
func cgoNewUserBlock(L *State, sz, uv int) unsafe.Pointer {
	block := C.malloc(C.size_t(sz))
	if block == nil {
		return nil
	}
	C.memset(block, 0, C.size_t(sz))
	u := luaS_newudata(L, uv, unsafe.Pointer(block))
	L.checkStack(1)
	setuvalue(&L.stack[L.top], u)
	L.top++
	runtime.SetFinalizer(u, func(*Udata) {
		C.free(block)
		cdlUserFrees.Add(1)
	})
	return block
}

// cdlUserFrees counts the userdata blocks that have been freed. It is
// here so the test can see that the finalizer runs at all.
var cdlUserFrees atomic.Uint64

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

// ---------------------------------------------------------------------
// luaL_checkudata / luaL_testudata
// ---------------------------------------------------------------------

// CDL_IDSIZE is LUA_IDSIZE: the size of lua_Debug's short_src, which
// include/lua.h declares as 60 and cdlbridge.h has to match.
const CDL_IDSIZE = 60

// cdlTestUserData is luaL_testudata (lauxlib.c:245): it answers the
// userdata at 'arg' when it carries the metatable registered under
// 'tname', and nil otherwise; with 'check' it raises instead.
func cdlTestUserData(L *State, arg int, tname string, check bool) unsafe.Pointer {
	p := L.ToUserData(arg)
	if p == nil || !L.GetMetaTable(arg) {
		if check {
			L.TypeError(arg, tname)
		}
		return nil
	}
	L.GetField(LUA_REGISTRYINDEX, tname) /* the registered metatable */
	if !L.RawEqual(-1, -2) {
		L.Pop(2)
		if check {
			L.TypeError(arg, tname)
		}
		return nil
	}
	L.Pop(2)
	if block, ok := p.(unsafe.Pointer); ok {
		return block
	}
	return nil
}

// dbUpValueJoin2 is dbUpValueJoin for lua_upvaluejoin, which takes the
// two functions and the two upvalue numbers as arguments rather than
// reading them from the stack.
func dbUpValueJoin2(L *State, f1, n1, f2, n2 int) {
	a := clLvalue(index2value(L, f1))
	b := clLvalue(index2value(L, f2))
	if a == nil || b == nil {
		return
	}
	if n1 < 1 || n1 > len(a.upvals) || n2 < 1 || n2 > len(b.upvals) {
		return
	}
	a.upvals[n1-1] = b.upvals[n2-1]
}

// ---------------------------------------------------------------------
// identity of a C function
// ---------------------------------------------------------------------

// cdlPushCClosure pushes 'f' as a C closure with 'nup' upvalues, always
// allocating one -- even when there are no upvalues.
//
// PUC-Lua's lua_pushcclosure uses a *light* C function when n is 0,
// whose identity is the C function pointer, so two different extension
// functions are two different addresses.  A Go func value has no such
// identity: every closure cFunc makes is the same code address, so all
// of them would compare equal, and 'luaL_argerror' -- which finds the
// name of the running function by searching package.loaded for it --
// would keep answering the same one.  Wrapping each in a closure of its
// own gives it the identity C gets from the address of its function.
func cdlPushCClosure(L *State, f GoFunc, nup int) {
	cl := luaF_newCclosure(L, nup)
	cl.f = f
	L.checkStack(1)
	L.top -= nup
	for i := 0; i < nup; i++ {
		cl.upvalue[i] = L.stack[L.top+i]
	}
	setclCvalue(&L.stack[L.top], cl)
	L.top++
}
