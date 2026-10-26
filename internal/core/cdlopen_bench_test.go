//go:build cgo && unix

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const cBenchModSrc = `#include <stdio.h>
#include <string.h>
#include "lua.h"
#include "lauxlib.h"

static int l_inc(lua_State *L) {
  lua_Integer n = lua_tointeger(L, 1);
  lua_pushinteger(L, n + 1);
  return 1;
}

static int l_str_roundtrip(lua_State *L) {
  size_t len = 0;
  const char *s = lua_tolstring(L, 1, &len);
  lua_pushlstring(L, s, len);
  lua_pushstring(L, "status_ok");
  return 2;
}

static int l_str_new(lua_State *L) {
  /* Build a 48-byte string (> LUAI_MAXSHORTLEN=40) so lua_pushlstring
     allocates a new long TString every time. */
  char buf[64];
  lua_Integer i = lua_tointeger(L, 1);
  memset(buf, 'a', 48);
  buf[0] = (char)('0' + (i & 7));
  buf[48] = '\0';
  lua_pushlstring(L, buf, 48);
  return 1;
}

static int l_table_fields(lua_State *L) {
  lua_getfield(L, 1, "x");
  lua_Integer x = lua_tointeger(L, -1);
  lua_getfield(L, 1, "y");
  lua_Integer y = lua_tointeger(L, -1);
  lua_pop(L, 2);
  lua_pushinteger(L, x + y + 1);
  lua_setfield(L, 1, "z");
  lua_getglobal(L, "g_scale");
  lua_Integer sc = lua_tointeger(L, -1);
  lua_pop(L, 1);
  lua_pushinteger(L, (x + y) * sc);
  lua_setglobal(L, "g_out");
  return 0;
}

static const luaL_Reg bench_funcs[] = {
  {"inc", l_inc},
  {"str_roundtrip", l_str_roundtrip},
  {"str_new", l_str_new},
  {"table_fields", l_table_fields},
  {NULL, NULL}
};

int luaopen_cbench(lua_State *L) {
  luaL_newlib(L, bench_funcs);
  return 1;
}
`

func buildBenchCExt(b *testing.B, dir, name, src string) {
	b.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		b.Skip("no C compiler in PATH")
	}
	cfile := filepath.Join(dir, name+".c")
	sofile := filepath.Join(dir, name+".so")
	if err := os.WriteFile(cfile, []byte(src), 0o644); err != nil {
		b.Fatal(err)
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		b.Fatal(err)
	}
	args := []string{"-O2", "-shared", "-fPIC", "-I" + inc, "-o", sofile, cfile}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		b.Skipf("cannot build C extension (%v): %s", err, out)
	}
}

// BenchmarkCCall measures calling a C function from Lua 200,000 times
// (cFunc entry/exit + lua_tointegerx + lua_pushinteger), directly
// comparable to BenchmarkGoCall.
func BenchmarkCCall(b *testing.B) {
	dir := b.TempDir()
	buildBenchCExt(b, dir, "cbench", cBenchModSrc)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"cbench = require('cbench')\n"
	if err := L.DoString(setup); err != nil {
		b.Fatal(err)
	}
	if err := L.LoadString(`
		local inc = cbench.inc
		local s = 0
		for i = 1, 200000 do s = inc(s) end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkCStringBridge measures lua_tolstring + lua_pushlstring +
// lua_pushstring on cached/interned strings across 100,000 C calls.
func BenchmarkCStringBridge(b *testing.B) {
	dir := b.TempDir()
	buildBenchCExt(b, dir, "cbench", cBenchModSrc)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"cbench = require('cbench')\n"
	if err := L.DoString(setup); err != nil {
		b.Fatal(err)
	}
	if err := L.LoadString(`
		local rt = cbench.str_roundtrip
		local s = "hello_world_bridge_string"
		local a, st
		for i = 1, 100000 do
			a, st = rt(s)
		end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkCStringNew measures lua_pushlstring on newly allocated strings
// across 100,000 C calls.
func BenchmarkCStringNew(b *testing.B) {
	dir := b.TempDir()
	buildBenchCExt(b, dir, "cbench", cBenchModSrc)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"cbench = require('cbench')\n"
	if err := L.DoString(setup); err != nil {
		b.Fatal(err)
	}
	if err := L.LoadString(`
		local sn = cbench.str_new
		local s
		for i = 1, 100000 do
			s = sn(i)
		end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkCTableFields measures lua_getfield, lua_setfield,
// lua_getglobal, and lua_setglobal across 100,000 C extension calls.
func BenchmarkCTableFields(b *testing.B) {
	dir := b.TempDir()
	buildBenchCExt(b, dir, "cbench", cBenchModSrc)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"cbench = require('cbench')\n" +
		"g_scale = 3\n" +
		"g_out = 0\n"
	if err := L.DoString(setup); err != nil {
		b.Fatal(err)
	}
	if err := L.LoadString(`
		local tf = cbench.table_fields
		local t = { x = 10, y = 20, z = 0 }
		for i = 1, 100000 do
			tf(t)
		end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkCJSONEncodeDecode measures end-to-end C extension performance
// using Lua CJSON encode + decode in a loop.
func BenchmarkCJSONEncodeDecode(b *testing.B) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		b.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/cjson")
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "lua_cjson.c")); err != nil {
		b.Skipf("testdata/cjson not available: %v", err)
	}
	dir := b.TempDir()
	var names []string
	for _, f := range cjsonHeaders {
		if err := copyFile(filepath.Join(src, f), filepath.Join(dir, f)); err != nil {
			b.Fatal(err)
		}
	}
	for _, f := range cjsonSources {
		dst := filepath.Join(dir, f)
		if err := copyFile(filepath.Join(src, f), dst); err != nil {
			b.Fatal(err)
		}
		names = append(names, dst)
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		b.Fatal(err)
	}
	args := []string{"-O2", "-std=gnu99", "-DNDEBUG", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(dir, "cjson.so")}
	args = append(args, names...)
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		b.Skipf("cannot build cjson (%v): %s", err, out)
	}

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"cjson = require('cjson')\n"
	if err := L.DoString(setup); err != nil {
		b.Fatal(err)
	}
	if err := L.LoadString(`
		local encode = cjson.encode
		local decode = cjson.decode
		local obj = {
			id = 12345,
			name = "alice",
			active = true,
			scores = {10, 20, 30, 40},
			meta = {role = "admin", region = "ap-east"}
		}
		for i = 1, 10000 do
			local s = encode(obj)
			local t = decode(s)
		end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}
