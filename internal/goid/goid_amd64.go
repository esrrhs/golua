//go:build amd64

package goid

// Get returns a unique identifier for the calling goroutine.
func Get() uintptr
