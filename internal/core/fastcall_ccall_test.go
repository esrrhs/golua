//go:build cgo && unix && amd64 && !golua_fastc

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFastCallCFunction loads a tiny C extension through cFunc/callCFunction
// (default cgo or -tags golua_purego).
func TestFastCallCFunction(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "t.c")
	so := filepath.Join(dir, "t.so")
	code := `#include "lua.h"
#include "lauxlib.h"
static int l_inc(lua_State *L) {
  lua_Integer n = lua_tointeger(L, 1);
  lua_pushinteger(L, n + 1);
  return 1;
}
int luaopen_t(lua_State *L) {
  lua_pushcfunction(L, l_inc);
  lua_setglobal(L, "inc");
  return 0;
}
`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("cc", "-O2", "-shared", "-fPIC", "-I"+inc, "-o", so, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("cc: %v (%s)", err, out)
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	setup := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\nrequire('t')\n"
	if err := L.DoString(setup); err != nil {
		t.Fatal(err)
	}
	if err := L.DoString(`assert(inc(41) == 42)`); err != nil {
		t.Fatal(err)
	}
}
