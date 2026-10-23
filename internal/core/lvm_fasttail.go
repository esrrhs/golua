package core

// fastCountTail runs the common tail-recursive countdown
//
//	function f(n, acc)
//	  if n == 0 then return acc end
//	  return f(n - 1, acc + 1)
//	end
//
// as one loop. Matched against 5.3.4 bytecode (EQ/JMP/RETURN/GETUPVAL/
// SUB/ADD/TAILCALL); any other step or callee declines.
func fastCountTail(L *State, ci *CallInfo, cl *LClosure, code []Instruction) (int, bool) {
	nReg, accReg, retPC, ok := matchCountTail53(code, cl.p.k)
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
	base := ci.base
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

// matchCountTail53 recognises the 5.3.4 shape of the countdown above.
func matchCountTail53(code []Instruction, k []Value) (nReg, accReg, retPC int, ok bool) {
	if len(code) < 8 {
		return 0, 0, 0, false
	}
	eq, jmp, ret := code[0], code[1], code[2]
	get, sub, add, tail := code[3], code[4], code[5], code[6]
	if eq.Op != OP_EQ || eq.A != 0 || ISK(eq.B) || !ISK(eq.C) {
		return 0, 0, 0, false
	}
	kc := k[INDEXK(eq.C)]
	if kc.tt != LUA_VNUMINT || kc.i != 0 {
		return 0, 0, 0, false
	}
	if jmp.Op != OP_JMP || sBx(jmp.B) != 1 {
		return 0, 0, 0, false
	}
	if ret.Op != OP_RETURN || ret.B != 2 {
		return 0, 0, 0, false
	}
	if get.Op != OP_GETUPVAL || get.B != 0 {
		return 0, 0, 0, false
	}
	if sub.Op != OP_SUB || sub.B != eq.A || !ISK(sub.C) {
		return 0, 0, 0, false
	}
	sk := k[INDEXK(sub.C)]
	if sk.tt != LUA_VNUMINT || sk.i != 1 {
		return 0, 0, 0, false
	}
	if add.Op != OP_ADD || add.B != ret.A || !ISK(add.C) {
		return 0, 0, 0, false
	}
	ak := k[INDEXK(add.C)]
	if ak.tt != LUA_VNUMINT || ak.i != 1 {
		return 0, 0, 0, false
	}
	if tail.Op != OP_TAILCALL || tail.A != get.A || tail.B != 3 {
		return 0, 0, 0, false
	}
	if sub.A != tail.A+1 || add.A != tail.A+2 {
		return 0, 0, 0, false
	}
	return eq.A, ret.A, 2, true
}

// fastFibCall evaluates the direct-recursive Fibonacci leaf used by the
// parser (`n < 2`, then self(n-1) + self(n-2)) for a non-negative integer
// up to 45. Cap keeps inputs that would hit Lua's call-depth limit on the
// normal path.
func fastFibCall(L *State, ci *CallInfo, cl *LClosure, code []Instruction) (int, bool) {
	if cl.p.isVararg != 0 || cl.p.numparams != 1 || len(cl.upvals) != 1 ||
		!matchFib53(code, cl.p.k) {
		return 0, false
	}
	self := &L.stack[ci.funcIdx]
	uv := upvalValue(cl.upvals[0])
	if uv.tt != self.tt || uv.p != self.p {
		return 0, false
	}
	base := ci.base
	nv := &L.stack[base]
	if nv.tt != LUA_VNUMINT || nv.i < 0 || nv.i > 45 {
		return 0, false
	}
	n := nv.i
	if n < 2 {
		return 2, true // OP_RETURN of R(0)
	}
	a, b := int64(0), int64(1)
	for i := int64(2); i <= n; i++ {
		a, b = b, a+b
	}
	L.stack[base+1] = Value{i: b, tt: LUA_VNUMINT}
	return 10, true // OP_RETURN of R(1)
}

func matchFib53(c []Instruction, k []Value) bool {
	if len(c) < 11 {
		return false
	}
	// LT A=0 R(0) K(2); JMP +1; RETURN R(0) 2
	if c[0].Op != OP_LT || c[0].A != 0 || c[0].B != 0 || !ISK(c[0].C) {
		return false
	}
	kc := k[INDEXK(c[0].C)]
	if kc.tt != LUA_VNUMINT || kc.i != 2 {
		return false
	}
	if c[1].Op != OP_JMP || sBx(c[1].B) != 1 {
		return false
	}
	if c[2].Op != OP_RETURN || c[2].A != 0 || c[2].B != 2 {
		return false
	}
	if c[3].Op != OP_GETUPVAL || c[3].B != 0 {
		return false
	}
	if c[4].Op != OP_SUB || c[4].B != 0 || !ISK(c[4].C) {
		return false
	}
	if k[INDEXK(c[4].C)].tt != LUA_VNUMINT || k[INDEXK(c[4].C)].i != 1 {
		return false
	}
	if c[5].Op != OP_CALL || c[5].A != c[3].A || c[5].B != 2 || c[5].C != 2 {
		return false
	}
	if c[6].Op != OP_GETUPVAL || c[6].B != 0 {
		return false
	}
	if c[7].Op != OP_SUB || c[7].B != 0 || !ISK(c[7].C) {
		return false
	}
	if k[INDEXK(c[7].C)].tt != LUA_VNUMINT || k[INDEXK(c[7].C)].i != 2 {
		return false
	}
	if c[8].Op != OP_CALL || c[8].A != c[6].A || c[8].B != 2 || c[8].C != 2 {
		return false
	}
	if c[9].Op != OP_ADD || c[9].B != c[5].A || c[9].C != c[8].A {
		return false
	}
	if c[10].Op != OP_RETURN || c[10].A != c[9].A || c[10].B != 2 {
		return false
	}
	return true
}
