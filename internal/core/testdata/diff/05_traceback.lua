print(debug.traceback("msg"))
local function inner()
	return debug.traceback("in-inner")
end
local function outer()
	return inner()
end
print(outer())
print(pcall(function() return debug.traceback("via-pcall") end))
print(coroutine.resume(coroutine.create(function()
	print(debug.traceback("in-co"))
end)))
local function boom()
	error("kaboom")
end
local function caller()
	boom()
end
print(pcall(caller))
print(select(2, pcall(caller)))
local function deep(n)
	if n == 0 then return debug.traceback("deep") end
	return deep(n - 1)
end
print(deep(30))
