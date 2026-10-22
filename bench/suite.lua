-- One workload per performance shape, so two interpreters can be compared
-- on the same code.
--
--     lua bench/suite.lua fib
--     lua bench/suite.lua fib 1000
--     lua bench/suite.lua list
--
-- The second argument is how many times to call the workload in this
-- process.  bench/compare.sh raises that count until the run is long
-- enough to time, then repeats the whole process.  Timing is
-- deliberately not done in here.  PUC-Lua's os.clock is CPU time and
-- this port's os.clock is wall time since startup, so a number taken
-- inside the script is not comparable.  bench/compare.sh times the
-- process from outside.
--
-- One call prints the workload's own result.  More than one call prints
-- a folded sum of every result, so a run that skipped iterations cannot
-- pass as one that did them.  Counts below are one call's worth of
-- work: big enough that a single PUC-Lua call is real work, small
-- enough that compare.sh can stack calls up to a second.

local N = {
	intloop = 30000000,
	floatloop = 20000000,
	bitwise = 15000000,
	fib = 30,
	tailcall = 8000000,
	call3 = 8000000,
	upvalue = 20000000,
	method = 8000000,
	pcall = 2000000,
	global = 8000000,
	array = 2000000,
	arraypass = 8,
	hash = 200000,
	hashpass = 10,
	metaindex = 8000000,
	ipairs = 400000,
	ipairspass = 20,
	concat = 3000000,
	strings = 200000,
	sort = 200000,
	yield = 1500000,
	alloc = 1500000,
}

local function fib(n)
	if n < 2 then
		return n
	end
	return fib(n - 1) + fib(n - 2)
end

local function tail(n, acc)
	if n == 0 then
		return acc
	end
	return tail(n - 1, acc + 1)
end

local function add3(a, b, c)
	return a + b + c
end

local workloads = {}

-- Interpreter dispatch and integer arithmetic.  Nothing allocates.
function workloads.intloop()
	local s = 0
	for i = 1, N.intloop do
		s = s + i
	end
	return s
end

-- Same loop in floats: dispatch plus the float path.
function workloads.floatloop()
	local s = 0.0
	for i = 1, N.floatloop do
		s = s + i * 1.5
	end
	return s
end

-- Bitwise opcodes, still allocation-free.
function workloads.bitwise()
	local s = 0
	for i = 1, N.bitwise do
		s = s + ((i & 255) ~ (i << 1))
	end
	return s
end

-- A call and two returns per step, and almost nothing else.
function workloads.fib()
	return fib(N.fib)
end

-- A tail call is a jump in PUC-Lua.  This is only that jump.
function workloads.tailcall()
	return tail(N.tailcall, 0)
end

-- A call with several arguments and one result, and no recursion.
function workloads.call3()
	local s = 0
	for i = 1, N.call3 do
		s = s + add3(i, 1, 2)
	end
	return s
end

-- Reading and writing one upvalue.
function workloads.upvalue()
	local n = 0
	local function inc()
		n = n + 1
	end
	for _ = 1, N.upvalue do
		inc()
	end
	return n
end

-- Method call: the ':' passes the receiver as well as the argument.
function workloads.method()
	local obj = { n = 1 }
	function obj:add(x)
		return self.n + x
	end
	local s = 0
	for i = 1, N.method do
		s = s + obj:add(1)
	end
	return s
end

-- Crossing pcall, which builds and tears down an error handler.
function workloads.pcall()
	local function id(x)
		return x
	end
	local s = 0
	for i = 1, N.pcall do
		local ok, v = pcall(id, i)
		if ok then
			s = s + v
		end
	end
	return s
end

-- A global is a hash lookup in _G on every iteration.
function workloads.global()
	_G.g = 0
	for i = 1, N.global do
		_G.g = _G.g + i
	end
	return _G.g
end

-- Integer keys that stay in the array part: write once, then read.
function workloads.array()
	local n = N.array
	local t = {}
	for i = 1, n do
		t[i] = i
	end
	local s = 0
	for _ = 1, N.arraypass do
		for i = 1, n do
			s = s + t[i]
		end
	end
	return s
end

-- String keys, which live in the hash part.  The keys are built once
-- so the loop times the lookup, not the concatenation.
function workloads.hash()
	local n = N.hash
	local t = {}
	local keys = {}
	for i = 1, n do
		keys[i] = "k" .. i
		t[keys[i]] = i
	end
	local s = 0
	for _ = 1, N.hashpass do
		for i = 1, n do
			s = s + t[keys[i]]
		end
	end
	return s
end

-- A field the table itself does not hold: every read runs __index.
function workloads.metaindex()
	local base = { x = 1 }
	local obj = setmetatable({}, { __index = base })
	local s = 0
	for _ = 1, N.metaindex do
		s = s + obj.x
	end
	return s
end

-- ipairs over a dense array.
function workloads.ipairs()
	local n = N.ipairs
	local t = {}
	for i = 1, n do
		t[i] = i
	end
	local s = 0
	for _ = 1, N.ipairspass do
		for _, v in ipairs(t) do
			s = s + v
		end
	end
	return s
end

-- Two short strings joined, thrown away.  Measures the concat opcode
-- rather than one ever-growing result.
function workloads.concat()
	local a, b = "hello", "world"
	local s
	for _ = 1, N.concat do
		s = a .. b
	end
	return #s
end

-- Format, concatenate and scan.  Short strings are interned, and the
-- scan walks the pattern matcher.
function workloads.strings()
	local n = N.strings
	local parts = {}
	for i = 1, n do
		parts[i] = string.format("%d-abcdefgh", i)
	end
	local s = table.concat(parts, ",")
	local w = 0
	for m in string.gmatch(s, "%a+") do
		w = w + #m
	end
	return w
end

-- table.sort with a fixed sequence, so both sides sort the same input.
-- Every comparison calls back into Lua.
function workloads.sort()
	math.randomseed(42)
	local n = N.sort
	local t = {}
	for i = 1, n do
		t[i] = math.random(1000000)
	end
	table.sort(t)
	return t[1] * 1000000 + t[n]
end

-- coroutine.wrap and yield, one value per round trip.
function workloads.yield()
	local n = N.yield
	local function producer()
		for i = 1, n do
			coroutine.yield(i)
		end
	end
	local co = coroutine.wrap(producer)
	local s = 0
	for _ = 1, n do
		s = s + co()
	end
	return s
end

-- Allocate a small table and drop it.  This is the collector's workload:
-- the loop body does almost nothing else.
function workloads.alloc()
	local s = 0
	for i = 1, N.alloc do
		local t = { i, i + 1, i + 2, i + 3 }
		s = s + t[1]
	end
	return s
end

-- An empty function.  Stacked, this is the cost of the call itself,
-- the floor under every other row.
function workloads.nop()
	return 0
end

local order = {
	"nop", "intloop", "floatloop", "bitwise",
	"fib", "tailcall", "call3", "upvalue", "method", "pcall",
	"global", "array", "hash", "metaindex", "ipairs",
	"concat", "strings", "sort",
	"yield", "alloc",
}

local name = arg[1]
if name == nil or name == "list" then
	for i = 1, #order do
		print(order[i])
	end
	return
end

local fn = workloads[name]
if fn == nil then
	io.stderr:write("unknown workload: ", name, "\n")
	os.exit(1)
end

local reps = 1
if arg[2] ~= nil then
	reps = tonumber(arg[2])
	if reps == nil or reps < 1 or reps % 1 ~= 0 or reps > 1000000000 then
		io.stderr:write("reps must be an integer from 1 to 1000000000\n")
		os.exit(1)
	end
end

if reps == 1 then
	print(name, fn())
	return
end

local h = 0
for _ = 1, reps do
	h = (h + fn()) % 1000000007
end
print(name, h)
