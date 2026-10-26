//go:build !cgo || !unix

package core

// exerciseCAPI is the C boundary. Without cgo, or on Windows where the
// loader backend is absent, there is nothing to call.
func exerciseCAPI(dir string) error {
	_ = dir
	return nil
}
