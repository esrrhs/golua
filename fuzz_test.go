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

// fuzzSnippets is the half of the fuzz that cannot be built by combining
// operands: things that have to be written out -- syntax errors,
// metamethods, coroutines, hooks, patterns, os.date, and errors that are
// not strings.  Each one is compiled and then called, so a snippet
// covers both what the compiler says about it and what it answers.
//
// Two rules for writing one: it must not print anything of its own --
// the harness prints exactly one line per case, and an extra line would
// put every later case out of step -- and it must not depend on the
// clock, on the random seed, on a locale or on an address, because none
// of those are the same on both sides.
var fuzzSnippets = []string{
	/* ---- what the compiler says ---- */
	"if then end", "local", "for i=1,10 do", "function (", "a = ",
	"return return", "while do end", "repeat until", "local 1 = 2",
	"::a:: ::a::", "goto nowhere", "break", "x = 1 +", "f('a'",
	"t = {1,,2}", "t = {[1]=}", "--[[ unterminated", "'unterminated",
	"[[unterminated", "x = 0x", "x = 1e", "x = \\", "return 1 return 2",
	"local function end", "a.b.c = function", "for k,v in nil do end",
	"x <close> = 1", "local x <const> = 1 x = 2", "x = 3 ...",
	"do end end", "local a; local a", "x = 9223372036854775808",
	"x = 0xfffffffffffffffff", "x = '\\u{110000}'", "x = 1e999",
	"return", "x = [[a]] [[b]]", "return ...", "x = 1 // ", "x = ~",
	"x = #", "x = (", "x = )", "x = a b", "x = 'a' 'b'", "goto",
	"x==1 and", "if x then elseif end", "local x <nope> = 1", "x = '\\x'",
	"while true do break end", "for i=1,2,3,4 do end",
	"x = function(a,a) end", "x = function(..., a) end",
	"f(a,)", "f(,)", "x = a[b", "x = a:b:c()", "return 1,", "x -= 1",
	"x ++", "local x = 1 goto a ::a::",

	/* ---- what the VM says ---- */
	"x = 1 // 0", "x = 1 % 0", "x = 1e308 * 10", "x = 2^63",
	"x = -9223372036854775808 // -1", "x = math.maxinteger + 1",
	"x = 0/0", "x = 2^1000", "x = #'a' + #'b'", "x = 'a' .. 1 .. {}",
	"x = 1 .. nil", "x = ('a'):nope()", "x = #nil", "x = -'a'",
	"x = 'a' < 1", "x = {} + 1", "x = {} .. 'a'", "x = nil()",
	"x = 1()", "x = true.x", "x = (nil).x",
	"local t = setmetatable({}, {__index = function() error('boom', 0) end}) return t.x",

	/* ---- the libraries at their edges ---- */
	"x = ('%d'):format('a')", "x = string.rep('a', -1)",
	"x = string.sub('a', 5)", "x = table.concat({{}})",
	"x = table.insert({}, 1, 2, 3)", "x = table.remove({}, 5)",
	"x = select(0, 1)", "x = select(-1, 1, 2)", "x = math.floor('a')",
	"x = math.floor(2^63)", "x = tonumber('a', 99)", "x = tonumber(nil)",
	"x = tostring()", "x = utf8.char(-1)", "x = utf8.char(0x7FFFFFFF+1)",
	"x = utf8.codepoint('a', 2)", "x = utf8.offset('a', 5)",
	"x = string.pack('!8i', 1)", "x = string.unpack('i', 'a')",
	"x = string.unpack('s', 'a')", "x = string.pack('c0','')",
	"x = string.format('%99d', 1)", "x = string.format('%c', 300)",
	"x = string.format('%p', 1)", "x = string.format('%a', 1)",
	"x = string.format('%-s','a')",

	/* ---- metamethods ---- */
	"local t = setmetatable({}, {__add = function(a,b) return 'add' end}) return t + 1",
	"local t = setmetatable({}, {__concat = function(a,b) return 'cc' end}) return t .. 'x'",
	"local t = setmetatable({}, {__len = function() return 42 end}) return #t",
	"local t = setmetatable({}, {__eq = function() return true end}) return t == setmetatable({}, getmetatable(t))",
	"local t = setmetatable({}, {__eq = function() return true end}) return t == 1",
	"local t = setmetatable({}, {__lt = function() return true end}) return t < 1",
	"local t = setmetatable({}, {__le = function() return false end}) return t <= 1",
	"local t = setmetatable({}, {__call = function(s, a) return 'called', a end}) return t(7)",
	"local t = setmetatable({}, {__tostring = function() return 'TS' end}) return tostring(t)",
	"local t = setmetatable({}, {__index = {x = 1}}) return t.x, t.y",
	"local t = setmetatable({}, {__newindex = function(t,k,v) rawset(t,k,v*2) end}) t.x = 3 return t.x",
	"local t = setmetatable({}, {__unm = function() return 'neg' end}) return -t",
	"local t = setmetatable({}, {__idiv = function() return 'id' end}) return t // 1",
	"local t = setmetatable({}, {__mod = function() return 'md' end}) return t % 1",
	"local t = setmetatable({}, {__pow = function() return 'pw' end}) return t ^ 1",
	"local t = setmetatable({}, {__band = function() return 'and' end}) return t & 1",
	"local t = setmetatable({}, {__shl = function() return 'shl' end}) return t << 1",
	"local t = setmetatable({}, {__gc = function() end}) return type(t)",
	"local t = setmetatable({}, {__name = 'MyType'}) return tostring(t):sub(1,7)",
	"local mt = {__index = function(t,k) return k end} local t = setmetatable({}, mt) return t.a, t.b, rawget(t,'a')",
	"local t = setmetatable({}, {__metatable = 'locked'}) return (pcall(function() setmetatable(t, {}) end)), getmetatable(t)",
	"local t = setmetatable({}, {__pairs = function(t) return function(_, k) if not k then return 1, 'one' end end, t, nil end}) local r = {} for k,v in pairs(t) do r[#r+1]=k..v end return table.concat(r,',')",
	"return (setmetatable({}, {__index = function() error({code=1}, 0) end})).x",
	"local t = setmetatable({}, {__tostring = function() return {} end}) return pcall(tostring, t)",
	"local t = setmetatable({}, {__len = function() return 'x' end}) return pcall(function() return #t end)",
	"local a = setmetatable({}, {__index = setmetatable({}, {__index = {deep = 9}}}) return a.deep",
	"local t = setmetatable({}, {__close = function(_, e) return e end}) local function f() local x <close> = t error('e', 0) end return pcall(f)",

	/* ---- coroutines ---- */
	"local co = coroutine.create(function(a) local b = coroutine.yield(a+1) return b*2 end) return coroutine.resume(co, 1)",
	"local co = coroutine.create(function(a) local b = coroutine.yield(a+1) return b*2 end) coroutine.resume(co, 1) return coroutine.resume(co, 10)",
	"local co = coroutine.create(function(a) local b = coroutine.yield(a+1) return b*2 end) coroutine.resume(co,1) coroutine.resume(co,10) return coroutine.resume(co, 0)",
	"local co = coroutine.create(function() error('inner', 0) end) return coroutine.resume(co)",
	"local co = coroutine.create(function() error({c=1}, 0) end) local ok, e = coroutine.resume(co) return ok, type(e), e.c",
	"local co = coroutine.create(function() coroutine.yield() error('after', 0) end) coroutine.resume(co) return coroutine.resume(co)",
	"local co = coroutine.wrap(function() for i=1,3 do coroutine.yield(i) end end) return co(), co(), co()",
	"local co = coroutine.wrap(function() coroutine.yield(1) error('w', 0) end) co() return pcall(co)",
	"local co = coroutine.create(function() return 1 end) coroutine.resume(co) return coroutine.status(co)",
	"local co = coroutine.create(function() coroutine.yield() end) coroutine.resume(co) return coroutine.status(co)",
	"local co = coroutine.create(function() coroutine.yield() end) coroutine.resume(co) return coroutine.close(co)",
	"local co = coroutine.create(function() local ok = pcall(coroutine.yield) return ok end) coroutine.resume(co) return coroutine.resume(co)",
	"return coroutine.resume(coroutine.running())",
	"local co = coroutine.create(function() return coroutine.isyieldable() end) return select(2, coroutine.resume(co))",
	"local co = coroutine.create(function() coroutine.close(coroutine.running()) end) return coroutine.resume(co)",
	"local co = coroutine.create(function() local x <close> = setmetatable({}, {__close = function() coroutine.yield('in close') end}) coroutine.yield('body') end) return coroutine.resume(co), coroutine.resume(co)",
	"local co = coroutine.create(function() return debug.traceback('tb') end) return select(2, coroutine.resume(co)):gsub('[%w%p]+:%d+:', 'X:'):sub(1, 20)",

	/* ---- hooks ---- */
	"local t = {} debug.sethook(function() local a = debug.getinfo(2, 'Sl') t[#t+1] = a.currentline end, 'l') local function f() local x = 1 local y = 2 return x + y end f() debug.sethook() return table.concat(t, ',')",
	"local n = 0 debug.sethook(function() n = n + 1 end, 'c') local function f() return 1 end f() f() debug.sethook() return n > 0",
	"local t = {} debug.sethook(function(what) t[#t+1] = what end, 'cr', 0) local function f() return 1 end f() debug.sethook() return table.concat(t, ',')",
	"return (pcall(debug.sethook, print, 'x'))",
	"return (pcall(debug.sethook, print, 'l', 'x'))",
	"debug.sethook(function() error('hook', 0) end, 'l') local ok = pcall(function() local x = 1 return x end) debug.sethook() return ok",
	"return debug.gethook()",

	/* ---- patterns ---- */
	"return string.gsub('hello world', '(%w+)', function(w) return w:upper() end)",
	"return string.gsub('hello', 'l', function() return nil end)",
	"return string.gsub('hello', 'l', function() return false end)",
	"return string.gsub('hello', 'l', {l = 'L'})",
	"return string.gsub('hello', '(l)', '%1%1')",
	"return string.gsub('hello', 'l', '%%')",
	"return string.gsub('abc', '%f[%w]%w', '<%0>')",
	"return string.match('abc', '()b()')",
	"return string.find('abc', '()')",
	"return string.gsub('abc', '()', '[%1]')",
	"return string.match('((x))', '%b()')",
	"return string.match('a1b2c3', '%d+', 4)",
	"return (pcall(string.find, 'a', '('))",
	"return (pcall(string.find, 'a', '%'))",
	"return (pcall(string.gsub, 'a', '%f[%z]', 'x'))",
	"return (pcall(string.match, 'a', '%1'))",
	"return (pcall(string.rep, 'a', 1e10))",
	"return (pcall(string.rep, 'abcdef', 100, 'x'))",

	/* ---- os.date, at instants that are the same everywhere ---- */
	"return os.date('!%Y-%m-%d %H:%M:%S', 0)",
	"return os.date('!%Y-%m-%d %H:%M:%S', 1234567890)",
	"return os.date('!*t', 0).year .. ' ' .. os.date('!*t', 0).yday",
	"return os.date('!%j %U %W %w %a %A %b %B', 1234567890)",
	"return os.date('!%c', 0)",
	"return os.date('!%x', 1234567890)",
	"return os.date('!%X', 1234567890)",
	"return os.date('!%p %I %H', 1234567890)",
	"return os.difftime(100, 50)",
	"return type(os.time()), type(os.clock())",

	/* ---- errors that are not strings ---- */
	"local ok, e = xpcall(function() error(setmetatable({}, {__tostring = function() return 'OBJ' end}), 0) end, function(m) return 'H:' .. tostring(m) end) return ok, e",
	"local ok, e = xpcall(function() error(nil, 0) end, function(m) return 'H:' .. tostring(m) end) return ok, e",
	"local ok, e = xpcall(function() error(42, 0) end, function(m) return m + 1 end) return ok, e",
	"local ok = pcall(error) return ok",
	"local ok, e = pcall(error, 'x', 2) return ok, e:gsub('[%w%p]+:%d+:', 'X:')",
	"return (pcall(function() return nil + 1 end))",
	"return (pcall(function() local t = nil return t.x.y end))",
	"return (pcall(function() return #nil end))",

	/* ---- running out of stack ---- */
	"local function f() return 1 + f() end return (select(2, pcall(f))):gsub('[%w%p]+:%d+:', 'X:')",
	"local function f() return (f()) end return (select(2, pcall(f))):gsub('[%w%p]+:%d+:', 'X:')",

	/* ---- deep but bounded ---- */
	"local function f(n) if n == 0 then return 0 end return 1 + f(n-1) end return f(150)",
	"local mt = {} for i = 1, 100 do mt = {__index = mt} end return mt and 'ok'",
}

// fuzzChunk wraps the expressions and the snippets in the harness that
// prints one line per case: the number, whether it survived, and every
// value it answered, quoted so that a NUL or a newline in one cannot
// shift anything.  Errors are caught, because half of what is being
// compared is what the error says.
//
// luaQuote writes s as a Lua string literal, escaping what Lua escapes:
// backslash, quote, newline, and anything else that is not printable as
// \xHH.  A snippet is handed to 'load' as a string, so the compiler
// sees exactly the characters written here.

func luaQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString("\\\\")
		case c == '"':
			b.WriteString("\\\"")
		case c == '\n':
			b.WriteString("\\n")
		case c == '\t':
			b.WriteString("\\t")
		case c == '\r':
			b.WriteString("\\r")
		case c < 0x20 || c >= 0x7f:
			b.WriteString("\\x" + hexOf(c))
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func hexOf(b byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[b>>4], digits[b&0xf]})
}

func fuzzChunk(exprs, snippets []string) string {
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
	/* A snippet is compiled and then called, so one line covers both
	   what the compiler says about it and what it answers.  The chunk
	   name is "=c" on purpose: without it an error message would name
	   the file and the line in it, which is a fact about where the
	   snippet happens to sit and not about Lua. */
	for i, s := range snippets {
		b.WriteString("P(" + strconv.Itoa(len(exprs)+i+1) + `, (function()
  local f, e = load(` + luaQuote(s) + `, "=c")
  if not f then return false, e end
  return pcall(f)
end)())` + "\n")
	}
	b.WriteString(`print("count", ` + strconv.Itoa(len(exprs)+len(snippets)) + ")\n")
	return b.String()
}

func TestFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the differential fuzz")
	}
	exprs := fuzzExprs()
	snippets := fuzzSnippets
	total := len(exprs) + len(snippets)
	dir := t.TempDir()
	path := filepath.Join(dir, "fuzz.lua")
	if err := os.WriteFile(path, []byte(fuzzChunk(exprs, snippets)), 0o644); err != nil {
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
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(total)) {
			t.Fatalf("the fuzz chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d expressions and %d snippets, all of them PUC-Lua's",
			len(exprs), len(snippets))
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
		/* one line of output per case, in order: the expressions
		   first, then the snippets */
		what := "?"
		if i < len(exprs) {
			what = exprs[i]
		} else if i-len(exprs) < len(snippets) {
			what = snippets[i-len(exprs)]
		}
		t.Errorf("line %d\n  %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if n == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up:\n%s", total, sideBySide(w, g))
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
