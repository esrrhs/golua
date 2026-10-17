-- '__gc' is what PUC-Lua's collector does that Go's cannot, so this port
-- has to do it itself (see lgc.go).  The observable contract:
--
--   * an object that is still reachable when the state closes is
--     finalised all the same (C's 'separatetobefnz(g, 1)' takes every
--     object that has a '__gc', reachable or not);
--   * an object that is dropped reaches its '__gc' once allocating has
--     driven a collector step -- 'collectgarbage' is inert here, there
--     being no collector in this port to start or stop;
--   * an object made inside a coroutine is finalised too, even though
--     the coroutine's stack is still there.

local seen = 0
local mt = {}
function mt.__gc (o)
  seen = seen + 1
  print("gc", o[1])
end

-- PUC-Lua collects because it allocates; here the step an allocation
-- drives is what runs the metamethod (see gcStep).
local function cycle(n)
  for i = 1, 2000000 do
    local a = {}
    if seen >= n then break end
  end
end

-- dropped
do
  local o = setmetatable({"dropped"}, mt)
end
cycle(1)

-- made inside a coroutine: the dead coroutine's stack must not hold it
local co = coroutine.wrap(function ()
  setmetatable({"in a coroutine"}, mt)
end)
co()
cycle(2)

-- still reachable when the state closes: finalised all the same
_G.keep = setmetatable({"reachable"}, mt)

print("done")
