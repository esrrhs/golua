-- Metamethods: which one is looked up, on which operand, in what order,
-- and what happens when it is missing, not a function, or raises.  The
-- wording of the error when there is none is part of it too.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    elseif type(v) == "number" then t[i] = string.format("%.17g", v)
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...)
  local ok, e = pcall(f, ...)
  if ok then return "ok|" .. show(...) end
  return "err|" .. (tostring(e):gsub("^[^:]*:%d+: ", ""))
end
-- a fresh table with the given metamethods
local function T(mm) return setmetatable({}, mm) end

print("== arithmetic, one operand at a time ==")
for _, name in ipairs{"__add", "__sub", "__mul", "__div", "__idiv", "__mod",
                      "__pow", "__unm"} do
  local on1 = T({[name] = function(a, b) return "L" end})
  local on2 = T({[name] = function(a, b) return "R" end})
  local both = T({[name] = function(a, b) return "B" end})
  if name == "__unm" then
    print(name, rec(function() return -on1 end), rec(function() return -T({}) end))
  else
    print(name, rec(function() return on1 + 1 end),
                rec(function() return 1 + on2 end),
                rec(function() return on1 + T({}) end),
                rec(function() return T({}) + on2 end),
                rec(function() return both + on2 end))
  end
end

print("== bitwise ==")
for _, name in ipairs{"__band", "__bor", "__bxor", "__bnot", "__shl", "__shr"} do
  local on1 = T({[name] = function(a, b) return 1 end})
  local on2 = T({[name] = function(a, b) return 2 end})
  if name == "__bnot" then
    print(name, rec(function() return ~on1 end), rec(function() return ~T({}) end))
  else
    print(name, rec(function() return on1 & 1 end), rec(function() return 1 & on2 end),
          rec(function() return T({}) & 1 end))
  end
end

print("== concat ==")
local c = T({__concat = function(a, b) return "C" end})
print(rec(function() return c .. "x" end), rec(function() return "x" .. c end))
print(rec(function() return c .. c end))
print(rec(function() return c .. "x" .. "y" end))
print(rec(function() return "x" .. c .. "y" end))
print(rec(function() return c .. c .. c end))
print(rec(function() return T({}) .. "x" end))
print(rec(function() return "x" .. T({}) end))

print("== length and comparison ==")
local ln = T({__len = function() return 42 end})
print(rec(function() return #ln end), rec(function() return #T({}) end))
print(rec(function() return #setmetatable({}, {__len = function() return "s" end}) end))
print(rec(function() return #setmetatable({}, {__len = function() end}) end))
local eq = T({__eq = function() return true end})
print(rec(function() return eq == T({}) end), rec(function() return T({}) == eq end))
print(rec(function() return eq == eq end), rec(function() return T({}) == T({}) end))
local eqf = function() return false end
print(rec(function() return T({__eq = eqf}) == T({__eq = eqf}) end))
print(rec(function() return setmetatable({}, {__eq = function() return nil end}) ==
                         setmetatable({}, {__eq = function() return nil end}) end))
local lt = T({__lt = function() return true end})
print(rec(function() return lt < T({}) end), rec(function() return T({}) < lt end))
print(rec(function() return lt < lt end))
local le = T({__le = function() return true end})
print(rec(function() return le <= T({}) end), rec(function() return T({}) <= le end))
print(rec(function() return lt <= T({}) end))
print(rec(function() return le < T({}) end))
print(rec(function() return T({__le = function() return false end}) < T({}) end))
print(rec(function() return T({__lt = function() return nil end}) <= T({__lt = function() return nil end}) end))
print(rec(function() return eq < 1 end), rec(function() return 1 <= eq end))

print("== index and newindex ==")
local idx = T({__index = function(t, k) return "I:" .. tostring(k) end})
print(rec(function() return idx.x end), rec(function() return idx[1] end))
local idxt = T({__index = {x = "from table"}})
print(rec(function() return idxt.x end), rec(function() return idxt.y end))
local base = {b = "base"}
print(rec(function() return T({__index = base}).b end))
local chain = T({__index = T({__index = base})})
print(rec(function() return chain.b end))
print(rec(function() return T({__index = 5}).x end))
print(rec(function() return T({__index = "s"}).x end))
print(rec(function() return T({__index = function() return nil end}).x end))
print(rec(function() return T({__index = function() return 1, 2 end}).x end))
print(rec(function() return T({__index = function() error("in __index") end}).x end))
local ni = T({__newindex = function(t, k, v) rawset(t, k, "N" .. tostring(v)) end})
ni.x = 1
print(ni.x, rawget(ni, "x"))
local nitr = {}
T({__newindex = nitr}).y = 7
print(nitr.y)
print(rec(function() T({__newindex = 5}).z = 1 end))
print(rec(function() T({__newindex = function() error("in __newindex") end}).z = 1 end))
print(rec(function() T({}).z = 1 end))
local ro = setmetatable({}, {__index = {a = 1}, __newindex = function() error("ro") end})
print(rec(function() ro.a = 2 end), ro.a)

print("== call ==")
print(rec(function() return T({__call = function(self, a, b) return "C", a, b end})(1, 2) end))
print(rec(function() return T({__call = function() end})() end))
print(rec(function() return T({__call = 5})() end))
print(rec(function() return T({__call = T({__call = function() return "nested" end})})() end))
print(rec(function() return T({__call = function() error("in __call") end})() end))
local mt = {__call = function(self) return self.v end}
print(rec(function() local t = setmetatable({v = 9}, mt) return t() end))

print("== tostring, name, metatable, mode, pairs ==")
print(rec(tostring, T({__tostring = function() return "TS" end})))
print(rec(tostring, T({__tostring = function() return 42 end})))
print(rec(tostring, T({__tostring = function() error("bad") end})))
print(rec(tostring, T({__name = "MyType"})))
print(rec(tostring, T({__name = 5})))
print(rec(function() return T({__tostring = function() return {} end}) end))
local prot = setmetatable({}, {__metatable = "locked"})
print(rec(getmetatable, prot), rec(setmetatable, prot, {}))
print(rec(setmetatable, prot, nil))
print(rec(function() return prot.x end))
print(rec(setmetatable, {}, 5))
print(rec(setmetatable, {}, nil))
print(rec(getmetatable, 1), rec(getmetatable, nil))
print(rec(function() local t = {} return setmetatable(t, t) end))
print(rec(function() local a, b = {}, {} setmetatable(a, {__index = b}) setmetatable(b, {__index = a}) return a.x end))
local weak = setmetatable({}, {__mode = "k"})
print(rec(function() return weak[{}] end))
print(rec(function() weak[{}] = 1 return "set" end))
print(rec(function() return setmetatable({}, {__mode = "x"}) end))
print(rec(pairs, T({__pairs = function(t) return function() end, t, nil end})))
print(rec(function() local n = 0 for k in pairs(T({__pairs = function() return function() return nil end, nil, nil end})) do n = n + 1 end return n end))
print(rec(pairs, T({__pairs = 5})))

print("== raw ==")
local r = T({__index = function() return "I" end, __newindex = function() error("no") end,
             __len = function() return 99 end, __eq = function() return true end})
print(rec(rawget, r, "x"), rec(function() rawset(r, "x", 1) return rawget(r, "x") end))
print(rec(rawlen, r), rec(rawlen, "abc"), rec(rawlen, 5), rec(rawlen, {1, 2}))
print(rec(rawequal, r, {}), rec(rawequal, 1, 1), rec(rawequal, 1, "1"))
print(rec(rawlen), rec(rawequal, 1))

print("== a metamethod that is reached through __index ==")
local indir = setmetatable({}, {__index = {__add = function() return "via index" end}})
print(rec(function() return indir + 1 end))
print(rec(function() return indir .. "x" end))

print("== close ==")
print(rec(function() local x <close> = T({__close = function() end}) return "ok" end))
print(rec(function() local x <close> = T({__close = function() error("c1") end}) return "ok" end))
print(rec(function()
  local x <close> = T({__close = function() error("c1") end})
  local y <close> = T({__close = function() error("c2") end})
  return "ok"
end))
print(rec(function() local x <close> = setmetatable({}, {__close = 5}) return "ok" end))
print(rec(function() local x <close> = nil return "ok" end))
print(rec(function() local x <close> = 5 return "ok" end))
print(rec(function() local x <const> = 1 return x end))
print(rec(function()
  local t = {}
  for i = 1, 3 do t[#t + 1] = (function() local c <close> = T({__close = function(_, e) t[#t+1] = tostring(e) end}) return i end)() end
  return table.concat(t, ",")
end))
print(rec(function()
  local out = {}
  local ok, e = pcall(function()
    local a <close> = T({__close = function(_, err) out[#out+1] = "a:" .. tostring(err) end})
    error("body")
  end)
  return ok, e, table.concat(out, ",")
end))
print("done")
