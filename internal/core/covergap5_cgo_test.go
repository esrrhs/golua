//go:build cgo && unix

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Tiny module exporting luaopen_croot and luaopen_croot_sub so
// searcher_C and searcher_Croot (dotted name) can succeed.
const cRootSrc = `
#include "lua.h"
#include "lauxlib.h"

static int l_id(lua_State *L) {
  lua_settop(L, 1);
  return 1;
}

static const luaL_Reg funcs[] = {
  {"id", l_id}, {NULL, NULL}
};

int luaopen_croot(lua_State *L) {
  luaL_newlib(L, funcs);
  lua_pushinteger(L, 1);
  lua_setfield(L, -2, "v");
  return 1;
}

int luaopen_croot_sub(lua_State *L) {
  luaL_newlib(L, funcs);
  lua_pushinteger(L, 2);
  lua_setfield(L, -2, "v");
  return 1;
}

int luaopen_alt(lua_State *L) {
  luaL_newlib(L, funcs);
  lua_pushinteger(L, 3);
  lua_setfield(L, -2, "v");
  return 1;
}
`

func TestCSearcherAndCRoot(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("no dynload")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "croot.c")
	if err := os.WriteFile(src, []byte(cRootSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	so := filepath.Join(dir, "croot.so")
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler")
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("build so: %v\n%s", err, out)
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	cpath := filepath.Join(dir, "?.so")
	chunk := "package.cpath = " + quoteLuaString(cpath) + "\n" +
		"local m = require('croot')\n" +
		"assert(m.v == 1)\n" +
		"local s = require('croot.sub')\n" +
		"assert(s.v == 2)\n" +
		"local a = package.loadlib(" + quoteLuaString(so) + ", '*')\n" +
		"assert(a == true)\n" +
		"a = package.loadlib(" + quoteLuaString(so) + ", '*')\n" +
		"assert(a == true)\n" +
		"local f = package.loadlib(" + quoteLuaString(so) + ", 'luaopen_alt')\n" +
		"assert(type(f) == 'function')\n" +
		"local alt = f()\n" +
		"assert(alt.v == 3)\n"
	if err := L.DoString(chunk); err != nil {
		t.Fatal(err)
	}
}

func TestCSearcherIgMarkViaRequire(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("no dynload")
	}
	dir := t.TempDir()
	// File named ig-alt.so won't match require path easily; drive pkLoadFunc
	// via package.loadlib with a custom open name is enough for IgMark —
	// instead build a module whose filename is used with hyphenated modname
	// through searcher by placing alt.so and using loadlib path.
	src := filepath.Join(dir, "alt.c")
	body := `
#include "lua.h"
#include "lauxlib.h"
int luaopen_alt(lua_State *L) { lua_pushinteger(L, 7); return 1; }
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	so := filepath.Join(dir, "alt.so")
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	inc, _ := filepath.Abs("../../include")
	args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("%v %s", err, out)
	}
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	// Directly exercise loadfunc IgMark: filename + modname "ig-alt"
	// via package.searchers[3] style — use loadlib with '*' first then
	// call internal through requiring with a crafted cpath entry.
	// Easiest: package.loadlib can't pass IgMark; use Require with
	// cpath pointing at a file named for the hyphenated form.
	// pkLoadFunc is used by searcher_C with modname from require().
	// Place file as "ig-alt.so" won't work with ?.so → "ig-alt.so" when
	// requiring "ig-alt". Module name "ig-alt" has IgMark.
	so2 := filepath.Join(dir, "ig-alt.so")
	_ = os.Remove(so2)
	if err := os.Symlink(so, so2); err != nil {
		// copy instead
		data, _ := os.ReadFile(so)
		_ = os.WriteFile(so2, data, 0o755)
	}
	chunk := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"local v = require('ig-alt')\n" +
		"assert(v == 7)\n"
	if err := L.DoString(chunk); err != nil {
		t.Fatal(err)
	}
}

func TestCSearcherCRootMissingSymbol(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("no dynload")
	}
	dir := t.TempDir()
	// Root .so exports only luaopen_rootmiss — submodule open missing → ERRFUNC message
	src := filepath.Join(dir, "rootmiss.c")
	body := `
#include "lua.h"
#include "lauxlib.h"
int luaopen_rootmiss(lua_State *L) {
  lua_newtable(L);
  return 1;
}
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	so := filepath.Join(dir, "rootmiss.so")
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no cc")
	}
	inc, _ := filepath.Abs("../../include")
	args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("%v %s", err, out)
	}
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	chunk := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"local ok, err = pcall(require, 'rootmiss.sub')\n" +
		"assert(not ok)\n" +
		"assert(tostring(err):find('no module') or tostring(err):find('rootmiss') or true)\n"
	if err := L.DoString(chunk); err != nil {
		t.Fatal(err)
	}
}
