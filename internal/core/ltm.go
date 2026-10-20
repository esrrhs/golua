// Translation of ltm.h / ltm.c (PUC-Lua 5.4).
package core

// TMS: tag-method names, in the same order as PUC-Lua.
type TMS int

const (
	TM_INDEX TMS = iota
	TM_NEWINDEX
	TM_GC
	TM_MODE
	TM_LEN
	TM_EQ
	TM_ADD
	TM_SUB
	TM_MUL
	TM_MOD
	TM_POW
	TM_DIV
	TM_IDIV
	TM_BAND
	TM_BOR
	TM_BXOR
	TM_SHL
	TM_SHR
	TM_UNM
	TM_BNOT
	TM_LT
	TM_LE
	TM_CONCAT
	TM_CALL
	TM_CLOSE
	TM_N
)

var nilObject = Value{tt: LUA_VNIL}

var luaT_eventname = [TM_N]string{
	"__index", "__newindex",
	"__gc", "__mode", "__len", "__eq",
	"__add", "__sub", "__mul", "__mod", "__pow",
	"__div", "__idiv",
	"__band", "__bor", "__bxor", "__shl", "__shr",
	"__unm", "__bnot", "__lt", "__le",
	"__concat", "__call", "__close",
}

var luaT_typenames = [LUA_NUMTYPES + 3]string{
	"no value",
	"nil", "boolean", "userdata", "number",
	"string", "table", "function", "userdata", "thread",
	"upvalue", "proto",
}

// luaT_init initialises the tag-method names (ltm.c).
func luaT_init(L *State) {
	for i := 0; i < int(TM_N); i++ {
		L.g.tmNames[i] = luaS_new(L, luaT_eventname[i])
	}
}

// cvt2str reports whether a value can be converted to a string (lvm.h).
func cvt2str(o *Value) bool { return ttisnumber(o) }

// notm reports whether a metamethod is absent.
func notm(tm *Value) bool { return tm == nil || ttisnil(tm) }

// fasttm is the optimized metamethod lookup (ltm.h).
func fasttm(L *State, et *Table, e TMS) *Value {
	if et == nil {
		return nil
	}
	if (et.flags & (1 << uint(e))) != 0 {
		return nil
	}
	return luaT_gettm(et, e, L.g.tmNames[e])
}

// luaT_gettm looks up a metamethod in a metatable (ltm.c).
func luaT_gettm(events *Table, event TMS, ename *TString) *Value {
	tm := luaH_getshortstr(events, ename)
	if notm(tm) { /* no tag method? */
		events.flags |= byte(1 << uint(event)) /* cache this fact */
		return nil
	}
	return tm
}

// luaT_gettmbyobj looks up a metamethod for an object (ltm.c).
func luaT_gettmbyobj(L *State, o *Value, event TMS) *Value {
	var mt *Table
	switch ttype(o) {
	case LUA_TTABLE:
		mt = hvalue(o).metatable
	case LUA_TUSERDATA:
		mt = uvalue(o).metatable
	default:
		mt = L.g.mt[ttype(o)]
	}
	if mt == nil {
		return &nilObject
	}
	return luaH_getshortstr(mt, L.g.tmNames[event])
}

// luaT_objtypename returns the type name of an object, honouring __name.
func luaT_objtypename(L *State, o *Value) string {
	var mt *Table
	if ttistable(o) {
		mt = hvalue(o).metatable
	} else if ttisfulluserdata(o) {
		mt = uvalue(o).metatable
	}
	if mt != nil {
		name := luaH_getshortstr(mt, luaS_new(L, "__name"))
		if ttisstring(name) {
			/* C hands the name back as a 'const char *' for whoever
			   asked to format it with '%s', which stops at the zero. */
			return luaCStr(tsvalue(name).val)
		}
	}
	switch ttype(o) {
	case LUA_TLIGHTUSERDATA, LUA_TUSERDATA:
		return "userdata"
	}
	return luaT_typenames[ttype(o)+1]
}

// luaT_callTM calls a metamethod with three arguments (ltm.c).
func luaT_callTM(L *State, f, p1, p2, p3 *Value) {
	funcIdx := L.top
	L.checkStack(4)
	L.stack[funcIdx] = *f
	L.stack[funcIdx+1] = *p1
	L.stack[funcIdx+2] = *p2
	L.stack[funcIdx+3] = *p3
	L.top = funcIdx + 4
	if isLuacode(L.ci) {
		luaD_call(L, funcIdx, 0)
	} else {
		luaD_callnoyield(L, funcIdx, 0)
	}
}

// luaT_callTMres calls a metamethod with two arguments and moves the
// result to 'res' (ltm.c).
func luaT_callTMres(L *State, f, p1, p2 *Value, res int) {
	funcIdx := L.top
	L.checkStack(3)
	L.stack[funcIdx] = *f
	L.stack[funcIdx+1] = *p1
	L.stack[funcIdx+2] = *p2
	L.top = funcIdx + 3
	if isLuacode(L.ci) {
		luaD_call(L, funcIdx, 1)
	} else {
		luaD_callnoyield(L, funcIdx, 1)
	}
	L.stack[res] = L.stack[L.top-1]
	L.top--
}

func callbinTM(L *State, p1, p2 *Value, res int, event TMS) bool {
	tm := luaT_gettmbyobj(L, p1, event) /* try first operand */
	if notm(tm) {
		tm = luaT_gettmbyobj(L, p2, event) /* try second operand */
	}
	if notm(tm) {
		return false
	}
	luaT_callTMres(L, tm, p1, p2, res)
	return true
}

// luaT_trybinTM tries a binary metamethod; raises an error if there is none.
func luaT_trybinTM(L *State, p1, p2 *Value, res int, event TMS) {
	if !callbinTM(L, p1, p2, res, event) {
		switch event {
		case TM_BAND, TM_BOR, TM_BXOR, TM_SHL, TM_SHR, TM_BNOT:
			if ttisnumber(p1) && ttisnumber(p2) {
				luaG_tointerror(L, p1, p2)
			} else {
				luaG_opinterror(L, p1, p2, "perform bitwise operation on")
			}
		default:
			luaG_opinterror(L, p1, p2, "perform arithmetic on")
		}
	}
}

// luaT_tryconcatTM concatenates the two values on top of the stack,
// using the __concat metamethod if needed (ltm.c).
func luaT_tryconcatTM(L *State) {
	top := L.top
	if !callbinTM(L, &L.stack[top-2], &L.stack[top-1], top-2, TM_CONCAT) {
		luaG_concaterror(L, &L.stack[top-2], &L.stack[top-1])
	}
}

// luaT_trybinassocTM tries p1's metamethod, then p2's (comparisons).
func luaT_trybinassocTM(L *State, p1, p2 *Value, flip bool, res int, event TMS) {
	if flip {
		luaT_trybinTM(L, p2, p1, res, event)
	} else {
		luaT_trybinTM(L, p1, p2, res, event)
	}
}

// luaT_trybiniTM tries a metamethod with an integer second operand.
func luaT_trybiniTM(L *State, p1 *Value, i2 int64, flip bool, res int, event TMS) {
	var aux Value
	setivalue(&aux, i2)
	luaT_trybinassocTM(L, p1, &aux, flip, res, event)
}

// luaT_callorderTM calls an order metamethod; raises an error if none.
//
// 'LUA_COMPAT_LT_LE' is on, as it is in the reference build: a value with
// no '__le' still answers '<=' by trying '!(p2 < p1)'.  Without that a
// type that only defines '__lt' cannot be ordered at all, and 'a <= b'
// would raise "attempt to compare two table values" where PUC-Lua answers
// false.
//
// deviation: C marks the call with 'CIST_LEQ' so that the VM can negate
// the result if the metamethod yields across the comparison.  This port
// runs the metamethod to completion inside 'callbinTM', so the result is
// already there and the negation happens here instead.
func luaT_callorderTM(L *State, p1, p2 *Value, event TMS) bool {
	if callbinTM(L, p1, p2, L.top, event) { /* try original event */
		return !isFalse(&L.stack[L.top])
	}
	if event == TM_LE {
		/* try '!(p2 < p1)' for '(p1 <= p2)' */
		if callbinTM(L, p2, p1, L.top, TM_LT) { /* inverted operands */
			return isFalse(&L.stack[L.top])
		}
	}
	luaG_ordererror(L, p1, p2) /* no metamethod found */
	return false
}

// luaT_callorderiTM is luaT_callorderTM with an integer/float operand.
func luaT_callorderiTM(L *State, p1 *Value, v2 int64, flip, isfloat bool, event TMS) bool {
	var aux, p2 *Value
	if isfloat {
		var f Value
		setfltvalue(&f, float64(v2))
		aux = &f
	} else {
		var i Value
		setivalue(&i, v2)
		aux = &i
	}
	if flip { /* arguments were exchanged? */
		p2 = p1
		p1 = aux
	} else {
		p2 = aux
	}
	return luaT_callorderTM(L, p1, p2, event)
}

// luaT_adjustvarargs moves the function and its fixed parameters above
// the extra arguments of a vararg function (ltm.c).
func luaT_adjustvarargs(L *State, nfixparams int, ci *CallInfo, p *Proto) {
	actual := L.top - ci.funcIdx - 1 // number of arguments
	nextra := actual - nfixparams
	ci.nExtraArgs = nextra
	L.checkStack(int(p.maxstacksize) + 1)
	/* copy function to the top of the stack */
	L.stack[L.top] = L.stack[ci.funcIdx]
	L.top++
	/* move fixed parameters to the top of the stack */
	for i := 1; i <= nfixparams; i++ {
		L.stack[L.top] = L.stack[ci.funcIdx+i]
		L.top++
		setnilvalue(&L.stack[ci.funcIdx+i])
	}
	ci.funcIdx += actual + 1
	ci.top += actual + 1
}

// luaT_getvarargs moves 'wanted' extra arguments to 'where' (ltm.c).
func luaT_getvarargs(L *State, ci *CallInfo, where int, wanted int) {
	nextra := ci.nExtraArgs
	if wanted < 0 {
		wanted = nextra
		L.checkStack(nextra)
		L.top = where + nextra
	}
	i := 0
	for ; i < wanted && i < nextra; i++ {
		L.stack[where+i] = L.stack[ci.funcIdx-nextra+i]
	}
	for ; i < wanted; i++ { /* complete with nil */
		setnilvalue(&L.stack[where+i])
	}
}
