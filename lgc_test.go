package lua

import "testing"

// runChunk runs a chunk and returns its single result as a string.
func runChunk(t *testing.T, src string) string {
	t.Helper()
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString(src, "=test"); err != nil {
		t.Fatalf("load: %v", err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("run: %s", msg)
	}
	s, _ := L.ToString(-1)
	return s
}

// The two things a Lua program can see the collector do, which this port
// has to supply on top of Go's (see lgc.go and lgc_weak.go).  Both are
// driven by allocating until the step fires; the loops are bounded so
// that a regression fails instead of hanging.

func TestFinalizerRuns(t *testing.T) {
	got := runChunk(t, `
		local ran = nil
		do
			local o = {}
			setmetatable(o, {__gc = function (self)
				ran = debug.getinfo(1).name
			end})
		end
		collectgarbage()   -- must be enough on its own
		return ran
	`)
	if got != "__gc" {
		t.Errorf("__gc did not run (or reported itself as %q)", got)
	}
}

func TestWeakValues(t *testing.T) {
	got := runChunk(t, `
		local cache = setmetatable({}, {__mode = "v"})
		do
			local o = {}     -- nothing but 'cache' refers to it
			cache[1] = o
		end
		collectgarbage()   -- must be enough on its own
		return tostring(cache[1])
	`)
	if got != "nil" {
		t.Errorf("weak value was not cleared: %s", got)
	}
}

func TestWeakKeys(t *testing.T) {
	got := runChunk(t, `
		local t = setmetatable({}, {__mode = "k"})
		do
			local key = {}   -- nothing but 't' refers to it
			t[key] = "value"
		end
		collectgarbage()   -- must be enough on its own
		local n = 0
		for _ in pairs(t) do n = n + 1 end
		return tostring(n)
	`)
	if got != "0" {
		t.Errorf("weak key was not cleared: %s entries left", got)
	}
}

// A weakly held entry that is still referred to elsewhere must survive.
func TestWeakKeepsWhatIsAlive(t *testing.T) {
	got := runChunk(t, `
		local keep = {}
		local t = setmetatable({}, {__mode = "kv"})
		t[keep] = keep
		for i = 1, 6000 do
			local a = {}
			if i % 512 == 0 then collectgarbage() end
			if t[keep] ~= keep then return "lost" end
		end
		return "kept"
	`)
	if got != "kept" {
		t.Errorf("a live entry was dropped from a weak table")
	}
}

func TestCollectGarbageSweepsWeak(t *testing.T) {
	got := runChunk(t, `
		local t = setmetatable({}, {__mode = "v"})
		do
			local o = {}
			t[1] = o
		end
		collectgarbage()   -- must be enough on its own
		return tostring(t[1])
	`)
	if got != "nil" {
		t.Errorf(`collectgarbage("collect") did not sweep the weak table: %s`, got)
	}
}

// Only tables, userdata, closures and threads can be weak, as in C.
func TestWeakIgnoresValues(t *testing.T) {
	got := runChunk(t, `
		local t = setmetatable({}, {__mode = "kv"})
		t["s"] = 1
		t[1] = "x"
		for i = 1, 6000 do
			local a = {}
			if i % 512 == 0 then collectgarbage() end
		end
		return tostring(t["s"]) .. tostring(t[1])
	`)
	if got != "1x" {
		t.Errorf("strings and numbers must not be weakened: %s", got)
	}
}

// The step has to fire on allocation alone: a program that never calls
// 'collectgarbage' still has to see its '__gc' run, which is what a
// module such as 'tracegc' -- and every 'db.lua' style loop -- relies
// on.  The loop is long enough to outlast one step (see gcStepDebt).
func TestFinalizerRunsOnAllocation(t *testing.T) {
	got := runChunk(t, `
		local ran = false
		do
			local o = {}
			setmetatable(o, {__gc = function () ran = true end})
		end
		for i = 1, 1000000 do
			local a = {}
			if ran then break end
		end
		return tostring(ran)
	`)
	if got != "true" {
		t.Errorf("__gc never ran from a collector step: %s", got)
	}
}

// A weak table has to be swept on allocation alone, for the same reason.
func TestWeakSweptOnAllocation(t *testing.T) {
	got := runChunk(t, `
		local cache = setmetatable({}, {__mode = "v"})
		do
			local o = {}     -- nothing but 'cache' refers to it
			cache[1] = o
		end
		for i = 1, 1000000 do
			local a = {}
			if cache[1] == nil then return "cleared" end
		end
		return "kept"
	`)
	if got != "cleared" {
		t.Errorf("weak value was not cleared by a collector step")
	}
}
