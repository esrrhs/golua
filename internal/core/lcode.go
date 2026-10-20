// Translation of lcode.h / lcode.c (PUC-Lua 5.4.9).
//
// Adaptations:
//   - instructions are structs, so GETARG_x/SETARG_x become field accesses;
//   - Proto.lineinfo holds absolute lines, so savelineinfo/removelastlineinfo
//     lose the abslineinfo machinery;
//   - luaC_barrier is gone (no GC);
//   - luaM_growvector becomes append.
package core

import "math"

// NO_JUMP marks the end of a patch list (lcode.h).
const NO_JUMP = -1

// MAXREGS is the maximum number of registers in a Lua function (must fit in
// 8 bits).
const MAXREGS = 255

// hasjumps reports whether an expression has pending jumps.
func hasjumps(e *expdesc) bool { return e.t != e.f }

// BinOpr enumerates binary operators (ORDER OPR).
type BinOpr int

const (
	/* arithmetic operators */
	OPR_ADD BinOpr = iota
	OPR_SUB
	OPR_MUL
	OPR_MOD
	OPR_POW
	OPR_DIV
	OPR_IDIV
	/* bitwise operators */
	OPR_BAND
	OPR_BOR
	OPR_BXOR
	OPR_SHL
	OPR_SHR
	/* string operator */
	OPR_CONCAT
	/* comparison operators */
	OPR_EQ
	OPR_LT
	OPR_LE
	OPR_NE
	OPR_GT
	OPR_GE
	/* logical operators */
	OPR_AND
	OPR_OR
	OPR_NOBINOPR
)

// foldbinop reports whether an operation is foldable (arithmetic or bitwise).
func foldbinop(op BinOpr) bool { return op <= OPR_SHR }

// UnOpr enumerates unary operators.
type UnOpr int

const (
	OPR_MINUS UnOpr = iota
	OPR_BNOT
	OPR_NOT
	OPR_LEN
	OPR_NOUNOPR
)

// ---------------------------------------------------------------------
// constants
// ---------------------------------------------------------------------

// luaK_semerror raises a semantic error (no "near <token>" in the message).
func luaK_semerror(ls *LexState, msg string) {
	ls.t.token = 0 /* remove "near <token>" from final message */
	luaX_syntaxerror(ls, msg)
}

// tonumeral fills v with the value of a numeric constant expression.
func tonumeral(e *expdesc, v *Value) bool {
	if hasjumps(e) {
		return false /* not a numeral */
	}
	switch e.k {
	case VKINT:
		if v != nil {
			setivalue(v, e.ival)
		}
		return true
	case VKFLT:
		if v != nil {
			setfltvalue(v, e.nval)
		}
		return true
	}
	return false
}

// const2val returns the value of a <const> expression.
func const2val(fs *FuncState, e *expdesc) *Value {
	return &fs.ls.dyd.actvar.arr[e.info].k
}

// luaK_exp2const fills v with the value of a constant expression.
func luaK_exp2const(fs *FuncState, e *expdesc, v *Value) bool {
	if hasjumps(e) {
		return false /* not a constant */
	}
	switch e.k {
	case VFALSE:
		setbfvalue(v)
		return true
	case VTRUE:
		setbtvalue(v)
		return true
	case VNIL:
		setnilvalue(v)
		return true
	case VKSTR:
		setsvalue(v, e.strval)
		return true
	case VCONST:
		setobj(v, const2val(fs, e))
		return true
	}
	return tonumeral(e, v)
}

// ---------------------------------------------------------------------
// instructions
// ---------------------------------------------------------------------

// previousinstruction returns the index of the previous instruction, or -1
// when there may be a jump target in between (to avoid wrong optimizations).
func previousinstruction(fs *FuncState) int {
	if fs.pc > fs.lasttarget {
		return fs.pc - 1
	}
	return -1
}

// luaK_nil codes OP_LOADNIL, merging with a previous compatible one.
func luaK_nil(fs *FuncState, from, n int) {
	l := from + n - 1 /* last register to set nil */
	if previous := previousinstruction(fs); previous >= 0 && fs.f.code[previous].Op == OP_LOADNIL {
		pfrom := fs.f.code[previous].A
		pl := pfrom + fs.f.code[previous].B
		if (pfrom <= from && from <= pl+1) || (from <= pfrom && pfrom <= l+1) {
			if pfrom < from {
				from = pfrom /* from = min(from, pfrom) */
			}
			if pl > l {
				l = pl /* l = max(l, pl) */
			}
			fs.f.code[previous].A = from
			fs.f.code[previous].B = l - from
			return
		}
	}
	luaK_codeABC(fs, OP_LOADNIL, from, n-1, 0)
}

// getjump returns the destination of the jump at 'pc'.
func getjump(fs *FuncState, pc int) int {
	offset := fs.f.code[pc].B - OFFSET_sJ
	if offset == NO_JUMP { /* point to itself represents end of list */
		return NO_JUMP
	}
	return (pc + 1) + offset
}

// fixjump fixes the jump at 'pc' to jump to 'dest'.
func fixjump(fs *FuncState, pc, dest int) {
	offset := dest - (pc + 1)
	if !(-OFFSET_sJ <= offset && offset <= MAXARG_sJ-OFFSET_sJ) {
		luaX_syntaxerror(fs.ls, "control structure too long")
	}
	fs.f.code[pc].Op = OP_JMP
	fs.f.code[pc].B = offset + OFFSET_sJ
}

// luaK_concat concatenates jump-list 'l2' into jump-list 'l1'.
func luaK_concat(fs *FuncState, l1 *int, l2 int) {
	if l2 == NO_JUMP {
		return /* nothing to concatenate? */
	}
	if *l1 == NO_JUMP {
		*l1 = l2 /* 'l1' points to 'l2' */
		return
	}
	list := *l1
	for next := getjump(fs, list); next != NO_JUMP; next = getjump(fs, list) {
		list = next /* find last element */
	}
	fixjump(fs, list, l2) /* last element links to 'l2' */
}

// luaK_jump codes a jump with a pending destination.
func luaK_jump(fs *FuncState) int {
	return codesJ(fs, OP_JMP, NO_JUMP, 0)
}

// luaK_ret codes a 'return' instruction.
func luaK_ret(fs *FuncState, first, nret int) {
	op := OP_RETURN
	switch nret {
	case 0:
		op = OP_RETURN0
	case 1:
		op = OP_RETURN1
	}
	luaK_codeABC(fs, op, first, nret+1, 0)
}

// condjump codes a conditional jump (a test/comparison plus a jump).
func condjump(fs *FuncState, op, A, B, C, k int) int {
	luaK_codeABCk(fs, op, A, B, C, k)
	return luaK_jump(fs)
}

// luaK_getlabel returns the current pc and marks it as a jump target.
func luaK_getlabel(fs *FuncState) int {
	fs.lasttarget = fs.pc
	return fs.pc
}

// getjumpcontrol returns the index of the instruction controlling a jump.
func getjumpcontrol(fs *FuncState, pc int) int {
	if pc >= 1 && testTMode(fs.f.code[pc-1].Op) {
		return pc - 1
	}
	return pc
}

// patchtestreg patches the destination register of a TESTSET.
func patchtestreg(fs *FuncState, node, reg int) bool {
	i := getjumpcontrol(fs, node)
	if fs.f.code[i].Op != OP_TESTSET {
		return false /* cannot patch other instructions */
	}
	if reg != NO_REG && reg != fs.f.code[i].B {
		fs.f.code[i].A = reg
	} else {
		/* no register to put value or register already has the value;
		   change instruction to simple test */
		fs.f.code[i] = newABC(OP_TEST, fs.f.code[i].B, 0, 0, fs.f.code[i].K)
	}
	return true
}

// removevalues traverses a list of tests ensuring none produces a value.
func removevalues(fs *FuncState, list int) {
	for ; list != NO_JUMP; list = getjump(fs, list) {
		patchtestreg(fs, list, NO_REG)
	}
}

// patchlistaux traverses a list of tests, patching their destination address
// and registers.
func patchlistaux(fs *FuncState, list, vtarget, reg, dtarget int) {
	for list != NO_JUMP {
		next := getjump(fs, list)
		if patchtestreg(fs, list, reg) {
			fixjump(fs, list, vtarget)
		} else {
			fixjump(fs, list, dtarget) /* jump to default target */
		}
		list = next
	}
}

// luaK_patchlist makes all jumps in 'list' jump to 'target'.
func luaK_patchlist(fs *FuncState, list, target int) {
	patchlistaux(fs, list, target, NO_REG, target)
}

// luaK_patchtohere makes all jumps in 'list' jump to the current position.
func luaK_patchtohere(fs *FuncState, list int) {
	hr := luaK_getlabel(fs) /* mark "here" as a jump target */
	luaK_patchlist(fs, list, hr)
}

// luaK_jumpto emits a jump to 't' (lcode.h).
func luaK_jumpto(fs *FuncState, t int) {
	luaK_patchlist(fs, luaK_jump(fs), t)
}

// ---------------------------------------------------------------------
// line information
// ---------------------------------------------------------------------

// savelineinfo saves the line of the last coded instruction. PUC-Lua stores
// line deltas plus an abslineinfo array; here 'lineinfo' holds absolute
// lines, which is equivalent and much simpler.
func savelineinfo(fs *FuncState, f *Proto, line int) {
	fs.f.lineinfo = append(fs.f.lineinfo, int32(line))
	fs.previousline = line
}

// removelastlineinfo removes the line information of the last instruction.
func removelastlineinfo(fs *FuncState) {
	if n := len(fs.f.lineinfo); n > 0 {
		fs.f.lineinfo = fs.f.lineinfo[:n-1]
	}
}

// removelastinstruction removes the last instruction created.
func removelastinstruction(fs *FuncState) {
	removelastlineinfo(fs)
	fs.pc--
	if n := len(fs.f.code); n > fs.pc {
		fs.f.code = fs.f.code[:fs.pc]
	}
}

// ---------------------------------------------------------------------
// emitting instructions
// ---------------------------------------------------------------------

// luaK_code emits instruction 'i', returning its position.
func luaK_code(fs *FuncState, i Instruction) int {
	f := fs.f
	f.code = append(f.code, i)
	fs.pc++
	savelineinfo(fs, f, fs.ls.lastline)
	return fs.pc - 1
}

// luaK_codeABCk formats and emits an iABC instruction.
func luaK_codeABCk(fs *FuncState, o, a, b, c, k int) int {
	return luaK_code(fs, newABC(o, a, b, c, k))
}

// luaK_codeABC formats and emits an iABC instruction with k == 0.
func luaK_codeABC(fs *FuncState, o, a, b, c int) int {
	return luaK_codeABCk(fs, o, a, b, c, 0)
}

// luaK_codeABx formats and emits an iABx instruction.
func luaK_codeABx(fs *FuncState, o, a, bc int) int {
	return luaK_code(fs, newABC(o, a, bc, 0, 0))
}

// codeAsBx formats and emits an iAsBx instruction.
func codeAsBx(fs *FuncState, o, a, bc int) int {
	return luaK_codeABx(fs, o, a, bc+OFFSET_sBx)
}

// codesJ formats and emits an isJ instruction. PUC-Lua packs the 'sJ'
// operand in the same bit position as 'A'; here it goes in the B field,
// which is what luaV_execute, getjump and fixjump read.
func codesJ(fs *FuncState, o, sj, k int) int {
	j := sj + OFFSET_sJ
	return luaK_code(fs, newABC(o, 0, j, 0, k))
}

// codeextraarg emits an "extra argument" instruction.
func codeextraarg(fs *FuncState, a int) int {
	return luaK_code(fs, newABC(OP_EXTRAARG, a, 0, 0, 0))
}

// luaK_codek emits a "load constant" instruction.
func luaK_codek(fs *FuncState, reg, k int) int {
	if k <= MAXARG_Bx {
		return luaK_codeABx(fs, OP_LOADK, reg, k)
	}
	p := luaK_codeABx(fs, OP_LOADKX, reg, 0)
	codeextraarg(fs, k)
	return p
}

// ---------------------------------------------------------------------
// registers
// ---------------------------------------------------------------------

// luaK_checkstack checks the register-stack level.
func luaK_checkstack(fs *FuncState, n int) {
	newstack := fs.freereg + n
	if newstack > int(fs.f.maxstacksize) {
		if newstack >= MAXREGS {
			luaX_syntaxerror(fs.ls, "function or expression needs too many registers")
		}
		fs.f.maxstacksize = byte(newstack)
	}
}

// luaK_reserveregs reserves 'n' registers.
func luaK_reserveregs(fs *FuncState, n int) {
	luaK_checkstack(fs, n)
	fs.freereg += n
}

// freereg frees register 'reg', if it is neither a constant nor a local.
func freereg(fs *FuncState, reg int) {
	if reg >= luaY_nvarstack(fs) {
		fs.freereg--
	}
}

// freeregs frees two registers in the proper order.
func freeregs(fs *FuncState, r1, r2 int) {
	if r1 > r2 {
		freereg(fs, r1)
		freereg(fs, r2)
	} else {
		freereg(fs, r2)
		freereg(fs, r1)
	}
}

// freeexp frees the register used by expression 'e' (if any).
func freeexp(fs *FuncState, e *expdesc) {
	if e.k == VNONRELOC {
		freereg(fs, e.info)
	}
}

// freeexps frees the registers used by two expressions.
func freeexps(fs *FuncState, e1, e2 *expdesc) {
	r1 := -1
	if e1.k == VNONRELOC {
		r1 = e1.info
	}
	r2 := -1
	if e2.k == VNONRELOC {
		r2 = e2.info
	}
	freeregs(fs, r1, r2)
}

// ---------------------------------------------------------------------
// constants table
// ---------------------------------------------------------------------

// addk adds constant 'v' to the prototype's list of constants, reusing
// entries via the scanner's table.
func addk(fs *FuncState, key, v *Value) int {
	L := fs.ls.L
	f := fs.f
	/* The table is the *scanner's* ('fs->ls->h'), shared by every
	   function of the chunk, not one per function -- lcode.c:
	   luaH_get(fs->ls->h, key). That is why a constant can go
	   unreused: the entry holds an index into whichever function's
	   constant table was being filled when the entry was made, and a
	   later lookup only reuses it if that index still names the same
	   value in the function being compiled now. Compiling
	   'f = function() return print end; print(f)' therefore gives
	   'print' two constant slots, and that is what PUC-Lua's luac
	   prints. */
	idx := luaH_get(L, fs.ls.h, key) /* query scanner table */
	if ttisinteger(idx) {            /* is there an index there? */
		k := int(ivalue(idx))
		/* correct value? (warning: must distinguish floats from integers!) */
		if k < fs.nk && ttypetag(&f.k[k]) == ttypetag(v) && luaV_rawequalobj(&f.k[k], v) {
			return k /* reuse index */
		}
	}
	/* constant not found; create a new entry */
	k := fs.nk
	var val Value
	setivalue(&val, int64(k))
	luaH_finishset(L, fs.ls.h, key, idx, &val)
	f.k = append(f.k, *v) /* grows by one; nk == len(f.k) - 1 */
	fs.nk++
	return k
}

// stringK adds a string to the list of constants.
func stringK(fs *FuncState, s *TString) int {
	var o Value
	setsvalue(&o, s)
	return addk(fs, &o, &o) /* use string itself as key */
}

// luaK_intK adds an integer to the list of constants.
func luaK_intK(fs *FuncState, n int64) int {
	var o Value
	setivalue(&o, n)
	return addk(fs, &o, &o)
}

// luaK_numberK adds a float to the list of constants. Floats with integral
// values need a different key, to avoid colliding with actual integers.
func luaK_numberK(fs *FuncState, r float64) int {
	var o Value
	var ik int64
	setfltvalue(&o, r)
	if !luaV_flttointeger(r, &ik, F2Ieq) { /* not an integral value? */
		return addk(fs, &o, &o) /* use number itself as key */
	}
	/* must build an alternative key */
	const nbm = 53 /* MANT_DIG of a double */
	q := math.Ldexp(1.0, -nbm+1)
	k := q
	if ik != 0 {
		k = r + r*q
	}
	var kv Value
	setfltvalue(&kv, k)
	/* result is not an integral value, unless value is too large */
	return addk(fs, &kv, &o)
}

func boolF(fs *FuncState) int {
	var o Value
	setbfvalue(&o)
	return addk(fs, &o, &o)
}

func boolT(fs *FuncState) int {
	var o Value
	setbtvalue(&o)
	return addk(fs, &o, &o)
}

func nilK(fs *FuncState) int {
	var k, v Value
	setnilvalue(&v)
	/* cannot use nil as key; instead use table itself to represent nil */
	sethvalue(&k, fs.ls.h)
	return addk(fs, &k, &v)
}

// fitsC reports whether i can be stored in an 'sC' operand.
func fitsC(i int64) bool {
	return uint64(i)+uint64(OFFSET_sC) <= uint64(MAXARG_C)
}

// fitsBx reports whether i can be stored in an 'sBx' operand.
func fitsBx(i int64) bool {
	return -OFFSET_sBx <= i && i <= MAXARG_Bx-OFFSET_sBx
}

// luaK_int codes an integer constant into a register.
func luaK_int(fs *FuncState, reg int, i int64) {
	if fitsBx(i) {
		codeAsBx(fs, OP_LOADI, reg, int(i))
	} else {
		luaK_codek(fs, reg, luaK_intK(fs, i))
	}
}

func luaK_float(fs *FuncState, reg int, f float64) {
	var fi int64
	if luaV_flttointeger(f, &fi, F2Ieq) && fitsBx(fi) {
		codeAsBx(fs, OP_LOADF, reg, int(fi))
	} else {
		luaK_codek(fs, reg, luaK_numberK(fs, f))
	}
}

// const2exp converts a constant value into an expression description.
func const2exp(v *Value, e *expdesc) {
	switch ttypetag(v) {
	case LUA_VNUMINT:
		e.k = VKINT
		e.ival = ivalue(v)
	case LUA_VNUMFLT:
		e.k = VKFLT
		e.nval = fltvalue(v)
	case LUA_VFALSE:
		e.k = VFALSE
	case LUA_VTRUE:
		e.k = VTRUE
	case LUA_VNIL:
		e.k = VNIL
	case LUA_VSHRSTR, LUA_VLNGSTR:
		e.k = VKSTR
		e.strval = tsvalue(v)
	}
}

// luaK_setreturns fixes an expression to return 'nresults' results.
func luaK_setreturns(fs *FuncState, e *expdesc, nresults int) {
	pc := &fs.f.code[e.info]
	if e.k == VCALL { /* expression is an open function call? */
		pc.C = nresults + 1
	} else {
		pc.C = nresults + 1
		pc.A = fs.freereg
		luaK_reserveregs(fs, 1)
	}
}

// luaK_setmultret fixes an expression to return all its results.
func luaK_setmultret(fs *FuncState, e *expdesc) {
	luaK_setreturns(fs, e, LUA_MULTRET)
}

// str2K converts a VKSTR expression into a VK expression.
func str2K(fs *FuncState, e *expdesc) {
	e.info = stringK(fs, e.strval)
	e.k = VK
}

// luaK_setoneret fixes an expression to return one result.
func luaK_setoneret(fs *FuncState, e *expdesc) {
	if e.k == VCALL { /* expression is an open function call? */
		/* already returns 1 value */
		e.k = VNONRELOC /* result has fixed position */
		e.info = fs.f.code[e.info].A
	} else if e.k == VVARARG {
		fs.f.code[e.info].C = 2
		e.k = VRELOC /* can relocate its simple result */
	}
}

// ---------------------------------------------------------------------
// expressions
// ---------------------------------------------------------------------

// luaK_dischargevars ensures that 'e' is not a variable (nor a <const>).
func luaK_dischargevars(fs *FuncState, e *expdesc) {
	switch e.k {
	case VCONST:
		const2exp(const2val(fs, e), e)
	case VLOCAL: /* already in a register */
		e.info = e.varRidx
		e.k = VNONRELOC /* becomes a non-relocatable value */
	case VUPVAL: /* move value to some (pending) register */
		e.info = luaK_codeABC(fs, OP_GETUPVAL, 0, e.info, 0)
		e.k = VRELOC
	case VINDEXUP:
		e.info = luaK_codeABC(fs, OP_GETTABUP, 0, e.indT, e.indIdx)
		e.k = VRELOC
	case VINDEXI:
		freereg(fs, e.indT)
		e.info = luaK_codeABC(fs, OP_GETI, 0, e.indT, e.indIdx)
		e.k = VRELOC
	case VINDEXSTR:
		freereg(fs, e.indT)
		e.info = luaK_codeABC(fs, OP_GETFIELD, 0, e.indT, e.indIdx)
		e.k = VRELOC
	case VINDEXED:
		freeregs(fs, e.indT, e.indIdx)
		e.info = luaK_codeABC(fs, OP_GETTABLE, 0, e.indT, e.indIdx)
		e.k = VRELOC
	case VVARARG, VCALL:
		luaK_setoneret(fs, e)
	}
}

// discharge2reg puts the value of 'e' into register 'reg'.
func discharge2reg(fs *FuncState, e *expdesc, reg int) {
	luaK_dischargevars(fs, e)
	switch e.k {
	case VNIL:
		luaK_nil(fs, reg, 1)
	case VFALSE:
		luaK_codeABC(fs, OP_LOADFALSE, reg, 0, 0)
	case VTRUE:
		luaK_codeABC(fs, OP_LOADTRUE, reg, 0, 0)
	case VKSTR:
		str2K(fs, e)
		fallthrough
	case VK:
		luaK_codek(fs, reg, e.info)
	case VKFLT:
		luaK_float(fs, reg, e.nval)
	case VKINT:
		luaK_int(fs, reg, e.ival)
	case VRELOC:
		fs.f.code[e.info].A = reg /* instruction will put result in 'reg' */
	case VNONRELOC:
		if reg != e.info {
			luaK_codeABC(fs, OP_MOVE, reg, e.info, 0)
		}
	default:
		return /* VJMP: nothing to do */
	}
	e.info = reg
	e.k = VNONRELOC
}

// discharge2anyreg puts the value of 'e' in any register.
func discharge2anyreg(fs *FuncState, e *expdesc) {
	if e.k != VNONRELOC { /* no fixed register yet? */
		luaK_reserveregs(fs, 1)            /* get a register */
		discharge2reg(fs, e, fs.freereg-1) /* put value there */
	}
}

func code_loadbool(fs *FuncState, A, op int) int {
	luaK_getlabel(fs) /* those instructions may be jump targets */
	return luaK_codeABC(fs, op, A, 0, 0)
}

// need_value checks whether a jump list has a jump that does not produce a
// value or produces an inverted value.
func need_value(fs *FuncState, list int) bool {
	for ; list != NO_JUMP; list = getjump(fs, list) {
		if fs.f.code[getjumpcontrol(fs, list)].Op != OP_TESTSET {
			return true
		}
	}
	return false
}

// exp2reg puts the final result of 'e' (including its jump lists) in 'reg'.
func exp2reg(fs *FuncState, e *expdesc, reg int) {
	discharge2reg(fs, e, reg)
	if e.k == VJMP { /* expression itself is a test? */
		luaK_concat(fs, &e.t, e.info) /* put this jump in 't' list */
	}
	if hasjumps(e) {
		p_f := NO_JUMP /* position of an eventual LOAD false */
		p_t := NO_JUMP /* position of an eventual LOAD true */
		if need_value(fs, e.t) || need_value(fs, e.f) {
			fj := NO_JUMP
			if e.k != VJMP {
				fj = luaK_jump(fs)
			}
			p_f = code_loadbool(fs, reg, OP_LFALSESKIP) /* skip next inst. */
			p_t = code_loadbool(fs, reg, OP_LOADTRUE)
			/* jump around these booleans if 'e' is not a test */
			luaK_patchtohere(fs, fj)
		}
		final := luaK_getlabel(fs) /* position after whole expression */
		patchlistaux(fs, e.f, final, reg, p_f)
		patchlistaux(fs, e.t, final, reg, p_t)
	}
	e.f = NO_JUMP
	e.t = NO_JUMP
	e.info = reg
	e.k = VNONRELOC
}

// luaK_exp2nextreg puts the final result of 'e' in the next free register.
func luaK_exp2nextreg(fs *FuncState, e *expdesc) {
	luaK_dischargevars(fs, e)
	freeexp(fs, e)
	luaK_reserveregs(fs, 1)
	exp2reg(fs, e, fs.freereg-1)
}

// luaK_exp2anyreg puts the final result of 'e' in any register, returning it.
func luaK_exp2anyreg(fs *FuncState, e *expdesc) int {
	luaK_dischargevars(fs, e)
	if e.k == VNONRELOC { /* expression already has a register? */
		if !hasjumps(e) { /* no jumps? */
			return e.info /* result is already in a register */
		}
		if e.info >= luaY_nvarstack(fs) { /* reg. is not a local? */
			exp2reg(fs, e, e.info) /* put final result in it */
			return e.info
		}
		/* else expression has jumps and cannot change its register to hold
		   the jump values, because it is a local variable. Go through. */
	}
	luaK_exp2nextreg(fs, e) /* default: use next available register */
	return e.info
}

// luaK_exp2anyregup ensures the result is in a register or an upvalue.
func luaK_exp2anyregup(fs *FuncState, e *expdesc) {
	if e.k != VUPVAL || hasjumps(e) {
		luaK_exp2anyreg(fs, e)
	}
}

// luaK_exp2val ensures the result is in a register or is a constant.
func luaK_exp2val(fs *FuncState, e *expdesc) {
	if e.k == VJMP || hasjumps(e) {
		luaK_exp2anyreg(fs, e)
	} else {
		luaK_dischargevars(fs, e)
	}
}

// luaK_exp2K tries to make 'e' a K expression with an index in the range of
// R/K indices.
func luaK_exp2K(fs *FuncState, e *expdesc) bool {
	if hasjumps(e) {
		return false
	}
	var info int
	switch e.k { /* move constants to 'k' */
	case VTRUE:
		info = boolT(fs)
	case VFALSE:
		info = boolF(fs)
	case VNIL:
		info = nilK(fs)
	case VKINT:
		info = luaK_intK(fs, e.ival)
	case VKFLT:
		info = luaK_numberK(fs, e.nval)
	case VKSTR:
		info = stringK(fs, e.strval)
	case VK:
		info = e.info
	default:
		return false /* not a constant */
	}
	if info <= MAXINDEXRK { /* does constant fit in 'argC'? */
		e.k = VK /* make expression a 'K' expression */
		e.info = info
		return true
	}
	return false
}

// exp2RK ensures the result is a valid R/K index.
func exp2RK(fs *FuncState, e *expdesc) bool {
	if luaK_exp2K(fs, e) {
		return true
	}
	luaK_exp2anyreg(fs, e)
	return false
}

func codeABRK(fs *FuncState, o, a, b int, ec *expdesc) {
	k := 0
	if exp2RK(fs, ec) {
		k = 1
	}
	luaK_codeABCk(fs, o, a, b, ec.info, k)
}

// luaK_storevar stores the value of 'ex' into variable 'var'.
func luaK_storevar(fs *FuncState, var_ *expdesc, ex *expdesc) {
	switch var_.k {
	case VLOCAL:
		freeexp(fs, ex)
		exp2reg(fs, ex, var_.varRidx) /* compute 'ex' into proper place */
		return
	case VUPVAL:
		e := luaK_exp2anyreg(fs, ex)
		luaK_codeABC(fs, OP_SETUPVAL, e, var_.info, 0)
	case VINDEXUP:
		codeABRK(fs, OP_SETTABUP, var_.indT, var_.indIdx, ex)
	case VINDEXI:
		codeABRK(fs, OP_SETI, var_.indT, var_.indIdx, ex)
	case VINDEXSTR:
		codeABRK(fs, OP_SETFIELD, var_.indT, var_.indIdx, ex)
	case VINDEXED:
		codeABRK(fs, OP_SETTABLE, var_.indT, var_.indIdx, ex)
	}
	freeexp(fs, ex)
}

// luaK_self emits a SELF instruction (converts 'e' into 'e:key(e,').
func luaK_self(fs *FuncState, e, key *expdesc) {
	luaK_exp2anyreg(fs, e)
	ereg := e.info /* register where 'e' was placed */
	freeexp(fs, e)
	e.info = fs.freereg /* base register for op_self */
	e.k = VNONRELOC     /* self expression has a fixed register */
	luaK_reserveregs(fs, 2)
	codeABRK(fs, OP_SELF, e.info, ereg, key)
	freeexp(fs, key)
}

// negatecondition negates a comparison expression.
func negatecondition(fs *FuncState, e *expdesc) {
	pc := getjumpcontrol(fs, e.info)
	fs.f.code[pc].K ^= 1
}

// jumponcond emits code to jump if 'e' is 'cond'.
func jumponcond(fs *FuncState, e *expdesc, cond int) int {
	if e.k == VRELOC {
		ie := fs.f.code[e.info]
		if ie.Op == OP_NOT {
			removelastinstruction(fs) /* remove previous OP_NOT */
			k := 0
			if cond == 0 {
				k = 1
			}
			return condjump(fs, OP_TEST, ie.B, 0, 0, k)
		}
	}
	discharge2anyreg(fs, e)
	freeexp(fs, e)
	return condjump(fs, OP_TESTSET, NO_REG, e.info, 0, cond)
}

// luaK_goiftrue emits code to go through if 'e' is true, jump otherwise.
func luaK_goiftrue(fs *FuncState, e *expdesc) {
	luaK_dischargevars(fs, e)
	var pc int
	switch e.k {
	case VJMP: /* condition? */
		negatecondition(fs, e) /* jump when it is false */
		pc = e.info            /* save jump position */
	case VK, VKFLT, VKINT, VKSTR, VTRUE:
		pc = NO_JUMP /* always true; do nothing */
	default:
		pc = jumponcond(fs, e, 0) /* jump when false */
	}
	luaK_concat(fs, &e.f, pc) /* insert new jump in false list */
	luaK_patchtohere(fs, e.t) /* true list jumps to here (to go through) */
	e.t = NO_JUMP
}

// luaK_goiffalse emits code to go through if 'e' is false, jump otherwise.
func luaK_goiffalse(fs *FuncState, e *expdesc) {
	luaK_dischargevars(fs, e)
	var pc int
	switch e.k {
	case VJMP:
		pc = e.info /* already jump if true */
	case VNIL, VFALSE:
		pc = NO_JUMP /* always false; do nothing */
	default:
		pc = jumponcond(fs, e, 1) /* jump if true */
	}
	luaK_concat(fs, &e.t, pc) /* insert new jump in 't' list */
	luaK_patchtohere(fs, e.f) /* false list jumps to here (to go through) */
	e.f = NO_JUMP
}

// codenot codes 'not e', doing constant folding.
func codenot(fs *FuncState, e *expdesc) {
	switch e.k {
	case VNIL, VFALSE:
		e.k = VTRUE /* true == not nil == not false */
	case VK, VKFLT, VKINT, VKSTR, VTRUE:
		e.k = VFALSE /* false == not "x" == not 0.5 == not 1 == not true */
	case VJMP:
		negatecondition(fs, e)
	case VRELOC, VNONRELOC:
		discharge2anyreg(fs, e)
		freeexp(fs, e)
		e.info = luaK_codeABC(fs, OP_NOT, 0, e.info, 0)
		e.k = VRELOC
	}
	/* interchange true and false lists */
	e.f, e.t = e.t, e.f
	removevalues(fs, e.f) /* values are useless when negated */
	removevalues(fs, e.t)
}

// isKstr reports whether 'e' is a short literal string in 'k'.
func isKstr(fs *FuncState, e *expdesc) bool {
	return e.k == VK && !hasjumps(e) && e.info <= MAXARG_B &&
		ttisshrstring(&fs.f.k[e.info])
}

// isKint reports whether 'e' is a literal integer.
func isKint(e *expdesc) bool { return e.k == VKINT && !hasjumps(e) }

// isCint reports whether 'e' is a literal integer fitting in register C.
func isCint(e *expdesc) bool {
	return isKint(e) && uint64(e.ival) <= uint64(MAXARG_C)
}

// isSCint reports whether 'e' is a literal integer fitting in register sC.
func isSCint(e *expdesc) bool { return isKint(e) && fitsC(e.ival) }

// isSCnumber reports whether 'e' is a literal integer or float in the proper
// range to fit in a register (sB or sC).
func isSCnumber(e *expdesc, pi *int, isfloat *int) bool {
	var i int64
	if e.k == VKINT {
		i = e.ival
	} else if e.k == VKFLT && luaV_flttointeger(e.nval, &i, F2Ieq) {
		*isfloat = 1
	} else {
		return false /* not a number */
	}
	if !hasjumps(e) && fitsC(i) {
		*pi = int2sC(int(i))
		return true
	}
	return false
}

// luaK_indexed creates the expression 't[k]'.
func luaK_indexed(fs *FuncState, t, k *expdesc) {
	if k.k == VKSTR {
		str2K(fs, k)
	}
	if t.k == VUPVAL && !isKstr(fs, k) { /* upvalue indexed by non 'Kstr'? */
		luaK_exp2anyreg(fs, t) /* put it in a register */
	}
	if t.k == VUPVAL {
		t.indT = t.info   /* upvalue index */
		t.indIdx = k.info /* literal short string */
		t.k = VINDEXUP
	} else {
		/* register index of the table */
		if t.k == VLOCAL {
			t.indT = t.varRidx
		} else {
			t.indT = t.info
		}
		if isKstr(fs, k) {
			t.indIdx = k.info /* literal short string */
			t.k = VINDEXSTR
		} else if isCint(k) {
			t.indIdx = int(k.ival) /* integer constant in proper range */
			t.k = VINDEXI
		} else {
			t.indIdx = luaK_exp2anyreg(fs, k) /* register */
			t.k = VINDEXED
		}
	}
}

// validop reports whether folding 'op' over v1/v2 can raise an error.
func validop(op int, v1, v2 *Value) bool {
	switch op {
	case LUA_OPBAND, LUA_OPBOR, LUA_OPBXOR, LUA_OPSHL, LUA_OPSHR, LUA_OPBNOT:
		var i int64
		return luaV_tointegerns(v1, &i, LUA_FLOORN2I) &&
			luaV_tointegerns(v2, &i, LUA_FLOORN2I)
	case LUA_OPDIV, LUA_OPIDIV, LUA_OPMOD: /* division by 0 */
		return nvalue(v2) != 0
	}
	return true
}

// constfolding tries to constant-fold an operation.
func constfolding(fs *FuncState, op int, e1, e2 *expdesc) bool {
	var v1, v2, res Value
	if !tonumeral(e1, &v1) || !tonumeral(e2, &v2) || !validop(op, &v1, &v2) {
		return false /* non-numeric operands or not safe to fold */
	}
	luaO_rawarith(fs.ls.L, op, &v1, &v2, &res) /* does operation */
	if ttisinteger(&res) {
		e1.k = VKINT
		e1.ival = ivalue(&res)
	} else {
		/* folds neither NaN nor 0.0 (to avoid problems with -0.0) */
		n := fltvalue(&res)
		if math.IsNaN(n) || n == 0 {
			return false
		}
		e1.k = VKFLT
		e1.nval = n
	}
	return true
}

// ORDER OPR - ORDER OP
func binopr2op(opr, baser BinOpr, base int) int {
	return int(opr) - int(baser) + base
}

func unopr2op(opr UnOpr) int {
	return int(opr) - int(OPR_MINUS) + OP_UNM
}

func binopr2TM(opr BinOpr) TMS {
	return TMS(int(opr) - int(OPR_ADD) + int(TM_ADD))
}

// codeunexpval emits code for unary expressions that produce values.
func codeunexpval(fs *FuncState, op int, e *expdesc, line int) {
	r := luaK_exp2anyreg(fs, e) /* opcodes operate only on registers */
	freeexp(fs, e)
	e.info = luaK_codeABC(fs, op, 0, r, 0)
	e.k = VRELOC /* all those operations are relocatable */
	luaK_fixline(fs, line)
}

// finishbinexpval emits code for binary expressions that produce values.
func finishbinexpval(fs *FuncState, e1, e2 *expdesc, op, v2, flip, line, mmop int, event TMS) {
	v1 := luaK_exp2anyreg(fs, e1)
	pc := luaK_codeABCk(fs, op, 0, v1, v2, 0)
	freeexps(fs, e1, e2)
	e1.info = pc
	e1.k = VRELOC /* all those operations are relocatable */
	luaK_fixline(fs, line)
	luaK_codeABCk(fs, mmop, v1, v2, int(event), flip) /* to call metamethod */
	luaK_fixline(fs, line)
}

// codebinexpval emits code for a binary expression over two registers.
func codebinexpval(fs *FuncState, opr BinOpr, e1, e2 *expdesc, line int) {
	op := binopr2op(opr, OPR_ADD, OP_ADD)
	v2 := luaK_exp2anyreg(fs, e2) /* make sure 'e2' is in a register */
	finishbinexpval(fs, e1, e2, op, v2, 0, line, OP_MMBIN, binopr2TM(opr))
}

// codebini codes a binary operator with an immediate operand.
func codebini(fs *FuncState, op int, e1, e2 *expdesc, flip, line int, event TMS) {
	v2 := int2sC(int(e2.ival)) /* immediate operand */
	finishbinexpval(fs, e1, e2, op, v2, flip, line, OP_MMBINI, event)
}

// codebinK codes a binary operator with a K operand.
func codebinK(fs *FuncState, opr BinOpr, e1, e2 *expdesc, flip, line int) {
	event := binopr2TM(opr)
	v2 := e2.info /* K index */
	op := binopr2op(opr, OPR_ADD, OP_ADDK)
	finishbinexpval(fs, e1, e2, op, v2, flip, line, OP_MMBINK, event)
}

// finishbinexpneg tries to code a binary operator negating its 2nd operand.
func finishbinexpneg(fs *FuncState, e1, e2 *expdesc, op, line int, event TMS) bool {
	if !isKint(e2) {
		return false /* not an integer constant */
	}
	i2 := e2.ival
	if !(fitsC(i2) && fitsC(-i2)) {
		return false /* not in the proper range */
	}
	v2 := int(i2)
	finishbinexpval(fs, e1, e2, op, int2sC(-v2), 0, line, OP_MMBINI, event)
	/* correct metamethod argument */
	fs.f.code[fs.pc-1].B = int2sC(v2)
	return true /* successfully coded */
}

func swapexps(e1, e2 *expdesc) {
	*e1, *e2 = *e2, *e1
}

// codebinNoK codes a binary operator with no constant operand.
func codebinNoK(fs *FuncState, opr BinOpr, e1, e2 *expdesc, flip, line int) {
	if flip != 0 {
		swapexps(e1, e2) /* back to original order */
	}
	codebinexpval(fs, opr, e1, e2, line) /* use standard operators */
}

// codearith codes an arithmetic operator.
func codearith(fs *FuncState, opr BinOpr, e1, e2 *expdesc, flip, line int) {
	if tonumeral(e2, nil) && luaK_exp2K(fs, e2) { /* K operand? */
		codebinK(fs, opr, e1, e2, flip, line)
	} else {
		codebinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codecommutative codes a commutative operator.
func codecommutative(fs *FuncState, op BinOpr, e1, e2 *expdesc, line int) {
	flip := 0
	if tonumeral(e1, nil) { /* is first operand a numeric constant? */
		swapexps(e1, e2) /* change order */
		flip = 1
	}
	if op == OPR_ADD && isSCint(e2) { /* immediate operand? */
		codebini(fs, OP_ADDI, e1, e2, flip, line, TM_ADD)
	} else {
		codearith(fs, op, e1, e2, flip, line)
	}
}

// codebitwise codes a bitwise operator.
func codebitwise(fs *FuncState, opr BinOpr, e1, e2 *expdesc, line int) {
	flip := 0
	if e1.k == VKINT {
		swapexps(e1, e2) /* 'e2' will be the constant operand */
		flip = 1
	}
	if e2.k == VKINT && luaK_exp2K(fs, e2) { /* K operand? */
		codebinK(fs, opr, e1, e2, flip, line)
	} else { /* no constants */
		codebinNoK(fs, opr, e1, e2, flip, line)
	}
}

// codeorder emits code for order comparisons.
func codeorder(fs *FuncState, opr BinOpr, e1, e2 *expdesc) {
	var r1, r2, im int
	isfloat := 0
	var op int
	if isSCnumber(e2, &im, &isfloat) {
		/* use immediate operand */
		r1 = luaK_exp2anyreg(fs, e1)
		r2 = im
		op = binopr2op(opr, OPR_LT, OP_LTI)
	} else if isSCnumber(e1, &im, &isfloat) {
		/* transform (A < B) to (B > A) and (A <= B) to (B >= A) */
		r1 = luaK_exp2anyreg(fs, e2)
		r2 = im
		op = binopr2op(opr, OPR_LT, OP_GTI)
	} else { /* regular case, compare two registers */
		r1 = luaK_exp2anyreg(fs, e1)
		r2 = luaK_exp2anyreg(fs, e2)
		op = binopr2op(opr, OPR_LT, OP_LT)
	}
	freeexps(fs, e1, e2)
	e1.info = condjump(fs, op, r1, r2, isfloat, 1)
	e1.k = VJMP
}

// codeeq emits code for equality comparisons.
func codeeq(fs *FuncState, opr BinOpr, e1, e2 *expdesc) {
	var r1, r2, im int
	isfloat := 0
	var op int
	if e1.k != VNONRELOC {
		swapexps(e1, e2)
	}
	r1 = luaK_exp2anyreg(fs, e1) /* 1st expression must be in register */
	if isSCnumber(e2, &im, &isfloat) {
		op = OP_EQI
		r2 = im /* immediate operand */
	} else if exp2RK(fs, e2) { /* 2nd expression is constant? */
		op = OP_EQK
		r2 = e2.info /* constant index */
	} else {
		op = OP_EQ /* will compare two registers */
		r2 = luaK_exp2anyreg(fs, e2)
	}
	freeexps(fs, e1, e2)
	eq := 0
	if opr == OPR_EQ {
		eq = 1
	}
	e1.info = condjump(fs, op, r1, r2, isfloat, eq)
	e1.k = VJMP
}

// luaK_prefix applies a unary operator to 'e'.
func luaK_prefix(fs *FuncState, opr UnOpr, e *expdesc, line int) {
	ef := &expdesc{k: VKINT, ival: 0, t: NO_JUMP, f: NO_JUMP}
	luaK_dischargevars(fs, e)
	switch opr {
	case OPR_MINUS, OPR_BNOT: /* use 'ef' as fake 2nd operand */
		if constfolding(fs, int(opr)+LUA_OPUNM, e, ef) {
			break
		}
		fallthrough
	case OPR_LEN:
		codeunexpval(fs, unopr2op(opr), e, line)
	case OPR_NOT:
		codenot(fs, e)
	}
}

// luaK_infix processes the 1st operand of a binary operation.
func luaK_infix(fs *FuncState, op BinOpr, v *expdesc) {
	luaK_dischargevars(fs, v)
	switch op {
	case OPR_AND:
		luaK_goiftrue(fs, v) /* go ahead only if 'v' is true */
	case OPR_OR:
		luaK_goiffalse(fs, v) /* go ahead only if 'v' is false */
	case OPR_CONCAT:
		luaK_exp2nextreg(fs, v) /* operand must be on the stack */
	case OPR_ADD, OPR_SUB, OPR_MUL, OPR_DIV, OPR_IDIV, OPR_MOD, OPR_POW,
		OPR_BAND, OPR_BOR, OPR_BXOR, OPR_SHL, OPR_SHR:
		if !tonumeral(v, nil) {
			luaK_exp2anyreg(fs, v)
		}
	case OPR_EQ, OPR_NE:
		if !tonumeral(v, nil) {
			exp2RK(fs, v)
		}
	case OPR_LT, OPR_LE, OPR_GT, OPR_GE:
		var dummy, dummy2 int
		if !isSCnumber(v, &dummy, &dummy2) {
			luaK_exp2anyreg(fs, v)
		}
	}
}

// codeconcat codes '(e1 .. e2)', merging with a previous CONCAT.
func codeconcat(fs *FuncState, e1, e2 *expdesc, line int) {
	if ie2 := previousinstruction(fs); ie2 >= 0 && fs.f.code[ie2].Op == OP_CONCAT {
		n := fs.f.code[ie2].B /* # of elements concatenated in 'e2' */
		freeexp(fs, e2)
		fs.f.code[ie2].A = e1.info /* correct first element ('e1') */
		fs.f.code[ie2].B = n + 1   /* will concatenate one more element */
	} else { /* 'e2' is not a concatenation */
		luaK_codeABC(fs, OP_CONCAT, e1.info, 2, 0) /* new concat opcode */
		freeexp(fs, e2)
		luaK_fixline(fs, line)
	}
}

// luaK_posfix finalizes the code for a binary operation.
func luaK_posfix(fs *FuncState, opr BinOpr, e1, e2 *expdesc, line int) {
	luaK_dischargevars(fs, e2)
	if foldbinop(opr) && constfolding(fs, int(opr)+LUA_OPADD, e1, e2) {
		return /* done by folding */
	}
	switch opr {
	case OPR_AND:
		luaK_concat(fs, &e2.f, e1.f)
		*e1 = *e2
	case OPR_OR:
		luaK_concat(fs, &e2.t, e1.t)
		*e1 = *e2
	case OPR_CONCAT: /* e1 .. e2 */
		luaK_exp2nextreg(fs, e2)
		codeconcat(fs, e1, e2, line)
	case OPR_ADD, OPR_MUL:
		codecommutative(fs, opr, e1, e2, line)
	case OPR_SUB:
		if finishbinexpneg(fs, e1, e2, OP_ADDI, line, TM_SUB) {
			break /* coded as (r1 + -I) */
		}
		fallthrough
	case OPR_DIV, OPR_IDIV, OPR_MOD, OPR_POW:
		codearith(fs, opr, e1, e2, 0, line)
	case OPR_BAND, OPR_BOR, OPR_BXOR:
		codebitwise(fs, opr, e1, e2, line)
	case OPR_SHL:
		if isSCint(e1) {
			swapexps(e1, e2)
			codebini(fs, OP_SHLI, e1, e2, 1, line, TM_SHL) /* I << r2 */
		} else if !finishbinexpneg(fs, e1, e2, OP_SHRI, line, TM_SHL) {
			/* regular case (two registers) */
			codebinexpval(fs, opr, e1, e2, line)
		}
	case OPR_SHR:
		if isSCint(e2) {
			codebini(fs, OP_SHRI, e1, e2, 0, line, TM_SHR) /* r1 >> I */
		} else {
			codebinexpval(fs, opr, e1, e2, line)
		}
	case OPR_EQ, OPR_NE:
		codeeq(fs, opr, e1, e2)
	case OPR_GT, OPR_GE:
		/* '(a > b)' <=> '(b < a)';  '(a >= b)' <=> '(b <= a)' */
		swapexps(e1, e2)
		opr = opr - OPR_GT + OPR_LT
		fallthrough
	case OPR_LT, OPR_LE:
		codeorder(fs, opr, e1, e2)
	}
}

// luaK_fixline changes the line information of the current position.
func luaK_fixline(fs *FuncState, line int) {
	removelastlineinfo(fs)
	savelineinfo(fs, fs.f, line)
}

// luaK_settablesize fills in a NEWTABLE instruction (and its extra argument).
func luaK_settablesize(fs *FuncState, pc, ra, asize, hsize int) {
	rb := 0
	if hsize != 0 {
		rb = luaO_ceillog2(uint32(hsize)) + 1 /* hash size */
	}
	extra := asize / (MAXARG_C + 1) /* higher bits of array size */
	rc := asize % (MAXARG_C + 1)    /* lower bits of array size */
	k := 0
	if extra > 0 {
		k = 1 /* true iff needs extra argument */
	}
	fs.f.code[pc] = newABC(OP_NEWTABLE, ra, rb, rc, k)
	fs.f.code[pc+1] = newABC(OP_EXTRAARG, extra, 0, 0, 0)
}

// luaK_setlist emits a SETLIST instruction.
func luaK_setlist(fs *FuncState, base, nelems, tostore int) {
	if tostore == LUA_MULTRET {
		tostore = 0
	}
	if nelems <= MAXARG_C {
		luaK_codeABC(fs, OP_SETLIST, base, tostore, nelems)
	} else {
		extra := nelems / (MAXARG_C + 1)
		nelems %= (MAXARG_C + 1)
		luaK_codeABCk(fs, OP_SETLIST, base, tostore, nelems, 1)
		codeextraarg(fs, extra)
	}
	fs.freereg = base + 1 /* free registers with list values */
}

// finaltarget returns the final target of a jump (skipping jumps to jumps).
func finaltarget(code []Instruction, i int) int {
	for count := 0; count < 100; count++ { /* avoid infinite loops */
		pc := code[i]
		if pc.Op != OP_JMP {
			break
		}
		i += pc.B - OFFSET_sJ + 1
	}
	return i
}

// luaK_finish does a final pass over the code of a function, doing small
// peephole optimizations and adjustments.
func luaK_finish(fs *FuncState) {
	p := fs.f
	for i := 0; i < fs.pc; i++ {
		pc := &p.code[i]
		switch pc.Op {
		case OP_RETURN0, OP_RETURN1:
			if !(fs.needclose || p.isVararg != 0) {
				continue /* no extra work */
			}
			/* else use OP_RETURN to do the extra work */
			pc.Op = OP_RETURN
			fallthrough
		case OP_RETURN, OP_TAILCALL:
			if fs.needclose {
				pc.K = 1 /* signal that it needs to close */
			}
			if p.isVararg != 0 {
				pc.C = int(p.numparams) + 1 /* signal that it is vararg */
			}
		case OP_JMP:
			target := finaltarget(p.code, i)
			fixjump(fs, i, target)
		}
	}
}
