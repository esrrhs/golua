package core

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestCoverGap13StrFormatZeroInt(t *testing.T) {
	// Direct: zero+prec<0 branch is unreachable via string.format (prec==0 there).
	_ = strFormatZeroInt(strFmtSpec{plus: true, zero: true, prec: -1, width: 5})
	_ = strFormatZeroInt(strFmtSpec{space: true, zero: true, prec: -1, width: 5})
	_ = strFormatZeroInt(strFmtSpec{plus: true, zero: true, left: true, prec: -1, width: 5})
	_ = strFormatZeroInt(strFmtSpec{plus: true, zero: true, prec: -1, width: 1})
	_ = strFormatZeroInt(strFmtSpec{plus: true, prec: 0, width: 5})
	_ = strFormatZeroInt(strFmtSpec{space: true, prec: 0, width: 3})
	_ = strPadField(strFmtSpec{left: true, width: 5}, "x")
	_ = strPadField(strFmtSpec{zero: true, prec: -1, width: 5}, "1")
	_ = strPadField(strFmtSpec{width: 5}, "1")
	_, _ = strFormFlagsWidth("%-+ #05.2d")
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
say(string.format("%.0d", 0))
say(string.format("%+.0d", 0))
say(string.format("% .0d", 0))
say(string.format("%+5.0d", 0))
say(string.format("%-5.0d", 0))
say(string.format("%#o", 0))
say(string.format("%#.0o", 0))
say(string.format("%#05x", 1))
say(string.format("%#05X", 1))
say(string.format("%05d", 0))
say(string.format("%q", "a\0b\n\t\\\"\r"))
say(string.format("%a", 0.0))
say(string.format("%A", -0.0))
say(string.format("%a", 1/0))
say(string.format("%a", 0/0))
say(string.format("%.0f", 1.5))
say(string.format("%+08.2f", 1.5))
say(string.pack("<i2", -2))
say(string.pack(">I2", 3))
say(string.pack("!4i4", 1))
say(string.pack(" z", "hi"))
say(select(1, string.unpack("z", string.pack("z", "ab"))))
boom = function(f) out[#out+1]=tostring(not pcall(f)) end
boom(function() string.format("%*") end)
boom(function() string.pack("c0") end)
boom(function() string.unpack("c5", "ab") end)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap13DebugInteractive(t *testing.T) {
	// dbReadCommand edges
	r1 := bufio.NewReader(strings.NewReader("print(1)\ncont\n"))
	line, err := dbReadCommand(r1)
	if err != nil || !strings.HasPrefix(line, "print") {
		t.Fatalf("line=%q err=%v", line, err)
	}
	line, err = dbReadCommand(r1)
	if err != nil || line != "cont\n" {
		t.Fatalf("cont=%q err=%v", line, err)
	}
	// EOF with partial
	r2 := bufio.NewReader(strings.NewReader("partial"))
	line, err = dbReadCommand(r2)
	if line != "partial" {
		t.Fatalf("partial=%q", line)
	}
	// EOF empty
	r3 := bufio.NewReader(strings.NewReader(""))
	_, err = dbReadCommand(r3)
	if err == nil {
		t.Fatal("want EOF")
	}
	// long line truncated at dbDebugLine-1
	long := strings.Repeat("a", dbDebugLine+10) + "\n"
	r4 := bufio.NewReader(strings.NewReader(long))
	line, _ = dbReadCommand(r4)
	if len(line) != dbDebugLine-1 {
		t.Fatalf("len=%d", len(line))
	}

	// Drive dbDebugFunc with stubbed stdin
	oldIn := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldIn
		_ = r.Close()
	})
	go func() {
		_, _ = w.Write([]byte("print(1+1)\nbad(\ncont\n"))
		_ = w.Close()
	}()
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	_ = dbDebugFunc(L)
}

func TestCoverGap13LuacEdges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	if err := os.WriteFile(src, []byte("return true,false,nil,1.0,\"hi\""), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	_ = Luac([]string{"luac", "-l", "-l", src}, &out, &errb)

	// /dev/full forces write error on dump to a real file path
	errb.Reset()
	out.Reset()
	code := Luac([]string{"luac", "-o", "/dev/full", src}, &out, &errb)
	if code == 0 {
		t.Log("unexpected ok on /dev/full (may be sandbox)")
	}

	// Direct printConstant / printType / printHeader with crafted Proto
	c := &luac{out: &out, err: &errb}
	f := &Proto{
		k: make([]Value, 8),
		code: []Instruction{
			newABx(OP_LOADK, 0, 0),
			newABx(OP_LOADK, 0, 1),
			newABx(OP_LOADK, 0, 2),
			newABx(OP_LOADK, 0, 3),
			newABx(OP_LOADK, 0, 4),
			newABx(OP_LOADK, 0, 5),
			newABC(OP_RETURN, 0, 1, 0, 0),
		},
		sizecode: 7,
		sizek:    6,
		source:   &TString{tt: LUA_VSHRSTR, val: "@src.lua"},
	}
	setnilvalue(&f.k[0])
	setbfvalue(&f.k[1]) // false? check helpers
	setbtvalue(&f.k[2])
	setfltvalue(&f.k[3], 2.0) // looks like integer → ".0"
	setivalue(&f.k[4], 7)
	ts := &TString{tt: LUA_VSHRSTR, val: "s"}
	setsvalue(&f.k[5], ts)
	for i := 0; i < 6; i++ {
		c.printType(f, i)
		c.printConstant(f, i)
	}
	// default tag arm
	f.k[0].tt = 0xff
	c.printType(f, 0)
	c.printConstant(f, 0)
	// source variants for printHeader
	f.source = &TString{tt: LUA_VSHRSTR, val: "=chunk"}
	c.printHeader(f)
	f.source = &TString{tt: LUA_VSHRSTR, val: string(dmpSignature[0]) + "bin"}
	c.printHeader(f)
	f.source = &TString{tt: LUA_VSHRSTR, val: "plain"}
	f.linedefined = 1
	c.printHeader(f)
	f.source = nil
	c.printHeader(f)
	f.upvalues = []Upvaldesc{{}}
	f.sizeupvalues = 1
	_ = c.upvalName(f, 0)
	f.upvalues[0].name = &TString{tt: LUA_VSHRSTR, val: "_ENV"}
	_ = c.upvalName(f, 0)

	// too many input files via CheckStack hook (avoids 1e6-file alloc)
	oldCS := luacCheckStackFn
	luacCheckStackFn = func(L *State, n int) bool { return false }
	t.Cleanup(func() { luacCheckStackFn = oldCS })
	c2 := &luac{out: &out, err: &errb}
	if c2.pmain([]string{"x.lua"}) == 0 {
		t.Fatal("expected too many input files")
	}

	// LOADKX / SETTABUP etc via rich source
	body := `
local t = {}
t.x = 1
_ENV.y = 2
local a = true
local b = false
local c = nil
local d = 1.0
local e = "x"
local big = ` + "`" + strings.Repeat("k", 300) + "`" + `
return t.x, a, b, c, d, e, big, ({})[1]
`
	// Fix: use normal string
	body = `
local t={}
t.x=1
_ENV.y=2
local a=true; local b=false; local c=nil
local d=1.0; local e="x"
return function(u)
  u:meth(1)
  u.f = 2
  u[3] = 4
  return u.x + 1.0, true, false, nil
end, t
`
	src2 := filepath.Join(dir, "rich.lua")
	if err := os.WriteFile(src2, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", "-l", src2}, &out, &errb)

	// combine multiple files
	src3 := filepath.Join(dir, "b.lua")
	_ = os.WriteFile(src3, []byte("return 2"), 0o644)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l", src, src3}, &out, &errb)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-o", filepath.Join(dir, "out.luac"), src, src3}, &out, &errb)
}

func TestCoverGap13IOWaitPipeAndCorners(t *testing.T) {
	_ = ioCapitalise("")
	_ = ioCapitalise("abc")
	_ = ioFileError("open", "x", errors.New("plain"))
	_ = ioFileError("open", "x", syscall.ENOENT)
	_ = ioErrno(errors.New("x"))
	what, code := ioProcessStatus(nil)
	_ = what
	_ = code
	what, code = ioProcessStatus(errors.New("not exit"))
	_ = what
	_ = code
	// signal exit
	cmd := exec.Command("sh", "-c", "kill -9 $$")
	err := cmd.Run()
	what, code = ioProcessStatus(err)
	_ = what
	_ = code

	s := &ioStream{canR: false, canW: false}
	_ = s.reader()
	_ = s.writer()
	s2 := &ioStream{canW: true}
	if err := s2.writeString("x"); err == nil {
		// nil writer → ioWriteError
	}
	s3 := &ioStream{}
	_ = s3.doClose()

	// waitPipe with flush failure on write pipe
	pr, pw := io.Pipe()
	s4 := &ioStream{
		wr:  pw,
		cmd: exec.Command("true"),
	}
	s4.wbuf = bufio.NewWriterSize(&failWriter{}, 4)
	_, _ = s4.wbuf.Write([]byte("xxxx")) // fill so flush will write
	_ = s4.cmd.Start()
	_, _, _ = s4.waitPipe()
	_ = pr.Close()

	// waitPipe read side non-ExitError
	s5 := &ioStream{
		rd:  io.NopCloser(strings.NewReader("")),
		cmd: exec.Command("sh", "-c", "exit 3"),
	}
	_ = s5.cmd.Start()
	_, _, _ = s5.waitPipe()

	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local p = io.popen("false", "r")
if p then say(select(1, p:close()) == nil or true) end
local p2 = io.popen("echo hi", "r")
if p2 then
  say(p2:read("*a") ~= nil)
  say(p2:close())
end
-- rename same file
local n = os.tmpname()
local f=io.open(n,"w"); f:write("x"); f:close()
say(os.rename(n, n) == true or true)
os.remove(n)
-- lines iterator close
local n2 = os.tmpname()
local f2=io.open(n2,"w"); f2:write("a\nb\n"); f2:close()
for l in io.lines(n2) do say(l) end
os.remove(n2)
return table.concat(out,"|")
`)
	_ = got
}

// Ensure ioStream.flush uses wbuf; for waitPipe we need s.wbuf set.
func TestCoverGap13WaitPipeFlushViaWbuf(t *testing.T) {
	pr, pw := io.Pipe()
	s := &ioStream{wr: pw, canW: true, cmd: exec.Command("cat")}
	if err := s.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	s.wbuf = bufio.NewWriterSize(&failWriter{}, 8)
	_, _, err := s.waitPipe()
	_ = err
	_ = pr.Close()
}

func TestCoverGap13CoroCloseEdges(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
-- dead after error
local co = coroutine.create(function() error("x") end)
coroutine.resume(co)
say(coroutine.status(co))
say(select(1, coroutine.close(co)))
say(select(1, coroutine.close(co))) -- second close true
-- yield then close
local co2 = coroutine.create(function()
  coroutine.yield(1)
  return 2
end)
coroutine.resume(co2)
say(coroutine.close(co2))
-- wrap close path via error
local w = coroutine.wrap(function()
  local t <close> = setmetatable({}, {__close=function() end})
  error("boom")
end)
boom(function() w() end)
-- to-be-closed that errors on close
local co3 = coroutine.create(function()
  local t <close> = setmetatable({}, {__close=function() error("c") end})
  error("body")
end)
coroutine.resume(co3)
local ok, err = coroutine.close(co3)
say(ok)
say(err ~= nil)
return table.concat(out,"|")
`)
	_ = got

	// Direct coAuxResume stack-full arms
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	co := L.NewThread()
	co.reallocStack(ERRORSTACKSIZE)
	co.top = co.stackLast - 1
	L.PushString("too")
	_ = coAuxResume(L, co, 1)
}

func TestCoverGap13LocaleCtypeHooks(t *testing.T) {
	was := ctypeIsC.Load()
	ctypeIsC.Store(false)
	t.Cleanup(func() { ctypeIsC.Store(was) })
	// With ctype hooks nil, still take ASCII path; force hooks non-nil.
	oldL, oldU := ctypeToLower, ctypeToUpper
	oldA, oldC := ctypeIsAlpha, ctypeIsCntrl
	oldD, oldG := ctypeIsDigit, ctypeIsGraph
	oldLo, oldP := ctypeIsLower, ctypeIsPunct
	oldS, oldUp := ctypeIsSpace, ctypeIsUpper
	oldAl, oldX := ctypeIsAlnum, ctypeIsXDigit
	t.Cleanup(func() {
		ctypeToLower, ctypeToUpper = oldL, oldU
		ctypeIsAlpha, ctypeIsCntrl = oldA, oldC
		ctypeIsDigit, ctypeIsGraph = oldD, oldG
		ctypeIsLower, ctypeIsPunct = oldLo, oldP
		ctypeIsSpace, ctypeIsUpper = oldS, oldUp
		ctypeIsAlnum, ctypeIsXDigit = oldAl, oldX
	})
	ctypeToLower = func(c byte) byte { return c }
	ctypeToUpper = func(c byte) byte { return c }
	ctypeIsAlpha = func(c byte) bool { return false }
	ctypeIsCntrl = func(c byte) bool { return false }
	ctypeIsDigit = func(c byte) bool { return false }
	ctypeIsGraph = func(c byte) bool { return false }
	ctypeIsLower = func(c byte) bool { return false }
	ctypeIsPunct = func(c byte) bool { return false }
	ctypeIsSpace = func(c byte) bool { return false }
	ctypeIsUpper = func(c byte) bool { return false }
	ctypeIsAlnum = func(c byte) bool { return false }
	ctypeIsXDigit = func(c byte) bool { return false }
	_ = localeToLower('A')
	_ = localeToUpper('a')
	_ = localeIsAlpha('a')
	_ = localeIsCntrl(1)
	_ = localeIsDigit('1')
	_ = localeIsGraph('x')
	_ = localeIsLower('a')
	_ = localeIsPunct('.')
	_ = localeIsSpace(' ')
	_ = localeIsUpper('A')
	_ = localeIsAlnum('a')
	_ = localeIsXDigit('f')
	// localeNote arms
	oldSC := strColl
	strColl = func(a, b string) int { return strings.Compare(a, b) }
	t.Cleanup(func() { strColl = oldSC })
	localeNote(0, "C")
	localeNote(0, "en_US.UTF-8")
	localeNote(1, "C")
	localeNote(2, "POSIX")
	localeNote(2, "en_US")
	localeNote(3, "x")
}

func TestCoverGap13LoadlibTinySO(t *testing.T) {
	so, err := filepath.Abs("testdata/tiny.so")
	if err != nil || !pkReadable(so) {
		t.Skip("tiny.so missing")
	}
	dir := filepath.Dir(so)
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// package.loadlib success
	L.GetGlobal("package")
	L.GetField(-1, "loadlib")
	L.PushString(so)
	L.PushString("luaopen_tiny")
	if L.PCall(2, 3, 0) != LUA_OK {
		t.Fatalf("loadlib: %v", L.ToStringX(-1))
	}
	L.SetTop(0)
	// star sym
	L.GetGlobal("package")
	L.GetField(-1, "loadlib")
	L.PushString(so)
	L.PushString("*")
	_ = L.PCall(2, 3, 0)
	L.SetTop(0)
	// already loaded + star
	L.GetGlobal("package")
	L.GetField(-1, "loadlib")
	L.PushString(so)
	L.PushString("*")
	_ = L.PCall(2, 3, 0)
	L.SetTop(0)
	// ignore-mark modname path via pkLoadFunc
	func() {
		defer func() { _ = recover() }()
		_ = pkLoadFunc(L, so, "x-tiny")
	}()
	L.SetTop(0)
	func() {
		defer func() { _ = recover() }()
		_ = pkLoadFunc(L, so, "tiny")
	}()
	L.SetTop(0)
	func() {
		defer func() { _ = recover() }()
		_ = pkLookForFunc(L, so, "no_such_sym")
	}()
	L.SetTop(0)
	_ = pkCheckCLib(L, so)
	// via Lua searchers / require with cpath pointing at testdata
	chunk := `
package.cpath = [[` + dir + `/?.so]]
local ok, err = pcall(require, "tiny")
local ok2, err2 = pcall(require, "tiny.sub")
local f, e, where = package.loadlib([[` + so + `]], "luaopen_tiny")
return tostring(ok), tostring(err), tostring(ok2), type(f), tostring(where)
`
	_ = runChunk(t, chunk)
}

func TestCoverGap13APIAndDO(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	// pcallK / Error / Compare edges
	L.PushGoFunc(func(L *State) int {
		L.PushString("e")
		return L.Error()
	})
	_ = L.PCall(0, 0, 0)
	L.SetTop(0)

	L.PushInteger(1)
	L.PushInteger(1)
	_ = L.RawEqual(-2, -1)
	L.SetTop(0)

	// UpvalueId / Join
	got := runChunk(t, `
local function f(a) return function() return a end end
local a,b = f(1), f(2)
local u1 = debug.upvalueid(a, 1)
local u2 = debug.upvalueid(b, 1)
debug.upvaluejoin(a, 1, b, 1)
return debug.upvalueid(a,1) == debug.upvalueid(b,1)
`)
	_ = got

	// luaD_seterrorobj via various statuses — throw through pcall
	L.reallocStack(ERRORSTACKSIZE)
	_ = luaD_checkminstack(L)

	// shrink stack
	L2 := NewState()
	defer L2.Close()
	L2.CheckStack(1000)
	luaD_shrinkstack(L2)
	luaE_shrinkCI(L2)
}

func TestCoverGap13ParserVMEdges(t *testing.T) {
	cases := []string{
		`local a <const> = 1; return a`,
		`local a <close> = setmetatable({}, {__close=function() end}); return 1`,
		`goto a; ::a:: return 1`,
		`local function f(...) return ... end; return f(1,2)`,
		`return (function(a,b,c) return a+b+c end)(1,2,3)`,
		`for i=1,0 do end; return true`,
		`local ok=pcall(function() for i=1,10,0 do break end end); return ok==false`,
		`local t={}; for i=1,3 do t[i]=i end; return t[3]`,
		`local a=1.5; return a//1, a%1`,
		`return 1<<63, (-1)>>1`,
		`local mt={__lt=function(a,b) return true end, __le=function(a,b) return true end}
local a=setmetatable({},mt); local b=setmetatable({},mt); return a<b, a<=b`,
		`local mt={__eq=function() return true end}
return setmetatable({},mt)==setmetatable({},mt)`,
	}
	for _, src := range cases {
		_ = runChunk(t, "return (function() "+src+" end)()")
	}
	// compile errors
	L := NewState()
	defer L.Close()
	for _, bad := range []string{
		`goto a`,
		`::a:: ::a::`,
		`local a <const> = 1; a = 2`,
		`break`,
		`return (function() break end)()`,
	} {
		_ = L.LoadString(bad, "=bad")
		L.SetTop(0)
	}
}

func TestCoverGap13ObjectHelpers(t *testing.T) {
	L := NewState()
	defer L.Close()
	var r Value
	var a, b Value
	setivalue(&a, 10)
	setivalue(&b, 3)
	_ = luaO_rawarith(L, LUA_OPADD, &a, &b, &r)
	_ = luaO_rawarith(L, LUA_OPBNOT, &a, &b, &r)
	setfltvalue(&a, 1.5)
	_ = luaO_rawarith(L, LUA_OPUNM, &a, &b, &r)
	_ = luaO_str2num("1.5", &r)
	_ = luaO_str2num("0x10", &r)
	_ = luaO_str2num("nope", &r)
	_ = luaO_chunkid("=x")
	_ = luaO_chunkid("@very/long/path/that/needs/truncation/for/the/chunkid/buffer")
	_ = luaO_chunkid("[string \"hi\"]")
	_ = opName(255)
	_ = opName(0)
}

func TestCoverGap13LAPICorners(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushNil()
	_ = L.ToGoFunction(-1)
	L.PushGoFunc(func(L *State) int { return 0 })
	_ = L.ToGoFunction(-1)
	L.SetTop(0)
	L.NewTable()
	L.PushString("k")
	L.PushInteger(1)
	L.SetTable(-3)
	L.PushString("k")
	_ = L.GetTable(-2)
	L.SetTop(0)
	ud := L.NewUserData([]byte{1, 2, 3, 4}, 0)
	_ = ud
	L.NewTable()
	L.SetMetaTable(-2)
	L.SetTop(0)
	// CloseSlot / ToClose
	got := runChunk(t, `
local t <close> = setmetatable({}, {__close=function() end})
return true
`)
	_ = got
	func() {
		defer func() { _ = recover() }()
		L.PushInteger(1)
		L.ToClose(-1)
	}()
	L.SetTop(0)
}
