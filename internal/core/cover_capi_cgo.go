//go:build cgo && unix

package core

/*
#include "cdlbridge.h"
#include <string.h>

// coverTestReader feeds one chunk then EOF for lua_load's non-nil reader arm.
static const char *coverTestReader(lua_State *L, void *data, size_t *size) {
	const char **pp = (const char **)data;
	(void)L;
	if (pp == NULL || *pp == NULL) {
		*size = 0;
		return NULL;
	}
	*size = strlen(*pp);
	const char *r = *pp;
	*pp = NULL;
	return r;
}

lua_Reader cover_test_reader(void) { return coverTestReader; }
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

// capiFailAt forces the Nth capiCheck to fail when >= 0 (test-only).
var capiFailAt = -1
var capiCheckN int

func capiResetChecks() {
	capiCheckN = 0
	capiFailAt = -1
}

func capiHasChecks() bool { return true }

// capiCheck reports an error when ok is false. Tests set capiFailAt to
// exercise every return arm without corrupting the happy path.
func capiCheck(ok bool, format string, args ...any) error {
	n := capiCheckN
	capiCheckN++
	if capiFailAt == n {
		ok = false
	}
	if !ok {
		return fmt.Errorf(format, args...)
	}
	return nil
}

// exerciseCAPI calls the C entry points an extension would call. They
// live in this file, not a _test.go, because cgo is not allowed in tests.
func exerciseCAPI(dir string) error {
	capiCheckN = 0
	L := NewState()
	defer L.Close()
	p := L.statePtr()
	var pinned []*C.char
	cs := func(s string) *C.char {
		q := C.CString(s)
		pinned = append(pinned, q)
		return q
	}
	defer func() {
		for _, q := range pinned {
			C.free(unsafe.Pointer(q))
		}
	}()

	luaL_checkversion_(p, C.lua_Number(luaVersionNum), C.size_t(luaLNumSizes))
	if err := capiCheck(!(lua_version(p) == 0 || lua_newstate(nil, nil) != nil || luaL_newstate() != nil), "version / newstate"); err != nil {
		return err
	}
	luaL_openlibs(p)
	if err := capiCheck(lua_gettop(p) == 0, "openlibs left %d", lua_gettop(p)); err != nil {
		return err
	}

	lua_pushinteger(p, 11)
	lua_pushinteger(p, 22)
	lua_pushstring(p, cs("hi"))
	if err := capiCheck(C.GoString(lua_tostring(p, -1)) == "hi", "tostring"); err != nil {
		return err
	}
	lua_pop(p, 1)
	lua_insert(p, 1)
	lua_remove(p, 1)
	if err := capiCheck(!(lua_tointeger(p, -1) != 11 || lua_tonumber(p, -1) != 11), "pop/insert/remove"); err != nil {
		return err
	}
	lua_settop(p, 0)

	lua_pushglobaltable(p)
	lua_newtable(p)
	if err := capiCheck(lua_type(p, -1) == LUA_TTABLE, "newtable"); err != nil {
		return err
	}
	var key int
	lua_pushinteger(p, 5)
	lua_rawsetp(p, -2, unsafe.Pointer(&key))
	if err := capiCheck(!(lua_rawgetp(p, -1, unsafe.Pointer(&key)) != LUA_TNUMBER || lua_tointeger(p, -1) != 5), "rawgetp"); err != nil {
		return err
	}
	lua_pop(p, 1)
	if err := capiCheck(lua_topointer(p, -1) != nil, "topointer"); err != nil {
		return err
	}
	if err := capiCheck(lua_newuserdata(p, 8) != nil, "newuserdata"); err != nil {
		return err
	}
	lua_settop(p, 0)

	src, name, mode := cs("return 7"), cs("=capi"), cs("t")
	if err := capiCheck(lua_load(p, nil, unsafe.Pointer(src), name, mode) == LUA_OK, "load: %s", C.GoString(lua_tostring(p, -1))); err != nil {
		return err
	}
	lua_call(p, 0, 1)
	if err := capiCheck(lua_tointeger(p, -1) == 7, "loaded chunk"); err != nil {
		return err
	}
	lua_pop(p, 1)

	path := filepath.Join(dir, "c.lua")
	writeErr := os.WriteFile(path, []byte("return 8"), 0o644)
	if err := capiCheck(writeErr == nil, "writefile"); err != nil {
		return err
	}
	if err := capiCheck(luaL_loadfile(p, cs(path)) == LUA_OK, "loadfile: %s", C.GoString(lua_tostring(p, -1))); err != nil {
		return err
	}
	lua_pop(p, 1)
	if err := capiCheck(luaL_loadfilex(p, cs(path), mode) == LUA_OK, "loadfilex"); err != nil {
		return err
	}
	lua_pop(p, 1)
	if err := capiCheck(luaL_loadfile(p, cs(filepath.Join(dir, "missing.lua"))) != LUA_OK, "missing file loaded"); err != nil {
		return err
	}
	/* the loader returns the error instead of pushing it */
	lua_settop(p, 0)

	if err := capiCheck(luaL_optnumber(p, 1, 4.5) == 4.5, "optnumber default %v", luaL_optnumber(p, 1, 4.5)); err != nil {
		return err
	}
	lua_pushinteger(p, 3)
	if err := capiCheck(!(lua_gettop(p) != 1 || luaL_optnumber(p, 1, 0) != 3), "optnumber arg top=%d val=%v", lua_gettop(p), luaL_optnumber(p, 1, 0)); err != nil {
		return err
	}
	lua_settop(p, 0)

	lua_newtable(p)
	lua_newtable(p)
	if err := capiCheck(lua_load(p, nil, unsafe.Pointer(cs("return function() return 'yes' end")), name, mode) == LUA_OK, "meta function"); err != nil {
		return err
	}
	lua_call(p, 0, 1)
	lua_setfield(p, -2, cs("__tostring"))
	lua_pushstring(p, cs("yes"))
	lua_setfield(p, -2, cs("__name"))
	lua_setmetatable(p, -2)
	if err := capiCheck(luaL_getmetafield(p, -1, cs("__name")) != 0, "getmetafield"); err != nil {
		return err
	}
	if err := capiCheck(C.GoString(lua_tostring(p, -1)) == "yes", "metafield value"); err != nil {
		return err
	}
	lua_pop(p, 1)
	if err := capiCheck(luaL_callmeta(p, -1, cs("__tostring")) != 0, "callmeta"); err != nil {
		return err
	}
	if err := capiCheck(C.GoString(lua_tostring(p, -1)) == "yes", "callmeta result %q", C.GoString(lua_tostring(p, -1))); err != nil {
		return err
	}
	lua_settop(p, 0)

	tn := lua_typename(p, LUA_TSTRING)
	if err := capiCheck(C.GoString(tn) == "string", "typename"); err != nil {
		return err
	}
	/* The pointer is a stable interned literal, the same one PUC-Lua
	   returns. Freeing it would corrupt the next lua_typename. */
	if err := capiCheck(lua_typename(p, LUA_TSTRING) == tn, "typename not stable"); err != nil {
		return err
	}

	lua_pushstring(p, cs("  42 "))
	if err := capiCheck(!(lua_stringtonumber(p, cs("  42 ")) == 0 || lua_tointeger(p, -1) != 42), "stringtonumber"); err != nil {
		return err
	}
	lua_pop(p, 2)

	if err := capiCheck(!(luaL_execresult(p, 1) != 1 || lua_toboolean(p, -1) == 0), "execresult"); err != nil {
		return err
	}
	lua_pop(p, 1)

	nameMT := cs("COVER_MT")
	if err := capiCheck(luaL_newmetatable(p, nameMT) == 1, "newmetatable"); err != nil {
		return err
	}
	lua_pop(p, 1)
	lua_newuserdata(p, 4)
	luaL_setmetatable(p, nameMT)
	if err := capiCheck(luaL_testudata(p, -1, nameMT) != nil, "testudata"); err != nil {
		return err
	}
	luaL_getmetatable(p, nameMT)
	if err := capiCheck(lua_type(p, -1) == LUA_TTABLE, "getmetatable"); err != nil {
		return err
	}
	lua_settop(p, 0)

	if err := capiCheck(lua_pushthread(p) != 0, "pushthread on the main state"); err != nil {
		return err
	}
	lua_pop(p, 1)
	th := lua_newthread(p)
	if err := capiCheck(lua_tothread(p, -1) == th, "tothread"); err != nil {
		return err
	}
	if err := capiCheck(lua_closethread(th, p) >= 0, "closethread"); err != nil {
		return err
	}
	lua_pop(p, 1)

	lua_getglobal(p, cs("type"))
	_ = lua_tocfunction(p, -1)
	if err := capiCheck(lua_type(p, -1) == LUA_TFUNCTION, "type is not a function"); err != nil {
		return err
	}
	lua_pop(p, 1)

	if err := capiCheck(lua_load(p, nil, unsafe.Pointer(cs("return function(x) return x end")), name, mode) == LUA_OK, "load closure"); err != nil {
		return err
	}
	lua_call(p, 0, 1)
	if err := capiCheck(lua_dump(p, nil, nil, 0) == 1, "dump with no writer"); err != nil {
		return err
	}
	lua_pop(p, 1)

	lua_setwarnf(p, nil, nil)
	if err := capiCheck(lua_gethook(p) == nil, "gethook"); err != nil {
		return err
	}
	_ = lua_gethookmask(p)
	_ = lua_gethookcount(p)
	lua_setallocf(p, C.lua_Alloc(C.cdl_allocfn()), nil)
	if err := capiCheck(lua_getallocf(p, nil) != nil, "allocf"); err != nil {
		return err
	}

	for _, what := range []C.int{
		luaGCStop, luaGCRestart, luaGCCollect, luaGCCount, luaGCCountB,
		luaGCStep, luaGCSetPause, luaGCSetStepMul, luaGCIsRunning, luaGCGen, luaGCInc,
	} {
		cdl_gc_cb(p, what, 200, 0, 0)
	}

	fresh := NewState()
	fp := fresh.statePtr()
	if err := capiCheck(!(luaopen_base(fp) == 0 || luaopen_coroutine(fp) == 0 || luaopen_table(fp) == 0 ||
		luaopen_io(fp) == 0 || luaopen_os(fp) == 0 || luaopen_string(fp) == 0 ||
		luaopen_utf8(fp) == 0 || luaopen_math(fp) == 0 || luaopen_debug(fp) == 0 ||
		luaopen_package(fp) == 0), "luaopen"); err != nil {
		lua_close(fp)
		return err
	}
	lua_close(fp)

	if err := capiCheck(capiPanics(func() { lua_yield(p, 0) }), "yield outside a coroutine did not fail"); err != nil {
		return err
	}
	if err := capiCheck(capiPanics(func() { cdl_runerror_cb(p, cs("nope")) }), "runerror did not fail"); err != nil {
		return err
	}

	/* Extra C API surface that earlier cases skipped. */
	lua_settop(p, 0)
	lua_pushnil(p)
	lua_pushboolean(p, 1)
	lua_pushnumber(p, 1.25)
	lua_pushlstring(p, cs("ab"), 2)
	var isnum C.int
	_ = lua_tonumberx(p, -2, &isnum)
	_ = lua_tointegerx(p, -2, &isnum)
	_ = lua_isnil(p, 1)
	_ = lua_isboolean(p, 2)
	_ = lua_isnumber(p, 3)
	_ = lua_isinteger(p, 3)
	_ = lua_isstring(p, -1)
	_ = lua_isnone(p, 99)
	_ = lua_isnoneornil(p, 1)
	_ = lua_rawequal(p, 2, 2)
	_ = lua_absindex(p, -1)
	_ = lua_checkstack(p, 8)
	lua_pushvalue(p, -1)
	lua_rotate(p, 1, 1)
	lua_copy(p, -1, 1)
	var len C.size_t
	_ = lua_tolstring(p, -1, &len)
	_ = lua_rawlen(p, -1)
	lua_settop(p, 0)

	lua_createtable(p, 2, 2)
	lua_pushinteger(p, 1)
	lua_seti(p, -2, 1)
	lua_geti(p, -1, 1)
	lua_pop(p, 1)
	lua_pushstring(p, cs("k"))
	lua_pushinteger(p, 9)
	lua_rawset(p, -3)
	lua_pushstring(p, cs("k"))
	lua_rawget(p, -2)
	lua_pop(p, 1)
	lua_pushinteger(p, 3)
	lua_rawseti(p, -2, 2)
	lua_rawgeti(p, -1, 2)
	lua_pop(p, 1)
	lua_len(p, -1)
	lua_pop(p, 1)
	lua_pushstring(p, cs("a"))
	lua_pushstring(p, cs("b"))
	lua_concat(p, 2)
	lua_pop(p, 1)
	lua_pushinteger(p, 2)
	lua_pushinteger(p, 3)
	lua_arith(p, C.int(LUA_OPADD))
	lua_pop(p, 1)
	lua_pushinteger(p, 1)
	lua_pushinteger(p, 2)
	_ = lua_compare(p, -2, -1, C.int(LUA_OPLT))
	lua_settop(p, 0)

	lua_pushstring(p, cs("x"))
	var ls C.size_t
	_ = luaL_checklstring(p, 1, &ls)
	_ = luaL_optlstring(p, 2, cs("def"), &ls)
	_ = luaL_optlstring(p, 2, nil, &ls)
	_ = capiPanics(func() { _ = luaL_checkinteger(p, 99) })
	lua_settop(p, 0)
	lua_pushinteger(p, 4)
	_ = luaL_checkinteger(p, 1)
	_ = luaL_optinteger(p, 2, 7)
	_ = luaL_checknumber(p, 1)
	lua_settop(p, 0)

	var B C.luaL_Buffer
	luaL_buffinit(p, &B)
	luaL_addstring(&B, cs("hi"))
	luaL_addchar_(&B, '!')
	lua_pushstring(p, cs("x"))
	luaL_addvalue(&B)
	luaL_pushresult(&B)

	var B2 C.luaL_Buffer
	_ = luaL_buffinitsize(p, &B2, 32)
	luaL_addlstring(&B2, cs("ab"), 2)
	luaL_pushresult(&B2)
	lua_settop(p, 0)

	lua_pushinteger(p, 1)
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	lua_pushboolean(p, 0)
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	lua_pushboolean(p, 1)
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	lua_pushnil(p)
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	lua_pushstring(p, cs("s"))
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	lua_newtable(p)
	lua_newtable(p)
	lua_pushstring(p, cs("T"))
	lua_setfield(p, -2, cs("__name"))
	lua_setmetatable(p, -2)
	_ = luaL_tolstring(p, -1, &ls)
	lua_settop(p, 0)
	luaL_traceback(p, p, cs("tb"), 0)
	lua_settop(p, 0)

	_ = luaL_fileresult(p, 1, cs("ok"))
	lua_settop(p, 0)
	_ = luaL_fileresult(p, 0, cs("fail"))
	lua_settop(p, 0)

	if lua_load(p, nil, unsafe.Pointer(cs("local a=1; return function(x) local y=x+a; return y end")), name, mode) == LUA_OK {
		lua_call(p, 0, 1)
		var ar C.lua_Debug
		_ = lua_getinfo(p, cs(">nS"), &ar)
		/* function consumed by '>': reload */
		if lua_load(p, nil, unsafe.Pointer(cs("local a=1; return function(x) return x+a end")), name, mode) == LUA_OK {
			lua_call(p, 0, 1)
			_ = lua_getupvalue(p, -1, 1)
			lua_pop(p, 1)
			lua_pushinteger(p, 9)
			_ = lua_setupvalue(p, -2, 1)
		}
		lua_settop(p, 0)
	}

	lua_pushlightuserdata(p, unsafe.Pointer(&key))
	_ = lua_islightuserdata(p, -1)
	_ = lua_touserdata(p, -1)
	lua_pop(p, 1)

	_ = lua_pushstring(p, nil)
	lua_settop(p, 0)

	if lua_load(p, nil, unsafe.Pointer(cs("error('e')")), name, mode) == LUA_OK {
		_ = lua_pcall(p, 0, 0, 0)
	}
	lua_settop(p, 0)

	/* Non-nil lua_Reader path (cdl_call_reader). */
	src2 := cs("return 42")
	/* coverTestReader takes const char**; pass pointer to C string pointer. */
	var chunk *C.char = src2
	if lua_load(p, C.cover_test_reader(), unsafe.Pointer(&chunk), name, mode) == LUA_OK {
		lua_call(p, 0, 1)
		_ = lua_tointeger(p, -1)
	}
	lua_settop(p, 0)
	/* Empty reader → EOF immediately */
	var empty *C.char = nil
	_ = lua_load(p, C.cover_test_reader(), unsafe.Pointer(&empty), name, mode)
	lua_settop(p, 0)

	chunk3 := cs("local x=1; return x")
	var c3 *C.char = chunk3
	if lua_load(p, C.cover_test_reader(), unsafe.Pointer(&c3), name, nil) == LUA_OK {
		lua_call(p, 0, 1)
	}
	lua_settop(p, 0)

	return nil
}

func capiPanics(fn func()) (panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()
	fn()
	return false
}
