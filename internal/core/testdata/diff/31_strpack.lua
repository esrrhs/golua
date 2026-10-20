-- string.pack / unpack / packsize, byte for byte.
--
-- The interesting corner is a missing argument: C's 'str_pack' pushes a
-- nil before it walks the format (lstrlib.c: "mark to separate arguments
-- from string buffer"), so the slot a missing value would have sat in
-- already holds a nil and the message says "got nil" rather than the
-- "got no value" every other library function reports.

local function hex(s)
  return (s:gsub(".", function(c) return string.format("%02x", string.byte(c)) end))
end
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
local function rec(f, ...) return show(pcall(f, ...)) end

-- 1. the marker nil: absent arguments are reported as nil, not "no value"
print(rec(string.pack, "B"))
print(rec(string.pack, "BB", 1))
print(rec(string.pack, "BBB", 1))
print(rec(string.pack, "s"))
print(rec(string.pack, "d"))
print(rec(math.abs))      -- every other function says "no value"
print(rec(string.rep))

-- 2. sizes
for _, f in ipairs{"b", "B", "h", "H", "l", "L", "j", "J", "T", "f", "d", "n",
                   "i", "I", "i1", "i2", "i4", "i8", "i16", "I1", "I8", "I16",
                   "x", "z", "s", "s1", "s4", "s8", "Xi", "Xs", "Xn", "X1",
                   "X4", "X8", "X0", "X16"} do
  print(f, rec(string.packsize, f))
end

-- 3. integers, both ends
for _, f in ipairs{"b", "B", "h", "H", "l", "L", "j", "J", "T"} do
  local ok, p = pcall(string.pack, f, -1)
  print(f, hex(string.pack(f, 0)), hex(string.pack(f, 1)), ok and hex(p) or tostring(p))
end
print(hex(string.pack("<I4", 0x01020304)), hex(string.pack(">I4", 0x01020304)))
print(hex(string.pack("=I4", 0x01020304)), hex(string.pack("!4I4", 0x01020304)))
print(hex(string.pack(">i8", -1)), hex(string.pack("<i8", -1)))
print(hex(string.pack(">I8", math.maxinteger)), hex(string.pack("<I8", math.maxinteger)))
for n = 1, 16 do
  print("i" .. n, hex(string.pack("<i" .. n, -1)), hex(string.pack(">i" .. n, -1)))
end
for n = 1, 16 do
  print("I" .. n, hex(string.pack("<I" .. n, 0xff)), hex(string.pack(">I" .. n, 0xff)))
end
print(rec(string.pack, "<i1", 128), rec(string.pack, "<I1", 256))
print(rec(string.pack, "<i17", 1), rec(string.pack, "<i0", 1))
print(rec(string.pack, "i", 1.5), rec(string.pack, "B", {}))

-- 4. floats, including the non-finite ones
print(hex(string.pack("<f", 1.0)), hex(string.pack(">f", 1.0)))
print(hex(string.pack("<d", 1.0)), hex(string.pack(">d", 1.0)))
print(hex(string.pack("<d", 0.0)), hex(string.pack("<d", -0.0)))
print(hex(string.pack("<d", 1/0)), hex(string.pack("<d", -1/0)))
print(hex(string.pack("<d", 0/0)))
print(hex(string.pack("<f", 1/0)), hex(string.pack("<f", 0/0)))
print(hex(string.pack("<f", math.pi)), hex(string.pack(">f", math.pi)))
print(show(string.unpack("<f", string.pack("<f", 1/0))))
print(show(string.unpack("<d", string.pack("<d", 0.1))))
print(rec(string.pack, "d", "x"))

-- 5. strings
print(hex(string.pack("s", "hello")))
print(hex(string.pack("s1", "hello")), hex(string.pack("s2", "hello")))
print(hex(string.pack("<s4", "hello")), hex(string.pack(">s4", "hello")))
print(hex(string.pack("z", "hello")))
print(hex(string.pack("c0", "")), hex(string.pack("c5", "hello")))
print(hex(string.pack("c10", "hi")))
print(rec(string.pack, "c10", "toolongforit"))
print(rec(string.pack, "s1", ("x"):rep(300)))
print(rec(string.pack, "z", "a\0b"))
print(rec(string.pack, "s", 42))
print(show(string.unpack("z", "hi\0rest")))
print(show(string.unpack("s1", string.pack("s1", "hi"))))
print(show(string.unpack("c3", "abcdef")))
print(rec(string.unpack, "z", "no terminator"))
print(rec(string.unpack, "c3", "ab"))

-- 6. padding and alignment
print(hex(string.pack("xxx", nil)))
print(hex(string.pack("!8Xi", 0)), rec(string.packsize, "!8Xi"))
print(rec(string.packsize, "!0Xi"), rec(string.packsize, "!9Xi"))
print(hex(string.pack("!4Xi", 0)), hex(string.pack("Xi", 0)))
print(hex(string.pack("i4Xi", 1)), hex(string.pack("bXi", 1)))
print(hex(string.pack("!8bXi", 1)), hex(string.pack("!8d", 1.0)))
print(rec(string.packsize, "X"), rec(string.packsize, "Xq"))
print(rec(string.packsize, "q"), rec(string.packsize, ""))
print(rec(string.packsize, 42), rec(string.packsize))

-- 7. unpack with a position
local packed = string.pack("<I2<i2c3", 1, -2, "abc")
print(hex(packed))
print(show(string.unpack("<I2<i2c3", packed)))
print(show(string.unpack("<i2", packed, 3)))
print(show(string.unpack("<c3", packed, 5)))
print(rec(string.unpack, "<I2", packed, 100))
print(rec(string.unpack, "<I2", packed, 0))
print(rec(string.unpack, "<I2", packed, 6))
print(rec(string.unpack, "<I2", packed, #packed + 1))
print(show(string.unpack("B", "\1\2\3", 2)))

-- 8. round trips
for _, f in ipairs{"<I4<i4", ">I4>i4", "<i1<i2<i4<i8", ">I1>I2>I4>I8",
                   "<f<d", ">f>d", "<s", ">s4", "z", "<c4", "<I3>I3"} do
  local vals = {}
  for c in f:gmatch("[a-zA-Z]") do
    if c == "s" or c == "z" or c == "c" then vals[#vals + 1] = "xy"
    elseif c == "f" or c == "d" or c == "n" then vals[#vals + 1] = 1.5
    else vals[#vals + 1] = -3 end
  end
  if #vals > 0 then
    local ok, p = pcall(string.pack, f, table.unpack(vals))
    print(f, ok and (#p .. "|" .. show(string.unpack(f, p))) or tostring(p))
  end
end

-- 9. range checks
print(rec(string.pack, "<i4", math.mininteger))
print(rec(string.pack, "<I4", -1), rec(string.pack, "<I4", 2^32))
print(rec(string.pack, "<I8", 0xffffffffffffffff))
print(rec(string.pack, "j", 2^63))
print(math.type(string.unpack("<I4", string.pack("<I4", 0xffffffff))))
print(math.type(string.unpack("<i4", string.pack("<i4", -1))))
print(show(string.unpack("B", "\255")))
