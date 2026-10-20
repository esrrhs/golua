local ops = {}
local mt = {
	__add = function(a, b) ops[#ops + 1] = "add"; return a.v + b.v end,
	__sub = function(a, b) return a.v - b.v end,
	__mul = function(a, b) return a.v * b.v end,
	__div = function(a, b) return a.v / b.v end,
	__mod = function(a, b) return a.v % b.v end,
	__pow = function(a, b) return a.v ^ b.v end,
	__unm = function(a) return -a.v end,
	__idiv = function(a, b) return a.v // b.v end,
	__band = function(a, b) return a.v & b.v end,
	__bor = function(a, b) return a.v | b.v end,
	__bxor = function(a, b) return a.v ~ b.v end,
	__bnot = function(a) return ~a.v end,
	__shl = function(a, b) return a.v << b.v end,
	__shr = function(a, b) return a.v >> b.v end,
	__concat = function(a, b) return tostring(a) .. tostring(b) end,
	__len = function(a) return 99 end,
	__eq = function(a, b) return a.v == b.v end,
	__lt = function(a, b) return a.v < b.v end,
	__le = function(a, b) return a.v <= b.v end,
	__index = function(t, k) return "idx:" .. tostring(k) end,
	__newindex = function(t, k, v) rawset(t, k, "set:" .. tostring(v)) end,
	__call = function(self, a, b) return (a or 0) + (b or 0) end,
	__tostring = function(self) return "obj(" .. tostring(self.v) .. ")" end,
	__close = function(self, err) ops[#ops + 1] = "close:" .. tostring(err) end,
}
local function mk(v) return setmetatable({v = v}, mt) end
local a, b = mk(10), mk(3)
print(a + b, a - b, a * b, a / b, a % b, a ^ b, -a, a // b)
print(a & b, a | b, a ~ b, ~a, a << b, a >> b)
print(#a, a == mk(10), a < b, a <= b, a > b, a >= b)
print(tostring(a), a(1, 2))
a.foo = 7
print(rawget(a, "foo"))
print(a.missing)
do
	local z <close> = mk(1)
end
print(table.concat(ops, ","))
local base = {greet = function(self) return "hi " .. self.n end}
base.__index = base
local o = setmetatable({n = "bob"}, base)
print(o:greet())
print(getmetatable(o) == base)
local prot = setmetatable({}, {__metatable = "locked"})
print(getmetatable(prot))
print(pcall(setmetatable, prot, {}))
print(rawequal(a, a), rawequal(a, mk(10)))
local inherit = setmetatable({}, {__index = base})
inherit.__index = inherit
print(setmetatable({n = "amy"}, inherit):greet())
print(pcall(function() return nil + 1 end))
print(pcall(function() return {} + {} end))
print(pcall(function() return #print end))
