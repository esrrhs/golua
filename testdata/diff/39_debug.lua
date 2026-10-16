-- the debug library in depth: getinfo's fields, locals and upvalues,
-- hooks on coroutines, traceback levels, and the registry.
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

local function fields(f, what)
  local ar = debug.getinfo(f, what)
  if type(f) == "number" then ar = debug.getinfo(f + 1, what) end
  if not ar then return "nil" end
  local out, keys = {}, {}
  for k in pairs(ar) do keys[#keys + 1] = k end
  table.sort(keys)
  for _, k in ipairs(keys) do
    local v = ar[k]
    out[#out + 1] = k .. "=" .. (type(v) == "string" and v or tostring(v))
  end
  return table.concat(out, " ")
end

print("== getinfo 'what' ==")
local function sample(a, b) local c = a + b return c end
for _, what in ipairs{"", "n", "S", "l", "u", "f", "L", "t",
                      "nSl", "nSlu", "nSluf", "nSlufL", "nSlufLt", "Sln", "x"} do
  local ok, r = pcall(fields, sample, what)
  print(show(what), ok and r or "ERR:" .. tostring(r))
end
print(fields(print, "nS"))
print(fields(0, "nSluf"))
print(fields(1, "nSl"))
print(fields(100, "nSl"))
print(fields(-1, "nSl"))
print(rec(debug.getinfo))
print(rec(debug.getinfo, 42))
print(rec(debug.getinfo, print, 42))
print(rec(debug.getinfo, print, "n", 42))

print("== getinfo of callers ==")
local function lvl(n) return debug.getinfo(n) end
do
  local function outer() return lvl(2) end
  local ar = outer()
  print(ar.what, ar.namewhat, ar.name, ar.nups, ar.nparams, ar.isvararg)
end
do
  local function outer2() return debug.getinfo(1, "n").name end
  print(outer2())
end
do
  local t = {}
  t.method = function(self) return debug.getinfo(1, "n") end
  print(t:method().name, t:method().namewhat)
end
print((function() return debug.getinfo(1, "n").name end)())

print("== locals ==")
do
  local function f(a, b)
    local c = a + b
    local d = "dee"
    local out = {}
    for i = 1, 10 do
      local n, v = debug.getlocal(1, i)
      if not n then break end
      out[#out + 1] = n .. "=" .. tostring(v)
    end
    return table.concat(out, ",")
  end
  print(f(1, 2))
end
do
  local function f()
    local x = 1
    local ok = debug.setlocal(1, 1, 99)
    return ok, x
  end
  print(f())
end
print(rec(debug.getlocal, 1, 0))
print(rec(debug.getlocal, 100, 1))
print(rec(debug.getlocal, 1, 1000))
print(rec(debug.getlocal, 0, 1))
print(rec(debug.getlocal))
print(rec(debug.setlocal, 1, 1, 1))
print(rec(debug.setlocal, 100, 1, 1))
print(rec(debug.setlocal, 1, 1000, 1))
do
  local n = 0
  local function g() n = n + 1 end
  g()
  print(debug.getinfo(g, "u").nups)
end

print("== upvalues ==")
do
  local a, b = 1, 2
  local function f() return a + b end
  local out = {}
  for i = 1, 5 do
    local n, v = debug.getupvalue(f, i)
    if not n then break end
    out[#out + 1] = n .. "=" .. tostring(v)
  end
  print(table.concat(out, ","))
  print(debug.setupvalue(f, 1, 10), debug.setupvalue(f, 2, 20), f())
  print(debug.getupvalue(f, 1))
  print(rec(debug.getupvalue, f, 100))
  print(rec(debug.setupvalue, f, 100, 1))
  print(rec(debug.getupvalue, print, 1))
  print(rec(debug.getupvalue, 42, 1))
  print(rec(debug.getupvalue))
  print(debug.getupvalue(f, 0) == nil)
end
print(rec(debug.upvalueid, print, 1))
print(rec(debug.upvalueid, 42, 1))
do
  local x = 1
  local f1 = function() return x end
  local f2 = function() return x end
  print(debug.upvalueid(f1, 1) == debug.upvalueid(f2, 1))
  print(debug.upvaluejoin(f1, 1, f2, 1))
  print(debug.upvalueid(f1, 1) == debug.upvalueid(f2, 1))
end

print("== metatables ==")
print(debug.getmetatable(42), debug.getmetatable(""), debug.getmetatable(nil))
print(debug.getmetatable(print), debug.getmetatable({}) == nil)
do
  local t = {}
  print(debug.setmetatable(t, {__index = function() return 7 end}) == t, t.x)
  print(debug.getmetatable(t) ~= nil)
  print(rec(debug.setmetatable, t))
  print(rec(debug.setmetatable, 42, {}))
  print(rec(debug.setmetatable, t, 42))
end

print("== uservalues and registry ==")
print(rec(debug.getuservalue, 42))
print(rec(debug.getuservalue))
print(type(debug.getregistry()))
do
  local r = debug.getregistry()
  print(r[1] ~= nil, r[2] ~= nil, type(r["_LOADED"]))
end

print("== traceback ==")
print(debug.traceback():match("^stack traceback:") ~= nil)
print(debug.traceback("msg"):match("^msg") ~= nil)
print(rec(debug.traceback, 42))
print(rec(debug.traceback, "a", "b"))
print(rec(debug.traceback, "a", 100))
print(rec(debug.traceback, "a", 0))
print(rec(debug.traceback, "a", -1))
print(show(pcall(debug.traceback)))
do
  local function f() return debug.traceback("in f", 1) end
  local s = f()
  print(s:match("in f") ~= nil, s:match("f31_debug2%.lua") ~= nil)
end
do
  local co = coroutine.create(function() coroutine.yield() end)
  coroutine.resume(co)
  local s = debug.traceback(co)
  print(type(s), s:match("stack traceback") ~= nil)
end
print(rec(debug.traceback, 42))
print(rec(debug.traceback, {}))

print("== sethook ==")
do
  local events = {}
  local function h(...) events[#events + 1] = select(1, ...) end
  debug.sethook(h, "cr", 0)
  local function f() return 1 end
  f()
  debug.sethook()
  print(table.concat(events, ","))
end
do
  local n = 0
  local function h() n = n + 1 end
  debug.sethook(h, "", 100)
  local s = 0
  for i = 1, 10000 do s = s + i end
  debug.sethook()
  print("count hook fired", n > 0, s)
end
do
  local lines = {}
  local function h(_, l) lines[#lines + 1] = l end
  debug.sethook(h, "l")
  local x = 1
  local y = 2
  local z = x + y
  debug.sethook()
  print(#lines > 0)
end
print(debug.gethook() == nil)
do
  debug.sethook(function() end, "l")
  local h, m, c = debug.gethook()
  print(type(h), m, c)
  debug.sethook()
  print(debug.gethook())
end
print(rec(debug.sethook))
print(rec(debug.sethook, 42))
print(rec(debug.sethook, print, "x"))
print(rec(debug.sethook, print, "l", "x"))
do
  local co = coroutine.create(function() for i = 1, 3 do coroutine.yield() end end)
  local n = 0
  debug.sethook(co, function() n = n + 1 end, "l")
  coroutine.resume(co)
  coroutine.resume(co)
  debug.sethook(co)
  print("coroutine hook", n > 0)
end

print("== misc ==")
print(rec(debug.setmetatable))
print(rec(debug.getuservalue, io.stdout))
print(type(debug.getinfo(1, "S").source))
print(debug.getinfo(1, "S").short_src)
print(debug.getinfo(print, "S").short_src)
print(debug.getinfo(print, "S").what)
print(debug.getinfo(print, "n").name)
print("done")
