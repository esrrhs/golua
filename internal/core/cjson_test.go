//go:build cgo && unix

package core

// Lua CJSON, like LPeg, is a real C extension: three translation units,
// published and used far away from here, run here through its own 105-case
// suite. Where LPeg exercises patterns, userdata and the buffer API, CJSON
// exercises the other half of what an extension does -- encoding and
// decoding, walking tables with lua_next, converting numbers and strings
// back and forth, and error messages built with luaL_error -- and it is the
// one that found the locale:
//
//   Its suite sets os.setlocale("cs_CZ") to force a decimal comma, and
//   checks what a number then looks like. os.setlocale used to answer nil
//   for everything but "C", which is also what kept the official suite's
//   literals.lua from running its decimal-point tests.
//
// Two kinds of line are left out of the comparison, and both are left out
// on purpose:
//
//   - a line that prints a table with string keys. PUC-Lua's pairs walks a
//     table in hash order and the hash is seeded afresh in every process,
//     so the reference interpreter prints these two ways on two runs of
//     its own. The cases that produce them still count: their PASS or FAIL
//     is compared.
//   - the one line that carries the module's own version number.
//
// And one line is *rewritten* when the machine cannot run it as written.
// The golden was captured where the C library has cs_CZ, so two lines
// print a number with a decimal comma; a host that does not have that
// locale installed -- a stock glibc image generates only C and en_US --
// answers '.' there, and the port, which asks the same C library, answers
// '.' too.  Whether the locale is installed is the host's business, so
// the golden is made to agree with it rather than the other way round.
//
// The suite wants a 4 MB utf8.dat, which its genutf8.pl builds with perl.
// Writing it here instead keeps the test to things the machine certainly
// has -- a C compiler -- and it is the same bytes: every codepoint except
// the surrogates, in UTF-8.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var cjsonSources = []string{"lua_cjson.c", "strbuf.c", "fpconv.c"}

var cjsonHeaders = []string{"strbuf.h", "fpconv.h"}

var cjsonData = []string{
	"test.lua", "agentzh.t", "types.json", "numbers.json",
	"example1.json", "example2.json", "example3.json", "example4.json",
	"example5.json", "rfc-example1.json", "rfc-example2.json",
	"octets-escaped.dat",
}

func TestCJSON(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/cjson")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "tests", "test.lua")); err != nil {
		t.Skipf("testdata/cjson is not there (%v)", err)
	}

	dir := t.TempDir()
	work := filepath.Join(dir, "tests")
	if err := os.MkdirAll(filepath.Join(work, "cjson"), 0o755); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(cjsonSources))
	for _, f := range cjsonHeaders {
		if err := copyFile(filepath.Join(src, f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range cjsonSources {
		dst := filepath.Join(dir, f)
		if err := copyFile(filepath.Join(src, f), dst); err != nil {
			t.Fatal(err)
		}
		names = append(names, dst)
	}
	for _, f := range cjsonData {
		if err := copyFile(filepath.Join(src, "tests", f), filepath.Join(work, f)); err != nil {
			t.Fatal(err)
		}
	}
	/* cjson.util, which the suite requires */
	if err := copyFile(filepath.Join(src, "tests", "cjson", "util.lua"),
		filepath.Join(work, "cjson", "util.lua")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "utf8.dat"), utf8Dat(), 0o644); err != nil {
		t.Fatal(err)
	}

	inc, err := filepath.Abs("include")
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-O2", "-std=c99", "-DNDEBUG", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(work, "cjson.so")}
	args = append(args, names...)
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build cjson here (%v): %s", err, out)
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		/* print, exactly: the arguments separated by tabs, then a newline.
		   Splitting on the newlines inside them too is what makes the
		   count come out right -- one of the suite's own lines is built
		   with a trailing "\n" and so prints as two. */
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

	/* The suite reads its data files -- utf8.dat, octets-escaped.dat, the
	   .json files -- by relative name, as its own runtests.sh does. */
	t.Chdir(work)

	/* 'arg' is the interpreter's, not the language's: the suite walks it
	   at the end for extra files to check, and a bare state has none --
	   an empty table is what running it with no arguments gives. */
	if err := L.DoString("arg = {}\n" +
		"package.cpath = " + quoteLuaString(filepath.Join(work, "?.so")) +
		"\npackage.path = " + quoteLuaString(filepath.Join(work, "?.lua")) + ";"); err != nil {
		t.Fatalf("setting the search paths: %v", err)
	}
	if err := L.DoFile(filepath.Join(work, "test.lua")); err != nil {
		t.Fatalf("running cjson's own suite: %v", err)
	}

	compareLines(t, stableLines(got), stableLines(cjsonWant(t, src)))
}

// stableLines drops the lines that cannot be compared: the ones printing a
// table with string keys, whose order PUC-Lua does not fix, and the one
// naming the module's version, which is CJSON's and not this port's.  It
// also spells a NaN the one way: its sign is the platform's to print --
// glibc says "-nan" for what macOS says is "nan" (README §6.7) -- and
// 'tostring(0/0)' is not the subject of this test.
func stableLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.Contains(l, `["`) || strings.Contains(l, "Lua CJSON version") {
			continue
		}
		out = append(out, strings.ReplaceAll(l, "-nan", "nan"))
	}
	return out
}

func cjsonWant(t *testing.T, src string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if !cjsonHasCommaLocale() {
		/* see the note at the top: a decimal comma is a comma with a
		   digit on either side of it, and nothing else in the golden
		   writes one */
		re := regexp.MustCompile(`([0-9]),([0-9])`)
		for i, l := range lines {
			lines[i] = re.ReplaceAllString(l, "$1.$2")
		}
	}
	return lines
}

// cjsonHasCommaLocale asks the C library whether it has the locale the
// suite sets, rather than assuming it from the platform: which locales are
// installed is a property of the machine, exactly as it is in PUC-Lua.
//
// The question is asked by setting the locale, and a locale is not a thing
// a state owns but a thing the process owns -- it is C's, and the decimal
// point every later number is written with comes from it.  So it is put
// back before the answer is given: without that, the whole package prints
// "8,2184074615549724e+307" from here on.
func cjsonHasCommaLocale() bool {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	const probe = `
		local was = os.setlocale()
		local had = os.setlocale("cs_CZ")
		os.setlocale(was)
		return had
	`
	if err := L.LoadString(probe, "=locale"); err != nil {
		return false
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		return false
	}
	s, ok := L.ToString(-1)
	return ok && s != ""
}

// utf8Dat is what genutf8.pl writes: every codepoint but the surrogates,
// encoded in UTF-8 one after another.
func utf8Dat() []byte {
	var b bytes.Buffer
	b.Grow(4 << 20)
	for cp := rune(0); cp <= 0xD7FF; cp++ {
		b.WriteRune(cp)
	}
	for cp := rune(0xE000); cp <= 0x10FFFF; cp++ {
		b.WriteRune(cp)
	}
	return b.Bytes()
}
