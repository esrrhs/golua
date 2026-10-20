-- A grid of tables, then a given number of full collections.
--
-- This is the memory-bound counterpart to the CPU workloads: it builds
-- 'n' six-field nodes and then collects, so what it measures is the
-- cost of a value and of a node, and the cost of walking them.  Run it
-- once with 'rounds' at zero to time the build alone, and once with a
-- larger count to time the collections; the difference over 'rounds'
-- is one pass of the collector.
--
--     lua bench/mesh_gc.lua 2000000 0     -- build only
--     lua bench/mesh_gc.lua 2000000 20    -- build plus twenty passes
--
-- Six fields put every node in the hash part, which is where a value's
-- size and a node's size show up most; a run of plain integers would
-- only measure the array part, where a value is all that is stored.
local n = tonumber(arg[1] or 2000000)
local rounds = tonumber(arg[2] or 1)

local t = {}
for i = 1, n do
  t[i] = {x = i, y = i, z = i, a = i, b = i, c = i}
end

collectgarbage("collect")
for _ = 1, rounds do
  collectgarbage("collect")
end

print("ok")
