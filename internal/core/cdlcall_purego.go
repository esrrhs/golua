//go:build cgo && unix && golua_purego && !golua_fastc

package core

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

/*
#include "cdlbridge.h"
*/
import "C"

// callCFunction invokes a lua_CFunction via purego.SyscallN.
// Still goes through runtime.cgocall; kept for A/B against cgo wrappers.
func callCFunction(f unsafe.Pointer, p *C.lua_State) int {
	r1, _, _ := purego.SyscallN(uintptr(f), uintptr(unsafe.Pointer(p)))
	return int(r1)
}
