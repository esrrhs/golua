//go:build cgo && unix

// The dynamic-loading backend: what lets this port call a C extension.
//
// A Lua extension is a shared object exporting luaopen_<name> that calls
// the Lua C API.  On Unix it is built *without* linking liblua, and the
// loader resolves lua_* and luaL_* from the host binary -- so the host has
// to export them, and a Go binary can, but "can" takes a different amount
// of help on each platform:
//
//   - macOS exports every global symbol of an executable by default, and
//     the extension is compiled with -undefined dynamic_lookup so the
//     names stay unresolved until the loader finds them.  Nothing to do.
//
//   - ELF exports nothing an executable does not ask for, and asking is
//     the linker's --export-dynamic.  Go's own linker adds
//     --export-dynamic-symbol=<name> for each //export-ed Go function,
//     which is most of the API, but not for the four that are C: see
//     cdlbridge.c, and the #cgo LDFLAGS below.  Without it a PUC-Lua
//     shared object stops at "undefined symbol: lua_pushfstring".
//
// So the names below are the real lua_* and luaL_* names, and each is a
// thin forwarder onto the same State methods the rest of the port uses --
// nothing here reimplements anything.
//
// Two details shape the file:
//
//   - lua_State is an opaque handle C never dereferences: the address
//     of a malloc'd word holding an integer.  That integer names the
//     state in a weak registry.  No Go pointer lives in C memory (which
//     cgo forbids), and encoding the id in the pointer itself would
//     mean turning a uintptr into a pointer, which is the one thing
//     go vet will not let past.  A cgo.Handle would pin the state for
//     the life of the block, so a coroutine C had only looked at could
//     never be collected.
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
package core

/*
// The ELF half of "the host has to export lua_*": ask the linker to put
// *every* global symbol of the executable into its dynamic table, which
// is what dlopen searches.  A //export-ed Go function already gets there
// (Go's linker adds one --export-dynamic-symbol per name), but the four
// variadic functions are C -- see cdlbridge.c -- and nothing exports a
// plain C symbol of an executable without this.
//
// It is a link-time property of the final binary, so it travels with the
// package: #cgo LDFLAGS of an imported package reach the link of
// anything built against it (//go:cgo_ldflag).  Which is the point --
// the promise is that a .so written for PUC-Lua loads as it is, and it
// cannot be kept by asking whoever embeds this library to remember a
// flag.  macOS needs nothing, and -undefined dynamic_lookup is what the
// extension's own build carries there.
// -ldl is required on glibc before 2.34, where dlopen/dlsym/dlerror live
// in libdl rather than libc. Newer glibc keeps libdl as an empty stub,
// so the flag is harmless there.
#cgo linux LDFLAGS: -Wl,--export-dynamic -ldl

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
	"fmt"
	"runtime"
	"sync"
	"unsafe"
)

// ---------------------------------------------------------------------
// lua_State as an opaque handle
// ---------------------------------------------------------------------

// A `lua_State *` has to be an address C can hold, even though C never
// dereferences it: cgo will not let a Go pointer be left in C memory,
// and the state is not a C object to begin with. So each state gets one
// malloc'd word holding an integer that names it in a weak registry,
// and that address is what C carries around. Making the number up out
// of a Go address would answer the same question, but only by converting
// a uintptr to a pointer, which is exactly what the rules (and go vet)
// forbid. The registry does not keep the state alive: when Go drops it
// the block is freed, and Close frees every thread's block at once.
// luaExtraSpace is LUA_EXTRASPACE (luaconf.h): the bytes PUC-Lua keeps in
// front of every lua_State, which 'lua_getextraspace' hands to C.
const luaExtraSpace = unsafe.Sizeof(uintptr(0))

func (L *State) statePtr() *C.lua_State {
	if L.cstate == nil {
		/* The block is LUA_EXTRASPACE bytes followed by the id:
		   'lua_getextraspace' hands C the bytes *before* the pointer it
		   was given, and PUC-Lua really does keep them there, so they
		   have to exist and be addressable. */
		size := unsafe.Sizeof(uintptr(0))
		base := C.malloc(C.size_t(luaExtraSpace) + C.size_t(size))
		if base == nil {
			panic("lua: out of memory for a lua_State handle")
		}
		C.memset(base, 0, C.size_t(luaExtraSpace)+C.size_t(size))
		block := unsafe.Pointer(uintptr(base) + luaExtraSpace)
		tok := &cstateBlock{base: base}
		tok.id = registerState(L)
		*(*uintptr)(block) = tok.id
		L.cstate = block
		L.cstateBase = base
		L.cstateTok = tok
		cstateLive.Add(1)
		L.g.trackCState(tok)
		runtime.AddCleanup(L, func(t *cstateBlock) { t.release() }, tok)
	}
	return (*C.lua_State)(L.cstate)
}

func stateOf(p *C.lua_State) *State {
	if p == nil {
		panic("lua: C handed back a null lua_State")
	}
	L := stateByID(*(*uintptr)(unsafe.Pointer(p)))
	if L == nil {
		panic("lua: C handed back an unknown lua_State")
	}
	return L
}

// cgoReleaseState gives back every lua_State block of this global state.
// It is called from State.Close, which is in a file built either way,
// hence the indirection: without cgo there is no block and nothing to
// give back. Child threads are included: Close only runs on the main
// thread, and a thread C was shown would otherwise keep its block.
func cgoReleaseState(L *State) {
	if L == nil || L.g == nil {
		return
	}
	L.g.releaseCStates()
	releaseCgoBufsOf(L.g)
}

// cFunc wraps a C function pointer as the GoFunc the VM calls: the VM
// sees an ordinary closure, and the closure crosses into C.
func cFunc(f unsafe.Pointer) GoFunc {
	return func(L *State) int {
		p := L.statePtr()
		cgoEnter()
		defer func() {
			cgoLeave()
			runtime.KeepAlive(L)
		}()
		return int(C.cdl_call_cfunction(C.lua_CFunction(f), p))
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
	stateOf(p).CloseSlot(idxInt(idx))
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
func lua_typename(p *C.lua_State, tp C.int) *C.char { return internCStr(TypeName(int(tp))) }

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
// the message it goes into. It is not freed from the finalizer itself:
// Go can collect the string while a .so is still inside a cgo call, and
// C.free at that moment is a use-after-free. The finalizer only detaches
// the pointer; cgoFree returns it once no such call is in progress.
func tsCStr(ts *TString) unsafe.Pointer {
	p := unsafe.Pointer(C.CString(ts.val))
	cstrMu.Lock()
	if ts.cstr != nil {
		old := ts.cstr
		cstrMu.Unlock()
		C.free(p)
		return old
	}
	ts.cstr = p
	cstrMu.Unlock()
	runtime.SetFinalizer(ts, freeTStringC)
	return p
}

func freeTStringC(t *TString) {
	cstrMu.Lock()
	p := t.cstr
	t.cstr = nil
	cstrMu.Unlock()
	if p == nil {
		return
	}
	cstrDetached.Add(1)
	cgoFree(p)
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

// pushedCStr is the internal copy of the string just pushed. PUC-Lua
// returns that, not the caller's buffer: the caller may free its own
// memory and keep the pointer.
func pushedCStr(L *State) *C.char {
	v := index2value(L, -1)
	if v == nil || !ttisstring(v) {
		return nil
	}
	return (*C.char)(tsCStr(tsvalue(v)))
}

//export lua_pushlstring
func lua_pushlstring(p *C.lua_State, s *C.char, l C.size_t) *C.char {
	L := stateOf(p)
	if s == nil || l == 0 {
		L.PushString("")
		return pushedCStr(L)
	}
	L.PushBytes([]byte(C.GoStringN(s, C.int(l))))
	return pushedCStr(L)
}

//export lua_pushstring
func lua_pushstring(p *C.lua_State, s *C.char) *C.char {
	if s == nil {
		stateOf(p).PushNil()
		return nil
	}
	L := stateOf(p)
	L.PushString(C.GoString(s))
	return pushedCStr(L)
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

// C's never returns: it calls the message handler, then longjmps to the
// innermost *protected* setjmp -- not the innermost C frame -- and the
// extension's own frames are abandoned without running.  What that
// means for an extension, and all it means, is that no C code of its
// runs on the way out.
//
// This raises instead, and the raise is what the innermost PCall
// recovers.  The extension's frames are abandoned here too: Go's
// runtime does not try to unwind them, it unwinds the g0 stack the
// callback was running on -- which is where those frames live -- and
// gives the stack pointer back (runtime.unwindm).  So an extension
// cannot tell the two apart: the same frames are left, the same C code
// is skipped, and the stack comes back either way.
//
// A longjmp would buy nothing observable and would cost a setjmp in
// every call to C, a chain of jump buffers per thread, and a rule for
// which frames to cross -- one that has to know where Go's protection
// boundaries are, because the target is the innermost pcall and not the
// innermost C frame, and the two are not the same whenever an extension
// calls lua_pcall itself.  TestCModuleErrorStress is what keeps the one
// failure mode that matters honest: if abandoning those frames ever
// stopped giving the C stack back, 30 000 raises would run out of it.
//
//export lua_error
func lua_error(p *C.lua_State) C.int {
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
		cgoEnter()
		defer cgoLeave()
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
	/* luaL_checklstring is lua_tolstring plus a type error. The pointer
	   is the string's own copy, so it stays valid for as long as the
	   value does. */
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
	L := stateOf(p)
	i := idxInt(idx)
	/* PUC-Lua leaves the string on the stack and answers a pointer into
	   it. That is not a detail: 'luaL_tolstring' is how an extension
	   spells 'tostring' in the middle of building a stack, and code
	   that pops what it pushed afterwards is the ordinary case --
	   lauxlib's own luaL_traceback does exactly that. Answering only a
	   pointer leaves the stack one short of what the caller expects,
	   and the next absolute index names the wrong value. */
	if !L.CallMeta(i, "__tostring") {
		switch L.Type(i) {
		case LUA_TNUMBER:
			s := L.ToStringX(i) /* as '%I' or '%f' would print it */
			L.PushString(s)
		case LUA_TSTRING:
			L.PushValue(i)
		case LUA_TBOOLEAN:
			if L.ToBoolean(i) {
				L.PushString("true")
			} else {
				L.PushString("false")
			}
		case LUA_TNIL:
			L.PushString("nil")
		default: /* as '%s: %p', with the '__name' metafield if there is one */
			kind := L.TypeName(i)
			tt := L.GetMetaField(i, "__name")
			if tt == LUA_TSTRING {
				if s, ok := L.ToString(-1); ok {
					kind = luaCStr(s) /* '%s' stops at the zero */
				}
			}
			L.PushString(fmt.Sprintf("%s: %p", kind, lua_topointer(p, idx)))
			if tt != LUA_TNIL {
				L.Remove(-2) /* remove '__name' */
			}
		}
	}
	return lua_tolstring(p, -1, l)
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

// The block behind a string buffer is C memory: luaL_prepbuffsize hands
// out its address and the extension writes into it after the call has
// returned, which rules out Go memory both by the cgo rules and because
// the collector may move it. These two are how lauxlib.go asks for one,
// having no C of its own.
func init() {
	reallocBlock = func(old unsafe.Pointer, sz int) unsafe.Pointer {
		if sz == 0 {
			if old != nil {
				C.free(old)
			}
			return nil
		}
		return C.realloc(old, C.size_t(sz)) /* nil when it cannot grow */
	}
	bufFree = func(b *Buffer) {
		if b.scratch != nil {
			p := b.scratch
			b.scratch = nil
			b.slen = 0
			cgoFree(p)
		}
	}
	releaseUdataBuf = releaseUdataBufID
}

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
	buf.cblock = true /* C is about to be handed the address */
	/* Make sure there is a block from the start: 'b' and 'size' are the
	   buffer as far as an extension is concerned, and luaL_addchar is a
	   macro that writes into 'b[n]' once it has checked 'n < size'. */
	buf.room(0)
	cgoBufMu.Lock()
	cgoBufNext++
	id := cgoBufNext
	cgoBufs[id] = buf
	cgoBufMu.Unlock()
	/* The box is what keeps the buffer reachable while C uses it, and
	   what releases it once that userdata is gone. A pointer from the
	   buffer back to the box would pin both forever. */
	if L.top > 0 && ttisfulluserdata(&L.stack[L.top-1]) {
		u := uvalue(&L.stack[L.top-1])
		u.bufID = id
		setUdataFinalizer(L, u)
	}
	/* the visible fields are the extension's to read, and 'ref' lives at
	   the end of the struct that extension declared, so both are written
	   through C rather than by a Go field access */
	C.cdl_buf_init(b, p, (*C.char)(buf.scratch), C.size_t(buf.slen))
	*C.cdl_buf_refp(b) = C.int(id)
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

// bufEnter picks up what the extension did to the visible fields itself.
// luaL_addsize and luaL_buffsub are macros over 'n', and luaL_addchar's
// writes into 'b[n++]' without calling anything, so 'n' is not ours to
// keep: it is read here and written back by bufLeave.
func bufEnter(b *C.luaL_Buffer) *Buffer {
	buf := bufOf(b)
	if buf != nil {
		buf.n = int(b.n)
	}
	return buf
}

func bufLeave(b *C.luaL_Buffer, buf *Buffer) {
	if buf == nil {
		return
	}
	b.n = C.size_t(buf.n)
	b.b = (*C.char)(buf.scratch)
	b.size = C.size_t(buf.slen)
}

//export luaL_prepbuffsize
func luaL_prepbuffsize(b *C.luaL_Buffer, sz C.size_t) *C.char {
	buf := bufEnter(b)
	if buf == nil {
		return nil
	}
	buf.room(int(sz)) /* the room, made if the block is too small */
	bufLeave(b, buf)
	if b.b == nil {
		return nil
	}
	return (*C.char)(unsafe.Add(unsafe.Pointer(b.b), uintptr(b.n)))
}

//export luaL_addchar_
func luaL_addchar_(b *C.luaL_Buffer, c C.char) {
	buf := bufEnter(b)
	if buf == nil {
		return
	}
	buf.AddChar(byte(c))
	bufLeave(b, buf)
}

//export luaL_addlstring
func luaL_addlstring(b *C.luaL_Buffer, s *C.char, l C.size_t) {
	buf := bufEnter(b)
	if buf == nil || s == nil || l == 0 {
		return
	}
	buf.AddBytes(C.GoBytes(unsafe.Pointer(s), C.int(l)))
	bufLeave(b, buf)
}

//export luaL_addstring
func luaL_addstring(b *C.luaL_Buffer, s *C.char) {
	buf := bufEnter(b)
	if buf == nil || s == nil {
		return
	}
	buf.AddString(C.GoString(s))
	bufLeave(b, buf)
}

//export luaL_addvalue
func luaL_addvalue(b *C.luaL_Buffer) {
	buf := bufEnter(b)
	if buf == nil {
		return
	}
	buf.AddValue()
	bufLeave(b, buf)
}

//export luaL_pushresult
func luaL_pushresult(b *C.luaL_Buffer) {
	buf := bufEnter(b)
	if buf == nil {
		return
	}
	L := buf.L
	/* PUC-Lua pushes 'B->b[0 .. B->n]' and takes the buffer's box off the
	   stack: one value appears where one value was. */
	L.PushString(buf.String())
	buf.n = 0
	/* PUC-Lua keeps the box directly under the top and removes it from
	   there. Not from a remembered index: an extension may lua_remove
	   something below it -- lpeg does, discarding old dynamic captures
	   -- and an absolute index would then be off by one, leaving the box
	   behind and the stack one deep for the lua_call that follows.
	   Remove it only if it is still there, so that a buffer used some
	   other way cannot lose a value. */
	if buf.boxPtr != 0 && L.top >= 2 {
		if v := &L.stack[L.top-2]; ttisfulluserdata(v) && uintptr(unsafe.Pointer(uvalue(v))) == buf.boxPtr {
			L.Remove(-2)
		}
	}
	bufLeave(b, buf)
	if b != nil {
		id := int(*C.cdl_buf_refp(b))
		*C.cdl_buf_refp(b) = 0
		releaseCgoBuf(id)
	}
}

// releaseCgoBuf drops one buffer and gives its block back. Calling it
// twice is harmless: the second time the id is already gone.
func releaseCgoBuf(id int) {
	if id == 0 {
		return
	}
	cgoBufMu.Lock()
	buf := cgoBufs[id]
	delete(cgoBufs, id)
	cgoBufMu.Unlock()
	if buf != nil && bufFree != nil {
		bufFree(buf)
	}
}

func releaseUdataBufID(u *Udata) {
	if u == nil || u.bufID == 0 {
		return
	}
	id := u.bufID
	u.bufID = 0
	releaseCgoBuf(id)
}

// releaseCgoBufsOf gives back the buffers that belong to g. Close uses
// it so a buffer whose box never became unreachable does not outlive
// the state.
func releaseCgoBufsOf(g *globalState) {
	cgoBufMu.Lock()
	var bufs []*Buffer
	for id, buf := range cgoBufs {
		if buf != nil && buf.L != nil && buf.L.g == g {
			delete(cgoBufs, id)
			bufs = append(bufs, buf)
		}
	}
	cgoBufMu.Unlock()
	for _, buf := range bufs {
		if bufFree != nil {
			bufFree(buf)
		}
	}
}

func cgoBufCount() int {
	cgoBufMu.Lock()
	n := len(cgoBufs)
	cgoBufMu.Unlock()
	return n
}

//export luaL_pushresultsize
func luaL_pushresultsize(b *C.luaL_Buffer, sz C.size_t) {
	/* PUC-Lua's is luaL_addsize followed by luaL_pushresult, and the size
	   is the part an extension relies on: luaL_buffinitsize hands out
	   room, the extension fills it directly, and this is where it says
	   how much it wrote. */
	if b != nil {
		b.n += sz
	}
	luaL_pushresult(b)
}

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
	/* '*' means "load only the library", and that is the one case in
	   which loadlib.c opens it RTLD_GLOBAL: the symbols are meant to be
	   visible to libraries loaded afterwards. */
	global := sym == "*"
	var h unsafe.Pointer
	if global {
		h = unsafe.Pointer(C.cdl_dlopen_global(cpath))
	} else {
		h = unsafe.Pointer(C.cdl_dlopen(cpath))
	}
	if h == nil {
		L.PushString(cdlErrMsg())
		return pkErrLib
	}
	cdlHandles.Store(path, h)
	if global {
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
