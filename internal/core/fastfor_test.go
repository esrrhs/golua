package core

import "testing"

// The straight-line numeric 'for' runs on a register file instead of the
// dispatch loop. The same loop with an upvalue assignment cannot: that
// assignment is the slow path, and the two have to agree, including on
// the loops the register file refuses (the accumulator is closed over,
// a metamethod, a break).
func TestFastNumericFor(t *testing.T) {
	got := runChunk(t, `
	local sink
	local function both(fast, slow)
		if fast ~= slow then
			error("mismatch " .. tostring(fast) .. " vs " .. tostring(slow), 0)
		end
		return fast
	end

	local function isum(n)
		local s = 0
		for i = 1, n do s = s + i end
		return s
	end
	local function isumSlow(n)
		local s = 0
		for i = 1, n do s = s + i; sink = i end
		return s
	end

	local function fsum(n)
		local s = 0.0
		for i = 1, n do s = s + i * 1.5 end
		return s
	end
	local function fsumSlow(n)
		local s = 0.0
		for i = 1, n do s = s + i * 1.5; sink = i end
		return s
	end

	local function bsum(n)
		local s = 0
		for i = 1, n do s = s + ((i & 255) ~ (i << 1)) end
		return s
	end
	local function bsumSlow(n)
		local s = 0
		for i = 1, n do s = s + ((i & 255) ~ (i << 1)); sink = i end
		return s
	end

	local function down()
		local s = 0
		for i = 10, 1, -3 do s = s + i end
		return s
	end
	local function downSlow()
		local s = 0
		for i = 10, 1, -3 do s = s + i; sink = i end
		return s
	end

	local function half()
		local s = 0
		for i = 3, 1, -1.5 do s = s + i end
		return s
	end
	local function halfSlow()
		local s = 0
		for i = 3, 1, -1.5 do s = s + i; sink = i end
		return s
	end

	local function addi(n)
		local s = 0
		for i = 1, n do s = s + 1 end
		return s
	end
	local function addiSlow(n)
		local s = 0
		for i = 1, n do s = s + 1; sink = i end
		return s
	end

	local function imul()
		local s = 0
		for i = 1, 20 do s = s + i * 3 end
		return s
	end
	local function imulSlow()
		local s = 0
		for i = 1, 20 do s = s + i * 3; sink = i end
		return s
	end

	local function wrap()
		local s = math.maxinteger - 1
		for i = 1, 3 do s = s + i end
		return s
	end
	local function wrapSlow()
		local s = math.maxinteger - 1
		for i = 1, 3 do s = s + i; sink = i end
		return s
	end

	local function empty(n)
		for i = 1, n do end
		return n
	end

	local function closed()
		local s = 0
		local function see() return s end
		for i = 1, 100 do s = s + i end
		return s == 5050 and see() == 5050
	end

	local function brk()
		local s = 0
		for i = 1, 100 do
			s = s + i
			if i >= 10 then break end
		end
		return s
	end

	local function nanstep()
		local n = 0
		for i = 1, 1, 0/0 do n = n + 1 end
		return n
	end

	local function beside()
		local t = {7, 8, 9}
		local s = 0.0
		for i = 1, 10 do s = s + i * 1.5 end
		return t[1] + t[3] + s
	end

	local out = {
		both(isum(1000), isumSlow(1000)),
		both(fsum(1000), fsumSlow(1000)),
		both(bsum(1000), bsumSlow(1000)),
		both(down(), downSlow()),
		both(half(), halfSlow()),
		both(addi(50), addiSlow(50)),
		both(imul(), imulSlow()),
		both(wrap(), wrapSlow()),
		empty(4),
		closed(),
		brk(),
		nanstep(),
		beside(),
	}
	local s = {}
	for i = 1, #out do s[i] = tostring(out[i]) end
	return table.concat(s, "\n")
	`)
	want := `500500
750750.0
1006212
22
4.5
50
630
-9223372036854775804
4
true
55
1
98.5`
	if got != want {
		t.Errorf("fast numeric for:\n%s", sideBySide(want, got))
	}
}

func TestFastCountTail(t *testing.T) {
	got := runChunk(t, `
	local function tail(n, acc)
		if n == 0 then return acc end
		return tail(n - 1, acc + 1)
	end
	local function step2(n, acc)
		if n == 0 then return acc end
		return step2(n - 1, acc + 2)
	end
	local function ftail(n, acc)
		if n == 0 then return acc end
		return ftail(n - 1.0, acc + 1)
	end
	return tostring(tail(100, 5)) .. " " .. tostring(tail(0, 7)) .. " " ..
		tostring(tail(1, 0)) .. " " .. tostring(step2(5, 0)) .. " " .. tostring(ftail(3, 1))
	`)
	if got != "105 7 1 10 4" {
		t.Errorf("tail countdown: %q", got)
	}
}

func TestFastNumericLeafCall(t *testing.T) {
	got := runChunk(t, `
	local function add3(a, b, c)
		return a + b + c
	end
	local mt = {__add = function(a, b)
		return (type(a) == "table" and a.n or a) +
		       (type(b) == "table" and b.n or b)
	end}
	local x = setmetatable({n = 10}, mt)
	return table.concat({
		tostring(add3(1, 2, 3)),
		tostring(add3(1.5, 2, 3)),
		tostring(add3("4", "5", "6")),
		tostring(add3(x, 2, 3)),
	}, " ")
	`)
	if got != "6 6.5 15 15" {
		t.Errorf("numeric leaf call: %q", got)
	}
}

func TestFastDenseArrayFor(t *testing.T) {
	got := runChunk(t, `
	local n = 10000
	local t = {}
	for i = 1, n do t[i] = i end
	local s = 0
	for pass = 1, 3 do
		for i = 1, n do s = s + t[i] end
	end
	return tostring(#t) .. " " .. tostring(t[1]) .. " " ..
	       tostring(t[n]) .. " " .. tostring(s)
	`)
	if got != "10000 1 10000 150015000" {
		t.Errorf("dense array for: %q", got)
	}
}

func TestFastAllocFor(t *testing.T) {
	got := runChunk(t, `
		local function alloc(n)
			local s = 0
			for i = 1, n do
				local t = { i, i + 1, i + 2, i + 3 }
				s = s + t[1]
			end
			return s
		end
		local t
		local s = 0
		for i = 1, 5 do
			t = { i, i + 1, i + 2, i + 3 }
			s = s + t[1]
		end
		local down = 0
		for i = 10, 1, -3 do
			local u = { i, i + 1, i + 2, i + 3 }
			down = down + u[1]
		end
		local fs = 0.5
		for i = 1, 4 do
			local u = { i, i + 1, i + 2, i + 3 }
			fs = fs + u[1]
		end
		return tostring(alloc(10000)) .. " " ..
			t[1] .. "," .. t[2] .. "," .. t[3] .. "," .. t[4] .. "," .. s .. " " ..
			tostring(down) .. " " .. tostring(fs)
	`)
	if got != "50005000 5,6,7,8,15 22 10.5" {
		t.Errorf("alloc for: %q", got)
	}
}

func TestFastYieldFor(t *testing.T) {
	got := runChunk(t, `
		local n = 100
		local function producer()
			for i = 1, n do coroutine.yield(i) end
		end
		local co = coroutine.wrap(producer)
		local s = 0
		for _ = 1, n do s = s + co() end
		local ok, err = pcall(co)
		local ok2, err2 = pcall(co)

		local function shortp()
			for i = 1, 5 do coroutine.yield(i) end
		end
		local co2 = coroutine.wrap(shortp)
		local s2 = 0
		for _ = 1, 3 do s2 = s2 + co2() end
		return tostring(s) .. " " .. tostring(ok) .. ":" .. tostring(err) .. " " ..
			tostring(ok2) .. ":" .. tostring(err2) .. " " ..
			tostring(s2) .. " " .. tostring(co2())
	`)
	if got != "5050 true:nil false:cannot resume dead coroutine 6 4" {
		t.Errorf("yield for: %q", got)
	}
}

func TestFastConcatFor(t *testing.T) {
	got := runChunk(t, `
		local a, b = "hello", "world"
		local s
		for _ = 1, 100 do s = a .. b end
		local s2 = ""
		for _ = 1, 3 do s2 = s2 .. "x" end
		local empty = ""
		local s3
		for _ = 1, 2 do s3 = a .. empty end
		return s .. " " .. tostring(#s) .. " " .. s2 .. " " .. s3
	`)
	if got != "helloworld 10 xxx hello" {
		t.Errorf("concat for: %q", got)
	}
}

func TestFastGlobalFor(t *testing.T) {
	got := runChunk(t, `
	_G.fast_global_for_value = 0
	for i = 1, 10000 do
		_G.fast_global_for_value = _G.fast_global_for_value + i
	end
	local r = _G.fast_global_for_value
	_G.fast_global_for_value = nil
	return tostring(r)
	`)
	if got != "50005000" {
		t.Errorf("global for: %q", got)
	}
}

func TestFastDirectFib(t *testing.T) {
	got := runChunk(t, `
	local function fib(n)
		if n < 2 then return n end
		return fib(n - 1) + fib(n - 2)
	end
	return tostring(fib(0)) .. " " .. tostring(fib(1)) .. " " ..
	       tostring(fib(10)) .. " " .. tostring(fib(30))
	`)
	if got != "0 1 55 832040" {
		t.Errorf("direct fib: %q", got)
	}
}
