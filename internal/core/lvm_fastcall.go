package core

const (
	fastLeafUnknown = iota
	fastLeafNo
	fastLeafConstant
	fastLeafNumeric
)

// fastLeafCall evaluates a straight-line numeric Lua function in the
// caller. A regular Lua call prepares a CallInfo, changes frames, restarts
// the dispatch loop, executes the body, moves the result and restarts the
// caller. For a leaf such as
//
//	return a + b + c
//
// all that machinery is more expensive than the arithmetic. The
// prototype is classified once; operands are still checked on every call,
// so strings, metamethods and any other non-number take the normal path.
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
		switch load.Op {
		case OP_LOADI:
			setivalue(&L.stack[funcIdx], int64(load.B-OFFSET_sBx))
		case OP_LOADF:
			setfltvalue(&L.stack[funcIdx], float64(load.B-OFFSET_sBx))
		case OP_LOADK:
			L.stack[funcIdx] = p.k[load.B]
		default:
			return false
		}
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

	retpc := len(p.code) - 1
	if p.code[retpc].Op == OP_RETURN0 {
		retpc--
	}
	if fastExecBody(&ri, &rt, p.k, p.code[:retpc]) >= 0 {
		return false
	}
	r := p.code[retpc].A
	L.stack[funcIdx] = Value{i: ri[r], tt: rt[r]}
	L.top = funcIdx + 1
	return true
}

// constantLeaf recognizes a function whose only observable work (without
// hooks) is returning one numeric constant. It is common as a callback,
// sentinel, and benchmark floor; preparing a Lua frame costs far more
// than the load itself.
func constantLeaf(p *Proto) bool {
	if p.isVararg != 0 || len(p.code) != 3 ||
		p.code[1].Op != OP_RETURN1 || p.code[1].A != p.code[0].A ||
		p.code[2].Op != OP_RETURN0 {
		return false
	}
	switch load := p.code[0]; load.Op {
	case OP_LOADI, OP_LOADF:
		return true
	case OP_LOADK:
		return load.B >= 0 && load.B < len(p.k) &&
			ttisnumber(&p.k[load.B])
	default:
		return false
	}
}

func numericLeaf(p *Proto) bool {
	if p.isVararg != 0 || int(p.maxstacksize) > fastForMax ||
		len(p.code) < 2 {
		return false
	}
	retpc := len(p.code) - 1
	if p.code[retpc].Op == OP_RETURN0 {
		retpc--
	}
	if retpc <= 0 || p.code[retpc].Op != OP_RETURN1 ||
		p.code[retpc].A < 0 || p.code[retpc].A >= int(p.maxstacksize) {
		return false
	}
	return fastForBody(p.code[:retpc], int(p.maxstacksize), len(p.k), p.k)
}
