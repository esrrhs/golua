// Translation of ltm.h / ltm.c (PUC-Lua 5.4.9): Tag methods.
package core

/*
 * WARNING: if you change the order of this enumeration,
 * grep "ORDER TM" and "ORDER OP"
 */
// TMS: tag-method names, same order as PUC-Lua 5.4.9 ltm.h (incl. TM_CLOSE).
type TMS int

const (
	TM_INDEX TMS = iota
	TM_NEWINDEX
	TM_GC
	TM_MODE
	TM_LEN
	TM_EQ /* last tag method with fast access */
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
	TM_N /* number of elements in the enum */
)

/*
** Mask with 1 in all fast-access methods. A 1 in any of these bits
** in the flag of a (meta)table means the metatable does not have the
** corresponding metamethod field. (Bit 7 of the flag is used for
** 'isrealasize'.)
** C: #define maskflags (~(~0u << (TM_EQ + 1)))
** Defined/used in ltable.go (same value 0x3F); not duplicated here.
 */

/*
** Test whether there is no tagmethod.
** (Because tagmethods use raw accesses, the result may be an "empty" nil.)
** C: #define notm(tm) ttisnil(tm)
** Go also accepts a nil *Value (luaT_gettm / fasttm return nil when absent).
 */
func notm(tm *Value) bool { return tm == nil || ttisnil(tm) }

/*
** function to be used with macro "fasttm": optimized for absence of
** tag methods. C: gfasttm / fasttm macros in ltm.h.
 */
func gfasttm(g *globalState, et *Table, e TMS) *Value {
	if et == nil {
		return nil
	}
	if (et.flags & (1 << uint(e))) != 0 {
		return nil
	}
	return luaT_gettm(et, e, g.tmNames[e])
}

func fasttm(L *State, et *Table, e TMS) *Value {
	return gfasttm(L.g, et, e)
}

// ttypename is C #define ttypename(x) luaT_typenames_[(x) + 1]
func ttypename(x int) string { return luaT_typenames_[x+1] }

/* C shares one static udatatypename[] for light + full userdata slots */
const udatatypename = "userdata"

/* luaT_typenames_ — C: LUAI_DDEF const char *const luaT_typenames_[LUA_TOTALTYPES] */
var luaT_typenames_ = [LUA_TOTALTYPES]string{
	"no value",
	"nil", "boolean", udatatypename, "number",
	"string", "table", "function", udatatypename, "thread",
	"upvalue", "proto", /* these last cases are used for tests only */
}

// luaT_typenames keeps the historical name used by lapi.go (C: luaT_typenames_).
var luaT_typenames = luaT_typenames_

// luaT_init initialises the tag-method names (ltm.c).
// FLAG: C also luaC_fix's each name; omitted — intentional Go-GC (BRIEF KEEP).
func luaT_init(L *State) {
	/* ORDER TM */
	luaT_eventname := [TM_N]string{
		"__index", "__newindex",
		"__gc", "__mode", "__len", "__eq",
		"__add", "__sub", "__mul", "__mod", "__pow",
		"__div", "__idiv",
		"__band", "__bor", "__bxor", "__shl", "__shr",
		"__unm", "__bnot", "__lt", "__le",
		"__concat", "__call", "__close",
	}
	for i := 0; i < int(TM_N); i++ {
		L.g.tmNames[i] = luaS_new(L, luaT_eventname[i])
		/* C: luaC_fix(L, obj2gco(G(L)->tmname[i])); — never collect these names */
	}
}

// cvt2str reports whether a value can be converted to a string (lvm.h).
// FLAG: placement — lives in C lvm.h; kept here because callers (lapi/lvm/ldebug)
// already import via this package symbol. Apply debt: move to lvm.go on merge if desired.
func cvt2str(o *Value) bool { return ttisnumber(o) }

// luaT_gettm looks up a metamethod in a metatable (ltm.c).
func luaT_gettm(events *Table, event TMS, ename *TString) *Value {
	tm := luaH_getshortstr(events, ename)
	/* lua_assert(event <= TM_EQ) */
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
	if mt != nil {
		return luaH_getshortstr(mt, L.g.tmNames[event])
	}
	return &L.g.nilValue /* C: &G(L)->nilvalue */
}

/*
** Return the name of the type of an object. For tables and userdata
** with metatable, use their '__name' metafield, if present.
 */
func luaT_objtypename(L *State, o *Value) string {
	var mt *Table
	/* C: if ((ttistable(o) && (mt = hvalue(o)->metatable) != NULL) ||
	   (ttisfulluserdata(o) && (mt = uvalue(o)->metatable) != NULL)) */
	if ttistable(o) {
		mt = hvalue(o).metatable
	}
	if mt == nil && ttisfulluserdata(o) {
		mt = uvalue(o).metatable
	}
	if mt != nil {
		name := luaH_getshortstr(mt, luaS_new(L, "__name"))
		if ttisstring(name) { /* is '__name' a string? */
			/* C: return getstr(tsvalue(name));  (%s stops at NUL) */
			return luaCStr(getstr(tsvalue(name))) /* use it as type name */
		}
	}
	return ttypename(ttype(o)) /* else use standard type name */
}

// luaT_callTM calls a metamethod with three arguments (ltm.c).
// FLAG: migrateNested — intentional Go coroutine bridge (BRIEF KEEP / task OK).
// C assumes EXTRA_STACK; no luaD_checkstack here.
func luaT_callTM(L *State, f, p1, p2, p3 *Value) {
	L.migrateNested()
	funcIdx := L.top
	L.stack[funcIdx] = *f    /* push function (assume EXTRA_STACK) */
	L.stack[funcIdx+1] = *p1 /* 1st argument */
	L.stack[funcIdx+2] = *p2 /* 2nd argument */
	L.stack[funcIdx+3] = *p3 /* 3rd argument */
	L.top = funcIdx + 4
	/* metamethod may yield only when called from Lua code */
	if isLuacode(L.ci) {
		luaD_call(L, funcIdx, 0)
	} else {
		luaD_callnoyield(L, funcIdx, 0)
	}
}

// luaT_callTMres calls a metamethod with two arguments and moves the
// result to 'res' (ltm.c).
// FLAG: migrateNested — intentional Go coroutine bridge (BRIEF KEEP / task OK).
// C uses savestack/restorestack because StkId is a pointer; Go res is a
// stable stack index across realloc. C assumes EXTRA_STACK.
func luaT_callTMres(L *State, f, p1, p2 *Value, res int) {
	L.migrateNested()
	result := res /* C: ptrdiff_t result = savestack(L, res) */
	funcIdx := L.top
	L.stack[funcIdx] = *f    /* push function (assume EXTRA_STACK) */
	L.stack[funcIdx+1] = *p1 /* 1st argument */
	L.stack[funcIdx+2] = *p2 /* 2nd argument */
	L.top = funcIdx + 3
	/* metamethod may yield only when called from Lua code */
	if isLuacode(L.ci) {
		luaD_call(L, funcIdx, 1)
	} else {
		luaD_callnoyield(L, funcIdx, 1)
	}
	/* C: res = restorestack(L, result); setobjs2s(L, res, --L->top.p) */
	L.top--
	L.stack[result] = L.stack[L.top] /* move result to its place */
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
	if !callbinTM(L, p1, p2, res, event) { /* l_unlikely */
		switch event {
		case TM_BAND, TM_BOR, TM_BXOR, TM_SHL, TM_SHR, TM_BNOT:
			if ttisnumber(p1) && ttisnumber(p2) {
				luaG_tointerror(L, p1, p2)
			} else {
				luaG_opinterror(L, p1, p2, "perform bitwise operation on")
			}
			/* C FALLTHROUGH after noreturn errors — Go error path does not return */
		default:
			luaG_opinterror(L, p1, p2, "perform arithmetic on")
		}
	}
}

// luaT_tryconcatTM concatenates the two values on top of the stack,
// using the __concat metamethod if needed (ltm.c).
func luaT_tryconcatTM(L *State) {
	top := L.top
	if !callbinTM(L, &L.stack[top-2], &L.stack[top-1], top-2, TM_CONCAT) { /* l_unlikely */
		luaG_concaterror(L, &L.stack[top-2], &L.stack[top-1])
	}
}

// luaT_trybinassocTM tries binary TM with optional flipped operands (ltm.c).
func luaT_trybinassocTM(L *State, p1, p2 *Value, flip bool, res int, event TMS) {
	if flip {
		luaT_trybinTM(L, p2, p1, res, event)
	} else {
		luaT_trybinTM(L, p1, p2, res, event)
	}
}

// luaT_trybiniTM tries a metamethod with an integer second operand (ltm.c).
func luaT_trybiniTM(L *State, p1 *Value, i2 int64, flip bool, res int, event TMS) {
	var aux Value
	setivalue(&aux, i2)
	luaT_trybinassocTM(L, p1, &aux, flip, res, event)
}

/*
** Calls an order tag method.
** For lessequal, LUA_COMPAT_LT_LE keeps compatibility with old
** behavior: if there is no '__le', try '__lt', based on l <= r iff
** !(r < l) (assuming a total order). If the metamethod yields during
** this substitution, the continuation has to know about it (to negate
** the result of r<l); bit CIST_LEQ in the call status keeps that
** information.
**
** LUA_COMPAT_LT_LE is defined in PUC 5.4.9 luaconf.h — always on here.
**
** FLAG: C marks the call with CIST_LEQ so the VM can negate the result
** if the metamethod yields across the comparison. This port runs the
** metamethod to completion inside callbinTM (migrateNested), so the
** result is already there and the negation happens here instead.
** CIST_LEQ is defined here (belongs in lstate.h; not yet in lstate.go).
 */
// CIST_LEQ is lstate.h's bit under LUA_COMPAT_LT_LE: using __lt for __le.
const CIST_LEQ = uint16(1 << 13)

func luaT_callorderTM(L *State, p1, p2 *Value, event TMS) bool {
	if callbinTM(L, p1, p2, L.top, event) { /* try original event */
		return !l_isfalse(&L.stack[L.top])
	}
	/* #if defined(LUA_COMPAT_LT_LE) — always on in 5.4.9 */
	if event == TM_LE {
		/* try '!(p2 < p1)' for '(p1 <= p2)' */
		L.ci.callstatus |= CIST_LEQ /* mark it is doing 'lt' for 'le' */
		if callbinTM(L, p2, p1, L.top, TM_LT) {
			L.ci.callstatus ^= CIST_LEQ /* clear mark */
			return l_isfalse(&L.stack[L.top])
		}
		/* else error will remove this 'ci'; no need to clear mark */
	}
	luaG_ordererror(L, p1, p2) /* no metamethod found */
	return false               /* to avoid warnings */
}

// luaT_callorderiTM is luaT_callorderTM with an integer/float operand (ltm.c).
// C takes int v2; Go uses int64 (lua_Integer / opcode immediate as int64).
func luaT_callorderiTM(L *State, p1 *Value, v2 int64, flip, isfloat bool, event TMS) bool {
	var aux Value
	var p2 *Value
	if isfloat {
		setfltvalue(&aux, float64(v2))
	} else {
		setivalue(&aux, v2)
	}
	if flip { /* arguments were exchanged? */
		p2 = p1
		p1 = &aux /* correct them */
	} else {
		p2 = &aux
	}
	return luaT_callorderTM(L, p1, p2, event)
}

// luaT_adjustvarargs moves the function and its fixed parameters above
// the extra arguments of a vararg function (ltm.c).
func luaT_adjustvarargs(L *State, nfixparams int, ci *CallInfo, p *Proto) {
	actual := L.top - ci.funcIdx - 1 /* number of arguments */
	nextra := actual - nfixparams    /* number of extra arguments */
	ci.nExtraArgs = nextra
	L.checkStack(int(p.maxstacksize) + 1) /* C: luaD_checkstack */
	/* copy function to the top of the stack */
	L.stack[L.top] = L.stack[ci.funcIdx]
	L.top++
	/* move fixed parameters to the top of the stack */
	for i := 1; i <= nfixparams; i++ {
		L.stack[L.top] = L.stack[ci.funcIdx+i]
		L.top++
		setnilvalue(&L.stack[ci.funcIdx+i]) /* erase original parameter (for GC) */
	}
	ci.funcIdx += actual + 1
	ci.top += actual + 1
	/* lua_assert(L->top.p <= ci->top.p && ci->top.p <= L->stack_last.p) */
}

// luaT_getvarargs moves 'wanted' extra arguments to 'where' (ltm.c).
func luaT_getvarargs(L *State, ci *CallInfo, where int, wanted int) {
	nextra := ci.nExtraArgs
	if wanted < 0 {
		wanted = nextra        /* get all extra arguments available */
		L.checkStack(nextra)   /* C: checkstackGCp(L, nextra, where) — GC skipped */
		L.top = where + nextra /* next instruction will need top */
	}
	i := 0
	for ; i < wanted && i < nextra; i++ {
		L.stack[where+i] = L.stack[ci.funcIdx-nextra+i]
	}
	for ; i < wanted; i++ { /* complete required results with nil */
		setnilvalue(&L.stack[where+i])
	}
}
