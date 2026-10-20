-- Coroutines and the debug library.
--
-- The coroutine part walks the state machine in every order: resume a
-- dead one, yield across a metamethod and across a C boundary, close one
-- that is suspended, and ask for the status after each.  The debug part
-- asks for the same information from outside and from inside.
--
-- What pins 'sethook' is that a mask with no recognised character in it
-- installs no hook at all: 'lua_sethook' turns the hook off when the
-- mask is empty (ldebug.c), so 'debug.sethook(f, "z")' stores f in the
-- hook table but 'debug.gethook()' answers nil, not f -- this port used
-- to answer f.  'debug.gethook' asks whether a hook is installed, not
-- whether a function was given.
--
-- Addresses are not asserted: threads, tables and functions are compared
-- for identity, and 'debug.traceback' has its addresses taken out before
-- it is printed.

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

print("== create and status ==")
local co = coroutine.create(function(a, b)
  local c = coroutine.yield(a + b)
  return c * 2
end)
print(coroutine.status(co), rec(coroutine.resume, co, 1, 2))
print(coroutine.status(co), rec(coroutine.resume, co, 10))
print(coroutine.status(co), rec(coroutine.resume, co, 10))
print(rec(coroutine.resume, 5))
print(rec(coroutine.create, 5))
print(rec(coroutine.create))

print("== a coroutine that fails ==")
local co2 = coroutine.create(function() error("inside") end)
print(rec(coroutine.resume, co2))
print(coroutine.status(co2))
print(rec(coroutine.resume, co2))

print("== running ==")
print(show(coroutine.running()))
local main, ismain = coroutine.running()
print(ismain)
print(show(coroutine.isyieldable()))
local co3 = coroutine.create(function()
  print(show(coroutine.running()))
  print(show(coroutine.isyieldable()))
  print(show(coroutine.status(coroutine.running())))
end)
print(rec(coroutine.resume, co3))

print("== wrap ==")
local w = coroutine.wrap(function(a) local b = coroutine.yield(a) return b end)
print(rec(w, 1), rec(w, 2), rec(w, 3))
print(rec(coroutine.wrap, 5), rec(coroutine.wrap))

print("== yield across a metamethod and a C boundary ==")
local mt = {__index = function(t, k) return coroutine.yield("mm") end}
local co4 = coroutine.create(function()
  local t = setmetatable({}, mt)
  return t.x
end)
print(rec(coroutine.resume, co4), rec(coroutine.resume, co4, "v"))
local co5 = coroutine.create(function()
  return string.gsub("abc", ".", function(c) return coroutine.yield(c) end)
end)
print(rec(coroutine.resume, co5))
print(rec(coroutine.resume, co5, "X"))
local co6 = coroutine.create(function()
  return (table.sort({3, 1, 2}, function(a, b) coroutine.yield() return a < b end))
end)
print(rec(coroutine.resume, co6))

print("== close ==")
local co7 = coroutine.create(function()
  local ok, e = pcall(function() coroutine.yield() end)
  return "closed with " .. tostring(e)
end)
print(rec(coroutine.resume, co7))
print(rec(coroutine.close, co7))
print(coroutine.status(co7))
print(rec(coroutine.close, co7))
print(rec(coroutine.close, 5))
print(rec(coroutine.close))
local co8 = coroutine.create(function() return 1 end)
print(rec(coroutine.close, co8), coroutine.status(co8))
local co9 = coroutine.create(function() error("x") end)
print(rec(coroutine.resume, co9), rec(coroutine.close, co9))

print("== to-be-closed in a coroutine ==")
local log = {}
local co10 = coroutine.create(function()
  local obj <close> = setmetatable({}, {__close = function() log[#log + 1] = "c" end})
  coroutine.yield("y")
  return "r"
end)
print(rec(coroutine.resume, co10))
print(rec(coroutine.close, co10))
print(show(table.concat(log, "")))

print("== debug.getinfo ==")
local function target(a) return a end
print(show(pcall(debug.getinfo, target)))
print(show(debug.getinfo(target).short_src, debug.getinfo(target).linedefined,
      debug.getinfo(target).currentline, debug.getinfo(target).what,
      debug.getinfo(target).source, debug.getinfo(target).nups,
      debug.getinfo(target).nparams, debug.getinfo(target).isvararg,
      debug.getinfo(target).func == target))
print(show(debug.getinfo(target, "n").name, debug.getinfo(target, "S").source,
      debug.getinfo(target, "l").currentline, debug.getinfo(target, "u").nups,
      debug.getinfo(target, "f").func == target, debug.getinfo(target, "t").istailcall))
print(show(debug.getinfo(1).currentline, debug.getinfo(1).what))
print(show(debug.getinfo(2)))
print(rec(debug.getinfo, 5))
print(rec(debug.getinfo))
print(rec(debug.getinfo, target, "z"))
print(show(debug.getinfo(print).what, debug.getinfo(print).short_src))
print(show(debug.getinfo(print, "n").name, debug.getinfo(print, "n").namewhat))

print("== debug.getlocal and setlocal ==")
local function lv(a, b) local c = 3 return debug.getlocal(1, 1) end
print(show(lv(10, 20)))
local function ls(a) local c = 1 debug.setlocal(1, 2, 99) return c end
print(show(ls(10)))
print(rec(debug.getlocal, 1, 0), rec(debug.getlocal, 1, 100))
print(rec(debug.setlocal, 1, 0, 1), rec(debug.setlocal, 1, 100, 1))
print(rec(debug.getlocal, 5, 1), rec(debug.setlocal, 5, 1, 1))

print("== upvalues ==")
local up = "x"
local function uses() return up end
print(show(debug.getupvalue(uses, 1)))
print(rec(debug.getupvalue, uses, 2), rec(debug.getupvalue, uses, 0))
print(show(debug.setupvalue(uses, 1, "y")), show(uses()))
print(rec(debug.setupvalue, uses, 2, "z"))
print(rec(debug.getupvalue, 5, 1), rec(debug.setupvalue, 5, 1, 1))
print(show(debug.upvalueid(uses, 1) == debug.upvalueid(uses, 1)))
print(rec(debug.upvalueid, 5, 1), rec(debug.upvalueid, uses, 9))
local other = function() return up end
print(rec(debug.upvaluejoin, other, 1, uses, 1))
print(rec(debug.upvaluejoin, 5, 1, uses, 1))

print("== traceback and sethook ==")
print((debug.traceback():gsub("0x%x+", "0xA")))
print((debug.traceback("msg"):gsub("0x%x+", "0xA")))
print((debug.traceback("msg", 2):gsub("0x%x+", "0xA")))
print(rec(debug.traceback, 5))
print(rec(debug.sethook))
print(rec(debug.sethook, function() end, "crl", 10))
print(rec(debug.sethook, function() end, "z"))
print(rec(debug.sethook, 5, "c"))
print(show(debug.gethook()))
print(rec(debug.getmetatable, 5), show(debug.getmetatable({})))
print(show(type(debug.getregistry())))
print(rec(debug.setmetatable, 5, {}))
print(rec(debug.getuservalue, 5), rec(debug.setuservalue, 5, 1))
print(type(debug.setcstacklimit(200)), type(debug.setcstacklimit(1000000)))
print(rec(debug.setcstacklimit, "x"))
print("done")
