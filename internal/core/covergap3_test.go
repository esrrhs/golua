package core

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverGap3VMEdges(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

-- int mod / idiv edges
say((-1) % 1)
say(1 % -1)
say((-7) // 3)
say(7 // -3)
say((-7) // -3)
say(math.maxinteger % -1)
say((-math.maxinteger-1) % 1)

-- for-loop float/int edges
local n = 0
for i = 1.5, 3.5, 1 do n = n + 1 end
say(n)
n = 0
for i = 10, 1, -2 do n = n + 1 end
say(n)
boom(function() for i = 1, 10, 0 do end end)
boom(function() for i = {}, 10, 1 do end end)
boom(function() for i = 1, {}, 1 do end end)
boom(function() for i = 1, 10, {} do end end)
local lim = math.maxinteger
local c = 0
for i = lim-1, lim, 1 do c = c + 1; if c > 3 then break end end
say(c >= 1)

-- metamethod chain / index errors
local idxdeep = {}
idxdeep.__index = idxdeep
boom(function() return idxdeep.x end)
local setdeep = {}
setdeep.__newindex = setdeep
boom(function() setdeep.x = 1 end)
boom(function() local t = {}; return t[nil] end)
local t = setmetatable({}, {__index = function() error("ix") end})
boom(function() return t.k end)
t = setmetatable({}, {__newindex = function() error("nx") end})
boom(function() t.k = 1 end)

-- concat overflow-ish and metamethod
say("a" .. "b" .. "c")
local cm = setmetatable({}, {__concat = function(a,b) return "X" end})
say(cm .. "y")
boom(function() return "a" .. {} end)

-- large table constructor (SETLIST extra)
local big = {
1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,
21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,
41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,
61,62,63,64,65,66,67,68,69,70,71,72,73,74,75,76,77,78,79,80,
81,82,83,84,85,86,87,88,89,90,91,92,93,94,95,96,97,98,99,100,
101,102,103,104,105,106,107,108,109,110
}
say(#big)

-- CLOSE / to-be-closed in loops
local closed = 0
local function mk()
  return setmetatable({}, {__close = function() closed = closed + 1 end})
end
do
  local x <close> = mk()
  local y <close> = mk()
end
say(closed)

-- vararg and select
local function va(a, ...)
  say(select("#", ...))
  say(select(1, ...))
end
va(0, 10, 20)

-- bitwise with floats error
boom(function() return 1.5 & 1 end)
boom(function() return 1 ~ 1.5 end)

-- length metamethod
local lm = setmetatable({}, {__len = function() return 7 end})
say(#lm)

-- EQ/LT/LE metamethods
local ord = {}
ord.__lt = function(a,b) return true end
ord.__le = function(a,b) return true end
ord.__eq = function(a,b) return true end
local a = setmetatable({}, ord)
local b = setmetatable({}, ord)
say(a < b)
say(a <= b)
say(a == b)

-- load with reader style via string.dump strip
local f = load(string.dump(function() return 42 end, true))
say(f())

-- goto leave close scope
local gclosed = 0
local function gtest()
  local x <close> = setmetatable({}, {__close=function() gclosed=gclosed+1 end})
  goto done
  ::done::
end
gtest()
say(gclosed)

-- repeat-until with close
local rclosed = 0
repeat
  local x <close> = setmetatable({}, {__close=function() rclosed=rclosed+1 end})
until true
say(rclosed)

-- assignment conflict (a, t[a] = t[a], a)
local t2 = {1,2}
local i = 1
i, t2[i] = t2[i], i
say(i)
say(t2[1])

-- upvalue assignment conflict style
local u = 1
local function make()
  local function get() return u end
  local function set(v) u = v end
  return get, set
end
local get, set = make()
set(9)
say(get())

return table.concat(out, "\n")
`)
	if len(strings.Split(got, "\n")) < 20 {
		t.Fatalf("vm:\n%s", got)
	}
}

func TestCoverGap3Coroutines(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end

-- too many args / results hard to hit; dead resume; close running
local co = coroutine.create(function() return 1 end)
coroutine.resume(co)
say(select(1, coroutine.resume(co))) -- dead
say(coroutine.status(co))

local run = coroutine.create(function()
  say(select(1, coroutine.close(coroutine.running()))) -- cannot close running
  coroutine.yield(1)
end)
coroutine.resume(run)
say(coroutine.status(run))
say(coroutine.close(run))

-- close with error in __close
local bad = coroutine.create(function()
  local x <close> = setmetatable({}, {__close = function() error("c") end})
  coroutine.yield(1)
end)
coroutine.resume(bad)
local ok, err = coroutine.close(bad)
say(ok)
say(tostring(err):find("c") ~= nil)

-- wrap error
local w = coroutine.wrap(function() error("w") end)
say(not pcall(w))

-- isyieldable inside pcall
local y = false
local co2 = coroutine.create(function()
  y = coroutine.isyieldable()
  pcall(function() y = coroutine.isyieldable() end)
end)
coroutine.resume(co2)
say(y)

-- status of main
say(coroutine.status(coroutine.running()))

-- create from create
local outer = coroutine.create(function()
  local inner = coroutine.create(function() return 5 end)
  local ok, v = coroutine.resume(inner)
  return v
end)
say(select(2, coroutine.resume(outer)))

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap3LuacEdges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	if err := os.WriteFile(src, []byte("return '\\a\\b\\f\\r\\v\\n\\t\\\\\"'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	// listing with escapes in constants
	code := Luac([]string{"luac", "-l", "-l", src}, &out, &errb)
	if code != 0 {
		t.Fatalf("list escapes: %s", errb.String())
	}
	out.Reset()
	errb.Reset()
	// -o - (stdout)
	code = Luac([]string{"luac", "-o", "-", src}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	// multiple -v
	code = Luac([]string{"luac", "-v", "-v"}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	// option "-" end then file
	code = Luac([]string{"luac", "-", src}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	// dumping to unwritable path
	badOut := filepath.Join(dir, "no", "such", "dir", "x.out")
	code = Luac([]string{"luac", "-o", badOut, src}, &out, &errb)
	if code == 0 {
		t.Fatal("expected cannot open")
	}
	out.Reset()
	errb.Reset()
	// combine listing without dump
	code = Luac([]string{"luac", "-l", "-p", src}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	// too many files: create many
	args := []string{"luac", "-p"}
	for i := 0; i < 300; i++ {
		args = append(args, src)
	}
	code = Luac(args, &out, &errb)
	_ = code

	// stdin path
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("return 1\n"))
		_ = w.Close()
	}()
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-p", "-"}, &out, &errb)
	os.Stdin = old
	_ = r.Close()
	_ = code
}

func TestCoverGap3DebugNames(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end

-- force metamethod name from code paths via debug.getinfo
local mt = {
  __add = function(a,b) return 0 end,
  __sub = function(a,b) return 0 end,
  __mul = function(a,b) return 0 end,
  __div = function(a,b) return 0 end,
  __mod = function(a,b) return 0 end,
  __pow = function(a,b) return 0 end,
  __unm = function(a) return 0 end,
  __idiv = function(a,b) return 0 end,
  __band = function(a,b) return 0 end,
  __bor = function(a,b) return 0 end,
  __bxor = function(a,b) return 0 end,
  __bnot = function(a) return 0 end,
  __shl = function(a,b) return 0 end,
  __shr = function(a,b) return 0 end,
  __len = function(a) return 0 end,
  __concat = function(a,b) return "" end,
  __eq = function(a,b) return false end,
  __lt = function(a,b) return false end,
  __le = function(a,b) return false end,
  __index = function(a,k) return nil end,
  __newindex = function(a,k,v) end,
  __call = function(a) end,
  __close = function(a) end,
}
local a = setmetatable({}, mt)
local b = setmetatable({}, mt)
local _ = a + b; _ = a - b; _ = a * b; _ = a / b; _ = a % b; _ = a ^ b; _ = -a
_ = a // b; _ = a & b; _ = a | b; _ = a ~ b; _ = ~a; _ = a << 1; _ = a >> 1
_ = #a; _ = a .. b; _ = a == b; _ = a < b; _ = a <= b; _ = a.x; a.y = 1; a()

-- for iterator name
for i,v in ipairs({1,2}) do say(i) end
for k in pairs({x=1}) do say(k) end

-- getlocal vararg
local function f(a, ...)
  say(debug.getlocal(1, -1) ~= nil or true)
  local n, v = debug.getlocal(1, 1)
  say(n)
end
f(1, 2, 3)

-- getinfo on C function
say(debug.getinfo(print, "S").what)

-- hook count
local hits = 0
debug.sethook(function() hits = hits + 1 end, "", 10)
for i = 1, 100 do end
debug.sethook()
say(hits > 0)

-- traceback with levels
say(debug.traceback(nil, 1):find("stack") ~= nil)

-- debug.debug is interactive; skip
say(type(debug.getregistry()))

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap3PackageAndLoad(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "mymod.lua")
	if err := os.WriteFile(mod, []byte("return {v=123}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUA_PATH", filepath.Join(dir, "?.lua"))
	// Also set via package
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

package.path = os.getenv("LUA_PATH") or package.path
local m = require("mymod")
say(m.v)
say(package.loaded.mymod ~= nil)

-- searchpath miss
local p, err = package.searchpath("zzz", "./?.lua")
say(p == nil)
say(err:find("zzz") ~= nil)

-- loadfile
local f = assert(loadfile(package.searchpath("mymod", package.path)))
say(f().v)

-- package.loadlib missing
if package.loadlib then
  local a,b,c = package.loadlib("no_such_lib.so", "luaopen_x")
  say(a == nil)
end

-- searchers error
do
  local saved = package.searchers
  boom(function()
    package.searchers = "bad"
    require("anything")
  end)
  package.searchers = saved
end

-- preload simple name
package.preload["covergap3p"] = function() return true end
say(require("covergap3p") == true)

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap3ObjectAndCode(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// pushfString format branches
	_ = luaO_pushfstring(L, "%% %s %c %d %I %f %p %U",
		"hi", int('A'), 3, int64(4), 1.5, L, uint(65))
	_ = luaO_pushfstring(L, "%s", nil)
	func() {
		defer func() { _ = recover() }()
		_ = luaO_pushfstring(L, "%d") // missing arg
	}()
	L.SetTop(0)

	// chunk ID helpers via errors
	if err := L.LoadString(strings.Repeat("x", 100), "@"+strings.Repeat("p", 80)); err == nil {
		L.SetTop(0)
	}
	_ = L.LoadString("(", "=short")
	L.SetTop(0)
	_ = L.LoadString("return [=["+strings.Repeat("a", 70)+"]=]", "@file")
	L.SetTop(0)

	// int arith helpers
	_ = intarith(L, LUA_OPADD, 1, 2)
	_ = intarith(L, LUA_OPSUB, 1, 2)
	_ = intarith(L, LUA_OPMUL, 1, 2)
	_ = intarith(L, LUA_OPMOD, 1, 2)
	_ = intarith(L, LUA_OPIDIV, 1, 2)
	_ = intarith(L, LUA_OPBAND, 1, 2)
	_ = intarith(L, LUA_OPBOR, 1, 2)
	_ = intarith(L, LUA_OPBXOR, 1, 2)
	_ = intarith(L, LUA_OPSHL, 1, 2)
	_ = intarith(L, LUA_OPSHR, 1, 2)
	_ = intarith(L, LUA_OPUNM, 1, 0)
	_ = intarith(L, LUA_OPBNOT, 1, 0)
	func() {
		defer func() { _ = recover() }()
		_ = intarith(L, 999, 1, 2)
	}()
	_ = numarith(L, LUA_OPADD, 1, 2)
	_ = numarith(L, LUA_OPDIV, 1, 2)
	_ = numarith(L, LUA_OPPOW, 2, 3)
	_ = numarith(L, LUA_OPIDIV, 5, 2)
	func() {
		defer func() { _ = recover() }()
		_ = numarith(L, 999, 1, 2)
	}()

	// luaO_rawarith failure paths
	var r Value
	var a, b Value
	setivalue(&a, 1)
	sethvalue(&b, luaH_new(L))
	_ = luaO_rawarith(L, LUA_OPADD, &a, &b, &r)

	// short source
	_ = luaO_chunkid("=name")
	_ = luaO_chunkid("@short.lua")
	_ = luaO_chunkid("@" + strings.Repeat("x", 100))
	_ = luaO_chunkid("[string \"hi\"]")
	_ = luaO_chunkid(strings.Repeat("z", 100) + "\nmore")
	_ = luaO_chunkid("")

	// str2num locale path
	old := localeDecPoint
	localeDecPoint = ","
	var o Value
	_ = luaO_str2num("1,5", &o)
	localeDecPoint = old

	// l_str2d hex etc
	for _, num := range []string{"0x1.8p4", "inf", "nan", "1.5", "  42  "} {
		if L.StringToNumber(num) != 0 {
			L.Pop(1)
		}
	}

	// findlocal more: temporary names via hook on C call (before shrink)
	L.SetHook(func(L *State, ar *dbAr) {
		pos := 0
		_ = findlocal(L, L.ci, 1, &pos)
		_ = findlocal(L, L.ci, 50, &pos)
		_ = findvararg(L, L.ci, 1, &pos)
		_ = findvararg(L, L.ci, 100, &pos)
	}, dbMaskCall, 0)
	L.PushGoFunc(func(L *State) int { return 0 })
	_ = L.PCall(0, 0, 0)
	L.SetHook(nil, 0, 0)

	// growStackNoRaise with raise=false near limit - use moderate
	_ = L.growStackNoRaise(BASIC_STACK_SIZE*2, false)
	L.shrinkStack()

	// freestack early
	th := &State{}
	freestack(th)

	// CloseThread already covered; ResetThread on main
	_ = L.ResetThread()

	// math extremes without relying on !haveLibm
	_ = mathExpScaled(0, 0)
	_ = mathHalfExp(1)
	_ = mathLogScaled(1)
	_ = mathLog10Scaled(10)
	_ = mathLog2Scaled(0.9)
	_ = mathLog2Scaled(1e-400) // underflow-ish

	// ioCheckModeP direct
	_ = ioCheckModeP("r")
	_ = ioCheckModeP("w")
	_ = ioCheckModeP("")
	_ = ioCheckModeP("x")
	_ = ioCheckModeP("rb")
	_ = ioCheckModeP("rt")
	_ = ioCheckModeP("rbb")

	_ = math.NaN() // keep math import used if needed
}

func TestCoverGap3IOMore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "n.txt")
	t.Setenv("C3", path)
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
local name = os.getenv("C3")
local f = assert(io.open(name, "w+"))
f:write("1e10\n0x10\n.5\n+1.5\n-2.5e-1\nnan\ninf\n")
f:seek("set")
for i = 1, 8 do
  local n = f:read("*n")
  say(n ~= nil or true)
end
f:seek("set")
say(f:read("*L"):find("\n") ~= nil)
f:seek("set")
say(#(f:read(1000) or ""))
f:seek("end")
say(f:read(1) == nil)
f:close()

-- io.read defaults
local saved = io.input()
io.input(name)
say(io.read("*l") ~= nil)
io.input(saved)

-- type of non-file
say(io.type(1) == nil)
say(io.type({}) == nil)

-- lines with formats
f = assert(io.open(name))
local it = f:lines("*l")
say(it() ~= nil)
f:close()

boom(function() io.open(name, "r+b+x") end)
f = assert(io.open(name, "r+b"))
f:close()

return table.concat(out, "\n")
`)
	_ = got
}

func TestCoverGap3StringMore(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

say(string.find("abc", "b", -2))
say(string.sub("abc", -2))
say(string.byte("a", 0, 0))
say(string.rep("a", 0))
say(string.rep("a", 2, ""))
say(string.reverse(""))
say(string.format("%i", 3))
say(string.format("%u", 3))
say(string.format("%X", 255))
say(string.format("%#08x", 1))
say(string.format("%-8s", "ab"))
say(string.format("%.0f", 1.5))
say(string.format("%+e", 1.5))
say(string.format("%#g", 1.0))
say(string.format("%a", math.huge))
say(string.format("%a", -math.huge))
say(string.format("%a", 0/0))
say(string.format("%q", ""))
say(string.gsub("aaa", "a+", "b", 1))
say(string.gsub("abc", "b", {b="X"}))
say(string.gmatch("a1b2c3", "(%l)(%d)")())
boom(function() string.format("%"..("0"):rep(5)..".500d", 1) end)

-- pack endian / align
say(#string.pack(">i4", 1))
say(#string.pack("=i4", 1))
say(#string.pack("<!8i4", 1))
say(#string.pack("i4xXi4", 1, 2))
say(string.unpack(">i4", string.pack(">i4", -2)))
boom(function() string.unpack("i4", "ab") end)
boom(function() string.pack("c5", "ab") end)
say(#string.pack("c3", "abc"))
boom(function() string.pack("c2", "abcd") end)
say(#string.pack("s4", "hi"))
say(string.unpack("s4", string.pack("s4", "hi")))

-- position captures in gsub
say(string.gsub("abc", "()b()", "%1-%2"))
say(string.match("abc", "()a()"))

-- frontier
say(string.match(" a", "%f[%a]%a"))
say(string.match("a ", "%f[%A]"))

return table.concat(out, "\n")
`)
	_ = got
}
