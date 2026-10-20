package core

import "testing"

// TestLocaleCollateAndCtype pins l_strcmp and the ctype predicates.
// PUC-Lua orders strings with strcoll and classifies bytes with ctype.h,
// so a collate locale that puts "á" between "a" and "m", and a ctype
// locale in which "á" is a letter, have to be visible from Lua.
func TestLocaleCollateAndCtype(t *testing.T) {
	if setLocaleHook == nil {
		t.Skip("no C locale in this build")
	}
	if _, ok := setLocale(0, "C"); !ok {
		t.Fatal("cannot select the C locale")
	}
	t.Cleanup(func() { setLocale(0, "C") })
	if _, ok := setLocale(1, "pt_BR.iso88591"); !ok {
		t.Skip("pt_BR.iso88591 is not installed")
	}
	setLocale(0, "C")

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	err := L.DoString(`
		assert(os.setlocale("C"))
		assert(not ("\xe1lo" < "amo"))
		assert(os.setlocale("pt_BR.iso88591", "collate"))
		assert(os.setlocale(nil, "collate") == "pt_BR.iso88591")
		assert(os.setlocale(nil, "ctype") == "C")
		assert("alo" < "\xe1lo" and "\xe1lo" < "amo")
		assert(os.setlocale("pt_BR.iso88591", "ctype"))
		assert(string.gsub("\xe1\xe9\xed\xf3\xfa", "%a", "x") == "xxxxx")
		assert(string.upper("\xe1\xc1\xe9{xuxu}\xe7\xe3o") == "\xc1\xc1\xc9{XUXU}\xc7\xc3O")
		assert(string.lower("\xc1") == "\xe1")
		assert(os.setlocale("C"))
		assert(not ("\xe1lo" < "amo"))
		assert(string.gsub("\xe1\xe9\xed\xf3\xfa", "%a", "x") == "\xe1\xe9\xed\xf3\xfa")
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// TestStringOrderWithZeros pins the C-locale half of l_strcmp: a zero is
// a byte like any other, and the shorter string is the smaller one when
// the shared prefix matches.
func TestStringOrderWithZeros(t *testing.T) {
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	err := L.DoString(`
		assert("a" < "a\0")
		assert("a\0a" < "a\0b")
		assert("a\0b" < "ab")
	`)
	if err != nil {
		t.Fatal(err)
	}
}
