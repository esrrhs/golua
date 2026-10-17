//go:build cgo

package lua

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// globalInteger returns the value of a global as an integer.
func globalInteger(t *testing.T, L *State, name string) int64 {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	n, _ := L.ToInteger(-1)
	return n
}

// globalIsNil tells whether a global is nil.
func globalIsNil(t *testing.T, L *State, name string) bool {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	return L.IsNil(-1)
}

// cModuleSrc is a small C extension in the ordinary style: it is found
// by 'require' through package.cpath, opens with luaopen_<name>, and
// registers its functions with luaL_newlib. The header it includes is
// this repository's own include/lua.h, whose names are the real lua_*
// and luaL_* ones, so the source is exactly what one would write for
// PUC-Lua.
const cModuleSrc = `
#include <string.h>
#include "lua.h"
#include "lauxlib.h"

static int l_add (lua_State *L) {
  lua_Integer a = luaL_checkinteger(L, 1);
  lua_Integer b = luaL_checkinteger(L, 2);
  lua_pushinteger(L, a + b);
  return 1;
}

static int l_len (lua_State *L) {
  size_t n;
  luaL_checklstring(L, 1, &n);
  lua_pushinteger(L, (lua_Integer)n);
  return 1;
}

static int l_echo (lua_State *L) {
  lua_settop(L, 1);
  lua_pushvalue(L, 1);
  return 1;
}

static const luaL_Reg funcs[] = {
  {"add", l_add}, {"len", l_len}, {"echo", l_echo}, {NULL, NULL}
};

int luaopen_ctest (lua_State *L) {
  luaL_newlib(L, funcs);
  return 1;
}
`

// quoteLuaString writes s as a Lua literal in single quotes.
func quoteLuaString(s string) string {
	const hex = "0123456789abcdef"
	var b []byte
	b = append(b, '\'')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\', '\'':
			b = append(b, '\\', c)
		case '\n':
			b = append(b, '\\', 'n')
		default:
			if c < 0x20 {
				b = append(b, '\\', '0'+c/8, hex[c%8])
			} else {
				b = append(b, c)
			}
		}
	}
	return string(append(b, '\''))
}

// hasUndefinedDynamicLookup tells whether the linker takes
// -undefined dynamic_lookup, which is how a macOS extension leaves the
// lua_* names for the host to answer at dlopen time. Other linkers
// resolve them (or fail) without being told.
func hasUndefinedDynamicLookup() bool { return runtime.GOOS == "darwin" }

// buildCModule compiles cModuleSrc into a shared object next to dir and
// returns its path. It reports false when there is no C compiler, or
// when the host linker will not let the object leave lua_* unresolved
// until load time - the whole point of the exercise, and not something
// every platform's toolchain permits.
func buildCModule(t *testing.T, dir string) (string, bool) {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src := filepath.Join(dir, "ctest.c")
	so := filepath.Join(dir, "ctest.so")
	inc, err := filepath.Abs("include")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(cModuleSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	cmd := exec.Command(cc, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cannot build a C extension here (%v): %s", err, out)
	}
	return so, true
}

// TestLoadCModule requires a real C extension and calls into it. It is
// the one test that proves the dlopen path end to end: the host has to
// export the lua_* names, the loader has to resolve luaopen_ctest, and
// the functions the module registers have to run against this state.
func TestLoadCModule(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	if _, ok := buildCModule(t, dir); !ok {
		return
	}
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	src := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"local m = require('ctest')\n" +
		"add = m.add(3, 4)\n" +
		"neg = m.add(-7, 2)\n" +
		"len = m.len('hello')\n" +
		"echo = m.echo('x')\n" +
		/* the module table has no array part, so count its entries */
		"n = 0\nfor k in pairs(m) do n = n + 1 end\n" +
		"kind = type(m.add)\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := globalInteger(t, L, "add"); got != 7 {
		t.Fatalf("ctest.add(3, 4) = %d, want 7", got)
	}
	if got := globalInteger(t, L, "neg"); got != -5 {
		t.Fatalf("ctest.add(-7, 2) = %d, want -5", got)
	}
	if got := globalInteger(t, L, "len"); got != 5 {
		t.Fatalf("ctest.len('hello') = %d, want 5", got)
	}
	if got := globalString(t, L, "echo"); got != "x" {
		t.Fatalf("ctest.echo('x') = %q, want %q", got, "x")
	}
	if got := globalInteger(t, L, "n"); got != 3 {
		t.Fatalf("ctest has %d entries, want 3", got)
	}
	if got := globalString(t, L, "kind"); got != "function" {
		t.Fatalf("type(ctest.add) = %q, want %q", got, "function")
	}
}

// TestLoadCModuleMissingSymbol pins the two failures 'package.loadlib'
// reports: the library that will not open, and the library that opens
// but has no such function. Both messages are dlerror()'s own, so they
// are only checked as far as the platform lets them be.
func TestLoadCModuleMissingSymbol(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	so, ok := buildCModule(t, dir)
	if !ok {
		return
	}
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	src := "a, b, c = package.loadlib(" + quoteLuaString(so) + ", 'luaopen_nope')\n" +
		"d, e, f = package.loadlib('nosuch.so', 'luaopen_nosuch')\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := globalString(t, L, "c"); got != "init" {
		t.Fatalf("package.loadlib 'where' = %q, want %q", got, "init")
	}
	if got := globalString(t, L, "e"); got == "" {
		t.Fatal("package.loadlib gave no message for a missing library")
	}
	if got := globalString(t, L, "f"); got != "open" {
		t.Fatalf("package.loadlib 'where' = %q, want %q", got, "open")
	}
	if !globalIsNil(t, L, "a") || !globalIsNil(t, L, "d") {
		t.Fatal("package.loadlib did not return nil on failure")
	}
}
