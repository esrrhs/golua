package core

import (
	"strings"
	"testing"
)

// newDebugState returns a state with the base and the debug libraries
// opened. (linit.go registers the libraries through RequireF; the tests
// do it by hand so that they do not depend on linit.go.)
func newDebugState(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenBase(L)
	L.Pop(1) /* the base library leaves the global table on the stack */
	OpenDebug(L)
	L.SetGlobal("debug")
	return L
}

// globalString returns the value of a global as a string.
func dbGlobalString(t *testing.T, L *State, name string) string {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	s, _ := L.ToString(-1)
	return s
}

func TestDebugTraceback(t *testing.T) {
	L := newDebugState(t)
	if err := L.DoString("tb = debug.traceback()"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	tb := dbGlobalString(t, L, "tb")
	if !strings.Contains(tb, "chunk:") {
		t.Fatalf("debug.traceback() = %q", tb)
	}
	if !strings.Contains(tb, "stack traceback:") {
		t.Fatalf("debug.traceback() = %q", tb)
	}

	/* with a message and a level */
	if err := L.DoString("tb2 = debug.traceback('boom', 1)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	tb2 := dbGlobalString(t, L, "tb2")
	if !strings.HasPrefix(tb2, "boom\n") || !strings.Contains(tb2, "chunk:") {
		t.Fatalf("debug.traceback('boom', 1) = %q", tb2)
	}

	/* a non-string message is returned untouched */
	if err := L.DoString("tb3 = debug.traceback({})"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("tb3")
	if !L.IsTable(-1) {
		t.Fatalf("debug.traceback({}) returned %v", L.Type(-1))
	}
	L.Pop(1)
}

func TestDebugGetInfo(t *testing.T) {
	L := newDebugState(t)
	src := "info = debug.getinfo(1)\n" +
		"src = info.source\n" +
		"ssrc = info.short_src\n" +
		"line = info.currentline\n" +
		"what = info.what\n" +
		"nups = info.nups\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "src"); got != "=chunk" {
		t.Fatalf("info.source = %q, want =chunk", got)
	}
	if got := dbGlobalString(t, L, "ssrc"); got != "chunk" {
		t.Fatalf("info.short_src = %q, want chunk", got)
	}
	if got := dbGlobalString(t, L, "what"); got != "main" {
		t.Fatalf("info.what = %q, want main", got)
	}
	L.GetGlobal("line")
	line, ok := L.ToInteger(-1)
	L.Pop(1)
	if !ok || line != 1 {
		t.Fatalf("info.currentline = %v (ok=%v), want 1", line, ok)
	}
	/* the main chunk has one upvalue, '_ENV' */
	L.GetGlobal("nups")
	nups, _ := L.ToInteger(-1)
	L.Pop(1)
	if nups != 1 {
		t.Fatalf("info.nups = %d, want 1", nups)
	}
}

func TestDebugGetInfoFunction(t *testing.T) {
	L := newDebugState(t)
	/* a Go (C) function */
	src := "i = debug.getinfo(print)\n" +
		"what = i.what\n" +
		"source = i.source\n" +
		"short = i.short_src\n" +
		"ld = i.linedefined\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "what"); got != "C" {
		t.Fatalf("getinfo(print).what = %q, want C", got)
	}
	if got := dbGlobalString(t, L, "source"); got != "=[C]" {
		t.Fatalf("getinfo(print).source = %q", got)
	}
	if got := dbGlobalString(t, L, "short"); got != "[C]" {
		t.Fatalf("getinfo(print).short_src = %q", got)
	}
	L.GetGlobal("ld")
	ld, _ := L.ToInteger(-1)
	L.Pop(1)
	if ld != -1 {
		t.Fatalf("getinfo(print).linedefined = %d, want -1", ld)
	}

	/* a Lua function: 'f', 'L' and 'u' options */
	src = "local function f(a, b) return a + b end\n" +
		"j = debug.getinfo(f, 'fLSu')\n" +
		"isfunc = type(j.func) == 'function'\n" +
		"islines = type(j.activelines) == 'table'\n" +
		"nparams = j.nparams\n" +
		"isvararg = j.isvararg\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("isfunc")
	if !L.ToBoolean(-1) {
		t.Fatal("getinfo(f, 'f').func is not a function")
	}
	L.Pop(1)
	L.GetGlobal("islines")
	if !L.ToBoolean(-1) {
		t.Fatal("getinfo(f, 'L').activelines is not a table")
	}
	L.Pop(1)
	L.GetGlobal("nparams")
	if n, _ := L.ToInteger(-1); n != 2 {
		t.Fatalf("getinfo(f).nparams = %d, want 2", n)
	}
	L.Pop(1)
}

func TestDebugGetLocal(t *testing.T) {
	L := newDebugState(t)
	src := "local function f(a)\n" +
		"  local xx = 42\n" +
		"  local n, v = debug.getlocal(1, 1)\n" +
		"  gname = n\n" +
		"  gvalue = v\n" +
		"  local n2, v2 = debug.getlocal(1, 2)\n" +
		"  gname2 = n2\n" +
		"  gvalue2 = v2\n" +
		"end\n" +
		"f(7)\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "gname"); got != "a" {
		t.Fatalf("debug.getlocal(1, 1) name = %q, want a", got)
	}
	L.GetGlobal("gvalue")
	if v, _ := L.ToInteger(-1); v != 7 {
		t.Fatalf("debug.getlocal(1, 1) value = %v, want 7", v)
	}
	L.Pop(1)
	if got := dbGlobalString(t, L, "gname2"); got != "xx" {
		t.Fatalf("debug.getlocal(1, 2) name = %q, want xx", got)
	}
	L.GetGlobal("gvalue2")
	if v, _ := L.ToInteger(-1); v != 42 {
		t.Fatalf("debug.getlocal(1, 2) value = %v, want 42", v)
	}
	L.Pop(1)

	/* out of range index: nil */
	if err := L.DoString("bad = debug.getlocal(1, 100)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("bad")
	if !L.IsNil(-1) {
		t.Fatalf("debug.getlocal(1, 100) = %v, want nil", L.Type(-1))
	}
	L.Pop(1)

	/* the parameter name of a function that is not running */
	if err := L.DoString("pn = debug.getlocal(f2, 1)"); err == nil {
		/* f2 is nil: getlocal of a non-function is an error */
		t.Fatal("debug.getlocal(nil, 1) did not raise an error")
	}
}

func TestDebugSetLocal(t *testing.T) {
	L := newDebugState(t)
	src := "local function f()\n" +
		"  local xx = 1\n" +
		"  sn = debug.setlocal(1, 1, 99)\n" +
		"  return xx\n" +
		"end\n" +
		"r = f()\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "sn"); got != "xx" {
		t.Fatalf("debug.setlocal returned %q, want xx", got)
	}
	L.GetGlobal("r")
	if v, _ := L.ToInteger(-1); v != 99 {
		t.Fatalf("after setlocal the local is %v, want 99", v)
	}
	L.Pop(1)
}

func TestDebugUpValues(t *testing.T) {
	L := newDebugState(t)
	src := "local upv = 7\n" +
		"function g() return upv end\n" +
		"n1, v1 = debug.getupvalue(g, 1)\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "n1"); got != "upv" {
		t.Fatalf("debug.getupvalue name = %q, want upv", got)
	}
	L.GetGlobal("v1")
	if v, _ := L.ToInteger(-1); v != 7 {
		t.Fatalf("debug.getupvalue value = %v, want 7", v)
	}
	L.Pop(1)

	/* setupvalue changes the upvalue (and the closure sees it) */
	src = "n2 = debug.setupvalue(g, 1, 99)\n" +
		"r = g()\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "n2"); got != "upv" {
		t.Fatalf("debug.setupvalue returned %q, want upv", got)
	}
	L.GetGlobal("r")
	if v, _ := L.ToInteger(-1); v != 99 {
		t.Fatalf("after setupvalue g() = %v, want 99", v)
	}
	L.Pop(1)

	/* an out of range index returns nothing */
	if err := L.DoString("cnt = select('#', debug.getupvalue(g, 5))"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("cnt")
	if v, _ := L.ToInteger(-1); v != 0 {
		t.Fatalf("debug.getupvalue(g, 5) returned %d values, want 0", v)
	}
	L.Pop(1)

	/* a Go function has upvalues with no name */
	if err := L.DoString("n3, v3 = debug.getupvalue(print, 1)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "n3"); got != "" {
		t.Fatalf("debug.getupvalue(print, 1) name = %q, want empty", got)
	}
}

func TestDebugMetaTable(t *testing.T) {
	L := newDebugState(t)
	src := "t = {}\n" +
		"mt = {}\n" +
		"r = debug.setmetatable(t, mt)\n" +
		"same = debug.getmetatable(t) == mt\n" +
		"none = debug.getmetatable(42)\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("same")
	if !L.ToBoolean(-1) {
		t.Fatal("debug.setmetatable/getmetatable do not agree")
	}
	L.Pop(1)
	L.GetGlobal("none")
	if !L.IsNil(-1) {
		t.Fatalf("debug.getmetatable(42) = %v, want nil", L.Type(-1))
	}
	L.Pop(1)
	/* setmetatable returns its first argument */
	L.GetGlobal("r")
	L.GetGlobal("t")
	if !L.RawEqual(-1, -2) {
		t.Fatal("debug.setmetatable did not return its first argument")
	}
	L.Pop(2)
}

func TestDebugGetRegistry(t *testing.T) {
	L := newDebugState(t)
	if err := L.DoString("r = debug.getregistry()"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("r")
	defer L.Pop(1)
	if !L.IsTable(-1) {
		t.Fatal("debug.getregistry did not return a table")
	}
	/* it is the registry itself: registry[1] is the main thread */
	L.GetI(-1, LUA_RIDX_MAINTHREAD)
	if !L.IsThread(-1) {
		t.Fatal("debug.getregistry()[1] is not the main thread")
	}
	L.Pop(1)
}

func TestDebugHook(t *testing.T) {
	L := newDebugState(t)
	/* deviation: the hook is recorded but never called, so setting one
	   must not disturb the program */
	src := "local count = 0\n" +
		"local function h() count = count + 1 end\n" +
		"debug.sethook(h, 'crl', 10)\n" +
		"hh, mm, cc = debug.gethook()\n" +
		/* rawequal and not '==': see the note about luaV_equalobj below */
		"ishook = rawequal(hh, h)\n" +
		"isnil = hh == nil\n" +
		/* the hook never runs: the loop must not be affected */
		"for i = 1, 10 do end\n" +
		"debug.sethook()\n" +
		"off = debug.gethook()\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := dbGlobalString(t, L, "mm"); got != "crl" {
		t.Fatalf("debug.gethook mask = %q, want crl", got)
	}
	L.GetGlobal("cc")
	if v, _ := L.ToInteger(-1); v != 10 {
		t.Fatalf("debug.gethook count = %v, want 10", v)
	}
	L.Pop(1)
	L.GetGlobal("ishook")
	if !L.ToBoolean(-1) {
		t.Fatal("debug.gethook did not return the hook function")
	}
	L.Pop(1)
	/* core bug (reported): luaV_equalobj in ldebug.go has no 'default:
	   gcvalue(t1) == gcvalue(t2)' branch, so '==' between two functions
	   is always false. 'hh == nil' must still be false. */
	L.GetGlobal("isnil")
	if L.ToBoolean(-1) {
		t.Fatal("debug.gethook() returned no hook")
	}
	L.Pop(1)
	/* after sethook() there is no hook at all */
	L.GetGlobal("off")
	if !L.IsNil(-1) {
		t.Fatalf("after debug.sethook() gethook = %v, want nil", L.Type(-1))
	}
	L.Pop(1)
}

func TestDebugMisc(t *testing.T) {
	L := newDebugState(t)
	/* getuservalue of a non-userdata is nil (or fail + false) */
	if err := L.DoString("uv = debug.getuservalue(42)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("uv")
	if !L.IsNil(-1) {
		t.Fatalf("debug.getuservalue(42) = %v, want nil", L.Type(-1))
	}
	L.Pop(1)

	/* setcstacklimit accepts a limit and answers with the current one */
	if err := L.DoString("lim = debug.setcstacklimit(100)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("lim")
	if _, ok := L.ToInteger(-1); !ok {
		t.Fatal("debug.setcstacklimit did not return a number")
	}
	L.Pop(1)

	/* debug.debug is not interactive here: it returns nothing */
	if err := L.DoString("cnt = select('#', debug.debug())"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("cnt")
	if v, _ := L.ToInteger(-1); v != 0 {
		t.Fatalf("debug.debug returned %d values, want 0", v)
	}
	L.Pop(1)
}

func TestDebugGetInfoBadOptions(t *testing.T) {
	L := newDebugState(t)
	/* '>' is not allowed as a user option */
	if err := L.DoString("debug.getinfo(1, '>S')"); err == nil {
		t.Fatal("debug.getinfo with '>' did not raise an error")
	}
	L.SetTop(0)
	/* an unknown option is invalid */
	err := L.DoString("debug.getinfo(1, 'Z')")
	if err == nil {
		t.Fatal("debug.getinfo with an invalid option did not raise an error")
	}
	if !strings.Contains(err.Error(), "invalid option") {
		t.Fatalf("error message = %q", err.Error())
	}
	L.SetTop(0)
	/* a level out of range returns nil */
	if err := L.DoString("out = debug.getinfo(100)"); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("out")
	if !L.IsNil(-1) {
		t.Fatalf("debug.getinfo(100) = %v, want nil", L.Type(-1))
	}
	L.Pop(1)
}
