package core

import (
	"os"
	"strings"
	"sync/atomic"
)

// The locale.
//
// os.setlocale is the C library's. Three things a Lua program can see
// change with it:
//
//   - the decimal point (LC_NUMERIC). Under pt_BR, 'print(1.5)' writes
//     "1,5" and 'tonumber("3,4")' is 3.4. Number literals in source are
//     unaffected.
//   - string order (LC_COLLATE). '<' and '<=' go through strcoll, one
//     segment between zeros at a time (lvm.c l_strcmp). In the C locale
//     that is ordinary byte order, zeros included.
//   - character classes (LC_CTYPE). string.upper, string.lower, the
//     pattern classes and tonumber's digit scan use ctype.h.
//
// Go has no setlocale of its own, so this reaches C's when the build has
// cgo (see cdlopen_locale.go). Without cgo there is no C library to ask:
// a name is accepted and only the decimal point changes, which is what a
// program that calls os.setlocale for the decimal comma needs, and the
// collate and ctype answers stay those of the C locale.

// localeName is what os.setlocale reports when asked, and localeDecPoint
// is the decimal point of that locale: the one character number
// formatting has to use instead of '.', and that string-to-number
// conversion has to accept as well as '.'.
var (
	localeName     = "C"
	localeDecPoint = "."
)

// setLocaleHook is the C library's setlocale. It answers the name the
// library settled on and the decimal point that goes with it, or false
// when the locale is not available. It is nil without cgo.
//
// localeQueryHook is setlocale(cat, NULL): the name of that one category.
// strColl is strcoll, and the ctype hooks are ctype.h. They are nil
// without cgo, and the C-locale stand-ins below are what runs then.
var (
	setLocaleHook   func(cat int, name string) (string, string, bool)
	localeQueryHook func(cat int) string
	strColl         func(a, b string) int

	ctypeToLower  func(byte) byte
	ctypeToUpper  func(byte) byte
	ctypeIsAlpha  func(byte) bool
	ctypeIsCntrl  func(byte) bool
	ctypeIsDigit  func(byte) bool
	ctypeIsGraph  func(byte) bool
	ctypeIsLower  func(byte) bool
	ctypeIsPunct  func(byte) bool
	ctypeIsSpace  func(byte) bool
	ctypeIsUpper  func(byte) bool
	ctypeIsAlnum  func(byte) bool
	ctypeIsXDigit func(byte) bool
)

// collateIsC / ctypeIsC select the C-locale fast path. strcoll of the C
// locale is byte order, and the ctype predicates are the ASCII ones, so
// a program that never calls os.setlocale does not pay for a cgo call on
// every comparison. A name of "C" or "POSIX" is that locale; anything
// the C library spells differently, including "C.UTF-8", is not.
var collateIsC atomic.Bool
var ctypeIsC atomic.Bool

func init() {
	collateIsC.Store(true)
	ctypeIsC.Store(true)
}

// localeCategories are the categories os.setlocale takes, in the order
// Lua lists them, as LC_* constants.
var localeCategories = []string{"all", "collate", "ctype", "monetary", "numeric", "time"}

// commaLangs are the language codes whose numeric format writes a comma
// as the decimal separator, the way C's setlocale would tell printf to.
// A locale name is "lang", "lang_TERRITORY", "lang.encoding" or
// "lang_TERRITORY.encoding"; the language part is what decides the
// decimal point, so "pt_BR.UTF-8", "pt_PT" and "pt" all answer ",".
// This table is what lets os.setlocale work without cgo: Go has no
// setlocale of its own, but the only thing a Lua program can see of the
// locale is the decimal point, and that is a property of the language.
var commaLangs = map[string]bool{
	"de": true, "fr": true, "it": true, "nl": true, "pt": true,
	"es": true, "ru": true, "pl": true, "sv": true, "no": true,
	"nb": true, "nn": true, "da": true, "fi": true, "cs": true,
	"hu": true, "ro": true, "uk": true, "el": true, "tr": true,
	"hr": true, "sr": true, "sk": true, "sl": true, "bg": true,
	"sq": true, "et": true, "lv": true, "lt": true, "is": true,
	"ka": true, "hy": true, "az": true, "kk": true, "uz": true,
	"be": true, "mk": true, "bs": true, "eu": true, "ca": true,
	"gl": true, "cy": true, "ga": true, "mt": true,
}

// langOf extracts the language code from a locale name: everything up to
// the first '_', '.' or '@'. "pt_BR.UTF-8" answers "pt", "C" answers "C".
func langOf(name string) string {
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || c == '.' || c == '@' {
			return name[:i]
		}
	}
	return name
}

// decPointOf answers the decimal point a locale name asks for: "," for
// the languages in commaLangs, "." for everything else, and "." for the
// "C" and "POSIX" locales and for an empty name.
func decPointOf(name string) string {
	if name == "" || name == "C" || name == "POSIX" {
		return "."
	}
	if commaLangs[langOf(name)] {
		return ","
	}
	return "."
}

// envLocale is what 'os.setlocale("")' means in C: the locale the
// environment variables ask for, in the order C's setlocale looks them
// up for LC_ALL. An empty answer means "C".
func envLocale() string {
	for _, v := range []string{"LC_ALL", "LC_NUMERIC", "LANG"} {
		if s := os.Getenv(v); s != "" {
			return s
		}
	}
	return "C"
}

// setLocale is what os.setlocale does when a name is given. With cgo it
// asks the C library and records what the library answered. Without cgo
// there is no C library to ask, but the one thing a Lua program can see
// of the locale is the decimal point, so the name is accepted and the
// decimal point is read from decPointOf -- which is what lets old Lua
// projects that call os.setlocale run unchanged. 'catIdx' is the index
// in localeCategories: only "all" (0) and "numeric" (4) change the
// numeric decimal point, just as in C.
func setLocale(catIdx int, name string) (string, bool) {
	if setLocaleHook != nil {
		got, dec, ok := setLocaleHook(localeCatOf(catIdx), name)
		if !ok {
			return "", false
		}
		localeName = got
		if dec != "" {
			localeDecPoint = dec
		} else {
			localeDecPoint = "."
		}
		localeNote(catIdx, got)
		return localeName, true
	}
	/* Pure Go: no C library. Accept the name, and if the category is one
	   that controls numeric formatting, set the decimal point from the
	   language of the locale. */
	if name == "" {
		name = envLocale()
	}
	localeName = name
	if catIdx == 0 /* all */ || catIdx == 4 /* numeric */ {
		localeDecPoint = decPointOf(name)
	}
	return localeName, true
}

// localeCurrent is what 'os.setlocale(nil, cat)' answers. With cgo that
// is the C library's name for the one category, which is not the name of
// the last category anybody set: collate can be pt_BR while ctype is
// still "C", and "all" is then the long LC_CTYPE=... form.
func localeCurrent(catIdx int) string {
	if localeQueryHook != nil {
		return localeQueryHook(localeCatOf(catIdx))
	}
	return localeName
}

// localeNote records whether collate and ctype are still the C locale,
// so the comparison and the character classes can stay on the fast path.
func localeNote(catIdx int, name string) {
	if strColl == nil { /* no C library: both stay the C locale */
		return
	}
	c := name == "C" || name == "POSIX"
	switch catIdx {
	case 0: /* all */
		collateIsC.Store(c)
		ctypeIsC.Store(c)
	case 1: /* collate */
		collateIsC.Store(c)
	case 2: /* ctype */
		ctypeIsC.Store(c)
	}
}

// luaV_strcmp is l_strcmp (lvm.c). In the C locale it is byte order,
// which already handles a zero in the middle the way the segment loop
// does. Otherwise each stretch between zeros is strcoll's.
func luaV_strcmp(a, b string) int {
	if collateIsC.Load() || strColl == nil {
		return strings.Compare(a, b)
	}
	for {
		if c := strColl(a, b); c != 0 {
			return c
		}
		zl1 := strings.IndexByte(a, 0)
		zl2 := strings.IndexByte(b, 0)
		if zl1 < 0 {
			zl1 = len(a)
		}
		if zl2 < 0 {
			zl2 = len(b)
		}
		if zl2 == len(b) { /* 'b' is finished? */
			if zl1 == len(a) {
				return 0
			}
			return 1
		}
		if zl1 == len(a) { /* 'a' is finished, 'b' is not */
			return -1
		}
		a = a[zl1+1:]
		b = b[zl2+1:]
	}
}

func localeToLower(c byte) byte {
	if ctypeIsC.Load() || ctypeToLower == nil {
		return strASCIILower(c)
	}
	return ctypeToLower(c)
}

func localeToUpper(c byte) byte {
	if ctypeIsC.Load() || ctypeToUpper == nil {
		return strASCIIUpper(c)
	}
	return ctypeToUpper(c)
}

func localeIsAlpha(c byte) bool {
	if ctypeIsC.Load() || ctypeIsAlpha == nil {
		return strIsAlpha(c)
	}
	return ctypeIsAlpha(c)
}

func localeIsCntrl(c byte) bool {
	if ctypeIsC.Load() || ctypeIsCntrl == nil {
		return strIsCntrl(c)
	}
	return ctypeIsCntrl(c)
}

func localeIsDigit(c byte) bool {
	if ctypeIsC.Load() || ctypeIsDigit == nil {
		return strIsDigit(c)
	}
	return ctypeIsDigit(c)
}

func localeIsGraph(c byte) bool {
	if ctypeIsC.Load() || ctypeIsGraph == nil {
		return strIsGraph(c)
	}
	return ctypeIsGraph(c)
}

func localeIsLower(c byte) bool {
	if ctypeIsC.Load() || ctypeIsLower == nil {
		return strIsLower(c)
	}
	return ctypeIsLower(c)
}

func localeIsPunct(c byte) bool {
	if ctypeIsC.Load() || ctypeIsPunct == nil {
		return strIsPunct(c)
	}
	return ctypeIsPunct(c)
}

func localeIsSpace(c byte) bool {
	if ctypeIsC.Load() || ctypeIsSpace == nil {
		return strIsSpace(c)
	}
	return ctypeIsSpace(c)
}

func localeIsUpper(c byte) bool {
	if ctypeIsC.Load() || ctypeIsUpper == nil {
		return strIsUpper(c)
	}
	return ctypeIsUpper(c)
}

func localeIsAlnum(c byte) bool {
	if ctypeIsC.Load() || ctypeIsAlnum == nil {
		return strIsAlnum(c)
	}
	return ctypeIsAlnum(c)
}

func localeIsXDigit(c byte) bool {
	if ctypeIsC.Load() || ctypeIsXDigit == nil {
		return strIsXDigit(c)
	}
	return ctypeIsXDigit(c)
}

// num2strLocale writes a number the way the locale wants it: printf has
// already used '.', and the locale may ask for something else. The hot
// path is the common one -- a locale whose decimal point is '.' -- where
// nothing is copied at all.
func num2strLocale(s string) string {
	if localeDecPoint == "." {
		return s
	}
	return replaceDecPoint(s)
}

// replaceDecPoint swaps a number's '.' for the locale's decimal point.
// Only the last '.' it can be: a number in scientific notation has one
// decimal point at most, and 'inf' and 'nan' have none.
func replaceDecPoint(s string) string {
	i := indexByte(s, '.')
	if i < 0 {
		return s
	}
	return s[:i] + localeDecPoint + s[i+1:]
}

// str2numLocale helps read a number the way the locale writes it. It
// answers the string with the locale's decimal point turned back into
// '.', for the parser to try when the string as it stands does not read
// as a number -- the reference interpreter takes "3,4" and "3.4" alike
// once pt_BR is set.
func str2numLocale(s string) string {
	if localeDecPoint == "." {
		return s
	}
	i := indexString(s, localeDecPoint)
	if i < 0 {
		return s
	}
	return s[:i] + "." + s[i+1:]
}

// indexByte and indexString are strings.Index's two shapes, spelled out
// so that the two functions above read the same way.
func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func indexString(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
