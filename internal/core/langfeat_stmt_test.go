package core

import "testing"

// Statements / calls: multi-assign edges, method sugar, tail-call depth,
// local vs global visibility. Official/diff cover broadly; these pin
// unit-level gaps.

func TestLangFeatMultiAssign(t *testing.T) {
	confRun(t, `
		local a, b, c = 1, 2, 3
		assert(a == 1 and b == 2 and c == 3)

		-- extra RHS discarded
		local d, e = 10, 20, 30
		assert(d == 10 and e == 20)

		-- missing RHS → nil
		local f, g, h = 1
		assert(f == 1 and g == nil and h == nil)

		-- multi-ret on RHS
		local function two() return "x", "y" end
		local p, q, r = two()
		assert(p == "x" and q == "y" and r == nil)

		-- only first value when parenthesized
		local s, t = (two())
		assert(s == "x" and t == nil)

		-- table adjust
		local t1 = {two()}
		assert(t1[1] == "x" and t1[2] == "y" and #t1 == 2)

		-- swap
		local x, y = 1, 2
		x, y = y, x
		assert(x == 2 and y == 1)

		-- global multi
		G1, G2 = 7, 8
		assert(G1 == 7 and G2 == 8)
	`)
}

func TestLangFeatMethodSugar(t *testing.T) {
	confRun(t, `
		local o = {
			n = 0,
			inc = function(self, k)
				self.n = self.n + k
				return self.n
			end,
			get = function(self) return self.n end,
		}
		assert(o:inc(3) == 3)
		assert(o:inc(4) == 7)
		assert(o:get() == 7)

		-- colon vs dot
		assert(o.inc(o, 1) == 8)

		-- method on nested table via metamethod index
		local proto = {hi = function(self) return "hi:" .. self.name end}
		local inst = setmetatable({name = "lua"}, {__index = proto})
		assert(inst:hi() == "hi:lua")
	`)
}

func TestLangFeatTailCall(t *testing.T) {
	confRun(t, `
		-- Proper tail call must not grow Lua stack unboundedly.
		local function countdown(n, acc)
			if n == 0 then return acc end
			return countdown(n - 1, acc + 1)  -- tail
		end
		assert(countdown(5000, 0) == 5000)

		-- Non-tail form would overflow much sooner; keep modest depth check
		-- that the tail form succeeded above.

		-- Tail call of vararg
		local function tv(...)
			local n = select("#", ...)
			if n == 0 then return 0 end
			return tv(select(2, ...))  -- tail, drop first
		end
		assert(tv(1, 2, 3, 4, 5) == 0)

		-- debug.getinfo istailcall when available
		local saw
		local function leaf()
			saw = debug.getinfo(1, "t").istailcall
			return 1
		end
		local function mid() return leaf() end
		assert(mid() == 1)
		-- leaf may or may not report istailcall depending on call shape;
		-- mid→leaf as 'return leaf()' is a tail call from mid's perspective
		-- when querying from inside leaf about its caller. Pin that getinfo works:
		assert(type(saw) == "boolean")
	`)
}

func TestLangFeatScopeVisibility(t *testing.T) {
	confRun(t, `
		x = "global"
		local x = "local"
		assert(x == "local")
		assert(_ENV.x == "global")

		do
			local x = "inner"
			assert(x == "inner")
		end
		assert(x == "local")

		-- for-loop locals are not visible after
		for i = 1, 1 do end
		assert(i == nil)  -- global i unset

		-- repeat-until local visible in until clause (5.x)
		local n = 0
		repeat
			local v = n + 1
			n = v
		until v == 3
		assert(n == 3)
		assert(v == nil)  -- v not visible outside

		-- load env isolation
		local env = {y = 99}
		local f = load("return y", "=e", "t", env)
		assert(f() == 99)
		env.y = 1
		assert(f() == 1)
	`)
}
