-- Every way a C extension can suspend a coroutine and come back.
-- The output is PUC-Lua 5.4.9's, kept in expected.txt.
local k = require "k"

-- 1. lua_yieldk with a continuation: the resume runs it
local co = coroutine.create(function() return k.yielder() end)
print("y1", coroutine.resume(co))
print("y2", coroutine.resume(co))
print("y3", coroutine.resume(co))

-- 2. lua_yieldk without one: what resume was given comes back
local co = coroutine.create(function() return k.plain() end)
print("p1", coroutine.resume(co))
print("p2", coroutine.resume(co, "x", "y"))
print("p3", coroutine.resume(co))

-- 3. lua_callk: the callee yields and the continuation takes over
local co = coroutine.create(function()
  k.caller(function() coroutine.yield(9) end)
  return "done"
end)
print("c1", coroutine.resume(co))
print("c2", coroutine.resume(co))
print("c3", coroutine.resume(co))

-- 4. lua_callk with no continuation is not yieldable
local co = coroutine.create(function()
  return k.nocall(function() coroutine.yield(1) end)
end)
print("n1", coroutine.resume(co))

-- 5. lua_pcallk whose callee yields
local co = coroutine.create(function()
  return k.pcally(function() coroutine.yield(11) end)
end)
print("a1", coroutine.resume(co))
print("a2", coroutine.resume(co))

-- 6. lua_pcallk whose callee raises: the continuation hears the status
local co = coroutine.create(function()
  return k.pcallerr(function() error("boom") end)
end)
print("e1", coroutine.resume(co))

-- 7. lua_pcallk with a message handler, called as pcallmsg(handler, f)
local co = coroutine.create(function()
  return k.pcallmsg(function(e) return "H:" .. tostring(e) end,
                    function() error("oops") end)
end)
print("m1", coroutine.resume(co))

-- 8. one C function that suspends three times in a row
local co = coroutine.create(function() return k.two() end)
print("t1", coroutine.resume(co))
print("t2", coroutine.resume(co))
print("t3", coroutine.resume(co))
print("t4", coroutine.resume(co))
print("t5", coroutine.resume(co))

-- 9. a yield of nothing, with no continuation
local co = coroutine.create(function() return k.nyield() end)
print("z1", coroutine.resume(co))
print("z2", coroutine.resume(co, 1, 2, 3))

-- 10. close a coroutine that is parked inside a C yield
local co = coroutine.create(function() return k.yielder() end)
print("s1", coroutine.resume(co))
print("s2", coroutine.close(co))
print("s3", coroutine.resume(co))
print("s4", coroutine.status(co))

-- 11. a continuation that counts the results with lua_gettop, the way
--     PUC-Lua's own dofilecont does
local co = coroutine.create(function()
  return k.pall(function()
    coroutine.yield("skip")
    return "a", "b", "c"
  end)
end)
print("w1", coroutine.resume(co))
print("w2", coroutine.resume(co))

-- 12. the same, for a lua_yieldk: the continuation sees the two values
--     that were yielded and the one the resume was given
local co = coroutine.create(function() return k.ytop() end)
print("v1", coroutine.resume(co))
print("v2", coroutine.resume(co, "A"))

-- 13. lua_isyieldable, from inside a coroutine and from outside one
local co = coroutine.create(function() return k.yable() end)
print("q1", coroutine.resume(co))
print("q2", k.yable())
