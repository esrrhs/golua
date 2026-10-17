package lua

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The other half of the fuzz, over a surface the expression fuzz cannot
// reach: pattern matching.  testdata/diff/32_pattern.lua and
// 72_gsubbit.lua are 366 lines of cases somebody thought of, and they
// pass; what nobody writes down is what the matcher does with the
// combinations -- a frontier followed by a set, a quantifier on a
// position capture, '%b' with a delimiter that is also a magic
// character.  lstrlib.c's 'match' is two hundred lines of exactly that
// kind of code.
//
// So the patterns here are built rather than chosen: every atom against
// every atom, and each of those against every subject.  The answers are
// PUC-Lua's, as everywhere else in the differential machinery.

// patAtoms are the pieces a pattern is built from: the classes (all
// twenty, including '%g' and '%G', which are 5.4's), the escaped magic
// characters, every shape a set can take, both anchors, the four
// quantifiers, captures of every kind, '%b', '%f', and the
// back-references.
//
// Some of them are not a complete pattern on their own -- '(' , '*',
// '%1' -- and that is deliberate: a malformed pattern is an error
// message, and the message is half of what this compares.
var patAtoms = []string{
	/* a single character, and the characters that mean something */
	".", "a", "b", "z", "1", "0", "_", "-", " ",
	/* the classes */
	"%a", "%A", "%c", "%C", "%d", "%D", "%g", "%G", "%l", "%L",
	"%p", "%P", "%s", "%S", "%u", "%U", "%w", "%W", "%x", "%X",
	/* the magic characters escaped */
	"%%", "%.", "%(", "%)", "%[", "%]", "%*", "%+", "%-", "%?", "%^", "%$",
	/* sets: plain, negated, a range, a class inside, and the four
	   places a ']' or a '-' can sit and still be a literal */
	"[abc]", "[^abc]", "[a-c]", "[^a-c]", "[%a]", "[%a%d]", "[^%a]",
	"[]]", "[^]]", "[a-]", "[-a]", "[%%]", "[%[]", "[%]]", "[[a]",
	/* anchors */
	"^", "$", "^a", "a$", "^$",
	/* quantifiers, which attach to whatever came before */
	"*", "+", "-", "?",
	/* captures: plain, a position, a class, and a quantified one */
	"(", ")", "()", "(a)", "(.)", "(%a)", "(a*)", "([a-c])",
	/* balanced, and the frontier */
	"%b()", "%bxy", "%b[]", "%f[%a]", "%f[%w]", "%f[^%a]", "%f[%s]",
	/* back-references, valid and not */
	"%1", "%2", "%3",
}

// patSubjects are what the patterns are run against.  They are chosen
// to answer the questions a pattern asks: is there anything here at all,
// does it start or end with the thing, are the delimiters balanced, is
// there a boundary, is there a NUL in the middle of it.
var patSubjects = []string{
	"", "a", "ab", "abc", "aaa", "a1b2c3", "Hello, World!",
	"  \t ", "()", "(())", "a(b)c", "%", "%%", "a.b", "1.5e3",
	"abcdef", "xyz", "A_B-C", "[[x]]", "a\nb", "a\000b", "\000",
	"aXbXc", "---", "123 456",
}

// patPatterns is every atom on its own, then every atom followed by
// every atom, then a handful written out because they need three pieces
// to mean anything.
var patTriples = []string{
	"(a)(b)", "(a)(b)%2", "(a)(b)%1%2", "^(a)(b)$", "()a()",
	"(%a)(%d)", "%b()%1", "%f[%a]%a+", "^%s*(%a+)%s*$",
	"([a-c])([a-c])([a-c])", "(a*)b", "^(.-)$", "(.)%1",
	"[]]", "[^]]", "%a+%d", "%d+%.?%d*", "()%a()", "%b(())",
}

func patPatterns() []string {
	atoms := patAtoms
	n := len(atoms)
	pats := make([]string, 0, n+n*n+len(patTriples))
	pats = append(pats, atoms...)
	/* Anchors and quantifiers only mean something in front of
	   something, so every pair is tried; that is 70 x 70 of them. */
	for _, a := range atoms {
		for _, b := range atoms {
			pats = append(pats, a+b)
		}
	}
	pats = append(pats, patTriples...)
	return pats
}

// patChunk is the harness: one line per pattern and subject, and on it
// what each of the six calls answered -- 'find' twice so that an
// initial position is exercised, 'match', 'gsub' with a literal and
// with '%0', and 'gmatch' with a bound on it, since a pattern that
// matches nothing at all would otherwise walk the whole string.
//
// A call that raises says so: half of what is being compared is the
// message, and 'show' puts a '!' in front of it so that a failure can
// never look like an answer.
const patChunk = fuzzQuote + `local function show(ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  local s = table.concat(t, ",")
  if not ok then return "!" .. s end
  return s
end
local function gm(s, p)
  local out = {}
  for a, b in string.gmatch(s, p) do
    out[#out + 1] = q(a) .. "/" .. tostring(b)
    if #out >= 8 then break end
  end
  return table.concat(out, ";")
end
`

// patLine is the line for one pattern and one subject.  The pattern and
// the subject are written as Lua literals by luaQuote, so what the
// matcher sees is exactly the bytes listed above.
func patLine(i int, pat, subj string) string {
	return "print(" + strconv.Itoa(i) +
		", show(pcall(string.find, " + luaQuote(subj) + ", " + luaQuote(pat) + "))" +
		", show(pcall(string.find, " + luaQuote(subj) + ", " + luaQuote(pat) + ", 3))" +
		", show(pcall(string.match, " + luaQuote(subj) + ", " + luaQuote(pat) + "))" +
		", show(pcall(string.gsub, " + luaQuote(subj) + ", " + luaQuote(pat) + ", \"X\"))" +
		", show(pcall(string.gsub, " + luaQuote(subj) + ", " + luaQuote(pat) + ", \"%0\"))" +
		", show(pcall(gm, " + luaQuote(subj) + ", " + luaQuote(pat) + ")))\n"
}

func TestPatternFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the pattern fuzz")
	}
	pats := patPatterns()
	subs := patSubjects
	total := len(pats) * len(subs)

	var b strings.Builder
	b.WriteString(patChunk)
	n := 0
	for _, p := range pats {
		for _, s := range subs {
			n++
			b.WriteString(patLine(n, p, s))
		}
	}
	b.WriteString(`print("count", ` + strconv.Itoa(total) + ")\n")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "patfuzz.lua"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	want := patRun(t, ref, dir)
	got := patRun(t, diffBuild(t), dir)
	/* The same guard as the expression fuzz: a chunk that does not
	   load prints the same syntax error on both sides, and identical
	   output is what success looks like. */
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(total)) {
			t.Fatalf("the pattern chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d patterns against %d subjects, %d answers, all of them PUC-Lua's",
			len(pats), len(subs), total*6)
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
		/* line i is pattern i/len(subs) against subject i%len(subs) */
		what := "?"
		if i < total {
			what = "pattern " + pats[i/len(subs)] + " on " +
				strconv.Quote(subs[i%len(subs)])
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", total)
	}
}

// patRun runs one interpreter over the chunk.  argv[0] is "lua" on both
// sides and the working directory is the chunk's, so a "patfuzz.lua:70:"
// in an error message is the same string either way.
func patRun(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "patfuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}
