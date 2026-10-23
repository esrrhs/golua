package core

import "testing"

// Metamethod gaps: __ipairs (5.3 LUA_COMPAT_IPAIRS), and focused
// arithmetic/bitwise TM asserts (differential 67_meta / official events
// cover exhaustively — unit layer previously relied on them alone).

func TestLangFeatIPairsMetamethod(t *testing.T) {
	confRun(t, `
		-- Lua 5.3 with LUA_COMPAT_IPAIRS: ipairs looks up __ipairs
		local seen = {}
		local obj = setmetatable({10, 20, 30}, {
			__ipairs = function(t)
				seen.called = true
				return function(_, i)
					i = i + 1
					if i <= 2 then return i, t[i] * 10 end
				end, t, 0
			end,
		})
		local acc = {}
		for i, v in ipairs(obj) do
			acc[#acc + 1] = i .. ":" .. v
		end
		assert(seen.called)
		assert(table.concat(acc, ",") == "1:100,2:200")

		-- without __ipairs, default integer traversal
		local plain = {7, 8}
		local p = {}
		for i, v in ipairs(plain) do p[#p + 1] = i * v end
		assert(p[1] == 7 and p[2] == 16)

		-- __pairs still used by pairs
		local pobj = setmetatable({}, {
			__pairs = function()
				return function(_, k)
					if k == nil then return "k", "v" end
				end
			end,
		})
		local pk, pv = pairs(pobj)(pobj, nil)
		assert(pk == "k" and pv == "v")
	`)
}

func TestLangFeatArithmeticBitwiseTM(t *testing.T) {
	confRun(t, `
		local function box(v)
			return setmetatable({v = v}, {
				__add  = function(a, b) return box((type(a)=="table" and a.v or a) + (type(b)=="table" and b.v or b)) end,
				__sub  = function(a, b) return box((type(a)=="table" and a.v or a) - (type(b)=="table" and b.v or b)) end,
				__mul  = function(a, b) return box((type(a)=="table" and a.v or a) * (type(b)=="table" and b.v or b)) end,
				__div  = function(a, b) return box((type(a)=="table" and a.v or a) / (type(b)=="table" and b.v or b)) end,
				__idiv = function(a, b) return box((type(a)=="table" and a.v or a) // (type(b)=="table" and b.v or b)) end,
				__mod  = function(a, b) return box((type(a)=="table" and a.v or a) % (type(b)=="table" and b.v or b)) end,
				__pow  = function(a, b) return box((type(a)=="table" and a.v or a) ^ (type(b)=="table" and b.v or b)) end,
				__unm  = function(a) return box(-(a.v)) end,
				__band = function(a, b) return box((type(a)=="table" and a.v or a) & (type(b)=="table" and b.v or b)) end,
				__bor  = function(a, b) return box((type(a)=="table" and a.v or a) | (type(b)=="table" and b.v or b)) end,
				__bxor = function(a, b) return box((type(a)=="table" and a.v or a) ~ (type(b)=="table" and b.v or b)) end,
				__bnot = function(a) return box(~(a.v)) end,
				__shl  = function(a, b) return box((type(a)=="table" and a.v or a) << (type(b)=="table" and b.v or b)) end,
				__shr  = function(a, b) return box((type(a)=="table" and a.v or a) >> (type(b)=="table" and b.v or b)) end,
				__eq   = function(a, b) return a.v == (type(b)=="table" and b.v or b) end,
				__tostring = function(a) return "box:" .. tostring(a.v) end,
			})
		end

		local a, b = box(10), box(3)
		assert((a + b).v == 13)
		assert((a - 1).v == 9)
		assert((2 * b).v == 6)
		assert((a / b).v == 10 / 3)
		assert((a // b).v == 3)
		assert((a % b).v == 1)
		assert((box(2) ^ box(5)).v == 32)
		assert((-a).v == -10)

		assert((a & b).v == 2)
		assert((a | b).v == 11)
		assert((a ~ b).v == 9)
		assert((~box(0)).v == -1)
		assert((box(1) << box(4)).v == 16)
		assert((box(16) >> box(2)).v == 4)

		-- missing TM errors
		local bare = setmetatable({}, {})
		assert(not pcall(function() return bare + 1 end))
		assert(not pcall(function() return bare & 1 end))
		assert(not pcall(function() return ~bare end))
	`)
}
