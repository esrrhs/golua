-- The width and the precision of '%s' in string.format.
--
-- What pins them is that C counts both in *bytes* while Go's fmt counts
-- runes.  "\xe4\xbd\xa0\xe5\xa5\xbd" is six bytes and two characters, so
-- '%6s' pads it with nothing in PUC-Lua and used to pad it with four
-- spaces here, and '%.3s' is the first three bytes -- one character --
-- where this port used to take the first three characters, which is the
-- whole string.  Anything that lines text up in columns with '%s' is
-- affected as soon as a character is not ASCII.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end

local two = "\xe4\xbd\xa0\xe5\xa5\xbd"      -- two characters, six bytes
local four = "\xf0\x9f\x98\x80\xf0\x9f\x98\x81" -- two characters, eight bytes
local bad = "\xff\xfe"                      -- not UTF-8 at all
print(#two, #four, #bad)

print("== width ==")
for _, f in ipairs{"%s", "%1s", "%2s", "%3s", "%5s", "%6s", "%7s", "%10s",
                   "%-1s", "%-6s", "%-10s", "%.s", "%99s"} do
  -- a width may not start with '0': '%s' takes no '0' flag
  print(string.format("%-6s", string.format("%q", f)), show(string.format("[" .. f .. "]", two)))
end

print("== precision ==")
for _, f in ipairs{"%.s", "%.1s", "%.2s", "%.3s", "%.4s", "%.5s", "%.6s",
                   "%.7s", "%.0s", "%6.3s", "%-6.3s", "%3.1s", "%10.2s"} do
  print(string.format("%-8s", string.format("%q", f)), show(string.format("[" .. f .. "]", two)))
end

print("== four bytes per character ==")
for _, f in ipairs{"%s", "%4s", "%8s", "%-8s", "%.4s", "%.1s", "%8.4s"} do
  print(string.format("%-6s", string.format("%q", f)), show(string.format("[" .. f .. "]", four)))
end

print("== bytes that are not UTF-8 ==")
for _, f in ipairs{"%s", "%4s", "%-4s", "%.1s", "%.2s", "%4.1s"} do
  print(string.format("%-6s", string.format("%q", f)), show(string.format("[" .. f .. "]", bad)))
end

print("== ASCII is the same either way ==")
for _, f in ipairs{"%s", "%5s", "%-5s", "%.2s", "%5.2s", "%-5.2s"} do
  print(string.format("%-6s", string.format("%q", f)), show(string.format("[" .. f .. "]", "abc")))
end
for _, f in ipairs{"%5s", "%-5s", "%.2s"} do
  print(string.format("%-6s", string.format("%q", f)), show(string.format("[" .. f .. "]", "")))
end

print("== the length is what decides, not the characters ==")
for _, s in ipairs{"a", "ab", "abc", "\xc3\xa9", "\xc3\xa9\xc3\xa8",
                   "\xe4\xb8\xad\xe6\x96\x87\xe5\xad\x97"} do
  print(show(s), #string.format("%8s", s), #string.format("%-8s", s),
        #string.format("%.2s", s), show(string.format("%8s", s)))
end

print("== a string too long to be formatted ==")
local long = string.rep("x", 100)
local long8 = string.rep("\xe4\xbd\xa0", 40) -- 120 bytes, 40 characters
print(#string.format("%10s", long), #string.format("%10s", long8))
print(#string.format("%.10s", long), #string.format("%.10s", long8))
print(#string.format("%10.5s", long8), show(string.format("%.5s", long8)))
print(#string.format("%-10s", long8))

print("== zeros and the other specifiers ==")
print(rec(string.format, "%5s", "a\0b"))
print(rec(string.format, "%.5s", "a\0b"))
print(show(string.format("[%5d]", 42)))
print(show(string.format("[%-5d]", 42)))
print(show(string.format("[%5.2f]", 3.14159)))
print(show(string.format("[%5s%d]", "a", 1)))
print(show(string.format("[%s%s%s]", "a", "b", two)))
print(show(string.format("[%1$s]", "a")))
print(rec(string.format, "%1s", 42))
print(rec(string.format, "%5", "a"))
print(show(string.format("%q", two)))
print(show(string.rep(two, 2)))
print("done")
