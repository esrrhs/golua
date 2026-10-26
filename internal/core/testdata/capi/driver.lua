-- The driver for capi.c.  Run under PUC-Lua 5.3.4 to produce
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
