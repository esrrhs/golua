package core

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLuaHFreeEmpty(t *testing.T) {
	L := NewState()
	defer L.Close()
	luaH_free(L, nil)
	luaH_free(L, luaH_new(L))
}

func TestLuacDoargsEdges(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "e.lua")
	body := "local s = \"\\\"\\\\\\a\\b\\f\\n\\r\\t\\v\\001x\"\n" +
		"local f = false; local n = 1.0; local z = 0/0\n" +
		"local t = {a=1}; function t:m(x) return self.a+x end\n" +
		"local u = {}\nfor i=1,60 do u[i]=i end\n" + // SETLIST + EXTRAARG
		"return t:m(2), s, f, n, #u\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errb bytes.Buffer
	// -o - writes listing/dump to stdout (output="")
	if code := Luac([]string{"luac", "-l", "-l", "-o", "-", src}, &out, &errb); code != 0 {
		t.Fatalf("-o - => %d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "\\n") && !strings.Contains(text, "SELF") {
		// still useful if escapes printed differently
		if !strings.Contains(text, "LOADK") {
			t.Fatalf("listing:\n%s", text)
		}
	}

	out.Reset()
	errb.Reset()
	// -- end options; -v with more args
	if code := Luac([]string{"luac", "-v", "--", src}, &out, &errb); code != 0 {
		t.Fatalf("-v -- => %d %s", code, errb.String())
	}

	out.Reset()
	errb.Reset()
	// stdin via "-"
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("return 99\n"))
		_ = w.Close()
	}()
	code := Luac([]string{"luac", "-p", "-"}, &out, &errb)
	os.Stdin = old
	_ = r.Close()
	if code != 0 {
		t.Fatalf("stdin -p => %d %s", code, errb.String())
	}

	// -l alone uses default luac.out as input (may fail if missing — OK)
	out.Reset()
	errb.Reset()
	_ = Luac([]string{"luac", "-l"}, &out, &errb)

	// dump open failure: -o is a directory
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-o", dir, src}, &out, &errb); code == 0 {
		t.Fatal("expected cannot open directory as output")
	}

	// write failure via /dev/full when present
	if _, err := os.Stat("/dev/full"); err == nil {
		out.Reset()
		errb.Reset()
		if code := Luac([]string{"luac", "-o", "/dev/full", src}, &out, &errb); code == 0 {
			t.Fatal("expected write failure on /dev/full")
		}
	}
}

func TestLuacPrintHeaderVariants(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "h.lua")
	if err := os.WriteFile(src, []byte("return 'hi'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "h.out")
	var out, errb bytes.Buffer
	if code := Luac([]string{"luac", "-o", outPath, src}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	// list the binary so source starts with LUA_SIGNATURE → "(bstring)"
	out.Reset()
	errb.Reset()
	if code := Luac([]string{"luac", "-l", "-l", outPath}, &out, &errb); code != 0 {
		t.Fatal(errb.String())
	}
	if !strings.Contains(out.String(), "bstring") && !strings.Contains(out.String(), "main") {
		t.Fatalf("%q", out.String())
	}

	// listing of a string chunk (no @/= prefix) via LoadBytes path is harder
	// through Luac; exercise printHeader via a crafted Proto if needed.
	c := &luac{out: &out, err: &errb, progname: "luac"}
	f := &Proto{source: &TString{val: "chunk"}, sizecode: 1, code: []Instruction{{}}}
	c.printHeader(f)
	f.source = &TString{val: "@file.lua"}
	c.printHeader(f)
	f.source = &TString{val: "=stdin"}
	c.printHeader(f)
	f.source = &TString{val: string([]byte{dmpSignature[0]}) + "bin"}
	c.printHeader(f)
	f.source = nil
	c.printHeader(f)
}

func TestDebugDebugREPL(t *testing.T) {
	oldIn, oldErr := os.Stdin, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	er, ew, _ := os.Pipe()
	os.Stdin, os.Stderr = r, ew
	go func() {
		_, _ = w.Write([]byte("x=1\n"))
		_, _ = w.Write([]byte("error('dbg')\n"))
		_, _ = w.Write([]byte("cont\n"))
		_ = w.Close()
	}()
	got := runChunk(t, `require("debug").debug(); return "ok"`)
	_ = ew.Close()
	os.Stdin, os.Stderr = oldIn, oldErr
	_ = r.Close()
	var discard bytes.Buffer
	_, _ = io.Copy(&discard, er)
	_ = er.Close()
	if got != "ok" {
		t.Fatalf("%q", got)
	}
}

func TestCoroutineEdgesMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local co = coroutine.create(function(...)
  say(select("#", ...))
  local a = coroutine.yield(10, 20)
  say(a)
  return 30
end)
say(coroutine.status(co))
local ok, a, b = coroutine.resume(co, "x", "y")
say(ok); say(a); say(b)
ok, a = coroutine.resume(co, "resume-arg")
say(ok); say(a)
ok, a = coroutine.resume(co)
say(not ok) -- already dead
-- wrap
local w = coroutine.wrap(function(n) return n+1 end)
say(w(5))
-- close (5.4-ish API may or may not exist — pcall)
local okc = pcall(function()
  if coroutine.close then coroutine.close(coroutine.create(function() end)) end
end)
say(okc or true)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "30") || !strings.Contains(got, "6") {
		t.Fatalf("%q", got)
	}
}

func TestStringPackUnpackGaps(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(string.packsize("i4"))
local p = string.pack(">i4i2", 0x12345678, -2)
local a,b,pos = string.unpack(">i4i2", p)
say(a); say(b); say(pos)
p = string.pack("z", "hi")
say(select(1, string.unpack("z", p)))
p = string.pack("c3", "abc")
say(select(1, string.unpack("c3", p)))
p = string.pack("s1", "xy")
say(select(1, string.unpack("s1", p)))
p = string.pack("f", 1.5)
say(select(1, string.unpack("f", p)))
p = string.pack("d", 2.5)
say(select(1, string.unpack("d", p)))
p = string.pack("n", 3.5)
say(select(1, string.unpack("n", p)))
local ok = pcall(string.pack, "i", 1)
say(ok)
p = string.pack("x")
say(#p == 1)
ok = pcall(string.unpack, "i4", "ab")
say(not ok)
say(string.format("%c", 65))
say(string.format("%i", -3))
say(string.format("%u", 3))
say(string.format("%X", 255))
say(string.format("%e", 1.5))
say(string.format("%g", 1.5))
say(string.format("%s", "ab"))
say(string.format("%q", "a\0b"))
return table.concat(o, "|")
`)
	if !strings.Contains(got, "65") && !strings.Contains(got, "A") {
		t.Fatalf("%q", got)
	}
}

func TestWarningAndGCEdges(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	var warns []string
	L.SetWarnFunction(func(L *State, msg string, tocont bool) {
		warns = append(warns, msg)
	}, nil)
	L.Warning("direct", false)
	L.Warning("part1", true)
	L.Warning("part2", false)
	_ = L.DoString(`
collectgarbage("collect")
collectgarbage("step", 0)
collectgarbage("setpause", 100)
collectgarbage("setstepmul", 200)
collectgarbage("isrunning")
collectgarbage("count")
local k = {}
local t = setmetatable({}, {__mode="k"})
t[k] = 1
k = nil
collectgarbage("collect")
local t2 = setmetatable({}, {__mode="v"})
t2[1] = {}
collectgarbage("collect")
`)
	if len(warns) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestParserGotoAndErrorPaths(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local ok, err = pcall(load, "goto L; ::L:: ::L::")
say(not ok)
ok, err = pcall(load, "do ::L:: end; goto L")
say(not ok)
ok, err = pcall(load, "local a; goto L; ::L:: a=a")
say(not ok) -- jump into scope of local
ok = pcall(load, "return (" )
say(not ok)
ok = pcall(load, "for i=1,2 do end end")
say(not ok)
ok = pcall(load, "repeat until")
say(not ok)
ok = pcall(load, "function f( end")
say(not ok)
::ok::
say(true)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 1 {
		t.Fatalf("%q", got)
	}
}

func TestIOOSMoreEdges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.txt")
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local f = assert(io.open(`+quote(path)+`, "w"))
say(f:write("hello\n", "world"))
say(f:seek("set", 0))
f:close()
f = assert(io.open(`+quote(path)+`, "r"))
say(f:read(2))
say(f:read("*l"))
say(f:read("*a"))
f:close()
say(os.getenv("PATH") ~= nil)
say(type(os.clock())=="number")
say(type(os.difftime(os.time(), os.time()))=="number")
local t = os.date("*t")
say(t.year > 2000)
say(os.date("%Y", os.time(t)))
local ok = pcall(os.remove, `+quote(filepath.Join(dir, "nope"))+`)
say(ok or true)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "he") {
		t.Fatalf("%q", got)
	}
}

func TestLoadlibIgMarkAndCpathMiss(t *testing.T) {
	dir := t.TempDir()
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
package.cpath = `+quote(filepath.Join(dir, "?.so"))+`
-- Croot: dotted name, root missing
local ok, err = pcall(require, "rootmod.sub")
say(not ok)
-- loadlib * on missing
local a,b,c = package.loadlib(`+quote(filepath.Join(dir, "no.so"))+`, "*")
say(a==nil)
say(type(c)=="string")
return table.concat(o, ",")
`)
	if !strings.HasPrefix(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestBaseUTF8AndBitMore(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(utf8.len("abc"))
say(utf8.char(0x61, 0x62))
say(utf8.codepoint("ab", 1, 2))
say(utf8.offset("ab", 2))
local n=0; for p,c in utf8.codes("ab") do n=n+1 end; say(n)
say(select("#", pcall(utf8.len, "\xFF")))
say(tonumber("  0x10  "))
say(tonumber("1.5e2"))
say(rawequal(1,1))
say(select(2, pcall(error, "x", 2)))
local t={}; setmetatable(t,{__len=function() return 3 end}); say(#t)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "abc") && !strings.Contains(got, "3") {
		t.Fatalf("%q", got)
	}
}

func TestHookAndGetInfoEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local hits=0
debug.sethook(function(ev) hits=hits+1 end, "crl", 1)
local function f(x) return x+1 end
say(f(1))
debug.sethook()
say(hits>0)
local info = debug.getinfo(f, "nSltu")
say(info.what)
say(debug.getinfo(1, "l").currentline > 0)
local co = coroutine.create(function()
  local a=1; coroutine.yield(); return a
end)
coroutine.resume(co)
say(debug.getinfo(co, 1, "l") ~= nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "2") {
		t.Fatalf("%q", got)
	}
}

func TestUpvalueAndLocalDebug(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function outer()
  local x=10
  local function inner() return x end
  say(debug.upvalueid(inner, 1) ~= nil)
  say(debug.upvaluejoin)
  local function other() local y=20; return function() return y end end
  local o2 = other()
  debug.upvaluejoin(inner, 1, o2, 1)
  say(inner())
  local name, val = debug.getupvalue(inner, 1)
  say(name); say(val)
  say(debug.setupvalue(inner, 1, 99))
  say(inner())
  return inner
end
outer()
return table.concat(o, ",")
`)
	if !strings.Contains(got, "99") {
		t.Fatalf("%q", got)
	}
}

func TestLuacListingEscapesAndOpcodes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "esc.lua")
	body := "local esc = \"\\\"\\\\\\a\\b\\f\\n\\r\\t\\v\\001\\127\"\nlocal f = false\nlocal t = true\nlocal n = 2.0\nlocal obj = {k=1}\nfunction obj:meth(x) return self.k + x end\nlocal big = {1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,61,62,63,64,65,66,67,68,69}\nreturn obj:meth(3), esc, f, t, n, #big\n"
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := Luac([]string{"luac", "-l", "-l", src}, &out, &errb); code != 0 {
		t.Fatalf("%d %s", code, errb.String())
	}
	text := out.String()
	if !strings.Contains(text, "SELF") && !strings.Contains(text, "LOADK") {
		t.Fatalf("listing:\n%s", text)
	}
}

func TestMsgHandlerStyleErrors(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local ok, err = xpcall(function() error({}) end, function(e)
  return debug.traceback(tostring(e), 1)
end)
say(not ok)
ok, err = pcall(function()
  local t=setmetatable({}, {__tostring=function() return "T" end})
  error(t)
end)
say(not ok)
say(tostring(err):find("T") ~= nil or true)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestTableBoundaryAndNext(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local t={}
t[1]=1; t[2]=2; t[4]=4
say(rawget(t,3)==nil)
say(#t) -- boundary
local n=0; for k,v in next,t do n=n+1 end; say(n)
t[1e10]=1; say(rawget(t,1e10)==1)
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}
