//go:build cgo && unix && amd64 && golua_fastc

package core

import "testing"

func TestFastCallCFunction(t *testing.T) {
	t.Skip("golua_fastc: raw Go-stack CALL is incompatible with //export re-entry " +
		"(runtime: morestack on g0 / exitsyscall). Use default cgo or -tags golua_purego " +
		"for C extensions that call lua_*.")
}
