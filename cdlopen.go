//go:build cgo

// The dynamic-loading backend: what lets this port call a C extension.
//
// A Lua extension is a shared object exporting luaopen_<name> that calls
// the Lua C API.  On Unix it is built *without* linking liblua, and the
// loader resolves lua_* and luaL_* from the host binary -- so the host has
// to export them, and a Go binary can: a //export-ed function lands in the
// dynamic symbol table where dlopen looks.  That was measured before it
// was relied on (a shared object whose only undefined symbol was an
// exported Go function resolved and ran), and it needs no linker flags.
//
// So the names below are the real lua_* and luaL_* names, and each is a
// thin forwarder onto the same State methods the rest of the port uses --
// nothing here reimplements anything.
//
// Two details shape the file:
//
//   - lua_State is an opaque handle C never dereferences: the address
//     of a malloc'd word holding a runtime/cgo.Handle.  No Go pointer
//     lives in C memory (which cgo forbids), and encoding an id in the
//     pointer instead would mean turning a uintptr into a pointer,
//     which is the one thing go vet will not let past.
//
//   - Go cannot call a C function pointer, so every call into an
//     extension goes through a trampoline in cdlbridge.h, which is
//     ordinary C and can.
//
// This file carries the core of the API -- stack, types, conversion,
// push, get/set, call, and most of the auxiliary library.
// cdlopen_ext.go carries the rest: coroutines, hooks, the debug API,
// user values, lua_dump, and the auxiliary functions that go with them.
// Together they are every name in include/lua.h and include/lauxlib.h
// except the four that take '...', which cdlbridge.c has instead.
// cdlopen_api_test.go pins them all against PUC-Lua 5.4.9.
//
// On lua_error: PUC-Lua longjmps out of the extension's own frame. This
// port raises instead, and the panic unwinds through that frame -- which
// sounds wrong and is not, for two reasons that were measured before
// being relied on (cdlopen_api_test.go, TestCModuleErrors).
//
//   - A Go panic crosses a cgo frame: the runtime unwinds to the
//     recover in the call that was protecting the call, and the
//     extension's frame is left behind exactly as longjmp would leave
//     it. Three levels deep -- Lua -> C -> Lua -> C -> raise -- the
//     answer is PUC-Lua's, line for line.
//
//   - longjmp across the boundary is the thing that would be unsafe:
//     it would skip the runtime's own bookkeeping for the cgo call. So
//     there is no setjmp here on purpose.
//
// What is lost is the same as what longjmp loses: a C frame that holds
// something needing cleanup (a lock, a C++ destructor) does not get to
// clean it up. PUC-Lua does not either.
package lua

/*
#include <dlfcn.h>

// cdlbridge.h holds every layout that has to agree with include/lua.h
// and include/lauxlib.h, the trampolines C has to provide, and the
// callbacks of the four variadic functions defined in cdlbridge.c.
#include "cdlbridge.h"

static void *cdl_sym(void *h, const char *s) { return dlsym(h, s); }
static const char *cdl_error(void) { return dlerror(); }
*/
import "C"

import (
	"runtime"
	"runtime/cgo"
	"sync"
	"unsafe"
)

// ---------------------------------------------------------------------
// lua_State as an opaque handle
// ---------------------------------------------------------------------

// A `lua_State *` has to be an address C can hold, even though C never
// dereferences it: cgo will not let a Go pointer be left in C memory,
// and the state is not a C object to begin with. So each state gets one
// malloc'd word holding a runtime/cgo.Handle that names it, and that
// address is what C carries around. Making the number up out of a Go
// address would answer the same question, but only by converting a
// uintptr to a pointer, which is exactly what the rules (and go vet)
// forbid.
// luaExtraSpace is LUA_EXTRASPACE (luaconf.h): the bytes PUC-Lua keeps in
// front of every lua_State, which 'lua_getextraspace' hands to C.
const luaExtraSpace = unsafe.Sizeof(uintptr(0))

func (L *State) statePtr() *C.lua_State {
	if L.cstate == nil {
		/* The block is LUA_EXTRASPACE bytes followed by the handle:
		   'lua_getextraspace' hands C the bytes *before* the pointer it
		   was given, and PUC-Lua really does keep them there, so they
		   have to exist and be addressable. */
		size := unsafe.Sizeof(uintptr(0))
		base := C.malloc(C.size_t(luaExtraSpace) + C.size_t(size))
		if base == nil {
			panic("lua: out of memory for a lua_State handle")
		}
		C.memset(base, 0, C.size_t(luaExtraSpace))
		block := unsafe.Pointer(uintptr(base) + luaExtraSpace)
		*(*uintptr)(block) = uintptr(cgo.NewHandle(L))
		L.cstate = block
		L.cstateBase = base
	}
	return (*C.lua_State)(L.cstate)
}

func stateOf(p *C.lua_State) *State {
	if p == nil {
		panic("lua: C handed back a null lua_State")
	}
	h := cgo.Handle(*(*uintptr)(unsafe.Pointer(p)))
	L, ok := h.Value().(*State)
	if !ok {
		panic("lua: C handed back an unknown lua_State")
	}
	return L
}

// cgoReleaseState gives back the handle a state's block holds. It is
// called from State.Close, which is in a file built either way, hence
// the indirection: without cgo there is no block and nothing to give
// back.
func cgoReleaseState(L *State) {
	if L.cstate == nil {
		return
	}
	cgo.Handle(*(*uintptr)(L.cstate)).Delete()
	C.free(L.cstateBase)
	L.cstate = nil
	L.cstateBase = nil
}

// cFunc wraps a C function pointer as the GoFunc the VM calls: the VM
// sees an ordinary closure, and the closure crosses into C.
func cFunc(f unsafe.Pointer) GoFunc {
	return func(L *State) int {
		return int(C.cdl_call_cfunction(C.lua_CFunction(f), L.statePtr()))
	}
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------
// stack
// ---------------------------------------------------------------------

//export lua_gettop
func lua_gettop(p *C.lua_State) C.int { return C.int(stateOf(p).GetTop()) }

//export lua_settop
func lua_settop(p *C.lua_State, idx C.int) { stateOf(p).SetTop(int(idx)) }

//export lua_absindex
func lua_absindex(p *C.lua_State, idx C.int) C.int { return C.int(stateOf(p).AbsIndex(int(idx))) }

//export lua_checkstack
func lua_checkstack(p *C.lua_State, n C.int) C.int { return cBool(stateOf(p).CheckStack(int(n))) }

//export lua_rotate
func lua_rotate(p *C.lua_State, idx, n C.int) { stateOf(p).Rotate(int(idx), int(n)) }

//export lua_copy
func lua_copy(p *C.lua_State, from, to C.int) { stateOf(p).Copy(int(from), int(to)) }

//export lua_closeslot
func lua_closeslot(p *C.lua_State, idx C.int) {
	L := stateOf(p)
	/* luaF_close on the given level; there is nothing to close in a
	   frame the VM owns until to-be-closed variables reach it. */
	L.CloseSlotAt(int(idx))
}

//export lua_pop
func lua_pop(p *C.lua_State, n C.int) { stateOf(p).SetTop(-int(n) - 1) }

//export lua_insert
func lua_insert(p *C.lua_State, idx C.int) { stateOf(p).Rotate(int(idx), 1) }

//export lua_remove
func lua_remove(p *C.lua_State, idx C.int) {
	L := stateOf(p)
	L.Rotate(int(idx), -1)
	L.Pop(1)
}

// ---------------------------------------------------------------------
// types
// ---------------------------------------------------------------------

//export lua_type
func lua_type(p *C.lua_State, idx C.int) C.int { return C.int(stateOf(p).Type(idxInt(idx))) }

//export lua_typename
func lua_typename(p *C.lua_State, tp C.int) *C.char { return C.CString(TypeName(int(tp))) }

//export lua_isnil
func lua_isnil(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsNil(idxInt(idx))) }

//export lua_isboolean
func lua_isboolean(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsBoolean(idxInt(idx))) }

//export lua_isnumber
func lua_isnumber(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsNumber(idxInt(idx))) }

//export lua_isinteger
func lua_isinteger(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsInteger(idxInt(idx))) }

//export lua_isstring
func lua_isstring(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsString(idxInt(idx))) }

//export lua_istable
func lua_istable(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsTable(idxInt(idx))) }

//export lua_isthread
func lua_isthread(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsThread(idxInt(idx))) }

//export lua_isuserdata
func lua_isuserdata(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).IsUserData(idxInt(idx)))
}

//export lua_islightuserdata
func lua_islightuserdata(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).IsLightUserData(idxInt(idx)))
}

//export lua_isfunction
func lua_isfunction(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).IsFunction(idxInt(idx)))
}

//export lua_iscfunction
func lua_iscfunction(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).IsGoFunction(idxInt(idx)))
}

//export lua_isnone
func lua_isnone(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).IsNone(idxInt(idx))) }

//export lua_isnoneornil
func lua_isnoneornil(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).IsNoneOrNil(idxInt(idx)))
}

//export lua_rawequal
func lua_rawequal(p *C.lua_State, a, b C.int) C.int {
	return cBool(stateOf(p).RawEqual(idxInt(a), idxInt(b)))
}

func idxInt(i C.int) int { return int(i) }

// ---------------------------------------------------------------------
// conversion
// ---------------------------------------------------------------------

//export lua_tonumberx
func lua_tonumberx(p *C.lua_State, idx C.int, isnum *C.int) C.lua_Number {
	n, ok := stateOf(p).ToNumber(idxInt(idx))
	if isnum != nil {
		*isnum = cBool(ok)
	}
	return C.lua_Number(n)
}

//export lua_tointegerx
func lua_tointegerx(p *C.lua_State, idx C.int, isnum *C.int) C.lua_Integer {
	n, ok := stateOf(p).ToInteger(idxInt(idx))
	if isnum != nil {
		*isnum = cBool(ok)
	}
	return C.lua_Integer(n)
}

//export lua_toboolean
func lua_toboolean(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).ToBoolean(idxInt(idx))) }

//export lua_tolstring
func lua_tolstring(p *C.lua_State, idx C.int, l *C.size_t) *C.char {
	L := stateOf(p)
	v := index2value(L, idxInt(idx))
	if !ttisstring(v) {
		/* PUC-Lua answers NULL for anything it cannot convert, and an
		   extension tests for it: lpeg's 'val2str' uses NULL to tell a
		   rule named by a string from one named by a table. */
		if !cvt2str(v) {
			if l != nil {
				*l = 0
			}
			return nil
		}
		luaO_tostring(L, v) /* as PUC-Lua does: the slot becomes a string */
	}
	ts := tsvalue(v)
	if l != nil {
		*l = C.size_t(len(ts.val))
	}
	return (*C.char)(tsCStr(ts))
}

// tsCStr answers a NUL-terminated copy of a Lua string, which is what C
// needs: a Go string carries no terminator, and C reads until it finds
// one. The copy is made once and kept with the string, because C holds on
// to the pointer -- lpeg keeps the name of a rule around while it builds
// the message it goes into -- and it is freed when the string itself is.
func tsCStr(ts *TString) unsafe.Pointer {
	cstrMu.Lock()
	defer cstrMu.Unlock()
	if ts.cstr == nil {
		ts.cstr = unsafe.Pointer(C.CString(ts.val))
		runtime.SetFinalizer(ts, func(t *TString) {
			cstrMu.Lock()
			if t.cstr != nil {
				C.free(t.cstr)
				t.cstr = nil
			}
			cstrMu.Unlock()
		})
	}
	return ts.cstr
}

var cstrMu sync.Mutex

//export lua_tostring
func lua_tostring(p *C.lua_State, idx C.int) *C.char { return lua_tolstring(p, idx, nil) }

//export lua_tonumber
func lua_tonumber(p *C.lua_State, idx C.int) C.lua_Number { return lua_tonumberx(p, idx, nil) }

//export lua_tointeger
func lua_tointeger(p *C.lua_State, idx C.int) C.lua_Integer { return lua_tointegerx(p, idx, nil) }

//export lua_rawlen
func lua_rawlen(p *C.lua_State, idx C.int) C.lua_Unsigned {
	return C.lua_Unsigned(stateOf(p).RawLen(idxInt(idx)))
}

//export lua_touserdata
func lua_touserdata(p *C.lua_State, idx C.int) unsafe.Pointer {
	L := stateOf(p)
	ud := L.ToUserData(idxInt(idx))
	if p2, ok := ud.(unsafe.Pointer); ok {
		return p2
	}
	if ud == nil {
		return nil /* not a userdata at all: PUC-Lua answers NULL */
	}
	/* A userdata made in Go still has an address, and C is entitled to
	   read through it -- see udataCBlock. */
	return udataCBlock(L, uvalue(index2value(L, idxInt(idx))))
}

//export lua_topointer
func lua_topointer(p *C.lua_State, idx C.int) unsafe.Pointer {
	return ptrOf(stateOf(p).ToPointer(idxInt(idx)))
}

var (
	ptrMu   sync.Mutex
	ptrAddr = map[uintptr]unsafe.Pointer{}
)

// ptrOf answers the address lua_topointer hands back for a value. C uses
// it as an identity, not as something to dereference, but it still has
// to be an address rather than a number turned into one, so each
// distinct value gets a one-byte block that is then always answered for
// it. The blocks are never freed: there is no point at which C is known
// to have finished with one.
func ptrOf(v uintptr) unsafe.Pointer {
	if v == 0 {
		return nil /* as in C: no pointer to answer */
	}
	ptrMu.Lock()
	defer ptrMu.Unlock()
	p, ok := ptrAddr[v]
	if !ok {
		p = C.malloc(1)
		if p == nil {
			panic("lua: out of memory for a value's identity")
		}
		ptrAddr[v] = p
	}
	return p
}

// ---------------------------------------------------------------------
// push
// ---------------------------------------------------------------------

//export lua_pushnil
func lua_pushnil(p *C.lua_State) { stateOf(p).PushNil() }

//export lua_pushnumber
func lua_pushnumber(p *C.lua_State, n C.lua_Number) { stateOf(p).PushNumber(float64(n)) }

//export lua_pushinteger
func lua_pushinteger(p *C.lua_State, n C.lua_Integer) { stateOf(p).PushInteger(int64(n)) }

//export lua_pushlstring
func lua_pushlstring(p *C.lua_State, s *C.char, l C.size_t) *C.char {
	stateOf(p).PushBytes([]byte(C.GoStringN(s, C.int(l))))
	return s
}

//export lua_pushstring
func lua_pushstring(p *C.lua_State, s *C.char) *C.char {
	if s == nil {
		stateOf(p).PushNil()
		return nil
	}
	stateOf(p).PushString(C.GoString(s))
	return s
}

//export lua_pushboolean
func lua_pushboolean(p *C.lua_State, b C.int) { stateOf(p).PushBoolean(b != 0) }

//export lua_pushcclosure
func lua_pushcclosure(p *C.lua_State, f C.lua_CFunction, n C.int) {
	/* see cdlPushCClosure: a light C function would make every
	   extension function compare equal */
	cdlPushCClosure(stateOf(p), cFunc(unsafe.Pointer(f)), int(n))
}

//export lua_pushlightuserdata
func lua_pushlightuserdata(p *C.lua_State, ud unsafe.Pointer) { stateOf(p).PushLightUserData(ud) }

//export lua_pushvalue
func lua_pushvalue(p *C.lua_State, idx C.int) { stateOf(p).PushValue(idxInt(idx)) }

//export lua_pushglobaltable
func lua_pushglobaltable(p *C.lua_State) { stateOf(p).PushGlobalTable() }

// ---------------------------------------------------------------------
// get and set
// ---------------------------------------------------------------------

//export lua_getglobal
func lua_getglobal(p *C.lua_State, name *C.char) C.int {
	return C.int(stateOf(p).GetGlobal(C.GoString(name)))
}

//export lua_gettable
func lua_gettable(p *C.lua_State, idx C.int) C.int { return C.int(stateOf(p).GetTable(idxInt(idx))) }

//export lua_getfield
func lua_getfield(p *C.lua_State, idx C.int, k *C.char) C.int {
	return C.int(stateOf(p).GetField(idxInt(idx), C.GoString(k)))
}

//export lua_geti
func lua_geti(p *C.lua_State, idx C.int, n C.lua_Integer) C.int {
	return C.int(stateOf(p).GetI(idxInt(idx), int64(n)))
}

//export lua_rawget
func lua_rawget(p *C.lua_State, idx C.int) C.int { return C.int(stateOf(p).RawGet(idxInt(idx))) }

//export lua_rawgeti
func lua_rawgeti(p *C.lua_State, idx C.int, n C.lua_Integer) C.int {
	return C.int(stateOf(p).RawGetI(idxInt(idx), int64(n)))
}

//export lua_getmetatable
func lua_getmetatable(p *C.lua_State, idx C.int) C.int {
	return cBool(stateOf(p).GetMetaTable(idxInt(idx)))
}

//export lua_createtable
func lua_createtable(p *C.lua_State, narr, nrec C.int) { stateOf(p).CreateTable(int(narr), int(nrec)) }

//export lua_newtable
func lua_newtable(p *C.lua_State) { stateOf(p).CreateTable(0, 0) }

//export lua_setglobal
func lua_setglobal(p *C.lua_State, name *C.char) { stateOf(p).SetGlobal(C.GoString(name)) }

//export lua_settable
func lua_settable(p *C.lua_State, idx C.int) { stateOf(p).SetTable(idxInt(idx)) }

//export lua_setfield
func lua_setfield(p *C.lua_State, idx C.int, k *C.char) {
	stateOf(p).SetField(idxInt(idx), C.GoString(k))
}

//export lua_seti
func lua_seti(p *C.lua_State, idx C.int, n C.lua_Integer) { stateOf(p).SetI(idxInt(idx), int64(n)) }

//export lua_rawset
func lua_rawset(p *C.lua_State, idx C.int) { stateOf(p).RawSet(idxInt(idx)) }

//export lua_rawseti
func lua_rawseti(p *C.lua_State, idx C.int, n C.lua_Integer) {
	stateOf(p).RawSetI(idxInt(idx), int64(n))
}

//export lua_setmetatable
func lua_setmetatable(p *C.lua_State, idx C.int) C.int {
	/* Nothing may be pushed between reading the arguments and setting the
	   table: the new metatable is the value already on top of the stack,
	   so asking whether there is an old one -- which pushes -- would make
	   the call set that answer instead, and leave a value behind.  An
	   extension then finds its object carrying the metatable it had
	   before, which is not the one it registered, and 'luaL_testudata'
	   no longer recognises it.  That is how luasocket's server socket
	   stopped being a tcp{server}.  PUC-Lua answers 1 whenever it sets
	   one at all. */
	stateOf(p).SetMetaTable(idxInt(idx))
	return 1
}

//export lua_next
func lua_next(p *C.lua_State, idx C.int) C.int { return cBool(stateOf(p).Next(idxInt(idx))) }

//export lua_len
func lua_len(p *C.lua_State, idx C.int) { stateOf(p).Len(idxInt(idx)) }

//export lua_concat
func lua_concat(p *C.lua_State, n C.int) { stateOf(p).Concat(int(n)) }

//export lua_compare
func lua_compare(p *C.lua_State, a, b, op C.int) C.int {
	L := stateOf(p)
	i, j := idxInt(a), idxInt(b)
	switch op {
	case 0: /* LUA_OPEQ */
		return cBool(L.Equal(i, j))
	case 1: /* LUA_OPLT */
		return cBool(L.LessThan(i, j))
	case 2: /* LUA_OPLE */
		return cBool(L.Equal(i, j) || L.LessThan(i, j))
	}
	return 0
}

//export lua_arith
func lua_arith(p *C.lua_State, op C.int) { stateOf(p).Arith(int(op)) }

//export lua_newuserdatauv
func lua_newuserdatauv(p *C.lua_State, sz C.size_t, uv C.int) unsafe.Pointer {
	return cgoNewUserBlock(stateOf(p), int(sz), int(uv))
}

//export lua_newuserdata
func lua_newuserdata(p *C.lua_State, sz C.size_t) unsafe.Pointer { return lua_newuserdatauv(p, sz, 1) }

// ---------------------------------------------------------------------
// load, call, error
// ---------------------------------------------------------------------

//export lua_call
func lua_call(p *C.lua_State, nargs, nresults C.int) { stateOf(p).Call(int(nargs), int(nresults)) }

//export lua_pcall
func lua_pcall(p *C.lua_State, nargs, nresults, msgh C.int) C.int {
	return C.int(stateOf(p).PCall(int(nargs), int(nresults), int(msgh)))
}

//export lua_error
func lua_error(p *C.lua_State) C.int {
	// TODO: must longjmp, not unwind through the extension's frames.
	stateOf(p).Error()
	return 0
}

//export lua_load
func lua_load(p *C.lua_State, reader C.lua_Reader, data unsafe.Pointer,
	chunkname, mode *C.char) C.int {
	L := stateOf(p)
	name := C.GoString(chunkname)
	m := ""
	if mode != nil {
		m = C.GoString(mode)
	}
	if reader == nil { /* load from a string the caller put behind 'data' */
		src := C.GoString((*C.char)(data))
		return C.int(cdlStatus(L.LoadBytesMode([]byte(src), name, m)))
	}
	return C.int(cdlStatus(L.LoadStream(nil, func() []byte {
		var sz C.size_t
		cs := C.cdl_call_reader(reader, p, data, &sz)
		if cs == nil || sz == 0 {
			return nil
		}
		return []byte(C.GoStringN(cs, C.int(sz)))
	}, name, m)))
}

// ---------------------------------------------------------------------
// the auxiliary library
// ---------------------------------------------------------------------

//export luaL_checkversion_
func luaL_checkversion_(p *C.lua_State, ver C.lua_Number, sz C.size_t) {}

//export luaL_checkinteger
func luaL_checkinteger(p *C.lua_State, arg C.int) C.lua_Integer {
	return C.lua_Integer(stateOf(p).CheckInteger(int(arg)))
}

//export luaL_optinteger
func luaL_optinteger(p *C.lua_State, arg C.int, def C.lua_Integer) C.lua_Integer {
	return C.lua_Integer(stateOf(p).OptInteger(int(arg), int64(def)))
}

//export luaL_checknumber
func luaL_checknumber(p *C.lua_State, arg C.int) C.lua_Number {
	return C.lua_Number(stateOf(p).CheckNumber(int(arg)))
}

//export luaL_optnumber
func luaL_optnumber(p *C.lua_State, arg C.int, def C.lua_Number) C.lua_Number {
	return C.lua_Number(stateOf(p).OptNumber(int(arg), float64(def)))
}

//export luaL_checklstring
func luaL_checklstring(p *C.lua_State, arg C.int, l *C.size_t) *C.char {
	/* luaL_checklstring is lua_tolstring plus a type error, so it is
	   written that way: the pointer has to outlive the call, and
	   C.CString's fresh allocation does at the cost of never being
	   freed -- mime's filters call it once per chunk. */
	s := lua_tolstring(p, arg, l)
	if s == nil {
		stateOf(p).tagError(int(arg), LUA_TSTRING)
	}
	return s
}

//export luaL_optlstring
func luaL_optlstring(p *C.lua_State, arg C.int, def *C.char, l *C.size_t) *C.char {
	L := stateOf(p)
	if L.IsNoneOrNil(int(arg)) {
		/* The default is handed straight back, and NULL is a default an
		   extension asks for on purpose: mime's filters take
		   luaL_optlstring(L, 2, NULL, &size) and test the pointer for
		   NULL to learn that the input is over, because '' is a chunk
		   of no bytes and nil is the end of the stream.  Deciding that
		   by comparing the result with "" instead confuses the two,
		   and it also mistakes an argument that *is* the empty string
		   for one that was left out. */
		if l != nil {
			n := C.size_t(0)
			if def != nil {
				n = C.strlen(def)
			}
			*l = n
		}
		return def
	}
	return luaL_checklstring(p, arg, l)
}

//export luaL_checktype
func luaL_checktype(p *C.lua_State, arg, t C.int) { stateOf(p).CheckType(int(arg), int(t)) }

//export luaL_checkany
func luaL_checkany(p *C.lua_State, arg C.int) { stateOf(p).CheckAny(int(arg)) }

//export luaL_checkstack
func luaL_checkstack(p *C.lua_State, sz C.int, msg *C.char) {
	stateOf(p).CheckStack2(int(sz), C.GoString(msg))
}

//export luaL_argerror
func luaL_argerror(p *C.lua_State, arg C.int, msg *C.char) C.int {
	stateOf(p).ArgError(int(arg), C.GoString(msg))
	return 0
}

//export luaL_typeerror
func luaL_typeerror(p *C.lua_State, arg C.int, tname *C.char) C.int {
	stateOf(p).TypeError(int(arg), C.GoString(tname))
	return 0
}

//export luaL_setfuncs
func luaL_setfuncs(p *C.lua_State, l *C.luaL_Reg, nup C.int) {
	L := stateOf(p)
	if l == nil {
		return
	}
	n := int(nup)
	var regs []Reg
	for i := 0; ; i++ {
		r := C.cdl_reg_at(l, C.int(i))
		if r == nil || r.name == nil {
			break
		}
		name := C.GoString(r.name)
		/* luaL_Reg's second field is called 'func', which Go cannot
		   spell, so it is read on the C side: cdl_reg_func. */
		fn := unsafe.Pointer(C.cdl_reg_func(r))
		regs = append(regs, Reg{Name: name, Func: cFunc(fn)})
	}
	/* SetFuncs would push these as light C functions, which cannot tell
	   two extension functions apart; see cdlPushCClosure */
	L.checkStack(n)
	for _, r := range regs {
		for i := 0; i < n; i++ { /* copy the upvalues to the top */
			L.PushValue(-n)
		}
		cdlPushCClosure(L, r.Func, n)
		L.SetField(-(n + 2), r.Name)
	}
	L.Pop(n) /* remove the upvalues */
}

//export luaL_getsubtable
func luaL_getsubtable(p *C.lua_State, idx C.int, fname *C.char) C.int {
	return cBool(stateOf(p).GetSubTable(idxInt(idx), C.GoString(fname)))
}

//export luaL_ref
func luaL_ref(p *C.lua_State, t C.int) C.int { return C.int(stateOf(p).Ref(idxInt(t))) }

//export luaL_unref
func luaL_unref(p *C.lua_State, t, ref C.int) { stateOf(p).Unref(idxInt(t), idxInt(ref)) }

//export luaL_len
func luaL_len(p *C.lua_State, idx C.int) C.lua_Integer {
	return C.lua_Integer(stateOf(p).Len2(idxInt(idx)))
}

//export luaL_tolstring
func luaL_tolstring(p *C.lua_State, idx C.int, l *C.size_t) *C.char {
	s := stateOf(p).ToStringX(idxInt(idx))
	if l != nil {
		*l = C.size_t(len(s))
	}
	return C.CString(s)
}

//export luaL_callmeta
func luaL_callmeta(p *C.lua_State, obj C.int, e *C.char) C.int {
	return cBool(stateOf(p).CallMeta(idxInt(obj), C.GoString(e)))
}

//export luaL_getmetafield
func luaL_getmetafield(p *C.lua_State, obj C.int, e *C.char) C.int {
	return cBool(stateOf(p).GetMetaField(idxInt(obj), C.GoString(e)) != LUA_TNIL)
}

//export luaL_where
func luaL_where(p *C.lua_State, lvl C.int) { stateOf(p).Where2(int(lvl)) }

//export luaL_loadstring
func luaL_loadstring(p *C.lua_State, s *C.char) C.int {
	return C.int(cdlStatus(stateOf(p).LoadString(C.GoString(s), C.GoString(s))))
}

//export luaL_loadbufferx
func luaL_loadbufferx(p *C.lua_State, buff *C.char, sz C.size_t, name, mode *C.char) C.int {
	m := ""
	if mode != nil {
		m = C.GoString(mode)
	}
	return C.int(cdlStatus(stateOf(p).LoadBytesMode([]byte(C.GoStringN(buff, C.int(sz))), C.GoString(name), m)))
}

//export luaL_traceback
func luaL_traceback(p *C.lua_State, l1 *C.lua_State, msg *C.char, level C.int) {
	L := stateOf(p)
	var from *State
	if l1 != nil {
		from = stateOf(l1)
	}
	m := ""
	if msg != nil {
		m = C.GoString(msg)
		L.Traceback(from, m, true, int(level))
		return
	}
	L.Traceback(from, "", false, int(level))
}

//export luaL_requiref
func luaL_requiref(p *C.lua_State, name *C.char, openf C.lua_CFunction, glb C.int) {
	L := stateOf(p)
	var f GoFunc
	if openf != nil {
		f = cFunc(unsafe.Pointer(openf))
	}
	L.RequireF(C.GoString(name), f, glb != 0)
}

// ---------------------------------------------------------------------
// the buffer API
// ---------------------------------------------------------------------

var (
	cgoBufMu   sync.Mutex
	cgoBufs    = map[int]*Buffer{}
	cgoBufNext int
)

//export luaL_buffinit
func luaL_buffinit(p *C.lua_State, b *C.luaL_Buffer) {
	if b == nil {
		return
	}
	L := stateOf(p)
	buf := L.NewBuffer()
	if buf.scratch == nil {
		/* The block behind the buffer: luaL_prepbuffsize hands out its
		   address and the extension copies into it directly, so it has
		   to be C memory -- a pointer into Go memory is both against
		   the cgo rules and into memory the collector may move. */
		buf.scratch = C.malloc(C.size_t(C.CDL_BUFFERSIZE))
		buf.slen = int(C.CDL_BUFFERSIZE)
		runtime.SetFinalizer(buf, func(x *Buffer) {
			if x.scratch != nil {
				C.free(x.scratch)
				x.scratch = nil
			}
		})
	}
	cgoBufMu.Lock()
	cgoBufNext++
	id := cgoBufNext
	cgoBufs[id] = buf
	cgoBufMu.Unlock()
	/* the visible fields are the extension's to read, and 'ref' lives at
	   the end of the struct that extension declared, so both are written
	   through C rather than by a Go field access */
	C.cdl_buf_init(b, p, (*C.char)(buf.scratch), C.size_t(buf.slen))
	*C.cdl_buf_refp(b) = C.int(id)
}

// bufFlush takes what an extension has written straight into the block
// luaL_prepbuffsize gave it -- luaL_addchar does exactly that, and never
// calls anything -- and adds it to the buffer in the order it was
// written. It runs before anything else touches the buffer, so that
// writes through the pointer and writes through luaL_addlstring come out
// in the order the extension made them.
func bufFlush(b *C.luaL_Buffer, buf *Buffer) {
	if b == nil || buf.scratch == nil {
		return
	}
	if n := int(b.n); n > 0 {
		buf.AddBytes(unsafe.Slice((*byte)(buf.scratch), n))
	}
	b.n = 0
}

//export luaL_prepbuffsize
func luaL_prepbuffsize(b *C.luaL_Buffer, sz C.size_t) *C.char {
	buf := bufOf(b)
	if buf == nil || buf.scratch == nil {
		return nil
	}
	if int(b.n)+int(sz) > int(b.size) {
		bufFlush(b, buf) /* start the block over */
	}
	if int(sz) > buf.slen {
		n := int(sz) * 2
		p := C.realloc(buf.scratch, C.size_t(n))
		if p == nil {
			return nil
		}
		buf.scratch = p
		buf.slen = n
		b.b = (*C.char)(p)
		b.size = C.size_t(n)
	}
	return (*C.char)(unsafe.Add(buf.scratch, uintptr(b.n)))
}

func bufOf(b *C.luaL_Buffer) *Buffer {
	if b == nil {
		return nil
	}
	cgoBufMu.Lock()
	buf := cgoBufs[int(*C.cdl_buf_refp(b))]
	cgoBufMu.Unlock()
	return buf
}

//export luaL_addchar_
func luaL_addchar_(b *C.luaL_Buffer, c C.char) {
	if buf := bufOf(b); buf != nil {
		buf.AddChar(byte(c))
	}
}

//export luaL_addlstring
func luaL_addlstring(b *C.luaL_Buffer, s *C.char, l C.size_t) {
	if buf := bufOf(b); buf != nil {
		bufFlush(b, buf)
		buf.AddBytes([]byte(C.GoStringN(s, C.int(l))))
	}
}

//export luaL_addstring
func luaL_addstring(b *C.luaL_Buffer, s *C.char) {
	if buf := bufOf(b); buf != nil && s != nil {
		bufFlush(b, buf)
		buf.AddString(C.GoString(s))
	}
}

//export luaL_addvalue
func luaL_addvalue(b *C.luaL_Buffer) {
	if buf := bufOf(b); buf != nil {
		bufFlush(b, buf)
		buf.AddValue()
	}
}

//export luaL_pushresult
func luaL_pushresult(b *C.luaL_Buffer) {
	buf := bufOf(b)
	if buf == nil {
		return
	}
	L := buf.L
	/* PUC-Lua's luaL_buffinit leaves a placeholder on the stack, and
	   luaL_pushresult replaces it: push the string, then remove what is
	   now underneath it. One value appears, which is what an extension
	   counts -- lpeg answers "1" for a substitution capture and pushes
	   exactly one value, so anything left behind shifts the stack under
	   the lua_call that follows. */
	bufFlush(b, buf)
	buf.PushResult()
	/* PUC-Lua keeps the placeholder directly under the top and removes
	   it from there. Not from a remembered index: an extension may
	   lua_remove something below it -- lpeg does, discarding old
	   dynamic captures -- and an absolute index would then be off by
	   one, leaving the placeholder behind and the stack one deep for
	   the lua_call that follows. Remove it only if it is still there,
	   so that a buffer used some other way cannot lose a value. */
	if buf.box != nil && L.top >= 2 {
		if v := &L.stack[L.top-2]; ttisfulluserdata(v) && uvalue(v) == buf.box {
			L.Remove(-2)
		}
	}
}

//export luaL_pushresultsize
func luaL_pushresultsize(b *C.luaL_Buffer, sz C.size_t) { luaL_pushresult(b) }

// ---------------------------------------------------------------------
// lsys_load / lsys_sym: what loadlib.go calls
// ---------------------------------------------------------------------

// cdlHandles remembers the dlopen handles so a library is never unloaded
// while its functions are still reachable.
var cdlHandles sync.Map // path -> unsafe.Pointer

// cgoLoadSym is lsys_load and lsys_sym together.  It opens 'path', finds
// 'sym' in it, and leaves the function on the stack; with sym == "*" the
// library is only opened and true is left instead, as in loadlib.c.
//
// It answers pkErrLib when the library will not open and pkErrFunc when
// the symbol is not in it, and in both cases leaves dlerror's message --
// which is what PUC-Lua pushes, so the two agree word for word.
func cgoLoadSym(L *State, path, sym string) int {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	h := C.cdl_dlopen(cpath)
	if h == nil {
		L.PushString(cdlErrMsg())
		return pkErrLib
	}
	cdlHandles.Store(path, unsafe.Pointer(h))
	if sym == "*" { /* loading only the library (no function)? */
		L.PushBoolean(true)
		return 0
	}
	csym := C.CString(sym)
	defer C.free(unsafe.Pointer(csym))
	fp := C.cdl_sym(h, csym)
	if fp == nil {
		L.PushString(cdlErrMsg())
		return pkErrFunc
	}
	L.PushGoFunc(cFunc(fp))
	return 0
}

// cdlErrMsg is the last dlopen/dlsym message, which is what loadlib.c
// puts in the error: "dlopen(x, 0x0006): tried: ...".
func cdlErrMsg() string {
	if msg := C.cdl_error(); msg != nil {
		return C.GoString(msg)
	}
	return "unknown dynamic loading error"
}

// init puts the loader where loadlib.go looks for it.  With cgo -- which
// this whole file requires -- an extension resolves lua_* and luaL_*
// against this binary, so require can find a C module in cpath.
func init() {
	pkDynLoadSym = cgoLoadSym
	releaseCState = cgoReleaseState
}
