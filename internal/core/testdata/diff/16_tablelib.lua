-- table library, iteration order-independent where possible, sorting and
-- the generic 'for' protocol.
local t = {1, 2, 3, 4, 5}
print(table.concat(t), table.concat(t, ","), table.concat(t, ",", 2, 4))
print(table.concat({}, "x"), table.concat({1}, "x"))
print(pcall(table.concat, {1, {}}, ","))

table.insert(t, 6)
print(table.concat(t, ","))
table.insert(t, 1, 0)
print(table.concat(t, ","))
print(table.remove(t), table.concat(t, ","))
print(table.remove(t, 1), table.concat(t, ","))
print(pcall(table.insert, t, 100, 1))
print(pcall(table.remove, t, 100))

local p = table.pack(1, nil, 3)
print(p.n, p[1], p[2], p[3])
print(table.unpack({1, 2, 3}))
print(table.unpack({1, 2, 3}, 2))
print(table.unpack({1, 2, 3}, 2, 3))
print(pcall(table.unpack, {}, 1, 1e7))

local a = {1, 2, 3, 4, 5}
table.move(a, 1, 3, 3)
print(table.concat(a, ","))
local b = {10, 20, 30, 40, 50}
table.move(a, 1, 5, 1, b)
print(table.concat(b, ","))
print(table.move({1, 2}, 1, 2, 3) == nil or "moved")

-- sort
local s = {5, 3, 1, 4, 2}
table.sort(s)
print(table.concat(s, ","))
table.sort(s, function(x, y) return x > y end)
print(table.concat(s, ","))
print(pcall(table.sort, {3, 1, 2}, function() return true end))
local s2 = {"bb", "a", "ccc"}
table.sort(s2)
print(table.concat(s2, ","))

-- next / pairs / ipairs
local n = 0
for k, v in pairs({a = 1, b = 2}) do n = n + 1 end
print(n)
print(next({}))
local tt = {x = 1}
local k1, v1 = next(tt)
print(k1, v1, next(tt, k1))
print(pcall(next, {}))
print(pcall(next, tt, "zzz"))

-- __pairs / __ipairs
local obj = setmetatable({}, {
	__pairs = function(t) return function(_, k) if not k then return 1, "one" end end, t, nil end,
	__index = function(_, k) return "idx:" .. tostring(k) end,
})
for k, v in pairs(obj) do print("pairs", k, v) end
print(obj[1], obj.foo)

-- rawget / rawset / rawequal / rawlen
local r = setmetatable({}, {__index = function() return "MM" end, __newindex = function() error("no") end,
	__len = function() return 99 end, __eq = function() return true end})
print(r.anything)
print(rawget(r, "x"))
rawset(r, "x", 1)
print(rawget(r, "x"), r.x)
print(#r, rawlen(r))
print(rawequal(r, {}), r == {})

-- table constructors with holes and # semantics
local h = {1, 2, nil, 4}
print(#h >= 1)
local h2 = {}
h2[1] = 1
h2[2] = 2
h2[4] = 4
print(#h2 >= 1)

-- numeric keys and float keys
local f = {}
f[1.0] = "float-one"
print(f[1], f[1.0])
f[2.5] = "x"
print(f[2.5])

-- remove with float index
print(pcall(table.remove, {1, 2, 3}, 2.0))
print(pcall(table.remove, {1, 2, 3}, 2.5))
print(pcall(table.concat, {1, 2}, ",", 1.0, 2.0))
