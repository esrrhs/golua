package lua

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The third fuzz, over the one corner of the standard library that is
// not translated from Lua's own C but written against C's printf and
// then re-expressed: 'string.format'.  PUC-Lua hands the whole
// conversion to the platform's sprintf, so what it answers is what
// glibc or libSystem answers, and a port has to reproduce that from
// scratch -- strParseFormat, strPadField, strFormatUnsigned and the
// float path in lstrlib.go.  Widths, precisions, the four flags and
// eighteen conversions have to agree in every combination, and no
// hand-written case covers a fraction of them.
//
// testdata/diff has a handful of formats; fuzz_test.go has thirty.
// This is the grid: 11 340 format strings, each against eighteen
// values, and each with no value at all.

// fmtFlags are the flags C's printf knows, and the combinations of
// them that actually do something: '-0' is zero-padding with a
// left-justified field, '+#' is a forced sign on an alternate form.
var fmtFlags = []string{
	"", "-", "+", " ", "0", "#",
	"-0", "+0", " 0", "#0", "-+", "-#", "0#", "+#", "-0#",
}

var fmtWidths = []string{"", "1", "2", "5", "10", "20"}

// fmtPrecs includes ".", which is a precision of zero written without
// the digit, and a precision wider than any value here.
var fmtPrecs = []string{"", ".", ".0", ".1", ".3", ".8", ".20"}

var fmtConvs = []string{
	"d", "i", "u", "o", "x", "X",
	"e", "E", "f", "F", "g", "G", "a", "A",
	"c", "s", "q", "%",
}

// fmtVals is what the formats are asked to convert: integers at both
// ends of the range, floats that exercise rounding and the two
// spellings of zero, the three that are not numbers, strings, and a
// boolean -- because what 'format' says when handed a boolean for '%d'
// is part of what is being compared.
//
// There is deliberately no table here.  '%20s' with one is a case
// that cannot be compared: tostring of a table is its address, the
// width is padded to the length that address happens to have, and
// the normaliser rewrites the address *after* the padding was
// decided -- so PUC-Lua answers " table: 0xADDR" and this port
// answers "table: 0xADDR", the difference being how many hex digits
// each one's heap addresses have.  Nothing about Lua is being asked.
var fmtVals = []string{
	"0", "1", "-1", "255", "-255", "math.maxinteger", "math.mininteger",
	"0.0", "-0.0", "1.5", "-1.5", "0.1", "1/3", "1e300", "1e-300",
	"math.huge", "-math.huge", "(0/0)",
	`""`, `"hello"`, "\"" + "a\\0b" + "\"", "true",
}

// fmtStrings is every format the grid describes.
func fmtStrings() []string {
	out := make([]string, 0, len(fmtFlags)*len(fmtWidths)*len(fmtPrecs)*len(fmtConvs))
	for _, c := range fmtConvs {
		for _, f := range fmtFlags {
			for _, w := range fmtWidths {
				for _, p := range fmtPrecs {
					out = append(out, "%"+f+w+p+c)
				}
			}
		}
	}
	return out
}

const fmtChunk = fuzzQuote + `local function show(ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  local s = table.concat(t, ",")
  if not ok then return "!" .. s end
  return s
end
`

// fmtLine runs one format against one value, and then against nothing
// at all -- "no value" being its own answer, and its own error.
func fmtLine(i int, form, val string) string {
	return "print(" + strconv.Itoa(i) +
		", show(pcall(string.format, " + luaQuote(form) + ", " + val + ")))\n"
}

func fmtLineNoVal(i int, form string) string {
	return "print(" + strconv.Itoa(i) +
		", show(pcall(string.format, " + luaQuote(form) + ")))\n"
}

func TestFormatFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the format fuzz")
	}
	forms := fmtStrings()
	vals := fmtVals
	/* One line per format and value, then one per format with no
	   value at all. */
	perForm := len(vals) + 1
	total := len(forms) * perForm

	var b strings.Builder
	b.WriteString(fmtChunk)
	n := 0
	for _, f := range forms {
		for _, v := range vals {
			n++
			b.WriteString(fmtLine(n, f, v))
		}
		n++
		b.WriteString(fmtLineNoVal(n, f))
	}
	b.WriteString(`print("count", ` + strconv.Itoa(total) + ")\n")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fmtfuzz.lua"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fmtRun(t, ref, dir)
	got := fmtRun(t, diffBuild(t), dir)
	/* As in the other two: a chunk that does not load prints the same
	   syntax error on both sides, so the count is asserted first. */
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(total)) {
			t.Fatalf("the format chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d formats against %d values, %d answers, all of them PUC-Lua's",
			len(forms), len(vals), total)
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	reported := 0
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var a, b string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			b = gl[i]
		}
		if a == b {
			continue
		}
		reported++
		if reported > 20 {
			t.Logf("... and more")
			break
		}
		what := "?"
		if i < total {
			/* the last line of each format's block is the one with
			   no value */
			if k := i % perForm; k < len(vals) {
				what = "format " + forms[i/perForm] + " with " + vals[k]
			} else {
				what = "format " + forms[i/perForm] + " with no value"
			}
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", total)
	}
}

func fmtRun(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "fmtfuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}
