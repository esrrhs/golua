package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The fifth fuzz, over the coroutine library: not over what a coroutine
// computes, but over *where* it suspends or fails and *how* it is driven.
//
// A coroutine here is a goroutine parked at the yield point, which is
// nothing like C's setjmp-style resume, so this is the corner of the port
// least able to rely on a translation being right because it looks right.
// A probe of forty-six lines was enough to find the first difference,
// which is how this corner was chosen:
//
//	'coroutine.close' on a coroutine suspended with a pending '<close>'
//	variable whose '__close' yielded answered the yielded value here,
//	where PUC-Lua answers
//	false, "attempt to yield across a C-call boundary".
//
// Closing a to-be-closed variable is two calls in C, and the 'yy' they
// hand 'luaF_close' differs.  'closepaux', which is what
// 'luaE_resetthread' -- and so 'coroutine.close' -- asks, passes 0: the
// metamethod may not suspend.  'finishpcallk', which is what a 'pcall' in
// a yieldable thread is recovered by, passes 1: it may.  This port closed
// both ways with 1, so 'coroutine.close' answered the yielded value.
//
// Fixing it by passing 0 to both breaks 'locals.lua' the other way: that
// test suspends three times inside the '__close' of a function that
// raised.  So 'yy' is now an argument of luaD_closeprotected, and each
// caller says which of the two it is being.  Both halves are pinned in
// corolib_test.go; what is left is the grid.

// coroCtxs is every place a coroutine can suspend or fail in.  Each one
// is a function body with the action left open: an ordinary call, under
// one and under two 'pcall's, under 'xpcall', in each of the metamethods
// that Lua itself calls, in a 'for' iterator, in the callbacks of
// 'table.sort' and 'string.gsub', in the reader 'load' asks, and in a
// coroutine of its own.
//
// The last five are the interesting ones: they are C functions calling
// back into Lua, and a yield from inside one of them is the error
// "attempt to yield across a C-call boundary" -- or is not, depending on
// whether that C function is yieldable, which is not written down
// anywhere but in ldo.c.
var coroCtxs = []string{
	`%s`,
	`return pcall(function() %s end)`,
	`return pcall(function() return pcall(function() %s end) end)`,
	`return xpcall(function() %s end, function(e) return "H:" .. tostring(e) end)`,
	`local t = setmetatable({}, {__index = function(_, k) %s end}) return t.k`,
	`local t = setmetatable({}, {__newindex = function(_, k, v) %s end}) t.k = 1 return "nw"`,
	`local t = setmetatable({}, {__call = function(_, a) %s end}) return t(1)`,
	`local t = setmetatable({}, {__add = function(a, b) %s end}) return t + 1`,
	`local t = setmetatable({}, {__eq = function(a, b) %s end}) return t == setmetatable({}, getmetatable(t))`,
	`local t = setmetatable({}, {__lt = function(a, b) %s end}) return t < setmetatable({}, getmetatable(t))`,
	`local t = setmetatable({}, {__le = function(a, b) %s end}) return t <= setmetatable({}, getmetatable(t))`,
	`local t = setmetatable({}, {__len = function() %s end}) return #t`,
	`local t = setmetatable({}, {__concat = function(a, b) %s end}) return t .. "x"`,
	`local t = setmetatable({}, {__tostring = function() %s end}) return tostring(t)`,
	`local t = setmetatable({}, {__close = function(_, e) %s end}) do local x <close> = t end return "closed"`,
	// the same variable, but suspended while it is still pending: this
	// is the one 'coroutine.close' has to close, through the 'yy' that
	// is 0, and not the one the end of the block closes, through the
	// 'yy' that is 1
	`local t = setmetatable({}, {__close = function(_, e) %s end}) local x <close> = t coroutine.yield("body") return "after"`,
	// these three have code after the action, so the action goes in a
	// 'do' block: an action that ends in 'return' is then the last
	// statement of that block, which Lua allows, instead of being a
	// 'return' in the middle of the callback, which it does not
	`local s = 0 for v in function(_, i) if i > 2 then return nil end do %s end return i + 1, i end, nil, 1 do s = s + v end return s`,
	`local n = 0 local ok = pcall(table.sort, {3, 1, 2}, function(a, b) n = n + 1; do %s end return a < b end) return ok, n`,
	`return string.gsub("ab", ".", function(c) do %s end return c end)`,
	`local n = 0 return load(function() n = n + 1 if n == 1 then %s end end)`,
	`local w = coroutine.wrap(function() %s end) return w()`,
	`local inner = coroutine.create(function() %s end) return select(2, coroutine.resume(inner))`,
}

// coroCtxNames names them, so that a difference reads as a place and not
// as a line number.
var coroCtxNames = []string{
	"bare", "pcall", "pcall/2", "xpcall",
	"__index", "__newindex", "__call", "__add", "__eq", "__lt", "__le",
	"__len", "__concat", "__tostring", "__close", "__close/pending",
	"for-iterator", "table.sort", "string.gsub", "load-reader",
	"wrapped", "resumed",
}

// coroActs is what the coroutine does at that place: suspend, fail,
// finish, or suspend and then do one of the other two.  The last one
// suspends twice, which is the only way to ask whether the second
// suspension lands in the same state as the first.
var coroActs = []string{
	`coroutine.yield("Y")`,
	`error("E")`,
	`coroutine.yield("Y") error("E")`,
	`coroutine.yield("Y") return "R"`,
	`return "R"`,
	`coroutine.yield("Y") coroutine.yield("Y2") return "R"`,
}

// coroDrivers is how each coroutine is driven.  'resume' is the ordinary
// one; 'wrap' is the same thing through 'coroutine.wrap', whose error is
// raised rather than returned; 'close' is 'coroutine.close' on a
// suspended coroutine, and then a resume of what is left of it.
var coroDrivers = []string{"resume", "wrap", "close"}

// coroChunk is the harness.  'drive' takes the body as a *function*, so
// that no case has to travel through a string and be escaped.
//
// Every result goes through 'q', and 'pcall' wraps every resume and every
// close, so that one case failing cannot stop the other 395 from being
// asked and cannot put a second line where one was expected.
const coroChunk = fuzzQuote + `local function P(i, kind, ctx, f)
  local t = {}
  local function step(r)
    -- every element, index 1 included: it is the boolean pcall answered,
    -- and table.concat wants a string or a number, not a boolean
    for j = 1, #r do r[j] = q(r[j]) end
    t[#t + 1] = table.concat(r, ",")
  end
  if kind == "resume" then
    local co = coroutine.create(f)
    for k = 1, 4 do step({pcall(coroutine.resume, co, "A" .. k)}) end
    t[#t + 1] = coroutine.status(co)
  elseif kind == "wrap" then
    local w = coroutine.wrap(f)
    for k = 1, 3 do step({pcall(w, "A" .. k)}) end
  else
    local co = coroutine.create(f)
    step({pcall(coroutine.resume, co, "A1")})
    step({pcall(coroutine.close, co)})
    step({pcall(coroutine.resume, co)})
    t[#t + 1] = coroutine.status(co)
  end
  print(i, kind, ctx, table.concat(t, "\t"))
end
`

func TestCoroutineFuzzDifferential(t *testing.T) {
	t.Skip("Lua 5.3.4: corofuzz chunk uses <close>/coroutine.close")
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the coroutine fuzz")
	}
	cases := len(coroCtxs) * len(coroActs) * len(coroDrivers)

	var b strings.Builder
	b.WriteString(coroChunk)
	n := 0
	for i, ctx := range coroCtxs {
		for _, act := range coroActs {
			body := strings.Replace(ctx, "%s", act, 1)
			for _, kind := range coroDrivers {
				n++
				b.WriteString("P(" + strconv.Itoa(n) + ", " +
					strconv.Quote(kind) + ", " +
					strconv.Quote(coroCtxNames[i]) + ", " +
					"function()\n" + body + "\nend)\n")
			}
		}
	}
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "corofuzz.lua"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	want := fmtRunFile(t, ref, dir, "corofuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dir, "corofuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the coroutine chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d places to suspend in x %d things to do x %d ways to drive, "+
			"%d coroutines, all of them PUC-Lua's",
			len(coroCtxs), len(coroActs), len(coroDrivers), cases)
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	perCase := len(coroDrivers)
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
		if i < cases {
			what = fmt.Sprintf("%s in %s, driven by %s",
				coroActs[(i/perCase)%len(coroActs)],
				coroCtxNames[(i/perCase)/len(coroActs)],
				coroDrivers[i%perCase])
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, b)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", cases)
	}
}
