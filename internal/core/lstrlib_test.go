package core

import (
	"os"
	"path/filepath"
	"strings"
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
// Half of the last digit kept is 8, so a first dropped digit above 8
// rounds up and one below does not -- but a first dropped digit of
// exactly 8 is a tie, and whether a tie rounds up is decided by what
// comes after that digit and then by the digit it lands on.  Deciding
// from the first dropped digit alone is what made '%.8a' of 1e-300
// answer 0x1.56e1fc2fp-997 here and 0x1.56e1fc30p-997 in C: the digits
// are 56e1fc2f|8f359, the first dropped digit is a bare '8', and the
// tail behind it is not zero.
//
// None of these is a tie, on purpose: a tie is the one thing the C
// libraries disagree about, so it has a test of its own below.  The
// seventh value is e and not pi for the same reason -- pi at a
// precision of 12 is 0x1.921fb54442d1|8, which is exactly a tie.
func TestFormatHexFloatRounding(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	for _, x in ipairs{1e-300, 5e-324, 0.1, 1/3, 1e300, 2.718281828459045,
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
2.7182818284590451: 0x1p+1 0x1.6p+1 0x1.5bfp+1 0x1.5bf0a8b1p+1 0x1.5bf0a8b14577p+1 0x1.5bf0a8b145769p+1 0x1.5bf0a8b1457690000000p+1
2.2250738585072014e-308: 0x1p-1022 0x1.0p-1022 0x1.000p-1022 0x1.00000000p-1022 0x1.000000000000p-1022 0x1.0000000000000p-1022 0x1.00000000000000000000p-1022`
	if got != want {
		t.Errorf("%%a rounds differently:\n%s", sideBySide(want, got))
	}
}

// A tie is the one rounding the C libraries do differently, so what the
// port does about it is pinned against the reference rather than
// against a number, and the two cases are asked separately.
//
//	'even' -- 0.00034455349532716431, whose thirteen hexadecimal
//	digits are 694a5fe80c9e|8 -- is a tie that lands on an even digit.
//	Both rules leave it alone: to even is to stay, and down is to stay.
//	Every C library answers 0x1.694a5fe80c9ep-12, so this one is
//	compared like any other case.
//
//	'odd' -- 3.1851172729436392, 97b1ec3c9bdb|8 -- is a tie that lands
//	on an odd one, 'b'.  glibc hands that digit to 'round_away', which
//	rounds away from zero when it is odd, and answers ...bdc; Apple's
//	library rounds a tie down whatever the digit is, and answers ...bdb.
//	This port answers ...bdc -- what glibc answers, what the C library's
//	own '%f' answers on every platform ('%.0f' of 1.5 is 2), and what
//	makes '%a' agree with '%f' inside one call.  So what is asserted of
//	it is the rule: the digit it lands on is even.  It is only compared
//	to the reference when the reference was built against a library that
//	rounds a tie the same way.
func TestFormatHexFloatTieRounding(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to compare a %a tie")
	}
	const src = `local out = {}
	for _, x in ipairs{0.00034455349532716431, 3.1851172729436392,
	                   1.0, 2.0, 0.5} do
		out[#out + 1] = string.format("%.12a", x)
	end
	print(table.concat(out, "\n"))
	`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tie.lua"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSpace(fmtRunFile(t, ref, dir, "tie.lua")), "\n")
	got := strings.Split(strings.TrimSpace(fmtRunFile(t, diffBuild(t), dir, "tie.lua")), "\n")
	if len(want) != 5 || len(got) != 5 {
		t.Fatalf("the tie chunk did not answer five times:\nPUC-Lua: %q\nthis:    %q",
			want, got)
	}
	/* The even tie and the three powers of two, none of which rounds
	   at all, are the same everywhere. */
	for _, i := range []int{0, 2, 3, 4} {
		if want[i] != got[i] {
			t.Errorf("%%.12a of %s: PUC-Lua answers %s, this answers %s",
				[]string{"the even tie", "", "", "1.0", "2.0"}[i], want[i], got[i])
		}
	}
	mant := got[1]
	if i := strings.IndexByte(mant, 'p'); i >= 0 {
		mant = mant[:i]
	}
	if !strings.ContainsAny(mant[len(mant)-1:], "02468ace") {
		t.Errorf("%%.12a of 3.1851172729436392 is a tie on an odd digit, so it "+
			"has to land on an even one; this port answers %s", got[1])
	}
	if fmtRefTiesDown(t, ref) {
		/* Apple's library: the one case in the whole library where a
		   difference is the C library's and not this port's. */
		t.Logf("PUC-Lua answers %s and this port %s: a %%a tie on an odd digit, "+
			"which the reference's C library rounds down and this port rounds "+
			"to even, the way glibc and that library's own '%%f' do", want[1], got[1])
	} else if want[1] != got[1] {
		t.Errorf("%%.12a of 3.1851172729436392: PUC-Lua answers %s, this answers %s",
			want[1], got[1])
	}
}

// A subnormal is the other thing the C libraries spell differently, and
// unlike the tie the standard lets them: of a normalised float the
// leading hexadecimal digit is 1, and of one too small to have a 1 it is
// unspecified. glibc keeps the exponent of the smallest normal float and
// writes the raw 52-bit fraction after the point, so 5e-324 is
// 0x0.0000000000001p-1022; Apple's library normalises it, and so does
// this port, for which it is 0x1p-1074 -- what Go's own hexadecimal
// conversion writes, and one number rather than two spellings of one.
//
// '%q' writes a float the same way, which is why it is asked here too:
// it is the conversion a program uses to write a number back out.
//
// What is asserted is therefore the rule -- every answer this port gives
// starts with "0x1" -- and the values are only compared when the
// reference's library normalises as well.
func TestFormatSubnormalIsNormalised(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to compare a subnormal")
	}
	const src = `local out = {}
	-- the ends of the subnormal range, and the smallest normal float
	for _, x in ipairs{5e-324, 1e-323, 1e-320, 1e-308,
	                   2.2250738585072014e-308 / 2,
	                   2.2250738585072014e-308} do
	  out[#out + 1] = string.format("%a", x) .. " " .. string.format("%q", x)
	end
	print(table.concat(out, "\n"))
	`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sub.lua"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	want := strings.Split(strings.TrimSpace(fmtRunFile(t, ref, dir, "sub.lua")), "\n")
	got := strings.Split(strings.TrimSpace(fmtRunFile(t, diffBuild(t), dir, "sub.lua")), "\n")
	if len(want) != 6 || len(got) != 6 {
		t.Fatalf("the subnormal chunk did not answer six times:\nPUC-Lua: %q\nthis:    %q",
			want, got)
	}
	for i, line := range got {
		for _, field := range strings.Fields(line) {
			if !strings.HasPrefix(field, "0x1") {
				t.Errorf("a subnormal is normalised here, so every hexadecimal "+
					"answer starts with 0x1; this port answers %q on line %d", field, i+1)
			}
		}
	}
	if fmtRefNormalisesSubnormals(t, ref) {
		for i := range want {
			if want[i] != got[i] {
				t.Errorf("subnormal %d: PUC-Lua answers %s, this answers %s",
					i+1, want[i], got[i])
			}
		}
		return
	}
	/* glibc: the second case in the library where a difference is the C
	   library's and not this port's. */
	t.Logf("PUC-Lua answers %q and this port %q: a subnormal, whose leading "+
		"hexadecimal digit C leaves the library to choose", want[0], got[0])
}
