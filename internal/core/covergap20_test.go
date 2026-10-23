package core

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestCoverGap20GCContinueInVM(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.g.pendN.Store(1)
	t.Cleanup(func() { L.g.pendN.Store(0) })
	if err := L.LoadString(`
local t = {1,2,3}
local function f() return t end
local s = "a".."b".."c"
return f(), #t, s
`, "=gc"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 3, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
}

func TestCoverGap20FastYieldNow(t *testing.T) {
	got := runChunk(t, `
local co = coroutine.create(function()
  local t = setmetatable({}, {
    __index = function() return coroutine.yield("from-tm") end
  })
  return t.k
end)
local ok, v = coroutine.resume(co)
assert(ok and v == "from-tm")
ok, v = coroutine.resume(co, 99)
assert(ok and v == 99)
co = coroutine.create(function()
  local f = setmetatable({}, {
    __call = function() return coroutine.yield("call") end
  })
  return f()
end)
ok, v = coroutine.resume(co)
assert(ok and v == "call")
ok, v = coroutine.resume(co, 7)
assert(ok and v == 7)
return true
`)
	_ = got
}

func TestCoverGap20CoAuxResumeSalvage(t *testing.T) {
	L := NewState()
	L.OpenLibs()
	co := L.NewThread()
	co.PushGoFunc(func(L *State) int {
		L.top = 0
		panic("empty-top-panic")
	})
	_ = coAuxResume(L, co, 0)
	co.dead = true
	if co.co != nil {
		co.co.live = false
		co.co.sync = true
	}
	L.Close()
}

func TestCoverGap20IORemain(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	s := &ioStream{canW: true, nobuf: true, wr: errWriter{}}
	if err := s.writeString("x"); err == nil {
		t.Fatal("expected write err")
	}

	br := bufio.NewReader(strings.NewReader("1"))
	rn := ioNewNumReader(br)
	_ = rn.next()

	f := mustTemp(t)
	_, _ = f.WriteString("abcdefghij")
	_, _ = f.Seek(0, 0)
	s2 := &ioStream{canR: true, canW: true, f: f, nobuf: true}
	s2.buf = bufio.NewReaderSize(f, 64)
	_, _ = s2.buf.Peek(5)
	_, _ = s2.seek(0, 1)

	s3 := &ioStream{canW: true, f: mustTemp(t)}
	s3.wbuf = bufio.NewWriterSize(errWriter{}, 8)
	_ = s3.writeString("zzzzzzzzzz")
	_, _ = s3.seek(0, 0)

	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	_ = os.WriteFile(path, []byte("1\n"), 0o644)
	L.PushString(path)
	L.SetGlobal("P")
	_ = L.LoadString(`
local f=assert(io.open(P,"r"))
local ok = pcall(function() f:read("*z") end)
f:close()
local p=io.popen("true","r")
if p then local a=p:seek("set"); p:close(); return ok, a==nil end
return ok, true
`, "=z")
	_ = L.PCall(0, 2, 0)

	old := ioIsWindowsFn
	ioIsWindowsFn = func() bool { return false }
	_ = ioRename("/no/a20", "/no/b20")
	ioIsWindowsFn = old

	// cfp arm of tostring
	L.SetTop(0)
	st := &ioStream{cfp: unsafe.Pointer(uintptr(1)), closef: func(*State) int { return 0 }}
	L.NewUserData(st, 1)
	L.GetField(LUA_REGISTRYINDEX, "FILE*")
	if !L.IsNil(-1) {
		L.SetMetaTable(-2)
		_ = ioFToString(L)
	}
}

func TestCoverGap20LDebugMore(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// LOADKX name via getobjname / basicgetobjname with real EXTRAARG
	p := &Proto{
		k: []Value{},
		code: []Instruction{
			newABx(OP_LOADKX, 0, 0),
			newAx(OP_EXTRAARG, 0),
		},
	}
	var kv Value
	setivalue(&kv, 7)
	p.k = []Value{kv}
	p.sizek = 1
	var name string
	pc := 0
	_ = basicgetobjname(p, &pc, 0, &name)

	// GETI name
	p2 := &Proto{code: []Instruction{newABC(OP_GETI, 0, 0, 3, 0)}}
	_ = getobjname(p2, 0, 0, &name)

	// findsetreg TFORCALL change
	p3 := &Proto{code: []Instruction{
		newABC(OP_MOVE, 0, 0, 0, 0),
		newABC(OP_TFORCALL, 0, 0, 1, 0),
	}}
	_ = findsetreg(p3, 1, 5)

	// findlocal temporary via Lua with deep locals already in gap19
	got := runChunk(t, `
local function f(a, ...)
  local b = a
  return debug.getlocal(1, -1), select("#", ...)
end
return f(1,2,3)
`)
	_ = got

	// getUpvalName hit: real Lua upvalue
	got = runChunk(t, `
local x = 1
local function f() return x end
local name, val = debug.getupvalue(f, 1)
return name, val
`)
	_ = got
}

func TestCoverGap20LuacRemain(t *testing.T) {
	var out, errb bytes.Buffer
	dir := t.TempDir()
	// dump writer: force werr then second write via /dev/full
	L := NewState()
	defer L.Close()
	_ = L.LoadString(`return 1`, "=d")
	proto := clLvalue(&L.stack[L.top-1]).p
	c := &luac{out: &out, err: &errb, output: "/dev/full", dumping: 1}
	_ = c.dump(L, proto)

	// combine LoadBytes fail: use CheckStack hook to fail pmain path
	// pmain f==nil via combine returning nil — LoadBytes fail hard.
	// loadStdin: replace os.Stdin briefly? skip

	// listing that exercises SETLIST isk EXTRAARG via source
	src := filepath.Join(dir, "s.lua")
	_ = os.WriteFile(src, []byte(`
local t = {1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,
21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,
41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60}
_ENV.x = 1
local a = t[1]
return a, ({})[1], ...
`), 0o644)
	_ = Luac([]string{"luac", "-l", "-l", src}, &out, &errb)

	// empty write callback: call dump writer with crafted empty via
	// luacFileClose already tested; invoke luaU_dump with empty-friendly proto
	_ = luaU_dump(L, proto, func(L *State, b []byte) int {
		if len(b) == 0 {
			return 0
		}
		return 0
	}, false)
}

func TestCoverGap20HookYieldTrace(t *testing.T) {
	// CIST_HOOKYIELD: line hook that yields (best-effort)
	got := runChunk(t, `
local co = coroutine.create(function()
  local n = 0
  debug.sethook(function()
    n = n + 1
    if n == 1 then
      debug.sethook()
      coroutine.yield("hook")
    end
  end, "l")
  local x = 1
  x = x + 1
  return x
end)
local ok, v = coroutine.resume(co)
if ok and v == "hook" then
  ok, v = coroutine.resume(co)
end
return ok ~= nil
`)
	_ = got
}

func TestCoverGap20ShrinkAndResume(t *testing.T) {
	L := NewState()
	defer L.Close()
	// shrinkStack: size large, used just above size/2 so newsize=used
	L.reallocStack(BASIC_STACK_SIZE * 16)
	used := BASIC_STACK_SIZE*8 + EXTRA_STACK
	L.top = used - EXTRA_STACK
	L.shrinkStack()
	// newsize < BASIC clamp
	L.reallocStack(BASIC_STACK_SIZE * 4)
	L.top = 2
	L.shrinkStack()
}
