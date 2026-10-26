package core

import "testing"

// Operator precedence, short-circuit, and unary combinations.
// Smoke/conformance cover individual ops; this pins the precedence table
// and and/or side-effect order that unit tests lacked.

func TestLangFeatOperatorPrecedence(t *testing.T) {
	confRun(t, `
		-- ^ is right-associative and binds tightest among arith
		assert(2 ^ 3 ^ 2 == 512)       -- 2^(3^2), not (2^3)^2
		assert(2 ^ 2 * 3 == 12)
		assert(-2 ^ 2 == -4)           -- unary minus after ^
		assert(2 + 3 * 4 == 14)
		assert(2 * 3 + 4 == 10)
		assert(10 - 3 - 2 == 5)        -- left-assoc
		assert(20 / 4 / 2 == 2.5)
		assert(7 // 2 * 3 == 9)
		assert(2 + 3 // 2 == 3)
		assert(10 % 4 + 1 == 3)

		-- bitwise vs arith: arith binds tighter (Lua 5.3)
		assert(1 << 2 + 1 == 8)        -- 1<<(2+1)
		assert(1 + 2 << 1 == 6)        -- (1+2)<<1
		-- relational binds looser than bitwise: (0xF & (0x3+1)) == 4
		assert(0xF & 0x3 + 1 == 4)
		assert(1 | 2 & 4 == 1)         -- & tighter than | : 1|(2&4)=1|0=1
		assert(7 ~ 3 & 1 == 6)         -- 7~(3&1)=7~1=6

		-- relational vs concat / arith
		assert(1 + 2 < 4)
		assert(not (1 + 2 > 4))
		assert("a" .. "b" == "ab")
		assert(1 .. 2 == "12")

		-- logical lowest; and/or return operands
		assert((1 + 1 == 2 and 3 or 4) == 3)
		assert((false and 3 or 4) == 4)
		assert((nil or false or 5) == 5)
	`)
}

func TestLangFeatShortCircuit(t *testing.T) {
	confRun(t, `
		local log = {}
		local function tap(name, v)
			log[#log + 1] = name
			return v
		end

		-- and: stop on first falsy
		local r = tap("a", true) and tap("b", false) and tap("c", true)
		assert(r == false)
		assert(table.concat(log, ",") == "a,b")

		log = {}
		r = tap("a", 1) and tap("b", 2) and tap("c", 3)
		assert(r == 3 and table.concat(log, ",") == "a,b,c")

		-- or: stop on first truthy
		log = {}
		r = tap("a", nil) or tap("b", false) or tap("c", "ok")
		assert(r == "ok" and table.concat(log, ",") == "a,b,c")

		log = {}
		r = tap("a", 0) or tap("b", 9)  -- 0 is truthy in Lua
		assert(r == 0 and table.concat(log, ",") == "a")

		-- not
		assert(not nil and not false)
		assert(not not 1)
		assert(not (1 and false))
	`)
}

func TestLangFeatUnaryMix(t *testing.T) {
	confRun(t, `
		assert(~~5 == 5)
		assert(- -3 == 3)
		assert(not not true)
		assert(#{1,2,3} == 3)
		local t = setmetatable({}, {__len = function() return 7 end, __unm = function() return 11 end, __bnot = function() return 13 end})
		assert(#t == 7 and -t == 11 and ~t == 13)
	`)
}
