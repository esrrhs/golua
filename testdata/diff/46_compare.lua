-- integer edges, comparison and the metamethod fallbacks.  This is
-- what pins 'LUA_COMPAT_LT_LE': with only '__lt' defined, 'a <= b' is
-- 'not (b < a)', not an error.
-- integer edges, comparison, metamethod fallbacks, '#' and holes,
-- coroutine error propagation, and whether 'next' order is stable
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end

print("== integer division and modulo ==")
local mi, ma = math.mininteger, math.maxinteger
print(show(7 // 2, -7 // 2, 7 // -2, -7 // -2))
print(show(7 % 2, -7 % 2, 7 % -2, -7 % -2))
print(show(7.0 // 2, -7.0 // 2, 7.5 % 2, -7.5 % 2))
print(rec(function() return 1 // 0 end))
print(rec(function() return 1 % 0 end))
print(rec(function() return 1.0 // 0 end))
print(rec(function() return 1.0 % 0 end))
print(rec(function() return mi // -1 end))
print(rec(function() return mi % -1 end))
print(show(mi // 1, mi % 1))
print(show(ma // 1, ma % 1))
print(show(0 // 1 == 0 and math.type(0 // 1)))
print(show(math.type(7 // 2), math.type(7 % 2), math.type(7.0 // 2)))
print(show(2 ^ 63, math.type(2 ^ 63)))
print(show(mi, ma))
print(rec(function() return ma + 1 end))
print(rec(function() return mi - 1 end))
print(rec(function() return ma * 2 end))
print(rec(function() return -mi end))
print(show(mi == -ma - 1))

print("== comparison ==")
print(show(1 < 2, "1" < "2", 1 < 1.5, "a" < "b"))
print(rec(function() return 1 < "2" end))
print(rec(function() return 1 <= nil end))
print(rec(function() return {} < {} end))
print(rec(function() return nil == false end))
print(rec(function() return 1 == "1" end))
print(rec(function() return 1.0 == 1 end))
print(show(1.0 == 1, rawequal(1.0, 1)))
print(show("a" .. 1, 1 .. "a", 1.0 .. "", 1 .. 2))
print(rec(function() return {} .. "" end))
print(rec(function() return nil .. "" end))

print("== metamethods ==")
do
  local mt = {__lt = function(a, b) return true end}
  local a, b = setmetatable({}, mt), setmetatable({}, mt)
  print(show(a < b, a > b))
  print(rec(function() return a <= b end))  -- no __le: 'not (b < a)'
  print(show(a <= b))
end
do
  local mt = {__le = function() return false end, __lt = function() return true end}
  local a, b = setmetatable({}, mt), setmetatable({}, mt)
  print(show(a <= b, a < b))
end
do
  local mt = {__eq = function() return "x" end}
  local a, b = setmetatable({}, mt), setmetatable({}, mt)
  print(show(a == b, a ~= b))
  print(show(a == a))
  print(rec(function() local c = setmetatable({}, mt); return a == c end))
end
do
  local mt = {__concat = function(a, b) return "C" end}
  print(show(setmetatable({}, mt) .. "x"))
  print(show("x" .. setmetatable({}, mt)))
  print(rec(function() return setmetatable({}, {__concat = 42}) .. "x" end))
end
do
  local mt = {__len = function() return 42 end}
  print(show(#setmetatable({}, mt)))
  print(rec(function() return #setmetatable({}, {__len = function() return "x" end}) end))
  print(show(#setmetatable({}, {__len = function() return 1.0 end})))
end
do
  local t = setmetatable({}, {__index = function(t, k) return k * 10 end})
  print(show(t[1], t.x))
  print(rec(function() return t.x end))
  local u = setmetatable({}, {__index = {a = 1}})
  print(show(u.a, u.b))
  local v = setmetatable({}, {__newindex = function(t, k, v) rawset(t, k, v * 2) end})
  v.x = 5
  print(show(v.x))
  print(rec(function() local w = setmetatable({}, {__newindex = 42}); w.x = 1 end))
end
do
  local t = setmetatable({}, {__call = function(self, a, b) return a + b end})
  print(show(t(1, 2)))
  print(rec(setmetatable({}, {__call = 42})))
end
do
  local t = setmetatable({}, {__unm = function() return "U" end})
  print(show(-t))
  local u = setmetatable({}, {__add = function(a, b) return "A" end})
  print(show(u + 1, 1 + u))
  print(rec(function() return u + u end))
  local v = setmetatable({}, {__idiv = function() return "I" end})
  print(show(v // 1))
  local w = setmetatable({}, {__mod = function() return "M" end})
  print(rec(function() return w % 1 end))
  local x = setmetatable({}, {__pow = function() return "P" end})
  print(show(x ^ 1))
end
do
  local t = setmetatable({}, {__index = function() error("get") end})
  print(rec(function() return t.x end))
  local u = setmetatable({}, {__tostring = function() return {} end})
  print(rec(tostring, u))
end

print("== length and holes ==")
print(show(#{}, #{1}, #{1, 2, 3}))
print(show(#{1, nil, 3}))
print(show(#{nil, nil, 3}))
print(show(#{[1] = 1, [2] = 2, [4] = 4}))
do
  local t = {}
  for i = 1, 100 do t[i] = i end
  t[50] = nil
  print(show(#t, t[50]))
  t[100] = nil
  print(show(#t))
end
do
  local t = {1, 2, 3}
  print(show(table.remove(t, 2), #t, t[2]))
end

print("== coroutine error propagation ==")
do
  local co = coroutine.wrap(function() error("x") end)
  print(rec(co))
end
do
  local co = coroutine.wrap(function() error({code = 1}) end)
  local ok, e = pcall(co)
  print(ok, type(e))
end
do
  local co = coroutine.wrap(function() coroutine.yield(1) error("y") end)
  print(show(co()))
  print(rec(co))
end
do
  local co = coroutine.create(function() coroutine.yield() end)
  coroutine.resume(co)
  print(show(coroutine.close(co)))
end
do
  local log = {}
  local co = coroutine.create(function()
    local x <close> = setmetatable({}, {__close = function() log[#log + 1] = "closed" end})
    coroutine.yield()
  end)
  coroutine.resume(co)
  print(show(coroutine.close(co)))
  print(table.concat(log, ","))
end
do
  local co = coroutine.create(function() coroutine.yield(1, 2, 3) end)
  print(rec(coroutine.resume, co))
  print(rec(coroutine.resume, co, "a", "b"))
end
do
  local co = coroutine.create(function(a, b) return a, b end)
  print(rec(coroutine.resume, co, 1, 2))
end
do
  local co = coroutine.create(coroutine.yield)
  print(rec(coroutine.resume, co, 1))
  print(rec(coroutine.resume, co, 2))
end
print(show(select('#', coroutine.resume(coroutine.create(function() end)))))

print("== next order is not part of the contract ==")
do
  local t = {}
  for i = 1, 20 do t["k" .. i] = i end
  local n = 0
  for k in pairs(t) do n = n + 1 end
  print(n)
end
print("done")
