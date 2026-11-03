//go:build amd64

package fastcall

// RawCall0 calls fn on the current Go stack with no cgocall.
// Unsafe with //export; for microbenchmarks only.
func RawCall0(fn uintptr) uintptr

// RawCall1 calls fn(a1) on the current Go stack with no cgocall.
func RawCall1(fn, a1 uintptr) uintptr

// RawCall2 calls fn(a1, a2) on the current Go stack with no cgocall.
func RawCall2(fn, a1, a2 uintptr) uintptr
