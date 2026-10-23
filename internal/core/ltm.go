// Translation of ltm.h / ltm.c (PUC-Lua 5.3.4).
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
	TM_N
)

// luaO_nilobject is the singleton nil TValue (lobject.c luaO_nilobject_).
var luaO_nilobject = Value{tt: LUA_TNIL}

// nilObject keeps older call sites compiling.
var nilObject = luaO_nilobject

var luaT_eventname = [TM_N]string{
	"__index", "__newindex",
	"__gc", "__mode", "__len", "__eq",
	"__add", "__sub", "__mul", "__mod", "__pow",
	"__div", "__idiv",
	"__band", "__bor", "__bxor", "__shl", "__shr",
	"__unm", "__bnot", "__lt", "__le",
	"__concat", "__call",
}

// luaT_typenames_ (ltm.c 5.3.4): indices are ttnov+1; last is proto for tests.
var luaT_typenames = [LUA_TOTALTAGS]string{
	"no value",
	"nil", "boolean", "userdata", "number",
	"string", "table", "function", "userdata", "thread",
	"proto",
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
		return &luaO_nilobject
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
	L.migrateNested()
	funcIdx := L.top
	L.checkStack(4)
	L.stack[funcIdx] = *f
	L.stack[funcIdx+1] = *p1
	L.stack[funcIdx+2] = *p2
	L.stack[funcIdx+3] = *p3
	L.top = funcIdx + 4
	if isLua(L.ci) {
		luaD_call(L, funcIdx, 0)
	} else {
		luaD_callnoyield(L, funcIdx, 0)
	}
}

// luaT_callTMres calls a metamethod with two arguments and moves the
// result to 'res' (ltm.c).
func luaT_callTMres(L *State, f, p1, p2 *Value, res int) {
	L.migrateNested()
	funcIdx := L.top
	L.checkStack(3)
	L.stack[funcIdx] = *f
	L.stack[funcIdx+1] = *p1
	L.stack[funcIdx+2] = *p2
	L.top = funcIdx + 3
	if isLua(L.ci) {
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
		case TM_CONCAT:
			luaG_concaterror(L, p1, p2)
		case TM_BAND, TM_BOR, TM_BXOR, TM_SHL, TM_SHR, TM_BNOT:
			/* 5.3: tonumber (with string coerce), not ttisnumber */
			var dummy float64
			if luaV_tonumber_(L, p1, &dummy) && luaV_tonumber_(L, p2, &dummy) {
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

// luaT_callorderTM is luaT_callorderTM (ltm.c 5.3.4).
// Returns -1 if no metamethod, else 0/1 for the boolean result.
// LT_LE fallback lives in luaV_lessequal (lvm.c), not here.
func luaT_callorderTM(L *State, p1, p2 *Value, event TMS) int {
	if !callbinTM(L, p1, p2, L.top, event) {
		return -1 /* no metamethod */
	}
	if isFalse(&L.stack[L.top]) {
		return 0
	}
	return 1
}

