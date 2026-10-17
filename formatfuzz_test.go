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

// fmtChunk is what the generated cases sit in.  Besides 'q' and 'show'
// it carries the one case the grid cannot ask, and says so:
//
//	'tie' says of a %a precision and a value whether the conversion
//	lands exactly on a tie -- the first digit dropped is 8 and nothing
//	behind it is nonzero -- and how the reference's C library breaks
//	that tie.  glibc hands the last digit kept to 'round_away', which
//	rounds away from zero only when the digit is odd, so a tie always
//	lands on an even one; Apple's library rounds a tie *down* whatever
//	that digit is.  '%.0A' of 1.5 is 0X2P+0 under the first and
//	0X1P+0 under the second, and no port can answer both.
//
// This port answers 0X2P+0 -- the one glibc gives, the one the C
// library's own '%f' gives on every platform ('%.0f' of 1.5 is 2), and
// the one that makes '%a' agree with '%f' inside a single call.  So
// on a reference whose library answers 0X1P+0 the cases that turn on
// it are not asked on either side: SKIP is true, 'tie' counts them,
// and both sides print a dash.  They are still printed, so the count
// the comparison rests on is unchanged.
//
// Only a tie on an *odd* digit is undecided.  An even digit kept is
// where both rules land anyway -- to even is to stay, and down is to
// stay -- so those are compared like any other case.
//
// The chunk starts with SKIP because 'tie' has to see it, and a Lua
// local is only visible to functions written after it.
const fmtChunk = fuzzQuote + `local NSKIP = 0
local function isTie(prec, v)
  if prec < 0 or type(v) ~= "number" then return false end
  local frac = string.format("%a", math.abs(v)):match("0x1%.([0-9a-f]+)p")
  if not frac or #frac <= prec then return false end
  local tail = frac:sub(prec + 1)
  if tail:sub(1, 1) ~= "8" or tail:sub(2):find("[^0]") then return false end
  local last = (prec == 0) and "1" or frac:sub(prec, prec)
  return last:find("[13579bdf]") ~= nil
end
local function tie(prec, v)
  if SKIP and isTie(prec, v) then NSKIP = NSKIP + 1; return true end
  return false
end
local function show(ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  local s = table.concat(t, ",")
  if not ok then return "!" .. s end
  return s
end
`

// fmtLine runs one format against one value.  'guard' is the Lua
// expression that says whether this is a case the C libraries disagree
// about, or "" when it cannot be.
func fmtLine(i int, form, val, guard string) string {
	ans := "show(pcall(string.format, " + luaQuote(form) + ", " + val + "))"
	if guard != "" {
		ans = guard + " and \"-\" or " + ans
	}
	return "print(" + strconv.Itoa(i) + ", " + ans + ")\n"
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
	skip := fmtRefTiesDown(t, ref)

	var b strings.Builder
	b.WriteString("local SKIP = " + strconv.FormatBool(skip) + "\n")
	b.WriteString(fmtChunk)
	n := 0
	for _, f := range forms {
		/* Only '%a' and '%A' round in hexadecimal, so only they can
		   land on a tie. */
		prec := -1
		if f[len(f)-1] == 'a' || f[len(f)-1] == 'A' {
			prec = fmtPrecOf(f)
		}
		for _, v := range vals {
			n++
			guard := ""
			if prec >= 0 {
				guard = "tie(" + strconv.Itoa(prec) + ", " + v + ")"
			}
			b.WriteString(fmtLine(n, f, v, guard))
		}
		n++
		b.WriteString(fmtLineNoVal(n, f))
	}
	b.WriteString(`print("count", ` + strconv.Itoa(total) + ")\n")
	b.WriteString("print(\"skipped\", NSKIP)\n")

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
		t.Logf("%d formats against %d values, %d answers, all of them PUC-Lua's%s",
			len(forms), len(vals), total, fmtSkipped(got))
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
	return fmtRunFile(t, bin, dir, "fmtfuzz.lua")
}

func fmtRunFile(t *testing.T, bin, dir, name string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", name},
		Dir:  dir,
		Env:  diffEnv(),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// fmtRefTiesDown asks the reference which C library it was built
// against, by asking the one question the two answer differently.
//
// 'string.format("%.0A", 1.5)' is exactly a tie: 1.5 is 0x1.8p+0, so
// what is being dropped is precisely half of the last digit kept.
// glibc -- and, going by its own '%f', every library but one -- rounds
// a tie to even, which here means up, to 0X2P+0.  Apple's library
// rounds a tie down whatever the last digit is, and answers 0X1P+0.
func fmtRefTiesDown(t *testing.T, ref string) bool {
	t.Helper()
	dir := t.TempDir()
	const name = "tieprobe.lua"
	const src = `print(string.format("%.0A", 1.5))` + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	switch out := strings.TrimSpace(fmtRunFile(t, ref, dir, name)); out {
	case "0X1P+0":
		return true /* Apple's */
	case "0X2P+0":
		return false /* glibc's, and this port's */
	default:
		t.Fatalf("the reference answered %q to '%%.0A' of 1.5, which is neither "+
			"0X1P+0 (a library that rounds a tie down) nor 0X2P+0 (one that "+
			"rounds it to even); how it breaks a %%a tie has to be known "+
			"before the grid can be compared", out)
		return false
	}
}

// fmtSkipped reports how many of the grid's answers were not asked,
// and why, for the log line.
func fmtSkipped(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if n, ok := strings.CutPrefix(line, "skipped\t"); ok && n != "0" {
			return ", " + n + " of them not asked: a %a tie, which this " +
				"port rounds to even and the reference's C library rounds down"
		}
	}
	return ""
}

// fmtPrecOf reads the precision out of a format string: '.', '.0' and
// no precision at all are the three ways of writing the ones that can
// land on a tie.  -1 is "no precision", which in '%a' means the
// shortest representation, which drops nothing and so never ties.
func fmtPrecOf(form string) int {
	i := strings.IndexByte(form, '.')
	if i < 0 {
		return -1
	}
	s := strings.TrimRight(form[i+1:], "aA")
	if s == "" {
		return 0 /* "%." and "%.a" are both a precision of zero */
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}
