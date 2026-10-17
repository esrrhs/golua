//go:build !cgo

package lua

// localeCatOf answers the LC_* value of a category name: without cgo there
// is no C library to hand one to, so any value will do. With cgo the same
// name is defined in cdlopen_locale.go, which reads C's own constants.
func localeCatOf(which int) int { return 0 }
