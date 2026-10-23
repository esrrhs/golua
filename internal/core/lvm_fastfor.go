package core

import (
	"math"
)

// fastForMax is the largest frame a numeric 'for' (or leaf call) will run
// outside the dispatch loop. Hot loops are a handful of slots; anything larger falls back to luaV_execute.
const fastForMax = 64

// fastNumericFor runs a numerical 'for' whose body is straight-line
// arithmetic (or a few recognised table/string shapes), without
// re-entering the dispatch loop.
//
// Adapted for Lua 5.3.4: after FORPREP, pc points at the body, jump is
// FORPREP's sBx (distance to FORLOOP), and the control registers use the
// 5.3 limit/step model (not 5.4's prepaid count). Any hook, open upvalue,
// non-number operand, or unrecognised opcode declines to the slow path.
func fastNumericFor(L *State, ci *CallInfo, code []Instruction, k []Value, pc, jump, fora int) (int, bool) {
	if L.hookmask != 0 || ci.trap || jump < 0 || pc+jump >= len(code) {
		return 0, false
	}
	if code[pc+jump].Op != OP_FORLOOP {
		return 0, false
	}
	base := ci.base
	nreg := ci.top - base
	if nreg <= 0 || nreg > fastForMax || fora < 0 || fora+3 >= nreg || base+nreg > len(L.stack) {
		return 0, false
	}
	body := code[pc : pc+jump]
	if npc, ok := fastArrayFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastGlobalFor(L, ci, k, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if npc, ok := fastConcatFor(L, ci, body, pc, jump, fora, nreg); ok {
		return npc, true
	}
	if !fastForBody53(body, nreg, len(k), k) {
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
	incoming, written, okIn := fastIncoming53(body, nreg, k)
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
	if fastIntAdd53(body, &ri, &rt, fora) {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return exit, true
	}
	if fastFMA53(body, &ri, &rt, k, fora) {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return exit, true
	}
	if fastBit53(body, &ri, &rt, k, fora) {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return exit, true
	}
	if bail := fastForGeneric53(&ri, &rt, k, body, fora, stepTT == LUA_VNUMINT); bail >= 0 {
		fastForCommit(stack, &ri, &rt, &skip, nreg)
		return pc + bail, true
	}
	fastForCommit(stack, &ri, &rt, &skip, nreg)
	return exit, true
}

func overlapsFor(r, fora int) bool { return r >= fora && r <= fora+3 }

func fastForReg(r, fora int) bool { return r >= fora && r <= fora+3 }

func fastForCommit(stack []Value, ri *[fastForMax]int64, rt *[fastForMax]uint8, skip *[fastForMax]bool, nreg int) {
	for i := 0; i < nreg; i++ {
		if skip[i] {
			continue
		}
		if stack[i].p != nil || stack[i].tt != rt[i] {
			stack[i] = Value{tt: rt[i], i: ri[i]}
		} else if stack[i].i != ri[i] {
			stack[i].i = ri[i]
		}
	}
}

// fastIncoming53 marks registers the body reads before writing, using
// 5.3 RK operands (ISK bit) instead of 5.4's separate *K/*I opcodes.
func fastIncoming53(body []Instruction, nreg int, k []Value) (in, written [fastForMax]bool, ok bool) {
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
	liveRK := func(rk int) bool {
		if ISK(rk) {
			return true
		}
		return live(rk)
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
		case OP_MOVE:
			if !live(ins.B) || !def(ins.A) {
				return in, written, false
			}
		case OP_LOADK:
			if !def(ins.A) {
				return in, written, false
			}
		case OP_LOADNIL:
			for r := ins.A; r <= ins.A+ins.B; r++ {
				if !def(r) {
					return in, written, false
				}
			}
		case OP_ADD, OP_SUB, OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
			OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			if !liveRK(ins.B) || !liveRK(ins.C) || !def(ins.A) {
				return in, written, false
			}
		case OP_UNM, OP_BNOT:
			if !liveRK(ins.B) || !def(ins.A) {
				return in, written, false
			}
		default:
			return in, written, false
		}
	}
	_ = k
	return in, written, true
}

// fastIntAdd53 is 'acc = acc + i' (or 'acc = i + acc') inside an integer
// 'for'. Body is a single ADD (5.3 has no trailing MMBIN).
func fastIntAdd53(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, fora int) bool {
	if len(body) != 1 || body[0].Op != OP_ADD {
		return false
	}
	ins := body[0]
	ctrl := fora + 3
	acc := ins.A
	if ISK(ins.B) || ISK(ins.C) || fastForReg(acc, fora) {
		return false
	}
	if !((ins.B == acc && ins.C == ctrl) || (ins.C == acc && ins.B == ctrl)) {
		return false
	}
	if rt[acc] != LUA_VNUMINT || rt[fora] != LUA_VNUMINT ||
		rt[fora+1] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
		return false
	}
	idx := ri[fora]
	limit := ri[fora+1]
	step := ri[fora+2]
	sum := ri[acc]
	for {
		next := idx + step
		goon := limit <= next
		if step > 0 {
			goon = next <= limit
		}
		if !goon {
			break
		}
		idx = next
		sum += idx
	}
	ri[acc] = sum
	ri[fora] = idx
	ri[fora+3] = idx
	rt[fora+3] = LUA_VNUMINT
	return true
}

// fastFMA53 is 'acc = acc + i * k' with a MUL (RK constant) then ADD.
func fastFMA53(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, fora int) bool {
	if len(body) != 2 || body[0].Op != OP_MUL || body[1].Op != OP_ADD {
		return false
	}
	mul, add := body[0], body[1]
	ctrl := fora + 3
	tmp, acc := mul.A, add.A
	if fastForReg(tmp, fora) || fastForReg(acc, fora) || acc == tmp {
		return false
	}
	// mul tmp = ctrl * K  (or K * ctrl)
	var kv Value
	switch {
	case mul.B == ctrl && ISK(mul.C):
		kv = k[INDEXK(mul.C)]
	case mul.C == ctrl && ISK(mul.B):
		kv = k[INDEXK(mul.B)]
	default:
		return false
	}
	if !((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) {
		return false
	}
	if ISK(add.B) || ISK(add.C) {
		return false
	}
	if rt[fora] != LUA_VNUMINT && rt[fora] != LUA_VNUMFLT {
		return false
	}
	if rt[acc] != LUA_VNUMINT && rt[acc] != LUA_VNUMFLT {
		return false
	}
	if kv.tt != LUA_VNUMINT && kv.tt != LUA_VNUMFLT {
		return false
	}
	allInt := rt[fora] == LUA_VNUMINT && rt[fora+1] == LUA_VNUMINT &&
		rt[fora+2] == LUA_VNUMINT && rt[acc] == LUA_VNUMINT && kv.tt == LUA_VNUMINT
	if allInt {
		return fastFMAInt53(ri, rt, fora, acc, tmp, kv.i)
	}
	return fastFMAFloat53(ri, rt, fora, acc, tmp, kv)
}

func fastFMAInt53(ri *[fastForMax]int64, rt *[fastForMax]uint8, fora, acc, tmp int, km int64) bool {
	idx := ri[fora]
	limit := ri[fora+1]
	step := ri[fora+2]
	sum := ri[acc]
	var prod int64
	for {
		next := idx + step
		goon := limit <= next
		if step > 0 {
			goon = next <= limit
		}
		if !goon {
			break
		}
		idx = next
		prod = idx * km
		sum += prod
	}
	ri[acc] = sum
	ri[tmp] = prod
	rt[tmp] = LUA_VNUMINT
	ri[fora] = idx
	ri[fora+3] = idx
	rt[fora+3] = LUA_VNUMINT
	return true
}

func fastFMAFloat53(ri *[fastForMax]int64, rt *[fastForMax]uint8, fora, acc, tmp int, kv Value) bool {
	kf, ok := fastNum(kv.tt, kv.i)
	if !ok {
		return false
	}
	sum, ok := fastNum(rt[acc], ri[acc])
	if !ok {
		return false
	}
	var prod float64
	if rt[fora] == LUA_VNUMINT && rt[fora+1] == LUA_VNUMINT && rt[fora+2] == LUA_VNUMINT {
		idx := ri[fora]
		limit := ri[fora+1]
		step := ri[fora+2]
		for {
			next := idx + step
			goon := limit <= next
			if step > 0 {
				goon = next <= limit
			}
			if !goon {
				break
			}
			idx = next
			prod = float64(idx) * kf
			sum += prod
		}
		ri[fora] = idx
		ri[fora+3] = idx
		rt[fora+3] = LUA_VNUMINT
	} else if rt[fora] == LUA_VNUMFLT && rt[fora+1] == LUA_VNUMFLT && rt[fora+2] == LUA_VNUMFLT {
		step := math.Float64frombits(uint64(ri[fora+2]))
		limit := math.Float64frombits(uint64(ri[fora+1]))
		idx := math.Float64frombits(uint64(ri[fora]))
		for {
			next := idx + step
			goon := limit <= next
			if step > 0 {
				goon = next <= limit
			}
			if !goon {
				break
			}
			idx = next
			prod = idx * kf
			sum += prod
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

// fastBit53 is 'acc = acc + ((i & k) ~ (i << imm))'.
func fastBit53(body []Instruction, ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, fora int) bool {
	if len(body) != 4 ||
		body[0].Op != OP_BAND || body[1].Op != OP_SHL ||
		body[2].Op != OP_BXOR || body[3].Op != OP_ADD {
		return false
	}
	band, sh, bx, add := body[0], body[1], body[2], body[3]
	ctrl := fora + 3
	t1, t2, acc := band.A, sh.A, add.A
	// band t1 = ctrl & K
	if band.B != ctrl || !ISK(band.C) {
		return false
	}
	kv := k[INDEXK(band.C)]
	if kv.tt != LUA_VNUMINT {
		return false
	}
	// sh t2 = ctrl << K  (imm in constant)
	if sh.B != ctrl || !ISK(sh.C) {
		return false
	}
	sk := k[INDEXK(sh.C)]
	if sk.tt != LUA_VNUMINT {
		return false
	}
	ic := sk.i
	if bx.A != t1 || ISK(bx.B) || ISK(bx.C) ||
		!((bx.B == t1 && bx.C == t2) || (bx.B == t2 && bx.C == t1)) {
		return false
	}
	if fastForReg(acc, fora) || fastForReg(t1, fora) || fastForReg(t2, fora) ||
		acc == t1 || acc == t2 || t1 == t2 {
		return false
	}
	if ISK(add.B) || ISK(add.C) ||
		!((add.B == acc && add.C == t1) || (add.C == acc && add.B == t1)) {
		return false
	}
	if rt[acc] != LUA_VNUMINT || rt[fora] != LUA_VNUMINT ||
		rt[fora+1] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
		return false
	}
	idx := ri[fora]
	limit := ri[fora+1]
	step := ri[fora+2]
	sum := ri[acc]
	mask := kv.i
	var mixed, shifted int64
	for {
		next := idx + step
		goon := limit <= next
		if step > 0 {
			goon = next <= limit
		}
		if !goon {
			break
		}
		idx = next
		shifted = luaV_shiftl(idx, ic)
		mixed = (idx & mask) ^ shifted
		sum += mixed
	}
	ri[acc] = sum
	ri[t1] = mixed
	rt[t1] = LUA_VNUMINT
	ri[t2] = shifted
	rt[t2] = LUA_VNUMINT
	ri[fora] = idx
	ri[fora+3] = idx
	rt[fora+3] = LUA_VNUMINT
	return true
}

// fastForGeneric53 replays the body and the 5.3 FORLOOP step (limit-based).
// A non-negative return is a body index (or len(body) for the FORLOOP) where
// luaV_execute must take over. -1 means the loop finished.
func fastForGeneric53(ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, body []Instruction, fora int, intFor bool) int {
	for {
		if intFor {
			if rt[fora] != LUA_VNUMINT || rt[fora+1] != LUA_VNUMINT || rt[fora+2] != LUA_VNUMINT {
				return len(body)
			}
			step := ri[fora+2]
			idx := ri[fora] + step
			limit := ri[fora+1]
			goon := limit <= idx
			if step > 0 {
				goon = idx <= limit
			}
			if !goon {
				return -1
			}
			ri[fora] = idx
			ri[fora+3] = idx
			rt[fora+3] = LUA_VNUMINT
		} else {
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
		if b := fastExecBody53(ri, rt, k, body); b >= 0 {
			return b
		}
	}
}

// fastArrayFor recognises dense integer array fill and sum loops.
func fastArrayFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	base := ci.base
	for _, uv := range L.openupval {
		if uv != nil && uv.open && uv.L == L && uv.idx >= base && uv.idx < base+nreg {
			return 0, false
		}
	}
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT {
		return 0, false
	}
	exit := pc + jump + 1
	ctrl := fora + 3

	if len(body) == 1 && body[0].Op == OP_SETTABLE {
		ins := body[0]
		// t[i] = i  with both key and value the control variable
		if ISK(ins.B) || ISK(ins.C) || ins.B != ctrl || ins.C != ctrl ||
			ins.A < 0 || ins.A >= nreg ||
			stack[ins.A].tt != ctb(LUA_VTABLE) ||
			stack[fora+2].i != 1 {
			return 0, false
		}
		// After FORPREP, fora holds init-step; only start at 1 with step 1.
		if stack[fora].i+stack[fora+2].i != 1 {
			return 0, false
		}
		h := hvalue(&stack[ins.A])
		if h.metatable != nil {
			return 0, false
		}
		limit := stack[fora+1].i
		if limit < 1 || uint64(limit) > uint64(^uint32(0)) {
			return 0, false
		}
		last := uint32(limit)
		if luaH_realasize(h) < last {
			luaH_resizearray(L, h, last)
		}
		for j := uint32(0); j < last; j++ {
			setivalue(&h.array[j], int64(j+1))
		}
		setivalue(&stack[fora], limit)
		setivalue(&stack[fora+3], limit)
		return exit, true
	}

	if len(body) == 2 && body[0].Op == OP_GETTABLE && body[1].Op == OP_ADD {
		get, add := body[0], body[1]
		tmp, acc := get.A, add.A
		if ISK(get.C) || get.C != ctrl ||
			tmp < 0 || tmp >= nreg || acc < 0 || acc >= nreg ||
			get.B < 0 || get.B >= nreg ||
			stack[get.B].tt != ctb(LUA_VTABLE) ||
			ISK(add.B) || ISK(add.C) ||
			!((add.B == acc && add.C == tmp) || (add.C == acc && add.B == tmp)) ||
			(stack[acc].tt != LUA_VNUMINT && stack[acc].tt != LUA_VNUMFLT) {
			return 0, false
		}
		h := hvalue(&stack[get.B])
		if h.metatable != nil {
			return 0, false
		}
		idx := stack[fora].i
		limit := stack[fora+1].i
		step := stack[fora+2].i
		accTT, accBits := stack[acc].tt, stack[acc].i
		var last Value
		for {
			next := idx + step
			goon := limit <= next
			if step > 0 {
				goon = next <= limit
			}
			if !goon {
				break
			}
			idx = next
			if uint64(idx)-1 >= uint64(h.alimit) {
				return 0, false
			}
			last = h.array[idx-1]
			tt, bits, ok := fastBin53(OP_ADD, accTT, last.tt, accBits, last.i)
			if !ok {
				return 0, false
			}
			accTT, accBits = tt, bits
		}
		stack[acc] = Value{i: accBits, tt: accTT}
		stack[tmp] = last
		setivalue(&stack[fora], idx)
		setivalue(&stack[fora+3], idx)
		return exit, true
	}
	return 0, false
}

// fastConcatFor recognises loop-invariant 's = a .. b' (5.3: MOVE MOVE CONCAT).
// The join runs once; later iterations only store the same result.
func fastConcatFor(L *State, ci *CallInfo, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 3 || L.hasPending() ||
		body[0].Op != OP_MOVE || body[1].Op != OP_MOVE || body[2].Op != OP_CONCAT {
		return 0, false
	}
	m1, m2, cat := body[0], body[1], body[2]
	tmp := m1.A
	srcA, srcB := m1.B, m2.B
	dst := cat.A
	if m2.A != tmp+1 || cat.B != tmp || cat.C != tmp+1 ||
		tmp < 0 || tmp+1 >= nreg || dst < 0 || dst >= nreg ||
		srcA < 0 || srcA >= nreg || srcB < 0 || srcB >= nreg ||
		srcA == tmp || srcA == tmp+1 || srcB == tmp || srcB == tmp+1 {
		return 0, false
	}
	if overlapsFor(srcA, fora) || overlapsFor(srcB, fora) ||
		overlapsFor(tmp, fora) || overlapsFor(tmp+1, fora) || overlapsFor(dst, fora) {
		return 0, false
	}
	base := ci.base
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT ||
		!ttisstring(&stack[srcA]) || !ttisstring(&stack[srcB]) {
		return 0, false
	}
	a, b := tsvalue(&stack[srcA]), tsvalue(&stack[srcB])
	idx := stack[fora].i
	limit := stack[fora+1].i
	step := stack[fora+2].i
	// Advance control to the final index without running the body.
	final := idx
	for {
		next := final + step
		goon := limit <= next
		if step > 0 {
			goon = next <= limit
		}
		if !goon {
			break
		}
		final = next
	}
	if final == idx {
		// zero trips: nothing to concatenate
		return pc + jump + 1, true
	}
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
	setivalue(&stack[fora], final)
	setivalue(&stack[fora+3], final)
	L.gcAfterAlloc()
	return pc + jump + 1, true
}

// fastGlobalFor recognises '_G.name = _G.name + i' as emitted by 5.3.4:
// GETTABUP _G, GETTABUP _G, GETTABLE field, ADD, SETTABLE field.
func fastGlobalFor(L *State, ci *CallInfo, k []Value, body []Instruction, pc, jump, fora, nreg int) (int, bool) {
	if len(body) != 5 ||
		body[0].Op != OP_GETTABUP || body[1].Op != OP_GETTABUP ||
		body[2].Op != OP_GETTABLE || body[3].Op != OP_ADD ||
		body[4].Op != OP_SETTABLE {
		return 0, false
	}
	g1, g2, get, add, set := body[0], body[1], body[2], body[3], body[4]
	ctrl := fora + 3
	if g1.B != g2.B || g1.C != g2.C || !ISK(g1.C) ||
		get.B != g2.A || !ISK(get.C) ||
		set.A != g1.A || set.B != get.C || !ISK(set.B) || set.C != add.A || ISK(set.C) ||
		add.A != get.A ||
		!((!ISK(add.B) && add.B == add.A && !ISK(add.C) && add.C == ctrl) ||
			(!ISK(add.C) && add.C == add.A && !ISK(add.B) && add.B == ctrl)) {
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
	gslot := vmFastGet(L, envv, &k[INDEXK(g1.C)])
	if gslot == nil || gslot.tt != ctb(LUA_VTABLE) {
		return 0, false
	}
	globals := hvalue(gslot)
	if globals.metatable != nil {
		return 0, false
	}
	slot := vmFastGet(L, gslot, &k[INDEXK(get.C)])
	if slot == nil || slot.tt != LUA_VNUMINT {
		return 0, false
	}
	base := ci.base
	stack := L.stack[base : base+nreg]
	if stack[fora].tt != LUA_VNUMINT || stack[fora+1].tt != LUA_VNUMINT ||
		stack[fora+2].tt != LUA_VNUMINT {
		return 0, false
	}
	idx := stack[fora].i
	limit := stack[fora+1].i
	step := stack[fora+2].i
	value := slot.i
	for {
		next := idx + step
		goon := limit <= next
		if step > 0 {
			goon = next <= limit
		}
		if !goon {
			break
		}
		idx = next
		value += idx
	}
	slot.i = value
	stack[add.A] = Value{i: value, tt: LUA_VNUMINT}
	stack[g1.A] = *gslot
	setivalue(&stack[fora], idx)
	setivalue(&stack[fora+3], idx)
	return pc + jump + 1, true
}
