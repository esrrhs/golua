-- How deep a program can go before Lua stops it, what the object thrown
-- by 'error' can be, and how the two cross a coroutine boundary.
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
local function P(...) print(...) end

print("== recursion depth ==")
-- Count how many nested Lua calls fit, and what stops them.
do
  local depth = 0
  local function down()
    depth = depth + 1
    return down() + 1
  end
  local ok, e = pcall(down)
  P("plain", ok, e, depth)
end
-- pcall costs a C call each level, so the ceiling is lower.
do
  local depth = 0
  local function down()
    depth = depth + 1
    local ok, e = pcall(down)
    if not ok then error(e, 0) end
    return 1
  end
  local ok, e = pcall(down)
  P("pcall", ok, e, depth)
end
-- xpcall, and a message handler that is itself deep.
do
  local depth = 0
  local function down()
    depth = depth + 1
    return select(2, xpcall(down, function(m) return "h:" .. tostring(m) end)) or down()
  end
  local ok, e = pcall(function() return xpcall(down, function(m) return "h:" .. tostring(m) end) end)
  P("xpcall", ok, depth)
end
-- Recursion through a metamethod.
do
  local depth = 0
  local mt = {}
  mt.__index = function(t, k)
    depth = depth + 1
    return t.x
  end
  local t = setmetatable({}, mt)
  local ok, e = pcall(function() return t.y end)
  P("__index", ok, e, depth)
end
-- Recursion through tostring / __tostring.
do
  local depth = 0
  local t = setmetatable({}, {__tostring = function(s) depth = depth + 1; return tostring(s) end})
  P("__tostring", show(pcall(tostring, t)), depth)
  depth = 0
  local u = setmetatable({}, {__concat = function(a, b) depth = depth + 1; return a .. b end})
  P("__concat", show(pcall(function() return u .. "" end)), depth)
  depth = 0
  local v = setmetatable({}, {__eq = function(a, b) depth = depth + 1; return a == b end})
  P("__eq", show(pcall(function() return v == v end)), depth)
end
-- A for-in iterator that recurses.  Written so the call is not a tail
-- call: PUC-Lua turns a tail call into a jump, and 'return iter(s, c)'
-- would spin forever in both implementations.
do
  local depth = 0
  local function iter(s, c)
    depth = depth + 1
    local r = iter(s, c)
    return r
  end
  P("forin", show(pcall(function() for _ in iter, nil, nil do end end)), depth)
end
-- The tail-call form, bounded, to show the optimisation is there.
do
  local depth = 0
  local function iter(s, c)
    depth = depth + 1
    if depth > 100000 then return nil end
    return iter(s, c)
  end
  local n = 0
  for _ in iter, nil, nil do n = n + 1 end
  P("forin tail", n, depth)
end
-- Deeply nested tables (a constructor is compiled, not recursive).
do
  local src = "return " .. string.rep("{", 60) .. string.rep("}", 60)
  local f, e = load(src)
  P("nest60", f ~= nil, e)
  local ok, r = pcall(f)
  P("nest60 run", ok, type(r))
end
-- An unrolled recursion that stays inside the limit.
do
  local function fact(n) if n <= 1 then return 1 end; return n * fact(n - 1) end
  P("fact", fact(150))
end

print("== error: the object and the level ==")
P(rec(error))
P(rec(error, nil))
P(rec(error, nil, 0))
P(rec(error, nil, 1))
P(rec(error, false))
P(rec(error, false, 0))
P(rec(error, 42))
P(rec(error, 42, 0))
P(rec(error, {}))
P(rec(error, {}))
P(rec(error, setmetatable({}, {__tostring = function() return "OBJ" end})))
P(rec(error, "msg"))
P(rec(error, "msg", 0))
P(rec(error, "msg", 1))
P(rec(error, "msg", 2))
P(rec(error, "msg", 3))
P(rec(error, "msg", 100))
P(rec(error, "msg", -1))
P(rec(error, "msg", 1.0))
P(rec(error, "msg", 1.5))
P(rec(error, "msg", "1"))
-- 'error' inside a C function has no line to point at.
P(rec(function() return ("x"):rep(-1) end))
P(rec(function() return ({}).x.y end))
P(rec(function() return #nil end))
-- Level 1 points at the caller of the function that raised.
do
  local function raise() error("here", 2) end
  local function mid() raise() end
  P("lvl", show(pcall(mid)))
  local ok, e = pcall(mid)
  P("lvl2", ok, e)
end
-- Where the message comes from when it is raised in a coroutine.
do
  local co = coroutine.create(function() error("in coro") end)
  P("coro err", show(coroutine.resume(co)))
  P("status", coroutine.status(co))
  P("resume dead", show(coroutine.resume(co)))
  local w = coroutine.wrap(function() error("wrapped") end)
  P("wrap err", show(pcall(w)))
  P("wrap again", show(pcall(w)))
end
-- A non-string error object survives pcall untouched and is what
-- xpcall's handler receives.
do
  local obj = setmetatable({}, {__tostring = function() return "O" end})
  local ok, e = pcall(error, obj)
  P("identity", ok, e == obj)
  local got
  xpcall(function() error(obj) end, function(m) got = m end)
  P("handler", got == obj, tostring(got))
end

print("== coroutine boundaries ==")
do
  local co = coroutine.create(function(a, b)
    local c = coroutine.yield(a + b)
    return c * 2
  end)
  P("r1", show(coroutine.resume(co, 1, 2)))
  P("r2", show(coroutine.resume(co, 10)))
  P("status", coroutine.status(co))
  P("r3", show(coroutine.resume(co)))
end
-- Yielding across a C function is not allowed; across pcall it is.
P("across pcall", show(coroutine.resume(coroutine.create(function()
  return (coroutine.yield())
end))))
P("across sort", show(coroutine.resume(coroutine.create(function()
  table.sort({3, 1, 2}, function(a, b) coroutine.yield() return a < b end)
  return "done"
end))))
P("across gsub", show(coroutine.resume(coroutine.create(function()
  return (string.gsub("ab", ".", function(c) coroutine.yield() return c end))
end))))
P("across metamethod", show(coroutine.resume(coroutine.create(function()
  local t = setmetatable({}, {__index = function() return coroutine.yield() end})
  return t.x
end))))
P("across xpcall handler", show(coroutine.resume(coroutine.create(function()
  return xpcall(function() coroutine.yield() error("x") end, function(m) coroutine.yield() return m end)
end))))
-- isyieldable, running, close.
do
  local main = coroutine.running()
  P("main", main)
  P("main yieldable", coroutine.isyieldable())
  local co = coroutine.create(function()
    P("in coro yieldable", coroutine.isyieldable())
    local a, b = coroutine.running()
    P("running", a ~= nil, b)
    P("same", a == coroutine.running())
    return 1
  end)
  coroutine.resume(co)
end
do
  local co = coroutine.create(function()
    local ok, e = pcall(function() coroutine.yield() error("x") end)
    return ok, e
  end)
  coroutine.resume(co)
  P("close live", show(coroutine.close(co)))
  P("status", coroutine.status(co))
  local co2 = coroutine.create(function() return 1 end)
  P("close suspended", show(coroutine.close(co2)))
  local co3 = coroutine.create(function() return 1 end)
  coroutine.resume(co3)
  P("close dead", show(coroutine.close(co3)))
  P(rec(coroutine.close, 1))
  P(rec(coroutine.close, coroutine.running()))
end

print("== to-be-closed ==")
do
  local log = {}
  local function make(name)
    return setmetatable({}, {__close = function(self, err)
      log[#log + 1] = name .. (err and ("/" .. tostring(err)) or "")
    end})
  end
  do
    local c <close> = make("a")
    local d <close> = make("b")
    log[#log + 1] = "body"
  end
  P("normal", table.concat(log, ","))
  log = {}
  local ok, e = pcall(function()
    local c <close> = make("a")
    local d <close> = make("b")
    error("boom")
  end)
  P("error", ok, e, table.concat(log, ","))
  log = {}
  ok, e = pcall(function()
    local c <close> = make("a")
    local d <close> = setmetatable({}, {__close = function() error("in close") end})
    error("boom")
  end)
  P("close errs", ok, e, table.concat(log, ","))
  log = {}
  ok, e = pcall(function()
    local c <close> = 42
  end)
  P("not closable", ok, e)
  log = {}
  ok, e = pcall(function()
    local c <close> = nil
  end)
  P("nil close", ok, e)
  ok, e = pcall(function()
    local c <close> = setmetatable({}, {__close = 42})
  end)
  P("close not fn", ok, e)
  -- A to-be-closed variable in a coroutine that is closed mid-flight.
  log = {}
  local co = coroutine.create(function()
    local c <close> = make("coro")
    coroutine.yield()
    return 1
  end)
  coroutine.resume(co)
  P("coro close", show(coroutine.close(co)), table.concat(log, ","))
  -- <const> and re-assigning a to-be-closed variable are compile-time
  -- errors, so they have to reach the compiler as source text.
  local function comp(src)
    local f, e = load(src, "=c")
    if f == nil then return "refuse: " .. tostring(e) end
    local ok, r = pcall(f)
    return (ok and "ok" or "raise") .. ": " .. tostring(r)
  end
  P("const assign", comp("local c <const> = 1 c = 2 return c"))
  P("const ok", comp("local c <const> = 1 return c"))
  P("const read", comp("local c <const> = 1 return c + 1"))
  P("reassign close", comp("local c <close> = nil c = 1 return c"))
  P("close const", comp("local c <const> <close> = nil return 1"))
  P("dup attr", comp("local c <close> <close> = nil return 1"))
  P("attr unknown", comp("local c <nope> = 1 return c"))
  P("close global", comp("c <close> = nil return 1"))
  P("close in loop", comp("for i = 1, 2 do local c <close> = nil end return 1"))
  P("close break", comp("for i = 1, 2 do local c <close> = nil break end return 1"))
  P("close goto out", comp("do local c <close> = nil goto out end ::out:: return 1"))
  P("close return", comp("do local c <close> = nil return 1 end"))
end
