-- debug.* with the exception of the hooks: what a frame says about
-- itself, locals, upvalues, the registry and the C stack limit.
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
local function keys(t)
  local o = {}
  for k in pairs(t) do o[#o + 1] = tostring(k) end
  table.sort(o)
  return table.concat(o, ",")
end
-- the fields of an info table, always in the same order
local function info(what, ...)
  local ok, t = pcall(debug.getinfo, ...)
  if not ok then return "E:" .. tostring(t) end
  if type(t) ~= "table" then return tostring(t) end
  local o = {}
  for i = 1, #what do
    o[i] = tostring(t[what:sub(i, i)]) .. "=" .. tostring(t[what:sub(i, i)])
  end
  return table.concat(o, " ")
end

print("== getinfo: who ==")
local function named(a, b) return a end
print(info("n", named))
print(info("S", named))
print(info("l", named))
print(info("u", named))
print(info("t", named))
print(info("f", named))
print(info("L", named))
print(info("nSltufL", named))
print(info("n", print))
print(info("S", print))
print(info("l", print))
print(info("u", print))
print(info("t", print))
print(info("f", print))
print(info("L", print))
print(info("nSltufL", print))
print(info("nS", function() end))
print(info("nS", load("return 1")))
print(info("nS", load("return 1", "=chunkname")))
print(info("nS", load("return 1", "@filename")))
print(info("S", 1))
print(info("S", 2))
print(info("S", 0))
print(info("S", 100))
print(info("S", -1))
print(rec(debug.getinfo, "x"))
print(rec(debug.getinfo, {}))
print(rec(debug.getinfo))
print(rec(debug.getinfo, 1, "z"))
print(rec(debug.getinfo, 1, 5))

print("== getinfo at a level ==")
local function lvl1() return info("nSl", 1), info("nSl", 2), info("nSl", 3) end
local function lvl2() return lvl1() end
print(lvl2())
local function tail1() return lvl1() end
print(rec(tail1))

print("== getinfo of a coroutine ==")
local co = coroutine.create(function() coroutine.yield(1) end)
coroutine.resume(co)
print(info("nSl", co, 0))
print(info("nSl", co, 1))
print(rec(debug.getinfo, co, 5))
local co2 = coroutine.create(function() coroutine.yield(1) end)
print(info("nSl", co2, 0))
print(rec(debug.getinfo, co2, 1))

print("== getlocal / setlocal ==")
local function loc(a, b)
  local c = 3
  return debug.getlocal(1, 1), debug.getlocal(1, 2), debug.getlocal(1, 3),
         debug.getlocal(1, 4), debug.getlocal(1, 0), debug.getlocal(1, -1)
end
print(loc(10, 20))
local function loc2()
  local x = 1
  debug.setlocal(1, 1, 99)
  return x
end
print(loc2())
print(rec(debug.getlocal, 1, 1))
print(rec(debug.getlocal, 100, 1))
print(rec(debug.getlocal, print, 1))
print(rec(debug.getlocal, 1, 0))
print(rec(debug.getlocal, 1, "x"))
print(rec(debug.setlocal, 100, 1, 1))
print(rec(debug.setlocal, 1, 0, 1))
print(rec(debug.setlocal))
print(rec(debug.getlocal))
print(rec(function() local v = 1 return debug.getlocal(2, 1) end))

print("== getupvalue / setupvalue ==")
local up = 5
local function useup() return up end
print(debug.getupvalue(useup, 1))
print(debug.getupvalue(useup, 2))
print(debug.getupvalue(useup, 0))
print(debug.setupvalue(useup, 1, 7), useup())
print(rec(debug.getupvalue, print, 1))
print(rec(debug.getupvalue, 5, 1))
print(rec(debug.getupvalue, useup, "x"))
print(rec(debug.setupvalue, print, 1, 1))
print(rec(debug.setupvalue, 5, 1, 1))
print(rec(debug.getupvalue))
print(rec(debug.setupvalue))

print("== getmetatable / setmetatable ==")
print(debug.getmetatable(1), debug.getmetatable("x"), debug.getmetatable(nil))
print(debug.getmetatable(print), debug.getmetatable({}))
local t = setmetatable({}, {__index = function() return 1 end})
print(type(debug.getmetatable(t)))
print(type(debug.setmetatable(t, {})))
print(type(debug.getmetatable(t)))
print(rec(debug.setmetatable, 1, {}))
print(rec(debug.setmetatable, t, 5))
print(rec(debug.getmetatable))
print(rec(debug.setmetatable))

print("== getuservalue / setuservalue ==")
local f = io.stdout
print(debug.getuservalue(f))
print(rec(debug.getuservalue, 1))
print(rec(debug.getuservalue, {}))
print(rec(debug.getuservalue, f, 2))
print(rec(debug.setuservalue, f, 1))
print(rec(debug.setuservalue, 1, 1))
print(rec(debug.getuservalue))
print(rec(debug.setuservalue))

print("== traceback ==")
local function deep() return debug.traceback() end
local function deeper() return deep() end
local tb = deeper()
print(type(tb), tb:sub(1, 12))
print(#tb > 0)
print(show((debug.traceback("msg")):sub(1, 4)))
print(show((debug.traceback("msg", 1)):sub(1, 3)))
print(show((debug.traceback("msg", 2)):sub(1, 3)))
print(type(debug.traceback(nil, 3)))
print(type(debug.traceback(co)))
print(type(debug.traceback(co, "m")))
print(rec(debug.traceback, co, "m", 1))
print(rec(debug.traceback, 5))
print(rec(debug.traceback, "m", "x"))
print(rec(debug.traceback, {}))
print(show(debug.traceback("m", 0)))
print(show(debug.traceback("m", 100)))

print("== getregistry ==")
local reg = debug.getregistry()
print(type(reg), type(reg[1]), type(reg[2]), type(reg[3]), type(reg[4]))
print(#reg >= 0)
print(rec(debug.getregistry, 1))

print("== setcstacklimit ==")
print(rec(debug.setcstacklimit, 400))
print(type(debug.setcstacklimit(200)))
print(rec(debug.setcstacklimit))
print(rec(debug.setcstacklimit, "x"))
print(rec(debug.setcstacklimit, 0))
print(rec(debug.setcstacklimit, 1))
print(rec(debug.setcstacklimit, 2 ^ 40))

print("== the whole library ==")
local o = {}
for k in pairs(debug) do o[#o + 1] = k end
table.sort(o)
print(table.concat(o, ","))
print("done")
