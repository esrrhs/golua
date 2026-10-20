-- the language itself: integer against float, scoping, closures, varargs,
-- goto, the numeric 'for' at the edges of the integer range, and _ENV.

local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else t[i] = tostring(v) end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end

-- 1. integer against float
print(math.type(1), math.type(1.0), math.type(2^31), math.type(2^63))
print(math.tointeger(1.0), math.tointeger(1.5), math.tointeger("3"), math.tointeger(2^63))
print(tostring(1), tostring(1.0), tostring(-0.0), tostring(2^53))
print(tostring(math.maxinteger), tostring(math.mininteger))
print(tostring(1e15), tostring(1e16), tostring(1e300), tostring(1e-300))
print(string.format("%s|%s|%s|%s", 1, 1.0, -0.0, 2^63))
print(rec(string.format, "%d", 3.0), rec(string.format, "%d", 3.5))
print(rec(string.format, "%d", 2^63))
print(string.format("%d %i %u %x %o", -7, -7, 7, 255, 8))
print(string.format("%5.2f|%-5.2f|%+.2f|% d", 3.14159, 3.14159, 3.14159, 7))
print(string.format("%.3s|%.3d|%.0f", "abcdef", 7, 7.5))
print(rec(string.format, "%d", "7"), rec(string.format, "%f", "x"))
print(rec(string.format, "%s"), rec(string.format, "%y", 1))

-- 2. arithmetic at the edges
print(show(7 // 2, -7 // 2, 7.5 // 2, -7.5 // 2))
print(show(7 % 2, -7 % 2, 7.5 % 2, -7.5 % 2, 7 % -2, -7 % -2))
print(show(2^63, -2^63, 2^0.5, (-8)^(1/3)))
print(show(math.maxinteger + 1 == math.mininteger))
print(show(math.maxinteger * 2, math.mininteger // -1, math.mininteger % -1))
print(show(1 == 1.0, math.maxinteger == math.maxinteger + 0.0))
print(show(math.maxinteger < 2^63, 2^63 < math.huge))
print(show(3 & 5, 3 | 5, 3 ~ 5, ~5, 1 << 10, 1024 >> 3))
print(show(3.0 & 5, 3.0 | 5.0, -1 & 0xFF, 0xFF << 56))
print(rec(function() return 2^63 // 1 end))
print(show(math.maxinteger + 1.0, math.mininteger - 1.0))
print(show(math.fmod(7, 2), math.fmod(-7, 2), math.fmod(7, -2), math.fmod(7.5, 2)))
print(rec(math.fmod, 7, 0), rec(math.fmod, 7.0, 0))
print(show(math.floor(3.7), math.ceil(3.2), math.floor(-3.2), math.ceil(-3.7)))
print(rec(math.floor, 2^63), rec(math.floor, "3.5"))
print(show(math.abs(-3), math.abs(-3.5), math.abs(math.mininteger)))

-- 3. string coercion
print(show("10" + 1, "10" - 1, "10" * 2, "10" / 4, "10" ^ 2))
print(show("1e2" + 0, " 10 " + 0, "0x10" + 0, ".5" + 0))
print(rec(function() return "1e" + 0 end))
print(rec(function() return "x" + 1 end))
print(rec(function() return "1" < 2 end))
print(show(1 .. 2, 1.0 .. "", 1.5 .. "x", "a" .. 1 .. 2.0))
print(rec(function() return {} .. "" end), rec(function() return nil .. "" end))
print(show(#"", #"abc", #"\0\0", ("x"):rep(3)))
print(rec(function() return #1 end), rec(function() return #nil end))

-- 4. table keys: floats with an integral value share the integer's slot
local t = {}
t[1] = "int"; t[1.0] = "float"; t[2.0] = "two"
print(t[1], t[1.0], t[2], t[2.0], #t)
t["1"] = "str"
print(t["1"], t[1], rawlen(t))
t[3] = "three"; t[5] = "five"
print(#t, rawlen(t))
local k1, k2 = {}, {}
t[k1] = "t1"; t[k2] = "t2"
print(t[k1], t[k2], t[{}] == nil)
t[true] = "yes"; t[false] = "no"
print(t[true], t[false], t[print] == nil)
print(rec(function() local x = {}; return x[0/0] end))
print(rec(function() local x = {}; x[0/0] = 1 end))
print(rec(function() local x = {}; return x[nil] end))
print(rec(function() local x = {}; x[nil] = 1 end))

-- 5. constructors and multiple returns
local function three() return 1, 2, 3 end
print(show(#{three()}, #{three(), three()}, #{(three())}))
print(show(#{1, 2, three()}, #{three(), 4}))
print(show(({1, 2, 3})[2]))
print(rec(function() return #{1, nil, 3} end))
local a, b, c = three()
print(a, b, c)
local arr = {three()}
print(#arr, arr[1], arr[2], arr[3])
do local tt = {[1] = "x", [2] = "y"} print(show(tt[1], tt[2])) end

-- 6. closures and upvalues
local fns = {}
for i = 1, 3 do fns[i] = function() return i end end
print(fns[1](), fns[2](), fns[3]())
for _, v in ipairs{"x", "y"} do fns[#fns + 1] = function() return v end end
print(fns[4](), fns[5]())
local shared = 0
local function inc() shared = shared + 1; return shared end
print(inc(), inc(), shared)
do
  local n = 10
  local counter = {}
  for i = 1, 3 do counter[i] = function() n = n + i; return n end end
  print(counter[1](), counter[2](), counter[3]())
end

-- 7. varargs
local function vcount(...) return select('#', ...) end
print(vcount(), vcount(nil), vcount(nil, nil), vcount(1, nil, 3))
local function vpack(...) local t = {...} return #t end
print(vpack(), vpack(nil), vpack(1, nil, 3))
local function vfirst(...) return ... end
print(vfirst(1, 2, 3))
print((vfirst(1, 2, 3)))
print(select('#'), select('#', nil))

-- 8. goto
do
  local i, out = 0, {}
  ::top::
  i = i + 1
  out[#out + 1] = i
  if i < 3 then goto top end
  print(table.concat(out, ","))
end
do
  local out = {}
  for i = 1, 5 do
    if i % 2 == 0 then goto continue end
    out[#out + 1] = i
    ::continue::
  end
  print(table.concat(out, ","))
end
do
  local n = 0
  for i = 1, 3 do
    for j = 1, 3 do
      n = n + 1
      if j == 2 then goto next_i end
    end
    ::next_i::
  end
  print(n)
end

-- 9. the numeric 'for' at the edges of the integer range
do
  local out = {}
  for i = math.maxinteger - 2, math.maxinteger do out[#out + 1] = i end
  print(#out, out[1] == math.maxinteger - 2, out[3] == math.maxinteger)
end
do
  local n = 0
  for i = 1, 0 do n = n + 1 end
  for i = 1, 1, -1 do n = n + 1 end
  for i = 0, 1, -1 do n = n + 1 end
  print("empty loops", n)
end
do
  local out = {}
  for i = 1, 2.5, 0.5 do out[#out + 1] = i end
  print(#out, table.concat(out, ","))
end
do
  local out = {}
  for i = "1", "3" do out[#out + 1] = i end
  print(#out, table.concat(out, ","))
end
print(rec(function() for i = "x", 10 do end end))
print(rec(function() for i = 1, 10, 0 do end end))

-- 10. the generic 'for' with an explicit iterator
do
  local function iter(state, ctl)
    if ctl >= 3 then return nil end
    return ctl + 1, (ctl + 1) * 10
  end
  local out = {}
  for a, b in iter, nil, 0 do out[#out + 1] = tostring(a) .. ":" .. tostring(b) end
  print(table.concat(out, ","))
end
print(rec(function() for k in 42 do end end))
print(rec(function() for k in pairs(nil) do end end))

-- 11. _ENV
do
  local env = {print = print, tostring = tostring, x = 99}
  print(load("return x, print ~= nil", "=chunk", "t", env)())
end
do
  local env = setmetatable({}, {__index = _G})
  env.y = "shadow"
  print(load("return y, type", "=c", "t", env)())
end
print(rec(load, "return x", "=c", "t", 42))
print(rec(load, "return x", "=c", "b", _G))
print(_ENV == _G)

-- 12. metatables
do
  local t = setmetatable({}, {__metatable = "locked"})
  print(getmetatable(t))
  print(rec(setmetatable, t, {}))
  print(rec(setmetatable, t, nil))
end
print(rec(setmetatable, 42, {}), rec(setmetatable, {}, 42), rec(setmetatable, {}))
print(getmetatable("") == string, getmetatable(1) == nil)
print(rec(setmetatable, nil, {}))

-- 13. methods and strings as values
local obj = {v = 1, get = function(self) return self.v end}
print(obj:get(), obj.get(obj))
print(getmetatable(obj) == nil and setmetatable({}, {__index = obj}):get())
print(rec(function() local o = {}; return o:nope() end))
print(rec(function() local o = {}; o.x = 1; return o.x() end))
print(("abc"):upper(), ("abc"):sub(2, 3), ("%d"):format(7), ("a,b,c"):gsub(",", "|"))
