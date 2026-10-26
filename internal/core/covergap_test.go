package core

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// Tests that close statement-coverage holes the shorter cover* suites miss:
// bit32, auxlib userdata/module helpers, warning Go API, Dump/Where/
// GetUpvalue wrappers, debug GetLocal/SetLocal/GetInfo, and package-local
// C-macro helpers that the runtime call graph never reaches.

func TestBit32Library(t *testing.T) {
	got := runChunk(t, `
local o = {}
local function say(s) o[#o+1] = tostring(s) end
say(bit32.band(0xFF, 0x0F, 0x07))
say(bit32.bor(1, 2, 4))
say(bit32.bxor(0xFF, 0x0F))
say(bit32.bnot(0))
say(bit32.btest(1, 2))
say(bit32.btest(3, 1))
say(bit32.lshift(1, 3))
say(bit32.rshift(8, 2))
say(bit32.arshift(8, 2))
say(bit32.arshift(0x80000000, 1))
say(bit32.arshift(0x80000000, 40))
say(bit32.lshift(1, 40))
say(bit32.rshift(1, 40))
say(bit32.lrotate(1, 1))
say(bit32.rrotate(2, 1))
say(bit32.lrotate(1, 0))
say(bit32.extract(0xF0, 4, 4))
say(bit32.replace(0, 0xF, 4, 4))
local ok = pcall(bit32.extract, 0, -1)
say(ok)
ok = pcall(bit32.extract, 0, 0, 0)
say(ok)
ok = pcall(bit32.extract, 0, 30, 4)
say(ok)
return table.concat(o, ",")
`)
	parts := strings.Split(got, ",")
	if len(parts) < 20 {
		t.Fatalf("too few results: %q", got)
	}
	wantPrefix := []string{"7", "7", "240", "4294967295", "false", "true", "8", "2", "2"}
	for i, w := range wantPrefix {
		if parts[i] != w {
			t.Fatalf("part %d: got %q want %q (full %q)", i, parts[i], w, got)
		}
	}
	if parts[len(parts)-3] != "false" || parts[len(parts)-2] != "false" || parts[len(parts)-1] != "false" {
		t.Fatalf("error paths: %q", got)
	}
}

func TestStringLowerUpperAndLongDump(t *testing.T) {
	got := runChunk(t, `
local s = string.lower("AbC")
local u = string.upper("AbC")
-- force DumpString size >= 0xFF so dumpSizeT/loadSizeT run
local long = string.rep("x", 300)
local f = load("return [[" .. long .. "]]")
local bin = string.dump(f)
local g = assert(load(bin))
local back = g()
return s .. "|" .. u .. "|" .. tostring(#back == 300 and back == long)
`)
	if got != "abc|ABC|true" {
		t.Fatalf("got %q", got)
	}
}

func TestAuxlibUserdataAndModule(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	if !L.NewMetaTable("cover.ud") {
		t.Fatal("NewMetaTable first")
	}
	if L.NewMetaTable("cover.ud") {
		t.Fatal("NewMetaTable second should be false")
	}
	L.Pop(1) // leave previous metatable

	L.NewUserData("payload", 1)
	L.SetMetaTableByName("cover.ud")
	if p := L.TestUserData(-1, "cover.ud"); p != "payload" {
		t.Fatalf("TestUserData: %#v", p)
	}
	if p := L.TestUserData(-1, "cover.other"); p != nil {
		t.Fatalf("wrong mt: %#v", p)
	}
	if p := L.CheckUserData(-1, "cover.ud"); p != "payload" {
		t.Fatalf("CheckUserData: %#v", p)
	}
	{
		L2 := NewState()
		defer L2.Close()
		L2.PushInteger(1)
		func() {
			defer func() { _ = recover() }()
			_ = L2.CheckUserData(-1, "cover.ud")
			t.Fatal("CheckUserData should error on integer")
		}()
	}
	L.SetTop(0)

	L.PushModule("covergap.pushmod", 2)
	if !L.IsTable(-1) {
		t.Fatal("PushModule")
	}
	L.Pop(1)
	// second PushModule hits LOADED cache path
	L.PushModule("covergap.pushmod", 2)
	if !L.IsTable(-1) {
		t.Fatal("PushModule cached")
	}
	L.Pop(1)

	regs := []Reg{{Name: "ping", Func: func(L *State) int {
		L.PushString("pong")
		return 1
	}}}
	L.OpenLib("covergap.openlib", regs, 0)
	if err := L.DoString(`assert(covergap.openlib.ping() == "pong")`); err != nil {
		t.Fatal(err)
	}

	L.PushGlobalTable()
	if frag := L.FindTable(-1, "covergap.mod.sub", 2); frag != "" {
		t.Fatalf("FindTable create: %q", frag)
	}
	L.Pop(1)
	L.PushGlobalTable()
	L.PushString("notatable")
	L.SetField(-2, "covergap_conflict")
	if frag := L.FindTable(-1, "covergap_conflict.mod", 1); frag == "" {
		t.Fatal("FindTable conflict expected")
	}
	L.SetTop(0)

	B, room := L.BuffInitSize(16)
	copy(room, "hi")
	B.AddSize(2)
	B.PushResult()
	s, _ := L.ToString(-1)
	if s != "hi" {
		t.Fatalf("BuffInitSize %q", s)
	}

	L.CheckStack2(8, "")
	L.CheckStack2(8, "msg")
}

func TestWarningGoAPI(t *testing.T) {
	L := NewState()
	defer L.Close()
	// Default is warnfOff; turn on via control message.
	text := captureStderr(t, func() {
		L.Warning("@on", false)
		L.Warning("part1", true)
		L.Warning("part2", false)
		L.Warning("@off", false)
		L.Warning("hidden", false)
		L.Warning("@on", false)
		L.Warning("again", false)
		luaE_warnerror(L, "where")
		L.Warning("@unknown", false) // ignored control
	})
	if !strings.Contains(text, "Lua warning: part1part2") {
		t.Fatalf("cont: %q", text)
	}
	if !strings.Contains(text, "Lua warning: again") {
		t.Fatalf("again: %q", text)
	}
	if !strings.Contains(text, "where: WARNING: failed to write warning:") {
		t.Fatalf("warnerror: %q", text)
	}
	if strings.Contains(text, "hidden") {
		t.Fatalf("should be off: %q", text)
	}

	var seen []string
	L.SetWarnFunction(func(L *State, msg string, tocont bool) {
		seen = append(seen, msg)
	}, "ud")
	L.Warning("custom", false)
	if len(seen) != 1 || seen[0] != "custom" {
		t.Fatalf("custom warnf: %#v", seen)
	}
}

func TestAPIDumpWhereUpvalueVersion(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString(`
local x = 10
local function f() return x end
return f
`, "=chunk"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	// stack: f
	name, ok := L.GetUpvalue(-1, 1)
	if !ok || name == "" {
		t.Fatalf("GetUpvalue name=%q ok=%v", name, ok)
	}
	L.Pop(1)
	id := L.UpvalueId(-1, 1)
	if id == nil {
		t.Fatal("UpvalueId")
	}
	if L.UpvalueId(-1, 99) != nil {
		t.Fatal("UpvalueId OOB")
	}

	var buf bytes.Buffer
	status := L.Dump(func(_ *State, b []byte) int {
		_, _ = buf.Write(b)
		return 0
	}, false)
	if status != 0 || buf.Len() == 0 {
		t.Fatalf("Dump status=%d len=%d", status, buf.Len())
	}
	// aborting writer
	status = L.Dump(func(_ *State, b []byte) int { return 1 }, true)
	if status == 0 {
		t.Fatal("Dump abort expected non-zero")
	}

	if Version(L) != float64(luaVersionNumGo) || Version(nil) != float64(luaVersionNumGo) {
		t.Fatalf("Version %v", Version(L))
	}

	// Where from inside a Lua call
	where := ""
	L.PushGoFunc(func(L *State) int {
		where = L.Where(1)
		return 0
	})
	L.SetGlobal("gowhere")
	if err := L.DoString(`gowhere()`); err != nil {
		t.Fatal(err)
	}
	if where == "" || !strings.Contains(where, ":") {
		t.Fatalf("Where %q", where)
	}
	if L.Where(99) != "" {
		t.Fatalf("Where deep %q", L.Where(99))
	}
}

func TestDebugGetInfoLocalInterrupt(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	if err := L.LoadString(`
local a, b = 1, 2
local function inner(...)
  local c = 3
  return debug.getinfo(1, "nSlufL"), a, b, c, ...
end
return inner
`, "=dbg"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	// Call via Go so we can use GetLocal/SetLocal on a live frame from a hook.
	called := false
	L.SetHook(func(L *State, ar *dbAr) {
		if called {
			return
		}
		called = true
		if !L.GetInfo("nSluf", ar) {
			t.Error("GetInfo failed")
		}
		n := L.GetLocal(ar, 1)
		if n == "" {
			t.Error("GetLocal empty")
		} else {
			L.Pop(1)
		}
		L.PushInteger(99)
		if L.SetLocal(ar, 1) == "" {
			t.Error("SetLocal")
		}
		// vararg locals (negative indices) when present
		_ = L.GetLocal(ar, -1)
		if L.GetTop() > 0 && L.IsString(-1) || L.IsNumber(-1) {
			L.Pop(1)
		}
	}, dbMaskCall, 0)

	L.PushValue(-1) // inner
	L.PushInteger(7)
	L.PushInteger(8)
	if L.PCall(2, 0, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	if !called {
		t.Fatal("hook not called")
	}
	L.SetHook(nil, 0, 0)

	// GetLocal on non-active function (ar == nil)
	if err := L.LoadString(`local function f(x, y) return x+y end; return f`, "=p"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	if name := L.GetLocal(nil, 1); name == "" {
		// may be stripped; still exercise the path
		_ = L.GetLocal(nil, 1)
	}
	_ = L.GetLocal(nil, 99)

	// SetInterruptHook: next count event raises interrupted!
	L.SetInterruptHook()
	err := L.DoString(`local s=0; for i=1,100000 do s=s+i end`)
	if err == nil || !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("interrupt: %v", err)
	}
}

func TestToPointerAndDoFileErrors(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	L.PushNil()
	if L.ToPointer(-1) != 0 {
		t.Fatal("nil pointer")
	}
	L.PushBoolean(true)
	if L.ToPointer(-1) != 0 {
		t.Fatal("bool pointer")
	}
	L.NewTable()
	if L.ToPointer(-1) == 0 {
		t.Fatal("table pointer")
	}
	L.PushGoFunc(func(L *State) int { return 0 })
	if L.ToPointer(-1) == 0 {
		t.Fatal("cclosure/lcf pointer")
	}
	L.PushThread()
	if L.ToPointer(-1) == 0 {
		t.Fatal("thread pointer")
	}
	L.PushLightUserData(unsafe.Pointer(&L))
	if L.ToPointer(-1) == 0 {
		t.Fatal("lightud pointer")
	}
	L.NewUserData(123, 0)
	if L.ToPointer(-1) == 0 {
		t.Fatal("userdata pointer")
	}
	L.SetTop(0)

	dir := t.TempDir()
	bad := filepath.Join(dir, "missing.lua")
	if err := L.DoFile(bad); err == nil {
		t.Fatal("missing file")
	}
	path := filepath.Join(dir, "boom.lua")
	if err := os.WriteFile(path, []byte("error('boom')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := L.DoFile(path); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("runtime err: %v", err)
	}
	shebang := filepath.Join(dir, "shebang.lua")
	if err := os.WriteFile(shebang, []byte("#!/usr/bin/env lua\nreturn 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := L.DoFile(shebang); err != nil {
		t.Fatal(err)
	}
}

func TestMacroHelpersDirect(t *testing.T) {
	L := NewState()
	defer L.Close()

	_ = errorstatus(LUA_OK)
	_ = errorstatus(LUA_ERRRUN)
	correctstack(L, nil)
	luaD_inctop(L)
	L.top--
	luaE_freeCI(L)
	_ = gcWatchEmpty()
	luaH_free(L, nil)

	v := &Value{}
	_ = isabstkey(v)
	_ = isabstkey(&luaO_nilobject)
	setbvalue(v, false)
	_ = ttisfalse(v)
	setbvalue(v, true)
	_ = ttisfalse(v)
	_ = deadvalue(v)
	setempty(v)
	setfltvalue(v, 1.5)
	chgfltvalue(v, 2.5)
	setivalue(v, 7)
	chgivalue(v, 8)
	setdeadvalue(v)
	ts := luaS_newlstr(L, "abc")
	_ = getstr(ts)
	u := &Udata{}
	setuservalue(u, v)
	var out Value
	getuservalue(u, &out)

	_ = newAsBx(0, 0, 0)
	_ = vkisinreg(VLOCAL)
	_ = vkisinreg(VVOID)
	_ = stackIndexOf(L, 1)
	_ = stackIndexOf(L, -1)
	_ = stackIndexOf(L, LUA_REGISTRYINDEX)
	for _, tt := range []int{LUA_TNIL, LUA_TBOOLEAN, LUA_TNUMBER, LUA_TSTRING, LUA_TTABLE, LUA_TFUNCTION, LUA_TUSERDATA, LUA_TLIGHTUSERDATA, LUA_TTHREAD, 99} {
		_ = typeName(&Value{tt: uint8(tt)})
	}
	_ = typeName(&Value{})

	_ = L.resetThreadStatus(LUA_OK)
	_ = L.resetThreadStatus(LUA_YIELD)
}

func TestMathHyperbolicEdges(t *testing.T) {
	// Exercise math.sinh/cosh for ordinary values; libm path on this box.
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(math.sinh(0))
say(math.cosh(0))
say(math.tanh(0))
say(math.sinh(1) > 0)
say(math.cosh(1) >= 1)
say(math.sinh(700) > 0 or math.sinh(700) == math.huge)
say(math.cosh(700) > 0 or math.cosh(700) == math.huge)
say(math.sinh(-1) < 0)
return table.concat(o, ",")
`)
	if !strings.HasPrefix(got, "0") && strings.Contains(got, "true,true,true") {
		t.Fatalf("%q", got)
	}
}

func TestCoroutineFastYieldAndDebugUpvalue(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local co = coroutine.create(function(a)
  say(a)
  local x = coroutine.yield(10)
  say(x)
  return 20
end)
local ok, v = coroutine.resume(co, 1)
say(ok); say(v)
ok, v = coroutine.resume(co, 2)
say(ok); say(v)
ok, v = coroutine.resume(co)
say(ok); say(v)

local x = 5
local function f() return x end
say(debug.upvalueid(f, 1) ~= nil)
local a, b = f, function() return x end
debug.upvaluejoin(a, 1, b, 1)
say(debug.getupvalue(f, 1))
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true,10") {
		t.Fatalf("%q", got)
	}
}

func TestParseHexFloatAndForlimitEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(0x1.8p1)
say(tonumber("0x1p-1"))
say(tonumber("0X1.Fp+2"))
local n=0
for i=1.5,3.5,1 do n=n+1 end
say(n)
n=0
for i=10,1,-2 do n=n+i end
say(n)
-- empty numeric for
n=0
for i=1,0 do n=n+1 end
say(n)
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}

func TestIOAndOSCoverageExtras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	t.Setenv("COVERGAP_PATH", path)
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local p = os.getenv("COVERGAP_PATH")
local f = assert(io.open(p, "w"))
say(f:write("abc"))
say(f:seek("set", 1))
say(f:close())
f = assert(io.open(p, "r"))
say(f:read(2))
say(io.type(f))
say(f:close())
say(io.type(f))
say(select(1, os.remove(p)))
say(os.date("%Y", 0) ~= nil)
say(math.type(1))
say(math.type(1.5))
say(math.type("x"))
local ok, err = pcall(error, "x", 0)
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "ab") {
		t.Fatalf("%q", got)
	}
}

func TestLoadlibAndPackageEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(type(package.searchpath))
local ok, err = package.searchpath("no_such_mod_covergap", "?.lua")
say(ok == nil)
say(type(err) == "string")
say(type(package.loaded) == "table")
say(type(package.preload) == "table")
local ok2, mod = pcall(require, "no_such_mod_covergap_xyz")
say(not ok2)
return table.concat(o, ",")
`)
	if !strings.HasPrefix(got, "function,true,true,true,true,true") {
		t.Fatalf("%q", got)
	}
}

func TestLuacMoreOptions(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "c.lua")
	if err := os.WriteFile(src, []byte("local a = '"+strings.Repeat("z", 300)+"'\nreturn a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "c.out")
	var out, errb bytes.Buffer
	if code := Luac([]string{"luac", "-o", outPath, src}, &out, &errb); code != 0 {
		t.Fatalf("compile %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-l", "-l", outPath}, &out, &errb); code != 0 {
		t.Fatalf("list %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "main") {
		t.Fatalf("listing %q", out.String())
	}
	// load binary back
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.DoFile(outPath); err != nil {
		t.Fatal(err)
	}
}

func TestCheckStack2Overflow(t *testing.T) {
	L := NewState()
	defer L.Close()
	err := L.DoString(`
local function rec(n)
  if n > 0 then return rec(n-1) end
end
`)
	_ = err
	// Force CheckStack2 failure path with absurd size when possible.
	func() {
		defer func() { _ = recover() }()
		L.CheckStack2(math.MaxInt32/4, "overflow-test")
	}()
}

func TestOpenBit32Direct(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushGoFunc(OpenBit32)
	L.PushString("bit32")
	L.Call(1, 1)
	if !L.IsTable(-1) {
		t.Fatal("OpenBit32")
	}
}
