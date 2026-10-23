package core

import "math"

const (
	fastLeafUnknown = iota
	fastLeafNo
	fastLeafConstant
	fastLeafNumeric
)

// fastLeafCall evaluates a straight-line numeric Lua function in the
// caller without building a CallInfo. Adapted for Lua 5.3.4 RK encoding:
// constant leaves are LOADK + RETURN A 2 + RETURN 0 1; numeric leaves are
// MOVE/LOADK/arith-with-RK ending the same way (no OP_RETURN0/1, LOADI, MMBIN).
func fastLeafCall(L *State, funcIdx, nargs int) bool {
	fv := &L.stack[funcIdx]
	if fv.tt != ctb(LUA_VLCL) {
		return false
	}
	p := clLvalue(fv).p
	if p.fastLeaf == fastLeafUnknown {
		if constantLeaf(p) {
			p.fastLeaf = fastLeafConstant
		} else if numericLeaf(p) {
			p.fastLeaf = fastLeafNumeric
		} else {
			p.fastLeaf = fastLeafNo
		}
	}
	if p.fastLeaf == fastLeafConstant {
		load := p.code[0]
		if load.Op != OP_LOADK || load.B < 0 || load.B >= len(p.k) {
			return false
		}
		L.stack[funcIdx] = p.k[load.B]
		L.top = funcIdx + 1
		return true
	}
	if p.fastLeaf != fastLeafNumeric {
		return false
	}

	nreg := int(p.maxstacksize)
	if nreg <= 0 || nreg > fastForMax {
		return false
	}
	var ri [fastForMax]int64
	var rt [fastForMax]uint8
	nparams := int(p.numparams)
	for i := 0; i < nparams; i++ {
		if i >= nargs {
			rt[i] = LUA_VNIL
			continue
		}
		v := &L.stack[funcIdx+1+i]
		if v.tt != LUA_VNUMINT && v.tt != LUA_VNUMFLT {
			return false
		}
		ri[i], rt[i] = v.i, v.tt
	}

	/* 5.3: trailing RETURN 0 1 after RETURN A 2. */
	retpc := len(p.code) - 1
	if p.code[retpc].Op == OP_RETURN && p.code[retpc].B == 1 {
		retpc--
	}
	if retpc < 0 || p.code[retpc].Op != OP_RETURN || p.code[retpc].B != 2 {
		return false
	}
	if fastExecBody53(&ri, &rt, p.k, p.code[:retpc]) >= 0 {
		return false
	}
	r := p.code[retpc].A
	L.stack[funcIdx] = Value{i: ri[r], tt: rt[r]}
	L.top = funcIdx + 1
	return true
}

// constantLeaf: LOADK of a number, RETURN A 2, RETURN 0 1.
func constantLeaf(p *Proto) bool {
	if p.isVararg != 0 || len(p.code) != 3 {
		return false
	}
	if p.code[1].Op != OP_RETURN || p.code[1].B != 2 || p.code[1].A != p.code[0].A {
		return false
	}
	if p.code[2].Op != OP_RETURN || p.code[2].B != 1 {
		return false
	}
	load := p.code[0]
	return load.Op == OP_LOADK && load.B >= 0 && load.B < len(p.k) &&
		ttisnumber(&p.k[load.B])
}

func numericLeaf(p *Proto) bool {
	if p.isVararg != 0 || int(p.maxstacksize) > fastForMax || len(p.code) < 2 {
		return false
	}
	retpc := len(p.code) - 1
	if p.code[retpc].Op == OP_RETURN && p.code[retpc].B == 1 {
		retpc--
	}
	if retpc <= 0 || p.code[retpc].Op != OP_RETURN || p.code[retpc].B != 2 ||
		p.code[retpc].A < 0 || p.code[retpc].A >= int(p.maxstacksize) {
		return false
	}
	return fastForBody53(p.code[:retpc], int(p.maxstacksize), len(p.k), p.k)
}

// fastForBody53 accepts only 5.3 RK-safe straight-line numeric ops.
func fastForBody53(body []Instruction, nreg, nk int, k []Value) bool {
	for i := 0; i < len(body); i++ {
		ins := body[i]
		switch ins.Op {
		case OP_MOVE:
			if ins.A >= nreg || ins.B >= nreg {
				return false
			}
		case OP_LOADK:
			if ins.A >= nreg || ins.B >= nk || !ttisnumber(&k[ins.B]) {
				return false
			}
		case OP_LOADNIL:
			if ins.A >= nreg || ins.A+ins.B >= nreg {
				return false
			}
		case OP_ADD, OP_SUB, OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
			OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			if ins.A >= nreg {
				return false
			}
			if !fastRKOk(ins.B, nreg, nk, k) || !fastRKOk(ins.C, nreg, nk, k) {
				return false
			}
		case OP_UNM, OP_BNOT:
			if ins.A >= nreg || !fastRKOk(ins.B, nreg, nk, k) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func fastRKOk(rk, nreg, nk int, k []Value) bool {
	if ISK(rk) {
		idx := INDEXK(rk)
		return idx >= 0 && idx < nk && ttisnumber(&k[idx])
	}
	return rk >= 0 && rk < nreg
}

func fastRKVal(ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, rk int) (tt uint8, bits int64, ok bool) {
	if ISK(rk) {
		kv := k[INDEXK(rk)]
		if !ttisnumber(&kv) {
			return 0, 0, false
		}
		return kv.tt, kv.i, true
	}
	if rk < 0 || rk >= fastForMax {
		return 0, 0, false
	}
	tt = rt[rk]
	if tt != LUA_VNUMINT && tt != LUA_VNUMFLT {
		return 0, 0, false
	}
	return tt, ri[rk], true
}

// fastExecBody53 runs a 5.3 RK numeric body. Returns -1 on success, or the
// index of the instruction that needs the slow path.
func fastExecBody53(ri *[fastForMax]int64, rt *[fastForMax]uint8, k []Value, body []Instruction) int {
	for bi := 0; bi < len(body); bi++ {
		ins := body[bi]
		a := ins.A
		switch ins.Op {
		case OP_MOVE:
			ri[a] = ri[ins.B]
			rt[a] = rt[ins.B]
		case OP_LOADK:
			kv := k[ins.B]
			ri[a] = kv.i
			rt[a] = kv.tt
		case OP_LOADNIL:
			for r := a; r <= a+ins.B; r++ {
				rt[r] = LUA_VNIL
			}
		case OP_ADD, OP_SUB, OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
			OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
			xt, xb, okx := fastRKVal(ri, rt, k, ins.B)
			yt, yb, oky := fastRKVal(ri, rt, k, ins.C)
			if !okx || !oky {
				return bi
			}
			tt, bits, ok := fastBin53(ins.Op, xt, yt, xb, yb)
			if !ok {
				return bi
			}
			ri[a] = bits
			rt[a] = tt
		case OP_UNM:
			xt, xb, okx := fastRKVal(ri, rt, k, ins.B)
			if !okx {
				return bi
			}
			if xt == LUA_VNUMINT {
				ri[a] = -xb
				rt[a] = LUA_VNUMINT
			} else {
				r := -math.Float64frombits(uint64(xb))
				ri[a] = int64(math.Float64bits(r))
				rt[a] = LUA_VNUMFLT
			}
		case OP_BNOT:
			xt, xb, okx := fastRKVal(ri, rt, k, ins.B)
			if !okx || xt != LUA_VNUMINT {
				return bi
			}
			ri[a] = ^xb
			rt[a] = LUA_VNUMINT
		default:
			return bi
		}
	}
	return -1
}

func fastBin53(op int, xt, yt uint8, x, y int64) (uint8, int64, bool) {
	if xt == LUA_VNUMINT && yt == LUA_VNUMINT {
		switch op {
		case OP_ADD:
			return LUA_VNUMINT, x + y, true
		case OP_SUB:
			return LUA_VNUMINT, x - y, true
		case OP_MUL:
			return LUA_VNUMINT, x * y, true
		case OP_BAND:
			return LUA_VNUMINT, x & y, true
		case OP_BOR:
			return LUA_VNUMINT, x | y, true
		case OP_BXOR:
			return LUA_VNUMINT, x ^ y, true
		case OP_SHL:
			return LUA_VNUMINT, luaV_shiftl(x, y), true
		case OP_SHR:
			return LUA_VNUMINT, luaV_shiftl(x, -y), true
		case OP_IDIV:
			if y == 0 {
				return 0, 0, false
			}
			return LUA_VNUMINT, luaV_idivcore(x, y), true
		case OP_MOD:
			if y == 0 {
				return 0, 0, false
			}
			return LUA_VNUMINT, luaV_modcore(x, y), true
		}
	}
	n1, ok1 := fastNum(xt, x)
	n2, ok2 := fastNum(yt, y)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	var r float64
	switch op {
	case OP_ADD:
		r = n1 + n2
	case OP_SUB:
		r = n1 - n2
	case OP_MUL:
		r = n1 * n2
	case OP_DIV:
		r = n1 / n2
	case OP_POW:
		r = math.Pow(n1, n2)
	case OP_IDIV:
		r = math.Floor(n1 / n2)
	case OP_MOD:
		r = n1 - math.Floor(n1/n2)*n2
	default:
		return 0, 0, false
	}
	return LUA_VNUMFLT, int64(math.Float64bits(r)), true
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
