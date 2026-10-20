package core

import (
	"testing"
)

// TestStr2dRejectsWhatCRejects pins the three things 'l_str2d' was
// accepting that C's strtod never has, all of them found by the number
// fuzz and all of them a Go detail showing through:
//
//	underscores, because Go's ParseFloat takes the ones a Go literal may
//	have -- '1_000', '1_0.5', '1e1_0', '0x_1p0' -- and C's strtod has
//	never heard of them;
//
//	the Unicode spaces, because strings.TrimSpace trims U+00A0, U+2003
//	and U+3000 and C's isspace does not, so 'tonumber("\u{3000}1")' was
//	1 here and nil in PUC-Lua;
//
//	and both of those again on the hexadecimal path, which is a second
//	parser -- the one that takes '0xAA.0', the numerals Go's ParseFloat
//	will not take without an exponent -- and had its own copy of the
//	Unicode-space trim.
//
// The six bytes C calls space still are space, at either end, because
// that is what l_str2d does when it walks past the end of the
// conversion: 'tonumber(" 1 ")' is 1.  There are three answers per
// string, and the third is what the *lexer* makes of it, which is a
// different scanner: a numeral 'tonumber' reads is not necessarily one
// 'load' accepts -- with a base, 'tonumber' does not even skip the
// spaces it skips without one.
func TestStr2dRejectsWhatCRejects(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	-- '%q' cannot be used to print these: it escapes a newline as a
	-- backslash followed by a real one, and several of them have one
	local function q(s)
	  return '"' .. s:gsub("[%c\"\\]", function(c)
	    return string.format("\\%03d", c:byte())
	  end) .. '"'
	end
	local strs = {
	  "1_000", "1_0.5", "1.5_0", "1e1_0", "0x_1p0", "0x1_p0", "_1", "1_",
	  "1__0", "0x_1", "1_000_",
	  "\u{00a0}1", "\u{2003}1", "\u{3000}1", "1\u{00a0}", "\u{00a0}1 ",
	  "0x10\u{00a0}", "0x10\u{2003}", "0x10\u{3000}", "\u{00a0}0x10",
	  -- and what is still a number, spaces and all
	  " 1", "1 ", " 1 ", "\t1\n", "\v1\f", "\r1", "  0x10  ", "\t\n\r\v\f 1 \t\n\r\v\f",
	}
	for _, s in ipairs(strs) do
	  -- raw, and not q(s): what is being asked is what the lexer makes
	  -- of the numeral, and q(s) would make it a string literal instead
	  local src = "return " .. s
	  local f = load(src, "=c")
	  out[#out + 1] = q(s) .. "|" .. tostring(tonumber(s)) .. "|" ..
	    tostring(tonumber(s, 16)) .. "|" .. (f and "ok" or "no")
	end
	return table.concat(out, "\n")
	`)
	want := `"1_000"|nil|nil|no
"1_0.5"|nil|nil|no
"1.5_0"|nil|nil|no
"1e1_0"|nil|nil|no
"0x_1p0"|nil|nil|no
"0x1_p0"|nil|nil|no
"_1"|nil|nil|ok
"1_"|nil|nil|no
"1__0"|nil|nil|no
"0x_1"|nil|nil|no
"1_000_"|nil|nil|no
" 1"|nil|nil|no
" 1"|nil|nil|no
"　1"|nil|nil|no
"1 "|nil|nil|no
" 1 "|nil|nil|no
"0x10 "|nil|nil|no
"0x10 "|nil|nil|no
"0x10　"|nil|nil|no
" 0x10"|nil|nil|no
" 1"|1|1|ok
"1 "|1|1|ok
" 1 "|1|1|ok
"\0091\010"|1|1|ok
"\0111\012"|1|1|ok
"\0131"|1|1|ok
"  0x10  "|16|nil|ok
"\009\010\013\011\012 1 \009\010\013\011\012"|1|1|ok`
	if got != want {
		t.Errorf("tonumber reads differently:\n%s", sideBySide(want, got))
	}
}
