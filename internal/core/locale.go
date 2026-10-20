package core

import "os"

// The locale, as far as numbers are concerned.
//
// os.setlocale is the C library's, and what it changes that a Lua program
// can see is one thing: the decimal point. Under pt_BR, 'print(1.5)'
// writes "1,5", 'string.format("%.3f", 4.51)' writes "4,510", and
// 'tonumber("3,4")' is 3.4 -- while 'tonumber("3.4")' still is, because
// the reference interpreter accepts both once the locale is set. Number
// literals in source are unaffected: 'load("return 3.4")' is 3.4 and
// 'load("return .4,3")' is still two values.
//
// Go has no setlocale of its own, so this reaches C's when the build has
// cgo (see cdlopen_locale.go) and answers "not available" when it does
// not -- which is exactly what a C build answers for a locale that is not
// installed, and what this port answered for every locale but "C" before.

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
var setLocaleHook func(cat int, name string) (string, string, bool)

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
