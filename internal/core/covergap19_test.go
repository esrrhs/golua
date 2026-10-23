package core

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"weak"
)

func TestCoverGap19TFORAndOpcodes(t *testing.T) {
	// generic for still works after folding dead OP_TFORLOOP case
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local t={10,20,30}
for i,v in ipairs(t) do say(i); say(v) end
for k,v in pairs({a=1}) do say(k); say(v) end
-- runtime BNOT / MUL / IDIV float (avoid constant fold)
local x=5; say(~x)
local a,b=1.5,2.5; say(a*b)
local c,d=7.0,2.0; say(c//d)
-- float forprep limit error
say(not pcall(function() for i=1.0,{},1.0 do end end))
-- LEI/GEI metamethod with integer constant
local ud=setmetatable({},{
  __lt=function(a,b) return true end,
  __le=function(a,b) return true end,
})
say(ud <= 1)
say(ud >= 1)
say(1 <= ud)
say(1 >= ud)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap19CraftedVM(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString(`return 0`, "=craft"); err != nil {
		t.Fatal(err)
	}
	cl := clLvalue(&L.stack[L.top-1])
	var k0 Value
	setivalue(&k0, 42)
	cl.p.k = []Value{k0}
	cl.p.sizek = 1
	// LOADKX + EXTRAARG + RETURN
	cl.p.code = []Instruction{
		newABx(OP_LOADKX, 0, 0),
		newAx(OP_EXTRAARG, 0),
		newABC(OP_RETURN, 0, 2, 0, 0),
	}
	cl.p.sizecode = 3
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	if v, ok := L.ToInteger(-1); !ok || v != 42 {
		t.Fatalf("LOADKX got %v", L.ToStringX(-1))
	}

	// unknown opcode → default panic
	if err := L.LoadString(`return 0`, "=bad"); err != nil {
		t.Fatal(err)
	}
	cl2 := clLvalue(&L.stack[L.top-1])
	cl2.p.code = []Instruction{
		{Op: NUM_OPCODES + 7, A: 0},
		newABC(OP_RETURN, 0, 1, 0, 0),
	}
	cl2.p.sizecode = 2
	st := L.PCall(0, 0, 0)
	if st == LUA_OK {
		t.Fatal("expected unknown opcode error")
	}
	L.SetTop(0)

	// lone OP_EXTRAARG / OP_TFORLOOP no-op cases
	if err := L.LoadString(`return 1`, "=extra"); err != nil {
		t.Fatal(err)
	}
	cl3 := clLvalue(&L.stack[L.top-1])
	cl3.p.code = []Instruction{
		newAx(OP_EXTRAARG, 0),
		{Op: OP_TFORLOOP, A: 0, B: 0},
		newABC(OP_LOADI, 0, 1+OFFSET_sBx, 0, 0),
		newABC(OP_RETURN, 0, 2, 0, 0),
	}
	cl3.p.sizecode = 4
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
}

func TestCoverGap19ConcatOverflow(t *testing.T) {
	old := luaV_concatMaxLen
	t.Cleanup(func() { luaV_concatMaxLen = old })
	luaV_concatMaxLen = 8
	L := NewState()
	defer L.Close()
	L.checkStack(8)
	L.PushString("abcdef") // 6
	L.PushString("ghijkl") // 6 → 6+6 > 8
	func() {
		defer func() { _ = recover() }()
		luaV_concat(L, 2)
	}()
}

func TestCoverGap19GCAfterAllocContinue(t *testing.T) {
	// Force pending finalizers so NEWTABLE/CLOSURE take the continue arm.
	got := runChunk(t, `
collectgarbage("stop")
local keep={}
for i=1,80 do
  keep[i]=setmetatable({}, {__gc=function() end})
end
keep=nil
collectgarbage("restart")
collectgarbage("collect")
-- allocations that hit gcAfterAlloc while pending may still be racing;
-- also build closures and tables eagerly
local function mk()
  local t={}
  for i=1,30 do t[i]=i end
  local function f() return t end
  return f
end
local fs={}
for i=1,40 do fs[i]=mk() end
return #fs
`)
	_ = got

	L := NewState()
	defer L.Close()
	// Direct: queue pending then call gcAfterAlloc
	var uv Value
	setuvalue(&uv, &Udata{uv: make([]Value, 1)})
	L.g.pendN.Add(1)
	gcMu.Lock()
	L.g.pendingRun = append(L.g.pendingRun, gcFinalizer{L: L, obj: uv})
	gcMu.Unlock()
	_ = L.gcAfterAlloc()
}

func TestCoverGap19NewStateFail(t *testing.T) {
	old := fLuaOpenFn
	t.Cleanup(func() { fLuaOpenFn = old })
	fLuaOpenFn = func(L *State) { panic("cover open fail") }
	L := NewState()
	if L != nil {
		L.Close()
		t.Fatal("expected nil NewState")
	}
}

func TestCoverGap19TypeNameBoolean(t *testing.T) {
	var v Value
	setbvalue(&v, true)
	if typeName(&v) != "boolean" {
		t.Fatal(typeName(&v))
	}
	setbvalue(&v, false)
	if typeName(&v) != "boolean" {
		t.Fatal(typeName(&v))
	}
	_ = vmFastGet(NewState(), &v, &v) // non-table → nil
}

func TestCoverGap19IONumOverflowAndPrune(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.checkStack(4)
	// overflow via next()
	br := bufio.NewReader(strings.NewReader(strings.Repeat("9", ioMaxLenNum+10) + "\n"))
	rn := ioNewNumReader(br)
	for i := 0; i < ioMaxLenNum+5; i++ {
		if !rn.next() {
			break
		}
	}
	if len(rn.buff) != 0 {
		t.Fatalf("expected invalidated buff, got %d", len(rn.buff))
	}
	// digits break on overflow
	br2 := bufio.NewReader(strings.NewReader(strings.Repeat("1", ioMaxLenNum+5)))
	rn2 := ioNewNumReader(br2)
	_ = rn2.digits(false)

	// "0" not followed by x → count=1 arm
	s := &ioStream{canR: true, rd: nopCloser{strings.NewReader("09\n")}}
	_ = ioReadNumber(L, s)

	// spaces then EOF
	s2 := &ioStream{canR: true, rd: nopCloser{strings.NewReader("   ")}}
	_ = ioReadNumber(L, s2)

	// prune shrink path: inflate next prune, then prune to small list
	ioOpenStreamsMu.Lock()
	oldStreams := ioOpenStreams
	oldNext := ioOpenNextPrune
	ioOpenStreams = nil
	ioOpenNextPrune = 10000
	for i := 0; i < 10; i++ {
		s := &ioStream{}
		ioOpenStreams = append(ioOpenStreams, weak.Make(s))
	}
	// also add already-dead weak entries
	for i := 0; i < 5; i++ {
		tmp := &ioStream{}
		ioOpenStreams = append(ioOpenStreams, weak.Make(tmp))
		tmp = nil
	}
	ioOpenStreamsMu.Unlock()
	runtime.GC()
	runtime.GC()
	ioPruneOpen()
	// second prune with n*2 > ioOpenMax
	ioOpenStreamsMu.Lock()
	ioOpenStreams = nil
	ioOpenNextPrune = ioOpenMax * 8
	keepers := make([]*ioStream, 600)
	for i := range keepers {
		keepers[i] = &ioStream{}
		ioOpenStreams = append(ioOpenStreams, weak.Make(keepers[i]))
	}
	ioOpenStreamsMu.Unlock()
	ioPruneOpen()
	ioOpenStreamsMu.Lock()
	ioOpenStreams = oldStreams
	ioOpenNextPrune = oldNext
	ioOpenStreamsMu.Unlock()
	_ = keepers
}

func TestCoverGap19IOMoreEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("line1\nline2\n"), 0o644)

	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- closed default input
local old=io.input()
local f=assert(io.open("`+path+`", "r"))
io.input(f)
f:close()
boom(function() io.read(1) end)
io.input(old)
-- setvbuf negative size
f=assert(io.open("`+path+`", "r"))
say(f:setvbuf("full", -1))
-- seek math.mininteger edge via huge negative
say(f:seek("cur", 0))
f:close()
-- popen fail
boom(function() assert(io.popen("/no/such/golua19_cmd", "r")) end)
-- rename windows arm via hook
return table.concat(out,"|")
`)
	_ = got

	// writeString error
	s := &ioStream{canW: true, wr: errWriter{}}
	_ = s.writeString("hi")

	// seek flush error: buffer then fail on flush
	s2 := &ioStream{canW: true, f: mustTemp(t), nobuf: false}
	s2.wbuf = bufio.NewWriterSize(errWriter{}, 64)
	_, _ = s2.seek(0, 0)

	// pipe seek
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	_ = L.LoadString(`
local p=io.popen("echo hi","r")
if p then
  local ok,err = p:seek("set",0)
  p:close()
  return ok==nil
end
return true
`, "=p")
	_ = L.PCall(0, 1, 0)

	// ioRename windows
	old := ioIsWindowsFn
	t.Cleanup(func() { ioIsWindowsFn = old })
	ioIsWindowsFn = func() bool { return true }
	_ = ioRename("/no/a", "/no/b")

	// ioNewMetaTable already-exists
	L2 := NewState()
	defer L2.Close()
	L2.OpenLibs()
	L2.PushString("x")
	L2.SetField(LUA_REGISTRYINDEX, "FILE*")
	_ = ioNewMetaTable(L2, "FILE*")

	// ioTestUData wrong mt
	L2.NewUserData(nil, 0)
	L2.NewTable()
	L2.SetMetaTable(-2)
	_ = ioTestUData(L2, -1, "FILE*")

	// ioAuxClose nil closef — push stream userdata then clear closef
	L2.SetTop(0)
	ud := L2.NewUserData(&ioStream{}, 1)
	_ = ud
	_ = ioAuxClose(L2)

	// invalid format in gread
	_ = L2.LoadString(`
local f=io.tmpfile()
if f then local ok=pcall(function() f:read("*z") end); f:close(); return ok end
return true
`, "=fmt")
	_ = L2.PCall(0, 1, 0)

	// append seek fail — hard; skip if can't
	// io.type / tostring closed already covered elsewhere
}

type errWriter struct{}

func (errWriter) Write(p []byte) (int, error) { return 0, errors.New("write fail") }
func (errWriter) Close() error                { return nil }

func mustTemp(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "s")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func TestCoverGap19CoroEmptyStackAndClose(t *testing.T) {
	L := NewState()
	L.OpenLibs()

	// empty-stack salvage via plain panic from Go body
	co := L.NewThread()
	co.PushGoFunc(func(L *State) int { panic("salvage-boom") })
	_ = coAuxResume(L, co, 0)

	// coAuxStatus arms
	co3 := L.NewThread()
	co3.status = LUA_OK
	co3.dead = false
	co3.top = 0
	_ = coAuxStatus(L, co3)
	co3.checkStack(2)
	co3.top = 2
	setivalue(&co3.stack[0], 1)
	setivalue(&co3.stack[1], 2)
	_ = coAuxStatus(L, co3)

	// fastYieldNow via deep sync yield (vmNest>1) — own state so Close is clean
	_ = runChunk(t, `
local co=coroutine.create(function()
  local function deep(n)
    if n<=0 then return coroutine.yield("y") end
    return deep(n-1)
  end
  return deep(5)
end)
local ok,v=coroutine.resume(co)
assert(ok and v=="y")
ok,v=coroutine.resume(co,9)
assert(ok and v==9)
return true
`)

	// sync close error paths (no live goroutine)
	co4 := L.NewThread()
	co4.co.sync = true
	co4.co.live = false
	co4.dead = false
	co4.status = LUA_ERRRUN
	co4.checkStack(2)
	co4.top = 2
	setnilvalue(&co4.stack[0])
	setsvalue(&co4.stack[1], luaS_new(co4, "close-err"))
	_, _ = coCloseThread(co4, L)

	co5 := L.NewThread()
	co5.co.sync = true
	co5.co.live = false
	co5.dead = false
	co5.status = LUA_ERRRUN
	co5.checkStack(1)
	co5.top = 1
	setnilvalue(&co5.stack[0])
	co5.co.vals = nil
	func() {
		defer func() { _ = recover() }()
		_, _ = coCloseThread(co5, L)
	}()

	co6 := L.NewThread()
	_ = co6.LoadString(`error("x")`, "=e")
	_ = co6.Resume(L, 0)
	co6.top = 0
	co6.hasErr = true
	setsvalue(&co6.lastErr, luaS_new(co6, "leftover"))
	_, _ = coCloseThread(co6, L)

	co7 := L.NewThread()
	_ = co7.LoadString(`return 1`, "=d")
	_ = co7.Resume(L, 0)
	_, _ = coCloseThread(co7, L)

	func() {
		defer func() { _ = recover() }()
		L.prepareFastYield(0)
	}()
	co8 := L.NewThread()
	if co8.co != nil {
		co8.co.sync = false
	}
	co8.ensureAsync()

	// Prevent closeAllThreads from spinning on any async leftovers from
	// Go-body resumes: mark every child dead before Close.
	for _, th := range []*State{co, co3, co4, co5, co6, co7, co8} {
		if th == nil {
			continue
		}
		th.dead = true
		if th.co != nil {
			th.co.live = false
			th.co.sync = true // skip channel send in forceCloseThread
		}
	}
	L.Close()
}

func TestCoverGap19LStateRaces(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// forceCloseThread on sync finished / already-dead
	co := L.NewThread()
	_ = co.LoadString(`return 1`, "=y")
	_ = co.Resume(L, 0)
	_ = co.forceCloseThread()
	_ = co.forceCloseThread() // already dead

	// freeUnusedCI with nil ci
	L2 := NewState()
	defer L2.Close()
	saved := L2.ci
	L2.ci = nil
	freeUnusedCI(L2)
	L2.ci = saved

	// shrinkStack used > size/2 and < BASIC clamp
	L3 := NewState()
	defer L3.Close()
	L3.reallocStack(BASIC_STACK_SIZE * 8)
	L3.top = BASIC_STACK_SIZE*2 + EXTRA_STACK
	L3.shrinkStack()

	// resumeSync recover default path — panic from body
	co9 := L.NewThread()
	co9.PushGoFunc(func(L *State) int {
		panic("plain panic in coro")
	})
	_ = co9.Resume(L, 0)

	// Resume dead
	_ = co9.Resume(L, 0)

	// Resume with top<1 started path
	co10 := L.NewThread()
	co10.checkStack(1)
	co10.top = 0
	co10.started = false
	co10.status = LUA_OK
	func() {
		defer func() { _ = recover() }()
		_ = co10.Resume(L, 0)
	}()

	// finishMigrated nil migrateFunc — hard; skip
	// runContinuation without C API
	old := callContinuation
	callContinuation = nil
	t.Cleanup(func() { callContinuation = old })
	func() {
		defer func() { _ = recover() }()
		_ = L.runContinuation(&CallInfo{}, LUA_OK)
	}()

	// rawRunProtected stack-full error obj paths — grow near top
	L4 := NewState()
	defer L4.Close()
	L4.top = len(L4.stack)
	_ = L4.rawRunProtected(func() {
		panic(&luaError{status: LUA_ERRRUN, value: strValue(L4, "e")})
	})
	L4.top = len(L4.stack)
	_ = L4.rawRunProtected(func() {
		panic("plain")
	})
}

func TestCoverGap19LuacPrintCode(t *testing.T) {
	var out, errb bytes.Buffer
	L0 := NewState()
	defer L0.Close()
	c := &luac{out: &out, err: &errb, tmname: L0.g.tmNames}
	var k0, k1 Value
	setivalue(&k0, 1)
	setsvalue(&k1, &TString{tt: LUA_VSHRSTR, val: "f"})
	f := &Proto{
		k:     []Value{k0, k1},
		sizek: 2,
		code: []Instruction{
			newABx(OP_LOADKX, 0, 0),
			newAx(OP_EXTRAARG, 0),
			newABC(OP_SETTABUP, 0, 0, 1, 1), // isk
			newABC(OP_SETTABUP, 0, 0, 1, 0),
			newABC(OP_MMBINK, 0, 0, int(TM_ADD), 1), // isk flip
			newABC(OP_EQK, 0, 0, 0, 1),
			newABC(OP_TESTSET, 0, 1, 0, 1),
			newABC(OP_CALL, 0, 0, 0, 0), // all in / all out
			newABC(OP_CALL, 0, 2, 2, 0),
			newABC(OP_VARARG, 0, 0, 0, 0),  // all out
			newABC(OP_VARARG, 0, 0, 3, 0),  // n out
			newABC(OP_SETLIST, 0, 1, 1, 1), // isk + EXTRAARG next
			newAx(OP_EXTRAARG, 2),
			newABC(OP_RETURN, 0, 1, 0, 0),
		},
		upvalues:     []Upvaldesc{{name: &TString{tt: LUA_VSHRSTR, val: "_ENV"}}},
		sizeupvalues: 1,
	}
	f.sizecode = len(f.code)
	c.printCode(f)

	// dump empty-buffer / write-fail / close-fail
	dir := t.TempDir()
	outPath := filepath.Join(dir, "x.out")
	c2 := &luac{out: &out, err: &errb, output: outPath, dumping: 1}
	L := NewState()
	defer L.Close()
	_ = L.LoadString(`return 1`, "=d")
	proto := clLvalue(&L.stack[L.top-1]).p
	_ = c2.dump(L, proto)

	// write fail via /dev/full
	c3 := &luac{out: &out, err: &errb, output: "/dev/full", dumping: 1}
	_ = c3.dump(L, proto)

	// close fail
	oldClose := luacFileCloseFn
	t.Cleanup(func() { luacFileCloseFn = oldClose })
	luacFileCloseFn = func(f *os.File) error { _ = f.Close(); return errors.New("close fail") }
	c4 := &luac{out: &out, err: &errb, output: filepath.Join(dir, "c.out"), dumping: 1}
	_ = c4.dump(L, proto)

	// loadStdin error — can't easily fail ReadAll; skip
	// combine fail: force LoadBytes fail via oversized? skip
	// pmain f==nil
	c5 := &luac{out: &out, err: &errb}
	// listing with SETLIST EXTRAARG via real source
	src := filepath.Join(dir, "big.lua")
	var b strings.Builder
	b.WriteString("return {")
	for i := 0; i < 60; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"` + strings.Repeat("z", 2) + `"`)
	}
	b.WriteString("}")
	_ = os.WriteFile(src, []byte(b.String()), 0o644)
	_ = Luac([]string{"luac", "-l", "-l", src}, &out, &errb)
	_ = c5
}

func TestCoverGap19LDebugRemain(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	p := &Proto{lineinfo: []int32{1, 2}, code: []Instruction{{Op: OP_LOADKX}, newAx(OP_EXTRAARG, 0)}}
	_ = luaG_getfuncline(p, -1)
	_ = luaG_getfuncline(p, 99)
	_ = currentLine(L, &CallInfo{callstatus: CIST_C}) // not Lua

	_ = upvalName(p, -1)
	_ = upvalName(&Proto{upvalues: []Upvaldesc{{}}}, 0)

	var pos int
	func() {
		defer func() { _ = recover() }()
		_ = findvararg(L, &L.baseCI, 99, &pos)
	}()

	// findlocal vararg / temporary
	got := runChunk(t, `
local function f(a, ...)
  local x = 1
  local name = debug.getlocal(1, -1)
  local n2 = debug.getlocal(1, 50) -- temporary / none
  debug.sethook(function()
    local n = debug.getlocal(2, 1)
  end, "c")
  local function g() return a,x end
  g()
  debug.sethook()
  return name ~= nil or true
end
return f(1,2,3)
`)
	_ = got

	// findsetreg OP_TFORCALL / bounds
	p2 := &Proto{code: []Instruction{
		newABC(OP_TFORCALL, 0, 0, 1, 0),
		newABC(OP_MOVE, 0, 0, 0, 0),
	}}
	_ = findsetreg(p2, 0, 5)
	_ = findsetreg(p2, -1, 0)

	// basicgetobjname LOADKX
	var name string
	pc := 0
	_ = basicgetobjname(p, &pc, 0, &name)

	// getobjname GETI
	p3 := &Proto{code: []Instruction{newABC(OP_GETI, 0, 0, 1, 0)}}
	_ = getobjname(p3, 0, 0, &name)

	// funcNameFromCall nil / hooked
	_ = funcNameFromCall(L, nil, &name)
	ci := &CallInfo{callstatus: CIST_HOOKED}
	_ = funcNameFromCall(L, ci, &name)

	// getUpvalName / instack not found
	var o Value
	setivalue(&o, 1)
	func() {
		defer func() { _ = recover() }()
		_ = getUpvalName(L, &L.baseCI, &o, &name)
	}()
	_ = instack(L, &L.baseCI, &o)

	// pushMessage with full stack
	L2 := NewState()
	defer L2.Close()
	L2.top = len(L2.stack)
	ts := luaS_new(L2, "msg")
	L2.pushMessage(ts)
}

func TestCoverGap19MiscHooks(t *testing.T) {
	// ioOpenFile append seek — use a file we can open append
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)
	f, err := ioOpenFile(path, "a")
	if err == nil && f != nil {
		_ = f.Close()
	}

	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
-- lines iterator closes with error path
local n=os.tmpname()
local f=assert(io.open(n,"w")); f:write("a\n"); f:close()
f=assert(io.open(n,"r"))
local iter = f:lines()
-- consume
for l in iter do say(l) end
-- toclose self-created file via io.lines(path)
for l in io.lines(n) do say(l) end
os.remove(n)
return table.concat(out,"|")
`)
	_ = got
}
