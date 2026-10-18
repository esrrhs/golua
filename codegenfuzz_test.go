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

// The seventh fuzz, and the only one that does not compare what a
// program prints.  It compares what the *compiler* produces: every
// snippet and every source in testdata is compiled on both sides and
// the bytes are compared.
//
// Byte code is a much stricter answer than output.  A difference in
// register allocation, in whether a jump is threaded, in whether two
// constants are folded, in the order of the constant table, in the line
// info, or in an upvalue's index changes the bytes while leaving what
// the program prints exactly the same -- and each of those is a place
// where a translation can be wrong and still look right.  Where a
// snippet does not compile at all, the complaint is compared instead,
// which is the lexer and the parser's own error messages for free.
//
// A probe of 419 snippets over this corpus, and of the 116 sources in
// testdata on their own, was byte-identical before this test existed;
// it is here so that it stays that way.

// codegenLits is every literal the lexer has to read: integers in
// decimal and hexadecimal, floats in both, the two ends of the integer
// range and one past it, and every kind of string escape.
var codegenLits = []string{
	"0", "1", "-1", "0x10", "0XaB", "0x.8", "0x1p4", "0x1P-4",
	"1.5", ".5", "5.", "1e2", "1E-2", "1e+2", ".5e-3", "0x1.fp3",
	"9223372036854775807", "9223372036854775808", "0xffffffffffffffff",
	`""`, `"a"`, `'a'`, `"\n"`, `"\t"`, `"\\"`, `"\""`, `"\065"`,
	`"A"`, `"\u{41}"`, `"\u{1F600}"`, `"a\z   b"`,
	`[[a]]`, `[==[a]==]`, `[==[]==]`, "nil", "true", "false",
}

// codegenOps is every operator against every shape of operand: two
// constants, a constant and a variable, two variables, a field, and the
// one operand that is a string and the one that is a number.
var codegenOps = []string{
	"+", "-", "*", "/", "//", "%", "^", "&", "|", "~", "<<", ">>",
	"..", "==", "~=", "<", "<=", ">", ">=",
}

var codegenUnOps = []string{"-", "not ", "#", "~"}

// codegenBodies is every statement form, each also compiled as the body
// of a function, because a 'return' or a 'break' is a different
// instruction inside one.
var codegenBodies = []string{
	"local x = 1 return x",
	"local x <const> = 1 return x",
	"local x <close> = nil return x",
	"local x, y, z = 1, 2, 3 return z",
	"local a; do local a = 2 end return a",
	"do local x = 1 end",
	"for i = 1, 10 do end",
	"for i = 1, 10, 2 do end",
	"for i = 10, 1, -1 do end",
	"for i = 1.0, 10.0 do end",
	"for i = 1, 10 do break end",
	"for i = 1, 10 do local j = i end",
	"for k, v in pairs({}) do end",
	"for k in next, {} do end",
	"for i = 1, 3 do for j = 1, 3 do end end",
	"while true do break end",
	"while nil do end",
	"repeat until true",
	"repeat local x = 1 until x",
	"repeat break until true",
	"if true then end",
	"if false then else end",
	"if false then elseif true then end",
	"if false then elseif false then else end",
	"if 1 then elseif 2 then elseif 3 then else end",
	"goto l ::l::",
	"::l:: goto l",
	"do goto l end ::l::",
	"for i = 1, 3 do goto continue ::continue:: end",
	"local x = 1 ::l:: x = x + 1 if x < 3 then goto l end return x",
	"return",
	"return 1",
	"return 1, 2",
	"return 1, 2, 3",
	"return f()",
	"return (f())",
	"local f = function() return 1 end return f()",
	"local function f() return 1 end return f()",
	"function f() return 1 end return f()",
	"local t = {} function t.a() end function t:b() end return t",
	"local t = {} function t.a.b.c() end return t",
	"local x = 1 local f = function() return x end x = 2 return f()",
	"local f = function(...) return ... end return f(1, 2)",
	`local f = function(...) return select("#", ...) end return f(1, nil, 3)`,
	"local f = function(...) local t = table.pack(...) return t.n end return f(1, nil)",
	"local f = function(a, b, ...) return a + b end return f(1, 2)",
	"local f = function() return 1, 2 end local a, b = f() return b",
	"local t = {1, 2, 3} return #t",
	`local t = {[1] = "a", x = "b", ["y"] = "c"} return t.x`,
	"local t = {f()} return t",
	"local t = {1, 2, x = 3} return t.x",
	"local t = {} t.x = 1 t.y = 2 return t.x + t.y",
	"local t = setmetatable({}, {__index = function() return 1 end}) return t.x",
	`local s = "abc" return s:upper()`,
	`local s = "abc" return ("x"):rep(3)`,
	"return #...",
	"local t = {} return t[1]",
	"local a = 1 a = a + 1 a = a * 2 return a",
	"return 1 + 2 * 3 - 4 / 2",
	"return (1 + 2) * 3",
	"return 2 ^ 3 ^ 2",
	"return -2 ^ 2",
	`return "a" .. "b" .. "c"`,
	"return 1 .. 2 .. 3",
	"return true and false",
	"return true or false",
	"return nil and 1",
	"return nil or 1",
	"local a = 1 return a and a or a",
	"return not not 1",
	"local t = {} return t and t.x and t.x.y",
	`return #"abc"`,
	"local t = {1, 2, 3} return t[#t]",
	"return math.max(1, 2)",
	"local t = {} return #t == 0",
	"do end do end do end",
	";",
	";;",
	"local x = 1;;;return x",
	"return;;",
}

// codegenUpvalues is what a closure can capture, and when.
var codegenUpvalues = []string{
	"local x = 1 local f = function() return x end return f()",
	"local x = 1 local f = function() x = x + 1 end f() return x",
	"local x = 1 local a = function() return x end local b = function() return x end return a() + b()",
	"local f = {} for i = 1, 3 do f[i] = function() return i end end return f[1]() + f[3]()",
	"local x = 1 do local y = 2 local f = function() return x + y end return f() end",
	"local function a() local x = 1 return function() return x end end return a()()",
	"local x <const> = 1 local f = function() return x end return f()",
	"for i = 1, 2 do local j = i * 2 end",
	"local co = coroutine.wrap(function() local x = 1 coroutine.yield(x) end) return co()",
	"local t = {} do local x <close> = nil end return t",
	"local x <close> = nil return 1",
	"local x <const>, y <close> = 1, nil return x",
}

// codegenBad is what must not compile.  The complaint is what is
// compared, so this is the lexer's and the parser's error messages over
// the same grid as everything else.
var codegenBad = []string{
	"return 1 return 2", "x = ", "local", "local 1", "for i = 1",
	"if then end", "while do end", "repeat", "until", "function end",
	"f(", "f(1,", ")", "{", "}", "{1,}", "[[unterminated",
	"x = 0x", "x = 1e", `x = "`, "x = '", "goto nowhere",
	"break", "::l:: ::l::", "return 0xfffffffffffffffff",
	"x = \\", "local x <close>", "local x <const>", "local x <nope> = 1",
	"a.b = ", "a[]", "a[1] = ", "local function 1() end",
	"x = 1 + ", "x = * 1", "do return 1 end return 2",
	`x = "a" .. `, "#", "x = #", "local t = {1, 2,}", "local t = {,}",
	"x = 1e999", `x = "a\q"`, `x = "\u{110000}"`, `x = "\u{}"`,
	`x = "\x"`, `x = "\400"`, "=1", "return ..", "x === 1",
	"local x = 1 local x = 2 return x", "::1::", "goto 1",
}

// codegenSnippets is the corpus.  Built rather than written out, and a
// snippet that contains a newline is a bug in the builder: the harness
// reads them one line at a time.
var codegenSnippets = func() []string {
	var s []string
	add := func(x string) {
		if strings.ContainsAny(x, "\n\r") {
			panic("a snippet with a newline in it: " + strconv.Quote(x))
		}
		s = append(s, x)
	}
	for _, l := range codegenLits {
		add("return " + l)
	}
	for _, op := range codegenOps {
		for _, p := range [][2]string{
			{"1", "2"}, {"1.0", "2"}, {`"a"`, `"b"`}, {"a", "b"},
			{"1", `"2"`}, {"(a)", "(b)"}, {"a.b", "a.c"},
		} {
			add("local a, b = 1, 2 return " + p[0] + " " + op + " " + p[1])
		}
	}
	for _, op := range codegenUnOps {
		for _, a := range []string{"1", "1.0", `"a"`, "a", "a.b", "(a)"} {
			add("local a = 1 return " + op + a)
		}
	}
	for _, b := range codegenBodies {
		add(b)
		add("local function outer() " + b + " end return outer")
	}
	s = append(s, codegenUpvalues...)
	s = append(s, codegenBad...)
	return s
}()

// codegenChunk is the harness.  It answers one line per case --
// number, source, where in the binary its bytes start, how many there
// are, and what went wrong if anything did -- and writes the bytes
// themselves to a second file.
//
// The bytes go to a file rather than to stdout because stdout is a text
// stream: on a platform where that means something, writing a NUL or a
// newline that is part of a float would not come back the same way it
// went in, and the two sides would then differ for a reason that has
// nothing to do with the compiler.
const codegenChunk = `
local spath, fpath, binpath = ...
local out = assert(io.open(binpath, "wb"))
local n, pos = 0, 0
local function line(src)
  n = n + 1
  io.write(n, "\t", src, "\t", pos)
end
local function ok(bin)
  out:write(bin)
  pos = pos + #bin
  io.write("\t", #bin, "\tok\n")
end
local function bad(how, msg)
  io.write("\t0\t", how, "\t", msg, "\n")
end
for src in io.lines(spath) do
  line(src)
  local f, e = load(src, "=c")
  if not f then bad("load", e)
  else
    local good, bin = pcall(string.dump, f)
    if good then ok(bin) else bad("dump", tostring(bin)) end
  end
end
for src in io.lines(fpath) do
  line(src)
  local f, e = loadfile(src)
  if not f then bad("load", e)
  else
    local good, bin = pcall(string.dump, f)
    if good then ok(bin) else bad("dump", tostring(bin)) end
  end
end
out:close()
io.write("count\t", n, "\n")
`

func TestCodegenFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the codegen fuzz")
	}

	dir := t.TempDir()
	snips := filepath.Join(dir, "snips.txt")
	files := filepath.Join(dir, "files.txt")
	if err := os.WriteFile(snips,
		[]byte(strings.Join(codegenSnippets, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	/* Every Lua source in the repository, official and ours.  The paths
	   have to be absolute: the interpreters run with the temp
	   directory as their working directory, so that what an error
	   message names is the same on both sides. */
	var list []string
	for _, pattern := range []string{"testdata/official/*.lua", "testdata/diff/*.lua"} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range m {
			abs, err := filepath.Abs(p)
			if err != nil {
				t.Fatal(err)
			}
			list = append(list, abs)
		}
	}
	if len(list) == 0 {
		t.Fatal("no sources found under testdata")
	}
	if err := os.WriteFile(files,
		[]byte(strings.Join(list, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	type side struct {
		name, bin, out, text string
	}
	run := func(name, exe string) side {
		t.Helper()
		harness := filepath.Join(dir, name+".lua")
		binPath := filepath.Join(dir, name+".bin")
		if err := os.WriteFile(harness, []byte(codegenChunk), 0o644); err != nil {
			t.Fatal(err)
		}
		c := &exec.Cmd{
			Path: exe,
			Args: []string{"lua", harness, snips, files, binPath},
			Dir:  dir,
			Env:  diffEnv(),
		}
		text, err := c.CombinedOutput()
		if err != nil && len(text) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := os.ReadFile(binPath)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return side{name: name, bin: string(b), out: exe, text: string(text)}
	}
	want := run("puc", ref)
	got := run("this", diffBuild(t))

	total := len(codegenSnippets) + len(list)
	for _, s := range []side{want, got} {
		if !strings.Contains(s.text, "count\t"+strconv.Itoa(total)) {
			t.Fatalf("the codegen chunk did not run to the end in %s:\n%s",
				s.name, s.text)
		}
	}

	/* The bytes first: they are the answer.  Finding where they part,
	   and then which case owns that offset, is what makes a failure
	   say something. */
	if want.bin != got.bin {
		i := 0
		for i < len(want.bin) && i < len(got.bin) && want.bin[i] == got.bin[i] {
			i++
		}
		t.Errorf("%d bytes compiled, and they part at byte %d "+
			"(%d on one side, %d on the other)", len(want.bin), i,
			len(want.bin), len(got.bin))
		t.Errorf("%s", codegenOwner(t, want.text, got.text, i))
		return
	}
	/* Then the lines, which is where a complaint that never became
	   bytes is compared. */
	w, g := diffNormalise(want.text), diffNormalise(got.text)
	if w == g {
		t.Logf("%d snippets and %d sources compiled to the same %d bytes, "+
			"all of them PUC-Lua's", len(codegenSnippets), len(list),
			len(want.bin))
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
		t.Errorf("line %d\n  PUC-Lua: %s\n  this:    %s", i+1, a, b)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", total)
	}
}

// codegenOwner says which case owns a byte offset, on both sides, from
// the "number, source, start, length" lines the harness wrote.
func codegenOwner(t *testing.T, want, got string, at int) string {
	t.Helper()
	owner := func(text string) string {
		for _, l := range strings.Split(text, "\n") {
			f := strings.Split(l, "\t")
			if len(f) < 5 || f[3] == "0" {
				continue
			}
			start, _ := strconv.Atoi(f[2])
			n, _ := strconv.Atoi(f[3])
			if at >= start && at < start+n {
				return fmt.Sprintf("%s: %s (bytes %d..%d)", f[0], f[1], start, start+n)
			}
		}
		return fmt.Sprintf("no case owns byte %d", at)
	}
	return "  PUC-Lua " + owner(want) + "\n  this    " + owner(got)
}
