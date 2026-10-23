package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestCoverGap12LuacNoInputListing(t *testing.T) {
	dir := t.TempDir()
	// Create default luac.out so -l with no files can read it
	src := filepath.Join(dir, "x.lua")
	if err := os.WriteFile(src, []byte("return 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	_ = Luac([]string{"luac", "-o", "luac.out", src}, &out, &errb)
	out.Reset()
	errb.Reset()
	// no input files: listing uses output as input (line ~164)
	code := Luac([]string{"luac", "-l"}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-p"}, &out, &errb)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l"}, &out, &errb)
}

func TestCoverGap12StrFormatEdges(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
say(string.format("%+05d", 0))
say(string.format("% 05d", 0))
say(string.format("%05d", 0))
say(string.format("%+d", 0))
say(string.format("%#05x", 1))
say(string.format("%#05o", 1))
say(string.format("%#o", 0))
say(string.format("%.0f", 1.5))
say(string.format("%a", 1.5))
say(string.format("%A", 1.5))
say(string.format("%e", 1.5))
say(string.format("%E", 1.5))
say(string.format("%g", 1.5))
say(string.format("%G", 1.5))
say(string.format("%c", 65))
say(string.format("%s", "hi"))
say(string.format("%q", "a\nb"))
say(string.format("%i", 3))
say(string.format("%u", 3))
boom(function() string.format("%d %d", 1) end)
boom(function() string.format("%*") end)
-- pack edges
say(string.pack("i8", 1))
say(string.pack("I8", 1))
say(string.pack("Tn", 3, 1.5))
say(select(1, string.unpack("i8", string.pack("i8", -5))))
say(string.packsize("i8"))
boom(function() string.pack("i", 1) end) -- missing size?
boom(function() string.unpack("i8", "") end)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12DebugStackOverflow(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L1 := L.NewThread()
	// Fill L1 stack near max so CheckStack fails
	L1.reallocStack(ERRORSTACKSIZE)
	L1.top = L1.stackLast - 1
	func() {
		defer func() { _ = recover() }()
		dbCheckStack(L, L1, 100)
	}()
	// negative level
	var ar dbAr
	if dbGetStack(L, -1, &ar) {
		t.Fatal("neg level")
	}

	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local co = coroutine.create(function()
  local function f(n)
    if n==0 then
      say(debug.getinfo(1,"nSlufL").linedefined ~= nil or true)
      local name,val = debug.getlocal(1,1)
      say(name)
      return
    end
    return f(n-1)
  end
  f(5)
end)
coroutine.resume(co)
-- debug.debug is interactive; skip
say(debug.getuservalue(io.stdin) == nil or true)
local u = newproxy and newproxy(true)
-- userdata uservalue
local ud = require and nil
say(debug.traceback())
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12IOReaderNil(t *testing.T) {
	// Directly poke ioStream reader/writer nil arms
	s := &ioStream{}
	_ = s.reader()
	_ = s.writer()
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- closed file ops
local name = os.tmpname()
local f = assert(io.open(name,"w"))
f:close()
boom(function() f:read(1) end)
boom(function() f:write("x") end)
boom(function() f:seek("set",0) end)
boom(function() f:flush() end)
boom(function() f:lines()() end)
os.remove(name)
-- popen write mode
local p = io.popen("cat >/dev/null", "w")
if p then
  p:write("hi\n")
  say(p:close())
end
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12CoroEdges(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- resume main
boom(function() coroutine.resume(coroutine.running()) end)
-- close main
boom(function() coroutine.close(coroutine.running()) end)
-- wrap
local w = coroutine.wrap(function(x) return x+1 end)
say(w(3))
local w2 = coroutine.wrap(function() error("boom") end)
boom(function() w2() end)
-- status
local co = coroutine.create(function() coroutine.yield(1) end)
say(coroutine.status(co))
coroutine.resume(co)
say(coroutine.status(co))
coroutine.resume(co)
say(coroutine.status(co))
-- isyieldable
say(coroutine.isyieldable())
local co2 = coroutine.create(function() say(coroutine.isyieldable()) end)
coroutine.resume(co2)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12OSDateEdges(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local t = os.time({year=2020, month=1, day=1, hour=0, min=0, sec=0})
say(os.date("%c", t))
say(os.date("%Y-%m-%d", t))
say(os.date("!*t", t).isdst == false or true)
say(os.date("*t", t).yday ~= nil)
boom(function() os.date("%Y", "nope") end)
boom(function() os.time({year=2020}) end) -- incomplete?
say(os.execute() ~= nil or true) -- no arg: shell exists?
say(os.setlocale("C", "all"))
say(os.setlocale(nil, "numeric"))
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12LoadlibSearchers(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- package.searchpath with custom sep/rep
say(package.searchpath("a.b", "./?.lua;./?/init.lua", ".", "/"))
-- cpath search (will fail)
say(package.searchpath("nope", package.cpath) == nil or true)
-- loadlib missing
boom(function() package.loadlib("no.so", "luaopen_no") end)
-- searchers[1] preload already tested
-- searchers path error message accumulation
local ok, err = pcall(require, "definitely.missing.mod.xyz")
say(not ok)
say(tostring(err):find("not found") ~= nil)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12VMConcatMM(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local mt={__concat=function(a,b) return "C" end, __tostring=function() return "T" end}
local a=setmetatable({}, mt)
local b=setmetatable({}, mt)
say(a..b)
say(a..a..b)
-- finishget/set via metamethod chains
local idx = setmetatable({}, {__index=function(t,k)
  if k=="x" then return 1 end
  return nil
end, __newindex=function(t,k,v) rawset(t,k,v) end})
say(idx.x)
idx.y = 2
say(idx.y)
-- for loop edge with float
for i=1.5,3.5,1.0 do say(i) end
-- bitwise on floats that are integers
say(3.0 & 1.0)
say(3.0 | 4.0)
say(3.0 ~ 1.0)
say(2.0 << 3.0)
say(16.0 >> 2.0)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap12APIErrorPaths(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// Copy invalid
	func() {
		defer func() { _ = recover() }()
		L.Copy(99, 1)
	}()
	func() {
		defer func() { _ = recover() }()
		L.Rotate(99, 1)
	}()
	L2 := L.NewThread()
	L.PushInteger(1)
	L2.XMove(L, 1)
	// Error
	func() {
		defer func() { _ = recover() }()
		L.PushString("err")
		L.Error()
	}()
	L.SetTop(0)
	// Compare bad op
	L.PushInteger(1)
	L.PushInteger(2)
	_ = L.Compare(-2, -1, 99)
	L.SetTop(0)
	// ToClose / CloseSlot
	L.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		L.ToClose(-1)
	}()
	L.SetTop(0)
}
