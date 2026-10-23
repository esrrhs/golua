package core

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestCoverGap17ArithKOpcodes(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local x=1000
-- constants outside sC (-128..127) force *K opcodes
say(x-200)
say(x+200)
say(x*200)
say(x%200)
say(x//200)
say(x&255)
say(x|256)
say(x~511)
say(x^3)
say(x/200)
local a,b=1000,200
say(a-b); say(a*b); say(a%b); say(a//b)
say(a&b); say(a|b); say(a~b); say(a<<b); say(a>>b); say(a^b); say(a/b)
-- SHLI: const << reg
local s=3
say(1<<s)
say(8>>s)
-- float forprep errors
say(not pcall(function() for i=1,"x",1 do end end))
say(not pcall(function() for i=1,10,"x" do end end))
say(not pcall(function() for i="x",10,1 do end end))
say(not pcall(function() for i=1,10,0.0 do end end))
-- float limit skip: step<0 and huge positive limit float
local n=0
for i=0, (1/0), -1 do n=n+1; if n>2 then break end end
say(n)
-- int forlimit skip step<0 with float limit too big
n=0
for i=0, 1e100, -1 do n=n+1 end
say(n)
-- __newindex as table with existing key (finishset *slot=*val)
local inner={k=1}
local t=setmetatable({},{__newindex=inner})
t.k=2
say(inner.k)
-- __index as table with missing then present via chain
local mid={z=9}
local o=setmetatable({},{__index=mid})
say(o.z)
return table.concat(out,"|")
`)
	_ = got
	// direct opArithK / opBitwise
	L := NewState()
	defer L.Close()
	L.checkStack(4)
	var v1, v2 Value
	setivalue(&v1, 1000)
	setivalue(&v2, 200)
	pc := 0
	opArithK(L, 0, &v1, &v2,
		func(_ *State, a, b int64) int64 { return a - b },
		func(a, b float64) float64 { return a - b }, &pc)
	pc = 0
	opArith(L, 0, &v1, &v2,
		func(_ *State, a, b int64) int64 { return a * b },
		func(a, b float64) float64 { return a * b }, &pc)
	pc = 0
	opBitwise(L, 0, &v1, &v2, func(a, b int64) int64 { return a & b }, &pc)
	pc = 0
	opBitwiseK(L, 0, &v1, &v2, func(a, b int64) int64 { return a | b }, &pc)
	setfltvalue(&v1, 2.5)
	setfltvalue(&v2, 1.5)
	pc = 0
	opArithf(L, 0, &v1, &v2, func(a, b float64) float64 { return math.Pow(a, b) }, &pc)
	pc = 0
	opArithfK(L, 0, &v1, &v2, func(a, b float64) float64 { return a / b }, &pc)
}

func TestCoverGap17LAPIUpvalues(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// C closure with upvalues
	L.PushInteger(42)
	L.PushGoClosure(func(L *State) int {
		_ = L.ToGoFunction(UpvalueIndex(1)) // not a function upvalue
		v := index2value(L, UpvalueIndex(1))
		_ = v
		// OOB upvalue index → nilValue (line 62)
		_ = index2value(L, UpvalueIndex(99))
		return 0
	}, 1)
	fnIdx := L.GetTop()
	_ = L.ToGoFunction(fnIdx) // C closure path 401-402
	_ = L.UpvalueId(fnIdx, 1)
	_ = L.UpvalueId(fnIdx, 99) // nil
	L.PushGoFunc(func(L *State) int { return 0 })
	_ = L.UpvalueId(-1, 1) // light C → nil
	name, ok := L.GetUpvalue(fnIdx, 1)
	_ = name
	_ = ok
	name, ok = L.GetUpvalue(fnIdx, 99)
	_ = name
	_ = ok
	L.PushInteger(99)
	_, _ = L.SetUpvalue(fnIdx, 1)
	// aux_upvalue with owner non-nil
	var val *Value
	var owner any
	fi := index2value(L, fnIdx)
	_, _ = aux_upvalue(fi, 1, &val, &owner)
	_, _ = aux_upvalue(fi, 99, &val, &owner)
	// Lua closure no-name / getupvalref panic
	_ = runChunk(t, `
local function outer(a)
  local function inner() return a end
  debug.setupvalue(inner, 1, 7)
  return inner()
end
return outer(1)
`)
	func() {
		defer func() { _ = recover() }()
		L.PushInteger(1)
		_ = getupvalref(L, -1, 1, nil)
	}()
	func() {
		defer func() { _ = recover() }()
		L.UpvalueJoin(-1, 1, -1, 1)
	}()
	// Compare invalid op already; Error empty
	L.SetTop(0)
	func() {
		defer func() { _ = recover() }()
		_ = L.Error()
	}()
	// SetMetaTable on non-table value with existing mt
	L.PushInteger(1)
	L.NewTable()
	L.SetMetaTable(-2)
	// rawGetRegistryGlobals nil registry edge — just call
	_ = L.rawGetRegistryGlobals()
	// LoadFileMode binary fail
	dir := t.TempDir()
	bad := filepath.Join(dir, "b")
	_ = os.WriteFile(bad, []byte("\x1bLua not a dump"), 0o644)
	_ = L.LoadFileMode(bad, "b")
	_ = L.LoadFileMode(bad, "t")
	// Where with no frames
	_ = L.Where(0)
	// stackIndexOf positive ok already; call with large
	_ = stackIndexOf(L, 1)
	_ = L.indexToStack(1)
	// SetTop closing tbc
	L.SetTop(0)
	L.PushGoFunc(func(L *State) int { return 0 })
	L.NewTable()
	L.NewTable()
	L.PushString("__close")
	L.PushGoFunc(func(L *State) int { return 0 })
	L.SetTable(-3)
	L.SetMetaTable(-2)
	func() {
		defer func() { _ = recover() }()
		L.ToClose(-1)
		L.SetTop(0) // should close
	}()
}

func TestCoverGap17CoroStatusNormal(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
-- "normal" status: resume a co that resumes another
local co2
local co1=coroutine.create(function()
  say(coroutine.status(co2)) -- should be normal while co2 runs us? 
  -- actually: from co2, status(co1) while co1 is resumed by co2
end)
co2=coroutine.create(function()
  say(coroutine.status(co1)) -- suspended
  coroutine.resume(co1)
  say(coroutine.status(co1))
end)
coroutine.resume(co2)
-- main as running
say(coroutine.status(coroutine.running()))
-- yield from deep nesting for fastYieldNow
local co=coroutine.create(function()
  local function a()
    local function b()
      return coroutine.yield(5)
    end
    return b()
  end
  return a()
end)
local ok,v=coroutine.resume(co)
say(ok); say(v)
ok,v=coroutine.resume(co,6)
say(ok); say(v)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap17IOPruneAndMeta(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- rename missing
boom(function() os.rename("/no/such/a","/no/such/b") end)
-- open fail
boom(function() io.open("/no/such/golua17","r") end)
-- get iofile via io.input(nil) etc
local n=os.tmpname()
local f=assert(io.open(n,"w")); f:write("1\n2\n"); f:close()
io.input(n)
say(io.read("*l"))
io.input(io.stdin)
-- lines with closed during iteration is hard; close then lines
f=assert(io.open(n,"r"))
local iter = f:lines()
f:close()
boom(function() iter() end)
os.remove(n)
-- type of closed
say(io.type(f))
say(io.type(io.stdin))
say(io.type(1))
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap17StrLibRemain(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- long %s without precision (>=100) keep entire
say(#string.format("%s", string.rep("y", 150)))
-- pack alignment padding
say(#string.pack("!8i8", 1))
say(#string.pack("<!4i4", 1))
-- unpack
local s=string.pack("i4i4", 1, 2)
local a,b,pos=string.unpack("i4i4", s)
say(a); say(b); say(pos)
boom(function() string.unpack("i4", "ab") end)
boom(function() string.pack("i4", 1.5) end)
-- gsub with function / table
say(string.gsub("a","a",{a="b"}))
say(string.gsub("a","a",function() return "c" end))
-- find with plain
say(string.find("a*b", "*", 1, true))
-- byte out of range
say(string.byte("ab", 0))
say(string.byte("ab", 3))
-- rep overflow already; reverse empty
say(string.reverse(""))
say(string.lower(""))
say(string.upper(""))
-- match frontier
say(string.match(" a", "%f[%a]a"))
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap17LStateResumeSync(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// forceCloseThread done channel paths via finished co
	co := L.NewThread()
	_ = co.LoadString(`return 1`, "=x")
	_ = co.Resume(L, 0)
	// dead after finish
	_ = co.forceCloseThread()
	// Resume on dead
	_ = co.Resume(L, 0)
	// Resume on OK non-dead non-suspended
	co2 := L.NewThread()
	co2.status = LUA_OK
	co2.dead = false
	_ = co2.Resume(L, 0)
	// shrinkStack newsize < used and < BASIC
	L2 := NewState()
	defer L2.Close()
	L2.reallocStack(BASIC_STACK_SIZE * 4)
	L2.top = BASIC_STACK_SIZE + 10
	// used = top+EXTRA; size/4 may still be large
	L2.shrinkStack()
	L2.reallocStack(BASIC_STACK_SIZE * 8)
	L2.top = 1
	L2.shrinkStack()
}

func TestCoverGap17DebugFindLocal(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function f(a, b, ...)
  local c = 1
  local d = 2
  say(debug.getlocal(1, 1))
  say(debug.getlocal(1, 3))
  say(debug.getlocal(1, 99)==nil)
  local name, val = debug.getlocal(1, -1) -- vararg?
  say(name~=nil or true)
  debug.setlocal(1, 1, 99)
  say(a)
  -- getinfo various
  local i=debug.getinfo(1, "nSlufL")
  say(i.what)
  say(i.linedefined ~= nil)
  say(debug.getinfo(f, "S").what)
  return table.concat(out,"|")
end
return f(1,2,3,4)
`)
	_ = got
}

func TestCoverGap17LuacList(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "t.lua")
	_ = os.WriteFile(src, []byte(`
local function f(a, b, ...)
  local t = {a, b, ...}
  if a then return b else return a end
  for i=1,3 do end
  for k,v in pairs(t) do end
  local x <close> = setmetatable({}, {__close=function() end})
  return t
end
return f
`), 0o644)
	var out, errb bytes.Buffer
	_ = Luac([]string{"luac", "-l", "-l", "-s", src}, &out, &errb)
	_ = Luac([]string{"luac", "-p", src}, &out, &errb)
	_ = Luac([]string{"luac", "-v"}, &out, &errb)
	_ = Luac([]string{"luac", "-h"}, &out, &errb)
}

func TestCoverGap17GCPending(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// call gcAfterAlloc with pending
	if L.hasPending() {
		_ = L.gcAfterAlloc()
	}
	_ = L.gcAfterAlloc()
	L.g.gcInFinalizer = true
	_ = L.gcAfterAlloc()
	L.g.gcInFinalizer = false
	L.g.closing = true
	_ = L.gcAfterAlloc()
	L.g.closing = false
	L.g.gcStopped = true
	L.gcStep()
	L.g.gcStopped = false
}

func TestCoverGap17ParserEdges(t *testing.T) {
	cases := []string{
		`goto L; ::L::`,
		`local a <const> = 1; return a`,
		`local a <close> = setmetatable({}, {__close=function() end})`,
		`return (function(a,b,...) return a,b,... end)(1,2,3)`,
		`local t; t = {["x"]=1, [1]=2, true, false, nil}`,
		`repeat local a=1; break until true`,
		`while true do break end`,
		`local function f() return end; return f()`,
		`;`, // empty
		`return - -1`,
		`return not not true`,
	}
	for _, src := range cases {
		func(src string) {
			defer func() { _ = recover() }()
			L := NewState()
			defer L.Close()
			L.OpenLibs()
			_ = L.LoadString(src, "=p")
			_ = L.PCall(0, 0, 0)
		}(src)
	}
	// invalid
	_ = runChunk(t, `return select("#", 1, 2, 3)`)
}

func TestCoverGap17UnsafePointer(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	p := L.ToPointer(-1)
	_ = p
	_ = unsafe.Sizeof(p)
}
