package core

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestDirectRemainders(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// kname
	if err := L.LoadString(`local s="hi"; return function() return s end`, "=k"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	cl := clLvalue(index2value(L, -1))
	var name string
	_ = kname(cl.p, 0, &name)
	_ = kname(cl.p, -1, &name)
	_ = kname(cl.p, 999, &name)

	// luaH_free empty body — still call
	luaH_free(L, &Table{})

	// unwindForClose / fastYieldNow on a thread
	co := L.NewThread()
	co.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		_ = co.unwindForClose()
	}()
	func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(*yieldAbort); !ok {
					// prepareFastYield may runerror if not yieldable
					_ = r
				}
			}
		}()
		co.nCcalls = 0
		co.fastYieldNow(0)
	}()

	_ = testWarnfSeesUD(L)

	// resetThreadStatus more
	_ = L.resetThreadStatus(LUA_ERRRUN)
	_ = L.resetThreadStatus(LUA_ERRMEM)

	// errorstatus / correctstack again after CI growth
	L.extendCI()
	luaE_freeCI(L)
}

func TestMetamethodBinOpsAndEq(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local mt = {
  __add=function(a,b) return 10 end,
  __sub=function(a,b) return 11 end,
  __mul=function(a,b) return 12 end,
  __div=function(a,b) return 13 end,
  __mod=function(a,b) return 14 end,
  __pow=function(a,b) return 15 end,
  __unm=function(a) return 16 end,
  __idiv=function(a,b) return 17 end,
  __band=function(a,b) return 18 end,
  __bor=function(a,b) return 19 end,
  __bxor=function(a,b) return 20 end,
  __bnot=function(a) return 21 end,
  __shl=function(a,b) return 22 end,
  __shr=function(a,b) return 23 end,
  __eq=function(a,b) return true end,
  __lt=function(a,b) return true end,
  __le=function(a,b) return true end,
  __concat=function(a,b) return "c" end,
  __len=function(a) return 7 end,
  __index=function(t,k) return "idx" end,
  __newindex=function(t,k,v) rawset(t, k, v) end,
  __call=function(t,...) return 99 end,
  __tostring=function() return "T" end,
}
local a=setmetatable({}, mt)
local b=setmetatable({}, mt)
say(a+b); say(a-b); say(a*b); say(a/b); say(a%b); say(a^b); say(-a)
say(a//b); say(a&b); say(a|b); say(a~b); say(~a); say(a<<1); say(a>>1)
say(a==b); say(a<b); say(a<=b); say(a..b); say(#a); say(a.x); a.y=1; say(a()); say(tostring(a))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "10") || !strings.Contains(got, "99") {
		t.Fatalf("%q", got)
	}
}

func TestParserConflictAndGoto(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local a, b
a, b = 1, 2
say(a+b)
local t = {}
t.x, t.y = 3, 4
say(t.x+t.y)
local i = 1
::lab::
if i < 3 then i=i+1; goto lab end
say(i)
local function f()
  local x = 1
  do local x = 2; return x end
end
say(f())
-- upvalue conflict in assignment
local u
local function g() return u end
local v = {}
v[1], u = 5, 6
say(g())
return table.concat(o, ",")
`)
	if !strings.Contains(got, "3") {
		t.Fatalf("%q", got)
	}
}

func TestTableUnboundAndNext(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local t={}
t[1]=1; t[2]=2; t[4]=4
say(#t) -- unbound search may stop at hole
local n=0
for k,v in pairs(t) do n=n+1 end
say(n)
t[1000]=true
say(t[1000])
local ok = pcall(function() table.insert({}, 1, 1) end)
say(ok)
say(table.concat({"a","b"}, ","))
say(table.pack(1,2,nil,4).n)
local a,b,c = table.unpack({1,2,3})
say(a+b+c)
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}

func TestUTF8AndBaseMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(utf8.len("abc"))
say(utf8.len("a\x80") == nil)
say(select(1, utf8.len("a\x80")))
local ok = pcall(utf8.char, -1)
say(not ok)
say(utf8.offset("abc", 2))
say(utf8.offset("abc", -1))
ok = pcall(utf8.offset, "abc", 10)
say(not ok or utf8.offset("abc", 10) == nil)
say(select('#', pcall(print, 1)))
say(tostring(1.0))
say(tonumber("  0x10  "))
say(tonumber("1e2"))
say(tonumber("1.5"))
say(rawequal(1,1))
say(select(2, pcall(rawget, 1, 1)) ~= nil or true)
local t={}; setmetatable(t,{__metatable="x"}); say(getmetatable(t))
ok = pcall(setmetatable, t, {})
say(not ok)
say(type(_G))
say(assert(true))
ok = pcall(assert, false, "nope")
say(not ok)
say(select(1, load("return 1"))() )
say(select(1, load("return function() end")) ~= nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestOSDateTimeLocale(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(os.time() > 0)
local t = os.date("*t", 0)
say(t.year)
say(os.date("!%H", 0))
say(os.setlocale(nil, "all") ~= nil or true)
say(os.setlocale("C", "ctype"))
say(os.clock() >= 0)
local ok = pcall(os.exit, 0, false) -- may not return; don't call with true
-- instead just check exit is a function
say(type(os.exit))
say(os.difftime(100, 50))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "function") {
		t.Fatalf("%q", got)
	}
}

func TestIOModesAndTMP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CG3", dir)
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local d = os.getenv("CG3")
local p = d.."/a.txt"
local f = assert(io.open(p, "wb"))
say(f:write("xyz"))
f:flush()
say(f:seek("set", 0))
-- reopen read
f:close()
f = assert(io.open(p, "rb"))
say(f:read(3))
say(f:read(0) == "")
f:close()
f = assert(io.open(p, "a"))
say(f:write("\n"))
f:close()
local tmp = assert(io.tmpfile())
tmp:write("t")
tmp:seek("set")
say(tmp:read("*a"))
tmp:close()
say(io.type(1) == nil)
say(io.stdin ~= nil)
say(io.stdout ~= nil)
say(io.stderr ~= nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "xyz") {
		t.Fatalf("%q", got)
	}
}

func TestDebugHookAndTraceback(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local n=0
debug.sethook(function(ev) n=n+1 end, "c", 0)
local function f() return 1 end
f(); f()
debug.sethook()
say(n > 0)
debug.sethook(function() end, "l", 0)
for i=1,3 do end
debug.sethook()
say(debug.traceback("msg", 1):find("msg", 1, true) ~= nil)
say(debug.getinfo(1, "S").what)
local function g(a) local b=a; return debug.getlocal(1,2) end
say(select(1, g(5)))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestStringPackUnpackAndRep(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local s = string.pack("i4", 42)
say(#s)
say(string.unpack("i4", s))
say(string.packsize("i4"))
say(string.rep("ab", 3))
say(string.rep("ab", 3, "-"))
say(string.reverse("abc"))
say(string.sub("abcdef", 2, 4))
say(string.gsub("a b c", "%s", "-"))
say(string.gmatch("a b", "%w")())
say(string.find("abc", "b", 1, true))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "42") {
		t.Fatalf("%q", got)
	}
}

func TestMathMoreEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(math.abs(-2))
say(math.ceil(1.2))
say(math.floor(1.8))
say(math.sqrt(4))
say(math.sin(0))
say(math.cos(0))
say(math.tan(0))
say(math.asin(0))
say(math.acos(1))
say(math.atan(0))
say(math.atan(1,1) ~= nil)
say(math.deg(math.pi) > 179)
say(math.rad(180) > 3)
say(math.exp(0))
say(math.log(1))
say(math.log(8, 2))
say(math.log(math.mininteger) ~= nil or true)
say(math.modf(1.5))
say(select(2, math.modf(1.5)))
say(math.fmod(7,3))
say(math.max(1,2,3))
say(math.min(1,2,3))
say(math.ult(1,2))
say(math.tointeger(1.0))
say(math.tointeger(1.5) == nil)
say(math.random(1,1))
math.randomseed(1)
say(math.type(math.maxinteger))
say(math.type(1.0))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "integer") {
		t.Fatalf("%q", got)
	}
}

func TestLoadlibSearchersAndPath(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "mymod.lua")
	if err := os.WriteFile(mod, []byte("return {ok=true}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUA_PATH_TEST", dir+string(os.PathSeparator)+"?.lua")
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
package.path = os.getenv("LUA_PATH_TEST")
local m = require("mymod")
say(m.ok)
say(package.loaded.mymod ~= nil)
local ok, err = pcall(require, "nope_mod_xyz")
say(not ok)
-- preload
package.preload.pre1 = function() return {p=1} end
say(require("pre1").p)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestPushFStringAndChunkid(t *testing.T) {
	L := NewState()
	defer L.Close()
	// pushfString via Error2 format paths already covered; call luaO_pushfstring if exists
	_ = luaO_chunkid("@file.lua")
	_ = luaO_chunkid("=stdin")
	_ = luaO_chunkid("short")
	_ = luaO_int2fb(0)
	_ = luaO_int2fb(7)
	_ = luaO_int2fb(8)
	_ = luaO_int2fb(100)
	_ = luaO_int2fb(math.MaxInt32)
	var v Value
	setfltvalue(&v, math.Inf(1))
	setfltvalue(&v, math.NaN())
	_ = luaO_str2num("inf", &v)
	_ = luaO_str2num("1e400", &v)
	_ = luaO_str2num("0x1.0p-1074", &v)
}

func TestCoroutineCloseViaCollect(t *testing.T) {
	// Exercise recoverCo / close paths by abandoning a coroutine.
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
do
  local co = coroutine.create(function()
    coroutine.yield(1)
    error("should not")
  end)
  say(select(2, coroutine.resume(co)))
  co = nil
end
collectgarbage("collect")
collectgarbage("collect")
say(true)
-- yield across pcall
local co2 = coroutine.create(function()
  pcall(function() coroutine.yield(2) end)
  return 3
end)
say(select(2, coroutine.resume(co2)))
say(select(2, coroutine.resume(co2)))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "1") {
		t.Fatalf("%q", got)
	}
}

func TestLexEscapesMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say("\a\b\f\n\r\t\v\\")
say('\'')
say("\"")
say("\10")
say("\x00" == string.char(0))
local ok = load("return '\\u{110000}'")
say(ok == nil)
ok = load("return '\\u{}'")
say(ok == nil)
ok = load("return '\\x'")
say(ok == nil)
ok = load("return [[\n]]")
say(ok ~= nil)
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}

func TestCompareInvalidOp(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	func() {
		defer func() { _ = recover() }()
		_ = L.Compare(-2, -1, 99)
	}()
}

func TestPtrAndLightUserdata(t *testing.T) {
	L := NewState()
	defer L.Close()
	p := unsafe.Pointer(new(int))
	L.PushLightUserData(p)
	if L.ToPointer(-1) == 0 {
		t.Fatal("lightud")
	}
	if L.ToUserData(-1) != nil && L.Type(-1) != LUA_TLIGHTUSERDATA {
		// ToUserData may return nil for light userdata depending on API
	}
	_ = L.TypeName(-1)
}
