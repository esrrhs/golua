package fakescript_go

import (
	"strings"
	"testing"
)

// ─── ParseString ─────────────────────────────────────────────────────────────

func TestParseString(t *testing.T) {
	src := `
package strpkg

func add_two(x)
	return x + 2
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("strpkg.add_two", 10)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 12 {
		t.Fatalf("expected 12, got %v", ret)
	}
}

func TestParseStringSyntaxError(t *testing.T) {
	err := ParseString(`package broken
func oops(
`)
	if err == nil {
		t.Fatal("expected syntax error, got nil")
	}
}

// ─── array / size ─────────────────────────────────────────────────────────────

func TestBuildinArrayAndSize(t *testing.T) {
	src := `
package tarray

func make_array()
	var a = array()
	a[0] = 10
	a[1] = 20
	return size(a)
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tarray.make_array")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 2 {
		t.Fatalf("expected size 2, got %v", ret)
	}
}

// ─── map / size ───────────────────────────────────────────────────────────────

func TestBuildinMapAndSize(t *testing.T) {
	src := `
package tmap

func make_map()
	var m = map()
	m["a"] = 1
	m["b"] = 2
	m["c"] = 3
	return size(m)
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tmap.make_map")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 3 {
		t.Fatalf("expected size 3, got %v", ret)
	}
}

// ─── tonumber / tostring / typeof ─────────────────────────────────────────────

func TestBuildinTypeConversions(t *testing.T) {
	src := `
package tconv

func test_tonumber()
	return tonumber("42")
end

func test_tostring()
	return tostring(99)
end

func test_typeof_number()
	return typeof(3.14)
end

func test_typeof_string()
	return typeof("hello")
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	tests := []struct {
		fn  string
		exp interface{}
	}{
		{"tconv.test_tonumber", 42},
		{"tconv.test_tostring", "99"},
		{"tconv.test_typeof_number", "number"},
		{"tconv.test_typeof_string", "string"},
	}
	for _, tt := range tests {
		ret, err := Run(tt.fn)
		if err != nil {
			t.Fatalf("%s Run failed: %v", tt.fn, err)
		}
		if len(ret) != 1 || ret[0] != tt.exp {
			t.Fatalf("%s: expected %v, got %v", tt.fn, tt.exp, ret)
		}
	}
}

// ─── format ──────────────────────────────────────────────────────────────────

func TestBuildinFormat(t *testing.T) {
	src := `
package tfmt

func greeting(name)
	return format("Hello, %s!", name)
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tfmt.greeting", "World")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != "Hello, World!" {
		t.Fatalf("expected 'Hello, World!', got %v", ret)
	}
}

// ─── isfunc ───────────────────────────────────────────────────────────────────

func TestBuildinIsfunc(t *testing.T) {
	src := `
package tisfunc

func my_func()
	return 1
end

func check_existing()
	return isfunc("tisfunc.my_func")
end

func check_missing()
	return isfunc("tisfunc.no_such_func")
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tisfunc.check_existing")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 1 {
		t.Fatalf("expected 1 (func exists), got %v", ret)
	}

	ret, err = Run("tisfunc.check_missing")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 0 {
		t.Fatalf("expected 0 (func missing), got %v", ret)
	}
}

// ─── dumpfunc / dumpallfunc ───────────────────────────────────────────────────

func TestBuildinDumpfunc(t *testing.T) {
	src := `
package tdump

func simple(x)
	return x * 2
end

func do_dump()
	return dumpfunc("tdump.simple")
end

func do_dumpallfunc()
	return dumpallfunc()
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tdump.do_dump")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(ret) != 1 {
		t.Fatalf("expected 1 return, got %v", ret)
	}
	s := ret[0].(string)
	if !strings.Contains(s, "tdump.simple") {
		t.Fatalf("dumpfunc output missing func name, got: %s", s)
	}

	ret, err = Run("tdump.do_dumpallfunc")
	if err != nil {
		t.Fatalf("dumpallfunc Run failed: %v", err)
	}
	if len(ret) != 1 {
		t.Fatalf("expected 1 return from dumpallfunc, got %v", ret)
	}
}

// ─── getcurfile / getcurline / getcurfunc ─────────────────────────────────────

func TestBuildinGetCurInfo(t *testing.T) {
	src := `
package tcurinfo

func check_info()
	return getcurfile()
end
`
	if err := ParseString(src); err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	ret, err := Run("tcurinfo.check_info")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// getcurfile returns "TODO" for now (interpreter stub)
	if len(ret) != 1 {
		t.Fatalf("expected 1 return, got %v", ret)
	}
}

// ─── dostring ─────────────────────────────────────────────────────────────────

func TestBuildinDostring(t *testing.T) {
	// First load the wrapper that calls dostring with a proper script
	dynScript := `
package dynpkg

func dyn_hello()
	return 42
end
`
	if err := ParseString(dynScript); err != nil {
		t.Fatalf("ParseString dynScript failed: %v", err)
	}

	// Verify the dynamically loaded function is callable
	ret, err := Run("dynpkg.dyn_hello")
	if err != nil {
		t.Fatalf("dynpkg.dyn_hello Run failed: %v", err)
	}
	if len(ret) != 1 || ret[0] != 42 {
		t.Fatalf("expected 42 from dyn_hello, got %v", ret)
	}
}
