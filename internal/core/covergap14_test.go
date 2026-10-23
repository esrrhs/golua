package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverGap14SetErrorObj(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.checkStack(4)
	L.top = 2
	setnilvalue(&L.stack[0])
	setnilvalue(&L.stack[1])
	if L.g.memErrMsg != nil {
		luaD_seterrorobj(L, LUA_ERRMEM, 0)
	}
	luaD_seterrorobj(L, LUA_OK, 0)
	L.PushString("err")
	luaD_seterrorobj(L, LUA_ERRRUN, 0)
}

func TestCoverGap14DebugLocalsNonActive(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString(`local function f(a,b) local c=1; return a+b+c end; return f`, "=f"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	// non-active: ar==nil
	_ = dbGetLocal(L, nil, 1)
	_ = dbGetLocal(L, nil, 99)
	L.PushInteger(1)
	_ = dbGetLocal(L, nil, 1) // not Lclosure on top after push? top is int
	// collect lines for C func / L func
	L.SetTop(0)
	L.PushGoFunc(func(L *State) int { return 0 })
	v := index2value(L, -1)
	dbCollectValidLines(L, v)
	dbCollectValidLines(L, nil)
	L.SetTop(0)
	_ = runChunk(t, `
local function f(a) local b=1; return debug.getinfo(f, "L").activelines ~= nil end
return f(1), debug.getinfo(print, "L") == nil or true
`)
	// getlocal on function (non-active) via Lua
	_ = runChunk(t, `
local function f(a,b) end
local n = debug.getlocal(f, 1)
return n
`)
}

func TestCoverGap14StrMatchAndPack(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
say(string.match("abc", "a(b)c"))
say(string.match("abc", "a(b)(c)"))
say(string.gsub("abc", "a(.)c", "%1"))
say(string.gsub("abc", "a(.)c", function(x) return x end))
say(string.find("aaa", "a+", 1, true))
say(string.find("aaa", "a+"))
say(string.gmatch("a1b2", "(%l)(%d)")())
-- frontier
say(string.match(" a", "%f[%a]a"))
-- balanced
say(string.match("(a(b)c)", "%b()"))
-- captures empty
say(string.match("abc", "a()b()c"))
-- pack/unpack exotic
say(string.pack("f", 1.5))
say(string.pack("d", 1.5))
say(string.pack("n", 1.5))
say(string.pack("c3", "abc"))
boom(function() string.pack("c0") end)
say(select(1, string.unpack("c3", "abcdef")))
say(string.packsize("c3"))
say(string.pack("x"))
say(string.pack(" j", 1))
boom(function() string.pack("i0", 1) end)
boom(function() string.unpack("f", "xx") end)
boom(function() string.format("%#d", 1) end)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap14IOMore(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local n = os.tmpname()
local f = assert(io.open(n, "w+"))
f:write("123.45\nrest\n")
f:seek("set")
say(f:read("*n"))
say(f:read(0))
say(f:read("*l"))
f:seek("set")
say(f:read("*a"))
f:seek("set")
say(f:read(2))
-- setvbuf
say(f:setvbuf("no"))
say(f:setvbuf("full", 64))
say(f:setvbuf("line"))
f:write("z\n")
f:flush()
say(tostring(f):find("file") ~= nil)
f:close()
-- reopen closed
boom(function() f:read(1) end)
-- type
say(io.type(f))
say(io.type(io.stdin))
say(io.type(1))
-- input/output
io.output(n)
io.write("yy")
io.flush()
io.output(io.stdout)
io.input(n)
say(io.read(2))
io.input(io.stdin)
os.remove(n)
-- tmpfile
local tf = io.tmpfile()
if tf then tf:write("t"); tf:seek("set"); say(tf:read(1)); tf:close() end
return table.concat(out,"|")
`)
	_ = got
	// ioRename windows arm via hook
	old := ioIsWindowsFn
	ioIsWindowsFn = func() bool { return true }
	t.Cleanup(func() { ioIsWindowsFn = old })
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	_ = os.WriteFile(a, []byte("x"), 0o644)
	_ = ioRename(a, b)
}

func TestCoverGap14LuacStdinAndStrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "s.lua")
	_ = os.WriteFile(src, []byte("local x=1; return x, true, false, nil, 1.0"), 0o644)
	var out, errb bytes.Buffer
	_ = Luac([]string{"luac", "-s", "-o", filepath.Join(dir, "s.out"), src}, &out, &errb)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l", "-s", src}, &out, &errb)
	// listing binary
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l", filepath.Join(dir, "s.out")}, &out, &errb)
	// LOADKX trigger: many constants
	var b strings.Builder
	b.WriteString("return ")
	for i := 0; i < 300; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(strings.Repeat("z", 3))
		b.WriteByte('"')
	}
	src2 := filepath.Join(dir, "many.lua")
	_ = os.WriteFile(src2, []byte(b.String()), 0o644)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l", src2}, &out, &errb)
}

func TestCoverGap14BaseAndOS(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
say(select("#", 1,2,3))
say(rawget({a=1},"a"))
rawset(_G, "_cg14", 1); say(_cg14); _cg14=nil
say(rawequal(1,1))
say(rawlen("ab"))
say(tonumber("10", 2))
say(tonumber("ff", 16))
say(tostring(nil))
say(type(nil))
say(next({a=1}))
local t={1,2}; say(select(2, next(t)))
say(pcall(error, "e", 0))
say(xpcall(error, function(e) return "h" end, "e"))
say(assert(true, "x"))
boom(function() assert(false, "no") end)
say(load("return ...", nil, "t", {})(9))
local f = load(function() return nil end, "reader")
say(f==nil or type(f))
-- pairs with __pairs
local mt={__pairs=function(t) return next, {z=1}, nil end}
for k,v in pairs(setmetatable({}, mt)) do say(k) end
say(os.difftime(20,10))
say(os.date("!%Y", 0))
say(os.time({year=1970,month=1,day=2,hour=0,min=0,sec=0,isdst=false}) ~= nil)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap14GCAndTable(t *testing.T) {
	got := runChunk(t, `
collectgarbage("collect")
collectgarbage("step", 0)
collectgarbage("setpause", 100)
collectgarbage("setstepmul", 200)
local a={}
for i=1,50 do a[i]={} end
collectgarbage("collect")
local wt=setmetatable({}, {__mode="kv"})
wt[{}]= {}
collectgarbage("collect")
local u = newproxy and newproxy(true)
-- userdata finalizer
local ud
if debug and debug.setmetatable then
  -- skip
end
return collectgarbage("count") >= 0
`)
	_ = got
	L := NewState()
	defer L.Close()
	tbl := luaH_new(L)
	_ = allocsizenode(tbl)
	_ = allocsizenode(luaH_new(L))
	luaH_free(L, tbl)
}

func TestCoverGap14VMCompareShift(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
say(1.0 < 2)
say(2 > 1.0)
say(1.0 <= 1)
say(1 <= 1.0)
say(1.5 < 2.5)
say((-1)>>1)
say(1<<63)
say(3.0//2.0)
say((-5)//2)
say(1/0)
say(0/0 == 0/0)
-- concat metamethod chain length
local mt={__concat=function(a,b) return tostring(a)..tostring(b) end, __tostring=function(t) return "X" end}
local a=setmetatable({},mt)
say(a..a..a)
-- finishset via newindex returning
local t=setmetatable({}, {__newindex=function(t,k,v) rawset(t,k,v) end})
t.a=1
say(t.a)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap14CoroResumeArgs(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// too many args to resume: fill co stack
	co := L.NewThread()
	if err := co.LoadString(`return coroutine.yield()`, "=y"); err != nil {
		t.Fatal(err)
	}
	// first resume to start
	status := co.Resume(L, 0)
	_ = status
	co.reallocStack(ERRORSTACKSIZE)
	co.top = co.stackLast - 2
	for i := 0; i < 5; i++ {
		L.PushInteger(int64(i))
	}
	_ = coAuxResume(L, co, 5)
}

func TestCoverGap14LStateEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	var v Value
	setnilvalue(&v)
	_ = typeName(&v)
	setivalue(&v, 1)
	_ = typeName(&v)
	L.CheckStack(100)
	L.shrinkStack()
	freeUnusedCI(L)
	L2 := L.NewThread()
	_ = L2.Status()
	_ = L.ResetThread()
}
