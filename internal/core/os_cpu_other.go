//go:build !unix

package core

// osProcessCPUTime has no portable non-cgo CPU timer on this GOOS.
// os.clock falls back to wall time since process start (SPEC).
func osProcessCPUTime() (float64, bool) { return 0, false }
