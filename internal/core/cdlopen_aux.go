//go:build cgo && unix

// The few C API pieces that are not a straight forwarder onto an existing
// State method, plus the allocation rule for userdata.
package core

/*
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
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
// userdata in a loop grows without bound.
//
// The freeing goes through setUdataFinalizer rather than being a
// finalizer of its own, because Go allows one finalizer per object and a
// userdata can also have a '__gc' -- lpeg's patterns do. One finalizer
// serves both, and when a '__gc' is waiting the block is freed after the
// metamethod has run, so that a C '__gc' still sees its own memory.
func cgoNewUserBlock(L *State, sz, uv int) unsafe.Pointer {
	block := C.malloc(C.size_t(sz))
	if block == nil {
		return nil
	}
	C.memset(block, 0, C.size_t(sz))
	u := luaS_newudata(L, uv, unsafe.Pointer(block))
	u.cblock = block
	u.len = sz /* lua_rawlen: the extension reads its size back */
	L.checkStack(1)
	setuvalue(&L.stack[L.top], u)
	L.top++
	setUdataFinalizer(L, u)
	return block
}

// udataCBlock is what lua_touserdata answers for a userdata that has no
// C block of its own -- one made in Go, which is what a file handle is.
//
// C is never given NULL for a full userdata, and an extension relies on
// that: LuaFileSystem takes a file handle, casts it to a luaL_Stream and
// reads 'closef' out of it, which is a read of 8 bytes past the pointer
// it was handed.  Given NULL it does not fail, it dies.  So every
// userdata gets a block to stand for it, made the first time C asks.
//
// It is not the object: it is zeroed, and nothing of the Go value is in
// it.  It exists so that the pointer C is given is one C may read.
func udataCBlock(L *State, u *Udata) unsafe.Pointer {
	if u.cblock != nil {
		return u.cblock
	}
	block := C.malloc(C.size_t(udataBlockSize))
	if block == nil {
		return nil
	}
	C.memset(block, 0, C.size_t(udataBlockSize))
	u.cblock = block
	u.len = udataBlockSize
	setUdataFinalizer(L, u) /* the block is freed with the userdata */
	if udataBlockFill != nil {
		udataBlockFill(u, block) /* a file handle: put a FILE* in it */
	}
	return block
}

// udataBlockFill is called on a block that was just made for a userdata,
// before C is given it.  It is the one chance to put something in it,
// and it is nil without cgo.  See liolib_cgo.go, where a file handle
// gets the FILE* an extension expects to find.
var udataBlockFill func(u *Udata, block unsafe.Pointer)

// udataBlockSize is how much C may read through the pointer it is given
// for a userdata made in Go: a luaL_Stream is a FILE* and a close
// function, and reading those two is the commonest thing an extension
// does with a file handle it did not make.
const udataBlockSize = 2 * 8

// cdlUserFrees counts the userdata blocks that have been freed. It is
// here so the test can see that the finalizer runs at all.
var cdlUserFrees atomic.Uint64

func init() {
	freeUdataCBlock = func(u *Udata) {
		if u.cblock == nil {
			return
		}
		p := u.cblock
		u.cblock = nil
		cdlUserFrees.Add(1)
		/* Not C.free: a .so may still be inside the call that dropped
		   the userdata, holding the pointer it was given. */
		cgoFree(p)
	}
}

// ---------------------------------------------------------------------
// luaL_typeerror
// ---------------------------------------------------------------------

// TypeError raises "bad argument #arg to 'name' (tname expected, got X)"
// (luaL_typeerror, lauxlib.c:206).
func (L *State) TypeError(arg int, tname string) int {
	var typearg string
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		/* 'luaL_typeerror' reads the name as a 'const char *' for '%s',
		   which is where a __name with a zero byte in it is cut short. */
		typearg = luaCStr(L.ToStringX(-1))
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
		return block /* it was made by lua_newuserdata: C's own block */
	}
	return udataCBlock(L, uvalue(index2value(L, arg))) /* made in Go */
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
