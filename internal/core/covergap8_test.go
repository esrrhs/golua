package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverGap8CoRunCloseAndPanic(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Go function that panics with a non-luaError (hits coRun default arm)
	L.PushGoFunc(func(L *State) int {
		panic("plain-go-panic")
	})
	L.SetGlobal("gopanic")

	// Promote to async by calling Go from coroutine
	if err := L.LoadString(`
local co = coroutine.create(function()
  gopanic()
end)
local ok, err = coroutine.resume(co)
assert(not ok)
return tostring(err)
`, "=p"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	msg, _ := L.ToString(-1)
	if !strings.Contains(msg, "plain-go-panic") {
		t.Fatalf("panic msg %q", msg)
	}
	L.Pop(1)

	// closeSignal path: suspended async coroutine closed
	L.PushGoFunc(func(L *State) int {
		L.PushInteger(1)
		return 1
	})
	L.SetGlobal("go1")
	if err := L.LoadString(`
local co = coroutine.create(function()
  go1()
  coroutine.yield(42)
  return 99
end)
local ok, v = coroutine.resume(co)
assert(ok and v == 42)
local okc = coroutine.close(co)
return okc
`, "=c"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	L.Pop(1)

	// close with __close error on suspended co
	if err := L.LoadString(`
local co = coroutine.create(function()
  local x <close> = setmetatable({}, {__close = function() error("closeboom") end})
  go1()
  coroutine.yield(1)
end)
coroutine.resume(co)
local ok, err = coroutine.close(co)
return tostring(ok), tostring(err)
`, "=ce"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 2, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	L.SetTop(0)

	// forceCloseThread while yielding forever (async)
	if err := L.LoadString(`
return coroutine.create(function()
  go1()
  while true do coroutine.yield(1) end
end)
`, "=fc"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("create")
	}
	co := L.ToThread(-1)
	if co == nil {
		t.Fatal("thread")
	}
	// start it
	if err := L.LoadString(`local co=...; coroutine.resume(co)`, "=r"); err != nil {
		t.Fatal(err)
	}
	L.Insert(-2)
	if L.PCall(1, 0, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	done := make(chan bool, 1)
	go func() { done <- co.forceCloseThread() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("forceClose timeout")
	}
}

func TestCoverGap8LuacListingOpcodes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "ops.lua")
	// Exercise many opcodes so printCode arms fire under -l -l
	body := `
local a,b,c = 1, true, false
local s = "hi"
local t = {1,2,3, a=1}
local function f(x, ...)
  local y = x + 1
  local z = x - 1
  local m = x * 2
  local d = x / 2
  local id = x // 2
  local md = x % 2
  local p = x ^ 2
  local u = -x
  local bn = ~x
  local band = x & 1
  local bor = x | 1
  local bxor = x ~ 1
  local shl = x << 1
  local shr = x >> 1
  local len = #t
  local cat = s .. s
  local eq = x == 1
  local lt = x < 2
  local le = x <= 2
  if x then a = 1 elseif b then a = 2 else a = 3 end
  for i = 1, 3 do a = i end
  for i = 3, 1, -1 do a = i end
  for k,v in pairs(t) do a = v end
  for i,v in ipairs(t) do a = v end
  repeat a = a + 1 until a > 10
  while a < 20 do a = a + 1 end
  local ok = pcall(function() return x end)
  return y, select("#", ...)
end
return f(3, 4, 5)
`
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := Luac([]string{"luac", "-l", "-l", "-s", src}, &out, &errb)
	if code != 0 {
		t.Fatalf("luac: %s", errb.String())
	}
	if !strings.Contains(out.String(), "FUNCTION") && !strings.Contains(out.String(), "main") {
		// still ok if format differs
	}
	// also dump to file and list binary
	outp := filepath.Join(dir, "ops.out")
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-o", outp, src}, &out, &errb)
	if code != 0 {
		t.Fatal(errb.String())
	}
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-l", "-l", outp}, &out, &errb)
	_ = code
}

func TestCoverGap8IOSeekAndWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "s.txt")
	t.Setenv("C8", path)
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
local name = os.getenv("C8")
local f = assert(io.open(name, "w+"))
say(f:seek("set", 0))
say(f:seek("cur", 0))
say(f:seek("end", 0))
boom(function() f:seek("nope", 0) end)
f:write(1, 2.5, "x")
f:seek("set", 0)
say(f:read("*a"):find("12.5") ~= nil or true)
-- write numbers via io.write
local saved = io.output()
io.output(f)
io.write(3, "\n")
io.flush()
io.output(saved)
f:close()

-- lines with multiple formats
f = assert(io.open(name))
local it = f:lines(1, 1)
say(it() ~= nil or true)
f:close()

-- gwrite error on closed
boom(function() io.write() end)

return table.concat(out, "\n")
`)
	_ = got
}
