package lua

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The differential suite in testdata/diff is 83 cases somebody wrote,
// each about one thing.  This is the other kind: not a case anybody
// thought of, but every combination of a set of operands, operators and
// library calls -- twelve thousand expressions, none of them chosen --
// run through PUC-Lua and through this port and compared line by line.
//
// It is exhaustive rather than random, so it is the same twelve thousand
// every run and a difference is always reproducible; and it is generated
// here rather than committed, so there is no 350 KB of Lua in the
// repository that nobody reads.
//
// It needs LUA_REF, because what it compares against is a real
// interpreter answering on this machine; without one it skips, as the
// rest of the differential machinery does.  CI sets it, so there it runs
// on every push, on both unixes.

// fuzzVals is what the expressions are built from: integers at both
// ends of the range, floats at both ends of theirs, the three values
// that are not numbers at all (nan, inf, -inf), and strings -- including
// one holding a NUL, which is how the 'constant' in an error message
// came to differ (see formatvarinfo).
var fuzzVals = []string{
	"0", "1", "-1", "2", "3", "7", "-7", "10", "255", "1000000",
	"9223372036854775807", "-9223372036854775808", "0x7fffffffffffffff",
	"0.0", "-0.0", "0.5", "-0.5", "1.5", "-1.5", "2.5", "-2.5",
	"1e15", "1e16", "1e300", "1e-300", "1e308", "5e-324", "3.14159265358979",
	"math.huge", "-math.huge", "(0/0)", "(1/0)", "(-1/0)",
	`"abc"`, `""`, `"10"`, `" 10 "`, `"0x10"`, `"1e2"`, `"3.14"`, `"a"`,
	`"\0"`, `"%d"`, `"\u{1F600}"`,
}

// fuzzOps is every operator but '^'.  Exponentiation is math.pow, and
// Go's is not the platform's libm's: measured over 3400 powers here, six
// of them answered 1 ulp apart (9223372036854775807 ^ -2.5 is
// 3.8705767444936e-48 there and 3.8705767444937e-48 here).  That gap is
// README §7's, it is not something to fix, and a fuzz that fails on a
// difference everybody has already agreed to is a fuzz nobody runs.
var fuzzOps = []string{
	"+", "-", "*", "/", "//", "%", "&", "|", "~", "<<", ">>",
	"==", "~=", "<", "<=", ">", ">=", "..",
}

var fuzzBases = []string{"2", "8", "10", "16", "36", "1", "37"}

var fuzzFormats = []string{
	"%d", "%i", "%f", "%g", "%G", "%e", "%E", "%s", "%q", "%x", "%X", "%o",
	"%c", "%5d", "%-5d", "%05d", "%+d", "% d", "%.2f", "%10.3f", "%-10.3f",
	"%.0f", "%.14g", "%.3e", "%5.2s", "%-5s", "%.3s", "%#x", "%#o", "%%",
	"%d%%", "a%sb",
}

var fuzzFormatArgs = []string{
	"0", "1", "-1", "255", "3.5", "-3.5", "1e300", "1e-300", "0.0", "-0.0",
	"math.maxinteger", "math.mininteger", "1/0", "-1/0", "0/0",
	`"x"`, `""`, `"a\nb"`, `"\0"`, `"%"`, "65", "3.14159", "3.0", "-3.0",
}

var fuzzStrs = []string{
	`"abc"`, `""`, `"Hello, World!"`, `"a,b,c"`, `"  trim  "`, `"\0abc"`,
	`"\u{1F600}"`, `"abcabc"`, `"3.14"`, `"10"`,
}

var fuzzPats = []string{
	`"."`, `"%a"`, `"%d"`, `"%s"`, `"%w+"`, `"^a"`, `"b$"`, `"%a+"`,
	`"(%a)(%d)"`, `"[]"`, `"[^a]"`, `"a*"`, `"%b()"`, `"()"`, `"%f[%w]%w"`,
}

// fuzzExprs is every expression the fuzz asks about.  Kept as a slice
// rather than written straight into the chunk, so that a difference can
// be reported as the expression it was, and not as a line number.
func fuzzExprs() []string {
	var e []string
	add := func(s string) { e = append(e, s) }

	/* every operand, with every operator, against every operand */
	for _, a := range fuzzVals {
		for _, b := range fuzzVals {
			for _, op := range fuzzOps {
				add(a + " " + op + " " + b)
			}
		}
	}

	for _, v := range fuzzVals {
		add("-(" + v + ")")
		add("#tostring(" + v + ")")
		add("not " + v)
		add("type(" + v + ")")
		add("math.type(" + v + ")")
		add("math.tointeger(" + v + ")")
		add("math.abs(" + v + ")")
		add("math.floor(" + v + ")")
		add("math.ceil(" + v + ")")
		add("math.sqrt(" + v + ")")
		add("tostring(" + v + ")")
		add("tonumber(" + v + ")")
		for _, b := range fuzzBases {
			add("tonumber(" + v + ", " + b + ")")
		}
	}

	for _, f := range fuzzFormats {
		for _, a := range fuzzFormatArgs {
			add("string.format(" + `"` + f + `"` + ", " + a + ")")
		}
	}

	for _, s := range fuzzStrs {
		for _, call := range []string{
			"string.len(%s)", "string.upper(%s)", "string.lower(%s)",
			"string.reverse(%s)", "string.byte(%s)", "string.sub(%s, 2)",
			"string.sub(%s, 2, 3)", "string.sub(%s, -2)", "string.rep(%s, 3)",
			"string.rep(%s, 0)",
		} {
			e = append(e, sprintf(call, s))
		}
		for _, p := range fuzzPats {
			add(sprintf("string.find(%s, %s)", s, p))
			add(sprintf("string.find(%s, %s, 2)", s, p))
			add(sprintf("string.find(%s, %s, 1, true)", s, p))
			add(sprintf("string.match(%s, %s)", s, p))
			add(sprintf("string.gsub(%s, %s, %s)", s, p, `"X"`))
			add(sprintf(`(function() local t = {} for w in string.gmatch(%s, %s) do t[#t+1] = w end return table.concat(t, ',') end)()`, s, p))
		}
	}

	for _, s := range []string{`"abc"`, `"\u{1F600}"`, `"héllo"`, `"\0"`, `"\xff"`, `""`} {
		add(sprintf("utf8.len(%s)", s))
		add(sprintf("utf8.codepoint(%s, 1, 1)", s))
		add(sprintf("utf8.offset(%s, 1)", s))
		add(sprintf("utf8.offset(%s, 2)", s))
		add(sprintf("utf8.offset(%s, -1)", s))
		add(sprintf(`(function() local t = {} for p, c in utf8.codes(%s) do t[#t+1] = p .. ':' .. c end return table.concat(t, ',') end)()`, s))
	}
	for _, c := range []string{"65", "0x1F600", "0", "127", "256", "0x7FFFFFFF"} {
		add("utf8.char(" + c + ")")
	}

	for _, n := range []string{"0", "1", "3", "10"} {
		add(sprintf(`(function() local t = {} for i = 1, %s do t[i] = i end return #t, table.concat(t, ',') end)()`, n))
	}
	for _, i := range []string{"1", "2", "4"} {
		for _, v := range []string{"9", "'x'", "nil"} {
			add(sprintf(`(function() local t = {1,2,3} table.insert(t, %s, %s) return table.concat(t, ',') end)()`, i, v))
		}
	}
	for _, i := range []string{"1", "2", "3", "5"} {
		add(sprintf(`(function() local t = {1,2,3} return table.remove(t, %s), #t end)()`, i))
	}
	add(`(function() local t = {1,2,3,4,5} return table.concat(table.move(t, 2, 4, 1), ','), table.concat(t, ',') end)()`)
	for _, i := range []string{"1", "2"} {
		for _, j := range []string{"2", "3"} {
			add(sprintf(`(function() return table.unpack({1,2,3}, %s, %s) end)()`, i, j))
		}
	}
	add(`(function() local p = table.pack(1, nil, 3) return p.n, p[1], p[3] end)()`)

	for _, a := range []string{"7", "-7", "7.5", "-7.5", "1e300", "0"} {
		for _, b := range []string{"3", "-3", "2.5", "0"} {
			add(sprintf("math.fmod(%s, %s)", a, b))
			add(sprintf("math.max(%s, %s)", a, b))
			add(sprintf("math.min(%s, %s)", a, b))
		}
	}
	for _, a := range []string{"0", "1", "-1", "math.maxinteger"} {
		for _, b := range []string{"0", "1", "-1", "math.mininteger"} {
			add(sprintf("math.ult(%s, %s)", a, b))
		}
	}
	for _, v := range []string{`{1,2,3}`, `"abc"`, `{}`} {
		add("rawlen(" + v + ")")
	}
	for _, s := range []string{"1, 2, 3", "1, nil, 3"} {
		add("select('#', " + s + ")")
		add("select(2, " + s + ")")
		add("select(-1, " + s + ")")
	}
	add("select(0, 1)")

	return e
}

// sprintf drops the arguments into the "%s" places of a Lua snippet, in
// order.  It is not fmt.Sprintf -- the formats are Lua, and a "%s" in one
// of them is where a value goes, not a verb to be rendered -- and it is
// one pass over the format on purpose: substituting one argument at a
// time rescans what was already substituted, and an argument can hold a
// "%s" of its own ("%s" is a Lua pattern), which the next substitute
// would then find and replace.  That is how the chunk came out as
// 'string.gsub("abc", ""X"", %s)' and both interpreters answered the
// same syntax error.
func sprintf(format string, args ...string) string {
	parts := strings.Split(format, "%s")
	if len(parts)-1 != len(args) {
		panic(fmt.Sprintf("sprintf: %q has %d places for %d arguments",
			format, len(parts)-1, len(args)))
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		if i < len(args) {
			b.WriteString(args[i])
		}
	}
	return b.String()
}

// fuzzChunk wraps the expressions in the harness that prints one line
// per expression: the number, whether it survived, and every value it
// answered, quoted so that a NUL or a newline in one cannot shift
// anything.  Errors are caught, because half of what is being compared
// is what the error says.
func fuzzChunk(exprs []string) string {
	var b strings.Builder
	b.WriteString(`local function q(v)
  if type(v) == "string" then return string.format("%q", v) end
  if type(v) == "table" or type(v) == "function" then return type(v) end
  return tostring(v)
end
local function P(i, ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  print(i, ok, table.concat(t, "\t"))
end
`)
	for i, e := range exprs {
		b.WriteString("P(" + strconv.Itoa(i+1) + ", pcall(function() return " + e + " end))\n")
	}
	b.WriteString(`print("count", ` + strconv.Itoa(len(exprs)) + ")\n")
	return b.String()
}

func TestFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the differential fuzz")
	}
	exprs := fuzzExprs()
	dir := t.TempDir()
	path := filepath.Join(dir, "fuzz.lua")
	if err := os.WriteFile(path, []byte(fuzzChunk(exprs)), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fuzzRun(t, ref, dir)
	got := fuzzRun(t, diffBuild(t), dir)
	/* Both sides have to have run the chunk to the end.  Without this
	   the test is capable of passing without testing anything: a chunk
	   that does not load prints the same syntax error on both sides,
	   and identical output is what success looks like.  It did. */
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(len(exprs))) {
			t.Fatalf("the fuzz chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d expressions, all of them PUC-Lua's", len(exprs))
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	/* The chunk did not run at all: say so, rather than reporting
	   twelve thousand differences that are all one syntax error. */
	if len(wl) < 2 || !strings.Contains(w, "count") {
		t.Fatalf("the fuzz chunk did not run in the reference interpreter:\n%s", w)
	}
	if len(gl) < 2 || !strings.Contains(g, "count") {
		t.Fatalf("the fuzz chunk did not run here:\n%s", g)
	}
	n := 0
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
		n++
		if n > 20 {
			t.Logf("... and more")
			break
		}
		/* one line of output per expression, in order */
		what := "?"
		if i < len(exprs) {
			what = exprs[i]
		}
		t.Errorf("line %d\n  %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if n == 0 {
		t.Errorf("%d expressions: the lines differ but none of them line up:\n%s", len(exprs), sideBySide(w, g))
	}
}

// fuzzRun runs one interpreter over the chunk.  argv[0] is "lua" on both
// sides and the working directory is the chunk's, so the "fuzz.lua:70:"
// in an error message is the same string either way -- which is the only
// reason the messages can be compared at all.
func fuzzRun(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "fuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(), /* TZ has to agree: see diffEnv */
	}
	out, _ := cmd.CombinedOutput() /* the exit status is not part of it */
	return string(out)
}
