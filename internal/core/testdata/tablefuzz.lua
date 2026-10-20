-- A randomised exercise of a table's array and hash parts.
--
-- The point is to check the invariants that hold whatever order 'next'
-- visits keys in -- and that order is different in every run, here and
-- in PUC-Lua alike, because the hash of a string and of a pointer is
-- seeded per state.  So nothing here looks at *where* a key comes out,
-- only at whether the traversal as a whole is sound:
--
--   * every key that is there is visited, once, and no other;
--   * 'next(t, k) resumes from k, and only from a k that is still there;
--   * while a traversal deletes as it goes -- which is what Lua 5.4
--     promises -- 'next(t)' answers the key the traversal is on, because
--     everything before it has gone;
--   * a lookup answers what was last written, across resizes of both
--     parts.
--
-- 'seed' is a global: the Go test sets it before the chunk, and
-- 'lua -e "seed = 7" testdata/tablefuzz.lua' sets it from a shell.

local seed = seed or 1
local s = (seed * 2654435761) % 2147483647

local function rnd(n)
	s = (s * 1103515245 + 12345) % 2147483648
	return ((s >> 16) % n) + 1
end

-- Every key the run will use, and what t[pool[i]] has to answer.
local pool = {}
local exp = {}

local function add(k) pool[#pool + 1] = k end

for i = 1, 8 do add(i) end                                    -- small integers
for i = 1, 4 do add(1000 + i) end                             -- larger ones
for i = 1, 4 do add(i + 0.5) end                              -- floats
for i = 1, 8 do add(string.format("k%d", i)) end              -- short strings
for i = 1, 4 do add(string.rep(string.format("L%d-", i), 30)) end  -- long ones
for i = 1, 6 do add({i}) end                                  -- tables
for i = 1, 4 do add(function() return i end) end              -- closures
add(true)
add(false)

local n = #pool
local t = {}
local report = {}

local function fail(fmt, ...)
	report[#report + 1] = string.format(fmt, ...)
end

-- Which pool entry is this key?  Identity, so a float is not an integer
-- and one table is not another.
local function idof(k)
	for i = 1, n do
		if pool[i] == k then return i end
	end
	return nil
end

local function live()
	local c = 0
	for i = 1, n do
		if exp[i] ~= nil then c = c + 1 end
	end
	return c
end

-- Random writes and deletes.
local function phase(rounds)
	for _ = 1, rounds do
		local i = rnd(n)
		if rnd(4) == 1 then
			t[pool[i]] = nil
			exp[i] = nil
		else
			local v = rnd(1000)
			t[pool[i]] = v
			exp[i] = v
		end
	end
end

-- Grow and shrink both parts, to make the table rehash while keys of
-- every kind are in it.  None of these keys are in the pool.
local function churn()
	for i = 1, 300 do t[2000 + i] = i end
	for i = 1, 300 do t[2000 + i] = nil end
	for i = 1, 300 do t[string.format("tmp%d", i)] = i end
	for i = 1, 300 do t[string.format("tmp%d", i)] = nil end
end

local function checkLookups()
	for i = 1, n do
		local got, want = t[pool[i]], exp[i]
		if got ~= want then
			fail("lookup %d: got %s, want %s", i, tostring(got), tostring(want))
		end
	end
end

local function checkTraversal()
	local seen, cnt = {}, 0
	local k = next(t)
	-- 'while k' would stop on a key of false, which is a perfectly good key
	while k ~= nil do
		local i = idof(k)
		if i == nil then
			fail("next returned %s, which is not one of the keys", tostring(k))
			break
		end
		if seen[i] then fail("next visited key %d twice", i) end
		seen[i] = true
		cnt = cnt + 1
		k = next(t, k)
	end
	if cnt ~= live() then
		fail("next saw %d keys, %d are in the table", cnt, live())
	end
	for i = 1, n do
		if exp[i] ~= nil and not seen[i] then fail("next missed key %d", i) end
		if exp[i] == nil and seen[i] then fail("next returned key %d, which is gone", i) end
	end
end

-- The one nextvar.lua asserts: a traversal that deletes as it goes must
-- find the key it is on at the front of the table every time, because
-- every key before it has been deleted.
local function checkDeleteDuringTraversal()
	local want = live()
	local cnt = 0
	local k = next(t)
	while k ~= nil do
		if next(t) ~= k then
			fail("next(t) is not the key the traversal is on: %s", tostring(k))
		end
		t[k] = nil
		cnt = cnt + 1
		k = next(t, k)
	end
	if cnt ~= want then
		fail("the traversal deleted %d keys, %d were in the table", cnt, want)
	end
	if next(t) ~= nil then fail("the table is not empty once everything is gone") end
	for i = 1, n do exp[i] = nil end
end

-- A weak-keyed table: once a cycle has taken a key, its node is a dead
-- key, and a traversal still has to walk over it -- that is what
-- 'keyisdead' is for.
local function checkWeak()
	local wk = setmetatable({}, {__mode = "k"})
	local held = {}
	for i = 1, 20 do
		local k = {i}
		held[i] = k
		wk[k] = i
	end
	local cnt = 0
	for i = 1, 10 do held[i] = nil end -- half of them are now unreachable
	for _ = 1, 2000 do local _ = {} end -- allocating is what drives a cycle
	local seen = {}
	local k = next(wk)
	while k ~= nil do
		if type(k) ~= "table" then
			fail("a weak table returned a %s", type(k))
			break
		end
		if seen[k] then fail("a weak table visited one key twice") end
		seen[k] = true
		cnt = cnt + 1
		k = next(wk, k)
	end
	if cnt > 20 then fail("a weak table of 20 entries returned %d", cnt) end
end

-- 'next' with a key the table does not have is an error, not a nil.
local function checkInvalidKey()
	local ok = pcall(next, t, 99999)
	if ok then fail("next with a key that is not in the table did not raise") end
end

for round = 1, 8 do
	phase(150)
	checkLookups()
	checkTraversal()
	phase(50)
	churn()
	checkLookups()
	checkTraversal()
	checkDeleteDuringTraversal()
	checkInvalidKey()
	checkWeak()
end

if #report > 0 then
	print("FAIL: " .. table.concat(report, " | "))
end
return (#report == 0) and "OK" or table.concat(report, " | ")
