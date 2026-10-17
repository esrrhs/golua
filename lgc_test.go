package lua

import (
	"fmt"
	"runtime"
	"testing"
	"time"
	"weak"
)

// gcDiag describes the state of the GC-delegation mechanism, and asks
// Go's own collector the two questions the whole design rests on.
//
// It is only ever printed when one of these tests fails, which is the
// point of it: every one of them fails the same way -- "the cycle did
// not do anything" -- and that can mean the object is still reachable,
// that no cycle ran, or that Go's finalizers and weak pointers do not
// behave here as they do everywhere else.  Without this the only thing
// a red CI can say is "did not run".
func gcDiag() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s := fmt.Sprintf("goroutines=%d gomaxprocs=%d numgc=%d heap=%d KB\n",
		runtime.NumGoroutine(), runtime.GOMAXPROCS(0), m.NumGC, m.HeapAlloc/1024)
	gcWatchMu.Lock()
	s += fmt.Sprintf("watched=%d gcWatched=%d gcWeakCount=%d gcWaiting=%d gcHold=%d\n",
		len(gcWatch), gcWatched.Load(), gcWeakCount.Load(), gcWaiting.Load(), len(gcHold))
	gcWatchMu.Unlock()
	gcThrMu.Lock()
	s += fmt.Sprintf("threads tracked=%d\n", len(gcThrs))
	gcThrMu.Unlock()

	/* Does Go's own machinery work in this process at all? */
	type box struct{ pad [64]byte }
	var wp weak.Pointer[box]
	func() {
		b := new(box)
		wp = weak.Make(b)
	}()
	runtime.GC()
	runtime.GC()
	s += fmt.Sprintf("a bare weak pointer clears here: %v\n", wp.Value() == nil)

	done := make(chan struct{}, 1)
	func() {
		b := new(box)
		runtime.SetFinalizer(b, func(*box) { done <- struct{}{} })
	}()
	for i := 0; i < 3; i++ {
		runtime.GC()
		runtime.Gosched()
	}
	select {
	case <-done:
		s += "a bare finalizer runs here: true\n"
	case <-time.After(200 * time.Millisecond):
		s += "a bare finalizer runs here: false\n"
	}
	return s
}

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
		t.Errorf("__gc did not run (or reported itself as %q)\n%s", got, gcDiag())
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
		t.Errorf("weak key was not cleared: %s entries left\n%s", got, gcDiag())
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
		t.Errorf("__gc never ran from a collector step: %s\n%s", got, gcDiag())
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
		t.Errorf("weak value was not cleared by a collector step\n%s", gcDiag())
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
		t.Errorf("collectgarbage inside a finalizer: %s\n%s", got, gcDiag())
	}
}
