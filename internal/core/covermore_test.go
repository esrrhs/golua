package core

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The functions below are reached only from edges a short script never
// takes: mixed int/float order at runtime, a \z escape, a lax UTF-8
// walk, a write-only file, a constructor past the constant-index limit
// of an instruction, and the warning state machine once it is turned on.

func TestEdgesTheShortScriptsMiss(t *testing.T) {
	wo := filepath.Join(t.TempDir(), "wo.txt")
	t.Setenv("COVER_WO", wo)

	got := runChunk(t, `
local out = {}
local function say(s) out[#out + 1] = tostring(s) end

local i, f = 2, 2.5
local big = math.maxinteger
local small = math.mininteger
local inf = 1/0
local nan = 0/0
say(i <= f)
say(f <= i)
say(big <= f)
say(f <= big)
say(big <= inf)
say(nan <= big)
say(small <= -1.5)
say(-1.5 <= small)
say(small <= -inf)

say("a\z 
	b" == "ab")

local s = "\xED\xA0\x80"
local iter, st, p = utf8.codes(s, true)
local _, c = iter(st, p)
say(c)
local strict = utf8.codes(s)
say(pcall(strict, s, 0))

local fh = assert(io.open(os.getenv("COVER_WO"), "w"))
local x, err, en = fh:read(1)
fh:close()
say(x == nil)
say(err)
say(en)

local function has(err, needle)
  return string.find(tostring(err), needle, 1, true) ~= nil
end
local function callfield()
  local t = {}
  local k = "nope"
  return t[k]()
end
local ok, err = pcall(callfield)
say(ok)
say(has(err, "nil"))
say(has(err, "?"))

local function callmethod()
  local t = {}
  return t:nope()
end
ok, err = pcall(callmethod)
say(ok)
say(has(err, "nope"))

local function callfloat()
  local t = {}
  return t[1.5]()
end
ok, err = pcall(callfloat)
say(ok)
say(has(err, "?"))

return table.concat(out, "\n")
`)
	want := strings.Join([]string{
		"true",
		"false",
		"false",
		"true",
		"true",
		"false",
		"true",
		"false",
		"false",
		"true",
		"55296",
		"false",
		"true",
		"Bad file descriptor",
		"9",
		"false",
		"true",
		"true",
		"false",
		"true",
		"false",
		"true",
	}, "\n")
	if got != want {
		t.Fatalf("edges:\n%s", sideBySide(want, got))
	}
}

func TestTooManyLocals(t *testing.T) {
	names := make([]string, MAXVARS+1)
	for i := range names {
		names[i] = fmt.Sprintf("a%d", i)
	}
	list := strings.Join(names, ",")
	L := NewState()
	defer L.Close()
	err := L.LoadString("local "+list+"\nreturn 1\n", "=test")
	if err == nil || !strings.Contains(err.Error(), "too many local variables (limit is 200) in main function") {
		t.Fatalf("main: %v", err)
	}
	err = L.LoadString("local function f()\nlocal "+list+"\nend\nreturn 1\n", "=test")
	if err == nil || !strings.Contains(err.Error(), "too many local variables (limit is 200) in function at line 1") {
		t.Fatalf("nested: %v", err)
	}
}

func TestWarningsAndStderr(t *testing.T) {
	t.Skip("Lua 5.3.4: no warn()")

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	text := captureStderr(t, func() {
		if err := L.LoadString(`
warn("@on")
warn("hello")
warn("foo", "bar")
warn("@off")
warn("hidden")
warn("@on")
return "ok"
`, "=test"); err != nil {
			t.Fatalf("load: %v", err)
		}
		if L.PCall(0, 1, 0) != LUA_OK {
			msg, _ := L.ToString(-1)
			t.Fatalf("run: %s", msg)
		}
		s, _ := L.ToString(-1)
		if s != "ok" {
			t.Fatalf("result %q", s)
		}
		// Already on: a second control message is recognised and not printed.
		L.Warning("@on", false)
		luaE_warnerror(L, "stderr")
	})
	want := "Lua warning: hello\nLua warning: foobar\nLua warning: stderr: WARNING: failed to write warning: \n"
	if text != want {
		t.Fatalf("stderr:\n%q", text)
	}
}

func TestLuacStdinAndCannot(t *testing.T) {
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	defer func() { os.Stdin = old }()
	go func() {
		_, _ = w.Write([]byte("return 41\n"))
		_ = w.Close()
	}()

	L := NewState()
	defer L.Close()
	c := &luac{progname: "luac", err: &bytes.Buffer{}}
	if err := c.loadStdin(L); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 41 {
		t.Fatalf("stdin chunk -> %d ok=%v", n, ok)
	}

	var buf bytes.Buffer
	c = &luac{progname: "luac", output: "out.luac", err: &buf}
	c.cannot("write")
	if buf.String() != "luac: cannot write out.luac: I/O error\n" {
		t.Fatalf("cannot: %q", buf.String())
	}
	buf.Reset()
	c.output = ""
	c.cannot("open")
	if buf.String() != "luac: cannot open (null): I/O error\n" {
		t.Fatalf("cannot null: %q", buf.String())
	}
}

func TestCallInfoPoolAndThrow(t *testing.T) {
	L := NewState()
	defer L.Close()
	if L.G() != L.g {
		t.Fatal("G")
	}
	L.extendCI()
	L.shrinkCI()
	if L.ci != &L.baseCI || L.ci.next != nil || L.nci != 0 {
		t.Fatal("shrinkCI")
	}
	L.OpenLibs()
	if err := L.LoadString("return 1", "=test"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}

	if resumeError(L, "resume failed") != LUA_ERRRUN {
		t.Fatal("resumeError status")
	}
	msg, _ := L.ToString(-1)
	if msg != "resume failed" {
		t.Fatalf("resumeError %q", msg)
	}

	func() {
		defer func() {
			e, ok := recover().(*luaError)
			if !ok || e.status != LUA_ERRRUN || e.Error() != "nil" {
				t.Fatalf("throw %#v", e)
			}
		}()
		L.throw(LUA_ERRRUN)
	}()

	var e luaError
	if e.Error() != "nil" {
		t.Fatal("nil error")
	}
	setsvalue(&e.value, luaS_new(L, "boom"))
	if e.Error() != "boom" {
		t.Fatal("string error")
	}
	setivalue(&e.value, 7)
	if e.Error() != "7" {
		t.Fatalf("number error %q", e.Error())
	}
	setbvalue(&e.value, true)
	if e.Error() != "true" {
		t.Fatal("bool error")
	}
	sethvalue(&e.value, luaH_new(L))
	if !strings.Contains(e.Error(), "table") {
		t.Fatalf("table error %q", e.Error())
	}
}

func TestTableStringKeysAndMetatable(t *testing.T) {
	L := NewState()
	defer L.Close()
	t0 := luaH_new(L)
	short := luaS_new(L, "k")
	long := luaS_new(L, strings.Repeat("k", LUAI_MAXSHORTLEN+1))
	var key, val Value
	setsvalue(&key, short)
	setivalue(&val, 3)
	luaH_set(L, t0, &key, &val)
	got := luaH_getstr(L, t0, short)
	if got == nil || !ttisinteger(got) || ivalue(got) != 3 {
		t.Fatal("short key")
	}
	setsvalue(&key, long)
	setivalue(&val, 4)
	luaH_set(L, t0, &key, &val)
	got = luaH_getstr(L, t0, long)
	if got == nil || !ttisinteger(got) || ivalue(got) != 4 {
		t.Fatal("long key")
	}
	mt := luaH_new(L)
	luaH_setmetatable(t0, mt)
	if t0.metatable != mt {
		t.Fatal("metatable")
	}
	luaH_free(L, t0)
	if luaH_getstr(L, t0, short) == nil {
		t.Fatal("free is a no-op")
	}
}

func TestExtraArgAndOrderHelpers(t *testing.T) {
	fs := &FuncState{ls: &LexState{}, f: &Proto{}, jpc: NO_JUMP, lasttarget: -1}
	luaK_codek(fs, 0, MAXARG_Bx+1)
	if len(fs.f.code) != 2 || fs.f.code[0].Op != OP_LOADKX || fs.f.code[1].Op != OP_EXTRAARG || fs.f.code[1].A != MAXARG_Bx+1 {
		t.Fatalf("codek %#v", fs.f.code)
	}
	/* 5.3: C counts flush batches; EXTRAARG only when C would exceed MAXARG_C */
	fs = &FuncState{ls: &LexState{}, f: &Proto{}, jpc: NO_JUMP, lasttarget: -1}
	luaK_setlist(fs, 0, MAXARG_C*LFIELDS_PER_FLUSH+1, 1)
	if len(fs.f.code) != 2 || fs.f.code[0].Op != OP_SETLIST || fs.f.code[0].C != 0 || fs.f.code[1].Op != OP_EXTRAARG {
		t.Fatalf("setlist %#v", fs.f.code)
	}

	if !leIntFloat(1, 1.5) || leIntFloat(math.MaxInt64, 1.5) {
		t.Fatal("leIntFloat")
	}
	if !leFloatInt(1.5, 2) || leFloatInt(1.5, math.MinInt64) {
		t.Fatal("leFloatInt")
	}
	if !leIntFloat(math.MaxInt64, math.Inf(1)) || leIntFloat(math.MinInt64, math.Inf(-1)) {
		t.Fatal("leIntFloat inf")
	}
	if leFloatInt(math.NaN(), 1) || !leFloatInt(math.Inf(-1), math.MaxInt64) {
		t.Fatal("leFloatInt non-integral")
	}

	L := NewState()
	defer L.Close()
	var o Value
	var i int64
	setsvalue(&o, luaS_new(L, "42"))
	if !tointeger(L, &o, &i) || i != 42 {
		t.Fatal("tointeger 42")
	}
	setsvalue(&o, luaS_new(L, "1.5"))
	if tointeger(L, &o, &i) {
		t.Fatal("tointeger 1.5")
	}
	setsvalue(&o, luaS_new(L, "xx"))
	if tointeger(L, &o, &i) {
		t.Fatal("tointeger xx")
	}
	setivalue(&o, 7)
	if !tointeger(L, &o, &i) || i != 7 {
		t.Fatal("tointeger int")
	}
}

func TestNamesUpvaluesAndWeakClosures(t *testing.T) {
	L := NewState()
	defer L.Close()
	if L.getLocalNameAt(&Value{}) != "?" {
		t.Fatal("C frame")
	}
	p := &Proto{locvars: []LocVar{{varname: luaS_new(L, "x"), startpc: 0, endpc: 10}}}
	setclLvalue(&L.stack[0], &LClosure{p: p})
	saved := L.ci.callstatus
	L.ci.callstatus &^= CIST_C
	L.ci.savedpc = 1
	if L.getLocalNameAt(&Value{}) != "x" {
		t.Fatalf("local %q", L.getLocalNameAt(&Value{}))
	}
	L.ci.savedpc = 20
	if L.getLocalNameAt(&Value{}) != "?" {
		t.Fatal("inactive local")
	}
	L.ci.callstatus = saved

	keep := &UpVal{open: true}
	gone := &UpVal{open: true, L: L, idx: 1}
	L.openupval = []*UpVal{gone, keep}
	luaF_unlinkupval(L, gone)
	if len(L.openupval) != 1 || L.openupval[0] != keep {
		t.Fatal("unlink")
	}
	missing := &UpVal{open: true, L: L}
	luaF_unlinkupval(L, missing)
	if missing.open || missing.L != nil {
		t.Fatal("unlink missing")
	}
	L.openupval = nil

	ks := luaS_new(L, "field")
	var kv Value
	setsvalue(&kv, ks)
	proto := &Proto{
		k: []Value{kv},
		code: []Instruction{
			newABC(OP_LOADK, 1, 0, 0, 0),
			newABC(OP_GETTABLE, 2, 0, 1, 0),
		},
	}
	var name string
	rname(proto, 1, 1, &name)
	if name != "field" {
		t.Fatalf("rname constant %q", name)
	}
	rname(proto, 0, 3, &name)
	if name != "?" {
		t.Fatalf("rname unknown %q", name)
	}
	rkname(proto, 0, newABC(OP_SELF, 0, 0, 0, 1), &name)
	if name != "field" {
		t.Fatalf("rkname k %q", name)
	}
	rkname(proto, 1, newABC(OP_SELF, 0, 0, 1, 0), &name)
	if name != "field" {
		t.Fatalf("rkname r %q", name)
	}

	checkWeak := func(v Value) {
		t.Helper()
		ref := makeWeakRef(&v)
		if ref == nil || ref.dead() {
			t.Fatal("live ref")
		}
		got := ref.value()
		if got.tt != v.tt || got.p != v.p {
			t.Fatalf("value %#v", got)
		}
	}
	var v Value
	cl := &LClosure{}
	setclLvalue(&v, cl)
	checkWeak(v)
	cc := &CClosure{}
	setclCvalue(&v, cc)
	checkWeak(v)
	th := NewState()
	defer th.Close()
	setthvalue(&v, th)
	checkWeak(v)

	if !(weakLClosureRef{}).dead() || !(weakCClosureRef{}).dead() || !(weakThreadRef{}).dead() {
		t.Fatal("dead refs")
	}
	zv := (weakLClosureRef{}).value()
	if !ttisLclosure(&zv) || clLvalue(&zv) != nil {
		t.Fatal("nil closure value")
	}
	if tt := (weakCClosureRef{}).value(); !ttisCclosure(&tt) || clCvalue(&tt) != nil {
		t.Fatal("nil C closure value")
	}
	if tt := (weakThreadRef{}).value(); !ttisthread(&tt) || thvalue(&tt) != nil {
		t.Fatal("nil thread value")
	}
}

func TestStringDebugCoroutineAndOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "n.txt")
	t.Setenv("COVER_NUM", path)

	got := runChunk(t, `
local out = {}
local function say(s) out[#out + 1] = tostring(s) end

say(string.match("(ab)", "%b()"))
say(string.match("abc", "%f[%a]%a+"))
say(string.match("abc", "[a-c]+"))
say(string.match("abc", "[^x]+"))
say(string.match("ab", "()b"))
say(string.match("axxb", "a.*b"))
say(string.match("axxb", "a.-b"))
say(pcall(string.match, "a", "%2"))
say(pcall(string.match, "a", "("))
say(string.format("%+5.0d", 0))
say(string.format("%#05x", 1))
say(string.format("%o", 8))
say(string.format("%q", 1/0))
say(string.format("%q", -1/0))
say(string.format("%q", 0/0))
say(string.sub(string.format("%a", 2), 1, 2))
local packed = string.pack("c4", "a")
say(#packed)
local unpacked = string.unpack("c4", packed)
say(#unpacked)
say(string.byte(unpacked))

local info = debug.getinfo(1, "nSl")
say(info.what)
say(type(info.short_src))
say(type(debug.traceback("mark", 1)))
local function f(a)
  local x = 7
  return debug.getlocal(1, 2)
end
local n, v = f(1)
say(n)
say(v)
local u = 3
local function h() return u end
n, v = debug.getupvalue(h, 1)
say(n)
say(v)
debug.setupvalue(h, 1, 9)
say(h())

local co = coroutine.create(function(x)
  local y = coroutine.yield(x + 1)
  return y
end)
local ok, val = coroutine.resume(co, 1)
say(ok)
say(val)
say(coroutine.status(co))
ok, val = coroutine.resume(co, 5)
say(ok)
say(val)
say(coroutine.status(co))
local co2 = coroutine.create(function() error("boom") end)
ok = coroutine.resume(co2)
say(ok)
say(coroutine.status(co2))
local dead = coroutine.create(function() end)
coroutine.resume(dead)
say(coroutine.status(dead))  -- 5.3.4: no coroutine.close

local i, fl = 2, 2.5
local big = math.maxinteger
local small = math.mininteger
say(i < fl)
say(fl < i)
say(big < fl)
say(fl < big)
say(big < (1/0))
say(small < -1.5)

local fh = assert(io.open(os.getenv("COVER_NUM"), "w+"))
fh:write("12.5\n")
fh:seek("set", 0)
say(fh:read("*n"))
fh:close()
fh = assert(io.open(os.getenv("COVER_NUM"), "a"))
fh:write("z")
fh:close()

local function g()
  goto cont
  ::cont::
  return 1
end
say(g())
local nrep = 0
repeat nrep = nrep + 1 until nrep == 2
say(nrep)

return table.concat(out, "\n")
`)
	want := strings.Join([]string{
		"(ab)",
		"abc",
		"abc",
		"abc",
		"2",
		"axxb",
		"axxb",
		"false",
		"false",
		"    +",
		"0x001",
		"10",
		"1e9999",
		"-1e9999",
		"(0/0)",
		"0x",
		"4",
		"4",
		"97",
		"main",
		"string",
		"string",
		"x",
		"7",
		"u",
		"3",
		"9",
		"true",
		"2",
		"suspended",
		"true",
		"5",
		"dead",
		"false",
		"dead",
		"dead",
		"true",
		"false",
		"false",
		"true",
		"true",
		"true",
		"12.5",
		"1",
		"2",
	}, "\n")
	if got != want {
		t.Fatalf("deeper:\n%s", sideBySide(want, got))
	}
}

func TestLuacListingAndDoFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	other := filepath.Join(dir, "b.lua")
	if err := os.WriteFile(src, []byte("local function f(x)\n  return x + 1\nend\nreturn f(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("return 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Luac([]string{"luac", "-v"}, &out, &errb); code != 0 || !strings.Contains(out.String(), "PUC-Rio") {
		t.Fatalf("version %d %q %q", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-l", "-s", "-o", filepath.Join(dir, "a.out"), src, other}, &out, &errb); code != 0 {
		t.Fatalf("list %d %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "main <") || !strings.Contains(out.String(), "RETURN") {
		t.Fatalf("listing:\n%s", out.String())
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-p", src}, &out, &errb); code != 0 || out.Len() != 0 {
		t.Fatalf("parse %d %q %q", code, out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-z"}, &out, &errb); code != 1 || !strings.Contains(errb.String(), "-z") {
		t.Fatalf("bad option %d %q", code, errb.String())
	}

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.DoFile(src); err != nil {
		t.Fatal(err)
	}
	n, ok := L.ToInteger(-1)
	if !ok || n != 2 {
		t.Fatalf("dofile %d", n)
	}
	L.SetTop(0)
	L.PushString("a")
	L.PushString("b")
	L.Concat(2)
	s, _ := L.ToString(-1)
	if s != "ab" {
		t.Fatalf("concat %q", s)
	}
	L.Concat(0)
	s, _ = L.ToString(-1)
	if s != "" {
		t.Fatalf("concat 0 %q", s)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	os.Stderr = old
	b, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
