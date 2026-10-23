package core

import (
	"bufio"
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverGap16VMHelpers(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.checkStack(8)
	// idiv special: n == -1
	func() {
		defer func() { _ = recover() }()
		_ = luaV_idiv(L, math.MinInt64, -1)
	}()
	_ = luaV_idiv(L, 7, -1)
	_ = luaV_idiv(L, -7, -1)
	// shift extremes
	_ = luaV_shiftl(1, -64)
	_ = luaV_shiftl(1, -65)
	_ = luaV_shiftl(1, 64)
	_ = luaV_shiftl(1, 63)
	_ = luaV_shiftl(-1, -64)
	// concat total==1 early return
	L.PushString("only")
	luaV_concat(L, 1)
	// concat overflow arm (force via huge strings) — skip alloc; call path with TM
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
-- idiv / shifts / arith K forms
local x=10
say(x-1)
say(x-100)
say(x*2)
say(x%3)
say(x//3)
say(x&3)
say(x|3)
say(x~3)
say(x<<2)
say(x>>2)
say(2<<x)
say(1<<63)
say((-1)>>1)
say(x^2)
say(x/2)
say(2.5-1)
say(2.5*2)
-- large array constructor -> EXTRAARG on NEWTABLE/SETLIST
local t={}
for i=1,300 do t[i]=i end
say(#t)
local u={1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,
21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,
41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,
61,62,63,64,65,66,67,68,69,70,71,72,73,74,75,76,77,78,79,80,
81,82,83,84,85,86,87,88,89,90,91,92,93,94,95,96,97,98,99,100,
101,102,103,104,105,106,107,108,109,110,111,112,113,114,115,116,117,118,119,120,
121,122,123,124,125,126,127,128,129,130,131,132,133,134,135,136,137,138,139,140,
141,142,143,144,145,146,147,148,149,150,151,152,153,154,155,156,157,158,159,160,
161,162,163,164,165,166,167,168,169,170,171,172,173,174,175,176,177,178,179,180,
181,182,183,184,185,186,187,188,189,190,191,192,193,194,195,196,197,198,199,200,
201,202,203,204,205,206,207,208,209,210,211,212,213,214,215,216,217,218,219,220,
221,222,223,224,225,226,227,228,229,230,231,232,233,234,235,236,237,238,239,240,
241,242,243,244,245,246,247,248,249,250,251,252,253,254,255,256,257,258,259,260}
say(#u)
-- generic for hits TFORCALL
local s=0
for k,v in pairs({a=1,b=2}) do s=s+v end
say(s)
for i,v in ipairs({10,20,30}) do s=s+v end
say(s)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16IndexNewindexChain(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
-- __index chain too long
local a={}
local b={}
setmetatable(a,{__index=b})
setmetatable(b,{__index=a})
say(not pcall(function() return a.x end))
-- __newindex chain too long
local c,d={},{}
setmetatable(c,{__newindex=d})
setmetatable(d,{__newindex=c})
say(not pcall(function() c.x=1 end))
-- finishset when slot already nonempty via metamethod table
local e=setmetatable({},{__newindex=function(t,k,v) rawset(t,k,v) end})
e.z=9
e.z=10
say(e.z)
-- index into non-table via __index that is a table with value
local f=setmetatable({},{__index={q=42}})
say(f.q)
-- vmFastGet non-table
local g=setmetatable({},{__index=function() return 1 end})
say(g.x)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16ChangedLine(t *testing.T) {
	p := &Proto{}
	_ = changedLine(p, 0, 1)  // nil lineinfo
	_ = changedLine(p, -1, 0) // oldpc < 0
	_ = getFuncline(p, 0)
	_ = getFuncline(p, -1)
	p.lineinfo = []int32{1, 1, 2}
	_ = changedLine(p, -1, 2)
	_ = changedLine(p, 0, 2)
	_ = getFuncline(p, 0)
	_ = getFuncline(p, 99)
}

func TestCoverGap16RemoveValues(t *testing.T) {
	fs := &FuncState{f: &Proto{}}
	// TESTSET at 0, JMP at 1 ending the list (offset == NO_JUMP)
	fs.f.code = []Instruction{
		newABC(OP_TESTSET, 0, 1, 0, 0),
		{Op: OP_JMP, B: NO_JUMP + OFFSET_sJ},
	}
	fs.pc = 2
	removevalues(fs, 1)
	// also a non-TESTSET controlling instr
	fs.f.code = []Instruction{
		newABC(OP_TEST, 0, 0, 0, 0),
		{Op: OP_JMP, B: NO_JUMP + OFFSET_sJ},
	}
	removevalues(fs, 1)
	// Lua that should exercise codenot → removevalues
	_ = runChunk(t, `
local a,b = true, false
return not (a and b), not (a or b), (a and b) and 1 or 2
`)
}

func TestCoverGap16SettableFast(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewTable()
	L.PushString("k")
	L.PushInteger(1)
	L.SetTable(-3) // insert
	L.PushString("k")
	L.PushInteger(2)
	L.SetTable(-3) // fast path overwrite existing
	// direct luaV_settable
	L.PushString("k")
	L.PushInteger(3)
	tval := index2value(L, 1)
	luaV_settable(L, tval, L.top-2, L.top-1)
	L.top -= 2
	// gettable panic on non-table
	func() {
		defer func() { _ = recover() }()
		L.PushInteger(1)
		_ = gettable(L, -1)
	}()
}

func TestCoverGap16IONilReader(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.checkStack(8)
	s := &ioStream{canR: true} // f/rd nil → reader()==nil
	ioReadAll(L, s)
	_ = ioReadChars(L, s, 5)
	_ = ioTestEOF(L, s)
	_ = ioReadNumber(L, s)
	// negative / huge n
	func() {
		defer func() { _ = recover() }()
		_ = ioReadChars(L, s, -1)
	}()
	func() {
		defer func() { _ = recover() }()
		_ = ioReadChars(L, &ioStream{canR: true, rd: io.NopCloser(strings.NewReader("abc"))}, 1<<31)
	}()
	// empty reader with real stream
	s2 := &ioStream{canR: true, rd: io.NopCloser(strings.NewReader(""))}
	_ = ioTestEOF(L, s2)
	_ = ioReadChars(L, s2, 3)
	ioReadAll(L, s2)
	// number reader overflow / empty
	br := bufio.NewReader(strings.NewReader(""))
	rn := ioNewNumReader(br)
	_ = rn
	br2 := bufio.NewReader(strings.NewReader(strings.Repeat("9", ioMaxLenNum+5)))
	rn2 := ioNewNumReader(br2)
	for i := 0; i < ioMaxLenNum+2; i++ {
		if !rn2.next() {
			break
		}
	}
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local n=os.tmpname()
local f=assert(io.open(n,"w")); f:write("1.5\n"); f:close()
f=assert(io.open(n,"r"))
say(f:read("*n"))
f:close()
f=assert(io.open(n,"r"))
boom(function() f:read(-1) end)
f:close()
-- closed file read
f=assert(io.open(n,"r")); f:close()
boom(function() f:read(1) end)
boom(function() f:lines()() end)
-- io.lines() with default input
local old=io.input()
io.input(n)
for l in io.lines() do say(l); break end
io.input(old)
os.remove(n)
-- popen close
local p=io.popen("true","r")
if p then say(p:close()) end
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16StrFormatPack(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
say(string.format("%p", {}))
say(string.format("%p", nil))
say(string.format("%10p", print))
boom(function() string.format("%10q", "a") end)
-- %s with zeros and long
boom(function() string.format("%5s", "a\0b") end)
say(string.format("%s", string.rep("x", 120)))
say(string.format("%20s", "hi"))
-- pack edges
boom(function() string.pack("c") end)
boom(function() string.pack("Z") end)
boom(function() string.pack("X") end)
boom(function() string.pack("Xy") end)
say(#string.pack("!4i4", 1))
say(#string.pack("c100000", "ab")) -- pad path (chunked)
-- match maxexpand fail
say(string.match("aaa", "a*b") == nil)
say(string.find("abc", "x*") ~= nil)
-- capture position that fails as text
say(string.match("ab", "()a()%1") == nil or true)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16OsClockHook(t *testing.T) {
	old := osCPUTimeFn
	t.Cleanup(func() { osCPUTimeFn = old })
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	osCPUTimeFn = func() (float64, bool) { return 0, false }
	_ = runChunk(t, `return os.clock()`)
	osCPUTimeFn = func() (float64, bool) { return 1.25, true }
	_ = runChunk(t, `return os.clock()`)
	// DST helpers / setboolfield undefined
	osSetBoolField(L, "isdst", -1)
	L.NewTable()
	osSetAllFields(L, osStartTime)
	_ = tIsDST(osStartTime)
	_ = tHasDSTRules(osStartTime)
}

func TestCoverGap16CoroStackFullResults(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	co := L.NewThread()
	if err := co.LoadString(`return coroutine.yield(1,2,3)`, "=y"); err != nil {
		t.Fatal(err)
	}
	st := co.Resume(L, 0)
	if st != LUA_YIELD {
		t.Fatalf("want yield got %d", st)
	}
	// fill L so CheckStack(nres+1) fails
	L.reallocStack(ERRORSTACKSIZE)
	// Leave 2 free slots so PushString of the error can succeed, but
	// CheckStack(nres+1) with nres=3 still fails (need 4 free).
	L.top = L.stackLast - 2
	func() {
		defer func() { _ = recover() }()
		_ = coAuxResume(L, co, 0)
	}()

	L2 := NewState()
	defer L2.Close()
	L2.OpenLibs()
	// sync close with pending to-be-closed that succeeds
	co2 := L2.NewThread()
	if err := co2.LoadString(`
local t <close> = setmetatable({}, {__close=function() end})
coroutine.yield(1)
`, "=c"); err != nil {
		t.Fatal(err)
	}
	_ = co2.Resume(L2, 0)
	if co2.co != nil {
		co2.co.sync = true
		co2.co.live = false
	}
	func() {
		defer func() { _ = recover() }()
		ok, ev := coCloseThread(co2, L2)
		_ = ok
		_ = ev
	}()

	co3 := L2.NewThread()
	co3.dead = true
	co3.hasErr = true
	setsvalue(&co3.lastErr, luaS_newlstr(L2, "prev"))
	co3.checkStack(2)
	setnilvalue(&co3.stack[0])
	co3.top = 1
	co3.status = LUA_ERRRUN
	func() {
		defer func() { _ = recover() }()
		ok, ev := coCloseThread(co3, L2)
		_ = ok
		_ = ev
	}()

	co4 := L2.NewThread()
	co4.dead = true
	co4.hasErr = true
	setnilvalue(&co4.lastErr)
	co4.checkStack(2)
	setnilvalue(&co4.stack[0])
	co4.top = 1
	co4.status = LUA_ERRRUN
	func() {
		defer func() { _ = recover() }()
		ok, ev := coCloseThread(co4, nil)
		_ = ok
		_ = ev
	}()

	co5 := L2.NewThread()
	if co5.co != nil {
		co5.co.sync = true
		co5.co.live = false
		co5.co.vals = nil
	}
	co5.status = LUA_ERRRUN
	co5.checkStack(2)
	setnilvalue(&co5.stack[0])
	co5.top = 1
	func() {
		defer func() { _ = recover() }()
		ok, ev := coCloseThread(co5, L2)
		_ = ok
		_ = ev
	}()

	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
-- wrap with error → auxwrap close path
local w=coroutine.wrap(function()
  local t <close> = setmetatable({}, {__close=function() end})
  error("e")
end)
say(not pcall(w))
-- status of initial / normal
local co=coroutine.create(function() coroutine.yield() end)
say(coroutine.status(co))
coroutine.resume(co)
say(coroutine.status(co))
-- yield from nested Lua (vmNest>1) sync path
local co2=coroutine.create(function()
  local function inner() return coroutine.yield(9) end
  return inner()
end)
local ok,v=coroutine.resume(co2)
say(ok); say(v)
ok,v=coroutine.resume(co2, 8)
say(ok); say(v)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16LAPIEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// Rotate invalid n / pseudo index
	L.PushInteger(1)
	L.PushInteger(2)
	func() {
		defer func() { _ = recover() }()
		L.Rotate(1, 99)
	}()
	func() {
		defer func() { _ = recover() }()
		L.Rotate(LUA_REGISTRYINDEX, 1)
	}()
	func() {
		defer func() { _ = recover() }()
		L.Rotate(1, -99)
	}()
	// XMove different G
	L2 := NewState()
	defer L2.Close()
	L2.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		L.XMove(L2, 1)
	}()
	// ToGoFunction on light C and closure
	L.PushGoFunc(func(L *State) int { return 0 })
	_ = L.ToGoFunction(-1)
	L.PushGoClosure(func(L *State) int { return 0 }, 0)
	_ = L.ToGoFunction(-1)
	_ = L.ToGoFunction(1) // not a function
	// NewUserData invalid nuvalue
	func() {
		defer func() { _ = recover() }()
		L.NewUserData(nil, -1)
	}()
	// ToClose / CloseSlot pseudo
	func() {
		defer func() { _ = recover() }()
		L.ToClose(LUA_REGISTRYINDEX)
	}()
	func() {
		defer func() { _ = recover() }()
		L.CloseSlot(LUA_REGISTRYINDEX)
	}()
	// index2value C closure upvalue OOB
	L.PushGoClosure(func(L *State) int {
		_ = index2value(L, UpvalueIndex(99))
		return 0
	}, 0)
	L.Call(0, 0)
	// stackIndexOf / indexToStack idx==0 panic
	func() {
		defer func() { _ = recover() }()
		_ = stackIndexOf(L, 0)
	}()
	func() {
		defer func() { _ = recover() }()
		_ = L.indexToStack(0)
	}()
	// pcallK yieldable with error
	co := L.NewThread()
	co.PushGoFunc(func(L *State) int {
		L.PushGoFunc(func(L *State) int {
			return L.Error2("boom")
		})
		return L.pcallK(0, 0, 0, true)
	})
	_ = co.Resume(L, 0)
	// Error() with nothing / Compare bad
	L.SetTop(0)
	L.PushString("e")
	func() {
		defer func() { _ = recover() }()
		_ = L.Error()
	}()
	L.PushInteger(1)
	L.PushInteger(2)
	func() {
		defer func() { _ = recover() }()
		_ = L.Compare(-2, -1, 99)
	}()
	// LoadFileMode missing
	_ = L.LoadFileMode("/no/such/file/golua_cover_16", "bt")
	// UpvalueJoin / UpvalueId edges
	_ = runChunk(t, `
local function f(a)
  local function g() return a end
  local function h() return a end
  debug.upvaluejoin(g,1,h,1)
  return debug.upvalueid(g,1) ~= nil
end
return f(1)
`)
	// SetMetaTable replace / rawGetRegistryGlobals
	L.NewTable()
	L.NewTable()
	L.SetMetaTable(-2)
	L.NewTable()
	L.SetMetaTable(-2)
	_ = L.rawGetRegistryGlobals()
	// Where
	_ = L.Where(1)
	_ = L.Where(0)
	_ = L.Where(99)
}

func TestCoverGap16LStateStackEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	// grow near max with raise
	L.reallocStack(LUAI_MAXSTACK - 10)
	L.top = LUAI_MAXSTACK - 20
	func() {
		defer func() { _ = recover() }()
		L.growStackNoRaise(100, true)
	}()
	L2 := NewState()
	defer L2.Close()
	L2.reallocStack(LUAI_MAXSTACK - 10)
	L2.top = LUAI_MAXSTACK - 20
	_ = L2.growStackNoRaise(100, false)
	// already ERRORSTACKSIZE
	L3 := NewState()
	defer L3.Close()
	L3.reallocStack(ERRORSTACKSIZE)
	L3.top = 10
	_ = L3.growStackNoRaise(8, false)
	func() {
		defer func() { _ = recover() }()
		L3.growStackNoRaise(8, true)
	}()
	// shrinkStack used < size/4
	L4 := NewState()
	defer L4.Close()
	L4.reallocStack(BASIC_STACK_SIZE * 8)
	L4.top = 2
	L4.shrinkStack()
	// closeAllThreads nil g
	((*globalState)(nil)).closeAllThreads(nil)
	// Resume C stack overflow
	co := L.NewThread()
	_ = co.LoadString(`return 1`, "=r")
	if L != nil {
		// bump from's nCcalls
		old := L.nCcalls
		L.nCcalls = uint32(LUAI_MAXCCALLS)
		_ = co.Resume(L, 0)
		L.nCcalls = old
	}
	// ResetThread async close path
	co5 := L.NewThread()
	_ = co5.LoadString(`coroutine.yield(1)`, "=y")
	_ = co5.Resume(L, 0)
	_ = co5.ResetThread()
	// resumeError nil
	_ = resumeError(nil, "x")
	// ensureAsync / prepareFastYield edges
	main := L
	func() {
		defer func() { _ = recover() }()
		main.prepareFastYield(0)
	}()
	co6 := L.NewThread()
	co6.ensureAsync()
	if co6.co != nil {
		co6.co.sync = true
		co6.ensureAsync()
	}
}

func TestCoverGap16PCallKYieldable(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local co=coroutine.create(function()
  local ok, a = pcall(function()
    error("x")
  end)
  say(ok)
  local ok2, b = pcall(function()
    return coroutine.yield(7)
  end)
  say(ok2); say(b)
  return "done"
end)
local ok,v=coroutine.resume(co)
say(ok); say(v)
ok,v=coroutine.resume(co, 8)
say(ok); say(v)
-- xpcall yieldable
local co2=coroutine.create(function()
  local ok,e=xpcall(function() error("z") end, function(m) return "H" end)
  say(ok); say(e)
end)
coroutine.resume(co2)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16DebugHooksTrace(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local n=0
debug.sethook(function(ev,line)
  n=n+1
  if n>20 then debug.sethook() end
end, "lcr", 1)
local function f(a) return a+1 end
say(f(1))
for i=1,5 do say(i) end
debug.sethook()
-- count hook only
debug.sethook(function() end, "", 2)
for i=1,5 do end
debug.sethook()
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap16LCodeMore(t *testing.T) {
	_ = runChunk(t, `
-- luaK_setoneret / exp2val / goto removevalues paths
local function f()
  local a = true and false or true
  local b = not (true and false)
  if a then return 1 elseif b then return 2 else return 3 end
end
local function g(x)
  local t = {[x]=1, [x+1]=2}
  return t[x]
end
local function h(...)
  local a = ...
  return a
end
return f(), g(1), h(9)
`)
}

func TestCoverGap16IOMoreCorners(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("hello\nworld\n"), 0o644)
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushString(path)
	L.SetGlobal("N")
	if err := L.LoadString(`
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local n=N
local f=assert(io.open(n,"r"))
say(f:read(0)=="")
say(tostring(f):find("file")~=nil)
f:close()
say(tostring(f):find("closed")~=nil)
local t=io.tmpfile()
if t then t:write("z"); t:seek("set"); say(t:read("*a")); t:close() end
f=assert(io.open(n,"r"))
say(f:setvbuf("no"))
say(f:setvbuf("full", 64))
say(f:setvbuf("line"))
boom(function() f:setvbuf("bad") end)
f:close()
boom(function() f:seek("set") end)
return table.concat(out,"|")
`, "=io"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
}

func TestCoverGap16GCAfterAlloc(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// queue a finalizer then allocate so gcAfterAlloc returns true
	_ = runChunk(t, `
collectgarbage("stop")
local n=0
local function mk()
  return setmetatable({}, {__gc=function() n=n+1 end})
end
for i=1,50 do mk() end
collectgarbage("restart")
collectgarbage("collect")
-- more allocs after pending
for i=1,20 do local t={} end
return n>=0
`)
}

func TestCoverGap16StrMatchDepth(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// force pattern too complex via tiny depth
	ms := &strMatchState{L: L, matchdepth: 0, src: "a", p: "a", srcEnd: 1, pEnd: 1}
	func() {
		defer func() { _ = recover() }()
		_, _ = strMatch(ms, 0, 0)
	}()
}

func TestCoverGap16LuacAndMisc(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	_ = os.WriteFile(src, []byte("local function f(a) return a end; return f"), 0o644)
	var out, errb bytes.Buffer
	_ = Luac([]string{"luac", "-l", "-l", src}, &out, &errb)
	// Where with active stack
	_ = runChunk(t, `
local function f()
  local s = debug.traceback()
  return s ~= nil
end
return f()
`)
}
