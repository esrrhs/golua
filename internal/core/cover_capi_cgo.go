//go:build cgo && unix

package core

/*
#include "cdlbridge.h"
*/
import "C"

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"
)

// exerciseCAPI calls the C entry points an extension would call. They
// live in this file, not a _test.go, because cgo is not allowed in tests.
func exerciseCAPI(dir string) error {
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
	if lua_version(p) == nil || *lua_version(p) == 0 || lua_newstate(nil, nil) != nil || luaL_newstate() != nil {
		return fmt.Errorf("version / newstate")
	}
	luaL_openlibs(p)
	if lua_gettop(p) != 0 {
		return fmt.Errorf("openlibs left %d", lua_gettop(p))
	}

	lua_pushinteger(p, 11)
	lua_pushinteger(p, 22)
	lua_pushstring(p, cs("hi"))
	if C.GoString(lua_tostring(p, -1)) != "hi" {
		return fmt.Errorf("tostring")
	}
	lua_pop(p, 1)
	lua_insert(p, 1)
	lua_remove(p, 1)
	if lua_tointeger(p, -1) != 11 || lua_tonumber(p, -1) != 11 {
		return fmt.Errorf("pop/insert/remove")
	}
	lua_settop(p, 0)

	lua_pushglobaltable(p)
	lua_newtable(p)
	if lua_type(p, -1) != LUA_TTABLE {
		return fmt.Errorf("newtable")
	}
	var key int
	lua_pushinteger(p, 5)
	lua_rawsetp(p, -2, unsafe.Pointer(&key))
	if lua_rawgetp(p, -1, unsafe.Pointer(&key)) != LUA_TNUMBER || lua_tointeger(p, -1) != 5 {
		return fmt.Errorf("rawgetp")
	}
	lua_pop(p, 1)
	if lua_topointer(p, -1) == nil {
		return fmt.Errorf("topointer")
	}
	if lua_newuserdata(p, 8) == nil {
		return fmt.Errorf("newuserdata")
	}
	lua_settop(p, 0)

	src, name, mode := cs("return 7"), cs("=capi"), cs("t")
	if lua_load(p, nil, unsafe.Pointer(src), name, mode) != LUA_OK {
		return fmt.Errorf("load: %s", C.GoString(lua_tostring(p, -1)))
	}
	lua_call(p, 0, 1)
	if lua_tointeger(p, -1) != 7 {
		return fmt.Errorf("loaded chunk")
	}
	lua_pop(p, 1)

	path := filepath.Join(dir, "c.lua")
	if err := os.WriteFile(path, []byte("return 8"), 0o644); err != nil {
		return err
	}
	if luaL_loadfile(p, cs(path)) != LUA_OK {
		return fmt.Errorf("loadfile: %s", C.GoString(lua_tostring(p, -1)))
	}
	lua_pop(p, 1)
	if luaL_loadfilex(p, cs(path), mode) != LUA_OK {
		return fmt.Errorf("loadfilex")
	}
	lua_pop(p, 1)
	if luaL_loadfile(p, cs(filepath.Join(dir, "missing.lua"))) == LUA_OK {
		return fmt.Errorf("missing file loaded")
	}
	/* the loader returns the error instead of pushing it */
	lua_settop(p, 0)

	if luaL_optnumber(p, 1, 4.5) != 4.5 {
		return fmt.Errorf("optnumber default %v", luaL_optnumber(p, 1, 4.5))
	}
	lua_pushinteger(p, 3)
	if lua_gettop(p) != 1 || luaL_optnumber(p, 1, 0) != 3 {
		return fmt.Errorf("optnumber arg top=%d val=%v", lua_gettop(p), luaL_optnumber(p, 1, 0))
	}
	lua_settop(p, 0)

	lua_newtable(p)
	lua_newtable(p)
	if lua_load(p, nil, unsafe.Pointer(cs("return function() return 'yes' end")), name, mode) != LUA_OK {
		return fmt.Errorf("meta function")
	}
	lua_call(p, 0, 1)
	lua_setfield(p, -2, cs("__tostring"))
	lua_pushstring(p, cs("yes"))
	lua_setfield(p, -2, cs("__name"))
	lua_setmetatable(p, -2)
	if luaL_getmetafield(p, -1, cs("__name")) == 0 {
		return fmt.Errorf("getmetafield")
	}
	if C.GoString(lua_tostring(p, -1)) != "yes" {
		return fmt.Errorf("metafield value")
	}
	lua_pop(p, 1)
	if luaL_callmeta(p, -1, cs("__tostring")) == 0 {
		return fmt.Errorf("callmeta")
	}
	if C.GoString(lua_tostring(p, -1)) != "yes" {
		return fmt.Errorf("callmeta result %q", C.GoString(lua_tostring(p, -1)))
	}
	lua_settop(p, 0)

	tn := lua_typename(p, LUA_TSTRING)
	if C.GoString(tn) != "string" {
		return fmt.Errorf("typename")
	}
	/* The pointer is a stable interned literal, the same one PUC-Lua
	   returns. Freeing it would corrupt the next lua_typename. */
	if lua_typename(p, LUA_TSTRING) != tn {
		return fmt.Errorf("typename not stable")
	}

	lua_pushstring(p, cs("  42 "))
	if lua_stringtonumber(p, cs("  42 ")) == 0 || lua_tointeger(p, -1) != 42 {
		return fmt.Errorf("stringtonumber")
	}
	lua_pop(p, 2)

	if luaL_execresult(p, 1) != 1 || lua_toboolean(p, -1) == 0 {
		return fmt.Errorf("execresult")
	}
	lua_pop(p, 1)

	nameMT := cs("COVER_MT")
	if luaL_newmetatable(p, nameMT) != 1 {
		return fmt.Errorf("newmetatable")
	}
	lua_pop(p, 1)
	lua_newuserdata(p, 4)
	luaL_setmetatable(p, nameMT)
	if luaL_testudata(p, -1, nameMT) == nil {
		return fmt.Errorf("testudata")
	}
	luaL_getmetatable(p, nameMT)
	if lua_type(p, -1) != LUA_TTABLE {
		return fmt.Errorf("getmetatable")
	}
	lua_settop(p, 0)

	if lua_pushthread(p) == 0 {
		return fmt.Errorf("pushthread on the main state")
	}
	lua_pop(p, 1)
	th := lua_newthread(p)
	if lua_tothread(p, -1) != th {
		return fmt.Errorf("tothread")
	}
	lua_pop(p, 1) /* 5.3.4: no lua_closethread */

	lua_getglobal(p, cs("type"))
	_ = lua_tocfunction(p, -1)
	if lua_type(p, -1) != LUA_TFUNCTION {
		return fmt.Errorf("type is not a function")
	}
	lua_pop(p, 1)

	if lua_load(p, nil, unsafe.Pointer(cs("return function(x) return x end")), name, mode) != LUA_OK {
		return fmt.Errorf("load closure")
	}
	lua_call(p, 0, 1)
	if lua_dump(p, nil, nil, 0) != 1 {
		return fmt.Errorf("dump with no writer")
	}
	lua_pop(p, 1)

	/* 5.3.4: no lua_setwarnf */
	if lua_gethook(p) != nil {
		return fmt.Errorf("gethook")
	}
	_ = lua_gethookmask(p)
	_ = lua_gethookcount(p)
	lua_setallocf(p, C.lua_Alloc(C.cdl_allocfn()), nil)
	if lua_getallocf(p, nil) == nil {
		return fmt.Errorf("allocf")
	}

	for _, what := range []C.int{
		luaGCStop, luaGCRestart, luaGCCollect, luaGCCount, luaGCCountB,
		luaGCStep, luaGCSetPause, luaGCSetStepMul, luaGCIsRunning,
	} {
		cdl_gc_cb(p, what, 200, 0, 0)
	}
	/* 5.3.4: unknown what (incl. 5.4 GCGEN/GCINC) → -1 */
	if cdl_gc_cb(p, 10, 0, 0, 0) != -1 || cdl_gc_cb(p, 11, 0, 0, 0) != -1 {
		return fmt.Errorf("lua_gc unknown option")
	}

	fresh := NewState()
	fp := fresh.statePtr()
	if luaopen_base(fp) == 0 || luaopen_coroutine(fp) == 0 || luaopen_table(fp) == 0 ||
		luaopen_io(fp) == 0 || luaopen_os(fp) == 0 || luaopen_string(fp) == 0 ||
		luaopen_utf8(fp) == 0 || luaopen_math(fp) == 0 || luaopen_debug(fp) == 0 ||
		luaopen_package(fp) == 0 {
		lua_close(fp)
		return fmt.Errorf("luaopen")
	}
	lua_close(fp)

	/* more auxlib / buffer / typeerror / gsub paths */
	lua_pushstring(p, cs("hello"))
	var blen C.size_t
	_ = luaL_tolstring(p, -1, &blen)
	lua_pop(p, 2)
	lua_newtable(p)
	_ = luaL_tolstring(p, -1, nil)
	lua_pop(p, 2)

	var B C.luaL_Buffer
	luaL_buffinit(p, &B)
	luaL_addstring(&B, cs("ab"))
	luaL_addlstring(&B, cs("cd"), 2)
	luaL_addgsub(&B, cs("x_y_z"), cs("_"), cs("+"))
	luaL_pushresult(&B)
	if C.GoString(lua_tostring(p, -1)) != "abcdx+y+z" {
		return fmt.Errorf("addgsub %q", C.GoString(lua_tostring(p, -1)))
	}
	lua_pop(p, 1)

	luaL_buffinitsize(p, &B, 32)
	dst := luaL_prepbuffsize(&B, 4)
	if dst != nil {
		*(*byte)(unsafe.Pointer(dst)) = 'Z'
		B.n++ /* luaL_addsize macro */
	}
	luaL_addchar_(&B, '!')
	luaL_pushresult(&B)
	lua_pop(p, 1)

	_ = luaL_optlstring(p, 1, cs("def"), &blen)
	lua_pushstring(p, cs("arg"))
	_ = luaL_optlstring(p, 1, cs("def"), &blen)
	lua_settop(p, 0)

	_ = luaL_fileresult(p, 1, nil)
	lua_settop(p, 0)
	_ = luaL_fileresult(p, 0, cs("/no/such"))
	lua_settop(p, 0)

	L3 := NewState()
	p3 := L3.statePtr()
	luaL_openlibs(p3)
	lua_pushinteger(p3, 1)
	if !capiPanics(func() { luaL_typeerror(p3, 1, cs("table")) }) {
		L3.Close()
		return fmt.Errorf("typeerror did not fail")
	}
	L3.Close()

	if !capiPanics(func() { lua_yield(p, 0) }) {
		return fmt.Errorf("yield outside a coroutine did not fail")
	}
	if !capiPanics(func() { cdl_runerror_cb(p, cs("nope")) }) {
		return fmt.Errorf("runerror did not fail")
	}

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
