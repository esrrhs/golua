-- '__gc' is what PUC-Lua's collector does that Go's cannot, so this port
-- has to do it itself (see lgc.go).  The observable contract:
--
--   * an object that is still reachable when the state closes is
--     finalised all the same (C's 'separatetobefnz(g, 1)' takes every
--     object that has a '__gc', reachable or not);
--   * an object dropped and collected is finalised by 'collectgarbage';
--   * an object made inside a coroutine is finalised too, even though
--     the coroutine's stack is still there;
--   * 'collectgarbage' reports the mode it replaced, and 'stop' really
--     does stop it as far as 'isrunning' is concerned.

local mt = {}
function mt.__gc (o)
  print("gc", o[1])
end

-- dropped: 'collectgarbage' has to reach it
do
  local o = setmetatable({"dropped"}, mt)
end
collectgarbage()

-- made inside a coroutine: the dead coroutine's stack must not hold it
local co = coroutine.wrap(function ()
  setmetatable({"in a coroutine"}, mt)
end)
co()
collectgarbage()

-- still reachable when the state closes: finalised all the same
_G.keep = setmetatable({"reachable"}, mt)

-- the mode is remembered, and changing it answers with the previous one
print(collectgarbage("generational"))
print(collectgarbage("incremental"))

collectgarbage("stop")
print(collectgarbage("isrunning"))
collectgarbage("restart")
print(collectgarbage("isrunning"))

print("done")
