-- Runs driver.lua with the chunk name "cext", which is the name the Go
-- harness loads it under.  luaL_where and any traceback put that name
-- in front of a line number, so it has to be the same on both sides --
-- and it must not depend on where the file happens to live.
local name = ...
local f = assert(io.open(name, "r"))
local src = assert(f:read("a"))
f:close()
assert(load(src, "=cext"))()
