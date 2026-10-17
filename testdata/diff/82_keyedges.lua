-- Where a number stops being an array index and becomes a hash key.
--
-- 'arrayindex' accepts only 1..2^31-1, and a float is an array index
-- only when it has an exact integer value -- so 1.0 is the key 1, -0.0
-- is the key 0 (which is a hash key, not an index), and 2^53 is a hash
-- key even though it is a whole number.  Every answer below is
-- deterministic: no key here lands in the hash part on account of the
-- hash seed.

local t = {}

t[0] = "zero"
t[-1] = "minus"
t[2 ^ 31] = "two31"
t[2 ^ 31 - 1] = "two31-1"
t[1.5] = "frac"
t[-0.0] = "negzero"
t[1.0] = "onefloat"
t["1"] = "onestring"
t[math.mininteger] = "minint"
t[math.maxinteger] = "maxint"
t[2 ^ 53] = "two53"

print(t[0], t[-0.0], t[0.0])
print(t[1], t[1.0], t["1"])
print(t[2 ^ 31], t[2 ^ 31 - 1], t[2 ^ 53])
print(t[math.mininteger], t[math.maxinteger])
print(#t)

local n = 0
for k, v in pairs(t) do n = n + 1 end
print("keys", n)

-- a float with a fractional part is its own key, never an integer's
local f = {}
f[1.5] = "a"
f[1] = "b"
print(f[1.5], f[1], #f)

-- an integer-valued float writes the integer
local g = {}
g[3.0] = "three"
print(g[3], g[3.0], #g)
g[3] = nil
print(g[3], #g)

-- the array part is there, and a far-out key does not change '#'
local h = {}
for i = 1, 100 do h[i] = i end
h[2 ^ 31] = 1
print(#h)
h[101] = 101
print(#h)
h[100] = nil
print(#h)

-- keys that cannot be used at all
print(pcall(function() local u = {} u[0 / 0] = 1 end))
print(pcall(function() local u = {} return u[0 / 0] end))
print(pcall(function() local u = {} u[nil] = 1 end))
print(pcall(function() local u = {} u[nil] = nil end))

-- 'rawlen' and 'rawget' see the same table the operator does
local r = {}
r[1], r[2], r[4] = "a", "b", "d"
print(#r, rawlen(r), rawlen("abc"), rawlen(7))
print(pcall(rawlen, true))
print("OK")
