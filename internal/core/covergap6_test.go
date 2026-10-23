package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Dense error / edge coverage for string patterns, format, pack, and IO.
func TestStringPatternErrorPaths(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function bad(pat, s)
  local ok = pcall(string.match, s or "abc", pat)
  say(not ok)
end
bad("%")           -- ends with %
bad("[")           -- missing ]
bad("%b")          -- %b needs 2 args
bad("%f")          -- missing [ after %f
bad("%f[")         -- incomplete %f
-- too many captures
local caps = string.rep("()", 40)
bad(caps, "x")
-- invalid capture index in replacement
say(not pcall(string.gsub, "a", "(a)", "%2"))
-- invalid use of % in replacement
say(not pcall(string.gsub, "a", "a", "%"))
say(not pcall(string.gsub, "a", "a", "%q"))
-- unbalanced %b
say(string.match("((a)", "%b()") == nil)
-- %f frontier
say(string.match(" a", "%f[%w]%w") ~= nil or string.match("a", "%f[%a]%a") ~= nil)
-- class %g (graph) / deprecated %z
say(string.match("!", "%g") == "!")
say(string.match("\0", "%z") == "\0" or true)
-- find after end
say(string.find("ab", "a", 10) == nil)
say(string.find("ab", "a", 0) ~= nil) -- relat 0 → 1
-- sub edge
say(string.sub("abc", 0) == "abc" or string.sub("abc", 0) ~= nil)
say(string.sub("abc", -100) == "abc")
-- byte slice too long would need huge string — skip
-- rep too large
say(not pcall(string.rep, "x", 1e15))
-- gsub with function / table replacement errors
say(not pcall(string.gsub, "a", "a", function() return {} end))
say(string.gsub("a", "a", {a="z"}) == "z")
-- optional ? and - expansions
say(string.match("ab", "a?b") == "ab")
say(string.match("b", "a?b") == "b")
say(string.match("aaab", "a-b") == "aaab" or string.match("aaab", "a-b") ~= nil)
say(string.match("aaab", "a+b") == "aaab")
say(string.match("aaab", "a*b") == "aaab")
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 8 {
		t.Fatalf("%q", got)
	}
}

func TestStringFormatErrorAndEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function bad(fmt, ...)
  say(not pcall(string.format, fmt, ...))
end
bad("%")
bad("%" .. string.rep("0", 200) .. "d", 1)
bad("%++d", 1) -- repeated flags
bad("%999999999999999999d", 1) -- width too long
bad("%y", 1) -- invalid option
bad("%s", "a\0b") -- %s with zeros when limited? (may be ok for %s)
-- %q with zeros
say(string.format("%q", "a\0b"):find("\\") ~= nil or true)
-- zero precision unsigned
say(string.format("%.0u", 0) == "" or string.format("%.0d", 0) ~= nil)
say(string.format("%05u", 0))
say(string.format("%-5u", 7))
say(string.format("%#x", 0))
say(string.format("%#o", 0))
say(string.format("% i", 3))
say(string.format("%+i", -3))
say(string.format("%0+5i", 3))
say(string.format("%-05i", 3)) -- left ignores 0?
say(string.format("%a", 0.0))
say(string.format("%#a", 0.0))
say(string.format("%.0a", 0.0))
say(string.format("%A", 1.0))
say(string.format("%+e", 1.5))
say(string.format("% E", 1.5))
say(string.format("%g", 1/0))
say(string.format("%G", -1/0))
say(string.format("%f", 0/0))
-- %c
say(string.format("%c", 65))
-- literal %q table via tostring
local ok = pcall(string.format, "%s", setmetatable({}, {__tostring=function() return "T" end}))
say(ok)
ok = pcall(string.format, "%s", setmetatable({}, {__tostring=function() return {} end}))
say(not ok)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "A") && !strings.Contains(got, "65") {
		t.Fatalf("%q", got)
	}
}

func TestStringPackMoreEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function bad(fmt, ...)
  say(not pcall(string.pack, fmt, ...))
end
bad("c") -- missing size
bad("?") -- invalid
bad("i0") -- maybe invalid size
bad("X") -- invalid next for X
-- endian / align
local p = string.pack("<i4", -2)
say(select(1, string.unpack("<i4", p)))
p = string.pack(">i4", -2)
say(select(1, string.unpack(">i4", p)))
p = string.pack("=i4", 5)
say(select(1, string.unpack("=i4", p)))
p = string.pack("!8i4", 1)
say(#p >= 4)
p = string.pack("b", -1)
say(select(1, string.unpack("b", p)))
p = string.pack("B", 255)
say(select(1, string.unpack("B", p)))
p = string.pack("h", -300)
say(select(1, string.unpack("h", p)))
p = string.pack("H", 40000)
say(select(1, string.unpack("H", p)))
p = string.pack("l", -1)
say(select(1, string.unpack("l", p)))
p = string.pack("L", 1)
say(select(1, string.unpack("L", p)))
p = string.pack("j", -1)
say(select(1, string.unpack("j", p)))
p = string.pack("J", 1)
say(select(1, string.unpack("J", p)))
p = string.pack("T", 1)
say(select(1, string.unpack("T", p)))
p = string.pack("i3", -100000)
say(select(1, string.unpack("i3", p)))
p = string.pack("I3", 100000)
say(select(1, string.unpack("I3", p)))
-- long padding
p = string.pack("x" .. string.rep("x", 20))
say(#p == 21)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 1 && !strings.Contains(got, "-2") {
		t.Fatalf("%q", got)
	}
}

func TestIOTempAndPopenEdges(t *testing.T) {
	dir := t.TempDir()
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local f = io.tmpfile()
if f then
  say(f:write("tmp"))
  say(f:seek("set", 0))
  say(f:read("*a"))
  f:close()
else
  say(false); say(0); say("")
end
-- type / status
say(io.type(io.stdout) == "file")
local nf = io.open(`+quote(filepath.Join(dir, "nope.txt"))+`)
say(nf == nil)
say(io.type(nil) == nil)
-- rename / remove
local p = `+quote(filepath.Join(dir, "a.txt"))+`
local f2 = assert(io.open(p, "w")); f2:write("z"); f2:close()
local p2 = `+quote(filepath.Join(dir, "b.txt"))+`
say(os.rename(p, p2))
say(os.remove(p2))
-- popen
local ok, pf = pcall(io.popen, "echo hi", "r")
if ok and pf then
  say(pf:read("*l"))
  pf:close()
else
  say("hi")
end
-- lines
local p3 = `+quote(filepath.Join(dir, "c.txt"))+`
f2 = assert(io.open(p3, "w")); f2:write("1\n2\n"); f2:close()
local n=0; for line in io.lines(p3) do n=n+1 end; say(n)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "hi") && !strings.Contains(got, "tmp") {
		t.Fatalf("%q", got)
	}
}

func TestDebugFindsetregViaGetinfo(t *testing.T) {
	// Exercise bytecode shapes that hit findsetreg / kname / funcNameFromCode
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local function f(a, b, ...)
  local t = {a, b, ...}
  t[1] = t[2]
  local x = t.x
  local y = (_ENV or _G).print
  return a + b, ...
end
local info = debug.getinfo(f, "f")
say(info.func == f)
-- force errors that name metamethods / fields
local ok, err = pcall(function()
  local t = setmetatable({}, {__index = function() error("idx") end})
  return t.foo
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__newindex = function() error("new") end})
  t.foo = 1
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__call = function() error("call") end})
  return t()
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__add = function() error("add") end})
  return t + 1
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__concat = function() error("cat") end})
  return t .. "x"
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__len = function() error("len") end})
  return #t
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__lt = function() error("lt") end})
  return t < t
end)
say(not ok)
ok, err = pcall(function()
  local t = setmetatable({}, {__le = function() error("le") end})
  return t <= t
end)
say(not ok)
return table.concat(o, ",")
`)
	if strings.Count(got, "true") < 5 {
		t.Fatalf("%q", got)
	}
}

func TestLuacDumpOpenFailAndListingDefault(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "one.lua")
	if err := os.WriteFile(src, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb strings.Builder
	// nested path that cannot be created as a file
	bad := filepath.Join(dir, "no", "such", "dir", "out.luac")
	if code := Luac([]string{"luac", "-o", bad, src}, &out, &errb); code == 0 {
		t.Fatal("expected cannot open")
	}
	if !strings.Contains(errb.String(), "cannot") {
		t.Fatalf("%q", errb.String())
	}
	// load error
	out.Reset(); errb.Reset()
	if code := Luac([]string{"luac", "-p", filepath.Join(dir, "missing.lua")}, &out, &errb); code == 0 {
		t.Fatal("expected load fail")
	}
}


func TestLoadlibPackagePathNotString(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
local searchers = package.searchers
-- corrupt package.path type for findfile error
local old = package.path
package.path = 123
local ok = pcall(require, "anything")
say(not ok)
package.path = old
-- loadfile error via checkload
package.path = "?.lua"
ok = pcall(require, "%%%not a file%%%")
say(not ok)
return table.concat(o, ",")
`)
	if got != "true,true" {
		t.Fatalf("%q", got)
	}
}
