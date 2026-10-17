package lua

import (
	"strconv"
	"strings"
	"testing"
)

// TestNextOverDeletedKeys pins what makes 'next' able to resume from a
// key that the traversal has just deleted: 'keyisdead' has to look at the
// key's *type*, and at nothing else.
//
// A node's 'next' is a chain offset, and a negative offset is ordinary --
// 'luaH_newkey' stores 'f - mp', which is negative whenever the free slot
// it found for a colliding key sits below that key's main position, and
// that is the common case.  Taking -2 for a "dead" marker therefore made
// nodes look dead at random, and 'equalkey' -- which lets a dead node
// match any collectable key, on purpose -- then compared a closure
// against a node holding a long string.  In C that is a pointer
// comparison; here it was a type assertion, and it panicked inside
// 'next'.  Because which node ends up with offset -2 depends on the hash
// order, it showed up as a failure of nextvar.lua in roughly one run in
// twenty, and only under '-race' often enough to notice.
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
// LUA_TDEADKEY, which is what 'keyisdead' must answer to.
func TestDeadKeyDoesNotBreakTraversal(t *testing.T) {
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
		t.Fatalf("weak table went from %d to %d entries; want 200 0",
			parseCount(got, 0), parseCount(got, 1))
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
