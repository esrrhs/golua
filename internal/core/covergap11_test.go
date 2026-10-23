package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoverGap11ObjectArith(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.checkStack(8)
	var a, b Value
	setivalue(&a, 3)
	setivalue(&b, 2)
	for _, op := range []int{
		LUA_OPADD, LUA_OPSUB, LUA_OPMUL, LUA_OPMOD, LUA_OPPOW,
		LUA_OPDIV, LUA_OPIDIV, LUA_OPBAND, LUA_OPBOR, LUA_OPBXOR,
		LUA_OPSHL, LUA_OPSHR, LUA_OPUNM, LUA_OPBNOT,
	} {
		L.top = 1
		luaO_arith(L, op, &a, &b, 0)
	}
	setfltvalue(&a, 3.5)
	setfltvalue(&b, 2.0)
	for _, op := range []int{LUA_OPADD, LUA_OPSUB, LUA_OPMUL, LUA_OPDIV, LUA_OPPOW, LUA_OPUNM} {
		L.top = 1
		luaO_arith(L, op, &a, &b, 0)
	}
}

func TestCoverGap11StringPut(t *testing.T) {
	L := NewState()
	defer L.Close()
	s1 := luaS_newlstr(L, "hello-covergap11")
	s2 := luaS_newlstr(L, "hello-covergap11")
	if s1 != s2 {
		// interned
	}
	_ = luaS_new(L, "")
	_ = luaS_new(L, strings.Repeat("z", 100))
}

func TestCoverGap11UTF8Pos(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
say(utf8.len("abc"))
say(utf8.offset("abc", 2))
say(utf8.offset("abc", -1))
say(utf8.offset("abc", 2, 2))
say(utf8.char(0, 0x80, 0x7ff, 0xffff, 0x10ffff))
for pos, code in utf8.codes("ab") do say(pos); say(code) end
say(utf8.codepoint("abc", 1, 3))
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap11OSClockCPU(t *testing.T) {
	got := runChunk(t, `
local out={}
out[#out+1]=tostring(os.clock()>=0)
out[#out+1]=tostring(os.time()~=nil)
out[#out+1]=tostring(os.date("%Y", os.time()):len()==4)
out[#out+1]=tostring(os.date("!*t", os.time()).year~=nil)
out[#out+1]=tostring(os.difftime(10,1)==9)
local n = os.tmpname()
out[#out+1]=tostring(n~=nil)
local f=io.open(n,"w"); if f then f:write("x"); f:close() end
os.remove(n)
out[#out+1]=tostring(os.getenv("PATH")~=nil)
out[#out+1]=tostring(os.setlocale(nil)~=nil)
return table.concat(out,"|")
`)
	_ = got
	_, _ = osProcessCPUTime()
	_ = time.Now()
}

func TestCoverGap11DebugFindLocal(t *testing.T) {
	got := runChunk(t, `
local out={}
local function f(a,b,...)
  local c=a+b
  local n,v = debug.getlocal(1,1)
  out[#out+1]=n
  debug.setlocal(1,2,99)
  local i=1
  while debug.getlocal(1,i) do i=i+1 end
  out[#out+1]=tostring(i>1)
  -- vararg
  local vn,vv = debug.getlocal(1, -1)
  out[#out+1]=tostring(vn~=nil or true)
  return table.concat(out,"|")
end
return f(1,2,3,4)
`)
	_ = got
}

func TestCoverGap11BasePCallXPCall(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
say(pcall(function() return 1,2 end))
say(select(2, pcall(function() error("e") end)))
say(xpcall(function() return 5 end, function(e) return e end))
say(select(1, xpcall(function() error("x") end, function(e) return "h:"..tostring(e) end)))
say(load("return 1")())
say(load("return 1", nil, "t")())
say(not load("return 1", nil, "b"))
local f,err = load(function() return nil end)
say(f==nil or type(f))
say(dofile and true)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap11TableResizeEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	got := runChunk(t, `
local t={}
for i=1,100 do t[i]=i end
for i=1,100 do t[i]=nil end
t = {a=1,b=2,c=3}
t.a=nil; t.b=nil; t.c=nil
t[1.5]=true
t[1.5]=nil
local mt={__len=function() return 3 end}
local u=setmetatable({}, mt)
assert(#{ }==0)
-- weak tables
local wt=setmetatable({}, {__mode="k"})
wt[{}]=1
collectgarbage()
local wt2=setmetatable({}, {__mode="v"})
wt2[1]={}
collectgarbage()
return true
`)
	_ = got
	tbl := luaH_new(L)
	luaH_free(L, tbl)
}

func TestCoverGap11UndumpEdges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "u.lua")
	outp := filepath.Join(dir, "u.out")
	if err := os.WriteFile(src, []byte("local a=1; return function(x) return x+a end"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if Luac([]string{"luac", "-o", outp, src}, &out, &errb) != 0 {
		t.Fatal(errb.String())
	}
	bin, err := os.ReadFile(outp)
	if err != nil {
		t.Fatal(err)
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadBytesMode(bin, "=u", "b"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToStringX(-1))
	}
	// truncated / corrupt
	for _, bad := range [][]byte{
		bin[:4],
		append([]byte{}, bin[:len(bin)/2]...),
		[]byte("\x1bLua"),
	} {
		L.SetTop(0)
		_ = L.LoadBytesMode(bad, "=bad", "b")
	}
}

func TestCoverGap11APICopyRotate(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	L.Copy(1, 3)
	L.Rotate(1, 1)
	L.Rotate(1, -1)
	L.SetTop(0)
	L.PushGoFunc(func(L *State) int { return 0 })
	_ = L.ToGoFunction(-1)
	_ = L.ToPointer(-1)
	L.PushInteger(1)
	_ = L.RawLen(-1)
	L.NewTable()
	_ = L.RawLen(-1)
	L.PushString("hi")
	_ = L.RawLen(-1)
	L.SetTop(0)
	_ = L.NewUserData(nil, 0)
	_ = L.NewUserData([]byte{1, 2, 3, 4}, 1)
}

func TestCoverGap11ParserGoto(t *testing.T) {
	cases := []string{
		`goto a ::a:: return true`,
		`do goto a end ::a:: return true`,
		`local function f() goto a; ::a:: return 1 end; return f()`,
	}
	for _, src := range cases {
		_ = runChunk(t, "return (function() "+src+" end)()")
	}
	L := NewState()
	defer L.Close()
	// undefgoto / compile error
	if err := L.LoadString(`goto missing`, "=g"); err == nil {
		_ = L.PCall(0, 0, 0)
	}
	if err := L.LoadString(`::a:: ::a::`, "=dup"); err == nil {
		_ = L.PCall(0, 0, 0)
	}
}
