-- The driver for capi.c.  Run under PUC-Lua 5.4.9 to produce
-- expected.txt, and inside this port to produce the same lines.
-- Everything is printed with 'print', which the harness replaces by a
-- collector, so a line is just the tab-joined tostring of its results.

local capi = require "capi"

-- A. the stack
print("stack", capi.stack())
print("stackm", capi.stackm())

-- B. arithmetic and comparison
print("arith", capi.arith())
print("cmp", capi.cmp())
print("catlen", capi.catlen())

-- C. tables and globals
print("tab", capi.tab())
capi_global = "g"
print("glob", capi.glob())

-- D. conversions and types
print("conv", capi.conv())
print("types", capi.types())

-- E. references
print("refs", capi.refs())

-- F. lauxlib
print("auxlib", capi.auxlib())
print("checks", capi.checks(5, 2.5))
print("checks4", capi.checks(5, 2.5, "hey", 9))
print("argerr", pcall(capi.argerr))
print("typeerr", pcall(capi.typeerr, 1))
print("where", capi.where())

-- G. userdata with user values
print("muv", capi.muv(3, "u1", "u2", "u3"))
print("muv0", capi.muv(0))

-- H. to-be-closed from C
do
  local closed = false
  local obj = setmetatable({}, {__close = function() closed = true end})
  local v1, v2 = capi.toclose(obj)
  print("toclose", closed, v2, type(v1))
end
do
  local order = {}
  local obj = setmetatable({}, {__close = function() order[#order + 1] = "c" end})
  local a, b = capi.closeslot(obj)
  order[#order + 1] = "b"
  print("closeslot", tostring(a), b, table.concat(order, ","))
end

-- I. warnings (they go to stderr; only check that it returns)
print("warn", capi.warn())

-- J. hooks and the debug API
print("hook", capi.hook(function() return 1 end))

local function named_for_info()
  return capi.info()
end
print("info", named_for_info())

local function loc_test()
  local aa = 10
  local bb = 20
  return capi.locals()
end
print("locals", loc_test())

print("upvalues", capi.upvalues())
print("join", capi.join())

-- K. coroutines driven from C
function capi_cobody()
  coroutine.yield("first")
  return "second"
end
function capi_cobody2()
  return nil
end
print("coro", capi.coro())
print("xmove", capi.xmove())
print("reset", capi.reset())

-- L. buffers
print("buf", capi.buf())
print("bufsize", capi.bufsize())
print("addgsub", capi.addgsub())

-- M. a file handle through luaL_Stream
print("stream", capi.stream(io.stdout))

-- N. limits and the panic handler
print("limits", capi.limits())

-- O. luaL_requiref
print("pullstring", capi.pullstring())
