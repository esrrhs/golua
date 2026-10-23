// Translation of lopcodes.h / lopcodes.c (PUC-Lua 5.3.4).
//
// PUC-Lua packs an instruction into a 32-bit word. Here an instruction is
// a plain struct: the fields are the same (Op, A, B, C). B doubles as
// Bx/sBx/Ax depending on the opcode. For RK operands, B/C carry the
// BITRK bit the way PUC-Lua encodes them (ISK / INDEXK / RKASK).
//
// Intentional OK vs C: Instruction is a struct, not a packed uint32, so
// MASK*/GET_*/SET_* bit-pack macros that mutate a word are omitted (field
// access instead). SIZE_*/POS_* layout constants match lopcodes.h; dump
// /undump pack/unpack live in lundump.go (Apply debt: share POS_* there).
// No 5.4 surface: no Instruction.K, no isIT/testITMode, no 5.4 OP_*.
package core

/*
===========================================================================
  We assume that instructions are unsigned numbers.
  All instructions have an opcode in the first 6 bits.
  Instructions can have the following fields:
	'A' : 8 bits
	'B' : 9 bits
	'C' : 9 bits
	'Ax' : 26 bits ('A', 'B', and 'C' together)
	'Bx' : 18 bits ('B' and 'C' together)
	'sBx' : signed Bx

  A signed argument is represented in excess K; that is, the number
  value is the unsigned value minus K. K is exactly the maximum value
  for that argument (so that -max is represented by 0, and +max is
  represented by 2*max), which is half the maximum for the corresponding
  unsigned argument.

  Go: Instruction is a struct (not a packed uint32). Fields hold the same
  values GETARG_* would extract; sBx is stored biased in B (see OFFSET_sBx).
===========================================================================
*/

// Instruction is the Go counterpart of Lua's Instruction word (5.3.4).
type Instruction struct {
	Op int
	A  int
	B  int // also Bx or sBx (biased) depending on the opcode
	C  int
}

// newABC is CREATE_ABC.
func newABC(op, a, b, c int) Instruction {
	return Instruction{Op: op, A: a, B: b, C: c}
}

// newABx is CREATE_ABx.
func newABx(op, a, bx int) Instruction {
	return Instruction{Op: op, A: a, B: bx}
}

// newAsBx builds an iAsBx instruction (C uses SETARG_sBx on CREATE_ABx).
func newAsBx(op, a, sbx int) Instruction {
	return Instruction{Op: op, A: a, B: sbx + OFFSET_sBx}
}

// newAx is CREATE_Ax (Ax stored in .A; struct fold of POS_Ax).
func newAx(op, ax int) Instruction {
	return Instruction{Op: op, A: ax}
}

// Opcodes (lopcodes.h), ORDER OP — same order as PUC-Lua 5.3.4 (47 ops).
const (
	OP_MOVE = iota /* A B     R(A) := R(B) */
	OP_LOADK       /* A Bx    R(A) := Kst(Bx) */
	OP_LOADKX      /* A       R(A) := Kst(extra arg) */
	OP_LOADBOOL    /* A B C   R(A) := (Bool)B; if (C) pc++ */
	OP_LOADNIL     /* A B     R(A), R(A+1), ..., R(A+B) := nil */
	OP_GETUPVAL    /* A B     R(A) := UpValue[B] */

	OP_GETTABUP /* A B C   R(A) := UpValue[B][RK(C)] */
	OP_GETTABLE /* A B C   R(A) := R(B)[RK(C)] */

	OP_SETTABUP /* A B C   UpValue[A][RK(B)] := RK(C) */
	OP_SETUPVAL /* A B     UpValue[B] := R(A) */
	OP_SETTABLE /* A B C   R(A)[RK(B)] := RK(C) */

	OP_NEWTABLE /* A B C   R(A) := {} (size = B,C) */

	OP_SELF /* A B C   R(A+1) := R(B); R(A) := R(B)[RK(C)] */

	OP_ADD  /* A B C   R(A) := RK(B) + RK(C) */
	OP_SUB  /* A B C   R(A) := RK(B) - RK(C) */
	OP_MUL  /* A B C   R(A) := RK(B) * RK(C) */
	OP_MOD  /* A B C   R(A) := RK(B) % RK(C) */
	OP_POW  /* A B C   R(A) := RK(B) ^ RK(C) */
	OP_DIV  /* A B C   R(A) := RK(B) / RK(C) */
	OP_IDIV /* A B C   R(A) := RK(B) // RK(C) */
	OP_BAND /* A B C   R(A) := RK(B) & RK(C) */
	OP_BOR  /* A B C   R(A) := RK(B) | RK(C) */
	OP_BXOR /* A B C   R(A) := RK(B) ~ RK(C) */
	OP_SHL  /* A B C   R(A) := RK(B) << RK(C) */
	OP_SHR  /* A B C   R(A) := RK(B) >> RK(C) */
	OP_UNM  /* A B     R(A) := -R(B) */
	OP_BNOT /* A B     R(A) := ~R(B) */
	OP_NOT  /* A B     R(A) := not R(B) */
	OP_LEN  /* A B     R(A) := length of R(B) */

	OP_CONCAT /* A B C   R(A) := R(B).. ... ..R(C) */

	OP_JMP /* A sBx   pc+=sBx; if (A) close all upvalues >= R(A - 1) */
	OP_EQ  /* A B C   if ((RK(B) == RK(C)) ~= A) then pc++ */
	OP_LT  /* A B C   if ((RK(B) <  RK(C)) ~= A) then pc++ */
	OP_LE  /* A B C   if ((RK(B) <= RK(C)) ~= A) then pc++ */

	OP_TEST    /* A C     if not (R(A) <=> C) then pc++ */
	OP_TESTSET /* A B C   if (R(B) <=> C) then R(A) := R(B) else pc++ */

	OP_CALL     /* A B C   R(A), ... ,R(A+C-2) := R(A)(R(A+1), ... ,R(A+B-1)) */
	OP_TAILCALL /* A B C   return R(A)(R(A+1), ... ,R(A+B-1)) */
	OP_RETURN   /* A B     return R(A), ... ,R(A+B-2) (see note) */

	OP_FORLOOP /* A sBx   R(A)+=R(A+2); if R(A) <?= R(A+1) then { pc+=sBx; R(A+3)=R(A) } */
	OP_FORPREP /* A sBx   R(A)-=R(A+2); pc+=sBx */

	OP_TFORCALL /* A C     R(A+3), ... ,R(A+2+C) := R(A)(R(A+1), R(A+2)) */
	OP_TFORLOOP /* A sBx   if R(A+1) ~= nil then { R(A)=R(A+1); pc += sBx } */

	OP_SETLIST /* A B C   R(A)[(C-1)*FPF+i] := R(A+i), 1 <= i <= B */

	OP_CLOSURE /* A Bx    R(A) := closure(KPROTO[Bx]) */

	OP_VARARG /* A B     R(A), R(A+1), ..., R(A+B-2) = vararg */

	OP_EXTRAARG /* Ax     extra (larger) argument for previous opcode */

	NUM_OPCODES // = OP_EXTRAARG+1 = 47
)

/*
  Notes (lopcodes.h):
  (*) In OP_CALL, if (B == 0) then B = top. If (C == 0), then 'top' is
  set to last_result+1, so next open instruction (OP_CALL, OP_RETURN,
  OP_SETLIST) may use 'top'.

  (*) In OP_VARARG, if (B == 0) then use actual number of varargs and
  set top (like in OP_CALL with C == 0).

  (*) In OP_RETURN, if (B == 0) then return up to 'top'.

  (*) In OP_SETLIST, if (B == 0) then B = 'top'; if (C == 0) then next
  'instruction' is EXTRAARG(real C).

  (*) In OP_LOADKX, the next 'instruction' is always EXTRAARG.

  (*) For comparisons, A specifies what condition the test should accept
  (true or false).

  (*) All 'skips' (pc++) assume that next instruction is a jump.
*/

// opNames is luaP_opnames (without C's trailing NULL sentinel).
var opNames = [NUM_OPCODES]string{
	"MOVE",
	"LOADK",
	"LOADKX",
	"LOADBOOL",
	"LOADNIL",
	"GETUPVAL",
	"GETTABUP",
	"GETTABLE",
	"SETTABUP",
	"SETUPVAL",
	"SETTABLE",
	"NEWTABLE",
	"SELF",
	"ADD",
	"SUB",
	"MUL",
	"MOD",
	"POW",
	"DIV",
	"IDIV",
	"BAND",
	"BOR",
	"BXOR",
	"SHL",
	"SHR",
	"UNM",
	"BNOT",
	"NOT",
	"LEN",
	"CONCAT",
	"JMP",
	"EQ",
	"LT",
	"LE",
	"TEST",
	"TESTSET",
	"CALL",
	"TAILCALL",
	"RETURN",
	"FORLOOP",
	"FORPREP",
	"TFORCALL",
	"TFORLOOP",
	"SETLIST",
	"CLOSURE",
	"VARARG",
	"EXTRAARG",
}

func opName(op int) string {
	if op >= 0 && op < NUM_OPCODES {
		return opNames[op]
	}
	return "???"
}

/*
** size and position of opcode arguments (lopcodes.h).
** 5.3 layout in a packed word: OP:6 A:8 C:9 B:9 (Bx=C+B, Ax=C+B+A).
*/
const (
	SIZE_C  = 9
	SIZE_B  = 9
	SIZE_Bx = SIZE_C + SIZE_B // 18
	SIZE_A  = 8
	SIZE_Ax = SIZE_C + SIZE_B + SIZE_A // 26
	SIZE_OP = 6

	POS_OP = 0
	POS_A  = POS_OP + SIZE_OP // 6
	POS_C  = POS_A + SIZE_A   // 14
	POS_B  = POS_C + SIZE_C   // 23
	POS_Bx = POS_C            // 14
	POS_Ax = POS_A            // 6

	MAXARG_Bx  = (1 << SIZE_Bx) - 1 // 262143
	MAXARG_sBx = MAXARG_Bx >> 1     // 131071
	MAXARG_Ax  = (1 << SIZE_Ax) - 1 // 67108863
	MAXARG_A   = (1 << SIZE_A) - 1  // 255
	MAXARG_B   = (1 << SIZE_B) - 1  // 511
	MAXARG_C   = (1 << SIZE_C) - 1  // 511

	// OFFSET_sBx is the excess-K bias for sBx (C: GETARG_sBx uses MAXARG_sBx).
	OFFSET_sBx = MAXARG_sBx

	// BITRK: high bit of B/C means "constant" (ISK / INDEXK / RKASK).
	BITRK      = 1 << (SIZE_B - 1) // 256
	MAXINDEXRK = BITRK - 1         // 255
)

// NO_REG is an invalid register that fits in 8 bits (MAXARG_A).
const NO_REG = MAXARG_A

// LFIELDS_PER_FLUSH is items to accumulate before a SETLIST instruction.
const LFIELDS_PER_FLUSH = 50

// MAXUPVAL is the maximum number of upvalues in a closure (C: lfunc.h).
const MAXUPVAL = 255

// sBx is GETARG_sBx: unsigned Bx minus excess-K bias.
func sBx(b int) int { return b - OFFSET_sBx }

// ISK tests whether an RK value is a constant (C returns bitmask; Go returns bool).
func ISK(x int) bool { return x&BITRK != 0 }

// INDEXK gets the constant index from an RK value.
func INDEXK(r int) int { return r & ^BITRK }

// RKASK codes a constant index as an RK value.
func RKASK(x int) int { return x | BITRK }

// OpMode — basic instruction format (lopcodes.h).
const (
	iABC = iota
	iABx
	iAsBx
	iAx
)

// OpArgMask — argument modes (lopcodes.h).
const (
	OpArgN = iota // argument is not used
	OpArgU        // argument is used
	OpArgR        // argument is a register or a jump offset
	OpArgK        // argument is a constant or register/constant
)

// opmode packs (t<<7)|(a<<6)|(b<<4)|(c<<2)|m — same as lopcodes.c.
func opmode(t, a, b, c, m int) byte {
	return byte((t << 7) | (a << 6) | (b << 4) | (c << 2) | m)
}

// opModes is luaP_opmodes — byte-identical to PUC-Lua 5.3.4.
var opModes = [NUM_OPCODES]byte{
	/*       T  A    B       C     mode		   opcode	*/
	opmode(0, 1, OpArgR, OpArgN, iABC),  /* OP_MOVE */
	opmode(0, 1, OpArgK, OpArgN, iABx),  /* OP_LOADK */
	opmode(0, 1, OpArgN, OpArgN, iABx),  /* OP_LOADKX */
	opmode(0, 1, OpArgU, OpArgU, iABC),  /* OP_LOADBOOL */
	opmode(0, 1, OpArgU, OpArgN, iABC),  /* OP_LOADNIL */
	opmode(0, 1, OpArgU, OpArgN, iABC),  /* OP_GETUPVAL */
	opmode(0, 1, OpArgU, OpArgK, iABC),  /* OP_GETTABUP */
	opmode(0, 1, OpArgR, OpArgK, iABC),  /* OP_GETTABLE */
	opmode(0, 0, OpArgK, OpArgK, iABC),  /* OP_SETTABUP */
	opmode(0, 0, OpArgU, OpArgN, iABC),  /* OP_SETUPVAL */
	opmode(0, 0, OpArgK, OpArgK, iABC),  /* OP_SETTABLE */
	opmode(0, 1, OpArgU, OpArgU, iABC),  /* OP_NEWTABLE */
	opmode(0, 1, OpArgR, OpArgK, iABC),  /* OP_SELF */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_ADD */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_SUB */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_MUL */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_MOD */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_POW */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_DIV */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_IDIV */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_BAND */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_BOR */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_BXOR */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_SHL */
	opmode(0, 1, OpArgK, OpArgK, iABC),  /* OP_SHR */
	opmode(0, 1, OpArgR, OpArgN, iABC),  /* OP_UNM */
	opmode(0, 1, OpArgR, OpArgN, iABC),  /* OP_BNOT */
	opmode(0, 1, OpArgR, OpArgN, iABC),  /* OP_NOT */
	opmode(0, 1, OpArgR, OpArgN, iABC),  /* OP_LEN */
	opmode(0, 1, OpArgR, OpArgR, iABC),  /* OP_CONCAT */
	opmode(0, 0, OpArgR, OpArgN, iAsBx), /* OP_JMP */
	opmode(1, 0, OpArgK, OpArgK, iABC),  /* OP_EQ */
	opmode(1, 0, OpArgK, OpArgK, iABC),  /* OP_LT */
	opmode(1, 0, OpArgK, OpArgK, iABC),  /* OP_LE */
	opmode(1, 0, OpArgN, OpArgU, iABC),  /* OP_TEST */
	opmode(1, 1, OpArgR, OpArgU, iABC),  /* OP_TESTSET */
	opmode(0, 1, OpArgU, OpArgU, iABC),  /* OP_CALL */
	opmode(0, 1, OpArgU, OpArgU, iABC),  /* OP_TAILCALL */
	opmode(0, 0, OpArgU, OpArgN, iABC),  /* OP_RETURN */
	opmode(0, 1, OpArgR, OpArgN, iAsBx), /* OP_FORLOOP */
	opmode(0, 1, OpArgR, OpArgN, iAsBx), /* OP_FORPREP */
	opmode(0, 0, OpArgN, OpArgU, iABC),  /* OP_TFORCALL */
	opmode(0, 1, OpArgR, OpArgN, iAsBx), /* OP_TFORLOOP */
	opmode(0, 0, OpArgU, OpArgU, iABC),  /* OP_SETLIST */
	opmode(0, 1, OpArgU, OpArgN, iABx),  /* OP_CLOSURE */
	opmode(0, 1, OpArgU, OpArgN, iABC),  /* OP_VARARG */
	opmode(0, 0, OpArgU, OpArgU, iAx),   /* OP_EXTRAARG */
}

func getOpMode(m int) int { return int(opModes[m] & 3) }
func getBMode(m int) int  { return int((opModes[m] >> 4) & 3) }
func getCMode(m int) int  { return int((opModes[m] >> 2) & 3) }

// testAMode is lopcodes.h testAMode (bit 6: instruction sets register A).
func testAMode(m int) bool { return opModes[m]&(1<<6) != 0 }

// testTMode is lopcodes.h testTMode (bit 7: operator is a test).
func testTMode(m int) bool { return opModes[m]&(1<<7) != 0 }
