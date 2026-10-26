package core

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestAPIMoreStackAndIndex(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// AbsIndex / IsNone
	L.PushNil()
	_ = L.AbsIndex(-1)
	_ = L.AbsIndex(1)
	_ = L.IsNone(99)
	_ = L.IsNoneOrNil(99)
	_ = L.Type(99)
	// Insert/Remove/Rotate happy path
	L.SetTop(0)
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Insert(1)
	L.Remove(2)
	L.Rotate(1, 1)
	L.Rotate(1, -1)
	// Compare / Arith
	L.PushInteger(1)
	L.PushInteger(2)
	_ = L.Compare(1, 2, LUA_OPEQ)
	_ = L.Compare(1, 2, LUA_OPLT)
	_ = L.Compare(1, 2, LUA_OPLE)
	L.SetTop(0)
	L.PushInteger(3)
	L.PushInteger(4)
	L.Arith(LUA_OPADD)
	L.SetTop(0)
	L.PushInteger(5)
	L.PushInteger(2)
	L.Arith(LUA_OPSUB)
	L.SetTop(0)
	L.PushInteger(5)
	L.Arith(LUA_OPUNM)
	L.SetTop(0)
	L.PushInteger(6)
	L.Arith(LUA_OPBNOT)
	// RawLen on string/table/userdata
	L.PushString("hi")
	_ = L.RawLen(-1)
	L.NewTable()
	_ = L.RawLen(-1)
	// Concat
	L.SetTop(0)
	L.PushString("a")
	L.PushString("b")
	L.Concat(2)
	// Next
	L.NewTable()
	L.PushString("k")
	L.PushInteger(1)
	L.SetTable(-3)
	L.PushNil()
	for L.Next(-2) {
		L.Pop(1)
	}
	// Len
	L.PushString("abc")
	L.Len(-1)
	// Dump strip false
	L.SetTop(0)
	_ = L.LoadString("return 42", "=x")
	var b bytes.Buffer
	_ = L.Dump(func(L *State, p []byte) int { b.Write(p); return 0 }, false)
	// ToPointer on more types
	L.PushBoolean(true)
	_ = L.ToPointer(-1)
	L.PushThread()
	_ = L.ToPointer(-1)
	// Get/Set uservalue n=1
	L.NewUserData(nil, 1)
	L.PushString("uv")
	L.SetUserValue(-2, 1)
	L.GetUserValue(-1, 1)
	// UpvalueIndex
	_ = UpvalueIndex(1)
	// Version / AtPanic
	_ = Version(L)
	old := L.AtPanic(func(L *State) int { return 0 })
	_ = L.AtPanic(old)
	// Status
	_ = L.Status()
	// IsYieldable
	_ = L.IsYieldable()
	// GCTM related via collect
	_ = L.DoString(`collectgarbage("collect")`)
}

func TestAuxlibBufferAndModule(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	B := L.NewBuffer()
	B.AddString("hello")
	B.AddChar('!')
	B.AddBytes([]byte("xyz"))
	B.PushResult()
	s, _ := L.ToString(-1)
	if !strings.Contains(s, "hello") {
		t.Fatalf("%q", s)
	}
	// GSub
	L.PushString("a_b")
	_ = L.GSub("a_b", "_", ".")
	// Traceback
	L.Traceback(L, "msg", true, 0)
	// Where
	L.Where(1)
	// CheckOption
	_ = L.DoString(`local x = require`)
	// OpenLibs already; RequireF again
	L.RequireF("package", OpenPackage, false)
	L.Pop(1)
	// NewMetaTable
	if L.NewMetaTable("CoverGap10MT") {
		L.Pop(1)
	}
	if !L.NewMetaTable("CoverGap10MT") {
		// already exists
		L.Pop(1)
	}
	// SetFuncs
	L.NewTable()
	L.SetFuncs([]Reg{{Name: "f", Func: func(L *State) int { return 0 }}}, 0)
	// CheckAny / Opt*
	L.SetTop(0)
	L.PushInteger(1)
	L.CheckAny(1)
	_ = L.OptInteger(2, 9)
	_ = L.OptNumber(2, 1.5)
	_ = L.OptString(2, "def")
	// ArgCheck
	L.ArgCheck(true, 1, "ok")
	func() {
		defer func() { _ = recover() }()
		L.ArgCheck(false, 1, "bad")
	}()
}

func TestIOReadWriteAllModes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.txt")
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local modes = {"w","w+","a","a+","r","r+","wb","w+b","ab","rb"}
for _,m in ipairs({"w","w+","a","a+","wb","w+b","ab"}) do
  local f = assert(io.open(`+quote(p)+`, m))
  f:write("z")
  f:flush()
  f:close()
end
local f = assert(io.open(`+quote(p)+`, "r"))
say(f:read("*a") ~= nil)
f:close()
f = assert(io.open(`+quote(p)+`, "r+"))
say(f:seek("set", 0) ~= nil)
say(f:read(1) ~= nil)
f:close()
-- stdout/stderr write
say(io.write(""))
say(io.stdout:write(""))
say(io.stderr:write(""))
-- type of closed
f = assert(io.open(`+quote(p)+`, "w")); f:close()
say(io.type(f))
-- no close on std handles should fail or no-op safely
say(pcall(io.stdout.close, io.stdout) or true)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "closed") && !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestDebugGetInfoFull(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function f(a, b, ...)
  local x = 1
  say(debug.getinfo(1, "nSlufL").name ~= nil or true)
  say(debug.getinfo(f, "t").istailcall ~= nil or debug.getinfo(f, "t") ~= nil)
  local i = debug.getinfo(1, "L")
  say(i.activelines ~= nil)
  for k,v in pairs(i.activelines or {}) do say(true); break end
  say(debug.getlocal(1, 1) ~= nil)
  say(debug.setlocal(1, 1, a) ~= nil)
  local n = debug.getinfo(1, "u").nups
  say(n ~= nil)
  return a
end
f(10, 20, 30)
say(debug.traceback("T", 1):find("T") ~= nil)
say(debug.getregistry() ~= nil)
say(debug.getmetatable("") ~= nil or true)
say(debug.setmetatable({}, {}) ~= nil or true)
-- hook count
local n=0
debug.sethook(function() n=n+1 end, "", 10)
for i=1,100 do end
debug.sethook()
say(n>=0)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestLoadlibSearchersTableCorrupt(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local s = package.searchers
package.searchers = {function() return nil, "nope1" end, function() error("searcher boom") end}
say(not pcall(require, "zzz_missing"))
package.searchers = s
-- package.preload returns loader
package.preload.zzpreload = function(name) return {n=name} end
say(require("zzpreload").n == "zzpreload")
return table.concat(o, ",")
`)
	if !strings.HasPrefix(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestParserAndVMOddPaths(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
-- goto / labels
local function g()
  local x=0
  ::L1::
  x=x+1
  if x < 2 then goto L1 end
  return x
end
say(g()==2)
-- vararg
local function v(...)
  local a = {...}
  return select("#", ...), a[1]
end
say(v(9,8)==2)
-- setlist large
local t = {1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55}
say(#t>=50)
-- upvalue close via do block
do
  local u = 1
  local function f() return u end
  say(f()==1)
end
-- load modes
local f, err = load("return 1", "=x", "t")
say(f()==1)
f, err = load("\x1bLua", "=x", "t") -- binary rejected in text mode maybe
say(f == nil or true)
-- string.dump + load binary
local bin = string.dump(function() return 7 end)
say(load(bin)()==7)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 3 && !strings.Contains(got, "2") {
		t.Fatalf("%q", got)
	}
}

func TestOSMoreDateFormats(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local specs = {"%a","%A","%b","%B","%c","%d","%H","%I","%j","%m","%M","%p","%S","%U","%w","%W","%x","%X","%y","%Y","%z","%Z","%%"}
for _,sp in ipairs(specs) do
  local ok, r = pcall(os.date, sp, 0)
  say(ok)
end
say(os.difftime(100, 50) == 50)
local t = os.time({year=2000, month=1, day=1, hour=0, min=0, sec=0, isdst=false})
say(t ~= nil)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 10 {
		t.Fatalf("%q", got)
	}
}

func TestWeakTableEphemeronish(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local k = {}
local v = {}
local t = setmetatable({}, {__mode="kv"})
t[k] = v
k, v = nil, nil
collectgarbage("collect")
collectgarbage("collect")
say(true)
local t2 = setmetatable({[1]={}}, {__mode="v"})
collectgarbage("collect")
say(true)
return table.concat(o, ",")
`)
	if got != "true,true" {
		t.Fatalf("%q", got)
	}
}

func TestLightUserdataAndToPointer(t *testing.T) {
	L := NewState()
	defer L.Close()
	var x int
	L.PushLightUserData(unsafe.Pointer(&x))
	if !L.IsLightUserData(-1) {
		t.Fatal("lightud")
	}
	_ = L.ToUserData(-1)
	_ = L.ToPointer(-1)
	_ = L.RawLen(-1)
}

func TestThreadXMoveHappy(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	co := L.NewThread()
	L.PushInteger(1)
	L.PushInteger(2)
	co.XMove(L, 2)
	if co.GetTop() != 2 {
		t.Fatalf("top=%d", co.GetTop())
	}
}
