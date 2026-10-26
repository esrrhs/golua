//go:build amd64

#include "textflag.h"

TEXT ·Get(SB), NOSPLIT, $0-8
	MOVQ R14, AX
	MOVQ AX, ret+0(FP)
	RET
