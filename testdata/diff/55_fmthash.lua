-- The '#' flag and the precision of zero in string.format.
--
-- Two things are pinned here, both places where C's printf and Go's fmt
-- disagree.
--
-- The width of a '%#05x' is the width of the whole field, the "0x"
-- prefix included: PUC-Lua answers "0x001" for the value 1, padding the
-- two characters of the prefix and the one digit up to five with zeros.
-- Go's fmt takes the width as the width of the digits and answers
-- "0x00001", which is what this port used to do.  A precision cancels
-- the '0' flag, so '%#05.0x' pads with spaces either way.
--
-- And a precision of zero drops every digit of a zero, but not its
-- sign: C answers "+" for '%+.0d' and " " for '% .0d', while Go's fmt
-- answers "" for both, because fmtInteger returns before it reaches the
-- sign.  On a '%o' the '#' flag raises the precision until the first
-- digit is a zero, so the zero comes back as "0" and not "".
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

local vals = {0, 1, 255}
local function line(form)
  local out = {}
  for _, v in ipairs(vals) do
    local ok, r = pcall(string.format, form, v)
    out[#out + 1] = ok and string.format("%q", r) or "E"
  end
  print(string.format("%-10s %s", form, table.concat(out, " ")))
end

print("== the width of '%#0Nx' covers the prefix ==")
for _, f in ipairs{"%#x", "%#5x", "%#05x", "%#010x", "%#01x", "%#02x",
                   "%#03x", "%#04x", "%#06x", "%#-05x", "%#5.0x",
                   "%#05.0x", "%#010.0x", "%#5.1x", "%#05.1x"} do
  line(f)
end

print("== the same for '%X' ==")
for _, f in ipairs{"%#X", "%#05X", "%#010X", "%#05.0X", "%#010.0X"} do
  line(f)
end

print("== '#' on '%o' keeps the leading zero ==")
for _, f in ipairs{"%#o", "%#.o", "%#.0o", "%#5.o", "%#05.o", "%#010.o",
                   "%#0.o", "%#0.0o", "%#-05.o", "%#5o", "%#05o",
                   "%#.1o", "%#.5o", "%#05.5o"} do
  line(f)
end

print("== without '#' the zero and its precision are plain ==")
for _, f in ipairs{"%x", "%.0x", "%05.0x", "%5.0x", "%05x", "%#0x",
                   "%o", "%.0o", "%05.0o", "%05o"} do
  line(f)
end

print("== a zero written with a precision of zero keeps its sign ==")
for _, f in ipairs{"%.0d", "%+.0d", "% .0d", "%+5.0d", "%-+5.0d", "%+05.0d",
                   "%5.0d", "%-5.0d", "%05.0d", "%+.d", "% .d", "%+3.d",
                   "%+.0i", "%+05d", "%+5.0i"} do
  line(f)
end
line("%.1d")
line("%+.1d")

print("== a value that is not zero is unaffected ==")
print(rec(string.format, "%+.0d", 1))
print(rec(string.format, "%+.0d", -1))
print(rec(string.format, "% .0d", -1))
print(rec(string.format, "%+.0f", 0.0))
print(rec(string.format, "%+5.0f", 0.0))
print(rec(string.format, "%+.0x", 0))

print("== '#' where it is not allowed ==")
print(rec(string.format, "%#d", 1))
print(rec(string.format, "%#s", "a"))
print(rec(string.format, "%#f", 1.0))
print(rec(string.format, "%#c", 65))
print("done")
