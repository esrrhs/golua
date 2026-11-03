//go:build cgo && unix && amd64

package core

/*
#include <stdint.h>

static int cdl_empty_c(void) { return 42; }
static int cdl_add1(int x) { return x + 1; }

static uintptr_t cdl_empty_c_fn(void) { return (uintptr_t)cdl_empty_c; }
static uintptr_t cdl_add1_fn(void) { return (uintptr_t)cdl_add1; }
*/
import "C"

func ffiEmptyC() int      { return int(C.cdl_empty_c()) }
func ffiAdd1C(x int) int  { return int(C.cdl_add1(C.int(x))) }
func ffiEmptyFn() uintptr { return uintptr(C.cdl_empty_c_fn()) }
func ffiAdd1Fn() uintptr  { return uintptr(C.cdl_add1_fn()) }
