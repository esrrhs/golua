//go:build amd64

#include "textflag.h"
#include "funcdata.h"

// RawCall* invoke C on the Go stack without runtime.cgocall.
// SysV: SP must be 8-mod-16 immediately before CALL. R12 saves SP.

// func RawCall0(fn uintptr) uintptr
TEXT ·RawCall0(SB), 0, $2048-16
	NO_LOCAL_POINTERS
	MOVQ fn+0(FP), AX
	MOVQ SP, R12
	ANDQ $~15, SP
	SUBQ $8, SP
	CALL AX
	MOVQ R12, SP
	MOVQ AX, ret+8(FP)
	RET

// func RawCall1(fn, a1 uintptr) uintptr
TEXT ·RawCall1(SB), 0, $2048-24
	NO_LOCAL_POINTERS
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ SP, R12
	ANDQ $~15, SP
	SUBQ $8, SP
	CALL AX
	MOVQ R12, SP
	MOVQ AX, ret+16(FP)
	RET

// func RawCall2(fn, a1, a2 uintptr) uintptr
TEXT ·RawCall2(SB), 0, $2048-32
	NO_LOCAL_POINTERS
	MOVQ fn+0(FP), AX
	MOVQ a1+8(FP), DI
	MOVQ a2+16(FP), SI
	MOVQ SP, R12
	ANDQ $~15, SP
	SUBQ $8, SP
	CALL AX
	MOVQ R12, SP
	MOVQ AX, ret+24(FP)
	RET
