-- Number formatting: the words C's printf writes for non-finite values
-- (the sign of a NaN is the library's choice), the hexadecimal float
-- conversions, and the conversions between strings and numbers.

local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else t[i] = tostring(v) end
  end
  return table.concat(t, "|")
end

local nan = 0 / 0
print(string.format("%f", nan), string.format("%g", -nan))
print(string.format("%.3f", nan), string.format("%e", -nan))
print(string.format("%E", nan), string.format("%G", -nan))
print(string.format("%5.2f|", -nan), string.format("%08.3f", -nan))
print(string.format("%a", nan), string.format("%A", -nan))
print(tostring(nan), tostring(-nan))
print(string.format("%f %f", math.huge, -math.huge))
print(string.format("%g %G %e %E", math.huge, math.huge, math.huge, math.huge))
print(string.format("%5s|%-5s|", math.huge, -math.huge))
print(string.format("%+f %+d", 1, 1))
print(string.format("%a %A", 1, 1))
print(string.format("%a", 0), string.format("%a", -0.0))
print(string.format("%a", 0.5), string.format("%a", 1/3), string.format("%a", 255))
print(string.format("%.3a", 1/3), string.format("%#.0a", 1), string.format("%a", 1e300))
print(string.format("%.17g", 1/3), string.format("%.17g", 2^53))
print(string.format("%.99f", 0.1))
print(string.format("%d %i %u %x %X %o", -255, -255, 255, 255, 255, 255))
print(string.format("%#x %#o %#X", 255, 8, 255))
print(string.format("%5.2f|%-8d|%+d|% d|%08.3f", 1.5, 42, 42, 42, 1.5))
print(string.format("%d", 2^53), string.format("%x", math.maxinteger))
print(show(pcall(string.format, "%d", 1.5)))
print(show(pcall(string.format, "%d", "10")))
print(show(pcall(string.format, "%x", -1)))
print(show(pcall(string.format, "%c", 65)), show(pcall(string.format, "%c", 300)))
print(show(pcall(string.format, "%y", 1)), show(pcall(string.format, "%d")))

for _, s in ipairs{"0x10", "0X1p4", "0x.8p1", "1e2", ".5", "5.", " 1 ", "1e999",
                   "0xffffffffffffffff", "9223372036854775808", "", "z", "0b101",
                   "inf", "nan", "0x", "1e", "--1"} do
  print(string.format("%q %s", s, show(pcall(tonumber, s))))
end
print(show(pcall(tonumber, "ff", 16)), show(pcall(tonumber, "z", 36)))
print(show(pcall(tonumber, "11", 2)), show(pcall(tonumber, "11", 1)))
print(show(pcall(tonumber, "11", 37)), show(pcall(tonumber, "1", nil)))

print(7 // 2, math.type(7 // 2), 7.0 // 2, math.type(7.0 // 2))
print(-7 // 2, 7 // -2, 7 % -2, -7 % 2)
print(7.5 % 2, math.type(7.5 % 2), 7 % 2.5, math.type(7 % 2.5))
print(show(pcall(function() return 1 // 0 end)))
print(show(pcall(function() return 1 % 0 end)))
print(math.mininteger // -1, math.mininteger % -1)
print(math.maxinteger + 1, math.mininteger - 1)
print("done")
