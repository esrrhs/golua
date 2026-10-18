package lua

import "testing"

func semRun(t *testing.T, src string) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenBase(L)
	OpenString(L)
	OpenTable(L)
	OpenMath(L)
	OpenUTF8(L)
	L.Pop(1) /* the global table pushed by OpenBase */
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v\nsource:\n%s", err, src)
	}
	return L
}

func semNum(t *testing.T, L *State, name string) float64 {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	if !L.IsNumber(-1) {
		t.Fatalf("global %s is %s", name, TypeName(L.Type(-1)))
	}
	n, _ := L.ToNumber(-1)
	return n
}

func TestSemantics(t *testing.T) {
	L := semRun(t, `
		-- string <-> number coercion
		sc = "10" + 5
		sc2 = 10 .. ""
		sc3 = "3.5" * 2
		sc4 = #(12345 .. "")

		-- closures & upvalue sharing
		local function counter()
			local n = 0
			return function() n = n + 1; return n end
		end
		local c = counter()
		c(); c(); c()
		rcnt = c()

		-- upvalue shared between two closures
		local function pair()
			local v = 0
			return function(x) v = x end, function() return v end
		end
		local set, get = pair()
		set(99)
		rup = get()

		-- varargs
		local function sum(...)
			local t = {...}
			local s = 0
			for i = 1, #t do s = s + t[i] end
			return s, #t
		end
		local s1, n1 = sum(1, 2, 3, 4)
		rvsum = s1
		rvn = n1

		-- table varargs packing
		local function pack(...)
			local t = {...}
			return #t
		end
		rpack = pack(1, 2, 3)

		-- nested closures 3 deep
		local function a()
			local x = 1
			return function()
				local y = 2
				return function() return x + y end
			end
		end
		rnest3 = a()()()

		-- goto / labels / break
		local acc = 0
		for i = 1, 10 do
			if i % 2 == 1 then goto skip end
			acc = acc + i
			::skip::
		end
		rgoto2 = acc

		-- repeat with local scoping
		local k = 0
		repeat
			local j = 1
			k = k + j
		until k >= 3
		rrep = k

		-- and / or short circuit
		local side = 0
		local function f() side = side + 1; return true end
		local _ = false and f() -- not called
		rshort1 = side
		local _ = true and f() -- called
		rshort2 = side
		local _ = true or f() -- not called
		local _ = false or f() -- called
		rshort = side

		-- integer division / float
		ridiv = 7 // 2
		rfdiv = 7 / 2
		rint = 3.0 == 3

		-- string comparison & concat chain
		rcmp = ("a" < "b") and 1 or 0
		rchain = "a" .. "b" .. "c" .. 1 .. 2

		-- multiple assignment & swap
		local p, q = 1, 2
		p, q = q, p
		rswap = p * 10 + q

		-- self call
		local obj = {v = 5}
		function obj:get() return self.v end
		function obj:set(x) self.v = x; return self end
		obj:set(11)
		rself = obj:get()

		-- while with break
		local w = 0
		while true do w = w + 1; if w == 4 then break end end
		rbrk = w
	`)
	for name, want := range map[string]float64{
		"sc": 15, "sc3": 7, "sc4": 5,
		"rcnt": 4, "rup": 99,
		"rvsum": 10, "rvn": 4, "rpack": 3,
		"rnest3": 3,
		"rgoto2": 30, // 2+4+6+8+10
		"rrep":   3, "rshort": 2, "rshort1": 0, "rshort2": 1,
		"ridiv": 3, "rfdiv": 3.5,
		"rcmp": 1, "rswap": 21, "rself": 11, "rbrk": 4,
	} {
		if got := semNum(t, L, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	L.GetGlobal("sc2")
	if s, _ := L.ToString(-1); s != "10" {
		t.Errorf("sc2 = %q", s)
	}
	L.Pop(1)
	L.GetGlobal("rchain")
	if s, _ := L.ToString(-1); s != "abc12" {
		t.Errorf("rchain = %q", s)
	}
	L.Pop(1)
	L.GetGlobal("rint")
	if !L.ToBoolean(-1) {
		t.Error("3.0 == 3 should be true")
	}
	L.Pop(1)
}

func TestMetamethods(t *testing.T) {
	L := semRun(t, `
		local mt = {}
		mt.__index = function(t, k) return k * 100 end
		mt.__add = function(a, b) return (a.x or 0) + (b.x or 0) end
		mt.__eq = function(a, b) return a.x == b.x end
		mt.__lt = function(a, b) return a.x < b.x end
		mt.__tostring = function(a) return "obj(" .. a.x .. ")" end
		mt.__call = function(self, v) self.x = v; return v * 2 end
		mt.__len = function(a) return 42 end
		mt.__concat = function(a, b) return "cc" end

		local a = setmetatable({x = 1}, mt)
		local b = setmetatable({x = 2}, mt)

		ridx = a[7]
		radd = a + b
		req = (a == setmetatable({x = 1}, mt)) and 1 or 0
		rlt = (a < b) and 1 or 0
		rlen = #a
		rconcat = ("x" .. a)
		rcall = a(21)
		rafter = a.x
		rrawget = rawget(a, "x")
		rrawlen = rawlen(a)
		raweq = rawequal(a, a) and 1 or 0
		rgetmt = (getmetatable(a) == mt) and 1 or 0

		local prot = setmetatable({}, {__metatable = "locked"})
		rmtprot = (getmetatable(prot) == "locked") and 1 or 0

		-- __newindex
		local log = {}
		local c = setmetatable({}, {__newindex = function(t, k, v) log[#log + 1] = k .. "=" .. v end})
		c.foo = 1
		rnewidx = #log

		-- arithmetic on strings (metatable-less)
		rstrnum = "6" // "3"

		-- error + pcall
		local ok, err = pcall(function() error("boom", 0) end)
		rpcall = ok and 1 or 0
		perr = err

		local ok2, e2 = pcall(function() local t = nil; return t.x end)
		rpcall2 = ok2 and 1 or 0
		perr2 = e2

		-- select / type
		rtype = type(a)
		rselect = select("#", 1, 2, 3)
		rtonum = tonumber("0x10")
		rtonum2 = tonumber("abc") -- nil
	`)
	for name, want := range map[string]float64{
		"ridx": 700, "radd": 3, "req": 1, "rlt": 1, "rlen": 42,
		"rcall": 42, "rafter": 21, "rrawget": 21 /* a(21) set a.x = 21 via __call */, "rrawlen": 0,
		"raweq": 1, "rgetmt": 1, "rmtprot": 1, "rnewidx": 1,
		"rstrnum": 2, "rpcall": 0, "rselect": 3, "rtonum": 16,
	} {
		if got := semNum(t, L, name); got != want {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	L.GetGlobal("rconcat")
	if s, _ := L.ToString(-1); s != "cc" {
		t.Errorf("rconcat = %q", s)
	}
	L.Pop(1)
	L.GetGlobal("rtype")
	if s, _ := L.ToString(-1); s != "table" {
		t.Errorf("rtype = %q", s)
	}
	L.Pop(1)
	L.GetGlobal("perr")
	if s, _ := L.ToString(-1); s != "boom" {
		t.Errorf("perr = %q", s)
	}
	L.Pop(1)
	/* rpcall2 is 1 when pcall wrongly succeeded */
	if got := semNum(t, L, "rpcall2"); got != 0 {
		t.Errorf("indexing nil should fail, but pcall returned %v", got)
	}
	L.GetGlobal("rtonum2")
	if !L.IsNil(-1) {
		t.Errorf("tonumber('abc') should be nil, got %v", L.Type(-1))
	}
	L.Pop(1)
}

// TestVarinfoStopsAtANUL pins where an error message stops naming the
// constant it is about.
//
// PUC-Lua builds " (constant '...')" with printf's "%s", and C's "%s"
// stops at a NUL -- so the constant in '"a\0b" & 1' is named 'a' and
// nothing after it.  A Lua string can hold a NUL: only the message
// cannot.  The port was printing the whole thing, NUL and all, which is
// the one difference the differential fuzz found in 72 463 expressions.
//
// It is here rather than only in testdata/diff because the fuzz needs a
// reference interpreter beside it, and this does not.
func TestVarinfoStopsAtANUL(t *testing.T) {
	/* Only the bitwise operators name the constant: 5.4 says "attempt
	   to add a 'string' with a 'number'" for the arithmetic ones, and
	   an index or a call names something else again. */
	got := runChunk(t, `
	local out = {}
	local function say(...)
		out[#out + 1] = select(2, pcall(...))
	end
	say(function() return "a\0b" & 1 end)
	say(function() return "a\0b" | 1 end)
	say(function() return "a\0b" ~ 1 end)
	say(function() return "a\0b" << 1 end)
	say(function() return "a\0b" >> 1 end)
	say(function() return ~"a\0b" end)
	say(function() return "ab" & 1 end)
	say(function() return "\0" & 1 end)
	say(function() return "\0ab" & 1 end)
	say(function() return "abc\0def" & 1 end)
	return table.concat(out, "\n")
	`)
	want := `test:6: attempt to perform bitwise operation on a string value (constant 'a')
test:7: attempt to perform bitwise operation on a string value (constant 'a')
test:8: attempt to perform bitwise operation on a string value (constant 'a')
test:9: attempt to perform bitwise operation on a string value (constant 'a')
test:10: attempt to perform bitwise operation on a string value (constant 'a')
test:11: attempt to perform bitwise operation on a string value (constant 'a')
test:12: attempt to perform bitwise operation on a string value (constant 'ab')
test:13: attempt to perform bitwise operation on a string value (constant '')
test:14: attempt to perform bitwise operation on a string value (constant '')
test:15: attempt to perform bitwise operation on a string value (constant 'abc')`
	if got != want {
		t.Errorf("the constant in an error message:\n%s", sideBySide(want, got))
	}
}
