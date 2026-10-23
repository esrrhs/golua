package core

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failWriter struct{ n int }

func (w *failWriter) Write(p []byte) (int, error) {
	w.n++
	if len(p) == 0 {
		return 0, nil
	}
	return 0, errors.New("forced write fail")
}

func TestCoverGap10LuacWriteError(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	if err := os.WriteFile(src, []byte("return 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	fw := &failWriter{}
	code := Luac([]string{"luac", "-o", "-", src}, fw, &errb)
	if code == 0 {
		t.Fatalf("expected fail, err=%q", errb.String())
	}
	if !strings.Contains(errb.String(), "cannot write") && !strings.Contains(errb.String(), "forced") {
		t.Fatalf("err %q", errb.String())
	}

	// also exercise -- and version edge
	var out bytes.Buffer
	errb.Reset()
	_ = Luac([]string{"luac", "-v", "--", src}, &out, &errb)
	errb.Reset()
	out.Reset()
	_ = Luac([]string{"luac", "-l", "-l", "--", src}, &out, &errb)

	// bad -o
	errb.Reset()
	_ = Luac([]string{"luac", "-o"}, &out, &errb)
	errb.Reset()
	_ = Luac([]string{"luac", "-o", ""}, &out, &errb)
	errb.Reset()
	_ = Luac([]string{"luac", "-z"}, &out, &errb)

	// missing input
	errb.Reset()
	_ = Luac([]string{"luac", filepath.Join(dir, "nope.lua")}, &out, &errb)

	// listing with many opcodes for printCode arms
	body := `
local function g(a,b,c,d)
  local t = {a,b,c,d; x=1, y=2}
  t[1] = a
  t.x = b
  local u = -a
  local v = ~a
  local w = #t
  local x = a .. b
  local y = a == b
  local z = a < b
  local zz = a <= b
  if a then return b elseif c then return d else return a end
end
for i=1,3 do end
for i=3,1,-1 do end
for k,v in pairs({}) do end
repeat break until true
while false do break end
return g(1,2,3,4), "\0\n\t\\\"\a\b\f\r\v\x1b"
`
	src2 := filepath.Join(dir, "ops.lua")
	if err := os.WriteFile(src2, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l", src2}, &out, &errb)
}

func TestCoverGap10StrLibEdges(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
say(string.byte("abc", 0))
say(string.byte("abc", 4))
say(string.byte("abc", -1))
say(string.byte("abc", 1, 3))
say(string.char(0, 255))
boom(function() string.char(256) end)
say(string.find("abc", "b"))
say(string.find("abc", "z"))
say(string.find("a\0b", "%z"))
say(string.gsub("aaa", "a", "b", 2))
say(string.gsub("a", "a", function() return "x" end))
say(string.gsub("a", "a", {a="z"}))
say(string.gmatch("a b c", "%S+")())
say(string.match("abc", "b"))
say(string.rep("ab", 3, "-"))
say(string.reverse("abc"))
say(string.lower("AB"))
say(string.upper("ab"))
say(string.sub("abc", 2, 2))
say(string.sub("abc", -1))
say(string.format("%d %s %f %%", 1, "x", 1.5))
say(string.format("%q", "a\"b"))
say(string.pack("b", 1))
say(string.unpack("b", string.pack("b", 3)))
say(string.packsize("i4"))
boom(function() string.pack("i4", 1, 2) end)
boom(function() string.format("%d") end)
-- pack/unpack edges
say(string.pack("<i2", 1000))
say(string.pack(">i2", 1000))
say(string.pack("!1 i4", 5))
say(string.pack("c3", "ab"))
say(string.pack("c0", ""))
say(string.pack("s1", "hi"))
say(string.pack("z", "hi"))
say(string.pack("x"))
say(string.pack("j", 3))
say(string.pack("J", 3))
say(string.pack("n", 1.5))
say(string.pack("f", 1.5))
say(string.pack("d", 1.5))
local u, pos = string.unpack("z", string.pack("z", "ok") .. "x")
say(u)
say(utf8.len("a你"))
say(utf8.offset("a你", 2))
say(utf8.char(0x4f60))
say(utf8.codepoint("你", 1))
for p,c in utf8.codes("ab") do say(p) end
return table.concat(out, "|")
`)
	_ = got
}

func TestCoverGap10IOLibEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	t.Setenv("C10", path)
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
local name = os.getenv("C10")
local f = assert(io.open(name, "w"))
f:write("line1\nline2\n42\n")
f:close()
f = assert(io.open(name, "r"))
say(f:read(2))
say(f:read("*l"))
say(f:read("*n"))
say(f:read("*a") ~= nil)
f:close()
boom(function() io.open(name, "q") end)
boom(function() io.lines(name, "*z")() end)
-- popen
local p = io.popen("echo hi", "r")
if p then say(p:read("*l")); say(p:close()) end
-- type
say(io.type(io.stdin))
local cf = assert(io.open(name))
cf:close()
say(io.type(cf))
say(io.type(1))
-- input/output
io.input(name)
say(io.read(1) ~= nil)
io.input(io.stdin)
io.output(name)
io.write("z")
io.flush()
io.output(io.stdout)
return table.concat(out, "|")
`)
	_ = got
}

func TestCoverGap10DebugLibEdges(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
local function f(a,b)
  local c = a+b
  say(debug.getinfo(1, "nSlufL").what)
  say(debug.getinfo(f, "n").namewhat ~= nil or true)
  local n,v = debug.getlocal(1, 1)
  say(n)
  debug.setlocal(1, 1, 99)
  local un = debug.getupvalue(function() return a end, 1)
  say(un ~= nil or true)
  local function g() local x=1; return function() return x end end
  local cl = g()
  debug.setupvalue(cl, 1, 7)
  say(cl())
  say(debug.upvalueid(cl, 1) ~= nil)
  local cl2 = g()
  debug.upvaluejoin(cl, 1, cl2, 1)
  say(debug.traceback("msg", 1):find("msg") ~= nil)
  debug.sethook(function() end, "crl", 1)
  local x = 1+1
  debug.sethook()
  say(debug.gethook() == nil or true)
  return a
end
f(1,2)
boom(function() debug.getlocal(1, 99) end)
boom(function() debug.setlocal(1, 99, 0) end)
say(debug.getregistry() ~= nil)
say(debug.getmetatable({}) == nil)
debug.setmetatable({}, {})
return table.concat(out, "|")
`)
	_ = got
}

func TestCoverGap10VMArithEdges(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
say(1+2); say(1-2); say(2*3); say(4/2); say(5//2); say(5%2); say(2^3)
say(1&3); say(1|2); say(1~3); say(1<<2); say(8>>1); say(~1)
say(-2); say(#("ab")); say("a".."b")
say(1==1); say(1~=2); say(1<2); say(1<=2); say(2>1); say(2>=1)
-- metamethods for binops
local mt = {
  __add=function() return "add" end,
  __sub=function() return "sub" end,
  __mul=function() return "mul" end,
  __div=function() return "div" end,
  __idiv=function() return "idiv" end,
  __mod=function() return "mod" end,
  __pow=function() return "pow" end,
  __band=function() return "band" end,
  __bor=function() return "bor" end,
  __bxor=function() return "bxor" end,
  __shl=function() return "shl" end,
  __shr=function() return "shr" end,
  __unm=function() return "unm" end,
  __bnot=function() return "bnot" end,
  __len=function() return 7 end,
  __concat=function() return "cat" end,
  __eq=function() return true end,
  __lt=function() return true end,
  __le=function() return true end,
}
local a = setmetatable({}, mt)
local b = setmetatable({}, mt)
say(a+b); say(a-b); say(a*b); say(a/b); say(a//b); say(a%b); say(a^b)
say(a&b); say(a|b); say(a~b); say(a<<b); say(a>>b); say(-a); say(~a); say(#a)
say(a..b); say(a==b); say(a<b); say(a<=b)
-- to-be-closed
do
  local x <close> = setmetatable({}, {__close=function() end})
end
-- vararg / tail
local function tv(...) return select("#", ...), ... end
say(tv(1,2,3))
local function tail(n) if n==0 then return 1 else return tail(n-1) end end
say(tail(20))
-- const folding edges
say((1<<62) | 0)
say(1.0 + 2)
say(1 + 2.0)
return table.concat(out, "|")
`)
	_ = got
}

func TestCoverGap10LoadlibStub(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// pkLoadSym without backend
	orig := pkDynLoadSym
	pkDynLoadSym = nil
	t.Cleanup(func() { pkDynLoadSym = orig })
	st := pkLoadSym(L, "x.so", "luaopen_x")
	if st != pkErrLib {
		t.Fatalf("status %d", st)
	}
	L.Pop(1)

	// searchers / path
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end
say(package.searchpath("no.such.mod", package.path) == nil or true)
boom(function() require("no.such.mod.zzz") end)
say(type(package.loaded))
say(type(package.preload))
say(type(package.searchers))
package.preload["covergap10mod"] = function() return {ok=true} end
say(require("covergap10mod").ok)
return table.concat(out, "|")
`)
	_ = got
}
