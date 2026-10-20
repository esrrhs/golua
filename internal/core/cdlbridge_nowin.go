//go:build cgo && !unix

// Why this file exists, since it does nothing: there is no dlopen on
// Windows, so every file that would use one -- cdlopen.go,
// cdlopen_ext.go, cdlopen_aux.go, cdlopen_locale.go, liolib_cgo.go --
// is built on unix only, and cdlbridge.c is empty there (see the #if at
// the top of it).
//
// But cdlbridge.c is still a C source file in this package, and cmd/go
// refuses a package that has one and does not use cgo:
//
//	C source files not allowed when not using cgo or SWIG
//
// "Uses cgo" means some Go file of the package imports "C", and with
// every file that would do so excluded there is none -- so a plain
// 'go build' on Windows with CGO_ENABLED=1, which is the default
// wherever a C toolchain is present, would fail on a package that
// otherwise builds perfectly well.  That is what this import is for.
// It costs one empty object file.
//
// No comment may sit directly above the import: cgo takes that one as
// its C preamble, and prose is not C.
package core

import "C"
