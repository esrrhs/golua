package core

const (
	fastLeafUnknown = iota
	fastLeafNo
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
		if numericLeaf(p) {
			p.fastLeaf = fastLeafNumeric
		} else {
			p.fastLeaf = fastLeafNo
		}
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
