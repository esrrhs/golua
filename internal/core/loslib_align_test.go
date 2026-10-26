package core

import (
	"os"
	"runtime"
	"testing"
	"time"
)

// TestOSClockCPUTime checks that os.clock tracks process CPU time on
// Unix (Getrusage), not wall time: a sleep advances wall far more than
// clock, matching C clock() semantics.
func TestOSClockCPUTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.clock is wall-since-start on Windows (SPEC; no Getrusage)")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local n = 0
		for i = 1, 3e6 do n = n + i end
		_G.t_burn = os.clock()
		assert(_G.t_burn > 0)
	`); err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t_burn")
	tBurn, _ := L.ToNumber(-1)
	L.Pop(1)
	time.Sleep(time.Second)
	if err := runLua(t, L, `_G.t_after = os.clock()`); err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("t_after")
	tAfter, _ := L.ToNumber(-1)
	grew := tAfter - tBurn
	if grew >= 0.25 {
		t.Fatalf("os.clock grew by %v over 1s wall sleep; want CPU time not wall", grew)
	}
	t.Logf("CPU clock after burn=%v after 1s sleep=%v grew=%v", tBurn, tAfter, grew)
}

// TestOSGetEnvLuaCStr checks that getenv stops at an embedded NUL the
// way C getenv(luaL_checkstring) does.
func TestOSGetEnvLuaCStr(t *testing.T) {
	const key = "GOLUA_LOS_ALIGN_ENV"
	if err := os.Setenv(key, "yes"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv(key) })

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.GetGlobal("os")
	L.GetField(-1, "getenv")
	L.PushString(key + "\x00BAR")
	if st := L.PCall(1, 1, 0); st != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("getenv pcall: %v %s", st, msg)
	}
	got, ok := L.ToString(-1)
	if !ok || got != "yes" {
		t.Fatalf("getenv with embedded NUL: got %q ok=%v, want %q", got, ok, "yes")
	}
}

// TestOSDateStrftimeCLocale pins a few libc-sensitive specs at UTC
// epoch 0 against the cgo strftime hook.
func TestOSDateStrftimeCLocale(t *testing.T) {
	if osStrftimeC == nil {
		t.Skip("no cgo strftime on this build")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	for _, spec := range []string{"%c", "%x", "%X", "%Y", "%Z"} {
		want, ok := osStrftimeC(0, true, spec)
		if !ok {
			t.Fatalf("libc strftime %s failed", spec)
		}
		if err := L.LoadString("return os.date('!"+spec+"', 0)", "=date"); err != nil {
			t.Fatal(err)
		}
		if st := L.PCall(0, 1, 0); st != LUA_OK {
			msg, _ := L.ToString(-1)
			t.Fatalf("os.date: %v", msg)
		}
		got, _ := L.ToString(-1)
		L.Pop(1)
		if got != want {
			t.Fatalf("os.date(!%s,0)=%q, libc=%q", spec, got, want)
		}
	}
}
