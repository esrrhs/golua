//go:build cgo

package core

/*
#include <stdlib.h>
*/
import "C"

// 5.3.4 luaconf (LUA_USE_POSIX): l_rand/l_srand → random/srandom,
// L_RANDMAX → 2147483647.
func lRand() int32     { return int32(C.random()) }
func lSrand(seed uint) { C.srandom(C.uint(seed)) }

const lRandMax = 2147483647
