-- What happens to an error raised inside a debug hook.  ldblib.c calls
-- the hook with lua_call, not lua_pcall, so the error leaves the hook and
-- travels to the protected call underneath it; and because lua_call is
-- not yieldable, a 'coroutine.yield' inside a hook is an error too.
--
-- Every hook here is installed *inside* the function being watched and
-- removes itself the moment it fires: a line hook fires when execution
-- reaches the next line, which for a hook installed in the caller is the
-- line of the call itself -- outside the 'pcall'.

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

local function work()
  local x = 0
  for i = 1, 3 do x = x + i end
  return x
end

-- once installs 'f' as a line hook that fires only the first time
local function once(f)
  debug.sethook(function(what, line)
    debug.sethook()
    return f(what, line)
  end, "l")
  return work()
end

-- 1. an error raised by a line hook, caught by the caller
print(show(pcall(once, function() error("boom") end)))
debug.sethook()

-- 2. the same, with a non-string error object
local ok, err = pcall(once, function() error({code = 7}) end)
print(ok, type(err), err.code)
debug.sethook()

-- 3. a hook that yields: the main thread cannot
print(show(pcall(once, function() coroutine.yield("y") end)))
debug.sethook()

-- 4. a hook that yields inside a coroutine: also an error, because the
--    hook is called with lua_call
local co = coroutine.create(function() return once(function()
  coroutine.yield("y")
end) end)
print(show(coroutine.resume(co)))
print(coroutine.status(co))
debug.sethook()

-- 5. an error raised by a hook inside a coroutine
local co2 = coroutine.create(function() return once(function()
  error("inner")
end) end)
print(show(coroutine.resume(co2)))
print(coroutine.status(co2))
debug.sethook()

-- 6. a hook is not called from inside itself
local seen = {}
local function install6()
  debug.sethook(function(what)
    seen[#seen + 1] = what
    if #seen == 1 then
      debug.sethook(function() seen[#seen + 1] = "nested" end, "l")
    end
  end, "l")
  return work()
end
print(pcall(install6))
debug.sethook()
print(#seen > 0, (table.concat(seen, ","):find("nested", 1, true)) ~= nil)

-- 7. the error does not remove the hook: it still fires afterwards
local n = 0
local function install7()
  debug.sethook(function() n = n + 1 end, "", 10)
  return work()
end
print(show(pcall(install7)))
debug.sethook()
print(n > 0)

-- 8. debug.gethook still answers after a hook raised an error
print(show(pcall(once, function() error("x") end)))
local h, m, c = debug.gethook()
print(type(h), m, c)
debug.sethook()
print(debug.gethook())
print("done")
