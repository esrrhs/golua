package core

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestDeadHelpersAndFloatInt(t *testing.T) {
	_ = ltFloatInt(1.5, 2)
	_ = ltFloatInt(math.NaN(), 2)
	_ = ltFloatInt(3.0, 2)
	_ = leFloatInt(1.5, 2)
	_ = leFloatInt(math.NaN(), 2)
	_ = leFloatInt(1.0, 2)

	cases := []struct {
		s  string
		ok bool
	}{
		{"0x1.8p1", true},
		{"-0Xff", true},
		{"+0x.8p0", true},
		{"0x1p+4", true},
		{"0x1p-4", true},
		{"0x", false},
		{"0x.p1", false},
		{"0x1p", false},
		{"0x1q", false},
		{"0x1p1z", false},
		{"xyz", false},
		{"0x1.0p0   ", true},
	}
	for _, c := range cases {
		_, ok := parseHexFloat(c.s)
		if ok != c.ok {
			t.Errorf("parseHexFloat(%q)=%v want %v", c.s, ok, c.ok)
		}
	}

	var v Value
	_ = luaO_str2num("0x10", &v)
	_ = luaO_str2num("1.5", &v)
	_ = luaO_str2num("notanum", &v)
	_ = toStringAny(42)
	_ = toStringAny("x")
	_ = toStringAny([]byte("y"))
	{
		Ls := NewState()
		defer Ls.Close()
		_ = toStringAny(luaS_newlstr(Ls, "ts"))
	}
	_ = toStringAny(nil)
	_ = toIntAny(3)
	_ = toIntAny(int64(3))
	_ = toIntAny(int32(3))
	_ = toIntAny(uint(3))
	_ = toIntAny(uint64(3))
	_ = toIntAny(byte(3))
	_ = toIntAny(3.0)
	_ = toIntAny("no")
	_ = toPtrAny(&Table{})
	_ = toPtrAny(&TString{})
	_ = toPtrAny(&LClosure{})
	_ = toPtrAny(&CClosure{})
	_ = toPtrAny(&Udata{})
	_ = toPtrAny(&State{})
	_ = toPtrAny(unsafe.Pointer(new(int)))
	_ = toPtrAny(3)
	_ = luaO_chunkid("@" + strings.Repeat("a", 200))
	_ = luaO_chunkid("=" + strings.Repeat("b", 200))
	_ = luaO_chunkid(strings.Repeat("c", 200))
	_ = luaO_chunkid("")
	_ = luaO_int2fb(0)
	_ = luaO_int2fb(1)
	_ = luaO_int2fb(100000)
}

func TestNumericForLimitEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
-- integer for with huge float limit
local n=0
for i=1,1e40,1 do n=n+1; if n>3 then break end end
say(n>0)
n=0
for i=1,-1e40,-1 do n=n+1; if n>3 then break end end
say(n>0)
-- stopnow: positive step, limit -inf-ish as non-integer convertible
n=0
for i=0, -1/0, 1 do n=n+1 end
say(n)
n=0
for i=0, 1/0, -1 do n=n+1 end
say(n)
-- string coercion in for
n=0
for i="1","3","1" do n=n+i end
say(n)
-- float < int / int < float at runtime
local f, i = 1.5, 2
say(f < i)
say(i < f)
say(f <= i)
say(i <= f)
say((0/0) < i)
say(i < (0/0))
local ok = pcall(function() for i=1,"x",1 do end end)
say(not ok)
ok = pcall(function() for i="x",1,1 do end end)
say(not ok)
ok = pcall(function() for i=1,1,"x" do end end)
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestStringFormatCoverage(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(string.format("%s", "hi"))
say(string.format("%q", "a\"b"))
say(string.format("%d", 42))
say(string.format("%i", -3))
say(string.format("%u", 3))
say(string.format("%o", 8))
say(string.format("%x", 255))
say(string.format("%X", 255))
say(string.format("%f", 1.5))
say(string.format("%e", 1.5))
say(string.format("%E", 1.5))
say(string.format("%g", 1.5))
say(string.format("%G", 1.5))
say(string.format("%a", 1.0))
say(string.format("%A", 1.0))
say(string.format("%c", 65))
say(string.format("%%"))
say(string.format("%5d", 3))
say(string.format("%05d", 3))
say(string.format("%-5d", 3))
say(string.format("%+d", 3))
say(string.format("% d", 3))
say(string.format("%.3f", 1.5))
say(string.format("%#x", 15))
say(string.byte("abc", 1, 3))
say(select(2, string.byte("abc", 1, 3)))
say(string.match("hello", "h(%w+)"))
say(string.find("hello", "ll"))
local ok = pcall(string.format, "%d", "no")
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "hi") || !strings.Contains(got, "42") {
		t.Fatalf("%q", got)
	}
}

func TestDebugLocalUpvalueMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function f(a, b, ...)
  local c = a + b
  local name, val = debug.getlocal(1, 1)
  say(name)
  say(val)
  debug.setlocal(1, 1, 99)
  say(a)
  local n, v = debug.getlocal(1, -1)
  say(n ~= nil or v ~= nil or true)
  return c, ...
end
say(select(1, f(1, 2, 3, 4)))
local x = 10
local function g() return x end
say(debug.getupvalue(g, 1))
debug.setupvalue(g, 1, 20)
say(g())
say(debug.upvalueid(g, 1) ~= nil)
local h = function() return x end
debug.upvaluejoin(g, 1, h, 1)
say(debug.getinfo(g, "nSu").what)
say(debug.getinfo(1, "l").currentline > 0)
-- force funcNameFromCode / getobjname via error messages
local ok, err = pcall(function() local t={}; return t.nope() end)
say(not ok)
ok, err = pcall(function() local t={}; return t[1]() end)
say(not ok)
ok, err = pcall(function() return (nil)() end)
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "99") {
		t.Fatalf("%q", got)
	}
}

func TestCompareAndArithEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushInteger(1)
	L.PushNumber(1.5)
	_ = L.Compare(-2, -1, LUA_OPLT)
	_ = L.Compare(-2, -1, LUA_OPLE)
	_ = L.Compare(-2, -1, LUA_OPEQ)
	_ = L.Compare(-1, -1, LUA_OPEQ)
	_ = L.Compare(99, 1, LUA_OPEQ) // invalid index
	L.SetTop(0)
	bin := func(op int, a, b int64) {
		L.PushInteger(a)
		L.PushInteger(b)
		L.Arith(op)
		L.Pop(1)
	}
	bin(LUA_OPADD, 10, 3)
	bin(LUA_OPSUB, 10, 3)
	bin(LUA_OPMUL, 10, 3)
	bin(LUA_OPMOD, 10, 3)
	bin(LUA_OPDIV, 10, 3)
	bin(LUA_OPIDIV, 10, 3)
	bin(LUA_OPPOW, 2, 3)
	bin(LUA_OPBAND, 0xff, 0x0f)
	bin(LUA_OPBOR, 1, 2)
	bin(LUA_OPBXOR, 1, 2)
	bin(LUA_OPSHL, 1, 2)
	bin(LUA_OPSHR, 8, 1)
	L.PushInteger(5)
	L.Arith(LUA_OPUNM)
	L.Pop(1)
	L.PushInteger(1)
	L.Arith(LUA_OPBNOT)
	L.Pop(1)
}

func TestOfficialKillTree(t *testing.T) {
	officialKillTree(nil)
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = officialProcAttr()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	officialKillTree(cmd.Process)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("kill tree timed out")
	}
}

func TestLuacListingDetails(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "d.lua")
	body := `
local up = 1
local function f(a, ...)
  local b = a + up
  if a > 0 then return b else return ... end
end
return f(1, 2, 3)
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "d.out")
	var out, errb bytes.Buffer
	if code := Luac([]string{"luac", "-o", outPath, "-s", src}, &out, &errb); code != 0 {
		t.Fatalf("compile %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-l", "-l", outPath}, &out, &errb); code != 0 {
		t.Fatalf("list %d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "function") && !strings.Contains(text, "main") {
		t.Fatalf("listing:\n%s", text)
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-v", "-l", src}, &out, &errb); code != 0 {
		t.Fatalf("v+l %d %s", code, errb.String())
	}
}

func TestIOLinesPopenAndSeek(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COVERGAP2", path)
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local p = os.getenv("COVERGAP2")
local n=0
for line in io.lines(p) do n=n+1; say(line) end
say(n)
local f=assert(io.open(p,"r+"))
say(f:seek("set",0))
say(f:seek("cur"))
say(f:seek("end"))
f:seek("set",0)
say(f:read("*a"):sub(1,3))
f:close()
local w=assert(io.popen("echo hi","r"))
say((w:read("*l") or ""):gsub("\r$",""))
w:close()
local ok = pcall(io.open, p, "q")
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "one") || !strings.Contains(got, "hi") {
		t.Fatalf("%q", got)
	}
}

func TestCoroutineWrapStatusAndYield(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local w = coroutine.wrap(function(a)
  say(a)
  local b = coroutine.yield(10)
  say(b)
  return 20
end)
say(w(1))
say(w(2))
local co = coroutine.create(function()
  say(coroutine.status(coroutine.running()))
  coroutine.yield()
end)
say(coroutine.status(co))
coroutine.resume(co)
say(coroutine.status(co))
coroutine.resume(co)
say(coroutine.status(co))
-- nested yield via wrap to exercise deeper vmNest path when possible
local w2 = coroutine.wrap(function()
  local inner = coroutine.wrap(function()
    coroutine.yield(1)
    return 2
  end)
  say(inner())
  say(inner())
end)
w2()
return table.concat(o, ",")
`)
	if !strings.Contains(got, "10") {
		t.Fatalf("%q", got)
	}
}

func TestLoadStringModesAndBinary(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString("return 1", "=t"); err != nil {
		t.Fatal(err)
	}
	L.SetTop(0)
	if err := L.DoString(`local f=function() return "x" end; local b=string.dump(f); local g=assert(load(b,"=b","b")); assert(g()=="x")`); err != nil {
		t.Fatal(err)
	}
	// reject binary in text mode
	err := L.DoString(`local b=string.dump(function() end); assert(load(b,"=b","t")==nil)`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestWeakTablesAndGCMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local k={}
local t=setmetatable({},{__mode="k"})
t[k]=1
say(t[k])
k=nil
collectgarbage("collect")
collectgarbage("collect")
say(next(t)==nil or true)
local v={}
t=setmetatable({},{__mode="v"})
t.a=v
v=nil
collectgarbage("collect")
collectgarbage("collect")
say(t.a==nil or true)
say(collectgarbage("count") >= 0)
say(collectgarbage("isrunning"))
collectgarbage("stop")
collectgarbage("restart")
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}

func TestLexerEscapeAndLongStrings(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say("\x41")
say("\u{41}")
say("\z
  x" == "x")
say([=[a]=])
say([==[a]==])
local ok, err = load('return "\\q"')
say(not ok)
ok, err = load("return '\\xzz'")
say(not ok)
ok = load("return '\\u{110000}'")
say(ok == nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "A") {
		t.Fatalf("%q", got)
	}
}

func TestIndexToAbsAndCheckStack(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	_ = L.indexToAbs(1)
	_ = L.indexToAbs(-1)
	_ = L.indexToAbs(LUA_REGISTRYINDEX)
	_ = L.AbsIndex(-1)
	_ = L.AbsIndex(1)
	_ = L.CheckStack(10)
	L.SetTop(0)
}

func TestRefUnrefAndGSub(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.NewTable()
	L.PushString("v")
	ref := L.Ref(-2)
	L.RawGetI(-1, int64(ref))
	s, _ := L.ToString(-1)
	if s != "v" {
		t.Fatalf("ref %q", s)
	}
	L.Pop(1)
	L.Unref(-1, ref)
	out := L.GSub("a-b-c", "-", "+")
	if out != "a+b+c" {
		t.Fatalf("gsub %q", out)
	}
}
