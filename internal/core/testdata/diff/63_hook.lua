-- debug.sethook: which events reach the hook, in what order, and what
-- the hook says about where it is.  A hook that counts instructions
-- also counts, indirectly, the instructions the compiler emits.
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

-- the event name as Lua spells it
local evname = {[0] = "call", [1] = "return", [2] = "line", [3] = "count",
                [4] = "tail call"}

print("== count hook ==")
local function count(body, n)
  local c = 0
  debug.sethook(function() c = c + 1 end, "", n)
  body()
  debug.sethook()
  return c
end
print(count(function() local x = 1 end, 1))
print(count(function() local x = 1 end, 2))
print(count(function() local x = 1 end, 10))
print(count(function() end, 1))
print(count(function() for i = 1, 10 do end end, 1))
print(count(function() for i = 1, 10 do end end, 5))
print(count(function() local s = 0 for i = 1, 100 do s = s + i end return s end, 25))
print(count(function() return math.floor(1.5) end, 1))
print(count(function() return ("x"):rep(3) end, 1))
print(count(function() local t = {} for i = 1, 10 do t[i] = i end return #t end, 7))
print(count(function() return pcall(function() error("x") end) end, 3))
print(count(function() local co = coroutine.create(function() coroutine.yield(1) end)
                       coroutine.resume(co) end, 4))
print(count(function() return select("#", 1, 2, 3) end, 2))
print(count(function() local a, b, c = 1, 2, 3 return a + b + c end, 2))
print(count(function() if true then return 1 else return 2 end end, 2))
print(count(function() local x = 0 while x < 10 do x = x + 1 end return x end, 3))
print(count(function() return 1 + 2 * 3 - 4 / 2 end, 1))
print(count(function() return string.format("%d", 1) end, 1))
print(count(function() local t = {1, 2, 3} return t[1] + t[2] + t[3] end, 2))

print("== call and return hooks ==")
local log
local function hook(ev, l)
  local i = debug.getinfo(2, "nSl")
  log[#log + 1] = string.format("%s@%s:%s:%s:%s", evname[ev], tostring(i.what),
                                tostring(i.name), tostring(i.namewhat),
                                tostring(i.currentline))
end
local function record(mask, body)
  log = {}
  debug.sethook(hook, mask)
  body()
  debug.sethook()
  local s = table.concat(log, " ")
  log = nil
  return s
end
local function leaf() return 1 end
local function middle() return leaf() end
local function top() return middle() end
print(record("c", function() leaf() end))
print(record("r", function() leaf() end))
print(record("cr", function() leaf() end))
print(record("cr", function() top() end))
print(record("c", function() top() end))
print(record("cr", function() return math.floor(1.5) end))
print(record("cr", function() return ("x"):rep(2) end))
print(record("cr", function() return pcall(leaf) end))
print(record("cr", function() local co = coroutine.wrap(leaf) return co() end))
print(record("cr", function() local t = setmetatable({}, {__index = function() return 1 end}) return t.x end))
print(record("cr", function() return (function() return 9 end)() end))

print("== tail calls ==")
local function tail() return leaf() end     -- 'return leaf()' is a tail call
print(record("cr", function() return tail() end))

print("== line hooks ==")
local function lines()
  local a = 1
  local b = 2
  return a + b
end
print(record("l", lines))
print(record("l", function() return 1 end))
print(record("l", function() for i = 1, 3 do end end))
print(record("l", function() if true then return 1 end return 2 end))
print(record("l", function() local t = {} t[1] = 1 return t[1] end))
print(record("cl", lines))
print(record("crl", lines))

print("== combined masks ==")
print(record("crl", function() return middle() end))
print(record("lc", function() return middle() end))
print(record("rc", function() return middle() end))
print(record("z", function() return middle() end))
print(record("crlz", function() return middle() end))

print("== count hooks mixed with the others ==")
local function mixed()
  debug.sethook(hook, "cr", 3)
  local s = 0
  for i = 1, 4 do s = s + i end
  debug.sethook()
  return s
end
log = {}
print(mixed())
print(table.concat(log, " "))

print("== who the hook is called with ==")
local seen
debug.sethook(function(...) seen = show(...) end, "l")
local q = 1
debug.sethook()
print(seen)
debug.sethook(function(...) seen = show(...) end, "c", 2)
local r = 1
debug.sethook()
print(seen)
debug.sethook(function(...) seen = show(...) end, "", 2)
local s2 = 1
debug.sethook()
print(seen)

print("== hooks and coroutines ==")
local co = coroutine.create(function()
  local n = 0
  debug.sethook(function() n = n + 1 end, "", 1)
  local x = 1
  debug.sethook()
  return n
end)
print(coroutine.resume(co))
local co2 = coroutine.create(function()
  debug.sethook(function() end, "cr")
  return debug.gethook()
end)
print(coroutine.resume(co2))

-- An error raised by a hook unwinds past the 'debug.sethook' that was
-- going to switch the hook off, so the hook is still installed when
-- 'pcall' answers and raises again while the answer is being formatted
-- -- from there it escapes to the interpreter, whose report carries
-- argv[0] and so cannot be compared.  The hooks below raise only once,
-- which leaves the event, the message and the hook still being
-- installed observable without leaving the pcall.
local function hookerr(mask, what, cnt)
  local n = 0
  debug.sethook(function() n = n + 1 if n == 1 then error(what) end end, mask, cnt)
  local ok, e = pcall(function() local x = 1 end)
  debug.sethook()
  return ok, e, n
end
print(rec(hookerr, "l", "from hook", 0))
print(rec(hookerr, "c", "from hook", 0))
print(rec(hookerr, "r", "from hook", 0))
print(rec(hookerr, "", "from hook", 1))
print(rec(hookerr, "l", {}, 0))
print(debug.gethook())

print("== sethook argument checking ==")
print(rec(debug.sethook))
print(rec(debug.sethook, nil, ""))
print(rec(debug.sethook, 5, "l"))
print(rec(debug.sethook, function() end, 5))
print(rec(debug.sethook, function() end, "l", "x"))
print(rec(debug.sethook, function() end, "l", -1))
print(rec(debug.sethook, function() end, "l", 0))
print(rec(debug.sethook, function() end, "l", 2 ^ 40))
print(rec(debug.sethook, function() end, "l", 1, "extra"))
print("done")
