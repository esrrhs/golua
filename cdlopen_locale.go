//go:build cgo && unix

package lua

// The C library's locale, which is the only one os.setlocale can really
// set: Go has nothing of the kind, and what a Lua program sees of the
// locale is one character -- the decimal point -- that C's library owns.

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
}

// localeCatOf turns the category os.setlocale was given into the LC_* it
// means, by its place in Lua's list. The LC_* are not plain numbers on
// every platform, so C answers this one.
func localeCatOf(which int) int { return int(C.cdl_lc_cat(C.int(which))) }
