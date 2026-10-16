-- require and the package table, without touching the file system:
-- preload does everything a module file would.
--
-- 'searcher_Croot' still looks for the root of a C submodule even where
-- nothing can be loaded: the search is what contributes its "no file"
-- lines, so 'require("a.b.c")' lists "./a.so" as well as "./a/b/c.so".
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

print("== package ==")
print(type(package), type(package.loaded), type(package.preload))
print(type(package.path), type(package.cpath), type(package.config))
print(show(package.config))
print(type(package.searchers), #package.searchers)
for i = 1, #package.searchers do print(i, type(package.searchers[i])) end
print(package.loaded.string == string, package.loaded.math == math)
print(package.loaded._G == _G, package.loaded.package == package)
print(type(package.loadlib))

print("== searchpath ==")
print(rec(package.searchpath, "nonexistent_module_xyz", "./?.lua;./?/init.lua"))
print(rec(package.searchpath, "a.b", "./?.lua", ".", "/"))
print(rec(package.searchpath, "a", "./?.lua"))
print(rec(package.searchpath, "a", "./?.lua", "."))
print(rec(package.searchpath, "a", "./?.lua", ".", "."))
print(rec(package.searchpath))
print(rec(package.searchpath, 42, "./?.lua"))
print(rec(package.searchpath, "a", 42))
print(rec(package.searchpath, "a", "./?.lua", 42))
print(rec(package.searchpath, "a", "./?.lua", ".", 42))
print(rec(package.searchpath, "a", ""))
print(rec(package.searchpath, "", "./?.lua"))

print("== require with preload ==")
package.preload["m1"] = function(name) return {name = name, v = 1} end
local m1 = require("m1")
print(type(m1), m1.name, m1.v)
print(require("m1") == m1)
print(package.loaded.m1 == m1)

package.preload["retnil"] = function() return nil end
print(rec(require, "retnil"))
print(require("retnil"))

package.preload["retfalse"] = function() return false end
print(rec(require, "retfalse"))
print(package.loaded.retfalse)

package.preload["retnothing"] = function() end
print(rec(require, "retnothing"))

package.preload["retsentinel"] = function() return true end
print(rec(require, "retsentinel"), package.loaded.retsentinel)

package.preload["multi"] = function() return 1, 2, 3 end
print(rec(require, "multi"))

package.preload["raises"] = function() error("module boom") end
print(rec(require, "raises"))
print(rec(require, "raises"))
print(package.loaded.raises)

package.preload["raises0"] = function() error("plain", 0) end
print(rec(require, "raises0"))

package.preload["nonstr"] = function() return 42 end
print(rec(require, "nonstr"))

print("== package.loaded is consulted first ==")
package.loaded["pre"] = "already"
print(require("pre"))
package.loaded["pre"] = false
print(rec(require, "pre"))
package.loaded["pre"] = nil
print(rec(require, "pre"))

print("== circular ==")
package.preload["ca"] = function() return require("cb") end
package.preload["cb"] = function() return require("ca") end
print(rec(require, "ca"))

print("== bad names ==")
print(rec(require))
print(rec(require, 42))
print(rec(require, nil))
print(rec(require, ""))
print(rec(require, "no_such_module_xyz"))
print(rec(require, "a.b.c"))
print(rec(require, "string"))
print(rec(require, "_G"))

print("== searchers can be replaced ==")
do
  local old = package.searchers
  package.searchers = {function(name) return function() return "from searcher:" .. name end end}
  print(rec(require, "custom"))
  package.searchers = old
end
do
  local old = package.searchers
  package.searchers = {42}
  print(rec(require, "whatever"))
  package.searchers = old
end
do
  local old = package.searchers
  package.searchers = {}
  print(rec(require, "whatever"))
  package.searchers = old
end
do
  local old = package.searchers
  package.searchers = {function() return 42 end}
  print(rec(require, "whatever"))
  package.searchers = old
end
do
  local old = package.searchers
  package.searchers = {function() error("searcher says no") end}
  print(rec(require, "whatever"))
  package.searchers = old
end

print("== loadlib ==")
-- the message names the system's loader and its search paths, so only
-- the shape is comparable: fail, a message, and where it failed.
local function shape(f, ...)
  local r = table.pack(pcall(f, ...))
  local out = {tostring(r[1])}
  for i = 2, r.n do out[i] = type(r[i]) end
  return table.concat(out, "|")
end
print(shape(package.loadlib, "nosuchlib", "nosuchfunc"))
print(shape(package.loadlib, "a"))
print(shape(package.loadlib))
print(shape(package.loadlib, 42, "f"))
print(shape(package.loadlib, "a", 42))
print(shape(package.loadlib, "a", "f", 42))

print("== module state ==")
print(type(package.loaded.m1), package.loaded.m1.v)
package.loaded.m1 = nil
package.preload["m1"] = function() return {v = 2} end
print(require("m1").v)
print(require("m1").v)
local count = 0
for _ in pairs(package.loaded) do count = count + 1 end
print("loaded entries", count > 0)
print("done")
