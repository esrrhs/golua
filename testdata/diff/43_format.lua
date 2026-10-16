-- string.format, tonumber and tostring: every specifier and every
-- edge.  Nothing here depends on the host: no clock, no files, no
-- addresses that are not heap pointers.
-- string.format, tonumber, tostring: every specifier and every edge
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

print("== integers ==")
for _, v in ipairs{0, 1, -1, 7, -7, 255, 2147483647, -2147483648,
                   math.maxinteger, math.mininteger} do
  print(show(string.format("%d %i %u %x %X %o %5d %-5d %05d %+d % d",
             v, v, v, v, v, v, v, v, v, v, v)))
end
print("== integer modifiers ==")
print(show(string.format("%#x %#X %#o", 255, 255, 8)))
print(show(string.format("%.5d %.0d", 42, 0)))
print(rec(string.format, "%d", 1.5))
print(rec(string.format, "%d", "10"))
print(rec(string.format, "%d", "0x10"))
print(rec(string.format, "%x", -1))

print("== floats ==")
for _, v in ipairs{0.0, 1.0, -1.0, 0.5, 1/3, 1e15, 1e-15, 1e300, 1e-300,
                   2.5, -2.5, 3.5, 0.1, 123456789.123456789} do
  print(show(string.format("%f %.3f %g %G %e %E %a %A %g %.14g",
             v, v, v, v, v, v, v, v, v, v)))
end
print("== float modifiers ==")
print(show(string.format("%10.2f|%-10.2f|%+f|% f|%.0f|%#.0f",
                         3.14159, 3.14159, 3.14159, 3.14159, 2.5, 2.0)))
print(rec(string.format, "%f", "x"))
print(rec(string.format, "%d %s", 1))
print(rec(string.format, "%s %d", 1, 2, 3))
print(rec(string.format, "%1000000d", 1))
print(rec(string.format, "%z", 1))

print("== inf and nan ==")
local inf = 1 / 0
local nan = 0 / 0
print(show(string.format("%f %g %e %a", inf, inf, inf, inf)))
print(show(string.format("%f %g %e %a", -inf, -inf, -inf, -inf)))
print(show(string.format("%f %g %e %a", nan, nan, nan, nan)))
print(rec(string.format, "%d", inf))
print(rec(string.format, "%d", nan))

print("== strings and %q ==")
print(show(string.format("%s %10s %-10s %.3s %5.2s", "abc", "abc", "abc", "abc", "abc")))
print(show(string.format("%q", "a\"b\\c\nd\te\0f")))
print(show(string.format("%q", "with 'quotes'")))
print(show(string.format("%q", 1)))
print(show(string.format("%q", 1.0)))
print(show(string.format("%q", true)))
print(show(string.format("%q", nil)))
print(rec(string.format, "%s", nil))
print(rec(string.format, "%s", {}))
print(rec(string.format, "%c", 65))
print(rec(string.format, "%c", 300))
print(rec(string.format, "%c"))
print(rec(string.format, "%c", "x"))

print("== percent ==")
print(show(string.format("%%")))
print(show(string.format("100%%")))
print(rec(string.format, "%"))
print(rec(string.format))
print(rec(string.format, nil))
print(rec(string.format, 42))
print(show(string.format("a%sb%dc", "x", 1)))

print("== tonumber ==")
local nums = {"0", "1", "-1", " 1 ", "\t1\n", "1e2", "1E2", ".5", "5.",
              "0x10", "0X10", "-0x10", "0x", "1e", "e1", "", " ", "1 2",
              "0x1p4", "0x.8p1", "0x1p-1", "inf", "nan", "0xfffffffffffff",
              "9223372036854775807", "9223372036854775808",
              "-9223372036854775808", "-9223372036854775809",
              "1e400", "-1e400", "0.0", "1e-400", "0b101", "010"}
for _, s in ipairs(nums) do
  print(string.format("%q", s), show(tonumber(s)))
end
print("== tonumber base ==")
for _, b in ipairs{2, 8, 10, 16, 36} do
  print(b, show(tonumber("z", b)))
end
print(show(tonumber("11", 2)))
print(show(tonumber("777", 8)))
print(show(tonumber("ff", 16)))
print(rec(tonumber, "1", 1))
print(rec(tonumber, "1", 37))
print(rec(tonumber, "1", 0))
print(rec(tonumber, "1", 1.5))
print(rec(tonumber, 1))
print(rec(tonumber, 1, 10))
print(rec(tonumber))
print(rec(tonumber, nil))

print("== tostring ==")
print(show(tostring(nil), tostring(true), tostring(1), tostring(1.0)))
print(show(tostring(-0.0)))
print(show(tostring(1e15), tostring(1e16)))
print(show(tostring(math.maxinteger), tostring(math.mininteger)))
print(show(tostring(0/0)))
print(show(tostring(1/0), tostring(-1/0)))
do
  local t = setmetatable({}, {__tostring = function() return "TS" end})
  print(show(tostring(t)))
  print(show(tostring(setmetatable({}, {__name = "Named"}))))
  print(show(rec(tostring, setmetatable({}, {__tostring = function() error("boom") end}))))
  print(show(rec(tostring, setmetatable({}, {__tostring = 42}))))
end
print("done")
