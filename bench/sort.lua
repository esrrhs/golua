-- table.sort over a large random array.
--
-- Every comparison is a call back into Lua, so this measures the call
-- path, the array part of a table and the sort loop together.  The seed
-- is fixed so that two interpreters sort the same input.
--
--     lua bench/sort.lua [n]
math.randomseed(42)
local n = tonumber(arg[1] or 200000)

local t = {}
for i = 1, n do
  t[i] = math.random(1000000)
end
table.sort(t)
print("sort", t[1], t[n])
