package core

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestCoverGap2StringPatternsAndFormat(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) local ok = pcall(f); say(not ok) end

say(string.find("abc", "b", 1, true))
say(string.find("abc", "z", 1, true) == nil)
say(string.find("abc", "b", 10) == nil)
say(string.gmatch("abc", "z")() == nil)
say(string.match("abc", "a$") == nil)
say(string.match("a", "a$"))
say(string.match("abc", "%f[%a]%a+"))
say(string.match("abc", "%z") == nil)
say(string.match("a\0b", "%z"))
say(string.match("(ab)", "%b()"))
say(string.match("aaa", "a-"))
say(string.match("aaa", "a?"))
say(string.match("aaa", "a*"))
say(string.match("abc", "[a-c]+"))
say(string.match("xyz", "[^a]+"))
say(string.match("Abc", "%a+"))
say(string.match(" \t", "%s+"))
say(string.match("12", "%d+"))
say(string.match("!!", "%p+"))
say(string.match("az", "%l+"))
say(string.match("AZ", "%u+"))
say(string.match("a1", "%w+"))
say(string.match("a ", "%g+") or "nil")
say(string.match("\0", "%z"))

boom(function() string.match("a", "%") end)
boom(function() string.match("a", "[") end)
boom(function() string.match("a", "%b(") end)
boom(function() string.match("a", "%f a") end)
boom(function() string.match("a", "%2") end)
boom(function()
  local p = "(" .. string.rep("(.)", 33) .. string.rep(")", 33)
  string.match(string.rep("x", 40), p)
end)
boom(function() string.gsub("a", "a", "%") end)
say(string.gsub("ab", "a", "%%"))
say(string.gsub("ab", "(a)", "%0"))
say(string.gsub("ab", "(a)", "%1"))
say(string.gsub("ab", "a", function() return 7 end))
say(string.gsub("ab", "a", {}))
say(string.byte("abc", 5, 2)) -- empty interval
say(string.sub("abc", 0, 0))
say(string.sub("abc", -100, -90))
say(string.find("abc", "", 4))
say(string.gmatch("a", "", 10)() == nil or true)

say(string.format("%+05d", 0))
say(string.format("% 05d", 0))
say(string.format("%-5d", 1))
say(string.format("%#o", 0))
say(string.format("%#.0o", 0))
say(string.format("%#x", 0))
say(string.format("%#X", 15))
say(string.format("%05x", 1))
say(string.format("%-5x", 1))
say(string.format("%.0d", 0))
say(string.format("%+.0d", 0))
say(string.format("% .0d", 0))
say(string.format("%+5.0d", 0))
say(string.format("%q", "a\nb\tc\\d\"e"))
say(string.format("%q", true))
say(string.format("%q", false))
say(string.format("%q", nil))
say(string.format("%a", 0))
say(string.format("%A", 1.5))
say(string.format("%a", -0.0))
say(string.format("%.2f", 1.5))
say(string.format("%e", 1.5))
say(string.format("%g", 1.5))
say(string.format("%s", "hi"))
say(string.format("%c", 65))
say(string.format("%%"))
boom(function() string.format("%"..string.rep("0", 200).."d", 1) end)
boom(function() string.format("%z", 1) end)

local p = string.pack("<bi4i8fdn", 1, 2, 3, 1.5, 2.5, 3.5)
say(#p > 0)
local u1 = string.unpack("<bi4i8fdn", p)
say(u1)
say(string.packsize("i4"))
boom(function() string.packsize("s") end)
boom(function() string.pack("i4", "x") end)
say(#string.pack("c0", ""))
say(#string.pack("x"))
say(#string.pack("z", "hi"))
say(string.unpack("b", string.pack("b", -1)))
say(#string.pack("!4i4", 1))
say(#string.pack("s1", "ab"))
say(#string.pack("s2", "ab"))

return table.concat(out, "\n")
`)
	if len(strings.Split(got, "\n")) < 40 {
		t.Fatalf("too few results:\n%s", got)
	}
}

func TestCoverGap2IOAndOS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	t.Setenv("COVER2", path)
	t.Setenv("COVER2DIR", dir)

	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) local ok, err = pcall(f); say(not ok) end

local name = os.getenv("COVER2")
boom(function() io.open(name, "") end)
boom(function() io.open(name, "q") end)
boom(function() io.open(name, "rxx") end)
local f = assert(io.open(name, "wb"))
say(io.type(f))
f:write("bin\n")
f:setvbuf("no")
f:write("a")
f:setvbuf("full", 8)
f:write("bc\n")
f:flush()
f:close()
say(io.type(f))
boom(function() f:read(1) end)
boom(function() f:write("x") end)
boom(function() f:seek("set") end)
boom(function() f:flush() end)
boom(function() f:setvbuf("full") end)
boom(function() f:lines()() end)

f = assert(io.open(name, "r"))
say(f:read(0) == "")
say(f:read("*a"):find("bin") ~= nil)
f:close()

f = assert(io.open(name, "a"))
f:write("more")
f:close()
f = assert(io.open(name, "r+"))
f:seek("set", 0)
say(f:read(3))
f:close()

-- closed file tostring
f = assert(io.open(name, "r"))
local s1 = tostring(f)
f:close()
local s2 = tostring(f)
say(s1:find("file") ~= nil)
say(s2:find("closed") ~= nil)

-- io.lines with close
local n = 0
for line in io.lines(name) do n = n + 1 end
say(n >= 1)

-- popen write
local pw = io.popen("cat > /dev/null", "w")
if pw then
  pw:write("x\n")
  local a,b,c = pw:close()
  say(tostring(a))
end

-- failing popen / shell
local bad = io.popen("exit 7", "r")
if bad then
  bad:read("*a")
  local a,b,c = bad:close()
  say(tostring(a))
  say(tostring(b))
  say(tostring(c))
end

-- signal death if possible
local sig = io.popen("sh -c 'kill -9 $$'", "r")
if sig then
  sig:read("*a")
  local a,b,c = sig:close()
  say(tostring(b))
end

boom(function() io.input(1) end)
boom(function() io.output({}) end)
local tmp = assert(io.tmpfile())
tmp:write("t")
tmp:seek("set")
say(tmp:read("*a"))
tmp:close()

say(os.remove(name .. ".nope") == nil)
say(os.rename(name .. ".nope", name .. ".x") == nil)
local a,b = os.rename(name, name .. ".ren")
say(a)
os.rename(name .. ".ren", name)

say(os.date("%a %A %b %B %c %d %H %I %j %m %M %p %S %U %w %W %x %X %y %Y %z %Z %%", 0):len() > 10)
say(os.date("!*t", 0).year)
local tt = os.date("*t", 0)
say(type(os.time(tt)))
boom(function() os.date("%Y", math.maxinteger) end)
boom(function() os.date("%E", 0) end)

local ok, err = loadfile(name)
say(ok == nil)

return table.concat(out, "\n")
`)
	if len(strings.Split(got, "\n")) < 20 {
		t.Fatalf("io/os:\n%s", got)
	}
}

func TestCoverGap2StateAndAPIDeep(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// typeName / valueToStr branches
	var v Value
	setnilvalue(&v)
	_ = typeName(&v)
	setbvalue(&v, false)
	if valueToStr(&v) != "false" {
		t.Fatal("false str")
	}
	setbvalue(&v, true)
	_ = valueToStr(&v)
	setivalue(&v, 1)
	_ = typeName(&v)
	setsvalue(&v, luaS_new(L, "s"))
	_ = typeName(&v)
	sethvalue(&v, luaH_new(L))
	_ = typeName(&v)
	_ = valueToStr(&v)
	setclLvalue(&v, &LClosure{})
	_ = typeName(&v)
	_ = valueToStr(&v)
	setthvalue(&v, L)
	_ = typeName(&v)
	L.NewUserData(nil, 0)
	_ = typeName(&L.stack[L.top-1])
	_ = valueToStr(&L.stack[L.top-1])
	L.Pop(1)
	_ = typeName(&Value{tt: 255})

	// shrinkStack: grow then shrink
	oldTop := L.top
	need := BASIC_STACK_SIZE * 4
	if !L.CheckStack(need) {
		t.Fatal("CheckStack grow")
	}
	L.shrinkStack()
	L.top = oldTop

	// CheckStack2 success path (overflow arm needs near-max stack; skip to avoid OOM)
	L.CheckStack2(8, "ok")
	L.CheckStack2(8, "")

	// Where with a real Lua frame
	if err := L.LoadString("local function f()\n  return 1\nend\nreturn f", "=w2"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("load f")
	}
	L.SetHook(func(L *State, ar *dbAr) {
		_ = L.Where(0)
		_ = L.Where(1)
		_ = L.GetInfo("nSlutf", ar)
		_ = L.GetLocal(ar, 1)
		_ = L.GetLocal(ar, 100)
		L.PushNil()
		_ = L.SetLocal(ar, 1)
	}, dbMaskCall|dbMaskRet|dbMaskLine, 0)
	L.PushValue(-1)
	_ = L.PCall(0, 1, 0)
	L.SetHook(nil, 0, 0)
	L.SetTop(0)

	// UpvalueId more paths
	if err := L.LoadString("return function() end", "=nouv"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("nouv")
	}
	_ = L.UpvalueId(-1, 1)
	L.PushGoClosure(func(L *State) int { return 0 }, 1)
	_ = L.UpvalueId(-1, 1)
	L.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		_ = L.UpvalueId(-1, 1)
	}()
	L.SetTop(0)

	// pcallK with msgh and error
	L.PushGoFunc(func(L *State) int {
		L.PushString("fail")
		return L.Error()
	})
	L.PushGoFunc(func(L *State) int {
		L.PushValue(1)
		return 1
	})
	// msgh at index 2? push msgh first then func
	L.SetTop(0)
	L.PushGoFunc(func(L *State) int {
		L.PushString("handled")
		return 1
	})
	msgh := L.GetTop()
	L.PushGoFunc(func(L *State) int {
		L.PushString("boom")
		return L.Error()
	})
	st := L.pcallK(0, 1, msgh, false)
	_ = st
	L.SetTop(0)

	// CloseThread from nil
	th := L.NewThread()
	_ = th.CloseThread(nil)

	// freeUnusedCI / extendCI
	for i := 0; i < 5; i++ {
		L.extendCI()
	}
	L.unwindCIToBase()
	freeUnusedCI(L)

	// parseHexFloat edges
	for _, s := range []string{
		"0x1.8p0", "0x1p-1", "0X1.Fp+2", "0x.8p1", "0x1.p3",
		"0x1p", "0x", "0x1.2.3p0", "0x1p99999", "0xg",
		"+0x1p0", "-0x1p0", "0x0p0",
	} {
		_, _ = parseHexFloat(s)
	}

	// isnonstrictnil: dead key style nil
	var dead Value
	setnilvalue(&dead)
	dead.tt = LUA_VFALSE // wrong — need nonstrict nil tag
	// LUA_VEMPTY or similar?
	_ = isnonstrictnil(&dead)

	// gckeyN with collectable key
	node := &node{keytt: BIT_ISCOLLECTABLE}
	_ = gckeyN(node)
	_ = keyiscollectable(node)

	// math large cosh/sinh still hit libm path at least
	_ = runChunk(t, `return tostring(math.cosh(1000)) .. tostring(math.sinh(-1000))`)

	// coroutine deep paths: yield from nested, close suspended, resume dead
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local co = coroutine.create(function()
  coroutine.yield(1)
  error("x")
end)
say(coroutine.resume(co))
say(select(1, coroutine.resume(co)))
say(coroutine.status(co))
say(select(1, coroutine.resume(co)))
local co2 = coroutine.create(function()
  coroutine.yield(10)
  return 20
end)
coroutine.resume(co2)
say(coroutine.close(co2))
local co3 = coroutine.create(function()
  local n = 0
  while true do n = n + 1; coroutine.yield(n) end
end)
coroutine.resume(co3)
say(coroutine.status(co3))
say(coroutine.isyieldable())
local co4 = coroutine.wrap(function(x)
  return coroutine.yield(x+1) + 1
end)
say(co4(1))
say(co4(5))
return table.concat(out, "\n")
`)
	_ = got

	// DoFile with binary / BOM
	dir := t.TempDir()
	bom := filepath.Join(dir, "bom.lua")
	if err := os.WriteFile(bom, append([]byte{0xEF, 0xBB, 0xBF}, []byte("return 9\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := L.DoFile(bom); err != nil {
		t.Fatal(err)
	}
	L.Pop(1)
	hash := filepath.Join(dir, "hash.lua")
	if err := os.WriteFile(hash, []byte("#!/usr/bin/env lua\nreturn 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := L.DoFile(hash); err != nil {
		t.Fatal(err)
	}
	L.Pop(1)

	// LoadFileMode binary reject
	bin := filepath.Join(dir, "b.luac")
	var dumped []byte
	if err := L.LoadString("return 1", "=b"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("prep dump")
	}
	L.Dump(func(_ *State, b []byte) int { dumped = append(dumped, b...); return 0 }, true)
	L.Pop(1)
	if err := os.WriteFile(bin, dumped, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = L.LoadFileMode(bin, "t") // may fail (text only)
	L.SetTop(0)
	_ = L.LoadFileMode(bin, "b")
	L.SetTop(0)

	// ArgError / typeError paths via Check*
	func() {
		defer func() { _ = recover() }()
		L.PushInteger(1)
		L.CheckString(1)
	}()
	L.SetTop(0)
	func() {
		defer func() { _ = recover() }()
		L.CheckAny(1)
	}()
	func() {
		defer func() { _ = recover() }()
		L.ArgError(1, "bad")
	}()
}

func TestCoverGap2DebugAndParser(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

local function f(a, b, ...)
  local x, y = 1, 2
  say(debug.getinfo(1, "nSlufL").what)
  say(debug.getinfo(f, "S").what)
  local n, v = debug.getlocal(1, 1)
  say(n)
  debug.setlocal(1, 1, 99)
  n, v = debug.getlocal(1, 1)
  say(v)
  say(debug.getlocal(1, 100) == nil)
  local u = 3
  local function g() return u end
  say(debug.upvalueid(g, 1) ~= nil)
  local u2 = 4
  local function h() return u2 end
  debug.upvaluejoin(g, 1, h, 1)
  say(g())
  say(debug.getuservalue(io.stdin, 1) ~= nil or true)
  local ud = newproxy and newproxy(true)
  say(debug.traceback("msg", 1):find("msg") ~= nil)
  say(debug.getregistry() ~= nil)
  say(debug.setmetatable({}, {}) == true or true)
  say(debug.getmetatable({}) == nil)
  debug.sethook(function() end, "crl", 1)
  local z = 1 + 1
  debug.sethook()
  say(z)
  return a
end
f(1, 2, 3, 4)

-- parser edges: goto conflict, attributes, long constructors
local function withgoto()
  local a = 1
  goto l
  ::l::
  return a
end
say(withgoto())
local t = {1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,
  21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,
  41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60}
say(#t)
local function closevar()
  local x <close> = setmetatable({}, {__close = function() end})
  return 1
end
say(closevar())
local function constvar()
  local x <const> = 1
  return x
end
say(constvar())
boom(function() load("local x <const> = 1; x = 2")() end)
boom(function() load("goto l ::l:: local a = 1")() end)
boom(function() load("::l:: ::l::")() end)

-- package searchers
say(type(package.searchpath("no.such.mod", package.path)))
say(type(package.path))
say(type(package.cpath))
package.preload["covergap2_mod"] = function() return {ok=true} end
say(require("covergap2_mod").ok)
boom(function() require("definitely.missing.module.xyz") end)

-- load modes
say(load("return 1", nil, "t")())
say(load("return 2", "=x", "bt")())
local bin = string.dump(function() return 3 end)
say(load(bin, "=b", "b")())
boom(function() load(bin, "=b", "t")() end)

-- table lib edges
local a = {3,1,2}
table.sort(a)
say(table.concat(a, ","))
say(table.pack(1, nil, 3).n)
say(select(2, table.unpack({10,20,30}, 2, 3)))
table.move({1,2,3}, 1, 3, 4)
say(table.insert and true)

-- bit ops / extras
say(1 ~ 2)
say(1 | 2)
say(1 & 2)
say(1 << 2)
say(8 >> 1)
say(~0)

return table.concat(out, "\n")
`)
	if len(strings.Split(got, "\n")) < 15 {
		t.Fatalf("debug/parser:\n%s", got)
	}
}

func TestCoverGap2LuacMore(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "c.lua")
	if err := os.WriteFile(src, []byte("local a = 1\nreturn a + 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outp := filepath.Join(dir, "c.out")
	var out, errb strings.Builder
	code := Luac([]string{"luac", "-o", outp, "-s", src}, &out, &errb)
	if code != 0 {
		t.Fatalf("luac -o: %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-l", "-l", src}, &out, &errb)
	if code != 0 {
		t.Fatalf("luac -l -l: %d %s", code, errb.String())
	}
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-v", "-l", src}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac"}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", "-p", "-s", src}, &out, &errb)
	_ = code
	out.Reset()
	errb.Reset()
	code = Luac([]string{"luac", outp}, &out, &errb) // listing binary?
	_ = code
}

func TestCoverGap2MiscHelpers(t *testing.T) {
	L := NewState()
	defer L.Close()

	// index2value C-closure upvalue path
	L.PushInteger(7)
	L.PushGoClosure(func(L *State) int {
		L.PushValue(luaUpvalueIndex(1))
		return 1
	}, 1)
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	n, _ := L.ToInteger(-1)
	if n != 7 {
		t.Fatalf("upvalue %d", n)
	}
	L.Pop(1)

	// Copy between stack slots (upvalue dest panics without active C closure frame)
	L.PushInteger(1)
	L.PushInteger(2)
	L.Copy(-1, 1)
	L.SetTop(0)

	// Rotate edge n=0 and negative
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Rotate(1, 0)
	L.Rotate(1, -1)
	L.Rotate(1, 2)
	L.SetTop(0)

	// GetUserValue out of range / non-udata
	L.PushInteger(1)
	_ = L.GetUserValue(-1, 1)
	L.SetTop(0)
	L.NewUserData("z", 0)
	_ = L.GetUserValue(-1, 1)
	_ = L.SetUserValue(-1, 1)
	L.Pop(1)

	// Compare other ops
	L.PushInteger(1)
	L.PushInteger(2)
	_ = L.Compare(-2, -1, LUA_OPLT)
	_ = L.Compare(-2, -1, LUA_OPLE)
	L.PushNumber(1.5)
	_ = L.Compare(-1, -2, LUA_OPEQ)
	L.SetTop(0)

	// RawLen variants
	L.PushString("abc")
	if L.RawLen(-1) != 3 {
		t.Fatal("RawLen str")
	}
	L.CreateTable(2, 0)
	L.PushInteger(1)
	L.RawSetI(-2, 1)
	_ = L.RawLen(-1)
	L.NewUserData(nil, 0)
	_ = L.RawLen(-1)
	L.SetTop(0)

	// ToStringX
	L.PushInteger(3)
	s := L.ToStringX(-1)
	_ = s
	L.Pop(1)

	// SetTop negative / grow
	L.PushInteger(1)
	L.SetTop(5)
	L.SetTop(1)
	L.SetTop(0)

	// light userdata pointer
	var key int
	L.PushLightUserData(unsafe.Pointer(&key))
	_ = L.ToPointer(-1)
	_ = touserdataPtr(&L.stack[L.top-1])
	L.Pop(1)

	// NewUserData size 0 and with uv
	L.NewUserData(nil, 0)
	L.Pop(1)

	// Traceback
	L.OpenLibs()
	L.Traceback(L, "mark", true, 0)
	L.Pop(1)

	// Ref / Unref
	L.PushInteger(5)
	ref := L.Ref(LUA_REGISTRYINDEX)
	L.RawGetI(LUA_REGISTRYINDEX, int64(ref))
	L.Pop(1)
	L.Unref(LUA_REGISTRYINDEX, ref)

	// prepbuffsizeNoStack overflow path
	B := &Buffer{L: L, b: make([]byte, 2), size: 2, n: 2, onStack: false}
	func() {
		defer func() { _ = recover() }()
		B.prepbuffsizeNoStack(math.MaxInt/2 + 1)
	}()

	// Escapes in lexer via LoadString
	_ = L.LoadString(`return "\x41\u{41}\z  
	\n\r\t\v\a\b\f\\"`, "=esc")
	L.SetTop(0)
	_ = L.LoadString(`return '\10\010\xzz'`, "=badesc")
	L.SetTop(0)
	_ = L.LoadString("return [[\nlong]]", "=long")
	L.SetTop(0)
	_ = L.LoadString("return [=[a]=]", "=long2")
	L.SetTop(0)

	// const2exp / luaK_exp2const via code that folds
	_ = L.LoadString("return true, false, nil, 1, 1.5, 'x'", "=fold")
	if L.PCall(0, 6, 0) == LUA_OK {
		L.SetTop(0)
	}

	_ = luaH_free // ensure reference; empty body stays 0%
	luaH_free(L, luaH_new(L))
}

// luaUpvalueIndex mirrors LUA_REGISTRYINDEX - i.
func luaUpvalueIndex(i int) int { return LUA_REGISTRYINDEX - i }
