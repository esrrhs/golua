-- string.pack, string.unpack and string.packsize.
--
-- The format string is a language of its own -- endianness, alignment,
-- sizes that are not byte multiples, and options that are only legal for
-- some of them -- so the whole space is asked here, including the formats
-- that have to be refused.
--
-- Two things are deliberately kept out:
--
--   - 'string.pack("c1000000000000", 1)'.  'getnum' stops reading digits
--     once the number passes (MAXSIZE-9)/10, so the option is 'c' with a
--     size of one billion and three digits are left over; PUC-Lua then
--     really does build the gigabyte before reporting "invalid format
--     option '0'".  A one million byte field pins the same code path --
--     a translation that pads the field one byte at a time by growing
--     the string is quadratic and never finishes -- for a thousandth of
--     the memory.
--
--   - the bytes of a large 'cN' field.  Only its length is printed, so
--     the golden file stays readable.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    elseif type(v) == "number" then t[i] = string.format("%.17g", v)
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end
-- like 'rec', but a packed string is reported by its length
local function recp(f, v)
  local ok, r = pcall(f, v)
  if not ok then return "false|" .. string.format("%q", r) end
  return "true|" .. #r
end
local function hex(s)
  local o = {}
  for i = 1, #s do o[i] = string.format("%02x", string.byte(s, i)) end
  return table.concat(o)
end

local opts = {"b", "B", "h", "H", "l", "L", "j", "J", "T", "f", "d", "n",
              "z", "x", "Xi", "Xo", "s", "s1", "s4", "c0", "c1", "c3",
              "c8", "i", "i1", "i2", "i3", "i4", "i5", "i7", "i8", "i16",
              "i17", "I", "I1", "I3", "I8", "I16", "I17", "i0", "I0",
              "b0", "b1", "b9", "h3", "l-1", "!!", "<", ">", "=", "!"}
print("== packsize ==")
for _, f in ipairs(opts) do
  print(string.format("%q", f), rec(string.packsize, f))
end
print("== packsize of a whole format ==")
for _, f in ipairs{"", "i4i4", "<i4>i4", "!8i1", "!8 b", "!4 d", "!8Xi",
                   "!8 s4", "!8 c3", "j", "T", "z", "s", "c0"} do
  print(string.format("%q", f), rec(string.packsize, f))
end

print("== pack and unpack, one value ==")
for _, f in ipairs{"b", "B", "h", "H", "l", "L", "j", "J", "T", "f", "d", "n",
                   "i1", "i2", "i3", "i4", "i8", "I1", "I3", "I8", "i", "I"} do
  local out = {}
  for _, v in ipairs{0, 1, -1, 127, -128, 255, 32767, -32768, 65535,
                     2147483647, -2147483648, math.maxinteger,
                     math.mininteger, 1.5, -1.5, 1 / 0, 0 / 0} do
    local ok, p = pcall(string.pack, f, v)
    out[#out + 1] = ok and hex(p) or "E"
  end
  print(string.format("%-4s", f), table.concat(out, " "))
end

print("== pack and unpack round trip ==")
for _, f in ipairs{"i4", "I4", "i8", "<i4", ">i4", "=i4", "!4i4", "!8i8",
                   "d", "n", "f", "j", "T", "b", "B", "h", "H", "l", "L"} do
  for _, v in ipairs{0, 1, -1, 1000000, -1000000, math.maxinteger,
                     math.mininteger, 3.25} do
    local ok, p = pcall(string.pack, f, v)
    local ok2, r, pos
    if ok then ok2, r, pos = pcall(string.unpack, f, p) end
    print(string.format("%-6s %-22s %s %s", f, string.format("%.17g", v),
          ok and hex(p) or "E", ok2 and (tostring(r) .. "@" .. tostring(pos)) or "E"))
  end
end

print("== strings: z, s, sN, cN ==")
for _, f in ipairs{"z", "s", "s1", "s2", "s4", "s8", "c0", "c1", "c3", "c8",
                   "c100"} do
  local out = {}
  for _, v in ipairs{"", "a", "abc", string.rep("x", 10)} do
    local ok, p = pcall(string.pack, f, v)
    if not ok then out[#out + 1] = "E"; goto cont end
    local ok2, r, pos = pcall(string.unpack, f, p)
    out[#out + 1] = hex(p) .. "->" .. (ok2 and string.format("%q", r) .. "@" .. tostring(pos) or "E")
    ::cont::
  end
  print(string.format("%-6s", f), table.concat(out, " "))
end

print("== several values at once ==")
print(hex(string.pack("i4i4i4", 1, 2, 3)), show(string.unpack("i4i4i4", string.pack("i4i4i4", 1, 2, 3))))
print(show(string.unpack("i4zi4", string.pack("i4zi4", 7, "ab", 9))))
print(show(string.unpack("<!4i4", string.pack("<!4i4", 1))))
print(show(string.unpack("i4 i4", string.pack("i4 i4", 1, 2))))

print("== unpack with a position ==")
local p = string.pack("i4i4i4", 1, 2, 3)
for _, pos in ipairs{0, 1, 5, 9, 12, 13, -1, -4, -5, -12, -13} do
  print(pos, rec(string.unpack, "i4", p, pos))
end

print("== a big fixed-size field ==")
print(string.packsize("c1000000"), #string.pack("c1000000", "ab"))
local big = string.pack("c1000000", "ab")
local r, pos = string.unpack("c1000000", big)
print(#r, r == "ab" .. string.rep("\0", 999998), pos)
big = string.pack("i4c1000000i4", 1, "ab", 2)
print(#big, string.byte(big, 1, 8), select("#", string.unpack("i4c1000000i4", big)))
print(show(string.unpack("i4xi4", string.pack("i4c1000000i4", 1, "ab", 2):sub(1, 9))))

print("== what has to be refused ==")
-- 'getnum' stops reading digits once the number passes (MAXSIZE-9)/10, so
-- the digits left over are the next, illegal, option: 'c2147483647' is
-- 'c' with a size of 214748364 and then a stray '7'.  Such a field is
-- never packed -- building a gigabyte to report its length is no way to
-- pin a parser -- so only 'packsize' is asked for the huge ones.
for _, f in ipairs{"", "i", "I", "i9", "i-1", "i17", "I0", "i0", "y", "!",
                   "!!", "!1", "!17", "!7i4", "!!8i4", "!8", "Xi8", "Xl",
                   "b0", "b9", "h0", "h9", "l0", "l17", "s0", "c", "c-1",
                   "z1", "x1", "f1", "d9", "n9", "j9", "T9", "i4.5", "%",
                   "i 4", " i4", "i4 ", "s-1", "c2147483647",
                   "c1000000000000", "c99999999999", "c2000000000"} do
  local ok, sz = pcall(string.packsize, f)
  local pk
  if ok and sz > 100000 then
    pk = "SKIP(" .. sz .. ")"
  else
    pk = recp(string.pack, f, 1)
  end
  print(string.format("%q", f), rec(string.packsize, f), pk)
end

print("== argument counts and types ==")
print(rec(string.pack, "i4"), rec(string.pack, "i4", 1, 2))
print(rec(string.pack), rec(string.pack, 5, 1))
print(rec(string.pack, "i4", "x"), rec(string.pack, "i4", {}))
print(rec(string.pack, "i4", nil), rec(string.pack, "i4", true))
print(rec(string.pack, "i4", 1.5), rec(string.pack, "i4", 2 ^ 63))
print(rec(string.pack, "z", 5))
print(rec(string.unpack, "i4", ""), rec(string.unpack, "i4", "abc"))
print(rec(string.unpack, "i4"), rec(string.unpack))
print(rec(string.unpack, 5, "abc"))
print(rec(string.packsize), rec(string.packsize, 5))
print("done")
