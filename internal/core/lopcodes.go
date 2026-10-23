// Translation of lopcodes.h / lopcodes.c (PUC-Lua 5.3.4).
//
// PUC-Lua packs an instruction into a 32-bit word. Here an instruction is
// a plain struct: the fields are the same (Op, A, B, C). B doubles as
// Bx/sBx/Ax depending on the opcode. For RK operands, B/C carry the
// BITRK bit the way PUC-Lua encodes them (ISK / INDEXK / RKASK).
// The K field is unused in 5.3 (kept so Instruction stays one type).
package core

// Instruction is the Go counterpart of Lua's Instruction word.
type Instruction struct {
	Op int
	A  int
	B  int // also Bx or sBx (biased) depending on the opcode
	C  int
	K  int // unused in 5.3; kept for struct compatibility
}

func newABC(op, a, b, c int, k ...int) Instruction {
	return Instruction{Op: op, A: a, B: b, C: c}
}

func newABx(op, a, bx int) Instruction {
	return Instruction{Op: op, A: a, B: bx}
}

func newAsBx(op, a, sbx int) Instruction {
	return Instruction{Op: op, A: a, B: sbx + OFFSET_sBx}
}

func newAx(op, ax int) Instruction {
	return Instruction{Op: op, A: ax}
}

// Opcodes (lopcodes.h), ORDER OP — same order as PUC-Lua 5.3.4.
const (
	OP_MOVE = iota
	OP_LOADK
	OP_LOADKX
	OP_LOADBOOL
	OP_LOADNIL
	OP_GETUPVAL
	OP_GETTABUP
	OP_GETTABLE
	OP_SETTABUP
	OP_SETUPVAL
	OP_SETTABLE
	OP_NEWTABLE
	OP_SELF
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
	OP_UNM
	OP_BNOT
	OP_NOT
	OP_LEN
	OP_CONCAT
	OP_JMP
	OP_EQ
	OP_LT
	OP_LE
	OP_TEST
	OP_TESTSET
	OP_CALL
	OP_TAILCALL
	OP_RETURN
	OP_FORLOOP
	OP_FORPREP
	OP_TFORCALL
	OP_TFORLOOP
	OP_SETLIST
	OP_CLOSURE
	OP_VARARG
	OP_EXTRAARG
	NUM_OPCODES
)

// opNames is luaP_opnames.
var opNames = [NUM_OPCODES]string{
	"MOVE", "LOADK", "LOADKX", "LOADBOOL", "LOADNIL", "GETUPVAL",
	"GETTABUP", "GETTABLE", "SETTABUP", "SETUPVAL", "SETTABLE",
	"NEWTABLE", "SELF", "ADD", "SUB", "MUL", "MOD", "POW", "DIV", "IDIV",
	"BAND", "BOR", "BXOR", "SHL", "SHR", "UNM", "BNOT", "NOT", "LEN",
	"CONCAT", "JMP", "EQ", "LT", "LE", "TEST", "TESTSET", "CALL",
	"TAILCALL", "RETURN", "FORLOOP", "FORPREP", "TFORCALL", "TFORLOOP",
	"SETLIST", "CLOSURE", "VARARG", "EXTRAARG",
}

func opName(op int) string {
	if op >= 0 && op < NUM_OPCODES {
		return opNames[op]
	}
	return "???"
}

// Field sizes from lopcodes.h (5.3: B and C are 9 bits).
const (
	SIZE_C  = 9
	SIZE_B  = 9
	SIZE_Bx = SIZE_C + SIZE_B // 18
	SIZE_A  = 8
	SIZE_Ax = SIZE_C + SIZE_B + SIZE_A // 26
	SIZE_OP = 6

	MAXARG_Bx  = (1 << SIZE_Bx) - 1 // 262143
	MAXARG_sBx = MAXARG_Bx >> 1     // 131071
	MAXARG_Ax  = (1 << SIZE_Ax) - 1
	MAXARG_A   = (1 << SIZE_A) - 1 // 255
	MAXARG_B   = (1 << SIZE_B) - 1 // 511
	MAXARG_C   = (1 << SIZE_C) - 1 // 511

	OFFSET_sBx = MAXARG_sBx

	// BITRK: high bit of B/C means "constant".
	BITRK      = 1 << (SIZE_B - 1) // 256
	MAXINDEXRK = BITRK - 1         // 255
)

// NO_REG is an invalid register that fits in 8 bits.
const NO_REG = MAXARG_A

// LFIELDS_PER_FLUSH is items to accumulate before SETLIST.
const LFIELDS_PER_FLUSH = 50

// MAXUPVAL is the maximum number of upvalues in a closure.
const MAXUPVAL = 255

func sBx(b int) int { return b - OFFSET_sBx }

func ISK(x int) bool  { return x&BITRK != 0 }
func INDEXK(r int) int { return r & ^BITRK }
func RKASK(x int) int { return x | BITRK }

// Op modes (from lopcodes.c).
const (
	iABC = iota
	iABx
	iAsBx
	iAx
)

const (
	OpArgN = iota // not used
	OpArgU        // used
	OpArgR        // register or jump offset
	OpArgK        // constant or register/constant
)

// opModes packs (t<<7)|(a<<6)|(b<<4)|(c<<2)|m per opcode.
var opModes = [NUM_OPCODES]byte{
	/* MOVE */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iABC,
	/* LOADK */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgN << 2) | iABx,
	/* LOADKX */ (0 << 7) | (1 << 6) | (OpArgN << 4) | (OpArgN << 2) | iABx,
	/* LOADBOOL */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgU << 2) | iABC,
	/* LOADNIL */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABC,
	/* GETUPVAL */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABC,
	/* GETTABUP */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgK << 2) | iABC,
	/* GETTABLE */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgK << 2) | iABC,
	/* SETTABUP */ (0 << 7) | (0 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* SETUPVAL */ (0 << 7) | (0 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABC,
	/* SETTABLE */ (0 << 7) | (0 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* NEWTABLE */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgU << 2) | iABC,
	/* SELF */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgK << 2) | iABC,
	/* ADD */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* SUB */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* MUL */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* MOD */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* POW */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* DIV */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* IDIV */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* BAND */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* BOR */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* BXOR */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* SHL */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* SHR */ (0 << 7) | (1 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* UNM */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iABC,
	/* BNOT */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iABC,
	/* NOT */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iABC,
	/* LEN */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iABC,
	/* CONCAT */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgR << 2) | iABC,
	/* JMP */ (0 << 7) | (0 << 6) | (OpArgR << 4) | (OpArgN << 2) | iAsBx,
	/* EQ */ (1 << 7) | (0 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* LT */ (1 << 7) | (0 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* LE */ (1 << 7) | (0 << 6) | (OpArgK << 4) | (OpArgK << 2) | iABC,
	/* TEST */ (1 << 7) | (0 << 6) | (OpArgN << 4) | (OpArgU << 2) | iABC,
	/* TESTSET */ (1 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgU << 2) | iABC,
	/* CALL */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgU << 2) | iABC,
	/* TAILCALL */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgU << 2) | iABC,
	/* RETURN */ (0 << 7) | (0 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABC,
	/* FORLOOP */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iAsBx,
	/* FORPREP */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iAsBx,
	/* TFORCALL */ (0 << 7) | (0 << 6) | (OpArgN << 4) | (OpArgU << 2) | iABC,
	/* TFORLOOP */ (0 << 7) | (1 << 6) | (OpArgR << 4) | (OpArgN << 2) | iAsBx,
	/* SETLIST */ (0 << 7) | (0 << 6) | (OpArgU << 4) | (OpArgU << 2) | iABC,
	/* CLOSURE */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABx,
	/* VARARG */ (0 << 7) | (1 << 6) | (OpArgU << 4) | (OpArgN << 2) | iABC,
	/* EXTRAARG */ (0 << 7) | (0 << 6) | (OpArgU << 4) | (OpArgU << 2) | iAx,
}

func getOpMode(m int) int     { return int(opModes[m] & 3) }
func getBMode(m int) int      { return int((opModes[m] >> 4) & 3) }
func getCMode(m int) int      { return int((opModes[m] >> 2) & 3) }
func testTMode(op int) bool   { return opModes[op]&(1<<7) != 0 }

// testITMode: 5.3 has no separate 'it' column; open ops use B/C==0.
func testITMode(op int) bool {
	switch op {
	case OP_CALL, OP_TAILCALL, OP_RETURN, OP_SETLIST, OP_VARARG:
		return true
	}
	return false
}

func isIT(i Instruction) bool {
	switch i.Op {
	case OP_CALL, OP_TAILCALL:
		return i.B == 0
	case OP_RETURN, OP_VARARG:
		return i.B == 0
	case OP_SETLIST:
		return i.B == 0
	}
	return false
}

// Temporary stubs so 5.4-era debug/disasm code still compiles during the port.
const (
	OP_LOADI = -1000
	OP_LOADF = -1001
	OP_LOADFALSE = -1002
	OP_LFALSESKIP = -1003
	OP_LOADTRUE = -1004
	OP_GETI = -1005
	OP_GETFIELD = -1006
	OP_SETI = -1007
	OP_SETFIELD = -1008
	OP_ADDI = -1009
	OP_ADDK = -1010
	OP_SUBK = -1011
	OP_MULK = -1012
	OP_MODK = -1013
	OP_POWK = -1014
	OP_DIVK = -1015
	OP_IDIVK = -1016
	OP_BANDK = -1017
	OP_BORK = -1018
	OP_BXORK = -1019
	OP_SHRI = -1020
	OP_SHLI = -1021
	OP_MMBIN = -1022
	OP_MMBINI = -1023
	OP_MMBINK = -1024
	OP_EQK = -1025
	OP_EQI = -1026
	OP_LTI = -1027
	OP_LEI = -1028
	OP_GTI = -1029
	OP_GEI = -1030
	OP_CLOSE = -1031
	OP_TBC = -1032
	OP_RETURN0 = -1033
	OP_RETURN1 = -1034
	OP_TFORPREP = -1035
	OP_VARARGPREP = -1036
	OFFSET_sJ = -1037
)

const (
	OFFSET_sB = 0
	OFFSET_sC = 0
)
