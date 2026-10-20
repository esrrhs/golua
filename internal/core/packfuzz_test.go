package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The tenth fuzz, over 'string.pack', 'string.unpack' and
// 'string.packsize'.
//
// These three are the last C mini-language in the standard library
// without a grid of its own: a format string is a program in a little
// language of sizes, alignments, endianness and options, and 'lstrlib.c'
// is an interpreter for it.  A probe of 907 hand-chosen cases over all
// three went 0 lines red, which by the rule of this project is not yet
// an answer -- the same corpus was then asked twice with 'lstrlib.go'
// deliberately broken, and saw the break both times:
//
//  1. the sign extension in 'packint', taken out.  A negative integer
//     packed into more than eight bytes is 0xff in the bytes above the
//     eighth, not 0x00: C writes them from 'MC', and this port writes
//     them from 'strMC'.  Without it, 'string.pack("i9", -1)' is
//     ffffffffffffff00 instead of ffffffffffffffffff.  36 lines red.
//
//  2. '*ntoalign' in 'getdetails', forced to 0.  'string.packsize(
//     "!8i1i4")' is 8, not 5: the 'i4' does not start at the fifth
//     byte but at the first multiple of eight the 'i1' has not used.
//     4 lines red.
//
// So the corpus can see a difference in there, and the grid is what
// asks the rest of the questions.  It is the smallest of the two
// answers above that the grid is really for: four lines out of 907 is
// what a hand-picked probe misses.
//
// There is one thing the format language cannot be asked about here,
// and it is not a difference: 'MAXALIGN' is 'offsetof(struct cD, u)'
// in C, which is the alignment of the widest thing a C compiler on the
// host knows how to store -- 8 on every machine this port runs on.  A
// '!16' therefore sets a maximum alignment of 16 and is accepted (the
// size is checked against 'MAXINTSIZE', not against 'MAXALIGN'), while
// a '!17' is refused.

// packChunk is the harness.
//
// Every answer is rendered before it is printed, and rendered the same
// way on both sides:
//
//   - a string is printed as its bytes in hex, because what the three
//     functions make and read is bytes, and a byte that is not
//     printable is the interesting case more often than not.  A format
//     string of one byte can be the NUL byte, and the complaint about
//     it contains that NUL.
//   - a question that was refused is printed as 'false|<what was
//     raised>', not skipped: a format that is rejected is being
//     answered too, and the rejection is most of what the grid is
//     asking about.  '|' is not a byte either side can produce.
var packChunk = `
local function hex(s)
  local t = {}
  for i = 1, #s do t[i] = string.format("%02x", s:byte(i)) end
  return table.concat(t)
end

local function q(v)
  local t = type(v)
  if t == "string" then return hex(v) end
  if t == "number" or t == "boolean" then return tostring(v) end
  if t == "nil" then return "nil" end
  return t
end

-- one question, whether it answered or complained
local function R(ok, ...)
  local t = {tostring(ok)}
  for i = 1, select("#", ...) do t[#t + 1] = q((select(i, ...))) end
  return table.concat(t, "|")
end

-- the answers are already rendered, so this only lines them up
local function P(i, name, ...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = (select(k, ...)) end
  print(i .. "\t" .. name .. "\t" .. table.concat(t, "\t"))
end

-- what 'packsize' says the format adds up to
local function ps(f)
  return R(pcall(string.packsize, f))
end

-- what 'pack' makes of it
local function pk(f, ...)
  return R(pcall(string.pack, f, ...))
end

-- the three at once: what the format adds up to, what 'pack' makes of
-- it, and what 'unpack' reads back out of that
local function all(f, ...)
  local a = table.pack(...)
  local function both()
    return string.unpack(f, string.pack(f, table.unpack(a, 1, a.n)))
  end
  return ps(f) .. " " ..
    R(pcall(string.pack, f, table.unpack(a, 1, a.n))) .. " " ..
    R(pcall(both))
end

-- 'unpack' out of a buffer of its own
local function up(f, s, pos)
  if pos ~= nil then return R(pcall(string.unpack, f, s, pos)) end
  return R(pcall(string.unpack, f, s))
end

-- 'unpack' out of what the format itself packed, at a position
local function upPos(f, pos, ...)
  local a = table.pack(...)
  local ok, p = pcall(string.pack, f, table.unpack(a, 1, a.n))
  if not ok then return R(ok, p) end
  if pos ~= nil then return R(pcall(string.unpack, f, p, pos)) end
  return R(pcall(string.unpack, f, p))
end

-- a round trip: the bytes, and what comes back out of them
local function rt(f, ...)
  local a = table.pack(...)
  local ok, p = pcall(string.pack, f, table.unpack(a, 1, a.n))
  if not ok then return R(ok, p) end
  return hex(p) .. " " .. R(pcall(string.unpack, f, p))
end
`

// packBuf is an eight-byte buffer of distinct bytes, for the questions
// 'unpack' is asked without anything to unpack.
const packBuf = `"\1\2\3\4\5\6\7\8"`

// packVal is the value an option takes, for a format built out of
// several of them.  The three that are not data take none: 'x' is
// padding, 'X' is alignment, and an endianness or an alignment on its
// own is only a setting.
type packOpt struct {
	o string
	v []string
}

func packOne(o, v string) packOpt {
	if v == "" {
		return packOpt{o: o}
	}
	return packOpt{o: o, v: []string{v}}
}

// packArgs is the values a list of options needs, in the order the
// format will ask for them.
func packArgs(opts []packOpt) string {
	var parts []string
	for _, o := range opts {
		parts = append(parts, o.v...)
	}
	return strings.Join(parts, ", ")
}

func TestPackFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the pack fuzz")
	}

	var b strings.Builder
	b.WriteString(packChunk)
	n, asked := 0, 0
	/* since is how many cases the group just written was. */
	last := 0
	since := func() int { d := n - last; last = n; return d }

	emit := func(name, call string) {
		n++
		if strings.HasPrefix(call, "all(") {
			asked += 3 /* 'packsize', 'pack' and the round trip back */
		} else {
			asked++
		}
		b.WriteString("P(" + strconv.Itoa(n) + ", " + strconv.Quote(name) +
			", " + call + ")\n")
	}
	lit := strconv.Quote

	// 1. Every byte of the alphabet, as a format of its own.
	//
	// The alphabet is all 256, not the 26 C looks up: a byte that is
	// not an option is an option refused, and the refusal is printed
	// with the byte in it, which is how 'invalid format option' is
	// pinned down.  's', 'c' and 'i' with no number take their
	// default; 'c' with none is refused.
	for c := 0; c < 256; c++ {
		f := lit(string([]byte{byte(c)}))
		tag := "byte " + strconv.Itoa(c)
		if c >= 32 && c < 127 {
			tag += " " + string(rune(c))
		}
		emit(tag+" size", "ps("+f+")")
		emit(tag+" pack 1", "pk("+f+", 1)")
		emit(tag+" unpack", "up("+f+", "+packBuf+")")
	}
	byteCases := since()

	// 2. Every size.
	//
	// 'i', 'I' and 's' read their size with 'getnumlimit', which
	// refuses anything outside [1, 16]; 'c' reads its with 'getnum',
	// which refuses nothing but a missing size.  Only the sizes that
	// are refused can say where the edge is, so the sweep runs past
	// it in both directions.
	intVals := []string{"0", "1", "-1", "127", "128", "255", "256",
		"2147483648", "2147483649", "9007199254740992",
		"-9007199254740992", "1.5"}
	for _, l := range []string{"i", "I"} {
		for sz := 0; sz <= 17; sz++ {
			f := lit(l + strconv.Itoa(sz))
			emit("size-of "+l+strconv.Itoa(sz), "ps("+f+")")
			for _, v := range intVals {
				emit(l+strconv.Itoa(sz)+" "+v, "pk("+f+", "+v+")")
			}
		}
	}
	// and the values at the edge of what each size can hold: the
	// largest it can hold, the smallest, and one past the largest
	for _, l := range []string{"i", "I"} {
		for sz := 1; sz <= 8; sz++ {
			pow := uint64(1) << uint(8*sz-1)
			f := lit(l + strconv.Itoa(sz))
			for _, v := range []string{
				strconv.FormatUint(pow-1, 10),
				"-" + strconv.FormatUint(pow, 10),
				strconv.FormatUint(pow, 10),
			} {
				emit("edge "+l+strconv.Itoa(sz)+" "+v, "pk("+f+", "+v+")")
			}
		}
	}
	strVals := []string{`""`, `"a"`, `"abc"`, `"0123456789"`,
		`string.rep("x", 20)`}
	for _, l := range []string{"s", "c"} {
		for sz := 0; sz <= 17; sz++ {
			f := lit(l + strconv.Itoa(sz))
			emit("size-of "+l+strconv.Itoa(sz), "ps("+f+")")
			for _, v := range strVals {
				emit(l+strconv.Itoa(sz)+" "+v, "pk("+f+", "+v+")")
			}
		}
	}
	sizeCases := since()

	// 3. Every alignment.
	//
	// '!' takes a number the same way 'i' does, and the number is a
	// maximum, not an order: what an option is aligned to is the
	// smaller of its own size and the maximum still standing.
	alignAfter := []packOpt{
		packOne("", ""), packOne("b", "1"), packOne("h", "1"),
		packOne("l", "1"), packOne("j", "1"), packOne("T", "1"),
		packOne("d", "1.5"), packOne("f", "1.5"), packOne("n", "1.5"),
		packOne("x", ""), packOne("z", `"ab"`), packOne("i1", "1"),
		packOne("i2", "1"), packOne("i4", "1"), packOne("i8", "1"),
		packOne("i16", "1"), packOne("I4", "1"), packOne("s4", `"ab"`),
		packOne("c4", `"xxxx"`), packOne("Xi4", "1"),
	}
	for sz := 0; sz <= 17; sz++ {
		for _, a := range alignAfter {
			f := lit("!" + strconv.Itoa(sz) + a.o)
			emit("size-of !"+strconv.Itoa(sz)+a.o, "ps("+f+")")
		}
	}
	for _, sz := range []int{0, 1, 2, 3, 4, 8, 16} {
		for _, a := range alignAfter {
			f := lit("!" + strconv.Itoa(sz) + a.o)
			emit("!"+strconv.Itoa(sz)+a.o, "pk("+f+packSep(a.v)+")")
		}
	}
	alignCases := since()

	// 4. Every endianness, over every option.
	//
	// '<' and '>' are an order, and they can be given in the middle
	// of a format as well as at its front; '=' is whatever the
	// machine is.
	endOpts := []packOpt{
		packOne("b", "1"), packOne("B", "1"), packOne("h", "1"),
		packOne("H", "1"), packOne("l", "1"), packOne("L", "1"),
		packOne("j", "1"), packOne("J", "1"), packOne("T", "1"),
		packOne("f", "1.5"), packOne("n", "1.5"), packOne("d", "1.5"),
		packOne("i1", "1"), packOne("i2", "1"), packOne("i4", "1"),
		packOne("i8", "1"), packOne("i16", "1"), packOne("I1", "1"),
		packOne("I4", "1"), packOne("s4", `"ab"`), packOne("c3", `"xxx"`),
		packOne("z", `"ab"`),
	}
	for _, e := range []string{"<", ">", "="} {
		for _, o := range endOpts {
			emit(e+o.o, "all("+lit(e+o.o)+packSep(o.v)+")")
		}
	}
	endCases := since()

	// 5. Every pair of options.
	//
	// Two options side by side is the smallest format in which
	// alignment means anything at all, and 16 of them two at a time
	// is every shape of it.
	twoOpts := []packOpt{
		packOne("b", "1"), packOne("B", "1"), packOne("h", "1"),
		packOne("H", "1"), packOne("l", "1"), packOne("L", "1"),
		packOne("j", "1"), packOne("J", "1"), packOne("T", "1"),
		packOne("f", "1.5"), packOne("n", "1.5"), packOne("d", "1.5"),
		packOne("i4", "1"), packOne("I4", "1"), packOne("s4", `"ab"`),
		packOne("c3", `"xxx"`),
	}
	for _, a := range twoOpts {
		for _, c := range twoOpts {
			emit(a.o+c.o, "all("+lit(a.o+c.o)+", "+packArgs([]packOpt{a, c})+")")
		}
	}
	twoCases := since()

	// 6. An 'X' between every pair.
	//
	// 'X' is the one option that looks at the option after itself:
	// it aligns to that option's size, and it is refused when there
	// is no such option, when the next thing is a 'c', or when the
	// next option is one whose size is 0.
	xOpts := []packOpt{
		packOne("i1", "1"), packOne("i2", "1"), packOne("i4", "1"),
		packOne("i8", "1"), packOne("I4", "1"), packOne("d", "1.5"),
		packOne("f", "1.5"), packOne("b", "1"), packOne("h", "1"),
		packOne("l", "1"), packOne("j", "1"), packOne("T", "1"),
	}
	for _, a := range xOpts {
		for _, c := range xOpts {
			f := a.o + "X" + c.o
			emit("X "+f, "all("+lit(f)+", "+packArgs([]packOpt{a, c})+")")
		}
	}
	xCases := since()

	// 7. 'unpack' at every position.
	//
	// A position is of the string, not of the format, and it can be
	// before the string starts, in the middle of it, or past its end.
	posFmts := []packOpt{
		packOne("i4", "1"),
		{o: "i4i4", v: []string{"1", "2"}},
		packOne("c3", `"xxx"`), packOne("s4", `"ab"`),
		packOne("z", `"ab"`), packOne("b", "1"), packOne("d", "1.5"),
		packOne("<i4", "1"), packOne(">i4", "1"),
		{o: "i1i2i4", v: []string{"1", "2", "3"}},
		{o: "Xi4i8", v: []string{"1", "2"}}, packOne("!8i8", "1"),
	}
	for _, p := range posFmts {
		for _, pos := range []string{"-3", "-2", "-1", "0", "1", "2",
			"3", "4", "5", "6", "9", "10", "1.5", `"5"`, "nil"} {
			emit("pos "+p.o+" "+pos,
				"upPos("+lit(p.o)+", "+pos+", "+packArgs([]packOpt{p})+")")
		}
	}
	posCases := since()

	// 8. Every option given the wrong thing.
	//
	// The three functions are C functions, so what they complain
	// about is an argument number and a type, and the formats that
	// are refused are refused as an argument too.
	wrongVals := []string{"nil", "true", `"1"`, "1.5", "{}", "print",
		"1/0", "0/0"}
	for _, o := range twoOpts {
		for _, v := range wrongVals {
			emit(o.o+" "+v, "pk("+lit(o.o)+", "+v+")")
		}
		emit(o.o+" nothing", "pk("+lit(o.o)+")")
	}
	for _, v := range []string{"1", "1.5", "true", "{}", "nil"} {
		emit("fmt "+v+" packsize", "ps("+v+")")
		emit("fmt "+v+" pack", "pk("+v+", 1)")
		emit("fmt "+v+" unpack", "up("+v+", "+packBuf+")")
	}
	for _, v := range []string{"1.5", `"x"`, "{}", "true"} {
		emit("pos "+v, "up("+`"i4"`+", "+packBuf+", "+v+")")
	}
	emit("no args pack", "pk()")
	emit("no args packsize", "ps()")
	emit("no args unpack", "up()")
	wrongCases := since()

	// 9. Round trips, which is what the format is for.
	rtInts := []string{"0", "1", "-1", "127", "-128", "255", "-256",
		"9007199254740992", "-9007199254740992"}
	for _, f := range []string{"<i1", "<i2", "<i4", "<i8", "<i16",
		">i1", ">i2", ">i4", ">i8", ">i16"} {
		for _, v := range rtInts {
			emit("rt "+f+" "+v, "rt("+lit(f)+", "+v+")")
		}
	}
	rtFloats := []string{"0.0", "-0.0", "1.0", "-1.0", "0.5", "(1/3)",
		"1e300", "1e-300"}
	for _, f := range []string{"<f", ">f", "<d", ">d", "<n", ">n"} {
		for _, v := range rtFloats {
			emit("rt "+f+" "+v, "rt("+lit(f)+", "+v+")")
		}
	}
	rtCases := since()

	// 10. The written-out formats, each saying something the nine
	//     groups above cannot put together on their own: an
	//     endianness in the middle, an 'X' with nothing after it,
	//     a size with no option after it, a size that is negative,
	//     three of the same thing in a row.
	misc := []packOpt{
		{o: "i4i4i4", v: []string{"1", "2", "3"}},
		{o: "<i4>i4", v: []string{"1", "2"}},
		{o: "=i4<i4>i4", v: []string{"1", "2", "3"}},
		{o: "i4!8i4", v: []string{"1", "2"}},
		{o: "!8i4i4", v: []string{"1", "2"}},
		{o: "i1!4i4", v: []string{"1", "2"}},
		packOne(" i4 ", "1"),
		{o: "i4 i4", v: []string{"1", "2"}},
		{o: "Xi4Xi8", v: []string{"1", "2"}},
		{o: "!8Xi8i8", v: []string{"1", "2"}},
		{o: "<!8i8i8", v: []string{"1", "2"}},
		{o: ">!4i4i4", v: []string{"1", "2"}},
		{o: "c3c3c3", v: []string{`"abc"`, `"def"`, `"ghi"`}},
		{o: "s4s4", v: []string{`"ab"`, `"cd"`}},
		{o: "zs4", v: []string{`"ab"`, `"cd"`}},
		{o: "s4z", v: []string{`"ab"`, `"cd"`}},
		{o: "bBhHlLjJ", v: []string{"1", "2", "3", "4", "5", "6", "7"}},
		{o: "i1I1i2I2", v: []string{"1", "2", "3", "4"}},
		{o: "!1i4i4i4i4", v: []string{"1", "2", "3", "4"}},
		{o: "!2i2i2", v: []string{"1", "2", "3"}},
		packOne("!16i1", "1"), packOne("!16i16", "1"),
		{o: "i16i16", v: []string{"1", "2"}},
		packOne("<i4", "1"), packOne(">i4<", "1"), packOne("=i4=", "1"),
		packOne("x", ""), {o: "xxx"}, {o: "X"},
		packOne("Xi4", "1"), packOne("Xi1", "1"), {o: "Xx"},
		packOne("Xz", `"ab"`), packOne("Xc3", `"abc"`),
		packOne("s", `"ab"`), packOne("z", `"ab"`), packOne("c0", `""`),
		packOne("i", "1"), packOne("I", "1"), packOne("s0", `"ab"`),
		packOne("s1", `"ab"`), packOne("s8", `"ab"`),
		packOne("s16", `"ab"`), packOne("i0", "1"),
		packOne("I0", "1"), {o: "!0"}, {o: "!17"},
		packOne("i17", "1"), packOne("I17", "1"),
		packOne("s17", `"ab"`), packOne("i-1", "1"),
		{o: "!-1"}, {o: ""},
	}
	for _, m := range misc {
		emit("size-of "+m.o, "ps("+lit(m.o)+")")
		emit("misc "+m.o, "all("+lit(m.o)+packSep(m.v)+")")
	}
	miscCases := since()

	// 11. A zero byte in a format, at every position of nine formats.
	//
	// This is the question the grid was built for and the only one it
	// answered.  C walks a format with 'while (*fmt != '\0')', so a
	// format ends at its first zero byte and not at the end of the
	// string: 'string.packsize("i4\0i4")' is 4, 'string.packsize(
	// "\0i4")' is 0, and a zero on its own is a format that is empty
	// rather than one that is refused.  A Go string is a length and
	// some bytes, so the port walked past the zero into the bytes
	// after it and found a format option that is the zero byte,
	// which is not one.
	zeroFmts := []packOpt{
		packOne("i4", "1"), {o: "i4i4", v: []string{"1", "2"}},
		packOne("z", `"ab"`), packOne("c3", `"xxx"`),
		packOne("Xi4", "1"), packOne("<i4", "1"),
		packOne("!8i8", "1"), packOne("s4", `"ab"`),
		packOne("d", "1.5"),
	}
	for _, o := range zeroFmts {
		for p := 0; p <= len(o.o); p++ {
			g := o.o[:p] + "\x00" + o.o[p:]
			tag := "zero in " + o.o + " at " + strconv.Itoa(p)
			emit("size-of "+tag, "ps("+lit(g)+")")
			emit(tag, "all("+lit(g)+packSep(o.v)+")")
		}
	}

	zeroCases := since()

	// 12. A zero byte in a value.
	//
	// 'z' walks the string it is given with 'strlen', so a zero in it
	// is where the string ends and the rest of it is nowhere: C
	// refuses it before a byte is written.  'c' does not look -- it
	// writes a fixed number of bytes and a zero is one of them --
	// and 's' writes a length, so it does not look either.
	zeroVals := []string{`""`, `"a"`, `"\0"`, `"a\0b"`, `"\0\0"`,
		`string.rep("\0", 3)`, `"ab\0cd"`}
	zeroNames := []string{"empty", "a", "one zero", "a zero b",
		"two zeros", "three zeros", "ab zero cd"}
	for _, l := range []string{"z", "c3", "c0", "c5", "s4"} {
		for i, v := range zeroVals {
			emit(l+" "+zeroNames[i], "pk("+lit(l)+", "+v+")")
		}
	}
	// and what 'z' reads out of data that has one in it
	zeroData := []string{`""`, `"ab\0cd"`, `"\0"`, `"ab\0"`, `"ab"`}
	for i, d := range zeroData {
		emit("unpack z from "+zeroNames[i], `up("z", `+d+`)`)
		emit("unpack z from "+zeroNames[i]+" at 2", `up("z", `+d+`, 2)`)
	}
	valueCases := since()

	// 13. Every alignment over every pair of options.
	//
	// What an option is aligned to is the smaller of its own size and
	// the maximum still standing, so an alignment only does anything
	// when there is something before the option it moves: '!8' over
	// 'i1i4' is 8, because the 'i4' does not start at the fifth byte
	// but at the eighth.  One option and an alignment is the one
	// shape in which that cannot be seen, which is why group 3 is
	// not enough on its own.
	alignTwo := []packOpt{
		packOne("i1", "1"), packOne("i2", "1"), packOne("i4", "1"),
		packOne("i8", "1"), packOne("d", "1.5"), packOne("b", "1"),
	}
	for _, sz := range []int{0, 1, 2, 3, 4, 8, 16} {
		for _, a := range alignTwo {
			for _, c := range alignTwo {
				f := "!" + strconv.Itoa(sz) + a.o + c.o
				emit(f, "all("+lit(f)+", "+packArgs([]packOpt{a, c})+")")
			}
		}
	}
	alignTwoCases := since()

	// 14. 'unpack' out of raw bytes.
	//
	// Everything above unpacks what it has just packed, and what
	// 'pack' makes is always a well-formed answer: the bytes it
	// writes above the eighth are the sign extension, so the check
	// that there is one -- 'unpackint' reading a size of more than
	// eight bytes -- never fails.  Reading a size of more than eight
	// bytes out of bytes that were not written by 'pack' is the only
	// way to ask whether the check is there: 'string.unpack("i9",
	// "\xff"x8 .. "\0"x8)' is refused with "9-byte integer does not
	// fit into Lua Integer", because the ninth byte is not 0xff and
	// the eight before it are a negative number.
	rawBufs := []string{
		`string.rep("\xff", 16)`,
		`string.rep("\0", 16)`,
		`string.rep("\xff", 8) .. string.rep("\0", 8)`,
		`string.rep("\0", 8) .. string.rep("\xff", 8)`,
		`string.rep("\x7f", 8) .. string.rep("\xff", 8)`,
		`string.rep("\x80", 8) .. string.rep("\0", 8)`,
	}
	for _, buf := range rawBufs {
		for _, l := range []string{"i", "I"} {
			for sz := 1; sz <= 16; sz++ {
				f := l + strconv.Itoa(sz)
				emit("raw "+f+" out of "+buf, `up("`+f+`", `+buf+`)`)
			}
		}
		for _, o := range []string{"b", "B", "h", "H", "l", "L", "j", "J",
			"T", "d", "f", "n", "s", "s1", "s4", "s8", "z", "c0", "c4",
			"c16", "x"} {
			emit("raw "+o+" out of "+buf, `up("`+o+`", `+buf+`)`)
		}
	}
	rawCases := since()

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	/* Each side gets a directory of its own so that the name of the
	   chunk is the same relative name in both. */
	dirs := map[string]string{
		"ref":  filepath.Join(t.TempDir(), "ref"),
		"ours": filepath.Join(t.TempDir(), "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "packfuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := fmtRunFile(t, ref, dirs["ref"], "packfuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dirs["ours"], "packfuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the pack chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("every byte of the alphabet as a format of its own: %d. "+
			"Every size of 'i', 'I', 's' and 'c' from 0 to 17, each with "+
			"the values at the edge of what it holds: %d. Every alignment "+
			"'!' can ask for, over 20 things to align: %d. Every "+
			"endianness over every option: %d. Every pair of options: %d. "+
			"An 'X' between every pair: %d. 'unpack' at 15 positions of 12 "+
			"formats: %d. Every option given 8 wrong things and given "+
			"nothing: %d. Round trips: %d. %d written-out formats: %d. A "+
			"zero byte at every position of 9 formats: %d. A zero byte in "+
			"a value: %d. Every alignment over every pair: %d. Every size "+
			"read out of 6 buffers of raw bytes: %d. %d cases, "+
			"%d questions, all of them PUC-Lua's",
			byteCases, sizeCases, alignCases, endCases, twoCases, xCases,
			posCases, wrongCases, rtCases, len(misc), miscCases, zeroCases,
			valueCases, alignTwoCases, rawCases, cases, asked)
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
		t.Errorf("case %d differs:\n  PUC-Lua: %s\n  this:    %s",
			i+1, a, c)
		if reported >= 1000000 {
			t.Errorf("... and %d more", (len(wl)-i-1)+(len(gl)-i-1))
			break
		}
	}
}

// packSep is the comma and the values, or nothing at all when there
// are none.  An 'x' or an 'X' takes no value, and 'string.pack' does
// not look at the arguments it does not need.
func packSep(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return ", " + strings.Join(v, ", ")
}

// TestPackInProcess is the three functions, driven in this process.
//
// The grid runs them in a subprocess, and a subprocess does not answer
// 'go test -cover' -- which is how 'str_pack', 'str_unpack',
// 'str_packsize' and the seven helpers behind them were found sitting
// at 0%, the last of 'lstrlib.go' that no test had ever entered.  What
// each of them answers is PUC-Lua's and the grid is what says so; this
// is only that they are entered at all, plus the two things the grid
// found, pinned where no reference interpreter is needed to see them.
func TestPackInProcess(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		-- 1. A format ends at its first zero byte, which is what the
		--    grid was built to ask and the first thing it answered.
		--    'C' walks a format with 'while (*fmt != '\0')'.
		assert(string.packsize("i4\0i4") == 4)
		assert(string.packsize("\0i4") == 0)
		assert(string.pack("i4\0i4", 1, 2) == string.pack("i4", 1))
		assert(select(2, string.unpack("i4\0i4", "\1\2\3\4")) == 5)
		-- 2. A length read out of eight bytes of 0xff is 2^64-1, and
		--    C compares it against what is left of the string as a
		--    'size_t': as a signed integer it is -1, which is less
		--    than whatever is left, and the slice behind the test
		--    went out of range.
		local ok, e = pcall(string.unpack, "s", string.rep("\255", 16))
		assert(not ok, tostring(e))
		assert(e == "bad argument #2 to 'string.unpack' "..
		  "(data string too short)", tostring(e))
		ok, e = pcall(string.unpack, "s8", string.rep("\255", 16))
		assert(not ok, tostring(e))
		-- 3. The answers themselves, so that a helper being entered
		--    is not the only thing this says.
		assert(string.packsize("i4") == 4)
		assert(string.pack("<i4", 1) == "\1\0\0\0")
		assert(string.pack(">i4", 1) == "\0\0\0\1")
		assert(string.pack("s4", "ab") == string.pack("i4", 2) .. "ab")
		assert(string.packsize("!8i1i4") == 8)
		assert(string.pack("i9", -1) == string.rep("\255", 9))
		assert((string.unpack("i9", string.rep("\255", 9))) == -1)
		assert(string.unpack("z", "ab\0cd") == "ab")
		assert(#string.pack("c5", "a") == 5)
		assert(string.unpack("c3", "abc") == "abc")
		-- 4. Every option, so that each of the seven helpers is
		--    entered: the two that read a size, the one that reads
		--    an alignment, and the four that move bytes.
		for _, o in ipairs({"b", "B", "h", "H", "l", "L", "j", "J",
		    "T", "f", "n", "d", "x", "z"}) do
		  pcall(string.packsize, o)
		  pcall(string.pack, o, 1)
		  pcall(string.unpack, o, "\1\2\3\4\5\6\7\8\9")
		end
		for _, o in ipairs({"i", "I", "s", "c"}) do
		  for n = 0, 17 do
		    pcall(string.packsize, o .. n)
		    pcall(string.pack, o .. n, 1)
		    pcall(string.unpack, o .. n, "\1\2\3\4\5\6\7\8")
		  end
		end
		for n = 0, 17 do
		  pcall(string.packsize, "!" .. n .. "i4")
		  pcall(string.pack, "!" .. n .. "Xi8", 1)
		  pcall(string.unpack, "!" .. n .. "i4", "\1\2\3\4\5\6\7\8")
		end
		for _, e in ipairs({"<", ">", "="}) do
		  for _, o in ipairs({"i1", "i2", "i4", "i8", "i16", "I4",
		      "d", "f", "n", "s4", "c3", "z"}) do
		    pcall(string.pack, e .. o, 1)
		    pcall(string.unpack, e .. o, "\1\2\3\4\5\6\7\8")
		  end
		end
		-- 5. The sizes above eight, which is where the sign
		--    extension and the fit check both live, read out of
		--    bytes that were not written by 'pack'.
		for _, b in ipairs({string.rep("\255", 16), string.rep("\0", 16),
		    string.rep("\255", 8) .. string.rep("\0", 8),
		    string.rep("\0", 8) .. string.rep("\255", 8)}) do
		  for n = 9, 16 do
		    pcall(string.unpack, "i" .. n, b)
		    pcall(string.unpack, "I" .. n, b)
		  end
		  pcall(string.unpack, "s", b)
		  pcall(string.unpack, "s8", b)
		  pcall(string.unpack, "z", b)
		end
		for n = 9, 16 do
		  pcall(string.pack, "i" .. n, -1)
		  pcall(string.unpack, "i" .. n, string.rep("\255", n))
		end
		return true
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("the pack engine does not answer what PUC-Lua answers")
	}
}
