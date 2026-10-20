-- The length operator, over tables built and pulled apart at random.
--
-- '#' is the one operator Lua defines only loosely: it answers *a*
-- border, and which one is up to the implementation.  So matching
-- PUC-Lua line for line is the only specification there is, and every
-- line below is a border the reference interpreter picked after a write
-- or a delete that may have moved the array part, shrunk 'alimit', or
-- pushed the search out into the hash part with 'unbound_search'.
--
-- The walk is deterministic: the keys are integers, and where an integer
-- ends up does not depend on the hash seed.

local s = 20251017
local function rnd(n)
	s = (s * 1103515245 + 12345) % 2147483648
	return ((s >> 16) % n) + 1
end

local function walk(t, rounds, span, tag)
	for i = 1, rounds do
		local k = rnd(span)
		if rnd(3) == 1 then
			t[k] = nil
			print(string.format("%s del %d -> %d", tag, k, #t))
		else
			t[k] = k
			print(string.format("%s set %d -> %d", tag, k, #t))
		end
	end
end

-- dense: keys inside a small array part, so 'alimit' is what answers
local a = {}
walk(a, 60, 16, "dense")

-- sparse: one far-out key, which is what sends '#' into unbound_search
local b = {}
b[1000] = 1
print("sparse 1000 -> " .. #b)
b[2000] = 1
print("sparse 2000 -> " .. #b)
walk(b, 40, 2048, "sparse")

-- a filled prefix, then holes punched in it from the far end backwards
local c = {}
for i = 1, 64 do c[i] = i end
print(string.format("prefix full -> %d", #c))
for i = 64, 1, -3 do
	c[i] = nil
	print(string.format("prefix del %d -> %d", i, #c))
end

-- ... and from the near end forwards, which is what shrinks 'alimit'
local d = {}
for i = 1, 64 do d[i] = i end
for i = 1, 64, 3 do
	d[i] = nil
	print(string.format("shrink del %d -> %d", i, #d))
end

-- integer-valued floats are the same key
local e = {}
e[1] = 1
e[2.0] = 2
e[3.0] = 3
print("float -> " .. #e)
e[2.0] = nil
print("float del -> " .. #e)

-- the limit left behind by a big array part that has been emptied
local f = {}
for i = 1, 300 do f[i] = i end
for i = 1, 300 do f[i] = nil end
print(string.format("emptied -> %d", #f))
f[1] = 1
print(string.format("refilled 1 -> %d", #f))
f[2] = 2
print(string.format("refilled 2 -> %d", #f))
f[4] = 4
print(string.format("refilled 4 -> %d", #f))
f[3] = 3
print(string.format("refilled 3 -> %d", #f))

-- 'table.insert' and 'table.remove' move the same border
local g = {}
for i = 1, 10 do
	table.insert(g, i)
	print(string.format("insert %d -> %d", i, #g))
end
for i = 1, 10 do
	table.remove(g, rnd(#g))
	print(string.format("remove -> %d", #g))
end

-- a walk over a table that has been through all of the above
local h = {}
walk(h, 80, 128, "mixed")
print("OK")
