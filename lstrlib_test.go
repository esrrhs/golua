package lua

import (
	"testing"
)

// TestFormatChecksTheValueBeforeTheConversion pins the order in which
// lstrlib.c's 'str_format' does its two checks, because the order is
// visible and it was backwards here.
//
// For 'd', 'i', 'u', 'o', 'x' and 'X' -- C's 'intcase' -- and for
// 'e', 'E', 'f', 'g' and 'G', the argument is read *first*:
//
//	lua_Integer n = luaL_checkinteger(L, arg);
//	checkformat(L, form, flags, 1);
//
// so a value that is not an integer is complained about even when the
// specification is itself invalid.  '%#d' with 1.5 answers "number
// has no integer representation", and only '%#d' with a real integer
// answers "invalid conversion specification".  Here the specification
// was being judged first, so every one of those reported the wrong
// complaint.
//
// For 'c' the order is the other way round -- 'checkformat' comes
// before 'luaL_checkinteger' -- so '%#c' with a string complains about
// the specification and never about the string.  That case is in the
// expected block too, so that fixing one order cannot swap the other.
func TestFormatChecksTheValueBeforeTheConversion(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	-- each pair is a value and what to call it in the answer
	local vals = {{1.5, "1.5"}, {1, "1"}, {true, "true"}, {{}, "a table"},
	              {"x", '"x"'}}
	local function say(form, v, name)
		local ok, e = pcall(string.format, form, v)
		out[#out + 1] = form .. "|" .. name .. "|" .. tostring(e)
	end
	for _, form in ipairs{"%#d", "%#i", "%#u", "%#o", "%+x", "%+X", "%+u",
	                      "%+o"} do
		say(form, 1.5, "1.5")
		say(form, 1, "1")
	end
	for _, form in ipairs{"%#d", "%+f", "%#f"} do
		for _, pair in ipairs(vals) do say(form, pair[1], pair[2]) end
	end
	-- 'c' judges the specification first: every value gets the same
	-- complaint, including one that is not a number at all
	for _, pair in ipairs(vals) do say("%#c", pair[1], pair[2]) end
	return table.concat(out, "\n")
	`)
	want := `%#d|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%#d|1|invalid conversion specification: '%#d'
%#i|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%#i|1|invalid conversion specification: '%#i'
%#u|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%#u|1|invalid conversion specification: '%#u'
%#o|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%#o|1|01
%+x|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%+x|1|invalid conversion specification: '%+x'
%+X|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%+X|1|invalid conversion specification: '%+X'
%+u|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%+u|1|invalid conversion specification: '%+u'
%+o|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%+o|1|invalid conversion specification: '%+o'
%#d|1.5|bad argument #2 to 'string.format' (number has no integer representation)
%#d|1|invalid conversion specification: '%#d'
%#d|true|bad argument #2 to 'string.format' (number expected, got boolean)
%#d|a table|bad argument #2 to 'string.format' (number expected, got table)
%#d|"x"|bad argument #2 to 'string.format' (number expected, got string)
%+f|1.5|+1.500000
%+f|1|+1.000000
%+f|true|bad argument #2 to 'string.format' (number expected, got boolean)
%+f|a table|bad argument #2 to 'string.format' (number expected, got table)
%+f|"x"|bad argument #2 to 'string.format' (number expected, got string)
%#f|1.5|1.500000
%#f|1|1.000000
%#f|true|bad argument #2 to 'string.format' (number expected, got boolean)
%#f|a table|bad argument #2 to 'string.format' (number expected, got table)
%#f|"x"|bad argument #2 to 'string.format' (number expected, got string)
%#c|1.5|invalid conversion specification: '%#c'
%#c|1|invalid conversion specification: '%#c'
%#c|true|invalid conversion specification: '%#c'
%#c|a table|invalid conversion specification: '%#c'
%#c|"x"|invalid conversion specification: '%#c'`
	if got != want {
		t.Errorf("string.format checks in the wrong order:\n%s", sideBySide(want, got))
	}
}

// TestFormatHexFloatRounding pins '%a' at every precision for a handful
// of values, because rounding a hexadecimal fraction is decided by more
// than the first digit that is dropped.
//
// Half of the last digit kept is 8, so a first dropped digit of 9 or
// more rounds up and one of 7 or less does not -- but a first dropped
// digit of exactly 8 is a tie, and the C library is observed to round
// a tie *down*: '%.12a' of a value whose thirteenth digit is '8' keeps
// the digit it had.  Deciding from the first dropped digit alone is
// what made '%.8a' of 1e-300 answer 0x1.56e1fc2fp-997 here and
// 0x1.56e1fc30p-997 in C: the digits are 56e1fc2f|8f359, the first
// dropped digit is a bare '8', and the tail behind it is not zero.
func TestFormatHexFloatRounding(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	for _, x in ipairs{1e-300, 5e-324, 0.1, 1/3, 1e300, math.pi,
	                   2.2250738585072014e-308} do
		local ds = {}
		for _, p in ipairs{0, 1, 3, 8, 12, 13, 20} do
			ds[#ds + 1] = string.format("%." .. p .. "a", x)
		end
		out[#out + 1] = string.format("%.17g: %s", x, table.concat(ds, " "))
	end
	return table.concat(out, "\n")
	`)
	want := `1e-300: 0x1p-997 0x1.5p-997 0x1.56ep-997 0x1.56e1fc30p-997 0x1.56e1fc2f8f36p-997 0x1.56e1fc2f8f359p-997 0x1.56e1fc2f8f3590000000p-997
4.9406564584124654e-324: 0x1p-1074 0x1.0p-1074 0x1.000p-1074 0x1.00000000p-1074 0x1.000000000000p-1074 0x1.0000000000000p-1074 0x1.00000000000000000000p-1074
0.10000000000000001: 0x2p-4 0x1.ap-4 0x1.99ap-4 0x1.9999999ap-4 0x1.99999999999ap-4 0x1.999999999999ap-4 0x1.999999999999a0000000p-4
0.33333333333333331: 0x1p-2 0x1.5p-2 0x1.555p-2 0x1.55555555p-2 0x1.555555555555p-2 0x1.5555555555555p-2 0x1.55555555555550000000p-2
1.0000000000000001e+300: 0x1p+996 0x1.8p+996 0x1.7e4p+996 0x1.7e43c880p+996 0x1.7e43c880075ap+996 0x1.7e43c8800759cp+996 0x1.7e43c8800759c0000000p+996
3.1415926535897931: 0x2p+1 0x1.9p+1 0x1.922p+1 0x1.921fb544p+1 0x1.921fb54442d1p+1 0x1.921fb54442d18p+1 0x1.921fb54442d180000000p+1
2.2250738585072014e-308: 0x1p-1022 0x1.0p-1022 0x1.000p-1022 0x1.00000000p-1022 0x1.000000000000p-1022 0x1.0000000000000p-1022 0x1.00000000000000000000p-1022`
	if got != want {
		t.Errorf("%%a rounds differently:\n%s", sideBySide(want, got))
	}
}

// A tie rounds down, which is the half of the rule the test above can
// only see if a value whose thirteenth digit is exactly 8 is asked
// for.  The first three were found by searching random doubles for a
// thirteenth digit of exactly 8 and nothing but zeros behind it; the
// last three are powers of two, whose thirteenth digit is a 0 and which
// therefore must not be touched by whatever makes the others round.
func TestFormatHexFloatRoundsATieDown(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	for _, x in ipairs{-1.989032661366619e+244, -1.3701135617873088e-206,
	                   1.0613447212142254e-202, 1.0, 2.0, 0.5} do
		out[#out + 1] = string.format("%.12a", x)
	end
	return table.concat(out, "\n")
	`)
	want := `-0x1.74de452e6b43p+811
-0x1.1985d5d9dc9fp-684
0x1.0a35a099950dp-671
0x1.000000000000p+0
0x1.000000000000p+1
0x1.000000000000p-1`
	if got != want {
		t.Errorf("%%a rounds a tie differently:\n%s", sideBySide(want, got))
	}
}
