//go:build cgo && unix

package core

// Continuations: the last part of the C API that was missing.
//
// lua_callk, lua_pcallk and lua_yieldk each take a lua_KFunction, which
// C calls when the coroutine is resumed instead of returning into the
// frame that suspended -- because in PUC-Lua that frame is gone: the
// yield long-jumps out of it and its stack is thrown away.
//
// This port cannot throw a C frame away.  A yield parks the coroutine's
// goroutine where it stands (see (*State).Yield), so the C frame is
// still there when the resume comes and simply goes on -- which is what
// makes the continuation reachable at all: the frame that asked for one
// is the frame that gets to call it.
//
// The one thing that cannot be reproduced is what PUC-Lua tells the
// continuation of a 'lua_callk' about the status: C always passes
// LUA_YIELD there, and here the C function calls the continuation
// itself, having seen the call return normally.  No continuation can
// tell the difference, because for 'lua_callk' that status is a
// constant, so nothing observable depends on it.  For 'lua_pcallk' the
// status does vary, and it is reproduced (test 'a2' below).
//
// Every line of the expected output is PUC-Lua 5.4.9's, kept in
// testdata/continuation/expected.txt.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestContinuation(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/continuation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "k.c")); err != nil {
		t.Skipf("testdata/continuation is not there (%v)", err)
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	work := filepath.Join(dir, "test")
	for _, d := range []string{build, work} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"k.c", "test.lua"} {
		if err := copyFile(filepath.Join(src, f), filepath.Join(build, f)); err != nil {
			t.Fatal(err)
		}
	}
	args := []string{"-O2", "-std=c99", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(work, "k.so"), filepath.Join(build, "k.c")}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build the extension here (%v): %s", err, out)
	}
	if err := copyFile(filepath.Join(build, "test.lua"),
		filepath.Join(work, "test.lua")); err != nil {
		t.Fatal(err)
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		n := L.GetTop()
		var b strings.Builder
		for i := 1; i <= n; i++ {
			if i > 1 {
				b.WriteByte('\t')
			}
			b.WriteString(lTolstring(L, i))
		}
		got = append(got, strings.Split(b.String(), "\n")...)
		return 0
	})
	L.SetGlobal("print")

	/* The suite names the file in one of its messages ('H:test.lua:48:
	   oops'), so it has to run under the name the expected output was
	   made with. */
	t.Chdir(work)
	if err := L.DoString("package.cpath = " +
		quoteLuaString(filepath.Join(work, "?.so")) + ";"); err != nil {
		t.Fatalf("setting the search path: %v", err)
	}
	/* Loaded from the bytes with 'test.lua' as the chunk name, which is
	   what 'lua test.lua' calls it, rather than from the path: DoFile
	   would put the whole temporary directory in the message. */
	data, err := os.ReadFile(filepath.Join(work, "test.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if err := L.LoadBytes(data, "@test.lua"); err != nil {
		t.Fatalf("loading the suite: %v", err)
	}
	if status := L.PCall(0, LUA_MULTRET, 0); status != LUA_OK {
		t.Fatalf("running the suite: %s; got %q", valueToStr(&L.stack[L.top-1]), got)
	}

	b, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(string(b), "\n")
	if n := len(want); n > 0 && want[n-1] == "" {
		want = want[:n-1]
	}
	compareLines(t, got, want)
}
