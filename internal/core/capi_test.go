//go:build cgo && unix

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCApiSurface runs testdata/capi, a C extension written to touch
// the parts of the C API that the other extensions in testdata/ happen
// not to reach -- the stack shuffles, lua_arith, every get/set pair,
// luaL_ref, the buffer API, lua_toclose, a C hook, the debug API,
// coroutines driven from C and luaL_Stream -- and compares each line
// with what PUC-Lua 5.3.4 answered for the same extension.
//
// It is the same arrangement as the other extension tests: the C file is
// compiled against include/, and the driver is loaded under the chunk
// name "cext" so that a "cext:35" in an error message is stable.
func TestCApiSurface(t *testing.T) {
	if !pkHasDynLoad() {
		t.Skip("built without cgo, so there is no dlopen")
	}
	src := filepath.Join("testdata", "capi")
	dir := t.TempDir()

	data, err := os.ReadFile(filepath.Join(src, "capi.c"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "capi.c"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !compileCExt(t, dir, "capi") {
		return
	}

	driver, err := os.ReadFile(filepath.Join(src, "driver.lua"))
	if err != nil {
		t.Fatal(err)
	}
	got := runCExtDriver(t, dir, string(driver))

	want, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(want), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	compareLines(t, got, lines)
}
