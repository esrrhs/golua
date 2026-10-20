-- where a coroutine may yield and where the C-call boundary stops it,
-- and how errors and 'close' travel through 'resume' and 'wrap'.

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

local function co(body, ...)
  local c = coroutine.create(body)
  return show(coroutine.resume(c, ...))
end

-- 1. the plain cases
print(co(function() coroutine.yield(1) return 2 end))
print(co(function() coroutine.yield() end))
print(co(function() coroutine.yield(1, 2, 3) end))
print(co(function() return 1, 2, 3 end))
print(co(function() error("boom") end))
print(co(function() error("boom", 0) end))
print(co(function() error({}) end))
print(co(function() error(setmetatable({}, {__tostring = function() return "T" end})) end))
print(co(function(a, b) coroutine.yield(a, b) return b, a end, "x", "y"))

-- 2. resuming past the end
do
  local c = coroutine.create(function() coroutine.yield(1) return 2 end)
  print(show(coroutine.resume(c)), show(coroutine.resume(c)), show(coroutine.resume(c)))
  print(coroutine.status(c))
end
do
  local c = coroutine.create(function() end)
  print(coroutine.status(c), show(coroutine.resume(c)), coroutine.status(c))
end

-- 3. running, status, yieldable
print(show(coroutine.running()))
print(co(function() return show(coroutine.running()) end))
print(co(function() print(coroutine.isyieldable()) coroutine.yield() end))
print(show(coroutine.isyieldable()))
do
  local c
  c = coroutine.create(function()
    print(coroutine.status(c))
    coroutine.yield()
    print(coroutine.status(c))
  end)
  print(show(coroutine.resume(c)), show(coroutine.resume(c)), coroutine.status(c))
end
print(show(pcall(coroutine.resume, coroutine.running())))
print(show(pcall(coroutine.close, coroutine.running())))

-- 4. what is yieldable and what is not
print(co(function() pcall(coroutine.yield, 1) return "ok" end))
print(co(function() return select('#', pcall(coroutine.yield, 1, 2)) end))
print(co(function() return xpcall(function() coroutine.yield(1) end, function(e) return "H" end) end))
print(co(function()
  local t = setmetatable({}, {__add = function() return coroutine.yield("add") end})
  return t + 1
end))
print(co(function()
  local t = setmetatable({}, {__index = function() return coroutine.yield("idx") end})
  return t.x
end))
print(co(function()
  local t = setmetatable({}, {__tostring = function() return coroutine.yield("ts") end})
  return tostring(t)
end))
print(co(function()
  local t = setmetatable({}, {__call = function() return coroutine.yield("call") end})
  return t()
end))
print(co(function()  -- not yieldable: the comparator is called from C
  return table.sort({3, 1, 2}, function(a, b) coroutine.yield("cmp") return a < b end)
end))
print(co(function()  -- not yieldable: gsub is a C function
  return ("abc"):gsub("%a", function(c) coroutine.yield("gsub") return c end)
end))
print(co(function()  -- not yieldable: the reader runs inside a protected call
  return load(function() coroutine.yield("reader") return nil end)
end))
print(co(function()
  local function iter(s, c)
    if c >= 2 then return nil end
    return c + 1, coroutine.yield("iter") or 0
  end
  for k, v in iter, nil, 0 do end
  return "done"
end))
print(co(function()
  for k, v in pairs({a = 1}) do coroutine.yield(k) end
  return "done"
end))
print(co(function()
  local x <close> = setmetatable({}, {__close = function() coroutine.yield("close") end})
  coroutine.yield("body")
  return "done"
end))

-- 5. wrap: results, errors, and calling a finished one
do
  local w = coroutine.wrap(function() coroutine.yield(1) return 2 end)
  print(w(), w(), show(pcall(w)))
end
do
  local w = coroutine.wrap(function() error("in wrap") end)
  print(show(pcall(w)), show(pcall(w)))
end
do
  local w = coroutine.wrap(function() error("in wrap", 0) end)
  print(show(pcall(w)))
end
do
  local w = coroutine.wrap(function() error({}) end)
  print(show(pcall(w)))
end
do
  local w = coroutine.wrap(function() coroutine.yield(coroutine.yield(1)) end)
  print(w(), w(2), w(3))
end

-- 6. nested coroutines
do
  local inner = coroutine.create(function()
    coroutine.yield("i1")
    coroutine.yield("i2")
    return "iend"
  end)
  local outer = coroutine.create(function()
    local a = coroutine.resume(inner)
    coroutine.yield("o1")
    local b = coroutine.resume(inner)
    coroutine.yield("o2")
    return a, b, select(2, coroutine.resume(inner))
  end)
  print(show(coroutine.resume(outer)))
  print(show(coroutine.resume(outer)))
  print(show(coroutine.resume(outer)))
end
do
  local c = coroutine.create(function()
    return coroutine.resume(coroutine.running())
  end)
  print(show(coroutine.resume(c)))
end

-- 7. close
do
  local closed = false
  local c = coroutine.create(function()
    local x <close> = setmetatable({}, {__close = function() closed = true end})
    coroutine.yield()
  end)
  print(show(coroutine.resume(c)))
  print(show(coroutine.close(c)), closed, coroutine.status(c))
end
do
  local c = coroutine.create(function() coroutine.yield() end)
  print(show(coroutine.resume(c)), show(coroutine.close(c)), show(coroutine.resume(c)))
end
do
  local c = coroutine.create(function() error("x") end)
  print(show(coroutine.resume(c)), show(coroutine.close(c)))
end
do
  local c = coroutine.create(function() end)
  print(show(coroutine.close(c)), show(coroutine.close(c)))
end
do
  local c = coroutine.create(function()
    local x <close> = setmetatable({}, {__close = function() error("in close") end})
    coroutine.yield()
  end)
  print(show(coroutine.resume(c)), show(coroutine.close(c)), coroutine.status(c))
end
print(show(pcall(coroutine.close, 42)), show(pcall(coroutine.close)))
print(show(pcall(coroutine.resume, 42)), show(pcall(coroutine.resume)))
print(show(pcall(coroutine.status, 42)), show(pcall(coroutine.create, 42)))
print(show(pcall(coroutine.wrap, 42)))
