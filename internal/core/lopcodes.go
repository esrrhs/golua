// Translation of lopcodes.h / lopcodes.c / lopnames.h (PUC-Lua 5.4).
//
// PUC-Lua packs an instruction into a 32-bit word. Here an instruction is
// a plain struct: the fields are the same (Op, A, B, C; B doubles as
// Bx/sBx/sJ), only the bit packing is gone.
package core

// Instruction is the Go counterpart of Lua's Instruction word.
// PUC-Lua packs Op/A/B/C/k into a 32-bit word; here they are plain
// fields (K is the 'k' bit).
type Instruction struct {
	Op int
	A  int
	B  int // also Bx, sBx or sJ depending on the opcode
	C  int
	K  int // the 'k' bit (0 or 1)
}

func newABC(op, a, b, c, k int) Instruction {
	return Instruction{Op: op, A: a, B: b, C: c, K: k}
}

// Opcodes (lopcodes.h), in the same order as PUC-Lua.
const (
	OP_MOVE = iota
	OP_LOADI
	OP_LOADF
	OP_LOADK
	OP_LOADKX
	OP_LOADFALSE
	OP_LFALSESKIP
	OP_LOADTRUE
	OP_LOADNIL
	OP_GETUPVAL
	OP_SETUPVAL
	OP_GETTABUP
	OP_GETTABLE
	OP_GETI
	OP_GETFIELD
	OP_SETTABUP
	OP_SETTABLE
	OP_SETI
	OP_SETFIELD
	OP_NEWTABLE
	OP_SELF
	OP_ADDI
	OP_ADDK
	OP_SUBK
	OP_MULK
	OP_MODK
	OP_POWK
	OP_DIVK
	OP_IDIVK
	OP_BANDK
	OP_BORK
	OP_BXORK
	OP_SHRI
	OP_SHLI
	OP_ADD
	OP_SUB
	OP_MUL
	OP_MOD
	OP_POW
	OP_DIV
	OP_IDIV
	OP_BAND
	OP_BOR
	OP_BXOR
	OP_SHL
	OP_SHR
	OP_MMBIN
	OP_MMBINI
	OP_MMBINK
	OP_UNM
	OP_BNOT
	OP_NOT
	OP_LEN
	OP_CONCAT
	OP_CLOSE
	OP_TBC
	OP_JMP
	OP_EQ
	OP_LT
	OP_LE
	OP_EQK
	OP_EQI
	OP_LTI
	OP_LEI
	OP_GTI
	OP_GEI
	OP_TEST
	OP_TESTSET
	OP_CALL
	OP_TAILCALL
	OP_RETURN
	OP_RETURN0
	OP_RETURN1
	OP_FORLOOP
	OP_FORPREP
	OP_TFORPREP
	OP_TFORCALL
	OP_TFORLOOP
	OP_SETLIST
	OP_CLOSURE
	OP_VARARG
	OP_VARARGPREP
	OP_EXTRAARG
	NUM_OPCODES
)

// opNames is the translation of lopnames.h.
var opNames = [NUM_OPCODES]string{
	"MOVE", "LOADI", "LOADF", "LOADK", "LOADKX", "LOADFALSE", "LFALSESKIP",
	"LOADTRUE", "LOADNIL", "GETUPVAL", "SETUPVAL", "GETTABUP", "GETTABLE",
	"GETI", "GETFIELD", "SETTABUP", "SETTABLE", "SETI", "SETFIELD",
	"NEWTABLE", "SELF", "ADDI", "ADDK", "SUBK", "MULK", "MODK", "POWK",
	"DIVK", "IDIVK", "BANDK", "BORK", "BXORK", "SHRI", "SHLI", "ADD",
	"SUB", "MUL", "MOD", "POW", "DIV", "IDIV", "BAND", "BOR", "BXOR",
	"SHL", "SHR", "MMBIN", "MMBINI", "MMBINK", "UNM", "BNOT", "NOT",
	"LEN", "CONCAT", "CLOSE", "TBC", "JMP", "EQ", "LT", "LE", "EQK",
	"EQI", "LTI", "LEI", "GTI", "GEI", "TEST", "TESTSET", "CALL",
	"TAILCALL", "RETURN", "RETURN0", "RETURN1", "FORLOOP", "FORPREP",
	"TFORPREP", "TFORCALL", "TFORLOOP", "SETLIST", "CLOSURE", "VARARG",
	"VARARGPREP", "EXTRAARG",
}

func opName(op int) string {
	if op >= 0 && op < NUM_OPCODES {
		return opNames[op]
	}
	return "???"
}

// Field sizes and offsets used by the code generator (lopcodes.h).
const (
	MAXARG_C  = (1 << 8) - 1  // 255
	MAXARG_sC = MAXARG_C >> 1 // 127
	// SIZE_B is 8 bits in 5.4, so MAXARG_B == MAXARG_C and the signed
	// interpretation of B uses OFFSET_sC (lopcodes.h: GETARG_sB is
	// defined as sC2int(GETARG_B(i))).
	MAXARG_B   = (1 << 8) - 1  // 255
	MAXARG_sB  = MAXARG_B >> 1 // 127
	MAXARG_Bx  = (1 << 17) - 1 // 131071
	MAXARG_sBx = MAXARG_Bx >> 1
	MAXARG_Ax  = (1 << 25) - 1
	MAXARG_sJ  = (1 << 25) - 1
	OFFSET_sC  = MAXARG_sC
	OFFSET_sB  = MAXARG_sB
	OFFSET_sBx = MAXARG_sBx
	OFFSET_sJ  = MAXARG_sJ >> 1
	MAXARG_A   = (1 << 8) - 1 // 255
	SIZE_A     = 8
	MAXINDEXRK = MAXARG_B
)

// NO_REG is an invalid register that fits in 8 bits (lopcodes.h).
const NO_REG = MAXARG_A

// LFIELDS_PER_FLUSH is the number of list items to accumulate before a
// SETLIST instruction (lopcodes.h).
const LFIELDS_PER_FLUSH = 50

// MAXUPVAL is the maximum number of upvalues in a closure (lfunc.h).
const MAXUPVAL = 255

func sBx(b int) int { return b - OFFSET_sBx }

// int2sC converts an integer into an 'sC' operand (lopcodes.h).
func int2sC(i int) int { return i + OFFSET_sC }

// testTMode reports whether the opcode is a test/comparison (the 'T' column
// of luaP_opmodes in lopcodes.c).
func testTMode(op int) bool {
	switch op {
	case OP_EQ, OP_LT, OP_LE, OP_EQK, OP_EQI, OP_LTI, OP_LEI, OP_GTI, OP_GEI,
		OP_TEST, OP_TESTSET:
		return true
	}
	return false
}

// testITMode reports whether the opcode uses L.top as an implicit operand
// (the 'it' column of luaP_opmodes in lopcodes.c).
func testITMode(op int) bool {
	switch op {
	case OP_CALL, OP_TAILCALL, OP_RETURN, OP_SETLIST, OP_VARARG, OP_VARARGPREP:
		return true
	}
	return false
}

// isIT reports whether the instruction uses L.top (lopcodes.h).
func isIT(i Instruction) bool { return testITMode(i.Op) && i.B == 0 }
