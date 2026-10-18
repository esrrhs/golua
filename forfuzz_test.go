package lua

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The sixth fuzz, over the numeric 'for'.
//
// OP_FORPREP and OP_FORLOOP decide whether a loop runs at all, what its
// control variable is, and when it stops -- in integers, where the
// counter can wrap past 'maxinteger', and in floats, where a step can
// round the index past the limit.  Nothing about that is written down
// anywhere but in lvm.c, and a probe of 16 250 loops found one case in
// 60 where the two disagreed:
//
//	for i = 0, 1, 0/0
//
// PUC-Lua does not run it; this port ran it once, with i = 0.0.
//
// C decides whether to skip the loop with
//
//	luai_numlt(0, step) ? luai_numlt(limit, init) : luai_numlt(init, limit)
//
// which is a ternary, and the port had written it as the pair of
// conjunctions that reads the same -- '(step > 0 && limit < init) ||
// (step <= 0 && init < limit)' -- but the two are only the same while
// step is a number that is comparable with zero.  A NaN is neither
// greater nor less than zero, so C takes the *else* branch and asks
// 'init < limit': 0 < 1, and the loop is skipped.  'step <= 0' is also
// false, so this port took neither branch and ran the loop.  The same
// shape in 'floatforloop' had the same rewrite.
//
// Both are fixed.  What is left is the grid, which is every combination
// of start, limit and step over the values below, asked twice: once
// with the step written out and once with it left to default to 1.

// forVals is what a start, a limit or a step can be: zeroes of both
// signs, integers and floats, the two ends of the integer range, the
// floats just outside it, the point where a float stops being able to
// count by ones, and the three values that are not comparable at all.
var forVals = []string{
	"0", "1", "-1", "2", "-2", "3", "10",
	"0.5", "-0.5", "1.5", "-1.5", "0.0", "-0.0",
	"1e15", "1e16", "1e300", "-1e300",
	"math.maxinteger", "math.mininteger",
	"2.0^63", "-2.0^63", "2.0^53",
	"1/0", "-1/0", "0/0",
}

// forBadVals is what a start, a limit or a step can be that is not a
// number: which of the three it is decides which of three different
// error messages comes out, and a string that *reads* as a number is
// not an error at all.
var forBadVals = []string{
	`"10"`, `"1e2"`, `" 10 "`, `"abc"`, `""`,
	"nil", "true", "{}", "print",
}

// forCap is how many times a loop is allowed to run before it is cut
// short.  Enough to see whether it starts, what it starts at and where
// it goes next; few enough that 'for i = 0, math.maxinteger' does not
// take the rest of the day.
const forCap = 6

// forChunk is the harness.  'run' answers whether it survived and, if
// it did, how many times it ran, the first value and the last -- the
// first because 'for i = 1, 2' and 'for i = 1.0, 2' differ in nothing
// else, the last because a loop cut short at forCap has no other way to
// say where it was going.
var forChunk = fuzzQuote + `
local function run(a, b, s, hasStep)
  local first, last, n = "none", "none", 0
  local ok, e = pcall(function()
    local body = function(i)
      n = n + 1
      if n == 1 then first = i end
      last = i
      if n >= ` + strconv.Itoa(forCap) + ` then return true end
    end
    if hasStep then
      for i = a, b, s do if body(i) then break end end
    else
      for i = a, b do if body(i) then break end end
    end
  end)
  if not ok then return false, e end
  return true, n, first, last
end
local function P(i, name, ...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = q((select(k, ...))) end
  print(i, name, table.concat(t, "\t"))
end
`

func TestForFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the 'for' fuzz")
	}

	var b strings.Builder
	b.WriteString(forChunk)
	var names []string
	n := 0
	emit := func(name, call string) {
		n++
		names = append(names, name)
		b.WriteString("P(" + strconv.Itoa(n) + ", " + strconv.Quote(name) +
			", " + call + ")\n")
	}

	for _, a := range forVals {
		for _, lim := range forVals {
			for _, s := range forVals {
				emit("for i = "+a+", "+lim+", "+s,
					"run("+a+", "+lim+", "+s+", true)")
			}
			/* the step left out, which is a different instruction:
			   lparser.c emits a constant 1 rather than no step, and
			   'forprep' then takes its 'step != 1' shortcut */
			emit("for i = "+a+", "+lim,
				"run("+a+", "+lim+", nil, false)")
		}
	}

	/* Each of the three positions in turn, the other two being numbers
	   that make the loop run, so that what is compared is the complaint
	   rather than a loop that would not have run anyway. */
	for _, v := range forBadVals {
		emit("for i = "+v+", 2", "run("+v+", 2, nil, false)")
		emit("for i = 1, "+v, "run(1, "+v+", nil, false)")
		emit("for i = 1, 3, "+v, "run(1, 3, "+v+", true)")
	}

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "forfuzz.lua"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fmtRunFile(t, ref, dir, "forfuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dir, "forfuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the 'for' chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d values in each of three positions x %d loops, plus %d "+
			"values that are not numbers: %d loops, all of them PUC-Lua's",
			len(forVals), len(forVals)*len(forVals)*(len(forVals)+1),
			len(forBadVals)*3, cases)
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
		if i < len(names) {
			what = names[i]
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if reported == 0 {
		t.Errorf("%d loops: the lines differ but none of them line up", cases)
	}
}
