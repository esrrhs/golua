-- string.dump and load: the binary chunk format has to survive a round
-- trip through our own writer and reader, and a chunk written by the
-- reference interpreter has to be loadable by ours.
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
local function rec(f, ...) return show(pcall(f, ...)) end
local function hex(s)
  local o = {}
  for i = 1, #s do o[i] = string.format("%02x", string.byte(s, i)) end
  return table.concat(o)
end

local f = function(a, b) return a + b end
print("== dump of a simple function ==")
local d = string.dump(f)
print(#d, type(d))
print(hex(d:sub(1, 32)))
print(d:sub(1, 4) == "\27Lua")
local g = load(d)
print(type(g), g ~= nil and g(1, 2) or nil)
print(g(3, 4), g(-1, 1), g(2.5, 0.5))

print("== strip ==")
local ds = string.dump(f, true)
print(#ds, #d, #ds <= #d)
print(load(ds)(1, 2))
print(hex(string.dump(f, false):sub(1, 32)) == hex(d:sub(1, 32)))
print(hex(string.dump(f, nil):sub(1, 32)) == hex(d:sub(1, 32)))

print("== what dump refuses ==")
print(rec(string.dump, print))
print(rec(string.dump, 5))
print(rec(string.dump))
print(rec(string.dump, f, 1))
print(rec(string.dump, f, "x"))
print(rec(string.dump, load("return 1")))
print(rec(string.dump, function() end))
print(rec(string.dump, function() return 1 end))

print("== kinds of function ==")
local up = 3
print(load(string.dump(function() return up end))())
print(load(string.dump(function(x) return x * 2 end))(21))
print(load(string.dump(function(...) return select("#", ...) end))(1, 2, 3))
print(load(string.dump(function() local a, b = 1, 2; return a + b end))())
print(load(string.dump(function() return "s", 1.5, true, nil end))())
print(load(string.dump(function() if true then return "t" else return "f" end end))())
print(load(string.dump(function() for i = 1, 3 do end return "loop" end))())
print(load(string.dump(function() local t = {}; t[1] = 1; return #t end))())
print(load(string.dump(function() return 2 ^ 10 end))())
print(load(string.dump(function() local s = "a" .. "b"; return s end))())
print(load(string.dump(function()
  local co = coroutine.wrap(function() coroutine.yield(7) end)
  return co()
end))())
print(load(string.dump(function() return ("x"):rep(3) end))())
print(load(string.dump(function() goto done ::done:: return "goto" end))())
print(load(string.dump(function() return math.maxinteger, math.mininteger end))())
print(rec(load(string.dump(function() error("in chunk") end))))

print("== load ==")
print(rec(load, d))
print(load("return 1 + 1")())
print(rec(load, "\27Lua"))
print(rec(load, "\27Lua garbage"))
print(rec(load, "return 1 +"))
print(rec(load, "@"))
print(rec(load, "=x"))
print(rec(load, "return", "=myname"))
print(select(2, pcall(load("return", "=myname"))))
print(rec(load, "return 1", nil, "t"))
print(type(load(d, nil, "b")), type(load(d, nil, "t")))
print(rec(load, d, nil, "t"))
print(rec(load, "return 1", nil, "b"))
print(rec(load, "return 1", nil, "z"))
print(rec(load, d, nil, "z"))
print(type(load("return 1", nil, "bt")), type(load(d, nil, "bt")))
print(rec(load, "return 1", nil, "x"))
print(rec(load, "return 1", "nm", 5))
print(rec(load, "return 1", 5, "t"))
print(type(load(d, "=bin")), type(load(d, "@bin")), type(load(d, "bin")))

print("== load with an environment ==")
local env = {print = print}
local h = load("return x", "=e", "t", env)
print(h())
env.x = 7
print(h())
local h2 = load("x = 1 return x", "=e2", "t", env)
print(h2(), env.x, x)
print(load("return _ENV", "=e3", "t", {a = 1})())
print(rec(load, "return 1", nil, "t", 5))
print(rec(load, "return 1", nil, "t", "x"))

print("== load of a dumped chunk keeps its upvalues ==")
local k = 99
local dumped = string.dump(function() return k end)
print(load(dumped)())

print("== a chunk written by the reference interpreter ==")
-- the bytes below are string.dump(function(x) return x * 3 end) from
-- PUC-Lua 5.4.9; they are inlined so that this file also pins reading
-- a chunk this port did not write.
local ref = "\27Lua\x54\x00\x19\x93\r\n\x1a\n\x04\x08\x04\x08\x08\x78\x56\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x28\x77\x40\x01\x01\x4c\x01\x02\xc7\x00\x00\x80\x1c\x40\x00\x01\x80\x00\x80\x00\xc0\x00\x01\xc1\x00\x00\x1c\x80\x40\x00\x1e\x00\x80\x00\x01\x00\x00\x00\x04\x06\x74\x65\x73\x74\x04\x01\x78\x00"
print(#ref, ref:sub(1, 4) == "\27Lua")
local rf = load(ref)
print(type(rf))
if type(rf) == "function" then print(rf(5), rf(-2)) end
print(rec(load, ref))
print(hex(ref) == hex(string.dump(function(x) return x * 3 end, false):sub(1, #ref)))
print("done")
