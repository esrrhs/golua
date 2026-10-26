//go:build !amd64

package goid

import "runtime"

// Get returns a unique identifier for the calling goroutine.
func Get() uintptr {
	var buf [32]byte
	n := runtime.Stack(buf[:], false)
	// "goroutine N ["
	const p = len("goroutine ")
	if n <= p {
		return 0
	}
	var id uintptr
	for i := p; i < n && buf[i] >= '0' && buf[i] <= '9'; i++ {
		id = id*10 + uintptr(buf[i]-'0')
	}
	return id
}
