-- The base library: the functions every program uses, asked over the
-- arguments they have to survive -- error objects that are not strings,
-- levels that point outside the stack, 'select' off both ends, numbers
-- in every base, and 'require' through 'package.preload' rather than
-- through the file system, so that nothing here depends on what is
-- installed.
--
-- One thing is deliberately not asserted: what 'collectgarbage("step")'
-- answers.  In PUC-Lua a single step almost never finishes a cycle --
-- 'lua_gc' answers true only when the collector has come back to
-- GCSpause -- so it answers false for hundreds of calls in a row, even
-- right after a full collection.  Here a step is a whole cycle, so it
-- answers true; answering false instead would leave
-- 'while not collectgarbage("step") do end' spinning forever, which is
-- worse than the difference.  The boolean is therefore turned into a
-- type.

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

print("== error ==")
print(rec(error, "x"))
print(rec(error, "x", 0))
print(rec(error, "x", 1))
print(rec(error, "x", 2))
print(rec(error, "x", 3))
print(rec(error, "x", -1))
print(rec(error))
print(rec(error, nil))
print(rec(error, 42))
print(rec(error, 42, 1))
print(rec(error, {}))
print(rec(error, setmetatable({}, {__tostring = function() return "!" end})))
print(rec(error, true, 0))
print(rec(error, "x", 1.0))
print(rec(error, "x", "1"))
print(rec(error, "x", 2 ^ 31))

print("== pcall and xpcall ==")
print(rec(pcall, error, "x"))
print(rec(pcall, error, nil))
print(rec(pcall, error, 42))
print(rec(pcall))
print(rec(pcall, 5))
print(rec(pcall, function() error("a") end))
print(rec(pcall, function() error({code = 1}) end))
print(show(pcall(function() return pcall(error, "y") end)))
print(rec(xpcall, function() error("a") end, function(e) return "H:" .. tostring(e) end))
print(rec(xpcall, function() error("a") end, function(e) return e end))
print(rec(xpcall, function() error(nil) end, function(e) return type(e) end))
print(rec(xpcall, function(a, b) return a + b end, print, 1, 2))
print(rec(xpcall, function() error("a") end, 5))
print(rec(xpcall, function() error("a") end))
print(rec(xpcall, error))
print(rec(xpcall))
print(rec(xpcall, function() error("a") end, function() error("in handler") end))
print(show(xpcall(function() error(setmetatable({}, {__tostring = function() return "T" end})) end,
       function(e) return tostring(e) end)))
print(show(select(2, xpcall(function() error("a") end, debug.traceback)):gsub("0x%x+", "0xA")))

print("== select ==")
print(rec(select, "#"))
print(rec(select, "#", 1, 2, 3))
print(rec(select, "#"))
print(rec(select, 1, "a", "b"))
print(rec(select, 2, "a", "b"))
print(rec(select, 3, "a", "b"))
print(rec(select, -1, "a", "b"))
print(rec(select, -2, "a", "b"))
print(rec(select, -3, "a", "b"))
print(rec(select, 0, "a"))
print(rec(select))
print(rec(select, "x", "a"))
print(rec(select, 1.0, "a"))
print(rec(select, 2 ^ 31, "a"))

print("== tonumber ==")
for _, v in ipairs{"10", "0x10", " 10 ", "10a", "", "1e2", ".5", "5.", "0x1p4",
                   "10", nil, 10, 10.0, true, {}, "0b101", "1e400", "-0"} do
  print(string.format("%q", tostring(v)), show(pcall(tonumber, v)))
end
for _, b in ipairs{2, 8, 10, 16, 36, 1, 37, 0, -1, 2.0, "10"} do
  print("base", tostring(b), show(pcall(tonumber, "z", b)),
        show(pcall(tonumber, "10", b)), show(pcall(tonumber, "", b)))
end
print(show(pcall(tonumber, "ff", 16)), show(pcall(tonumber, "0xff", 16)))
print(show(pcall(tonumber)), show(pcall(tonumber, nil, 10)))

print("== tostring and __tostring ==")
print(show(tostring(1), tostring(1.0), tostring("s"), tostring(nil),
      tostring(true), tostring(-0.0)))
local mt = {__tostring = function() return "OBJ" end}
print(show(tostring(setmetatable({}, mt))))
print(rec(tostring, setmetatable({}, {__tostring = function() return 42 end})))
print(rec(tostring, setmetatable({}, {__tostring = function() return nil end})))
print(rec(tostring, setmetatable({}, {__tostring = 5})))
print(rec(tostring))
print(show(tostring(1 / 0), tostring(-1 / 0), tostring(0 / 0)))

print("== rawlen rawget rawset rawequal ==")
print(show(rawlen({1, 2, 3})), show(rawlen("abc")), show(rawlen({})))
print(rec(rawlen, 5), rec(rawlen), rec(rawlen, nil))
print(show(rawget({a = 1}, "a")), show(rawget({a = 1}, "b")))
print(rec(rawget, 5, "a"), rec(rawget, {}, nil))
local t = {}
print(show(rawset(t, "k", "v")), show(t.k), rec(rawset, 5, "a", 1))
print(rec(rawset, t, nil, 1), rec(rawset, t, 0 / 0, 1))
print(show(rawequal(1, 1), rawequal(1, 1.0), rawequal("a", "a"),
      rawequal({}, {}), rawequal(nil, nil)))
print(rec(rawequal, 1), rec(rawequal))

print("== next and ipairs ==")
local n1 = {a = 1}
print(show(next(n1)), show(next(n1, "a")), rec(next, n1, "zz"))
print(rec(next, 5), rec(next), rec(next, {}, nil))
local it, st, ctl = ipairs({10, 20})
print(show(it(st, 0)), show(it(st, 1)), show(it(st, 2)), show(it(st, 3)))
print(rec(ipairs, 5), rec(ipairs), rec(ipairs, nil))
print(rec(ipairs, setmetatable({}, {__index = function(_, i) return i * 2 end})))

print("== metatables ==")
local m = {__metatable = "locked"}
local lt = setmetatable({}, m)
print(show(getmetatable(lt)), rec(setmetatable, lt, {}))
print(rec(setmetatable, 5, {}), rec(setmetatable, {}, 5), rec(setmetatable, {}))
print(show(getmetatable(5)), show(getmetatable("s")), show(getmetatable(nil)))
print(rec(getmetatable))
print(show(getmetatable(setmetatable({}, {__name = "N"}))))

print("== require through package.preload ==")
package.preload["_xy"] = function(name) return {name = name, loaded = true} end
print(show(require("_xy").name, require("_xy").loaded))
print(show(package.loaded["_xy"] ~= nil))
print(rec(require, "_no_such_module_xyz"))
print(rec(require))
print(rec(require, 5))
package.preload["_bad"] = function() error("boom") end
print(rec(require, "_bad"))
package.preload["_num"] = function() return 42 end
print(show(require("_num")))

print("== collectgarbage ==")
print(rec(collectgarbage))
print(rec(collectgarbage, "collect"))
print(type((collectgarbage("step"))))
print(rec(collectgarbage, "stop"))
print(rec(collectgarbage, "restart"))
print(rec(collectgarbage, "isrunning"))
print(rec(collectgarbage, "incremental"))
print(rec(collectgarbage, "generational"))
print(rec(collectgarbage, "bogus"))
print(type(collectgarbage("count")))

print("== warn ==")
print(rec(warn, "@on"), rec(warn, "@off"), rec(warn, "message"))
print(rec(warn), rec(warn, 5), rec(warn, "@bogus"))

print("== type and print ==")
print(type(nil), type(true), type(1), type(1.0), type("s"), type({}),
      type(print), type(coroutine.create(function() end)))
print(rec(type))
print(rec(print))
print("done")
