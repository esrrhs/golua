//go:build linux && amd64

#include "textflag.h"
#include "funcdata.h"

// Frame 32: entry SP is 8-mod-16 (asmcgocall CALL); push BP → 0-mod-16;
// sub $32 keeps 0-mod-16 so CALL to C satisfies SysV (needed for //export
// stubs that movaps to (%rsp)).
#define FRAME 32

GLOBL ·call0ABI0(SB), NOPTR|RODATA, $8
DATA ·call0ABI0(SB)/8, $call0<>(SB)

GLOBL ·call1ABI0(SB), NOPTR|RODATA, $8
DATA ·call1ABI0(SB)/8, $call1<>(SB)

GLOBL ·call2ABI0(SB), NOPTR|RODATA, $8
DATA ·call2ABI0(SB)/8, $call2<>(SB)

// Args are heap-allocated (see fastcall.go); no _cgo_topofstack fixup.

TEXT call0<>(SB), NOSPLIT, $FRAME
	MOVQ DI, 0(SP)
	MOVQ 0(DI), AX
	CALL AX
	MOVQ 0(SP), DI
	MOVQ AX, 8(DI)
	XORL AX, AX
	RET

TEXT call1<>(SB), NOSPLIT, $FRAME
	MOVQ DI, 0(SP)
	MOVQ 0(DI), AX
	MOVQ 8(DI), DI
	CALL AX
	MOVQ 0(SP), DI
	MOVQ AX, 16(DI)
	XORL AX, AX
	RET

TEXT call2<>(SB), NOSPLIT, $FRAME
	MOVQ DI, 0(SP)
	MOVQ 0(DI), AX
	MOVQ 8(DI), R11
	MOVQ 16(DI), SI
	MOVQ R11, DI
	CALL AX
	MOVQ 0(SP), DI
	MOVQ AX, 24(DI)
	XORL AX, AX
	RET
