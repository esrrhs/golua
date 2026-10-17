local function add(a, b) return a + b end
local t = {1, 2, 3, x = "y", [10] = 20}
for i = 1, #t do print(i, t[i]) end
for k, v in pairs(t) do print(k, v) end
local s = ("x"):rep(3) .. "abc"
if #s > 2 then print(s) elseif #s == 0 then print("e") else print("o") end
while false do end
repeat local z = 1 until z > 0
local a, b, c = 1, 2.5, nil
print(add(a, b), c, 1 // 2, 1 % 2, 2 ^ 10, -a, #s, not c, a & 1, a | 2, a ~ 3, a << 1, a >> 1)
goto done
::done::
local co = coroutine.wrap(function() coroutine.yield(1) end)
local mt = setmetatable({}, {__index = function(_, k) return k end, __add = function(a, b) return 42 end,
  __call = function(self, x) return x end, __tostring = function() return "T" end,
  __close = function() end, __gc = function() end, __len = function() return 7 end,
  __eq = function() return true end, __lt = function() return true end, __le = function() return true end,
  __concat = function(a, b) return a end, __unm = function(a) return a end,
  __newindex = function() end, __mod = add, __pow = add, __div = add, __idiv = add,
  __band = add, __bor = add, __bxor = add, __shl = add, __shr = add, __bnot = add})
print(mt.x, mt + 1, mt(4), tostring(mt), #mt, mt == mt, mt < mt, mt <= mt, mt .. "", -mt)
local x <close> = setmetatable({}, {__close = function() print("closed") end})
local y <const> = 42
do local inner = y end
print(table.unpack{1, 2, 3})
local f = function(...) return select('#', ...), ... end
print(f(1, nil, 3))
