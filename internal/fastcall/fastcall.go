//go:build linux && amd64

package fastcall

import (
	"sync"
	"unsafe"
)

//go:linkname runtime_cgocall runtime.cgocall
func runtime_cgocall(fn, arg unsafe.Pointer) int32

// Addresses of the C-ABI trampolines (see fastcall_linux_amd64.s).
var (
	call0ABI0 uintptr
	call1ABI0 uintptr
	call2ABI0 uintptr
)

type call0Args struct {
	fn uintptr
	r1 uintptr
}

type call1Args struct {
	fn uintptr
	a1 uintptr
	r1 uintptr
}

type call2Args struct {
	fn uintptr
	a1 uintptr
	a2 uintptr
	r1 uintptr
}

// Heap-backed arg blocks: a stack move during //export must not invalidate
// the pointer the trampoline holds. sync.Pool keeps steady-state alloc-free
// (same idea as ebitengine/purego). Stack + _cgo_topofstack was tried and
// still mis-wrote results under TestCModuleAPI; kept off the hot path.
var (
	call0Pool = sync.Pool{New: func() any { return new(call0Args) }}
	call1Pool = sync.Pool{New: func() any { return new(call1Args) }}
	call2Pool = sync.Pool{New: func() any { return new(call2Args) }}
)

// Call0 calls fn() via runtime.cgocall and returns the integer result.
func Call0(fn uintptr) uintptr {
	a := call0Pool.Get().(*call0Args)
	a.fn = fn
	a.r1 = 0
	runtime_cgocall(unsafe.Pointer(call0ABI0), unsafe.Pointer(a))
	r := a.r1
	call0Pool.Put(a)
	return r
}

// Call1 calls fn(a1) via runtime.cgocall and returns the integer result.
func Call1(fn, a1 uintptr) uintptr {
	a := call1Pool.Get().(*call1Args)
	a.fn = fn
	a.a1 = a1
	a.r1 = 0
	runtime_cgocall(unsafe.Pointer(call1ABI0), unsafe.Pointer(a))
	r := a.r1
	call1Pool.Put(a)
	return r
}

// Call2 calls fn(a1, a2) via runtime.cgocall and returns the integer result.
func Call2(fn, a1, a2 uintptr) uintptr {
	a := call2Pool.Get().(*call2Args)
	a.fn = fn
	a.a1 = a1
	a.a2 = a2
	a.r1 = 0
	runtime_cgocall(unsafe.Pointer(call2ABI0), unsafe.Pointer(a))
	r := a.r1
	call2Pool.Put(a)
	return r
}
