//go:build !cgo || !unix

package lua

// localeCatOf answers the LC_* value of a category name.  Without cgo there
// is no C library to hand one to, and off unix there is no dlopen and no
// <dlfcn.h> to build the bridge with -- see cdlbridge_nowin.go -- so the
// C side of the package is not there either way and any value will do.
// With cgo on unix the same name is defined in cdlopen_locale.go, which
// reads C's own constants.
func localeCatOf(which int) int { return 0 }
