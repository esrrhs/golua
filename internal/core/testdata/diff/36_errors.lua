-- the messages the virtual machine builds: arithmetic on the wrong types,
-- indexing, calls, the integer division by zero, names taken from the
-- debug information, and what a metamethod that is not a function does.

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

local function add(a, b) return a + b end
local function sub(a, b) return a - b end
local function idiv(a, b) return a // b end
local function mod(a, b) return a % b end
local function band(a, b) return a & b end
local function shl(a, b) return a << b end
local function concat(a, b) return a .. b end
local function len(a) return #a end
local function lt(a, b) return a < b end
local function le(a, b) return a <= b end
local function index(a, b) return a[b] end
local function newindex(a, b, c) a[b] = c end
local function call(a, ...) return a(...) end
local function bnot(a) return ~a end
local function unm(a) return -a end

local ops = {
  {"add", add}, {"sub", sub}, {"idiv", idiv}, {"mod", mod},
  {"band", band}, {"shl", shl}, {"concat", concat}, {"lt", lt}, {"le", le},
}
local vals = {nil, true, "10", "x", {}, print}

-- 1. every operator against the wrong types
for _, o in ipairs(ops) do
  for _, v in ipairs(vals) do
    print(o[1], show(v), rec(o[2], v, 1), rec(o[2], 1, v), rec(o[2], v, v))
  end
end
for _, o in ipairs{{"unm", unm}, {"bnot", bnot}, {"len", len}} do
  for _, v in ipairs(vals) do print(o[1], show(v), rec(o[2], v)) end
end
for _, v in ipairs(vals) do
  print("index", show(v), rec(index, v, 1), rec(index, {}, v))
end
for _, v in ipairs{nil, true, "x", print, 0/0} do
  print("newindex", show(v), rec(newindex, v, 1, 1), rec(newindex, {}, v, 1))
end
for _, v in ipairs(vals) do print("call", show(v), rec(call, v)) end

-- 2. the integer edges
print(rec(idiv, 1, 0), rec(idiv, -1, 0), rec(mod, 1, 0), rec(mod, -1, 0))
print(rec(idiv, math.mininteger, -1), rec(mod, math.mininteger, -1))
print(show(1.0 // 0, -1.0 // 0, 1.0 % 0, -1.0 % 0))
print(show(7 // 2, -7 // 2, 7 % 2, -7 % 2, 7.0 // 2, 7 % -2))
print(rec(bnot, 2.5), rec(band, 2.5, 1))
print(rec(shl, 1, 64), rec(shl, 1, -1))
print(rec(function() return 1 >> 64 end), rec(function() return 1 >> -1 end))
print(show(1 << 63, 1 >> 63, -1 >> 63, ~0, ~(-1)))
print(rec(band, "10", 1), rec(band, "10.5", 1))
print(show("10" + 1, "10" * 2, "10" // 3, "10" % 3, "3.0" // 2, "0x10" + 0))
print(math.type("10" + 0), math.type("10" // 1), math.type(10 // 1))

-- 3. names: local, upvalue, global, field, method
local function f_local()
  local mylocal
  return mylocal.x
end
print(rec(f_local))
local up
local function f_up() return up.x end
print(rec(f_up))
print(rec(function() return undefinedglobal.x end))
local tt = {}
print(rec(function() return tt.k.x end))
print(rec(function() return tt[1].x end))
print(rec(function() return tt.k() end))
print(rec(function() return tt:method() end))
do
  local a = {}
  print(rec(function() return a + 1 end))
  print(rec(function() return a() end))
end

-- 4. the numeric 'for'
print(rec(function() for i = "x", 10 do end end))
print(rec(function() for i = 1, "x" do end end))
print(rec(function() for i = 1, 10, "x" do end end))
print(rec(function() for i = 1, 10, 0 do end end))
print(rec(function() for i = 1, 2, 0.0 do end end))
print(rec(function() for i = 1, 10, nil do end end))
do
  local n = 0
  for i = 1, 10, 3 do n = n + i end
  for i = 10, 1, -3 do n = n + i end
  print("forstep", n)
end

-- 5. metamethods: working, not a function, and raising
local function with(f) return setmetatable({}, f) end
for _, op in ipairs(ops) do
  local name = op[1]
  local ev = ({add = "__add", sub = "__sub", idiv = "__idiv", mod = "__mod",
               band = "__band", shl = "__shl", concat = "__concat",
               lt = "__lt", le = "__le"})[name]
  local mt = {}
  mt[ev] = function() return "ok:" .. name end
  print("mt", name, rec(op[2], with(mt), 1))
  local bad = {}
  bad[ev] = 42
  print("mtbad", name, rec(op[2], with(bad), 1))
  local err = {}
  err[ev] = function() error("in " .. name) end
  print("mterr", name, rec(op[2], with(err), 1))
end
print(rec(unm, setmetatable({}, {__unm = function() return "U" end})))
print(rec(len, setmetatable({}, {__len = function() return 7 end})))
print(rec(len, setmetatable({}, {__len = function() return "7" end})))
print(rec(len, setmetatable({}, {__len = 7})))
print(rec(index, setmetatable({}, {__index = function(t, k) return k .. "!" end}), "k"))
print(rec(index, setmetatable({}, {__index = 42}), "k"))
print(rec(newindex, setmetatable({}, {__newindex = 42}), 1, 1))
print(rec(call, setmetatable({}, {__call = function(self, ...) return "C", ... end})))
print(rec(call, setmetatable({}, {__call = 42})))
do
  local a = setmetatable({}, {__eq = function() return "E" end})
  print(a == a)
  local b = setmetatable({}, {__lt = function() return true end})
  print(rec(function(x) return x > 1 end, b))
end

-- 6. error objects
print(rec(function() error(setmetatable({}, {__tostring = function() return "OBJ" end})) end))
print(rec(function() error(setmetatable({}, {__tostring = function() return 42 end})) end))
print(rec(function() error(setmetatable({}, {__name = "MyType"})) end))
print(tostring(setmetatable({}, {__name = "MyType"})))
print(rec(function() error({}) end))

-- 7. to-be-closed
print(rec(function() local x <close> = 42 end))
print(rec(function() local x <close> = nil end))
print(rec(function() local x <close> = setmetatable({}, {__close = function() end}) end))
print(rec(function() local x <close> = setmetatable({}, {__close = function() error("closing") end}) end))
print(rec(function()
  local x <close> = setmetatable({}, {__close = function() end})
  error("body")
end))
print(rec(function()
  local a <close> = setmetatable({}, {__close = function() print("a") end})
  local b <close> = setmetatable({}, {__close = function() print("b") end})
  return "ok"
end))
print(rec(function() local x <close> = io.stdout end))
print(rec(function() for i = 1, 3 do local x <close> = 42 end end))

-- 8. chunks that do not compile
for _, s in ipairs{"goto nowhere", "::a:: ::a::", "do goto a end ::a::",
                   "for i=1,10 do goto cont end", "local x <nope> = 1",
                   "local x <const> = 1; x = 2", "break",
                   "return 1 return 2", "function 1() end", "f(...)",
                   "local x <close>", "x::<label>"} do
  print(show(pcall(load, s)))
end
print(show(load("return 1")), show(pcall(load, "return")))
