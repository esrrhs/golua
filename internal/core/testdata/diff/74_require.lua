-- 'require' and 'package.searchpath': the two ways a module name becomes
-- a file name, what is looked up in 'package.loaded' first, and what is
-- left behind when a module raises instead of returning.
--
-- The modules it looks for live in ./req and are part of the suite, so
-- nothing here has to be created at run time.
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

print("== package.searchpath ==")
P(rec(package.searchpath, "m1", "./req/?.lua"))
P(rec(package.searchpath, "m1", "./req/?.lua", "="))
P(rec(package.searchpath, "m1", "./req/?.lua", ".", "/"))
P(rec(package.searchpath, "sub.m2", "./req/?.lua"))
P(rec(package.searchpath, "sub.m2", "./req/?.lua", ".", "/"))
P(rec(package.searchpath, "sub.m2", "./req/?.lua", ".", "."))
P(rec(package.searchpath, "sub.deep.m3", "./req/?.lua", ".", "/"))
P(rec(package.searchpath, "sub/deep/m3", "./req/?.lua", ".", "/"))
P(rec(package.searchpath, "nope", "./req/?.lua"))
P(rec(package.searchpath, "", "./req/?.lua"))
P(rec(package.searchpath, ".", "./req/?.lua"))
P(rec(package.searchpath, "..", "./req/?.lua"))
P(rec(package.searchpath, "m1", ""))
P(rec(package.searchpath, "m1", ";"))
P(rec(package.searchpath, "m1", "./req/?.lua;./req/sub/?.lua"))
P(rec(package.searchpath, "m2", "./req/?.lua;./req/sub/?.lua"))
P(rec(package.searchpath, "m1", "./req/?"))
P(rec(package.searchpath, "noext", "./req/?"))
P(rec(package.searchpath, "m1", "./req/?.lua;./nope/?.lua"))
-- Not 'package.searchpath()' with nothing at all: which argument the
-- error names is then whichever 'luaL_checkstring' the C compiler
-- evaluated first, which is unspecified (see 40_modules.lua).
P(rec(package.searchpath, "m1"))
P(rec(package.searchpath, 1, "x"))
P(rec(package.searchpath, "m1", 1))
P(rec(package.searchpath, "m1", "x", 1))
P(rec(package.searchpath, "m1", "x", ".", 1))
P(rec(package.searchpath, "m1", "x", "."))

print("== require: through package.path ==")
do
  local old = package.path
  package.path = "./req/?.lua;./req/sub/?.lua"
  P(rec(require, "m1"))
  P(rec(require, "m2"))
  P(rec(require, "m2"))
  P(rec(require, "nope"))
  P(rec(require, "nope.deeper"))
  P(rec(require, ""))
  P(rec(require, "."))
  package.path = old
end
-- What require gives back, and what it leaves in package.loaded.
do
  local old = package.path
  package.path = "./req/?.lua"
  P("rv", show(pcall(require, "rv")))
  P("loaded", package.loaded["rv"])
  -- A module that returns false is still loaded, and require answers it.
  P("false1", require("rf"))
  P("false2", require("rf"))
  P("none", require("rn"))
  P("none2", package.loaded["rn"])
  -- A module that raises: the entry is dropped, so it runs again.
  P("raise1", show(pcall(require, "re")))
  P("after err", package.loaded["re"])
  P("raise2", show(pcall(require, "re")))
  -- A module that yields.
  P("yield", show(pcall(require, "ry")))
  package.path = old
end

print("== require: preload and loaded ==")
do
  package.preload["pp"] = function(name) return "from preload:" .. name end
  P("preload", require("pp"), package.loaded["pp"])
  package.preload["pp"] = function(name) return nil end
  package.loaded["pp"] = nil
  P("preload nil", require("pp"))
  P("loaded after", package.loaded["pp"])
  package.preload["pq"] = 42
  P("preload not fn", show(pcall(require, "pq")))
  package.preload["pq"] = nil
  -- package.loaded wins over everything.
  package.loaded["pl"] = "canned"
  P("loaded wins", require("pl"))
  package.loaded["pl"] = false
  P("loaded false", require("pl"))
  package.loaded["pl"] = nil
  -- A preloaded module that raises.
  package.preload["pr"] = function() error("bad") end
  P("preload raises", show(pcall(require, "pr")))
  P("after", package.loaded["pr"])
  package.preload["pr"] = nil
  P(rec(require))
  P(rec(require, 1))
  P(rec(require, {}))
  P(rec(require, true))
end
-- require inside a coroutine, and inside another require.
do
  local old = package.path
  package.path = "./req/?.lua"
  P("nested", require("nest1"))
  package.loaded["nest1"] = nil; package.loaded["nest2"] = nil
  local co = coroutine.create(function() return require("nest1") end)
  P("in coro", show(coroutine.resume(co)))
  package.loaded["nest1"] = nil; package.loaded["nest2"] = nil
  -- A cycle: the second require sees the placeholder that is still true.
  P("cycle", tostring(require("cyc")), tostring(package.loaded["cyc"]))
  package.loaded["cyc"] = nil
  -- A module that yields inside a coroutine body, outside of require.
  local co2 = coroutine.create(function() return require("ry") end)
  P("yield in coro", show(coroutine.resume(co2)))
  package.loaded["ry"] = nil
  package.path = old
end
P("cpath type", type(package.cpath))
-- The message 'package.loadlib' answers comes from dlerror when the host
-- can dlopen and from the port when it cannot (a build without cgo), so
-- only the shape is safe to assert.
do
  local ok, a, b, c = pcall(package.loadlib, "x", "y")
  P("loadlib", ok, a, type(b), type(c), b == nil or type(b) == "string")
end
P("searchers", #package.searchers, type(package.searchers[1]), type(package.searchers[4]))
P("config", (package.config:gsub("\n", "\\n")))
P("loaded keys", type(package.loaded), type(package.loaded["string"]), type(package.loaded["_G"]))
