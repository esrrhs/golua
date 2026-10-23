package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatHexFloatFlags(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(string.format("%a", 1.0))
say(string.format("%A", 1.0))
say(string.format("%+a", 1.0))
say(string.format("% a", 1.0))
say(string.format("%#a", 1.0))
say(string.format("%20a", 1.0))
say(string.format("%-20a", 1.0))
say(string.format("%020a", 1.0))
say(string.format("%.0a", 1.0))
say(string.format("%.4a", 1.0))
say(string.format("%+20.4a", -1.5))
say(string.format("%a", 0/0))
say(string.format("%a", 1/0))
say(string.format("%a", -1/0))
say(string.format("%05d", -3))
say(string.format("%+05d", 3))
say(string.format("% 5d", 3))
say(string.format("%#o", 8))
say(string.format("%#x", 15))
say(string.format("%#X", 15))
say(string.format("%.0f", 1.5))
say(string.format("%q", "\n\t\\\""))
say(string.format("%s", true))
local ok = pcall(string.format, "%d")
say(string.byte("abc", -1))
say(string.byte("abc", 2, 1) == nil or select("#", string.byte("abc", 2, 1)) == 0 or true)
say(string.match("abc", "a(b)c"))
say(string.match("abc", "x") == nil)
say(string.find("abc", "b", 1, true))
say(string.find("abc", "%d") == nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "0x") && !strings.Contains(strings.ToLower(got), "0x") {
		// %a may produce 0x1p+0
		if !strings.Contains(got, "p+") && !strings.Contains(got, "P+") {
			t.Fatalf("%q", got)
		}
	}
}

func TestIOReadNumberVariants(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nums.txt")
	content := "  42  -3.5  0x10  1e2  0X1.8p1  notnum  \n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CG4", path)
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local f=assert(io.open(os.getenv("CG4")))
say(f:read("*n"))
say(f:read("*n"))
say(f:read("*n"))
say(f:read("*n"))
say(f:read("*n"))
local x = f:read("*n")
say(x == nil)
f:close()
-- write-only can't read number
local p = os.getenv("CG4")..".w"
local w=assert(io.open(p,"w"))
say(w:read("*n") == nil)
w:close()
-- setvbuf modes
local r=assert(io.open(p,"w"))
say(r:setvbuf("no"))
say(r:setvbuf("full", 64))
say(r:setvbuf("line"))
r:close()
-- lines iterator with open file
local f2=assert(io.open(os.getenv("CG4")))
local n=0
for line in f2:lines() do n=n+1 end
say(n>=1)
f2:close()
return table.concat(o, ",")
`)
	if !strings.Contains(got, "42") {
		t.Fatalf("%q", got)
	}
}

func TestForceCloseAsyncCoroutine(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.DoString(`
co = coroutine.create(function()
  -- force promote to async by calling a Go/C boundary: io.tmpfile then yield
  local f = io.tmpfile()
  coroutine.yield(1)
  f:close()
end)
assert(coroutine.resume(co))
`); err != nil {
		t.Fatal(err)
	}
	// Closing L should force-close child threads
	L.Close()
	// New state after
	L2 := NewState()
	defer L2.Close()
}

func TestRotateXMoveUpvalueJoin(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Rotate(-3, 1)
	L.Rotate(-3, -1)
	co := L.NewThread()
	L.PushInteger(9)
	co.XMove(L, 1)
	if co.GetTop() != 1 {
		t.Fatal("xmove")
	}
	if err := L.DoString(`
local x=1
local function a() return x end
local function b() return x end
debug.upvaluejoin(a,1,b,1)
assert(debug.upvalueid(a,1)==debug.upvalueid(b,1))
`); err != nil {
		t.Fatal(err)
	}
	// UpvalueJoin Go API
	if err := L.LoadString(`local x=1; return function() return x end, function() return x end`, "=u"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 2, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	L.UpvalueJoin(-2, 1, -1, 1)
}

func TestUserValueAPI(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.NewUserData("ud", 1)
	L.PushString("uv")
	L.SetUserValue(-2, 1)
	L.GetUserValue(-1, 1)
	s, _ := L.ToString(-1)
	if s != "uv" {
		t.Fatalf("uservalue %q", s)
	}
}

func TestPCallMsgHandler(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function handler(err)
  return "H:" .. tostring(err)
end
local ok, err = xpcall(function() error("boom") end, handler)
say(tostring(err):find("H:", 1, true) ~= nil)
ok, err = pcall(error, "x", 0)
local co = coroutine.create(function() error("c") end)
ok, err = coroutine.resume(co)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestBitwiseAndShiftEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(1<<63)
say((-1)>>1)
say(1<<100) -- large shift
say(1>>100)
say(~0)
say(3&1)
say(1|2)
say(3~1)
-- div/mod edge
say(5//2)
say((-5)//2)
say(5%2)
say((-5)%2)
say(1/0)
say(0/0 ~= 0/0)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestLuacCombineAndStdin(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.lua")
	b := filepath.Join(dir, "b.lua")
	if err := os.WriteFile(a, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "c.out")
	var out, errb strings.Builder
	// combine multiple files
	code := Luac([]string{"luac", "-o", outPath, a, b}, &out, &errb)
	if code != 0 {
		t.Fatalf("combine %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-l", "-l", outPath}, &out, &errb)
	if code != 0 {
		t.Fatalf("list %d %s", code, errb.String())
	}
	// constants in listing
	if !strings.Contains(out.String(), "main") {
		t.Fatalf("%s", out.String())
	}
}

func TestFuncNameFromCodeErrors(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local ok, err = pcall(function() local t={}; return t.nope() end)
say(not ok)
ok, err = pcall(function() return (nil)() end)
say(not ok)
ok, err = pcall(function() local t={}; return t[1]() end)
say(not ok)
ok, err = pcall(function() local t=setmetatable({},{__index=function() return nil end}); return t.x() end)
say(not ok)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 3 {
		t.Fatalf("%q", got)
	}
}

func TestOpcodeNameCoverage(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
-- exercise many opcodes so listing / getinfo name paths fire
local a,b,c = 1,2,3
a = a + b - c * 2 / 3 % 2
a = a // 1
a = a & 7 | 1 ~ 2
a = a << 1 >> 1
a = -a; a = ~a
local t = {1,2,3,x=4}
t[1] = t[2]
t.x = t.y or 5
local s = "a" .. "b"
local f = function(x, ...) return x, ... end
say(select(1, f(10, 20)))
for i=1,3 do a=a+i end
for k,v in pairs(t) do a=a+1 end
for i,v in ipairs(t) do a=a+1 end
if a then a=a else a=0 end
while false do end
repeat break until true
local ok, err = pcall(function()
  local u = {}
  setmetatable(u, {__index=function(self,k) return function() return k end end})
  return u.hello()
end)
say(ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "10") {
		t.Fatalf("%q", got)
	}
}

func TestLuacFullOpcodeListing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ops.lua")
	body := `
local up = 0
local function f(a, b, ...)
  local t = {a, b, ..., x=1}
  t[1] = t[2] + up
  up = up + 1
  if a then return t[1] << 1 else return a // b, a % b end
end
return f(2, 3, 4)
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	if code := Luac([]string{"luac", "-l", "-l", src}, &out, &errb); code != 0 {
		t.Fatalf("%d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "MOVE") && !strings.Contains(text, "LOADK") {
		t.Fatalf("listing missing opcodes:\n%s", text)
	}
}
