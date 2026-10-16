-- 'load' with a reader, a chunk name, an environment and a mode; the
-- debug library; the table library.  No reader answers a number: that
-- would never stop, because lua_isstring is true for numbers.
-- load (reader, chunkname, env, mode), the debug library, the table library
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

print("== chunk names ==")
print(rec(load, "return 1"))
print(rec(load, "return 1", "=name"))
print(rec(load, "return 1", "@file"))
print(rec(load, "return 1", "plain"))
print(rec(load, "return 1", ""))
print(rec(load, "return 1", 42))
print(rec(load, "return 1", nil, "t"))
print(rec(load, "return 1", nil, "b"))
print(rec(load, "return 1", nil, "bt"))
print(rec(load, "return 1", nil, "t", {}))
print(rec(load, "return 1", nil, "t", 42))
print(rec(load, "return 1", nil, "t", 42))
print(rec(load, 42))
print(rec(load))
do
  local f = load("error('x')", "=chunky")
  print(rec(f))
end
do
  local f = load("error('x')", "@filey")
  print(rec(f))
end
do
  local f = load("error('x')", "plainy")
  print(rec(f))
end
print(rec(load, "return 1", nil, "b"))

print("== environments ==")
do
  local env = {print = print, show = show}
  local f = load("return X", "=e", "t", env)
  env.X = 7
  print(show(f()))
  print(show(getmetatable(f)))
end
do
  local f = load("return _ENV", "=e", "t", {})
  print(show(f() ~= nil, type(f())))
end
do
  local f = load("x = 1 return x", "=e", "t", setmetatable({}, {__index = _G}))
  print(show(f()))
end

print("== readers ==")
do
  local i = 0
  local f = load(function()
    i = i + 1
    if i == 1 then return "return " end
    if i == 2 then return "1+" end
    if i == 3 then return "2" end
    return nil
  end)
  print(show(f()))
end
do
  -- a reader that always answers the same piece never stops: "return 1"
  -- is read again and again and the second copy is a syntax error
  local n = 0
  local f, err = load(function() n = n + 1; if n > 1 then return nil end; return "return 1" end)
  print(show(type(f), f and f() or err))
end
do
  local f = load(function() return "" end)
  print(show(f()))
end
-- a reader answering a number loops forever in PUC-Lua too: lua_isstring
-- is true for numbers, so 42 is read as "42" again and again
print(rec(load, function() error("in reader") end))
print(rec(load, function() return nil, "boom" end))
do
  -- two pieces are read before the reader stops, and "return 1return 2"
  -- is not a chunk: 'load' answers nil plus the syntax error
  local n = 0
  local f, err = load(function() n = n + 1; if n > 2 then return nil end; return "return " .. n end)
  print(show(type(f), f and f() or err))
end
do
  -- a reader that keeps returning pieces forever is kept in check by the
  -- number of iterations
  local n, f = 0, nil
  local ok, err = pcall(function()
    f = load(function() n = n + 1; if n > 1000 then return nil end; return " " end)
  end)
  print(ok, type(f), n > 1000)
end

print("== debug.getinfo ==")
local function target(a, b) return a + b end
print(show(debug.getinfo(target).source, debug.getinfo(target).short_src))
print(show(debug.getinfo(target).linedefined, debug.getinfo(target).lastlinedefined))
print(show(debug.getinfo(target).what, debug.getinfo(target).currentline))
print(show(debug.getinfo(print).what, debug.getinfo(print).source))
print(show(debug.getinfo(1).what, debug.getinfo(1).currentline ~= nil))
print(show(debug.getinfo(0)))
print(rec(debug.getinfo, 100))
print(rec(debug.getinfo, "x"))
print(rec(debug.getinfo))
do
  local co = coroutine.create(function() coroutine.yield() end)
  coroutine.resume(co)
  print(show(debug.getinfo(co, 0).what))
  print(rec(debug.getinfo, co, 1))
  print(rec(debug.getinfo, co, 0, "n"))
end

print("== debug.getlocal ==")
do
  local lv = {}
  local function f(a, b)
    local c = 3
    for i = 1, 10 do
      local n, v = debug.getlocal(1, i)
      if n then lv[#lv + 1] = n .. "=" .. tostring(v) else break end
    end
  end
  f(1, 2)
  print(table.concat(lv, " "))
end
print(rec(debug.getlocal, 100, 1))
print(rec(debug.getlocal, 1, 100))
print(rec(debug.getlocal, 1, 0))
print(rec(debug.getlocal))
do
  local function f(a) debug.setlocal(1, 1, 99); return a end
  print(show(f(1)))
  print(show(pcall(debug.setlocal, 1, 100, 1)))
  print(show(pcall(debug.setlocal, 1, 0, 1)))
end

print("== debug upvalues ==")
do
  local up = 1
  local function f() return up end
  print(show(debug.getupvalue(f, 1)))
  print(show(debug.getupvalue(f, 2)))
  print(show(debug.getupvalue(f, 0)))
  print(show(debug.setupvalue(f, 1, 42)))
  print(show(f()))
  print(show(debug.getupvalue(print, 1)))
  print(show(type(debug.upvalueid(f, 1))))
  print(rec(debug.upvalueid, f, 5))
  print(rec(debug.upvalueid, 42, 1))
  print(rec(debug.upvaluejoin))
end

print("== debug misc ==")
print(show(type(debug.getregistry())))
print(rec(debug.getmetatable))
print(rec(debug.getmetatable, 42))
print(show(debug.getmetatable("") ~= nil))
print(rec(debug.setmetatable, 42))
print(rec(debug.setmetatable, {}, 42))
print(rec(debug.getuservalue, 42))
print(rec(debug.setuservalue, 42))
print(show(type(debug.traceback())))
print(show(debug.traceback("msg"):sub(1, 3)))
print(rec(debug.traceback, 42))
print(rec(debug.sethook))
print(rec(debug.sethook, coroutine.create(function() end)))
print(rec(debug.sethook, function() end, "x"))
print(rec(debug.sethook, function() end, "cr", 42))
print(rec(debug.gethook))

print("== table library ==")
local t = {10, 20, 30}
print(show(table.concat(t)))
print(show(table.concat(t, ",")))
print(show(table.concat(t, ",", 2)))
print(show(table.concat(t, ",", 2, 3)))
print(show(table.concat({1, 2.5, "x"})))
print(rec(table.concat, {1, {}}))
print(rec(table.concat, t, 42))
print(rec(table.concat, t, ",", 4))
print(rec(table.concat, t, ",", 0))
print(rec(table.concat, {}))
print(rec(table.concat))
do
  local a = {1, 2, 3}
  print(show(table.unpack(a)))
  print(show(table.unpack(a, 2)))
  print(show(table.unpack(a, 2, 3)))
  print(show(table.unpack(a, 1, 0)))
  print(rec(table.unpack, a, 1, 4))
  print(rec(table.unpack, a, "1"))
  print(rec(table.unpack, 42))
  print(rec(table.unpack, a, 1, 1000000))
end
do
  local a = {3, 1, 2}
  table.sort(a)
  print(table.concat(a, ","))
  table.sort(a, function(x, y) return x > y end)
  print(table.concat(a, ","))
  print(rec(table.sort, {1, 2}, 42))
  print(rec(table.sort, {1, 2}, function() return "x" end))
  print(rec(table.sort, 42))
end
do
  local a = {1, 2, 3}
  print(show(table.insert(a, 4)))
  print(table.concat(a, ","))
  table.insert(a, 1, 0)
  print(table.concat(a, ","))
  print(rec(table.insert, a, 1, 2, 3))
  print(rec(table.insert, a, 100, 1))
  print(rec(table.insert, {}))
  print(rec(table.insert))
  print(rec(table.insert, 42, 1))
end
do
  local a = {1, 2, 3}
  print(show(table.remove(a)))
  print(table.concat(a, ","))
  print(show(table.remove(a, 1)))
  print(table.concat(a, ","))
  print(show(table.remove(a, 1)))
  print(show(table.remove(a, 1)))
  print(rec(table.remove, {1}, 5))
  print(rec(table.remove, {}))
  print(rec(table.remove))
end
do
  local a = {}
  for i = 1, 5 do a[i] = i end
  print(show(table.move(a, 2, 4, 1)))
  print(table.concat(a, ","))
  print(rec(table.move, {1, 2, 3}, 1, 3, 1, 42))
  print(rec(table.move, {1, 2, 3}, 1, 3, 1, {}))
  print(rec(table.move))
end
print("done")
