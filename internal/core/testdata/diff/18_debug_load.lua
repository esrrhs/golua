-- debug library, load/loadstring, error levels and xpcall.
local function f(a, b)
	local c = a + b
	return c
end
print(pcall(debug.getinfo, f))
local i = debug.getinfo(f, "Slnu")
print(i.what, i.linedefined, i.lastlinedefined, i.nups, i.nparams, i.isvararg, i.namewhat, i.name)
print(i.source, i.short_src, i.currentline)

local inf = debug.getinfo(1, "SnflL")
print(inf.currentline > 0, type(inf.func), type(inf.activelines))

-- locals and upvalues
local up = "outer"
local function g()
	local inner = 1
	return up .. inner
end
print(g())
local n = 1
while true do
	local name, val = debug.getlocal(g, n)
	if not name then break end
	print("local", n, name, val)
	n = n + 1
end
local un, uv = debug.getupvalue(g, 1)
print("upvalue", un, uv)
print(debug.setupvalue(g, 1, "changed"))
print(g())
print(debug.getupvalue(g, 2))
print(pcall(debug.getlocal, g, 100))

-- traceback
print(debug.traceback("msg"):match("^msg"))
print(debug.traceback():match("stack traceback") ~= nil)
print((debug.traceback("x", 2)):match("^x") ~= nil)

-- sethook is a no-op here (no hooks in this port's tests), but must exist
print(type(debug.sethook), type(debug.gethook))
print(debug.getmetatable(1), debug.getmetatable("x"), debug.getmetatable({}))
print(debug.setmetatable({}, {__tostring = function() return "MT" end}))
print(tostring(debug.setmetatable({}, {__tostring = function() return "MT" end})))
print(type(debug.getregistry()))

-- load
local f2, err = load("return 1 + 1")
print(f2, err)
print(f2())
print(load(""))
local fe = load("return +")
print(fe)
print(load(function() return nil end)())
print(load((function()
	local s = "return 42"
	return function() local r = s s = nil return r end
end)())())
print(pcall(load, "return 1", "=myname", "t", {}))
print(pcall(load, "return 1", "=myname", "b", {}))
local bchunk = string.dump(function() return 7 end)
print(load(bchunk)())
print(pcall(load, bchunk, "=x", "t"))

-- error levels
local function lvl3() error("deep") end
local function lvl2() lvl3() end
print(pcall(lvl2))
print(pcall(function() lvl2() end))
print(pcall(function() error("with level", 2) end))
print(pcall(function() error("with level", 0) end))
print(pcall(function() error("with level", 3) end))
print(pcall(error))
print(pcall(function() error(nil) end))
print(pcall(function() error(false) end))
print(pcall(function() error({}) end))
print(pcall(function() error(setmetatable({}, {__tostring = function() return "E!" end})) end))

-- xpcall
print(xpcall(function() return 1, 2 end, print))
print(xpcall(function() error("boom") end, function(e) return "H:" .. e end))
print(xpcall(function(a, b) return a + b end, print, 1, 2))
print(xpcall(error, function(e) return "caught" end, "msg"))
print(pcall(xpcall))
print(pcall(xpcall, function() end))
print(xpcall(function() error("x") end, function(e) error("in handler") end))

-- select / type / assert
print(select("#"), select("#", 1, 2, 3))
print(select(1, "a", "b"), select(2, "a", "b"), select(-1, "a", "b"))
print(pcall(select, 0, "a"))
print(pcall(select, 4, "a"))
print(assert(1, "msg"), assert(true))
print(pcall(assert, false))
print(pcall(assert, false, "custom"))
print(pcall(assert, nil, 42))
