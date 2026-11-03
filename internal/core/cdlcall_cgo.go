//go:build cgo && unix && !golua_purego && (!golua_fastc || !linux || !amd64)

package core

import "unsafe"

/*
#include "cdlbridge.h"
*/
import "C"

// callCFunction invokes a lua_CFunction through the cgo trampoline
// (runtime.cgocall + stack switch). Default production path. Also used
// when -tags golua_fastc is set on a non-linux/amd64 platform.
func callCFunction(f unsafe.Pointer, p *C.lua_State) int {
	return int(C.cdl_call_cfunction(C.lua_CFunction(f), p))
}
