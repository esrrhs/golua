-- string library: patterns, captures, anchors, %b, frontier, gsub with
-- function/table replacements, format specifiers, pack/unpack.
local s = "hello world, hello lua"

print(s:find("hello"))
print(s:find("hello", 5))
print(s:find("o w"))
print(s:find("xyz"))
print(s:find("%s+"))
print(s:find("hello", 1, true)) -- plain
print(s:find(".", 1, true))

print(s:match("(%w+) (%w+)"))
print(s:match("%w+"))
print(s:match("(%a+),"))
print(s:match("^hello"))
print(s:match("lua$"))
print(s:match("()l()")) -- position captures
print(("%d"):format(0))

for w in s:gmatch("%a+") do io.write(w, "|") end
print()
for k, v in ("a=1, b=2, c=3"):gmatch("(%w+)=(%w+)") do io.write(k, "=", v, "|") end
print()
for a, b in ("x-y"):gmatch("(%a)%-(%a)") do print(a, b) end

print(s:gsub("hello", "hi"))
print(s:gsub("%a+", "W"))
print(s:gsub("(%w+)", "<%1>"))
print(s:gsub("hello", "hi", 1))
print(("abc"):gsub("%a", function(c) return c:upper() .. "!" end))
print(("a1b2c3"):gsub("%d", {["1"] = "one", ["2"] = "two"}))
print(("abc"):gsub("", "-"))
print((""):gsub("", "x"))

-- character classes and magic escapes
print(("a.b"):find("%."))
print(("a%%b"):find("%%"))
print(("[x]"):find("%["))
print(("hello"):find("%f[%w]%w+")) -- frontier
print(("THE (quick) fox"):find("%b()"))
print(("aXbXc"):gsub("X", "."))

-- case, rep, sub, byte, char, reverse, len
print(("AbC"):upper(), ("AbC"):lower())
print(("ab"):rep(3), ("ab"):rep(0), ("ab"):rep(2, "-"))
print(("hello"):sub(2, 4), ("hello"):sub(-3), ("hello"):sub(0), ("hello"):sub(10))
print(("hello"):reverse(), ("hello"):len())
print(("ABC"):byte(1, 3))
print(string.char(65, 66, 67))
print(string.format("%5.2f|%-5d|%05d|%x|%X|%o|%c|%s|%q", 3.14159, 42, 7, 255, 255, 8, 65, "str", 'a"b\n'))
print(string.format("%d %i %u %f %g %e %E %a %A", 42, -42, 42, 1.5, 1.5, 1.5, 1.5, 1.5, 1.5))
print(string.format("%s %10s %-10s|", "a", "b", "c"))
print(pcall(string.format, "%d", "x"))
print(pcall(string.format, "%y", 1))
print(pcall(string.format, "%1000000d", 1))

-- tonumber / tostring
print(tonumber("10"), tonumber("0x10"), tonumber("10.5"), tonumber(" 10 "))
print(tonumber("10", 2), tonumber("ff", 16), tonumber("z", 36), tonumber("11", 2))
print(tonumber("abc"), tonumber(""), tonumber(nil))
print(tonumber(10, 10))
print(tostring(1) == "1", tostring(1.0) == "1.0", tostring(nil) == "nil")
print(1 .. "", 1.0 .. "", 1.5 .. "")

-- string.pack / unpack (5.4)
local p = string.pack("i4i2bBhP", 100000, 200, -1, 300, 400, "hello")
print(#p)
print(string.unpack("i4i2bBhP", p))
print(string.packsize("i4i2bBhP"))
print(pcall(string.pack, "z", "x"))

-- string.dump round trip
local f = function(a) return a * 2 end
local dumped = string.dump(f)
print(#dumped > 0)
print(pcall(load, dumped))
print((load(dumped))(21))
