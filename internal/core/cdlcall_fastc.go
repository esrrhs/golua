//go:build cgo && linux && amd64 && golua_fastc

package core

import (
	"unsafe"

	"github.com/esrrhs/golua/internal/fastcall"
)

/*
#include "cdlbridge.h"
*/
import "C"

// callCFunction invokes a lua_CFunction via internal/fastcall (thin
// runtime.cgocall trampoline). Experimental (-tags golua_fastc).
// Nested Go→C→Go(//export)→C is supported; see package fastcall.
func callCFunction(f unsafe.Pointer, p *C.lua_State) int {
	return int(fastcall.Call1(uintptr(f), uintptr(unsafe.Pointer(p))))
}
