package core_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	lua "github.com/esrrhs/golua"
)

// luacRun runs the compiler with the given argv and returns what it wrote
// to standard output and standard error, plus its exit status.
func luacRun(argv ...string) (out, err string, code int) {
	var o, e bytes.Buffer
	code = lua.Luac(append([]string{"luac"}, argv...), &o, &e)
	return o.String(), e.String(), code
}

// luacNorm replaces prototype addresses, which cannot be the same in two
// processes, with a placeholder. Everything else in a listing is
// determined by the source.
var luacPtr = regexp.MustCompile(`0x[0-9a-f]+`)

func luacNorm(s string) string { return luacPtr.ReplaceAllString(s, "0xPTR") }

// luacGolden reads one of the files PUC-Lua's luac wrote.  A Windows
// checkout turns their '\n' into '\r\n' (core.autocrlf), and what is
// being compared is what the compiler printed, so the carriage returns
// come out before the comparison rather than as a difference in every
// line of it.
func luacGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// TestLuacListing compares a full listing -- every instruction, every
// constant, every local and every upvalue of three chunks -- with what
// PUC-Lua's luac prints for the same sources. The three are compiled
// together, so the listing also covers the way several files are
// combined into one main chunk.
func TestLuacListing(t *testing.T) {
	dir := filepath.Join("testdata", "luac")
	t.Chdir(dir)
	want := luacGolden(t, "expected.txt")
	out, errOut, code := luacRun("-l", "-l", "f1.lua", "f2.lua", "f3.lua")
	if code != 0 {
		t.Fatalf("luac exited with %d: %s", code, errOut)
	}
	if errOut != "" {
		t.Fatalf("unexpected diagnostics: %s", errOut)
	}
	got := luacNorm(out)
	if got != string(want) {
		t.Errorf("listing differs from PUC-Lua's")
		gl := strings.Split(got, "\n")
		wl := strings.Split(string(want), "\n")
		for i := 0; i < len(gl) || i < len(wl); i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Errorf("line %d:\n ours: %s\n real: %s", i+1, g, w)
				break
			}
		}
	}
}

// TestLuacDumpBytes compiles the three chunks to a file and compares the
// bytes with the ones PUC-Lua's luac wrote, then loads the result back.
func TestLuacDumpBytes(t *testing.T) {
	dir := filepath.Join("testdata", "luac")
	t.Chdir(dir)
	for _, name := range []string{"combined.luac", "f1.luac"} {
		/* binary, and compared byte for byte: no normalising here */
		want, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(t.TempDir(), name)
		args := []string{"-o", out, "f1.lua", "f2.lua", "f3.lua"}
		if name == "f1.luac" {
			args = []string{"-o", out, "f1.lua"}
		}
		_, errOut, code := luacRun(args...)
		if code != 0 {
			t.Fatalf("luac exited with %d: %s", code, errOut)
		}
		got, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: %d bytes, PUC-Lua wrote %d", name, len(got), len(want))
		}
		/* the chunk has to load here too: a combined chunk is a main
		   chunk whose nested prototypes are the three files */
		L := lua.NewState()
		defer L.Close()
		L.OpenLibs()
		if err := L.LoadFile(out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if status := L.PCall(0, lua.LUA_MULTRET, 0); status != lua.LUA_OK {
			msg, _ := L.ToString(-1)
			t.Fatalf("%s: %s", name, msg)
		}
	}
}

// TestLuacParseOnly checks '-p': a good file compiles silently and a bad
// one is reported, with no output file written.
func TestLuacParseOnly(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile("ok.lua", []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("bad.lua", []byte("return 1 +\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := luacRun("-p", "ok.lua")
	if code != 0 || out != "" || errOut != "" {
		t.Errorf("'-p ok.lua' = (%q, %q, %d), want (\"\", \"\", 0)", out, errOut, code)
	}
	if _, err := os.Stat("luac.out"); !os.IsNotExist(err) {
		t.Errorf("'-p' wrote an output file: %v", err)
	}
	_, errOut, code = luacRun("-p", "bad.lua")
	if code != 1 {
		t.Errorf("'-p bad.lua' exited with %d, want 1", code)
	}
	const want = "luac: bad.lua:2: unexpected symbol near <eof>\n"
	if errOut != want {
		t.Errorf("'-p bad.lua' wrote %q, want %q", errOut, want)
	}
}

// TestLuacOptions covers the option handling: '-v', an unknown option, a
// missing input file and no input files at all.
func TestLuacOptions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	out, _, code := luacRun("-v")
	if code != 0 || out != lua.LUA_COPYRIGHT+"\n" {
		t.Errorf("'-v' = (%q, %d), want (%q, 0)", out, code, lua.LUA_COPYRIGHT+"\n")
	}
	_, errOut, code := luacRun("-z")
	if code != 1 || !strings.HasPrefix(errOut, "luac: unrecognized option '-z'\nusage:") {
		t.Errorf("'-z' = (%q, %d)", errOut, code)
	}
	_, errOut, code = luacRun("nope.lua")
	if code != 1 || !strings.HasPrefix(errOut, "luac: cannot open nope.lua: ") {
		t.Errorf("'nope.lua' = (%q, %d)", errOut, code)
	}
	_, errOut, code = luacRun()
	if code != 1 || !strings.HasPrefix(errOut, "luac: no input files given\nusage:") {
		t.Errorf("no arguments = (%q, %d)", errOut, code)
	}
	/* '-o' with no argument is the usage message too, and it must not
	   take the next option as the file name. */
	_, errOut, code = luacRun("-o")
	if code != 1 || !strings.HasPrefix(errOut, "luac: '-o' needs argument\nusage:") {
		t.Errorf("'-o' = (%q, %d)", errOut, code)
	}
}
