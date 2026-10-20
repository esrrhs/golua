-- os.tmpname builds its name from the "/tmp/lua_XXXXXX" template and
-- leaves the file on disk (mkstemp creates it, lua closes it at once).
-- The six random characters are matched away so that the case is
-- reproducible.
local n = os.tmpname()
print(type(n))
print(n:sub(1, 9)) -- the template, minus the six random characters
print(#n)
print(n:match("^/tmp/lua_%w%w%w%w%w%w$") ~= nil)

local f = io.open(n, "w") -- the file already exists: it can be written
print(f ~= nil)
f:write("x")
f:close()
print(io.open(n):read("a"))
print(os.remove(n))
local h, msg = io.open(n)
print(h, type(msg)) -- gone
