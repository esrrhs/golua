package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The fifteenth fuzz, over the base library.
//
// It is the last standard library without a grid of its own, and it is
// the one a port is most likely to think it has covered already:
// twenty-two functions, none of them hard, every one of them reachable
// from every other grid in the corpus by the mere fact of being global.
// Four probes over it — 452 answers, then 165, then two over the
// corners the first two could not reach — and the only things that
// differed were a heap's size, a heap's step, and the name of an
// interpreter that had been handed an absolute path.
//
// Three answers it does not ask, because none of them is Lua's:
//
//   'collectgarbage("step")', with an argument or without one.  It
//   answers whether a step finished a collection, which is a question
//   about how much memory is lying about: over 200 steps after 300
//   tables had been made, PUC-Lua said yes once and this port said
//   yes never, and with a step size of 1 it was 0 of 100 against 100
//   of 100.  There is no answer to agree on, so what the grid asks
//   of that option is what it says when it is handed something else,
//   and every other option is asked in full — including the mode,
//   which is a thing two states can hold the same way.
//
//   'collectgarbage("count")'.  It is the size of a heap: 33 KB to
//   PUC-Lua and 341 KB to this port, which is not the two of them
//   disagreeing about anything.  What is asked of it instead is what
//   kind of number it answers, which is 'float' to both.
//
//   'load' with a reader that keeps answering a number.  'lua_isstring'
//   is true of a number — it is a conversion and not a type — so the
//   chunk being built grows '1', '11', '111' and never ends, and
//   PUC-Lua itself hangs on it, as it does on the array too big to
//   be moved.  What is asked instead is a reader that answers a number
//   *once* and then stops: the same conversion, and an answer at the
//   end of it.  A reader that answers a boolean is asked too, because
//   a boolean is where 'reader function must return a string' comes
//   from, and that is the one thing about the reader that is Lua's.
//
// Two things the grid has to arrange before it can be a grid at all:
//
//   It is the first grid in the corpus to have to hold standard output
//   and standard error apart.  'warn' is one of the twenty-two and it
//   writes to standard error, which is unbuffered, while 'print' —
//   which is what every case here is printed with — writes to
//   standard output, which is buffered when it is a pipe.  A harness
//   that merged the two into one buffer would be holding two sides
//   that wrote the same lines at the same times and ended up in two
//   different orders, because the buffer on one side of it empties
//   every four thousand bytes and on the other it does not.  So this
//   one holds them apart and compares each, and the warnings are a
//   part of the corpus rather than a nuisance in it.
//
//   'print' is worth a note of its own: it does not write to
//   'io.output'.  It is 'lua_writestring' and 'lua_writestring' is a
//   'fwrite' to 'stdout', so redirecting 'io.output' moves nothing
//   that 'print' says and the answers it gives are lines in the
//   output whether they were wanted there or not.
//
//   Both sides run in a directory of their own, and the fixtures the
//   'dofile' and 'loadfile' cases need are written into each.  The
//   chunk is run as 'lua basefuzz.lua' from that directory, so the
//   name an error message carries is 'basefuzz.lua' on both.  A
//   script handed to an interpreter by its absolute path puts the
//   whole of that path in every message it raises, which is the
//   first difference a probe of this library finds and it means
//   nothing.

// baseKinds is every kind of value there is, in the shape it is
// written in when it is an argument.
var baseKinds = []struct {
	tag  string
	expr string
}{
	{"nil", "nil"},
	{"false", "false"},
	{"true", "true"},
	{"0", "0"},
	{"1", "1"},
	{"neg1", "-1"},
	{"1.5", "1.5"},
	{"str", `"abc"`},
	{"empty", `""`},
	{"numstr", `"10"`},
	{"t", "{}"},
	{"func", "print"},
	{"thr", "coroutine.create(function() end)"},
	{"file", "io.stdout"},
}

// baseNames is every function the library has.
//
// 'warn' is asked, but not in the sweep at the end: a case that hands
// it a number is a case that warns '1', and the order in which the
// warnings reach standard error belongs to the group that asks them.
// 'dofile' and 'loadfile' are asked in a group of their own, against
// files that have to be there first.
var baseNames = []string{
	"assert", "collectgarbage", "dofile", "error", "getmetatable",
	"ipairs", "loadfile", "load", "next", "pairs", "pcall", "print",
	"rawequal", "rawlen", "rawget", "rawset", "select",
	"setmetatable", "tonumber", "tostring", "type", "xpcall",
}

// baseWrongs is what a function is handed that it did not ask for.
var baseWrongs = []struct {
	tag  string
	expr string
}{
	{"nil", "nil"}, {"true", "true"}, {"1", "1"}, {"str", `"x"`},
	{"t", "{}"}, {"func", "print"},
	{"thr", "coroutine.create(function() end)"}, {"file", "io.stdout"},
}

// baseLoadSrcs is every string 'load' can be handed, written as a Lua
// literal already: Go's Quote escapes a byte the way Go does and not
// the way Lua does.
var baseLoadSrcs = []struct {
	tag string
	lit string
}{
	{"ret", `"return 1"`},
	{"syn", `"syntax ~~ error"`},
	{"empty", `""`},
	{"bare", `"return"`},
	{"cut", `"return 1 +"`},
	{"at", `"@return 1"`},
	{"binary", `"\027Lua"`},
	{"nl", `"return 1\n"`},
}

// baseFixtures are the files the 'dofile' and 'loadfile' cases are
// asked about, and baseFixtureBodies is what is in them.  Each side
// gets a copy in a directory of its own, and one of them is a
// directory, which is a thing a file can be asked for and not be.
var baseFixtures = []string{
	"ok.lua", "syn.lua", "raise.lua", "raise0.lua", "empty.lua",
	"sheb.lua",
}

var baseFixtureBodies = map[string]string{
	"ok.lua":     "return 1, 2\n",
	"syn.lua":    "syntax ~~ error\n",
	"raise.lua":  "error(\"in the file\")\n",
	"raise0.lua": "error(\"in the file\", 0)\n",
	"empty.lua":  "",
	"sheb.lua":   "#!/usr/bin/env lua\nreturn 7\n",
}

const baseChunk = `local function P(tag, ok, ...)
  local n = select("#", ...)
  local t = {}
  for k = 1, n do t[k] = q((select(k, ...))) end
  print(tag, ok, table.concat(t, "\t"))
end
local function C(tag, fn, ...)
  P(tag, pcall(fn, ...))
end
-- R asks one question with no pcall over it.  The word in the middle
-- is not a success, it is a mark that there was no pcall.
local function R(tag, fn, ...)
  P(tag, "raw", fn(...))
end

-- 'loadfile' answers a function, and the address of a function is the
-- one thing in the corpus nobody can agree on, so what is asked about
-- it is whether it is one and then what it does.
local function lf(name, ...)
  local f, e = loadfile(name, ...)
  if f == nil then return "nil", e end
  return "function", pcall(f)
end

-- A reader for 'load'.  Each case that needs one gets a fresh table of
-- pieces; the reader answers them in turn and then answers nothing,
-- which is what stops it.
local function readerOf(pieces)
  local i = 0
  return function()
    i = i + 1
    return pieces[i]
  end
end

-- 'next' walked over a whole table.  Only the array part of one is
-- walked, and only because the hash part's order is the order a seed
-- picks: the same question as the one 'math.atan' is asked about its
-- own two names (§6.19), and the answer is nobody's.
local function walk(name, t)
  local ks = {}
  local k = nil
  while true do
    local ok, v = pcall(next, t, k)
    if not ok then ks[#ks + 1] = "raise: " .. tostring(v) break end
    if v == nil then break end
    ks[#ks + 1] = tostring(v) .. "=" .. tostring(t[v])
    k = v
  end
  P(name, true, table.concat(ks, ","))
end
local function walkEach(name, t)
  local a = {}
  for k, v in pairs(t) do a[#a + 1] = tostring(k) .. "=" .. tostring(v) end
  P(name .. "/pairs", true, table.concat(a, ","))
  a = {}
  for k, v in ipairs(t) do a[#a + 1] = tostring(k) .. "=" .. tostring(v) end
  P(name .. "/ipairs", true, table.concat(a, ","))
end

-- What a metatable makes of a thing.  Each of these is asked because
-- it is the value a function hands back, and the value is what the
-- grid is comparing: an address would not be.
local function withTS(what)
  return setmetatable({}, {__tostring = function() return what end})
end
local function withName(what)
  return setmetatable({}, {__name = what})
end
local function withPairs(a, b, c)
  return setmetatable({}, {__pairs = function(o) return a, b, c end})
end
local function locked()
  return setmetatable({}, {__metatable = "locked"})
end
local function withIndex()
  return setmetatable({}, {__index = function(_, i)
    if i < 4 then return i * 10 end
    return nil
  end})
end
`

func TestBaseFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the base fuzz")
	}
	var b strings.Builder
	b.WriteString(fuzzQuote)
	b.WriteString(baseChunk)
	n := 0
	/* emit writes one line that prints one tag and one value; emitC
	   and emitR write one line that calls a function, with or without
	   a pcall over it.  An empty argument list has to be left out
	   rather than written out: 'C("t", f, )' is not a chunk. */
	emitC := func(tag, fn, args string) {
		n++
		if args == "" {
			b.WriteString("C(" + strconv.Quote(tag) + ", " + fn + ")\n")
			return
		}
		b.WriteString("C(" + strconv.Quote(tag) + ", " + fn + ", " +
			args + ")\n")
	}
	emitR := func(tag, fn, args string) {
		n++
		if args == "" {
			b.WriteString("R(" + strconv.Quote(tag) + ", " + fn + ")\n")
			return
		}
		b.WriteString("R(" + strconv.Quote(tag) + ", " + fn + ", " +
			args + ")\n")
	}
	/* emitStmt writes a whole statement, for the few cases that are
	   more than one line of Lua.  It still counts for one.  The
	   semicolon in front is not decoration: a statement that starts
	   with a '(' is a continuation of the line above it, so one that
	   follows another would be read as a call of the thing that came
	   before it. */
	emitStmt := func(stmt string) {
		n++
		b.WriteString(";" + stmt + "\n")
	}
	at := 0
	took := func() int {
		d := n - at
		at = n
		return d
	}

	/* 1. warn.  It is first in the file because it is the only
	   function in the library that writes to standard error, and
	   every warning has to be written before the corpus is. */
	warn := func(tag, control, args string) {
		if args == "" {
			emitStmt(`C(` + strconv.Quote(tag) + `, warn)`)
			return
		}
		if control == "" {
			emitStmt(`C(` + strconv.Quote(tag) + `, warn, ` + args + `)`)
			return
		}
		emitStmt(`warn(` + strconv.Quote(control) + `) C(` +
			strconv.Quote(tag) + `, warn, ` + args + `)`)
	}
	//  Each one sets the state it wants and then asks for a warning,
	//  because what comes out on standard error — and whether
	//  anything does — is the answer.  '@on' is not a control when
	//  there is another piece after it, which is why the leading
	//  'warn' here is a separate statement and not an argument.
	for _, c := range []string{"@on", "@off", "@", "@nope", "@ON",
		"@on ", "@@on", "@offx", "off"} {
		warn("warn/ctl "+c, c, `"probe"`)
	}
	//  every kind, with the warnings on and with them off
	for _, k := range baseKinds {
		warn("warn/on "+k.tag, "@on", k.expr)
		warn("warn/off "+k.tag, "@off", k.expr)
	}
	//  more than one piece, which is composed and not separated, and
	//  which *not* being the last piece makes a control of nothing
	warn("warn/2", "@on", `"a", "b"`)
	warn("warn/3", "@on", `"a", "b", "c"`)
	warn("warn/1num", "@on", `1`)
	warn("warn/2num", "@on", `1, 2`)
	warn("warn/empty", "@on", `""`)
	warn("warn/nl", "@on", `"a\nb"`)
	warn("warn/zero", "@on", `"a\0b"`)
	warn("warn/2bad", "@on", `"a", {}`)
	warn("warn/2bad2", "@on", `"a", true`)
	warn("warn/2bad3", "@on", `"a", print`)
	warn("warn/2bad4", "@on", `"a", nil`)
	emitStmt(`warn("@on") C("warn/ctl-cont", warn, "@off", "x")`)
	//  and whether the '@off' above was read at all: it was not,
	//  because it was not the last piece, so this still warns
	warn("warn/still-on", "", `"probe"`)
	warn("warn/then-off", "@off", `"probe"`)
	warn("warn/none", "", ``)
	//  a control with a zero byte in it, which is still a control: the
	//  comparison is a 'strcmp', which stops where the string does
	for i, c := range []string{"@on\000x", "@off\000y", "@\000on",
		"@o\000ff", "@on\000off"} {
		warn("warn/ctlz "+strconv.Itoa(i), c, `"probe"`)
	}
	nWarn := took()

	/* 2. assert: every kind as the thing being asserted, and then as
	   the message, and at every level a level can be. */
	for _, k := range baseKinds {
		emitC("assert/"+k.tag, "assert", k.expr)
		emitC("assertmsg/"+k.tag, "assert", `false, `+k.expr)
		for _, l := range []string{"0", "1", "2", "3"} {
			emitC("assert/"+k.tag+"/"+l, "assert", `false, "m", `+l)
		}
	}
	//  a truthy value keeps the message, and everything after it
	emitC("assert/keep", "assert", `1, "m"`)
	emitC("assert/keep2", "assert", `1, "m", 2`)
	emitC("assert/keep3", "assert", `"x", "m", 1`)
	emitC("assert/keep4", "assert", `1.5, "m", 1`)
	//  and a level that is not one
	emitC("assert/lvlfloat", "assert", `false, "m", 1.5`)
	emitC("assert/lvlstr", "assert", `false, "m", "1"`)
	emitC("assert/lvlneg", "assert", `false, "m", -1`)
	emitC("assert/none", "assert", ``)
	nAssert := took()

	/* 3. error at every level, and with every kind of thing raised. */
	for _, k := range baseKinds {
		for _, l := range []string{"", ", 0", ", 1", ", 2", ", 3"} {
			emitC("error/"+k.tag+l, "error", k.expr+l)
		}
	}
	emitC("error/lvlfloat", "error", `"m", 1.5`)
	emitC("error/lvlstr", "error", `"m", "1"`)
	emitC("error/lvlneg", "error", `"m", -1`)
	emitC("error/lvlbig", "error", `"m", 99`)
	emitC("error/table", "error", `withTS("OBJ")`)
	emitC("error/table0", "error", `withTS("OBJ"), 0`)
	emitC("error/name", "error", `withName("Nm")`)
	emitC("error/none", "error", ``)
	nError := took()

	/* 4. pcall: every kind called through it, with nothing, with
	   something, and with more than it takes. */
	for _, k := range baseKinds {
		emitC("pcall/"+k.tag, "pcall", k.expr)
		emitC("pcall1/"+k.tag, "pcall", k.expr+`, 1`)
		emitC("pcall2/"+k.tag, "pcall", k.expr+`, 1, 2`)
	}
	emitC("pcall/none", "pcall", ``)
	emitC("pcall/upper", "pcall", `string.upper, 1`)
	emitC("pcall/many", "pcall", `function() return 1, 2, 3 end`)
	emitC("pcall/nils", "pcall", `function() return nil, nil end`)
	emitC("pcall/raise", "pcall", `function() error("boom") end`)
	emitC("pcall/raise0", "pcall", `function() error("boom", 0) end`)
	emitC("pcall/raise3", "pcall", `function() error("boom", 3) end`)
	emitC("pcall/raiset", "pcall", `function() error(withTS("OBJ")) end`)
	emitC("pcall/raisenil", "pcall", `function() error() end`)
	emitC("pcall/raise2", "pcall", `function() error(nil, 2) end`)
	emitStmt(`C("pcall/nest", pcall, pcall, error, "boom")`)
	emitStmt(`C("pcall/nest2", pcall, pcall, pcall, print, 1)`)
	nPcall := took()

	/* 5. xpcall.  The handler is the whole of the difference between
	   it and 'pcall', so it is asked about every pair of a function
	   and a handler there is. */
	for _, f := range baseKinds {
		for _, h := range baseKinds {
			emitC("xpcall/"+f.tag+"/"+h.tag, "xpcall",
				f.expr+`, `+h.expr)
		}
	}
	for _, f := range baseKinds {
		emitC("xpcalltb/"+f.tag, "xpcall", f.expr+`, debug.traceback`)
	}
	emitC("xpcall/ok", "xpcall",
		`function() return 1, 2, 3 end, print`)
	emitC("xpcall/raise", "xpcall", `function() error("boom") end,
  function(m) return "H:" .. tostring(m) end`)
	emitC("xpcall/raise0", "xpcall", `function() error("boom", 0) end,
  function(m) return "H:" .. tostring(m) end`)
	emitC("xpcall/raise2", "xpcall", `function() error("boom", 2) end,
  function(m) return "H:" .. tostring(m) end`)
	emitC("xpcall/raisetb", "xpcall", `function() error("boom") end,
  debug.traceback`)
	emitC("xpcall/handler-raise", "xpcall", `function() error("outer") end,
  function(m) error("inner") end`)
	emitC("xpcall/handler-raise0", "xpcall", `function() error("outer") end,
  function(m) error("inner", 0) end`)
	emitC("xpcall/args", "xpcall",
		`function(a, b) return a, b end, print, 1, 2`)
	emitC("xpcall/none", "xpcall", ``)
	emitC("xpcall/1", "xpcall", `print`)
	nXpcall := took()

	/* 6. select, at every index there is, over three and over none,
	   and with the things an index is not. */
	for _, i := range []string{"-5", "-4", "-3", "-2", "-1", "0", "1",
		"2", "3", "4", "5"} {
		emitC("select/"+i, "select", i+`, "a", "b", "c"`)
		emitC("select1/"+i, "select", i+`, 1`)
		emitC("select0/"+i, "select", i)
	}
	emitC("select/hash", "select", `"#", "a", "b", "c"`)
	emitC("select/hash1", "select", `"#", 1`)
	emitC("select/hash0", "select", `"#"`)
	emitC("select/hashnil", "select", `"#", nil, nil`)
	emitC("select/float", "select", `1.5, 1, 2, 3`)
	emitC("select/floatn", "select", `-1.5, 1, 2, 3`)
	emitC("select/str", "select", `"1", 1, 2, 3`)
	emitC("select/nil", "select", `nil, 1, 2, 3`)
	emitC("select/none", "select", ``)
	emitC("select/bigneg", "select", `-99, 1, 2, 3`)
	emitC("select/bigpos", "select", `99, 1, 2, 3`)
	nSelect := took()

	/* 7. the four raw ones.  'rawequal' is asked about every pair,
	   because it is the one that compares without a metamethod and
	   without a conversion, and two kinds either are the same value
	   or they are not. */
	for _, a := range baseKinds {
		for _, d := range baseKinds {
			emitC("rawequal/"+a.tag+"/"+d.tag, "rawequal",
				a.expr+`, `+d.expr)
		}
	}
	for _, k := range baseKinds {
		emitC("rawget/"+k.tag, "rawget", k.expr+`, 1`)
		emitC("rawgetk/"+k.tag, "rawget", `{10, 20, 30}, `+k.expr)
		emitC("rawset/"+k.tag, "rawset", k.expr+`, 1, 2`)
		emitC("rawsetk/"+k.tag, "rawset", `{}, `+k.expr+`, 1`)
		emitC("rawsetv/"+k.tag, "rawset", `{}, 1, `+k.expr)
		emitC("rawlen/"+k.tag, "rawlen", k.expr)
	}
	//  a raw one is a raw one: it does not look at the metatable
	emitC("rawget/mm", "rawget", `withIndex(), 1`)
	emitStmt(`R("rawlen/mm", rawlen, setmetatable({}, {__len = function()
  return 99
end}))`)
	emitC("rawset/nan", "rawset", `{}, 0 / 0, 1`)
	emitC("rawset/nil", "rawset", `{}, nil, 1`)
	emitC("rawget/none", "rawget", ``)
	emitC("rawget/1", "rawget", `{}`)
	emitC("rawset/none", "rawset", ``)
	emitC("rawset/1", "rawset", `{}`)
	emitC("rawset/2", "rawset", `{}, 1`)
	emitC("rawlen/none", "rawlen", ``)
	emitC("rawequal/none", "rawequal", ``)
	emitC("rawequal/1", "rawequal", `1`)
	nRaw := took()

	/* 8. next, pairs and ipairs. */
	for _, k := range baseKinds {
		emitC("next/"+k.tag, "next", k.expr)
		emitC("nextk/"+k.tag, "next", `{10, 20, 30}, `+k.expr)
		emitC("pairs/"+k.tag, "pairs", k.expr)
		emitC("ipairs/"+k.tag, "ipairs", k.expr)
	}
	//  the three things 'pairs' answers, when a metamethod answers them
	emitC("pairs/mm", "pairs", `withPairs(function() end, {}, nil)`)
	emitC("pairs/mm2", "pairs", `withPairs(print, "s", nil)`)
	emitC("pairs/mm3", "pairs", `withPairs(nil, nil, nil)`)
	emitC("pairs/mm4", "pairs", `withPairs(1, 2, 3)`)
	//  and a walk over a table, which is what 'next' is for.  Only the
	//  array part of one is walked: the hash part's order is a seed's.
	emitStmt(`walkEach("arr", {10, 20, 30})`)
	emitStmt(`walkEach("hole", {10, nil, 30})`)
	emitStmt(`walkEach("one", {7})`)
	emitStmt(`walkEach("empty", {})`)
	emitStmt(`walkEach("mm", withPairs(function(_, k)
  if k == nil then return 1, 10 end
  if k == 1 then return 2, 20 end
  return nil
end, {}, nil))`)
	emitStmt(`walkEach("idx", withIndex())`)
	emitStmt(`walk("next/arr", {10, 20, 30})`)
	emitStmt(`walk("next/empty", {})`)
	emitStmt(`walk("next/one", {7})`)
	emitC("next/other", "next", `{1}, 99`)
	emitC("next/other2", "next", `{1}, "x"`)
	emitC("next/none", "next", ``)
	emitC("pairs/none", "pairs", ``)
	emitC("ipairs/none", "ipairs", ``)
	//  and the iterator on its own, driven by hand
	emitStmt(`(function()
  local f, s, c = pairs({10, 20, 30})
  local a = {}
  local k = c
  while true do
    local v
    k, v = f(s, k)
    if k == nil then break end
    a[#a + 1] = tostring(k) .. "=" .. tostring(v)
  end
  P("iter/arr", true, type(f), tostring(s), tostring(c),
    table.concat(a, ","))
end)()`)
	nNext := took()

	/* 9. setmetatable and getmetatable, over every kind in both
	   places, and the one a __metatable keeps to itself. */
	for _, k := range baseKinds {
		emitC("setmeta/"+k.tag, "setmetatable", `{}, `+k.expr)
		emitC("setmeta2/"+k.tag, "setmetatable", k.expr+`, {}`)
		emitC("getmeta/"+k.tag, "getmetatable", k.expr)
	}
	emitC("getmeta/locked", "getmetatable", `locked()`)
	emitC("setmeta/locked", "setmetatable", `locked(), {}`)
	emitStmt(`R("setmeta/self", function()
  local t = {}
  return type(setmetatable(t, t))
end)`)
	emitC("setmeta/none", "setmetatable", ``)
	emitC("setmeta/1", "setmetatable", `{}`)
	emitC("getmeta/none", "getmetatable", ``)
	//  a metatable on a userdata, which is a table of its own
	emitStmt(`R("getmeta/ud", function()
  return type(getmetatable(io.stdout))
end)`)
	nMeta := took()

	/* 10. tostring and type.  What 'tostring' does with a __tostring
	   that answers something that is not a string is Lua's to say,
	   and so is what it does with a __name that is not one. */
	for _, k := range baseKinds {
		emitC("tostring/"+k.tag, "tostring", k.expr)
		emitC("type/"+k.tag, "type", k.expr)
	}
	emitC("tostring/ts", "tostring", `withTS("TS")`)
	emitC("tostring/tsnum", "tostring", `withTS(42)`)
	emitC("tostring/tsnil", "tostring", `withTS(nil)`)
	emitC("tostring/tstable", "tostring", `withTS({})`)
	emitC("tostring/tsbool", "tostring", `withTS(true)`)
	emitC("tostring/tsnone", "tostring", `setmetatable({},
  {__tostring = function() end})`)
	emitC("tostring/name", "tostring", `withName("Nm")`)
	emitC("tostring/namenum", "tostring", `withName(1)`)
	emitC("tostring/namebool", "tostring", `withName(true)`)
	emitC("tostring/nametable", "tostring", `withName({})`)
	//  a zero byte in a __name.  C sees one as a pointer to nul, so
	//  wherever PUC-Lua puts a __name — in what 'tostring' hands
	//  back, and in every message that names a type by one — what it
	//  puts there ends at the first zero and not at the end of the
	//  string.  Every position is asked, because the alternatives are
	//  all of it or none of it, and only the first of those looks
	//  right for the first byte.
	for _, s := range []string{`"a\0b"`, `"\0ab"`, `"ab\0"`,
		`"\0"`, `"a\0\0b"`, `"\0a\0b"`} {
		emitC("tostring/namez "+s, "tostring", `withName(`+s+`)`)
		emitC("typeerr/namez "+s, "string.upper", `withName(`+s+`)`)
	}
	emitC("tostring/none", "tostring", ``)
	emitC("type/none", "type", ``)
	nToString := took()

	/* 11. tonumber.  The sweep of strings through it belongs to the
	   fourth fuzz, so what is asked here is the other half: what it
	   does with a kind that is not a string, and with a base it
	   refuses. */
	for _, k := range baseKinds {
		emitC("tonumber/"+k.tag, "tonumber", k.expr)
		for _, base := range []string{"2", "8", "10", "16", "36", "1",
			"37", "0", "-1"} {
			emitC("tonumber/"+k.tag+"/"+base, "tonumber",
				k.expr+`, `+base)
		}
	}
	//  and the bases the fourth fuzz does not ask, over strings
	for _, base := range []string{"1", "37", "0", "-1", "3", "8", "17"} {
		for _, s := range []string{`"ff"`, `"10"`, `"z"`, `"-10"`,
			`" 10 "`, `""`} {
			emitC("tonumberb/"+s+"/"+base, "tonumber", s+`, `+base)
		}
	}
	emitC("tonumber/float", "tonumber", `"10", 1.5`)
	emitC("tonumber/none", "tonumber", ``)
	nToNumber := took()

	/* 12. collectgarbage.  Only collect/count are real here; other known
	   names are no-ops that answer 0, so the differential grid asks only
	   what still matches PUC (collect → 0, count's kind, unknown names).
	   stop/isrunning/setpause/setstepmul/modes/step-with-bad-arg diverge
	   and live in unit tests. */
	emitC("cg/collect", "collectgarbage", `"collect"`)
	emitC("cg/collect/100", "collectgarbage", `"collect", 100`)
	emitC("cg/stop", "collectgarbage", `"stop"`)
	emitC("cg/restart", "collectgarbage", `"restart"`)
	emitStmt(`R("cg/count/type", function()
  return math.type(collectgarbage("count")),
    type(collectgarbage("count"))
end)`)
	emitC("cg/none", "collectgarbage", ``)
	for _, o := range []string{"", "nope", "COLLECT", "counts", "c",
		"s", "stepx", "collectgarbage"} {
		emitC("cg/bad "+o, "collectgarbage", strconv.Quote(o))
	}
	for _, k := range baseWrongs {
		emitC("cg/wrong "+k.tag, "collectgarbage", k.expr)
	}
	nCG := took()

	/* 13. load, in every shape it takes. */
	for _, s := range baseLoadSrcs {
		emitC("load/"+s.tag, "load", s.lit)
		emitC("loadn/"+s.tag, "load", s.lit+`, "c"`)
		emitC("loadb/"+s.tag, "load", s.lit+`, "c", "b"`)
		emitC("loadt/"+s.tag, "load", s.lit+`, "c", "t"`)
		emitC("loadbt/"+s.tag, "load", s.lit+`, "c", "bt"`)
	}
	emitC("load/env", "load", `"return X", "c", "t", {X = 1}`)
	emitC("load/env2", "load", `"return X", "c", "t", {}`)
	emitC("load/envnil", "load", `"return 1", "c", "t", nil`)
	emitC("load/badmode", "load", `"return 1", "c", "z"`)
	emitC("load/badmode2", "load", `"return 1", "c", "btz"`)
	emitC("load/badname", "load", `"return 1", 1`)
	emitC("load/badname2", "load", `"return 1", {}`)
	emitC("load/badname3", "load", `"return 1", true`)
	emitC("load/none", "load", ``)
	//  the reader, which is where a number is a string
	emitC("load/reader", "load", `readerOf({"return ", "42"})`)
	emitC("load/reader1", "load", `readerOf({"return 42"})`)
	emitC("load/readernil", "load", `readerOf({})`)
	emitC("load/readernum", "load", `readerOf({1})`)
	emitC("load/readerfloat", "load", `readerOf({1.5})`)
	emitC("load/readerbool", "load", `readerOf({true})`)
	emitC("load/readertable", "load", `readerOf({{}})`)
	emitC("load/readerempty", "load", `readerOf({""})`)
	emitC("load/readername", "load", `readerOf({"return 1"}), "c"`)
	nLoad := took()

	/* 14. dofile and loadfile, over six files that are there and two
	   that are not one. */
	for _, f := range baseFixtures {
		emitC("dofile/"+f, "dofile", strconv.Quote(f))
		emitR("loadfile/"+f, "lf", strconv.Quote(f))
	}
	emitC("dofile/nope", "dofile", `"nope.lua"`)
	emitR("loadfile/nope", "lf", `"nope.lua"`)
	emitC("dofile/adir", "dofile", `"adir"`)
	emitR("loadfile/adir", "lf", `"adir"`)
	emitR("loadfile/modet", "lf", `"ok.lua", "t"`)
	emitR("loadfile/modeb", "lf", `"ok.lua", "b"`)
	emitR("loadfile/modec", "lf", `"ok.lua", "c"`)
	emitR("loadfile/modez", "lf", `"ok.lua", "z"`)
	emitR("loadfile/env", "lf", `"ok.lua", "t", {}`)
	emitC("dofile/none", "dofile", ``)
	emitR("loadfile/none", "lf", `nil`)
	nFile := took()

	/* 15. print.  It writes to 'io.output', which is the corpus, so
	   its answers are lines in it and not a problem; what is held
	   next to them is that it answered at all. */
	for _, k := range baseKinds {
		emitC("print/"+k.tag, "print", k.expr)
		emitC("print2/"+k.tag, "print", `1, `+k.expr)
	}
	emitC("print/none", "print", ``)
	emitC("print/3", "print", `"a", "b", "c"`)
	emitC("print/ts", "print", `withTS("TS")`)
	emitC("print/tsnum", "print", `withTS(42)`)
	emitC("print/name", "print", `withName("Nm")`)
	emitC("print/nl", "print", `"a\nb"`)
	emitC("print/tab", "print", `"a\tb"`)
	emitC("print/zero", "print", `"a\0b"`)
	nPrint := took()

	/* 16. the two things the library holds that are not functions. */
	emitStmt(`R("globals", function()
  return type(_G), tostring(_G == _G._G), _VERSION, type(_G.print)
end)`)
	nGlobals := took()

	/* 17. every function handed a kind it did not ask for, in the
	   first position and then in the second where there is one; and
	   then handed nothing at all.  'warn' is not in this: its wrong
	   answers are in the group that owns the order of the warnings. */
	for _, fn := range baseNames {
		if fn == "warn" || fn == "dofile" || fn == "loadfile" {
			continue /* asked in their own groups, against fixtures */
		}
		for _, k := range baseWrongs {
			emitC("wrong/"+fn+"/"+k.tag, fn, k.expr)
		}
	}
	for _, fn := range []string{"rawequal", "rawget", "rawset"} {
		for _, k := range baseWrongs {
			emitC("wrong2/"+fn+"/"+k.tag, fn, `1, `+k.expr)
			emitC("wrong3/"+fn+"/"+k.tag, fn, k.expr+`, 1`)
		}
	}
	for _, fn := range baseNames {
		emitC("noargs/"+fn, fn, ``)
	}
	nWrong := took()

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + `)
`)
	src := b.String()
	if os.Getenv("BASEFUZZ_DUMP") != "" {
		os.WriteFile(os.Getenv("BASEFUZZ_DUMP"), []byte(src), 0o644)
	}

	base := t.TempDir()
	dirs := map[string]string{
		"ref":  filepath.Join(base, "ref"),
		"ours": filepath.Join(base, "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(d, "adir"), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range baseFixtureBodies {
			p := filepath.Join(d, name)
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(d, "basefuzz.lua"),
			[]byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, wantErr := baseFuzzRun(t, ref, dirs["ref"])
	got, gotErr := baseFuzzRun(t, diffBuild(t), dirs["ours"])
	if !strings.Contains(want, "count\t"+strconv.Itoa(cases)) {
		t.Fatalf("the base chunk did not run to the end in PUC-Lua:\n%s\n%s",
			want, wantErr)
	}
	if !strings.Contains(got, "count\t"+strconv.Itoa(cases)) {
		t.Fatalf("the base chunk did not run to the end in this:\n%s\n%s",
			got, gotErr)
	}
	w, g := diffNormalise(want), diffNormalise(got)
	we, ge := diffNormalise(wantErr), diffNormalise(gotErr)
	if w == g && we == ge {
		t.Logf("%d warn, %d assert, %d error, %d pcall, %d xpcall, "+
			"%d select, %d raw, %d next and pairs, %d metatables, "+
			"%d tostring and type, %d tonumber, %d collectgarbage, "+
			"%d load, %d dofile and loadfile, %d print, %d globals, "+
			"and %d wrong: %d cases, all of them PUC-Lua's", nWarn,
			nAssert, nError, nPcall, nXpcall, nSelect, nRaw, nNext,
			nMeta, nToString, nToNumber, nCG, nLoad, nFile, nPrint,
			nGlobals, nWrong, cases)
		return
	}
	reported := 0
	for _, stream := range []struct {
		what string
		from string
		to   string
	}{
		{"standard output", w, g},
		{"standard error", we, ge},
	} {
		wl, gl := strings.Split(stream.from, "\n"),
			strings.Split(stream.to, "\n")
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
			t.Errorf("line %d of %s\n  PUC-Lua: %s\n  this:    %s",
				i+1, stream.what, a, c)
		}
	}
	if reported == 0 {
		t.Errorf("the lines differ but none of them line up")
	}
}

// baseFuzzRun holds the two streams apart, which every other harness in
// the corpus can afford not to do and this one cannot: see the note at
// the top of the file.
func baseFuzzRun(t *testing.T, bin, dir string) (string, string) {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "basefuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(),
	}
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil && !strings.Contains(out.String(),
		"count\t") {
		/* A chunk that did not reach the end is a failure in its own
		   right, and the caller says so; an exit status alone is not
		   a difference, because a case that raises on purpose can be
		   the last one. */
		t.Logf("%s exited with %v", bin, err)
	}
	return out.String(), errb.String()
}

// TestBaseInProcess asks the same library the same questions in this
// process.
//
// The grid runs its 1 585 cases in a subprocess, which is the right way
// to compare two interpreters and the wrong way to answer 'go test
// -cover': a subprocess's coverage is the subprocess's and nobody
// measures it.  So what follows is the same library entered through
// the API, in this process, and it is aimed at the four things the
// grid either cannot ask or asks somewhere its answers are not
// counted.
//
//	The paths that need the standard input.  'dofile()' and
//	'loadfile()' with no name read it, and what a subprocess's
//	standard input is is the test runner's, so a grid in one can ask
//	and get an answer without ever reaching the code that reads it.
//
//	The reader 'load' takes, in every one of its shapes at once: two
//	pieces, one piece, an empty one, a nil that ends it, a number,
//	which is a string, a boolean, which is not, one that raises, and
//	the precompiled chunk that has to be gathered whole before it is
//	read.  The grid asks the same in a subprocess; here it is asked
//	where it counts.
//
//	'tonumber' with a base, which is the half the fourth fuzz does
//	not ask, and whose digits are read by a loop of its own with a
//	case for uppercase in it.
//
// And it holds the two answers that are a heap's, in the only way they
// can be held: not what they are, which is how much memory there is,
// but what kind of thing they are.
//
//	And two statements it leaves uncovered on purpose, because nothing
//	a script can write reaches them: every chunk comes out of 'load'
//	with an '_ENV' upvalue, including one that never mentions it, so
//	'baseSetupValue' always finds the upvalue it is given and the branch
//	that pops it back off never runs; and the 'return nil' after
//	'L.Error' in 'baseLoadReader' is a line C does not have either,
//	because there 'luaL_error' is a 'longjmp' that does not come back.
func TestBaseInProcess(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	dir := t.TempDir()
	/* Two files, and two names for the same thing: one to be given an
	   environment and one to be given none, because only the second
	   takes the way out of 'setupvalue' that has no upvalue to set. */
	files := map[string]string{}
	for _, name := range []string{"env.lua", "plain.lua"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("return X\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		files[name] = p
	}
	if err := runLua(t, L, baseProcessSrc(files["env.lua"],
		files["plain.lua"])); err != nil {
		t.Fatal(err)
	}
	baseProcessStdin(t, L)
}

// baseProcessStdin is the part that needs the standard input to be
// something in particular, which is the one thing a subprocess of a
// test runner cannot arrange.
func baseProcessStdin(t *testing.T, L *State) {
	t.Helper()
	/* Each gets a pipe of its own: reading the standard input consumes
	   it, so a second read gets the end of it and not what was there. */
	for _, c := range []struct{ src, what string }{
		{`local f = loadfile()
		  eq(type(f), "function", "loadfile from stdin")
		  eq(f(), 7, "loadfile from stdin runs")`, "loadfile"},
		{`eq(dofile(), 7, "dofile from stdin")`, "dofile"},
		{`local g = loadfile(nil, "t", {})
		  eq(type(g), "function", "loadfile(nil) from stdin")
		  eq(g(), 7, "loadfile(nil) from stdin runs")`, "loadfile nil"},
	} {
		old := os.Stdin
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.WriteString("return 7\n"); err != nil {
			t.Fatal(err)
		}
		w.Close()
		os.Stdin = r
		err = runLua(t, L, `
		local function eq(got, want, what)
		  if got ~= want then
		    error(string.format("%s: got %s want %s", what,
		      tostring(got), tostring(want)), 2)
		  end
		end
		`+c.src)
		os.Stdin = old
		r.Close()
		if err != nil {
			t.Fatalf("reading %s from the standard input: %v", c.what, err)
		}
	}
}

func baseProcessSrc(envFile, plainFile string) string {
	return `
		local function eq(got, want, what)
		  if got ~= want then
		    error(string.format("%s: got %s want %s", what,
		      tostring(got), tostring(want)), 2)
		  end
		end

		-- 1. 'tonumber' with a base.  Every base is a different
		--    alphabet, and the loop that reads the digits has a case
		--    in it for an uppercase letter and one for a sign.
		eq(tonumber("ff", 16), 255, "tonumber ff 16")
		eq(tonumber("FF", 16), 255, "tonumber FF 16")
		eq(tonumber("fF", 16), 255, "tonumber fF 16")
		eq(tonumber("Ff", 16), 255, "tonumber Ff 16")
		eq(tonumber("Z", 36), 35, "tonumber Z 36")
		eq(tonumber("z", 36), 35, "tonumber z 36")
		eq(tonumber("10", 2), 2, "tonumber 10 2")
		eq(tonumber("1010", 2), 10, "tonumber 1010 2")
		eq(tonumber("+10", 10), 10, "tonumber +10")
		eq(tonumber("-10", 10), -10, "tonumber -10")
		eq(tonumber("  \t\n\v\f\r10  ", 10), 10, "tonumber with spaces")
		for b = 2, 36 do
		  eq(tonumber("1", b), 1, "tonumber 1 in base " .. b)
		  eq(tonumber("0", b), 0, "tonumber 0 in base " .. b)
		end
		--    a base outside 2..36 is refused, and says so with a
		--    position in it
		local ok, e = pcall(tonumber, "ff", 37)
		eq(ok, false, "base 37 is refused")
		eq(e, "bad argument #2 to 'tonumber' (base out of range)",
		  "base 37 message")
		ok, e = pcall(tonumber, "ff", 1)
		eq(ok, false, "base 1 is refused")
		ok, e = pcall(tonumber, "ff", 0)
		eq(ok, false, "base 0 is refused")
		ok, e = pcall(tonumber, "ff", -1)
		eq(ok, false, "base -1 is refused")
		--    but a string that is not a numeral in that base is nil
		eq(tonumber("1x", 10), nil, "tonumber with a letter")
		eq(tonumber("", 10), nil, "tonumber empty")
		eq(tonumber("  ", 10), nil, "tonumber spaces only")
		eq(tonumber("+", 10), nil, "tonumber a sign alone")
		eq(tonumber("z", 10), nil, "tonumber z 10")

		-- 2. the reader 'load' is given, in every one of its shapes
		local i = 0
		local f = load(function()
		  i = i + 1
		  if i == 1 then return "return " end
		  if i == 2 then return "42" end
		  return nil
		end)
		eq(type(f), "function", "reader of two pieces")
		eq(f(), 42, "reader of two pieces runs")
		i = 0
		f = load(function()
		  i = i + 1
		  if i == 1 then return "return 42" end
		  return nil
		end)
		eq(f(), 42, "reader of one piece")
		i = 0
		f = load(function()
		  i = i + 1
		  if i == 1 then return "return 42" end
		  return ""
		end)
		eq(f(), 42, "an empty piece ends it")
		f = load(function() return nil end)
		eq(type(f), "function", "a nil piece is an empty chunk")
		eq(select("#", f()), 0, "an empty chunk answers nothing")
		--    a number is a string to 'lua_isstring', which is why a
		--    reader that keeps answering one never ends; here it
		--    answers one and then stops, and the chunk is not one
		i = 0
		local r1, r2 = load(function()
		  i = i + 1
		  if i == 1 then return 1 end
		  return nil
		end)
		eq(r1, nil, "a number is a chunk that will not parse")
		eq(type(r2), "string", "and it says so")
		--    a boolean is not a string at all
		r1, r2 = load(function() return true end)
		eq(r1, nil, "a boolean is not a string")
		-- C luaL_error prefixes "chunk:line: "; match the suffix.
		eq(type(r2), "string", "boolean reader message type")
		eq(r2:sub(-#"reader function must return a string"),
		  "reader function must return a string",
		  "the message for a reader that answers a boolean")
		--    and a reader that raises is a load that failed
		r1, r2 = load(function() error("in the reader") end)
		eq(r1, nil, "a raising reader")
		eq(type(r2), "string", "a raising reader's message")
		--    a precompiled chunk has to be gathered whole first
		local d = string.dump(load("return 7"))
		local k, n = 0, 3
		f = load(function()
		  k = k + 1
		  local a = (k - 1) * n + 1
		  if a > #d then return nil end
		  return d:sub(a, k * n)
		end, "=c", "b")
		eq(type(f), "function", "a binary chunk in pieces")
		eq(f(), 7, "a binary chunk in pieces runs")

		-- 3. an environment, which 'setupvalue' has to put in the
		--    first upvalue — and which a chunk with no upvalue has
		--    no room for
		local g = loadfile(` + strconv.Quote(envFile) + `, "t", {X = 1})
		eq(g(), 1, "loadfile with an environment")
		g = loadfile(` + strconv.Quote(envFile) + `)
		eq(type(g), "function", "loadfile without an environment")
		eq(g(), nil, "without it the chunk reads the global X")
		g = load("return X", "=c", "t", {X = 3})
		eq(g(), 3, "load with an environment")
		g = load("return 1", "=c", "t", {})
		eq(g(), 1, "load with an environment and no upvalue")
		g = loadfile(` + strconv.Quote(plainFile) + `, "t", {X = 4})
		eq(g(), 4, "loadfile with an environment again")

		-- 4. the two answers that are a heap's, held as kinds
		eq(math.type(collectgarbage("count")), "float",
		  "count is a float")
		eq(type(collectgarbage("count")), "number", "count is a number")
		eq(collectgarbage("count") > 0, true, "a heap has a size")
		eq(type(select(2, pcall(collectgarbage, "step"))), "number",
		  "step is ignored and answers 0")
		eq(select(2, pcall(collectgarbage, "step")), 0,
		  "step answers integer 0")
		eq(select(2, pcall(collectgarbage, "isrunning")), 0,
		  "isrunning is ignored and answers 0")
		eq(select(2, pcall(collectgarbage, "generational")), 0,
		  "generational is ignored and answers 0")
		--    and asked from a place where it cannot be asked at all.
		--    A finalizer runs with the collector stopped, so every
		--    option answers nil rather than a number.
		local seen
		do
		  local t <close> = nil
		  local doomed = setmetatable({}, {__gc = function()
		    seen = tostring(collectgarbage("count")) .. "," ..
		           tostring(collectgarbage("collect"))
		  end})
		  doomed = nil
		end
		collectgarbage("collect")
		eq(seen, "nil,nil", "every option is nil inside a finalizer")

		-- 5. 'select' past the end, which is clamped to it
		eq(select("#", select(5, 1, 2)), 0,
		  "select past the end answers nothing")
		eq(select("#", select(99, 1, 2, 3)), 0,
		  "select far past the end answers nothing")
		ok, e = pcall(select, -5, 1, 2)
		eq(ok, false, "select far before the beginning is refused")
		eq(e, "bad argument #1 to 'select' (index out of range)",
		  "the message for an index before the beginning")

		-- 6. a precompiled chunk given whole rather than by pieces
		local e = string.dump(load("return 9"))
		eq(load(e, "=c", "b")(), 9, "a binary chunk given whole")
	`
}
