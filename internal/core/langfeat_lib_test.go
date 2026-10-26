package core

import "testing"

// Light stdlib pins for gaps not asserted in conformance/baselib.
// Avoid GC; avoid duplicating heavy official/differential coverage.

func TestLangFeatUTF8Charpattern(t *testing.T) {
	confRun(t, `
		assert(type(utf8.charpattern) == "string")
		assert(#utf8.charpattern > 0)
		-- matches one UTF-8 char
		local s = "a\u{20AC}b"
		local i = 1
		local parts = {}
		while i <= #s do
			local a, b = string.find(s, utf8.charpattern, i)
			assert(a == i)
			parts[#parts + 1] = s:sub(a, b)
			i = b + 1
		end
		assert(#parts == 3)
		assert(parts[1] == "a" and parts[3] == "b")
		assert(utf8.codepoint(parts[2]) == 0x20AC)
	`)
}

func TestLangFeatBit32Smoke(t *testing.T) {
	// always-on in golua (FLAG); covergap already hits API — one assert pin
	confRun(t, `
		assert(bit32 ~= nil)
		assert(bit32.band(0xFF, 0x0F) == 0x0F)
		assert(bit32.bor(1, 2, 4) == 7)
		assert(bit32.bxor(0xFF, 0xF0) == 0x0F)
		assert(bit32.bnot(0) == 0xFFFFFFFF)
		assert(bit32.lshift(1, 3) == 8)
		assert(bit32.rshift(8, 2) == 2)
		assert(bit32.extract(0xF0, 4, 4) == 0xF)
		assert(bit32.replace(0, 0xA, 4, 4) == 0xA0)
	`)
}

func TestLangFeatSelectUnpackVersion(t *testing.T) {
	confRun(t, `
		assert(_VERSION == "Lua 5.3")
		assert(select("#", 1, nil, 3) == 3)
		assert(table.unpack({9, 8, 7}, 2, 3) == 8) -- first return only in assert? 
		local a, b = table.unpack({9, 8, 7}, 2, 3)
		assert(a == 8 and b == 7)
		-- unpack alias on base (5.3 compat / table.unpack)
		if unpack then
			local x, y = unpack({1, 2})
			assert(x == 1 and y == 2)
		end
	`)
}
