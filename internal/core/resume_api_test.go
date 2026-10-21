package core

import "testing"

// Public/C lua_resume leaves results on the resumed thread and accepts a
// nil from, matching PUC-Lua 5.4.
func TestResumeNilFromLeavesResultsOnCoroutine(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	co := L.NewThread()
	if err := L.LoadString(`return "ok", 2`, "chunk"); err != nil {
		t.Fatal(err)
	}
	co.top = 0
	co.XMove(L, 1)
	status := co.Resume(nil, 0)
	if status != LUA_OK {
		t.Fatalf("status %d", status)
	}
	if co.GetTop() != 2 {
		t.Fatalf("GetTop=%d, want 2 (results on coroutine)", co.GetTop())
	}
	s, ok := co.ToString(1)
	if !ok || s != "ok" {
		t.Fatalf("result[1]=%q ok=%v", s, ok)
	}
	n, ok := co.ToInteger(2)
	if !ok || n != 2 {
		t.Fatalf("result[2]=%d ok=%v", n, ok)
	}
}

// A second Resume on a finished coroutine must report one error result,
// not the previous successful nresults still sitting in co.vals.
func TestResumeDeadReportsOneError(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	co := L.NewThread()
	if err := L.LoadString(`return "ok", 2, 3`, "chunk"); err != nil {
		t.Fatal(err)
	}
	co.top = 0
	co.XMove(L, 1)
	if st := co.Resume(nil, 0); st != LUA_OK {
		t.Fatalf("first status %d", st)
	}
	if len(co.co.vals) != 3 {
		t.Fatalf("first nres=%d, want 3", len(co.co.vals))
	}
	st := co.Resume(nil, 0)
	if st != LUA_ERRRUN {
		t.Fatalf("second status %d, want ERRRUN", st)
	}
	if len(co.co.vals) != 1 {
		t.Fatalf("dead nres=%d, want 1 (stale success count)", len(co.co.vals))
	}
	s, _ := co.ToString(-1)
	if s != "cannot resume dead coroutine" {
		t.Fatalf("error %q", s)
	}
}

// Yielded values also stay on the coroutine for a direct Resume caller.
func TestResumeLeavesYieldOnCoroutine(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	co := L.NewThread()
	if err := L.LoadString(`coroutine.yield("y", 9); return "done"`, "chunk"); err != nil {
		t.Fatal(err)
	}
	co.top = 0
	co.XMove(L, 1)
	st := co.Resume(L, 0)
	if st != LUA_YIELD {
		t.Fatalf("first status %d", st)
	}
	if co.GetTop() != 2 {
		t.Fatalf("yield GetTop=%d, want 2", co.GetTop())
	}
	s, _ := co.ToString(1)
	n, _ := co.ToInteger(2)
	if s != "y" || n != 9 {
		t.Fatalf("yield results %q %d", s, n)
	}
	/* coroutine.resume moves them; a direct caller must xmove itself */
	L.XMove(co, 2)
	st = co.Resume(L, 0)
	if st != LUA_OK {
		t.Fatalf("second status %d", st)
	}
	if co.GetTop() != 1 {
		t.Fatalf("return GetTop=%d, want 1", co.GetTop())
	}
	s, _ = co.ToString(1)
	if s != "done" {
		t.Fatalf("return %q", s)
	}
}
