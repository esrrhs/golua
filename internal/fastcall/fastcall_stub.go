//go:build !amd64

package fastcall

func Call0(fn uintptr) uintptr         { panic("fastcall: only amd64") }
func Call1(fn, a1 uintptr) uintptr     { panic("fastcall: only amd64") }
func Call2(fn, a1, a2 uintptr) uintptr { panic("fastcall: only amd64") }
