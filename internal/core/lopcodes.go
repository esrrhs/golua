// Translation of lopcodes.h / lopcodes.c / lopnames.h (PUC-Lua 5.4.9).
//
// PUC-Lua packs an instruction into a 32-bit word. Here an instruction is
// a plain struct: the fields are the same (Op, A, B, C, K); B doubles as
// Bx/sBx/sJ depending on the opcode. Only the bit packing is gone.
//
// Intentional OK vs C: Instruction is a struct, not a packed uint32, so
// POS_*/MASK*/GET_*/SET_* bit-pack macros are omitted (field access instead).
// Pack/unpack for dump/undump lives in ldump/lundump (Apply debt: can share
// SIZE_*/POS_* from here).
package core

// Instruction is the Go counterpart of Lua's Instruction word (5.4.9).
// PUC-Lua packs Op/A/B/C/k into a 32-bit word; here they are plain fields
// (K is the 'k' bit).
type Instruction struct {
	Op int
	A  int
	B  int // also Bx, sBx (biased) or sJ (biased) depending on the opcode
	C  int
	K  int // the 'k' bit (0 or 1)
}

// newABC is CREATE_ABCk.
func newABC(op, a, b, c, k int) Instruction {
	return Instruction{Op: op, A: a, B: b, C: c, K: k}
}

// newABx is CREATE_ABx (Bx stored in .B).
func newABx(op, a, bx int) Instruction {
	return Instruction{Op: op, A: a, B: bx}
}

// newAsBx builds an iAsBx instruction (C: SETARG_sBx on CREATE_ABx).
func newAsBx(op, a, sbx int) Instruction {
	return Instruction{Op: op, A: a, B: sbx + OFFSET_sBx}
}

// newAx is CREATE_Ax (Ax stored in .A; struct fold of POS_Ax).
func newAx(op, ax int) Instruction {
	return Instruction{Op: op, A: ax}
}

// newSJ is CREATE_sJ (sJ biased value in .B; k in .K).
func newSJ(op, sj, k int) Instruction {
	return Instruction{Op: op, B: sj + OFFSET_sJ, K: k}
}

/*
===========================================================================
  We assume that instructions are unsigned 32-bit integers.
  All instructions have an opcode in the first 7 bits.
  Instructions can have the following formats:

        3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1 0 0 0 0 0 0 0 0 0 0
        1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
iABC          C(8)     |      B(8)     |k|     A(8)      |   Op(7)     |
iABx                Bx(17)               |     A(8)      |   Op(7)     |
iAsBx              sBx (signed)(17)      |     A(8)      |   Op(7)     |
iAx                           Ax(25)                     |   Op(7)     |
isJ                           sJ (signed)(25)            |   Op(7)     |

  A signed argument is represented in excess K: the represented value is
  the written unsigned value minus K, where K is half the maximum for the
  corresponding unsigned argument.
===========================================================================
*/

// OpMode — basic instruction formats (lopcodes.h enum OpMode).
const (
	iABC = iota
	iABx
	iAsBx
	iAx
	isJ
)

/*
** Grep "ORDER OP" if you change these enums. Opcodes marked with a (*)
** has extra descriptions in the notes after the enumeration.
*/
const (
	/*----------------------------------------------------------------------
	  name		args	description
	------------------------------------------------------------------------*/
	OP_MOVE = iota /*	A B	R[A] := R[B]					*/
	OP_LOADI       /*	A sBx	R[A] := sBx					*/
	OP_LOADF       /*	A sBx	R[A] := (lua_Number)sBx				*/
	OP_LOADK       /*	A Bx	R[A] := K[Bx]					*/
	OP_LOADKX      /*	A	R[A] := K[extra arg]				*/
	OP_LOADFALSE   /*	A	R[A] := false					*/
	OP_LFALSESKIP  /*	A	R[A] := false; pc++	(*)			*/
	OP_LOADTRUE    /*	A	R[A] := true					*/
	OP_LOADNIL     /*	A B	R[A], R[A+1], ..., R[A+B] := nil		*/
	OP_GETUPVAL    /*	A B	R[A] := UpValue[B]				*/
	OP_SETUPVAL    /*	A B	UpValue[B] := R[A]				*/

	OP_GETTABUP /*	A B C	R[A] := UpValue[B][K[C]:shortstring]		*/
	OP_GETTABLE /*	A B C	R[A] := R[B][R[C]]				*/
	OP_GETI     /*	A B C	R[A] := R[B][C]					*/
	OP_GETFIELD /*	A B C	R[A] := R[B][K[C]:shortstring]			*/

	OP_SETTABUP /*	A B C	UpValue[A][K[B]:shortstring] := RK(C)		*/
	OP_SETTABLE /*	A B C	R[A][R[B]] := RK(C)				*/
	OP_SETI     /*	A B C	R[A][B] := RK(C)				*/
	OP_SETFIELD /*	A B C	R[A][K[B]:shortstring] := RK(C)			*/

	OP_NEWTABLE /*	A B C k	R[A] := {}					*/

	OP_SELF /*	A B C	R[A+1] := R[B]; R[A] := R[B][RK(C):string]	*/

	OP_ADDI /*	A B sC	R[A] := R[B] + sC				*/

	OP_ADDK  /*	A B C	R[A] := R[B] + K[C]:number			*/
	OP_SUBK  /*	A B C	R[A] := R[B] - K[C]:number			*/
	OP_MULK  /*	A B C	R[A] := R[B] * K[C]:number			*/
	OP_MODK  /*	A B C	R[A] := R[B] % K[C]:number			*/
	OP_POWK  /*	A B C	R[A] := R[B] ^ K[C]:number			*/
	OP_DIVK  /*	A B C	R[A] := R[B] / K[C]:number			*/
	OP_IDIVK /*	A B C	R[A] := R[B] // K[C]:number			*/

	OP_BANDK /*	A B C	R[A] := R[B] & K[C]:integer			*/
	OP_BORK  /*	A B C	R[A] := R[B] | K[C]:integer			*/
	OP_BXORK /*	A B C	R[A] := R[B] ~ K[C]:integer			*/

	OP_SHRI /*	A B sC	R[A] := R[B] >> sC				*/
	OP_SHLI /*	A B sC	R[A] := sC << R[B]				*/

	OP_ADD  /*	A B C	R[A] := R[B] + R[C]				*/
	OP_SUB  /*	A B C	R[A] := R[B] - R[C]				*/
	OP_MUL  /*	A B C	R[A] := R[B] * R[C]				*/
	OP_MOD  /*	A B C	R[A] := R[B] % R[C]				*/
	OP_POW  /*	A B C	R[A] := R[B] ^ R[C]				*/
	OP_DIV  /*	A B C	R[A] := R[B] / R[C]				*/
	OP_IDIV /*	A B C	R[A] := R[B] // R[C]				*/

	OP_BAND /*	A B C	R[A] := R[B] & R[C]				*/
	OP_BOR  /*	A B C	R[A] := R[B] | R[C]				*/
	OP_BXOR /*	A B C	R[A] := R[B] ~ R[C]				*/
	OP_SHL  /*	A B C	R[A] := R[B] << R[C]				*/
	OP_SHR  /*	A B C	R[A] := R[B] >> R[C]				*/

	OP_MMBIN  /*	A B C	call C metamethod over R[A] and R[B]	(*)	*/
	OP_MMBINI /*	A sB C k	call C metamethod over R[A] and sB	*/
	OP_MMBINK /*	A B C k		call C metamethod over R[A] and K[B]	*/

	OP_UNM  /*	A B	R[A] := -R[B]					*/
	OP_BNOT /*	A B	R[A] := ~R[B]					*/
	OP_NOT  /*	A B	R[A] := not R[B]				*/
	OP_LEN  /*	A B	R[A] := #R[B] (length operator)			*/

	OP_CONCAT /*	A B	R[A] := R[A].. ... ..R[A + B - 1]		*/

	OP_CLOSE /*	A	close all upvalues >= R[A]			*/
	OP_TBC   /*	A	mark variable A "to be closed"			*/
	OP_JMP   /*	sJ	pc += sJ					*/
	OP_EQ    /*	A B k	if ((R[A] == R[B]) ~= k) then pc++		*/
	OP_LT    /*	A B k	if ((R[A] <  R[B]) ~= k) then pc++		*/
	OP_LE    /*	A B k	if ((R[A] <= R[B]) ~= k) then pc++		*/

	OP_EQK /*	A B k	if ((R[A] == K[B]) ~= k) then pc++		*/
	OP_EQI /*	A sB k	if ((R[A] == sB) ~= k) then pc++		*/
	OP_LTI /*	A sB k	if ((R[A] < sB) ~= k) then pc++			*/
	OP_LEI /*	A sB k	if ((R[A] <= sB) ~= k) then pc++		*/
	OP_GTI /*	A sB k	if ((R[A] > sB) ~= k) then pc++			*/
	OP_GEI /*	A sB k	if ((R[A] >= sB) ~= k) then pc++		*/

	OP_TEST    /*	A k	if (not R[A] == k) then pc++			*/
	OP_TESTSET /*	A B k	if (not R[B] == k) then pc++ else R[A] := R[B] (*) */

	OP_CALL     /*	A B C	R[A], ... ,R[A+C-2] := R[A](R[A+1], ... ,R[A+B-1]) */
	OP_TAILCALL /*	A B C k	return R[A](R[A+1], ... ,R[A+B-1])		*/

	OP_RETURN  /*	A B C k	return R[A], ... ,R[A+B-2]	(see note)	*/
	OP_RETURN0 /*		return						*/
	OP_RETURN1 /*	A	return R[A]					*/

	OP_FORLOOP /*	A Bx	update counters; if loop continues then pc-=Bx; */
	OP_FORPREP /*	A Bx	<check values and prepare counters>;
			if not to run then pc+=Bx+1;			*/

	OP_TFORPREP /*	A Bx	create upvalue for R[A + 3]; pc+=Bx		*/
	OP_TFORCALL /*	A C	R[A+4], ... ,R[A+3+C] := R[A](R[A+1], R[A+2]);	*/
	OP_TFORLOOP /*	A Bx	if R[A+2] ~= nil then { R[A]=R[A+2]; pc -= Bx }	*/

	OP_SETLIST /*	A B C k	R[A][C+i] := R[A+i], 1 <= i <= B		*/

	OP_CLOSURE /*	A Bx	R[A] := closure(KPROTO[Bx])			*/

	OP_VARARG /*	A C	R[A], R[A+1], ..., R[A+C-2] = vararg		*/

	OP_VARARGPREP /*	A	(adjust vararg parameters)			*/

	OP_EXTRAARG /*	Ax	extra (larger) argument for previous opcode	*/

	NUM_OPCODES // = OP_EXTRAARG+1 = 83
)

/*
===========================================================================
  Notes (lopcodes.h):

  (*) Opcode OP_LFALSESKIP is used to convert a condition to a boolean
  value, in a code equivalent to (not cond ? false : true).  (It
  produces false and skips the next instruction producing true.)

  (*) Opcodes OP_MMBIN and variants follow each arithmetic and
  bitwise opcode. If the operation succeeds, it skips this next
  opcode. Otherwise, this opcode calls the corresponding metamethod.

  (*) Opcode OP_TESTSET is used in short-circuit expressions that need
  both to jump and to produce a value, such as (a = b or c).

  (*) In OP_CALL, if (B == 0) then B = top - A. If (C == 0), then
  'top' is set to last_result+1, so next open instruction (OP_CALL,
  OP_RETURN*, OP_SETLIST) may use 'top'.

  (*) In OP_VARARG, if (C == 0) then use actual number of varargs and
  set top (like in OP_CALL with C == 0).

  (*) In OP_RETURN, if (B == 0) then return up to 'top'.

  (*) In OP_LOADKX and OP_NEWTABLE, the next instruction is always
  OP_EXTRAARG.

  (*) In OP_SETLIST, if (B == 0) then real B = 'top'; if k, then
  real C = EXTRAARG _ C (the bits of EXTRAARG concatenated with the
  bits of C).

  (*) In OP_NEWTABLE, B is log2 of the hash size (which is always a
  power of 2) plus 1, or zero for size zero. If not k, the array size
  is C. Otherwise, the array size is EXTRAARG _ C.

  (*) For comparisons, k specifies what condition the test should accept
  (true or false).

  (*) In OP_MMBINI/OP_MMBINK, k means the arguments were flipped
   (the constant is the first operand).

  (*) All 'skips' (pc++) assume that next instruction is a jump.

  (*) In instructions OP_RETURN/OP_TAILCALL, 'k' specifies that the
  function builds upvalues, which may need to be closed. C > 0 means
  the function is vararg, so that its 'func' must be corrected before
  returning; in this case, (C - 1) is its number of fixed parameters.

  (*) In comparisons with an immediate operand, C signals whether the
  original operand was a float. (It must be corrected in case of
  metamethods.)

===========================================================================
*/

// opNames is the translation of lopnames.h (ORDER OP; no C trailing NULL).
var opNames = [NUM_OPCODES]string{
	"MOVE",
	"LOADI",
	"LOADF",
	"LOADK",
	"LOADKX",
	"LOADFALSE",
	"LFALSESKIP",
	"LOADTRUE",
	"LOADNIL",
	"GETUPVAL",
	"SETUPVAL",
	"GETTABUP",
	"GETTABLE",
	"GETI",
	"GETFIELD",
	"SETTABUP",
	"SETTABLE",
	"SETI",
	"SETFIELD",
	"NEWTABLE",
	"SELF",
	"ADDI",
	"ADDK",
	"SUBK",
	"MULK",
	"MODK",
	"POWK",
	"DIVK",
	"IDIVK",
	"BANDK",
	"BORK",
	"BXORK",
	"SHRI",
	"SHLI",
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
	"MMBIN",
	"MMBINI",
	"MMBINK",
	"UNM",
	"BNOT",
	"NOT",
	"LEN",
	"CONCAT",
	"CLOSE",
	"TBC",
	"JMP",
	"EQ",
	"LT",
	"LE",
	"EQK",
	"EQI",
	"LTI",
	"LEI",
	"GTI",
	"GEI",
	"TEST",
	"TESTSET",
	"CALL",
	"TAILCALL",
	"RETURN",
	"RETURN0",
	"RETURN1",
	"FORLOOP",
	"FORPREP",
	"TFORPREP",
	"TFORCALL",
	"TFORLOOP",
	"SETLIST",
	"CLOSURE",
	"VARARG",
	"VARARGPREP",
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
** With struct Instruction, POS_* are unused for field access but kept
** for dump/undump bit layout documentation and Apply debt consolidation.
*/
const (
	SIZE_C  = 8
	SIZE_B  = 8
	SIZE_Bx = SIZE_C + SIZE_B + 1 // 17
	SIZE_A  = 8
	SIZE_Ax = SIZE_Bx + SIZE_A // 25
	SIZE_sJ = SIZE_Bx + SIZE_A // 25
	SIZE_OP = 7

	POS_OP = 0
	POS_A  = POS_OP + SIZE_OP // 7
	POS_k  = POS_A + SIZE_A   // 15
	POS_B  = POS_k + 1        // 16
	POS_C  = POS_B + SIZE_B   // 24
	POS_Bx = POS_k            // 15
	POS_Ax = POS_A            // 7
	POS_sJ = POS_A            // 7

	MAXARG_Bx = (1 << SIZE_Bx) - 1 // 131071
	OFFSET_sBx = MAXARG_Bx >> 1    // 65535

	MAXARG_Ax = (1 << SIZE_Ax) - 1 // 33554431
	MAXARG_sJ = (1 << SIZE_sJ) - 1 // 33554431
	OFFSET_sJ = MAXARG_sJ >> 1     // 16777215

	MAXARG_A = (1 << SIZE_A) - 1 // 255
	MAXARG_B = (1 << SIZE_B) - 1 // 255
	MAXARG_C = (1 << SIZE_C) - 1 // 255
	OFFSET_sC = MAXARG_C >> 1    // 127

	// Convenience aliases used by the rest of the port (GETARG_sB uses
	// sC2int / OFFSET_sC in C; MAXARG_sBx is OFFSET_sBx by another name).
	MAXARG_sC  = OFFSET_sC
	MAXARG_sB  = OFFSET_sC
	OFFSET_sB  = OFFSET_sC
	MAXARG_sBx = OFFSET_sBx

	// MAXINDEXRK (for debugging only) — lopcodes.h.
	MAXINDEXRK = MAXARG_B
)

// NO_REG is an invalid register that fits in 8 bits (lopcodes.h).
const NO_REG = MAXARG_A

// LFIELDS_PER_FLUSH is the number of list items to accumulate before a
// SETLIST instruction (lopcodes.h).
const LFIELDS_PER_FLUSH = 50

// MAXUPVAL is the maximum number of upvalues in a closure (C: lfunc.h;
// kept here as in current golua — Apply debt: may move to lfunc).
const MAXUPVAL = 255

// int2sC converts an integer into an 'sC' operand (lopcodes.h).
func int2sC(i int) int { return i + OFFSET_sC }

// sC2int converts an 'sC' operand back to a signed integer (lopcodes.h).
func sC2int(i int) int { return i - OFFSET_sC }

// sBx is GETARG_sBx: unsigned Bx minus excess-K bias.
func sBx(b int) int { return b - OFFSET_sBx }

// sJ is GETARG_sJ: unsigned sJ field minus excess-K bias.
func sJ(b int) int { return b - OFFSET_sJ }

/*
** masks for instruction properties. The format is:
** bits 0-2: op mode
** bit 3: instruction set register A
** bit 4: operator is a test (next instruction must be a jump)
** bit 5: instruction uses 'L->top' set by previous instruction (when B == 0)
** bit 6: instruction sets 'L->top' for next instruction (when C == 0)
** bit 7: instruction is an MM instruction (call a metamethod)
*/

// opmode packs (mm<<7)|(ot<<6)|(it<<5)|(t<<4)|(a<<3)|m — same as lopcodes.c.
func opmode(mm, ot, it, t, a, m int) byte {
	return byte((mm << 7) | (ot << 6) | (it << 5) | (t << 4) | (a << 3) | m)
}

// opModes is luaP_opmodes — byte-identical to PUC-Lua 5.4.9 lopcodes.c.
// ORDER OP
var opModes = [NUM_OPCODES]byte{
	/*       MM OT IT T  A  mode		   opcode  */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_MOVE */
	opmode(0, 0, 0, 0, 1, iAsBx), /* OP_LOADI */
	opmode(0, 0, 0, 0, 1, iAsBx), /* OP_LOADF */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_LOADK */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_LOADKX */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_LOADFALSE */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_LFALSESKIP */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_LOADTRUE */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_LOADNIL */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_GETUPVAL */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_SETUPVAL */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_GETTABUP */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_GETTABLE */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_GETI */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_GETFIELD */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_SETTABUP */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_SETTABLE */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_SETI */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_SETFIELD */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_NEWTABLE */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SELF */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_ADDI */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_ADDK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SUBK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_MULK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_MODK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_POWK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_DIVK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_IDIVK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BANDK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BORK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BXORK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SHRI */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SHLI */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_ADD */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SUB */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_MUL */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_MOD */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_POW */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_DIV */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_IDIV */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BAND */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BOR */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BXOR */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SHL */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_SHR */
	opmode(1, 0, 0, 0, 0, iABC),  /* OP_MMBIN */
	opmode(1, 0, 0, 0, 0, iABC),  /* OP_MMBINI */
	opmode(1, 0, 0, 0, 0, iABC),  /* OP_MMBINK */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_UNM */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_BNOT */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_NOT */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_LEN */
	opmode(0, 0, 0, 0, 1, iABC),  /* OP_CONCAT */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_CLOSE */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_TBC */
	opmode(0, 0, 0, 0, 0, isJ),   /* OP_JMP */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_EQ */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_LT */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_LE */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_EQK */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_EQI */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_LTI */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_LEI */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_GTI */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_GEI */
	opmode(0, 0, 0, 1, 0, iABC),  /* OP_TEST */
	opmode(0, 0, 0, 1, 1, iABC),  /* OP_TESTSET */
	opmode(0, 1, 1, 0, 1, iABC),  /* OP_CALL */
	opmode(0, 1, 1, 0, 1, iABC),  /* OP_TAILCALL */
	opmode(0, 0, 1, 0, 0, iABC),  /* OP_RETURN */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_RETURN0 */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_RETURN1 */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_FORLOOP */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_FORPREP */
	opmode(0, 0, 0, 0, 0, iABx),  /* OP_TFORPREP */
	opmode(0, 0, 0, 0, 0, iABC),  /* OP_TFORCALL */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_TFORLOOP */
	opmode(0, 0, 1, 0, 0, iABC),  /* OP_SETLIST */
	opmode(0, 0, 0, 0, 1, iABx),  /* OP_CLOSURE */
	opmode(0, 1, 0, 0, 1, iABC),  /* OP_VARARG */
	opmode(0, 0, 1, 0, 1, iABC),  /* OP_VARARGPREP */
	opmode(0, 0, 0, 0, 0, iAx),   /* OP_EXTRAARG */
}

func getOpMode(m int) int { return int(opModes[m] & 7) }

// testAMode is lopcodes.h testAMode (bit 3: instruction sets register A).
func testAMode(m int) bool { return opModes[m]&(1<<3) != 0 }

// testTMode is lopcodes.h testTMode (bit 4: operator is a test).
func testTMode(m int) bool { return opModes[m]&(1<<4) != 0 }

// testITMode is lopcodes.h testITMode (bit 5: uses L->top when B == 0).
func testITMode(m int) bool { return opModes[m]&(1<<5) != 0 }

// testOTMode is lopcodes.h testOTMode (bit 6: sets L->top when C == 0).
func testOTMode(m int) bool { return opModes[m]&(1<<6) != 0 }

// testMMMode is lopcodes.h testMMMode (bit 7: MM instruction).
func testMMMode(m int) bool { return opModes[m]&(1<<7) != 0 }

// isOT reports whether the instruction sets top for the next instruction
// (lopcodes.h).
func isOT(i Instruction) bool {
	return (testOTMode(i.Op) && i.C == 0) || i.Op == OP_TAILCALL
}

// isIT reports whether the instruction uses L.top (lopcodes.h).
func isIT(i Instruction) bool { return testITMode(i.Op) && i.B == 0 }
