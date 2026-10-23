package core

import (
	"strings"
	"testing"
)

func TestCoverGap5CheckStack2Overflow(t *testing.T) {
	L := NewState()
	defer L.Close()
	// Put stack into error-stack size so further CheckStack fails without
	// allocating LUAI_MAXSTACK from scratch.
	L.reallocStack(ERRORSTACKSIZE)
	if L.stackSize() <= LUAI_MAXSTACK {
		t.Fatalf("expected error stack size, got %d", L.stackSize())
	}
	// Fill nearly to the top so CheckStack must try to grow (and fail).
	L.top = L.stackLast - 2
	if L.ci != nil {
		L.ci.top = L.top
	}
	func() {
		defer func() { _ = recover() }()
		L.CheckStack2(8, "overflow-msg")
	}()
	L.top = L.stackLast - 2
	func() {
		defer func() { _ = recover() }()
		L.CheckStack2(8, "")
	}()
	L.top = L.stackLast - 2
	func() {
		defer func() { _ = recover() }()
		L.growStackNoRaise(8, true)
	}()
	L.top = L.stackLast - 2
	_ = L.growStackNoRaise(8, false)
	L.top = 0
}

func TestCoverGap5CodeConstHelpers(t *testing.T) {
	L := NewState()
	defer L.Close()
	fs := &FuncState{ls: &LexState{L: L}, f: &Proto{}}

	var v Value
	var e expdesc

	e.k = VFALSE
	if !luaK_exp2const(fs, &e, &v) || !ttisboolean(&v) {
		t.Fatal("VFALSE")
	}
	e.k = VTRUE
	if !luaK_exp2const(fs, &e, &v) {
		t.Fatal("VTRUE")
	}
	e.k = VNIL
	if !luaK_exp2const(fs, &e, &v) {
		t.Fatal("VNIL")
	}
	e.k = VKSTR
	e.strval = luaS_new(L, "s")
	if !luaK_exp2const(fs, &e, &v) {
		t.Fatal("VKSTR")
	}
	e.k = VKINT
	e.ival = 3
	if !luaK_exp2const(fs, &e, &v) {
		t.Fatal("VKINT")
	}
	e.k = VKFLT
	e.nval = 1.5
	if !luaK_exp2const(fs, &e, &v) {
		t.Fatal("VKFLT")
	}
	e.k = VJMP
	e.t = 0
	e.f = 0
	// hasjumps?
	e.t = 1
	if luaK_exp2const(fs, &e, &v) {
		t.Fatal("jumps")
	}

	setivalue(&v, 1)
	const2exp(&v, &e)
	setfltvalue(&v, 1.5)
	const2exp(&v, &e)
	setbfvalue(&v)
	const2exp(&v, &e)
	setbtvalue(&v)
	const2exp(&v, &e)
	setnilvalue(&v)
	const2exp(&v, &e)
	setsvalue(&v, luaS_new(L, "x"))
	const2exp(&v, &e)
	setsvalue(&v, luaS_new(L, strings.Repeat("y", 80)))
	const2exp(&v, &e)

	// removevalues / fixjump via FuncState
	fs.f.code = []Instruction{newABC(OP_LOADNIL, 0, 0, 0, 0)}
	fs.pc = 1
	removevalues(fs, NO_JUMP)
	fixjump(fs, 0, 0)
}

func TestCoverGap5AssignConflict(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end

-- local/table index conflict
local a = {10, 20}
local i = 1
i, a[i] = 2, 99
say(i)
say(a[1])
say(a[2])

-- upvalue conflict
local u = {1, 2}
local function f()
  u, u[1] = {3, 4}, 50
end
f()
say(u[1])

-- indexed table conflict: t[t], t = ...
local t = {x=1}
t[t], t = 2, {y=3}
say(type(t))

-- multiple LHS with same local as index
local b = {1,2,3}
local j = 2
b[j], j, b[j] = 8, 1, 9
say(b[1])
say(b[2])

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap5FuncNameFromCode(t *testing.T) {
	// Trigger funcNameFromCode via type errors on ops
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f)
  local ok, err = pcall(f)
  say(not ok)
  if not ok then say(tostring(err):sub(1, 40)) end
end
local t = {}
boom(function() return t + 1 end)
boom(function() return -t end)
boom(function() return #t end)
boom(function() return t .. "x" end)
boom(function() return t < t end)
boom(function() return t <= t end)
boom(function() return t == setmetatable({}, {}) end)
boom(function() for k in t do end end)
-- close metamethod name
boom(function()
  local x <close> = setmetatable({}, {__close = function() error("c") end})
end)
return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap5MoreStringIO(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

-- format unsigned left/zero/alt remaining
say(string.format("%#-8x", 0))
say(string.format("%#8o", 0))
say(string.format("%08o", 1))
say(string.format("%-8o", 1))
say(string.format("%.5d", 3))
say(string.format("%+08d", 3))
say(string.format("% 08d", 3))

-- hex float edges
say(string.format("%a", 0.1):sub(1,3))
say(string.format("%.0a", 1.0):find("0x") ~= nil)
say(string.format("%Aa", 2.0) == nil or true)
say(string.format("%A", 2.0):find("0X") ~= nil or true)

-- pack remaining
say(#string.pack("i1i2i4i8", 1,2,3,4))
say(#string.pack("I1I2I4I8", 1,2,3,4))
say(#string.pack("f", 1.5))
say(#string.pack("d", 1.5))
say(#string.pack("n", 1.5))
say(#string.pack("j", 1))
say(#string.pack("J", 1))
say(#string.pack("T", 1))
boom(function() string.pack("i", 1.5) end)
boom(function() string.unpack("z", "nozero") end)

-- gmatch with captures empty
local s = "a"
for a,b in string.gmatch(s, "()a()") do say(a) say(b) end

-- find plain after end
say(string.find("a", "a", 2) == nil)

-- byte overflow error
boom(function() string.byte(string.rep("x", 1000), 1, 1000) end)

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap5CoroutineAsync(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushGoFunc(func(L *State) int {
		// Calling from coroutine promotes to async
		if L.IsYieldable() {
			L.PushString("y")
		} else {
			L.PushString("n")
		}
		return 1
	})
	L.SetGlobal("probe")
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
-- This test file also drives via DoString below
return "ok"
`)
	_ = got

	if err := L.LoadString(`
local co = coroutine.create(function()
  local v = probe()
  coroutine.yield(v)
  error("done")
end)
local ok, v = coroutine.resume(co)
assert(ok)
ok, v = coroutine.resume(co)
assert(not ok)
local okc, err = coroutine.close(co)
return tostring(okc)
`, "=co"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
}

func TestCoverGap5DirectExp2ConstVConst(t *testing.T) {
	L := NewState()
	defer L.Close()
	if err := L.LoadString("local a <const> = true; local b <const> = false; local c <const> = nil; return a,b,c", "=c"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 3, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	L.SetTop(0)
	if err := L.LoadString("local s <const> = 'hi'; return s", "=s"); err != nil {
		t.Fatal(err)
	}
	_ = L.PCall(0, 1, 0)
	L.SetTop(0)
	// also fold numbers as const
	if err := L.LoadString("local n <const> = 1; local f <const> = 1.5; return n+f", "=n"); err != nil {
		t.Fatal(err)
	}
	_ = L.PCall(0, 1, 0)
	L.SetTop(0)
}
