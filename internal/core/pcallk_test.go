package core

import "testing"

// pcallK alignment tests vs PUC Lua 5.3.4 lapi.c.
//
// The yieldable branch (CIST_YPCALL + bare luaD_call) only runs when
// k!=nil and nny==0 — i.e. inside a resumed coroutine. Main-thread
// pcall uses the conventional luaD_pcall path, matching C.

func TestPCallKYieldableMarksYPCALL(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var sawYPCALL bool
	L.PushGoFunc(func(L *State) int {
		/* Mark lives on the pcallK caller frame (basePCall), not this one. */
		for ci := L.ci; ci != nil; ci = ci.prev {
			if ci.ypcall {
				sawYPCALL = true
				break
			}
		}
		return 0
	})
	L.SetGlobal("probe")

	if err := L.DoString(`
		local co = coroutine.create(function() pcall(probe) end)
		local ok, err = coroutine.resume(co)
		assert(ok, tostring(err))
	`); err != nil {
		t.Fatalf("coroutine pcall(probe): %v", err)
	}
	if !sawYPCALL {
		t.Fatal("expected ci.ypcall (CIST_YPCALL) during yieldable pcallK")
	}
}

func TestPCallKYieldableBareCallRecoversError(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	if err := L.DoString(`
		local co = coroutine.create(function()
			local ok, err = pcall(function() error("boom", 0) end)
			assert(ok == false, "pcall should fail")
			assert(err == "boom", tostring(err))
			local ok2, err2 = xpcall(function() error("x", 0) end,
				function(e) return "H:" .. e end)
			assert(ok2 == false)
			assert(err2 == "H:x", tostring(err2))
			return true
		end)
		local ok, v = coroutine.resume(co)
		assert(ok and v == true, tostring(v))
	`); err != nil {
		t.Fatalf("bare pcallK error recovery: %v", err)
	}
}

func TestPCallKYieldAcrossPCall(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	if err := L.DoString(`
		local co = coroutine.create(function()
			local ok, a, b = pcall(function()
				return coroutine.yield(10, 20)
			end)
			assert(ok == true)
			return a, b
		end)
		local ok, x, y = coroutine.resume(co)
		assert(ok and x == 10 and y == 20)
		ok, x, y = coroutine.resume(co, "A", "B")
		assert(ok and x == "A" and y == "B")
	`); err != nil {
		t.Fatalf("yield across pcall: %v", err)
	}
}

func TestPCallKNonYieldableUsesProtected(t *testing.T) {
	/* Main thread nny!=0 → conventional luaD_pcall branch. */
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	L.PushGoFunc(func(L *State) int {
		L.PushString("fail")
		return L.Error()
	})
	st := L.PCall(0, 0, 0)
	if st == LUA_OK {
		t.Fatal("expected PCall to catch error on main thread")
	}
	if !L.IsString(-1) {
		t.Fatalf("expected error object on stack, got %s", TypeName(L.Type(-1)))
	}
}

func TestPCallKYPCallClearedAfterReturn(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	if err := L.DoString(`
		local co = coroutine.create(function()
			pcall(function() end)
			pcall(function() error("x", 0) end)
		end)
		assert(coroutine.resume(co))
	`); err != nil {
		t.Fatal(err)
	}
	for ci := L.ci; ci != nil; ci = ci.prev {
		if ci.ypcall {
			t.Fatal("ci.ypcall still set after yieldable pcallK returned")
		}
	}
}

func TestPCallKErrFuncRestored(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	if err := L.DoString(`
		local co = coroutine.create(function()
			local handler = function(e) return "wrapped:" .. e end
			local ok, err = xpcall(function() error("e", 0) end, handler)
			assert(not ok and err == "wrapped:e", tostring(err))
			-- After xpcall, a plain error should not use the old handler.
			local ok2, err2 = pcall(function() error("plain", 0) end)
			assert(not ok2 and err2 == "plain", tostring(err2))
		end)
		local ok, err = coroutine.resume(co)
		assert(ok, tostring(err))
	`); err != nil {
		t.Fatalf("errfunc restore: %v", err)
	}
}
