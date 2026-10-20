package core

import "testing"

// run runs a chunk and returns the state (the standard library is not
// needed by these tests, so they avoid assert/pairs/print).
func run(t *testing.T, src string) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v\nsource:\n%s", err, src)
	}
	return L
}

func globalNum(t *testing.T, L *State, name string) float64 {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	n, ok := L.ToNumber(-1)
	if !ok {
		t.Fatalf("global %s is not a number", name)
	}
	return n
}

func TestSmokeArithmetic(t *testing.T) {
	L := run(t, `
		x = 1 + 2 * 3
		y = 10 // 3
		z = 2 ^ 10
		w = 7 % 3
		v = 1.5 + 0.25
		s = "a" .. "b"
		b = 1 < 2
		c = 1 > 2
		d = 2 >= 2
		e = 3 ~= 4
		f = -x
		g = #"hello"
		h = 1 & 3
		i2 = 1 | 2
		j = 5 ~ 3
		k = 1 << 4
		l = 16 >> 2
		m = not nil
		n = 0x10
		o = 1e2
	`)
	for name, want := range map[string]float64{
		"x": 7, "y": 3, "z": 1024, "w": 1, "v": 1.75,
		"f": -7, "g": 5, "h": 1, "i2": 3, "j": 6,
		"k": 16, "l": 4, "n": 16, "o": 100,
	} {
		if got := globalNum(t, L, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	L.GetGlobal("s")
	if got, _ := L.ToString(-1); got != "ab" {
		t.Errorf("s = %q", got)
	}
	L.Pop(1)
	for name, want := range map[string]bool{
		"b": true, "c": false, "d": true, "e": true, "m": true,
	} {
		L.GetGlobal(name)
		if got := L.ToBoolean(-1); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
		L.Pop(1)
	}
}

func TestSmokeControl(t *testing.T) {
	L := run(t, `
		local sum = 0
		for i = 1, 10 do sum = sum + i end
		rsum = sum

		local s2 = 0
		for i = 10, 1, -2 do s2 = s2 + i end
		rstep = s2

		local i = 0
		while i < 5 do i = i + 1 end
		rwhile = i

		repeat i = i - 1 until i == 0
		rrepeat = i

		if sum == 55 then rok = true else rok = false end

		do local q = 1; rdo = q end

		-- (the standard library is not registered yet, so roll a plain
		-- iterator instead of using 'next'/'pairs')
		local function iter(t, i)
			i = i + 1
			if i <= #t then return i, t[i] end
		end
		local n = 0
		for k, v in iter, {1, 2, 3}, 0 do n = n + 1 + k - k + v - v end
		rgen = n

		local a = 0
		for j = 1, 3 do
			if j == 2 then goto cont end
			a = a + j
			::cont::
		end
		rgoto = a
	`)
	if got := globalNum(t, L, "rsum"); got != 55 {
		t.Errorf("rsum = %v", got)
	}
	if got := globalNum(t, L, "rstep"); got != 30 { // 10+8+6+4+2
		t.Errorf("rstep = %v", got)
	}
	if got := globalNum(t, L, "rwhile"); got != 5 {
		t.Errorf("rwhile = %v", got)
	}
	if got := globalNum(t, L, "rrepeat"); got != 0 {
		t.Errorf("rrepeat = %v", got)
	}
	if got := globalNum(t, L, "rgen"); got != 3 {
		t.Errorf("rgen = %v", got)
	}
	if got := globalNum(t, L, "rgoto"); got != 4 { // 1 + 3
		t.Errorf("rgoto = %v", got)
	}
	L.GetGlobal("rok")
	if !L.ToBoolean(-1) {
		t.Error("rok should be true")
	}
}

func TestSmokeFunctions(t *testing.T) {
	L := run(t, `
		local function add(a, b) return a + b end
		radd = add(3, 4)

		function gmul(a) return a * 2 end
		rgmul = gmul(21)

		local function va(...) local a, b = ... return a, b end
		local p, q = va(1, 2)
		rva = p * 10 + q

		local obj = {}
		function obj:m(x) return x + 1 end
		rmeth = obj:m(1)

		local up = 10
		local function closure() return up end
		rcl1 = closure()
		up = 20
		rcl2 = closure()

		local function varargs(...)
			local t = {...}
			return t[1], t[2], t[3]
		end
		local a, b, c = varargs(1, 2, 3)
		rvar = a * 100 + b * 10 + c

		local function multi() return 1, 2, 3 end
		local m1, m2, m3 = multi()
		rmulti = m1 * 100 + m2 * 10 + m3

		local t = {}
		function t.f() return 42 end
		rfield = t.f()

		local co = function() return 7 end
		rco = co()
	`)
	for name, want := range map[string]float64{
		"radd": 7, "rgmul": 42, "rva": 12, "rmeth": 2,
		"rcl1": 10, "rcl2": 20, "rvar": 123, "rmulti": 123,
		"rfield": 42, "rco": 7,
	} {
		if got := globalNum(t, L, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestSmokeTables(t *testing.T) {
	L := run(t, `
		local t = {}
		for i = 1, 100 do t[i] = i * i end
		r100 = t[100]

		local a = {10, 20, 30}
		rlen = #a
		r2 = a[2]

		local rec = {x = 1, y = 2, ["z"] = 3}
		rrec = rec.x + rec.y * 10 + rec.z * 100

		local mixed = {1, 2, 3, a = "x", [10] = "y"}
		rmix = #mixed

		local nested = {a = {b = {c = 5}}}
		rnest = nested.a.b.c
	`)
	for name, want := range map[string]float64{
		"r100": 10000, "rlen": 3, "r2": 20, "rrec": 321, "rmix": 3, "rnest": 5,
	} {
		if got := globalNum(t, L, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
}

func TestSmokeSyntaxErrors(t *testing.T) {
	for _, src := range []string{
		"x = ==", "x = 1 +", "for i = 1 do end",
		"local", "if true then", "x = 'unterminated",
		"function () end", "x = 0x", "::a:: ::a::",
	} {
		L := NewState()
		err := L.DoString(src)
		L.Close()
		if err == nil {
			t.Errorf("expected a syntax error for %q", src)
		} else {
			t.Logf("%-24s -> %v", src, err)
		}
	}
}
