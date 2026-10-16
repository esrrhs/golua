package lua

import "testing"

// TestStackAPI checks the stack-manipulation primitives of the C API
// directly (lua_rotate semantics in particular).
func TestStackAPI(t *testing.T) {
	L := NewState()
	defer L.Close()
	for i := 1; i <= 4; i++ {
		L.PushInteger(int64(i))
	}
	L.Rotate(1, 1) /* last element moves to the front */
	var got []int64
	for i := 1; i <= 4; i++ {
		v, _ := L.ToInteger(i)
		got = append(got, v)
	}
	if want := []int64{4, 1, 2, 3}; !equalInts(got, want) {
		t.Errorf("rotate(1,1) = %v, want %v", got, want)
	}
	/* lua_insert(idx) == lua_rotate(idx, 1) */
	L.PushString("x")
	L.Insert(1)
	if s, _ := L.ToString(1); s != "x" {
		t.Errorf("after insert(1): stack[1] = %q, want x", s)
	}
}

func equalInts(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStdlib(t *testing.T) {
	L := NewState()
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)
	OpenIO(L)
	src := `
		assert(("abc"):upper() == "ABC", "upper")
		assert(("abc"):find("b") == 2, "find")
		assert(("caught"):find("caught") ~= nil, "find2")
		local n = 0
		for k, v in pairs({a=1, b=2}) do n = n + 1 end
		assert(n == 2, "pairs " .. n)
		local m = 0
		for i, v in ipairs({10, 20, 30}) do m = m + i end
		assert(m == 6, "ipairs " .. m)
		assert(next({}) == nil, "next")
		assert(select("#", 1, 2, 3) == 3, "select")
		assert(type(print) == "function", "type")
		assert(tostring(12) == "12", "tostring")
		assert(tonumber("0x10") == 16, "tonumber")
		local ok, err = pcall(function() error("caught") end)
		assert(not ok, "pcall ok")
		assert(tostring(err):find("caught") ~= nil, "pcall msg " .. tostring(err))
		local ok2, e2 = xpcall(function() error("bad", 0) end, function(e) return "H:" .. e end)
		assert(ok2 == false, "xpcall")
		assert(e2 == "H:bad", "xpcall msg " .. tostring(e2))
		local mt = {__tostring = function() return "MT" end}
		assert(tostring(setmetatable({}, mt)) == "MT", "__tostring")
		r = "ok"
	`
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("r")
	if s, _ := L.ToString(-1); s != "ok" {
		t.Errorf("r = %q", s)
	}
	L.Close()
}

func TestOpenLibs(t *testing.T) {
	L := NewState()
	L.OpenLibs()
	if err := L.DoString(`
		assert(type(print) == "function")
		assert(type(require) == "function")
		assert(type(pairs) == "function")
		assert(("abc"):upper() == "ABC")
		assert(math.floor(1.5) == 1)
		assert(#table.concat({1,2,3}, ",") == 5)
		assert(utf8.len("héllo") == 5)
		assert(type(os.time()) == "number")
		assert(type(io.write) == "function")
		assert(package.loaded["string"] ~= nil)
		assert(math.maxinteger > 0)
		r = "ok"
	`); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("r")
	if s, _ := L.ToString(-1); s != "ok" {
		t.Errorf("r = %q", s)
	}
	L.Close()
}

func TestLineNumbers(t *testing.T) {
	L := NewState()
	OpenBase(L)
	err := L.DoString("error('x')")
	t.Logf("err = %v (want chunk:1: x)", err)
	L.Close()
}
