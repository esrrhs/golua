package core

import (
	"sync"
	"unsafe"
)

// cstateBlock is one malloc'd lua_State handle. The type lives in every
// build so State can point at it; only the cgo backend allocates one,
// and only that backend frees it.
type cstateBlock struct {
	base unsafe.Pointer
	id   uintptr
	g    *globalState // so release can untrack after the State is gone
	once sync.Once
}
