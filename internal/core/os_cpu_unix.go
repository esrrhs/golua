//go:build unix

package core

import "syscall"

// osProcessCPUTime is the process CPU time in seconds (user + system),
// matching C clock()/CLOCKS_PER_SEC as closely as Go allows without cgo:
// syscall.Getrusage(RUSAGE_SELF).  C's clock() is typically the same
// accounting on Unix; Windows has no Getrusage here (see os_cpu_other.go).
func osProcessCPUTime() (float64, bool) {
	var r syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &r); err != nil {
		return 0, false
	}
	sec := float64(r.Utime.Sec) + float64(r.Utime.Usec)/1e6 +
		float64(r.Stime.Sec) + float64(r.Stime.Usec)/1e6
	return sec, true
}
