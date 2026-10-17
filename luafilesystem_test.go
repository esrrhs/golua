//go:build cgo && unix

package lua

// LuaFileSystem, the fourth real C extension run here.  Where the first
// three stay inside the interpreter, this one reaches into a file handle
// the interpreter made: lfs.setmode casts the userdata it is handed to a
// luaL_Stream and reads the FILE* and the close function out of it.
//
// That found two things, and the first of them is the reason this file
// exists:
//
//   luaL_testudata and lua_touserdata answered NULL for a userdata made
//   in Go -- which is what a file handle is -- so C was handed nothing
//   and read 8 bytes past it.  It did not fail, it died.  Every
//   userdata now has a block to stand for it, made the first time C
//   asks, because PUC-Lua answers with an address for any full userdata
//   and never with NULL.
//
//   The block was zeroed, so a handle looked closed.  An io file is a Go
//   *os.File and has no FILE*, so the block is now filled in the way
//   luaL_Stream is laid out -- a real FILE* on the same descriptor, and
//   a close function that is only an address -- and cleared when the
//   file is closed, which is how C sees a closed handle in PUC-Lua too.
//
// It also needed include/lualib.h, which did not exist: an extension
// includes it as a matter of course, and lfs.c would not compile without
// it.  Nor did lua.h define LUA_API, LUALIB_API or LUAMOD_API, which
// PUC-Lua's luaconf.h does and an extension declaring its own luaopen_
// uses.
//
// The suite writes into the directory it runs in, so it is run in a
// temporary one.  Every line it prints is PUC-Lua 5.4.9's, kept in
// testdata/luafilesystem/expected.txt.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaFileSystem(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/luafilesystem")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "lfs.c")); err != nil {
		t.Skipf("testdata/luafilesystem is not there (%v)", err)
	}
	inc, err := filepath.Abs("include")
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
	for _, f := range []string{"lfs.c", "lfs.h"} {
		if err := copyFile(filepath.Join(src, f), filepath.Join(build, f)); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyFile(filepath.Join(src, "tests", "test.lua"),
		filepath.Join(work, "test.lua")); err != nil {
		t.Fatal(err)
	}

	args := []string{"-O2", "-std=c99", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(work, "lfs.so"), filepath.Join(build, "lfs.c")}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build lfs here (%v): %s", err, out)
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	/* The suite writes each case's progress with io.write -- a dot at a
	   time -- and the verdict with print, so the two meet on one line of
	   the real output: '.............Ok!'.  print is captured here, so
	   io.write is made to leave what it is given in __pend for print to
	   pick up: a line is only complete when print ends it, exactly as
	   the newline is what ends it on a terminal. */
	var got []string
	L.PushGoFunc(func(L *State) int {
		L.GetGlobal("__pend")
		pend, _ := L.ToString(-1)
		L.Pop(1)
		L.PushString("")
		L.SetGlobal("__pend")
		n := L.GetTop()
		var b strings.Builder
		b.WriteString(pend)
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

	/* The suite writes into the directory it runs in -- files, links,
	   directories -- so it runs in a temporary one of its own. */
	t.Chdir(work)
	if err := L.DoString("arg = {}\n" +
		"__pend = ''\n" +
		"io.write = function(...)\n" +
		"  local t = {}\n" +
		"  for i = 1, select('#', ...) do t[i] = tostring((select(i, ...))) end\n" +
		"  __pend = __pend .. table.concat(t)\n" +
		"end\n" +
		"package.cpath = " + quoteLuaString(filepath.Join(work, "?.so")) + ";"); err != nil {
		t.Fatalf("setting the search path: %v", err)
	}
	if err := L.DoFile(filepath.Join(work, "test.lua")); err != nil {
		t.Fatalf("running lfs's own suite: %v; got %q", err, got)
	}

	compareLines(t, got, lfsWant(t, src))
}

func lfsWant(t *testing.T, src string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}
