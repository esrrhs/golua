//go:build cgo && unix && !golua_fastc && !golua_purego

package core

import "unsafe"

/*
#include "cdlbridge.h"
*/
import "C"

// callCFunction invokes a lua_CFunction through the cgo trampoline
// (runtime.cgocall + stack switch). Default production path.
func callCFunction(f unsafe.Pointer, p *C.lua_State) int {
	return int(C.cdl_call_cfunction(C.lua_CFunction(f), p))
}
