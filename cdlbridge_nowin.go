//go:build cgo && !unix

package lua

// Why this file exists, since it does nothing.
//
// There is no dlopen on Windows, so every file that would use one --
// cdlopen.go, cdlopen_ext.go, cdlopen_aux.go, cdlopen_locale.go,
// liolib_cgo.go -- is built on unix only, and cdlbridge.c is empty
// there (see the #if at the top of it).  But cdlbridge.c is still a C
// source file in this package, and cmd/go refuses a package that has
// one and does not use cgo:
//
//	C source files not allowed when not using cgo or SWIG
//
// "Uses cgo" means a Go file of the package imports "C", and with
// every such file excluded there is none -- so a plain
// 'go build' on Windows with CGO_ENABLED=1, which is the default
// wherever a C toolchain is present, would fail on a package that
// otherwise builds perfectly well.
//
// So this file imports "C" on the platforms that have no bridge to
// import it for.  It costs one empty object file.
import "C"
