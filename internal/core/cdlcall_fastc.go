//go:build cgo && unix && golua_fastc

package core

import (
	"unsafe"

	"github.com/esrrhs/golua/internal/fastcall"
)

/*
#include "cdlbridge.h"
*/
import "C"

// callCFunction invokes a lua_CFunction via internal/fastcall — no cgocall.
// Experimental (-tags golua_fastc). C may still //export back into Go;
// that re-entry is also outside the normal cgocall pairing.
func callCFunction(f unsafe.Pointer, p *C.lua_State) int {
	return int(fastcall.Call1(uintptr(f), uintptr(unsafe.Pointer(p))))
}
