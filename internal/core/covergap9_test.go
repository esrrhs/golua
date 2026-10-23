package core

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverGap9LuaHFree(t *testing.T) {
	L := NewState()
	defer L.Close()
	tbl := luaH_new(L)
	luaH_free(L, tbl)
}

func TestCoverGap9IOIsWindowsHook(t *testing.T) {
	orig := ioIsWindowsFn
	t.Cleanup(func() { ioIsWindowsFn = orig })

	ioIsWindowsFn = func() bool { return false }
	if ioCheckModeP("rb") || ioCheckModeP("rt") || ioCheckModeP("rbt") {
		t.Fatal("posix should reject rb/rt")
	}
	if !ioCheckModeP("r") || !ioCheckModeP("w") {
		t.Fatal("posix r/w")
	}
	if ioCheckModeP("") || ioCheckModeP("x") {
		t.Fatal("bad modes")
	}

	ioIsWindowsFn = func() bool { return true }
	if !ioCheckModeP("r") || !ioCheckModeP("w") {
		t.Fatal("win r/w")
	}
	if !ioCheckModeP("rb") || !ioCheckModeP("rt") || !ioCheckModeP("wb") || !ioCheckModeP("wt") {
		t.Fatal("win [rw][bt]")
	}
	if ioCheckModeP("rbt") || ioCheckModeP("rx") || ioCheckModeP("") {
		t.Fatal("win reject")
	}

	// ioShellCommand Windows arm
	cmd := ioShellCommand("echo hi")
	if cmd == nil || len(cmd.Args) < 2 {
		t.Fatal("shell")
	}
	ioIsWindowsFn = func() bool { return true }
	wcmd := ioShellCommand("echo hi")
	if wcmd.Path == "" && wcmd.Args[0] != "cmd" {
		// Args[0] should be cmd on Windows path
	}
	if len(wcmd.Args) < 3 || wcmd.Args[0] != "cmd" {
		t.Fatalf("windows shell args %#v", wcmd.Args)
	}
	ioIsWindowsFn = orig
	ucmd := ioShellCommand("echo hi")
	if len(ucmd.Args) < 3 || ucmd.Args[0] != "sh" {
		t.Fatalf("unix shell args %#v", ucmd.Args)
	}
}

func TestCoverGap9PkSetProgDirHook(t *testing.T) {
	origW, origE := pkIsWindowsFn, pkExecutableFn
	t.Cleanup(func() {
		pkIsWindowsFn = origW
		pkExecutableFn = origE
	})

	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// non-Windows: no-op
	pkIsWindowsFn = func() bool { return false }
	L.PushString("a!b")
	pkSetProgDir(L)
	if s, _ := L.ToString(-1); s != "a!b" {
		t.Fatalf("noop got %q", s)
	}
	L.Pop(1)

	// Windows happy path: rewrite !
	pkIsWindowsFn = func() bool { return true }
	pkExecutableFn = func() (string, error) {
		return filepath.Join(string(filepath.Separator)+"prog", "bin", "lua.exe"), nil
	}
	L.PushString("pre" + pkExecDir + "post")
	pkSetProgDir(L)
	got, _ := L.ToString(-1)
	dir := filepath.Join(string(filepath.Separator)+"prog", "bin")
	want := "pre" + dir + "post"
	if got != want {
		t.Fatalf("rewrite got %q want %q", got, want)
	}
	L.Pop(1)

	// Executable error
	pkExecutableFn = func() (string, error) { return "", errors.New("nope") }
	L.PushString("x!y")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected Error2 panic")
			}
		}()
		pkSetProgDir(L)
	}()
	L.SetTop(0)

	// empty / "." dir
	pkExecutableFn = func() (string, error) { return "lua.exe", nil } // Dir == "."
	L.PushString("x!y")
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected Error2 for empty dir")
			}
		}()
		pkSetProgDir(L)
	}()
	L.SetTop(0)
}

func TestCoverGap9MathCoshSinhSalvage(t *testing.T) {
	if haveLibm {
		t.Skip("salvage arms only under !haveLibm (nocgo)")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Value large enough that Sinh/Cosh overflow to Inf but half-exp may not.
	// On Go math, Sinh(~710+) overflows; try a range.
	for _, x := range []float64{700, 710, 720, 740, 800, 900, 1000} {
		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, "cosh")
		L.PushNumber(x)
		if L.PCall(1, 1, 0) != LUA_OK {
			t.Fatal(L.ToStringX(-1))
		}
		_, _ = L.ToNumber(-1)

		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, "sinh")
		L.PushNumber(x)
		if L.PCall(1, 1, 0) != LUA_OK {
			t.Fatal(L.ToStringX(-1))
		}
		_, _ = L.ToNumber(-1)

		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, "sinh")
		L.PushNumber(-x)
		if L.PCall(1, 1, 0) != LUA_OK {
			t.Fatal(L.ToStringX(-1))
		}
		_, _ = L.ToNumber(-1)
	}

	// Inf input: salvage condition !IsInf(x) is false
	for _, name := range []string{"cosh", "sinh"} {
		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, name)
		L.PushNumber(math.Inf(1))
		if L.PCall(1, 1, 0) != LUA_OK {
			t.Fatal(name, L.ToStringX(-1))
		}
	}

	// Direct half-exp path via mathCosh/mathSinh with finite overflow
	// Call the Go funcs directly with a known overflow finite.
	L.SetTop(0)
	L.PushNumber(1e308) // huge but finite; Cosh may Inf
	_ = mathCosh(L)
	L.SetTop(0)
	L.PushNumber(1e308)
	_ = mathSinh(L)
	L.SetTop(0)
	L.PushNumber(-1e308)
	_ = mathSinh(L)
}

func TestCoverGap9FuncNameFromCode(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Build a proto with many opcode kinds and call funcNameFromCode.
	ops := []int{
		OP_CALL, OP_TAILCALL, OP_TFORCALL,
		OP_SELF, OP_GETTABUP, OP_GETTABLE, OP_GETI, OP_GETFIELD,
		OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD,
		OP_MMBIN, OP_MMBINI, OP_MMBINK,
		OP_UNM, OP_BNOT, OP_LEN, OP_CONCAT, OP_EQ,
		OP_LT, OP_LTI, OP_GTI, OP_LE, OP_LEI, OP_GEI,
		OP_CLOSE, OP_RETURN,
		OP_MOVE, // default ""
	}
	p := &Proto{
		code: make([]Instruction, len(ops)),
	}
	for i, op := range ops {
		var ins Instruction
		ins.Op = op
		if op == OP_MMBIN || op == OP_MMBINI || op == OP_MMBINK {
			ins.C = int(TM_ADD) // any valid TM
		}
		ins.A = 0
		p.code[i] = ins
	}
	// ensure tm names exist
	_ = L.g.tmNames

	var name string
	for i := range ops {
		kind := funcNameFromCode(L, p, i, &name)
		_ = kind
	}
	// out of range
	if funcNameFromCode(L, p, -1, &name) != "" {
		t.Fatal("neg pc")
	}
	if funcNameFromCode(L, p, len(ops)+5, &name) != "" {
		t.Fatal("big pc")
	}

	// Also trigger via type errors on metamethod ops from Lua
	cases := []string{
		`local t=setmetatable({},{__index=function() error("ix") end}); return t[1]`,
		`local t=setmetatable({},{__newindex=function() error("nix") end}); t[1]=2`,
		`local t=setmetatable({},{__len=function() error("len") end}); return #t`,
		`local a=setmetatable({},{__unm=function() error("unm") end}); return -a`,
		`local a=setmetatable({},{__bnot=function() error("bn") end}); return ~a`,
		`local a=setmetatable({},{__add=function() error("add") end}); return a+1`,
		`local a=setmetatable({},{__eq=function() error("eq") end}); return a=={}`,
		`local a=setmetatable({},{__lt=function() error("lt") end}); return a<{}`,
		`local a=setmetatable({},{__le=function() error("le") end}); return a<={} `,
		`local a=setmetatable({},{__concat=function() error("cat") end}); return a..""`,
		`local a=setmetatable({},{__close=function() error("cl") end}); do local x <close> = a end`,
	}
	for _, src := range cases {
		L.SetTop(0)
		if err := L.LoadString(src, "=fn"); err != nil {
			t.Fatal(err)
		}
		_ = L.PCall(0, 0, 0) // expect error; still exercises naming
		L.SetTop(0)
	}
}

func TestCoverGap9CoRunClosing(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	L.PushGoFunc(func(L *State) int {
		L.PushInteger(1)
		return 1
	})
	L.SetGlobal("go1")

	// Create suspended async coroutine with to-be-closed, then set g.closing
	// and force-close so coRun closeSignal sees L.g.closing.
	if err := L.LoadString(`
return coroutine.create(function()
  local x <close> = setmetatable({}, {__close = function() error("should-skip") end})
  go1()
  coroutine.yield(1)
end)
`, "=cl"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	co := L.ToThread(-1)
	if co == nil {
		t.Fatal("thread")
	}
	// resume once to yield
	if err := L.LoadString(`local co=...; assert(coroutine.resume(co))`, "=r"); err != nil {
		t.Fatal(err)
	}
	L.Insert(-2)
	if L.PCall(1, 0, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}

	co.g.closing = true
	ok := co.forceCloseThread()
	_ = ok
	co.g.closing = false
}

func TestCoverGap9CapiFailArms(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("capi checks only under cgo+unix")
	}
	// Discover how many checks exist by running once and reading capiCheckN.
	capiResetChecks()
	dir := t.TempDir()
	if err := exerciseCAPI(dir); err != nil {
		t.Fatal(err)
	}
	n := capiCheckN
	if n < 10 {
		t.Fatalf("expected many checks, got %d", n)
	}
	// Force each check to fail once (early exit after first failure is fine).
	for i := 0; i < n; i++ {
		capiFailAt = i
		capiCheckN = 0
		err := exerciseCAPI(dir)
		capiResetChecks()
		if err == nil {
			// Some late checks may not be reached if earlier forced fail
			// changes control flow; still ok.
			continue
		}
		_ = fmt.Sprintf("%v", err)
	}
	capiResetChecks()
	// Final happy path still green
	if err := exerciseCAPI(dir); err != nil {
		t.Fatal(err)
	}
}

func TestCoverGap9DirectMathHalfExp(t *testing.T) {
	_ = mathHalfExp(1)
	_ = mathHalfExp(700)
	_ = mathHalfExp(1000)
}

func TestCoverGap9CoRunViaHook(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	L.PushGoFunc(func(L *State) int {
		panic("go-body-panic")
	})
	L.SetGlobal("gopanic")

	// Go function as coroutine body → promoteBody → coRun → recoverCo default
	if err := L.LoadString(`
local co = coroutine.create(gopanic)
local ok, err = coroutine.resume(co)
assert(not ok)
return tostring(err)
`, "=gb"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	if msg := L.ToStringX(-1); !strings.Contains(msg, "go-body-panic") {
		t.Fatalf("msg %q", msg)
	}
	L.Pop(1)

	// Hooked Lua body → promoteBody → coRun; yield then close → closeSignal
	if err := L.LoadString(`
debug.sethook(function() end, "l")
local co = coroutine.create(function()
  local x <close> = setmetatable({}, {__close = function() error("c") end})
  coroutine.yield(7)
  return 1
end)
local ok, v = coroutine.resume(co)
assert(ok and v == 7)
local okc, err = coroutine.close(co)
debug.sethook()
return tostring(okc), tostring(err)
`, "=hk"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 2, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	L.SetTop(0)

	// g.closing arm: hooked yield, then close while global state is closing
	if err := L.LoadString(`
debug.sethook(function() end, "l")
local co = coroutine.create(function()
  local x <close> = setmetatable({}, {__close = function() error("skip") end})
  coroutine.yield(1)
end)
assert(coroutine.resume(co))
debug.sethook()
return co
`, "=cl2"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	co := L.ToThread(-1)
	if co == nil {
		t.Fatal("thread")
	}
	L.Pop(1)
	co.g.closing = true
	_ = co.forceCloseThread()
	// also try coroutine.close path
	co.g.closing = false
}

func TestCoverGap9LuaReaderLoad(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("needs cgo")
	}
	// exerciseCAPI already calls lua_load with a C reader; this re-runs
	// the happy path alone so the reader arm is not masked by fail-at.
	capiResetChecks()
	if err := exerciseCAPI(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestCoverGap9RecoverCoRepanic(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.co = newCoRuntime()
	for _, sig := range []any{&yieldAbort{}, &migrateAbort{}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("expected re-panic")
				}
			}()
			L.recoverCo(sig)
		}()
	}
	// closing arm without a live body
	L.g.closing = true
	st, closed := L.recoverCo(&closeSignal{})
	if !closed || st != LUA_OK {
		t.Fatalf("closing %v %v", st, closed)
	}
	L.g.closing = false
	st, closed = L.recoverCo(&closeSignal{})
	_ = st
	_ = closed
	st, _ = L.recoverCo(&luaError{status: LUA_ERRRUN, value: Value{}})
	_ = st
	st, _ = L.recoverCo("plain")
	_ = st
}
