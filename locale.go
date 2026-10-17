package lua

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

// setLocale is what os.setlocale does when a name is given: it asks the
// C library, and records what the library answered. Without cgo the hook
// is nil and only "C" -- which is what Go behaves like anyway -- is
// available, which is what a C build does for a locale it does not have.
func setLocale(cat int, name string) (string, bool) {
	if setLocaleHook == nil {
		if name == "C" || name == "POSIX" {
			localeName, localeDecPoint = name, "."
			return localeName, true
		}
		return "", false
	}
	got, dec, ok := setLocaleHook(cat, name)
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
