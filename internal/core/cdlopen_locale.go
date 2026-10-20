//go:build cgo && unix

package core

// The C library's locale. os.setlocale, strcoll and the ctype predicates
// are all that library's: Go has none of them.

/*
#include "cdlbridge.h"
*/
import "C"

import "unsafe"

func init() {
	setLocaleHook = func(cat int, name string) (string, string, bool) {
		cname := C.CString(name)
		defer C.free(unsafe.Pointer(cname))
		r := C.cdl_setlocale(C.int(cat), cname)
		if r == nil {
			return "", "", false /* not available, as in C */
		}
		return C.GoString(r), C.GoString(C.cdl_decpoint()), true
	}
	localeQueryHook = func(cat int) string {
		r := C.cdl_setlocale(C.int(cat), nil) /* setlocale(cat, NULL) */
		if r == nil {
			return ""
		}
		return C.GoString(r)
	}
	strColl = func(a, b string) int {
		ca := C.CString(a)
		cb := C.CString(b)
		defer C.free(unsafe.Pointer(ca))
		defer C.free(unsafe.Pointer(cb))
		return int(C.cdl_strcoll(ca, cb))
	}
	ctypeToLower = func(c byte) byte { return byte(C.cdl_tolower(C.int(c))) }
	ctypeToUpper = func(c byte) byte { return byte(C.cdl_toupper(C.int(c))) }
	ctypeIsAlpha = func(c byte) bool { return C.cdl_isalpha(C.int(c)) != 0 }
	ctypeIsCntrl = func(c byte) bool { return C.cdl_iscntrl(C.int(c)) != 0 }
	ctypeIsDigit = func(c byte) bool { return C.cdl_isdigit(C.int(c)) != 0 }
	ctypeIsGraph = func(c byte) bool { return C.cdl_isgraph(C.int(c)) != 0 }
	ctypeIsLower = func(c byte) bool { return C.cdl_islower(C.int(c)) != 0 }
	ctypeIsPunct = func(c byte) bool { return C.cdl_ispunct(C.int(c)) != 0 }
	ctypeIsSpace = func(c byte) bool { return C.cdl_isspace(C.int(c)) != 0 }
	ctypeIsUpper = func(c byte) bool { return C.cdl_isupper(C.int(c)) != 0 }
	ctypeIsAlnum = func(c byte) bool { return C.cdl_isalnum(C.int(c)) != 0 }
	ctypeIsXDigit = func(c byte) bool { return C.cdl_isxdigit(C.int(c)) != 0 }
}

// localeCatOf turns the category os.setlocale was given into the LC_* it
// means, by its place in Lua's list. The LC_* are not plain numbers on
// every platform, so C answers this one.
func localeCatOf(which int) int { return int(C.cdl_lc_cat(C.int(which))) }
