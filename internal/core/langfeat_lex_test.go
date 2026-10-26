package core

import (
	"os"
	"path/filepath"
	"testing"
)

// Language-feature coverage: lexical comments, string escapes, numerals,
// shebang at load. Official literals.lua / diff 77_lex already exercise
// these thoroughly; these are focused deterministic Go asserts for gaps
// that unit tests did not pin directly.

func TestLangFeatCommentsAndLongBrackets(t *testing.T) {
	confRun(t, `
		-- short comment
		--[[ long
		     comment ]]
		--[=[ nested equals form ]=]
		local a = 1 --[[x]] + --[=[y]=] 2
		assert(a == 3)

		local s1 = [[hello
world]]
		assert(s1 == "hello\nworld")

		local s2 = [=[abc]]def]=]
		assert(s2 == "abc]]def")

		local s3 = [==[ x [= [ y ]=] z ]==]
		assert(s3:find("[= [", 1, true) and s3:find("]=]", 1, true))

		-- ----[[ is a short comment (second -- starts short comment to EOL)
		----[[
		assert(true)
	`)
}

func TestLangFeatStringEscapes(t *testing.T) {
	confRun(t, `
		assert("\n" == string.char(10))
		assert("\t" == string.char(9))
		assert("\a\b\f\r\v" == string.char(7,8,12,13,11))
		assert("\\" == "\\")
		assert("\"" == '"')
		assert("\'" == "'")

		-- decimal escapes
		assert("\0\1\255" == string.char(0, 1, 255))
		assert("\65B" == "AB") -- \65 then B

		-- hex escapes
		assert("\x41\x42" == "AB")
		assert("\x00\xff" == string.char(0, 255))

		-- UTF-8 unicode escapes
		assert("\u{41}" == "A")
		assert("\u{20AC}" == "\xE2\x82\xAC") -- euro
		assert("\u{1F600}" == utf8.char(0x1F600))

		-- \z skips following whitespace in the source (not escape sequences)
		assert("ab\z
		  c" == "abc")
		assert("x\z 	
 y" == "xy")

		-- UTF-8 raw bytes in a short string
		assert(#"你好" == 6)
		assert(utf8.len("你好") == 2)

		-- reject bad escapes at load
		assert(load("return '\\u{110000}'") == nil)
		assert(load("return '\\u{}'") == nil)
		assert(load("return '\\x'") == nil)
		assert(load("return '\\xGG'") == nil)
		assert(load("return '\\z") == nil) -- unterminated
	`)
}

func TestLangFeatNumerals(t *testing.T) {
	confRun(t, `
		assert(0x10 == 16 and math.type(0x10) == "integer")
		assert(0XFF == 255)
		assert(0x1p4 == 16.0 and math.type(0x1p4) == "float")
		assert(0x1.5p1 == 2.625)
		assert(1e2 == 100.0 and 1E2 == 100.0)
		assert(1.5e-1 == 0.15)
		assert(0x0.1p0 ~= nil)
		-- integer that fits
		assert(math.type(9223372036854775807) == "integer")
		-- float literal vs int
		assert(math.type(3.0) == "float" and 3.0 == 3)
	`)
}

func TestLangFeatShebangLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sb.lua")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env lua\nreturn 42\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	L := confNewState(t)
	if err := L.DoFile(path); err != nil {
		t.Fatalf("DoFile shebang: %v", err)
	}
	if !L.IsNumber(-1) {
		t.Fatalf("shebang chunk return type %s", TypeName(L.Type(-1)))
	}
	n, _ := L.ToNumber(-1)
	if n != 42 {
		t.Fatalf("shebang return %v, want 42", n)
	}
}
