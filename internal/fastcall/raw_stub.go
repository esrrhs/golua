//go:build !amd64

package fastcall

func RawCall0(fn uintptr) uintptr         { panic("fastcall: RawCall* only on amd64") }
func RawCall1(fn, a1 uintptr) uintptr     { panic("fastcall: RawCall* only on amd64") }
func RawCall2(fn, a1, a2 uintptr) uintptr { panic("fastcall: RawCall* only on amd64") }
