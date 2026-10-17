package lua

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The fourth fuzz, over the two conversions between a number and a
// string: 'tonumber', which has to decide what a Lua numeral is, and
// 'tostring' and '%q', which have to write one back.  Both halves are
// written against the C library -- 'l_str2d' is a strtod, and '%.14g'
// is a printf -- so both are reproductions rather than translations,
// which is the reason this file exists.
//
// A probe of sixty strings was enough to find the first two
// differences, which is how this corner was chosen:
//
//	'1_000' was 1000 here and nil in PUC-Lua, because Go's ParseFloat
//	accepts the underscores of a Go literal and C's strtod does not.
//
//	'"\u{3000}1"' was 1 here and nil there, because strings.TrimSpace
//	trims the Unicode spaces and C's isspace does not.
//
// Both are fixed; what is left is the grid.

// numSigns, numInts, numFracs and numExps are the pieces of a decimal
// numeral.  Every combination is asked, including the ones that cannot
// be a number: "" is not, and neither is "." nor "e5".
var numSigns = []string{"", "+", "-"}
var numInts = []string{"", "0", "1", "9", "10", "007", "42", "1234567890"}
var numFracs = []string{"", ".", ".0", ".5", ".123", ".0000001"}
var numExps = []string{"", "e", "e5", "E5", "e+5", "e-5", "e-", "e+"}

// numHex are the pieces of a hexadecimal numeral, which C's strtod
// takes and Go's ParseFloat only takes with an exponent.
var numHexMants = []string{"", "1", "f", "F", "1.8", ".8", "8.", "0",
	"deadbeef", "ffffffffffffffff", "1000000000000000000000000000000"}
var numHexExps = []string{"", "p0", "p-2", "P+3", "p", "p+", "p-", "p1000000"}

// numSpaces are the bytes a numeral may be wrapped in.  The first six
// are the ones C's isspace says are space; the last three are Unicode
// spaces, which look like the others and are not one.
var numSpaces = []string{" ", "\t", "\n", "\v", "\f", "\r",
	"\u00a0", "\u2003", "\u3000"}

// numOdds are the strings that are not pieces of anything: what a
// reader would try before giving up on a numeral.
var numOdds = []string{
	"", " ", "abc", "1a", "--", "++", "1-1", "0b101", ".", "e5", "1e",
	"inf", "nan", "infinity", "NAN", "Infinity", "0x", "0x.", "1..2",
	"1.2.3", "1e5e5", "1_000", "1_0.5", "1e1_0", "0x1_p0", "0x_1p0",
	"_1", "1_", "1__0", "0x_1", "1_000_", "0x", "0X", "--1", "++1",
	"+-1", "0x1p", "1e400", "1e-400", "-1e400", "9223372036854775808",
	"-9223372036854775809", "0xffffffffffffffffff", "1e1000000000000000000",
	"1e-1000000000000000000", "0.000000000000000000001",
}

// numStrings is every string 'tonumber' is asked about.
func numStrings() []string {
	out := make([]string, 0, 2000)
	for _, sg := range numSigns {
		for _, i := range numInts {
			for _, f := range numFracs {
				for _, e := range numExps {
					out = append(out, sg+i+f+e)
				}
			}
		}
	}
	for _, sg := range numSigns {
		for _, p := range []string{"0x", "0X"} {
			for _, m := range numHexMants {
				for _, e := range numHexExps {
					out = append(out, sg+p+m+e)
				}
			}
		}
	}
	for _, base := range []string{"1", "0x10", "-1.5", "1e5"} {
		for _, sp := range numSpaces {
			out = append(out, sp+base, base+sp, sp+base+sp)
		}
	}
	return append(out, numOdds...)
}

// numBases are the bases 'tonumber' takes a second argument for.
var numBases = []string{"", ", 16", ", 10", ", 2", ", 36"}

// numVals is what the other half converts back into a string.
var numVals = []string{
	"0", "-0.0", "1", "-1", "2", "1.5", "-1.5", "0.1", "0.5", "1/3", "2/3",
	"1e-5", "1e-4", "1e14", "1e15", "1e16", "1e17", "1e21", "1e-300", "1e300",
	"5e-324", "2.2250738585072014e-308", "3.141592653589793",
	"9007199254740992", "9007199254740993", "9223372036854775807",
	"-9223372036854775808", "123456789012345", "0.000001", "0.0000001",
	"100.0", "1e308", "1e-308", "math.huge", "-math.huge", "(0/0)",
	"0x10", "0x1p-1074", "123.456", "-123.456", "1e-15", "1e-16", "1e100",
	"1e-100", "2^62", "0x7fffffffffffffff", "7", "-7", "1e-323",
}

// numChunk is the harness, plus 'isSub' -- the same question the format
// fuzz asks, for the same reason: '%q' writes a float in hexadecimal,
// and of a subnormal float the leading digit is whichever the C library
// picked, so on a reference that spells 5e-324 as 0x0.0000000000001p-1022
// that one answer is a dash on both sides.  SKIPSUB has to come first:
// a Lua local is only visible to functions written after it.
const numChunk = fuzzQuote + `local SKIPSUB = %s
local function isSub(v)
  -- not math.abs, which of an integer answers the integer: abs of
  -- math.mininteger is math.mininteger, negative, and would look
  -- smaller than the smallest normal float
  if type(v) ~= "number" then return false end
  return v ~= 0 and v > -2.2250738585072014e-308 and v < 2.2250738585072014e-308
end
local function P(i, ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  print(i, ok, table.concat(t, "\t"))
end
`

// numLine asks one question about one string: what 'tonumber' makes of
// it with no base and with each of the four, and what the *lexer* makes
// of it -- 'return <s>' is a different scanner from 'tonumber', and a
// numeral that one accepts and the other does not is a difference.
func numLine(i int, s string) string {
	var b strings.Builder
	q := luaQuote(s)
	b.WriteString("P(" + strconv.Itoa(i) + ", pcall(tonumber, " + q + "))\n")
	for _, base := range numBases[1:] {
		b.WriteString("P(" + strconv.Itoa(i) + ", pcall(tonumber, " + q + base + "))\n")
	}
	b.WriteString("P(" + strconv.Itoa(i) + `, (function()
  local f, e = load(` + luaQuote("return "+s) + `, "=c")
  if not f then return false, e end
  return pcall(f)
end)())` + "\n")
	return b.String()
}

// numValLine asks what each of the five ways of writing a number makes
// of one value.
//
// '%q' is the one that can be left out: it writes a float in
// hexadecimal, and of a subnormal float the leading digit is whichever
// the C library picked (see fmtChunk in formatfuzz_test.go), so on a
// reference that spells 5e-324 as 0x0.0000000000001p-1022 that one
// answer is a dash on both sides and the other four are still asked.
func numValLine(i int, v string, skipSub bool) string {
	q := "string.format('%q', " + v + ")"
	if skipSub {
		q = "(SKIPSUB and isSub(" + v + ")) and '-' or " + q
	}
	return "P(" + strconv.Itoa(i) + ", pcall(function() return " +
		strings.Join([]string{
			"tostring(" + v + ")",
			q,
			"math.tointeger(" + v + ")",
			"string.format('%g', " + v + ")",
			"string.format('%e', " + v + ")",
		}, ", ") + " end))\n"
}

func TestNumberFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the number fuzz")
	}
	strs := numStrings()
	vals := numVals
	perStr := 1 + len(numBases) /* tonumber, with and without a base, plus the lexer */
	total := len(strs)*perStr + len(vals)
	skipSub := !fmtRefNormalisesSubnormals(t, ref)

	var b strings.Builder
	b.WriteString(strings.Replace(numChunk, "%s", strconv.FormatBool(skipSub), 1))
	n := 0
	for _, s := range strs {
		n += perStr
		b.WriteString(numLine(n/perStr, s))
	}
	for _, v := range vals {
		n++
		b.WriteString(numValLine(n, v, skipSub))
	}
	b.WriteString(`print("count", ` + strconv.Itoa(total) + ")\n")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "numfuzz.lua"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fmtRunFile(t, ref, dir, "numfuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dir, "numfuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(total)) {
			t.Fatalf("the number chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d strings read and %d values written, %d answers, all of them PUC-Lua's",
			len(strs), len(vals), total)
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
		/* the leading "N\t" is a line of the report below, not a case */
		what := "?"
		if i < len(strs)*perStr {
			what = "tonumber " + luaQuote(strs[i/perStr])
			switch i % perStr {
			case 0:
				what += " with no base"
			case perStr - 1:
				what += " as a literal"
			default:
				what += " in base " + strings.TrimPrefix(numBases[i%perStr], ", ")
			}
		} else if k := i - len(strs)*perStr; k < len(vals) {
			what = "writing " + vals[k]
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", total)
	}
}
