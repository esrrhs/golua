package core

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// os.exit(0, true) from a coroutine used to deadlock: Close asked the
// calling thread to exit via co.req while that same goroutine was inside
// closeAllThreads.
func TestCloseFromCoroutineDoesNotHang(t *testing.T) {
	old := osExitHook
	osExitHook = func(int) {}
	t.Cleanup(func() { osExitHook = old })

	L := NewState()
	L.OpenLibs()
	done := make(chan error, 1)
	go func() {
		done <- L.DoString(`
			local co = coroutine.create(function()
				os.exit(0, true)
			end)
			assert(coroutine.resume(co))
		`)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("DoString: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close from coroutine hung (closeAllThreads self-deadlock)")
	}
}

// C's close_state runs luaF_close on the main stack before finalizers.
func TestCloseRunsMainThreadTBC(t *testing.T) {
	L := NewState()
	ran := false
	L.NewTable()
	L.NewTable()
	L.PushGoFunc(func(L *State) int {
		ran = true
		return 0
	})
	L.SetField(-2, "__close")
	L.SetMetaTable(-2)
	luaF_newtbcupval(L, L.top-1)
	L.Close()
	if !ran {
		t.Fatal("main-thread to-be-closed __close did not run on Close")
	}
}

// C's lua_close does not run child-coroutine __close; a suspended wrap
// whose __close is os.exit must be collectable. Close still stops the
// child goroutine and runs __gc for watched objects.
func TestCloseCollectsSuspendedChildWithoutClose(t *testing.T) {
	L := NewState()
	L.OpenLibs()
	var log strings.Builder
	var mu sync.Mutex
	L.PushGoFunc(func(L *State) int {
		mu.Lock()
		log.WriteString(L.CheckString(1))
		log.WriteByte(';')
		mu.Unlock()
		return 0
	})
	L.SetGlobal("note")
	if err := L.DoString(`
		local mt = {
			__close = function() note("close") end,
			__gc = function() note("gc") end,
		}
		co = coroutine.create(function()
			local x <close> = setmetatable({}, mt)
			coroutine.yield()
		end)
		assert(coroutine.resume(co))
	`); err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("co")
	co := L.ToThread(-1)
	L.Pop(1)
	L.Close()
	if co != nil && !co.dead {
		t.Fatal("suspended child was not stopped")
	}
	mu.Lock()
	got := log.String()
	mu.Unlock()
	/* __close must not run on state Close (C does not); coroutine.close does. */
	if strings.Contains(got, "close;") {
		t.Fatalf("log %q: child __close ran on state Close", got)
	}
}

// Closing a dead coroutine must report a pending __close error, not the
// original body error that lastErr still holds.
func TestCoroutineCloseReportsCloseError(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	if err := L.DoString(`
		local co = coroutine.create(function()
			local x <close> = setmetatable({}, {
				__close = function() error("close", 0) end
			})
			error("body", 0)
		end)
		assert(not coroutine.resume(co))
		local ok, err = coroutine.close(co)
		_G.out = tostring(ok) .. " " .. tostring(err)
	`); err != nil {
		t.Fatal(err)
	}
	L.GetGlobal("out")
	s, _ := L.ToString(-1)
	if s != "false close" {
		t.Fatalf("got %q, want %q", s, "false close")
	}
}

// Registering a __gc object in one state must not lose its hold when
// another state collects. (States are not safe for concurrent DoString
// under the race detector; the hold list itself is what must stay
// isolated across sequential collect cycles.)
func TestGCHoldSurvivesOtherStateCollect(t *testing.T) {
	L1 := NewState()
	L2 := NewState()
	t.Cleanup(L1.Close)
	t.Cleanup(L2.Close)
	L1.OpenLibs()
	L2.OpenLibs()

	for i := 0; i < 50; i++ {
		if err := L1.DoString(`
			local o = {}
			setmetatable(o, {__gc = function() end})
		`); err != nil {
			t.Fatal(err)
		}
		if err := L2.DoString(`collectgarbage("collect")`); err != nil {
			t.Fatal(err)
		}
	}
	if err := L1.DoString(`collectgarbage("collect")`); err != nil {
		t.Fatal(err)
	}
}
