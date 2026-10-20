-- to-be-closed variables: order, error propagation, interaction with
-- coroutines, returns and loops.
local log = {}

local function make(name, err)
	return setmetatable({}, {
		__close = function(_, e)
			log[#log + 1] = name .. ":" .. tostring(e)
			if err then error(err) end
		end,
		__tostring = function() return "<" .. name .. ">" end,
	})
end

-- normal scope exit: error object is nil
do
	local a <close> = make("a")
	local b <close> = make("b")
	log[#log + 1] = "body"
end
print(table.concat(log, ","))

-- error in the body: the error object is passed to __close
log = {}
print(pcall(function()
	local a <close> = make("a")
	local b <close> = make("b")
	error("bodyerr")
end))
print(table.concat(log, ","))

-- __close itself raises: the second error wins, but every handler runs
log = {}
print(pcall(function()
	local a <close> = make("a")
	local b <close> = make("b", "closeerr")
	error("bodyerr")
end))
print(table.concat(log, ","))

-- __close that yields / normal return value is ignored
log = {}
local function f()
	local a <close> = make("a")
	return 1, 2, 3
end
print(f())
print(table.concat(log, ","))

-- inside a loop: closed on every iteration
log = {}
for i = 1, 3 do
	local x <close> = make("i" .. i)
	if i == 2 then goto continue end
	::continue::
end
print(table.concat(log, ","))

-- break / return out of a loop
log = {}
local function g()
	for i = 1, 5 do
		local x <close> = make("j" .. i)
		if i == 2 then return "ret" end
	end
end
print(g())
print(table.concat(log, ","))

-- tbc in a coroutine that is closed while suspended
log = {}
local co = coroutine.create(function()
	local x <close> = make("co")
	coroutine.yield(1)
	log[#log + 1] = "after-yield"
	return 2
end)
print(coroutine.resume(co))
print(coroutine.close(co))
print(table.concat(log, ","))

-- tbc in a coroutine that ends with an error
log = {}
local co2 = coroutine.create(function()
	local x <close> = make("co2")
	coroutine.yield(1)
	error("coerr")
end)
print(coroutine.resume(co2))
print(coroutine.resume(co2))
print(table.concat(log, ","))

-- a non-callable __close is an error
print(pcall(function()
	local x <close> = setmetatable({}, {__close = 42})
end))

-- <const>
do
	local c <const> = 10
	print(c)
end
print(pcall(function()
	local c <const> = 1
	c = 2
end))
