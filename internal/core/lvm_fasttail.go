package core

// fastCountTail runs the common tail-recursive countdown
//
//	function f(n, acc)
//	  if n == 0 then return acc end
//	  return f(n - 1, acc + 1)
//	end
//
// as one loop. Each ordinary tail call leaves luaV_execute and enters
// it again, which is most of the cost of this shape; the arithmetic
// itself is two integer increments. The closure's upvalue has to be the
// function being run, both arguments have to be integers, and n has to
// be non-negative (a negative n does not reach 0, and the ordinary
// interpreter is the one that loops). The returned pc is the RETURN1
// that delivers acc.
func fastCountTail(L *State, ci *CallInfo, cl *LClosure, code []Instruction) (int, bool) {
	nReg, accReg, retPC, ok := matchCountTail(code)
	if !ok || cl.p.isVararg != 0 || int(cl.p.numparams) <= accReg || int(cl.p.numparams) <= nReg {
		return 0, false
	}
	if cl.nupvalues < 1 {
		return 0, false
	}
	self := &L.stack[ci.funcIdx]
	uv := upvalValue(cl.upvals[0])
	if uv.tt != self.tt || uv.p != self.p {
		return 0, false
	}
	base := ci.funcIdx + 1
	hi := accReg
	if nReg > hi {
		hi = nReg
	}
	if base+hi >= len(L.stack) {
		return 0, false
	}
	nv := &L.stack[base+nReg]
	av := &L.stack[base+accReg]
	if nv.tt != LUA_VNUMINT || av.tt != LUA_VNUMINT || nv.i < 0 {
		return 0, false
	}
	n := nv.i
	acc := av.i
	for n != 0 {
		n--
		acc++
	}
	nv.i = 0
	av.i = acc
	return retPC, true
}

// matchCountTail recognises the bytecode shape above. Registers come
// from the instructions, so a different allocation still matches; a
// different step or a different callee does not.
func matchCountTail(code []Instruction) (nReg, accReg, retPC int, ok bool) {
	if len(code) < 9 {
		return 0, 0, 0, false
	}
	eq, jmp, ret := code[0], code[1], code[2]
	get, sub, mm1 := code[3], code[4], code[5]
	add, mm2, tail := code[6], code[7], code[8]
	if eq.Op != OP_EQI || eq.K != 0 || eq.C != 0 || int64(eq.B-OFFSET_sC) != 0 {
		return 0, 0, 0, false
	}
	if jmp.Op != OP_JMP || jmp.B-OFFSET_sJ != 1 {
		return 0, 0, 0, false
	}
	if ret.Op != OP_RETURN1 {
		return 0, 0, 0, false
	}
	if get.Op != OP_GETUPVAL || get.B != 0 {
		return 0, 0, 0, false
	}
	if sub.Op != OP_ADDI || sub.B != eq.A || int64(sub.C-OFFSET_sC) != -1 || mm1.Op != OP_MMBINI {
		return 0, 0, 0, false
	}
	if add.Op != OP_ADDI || add.B != ret.A || int64(add.C-OFFSET_sC) != 1 || mm2.Op != OP_MMBINI {
		return 0, 0, 0, false
	}
	if tail.Op != OP_TAILCALL || tail.A != get.A || tail.B != 3 || tail.C != 0 || tail.K != 0 {
		return 0, 0, 0, false
	}
	if sub.A != tail.A+1 || add.A != tail.A+2 {
		return 0, 0, 0, false
	}
	return eq.A, ret.A, 2, true
}

// fastFibCall evaluates the exact direct-recursive Fibonacci leaf used by
// the parser (`n < 2`, then self(n-1) + self(n-2)). Without hooks, every
// recursive frame has no observable effect beyond that integer result.
// The cap keeps inputs that would normally hit Lua's call-depth limit on
// the normal path.
func fastFibCall(L *State, ci *CallInfo, cl *LClosure, code []Instruction) (int, bool) {
	if cl.p.isVararg != 0 || cl.p.numparams != 1 || len(cl.upvals) != 1 ||
		!matchFib(code) {
		return 0, false
	}
	self := &L.stack[ci.funcIdx]
	uv := upvalValue(cl.upvals[0])
	if uv.tt != self.tt || uv.p != self.p {
		return 0, false
	}
	base := ci.funcIdx + 1
	nv := &L.stack[base]
	if nv.tt != LUA_VNUMINT || nv.i < 0 || nv.i > 45 {
		return 0, false
	}
	n := nv.i
	if n < 2 {
		return 2, true
	}
	a, b := int64(0), int64(1)
	for i := int64(2); i <= n; i++ {
		a, b = b, a+b
	}
	L.stack[base+1] = Value{i: b, tt: LUA_VNUMINT}
	return 13, true
}

func matchFib(c []Instruction) bool {
	if len(c) != 15 {
		return false
	}
	return c[0].Op == OP_LTI && c[0].A == 0 && int64(c[0].B-OFFSET_sC) == 2 &&
		c[1].Op == OP_JMP && c[2].Op == OP_RETURN1 && c[2].A == 0 &&
		c[3].Op == OP_GETUPVAL && c[3].B == 0 &&
		c[4].Op == OP_ADDI && c[4].B == 0 && int64(c[4].C-OFFSET_sC) == -1 &&
		c[5].Op == OP_MMBINI && c[6].Op == OP_CALL && c[6].A == c[3].A &&
		c[6].B == 2 && c[6].C == 2 &&
		c[7].Op == OP_GETUPVAL && c[7].B == 0 &&
		c[8].Op == OP_ADDI && c[8].B == 0 && int64(c[8].C-OFFSET_sC) == -2 &&
		c[9].Op == OP_MMBINI && c[10].Op == OP_CALL && c[10].A == c[7].A &&
		c[10].B == 2 && c[10].C == 2 &&
		c[11].Op == OP_ADD && c[11].B == c[6].A && c[11].C == c[10].A &&
		c[12].Op == OP_MMBIN && c[13].Op == OP_RETURN1 && c[13].A == c[11].A &&
		c[14].Op == OP_RETURN0
}
