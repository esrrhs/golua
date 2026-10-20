//go:build cgo && unix

package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This test pins the whole C API against PUC-Lua 5.4.9: every line the
// driver prints was captured from the reference interpreter running the
// same extension, and the two agree byte for byte.
//
// What it covers, beyond cdlopen_test.go's "it loads and runs":
//
//   - userdata with a registered metatable: luaL_newmetatable,
//     luaL_checkudata, luaL_testudata, and the error when the type is
//     wrong
//   - the two variadic functions: luaL_error with arguments, and
//     lua_pushfstring for every conversion Lua has, including the
//     "invalid option" error for one it does not
//   - luaL_checkoption, luaL_Buffer with luaL_addchar, user values
//   - lua_rawgetp / lua_rawsetp, lua_dump, lua_gc
//   - lua_getstack / lua_getinfo, read through a C lua_Debug
//   - lua_version, lua_isyieldable, lua_stringtonumber
//   - and that two extension functions are two different values, which
//     is what lets luaL_argerror name the function that is running

// cExtSrc is a C extension in the ordinary style: found by 'require',
// opened by luaopen_cext, registered with luaL_newlib.
const cExtSrc = `#include <string.h>
#include "lua.h"
#include "lauxlib.h"

typedef struct { int x; } Box;

static int l_box(lua_State *L) {
  Box *b = (Box *)lua_newuserdata(L, sizeof(Box));
  b->x = (int)luaL_checkinteger(L, 1);
  luaL_getmetatable(L, "Box");
  lua_setmetatable(L, -2);
  return 1;
}
static int l_unbox(lua_State *L) {
  Box *b = (Box *)luaL_checkudata(L, 1, "Box");
  lua_pushinteger(L, b->x);
  return 1;
}
static int l_isbox(lua_State *L) {
  lua_pushboolean(L, luaL_testudata(L, 1, "Box") != NULL);
  return 1;
}
static int l_boom(lua_State *L) {
  const char *who = luaL_checkstring(L, 1);
  int n = (int)luaL_checkinteger(L, 2);
  return luaL_error(L, "boom from %s: %d (%s)", who, n, "really");
}
static int l_fmt(lua_State *L) {
  int which = (int)luaL_checkinteger(L, 1);
  switch (which) {
    case 1: lua_pushfstring(L, "s=%s", luaL_checkstring(L, 2)); break;
    case 2: lua_pushfstring(L, "d=%d", (int)luaL_checkinteger(L, 2)); break;
    case 3: lua_pushfstring(L, "I=%I", (lua_Integer)luaL_checkinteger(L, 2)); break;
    case 4: lua_pushfstring(L, "f=%f", (lua_Number)luaL_checknumber(L, 2)); break;
    case 5: lua_pushfstring(L, "c=%c", (int)luaL_checkinteger(L, 2)); break;
    case 6: lua_pushfstring(L, "U=%U", (long)luaL_checkinteger(L, 2)); break;
    case 7: lua_pushfstring(L, "100%% done"); break;
    case 8: lua_pushfstring(L, "null=%s", (const char *)NULL); break;
    case 9: lua_pushfstring(L, "%.2f", 1.5); break;
    case 10: lua_pushfstring(L, "big=%I", (lua_Integer)9007199254740993LL); break;
    case 11: lua_pushfstring(L, "pi=%f", 3.14159265358979); break;
  }
  return 1;
}
static const char *const modes[] = {"fast", "slow", NULL};
static int l_mode(lua_State *L) {
  lua_pushstring(L, modes[luaL_checkoption(L, 1, "fast", modes)]);
  return 1;
}
static int l_join(lua_State *L) {
  luaL_Buffer b;
  const char *s = luaL_checkstring(L, 1);
  int n = (int)luaL_checkinteger(L, 2);
  luaL_buffinit(L, &b);
  for (int i = 0; i < n; i++) {
    const char *p = s;
    while (*p) luaL_addchar(&b, *p++);
  }
  luaL_pushresult(&b);
  return 1;
}
static int l_setuv(lua_State *L) { lua_settop(L, 2); lua_setiuservalue(L, 1, 1); return 0; }
static int l_getuv(lua_State *L) {
  if (lua_getiuservalue(L, 1, 1) == LUA_TNONE) lua_pushnil(L);
  return 1;
}
static int l_rawp(lua_State *L) {
  static int key = 7;
  lua_settop(L, 1);
  lua_pushinteger(L, 99);
  lua_rawsetp(L, 1, (void *)&key);
  lua_rawgetp(L, 1, (void *)&key);
  return 1;
}
static int writer(lua_State *L, const void *p, size_t sz, void *ud) {
  (void)L; (void)p; *(size_t *)ud += sz; return 0;
}
static int l_dumpsize(lua_State *L) {
  luaL_checktype(L, 1, LUA_TFUNCTION);
  lua_settop(L, 1);
  size_t total = 0;
  int st = lua_dump(L, writer, &total, 1);
  lua_pushinteger(L, (lua_Integer)total);
  lua_pushboolean(L, st == 0);
  return 2;
}
static int l_gcinfo(lua_State *L) {
  int kb = lua_gc(L, LUA_GCCOUNT);
  lua_pushinteger(L, kb);
  return 1;
}
static int l_whereami(lua_State *L) {
  lua_Debug ar;
  if (lua_getstack(L, 1, &ar) && lua_getinfo(L, "Sl", &ar)) {
    lua_pushfstring(L, "%s:%d", ar.short_src, ar.currentline);
    return 1;
  }
  lua_pushnil(L);
  return 1;
}
static int l_ver(lua_State *L) { lua_pushnumber(L, lua_version(L)); return 1; }
static int l_yieldable(lua_State *L) { lua_pushboolean(L, lua_isyieldable(L)); return 1; }
static int l_tonum(lua_State *L) {
  lua_pushinteger(L, (lua_Integer)lua_stringtonumber(L, luaL_checkstring(L, 1)));
  return 1;
}
/* lua_getextraspace reaches *behind* the lua_State* C was handed, so
   there has to be something there -- and it has to be the state's own,
   not somebody else's, or two states would share it. */
static int l_extra(lua_State *L) {
  void *p = lua_getextraspace(L);
  unsigned char *q = (unsigned char *)p;
  size_t i;
  int ok = 1;
  memset(p, 0x5A, LUA_EXTRASPACE);
  for (i = 0; i < LUA_EXTRASPACE; i++) if (q[i] != 0x5A) ok = 0;
  memset(p, 0xA5, LUA_EXTRASPACE);
  for (i = 0; i < LUA_EXTRASPACE; i++) if (q[i] != 0xA5) ok = 0;
  lua_pushboolean(L, ok);
  lua_pushinteger(L, (lua_Integer)LUA_EXTRASPACE);
  return 2;
}
static int l_noop(lua_State *L) { return 0; }

static const luaL_Reg funcs[] = {
  {"box", l_box}, {"unbox", l_unbox}, {"isbox", l_isbox},
  {"boom", l_boom}, {"fmt", l_fmt}, {"mode", l_mode}, {"join", l_join},
  {"setuv", l_setuv}, {"getuv", l_getuv}, {"rawp", l_rawp},
  {"dumpsize", l_dumpsize}, {"gcinfo", l_gcinfo}, {"whereami", l_whereami},
  {"ver", l_ver}, {"yieldable", l_yieldable}, {"tonum", l_tonum},
  {"noop", l_noop}, {"extra", l_extra}, {NULL, NULL}
};

int luaopen_cext(lua_State *L) {
  luaL_newmetatable(L, "Box");
  lua_pop(L, 1);
  luaL_newlib(L, funcs);
  return 1;
}
`

// cExtDriver exercises it from Lua. It runs with the chunk name "=cext",
// which is why the line in the 'where' answer is what it is.
const cExtDriver = `local m = require("cext")
local function P(...) print(...) end
-- identity: two extension functions must not compare equal
P("ident", m.box == m.unbox, m.noop == m.noop)
-- userdata + metatable
local box = m.box(41)
P("box", m.unbox(box), m.isbox(box), m.isbox(42))
P("badud", pcall(m.unbox, 42))
-- variadic luaL_error
P("boom", pcall(m.boom, "me", 7))
-- lua_pushfstring, every conversion
for i = 1, 11 do
  local ok, r = pcall(m.fmt, i, 65)
  P("fmt" .. i, ok, ok and (tostring(r):gsub("0x%x+", "0xPTR")) or (tostring(r):gsub("^.-:%d+: ", "")))
end
-- luaL_checkoption
P("mode", m.mode("slow"), m.mode(nil), pcall(m.mode, "nope"))
-- luaL_Buffer
P("join", m.join("ab", 3))
-- user values
m.setuv(box, "tag")
P("getuv", m.getuv(box))
-- rawsetp / rawgetp
P("rawp", m.rawp({}))
-- lua_dump
local n, ok = m.dumpsize(function(a) return a + 1 end)
P("dump", n > 0, ok)
-- lua_gc
P("gc", type(m.gcinfo()))
-- lua_getstack / lua_getinfo
local function inner() return m.whereami() end
P("where", inner())
-- the rest
P("ver", m.ver())
P("yield", m.yieldable())
P("tonum", m.tonum("3.5"), m.tonum("xyz"))
-- the bytes in front of the lua_State*
P("extra", m.extra())
`

// cExtWant is what PUC-Lua 5.4.9 prints for cExtDriver, line for line.
var cExtWant = []string{
	"ident\tfalse\ttrue",
	"box\t41\ttrue\tfalse",
	"badud\tfalse\tbad argument #1 to 'cext.unbox' (Box expected, got number)",
	"boom\tfalse\tboom from me: 7 (really)",
	"fmt1\ttrue\ts=65",
	"fmt2\ttrue\td=65",
	"fmt3\ttrue\tI=65",
	"fmt4\ttrue\tf=65.0",
	"fmt5\ttrue\tc=A",
	"fmt6\ttrue\tU=A",
	"fmt7\ttrue\t100% done",
	"fmt8\ttrue\tnull=(null)",
	"fmt9\tfalse\tinvalid option '%.' to 'lua_pushfstring'",
	"fmt10\ttrue\tbig=9007199254740993",
	"fmt11\ttrue\tpi=3.1415926535898",
	"mode\tslow\tfast\tfalse\tbad argument #1 to 'cext.mode' (invalid option 'nope')",
	"join\tababab",
	"getuv\ttag",
	"rawp\t99",
	"dump\ttrue\ttrue",
	"gc\tnumber",
	"where\tcext:31",
	"ver\t504.0",
	"yield\tfalse",
	"tonum\t4\t0",
	"extra\ttrue\t8",
}

// compileCExt compiles <modname>.c in dir into <modname>.so. It reports
// false when there is no C compiler, or when the platform will not let
// lua_* stay unresolved until load time.
func compileCExt(t *testing.T, dir, modname string) bool {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src := filepath.Join(dir, modname+".c")
	so := filepath.Join(dir, modname+".so")
	inc, err := filepath.Abs("include")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build a C extension here (%v): %s", err, out)
	}
	return true
}

// runCExtDriver runs a Lua chunk in a state whose cpath is dir, with
// 'print' collected, and answers the lines it printed. The chunk is
// named after it so that a "file:line" in the output is stable.
func runCExtDriver(t *testing.T, dir, driver string) []string {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		n := L.GetTop()
		parts := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			parts = append(parts, lTolstring(L, i)) /* as 'print' does: tostring */
		}
		got = append(got, strings.Join(parts, "\t"))
		return 0
	})
	L.SetGlobal("print")

	if err := L.DoString("package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so"))); err != nil {
		t.Fatalf("setting cpath: %v", err)
	}
	if err := L.LoadString(driver, "="+"cext"); err != nil {
		t.Fatalf("loading the driver: %v", err)
	}
	if st := L.PCall(0, 0, 0); st != LUA_OK {
		t.Fatalf("running the driver: %s", L.ToStringX(-1))
	}
	return got
}

// compareLines checks each line against what PUC-Lua answered.
func compareLines(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n got %q\nwant %q", i+1, got[i], want[i])
		}
	}
}

func TestCModuleAPI(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cext.c"), []byte(cExtSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if !compileCExt(t, dir, "cext") {
		return
	}
	compareLines(t, runCExtDriver(t, dir, cExtDriver), cExtWant)
}

// cErrSrc and cErrDriver cover an error crossing a C frame, which is the
// one thing 'lua_error' has to get right: PUC-Lua longjmps out of the
// extension's own frame, this port raises instead, and both have to
// leave the error with the caller that was protecting the call.
// Three levels deep (Lua -> C -> Lua -> C -> raise) and caught inside C
// as well, so neither direction is left untested.
const cErrSrc = `#include <stdio.h>
#include <stdlib.h>
#include "lua.h"
#include "lauxlib.h"

/* three levels: Lua -> C -> Lua -> C -> raise */
static int l_inner(lua_State *L) {
  lua_pushliteral(L, "from inner");
  return lua_error(L);
}
static int l_middle(lua_State *L) {
  lua_settop(L, 1);
  lua_call(L, 0, 1);   /* calls a Lua function that calls l_inner */
  lua_pushliteral(L, "unreachable");
  return 1;
}
/* C -> lua_call -> C -> lua_pcall -> raise: the error is caught inside */
static int l_catch_inside(lua_State *L) {
  lua_settop(L, 1);
  int st = lua_pcall(L, 0, 1, 0);
  lua_pushinteger(L, st);
  return 2;
}
static const luaL_Reg funcs[] = {
  {"inner", l_inner}, {"middle", l_middle},
  {"catch_inside", l_catch_inside}, {NULL, NULL}
};
int luaopen_nest(lua_State *L) { luaL_newlib(L, funcs); return 1; }
`

const cErrDriver = `local m = require("nest")
local function P(n, ...) print(n, ...) end
P("inner", pcall(m.inner))
P("middle", pcall(m.middle, function() return m.inner() end))
P("middle2", pcall(m.middle, function() error("lua err") end))
P("catch", pcall(m.catch_inside, function() return m.inner() end))
P("alive", _VERSION)
`

// cErrWant is what PUC-Lua 5.4.9 prints for cErrDriver.
var cErrWant = []string{
	"inner\tfalse\tfrom inner",
	"middle\tfalse\tfrom inner",
	"middle2\tfalse\tcext:5: lua err",
	"catch\ttrue\tfrom inner\t2",
	"alive\tLua 5.4",
}

func TestCModuleErrors(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nest.c"), []byte(cErrSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if !compileCExt(t, dir, "nest") {
		return
	}
	got := runCExtDriver(t, dir, cErrDriver)
	compareLines(t, got, cErrWant)
}

// TestCModuleUserDataFreed checks that the C block behind a userdata is
// given back when the userdata is collected. C.malloc is not memory the
// Go collector knows about, so an extension that makes userdata in a
// loop would otherwise grow without bound.
func TestCModuleUserDataFreed(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cext.c"), []byte(cExtSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if !compileCExt(t, dir, "cext") {
		return
	}
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	src := "package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) + "\n" +
		"local m = require('cext')\n" +
		"for i = 1, 2000 do local b = m.box(i) end\n" +
		"collectgarbage()\n"
	/* counted before the chunk runs: the collector may free some of the
	   blocks while it is running, and those count too */
	before := cdlUserFrees.Load()
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	for i := 0; i < 20 && cdlUserFrees.Load() == before; i++ {
		runtime.GC()
		time.Sleep(10 * time.Millisecond) /* finalizers run on their own goroutine */
	}
	if got := cdlUserFrees.Load(); got <= before {
		t.Fatalf("no userdata block was freed (%d before, %d after)", before, got)
	}
}
