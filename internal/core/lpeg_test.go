//go:build cgo && unix

package core

// LPeg is a real C extension, not one written for this test: six
// translation units, published and used far away from here. It is worth
// having because it reaches for the parts of the C API that a small test
// extension never does, and so it is the thing that finds what a test
// extension cannot:
//
//   - lua_rawlen to read back the size of a block it was given, which is
//     how it knows how big a pattern is -- a userdata that answers the
//     wrong number makes it allocate nothing and copy past the end
//   - lua_getallocf, and then a call through the pointer it gets, to
//     compile a pattern
//   - luaL_Buffer, written through luaL_addchar and through the address
//     luaL_prepbuffsize hands out, for substitution captures
//   - match-time captures, which call back into Lua from C in the middle
//     of a match, and so count on the stack being exactly as deep as the
//     C API says it is
//   - userdata carrying a metatable, a user value, and a C block
//
// What it runs is LPeg's own test suite, and every line that suite prints
// was captured from PUC-Lua 5.3.4 running the same extension, so this is
// a comparison against the reference interpreter and not against what
// this port happened to print one day.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lpegSources are the translation units of lpeg.so, in the order
// LPeg's own makefile lists them.
var lpegSources = []string{
	"lpvm.c", "lpcap.c", "lptree.c", "lpcode.c", "lpprint.c", "lpcset.c",
}

var lpegHeaders = []string{
	"lpcap.h", "lpcode.h", "lpcset.h", "lpprint.h", "lptree.h", "lptypes.h",
	"lpvm.h",
}

func TestLPeg(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/lpeg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "test.lua")); err != nil {
		t.Skipf("testdata/lpeg is not there (%v)", err)
	}

	dir := t.TempDir()
	names := make([]string, 0, len(lpegSources))
	for _, f := range lpegHeaders {
		if err := copyFile(filepath.Join(src, f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range lpegSources {
		if err := copyFile(filepath.Join(src, f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
		names = append(names, filepath.Join(dir, f))
	}
	for _, f := range []string{"test.lua", "re.lua"} { /* the suite, and the
		module it tests at the end */
		if err := copyFile(filepath.Join(src, f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	lpegAskForACycle(t, filepath.Join(dir, "test.lua"))

	inc, err := filepath.Abs("../../include")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-O2", "-std=c99", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(dir, "lpeg.so")}
	args = append(args, names...)
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build lpeg here (%v): %s", err, out)
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		n := L.GetTop()
		parts := make([]string, 0, n)
		for i := 1; i <= n; i++ {
			parts = append(parts, lTolstring(L, i))
		}
		got = append(got, strings.Join(parts, "\t"))
		return 0
	})
	L.SetGlobal("print")

	if err := L.DoString("package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) +
		"\npackage.path = " + quoteLuaString(filepath.Join(dir, "?.lua")) + ";" + ";"); err != nil {
		t.Fatalf("setting the search paths: %v", err)
	}
	if err := L.DoFile(filepath.Join(dir, "test.lua")); err != nil {
		t.Fatalf("running lpeg's own suite: %v", err)
	}

	want := lpegWant(t, src)
	compareLines(t, got, want)
}

// lpegAskForACycle rewrites the one place in LPeg's own suite that asks
// for a collection.
//
// 'test.lua' asserts, inside a match-time capture, that 'collectgarbage'
// has swept a weak table.  There is no collector here to ask for one --
// the objects are Go's and Go reclaims them -- so the line becomes what
// does drive a sweep in this port: allocating until the step fires (see
// gcStep).  Nothing the suite prints changes, which is why the golden
// output captured from PUC-Lua still applies.
func lpegAskForACycle(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const ask = "    collectgarbage();\n"
	const cycle = "    for i = 1, 200000 do local a = {} end   -- a cycle: see lgc.go\n"
	if !strings.Contains(string(b), ask) {
		t.Fatal("lpeg's test.lua no longer asks for a collection where it did")
	}
	out := strings.Replace(string(b), ask, cycle, 1)
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lpegWant reads the lines PUC-Lua printed, which are kept beside the
// sources so that this test needs neither the reference interpreter nor
// the network.
func lpegWant(t *testing.T, src string) []string {
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

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o644)
}
