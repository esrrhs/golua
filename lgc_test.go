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
// has to supply on top of Go's (see lgc.go and lgc_weak.go).  Since
// 'collectgarbage' is inert -- see TestCollectGarbageIsInert -- both are
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
		for i = 1, 1000000 do   -- allocating is what drives a cycle
			local a = {}
			if ran then break end
		end
		return ran
	`)
	if got != "__gc" {
		t.Errorf("__gc did not run (or reported itself as %q)", got)
	}
}

// 'collectgarbage' answers with 0 and brings nothing forward: the
// objects are Go's, and Go decides when to reclaim them.  What still
// runs a '__gc' is the step an allocation drives, and the closing of
// the state.
func TestCollectGarbageIsInert(t *testing.T) {
	got := runChunk(t, `
		local n = 0
		do
			local o = {}
			setmetatable(o, {__gc = function () n = n + 1 end})
		end
		local c = collectgarbage("collect")
		local s = collectgarbage("step")
		return tostring(c) .. "," .. tostring(s) .. "," .. tostring(n)
	`)
	if got != "0,0,0" {
		t.Errorf("collectgarbage drove a cycle: %s", got)
	}
}

func TestWeakKeys(t *testing.T) {
	got := runChunk(t, `
		local t = setmetatable({}, {__mode = "k"})
		do
			local key = {}   -- nothing but 't' refers to it
			t[key] = "value"
		end
		for i = 1, 1000000 do   -- allocating is what drives a cycle
			local a = {}
			if next(t) == nil then break end
		end
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
			local a = {}   -- allocating is what drives a cycle
			if t[keep] ~= keep then return "lost" end
		end
		return "kept"
	`)
	if got != "kept" {
		t.Errorf("a live entry was dropped from a weak table")
	}
}

// Only tables, userdata, closures and threads can be weak, as in C.
func TestWeakIgnoresValues(t *testing.T) {
	got := runChunk(t, `
		local t = setmetatable({}, {__mode = "kv"})
		t["s"] = 1
		t[1] = "x"
		for i = 1, 6000 do
			local a = {}   -- allocating is what drives a cycle
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

// TestCollectGarbageInsideAFinalizerAnswersNil pins that 'collectgarbage'
// is not merely inert but *refused* inside a '__gc': PUC-Lua stops the
// collector for the duration of a finalizer, 'lua_gc' answers -1 for
// every option, and lbaselib.c's 'checkvalres' turns that into a fail.
// A program can see the difference -- 'print(collectgarbage())' says nil
// there and 0 anywhere else.
func TestCollectGarbageInsideAFinalizerAnswersNil(t *testing.T) {
	got := runChunk(t, `
		local seen
		do
			local o = {}
			setmetatable(o, {__gc = function ()
				seen = tostring(collectgarbage()) .. "," ..
				       tostring(collectgarbage("count")) .. "," ..
				       tostring(collectgarbage("isrunning"))
			end})
		end
		for i = 1, 1000000 do local a = {} end   -- a cycle: see lgc.go
		return seen .. " / " .. tostring(collectgarbage())
	`)
	if got != "nil,nil,nil / 0" {
		t.Errorf("collectgarbage inside a finalizer: %s", got)
	}
}
