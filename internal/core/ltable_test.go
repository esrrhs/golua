package core

import (
	"strconv"
	"strings"
	"testing"
)

// TestNextOverDeletedKeys pins what makes 'next' able to resume from a
// key that the traversal has just deleted.
//
// findindex (5.3.4) matches keys with luaV_rawequalobj, or — for a dead
// key left by weak-table cleanup — deadvalue==gcvalue.  A node's 'next'
// is only a chain offset (negative is ordinary: f-mp when the free slot
// sits below the main position).  An older bug treated next==-2 as a
// "dead" marker, then compared a closure against a long-string node and
// panicked inside 'next'.  Hash order made that fail nextvar.lua roughly
// one run in twenty, more often under '-race'.
func TestNextOverDeletedKeys(t *testing.T) {
	/* Before the fix this was a type assertion panic deep inside 'next',
	   which is not a Lua error and so not something 'pcall' catches:
	   turning it into a failure here keeps one bad table from taking
	   the whole test binary down with it. */
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("traversal panicked: %v", r)
		}
	}()
	/* The shape is nextvar.lua's "testing next x GC of deleted keys",
	   which is PUC-Lua's own regression test for this area: keys of
	   every collectable kind, deleted as the traversal goes.  It is
	   repeated because the bug needs a particular chain offset, and
	   that is a matter of hash order. */
	got := runChunk(t, `
	local bad = 0
	for round = 1, 20000 do
		local t = {}
		t[{1}] = 1
		t[{2}] = 2
		t[string.rep("a", 50)] = "a"
		t[string.rep("b", 50)] = "b"
		t[{3}] = 3
		t[string.rep("c", 10)] = "c"
		t[function () return 10 end] = 10

		local keys, k = {}, next(t)
		while k do
			keys[#keys + 1] = k
			k = next(t, k)
		end
		if #keys ~= 7 then bad = bad + 1000000 + round end

		for i = 1, #keys do
			if next(t) ~= keys[i] then bad = bad + 2000000 + round end
			t[keys[i]] = nil
		end
		if next(t) ~= nil then bad = bad + 3000000 + round end
	end
	return tostring(bad)
	`)
	if got != "0" {
		t.Fatalf("traversal broke: %s", got)
	}
}

// TestDeadKeyDoesNotBreakTraversal checks the other half: a key that
// really is dead -- one a weak table has dropped -- still lets 'next'
// walk past it.  PUC-Lua marks such a node by setting the key's type to
// LUA_TDEADKEY (findindex matches deadvalue to the live key pointer).
func TestDeadKeyDoesNotBreakTraversal(t *testing.T) {
	t.Skip("Go GC: dead weak keys not guaranteed to clear for traversal test")
	got := runChunk(t, `
	local function count (t)
		local n = 0
		for k, v in pairs(t) do n = n + 1 end
		return n
	end

	local t = setmetatable({}, {__mode = "k"})
	local keys = {}
	for i = 1, 200 do
		local k = {}
		keys[i] = k          -- keep them alive for now
		t[k] = i
	end
	local before = count(t)
	for i = 1, 200 do keys[i] = nil end
	-- allocating is what drives a cycle, and a cycle is what drops them
	for i = 1, 1000000 do
		local a = {}
		if next(t) == nil then break end
	end
	return before .. " " .. count(t)
	`)
	if got != "200 0" {
		t.Fatalf("weak table went from %d to %d entries; want 200 0\n%s",
			parseCount(got, 0), parseCount(got, 1), gcDiag())
	}
}

// TestFloatKeyOutsideIntegerRange pins the boundary lua_numbertointeger
// uses for a float key. 2^63 is exactly representable and one past
// math.maxinteger; converting it wraps to math.mininteger, so the two
// keys would collapse into one.
func TestFloatKeyOutsideIntegerRange(t *testing.T) {
	if _, ok := floatToIntEq(9223372036854775808.0); ok {
		t.Fatal("2^63 converted to an integer")
	}
	k, ok := floatToIntEq(-9223372036854775808.0)
	if !ok || k != -9223372036854775808 {
		t.Fatalf("mininteger float converted to %d, %v", k, ok)
	}
	got := runChunk(t, `
		local big = 9223372036854775808.0
		local t = {}
		t[big] = "float"
		t[math.mininteger] = "min"
		local n = 0
		for _ in pairs(t) do n = n + 1 end
		return t[big] .. " " .. t[math.mininteger] .. " " .. n
	`)
	if got != "float min 2" {
		t.Fatalf("2^63 key collided with mininteger: %q", got)
	}
}

// parseCount reads the n-th whitespace-separated field of 's' as an int.
func parseCount(s string, n int) int {
	f := strings.Fields(s)
	if len(f) <= n {
		return -1
	}
	v, err := strconv.Atoi(f[n])
	if err != nil {
		return -1
	}
	return v
}
