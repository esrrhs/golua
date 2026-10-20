-- Bytes one node of the grid costs, as the collector counts them.
--
-- Build the grid, stop the collector so that the counting collection
-- below is the only one that can move the figure, take the count,
-- release every node, collect again: what went away, divided by 'n',
-- is the answer.
--
--     lua bench/node_size.lua 1000000
--
-- The figure is what 'collectgarbage("count")' reports, in kilobytes,
-- so it is the interpreter's own accounting rather than the resident
-- set -- which is what makes it comparable between interpreters with
-- different allocators.
local n = tonumber(arg[1] or 1000000)

local t = {}
for i = 1, n do
  t[i] = {x = i, y = i, z = i, a = i, b = i, c = i}
end
collectgarbage("collect")
collectgarbage("stop")

local before = collectgarbage("count")
for i = 1, n do
  t[i] = nil
end
collectgarbage("collect")

local after = collectgarbage("count")
print(string.format("%.1f", (before - after) * 1024 / n))
