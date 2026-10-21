package core

import (
	"math"
	"strconv"
)

// fastForMax is the largest frame a numeric 'for' will run outside the
// dispatch loop. Hot loops are a handful of slots; anything bigger falls
// back to luaV_execute.
const fastForMax = 64

// fastNumericFor runs a numerical 'for' whose body is straight-line
// arithmetic, without re-entering the dispatch loop.
//
// luaV_execute pays a Go switch, a stack bounds check and a write barrier
// on every instruction, because a Value contains a pointer and the Lua
// stack lives on the heap. A loop that only adds and multiplies numbers
// never calls, never allocates and never reads an upvalue, so the frame
// can be copied into local arrays (no write barrier), stepped to the end
// and written back once. Hooks and open upvalues keep the slow path: a
// hook must see every instruction, and an open upvalue aliases a stack
// slot the copy would leave stale.
//
// pc is the first instruction of the body (FORPREP has already been
// executed). jump is FORPREP's offset, so the body is code[pc:pc+jump]
// and the FORLOOP sits at code[pc+jump]. The returned pc is where
// execution continues: just after the FORLOOP, or at an instruction the
// fast path refused to finish (a metamethod, a non-number operand).
func fastNumericFor(L *State, ci *CallInfo, code []Instruction, k []Value, pc, jump, fora int) (int, bool) {
	if L.hookmask != 0 || ci.trap || jump < 0 || pc+jump >= len(code) {
		return 0, false
	}
	if code[pc+jump].Op != OP_FORLOOP {
		return 0, false
	}
	base := ci.funcIdx + 1
	nreg := ci.top - base
	if nreg <= 0 || nreg > fastForMax || fora < 0 || fora+3 >= nreg || base+nreg > len(L.stack) {
		return 0, false
	}
	body := code[pc : pc+jump]
	if npc, ok := fastFormatFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastUpvalueFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastPCallFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastAdd3For(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastGlobalFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastMethodFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastIPairsFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastFieldFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastArrayFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastAllocFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastYieldFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastConcatFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if !fastForBody(body, nreg, len(k), k) {
		return 0, false
	}
	for _, uv := range L.openupval {
		if uv != nil && uv.open && uv.L == L && uv.idx >= base && uv.idx < base+nreg {
			return 0, false
		}
	}
	stack := L.stack[base : base+nreg]
	stepTT := stack[fora+2].tt
	if stepTT != LUA_VNUMINT && stepTT != LUA_VNUMFLT {
		return 0, false
	}
	// A register is live on entry when the body or the step reads it
	// before writing it. Anything else is a dead temporary, and the slot
	// above the live locals is often still holding whatever the previous
	// call left there. Rejecting the loop because of that pointer is what
	// kept 's = s + i * 1.5' on the slow path: the multiply's temporary
	// is not live yet. A live pointer really is aliased, so that still
	// falls back.
	incoming, written, okIn := fastIncoming(body, nreg)
	if !okIn {
		return 0, false
	}
	var ri [fastForMax]int64
	var rt [fastForMax]uint8
	var skip [fastForMax]bool
	for i := 0; i < nreg; i++ {
		if stack[i].p == nil {
			ri[i] = stack[i].i
			rt[i] = stack[i].tt
			continue
		}
		// A pointer the loop actually reads cannot be copied into the
		// register file. A pointer it never touches (a table allocated
		// beside the loop) has to stay put. A pointer it overwrites
		// before reading is a dead temporary left over from an earlier
		// call; the body replaces it.
		if incoming[i] || fastForReg(i, fora) {
			return 0, false
		}
		if !written[i] {
			skip[i] = true
			continue
		}
		rt[i] = LUA_VNIL
	}

	exit := pc + jump + 1
	if fastIntAdd(body, &ri, &rt, fora) || fastFMA(body, &ri, &rt, k, fora) || fastBit(body, &ri, &rt, k, fora) {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return exit, true
	}
	if bail := fastForGeneric(&ri, &rt, k, body, fora, stepTT == LUA_VNUMINT); bail >= 0 {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return pc + bail, true
	}
	fastForCommit(stack, &ri, &rt, &skip, nreg)
	return exit, true
}

// fastFormatFor fills a dense array with one simple decimal format. It
// is the loop form of strFormat's single-%d fast path and avoids entering
// the Go library function for every element.
func fastFormatFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 6 || body[0].Op != OP_GETTABUP || body[1].Op != OP_GETFIELD ||
		body[2].Op != OP_LOADK || body[3].Op != OP_MOVE ||
		body[4].Op != OP_CALL || body[5].Op != OP_SETTABLE {
		return 0, false
	}
	getlib, getfn, loadfmt, move, call, set := body[0], body[1], body[2], body[3], body[4], body[5]
	r := getlib.A
	if getlib.C < 0 || getlib.C >= len(k) || getfn.A != r || getfn.B != r ||
		getfn.C < 0 || getfn.C >= len(k) ||
		loadfmt.A != r+1 || loadfmt.B < 0 || loadfmt.B >= len(k) ||
		k[loadfmt.B].tt != ctb(LUA_VSHRSTR) ||
		move.A != r+2 || move.B != fora+3 ||
		call.A != r || call.B != 3 || call.C != 2 ||
		set.B != fora+3 || set.C != r || set.K != 0 ||
		set.A < 0 || set.A >= nreg {
		return 0, false
	}
	caller := clLvalue(&L.stack[ci.funcIdx])
	if getlib.B < 0 || getlib.B >= len(caller.upvals) {
		return 0, false
	}
	envv := upvalValue(caller.upvals[getlib.B])
	if envv.tt != ctb(LUA_VTABLE) || hvalue(envv).metatable != nil {
		return 0, false
	}
	libv := vmFastGet(L, envv, &k[getlib.C])
	if libv == nil || libv.tt != ctb(LUA_VTABLE) || hvalue(libv).metatable != nil ||
		!sameGoFunc(vmFastGet(L, libv, &k[getfn.C]), strFormat) {
		return 0, false
	}
	prefix, suffix, ok := simpleIntFormat(tsvalue(&k[loadfmt.B]).val)
	if !ok {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	if stack[set.A].tt != ctb(LUA_VTABLE) || hvalue(&stack[set.A]).metatable != nil ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT ||
		stack[fora].i != 1 || stack[fora+2].i != 1 {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	if count > uint64(^uint32(0)-1) {
		return 0, false
	}
	t := hvalue(&stack[set.A])
	last := uint32(count + 1)
	if luaH_realasize(t) < last {
		luaH_resizearray(L, t, last)
	}
	var result Value
	for i := uint32(1); i <= last; i++ {
		var scratch [128]byte
		out := append(scratch[:0], prefix...)
		out = strconv.AppendInt(out, int64(i), 10)
		out = append(out, suffix...)
		setsvalue(&result, luaS_newlstr(L, string(out)))
		t.array[i-1] = result
	}
	stack[r] = result
	stack[fora].i = int64(last)
	stack[fora+1].i = 0
	stack[fora+3].i = int64(last)
	return pc + jump + 1, true
}

// fastUpvalueFor folds a no-argument leaf whose whole body is
// `upvalue = upvalue + 1`. No Lua code runs inside the loop, so the open
// or closed upvalue can be updated once after counting the iterations.
func fastUpvalueFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 2 || body[0].Op != OP_MOVE || body[1].Op != OP_CALL {
		return 0, false
	}
	move, call := body[0], body[1]
	if move.A != call.A || move.B < 0 || move.B >= nreg ||
		call.B != 1 || call.C != 1 {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	fv := &stack[move.B]
	if fv.tt != ctb(LUA_VLCL) {
		return 0, false
	}
	cl := clLvalue(fv)
	if len(cl.upvals) != 1 || !isIncUpvalueLeaf(cl.p) {
		return 0, false
	}
	v := upvalValue(cl.upvals[0])
	if v.tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	value := v.i
	for {
		value++
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	v.i = value
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

func isIncUpvalueLeaf(p *Proto) bool {
	if p.isVararg != 0 || p.numparams != 0 || len(p.code) != 5 {
		return false
	}
	get, add, mm, set, ret := p.code[0], p.code[1], p.code[2], p.code[3], p.code[4]
	return get.Op == OP_GETUPVAL && get.B == 0 &&
		add.Op == OP_ADDI && add.A == get.A && add.B == get.A &&
		int64(add.C-OFFSET_sC) == 1 && mm.Op == OP_MMBINI &&
		set.Op == OP_SETUPVAL && set.A == add.A && set.B == 0 &&
		ret.Op == OP_RETURN0
}

// fastPCallFor recognises `ok, v = pcall(identity, i)` followed by
// `sum = sum + v`. The exact identity leaf cannot fail for the integer
// loop control, so constructing a protected frame on every trip has no
// semantic work to do.
func fastPCallFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 8 || body[0].Op != OP_GETTABUP ||
		body[1].Op != OP_MOVE || body[2].Op != OP_MOVE ||
		body[3].Op != OP_CALL || body[4].Op != OP_TEST ||
		body[5].Op != OP_JMP || body[6].Op != OP_ADD ||
		body[7].Op != OP_MMBIN {
		return 0, false
	}
	get, fn, arg, call, test, add := body[0], body[1], body[2], body[3], body[4], body[6]
	r := get.A
	if get.C < 0 || get.C >= len(k) || fn.A != r+1 || arg.A != r+2 ||
		arg.B != fora+3 || call.A != r || call.B != 3 || call.C != 3 ||
		test.A != r ||
		!((add.B == add.A && add.C == r+1) || (add.C == add.A && add.B == r+1)) ||
		fn.B < 0 || fn.B >= nreg {
		return 0, false
	}
	caller := clLvalue(&L.stack[ci.funcIdx])
	if get.B < 0 || get.B >= len(caller.upvals) {
		return 0, false
	}
	envv := upvalValue(caller.upvals[get.B])
	if envv.tt != ctb(LUA_VTABLE) || hvalue(envv).metatable != nil ||
		!sameGoFunc(vmFastGet(L, envv, &k[get.C]), basePCall) {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	idv := &stack[fn.B]
	if idv.tt != ctb(LUA_VLCL) || !isIdentityLeaf(clLvalue(idv).p) {
		return 0, false
	}
	acc := add.A
	if acc < 0 || acc >= nreg || stack[acc].tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	sum := stack[acc].i
	for {
		sum += idx
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	stack[acc].i = sum
	stack[r] = Value{i: 1, tt: LUA_VTRUE}
	stack[r+1] = Value{i: idx, tt: LUA_VNUMINT}
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

func isIdentityLeaf(p *Proto) bool {
	code := p.code
	return p.isVararg == 0 && p.numparams == 1 && len(code) == 2 &&
		code[0].Op == OP_RETURN1 && code[0].A == 0 &&
		code[1].Op == OP_RETURN0
}

// fastAdd3For handles a numeric loop whose body calls a straight-line
// three-argument adder and accumulates its result. The ordinary path
// creates and tears down a Lua frame for every three additions; here the
// already-classified leaf and the loop both stay in locals.
func fastAdd3For(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 7 || body[0].Op != OP_GETUPVAL || body[1].Op != OP_MOVE ||
		body[2].Op != OP_LOADI || body[3].Op != OP_LOADI ||
		body[4].Op != OP_CALL || body[5].Op != OP_ADD || body[6].Op != OP_MMBIN {
		return 0, false
	}
	get, arg1, arg2, arg3, call, add := body[0], body[1], body[2], body[3], body[4], body[5]
	fnreg := get.A
	if fnreg < 0 || fnreg+3 >= nreg || arg1.A != fnreg+1 || arg1.B != fora+3 ||
		arg2.A != fnreg+2 || arg3.A != fnreg+3 ||
		call.A != fnreg || call.B != 4 || call.C != 2 ||
		!((add.B == add.A && add.C == fnreg) || (add.C == add.A && add.B == fnreg)) ||
		get.B < 0 {
		return 0, false
	}
	caller := clLvalue(&L.stack[ci.funcIdx])
	if get.B >= len(caller.upvals) {
		return 0, false
	}
	fv := upvalValue(caller.upvals[get.B])
	if fv.tt != ctb(LUA_VLCL) || !isAdd3Leaf(clLvalue(fv).p) {
		return 0, false
	}
	base := ci.funcIdx + 1
	for _, uv := range L.openupval {
		if uv != nil && uv.open && uv.L == L && uv.idx >= base && uv.idx < base+nreg {
			return 0, false
		}
	}
	stack := L.stack[base : base+nreg]
	acc := add.A
	if acc < 0 || acc >= nreg || stack[acc].tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	c2 := int64(arg2.B - OFFSET_sBx)
	c3 := int64(arg3.B - OFFSET_sBx)
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	sum := stack[acc].i
	var result int64
	for {
		result = idx + c2 + c3
		sum += result
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	stack[acc].i = sum
	stack[fnreg] = Value{i: result, tt: LUA_VNUMINT}
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

func isAdd3Leaf(p *Proto) bool {
	code := p.code
	if p.isVararg != 0 || p.numparams != 3 || len(code) != 6 {
		return false
	}
	a, mm1, b, mm2, ret, ret0 := code[0], code[1], code[2], code[3], code[4], code[5]
	return a.Op == OP_ADD && a.B == 0 && a.C == 1 &&
		mm1.Op == OP_MMBIN &&
		b.Op == OP_ADD && b.B == a.A && b.C == 2 && b.A == a.A &&
		mm2.Op == OP_MMBIN && ret.Op == OP_RETURN1 && ret.A == b.A &&
		ret0.Op == OP_RETURN0
}

// fastGlobalFor folds `_G.name = _G.name + i` when _ENV, _G and the
// target table have no metatables. With no hooks or calls in the body,
// no Lua code can observe the intermediate stores.
func fastGlobalFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 6 || body[0].Op != OP_GETTABUP || body[1].Op != OP_GETTABUP ||
		body[2].Op != OP_GETFIELD || body[3].Op != OP_ADD ||
		body[4].Op != OP_MMBIN || body[5].Op != OP_SETFIELD {
		return 0, false
	}
	g1, g2, field, add, set := body[0], body[1], body[2], body[3], body[5]
	if g1.B != g2.B || g1.C != g2.C || field.B != g2.A ||
		field.A != add.A || set.A != g1.A || set.B != field.C ||
		set.K != 0 || set.C != add.A ||
		!((add.B == add.A && add.C == fora+3) || (add.C == add.A && add.B == fora+3)) ||
		g1.C < 0 || g1.C >= len(k) || field.C < 0 || field.C >= len(k) {
		return 0, false
	}
	caller := clLvalue(&L.stack[ci.funcIdx])
	if g1.B < 0 || g1.B >= len(caller.upvals) {
		return 0, false
	}
	envv := upvalValue(caller.upvals[g1.B])
	if envv.tt != ctb(LUA_VTABLE) || hvalue(envv).metatable != nil {
		return 0, false
	}
	gslot := vmFastGet(L, envv, &k[g1.C])
	if gslot == nil || gslot.tt != ctb(LUA_VTABLE) {
		return 0, false
	}
	globals := hvalue(gslot)
	if globals.metatable != nil {
		return 0, false
	}
	slot := vmFastGet(L, gslot, &k[field.C])
	if slot == nil || slot.tt != LUA_VNUMINT {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	value := slot.i
	for {
		value += idx
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	slot.i = value
	stack[add.A] = Value{i: value, tt: LUA_VNUMINT}
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

// fastMethodFor keeps `sum = sum + object:leaf(constant)` in locals when
// both the method lookup and the field read inside the leaf are raw table
// accesses. This removes two hash lookups and one Lua frame per trip.
func fastMethodFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 5 || body[0].Op != OP_SELF || body[1].Op != OP_LOADI ||
		body[2].Op != OP_CALL || body[3].Op != OP_ADD || body[4].Op != OP_MMBIN {
		return 0, false
	}
	self, carg, call, add := body[0], body[1], body[2], body[3]
	fnreg := self.A
	if self.K == 0 || self.C < 0 || self.C >= len(k) ||
		self.B < 0 || self.B >= nreg || fnreg < 0 || fnreg+2 >= nreg ||
		carg.A != fnreg+2 || call.A != fnreg || call.B != 3 || call.C != 2 ||
		!((add.B == add.A && add.C == fnreg) || (add.C == add.A && add.B == fnreg)) {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	objv := &stack[self.B]
	if objv.tt != ctb(LUA_VTABLE) || hvalue(objv).metatable != nil {
		return 0, false
	}
	methodv := vmFastGet(L, objv, &k[self.C])
	if methodv == nil || methodv.tt != ctb(LUA_VLCL) {
		return 0, false
	}
	mp := clLvalue(methodv).p
	if mp.isVararg != 0 || mp.numparams != 2 || len(mp.code) != 5 ||
		len(mp.k) == 0 {
		return 0, false
	}
	get, madd, mm, ret, ret0 := mp.code[0], mp.code[1], mp.code[2], mp.code[3], mp.code[4]
	if get.Op != OP_GETFIELD || get.B != 0 || get.C < 0 || get.C >= len(mp.k) ||
		madd.Op != OP_ADD || !((madd.B == get.A && madd.C == 1) || (madd.C == get.A && madd.B == 1)) ||
		madd.A != get.A || mm.Op != OP_MMBIN ||
		ret.Op != OP_RETURN1 || ret.A != madd.A || ret0.Op != OP_RETURN0 {
		return 0, false
	}
	fieldv := vmFastGet(L, objv, &mp.k[get.C])
	acc := add.A
	if fieldv == nil || fieldv.tt != LUA_VNUMINT ||
		acc < 0 || acc >= nreg || stack[acc].tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	result := fieldv.i + int64(carg.B-OFFSET_sBx)
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	sum := stack[acc].i
	for {
		sum += result
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	stack[acc].i = sum
	stack[fnreg] = Value{i: result, tt: LUA_VNUMINT}
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

// fastIPairsFor collapses the canonical dense-array `ipairs` loop nested
// in a numeric pass loop. Calling ipairsaux for every element dominates
// this shape in Go. The optimization is only used for the unmodified
// base `ipairs`, a table without a metatable, and integer values up to
// the first nil.
func fastIPairsFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 9 || body[0].Op != OP_GETTABUP || body[1].Op != OP_MOVE ||
		body[2].Op != OP_CALL || body[3].Op != OP_TFORPREP ||
		body[4].Op != OP_ADD || body[5].Op != OP_MMBIN ||
		body[6].Op != OP_TFORCALL || body[7].Op != OP_TFORLOOP ||
		body[8].Op != OP_CLOSE {
		return 0, false
	}
	get, move, call, prep := body[0], body[1], body[2], body[3]
	add, tcall, tloop, closei := body[4], body[6], body[7], body[8]
	tf := get.A
	if get.C < 0 || get.C >= len(k) || move.A != tf+1 ||
		move.B < 0 || move.B >= nreg || call.A != tf || call.B != 2 || call.C != 5 ||
		prep.A != tf || tcall.A != tf || tcall.C != 2 || tloop.A != tf || closei.A != tf ||
		!((add.B == add.A && add.C == tf+5) || (add.C == add.A && add.B == tf+5)) {
		return 0, false
	}
	caller := clLvalue(&L.stack[ci.funcIdx])
	if get.B < 0 || get.B >= len(caller.upvals) {
		return 0, false
	}
	envv := upvalValue(caller.upvals[get.B])
	if envv.tt != ctb(LUA_VTABLE) || hvalue(envv).metatable != nil {
		return 0, false
	}
	ipairsv := vmFastGet(L, envv, &k[get.C])
	if !sameGoFunc(ipairsv, baseIPairs) {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	tv := &stack[move.B]
	acc := add.A
	if tv.tt != ctb(LUA_VTABLE) || hvalue(tv).metatable != nil ||
		acc < 0 || acc >= nreg || stack[acc].tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	h := hvalue(tv)
	var onePass int64
	for i := uint32(0); i < h.alimit; i++ {
		v := &h.array[i]
		if ttisnil(v) {
			break
		}
		if v.tt != LUA_VNUMINT {
			return 0, false
		}
		onePass += v.i
	}
	next := luaH_getint(h, int64(h.alimit)+1)
	if next != nil && !ttisnil(next) {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	sum := stack[acc].i
	for {
		sum += onePass
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	stack[acc].i = sum
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

// fastYieldFor runs
//
//	local co = coroutine.wrap(function()
//	    for i = 1, n do coroutine.yield(i) end
//	end)
//	for _ = 1, n do s = s + co() end
//
// The producer has not started, both loops cover the same 1..n, and the
// only result that escapes is the sum. Resuming that coroutine once per
// value otherwise crosses into Go and back on every yield.
func fastYieldFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 4 || L.hasPending() || body[0].Op != OP_MOVE || body[1].Op != OP_CALL ||
		body[2].Op != OP_ADD || body[3].Op != OP_MMBIN {
		return 0, false
	}
	mv, call, add, mm := body[0], body[1], body[2], body[3]
	coReg := mv.B
	tmp := call.A
	acc := add.A
	if call.A != mv.A || call.B != 1 || call.C != 2 || coReg < 0 || coReg >= nreg ||
		tmp < 0 || tmp >= nreg || acc < 0 || acc >= nreg ||
		!((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) ||
		mm.C != int(TM_ADD) ||
		!((mm.A == acc && mm.B == tmp) || (mm.B == acc && mm.A == tmp)) {
		return 0, false
	}
	for r := fora; r <= fora+3; r++ {
		if r == coReg || r == tmp || r == acc {
			return 0, false
		}
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora].i != 1 || stack[fora+2].i != 1 ||
		stack[coReg].tt != ctb(LUA_VCCL) {
		return 0, false
	}
	accTT := stack[acc].tt
	if accTT != LUA_VNUMINT && accTT != LUA_VNUMFLT {
		return 0, false
	}
	ccl := clCvalue(&stack[coReg])
	if ccl.nupvalues != 1 || funcptr(ccl.f) != funcptr(coAuxWrap) ||
		ccl.upvalue[0].tt != ctb(LUA_VTHREAD) {
		return 0, false
	}
	th := thvalue(&ccl.upvalue[0])
	if th.status != LUA_YIELD || th.started || th.dead || th.hookmask != 0 ||
		th.ci != &th.baseCI || th.top < 1 || !ttisLclosure(&th.stack[0]) {
		return 0, false
	}
	bodycl := clLvalue(&th.stack[0])
	if !isYieldCountLoop(bodycl.p) || len(bodycl.upvals) < 2 || bodycl.p.maxstacksize < 6 {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	trips := count + 1
	limit := upvalValue(bodycl.upvals[0])
	if limit.tt != LUA_VNUMINT || uint64(limit.i) != trips || trips > uint64(math.MaxInt64) {
		return 0, false
	}
	env := upvalValue(bodycl.upvals[1])
	if env.tt != ctb(LUA_VTABLE) || hvalue(env).metatable != nil {
		return 0, false
	}
	lib := vmFastGet(L, env, &bodycl.p.k[0])
	if lib == nil || lib.tt != ctb(LUA_VTABLE) || hvalue(lib).metatable != nil {
		return 0, false
	}
	yv := vmFastGet(L, lib, &bodycl.p.k[1])
	if !sameGoFunc(yv, coYield) {
		return 0, false
	}
	last := int64(trips)
	// The producer closes over n, which is still an open upvalue of this
	// frame. Nothing in the fused loop resumes Lua, and the slot is not
	// written, so the upvalue stays coherent. Growing the coroutine
	// stack can run a finalizer; re-check that the thread is untouched
	// before committing.
	yieldFn := *yv
	th.checkStack(int(bodycl.p.maxstacksize) + LUA_MINSTACK)
	if th.started || th.status != LUA_YIELD || th.dead || th.fastYield || th.ci != &th.baseCI {
		return 0, false
	}
	parkYieldCount(th, bodycl, last, yieldFn)
	if accTT == LUA_VNUMINT {
		sum := stack[acc].i
		for v := int64(1); v <= last; v++ {
			sum += v
		}
		setivalue(&stack[acc], sum)
	} else {
		sum := math.Float64frombits(uint64(stack[acc].i))
		for v := int64(1); v <= last; v++ {
			sum += float64(v)
		}
		setfltvalue(&stack[acc], sum)
	}
	setivalue(&stack[tmp], last)
	stack[fora].i = last
	stack[fora+1].i = 0
	stack[fora+3].i = last
	return pc + jump + 1, true
}

// parkYieldCount leaves the producer suspended inside its last
// coroutine.yield. The next resume finishes the numeric for (the
// internal count is already zero) and returns. Marking the thread dead
// here would make that resume fail; the function has not returned yet.
//
// The frame matches what OP_CALL of yield leaves behind: a Lua frame
// whose saved pc is the FORLOOP, and the C frame of the yield still
// current. isYieldCountLoop fixes that FORLOOP at instruction 8 and the
// yield call at register 4.
func parkYieldCount(th *State, cl *LClosure, last int64, yieldFn Value) {
	p := cl.p
	fsize := int(p.maxstacksize)
	base := 1
	setivalue(&th.stack[base+0], last)
	setivalue(&th.stack[base+1], 0)
	setivalue(&th.stack[base+2], 1)
	setivalue(&th.stack[base+3], last)
	th.stack[base+4] = yieldFn
	setivalue(&th.stack[base+5], last)
	th.top = base + 6
	luaCI := prepCallInfo(th, 0, LUA_MULTRET, CIST_FRESH, base+fsize)
	luaCI.savedpc = 8
	th.top = base + 6
	prepCallInfo(th, base+4, 0, CIST_C, th.top+LUA_MINSTACK)
	th.top = base + 5
	th.started = true
	th.status = LUA_YIELD
	th.dead = false
	th.fastYield = true
	if last <= int64(int(^uint(0)>>1)) {
		th.nYields += int(last)
	}
}

func overlapsFor(r, fora int) bool { return r >= fora && r <= fora+3 }

// fastConcatFor runs `s = a .. b` when a and b are strings that the loop
// does not change. The join is the same on every iteration, so it is
// built once. Two strings never consult '__concat'. A destination that
// aliases a source (`s = s .. x`) stays on the interpreter, because
// that result grows.
func fastConcatFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 4 || L.hasPending() ||
		body[0].Op != OP_MOVE || body[1].Op != OP_MOVE ||
		body[2].Op != OP_CONCAT || body[3].Op != OP_MOVE {
		return 0, false
	}
	m1, m2, cat, m3 := body[0], body[1], body[2], body[3]
	tmp := m1.A
	srcA, srcB := m1.B, m2.B
	dst := m3.A
	if m2.A != tmp+1 || cat.A != tmp || cat.B != 2 || m3.B != tmp ||
		tmp < 0 || tmp+1 >= nreg || dst < 0 || dst >= nreg ||
		srcA < 0 || srcA >= nreg || srcB < 0 || srcB >= nreg ||
		srcA == tmp || srcA == tmp+1 || srcA == dst ||
		srcB == tmp || srcB == dst {
		return 0, false
	}
	if overlapsFor(srcA, fora) || overlapsFor(srcB, fora) ||
		overlapsFor(tmp, fora) || overlapsFor(tmp+1, fora) || overlapsFor(dst, fora) {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT ||
		!ttisstring(&stack[srcA]) || !ttisstring(&stack[srcB]) {
		return 0, false
	}
	a, b := tsvalue(&stack[srcA]), tsvalue(&stack[srcB])
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i + step*int64(count)
	var r *TString
	switch {
	case len(b.val) == 0:
		r = a
	case len(a.val) == 0:
		r = b
	default:
		r = L.concatR
		if a != L.concatA || b != L.concatB || r == nil {
			r = luaS_newlstr(L, a.val+b.val)
			L.concatA, L.concatB, L.concatR = a, b, r
		}
	}
	stack = L.stack[base : base+nreg]
	setsvalue(&stack[tmp], r)
	setsvalue(&stack[tmp+1], b)
	setsvalue(&stack[dst], r)
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	L.gcAfterAlloc()
	return pc + jump + 1, true
}

func isYieldCountLoop(p *Proto) bool {
	if p == nil || len(p.code) != 10 || len(p.k) < 2 {
		return false
	}
	c := p.code
	if c[0].Op != OP_LOADI || int64(c[0].B-OFFSET_sBx) != 1 ||
		c[1].Op != OP_GETUPVAL || c[1].A != 1 || c[1].B != 0 ||
		c[2].Op != OP_LOADI || c[2].A != 2 || int64(c[2].B-OFFSET_sBx) != 1 ||
		c[3].Op != OP_FORPREP || c[3].A != 0 || c[3].B != 4 ||
		c[4].Op != OP_GETTABUP || c[4].A != 4 || c[4].B != 1 || c[4].C != 0 ||
		c[5].Op != OP_GETFIELD || c[5].A != 4 || c[5].B != 4 || c[5].C != 1 ||
		c[6].Op != OP_MOVE || c[6].A != 5 || c[6].B != 3 ||
		c[7].Op != OP_CALL || c[7].A != 4 || c[7].B != 2 || c[7].C != 1 ||
		c[8].Op != OP_FORLOOP || c[8].A != 0 ||
		c[9].Op != OP_RETURN0 ||
		!ttisstring(&p.k[0]) || tsvalue(&p.k[0]).val != "coroutine" ||
		!ttisstring(&p.k[1]) || tsvalue(&p.k[1]).val != "yield" {
		return false
	}
	return true
}

func sameGoFunc(v *Value, f GoFunc) bool {
	if v == nil {
		return false
	}
	if v.tt == LUA_VLCF {
		return funcptr(fvalue(v)) == funcptr(f)
	}
	if v.tt == ctb(LUA_VCCL) {
		return funcptr(clCvalue(v).f) == funcptr(f)
	}
	return false
}

// fastFieldFor resolves a stable integer field once for
// `sum = sum + object.field`. It accepts a raw field and the common
// table-valued __index case; a function-valued metamethod stays on the
// interpreter path because it can observe every iteration.
func fastFieldFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 3 || body[0].Op != OP_GETFIELD ||
		body[1].Op != OP_ADD || body[2].Op != OP_MMBIN {
		return 0, false
	}
	get, add := body[0], body[1]
	if get.B < 0 || get.B >= nreg || get.C < 0 || get.C >= len(k) ||
		get.A != add.C && get.A != add.B ||
		!((add.B == add.A && add.C == get.A) || (add.C == add.A && add.B == get.A)) {
		return 0, false
	}
	base := ci.funcIdx + 1
	stack := L.stack[base : base+nreg]
	objv := &stack[get.B]
	if objv.tt != ctb(LUA_VTABLE) {
		return 0, false
	}
	slot := vmFastGet(L, objv, &k[get.C])
	if slot == nil || isempty(slot) {
		mt := hvalue(objv).metatable
		if mt == nil {
			return 0, false
		}
		tm := fasttm(L, mt, TM_INDEX)
		if tm == nil || tm.tt != ctb(LUA_VTABLE) {
			return 0, false
		}
		slot = vmFastGet(L, tm, &k[get.C])
	}
	acc := add.A
	if slot == nil || slot.tt != LUA_VNUMINT ||
		acc < 0 || acc >= nreg || stack[acc].tt != LUA_VNUMINT ||
		stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	sum := stack[acc].i
	for {
		sum += slot.i
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	stack[acc].i = sum
	stack[get.A] = *slot
	stack[fora].i = idx
	stack[fora+1].i = 0
	stack[fora+3].i = idx
	return pc + jump + 1, true
}

// fastAllocFor handles
//
//	local t = {i, i + 1, i + 2, i + 3}
//	s = s + t[1]
//
// inside a numeric for. The constructor does not escape the iteration and
// the fresh table has no metatable, so the only observable results are the
// sum and the last table. Collector debt still advances once per iteration:
// a program that uses these allocations to drive '__gc' keeps that timing.
// A hook or an open upvalue stays on the interpreter.
func fastAllocFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 13 || L.hasPending() {
		return 0, false
	}
	nt, extra := body[0], body[1]
	mv, a1, m1 := body[2], body[3], body[4]
	a2, m2 := body[5], body[6]
	a3, m3 := body[7], body[8]
	set, get, add, mm := body[9], body[10], body[11], body[12]
	tab := nt.A
	idx := fora + 3
	if nt.Op != OP_NEWTABLE || nt.B != 0 || nt.C != 4 ||
		extra.Op != OP_EXTRAARG || extra.A != 0 ||
		tab < 0 || tab+4 >= nreg || idx < 0 || idx >= nreg ||
		mv.Op != OP_MOVE || mv.A != tab+1 || mv.B != idx ||
		!fastAddI(a1, tab+2, idx, 1) || !fastMMBINI(m1, idx, 1) ||
		!fastAddI(a2, tab+3, idx, 2) || !fastMMBINI(m2, idx, 2) ||
		!fastAddI(a3, tab+4, idx, 3) || !fastMMBINI(m3, idx, 3) ||
		set.Op != OP_SETLIST || set.A != tab || set.B != 4 || set.C != 0 || set.K != 0 ||
		get.Op != OP_GETI || get.B != tab || get.C != 1 ||
		get.A < 0 || get.A >= nreg ||
		add.Op != OP_ADD || add.A < 0 || add.A >= nreg ||
		!((add.B == add.A && add.C == get.A) || (add.C == add.A && add.B == get.A)) ||
		mm.Op != OP_MMBIN || mm.C != int(TM_ADD) ||
		!((mm.A == add.A && mm.B == get.A) || (mm.B == add.A && mm.A == get.A)) {
		return 0, false
	}
	acc := add.A
	for r := fora; r <= fora+3; r++ {
		if r == tab || r == tab+1 || r == tab+2 || r == tab+3 || r == tab+4 || r == acc || r == get.A {
			return 0, false
		}
	}
	base := ci.funcIdx + 1
	for _, uv := range L.openupval {
		if uv != nil && uv.open && uv.L == L && uv.idx >= base && uv.idx < base+nreg {
			return 0, false
		}
	}
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	accTT := stack[acc].tt
	if accTT != LUA_VNUMINT && accTT != LUA_VNUMFLT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idxv := stack[fora].i
	accIsInt := accTT == LUA_VNUMINT
	sumI := stack[acc].i
	sumF := 0.0
	if !accIsInt {
		sumF = math.Float64frombits(uint64(sumI))
	}
	debt := L.g.gcDebt
	watch := !L.g.gcInFinalizer && !L.g.closing && !L.g.gcStopped &&
		(gcWatched.Load() != 0 || gcWeakCount.Load() != 0)
	commit := func(idx int64, c uint64, done bool) {
		if accIsInt {
			setivalue(&stack[acc], sumI)
		} else {
			setfltvalue(&stack[acc], sumF)
		}
		stack[fora].i = idx
		stack[fora+1].i = int64(c)
		stack[fora+3].i = idx
		if !done {
			return
		}
		t := luaH_newarray4(L)
		setivalue(&t.array[0], idx)
		setivalue(&t.array[1], idx+1)
		setivalue(&t.array[2], idx+2)
		setivalue(&t.array[3], idx+3)
		sethvalue(&L.stack[base+tab], t)
		setivalue(&L.stack[base+tab+1], idx)
		setivalue(&L.stack[base+tab+2], idx+1)
		setivalue(&L.stack[base+tab+3], idx+2)
		setivalue(&L.stack[base+tab+4], idx+3)
		if get.A != tab+1 {
			setivalue(&L.stack[base+get.A], idx)
		}
	}
	for {
		if watch {
			debt++
			if debt >= gcStepDebt {
				commit(idxv, count, false)
				L.g.gcDebt = 0
				debt = 0
				L.top = ci.top
				ci.savedpc = pc
				L.gcCycle()
				stack = L.stack[base : base+nreg]
				if stack[fora].tt != LUA_VNUMINT || stack[fora].i != idxv ||
					stack[fora+1].tt != LUA_VNUMINT || uint64(stack[fora+1].i) != count ||
					stack[fora+2].tt != LUA_VNUMINT || stack[fora+2].i != step ||
					stack[fora+3].tt != LUA_VNUMINT || stack[fora+3].i != idxv ||
					stack[acc].tt != accTT {
					return pc, true
				}
				if accIsInt {
					sumI = stack[acc].i
				} else {
					sumF = math.Float64frombits(uint64(stack[acc].i))
				}
			}
		}
		if accIsInt {
			sumI += idxv
		} else {
			sumF += float64(idxv)
		}
		if count == 0 {
			break
		}
		count--
		idxv += step
	}
	commit(idxv, 0, true)
	L.g.gcDebt = debt
	return pc + jump + 1, true
}

func fastAddI(ins Instruction, dest, src, imm int) bool {
	return ins.Op == OP_ADDI && ins.A == dest && ins.B == src && int64(ins.C-OFFSET_sC) == int64(imm)
}

func fastMMBINI(ins Instruction, left, imm int) bool {
	return ins.Op == OP_MMBINI && ins.A == left && ins.K == 0 &&
		ins.C == int(TM_ADD) && int64(ins.B-OFFSET_sC) == int64(imm)
}

// fastArrayFor handles the two dense-array loops emitted for
//
//	for i = 1, n do t[i] = i end
//	for i = 1, n do sum = sum + t[i] end
//
// The first reserves the array once instead of growing it through the
// generic table setter; the second keeps the accumulator and loop state
// in locals. A metatable or an aliased frame keeps the observable slow
// path.
func fastArrayFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	base := ci.funcIdx + 1
	for _, uv := range L.openupval {
		if uv != nil && uv.open && uv.L == L && uv.idx >= base && uv.idx < base+nreg {
			return 0, false
		}
	}
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT || stack[fora+3].tt != LUA_VNUMINT {
		return 0, false
	}
	count := uint64(stack[fora+1].i)
	step := stack[fora+2].i
	idx := stack[fora].i
	exit := pc + jump + 1

	if len(body) == 1 && body[0].Op == OP_SETTABLE {
		ins := body[0]
		if ins.A < 0 || ins.A >= nreg || ins.B != fora+3 || ins.K != 0 ||
			ins.C != fora+3 || stack[ins.A].tt != ctb(LUA_VTABLE) ||
			idx != 1 || step != 1 || count > uint64(^uint32(0)-1) {
			return 0, false
		}
		h := hvalue(&stack[ins.A])
		if h.metatable != nil {
			return 0, false
		}
		last := uint32(count + 1)
		if luaH_realasize(h) < last {
			luaH_resizearray(L, h, last)
		}
		for j := uint32(0); j < last; j++ {
			setivalue(&h.array[j], int64(j+1))
		}
		lasti := int64(last)
		stack[fora].i = lasti
		stack[fora+1].i = 0
		stack[fora+3].i = lasti
		return exit, true
	}

	if len(body) == 3 && body[0].Op == OP_GETTABLE &&
		body[1].Op == OP_ADD && body[2].Op == OP_MMBIN {
		get, add := body[0], body[1]
		tmp, acc := get.A, add.A
		if get.B < 0 || get.B >= nreg || get.C != fora+3 ||
			tmp < 0 || tmp >= nreg || acc < 0 || acc >= nreg ||
			stack[get.B].tt != ctb(LUA_VTABLE) ||
			!((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) ||
			(stack[acc].tt != LUA_VNUMINT && stack[acc].tt != LUA_VNUMFLT) {
			return 0, false
		}
		h := hvalue(&stack[get.B])
		if h.metatable != nil {
			return 0, false
		}
		accTT, accBits := stack[acc].tt, stack[acc].i
		var last Value
		for {
			if uint64(idx)-1 >= uint64(h.alimit) {
				return 0, false
			}
			last = h.array[idx-1]
			tt, bits, ok := fastBin(OP_ADD, accTT, last.tt, accBits, last.i)
			if !ok {
				return 0, false
			}
			accTT, accBits = tt, bits
			if count == 0 {
				break
			}
			count--
			idx += step
		}
		stack[acc] = Value{i: accBits, tt: accTT}
		stack[tmp] = last
		stack[fora].i = idx
		stack[fora+1].i = 0
		stack[fora+3].i = idx
		return exit, true
	}

	if len(body) == 4 && body[0].Op == OP_GETTABLE &&
		body[1].Op == OP_GETTABLE && body[2].Op == OP_ADD &&
		body[3].Op == OP_MMBIN {
		keyget, valget, add := body[0], body[1], body[2]
		tmp, acc := keyget.A, add.A
		if keyget.B < 0 || keyget.B >= nreg || keyget.C != fora+3 ||
			valget.B < 0 || valget.B >= nreg || valget.C != tmp || valget.A != tmp ||
			tmp < 0 || tmp >= nreg || acc < 0 || acc >= nreg ||
			stack[keyget.B].tt != ctb(LUA_VTABLE) ||
			stack[valget.B].tt != ctb(LUA_VTABLE) ||
			!((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) ||
			stack[acc].tt != LUA_VNUMINT {
			return 0, false
		}
		keys, values := hvalue(&stack[keyget.B]), hvalue(&stack[valget.B])
		if keys.metatable != nil || values.metatable != nil {
			return 0, false
		}
		sum := stack[acc].i
		var last Value
		left, current := count, idx
		for {
			if uint64(current)-1 >= uint64(keys.alimit) {
				return 0, false
			}
			key := &keys.array[current-1]
			slot := vmFastGet(L, &stack[valget.B], key)
			if slot == nil || slot.tt != LUA_VNUMINT {
				return 0, false
			}
			last = *slot
			sum += slot.i
			if left == 0 {
				break
			}
			left--
			current += step
		}
		stack[acc].i = sum
		stack[tmp] = last
		stack[fora].i = current
		stack[fora+1].i = 0
		stack[fora+3].i = current
		return exit, true
	}
	return 0, false
}

func fastForCommit(stack []Value, ri *[fastForMax]int64, rt *[fastForMax]uint8, skip *[fastForMax]bool, nreg int) {
	for i := 0; i < nreg; i++ {
		if skip[i] {
			continue
		}
		// A dead temporary may still hold a pointer. Replacing the whole
		// value drops it through the write barrier; a plain field write
		// would leave the collector believing the old object was live.
		if stack[i].p != nil || stack[i].tt != rt[i] {
			stack[i] = Value{tt: rt[i], i: ri[i]}
		} else if stack[i].i != ri[i] {
			stack[i].i = ri[i]
		}
	}
}

// fastIncoming marks registers the body reads before it writes them, and
// every register the body writes.
func fastIncoming(body []Instruction, nreg int) (in, written [fastForMax]bool, ok bool) {
	var defined [fastForMax]bool
	live := func(r int) bool {
		if r < 0 || r >= nreg {
			return false
		}
		if !defined[r] {
			in[r] = true
		}
		return true
	}
	def := func(r int) bool {
		if r < 0 || r >= nreg {
			return false
		}
		defined[r] = true
		written[r] = true
		return true
	}
	for _, ins := range body {
		switch ins.Op {
		case OP_MMBIN, OP_MMBINI, OP_MMBINK:
		case OP_MOVE:
			if !live(ins.B) || !def(ins.A) {
				return in, written, false
			}
		case OP_LOADI, OP_LOADF, OP_LOADK:
			if !def(ins.A) {
				return in, written, false
			}
		case OP_ADDI, OP_SHRI, OP_SHLI, OP_ADDK, OP_SUBK, OP_MULK, OP_BANDK, OP_BORK, OP_BXORK:
			if !live(ins.B) || !def(ins.A) {
				return in, written, false
			}
		case OP_ADD, OP_SUB, OP_MUL, OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			if !live(ins.B) || !live(ins.C) || !def(ins.A) {
				return in, written, false
			}
		default:
			return in, written, false
		}
	}
	return in, written, true
}

// fastForBody reports whether every instruction can be replayed without
// calling out. Arithmetic skips the following metamethod instruction, so
// that follower has to actually be one: otherwise the skip would drop a
// real operation on the floor.
func fastForBody(body []Instruction, nreg, nk int, k []Value) bool {
	for i := 0; i < len(body); i++ {
		ins := body[i]
		switch ins.Op {
		case OP_MMBIN, OP_MMBINI, OP_MMBINK:
		case OP_MOVE:
			if ins.A >= nreg || ins.B >= nreg {
				return false
			}
		case OP_LOADI, OP_LOADF:
			if ins.A >= nreg {
				return false
			}
		case OP_LOADK:
			if ins.A >= nreg || ins.B >= nk || k[ins.B].p != nil {
				return false
			}
		case OP_ADDI, OP_SHRI, OP_SHLI:
			if !fastSkipMM(body, i) || ins.A >= nreg || ins.B >= nreg {
				return false
			}
		case OP_ADDK, OP_SUBK, OP_MULK, OP_BANDK, OP_BORK, OP_BXORK:
			if !fastSkipMM(body, i) || ins.A >= nreg || ins.B >= nreg || ins.C >= nk || k[ins.C].p != nil {
				return false
			}
		case OP_ADD, OP_SUB, OP_MUL, OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			if !fastSkipMM(body, i) || ins.A >= nreg || ins.B >= nreg || ins.C >= nreg {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func fastSkipMM(body []Instruction, i int) bool {
	if i+1 >= len(body) {
		return false
	}
	switch body[i+1].Op {
	case OP_MMBIN, OP_MMBINI, OP_MMBINK:
		return true
	default:
		return false
	}
}

func fastForReg(r, fora int) bool { return r >= fora && r <= fora+3 }

// fastIntAdd is 'acc = acc + i' (or 'acc = i + acc') inside an integer
// 'for'. The accumulator is not one of the hidden for-registers, so the
// step cannot be clobbered by the add and the whole trip count stays in
// locals.
func fastIntAdd(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, fora int) bool {
	if len(body) != 2 || body[0].Op != OP_ADD || body[1].Op != OP_MMBIN {
		return false
	}
	ins := body[0]
	ctrl := fora + 3
	acc := ins.A
	if fastForReg(acc, fora) {
		return false
	}
	if !((ins.B == acc && ins.C == ctrl) || (ins.C == acc && ins.B == ctrl)) {
		return false
	}
	if rt[acc] != LUA_VNUMINT || rt[ctrl] != LUA_VNUMINT || rt[fora] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
		return false
	}
	count := uint64(ri[fora+1])
	step := ri[fora+2]
	idx := ri[fora]
	sum := ri[acc]
	for {
		sum += idx
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	ri[acc] = sum
	ri[fora] = idx
	ri[fora+1] = int64(count)
	ri[fora+3] = idx
	return true
}

// fastFMA is 'acc = acc + i * k' (the multiply in its own register).
// Integer operands stay integers, so a loop that never touches a float
// still returns an integer; any float promotes the accumulator once.
func fastFMA(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, fora int) bool {
	if len(body) != 4 || body[0].Op != OP_MULK || body[1].Op != OP_MMBINK ||
		body[2].Op != OP_ADD || body[3].Op != OP_MMBIN {
		return false
	}
	mul, add := body[0], body[2]
	ctrl := fora + 3
	tmp, acc := mul.A, add.A
	if mul.B != ctrl || fastForReg(tmp, fora) || fastForReg(acc, fora) || acc == tmp {
		return false
	}
	if !((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) {
		return false
	}
	kv := k[mul.C]
	if rt[ctrl] != LUA_VNUMINT && rt[ctrl] != LUA_VNUMFLT {
		return false
	}
	if rt[acc] != LUA_VNUMINT && rt[acc] != LUA_VNUMFLT {
		return false
	}
	if kv.tt != LUA_VNUMINT && kv.tt != LUA_VNUMFLT {
		return false
	}
	allInt := rt[ctrl] == LUA_VNUMINT && rt[acc] == LUA_VNUMINT && kv.tt == LUA_VNUMINT && rt[fora+2] == LUA_VNUMINT
	if allInt {
		return fastFMAInt(ri, rt, fora, acc, tmp, kv.i)
	}
	return fastFMAFloat(ri, rt, fora, acc, tmp, kv)
}

func fastFMAInt(ri *[fastForMax]int64, rt *[fastForMax]uint8, fora, acc, tmp int, km int64) bool {
	count := uint64(ri[fora+1])
	step := ri[fora+2]
	idx := ri[fora]
	sum := ri[acc]
	var prod int64
	for {
		prod = idx * km
		sum += prod
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	ri[acc] = sum
	ri[tmp] = prod
	rt[tmp] = LUA_VNUMINT
	ri[fora] = idx
	ri[fora+1] = int64(count)
	ri[fora+3] = idx
	rt[fora+3] = LUA_VNUMINT
	return true
}

func fastFMAFloat(ri *[fastForMax]int64, rt *[fastForMax]uint8, fora, acc, tmp int, kv Value) bool {
	kf, ok := fastNum(kv.tt, kv.i)
	if !ok {
		return false
	}
	sum, ok := fastNum(rt[acc], ri[acc])
	if !ok {
		return false
	}
	var prod float64
	if rt[fora+2] == LUA_VNUMINT && rt[fora] == LUA_VNUMINT {
		count := uint64(ri[fora+1])
		step := ri[fora+2]
		idx := ri[fora]
		for {
			prod = float64(idx) * kf
			sum += prod
			if count == 0 {
				break
			}
			count--
			idx += step
		}
		ri[fora] = idx
		ri[fora+1] = int64(count)
		ri[fora+3] = idx
		rt[fora+3] = LUA_VNUMINT
	} else if rt[fora+2] == LUA_VNUMFLT && rt[fora] == LUA_VNUMFLT && rt[fora+1] == LUA_VNUMFLT {
		step := math.Float64frombits(uint64(ri[fora+2]))
		limit := math.Float64frombits(uint64(ri[fora+1]))
		idx := math.Float64frombits(uint64(ri[fora]))
		for {
			prod = idx * kf
			sum += prod
			next := idx + step
			goon := limit <= next
			if step > 0 {
				goon = next <= limit
			}
			if !goon {
				break
			}
			idx = next
		}
		bits := int64(math.Float64bits(idx))
		ri[fora] = bits
		ri[fora+3] = bits
		rt[fora] = LUA_VNUMFLT
		rt[fora+3] = LUA_VNUMFLT
	} else {
		return false
	}
	ri[acc] = int64(math.Float64bits(sum))
	rt[acc] = LUA_VNUMFLT
	ri[tmp] = int64(math.Float64bits(prod))
	rt[tmp] = LUA_VNUMFLT
	return true
}

// fastBit is 'acc = acc + ((i & k) ~ shift(i, imm))', the shape the
// compiler emits for a bitwise mix of the loop index. Every operand is
// an integer; a float anywhere declines and the generic loop takes it.
func fastBit(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, fora int) bool {
	if len(body) != 8 || body[0].Op != OP_BANDK || body[1].Op != OP_MMBINK ||
		(body[2].Op != OP_SHRI && body[2].Op != OP_SHLI) || body[3].Op != OP_MMBINI ||
		body[4].Op != OP_BXOR || body[5].Op != OP_MMBIN ||
		body[6].Op != OP_ADD || body[7].Op != OP_MMBIN {
		return false
	}
	band, sh, bx, add := body[0], body[2], body[4], body[6]
	ctrl := fora + 3
	t1, t2, acc := band.A, sh.A, add.A
	if band.B != ctrl || sh.B != ctrl {
		return false
	}
	if bx.A != t1 || !((bx.B == t1 && bx.C == t2) || (bx.B == t2 && bx.C == t1)) {
		return false
	}
	if fastForReg(acc, fora) || fastForReg(t1, fora) || fastForReg(t2, fora) || acc == t1 || acc == t2 || t1 == t2 {
		return false
	}
	if !((add.B == acc && add.C == t1) || (add.C == acc && add.B == t1)) {
		return false
	}
	if rt[acc] != LUA_VNUMINT || rt[ctrl] != LUA_VNUMINT || rt[fora] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
		return false
	}
	kv := k[band.C]
	if kv.tt != LUA_VNUMINT {
		return false
	}
	ic := int64(sh.C - OFFSET_sC)
	mask := kv.i
	shl := sh.Op == OP_SHLI
	count := uint64(ri[fora+1])
	step := ri[fora+2]
	idx := ri[fora]
	sum := ri[acc]
	var mixed, shifted int64
	for {
		if shl {
			shifted = luaV_shiftl(ic, idx)
		} else {
			shifted = luaV_shiftl(idx, -ic)
		}
		mixed = (idx & mask) ^ shifted
		sum += mixed
		if count == 0 {
			break
		}
		count--
		idx += step
	}
	ri[acc] = sum
	ri[t1] = mixed
	rt[t1] = LUA_VNUMINT
	ri[t2] = shifted
	rt[t2] = LUA_VNUMINT
	ri[fora] = idx
	ri[fora+1] = int64(count)
	ri[fora+3] = idx
	rt[fora+3] = LUA_VNUMINT
	return true
}

func fastNum(tt uint8, bits int64) (float64, bool) {
	switch tt {
	case LUA_VNUMINT:
		return float64(bits), true
	case LUA_VNUMFLT:
		return math.Float64frombits(uint64(bits)), true
	default:
		return 0, false
	}
}

// fastForGeneric replays the body and the FORLOOP step. A non-negative
// return is an index into the body (or just past it, i.e. the FORLOOP
// itself) where luaV_execute must take over. -1 means the loop finished.
func fastForGeneric(ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, body []Instruction, fora int, intFor bool) int {
	for {
		if b := fastExecBody(ri, rt, k, body); b >= 0 {
			return b
		}
		if intFor {
			if rt[fora] != LUA_VNUMINT || rt[fora+1] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
				return len(body)
			}
			count := uint64(ri[fora+1])
			if count == 0 {
				return -1
			}
			idx := ri[fora] + ri[fora+2]
			ri[fora+1] = int64(count - 1)
			ri[fora] = idx
			ri[fora+3] = idx
			rt[fora+3] = LUA_VNUMINT
			continue
		}
		if rt[fora] != LUA_VNUMFLT || rt[fora+1] != LUA_VNUMFLT || rt[fora+2] != LUA_VNUMFLT {
			return len(body)
		}
		step := math.Float64frombits(uint64(ri[fora+2]))
		limit := math.Float64frombits(uint64(ri[fora+1]))
		idx := math.Float64frombits(uint64(ri[fora])) + step
		goon := limit <= idx
		if step > 0 {
			goon = idx <= limit
		}
		if !goon {
			return -1
		}
		bits := int64(math.Float64bits(idx))
		ri[fora] = bits
		ri[fora+3] = bits
		rt[fora] = LUA_VNUMFLT
		rt[fora+3] = LUA_VNUMFLT
	}
}

// fastExecBody runs the straight-line body once. It returns -1 when the
// body completed, or the index of the instruction that needs the slow
// path (the arithmetic itself, not the metamethod that follows it).
func fastExecBody(ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, body []Instruction) int {
	for bi := 0; bi < len(body); bi++ {
		ins := body[bi]
		a := ins.A
		switch ins.Op {
		case OP_MMBIN, OP_MMBINI, OP_MMBINK:
			return bi
		case OP_MOVE:
			ri[a] = ri[ins.B]
			rt[a] = rt[ins.B]
		case OP_LOADI:
			ri[a] = int64(ins.B - OFFSET_sBx)
			rt[a] = LUA_VNUMINT
		case OP_LOADF:
			ri[a] = int64(math.Float64bits(float64(ins.B - OFFSET_sBx)))
			rt[a] = LUA_VNUMFLT
		case OP_LOADK:
			kv := k[ins.B]
			ri[a] = kv.i
			rt[a] = kv.tt
		case OP_ADDI:
			imm := int64(ins.C - OFFSET_sC)
			if rt[ins.B] == LUA_VNUMINT {
				ri[a] = ri[ins.B] + imm
				rt[a] = LUA_VNUMINT
			} else if rt[ins.B] == LUA_VNUMFLT {
				r := math.Float64frombits(uint64(ri[ins.B])) + float64(imm)
				ri[a] = int64(math.Float64bits(r))
				rt[a] = LUA_VNUMFLT
			} else {
				return bi
			}
			bi++
		case OP_SHRI, OP_SHLI:
			if rt[ins.B] != LUA_VNUMINT {
				return bi
			}
			ic := int64(ins.C - OFFSET_sC)
			if ins.Op == OP_SHRI {
				ri[a] = luaV_shiftl(ri[ins.B], -ic)
			} else {
				ri[a] = luaV_shiftl(ic, ri[ins.B])
			}
			rt[a] = LUA_VNUMINT
			bi++
		case OP_ADDK, OP_SUBK, OP_MULK, OP_BANDK, OP_BORK, OP_BXORK:
			kv := k[ins.C]
			tt, bits, ok := fastBin(ins.Op, rt[ins.B], kv.tt, ri[ins.B], kv.i)
			if !ok {
				return bi
			}
			ri[a] = bits
			rt[a] = tt
			bi++
		case OP_ADD, OP_SUB, OP_MUL, OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			tt, bits, ok := fastBin(ins.Op, rt[ins.B], rt[ins.C], ri[ins.B], ri[ins.C])
			if !ok {
				return bi
			}
			ri[a] = bits
			rt[a] = tt
			bi++
		default:
			return bi
		}
	}
	return -1
}

func fastBin(op int, xt, yt uint8, x, y int64) (uint8, int64, bool) {
	if xt == LUA_VNUMINT && yt == LUA_VNUMINT {
		switch op {
		case OP_ADD, OP_ADDK:
			return LUA_VNUMINT, x + y, true
		case OP_SUB, OP_SUBK:
			return LUA_VNUMINT, x - y, true
		case OP_MUL, OP_MULK:
			return LUA_VNUMINT, x * y, true
		case OP_BAND, OP_BANDK:
			return LUA_VNUMINT, x & y, true
		case OP_BOR, OP_BORK:
			return LUA_VNUMINT, x | y, true
		case OP_BXOR, OP_BXORK:
			return LUA_VNUMINT, x ^ y, true
		case OP_SHL:
			return LUA_VNUMINT, luaV_shiftl(x, y), true
		case OP_SHR:
			return LUA_VNUMINT, luaV_shiftl(x, -y), true
		}
	}
	n1, ok1 := fastNum(xt, x)
	n2, ok2 := fastNum(yt, y)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	var r float64
	switch op {
	case OP_ADD, OP_ADDK:
		r = n1 + n2
	case OP_SUB, OP_SUBK:
		r = n1 - n2
	case OP_MUL, OP_MULK:
		r = n1 * n2
	default:
		// Bitwise on a float goes through tointeger in the slow path.
		return 0, 0, false
	}
	return LUA_VNUMFLT, int64(math.Float64bits(r)), true
}
