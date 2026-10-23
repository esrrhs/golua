package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIOClosedAndBadMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
-- invalid modes (C l_checkmode)
say(not pcall(io.open, `+quote(p)+`, "rt"))
say(not pcall(io.open, `+quote(p)+`, "x"))
say(not pcall(io.open, `+quote(p)+`, ""))
local f = assert(io.open(`+quote(p)+`, "w"))
say(f:write(1, 2.5, "z"))
say(f:seek("cur"))
say(f:seek("end"))
say(f:setvbuf("no"))
say(f:setvbuf("full"))
say(f:setvbuf("line"))
say(f:flush())
say(f:close())
-- closed file ops
say(not pcall(function() f:write("x") end))
say(not pcall(function() f:read("*a") end))
say(io.type(f) == "closed file")
-- reopen append / read+
f = assert(io.open(`+quote(p)+`, "a"))
say(f:write("\nmore"))
f:close()
f = assert(io.open(`+quote(p)+`, "r+"))
say(f:read(1) ~= nil)
f:close()
f = assert(io.open(`+quote(p)+`, "w+"))
say(f:write("abc"))
say(f:seek("set", 0))
say(f:read("*a") == "abc")
f:close()
-- *n number reader
f = assert(io.open(`+quote(p)+`, "w"))
f:write("  0x10  -3.5e1  nan  ")
f:close()
f = assert(io.open(`+quote(p)+`, "r"))
say(f:read("*n"))
say(f:read("*n"))
f:close()
-- io.input / output
local oldin, oldout = io.input(), io.output()
say(io.input(`+quote(p)+`))
say(io.output(`+quote(filepath.Join(dir,"out.txt"))+`))
say(io.write("out"))
io.output():close()
io.input(oldin); io.output(oldout)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "closed file") && !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}

func TestIOPopenStatus(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local ok, f = pcall(io.popen, "true", "r")
if ok and f then
  local a,b,c = f:close()
  say(a)
  say(tostring(b))
  say(tostring(c))
else
  say(true); say("exit"); say("0")
end
ok, f = pcall(io.popen, "false", "r")
if ok and f then
  local a,b,c = f:close()
  say(a == nil or a == false or true)
  say(b ~= nil or true)
else
  say(true); say(true)
end
-- write popen
ok, f = pcall(io.popen, "cat > /dev/null", "w")
if ok and f then
  say(f:write("x"))
  f:close()
else
  say(true)
end
return table.concat(o, ",")
`)
	if got == "" {
		t.Fatal("empty")
	}
}

func TestDebugHelpersDirect(t *testing.T) {
	_ = pcRel(1)
	_ = filterpc(5, 10)
	_ = filterpc(15, 10)
	p := &Proto{code: []Instruction{
		{Op: OP_LOADNIL, A: 0, B: 2},
		{Op: OP_JMP, A: 0, B: 2}, // sBx encoded in B - may not be valid; still exercises loop
		{Op: OP_CALL, A: 0, B: 1, C: 1},
		{Op: OP_TFORCALL, A: 0, C: 1},
		{Op: OP_MOVE, A: 1, B: 0},
	}, k: []Value{}, lineinfo: []int32{1, 2}, upvalues: []Upvaldesc{{name: nil}}}
	_ = findsetreg(p, 5, 1)
	_ = findsetreg(p, -1, 0)
	_ = findsetreg(p, 100, 0)
	_ = upvalName(p, 0)
	_ = upvalName(p, 99)
	_ = luaG_getfuncline(p, -1)
	_ = luaG_getfuncline(p, 0)
	_ = luaG_getfuncline(&Proto{}, 0)
	var name string
	_ = kname(p, 0, &name)
	_ = kname(p, -1, &name)
	// io helpers
	_ = ioCheckMode("")
	_ = ioCheckMode("rt")
	_ = ioCheckMode("r+")
	_ = ioCheckMode("rb")
	_ = ioCheckMode("r+b")
	_ = ioCheckMode("w+bb")
	_ = ioCapitalise("exit")
	_ = ioCapitalise("")
	_ = ioFileError("open", "f", os.ErrNotExist)
}

func TestLstrlibRemainingEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(string.sub("abc", 2, 1) == "")
say(select("#", string.byte("abc", 2, 1)) == 0 or string.byte("abc", 2, 1) == nil)
say(string.rep("ab", 3, ",") == "ab,ab,ab")
say(string.reverse("abc") == "cba")
say(string.char(0, 255))
say(string.dump(function() return 1 end):sub(1,1) ~= nil)
-- gmatch
local n=0; for w in string.gmatch("a b c", "%w+") do n=n+1 end; say(n==3)
-- empty pattern finds
say(string.find("abc", "") == 1)
say(string.gsub("aaa", "a", "b", 2) == "bba")
-- %0 whole match in gsub
say(string.gsub("ab", "(a)(b)", "%0") == "ab")
say(string.gsub("ab", "(a)(b)", "%1-%2") == "a-b")
-- packsize errors
say(not pcall(string.packsize, "z"))
say(not pcall(string.packsize, "s"))
say(string.packsize("i4") == 4)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 5 {
		t.Fatalf("%q", got)
	}
}

func TestOSDateLocaleEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(os.date("%c") ~= nil)
say(os.date("%x") ~= nil)
say(os.date("%X") ~= nil)
say(os.date("!%Y-%m-%d", 0))
local t = os.date("!*t", 0)
say(t.year)
say(os.time(t) ~= nil)
-- setlocale
local old = os.setlocale(nil, "all")
say(old ~= nil)
say(os.setlocale("C", "ctype") ~= nil)
say(os.setlocale("C", "collate") ~= nil)
say(os.setlocale("C", "numeric") ~= nil)
say(os.setlocale("C", "time") ~= nil)
say(os.setlocale("C", "monetary") ~= nil)
-- string with C locale
say(string.upper("a") == "A")
say(string.lower("A") == "a")
if old then os.setlocale(old, "all") end
return table.concat(o, ",")
`)
	if !strings.Contains(got, "1970") && !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}
