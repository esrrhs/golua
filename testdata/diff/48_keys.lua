-- __name in error messages, protected metatables, table keys and the
-- raw functions.  Keys are what decide whether 1 and 1.0 share a slot.
-- __name in error messages, protected metatables, table keys, raw*
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

print("== __name ==")
do
  local t = setmetatable({}, {__name = "Point"})
  print(rec(function() return t + 1 end))
  print(rec(function() return -t end))
  print(rec(function() return #t end))
  print(rec(function() return t.x.y end))
  print(rec(function() return t() end))
  print(rec(function() return t .. "" end))
  print(rec(function() return t < t end))
  print(rec(function() return t == 1 end))
  print(rec(function() return t:foo() end))
  print(show(tostring(t):sub(1, 5)))
  print(rec(function() return t .. "" end))
  local u = setmetatable({}, {__name = "U", __tostring = function() return "S" end})
  print(show(tostring(u)))
  print(show(tostring(setmetatable({}, {__name = 42})):sub(1, 4)))
end

print("== protected metatables ==")
do
  local t = setmetatable({}, {__metatable = "locked"})
  print(show(getmetatable(t)))
  print(rec(setmetatable, t, {}))
  print(show(getmetatable(t)))
  print(rec(setmetatable, t, nil))
  print(show(getmetatable(setmetatable({}, {__metatable = false}))))
  print(show(getmetatable(setmetatable({}, {__metatable = nil})) ~= nil))
end

print("== keys ==")
print(rec(function() local t = {}; t[nil] = 1 end))
print(rec(function() local t = {}; t[0/0] = 1 end))
print(rec(function() local t = {}; return t[nil] end))
print(rec(function() local t = {}; return t[0/0] end))
print(rec(function() local t = {}; return t[nil] end))
do
  local t = {}
  t[1] = "int"
  t[1.0] = "float"
  print(show(t[1], #t))
  for k, v in pairs(t) do print(show(k, v, math.type(k))) end
end
do
  local t = {}
  t[2.0] = "two"
  print(show(t[2], math.type(next(t))))
  t[3.5] = "three"
  print(show(t[3.5], t[3]))
  t[-0.0] = "negzero"
  print(show(t[0], t[-0.0]))
end
do
  local t = {}
  t[2 ^ 63] = "big"
  print(show(t[2 ^ 63]))
  print(rec(function() local u = {}; u[2 ^ 70] = 1 end))
  print(rec(function() local u = {}; u[-2 ^ 70] = 1 end))
end
do
  local t = {[1] = "a", [2] = "b"}
  print(show(next(t) ~= nil))
  print(rec(next, 42))
  print(rec(next, {}))
end
do
  -- string keys that look like numbers
  local t = {}
  t["1"] = "str"
  print(show(t["1"], t[1]))
end
do
  local t = {}
  t[true] = 1
  t[false] = 2
  print(show(t[true], t[false]))
  print(rec(function() local u = {}; u[print] = 1; return u[print] end))
end

print("== raw ==")
print(rec(rawget, 42, 1))
print(rec(rawget))
print(rec(rawget, {}))
print(rec(rawset, 42, 1, 1))
print(rec(rawset, {}))
print(rec(rawlen, 42))
print(rec(rawlen))
print(rec(rawequal))
print(show(rawequal(1, 1.0), rawequal({}, {})))
print(show(rawlen("abc"), rawlen({1, 2})))
do
  local t = setmetatable({}, {__index = function() return "mt" end, __len = function() return 99 end})
  print(show(rawget(t, "x"), t.x, rawlen(t), #t))
  rawset(t, "y", 1)
  print(show(rawget(t, "y"), t.y))
end

print("== error objects ==")
print(rec(error, nil))
print(rec(error, nil, 0))
print(rec(error, false))
print(rec(error))
print(rec(error, {}))
print(rec(error, setmetatable({}, {__tostring = function() return "E" end})))
print(rec(error, "m", 1))
print(rec(error, "m", 2))
print(rec(error, "m", 3))
print(rec(error, "m", 0))
print(rec(error, "m", "1"))
print(rec(function() error("x") end))
print(rec(assert, false))
print(rec(assert, false, "msg"))
print(rec(assert, nil, {}))
print(rec(assert, false, nil))
print(show(assert(1, 2)))
print(rec(assert))
print(show(pcall(error, "e")))
print(show(xpcall(error, function(e) return "H:" .. tostring(e) end, "e")))
print(rec(xpcall, error))
print(rec(xpcall, error, 42))
print(rec(xpcall, 42, print))
print("done")
