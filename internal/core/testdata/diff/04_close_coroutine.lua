local log = {}
local mt = {__close = function(self, err) log[#log + 1] = self.id .. ":" .. tostring(err) end}

local co = coroutine.create(function()
	local a <close> = setmetatable({id = "a"}, mt)
	local b <close> = setmetatable({id = "b"}, mt)
	coroutine.yield("paused")
end)
print(coroutine.resume(co))
print(coroutine.close(co), coroutine.status(co), table.concat(log, ","))

log = {}
local co2 = coroutine.create(function()
	local r <close> = setmetatable({id = "r"}, mt)
	error("in-co", 0)
end)
print(coroutine.resume(co2))
print(table.concat(log, ","))

log = {}
do
	local n <close> = nil
	local f <close> = false
end
print("nil/false closed:", #log)

log = {}
print(pcall(function()
	local a <close> = setmetatable({id = "a"}, mt)
	error({code = 7}, 0)
end))
print(table.concat(log, ","))

log = {}
print(pcall(function()
	local a <close> = setmetatable({id = "a"}, mt)
	local b <close> = setmetatable({id = "b"}, mt)
	error("boom", 0)
end))
print(table.concat(log, ","))
print(coroutine.close(coroutine.running()))
