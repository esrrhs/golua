-- error positions: the first instruction of a function, nested frames,
-- non-string error objects and <close> propagation.
local n
local function f()
	return n.y
end
print(pcall(f))
print(pcall(function() return (nil).x end))
local t = {}
print(pcall(function() return t.a.b end))
print(pcall(function() return 1 // 0 end))
print(pcall(function() return 1 % 0 end))
print(pcall(function() error("e") end))
print(pcall(function() error({}) end))
print(pcall(function()
	error(setmetatable({}, {__tostring = function() return "TOSTR" end}))
end))
print(pcall(function()
	local x <close> = setmetatable({}, {__close = function(_, e)
		print("closed", e)
	end})
	error("z")
end))
local co = coroutine.create(function() error("in-co") end)
print(coroutine.resume(co))
print(coroutine.close(co))
