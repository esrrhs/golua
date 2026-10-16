-- closures & upvalues
local function counter()
	local n = 0
	return function() n = n + 1; return n end
end
local c = counter()
print(c(), c(), c())
local cs = {counter(), counter()}
print(cs[1](), cs[1](), cs[2]())

-- varargs
local function f(...)
	local n = select("#", ...)
	local a, b = ...
	return n, a, b, select(-1, ...)
end
print(f())
print(f(1))
print(f(1, 2, 3))
print(f(nil, nil))
local function g(...)
	local t = table.pack(...)
	return t.n, t[1], t[2]
end
print(g(1, nil, 3))

-- goto
do
	local i = 0
	::top::
	i = i + 1
	if i < 3 then goto top end
	print("goto", i)
end

-- numeric for
local s = 0
for i = 1, 10, 2 do s = s + i end
print("for1", s)
s = 0
for i = 10, 1, -3 do s = s + i end
print("for2", s)
s = 0
for x = 1.0, 2.0, 0.25 do s = s + 1 end
print("for3", s)
for i = 1, 0 do print("never") end

-- generic for
local t = {10, 20, 30}
local r = 0
for i, v in ipairs(t) do r = r + i * v end
print("ipairs", r)
local ks = {}
for k in pairs({x = 1, y = 2, z = 3}) do ks[#ks + 1] = k end
table.sort(ks)
print("pairs", table.concat(ks, ","))

-- integer/float
print(1 / 2, 3 // 2, 3 % 2, 2 ^ 63 == 2 ^ 63)
print(tostring(1.0), tostring(-0.0), tostring(1e15), tostring(1e16))
print(0.1 + 0.2 == 0.3, math.type(1 + 1), math.type(1 + 1.0))
print("10" + 5, "3.5" * 2, 10 .. 20)
print(1 == 1.0, math.maxinteger // 1)

-- scoping / shadowing
local x = "outer"
do
	local x = "inner"
	print(x)
end
print(x)

-- multiple returns & nesting
local function mr() return 1, 2, 3 end
print(mr())
print((mr()))
print(({mr()})[3])
print(select(2, mr()))
print(#{mr()})

-- string coercion in concat
print(1 .. 2 .. "a", 1.5 .. "", true .. "" == nil)
