-- The string library: format, concat and gmatch over short strings.
--
-- Short strings are the ones an interpreter interns, so this is the
-- workload that the interning scheme and the string hashing live in,
-- with the collector's attention on a large set of small objects.
--
--     lua bench/strings.lua [n]
local n = tonumber(arg[1] or 20000)

local parts = {}
for i = 1, n do
  parts[i] = string.format("%d-abcdefgh", i)
end
local s = table.concat(parts, ",")

local w = 0
for m in string.gmatch(s, "%a+") do
  w = w + #m
end
print("str", w)
