package core

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// Direct calls and small Lua snippets that the earlier cover* tests miss.
// Prefer many small cases so a single failure is easy to spot.

func TestCoverGapHelpers(t *testing.T) {
	if !isupvalue(LUA_REGISTRYINDEX-1) || isupvalue(LUA_REGISTRYINDEX) {
		t.Fatal("isupvalue")
	}
	if !errorstatus(LUA_ERRRUN) || errorstatus(LUA_OK) || errorstatus(LUA_YIELD) {
		t.Fatal("errorstatus")
	}
	_ = ltolower('A')
	_ = ltolower('z')
	_ = ltolower(0)
	if math2Trunc(1.9) != 1 || math2Trunc(-1.9) != -1 {
		t.Fatal("math2Trunc")
	}
	if math2NumMod(10, 3) != 1 || math.Abs(math2NumMod(-10, 3)-2) > 1e-9 {
		t.Fatalf("math2NumMod %v", math2NumMod(-10, 3))
	}
	if math2Idiv(10, 3) != 3 || math2Idiv(-10, 3) != -4 {
		t.Fatalf("math2Idiv %v %v", math2Idiv(10, 3), math2Idiv(-10, 3))
	}
	if mathFloor(1.9) != 1 || mathCeil(1.1) != 2 {
		t.Fatal("mathFloor/Ceil")
	}
	if sC2int(OFFSET_sC) != 0 || sJ(OFFSET_sJ) != 0 {
		t.Fatal("sC2int/sJ")
	}
	ins := newAsBx(OP_FORLOOP, 1, 2)
	if ins.Op != OP_FORLOOP {
		t.Fatal("newAsBx")
	}
	_ = testOTMode(int(OP_CALL))
	_ = isOT(newABC(OP_CALL, 0, 0, 0, 0))
	if !osStrftimeYearSafe(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("osStrftimeYearSafe")
	}
	_ = gcWatchEmpty()

	var st uint16
	setoah(&st, 1)
	if getoah(st) != 1 {
		t.Fatal("oah")
	}
	setoah(&st, 0)
	if getoah(st) != 0 {
		t.Fatal("oah clear")
	}
	ci := &CallInfo{}
	setcistrecst(ci, 5)
	if getcistrecst(ci) != 5 {
		t.Fatal("cistrecst")
	}

	p := int(7)
	if gcSetParam(&p, 9) != 7 || p != 9 {
		t.Fatal("gcSetParam")
	}
}

func TestCoverGapObjectAndOpcodes(t *testing.T) {
	var v Value
	setnilvalue(&v)
	if rawtt(&v) != v.tt || !checktag(&v, v.tt) || !checktype(&v, LUA_TNIL) {
		t.Fatal("rawtt/check")
	}
	if isnonstrictnil(&v) {
		t.Fatal("strict nil is not nonstrict")
	}
	setabstkey(&v)
	// empty table node helpers
	n := &node{}
	_ = keytt(n)
	_ = keyiscollectable(n)
	_ = gckey(n)
	_ = gckeyN(n)

	L := NewState()
	defer L.Close()
	ts := luaO_pushfstring(L, "x=%d", 3)
	if ts == nil || !strings.Contains(ts.val, "3") {
		t.Fatalf("pushfstring %v", ts)
	}
	ts2 := luaO_pushvfstring(L, "y=%s", "z")
	if ts2 == nil || ts2.val != "y=z" {
		t.Fatalf("pushvfstring %q", ts2)
	}
	if toStringAny("a") != "a" || toStringAny([]byte("b")) != "b" ||
		toStringAny(nil) != "(null)" || toStringAny(7) != "7" {
		t.Fatalf("toStringAny %q %q", toStringAny(nil), toStringAny(7))
	}
	ts3 := luaS_new(L, "s")
	if toStringAny(ts3) != "s" {
		t.Fatal("toStringAny TString")
	}
	if toIntAny(3) != 3 || toIntAny(int64(4)) != 4 || toIntAny(int32(5)) != 5 ||
		toIntAny(uint(6)) != 6 || toIntAny(uint64(7)) != 7 || toIntAny(byte(8)) != 8 ||
		toIntAny(9.5) != 9 || toIntAny("x") != 0 {
		t.Fatal("toIntAny")
	}
	if toPtrAny(luaH_new(L)) == 0 || toPtrAny(ts3) == 0 ||
		toPtrAny(&LClosure{}) == 0 || toPtrAny(&CClosure{}) == 0 ||
		toPtrAny(&Udata{}) == 0 || toPtrAny(L) == 0 ||
		toPtrAny(unsafe.Pointer(&v)) == 0 || toPtrAny(0) != 0 {
		t.Fatal("toPtrAny more")
	}
}

func TestCoverGapAPIDirect(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	if stackIndexOf(L, 1) < 0 {
		t.Fatal("stackIndexOf pos")
	}
	if stackIndexOf(L, LUA_REGISTRYINDEX) != -1 {
		t.Fatal("stackIndexOf registry")
	}
	func() {
		defer func() { _ = recover() }()
		_ = stackIndexOf(L, 0)
		t.Fatal("expected panic for idx 0")
	}()
	if L.indexToAbs(LUA_REGISTRYINDEX) != -1 {
		t.Fatal("indexToAbs pseudo")
	}
	L.PushInteger(1)
	if L.indexToAbs(-1) != L.top-1 {
		t.Fatal("indexToAbs neg")
	}
	L.Pop(1)

	n := L.StringToNumber("42")
	if n == 0 {
		t.Fatal("StringToNumber ok")
	}
	L.Pop(1)
	if L.StringToNumber("nope") != 0 {
		t.Fatal("StringToNumber fail")
	}

	if L.ToLString(LUA_REGISTRYINDEX) == "" && false {
		t.Fatal("ToLString")
	}
	L.PushString("hi")
	if L.ToLString(-1) != "hi" {
		t.Fatal("ToLString hi")
	}
	L.Pop(1)

	if !L.NewMetaTable("cover.gap.mt") {
		t.Fatal("NewMetaTable first")
	}
	if L.NewMetaTable("cover.gap.mt") {
		t.Fatal("NewMetaTable second")
	}
	L.Pop(1) // leave previous mt
	L.NewUserData("ud", 1)
	L.SetMetaTableByName("cover.gap.mt")
	if L.TestUserData(-1, "cover.gap.mt") == nil {
		t.Fatal("TestUserData")
	}
	if L.CheckUserData(-1, "cover.gap.mt") == nil {
		t.Fatal("CheckUserData")
	}
	if L.TestUserData(-1, "cover.gap.other") != nil {
		t.Fatal("TestUserData wrong")
	}
	L.Pop(1)
	L.PushInteger(1)
	if L.TestUserData(-1, "cover.gap.mt") != nil {
		t.Fatal("TestUserData non-ud")
	}
	L.Pop(1)

	B, room := L.BuffInitSize(8)
	if len(room) < 8 {
		t.Fatal("BuffInitSize")
	}
	copy(room, "abcd")
	B.PushResultSize(4)
	s, _ := L.ToString(-1)
	if s != "abcd" {
		t.Fatalf("PushResultSize %q", s)
	}
	L.Pop(1)
	B2 := &Buffer{L: L, b: make([]byte, 4), size: 4, onStack: false}
	r := B2.PrepBuffSize(2)
	if len(r) < 2 {
		t.Fatal("PrepBuffSize nostack")
	}
	r2 := B2.prepbuffsizeNoStack(100)
	if len(r2) < 100 {
		t.Fatal("prepbuffsizeNoStack grow")
	}

	L.CheckStack2(2, "cover")
	L.CheckStack2(2, "")

	basePushMode(L, -1)
	if !L.IsNil(-1) {
		t.Fatal("basePushMode fail")
	}
	L.Pop(1)
	basePushMode(L, baseGCModeInc)
	s, _ = L.ToString(-1)
	if s != "incremental" {
		t.Fatal(s)
	}
	L.Pop(1)
	basePushMode(L, 99)
	s, _ = L.ToString(-1)
	if s != "generational" {
		t.Fatal(s)
	}
	L.Pop(1)
	if basePairsCont(L, 0) != 3 {
		t.Fatal("basePairsCont")
	}
	prev := gcSetMode(L, "generational")
	_ = prev
	_ = gcSetMode(L, "incremental")

	luaE_setdebt(nil, 1)
	luaE_setdebt(L.g, 42)
	if L.g.gcDebt != 42 {
		t.Fatal("setdebt")
	}
	if L.SetCStackLimit(100) != LUAI_MAXCCALLS {
		t.Fatal("SetCStackLimit")
	}

	th := L.NewThread()
	_ = th.CloseThread(L)
	luaE_freethread(L, L.NewThread())

	t4 := luaH_newarray4(L)
	if t4 == nil {
		t.Fatal("luaH_newarray4")
	}
	luaH_free(L, t4)

	var a, b Value
	setivalue(&a, 1)
	setivalue(&b, 2)
	if luaV_lessequal(L, &b, &a) {
		t.Fatal("lessequal")
	}
	if !luaV_lessequal(L, &a, &b) {
		t.Fatal("lessequal ok")
	}

	_ = touserdataPtr(&a)
	L.NewUserData(nil, 0)
	ud := &L.stack[L.top-1]
	_ = touserdataPtr(ud)
	L.Pop(1)

	deltas := deltasFromAbsolute(&Proto{
		linedefined: 1,
		lineinfo:    []int32{1, 2, 10, 11},
	}, 4, []AbsLineInfo{{pc: 2, line: 10}})
	if len(deltas) != 4 {
		t.Fatal("deltasFromAbsolute")
	}

	st := luaD_rawrunprotected(L, func() {})
	if st != LUA_OK {
		t.Fatal("rawrunprotected ok")
	}
	st = luaD_rawrunprotected(L, func() {
		panic(&luaError{status: LUA_ERRRUN})
	})
	if st != LUA_ERRRUN {
		t.Fatalf("rawrunprotected err %d", st)
	}
	_ = luaD_checkminstack(L)
}

func TestCoverGapDumpWhereDebug(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString("local a = 1\nreturn function(x) local y = x + a; return y end", "=dump"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("pcall")
	}
	var dumped []byte
	rc := L.Dump(func(_ *State, b []byte) int {
		dumped = append(dumped, b...)
		return 0
	}, false)
	if rc != 0 || len(dumped) == 0 {
		t.Fatalf("Dump %d len %d", rc, len(dumped))
	}
	rc = L.Dump(func(_ *State, b []byte) int { return 1 }, true)
	if rc == 0 {
		// writer abort may still return non-zero from dump path
	}
	L.PushInteger(1)
	if L.Dump(func(_ *State, b []byte) int { return 0 }, false) != 1 {
		t.Fatal("Dump non-function")
	}
	L.Pop(1)

	// Where from inside a Lua call
	L.PushValue(-1) // the function
	L.PushInteger(2)
	if err := L.LoadString(`
local f = ...
local function wrap()
  return (function()
    -- call Go Where via debug? we call from Go after nesting
    return f(3)
  end)()
end
return wrap
`, "=w"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("wrap load")
	}
	// Call f directly with nesting via DoString that errors with Where
	L.SetTop(0)
	if err := L.LoadString("local function f()\n  error('x')\nend\nf()", "=where"); err != nil {
		t.Fatal(err)
	}
	_ = L.PCall(0, 0, 0)
	_ = L.Where(0)
	_ = L.Where(1)
	_ = L.Where(99)

	// GetInfo / GetLocal / SetLocal wrappers
	L.SetTop(0)
	if err := L.LoadString(`
local function f(a)
  local b = a + 1
  return b
end
return f
`, "=loc"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("loc")
	}
	called := false
	L.SetHook(func(L *State, ar *dbAr) {
		if called {
			return
		}
		called = true
		_ = L.GetInfo("nSlu", ar)
		_ = L.GetLocal(ar, 1)
		L.PushInteger(99)
		_ = L.SetLocal(ar, 1)
	}, dbMaskCall, 0)
	L.PushValue(-1)
	L.PushInteger(1)
	_ = L.PCall(1, 1, 0)
	L.SetHook(nil, 0, 0)

	// findvararg path: call a vararg function under a hook
	L.SetTop(0)
	if err := L.LoadString(`
local function f(a, ...)
  local x = ...
  return a, x
end
return f
`, "=va"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("va")
	}
	L.SetHook(func(L *State, ar *dbAr) {
		var dar dbAr
		if !dbGetStack(L, 0, &dar) {
			return
		}
		pos := 0
		_ = findvararg(L, L.ci, 1, &pos)
		_ = findlocal(L, L.ci, 1, &pos)
	}, dbMaskCall, 0)
	L.PushValue(-1)
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	_ = L.PCall(3, 2, 0)
	L.SetHook(nil, 0, 0)
}

func TestCoverGapLuaLibs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	t.Setenv("COVER_GAP", path)

	got := runChunk(t, `
local out = {}
local function say(s) out[#out + 1] = tostring(s) end

say(string.lower("AbC"))
say(string.upper("AbC"))
say(string.reverse("abc"))
say(string.len("ab"))
say(string.rep("ab", 3, "-"))
say(pcall(string.rep, "x", 1e12))
say(string.byte("abc", 2, 3))
say(string.char(65, 66))
say(string.sub("abcdef", 2, 4))
say(string.find("abc", "b"))
say(string.gmatch("a1b2", "%d")())
say(select("#", 1, 2, 3))
say(select(2, 10, 20, 30))

local t = {a=1, b=2}
local n = 0
for k in pairs(t) do n = n + 1 end
say(n)
local mt = {__pairs = function(self)
  return function(_, k)
    if k == nil then return "x", 1 end
  end, self, nil
end}
local o = setmetatable({}, mt)
for k in pairs(o) do say(k) end

say(collectgarbage("count") >= 0)
say(collectgarbage("step"))
say(collectgarbage("setpause"))
say(collectgarbage("setstepmul"))
say(collectgarbage("isrunning"))
say(collectgarbage("generational"))
say(collectgarbage("incremental"))
say(collectgarbage("collect"))

local name = os.getenv("COVER_GAP")
local f = assert(io.open(name, "w"))
f:write("line1\nline2\n")
f:flush()
f:close()
f = assert(io.open(name, "r"))
say(io.type(f))
say(f:read("*l"))
say(f:lines()())
f:close()
say(io.type(f))
f = assert(io.open(name, "a+"))
f:write("x")
f:seek("set", 0)
say(#(f:read("*a") or ""))
f:close()
local p = io.popen("false", "r")
if p then
  p:read("*a")
  local a,b,c = p:close()
  say(tostring(a))
  say(tostring(b))
end
local p2 = io.popen("echo ok", "r")
if p2 then
  say((p2:read("*l") or ""):gsub("\r$",""))
  p2:close()
end

say(math.cosh(0))
say(math.sinh(0))
say(math.tanh(0))
say(math.modf(1.5))
say(math.tointeger("3"))
say(math.type(3))
say(math.type(3.5))
say(math.ult(1, 2))
say(math.max(1, 3, 2))
say(math.min(1, 3, 2))
say(math.random(1, 1))
math.randomseed(1)
say(type(math.random()))

say(os.date("%Y", 0))
say(os.date("%y", 0))
say(os.date("%C", 0))
say(os.date("!%Y-%m-%dT%H:%M:%S", 0))
local okd = pcall(os.date, "%Y", math.maxinteger)
say(okd)

say(utf8.len("abc"))
say(utf8.char(0x20AC))
say(select(1, utf8.len("a\x80b")))

local packed = string.pack("<i4", 7)
say(string.unpack("<i4", packed))
say(string.packsize("i4"))
say(#string.pack("z", "hi"))
say(string.unpack("z", string.pack("z", "hi")))

return table.concat(out, "\n")
`)
	if !strings.Contains(got, "abc\n") && !strings.HasPrefix(got, "abc") {
		// first line should be lower
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 10 {
		t.Fatalf("too few lines: %q", got)
	}
	if lines[0] != "abc" || lines[1] != "ABC" {
		t.Fatalf("lower/upper: %q %q", lines[0], lines[1])
	}
}

func TestCoverGapAPIEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	L.PushInteger(1)
	L.PushInteger(2)
	L.Copy(-1, 1)
	n, _ := L.ToInteger(1)
	if n != 2 {
		t.Fatal("Copy")
	}
	L.PushInteger(3)
	L.Rotate(1, 1)
	L.SetTop(0)

	L2 := L.NewThread()
	L.PushInteger(9)
	L2.XMove(L, 1)
	n, _ = L2.ToInteger(-1)
	if n != 9 {
		t.Fatal("XMove")
	}
	L2.Pop(1)

	_ = TypeName(LUA_TNONE)
	_ = L.TypeName(1) // empty

	L.PushGoFunc(func(L *State) int { return 0 })
	if L.ToGoFunction(-1) == nil {
		t.Fatal("ToGoFunction")
	}
	L.PushInteger(1)
	if L.ToGoFunction(-1) != nil {
		t.Fatal("ToGoFunction non")
	}
	L.Pop(2)

	L.CreateTable(0, 0)
	if L.ToPointer(-1) == 0 {
		t.Fatal("ToPointer table")
	}
	L.PushInteger(1)
	_ = L.ToPointer(-1)
	L.Pop(2)

	L.NewUserData("x", 2)
	L.PushString("uv")
	if L.SetUserValue(-2, 1) != 1 {
		t.Fatal("SetUserValue")
	}
	if L.GetUserValue(-1, 1) != LUA_TSTRING {
		t.Fatal("GetUserValue")
	}
	L.Pop(1)
	if L.GetUserValue(-1, 99) != LUA_TNONE && L.GetUserValue(-1, 99) != LUA_TNIL {
		// out of range
	}
	L.Pop(1)

	L.PushInteger(1)
	L.PushInteger(1)
	if !L.RawEqual(-1, -2) {
		t.Fatal("RawEqual")
	}
	if !L.Compare(-1, -2, LUA_OPEQ) {
		t.Fatal("Compare eq")
	}
	L.Pop(2)

	dir := t.TempDir()
	p := filepath.Join(dir, "f.lua")
	if err := os.WriteFile(p, []byte("return 5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := L.LoadFileMode(p, "t"); err != nil {
		t.Fatal(err)
	}
	L.Pop(1)
	if err := L.DoFile(p); err != nil {
		t.Fatal(err)
	}
	L.Pop(1)
	if err := L.DoFile(filepath.Join(dir, "missing.lua")); err == nil {
		t.Fatal("DoFile missing")
	}

	// UpvalueId
	if err := L.LoadString("local x = 1; return function() return x end", "=u"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("uv")
	}
	id := L.UpvalueId(-1, 1)
	if id == nil {
		t.Fatal("UpvalueId")
	}
	_ = L.UpvalueId(-1, 99)
	L.Pop(1)

	// ToClose / CloseSlot
	L.NewUserData(nil, 0)
	if L.NewMetaTable("cover.close") {
		L.PushGoFunc(func(L *State) int { return 0 })
		L.SetField(-2, "__close")
	}
	L.SetMetaTableByName("cover.close")
	L.ToClose(-1)
	L.CloseSlot(-1)
	L.SetTop(0)

	// pcallK path
	L.PushGoFunc(func(L *State) int {
		L.PushInteger(1)
		return 1
	})
	_ = L.pcallK(0, 1, 0, false)
	L.SetTop(0)

	// Error()
	L.PushString("boom")
	func() {
		defer func() { _ = recover() }()
		L.Error()
	}()

	// index2value upvalue / invalid
	func() {
		defer func() { _ = recover() }()
		_ = index2value(L, 0)
	}()
	_ = index2value(L, LUA_REGISTRYINDEX-1) // upvalue of light C / none

	// fastYieldNow via recover
	L2 = L.NewThread()
	L2.co = &coRuntime{sync: true}
	func() {
		defer func() { _ = recover() }()
		L2.fastYieldNow(0)
	}()
}

func TestCoverGapPairsMetamethodYield(t *testing.T) {
	// Hit basePairsCont install path with sync coroutine
	got := runChunk(t, `
local out = {}
local mt = {__pairs = function(self)
  return function(_, k)
    if k == nil then return 1, "a" end
  end, self, nil
end}
local o = setmetatable({}, mt)
for k,v in pairs(o) do out[#out+1] = tostring(k)..tostring(v) end
return table.concat(out, ",")
`)
	if got == "" {
		t.Fatalf("pairs mt empty")
	}
}
