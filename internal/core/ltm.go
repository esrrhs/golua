// Translation of ltm.h / ltm.c (PUC-Lua 5.3.4): Tag methods.
//
// C surface: TMS enum, luaT_typenames_, luaT_init, luaT_gettm, luaT_gettmbyobj,
// luaT_objtypename, luaT_callTM(+hasres), luaT_callbinTM, luaT_trybinTM,
// luaT_callorderTM; macros gfasttm/fasttm/ttypename.
//
// Adaptations (intentional / FLAG — see NOTES.md):
//   - Index stack (StkId → int); checkStack instead of EXTRA_STACK assume.
//   - luaT_callTM split into luaT_callTM (3-arg, nresults=0) and
//     luaT_callTMres (2-arg + result slot, nresults=1).
//   - migrateNested before TM calls (goroutine-coroutine port).
//   - luaC_fix on tmnames omitted (Go GC).
//   - notm: nil pointer + nil TValue (Go pointer model).
//   - luaT_tryconcatTM extracted (C inlines trybinTM TM_CONCAT in lvm.c).
//   - cvt2str (from lvm.h) and luaO_nilobject hosted here for call-site convenience.
package core

// TMS: tag-method names, same order as PUC-Lua (ORDER TM / ORDER OP).
type TMS int

const (
	TM_INDEX TMS = iota
	TM_NEWINDEX
	TM_GC
	TM_MODE
	TM_LEN
	TM_EQ // last tag method with fast access
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
	TM_N // number of elements in the enum
)

// luaO_nilobject is the singleton nil TValue (lobject.c luaO_nilobject_).
// Hosted here because gettmbyobj returns it; also used by older call sites.
var luaO_nilobject = Value{tt: LUA_TNIL}

// nilObject keeps older call sites compiling.
var nilObject = luaO_nilobject

// luaT_eventname is C's static luaT_eventname[] inside luaT_init (ORDER TM).
var luaT_eventname = [TM_N]string{
	"__index", "__newindex",
	"__gc", "__mode", "__len", "__eq",
	"__add", "__sub", "__mul", "__mod", "__pow",
	"__div", "__idiv",
	"__band", "__bor", "__bxor", "__shl", "__shr",
	"__unm", "__bnot", "__lt", "__le",
	"__concat", "__call",
}

// luaT_typenames is C luaT_typenames_ (ltm.c): indices are ttnov+1;
// last slot "proto" is for tests only.
// C aliases both userdata slots to one static udatatypename[]; Go uses two
// equal "userdata" literals (content-identical).
var luaT_typenames = [LUA_TOTALTAGS]string{
	"no value",
	"nil", "boolean", "userdata", "number",
	"string", "table", "function", "userdata", "thread",
	"proto",
}

// luaT_init initialises the tag-method names (ltm.c luaT_init).
// C also luaC_fix's each name; omitted — Go GC (IGNORE).
func luaT_init(L *State) {
	for i := 0; i < int(TM_N); i++ {
		L.g.tmNames[i] = luaS_new(L, luaT_eventname[i])
	}
}

// cvt2str reports whether a value can be converted to a string (lvm.h).
// Default PUC (no LUA_NOCVTN2S): true iff ttisnumber.
// FLAG: placement — C hosts this in lvm.h; body matches default 5.3.4.
func cvt2str(o *Value) bool { return ttisnumber(o) }

// notm reports whether a metamethod is absent.
// FLAG: Go pointer model — nil pointer OR nil TValue (C only ttisnil).
func notm(tm *Value) bool { return tm == nil || ttisnil(tm) }

// fasttm is gfasttm/fasttm (ltm.h): optimized for absence of tag methods.
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
// C has lua_assert(event <= TM_EQ); omitted (debug-only).
func luaT_gettm(events *Table, event TMS, ename *TString) *Value {
	tm := luaH_getshortstr(events, ename)
	if notm(tm) { /* no tag method? */
		events.flags |= byte(1 << uint(event)) /* cache this fact */
		return nil
	}
	return tm
}

// luaT_gettmbyobj looks up a metamethod for an object (ltm.c).
// ttype ≡ C ttnov (variant-stripped).
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

// luaT_objtypename returns the type name of an object, honouring __name
// on table/fulluserdata metatables (ltm.c).
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
			/* C returns getstr(...) as const char* for '%s' (stops at NUL). */
			return luaCStr(tsvalue(name).val)
		}
	}
	/* C: return ttypename(ttnov(o)). Light/full userdata both map to
	   "userdata" via typenames; explicit cases keep that explicit. */
	switch ttype(o) {
	case LUA_TLIGHTUSERDATA, LUA_TUSERDATA:
		return "userdata"
	}
	return luaT_typenames[ttype(o)+1]
}

// luaT_callTM calls a metamethod with three arguments and no result
// (C luaT_callTM with hasres=0).
//
// FLAG: migrateNested — Go-only goroutine-coroutine port; no C analogue.
// FLAG: checkStack(4) — grow instead of C "assume EXTRA_STACK".
// FLAG: split from C's single hasres API (see also luaT_callTMres).
func luaT_callTM(L *State, f, p1, p2, p3 *Value) {
	L.migrateNested() // FLAG: goroutine port
	funcIdx := L.top
	L.checkStack(4) // FLAG: vs C EXTRA_STACK assume
	L.stack[funcIdx] = *f
	L.stack[funcIdx+1] = *p1
	L.stack[funcIdx+2] = *p2
	L.stack[funcIdx+3] = *p3
	L.top = funcIdx + 4
	/* metamethod may yield only when called from Lua code */
	if isLua(L.ci) {
		luaD_call(L, funcIdx, 0)
	} else {
		luaD_callnoyield(L, funcIdx, 0)
	}
}

// luaT_callTMres calls a metamethod with two arguments and moves the
// single result to stack slot 'res' (C luaT_callTM with hasres=1).
//
// FLAG: migrateNested — Go-only goroutine-coroutine port.
// FLAG: checkStack(3) — grow instead of EXTRA_STACK assume.
// FLAG: split half of C luaT_callTM(+hasres); res is an index (index stack).
func luaT_callTMres(L *State, f, p1, p2 *Value, res int) {
	L.migrateNested() // FLAG: goroutine port
	funcIdx := L.top
	L.checkStack(3) // FLAG: vs C EXTRA_STACK assume
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

// callbinTM is luaT_callbinTM (ltm.c): try p1 then p2; call with result.
// Unexported naming adaptation; semantics match C.
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

// luaT_trybinTM tries a binary metamethod; raises an error if there is none
// (ltm.c). C fallthrough after noreturn error helpers is dead — Go switch arms
// each error and do not fall through (equivalent).
func luaT_trybinTM(L *State, p1, p2 *Value, res int, event TMS) {
	if !callbinTM(L, p1, p2, res, event) {
		switch event {
		case TM_CONCAT:
			luaG_concaterror(L, p1, p2)
		case TM_BAND, TM_BOR, TM_BXOR, TM_SHL, TM_SHR, TM_BNOT:
			/* 5.3: tonumber (float+int+string coerce), not ttisnumber */
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

// luaT_tryconcatTM is the concat error path used by luaV_concat.
// FLAG: packaging — C inlines luaT_trybinTM(..., TM_CONCAT) in lvm.c;
// ≡ callbinTM(..., TM_CONCAT) else luaG_concaterror.
func luaT_tryconcatTM(L *State) {
	top := L.top
	if !callbinTM(L, &L.stack[top-2], &L.stack[top-1], top-2, TM_CONCAT) {
		luaG_concaterror(L, &L.stack[top-2], &L.stack[top-1])
	}
}

// luaT_callorderTM is luaT_callorderTM (ltm.c 5.3.4).
// Returns -1 if no metamethod, else 0/1 for the boolean result
// (!l_isfalse → 0/1). LT_LE fallback lives in luaV_lessequal (lvm), not here.
func luaT_callorderTM(L *State, p1, p2 *Value, event TMS) int {
	if !callbinTM(L, p1, p2, L.top, event) {
		return -1 /* no metamethod */
	}
	if isFalse(&L.stack[L.top]) {
		return 0
	}
	return 1
}
