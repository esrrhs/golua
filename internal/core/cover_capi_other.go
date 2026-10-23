//go:build !cgo || !unix

package core

// exerciseCAPI is the C boundary. Without cgo, or on Windows where the
// loader backend is absent, there is nothing to call.
func exerciseCAPI(dir string) error {
	_ = dir
	return nil
}

// Stubs so tests that reference the cgo check injection compile under nocgo.
var capiFailAt = -1
var capiCheckN int

func capiResetChecks() {
	capiCheckN = 0
	capiFailAt = -1
}

func capiHasChecks() bool { return false }
