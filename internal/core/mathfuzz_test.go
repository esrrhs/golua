package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The fourteenth fuzz, over the math library.
//
// It is the last standard library without a grid of its own, and it is
// the one whose answers a port can get wrong in two different ways at
// once. Half of it is arithmetic a port writes for itself: 'floor' and
// 'ceil' have to answer an *integer* for a float that fits in one,
// 'fmod' keeps a rule of its own for a divisor of -1 and another for a
// divisor of 0, and 'abs' of the smallest integer is the smallest
// integer. The other half is a pseudo-random generator: 'random' is
// xoshiro256** seeded by two integers, which means the sequence it
// gives is a thing two programs can be asked about and either agree on
// or not.
//
// Before a grid can be written over a library, somebody has to say
// which of its answers are the library's to give and which are Lua's.
// Two probes did that here, and they did not agree with what the
// README said.
//
// The first asked 4173 questions — every function over every value
// there is a name for — and 10 answers came back different.  All ten
// are in the last bit of a function the C library answers:
//
//   'sin', 'cos', 'tan', 'asin', 'acos', 'atan', 'atan2', 'exp',
//   'log', 'log10', 'cosh', 'sinh', 'tanh' and 'pow' are the C
//   library's. A cgo build calls that library; a build without cgo
//   calls Go's math, which is a different one. Either way the bits
//   belong to the host, not to a golden in this repository.
//   Without cgo, 'math.pow(4611686018427387904, 1.5)' is
//   9.903520314283e+27 to PUC-Lua and 9.9035203142831e+27 to Go: the
//   same number to fourteen digits and not to fifteen.  It is the '%a' and the '!%z'
//   of this library (§6.11, §6.16) and it is §6.7's subject — the
//   answer belongs to the library, not to Lua — so the grid asks none
//   of those fourteen answers, and nothing else here does.
//
//   What it asks of them instead is what they say about a table, a
//   boolean and nothing at all, which is theirs and not the library's
//   whatever they say about 0.5.  And it asks 'sqrt', the one
//   function up there a library cannot answer wrong: IEEE 754 says it
//   is rounded, so every library that follows it says the same thing.
//
// The second probe was over the five that were left, and it is the
// one that changed the README: 44 478 answers — every power of two
// from 2^-1074 to 2^1023, the subnormals, both infinities and the NaN,
// into 'frexp', 'ldexp', 'modf', 'deg' and 'rad' — and **not one of
// them differed**.  They are arithmetic and not the library's:
// 'frexp' and 'ldexp' are a shift, 'modf' subtracts the part it has
// just taken, and 'deg' and 'rad' multiply by 180/pi, a constant
// written into both libraries in the same 37 digits.  One rounding
// each, and two programs that follow IEEE 754 do it the same way.  So
// five functions §6.7 used to list with the library's are asked here
// after all, and §6.7 says so.
//
// Three answers it does not ask, and one it asks and then settles.
// None of the three is anybody's to give:
//
//   'math.randomseed()' with no arguments.  PUC-Lua takes its seed
//   from 'time(NULL)' and the address of a local, so what it answers
//   is two numbers that are different on any two machines and on any
//   two runs: §6.8.  There is nothing to compare.  Every other form
//   of 'randomseed' is asked, and so is every draw that follows one.
//
//   The answer of any of the fourteen up there to a number.  Without
//   cgo this port gets it right to fourteen digits and not to
//   fifteen; with cgo the bits are the host libm's.  A grid that
//   asked it would be red on one machine and green on another.
//
//   Which position 'math.fmod' complains about when it is handed
//   nothing at all.  It is the one place in the whole of PUC-Lua
//   where two argument checkers sit in the same argument list — the
//   two 'luaL_checknumber's of 'l_mathop(fmod)(...)' — and C leaves
//   the order they are evaluated in to the compiler: §6.20.  clang
//   answers "#1" and gcc answers "#2", from the same source on the
//   same day, and this port answers "#1" on both, which is what the
//   reference it was written against answers.  So the grid asks
//   'math.fmod(1)', the one question there with only one position
//   that can be wrong, and not 'math.fmod()'.
//
// And the one it asks and then makes one thing of:
//
//   Which of the two names 'atan' and 'atan2' an error message says.
//   They are one function -- 'atan2' is the name 'LUA_COMPAT_MATHLIB'
//   adds for it -- and the message does not name the function as the
//   source spelled it but as 'findfield' found it, walking
//   'package.loaded.math' with 'next'.  Which key it reaches first is
//   which the hash seed put there, and the seed is made from the clock
//   and a heap address, so it is a different one in every process.
//   PUC-Lua answered 'math.atan' 8 times in 12 and 'math.atan2' the
//   other 4; this port answered 7 and 5.  Two processes pick
//   independently and disagree half the time -- not the C library's,
//   not the clock's alone and not a port's.  So the ten messages
//   that could say either are asked, and then made to say one; see
//   mathSettle below.
//
// Four ways, then, that an answer this library does not own can be
// kept out of a corpus, and this half uses all four: leave the
// question out (the clock's seed and the fourteen's answers), take
// the part that is not anybody's out of the answer (the two names of
// 'atan'), ask a different question with one fewer thing to decide
// (the position 'fmod' complains about), and ask the question and
// hold only what is reproducible (the shape of 'randomseed()').  The
// fifth is the one the mutation test found and this file does not
// use: a thirty-first digit of pi.  'mathPI' is 37 digits of it, and
// dropping the last six changes nothing, because C parses the literal
// as a double and so does Go, and both round the same 37 digits to
// the same one of the 2^64 there are.  It is the one mutant here that
// is not a mutation, and it is kept, because a corpus that caught it
// would be one over-fitted to a constant.

// mathVal is one value the library can be handed, with the tag that
// names it in a case.  The tag matters because the answer is a number
// and not a name: what a case says about 1 and about 1.0 is the whole
// of the difference between them in Lua 5.4.
type mathVal struct {
	tag  string
	expr string
}

var mathVals = []mathVal{
	{"0", "0"}, {"neg0", "-0.0"}, {"1", "1"}, {"neg1", "-1"},
	{"7", "7"}, {"neg7", "-7"}, {"1f", "1.0"}, {"neg1f", "-1.0"},
	{"half", "0.5"}, {"neghalf", "-0.5"}, {"1.5", "1.5"}, {"neg1.5", "-1.5"},
	{"2.5", "2.5"}, {"neg2.5", "-2.5"}, {"1e15", "1e15"}, {"1e16", "1e16"},
	{"1e17", "1e17"}, {"1e300", "1e300"}, {"1e-300", "1e-300"},
	{"2p31", "2147483648"}, {"2p53", "9007199254740992"},
	/* the first integer a float cannot hold, which is what the
	   shortcut in 'floor' and 'ceil' is for: without it 2^53+1
	   would come back as 2^53 */
	{"2p53p1", "9007199254740993"},
	{"2p62", "4611686018427387904"}, {"neg2p62", "-4611686018427387904"},
	{"2p63f", "9223372036854775808.0"},
	/* the one float at the other end of the integers, and the only
	   one that is an integer: 2^63 is too big, -2^63 is not */
	{"neg2p63f", "-9223372036854775808.0"},
	{"maxint", "math.maxinteger"}, {"minint", "math.mininteger"},
	{"huge", "math.huge"}, {"neghuge", "-math.huge"},
	{"nan", "0/0"}, {"pi", "math.pi"},
	// the same numbers as strings, which is a different road into
	// every function: a string becomes a float and never an integer
	{"s1", `"1"`}, {"sneg1", `"-1"`}, {"s1.0", `"1.0"`}, {"s1.5", `"1.5"`},
	{"s_sp1", `" 1 "`}, {"s1e2", `"1e2"`}, {"s0x10", `"0x10"`},
	{"sabc", `"abc"`}, {"snone", `""`},
}

// mathFmodVals are the divisors and dividends of a 'fmod'.  A divisor
// of 0 is refused, a divisor of -1 is not, and the smallest integer
// over either of them is where the two rules show.
var mathFmodVals = []mathVal{
	{"7", "7"}, {"neg7", "-7"}, {"0", "0"}, {"1", "1"}, {"neg1", "-1"},
	{"3", "3"}, {"neg3", "-3"}, {"7.5", "7.5"}, {"neg7.5", "-7.5"},
	{"0.0", "0.0"}, {"1.0", "1.0"}, {"neg1.0", "-1.0"},
	{"minint", "math.mininteger"}, {"maxint", "math.maxinteger"},
	{"neg2p63f", "-9223372036854775808.0"},
}

// mathUltVals are the two ends of an 'ult', which reads its arguments
// as unsigned: the smallest integer is the largest one there.
var mathUltVals = []mathVal{
	{"0", "0"}, {"1", "1"}, {"neg1", "-1"}, {"2p62", "4611686018427387904"},
	{"minint", "math.mininteger"}, {"maxint", "math.maxinteger"},
	{"neg7", "-7"}, {"7", "7"},
}

// mathFns are the thirty-one names the library answers to, and the
// four whose answer to a bad argument says '#2': 'ldexp' cannot say
// that about an exponent it was never handed a mantissa for.
//
// Twelve of them are asked only here.  They are the ones the C library
// answers for PUC-Lua and Go's own answers for this port, and what
// they say about a table is a thing two programs can agree on
// whatever they say about 0.5.
var mathFns = []struct {
	name string
	two  bool
}{
	{"abs", false}, {"acos", false}, {"asin", false}, {"atan", false},
	{"atan2", true}, {"ceil", false}, {"cos", false}, {"cosh", false},
	{"deg", false}, {"exp", false}, {"floor", false}, {"fmod", true},
	{"frexp", false}, {"ldexp", true}, {"log", false}, {"log10", false},
	{"max", false}, {"min", false}, {"modf", false}, {"pow", true},
	{"rad", false}, {"random", false}, {"randomseed", false},
	{"sin", false}, {"sinh", false}, {"sqrt", false}, {"tan", false},
	{"tanh", false}, {"tointeger", false}, {"type", false},
	{"ult", false},
}

// mathLdexpVals are the mantissas of an 'ldexp'.  What makes one
// worth handing it is its exponent, not its own size, so the list is
// short: a power of two walks into the answer and out again.
var mathLdexpVals = []mathVal{
	{"0", "0"}, {"neg0", "-0.0"}, {"1", "1"}, {"neg1", "-1"},
	{"3", "3"}, {"1.5", "1.5"}, {"neg1.5", "-1.5"},
	{"2p62", "4611686018427387904"}, {"neg2p62", "-4611686018427387904"},
	{"maxint", "math.maxinteger"}, {"minint", "math.mininteger"},
	{"half", "0.5"}, {"tiny", "5e-324"}, {"huge", "math.huge"},
	{"neghuge", "-math.huge"}, {"nan", "0/0"},
}

// mathLdexpExps are the exponents of an 'ldexp'.  The last four are
// the ones C cannot hold in the 'int' it truncates to: 2^31 wraps to
// -2^31 and 2^32 to 0, so 'ldexp(1, 2147483648)' is 0.0 and
// 'ldexp(1, -2147483649)' is inf, which is 2^2147483647.
var mathLdexpExps = []mathVal{
	{"0", "0"}, {"1", "1"}, {"neg1", "-1"}, {"2", "2"}, {"neg2", "-2"},
	{"30", "30"}, {"neg30", "-30"}, {"53", "53"}, {"neg53", "-53"},
	{"300", "300"}, {"neg300", "-300"}, {"1024", "1024"},
	{"neg1024", "-1024"}, {"2p31m1", "2147483647"},
	{"2p31", "2147483648"}, {"neg2p31", "-2147483648"},
	{"neg2p31m1", "-2147483649"}, {"2p32", "4294967296"},
}

// mathFrexpVals are what a 'frexp' is asked besides the values above:
// the answer is a fraction and an exponent, and every power of two is
// an exponent there is a fraction for.  The ends are the ones that
// matter: 5e-324 is the smallest thing it can be handed and still be
// a number.
var mathFrexpVals = []mathVal{
	{"2p-1074", "5e-324"}, {"2p-1073", "1e-323"},
	{"2p-1022", "2.2250738585072014e-308"}, {"2p-60", "8.673617379884035e-19"},
	{"2p-31", "4.6566128730773926e-10"}, {"2p-4", "0.0625"},
	{"2p31", "2147483648"}, {"2p53", "9007199254740992"},
	{"2p62", "4611686018427387904"}, {"2p1023", "8.98846567431158e307"},
	{"maxfloat", "1.7976931348623157e308"}, {"1e300", "1e300"},
	{"1e-300", "1e-300"}, {"1e-310", "1e-310"}, {"1e-320", "1e-320"},
	{"1e15", "1e15"}, {"1e17", "1e17"}, {"third", "0.3333333333333333"},
}

var mathChunk = `
local n = 0
local function P(tag, ...)
  n = n + 1
  local p = {}
  for k = 1, select("#", ...) do p[k] = q((select(k, ...))) end
  print(n .. "\t" .. tag .. "\t" .. table.concat(p, "\t"))
end
-- a case that raises prints the message, which is part of the answer
local function C(fn, ...)
  local ok, v = pcall(fn, ...)
  if ok then return v end
  return "raise: " .. tostring(v)
end
-- two values for a function that answers two: 'randomseed' hands back
-- the two seeds it was given, and what it was given is not always what
-- it was handed
local function C2(fn, ...)
  local a = {pcall(fn, ...)}
  if not a[1] then return "raise: " .. tostring(a[2]) end
  return q(a[2]) .. "," .. q(a[3])
end
`

func TestMathFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the math fuzz")
	}
	var b strings.Builder
	b.WriteString(fuzzQuote)
	b.WriteString(mathChunk)
	n := 0
	emit := func(tag, call string) {
		n++
		b.WriteString("P(" + strconv.Quote(tag) + ", " + call + ")\n")
	}
	/* Each group counts itself, so the note the test writes says what
	   the corpus is made of without anybody having to add it up. */
	at := 0
	took := func() int {
		d := n - at
		at = n
		return d
	}

	// 1. The six functions whose answer is arithmetic and not the C
	//    library's, over every value there is a name for.  What they
	//    answer is a number, and in Lua 5.4 an integer and the float
	//    that is its equal are two different values that print two
	//    different things: 'floor' of 2.5 is 2, and 'abs' of the
	//    smallest integer is the smallest integer, which is what
	//    negating it does.
	for _, v := range mathVals {
		for _, fn := range []string{"abs", "floor", "ceil", "sqrt",
			"tointeger", "type"} {
			emit(fmt.Sprintf("%s/%s", fn, v.tag),
				"C(math."+fn+", "+v.expr+")")
		}
	}
	nRounding := took()

	// 2. 'fmod', one function with four rules inside it: an integer
	//    over an integer, an integer over -1 or 0, a float over a
	//    float, and the refusal that is only for one of them.
	for _, a := range mathFmodVals {
		for _, d := range mathFmodVals {
			emit(fmt.Sprintf("fmod/%s/%s", a.tag, d.tag),
				"C(math.fmod, "+a.expr+", "+d.expr+")")
		}
	}
	nFmod := took()

	// 3. 'max' and 'min', which compare in Lua's own way and so are
	//    asked about every pair: an integer and the float that is its
	//    equal are the same number to them and not the same value.
	for _, a := range mathVals {
		for _, d := range mathVals {
			emit(fmt.Sprintf("max/%s/%s", a.tag, d.tag),
				"C(math.max, "+a.expr+", "+d.expr+")")
			emit(fmt.Sprintf("min/%s/%s", a.tag, d.tag),
				"C(math.min, "+a.expr+", "+d.expr+")")
		}
	}
	// and three at a time, which is where a running maximum is asked
	for _, a := range []mathVal{{"1", "1"}, {"1f", "1.0"}, {"1.5", "1.5"}} {
		for _, d := range []mathVal{{"2", "2"}, {"2f", "2.0"}, {"2.5", "2.5"}} {
			for _, c := range []mathVal{{"3", "3"}, {"3f", "3.0"}, {"3.5", "3.5"}} {
				emit(fmt.Sprintf("max3/%s/%s/%s", a.tag, d.tag, c.tag),
					"C(math.max, "+a.expr+", "+d.expr+", "+c.expr+")")
				emit(fmt.Sprintf("min3/%s/%s/%s", a.tag, d.tag, c.tag),
					"C(math.min, "+a.expr+", "+d.expr+", "+c.expr+")")
			}
		}
	}
	nMaxMin := took()

	// 4. 'ult', which reads its two arguments as unsigned and so asks
	//    about an order that is not the order of the integers.
	for _, a := range mathUltVals {
		for _, d := range mathUltVals {
			emit(fmt.Sprintf("ult/%s/%s", a.tag, d.tag),
				"C(math.ult, "+a.expr+", "+d.expr+")")
		}
	}
	nUlt := took()

	// 5. 'modf', which answers twice: the part before the point and
	//    the part after it.  An integer is its own first answer and
	//    0.0 its second; a float whose fraction is 0 answers itself
	//    and 0.0; and an infinity is an infinity and a 0.0, which is
	//    the test the subtraction cannot be asked to make.
	for _, v := range mathVals {
		emit("modf/"+v.tag, "C2(math.modf, "+v.expr+")")
	}
	nModf := took()

	// 6. 'frexp', which answers a fraction in [0.5,1) and the
	//    exponent to raise 2 to.  It is a shift and not a
	//    calculation, so there is nothing two libraries can disagree
	//    about, and every power of two from the smallest number there
	//    is to the largest is asked.
	for _, v := range mathVals {
		emit("frexp/"+v.tag, "C2(math.frexp, "+v.expr+")")
	}
	for _, v := range mathFrexpVals {
		emit("frexp/"+v.tag, "C2(math.frexp, "+v.expr+")")
	}
	nFrexp := took()

	// 7. 'ldexp', the other half of 'frexp' and the only function in
	//    the library whose second argument is not a number but an
	//    int.  What C does with an exponent that will not fit in one
	//    is wrap it, and the four that do not fit are asked: 2^31 is
	//    -2^31 there, so 'ldexp(1, 2147483648)' is 0.0.
	for _, v := range mathLdexpVals {
		for _, e := range mathLdexpExps {
			emit("ldexp/"+v.tag+"/"+e.tag,
				"C(math.ldexp, "+v.expr+", "+e.expr+")")
		}
	}
	nLdexp := took()

	// 8. 'deg' and 'rad', which are a multiplication by 180/pi, a
	//    constant written into the library in the same digits on both
	//    sides.  One rounding, and not a function anybody's library
	//    gets to answer.
	for _, v := range mathVals {
		emit("deg/"+v.tag, "C(math.deg, "+v.expr+")")
		emit("rad/"+v.tag, "C(math.rad, "+v.expr+")")
	}
	nDegRad := took()

	// 9. 'random', which is the whole reason for this half.  Seeded
	//    with two integers it gives the same sequence on any machine,
	//    so every draw is a thing two programs can be asked about.
	//    The shapes are the number of arguments it is given: none is a
	//    float in [0,1), one is [1,m] except for 0, which is a full
	//    64-bit integer, and two is [m,n].
	b.WriteString("math.randomseed(1, 2)\n")
	for _, shape := range []struct {
		tag  string
		args string
	}{
		{"none", ""}, {"zero", "0"}, {"one", "1"}, {"two", "2"},
		{"six", "6"}, {"hundred", "100"}, {"2p31", "2147483648"},
		{"maxint", "math.maxinteger"}, {"1-1", "1, 1"}, {"1-2", "1, 2"},
		{"1-100", "1, 100"}, {"neg5-5", "-5, 5"}, {"0-0", "0, 0"},
		{"minint-maxint", "math.mininteger, math.maxinteger"},
		{"1-maxint", "1, math.maxinteger"},
	} {
		for i := 1; i <= 8; i++ {
			call := "C(math.random)"
			if shape.args != "" {
				call = "C(math.random, " + shape.args + ")"
			}
			emit(fmt.Sprintf("random/%s/%d", shape.tag, i), call)
		}
	}
	// and the one shape that is not a shape: three arguments is one
	// more than it takes, and it says so without a name or a number
	// in the message, which is the only message in the library that
	// has neither
	emit("random/1-2-3", "C(math.random, 1, 2, 3)")
	// and the same shapes after a seed of one integer, which is the
	// other form of 'randomseed'
	for _, seed := range []string{"0", "1", "-1", "42", "math.maxinteger"} {
		b.WriteString("math.randomseed(" + seed + ")\n")
		for i := 1; i <= 6; i++ {
			emit(fmt.Sprintf("random/seed%s/%d", seed, i),
				"C(math.random, 1, 100)")
		}
	}
	nRand := took()

	// 10. 'randomseed' itself, which answers with the two seeds it was
	//    given -- the second one defaulting to 0 -- and is the only
	//    function in the library that answers twice.  What it is given
	//    that is not an integer is asked too.
	for _, s := range []string{"0", "1", "-1", "42", "math.maxinteger",
		"math.mininteger", "1, 2", "1, 0", "0, 1", "-1, -1",
		"math.maxinteger, math.maxinteger", "1.5", `"42"`, "nil",
		"1, 2, 3"} {
		emit("randomseed/"+strings.ReplaceAll(s, ", ", "/"),
			"C2(math.randomseed, "+s+")")
	}
	// and a draw after each, which is the only way to see the seed
	for _, s := range []string{"0", "1", "-1", "42", "1, 2", "1, 0",
		"math.maxinteger"} {
		b.WriteString("math.randomseed(" + s + ")\n")
		emit("randomseed-draw/"+strings.ReplaceAll(s, ", ", "/"),
			"C(math.random, 0)")
	}
	nSeed := took()

	// 11. and every function asked about the things that are not
	//    numbers and never will be, which is where the messages are.
	//    This is the one question the grid asks of the twelve
	//    functions it does not otherwise ask: what they say about a
	//    table is theirs and not the C library's, whatever they say
	//    about 0.5.
	for _, k := range []struct{ tag, expr string }{
		{"nil", "nil"}, {"true", "true"}, {"abc", `"abc"`},
		{"table", "{}"}, {"func", "print"},
	} {
		for _, fn := range mathFns {
			switch {
			case fn.two:
				/* the message says '#2', because the first
				   argument has to be there for the second
				   one to be the wrong one */
				emit("wrong2/"+fn.name+"/"+k.tag,
					"C(math."+fn.name+", 1, "+k.expr+")")
			case fn.name == "log":
				/* the base is optional, so this one is
				   asked in both positions */
				emit("wrong/"+fn.name+"/"+k.tag,
					"C(math.log, "+k.expr+")")
				emit("wrong2/"+fn.name+"/"+k.tag,
					"C(math.log, 1, "+k.expr+")")
			default:
				emit("wrong/"+fn.name+"/"+k.tag,
					"C(math."+fn.name+", "+k.expr+")")
			}
		}
	}
	/* and with no arguments at all, which is the one thing every
	   function has to say about being handed nothing.

	   'fmod' is the one that is left out, and it is left out because
	   it is the only function in the library with two checkers in one
	   argument list: which of them C reaches first is the compiler's
	   business, so what it says then is "bad argument #1" under clang
	   and "#2" under gcc (§6.20).  What is asked instead is one
	   argument, which is a question with one position in it. */
	for _, fn := range mathFns {
		if fn.name == "random" || fn.name == "randomseed" {
			/* what they answer then is the clock's */
			continue
		}
		if fn.name == "fmod" {
			emit("onearg/fmod", "C(math.fmod, 1)")
			continue
		}
		emit("noargs/"+fn.name, "C(math."+fn.name+")")
	}
	nWrong := took()

	// 12. the four numbers that are written into the library
	for _, c := range []string{"pi", "huge", "maxinteger", "mininteger"} {
		emit("const/"+c, "q(math."+c+")")
	}
	nConst := took()

	cases := n
	if os.Getenv("MATHFUZZ_DUMP") != "" {
		os.WriteFile(os.Getenv("MATHFUZZ_DUMP"), []byte(b.String()), 0o644)
	}
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	base := t.TempDir()
	dirs := map[string]string{
		"ref":  filepath.Join(base, "ref"),
		"ours": filepath.Join(base, "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "mathfuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ours := diffBuild(t)
	want := mathRun(t, ref, dirs["ref"])
	got := mathRun(t, ours, dirs["ours"])
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the math chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := mathSettle(diffNormalise(want)), mathSettle(diffNormalise(got))
	if w == g {
		t.Logf("%d rounding, %d fmod, %d max and min, %d ult, %d modf, "+
			"%d frexp, %d ldexp, %d deg and rad, %d random, "+
			"%d randomseed, %d that are not numbers and %d constants: "+
			"%d cases, all of them PUC-Lua's", nRounding, nFmod, nMaxMin,
			nUlt, nModf, nFrexp, nLdexp, nDegRad, nRand, nSeed, nWrong,
			nConst, cases)
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	reported := 0
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var a, c string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			c = gl[i]
		}
		if a == c {
			continue
		}
		reported++
		if reported > 20 {
			t.Logf("... and more")
			break
		}
		t.Errorf("line %d\n  PUC-Lua: %s\n  this:    %s", i+1, a, c)
	}
	if reported == 0 {
		t.Errorf("the lines differ but none of them line up")
	}
}

// TestMathInProcess asks the same library the same questions in this
// process.
//
// The grid runs its 4 787 cases in a subprocess, which is the right way
// to compare two interpreters and the wrong way to answer 'go test
// -cover': a subprocess's coverage is the subprocess's and nobody
// measures it.  Four functions of lmathlib.go were at 0% for exactly
// that reason, and four more were half covered.  So what follows is
// the same library entered through the API, in this process, which is
// what puts every function of lmathlib.go at 100%.
//
// It also holds the two things the grid cannot.  'math.randomseed()'
// with nothing at all answers the clock and a heap address, so no two
// runs agree and there is nothing for a grid to compare; here what is
// asked of it is only what kind of thing it answers and how old it is.
// And 'math.random' with three arguments is the one arity error in the
// library whose message names nothing: not a position, not a number,
// not a function.
//
// Two of the answers here are the C library's, so they are asked only
// where every C library agrees: 2^10 and tanh(0).  The answers that are
// the library's to give at every other argument belong to
// lmathlib_test.go, which is where the port's own three corrections to
// Go's implementations are pinned.
func TestMathInProcess(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local function eq(got, want, what)
		  if got ~= want then
		    error(string.format("%s: got %s want %s", what,
		      tostring(got), tostring(want)), 2)
		  end
		end

		-- 1. The six whose answer is arithmetic, in both of their
		--    halves: an integer in is an integer out, and a float
		--    that will not fit in one is a float out.
		eq(math.abs(-3), 3, "abs int")
		eq(math.type(math.abs(-3)), "integer", "abs int type")
		eq(math.abs(-1.5), 1.5, "abs float")
		eq(math.type(math.abs(-1.5)), "float", "abs float type")
		eq(math.floor(2.5), 2, "floor")
		eq(math.type(math.floor(2.5)), "integer", "floor type")
		eq(math.floor(-2.5), -3, "floor neg")
		eq(math.floor(3), 3, "floor int")
		eq(math.floor(1e300), 1e300, "floor huge")
		eq(math.type(math.floor(1e300)), "float", "floor huge type")
		eq(math.ceil(2.5), 3, "ceil")
		eq(math.type(math.ceil(2.5)), "integer", "ceil type")
		eq(math.ceil(-2.5), -2, "ceil neg")
		eq(math.type(math.ceil(-2.5)), "integer", "ceil neg type")
		eq(math.ceil(3), 3, "ceil int")
		eq(math.ceil(1e300), 1e300, "ceil huge")
		eq(math.type(math.ceil(1e300)), "float", "ceil huge type")
		eq(math.sqrt(4), 2.0, "sqrt")
		eq(math.type(math.sqrt(4)), "float", "sqrt type")
		--    and the two signs it keeps: -0.0 has one and a NaN
		--    does not, and asking for the reciprocal is the only
		--    way to see the first, since -0.0 == 0.0.  They are
		--    what makes 'sqrt' the one function up there a library
		--    that follows IEEE 754 cannot answer wrongly.
		eq(1 / math.sqrt(-0.0), -math.huge, "sqrt -0")
		eq(1 / math.sqrt(0.0), math.huge, "sqrt 0")
		eq(math.sqrt(-1) ~= math.sqrt(-1), true, "sqrt -1")
		eq(math.sqrt(-math.huge) ~= math.sqrt(-math.huge), true,
		  "sqrt -inf")
		eq(math.tointeger(3.0), 3, "tointeger")
		eq(math.tointeger(3.5), nil, "tointeger frac")
		eq(math.tointeger("3"), 3, "tointeger str")
		eq(math.tointeger("abc"), nil, "tointeger str bad")
		eq(math.tointeger(2 ^ 63), nil, "tointeger too big")
		--    and the one integer a float cannot hold, which is what
		--    the shortcut at the top of 'floor' and 'ceil' is for:
		--    the float it would otherwise be asked about is 2^53.
		eq(math.floor(9007199254740993), 9007199254740993, "floor 2^53+1")
		eq(math.ceil(9007199254740993), 9007199254740993, "ceil 2^53+1")
		eq(math.abs(9007199254740993), 9007199254740993, "abs 2^53+1")
		eq(math.tointeger(9007199254740993), 9007199254740993,
		  "tointeger 2^53+1")
		--    and the float at the far end of the integers, which is
		--    an integer and is the smallest one
		eq(math.floor(-9223372036854775808.0), -9223372036854775808,
		  "floor -2^63")
		eq(math.type(math.floor(-9223372036854775808.0)), "integer",
		  "floor -2^63 type")
		eq(math.type(1), "integer", "type int")
		eq(math.type(1.0), "float", "type float")
		eq(math.type("x"), nil, "type str")

		-- 2. 'fmod' and its four rules.  The one that surprises is
		--    the float: a divisor of 0.0 is only refused for two
		--    integers, and for two floats C's fmod answers a NaN
		--    and says nothing.
		eq(math.fmod(7, 3), 1, "fmod")
		eq(math.type(math.fmod(7, 3)), "integer", "fmod type")
		eq(math.fmod(-7, 3), -1, "fmod neg")
		eq(math.fmod(7, -3), 1, "fmod neg divisor")
		eq(math.fmod(math.mininteger, -1), 0, "fmod minint")
		eq(math.type(math.fmod(math.mininteger, -1)), "integer",
		  "fmod minint type")
		eq(math.fmod(7.5, 2), 1.5, "fmod float")
		eq(math.fmod(-7.5, 2), -1.5, "fmod float neg")
		eq(math.fmod(7.5, -2), 1.5, "fmod float neg divisor")
		local ok, e = pcall(math.fmod, 7, 0)
		eq(ok, false, "fmod 0 ok")
		eq(e, "bad argument #2 to 'math.fmod' (zero)", "fmod 0 msg")

		-- 3. 'max' and 'min', which keep the first of two numbers
		--    that are equal and not the second: 1.0 and 1 are the
		--    same number and two different values, and which one
		--    comes back is which one went in first.
		eq(math.max(1, 2, 3), 3, "max")
		eq(math.min(1, 2, 3), 1, "min")
		--    and with the smallest last, which is the only order in
		--    which the running minimum is ever moved
		eq(math.min(3, 2, 1), 1, "min reversed")
		eq(math.type(math.min(3, 2, 1)), "integer", "min reversed type")
		eq(math.max(1.0, 1), 1.0, "max float first")
		eq(math.type(math.max(1.0, 1)), "float", "max float first type")
		eq(math.max(1, 1.0), 1, "max int first")
		eq(math.type(math.max(1, 1.0)), "integer", "max int first type")
		ok, e = pcall(math.max)
		eq(ok, false, "max none")
		eq(e, "bad argument #1 to 'math.max' (value expected)",
		  "max none msg")
		ok, e = pcall(math.min)
		eq(ok, false, "min none")
		eq(e, "bad argument #1 to 'math.min' (value expected)",
		  "min none msg")

		-- 4. 'ult', which reads its arguments as unsigned and so
		--    puts the smallest integer at the far end.
		eq(math.ult(-1, 1), false, "ult -1 1")
		eq(math.ult(1, -1), true, "ult 1 -1")
		eq(math.ult(0, 0), false, "ult 0 0")

		-- 5. 'modf', in all four of its shapes: an integer, a
		--    float, and the two infinities, which are the ones the
		--    subtraction would answer a NaN for.
		local a, b = math.modf(3.5)
		eq(a, 3.0, "modf ip")
		eq(b, 0.5, "modf fp")
		a, b = math.modf(-3.5)
		eq(a, -3.0, "modf neg ip")
		eq(b, -0.5, "modf neg fp")
		a, b = math.modf(3)
		eq(a, 3, "modf int ip")
		eq(math.type(a), "integer", "modf int ip type")
		eq(b, 0.0, "modf int fp")
		eq(math.type(b), "float", "modf int fp type")
		a, b = math.modf(9007199254740993)
		eq(a, 9007199254740993, "modf 2^53+1")
		eq(math.type(a), "integer", "modf 2^53+1 type")
		a, b = math.modf(math.huge)
		eq(a, math.huge, "modf inf")
		eq(b, 0.0, "modf inf fp")
		a, b = math.modf(-math.huge)
		eq(a, -math.huge, "modf -inf")
		eq(b, 0.0, "modf -inf fp")
		a, b = math.modf(1e300)
		eq(a, 1e300, "modf huge")
		eq(b, 0.0, "modf huge fp")

		-- 6. 'frexp' and 'ldexp', and the four exponents C
		--    cannot hold in the 'int' it truncates to.
		local f, x = math.frexp(8)
		eq(f, 0.5, "frexp frac")
		eq(x, 4, "frexp exp")
		f, x = math.frexp(0)
		eq(f, 0.0, "frexp 0")
		eq(x, 0, "frexp 0 exp")
		f, x = math.frexp(-0.0)
		eq(f, -0.0, "frexp -0")
		eq(x, 0, "frexp -0 exp")
		f, x = math.frexp(3)
		eq(f, 0.75, "frexp 3")
		eq(x, 2, "frexp 3 exp")
		eq(math.ldexp(0.5, 4), 8.0, "ldexp")
		eq(math.ldexp(1, 0), 1.0, "ldexp 0")
		eq(math.ldexp(1, 2147483648), 0.0, "ldexp wrap")
		eq(math.ldexp(1, -2147483649), math.huge, "ldexp wrap down")
		eq(math.ldexp(3, -1074), 1.4821969375237e-323, "ldexp subnormal")

		-- 7. 'deg' and 'rad', one multiplication by 180/pi.
		eq(math.deg(math.pi), 180.0, "deg pi")
		eq(math.deg(1), 57.295779513082323, "deg 1")
		eq(math.rad(180), math.pi, "rad 180")
		eq(math.rad(1), 0.017453292519943295, "rad 1")

		-- 8. The two the grid cannot ask at any other argument,
		--    because they are the C library's: 2^10 and tanh(0)
		--    are the only ones every library answers alike.  And
		--    the three the port had to work out for itself, at 0,
		--    where they are exact too.
		eq(math.pow(2, 10), 1024.0, "pow")
		eq(math.type(math.pow(2, 10)), "float", "pow type")
		eq(math.tanh(0), 0.0, "tanh")
		eq(math.exp(0), 1.0, "exp 0")
		eq(math.exp(-math.huge), 0.0, "exp -inf")
		eq(math.sinh(0), 0.0, "sinh 0")
		eq(math.cosh(0), 1.0, "cosh 0")

		-- 9. The generator, seeded with two integers, which is
		--    the only seed two programs can be asked about.
		--    These are PUC-Lua's own draws, and they are one
		--    stream: the third of them is the one that takes the
		--    high bit that makes the float in [0,1) a subtraction
		--    rather than a shift.
		math.randomseed(1, 2)
		--    and they are written to seventeen digits, which is one
		--    more than 'tostring' uses: the float is 53 bits of the
		--    word, and a corpus that only asked for fourteen would
		--    not notice a fifty-second.
		local draws = {}
		for i = 1, 6 do
		  draws[i] = string.format("%.17g", math.random())
		end
		eq(table.concat(draws, " "),
		  "0.44949358084855551 0.22576880156399304 "..
		  "0.73957236111052194 0.34305203708388043 "..
		  "0.13902460297941366 0.29598176925959574", "random f")
		math.randomseed(1, 2)
		draws = {}
		for i = 1, 6 do draws[i] = tostring(math.random()) end
		eq(table.concat(draws, " "),
		  "0.44949358084856 0.22576880156399 0.73957236111052 "..
		  "0.34305203708388 0.13902460297941 0.2959817692596",
		  "random f tostring")
		draws = {}
		for i = 1, 6 do draws[i] = tostring(math.random(6)) end
		eq(table.concat(draws, " "), "1 4 6 4 3 4", "random 6")
		draws = {}
		for i = 1, 6 do draws[i] = tostring(math.random(10, 20)) end
		eq(table.concat(draws, " "), "11 12 16 13 15 10", "random 10 20")
		-- 5.3.4: math.random(0) is empty interval (1..0); 5.4 returned a full int
		ok, e = pcall(math.random, 0)
		eq(ok, false, "random 0 empty")
		eq(string.find(e, "interval is empty") ~= nil, true, "random 0 msg")
		-- pcall already consumed one nextrand; burn 3 more to match 5.4's four random(0)
		for _ = 1, 3 do math.random() end
		draws = {}
		for i = 1, 3 do draws[i] = tostring(math.random(-5, 5)) end
		eq(table.concat(draws, " "), "-2 -2 0", "random -5 5")
		draws = {}
		for i = 1, 3 do draws[i] = tostring(math.random(1, 1)) end
		eq(table.concat(draws, " "), "1 1 1", "random 1 1")
		-- 5.3.4 rejects [minint, maxint] as "interval too large" (5.4 projects it)
		ok, e = pcall(math.random, math.mininteger, math.maxinteger)
		eq(ok, false, "random minint maxint")
		eq(string.find(e, "interval too large") ~= nil, true, "random minint maxint msg")
		a, b = math.randomseed(7)
		eq(a, 7, "randomseed 7")
		eq(b, 0, "randomseed 7 second")
		a, b = math.randomseed(7, 8)
		eq(a, 7, "randomseed 7 8")
		eq(b, 8, "randomseed 7 8 second")
		ok, e = pcall(math.randomseed, "abc")
		eq(ok, false, "randomseed str")
		eq(e, "bad argument #1 to 'math.randomseed' "..
		  "(number expected, got string)", "randomseed str msg")
		ok, e = pcall(math.randomseed, 1, "abc")
		eq(ok, false, "randomseed str 2")
		eq(e, "bad argument #2 to 'math.randomseed' "..
		  "(number expected, got string)", "randomseed str 2 msg")
		ok, e = pcall(math.random, 2, 1)
		eq(ok, false, "random empty")
		eq(e, "bad argument #1 to 'math.random' (interval is empty)",
		  "random empty msg")

		-- 10. The three shapes that are not shapes: three
		--     arguments is one more than 'random' takes, and it
		--     says so with no name and no number in the message,
		--     which makes it the only message in the library that
		--     has neither.
		for _, args in ipairs{{1, 2, 3}, {1, 2, 3, 4}, {0, 0, 0}} do
		  ok, e = pcall(math.random, table.unpack(args))
		  eq(ok, false, "random arity")
		  eq(e, "wrong number of arguments", "random arity msg")
		end

		-- 11. 'randomseed' with nothing at all, which is the one
		--     question the grid cannot ask: PUC-Lua takes its
		--     seed from the clock and from the address of a
		--     local, so what it answers is two numbers no two
		--     runs share.  What it answers *is* two integers,
		--     and the first is the time, which is a thing this
		--     process can be asked about (§6.8).
		local s1, s2 = math.randomseed()
		eq(math.type(s1), "integer", "randomseed none type 1")
		eq(math.type(s2), "integer", "randomseed none type 2")
		eq(s1 > 0, true, "randomseed none positive")
		eq(s2 ~= 0, true, "randomseed none not 0")
		local t = os.time()
		math.randomseed()
		s1, s2 = math.randomseed()
		eq(s1 >= t, true, "randomseed none is the time")
		eq(s2 ~= 0, true, "randomseed none addr")
		-- and the generator it leaves behind still draws
		eq(math.type(math.random(1, 100)), "integer", "random after")
	`); err != nil {
		t.Fatal(err)
	}
}

// mathSettle removes the one answer in this corpus that is not Lua's:
// which of the two names of 'atan'/'atan2' a message says.  It is the
// hash seed's, and the two sides answer it independently (see the note
// at the top of this file).
var mathAtan2Re = regexp.MustCompile(`'math\.atan2'`)

func mathSettle(s string) string {
	return mathAtan2Re.ReplaceAllString(s, "'math.atan'")
}

// mathRun runs the chunk, the way the reference was invoked.
func mathRun(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "mathfuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}
