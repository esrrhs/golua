//go:build cgo && unix

package core

import (
	"os"
	"path/filepath"
	"testing"
)

// cErrStressSrc is an extension whose functions raise, at depth: one
// that raises straight away, one that calls a Lua function -- which may
// itself raise -- before raising anyway, and one that nests C -> Lua ->
// C -> Lua ... as deep as the caller asks before raising at the bottom.
//
// The point of the stress cases is not what they print but that they
// survive.  An error has to leave the C frames it crosses behind in a
// state the next call can use, whether it leaves them by a longjmp --
// which is what PUC-Lua does -- or by the raise this port uses (see
// lua_error in cdlopen.go).
const cErrStressSrc = `#include <stdio.h>
#include <stdlib.h>
#include "lua.h"
#include "lauxlib.h"

static int l_raise(lua_State *L) {
  lua_pushliteral(L, "raised");
  return lua_error(L);
}

/* C -> Lua (which may raise) -> and then raise anyway */
static int l_thenraise(lua_State *L) {
  lua_settop(L, 1);
  lua_call(L, 0, 0);
  lua_pushliteral(L, "unreached");
  return lua_error(L);
}

/* n levels of C -> Lua -> C -> Lua -> ..., and a raise at the bottom:
   the error has to cross every one of them */
static int l_deep(lua_State *L) {
  int n = (int)luaL_checkinteger(L, 1);
  if (n > 0) {
    lua_getglobal(L, "step");
    lua_pushinteger(L, n - 1);
    lua_call(L, 1, 1);   /* Lua calls back into l_deep */
    return 1;
  }
  lua_pushliteral(L, "bottom");
  return lua_error(L);
}

static const luaL_Reg funcs[] = {
  {"raise", l_raise}, {"thenraise", l_thenraise}, {"deep", l_deep},
  {NULL, NULL}
};

int luaopen_stress(lua_State *L) { luaL_newlib(L, funcs); return 1; }
`

// TestCModuleErrorStress raises across C frames many times over, and
// from the bottom of a deep nest of them.
//
// Ten thousand times three is enough to show a leak of the C stack,
// which is the failure this is really guarding against: an error that
// abandons the C frames without giving the stack back would run out of
// it here, and not in any single case however carefully written.  Fifty
// levels of C -> Lua -> C asks the same question about the Go stack in
// between, where the raise is unwound.
func TestCModuleErrorStress(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stress.c"), []byte(cErrStressSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if !compileCExt(t, dir, "stress") {
		return
	}
	driver := `
	local m = require("stress")
	local n = 0
	for i = 1, 10000 do
	  local ok, e = pcall(m.raise)
	  if not ok and e == "raised" then n = n + 1 end
	  ok, e = pcall(m.thenraise, function() error("in lua", 0) end)
	  if not ok and e == "in lua" then n = n + 1 end
	  ok, e = pcall(m.thenraise, function() end)
	  if not ok and e == "unreached" then n = n + 1 end
	end
	print("survived", n)

	-- a nest of C -> Lua -> C -> Lua -> ..., and a raise at the bottom
	function step(n)
	  if n == 0 then return m.deep(0) end
	  return m.deep(n)
	end
	for _, d in ipairs{1, 10, 50} do
	  print(d, pcall(step, d))
	end
	print("alive", _VERSION)
	`
	got := runCExtDriver(t, dir, driver)
	want := []string{
		"survived	30000",
		"1	false	bottom",
		"10	false	bottom",
		"50	false	bottom",
		"alive	Lua 5.4",
	}
	compareLines(t, got, want)
}
