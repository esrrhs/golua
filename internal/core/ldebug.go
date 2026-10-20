// Translation of ldebug.h / ldebug.c (PUC-Lua 5.4), minus the debug hooks.
package core

import (
	"fmt"
	"strings"
)

// luaG_getfuncline returns the source line of an instruction.
func luaG_getfuncline(f *Proto, pc int) int {
	if len(f.lineinfo) == 0 { /* no debug information? */
		return -1
	}
	/* 'pc < 0' means "before the first instruction": C's 'getbaseline'
	   returns the line where the function was defined and the walk
	   over the (delta) line info adds nothing. This port stores
	   absolute lines, so only the base case needs handling. */
	if pc < 0 {
		return f.linedefined
	}
	if pc >= len(f.lineinfo) {
		return -1
	}
	return int(f.lineinfo[pc])
}

// currentLine returns the line being executed by a call frame.
func currentLine(L *State, ci *CallInfo) int {
	if !isLua(ci) {
		return -1
	}
	cl := clLvalue(&L.stack[ci.funcIdx])
	return luaG_getfuncline(cl.p, ci.savedpc-1)
}

// luaG_addinfo prefixes a message with "chunk:line: " (ldebug.c).
// pushMessage pushes a string without ever going through checkStack.
// It is used for error messages: 'stack overflow' is itself raised from
// growStack, and checkStack -> growStack -> luaG_runerror would recurse
// until the Go stack blows up.
func (L *State) pushMessage(ts *TString) {
	if L.top >= len(L.stack) {
		L.reallocStack(len(L.stack) + EXTRA_STACK)
	}
	setsvalue(&L.stack[L.top], ts)
	L.top++
}

func luaG_addinfo(L *State, msg string, src *TString, line int) string {
	buff := "?"
	if src != nil {
		buff = luaO_chunkid(src.val)
	}
	s := fmt.Sprintf("%s:%d: %s", buff, line, msg)
	L.pushMessage(luaS_newlstr(L, s))
	return s
}

// luaG_errormsg calls the message handler (if any) and raises the error
// (ldebug.c).
func luaG_errormsg(L *State) {
	if L.errfunc != 0 {
		errfunc := L.errfunc
		L.checkStack(2)
		L.stack[L.top] = L.stack[L.top-1] /* move argument */
		L.stack[L.top-1] = L.stack[errfunc]
		L.top++
		luaD_callnoyield(L, L.top-2, 1) /* call it */
	}
	luaD_throw(L, LUA_ERRRUN)
}

// luaG_runerror raises a runtime error with a formatted message.
func luaG_runerror(L *State, format string, args ...any) {
	ts := pushfString(L, format, args...)
	if L.ci != nil && isLua(L.ci) {
		cl := clLvalue(&L.stack[L.ci.funcIdx])
		luaG_addinfo(L, ts.val, cl.p.source, currentLine(L, L.ci))
	} else {
		/* no Lua frame to add position info to: push the message
		   itself, so that luaD_seterrorobj finds it on the top */
		L.pushMessage(ts)
	}
	luaG_errormsg(L)
}

// ---------------------------------------------------------------------
// Symbolic execution (ldebug.c)
//
// These are the helpers behind 'varinfo', the "(global 'print')" suffix
// that Lua appends to "attempt to ... a X value" messages. They walk the
// bytecode backwards looking for the instruction that produced a given
// register.
// ---------------------------------------------------------------------

// testAMode reports whether the opcode sets register A (the 'A' column of
// luaP_opmodes, lopcodes.h: testAMode).
func testAMode(op int) bool {
	switch op {
	case OP_MOVE, OP_LOADI, OP_LOADF, OP_LOADK, OP_LOADKX, OP_LOADFALSE,
		OP_LFALSESKIP, OP_LOADTRUE, OP_LOADNIL, OP_GETUPVAL, OP_GETTABUP,
		OP_GETTABLE, OP_GETI, OP_GETFIELD, OP_NEWTABLE, OP_SELF, OP_ADDI,
		OP_ADDK, OP_SUBK, OP_MULK, OP_MODK, OP_POWK, OP_DIVK, OP_IDIVK,
		OP_BANDK, OP_BORK, OP_BXORK, OP_SHRI, OP_SHLI, OP_ADD, OP_SUB,
		OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV, OP_BAND, OP_BOR, OP_BXOR,
		OP_SHL, OP_SHR, OP_UNM, OP_BNOT, OP_NOT, OP_LEN, OP_CONCAT,
		OP_TESTSET, OP_CALL, OP_TAILCALL, OP_FORLOOP, OP_FORPREP,
		OP_TFORLOOP, OP_CLOSURE, OP_VARARG, OP_VARARGPREP:
		return true
	}
	return false
}

// testMMMode reports whether the opcode is one of the metamethod helpers
// (lopcodes.h: testMMMode).
func testMMMode(op int) bool {
	switch op {
	case OP_MMBIN, OP_MMBINI, OP_MMBINK:
		return true
	}
	return false
}

// filterpc is 'filterpc' (ldebug.c).
func filterpc(pc, jmptarget int) int {
	if pc < jmptarget { /* is code conditional (inside a jump)? */
		return -1 /* cannot know who sets that register */
	}
	return pc /* current position sets that register */
}

// findsetreg tries to find the last instruction before 'lastpc' that
// modified register 'reg' (ldebug.c).
func findsetreg(p *Proto, lastpc, reg int) int {
	setreg := -1 /* keep last instruction that changed 'reg' */
	jmptarget := 0
	if lastpc < 0 || lastpc >= len(p.code) {
		return -1
	}
	if testMMMode(p.code[lastpc].Op) {
		lastpc-- /* previous instruction was not actually executed */
	}
	for pc := 0; pc < lastpc; pc++ {
		i := p.code[pc]
		a := i.A
		change := false /* true if current instruction changed 'reg' */
		switch i.Op {
		case OP_LOADNIL: /* set registers from 'a' to 'a+b' */
			change = a <= reg && reg <= a+i.B
		case OP_TFORCALL: /* affect all regs above its base */
			change = reg >= a+2
		case OP_CALL, OP_TAILCALL: /* affect all registers above base */
			change = reg >= a
		case OP_JMP: /* doesn't change registers, but changes 'jmptarget' */
			dest := pc + 1 + (i.B - OFFSET_sJ)
			if dest <= lastpc && dest > jmptarget {
				jmptarget = dest /* update 'jmptarget' */
			}
			change = false
		default: /* any instruction that sets A */
			change = testAMode(i.Op) && reg == a
		}
		if change {
			setreg = filterpc(pc, jmptarget)
		}
	}
	return setreg
}

// upvalName is 'upvalname' (ldebug.c).
func upvalName(p *Proto, uv int) string {
	if uv < 0 || uv >= len(p.upvalues) || p.upvalues[uv].name == nil {
		return "?"
	}
	return p.upvalues[uv].name.val
}

// kname finds a "name" for the constant 'index' (ldebug.c). It returns ""
// (C's NULL) when the constant is not a string.
func kname(p *Proto, index int, name *string) string {
	if index < 0 || index >= len(p.k) || !ttisstring(&p.k[index]) {
		*name = "?"
		return ""
	}
	*name = tsvalue(&p.k[index]).val
	return "constant"
}

// basicgetobjname is 'basicgetobjname' (ldebug.c). 'ppc' is both the pc to
// start from and, on return, the pc of the instruction that set 'reg'.
func basicgetobjname(p *Proto, ppc *int, reg int, name *string) string {
	pc := *ppc
	*name = luaF_getlocalname(p, reg+1, pc)
	if *name != "" { /* is a local? */
		return "local"
	}
	/* else try symbolic execution */
	pc = findsetreg(p, pc, reg)
	*ppc = pc
	if pc != -1 { /* could find instruction? */
		i := p.code[pc]
		switch i.Op {
		case OP_MOVE:
			b := i.B /* move from 'b' to 'a' */
			if b < i.A {
				return basicgetobjname(p, ppc, b, name) /* get name for 'b' */
			}
		case OP_GETUPVAL:
			*name = upvalName(p, i.B)
			return "upvalue"
		case OP_LOADK:
			return kname(p, i.B, name)
		case OP_LOADKX:
			if pc+1 < len(p.code) {
				return kname(p, p.code[pc+1].A, name)
			}
		}
	}
	return "" /* could not find reasonable name */
}

// rname finds a "name" for the register 'c' (ldebug.c).
func rname(p *Proto, pc, c int, name *string) {
	what := basicgetobjname(p, &pc, c, name) /* search for 'c' */
	if len(what) == 0 || what[0] != 'c' {    /* did not find a constant name? */
		*name = "?"
	}
}

// rkname finds a "name" for a 'C' value in an RK instruction (ldebug.c).
func rkname(p *Proto, pc int, i Instruction, name *string) {
	c := i.C /* key index */
	if i.K != 0 {
		kname(p, c, name) /* 'c' is a constant */
	} else {
		rname(p, pc, c, name) /* 'c' is a register */
	}
}

// isEnv checks whether the table indexed by instruction 'i' is the
// environment '_ENV' (ldebug.c).
func isEnv(p *Proto, pc int, i Instruction, isup bool) string {
	t := i.B /* table index */
	var name string
	if isup { /* is 't' an upvalue? */
		name = upvalName(p, t)
	} else { /* 't' is a register */
		what := basicgetobjname(p, &pc, t, &name)
		if what != "local" && what != "upvalue" {
			name = "" /* cannot be the variable _ENV */
		}
	}
	if name != "" && name == LUA_ENV {
		return "global"
	}
	return "field"
}

// getobjname is 'getobjname' (ldebug.c): extends basicgetobjname to handle
// table accesses.
func getobjname(p *Proto, lastpc, reg int, name *string) string {
	kind := basicgetobjname(p, &lastpc, reg, name)
	if kind != "" {
		return kind
	} else if lastpc != -1 { /* could find instruction? */
		i := p.code[lastpc]
		switch i.Op {
		case OP_GETTABUP:
			kname(p, i.C, name) /* key index */
			return isEnv(p, lastpc, i, true)
		case OP_GETTABLE:
			rname(p, lastpc, i.C, name) /* key index */
			return isEnv(p, lastpc, i, false)
		case OP_GETI:
			*name = "integer index"
			return "field"
		case OP_GETFIELD:
			kname(p, i.C, name) /* key index */
			return isEnv(p, lastpc, i, false)
		case OP_SELF:
			rkname(p, lastpc, i, name)
			return "method"
		}
	}
	return "" /* could not find reasonable name */
}

// funcNameFromCode is 'funcnamefromcode' (ldebug.c): it inspects the
// calling instruction to find a name for the function being called.
func funcNameFromCode(L *State, p *Proto, pc int, name *string) string {
	if pc < 0 || pc >= len(p.code) {
		return ""
	}
	i := p.code[pc] /* calling instruction */
	var tm TMS
	switch i.Op {
	case OP_CALL, OP_TAILCALL:
		return getobjname(p, pc, i.A, name) /* get function name */
	case OP_TFORCALL: /* for iterator */
		*name = "for iterator"
		return "for iterator"
	/* other instructions can do calls through metamethods */
	case OP_SELF, OP_GETTABUP, OP_GETTABLE, OP_GETI, OP_GETFIELD:
		tm = TM_INDEX
	case OP_SETTABUP, OP_SETTABLE, OP_SETI, OP_SETFIELD:
		tm = TM_NEWINDEX
	case OP_MMBIN, OP_MMBINI, OP_MMBINK:
		tm = TMS(i.C)
	case OP_UNM:
		tm = TM_UNM
	case OP_BNOT:
		tm = TM_BNOT
	case OP_LEN:
		tm = TM_LEN
	case OP_CONCAT:
		tm = TM_CONCAT
	case OP_EQ:
		tm = TM_EQ
	/* no cases for OP_EQI and OP_EQK, as they don't call metamethods */
	case OP_LT, OP_LTI, OP_GTI:
		tm = TM_LT
	case OP_LE, OP_LEI, OP_GEI:
		tm = TM_LE
	case OP_CLOSE, OP_RETURN:
		tm = TM_CLOSE
	default:
		return "" /* cannot find a reasonable name */
	}
	/* C stores the event names with the '__' prefix and skips it here */
	*name = strings.TrimPrefix(luaT_eventname[tm], "__")
	return "metamethod"
}

// funcNameFromCall is 'funcnamefromcall' (ldebug.c).
func funcNameFromCall(L *State, ci *CallInfo, name *string) string {
	switch {
	case ci == nil:
		return ""
	case ci.callstatus&CIST_HOOKED != 0: /* was it called inside a hook? */
		*name = "?"
		return "hook"
	case ci.callstatus&CIST_FIN != 0: /* was it called as a finalizer? */
		*name = "__gc"
		return "metamethod" /* report it as such */
	case isLua(ci):
		return funcNameFromCode(L, clLvalue(&L.stack[ci.funcIdx]).p,
			ci.savedpc-1, name) /* currentpc */
	}
	return ""
}

// getFuncName is 'getfuncname' (ldebug.c): the name of the function of
// the frame 'ci', as it appears in the calling code.
func getFuncName(L *State, ci *CallInfo, name *string) string {
	/* calling function is a known function? */
	if ci != nil && ci.callstatus&CIST_TAIL == 0 {
		return funcNameFromCall(L, ci.prev, name)
	}
	return "" /* no way to find a name */
}

// getUpvalName is 'getupvalname' (ldebug.c): checks whether 'o' is an
// upvalue of the running closure.
func getUpvalName(L *State, ci *CallInfo, o *Value, name *string) string {
	c := clLvalue(&L.stack[ci.funcIdx])
	for i := 0; i < c.nupvalues && i < len(c.upvals); i++ {
		if upvalValue(c.upvals[i]) == o {
			*name = upvalName(c.p, i)
			return "upvalue"
		}
	}
	return ""
}

// instack returns the register index of 'o' in the frame 'ci', or -1
// (ldebug.c).
func instack(L *State, ci *CallInfo, o *Value) int {
	base := ci.funcIdx + 1
	for pos := 0; base+pos < ci.top && base+pos < len(L.stack); pos++ {
		if &L.stack[base+pos] == o {
			return pos
		}
	}
	return -1 /* not found */
}

// formatvarinfo is 'formatvarinfo' (ldebug.c).
func formatvarinfo(kind, name string) string {
	if kind == "" {
		return "" /* no information */
	}
	/* C writes this with "%s", and a Lua string may hold a NUL -- the
	   constant in '"a\0b" & 1' is one -- which C's printf stops at.
	   PUC-Lua therefore reports that constant as 'a', and so does this:
	   without the cut the message would name bytes it never shows. */
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return fmt.Sprintf(" (%s '%s')", kind, name)
}

// varinfo returns " (global 'print')" style extra information for a value
// (ldebug.c).
func varinfo(L *State, o *Value) string {
	ci := L.ci
	if ci == nil || !isLua(ci) || o == nil {
		return ""
	}
	var name string
	kind := getUpvalName(L, ci, o, &name) /* check whether 'o' is an upvalue */
	if kind == "" {                       /* not an upvalue? */
		if reg := instack(L, ci, o); reg >= 0 { /* is 'o' a register? */
			p := clLvalue(&L.stack[ci.funcIdx]).p
			kind = getobjname(p, ci.savedpc-1, reg, &name) /* currentpc */
		}
	}
	return formatvarinfo(kind, name)
}

// luaG_typeerror raises "attempt to <op> a <type> value" (ldebug.c).
func luaG_typeerror(L *State, o *Value, op string) {
	t := luaT_objtypename(L, o)
	luaG_runerror(L, "attempt to %s a %s value%s", op, t, varinfo(L, o))
}

func luaG_concaterror(L *State, p1, p2 *Value) {
	if ttisstring(p1) || cvt2str(p1) {
		p1 = p2
	}
	luaG_typeerror(L, p1, "concatenate")
}

func luaG_opinterror(L *State, p1, p2 *Value, msg string) {
	if !ttisnumber(p1) { /* first operand is wrong? */
		p2 = p1 /* now second is wrong */
	}
	luaG_typeerror(L, p2, msg)
}

// luaG_tointerror reports that both operands are convertible to numbers
// but not to integers (ldebug.c).
//
// The message is "number%s has no integer representation" with 'varinfo'
// interpolated -- *not* the offending number itself. For a constant there
// is no variable information at all, so a folded constant yields plain
// "number has no integer representation", while 'math.huge' yields
// "number (field 'huge') has no integer representation".
func luaG_tointerror(L *State, p1, p2 *Value) {
	var temp int64
	if !tointegerns(p1, &temp) {
		p2 = p1 /* first operand is wrong */
	}
	luaG_runerror(L, "number%s has no integer representation", varinfo(L, p2))
}

func luaG_ordererror(L *State, p1, p2 *Value) {
	t1 := luaT_objtypename(L, p1)
	t2 := luaT_objtypename(L, p2)
	if t1 == t2 {
		luaG_runerror(L, "attempt to compare two %s values", t1)
	}
	luaG_runerror(L, "attempt to compare %s with %s", t1, t2)
}

// luaG_callerror raises "attempt to call a <type> value" (ldebug.c).
//
// Unlike every other type error, it first asks how the *call* came about:
// a value called while Lua was running a '__close' is reported as
// "attempt to call a nil value (metamethod 'close')", which is what makes
// a variable whose '__close' disappeared produce a useful message. Only
// when the call has no such explanation does it fall back to 'varinfo'.
func luaG_callerror(L *State, o *Value) {
	var name string
	extra := ""
	if kind := funcNameFromCall(L, L.ci, &name); kind != "" {
		extra = formatvarinfo(kind, name)
	} else {
		extra = varinfo(L, o)
	}
	t := luaT_objtypename(L, o)
	luaG_runerror(L, "attempt to call a %s value%s", t, extra)
}

func luaG_forerror(L *State, o *Value, what string) {
	luaG_runerror(L, "bad 'for' %s (number expected, got %s)",
		what, luaT_objtypename(L, o))
}

// luaG_findlocal returns the name of the local variable at a given
// stack level (ldebug.c). 'n' is 1-based, as in C: n == 1 is the first
// local, whose slot is 'ci.funcIdx + 1'. 'ci.savedpc - 1' is the
// instruction being executed (C's 'currentpc').
func luaG_findlocal(L *State, ci *CallInfo, n int) string {
	if !isLua(ci) {
		return ""
	}
	cl := clLvalue(&L.stack[ci.funcIdx])
	name := luaF_getlocalname(cl.p, n, ci.savedpc-1)
	if name == "" {
		return ""
	}
	return name
}

// ---------------------------------------------------------------------
// number conversions (lvm.c)
// ---------------------------------------------------------------------

// luaV_tonumber_ converts a value to a float, with string coercion.
func luaV_tonumber_(L *State, o *Value, n *float64) bool {
	switch {
	case ttisinteger(o):
		*n = float64(ivalue(o))
		return true
	case ttisfloat(o):
		*n = fltvalue(o)
		return true
	case ttisstring(o): // string coercion
		var v Value
		return luaO_str2num(tsvalue(o).val, &v) && numberToFloat(&v, n)
	}
	return false
}

func numberToFloat(v *Value, n *float64) bool {
	if ttisinteger(v) {
		*n = float64(ivalue(v))
		return true
	}
	if ttisfloat(v) {
		*n = fltvalue(v)
		return true
	}
	return false
}

// luaV_flttointeger converts a float to an integer with a rounding mode.
func luaV_flttointeger(n float64, p *int64, mode F2Imod) bool {
	var f float64
	switch mode {
	case F2Ifloor:
		f = mathFloor(n)
	case F2Iceil:
		f = mathCeil(n)
	default:
		f = n
	}
	return floatToInt(f, p)
}

func mathFloor(n float64) float64 {
	if n == math2Trunc(n) {
		return n
	}
	return math2Floor(n)
}

func mathCeil(n float64) float64 {
	if n == math2Trunc(n) {
		return n
	}
	return math2Ceil(n)
}

func floatToInt(n float64, p *int64) bool {
	if n == math2Trunc(n) && !math2IsNaN(n) && n >= -9.223372036854775808e18 && n < 9.223372036854775808e18 {
		*p = int64(n)
		return true
	}
	return false
}

// luaV_tointeger converts a value to an integer (with string coercion).
//
// C converts the string to a number first and *then* applies the rounding
// mode, so '"10.8"' becomes 10 under F2Ifloor and 11 under F2Iceil.
// Dropping the mode (as a plain conversion would) makes 'for i = 1,"10.8"'
// fail to convert and fall into the wrong, unbounded loop.
func luaV_tointeger(L *State, o *Value, p *int64, mode F2Imod) bool {
	if ttisstring(o) {
		var v Value
		if luaO_str2num(tsvalue(o).val, &v) {
			o = &v /* change it to point to its corresponding number */
		}
	}
	return luaV_tointegerns(o, p, mode)
}

// luaV_tointegerns converts a number to an integer without string coercion.
func luaV_tointegerns(o *Value, p *int64, mode F2Imod) bool {
	switch {
	case ttisinteger(o):
		*p = ivalue(o)
		return true
	case ttisfloat(o):
		return luaV_flttointeger(fltvalue(o), p, mode)
	}
	return false
}

// F2Imod is the rounding mode for float -> integer conversion (lvm.h).
type F2Imod int

const (
	F2Ieq F2Imod = iota
	F2Ifloor
	F2Iceil
)

// LUA_FLOORN2I is the rounding mode used to convert floats to integers
// (lvm.h). It is F2Ieq unless overridden.
const LUA_FLOORN2I = F2Ieq

// tonumberns converts to float without string coercion (lvm.h).
func tonumberns(o *Value, n *float64) bool {
	switch {
	case ttisfloat(o):
		*n = fltvalue(o)
		return true
	case ttisinteger(o):
		*n = float64(ivalue(o))
		return true
	}
	return false
}

// tointegerns converts to integer without string coercion (lvm.h).
func tointegerns(o *Value, i *int64) bool {
	if ttisinteger(o) {
		*i = ivalue(o)
		return true
	}
	return luaV_tointegerns(o, i, F2Ieq)
}

// tointeger converts to integer with string coercion (lvm.h).
func tointeger(L *State, o *Value, i *int64) bool {
	if ttisinteger(o) {
		*i = ivalue(o)
		return true
	}
	return luaV_tointeger(L, o, i, F2Ieq)
}

// luaV_equalobj / luaV_lessthan / luaV_lessequal implement comparisons
// with metamethods (lvm.c).
func luaV_equalobj(L *State, t1, t2 *Value) bool {
	var tm *Value
	if ttypetag(t1) != ttypetag(t2) { /* not the same variant? */
		if ttype(t1) != ttype(t2) || ttype(t1) != LUA_TNUMBER {
			return false /* only numbers can be equal with different variants */
		}
		/* two numbers with different variants: one of them is an
		   integer; if the other has no exact integer value they
		   cannot be equal, otherwise compare the integer values */
		var i1, i2 int64
		return tointegerns(t1, &i1) && tointegerns(t2, &i2) && i1 == i2
	}
	/* values have the same type and the same variant */
	switch ttypetag(t1) {
	case LUA_VNIL, LUA_VFALSE, LUA_VTRUE:
		return true
	case LUA_VNUMINT:
		return ivalue(t1) == ivalue(t2)
	case LUA_VNUMFLT:
		return fltvalue(t1) == fltvalue(t2)
	case LUA_VLIGHTUSERDATA:
		return pvalue(t1) == pvalue(t2)
	case LUA_VLCF: /* light C functions: identity */
		return gcvalue(t1) == gcvalue(t2)
	case LUA_VSHRSTR:
		return luaS_eqshrstr(tsvalue(t1), tsvalue(t2))
	case LUA_VLNGSTR:
		return luaS_eqlngstr(tsvalue(t1), tsvalue(t2))
	case LUA_VUSERDATA:
		if uvalue(t1) == uvalue(t2) {
			return true
		} else if L == nil {
			return false
		}
		tm = fasttm(L, uvalue(t1).metatable, TM_EQ)
		if tm == nil {
			tm = fasttm(L, uvalue(t2).metatable, TM_EQ)
		}
	case LUA_VTABLE:
		if hvalue(t1) == hvalue(t2) {
			return true
		} else if L == nil {
			return false
		}
		tm = fasttm(L, hvalue(t1).metatable, TM_EQ)
		if tm == nil {
			tm = fasttm(L, hvalue(t2).metatable, TM_EQ)
		}
	default:
		return gcvalue(t1) == gcvalue(t2)
	}
	if tm == nil { /* no TM? */
		return false /* objects are different */
	}
	luaT_callTMres(L, tm, t1, t2, L.top) /* call TM */
	return !isFalse(&L.stack[L.top])
}

// maxIntFitsF is MAXINTFITSF: 1 << NBM, with NBM the number of bits in
// the mantissa of a float (53 for an IEEE-754 double).
const maxIntFitsF = uint64(1) << 53

// lIntfitsf reports whether 'i' has an exact representation as a float
// (l_intfitsf in lvm.c). The addition is unsigned and wraps, exactly as
// in C, which is what turns the test into "is i in [-2^53, 2^53]".
func lIntfitsf(i int64) bool {
	return maxIntFitsF+uint64(i) <= 2*maxIntFitsF
}

// ltIntFloat returns 'i < f' (LTintfloat). When 'i' has no exact float
// representation, the two cannot be compared as floats: use the
// equivalence 'i < f <=> i < ceil(f)'.
func ltIntFloat(i int64, f float64) bool {
	if lIntfitsf(i) {
		return float64(i) < f
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
		return i < fi
	}
	return f > 0 /* 'f' is either greater or less than all integers */
}

func leIntFloat(i int64, f float64) bool {
	if lIntfitsf(i) {
		return float64(i) <= f
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
		return i <= fi
	}
	return f > 0
}

func ltFloatInt(f float64, i int64) bool {
	if lIntfitsf(i) {
		return f < float64(i)
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
		return fi < i
	}
	return f < 0
}

func leFloatInt(f float64, i int64) bool {
	if lIntfitsf(i) {
		return f <= float64(i)
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
		return fi <= i
	}
	return f < 0
}

// ltNum returns 'l < r' for two numbers (LTnum in lvm.c).
func ltNum(l, r *Value) bool {
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li < ivalue(r)
		}
		return ltIntFloat(li, fltvalue(r))
	}
	lf := fltvalue(l)
	if ttisfloat(r) {
		return lf < fltvalue(r)
	}
	return ltFloatInt(lf, ivalue(r))
}

// leNum returns 'l <= r' for two numbers (LEnum in lvm.c).
func leNum(l, r *Value) bool {
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li <= ivalue(r)
		}
		return leIntFloat(li, fltvalue(r))
	}
	lf := fltvalue(l)
	if ttisfloat(r) {
		return lf <= fltvalue(r)
	}
	return leFloatInt(lf, ivalue(r))
}

// luaV_lessthan is the main operation 'l < r' (lvm.c).
func luaV_lessthan(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) {
		return ltNum(l, r)
	}
	if ttisstring(l) && ttisstring(r) {
		return tsvalue(l).val < tsvalue(r).val
	}
	return luaT_callorderTM(L, l, r, TM_LT)
}

// luaV_lessequal is the main operation 'l <= r' (lvm.c).
func luaV_lessequal(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) {
		return leNum(l, r)
	}
	if ttisstring(l) && ttisstring(r) {
		return tsvalue(l).val <= tsvalue(r).val
	}
	return luaT_callorderTM(L, l, r, TM_LE)
}

// luaV_objlen is the main operation 'ra = #rb' (lvm.c).
func luaV_objlen(L *State, ra int, rb *Value) {
	var tm *Value
	switch ttypetag(rb) {
	case LUA_VTABLE:
		h := hvalue(rb)
		tm = fasttm(L, h.metatable, TM_LEN)
		if tm == nil {
			setivalue(&L.stack[ra], int64(luaH_getn(h)))
			return
		}
	case LUA_VSHRSTR, LUA_VLNGSTR:
		setivalue(&L.stack[ra], int64(len(tsvalue(rb).val)))
		return
	default:
		tm = luaT_gettmbyobj(L, rb, TM_LEN)
		if notm(tm) {
			luaG_typeerror(L, rb, "get length of")
			return
		}
	}
	luaT_callTMres(L, tm, rb, rb, ra)
}

// ---------------------------------------------------------------------
// hooks (ldebug.c)
// ---------------------------------------------------------------------

// SetHook installs a hook (lua_sethook). 'hook' is the Go function the VM
// calls; 'mask' is a combination of dbMaskCall, dbMaskRet, dbMaskLine and
// dbMaskCount; 'count' is the instruction count between count hooks.
func (L *State) SetHook(hook func(L *State, ar *dbAr), mask, count int) {
	L.installHook(hook, mask, count, true)
}

// setHook is the unexported counterpart used by the debug library: the
// hook it installs is the debug library's own, so 'debug.gethook' names
// the function in the hook table for it instead of saying "external
// hook".
func (L *State) setHook(hook func(L *State, ar *dbAr), mask, count int) {
	L.installHook(hook, mask, count, false)
}

func (L *State) installHook(hook func(L *State, ar *dbAr), mask, count int, external bool) {
	if hook == nil || mask == 0 { /* turn off hooks? */
		mask = 0
		hook = nil
	}
	L.hook = hook
	L.basehookcount = count
	L.hookcount = count /* resethookcount(L) */
	L.hookmask = byte(mask)
	L.hookExternal = external && hook != nil
	if mask != 0 {
		setTraps(L) /* to trace inside 'luaV_execute' */
	}
}

// HookExternal reports whether the current hook was installed through
// the C API (lua_sethook) rather than by 'debug.sethook' -- C's
// "hook != hookf".
func (L *State) HookExternal() bool { return L.hookExternal }

// GetHook returns the current hook function (lua_gethook).
func (L *State) GetHook() func(L *State, ar *dbAr) { return L.hook }

// GetHookMask returns the current hook mask (lua_gethookmask).
func (L *State) GetHookMask() int { return int(L.hookmask) }

// GetHookCount returns the current hook count (lua_gethookcount).
func (L *State) GetHookCount() int { return L.basehookcount }
