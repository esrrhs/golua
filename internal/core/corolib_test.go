package core

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

// coNew creates a state with the base and the coroutine libraries opened.
func coNew(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenBase(L)
	L.Pop(1) /* remove the global table left on the stack */
	OpenCoroutine(L)
	L.SetGlobal("coroutine")
	return L
}

// coRun runs a chunk, failing the test on any error.
func coRun(t *testing.T, L *State, src string) {
	t.Helper()
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v\nsource:\n%s", err, src)
	}
}

// coGlobal reads a global and returns it as a string.
func coGlobal(t *testing.T, L *State, name string) string {
	t.Helper()
	top := L.GetTop()
	L.GetGlobal(name)
	if L.Type(-1) == LUA_TNIL || L.Type(-1) == LUA_TNONE {
		t.Fatalf("global %s is not set", name)
	}
	s := L.ToStringX(-1)
	L.Pop(1)
	if L.GetTop() != top {
		t.Fatalf("stack unbalanced reading %s: %d != %d", name, L.GetTop(), top)
	}
	return s
}

// ---------------------------------------------------------------------
// create / resume
// ---------------------------------------------------------------------

func TestCoroutineCreateResume(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function(a)
			local x = coroutine.yield(a + 1)   -- first: 2
			local y = coroutine.yield(x * 10)  -- second: 30
			return y .. "/end"
		end)
		local ok1, v1 = coroutine.resume(co, 1)
		local ok2, v2 = coroutine.resume(co, 3)
		local ok3, v3 = coroutine.resume(co, "E")
		out = tostring(ok1) .. "," .. tostring(v1) .. "," ..
		      tostring(ok2) .. "," .. tostring(v2) .. "," ..
		      tostring(ok3) .. "," .. tostring(v3)
	`)
	if got, want := coGlobal(t, L, "out"), "true,2,true,30,true,E/end"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineResumeError(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function() error("inside") end)
		local ok, err = coroutine.resume(co)
		out = tostring(ok) .. "," .. tostring(err)
		-- resuming a dead coroutine
		local ok2, err2 = coroutine.resume(co)
		out = out .. "|" .. tostring(ok2) .. "," .. tostring(err2)
	`)
	got := coGlobal(t, L, "out")
	if !strings.HasPrefix(got, "false,") || !strings.Contains(got, "inside") {
		t.Errorf("out = %q, want error containing \"inside\"", got)
	}
	if !strings.HasSuffix(got, "|false,cannot resume dead coroutine") {
		t.Errorf("out = %q, want dead-coroutine diagnostic", got)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineResumeBadArgument(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local ok, err = pcall(coroutine.resume, 42)
		out = tostring(ok) .. "," .. tostring(err)
	`)
	got := coGlobal(t, L, "out")
	if !strings.HasPrefix(got, "false,") || !strings.Contains(got, "thread expected") {
		t.Errorf("out = %q, want \"thread expected\"", got)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineYieldOutside(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local ok, err = pcall(coroutine.yield, 1, 2)
		out = tostring(ok) .. "," .. tostring(err)
	`)
	got := coGlobal(t, L, "out")
	if !strings.HasPrefix(got, "false,") ||
		!strings.Contains(got, "attempt to yield from outside a coroutine") {
		t.Errorf("out = %q", got)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------
// wrap
// ---------------------------------------------------------------------

func TestCoroutineWrap(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local w = coroutine.wrap(function(a)
			local x = coroutine.yield(a + 1)
			return x * 2
		end)
		out = tostring(w(1)) .. "," .. tostring(w(10))
	`)
	if got, want := coGlobal(t, L, "out"), "2,20"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineWrapError(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		-- error raised after a yield: the wrapper must propagate it
		local w = coroutine.wrap(function()
			coroutine.yield(1)
			error("boom")
		end)
		w()
		local ok, err = pcall(w)
		out = tostring(ok) .. "," .. tostring(err)

		-- error raised before any yield
		local w2 = coroutine.wrap(function() error("early") end)
		local ok2, err2 = pcall(w2)
		out = out .. "|" .. tostring(ok2) .. "," .. tostring(err2)

		-- a non-string error object is propagated unchanged
		local w3 = coroutine.wrap(function() error({code = 7}) end)
		local ok3, err3 = pcall(w3)
		out = out .. "|" .. tostring(ok3) .. "," .. tostring(type(err3))
	`)
	got := coGlobal(t, L, "out")
	parts := strings.Split(got, "|")
	if len(parts) != 3 {
		t.Fatalf("out = %q, want 3 fields", got)
	}
	if !strings.HasPrefix(parts[0], "false,") || !strings.Contains(parts[0], "boom") {
		t.Errorf("wrapped error = %q, want it to contain \"boom\"", parts[0])
	}
	if !strings.HasPrefix(parts[1], "false,") || !strings.Contains(parts[1], "early") {
		t.Errorf("early error = %q, want it to contain \"early\"", parts[1])
	}
	if parts[2] != "false,table" {
		t.Errorf("non-string error = %q, want \"false,table\"", parts[2])
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------
// status
// ---------------------------------------------------------------------

func TestCoroutineStatus(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local st = {}
		local co
		st[#st + 1] = coroutine.status(coroutine.running())  -- main: running
		co = coroutine.create(function()
			st[#st + 1] = coroutine.status(co)              -- running
			coroutine.yield()
		end)
		st[#st + 1] = coroutine.status(co)                  -- suspended (fresh)
		coroutine.resume(co)
		st[#st + 1] = coroutine.status(co)                  -- suspended (yielded)
		coroutine.resume(co)
		st[#st + 1] = coroutine.status(co)                  -- dead
		out = st[1] .. "," .. st[2] .. "," .. st[3] .. "," .. st[4] .. "," .. st[5]
	`)
	if got, want := coGlobal(t, L, "out"), "running,suspended,running,suspended,dead"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineStatusError(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function() error("x") end)
		coroutine.resume(co)
		out = coroutine.status(co)
	`)
	if got, want := coGlobal(t, L, "out"), "dead"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------
// isyieldable / running
// ---------------------------------------------------------------------

func TestCoroutineIsYieldable(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local a = coroutine.isyieldable()                    -- main: false
		local co = coroutine.create(function()
			inside = coroutine.isyieldable()                 -- true
			coroutine.yield()
		end)
		coroutine.resume(co)
		out = tostring(a) .. "," .. tostring(inside) ..
		      "," .. tostring(coroutine.isyieldable(co))
	`)
	if got, want := coGlobal(t, L, "out"), "false,true,true"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineRunning(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local t, ismain = coroutine.running()
		local co = coroutine.create(function()
			ct, cm = coroutine.running()
		end)
		coroutine.resume(co)
		out = tostring(ismain) .. "," .. tostring(cm) ..
		      "," .. tostring(t == ct) .. "," .. type(t) .. "," .. type(ct)
	`)
	if got, want := coGlobal(t, L, "out"), "true,false,false,thread,thread"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------
// close
// ---------------------------------------------------------------------

func TestCoroutineCloseSuspended(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function()
			coroutine.yield(1)
			coroutine.yield(2)
		end)
		coroutine.resume(co)
		local ok, err = coroutine.close(co)
		out = tostring(ok) .. "," .. tostring(err) ..
		      "," .. coroutine.status(co)
		local ok2, err2 = coroutine.resume(co)
		out = out .. "|" .. tostring(ok2) .. "," .. tostring(err2)
		-- closing an already closed coroutine is a no-op
		local ok3, err3 = coroutine.close(co)
		out = out .. "|" .. tostring(ok3) .. "," .. tostring(err3)
	`)
	if got, want := coGlobal(t, L, "out"),
		"true,nil,dead|false,cannot resume dead coroutine|true,nil"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// TestCoroutineCloseMayNotYield pins the one thing the coroutine fuzz
// found, and the other half of it, so that fixing one half cannot hide
// behind the other.
//
// C runs a pending '__close' from 'closepaux', whose 'yy' is 0, so a
// metamethod closed by 'coroutine.close' is not allowed to suspend:
// 'coroutine.close' answers false, "attempt to yield across a C-call
// boundary", and not the value the metamethod yielded, which is what this
// port answered.  The same variable closed the ordinary way -- the end of
// its block -- is 'luaF_close' with 'yy' 1, and there suspending is
// allowed: the coroutine yields from inside its own '__close' and the
// next resume carries on.
func TestCoroutineCloseMayNotYield(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function()
			local x <close> = setmetatable({}, {
				__close = function(_, err) return coroutine.yield("from __close") end
			})
			coroutine.yield("body")
		end)
		coroutine.resume(co)
		-- 'x' is still pending: 'coroutine.close' is what closes it
		local ok, err = coroutine.close(co)
		out = tostring(ok) .. "," .. tostring(err) .. "," .. coroutine.status(co)

		local co2 = coroutine.create(function()
			local y <close> = setmetatable({}, {
				__close = function(_, err) coroutine.yield("closing") return "ignored" end
			})
			return "body"
		end)
		-- 'y' is closed by the end of the coroutine, which may suspend
		local ok1, v1 = coroutine.resume(co2)
		local ok2, v2 = coroutine.resume(co2)
		out2 = tostring(ok1) .. "," .. tostring(v1) ..
		       "|" .. tostring(ok2) .. "," .. tostring(v2) ..
		       "|" .. coroutine.status(co2)
	`)
	if got, want := coGlobal(t, L, "out"),
		"false,attempt to yield across a C-call boundary,dead"; got != want {
		t.Errorf("closed by coroutine.close: out = %q, want %q", got, want)
	}
	if got, want := coGlobal(t, L, "out2"), "true,closing|true,body|dead"; got != want {
		t.Errorf("closed by the end of the block: out2 = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineCloseRunning(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		-- a coroutine cannot close itself: the error escapes through resume
		local co
		co = coroutine.create(function()
			coroutine.close(co)
		end)
		local ok, err = coroutine.resume(co)
		-- nor can the main thread be closed
		local ok2, err2 = pcall(coroutine.close, coroutine.running())
		out = tostring(ok) .. "," .. tostring(err) ..
		      "|" .. tostring(ok2) .. "," .. tostring(err2)
	`)
	got := coGlobal(t, L, "out")
	parts := strings.Split(got, "|")
	if len(parts) != 2 {
		t.Fatalf("out = %q, want 2 fields", got)
	}
	for i, p := range parts {
		if !strings.HasPrefix(p, "false,") ||
			!strings.Contains(p, "cannot close a running coroutine") {
			t.Errorf("field %d = %q, want \"cannot close a running coroutine\"", i, p)
		}
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

func TestCoroutineCloseDead(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local co = coroutine.create(function() return 1 end)
		coroutine.resume(co)
		local ok, err = coroutine.close(co)
		out = tostring(ok) .. "," .. tostring(err)
	`)
	if got, want := coGlobal(t, L, "out"), "true,nil"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// ---------------------------------------------------------------------
// nested resume
// ---------------------------------------------------------------------

func TestCoroutineNestedResume(t *testing.T) {
	L := coNew(t)
	coRun(t, L, `
		local outer
		local inner = coroutine.create(function(a)
			local b = coroutine.yield(a * 2)
			-- 'outer' is running while it resumes us: not resumable
			nv2 = coroutine.status(outer)            -- normal
			local okr, errr = coroutine.resume(outer) -- not resumable
			nv2 = nv2 .. ";" .. tostring(okr) .. "," .. tostring(errr)
			return "in:" .. tostring(b)
		end)
		outer = coroutine.create(function()
			local ok1, v1 = coroutine.resume(inner, 21)
			local ok2, v2 = coroutine.resume(inner, 7)
			nv = tostring(ok1) .. "," .. tostring(v1) ..
			     ";" .. tostring(ok2) .. "," .. tostring(v2)
			-- the outer coroutine can still yield
			coroutine.yield("out1")
			return "done"
		end)
		local ok1, v1 = coroutine.resume(outer)
		local ok2, v2 = coroutine.resume(outer)
		out = nv .. "|" .. nv2 ..
		      "|" .. tostring(ok1) .. "," .. tostring(v1) ..
		      ";" .. tostring(ok2) .. "," .. tostring(v2) ..
		      "|" .. coroutine.status(outer)
	`)
	if got, want := coGlobal(t, L, "out"),
		"true,42;true,in:7|normal;false,cannot resume non-suspended coroutine"+
			"|true,out1;true,done|dead"; got != want {
		t.Errorf("out = %q, want %q", got, want)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not empty: %d", L.GetTop())
	}
}

// TestCoroutineGoLevel exercises the library through the Go API instead
// of Lua chunks: it checks that the values handed to and returned by a
// coroutine really travel across the goroutine boundary.
func TestCoroutineGoLevel(t *testing.T) {
	L := coNew(t)
	/* __co = coroutine.create(function(a) return coroutine.yield(a+1)*2 end) */
	L.GetGlobal("coroutine")
	L.GetField(-1, "create")
	L.PushGoFunc(func(L *State) int {
		a, _ := L.ToInteger(1)
		L.GetGlobal("coroutine")
		L.GetField(-1, "yield")
		L.PushInteger(a + 1)
		/* CallK: 'coroutine.yield' must be able to suspend, so the
		   call has to be the yieldable lua_callk, not lua_call. */
		L.CallK(1, 1)
		n, _ := L.ToInteger(-1)
		L.PushInteger(n * 2)
		return 1
	})
	L.Call(1, 1)
	/* stack: [coroutine, thread] */
	if L.Type(-1) != LUA_TTHREAD {
		t.Fatalf("create did not return a thread (got %s)", TypeName(L.Type(-1)))
	}
	L.SetGlobal("__co")
	L.Pop(1) /* remove the coroutine table */

	resume := func(args ...int64) (bool, int64) {
		L.GetGlobal("coroutine")
		L.GetField(-1, "resume") /* [coroutine, resume] */
		L.Remove(-2)             /* [resume] */
		L.GetGlobal("__co")      /* [resume, co] */
		for _, a := range args {
			L.PushInteger(a)
		}
		L.Call(1+len(args), 2)
		ok := L.ToBoolean(-2)
		v, _ := L.ToInteger(-1)
		L.Pop(2)
		return ok, v
	}
	if ok, v := resume(1); !ok || v != 2 {
		t.Errorf("first resume = (%v,%d), want (true,2)", ok, v)
	}
	if ok, v := resume(10); !ok || v != 20 {
		t.Errorf("second resume = (%v,%d), want (true,20)", ok, v)
	}
	if L.GetTop() != 0 {
		t.Errorf("stack not balanced: %d", L.GetTop())
	}
}
