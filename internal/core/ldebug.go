// Translation of ldebug.h / ldebug.c (PUC-Lua 5.3.4).
// Hook install/get and getstack/getinfo wrappers; luaG_traceexec is in lvm.go.
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
	if op < 0 || op >= NUM_OPCODES {
		return false
	}
	return opModes[op]&(1<<6) != 0
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
	jmptarget := 0 /* any code before this address is conditional */
	if lastpc < 0 || lastpc >= len(p.code) {
		return -1
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
			dest := pc + 1 + sBx(i.B)
			/* jump is forward and do not skip 'lastpc'? (5.3.4) */
			if pc < dest && dest <= lastpc {
				if dest > jmptarget {
					jmptarget = dest /* update 'jmptarget' */
				}
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

// kname finds a "name" for the constant at 'index' (plain K index).
func kname(p *Proto, index int, name *string) string {
	if index < 0 || index >= len(p.k) || !ttisstring(&p.k[index]) {
		*name = "?"
		return ""
	}
	*name = tsvalue(&p.k[index]).val
	return "constant"
}

// knameRK finds a name for an RK operand 'c' (ldebug.c kname).
func knameRK(p *Proto, pc, c int, name *string) {
	if ISK(c) { /* is 'c' a constant? */
		if kname(p, INDEXK(c), name) != "" {
			return
		}
	} else { /* 'c' is a register */
		what := getobjname(p, pc, c, name)
		if len(what) > 0 && what[0] == 'c' { /* found a constant name? */
			return
		}
	}
	*name = "?" /* no reasonable name found */
}

// getobjname is 'getobjname' (ldebug.c, Lua 5.3.4).
func getobjname(p *Proto, lastpc, reg int, name *string) string {
	*name = luaF_getlocalname(p, reg+1, lastpc)
	if *name != "" { /* is a local? */
		return "local"
	}
	pc := findsetreg(p, lastpc, reg)
	if pc != -1 { /* could find instruction? */
		i := p.code[pc]
		switch i.Op {
		case OP_MOVE:
			b := i.B /* move from 'b' to 'a' */
			if b < i.A {
				return getobjname(p, pc, b, name) /* get name for 'b' */
			}
		case OP_GETTABUP, OP_GETTABLE:
			k := i.C /* key index */
			t := i.B /* table index */
			var vn string
			if i.Op == OP_GETTABLE {
				vn = luaF_getlocalname(p, t+1, pc)
			} else {
				vn = upvalName(p, t)
			}
			knameRK(p, pc, k, name)
			if vn != "" && vn == LUA_ENV {
				return "global"
			}
			return "field"
		case OP_GETUPVAL:
			*name = upvalName(p, i.B)
			return "upvalue"
		case OP_LOADK:
			return kname(p, i.B, name)
		case OP_LOADKX:
			if pc+1 < len(p.code) {
				return kname(p, p.code[pc+1].A, name)
			}
		case OP_SELF:
			knameRK(p, pc, i.C, name)
			return "method"
		}
	}
	return "" /* could not find reasonable name */
}

// funcNameFromCode is 'funcnamefromcode' (ldebug.c, Lua 5.3.4).
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
	case OP_SELF, OP_GETTABUP, OP_GETTABLE:
		tm = TM_INDEX
	case OP_SETTABUP, OP_SETTABLE:
		tm = TM_NEWINDEX
	case OP_ADD, OP_SUB, OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
		OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
		/* ORDER OP / ORDER TM */
		tm = TMS(int(i.Op) - OP_ADD + int(TM_ADD))
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
	case OP_LT:
		tm = TM_LT
	case OP_LE:
		tm = TM_LE
	default:
		return "" /* cannot find a reasonable name */
	}
	/* 5.3.4: keep the "__" prefix (getstr(G(L)->tmname[tm])). */
	*name = luaT_eventname[tm]
	return "metamethod"
}

// getFuncName is getfuncname (ldebug.c, Lua 5.3.4).
//
// CIST_FIN is set on the *caller* of the finalizer (GCTM), so querying
// that frame yields namewhat "metamethod" / name "__gc". CIST_HOOKED is
// checked inside funcNameFromCode on the interrupted frame.
func getFuncName(L *State, ci *CallInfo, name *string) string {
	if ci == nil {
		return ""
	}
	if ci.callstatus&CIST_FIN != 0 { /* GCTM marked this frame */
		*name = "__gc"
		return "metamethod"
	}
	if ci.callstatus&CIST_TAIL == 0 && ci.prev != nil && isLua(ci.prev) {
		prev := ci.prev
		/* 5.3.4 funcnamefromcode: hooked frame reports as hook '?'. */
		if prev.callstatus&CIST_HOOKED != 0 {
			*name = "?"
			return "hook"
		}
		return funcNameFromCode(L, clLvalue(&L.stack[prev.funcIdx]).p,
			prev.savedpc-1, name)
	}
	return ""
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
// (ldebug.c isinstack + cast_int(o - ci->u.l.base)).
// Use Lua frame base (ci.base), not funcIdx+1: with varargs ci.base >
// funcIdx+1, so register indices for getobjname / varinfo must be relative
// to ci.base (PUC 5.3.4).
func instack(L *State, ci *CallInfo, o *Value) int {
	base := ci.base
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
	/* 5.3: tonumber (string coerce), not ttisnumber */
	var temp float64
	if !luaV_tonumber_(L, p1, &temp) { /* first operand is wrong? */
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
	/* 5.3: tointeger coerces strings; 5.4 used tointegerns */
	if !tointeger(L, p1, &temp) {
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
// hooks (ldebug.c)
// ---------------------------------------------------------------------

// SetHook installs a hook (lua_sethook). 'hook' is the Go function the VM
// calls; 'mask' is a combination of dbMaskCall, dbMaskRet, dbMaskLine and
// dbMaskCount; 'count' is the instruction count between count hooks.
func (L *State) SetHook(hook func(L *State, ar *dbAr), mask, count int) {
	L.installHook(hook, mask, count, true)
}

// SetInterruptHook installs the stand-alone interpreter's SIGINT hook
// (lua.c's lstop): the next call, return or count event raises
// "interrupted!" and clears the hook. laction in cmd/lua arms this from
// a signal; a second SIGINT uses the default disposition and kills the
// process, matching PUC when a long unbreakable C/Go call is in flight.
func (L *State) SetInterruptHook() {
	L.installHook(func(L *State, ar *dbAr) {
		L.installHook(nil, 0, 0, false)
		/* luaL_error(L, "interrupted!") */
		L.Error2("interrupted!")
	}, dbMaskCall|dbMaskRet|dbMaskCount, 1, true)
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
	/* ldebug.c lua_sethook: if (isLua(L->ci)) L->oldpc = L->ci->u.l.savedpc;
	   Go oldpc is pcRel-style, so savedpc-1. Avoids a spurious line hook
	   for the remainder of the current line after debug.sethook. */
	if ci := L.ci; ci != nil && isLua(ci) {
		if ci.savedpc > 0 {
			L.oldpc = ci.savedpc - 1
		} else {
			L.oldpc = 0
		}
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

// ---------------------------------------------------------------------
// Debug API (ldebug.c LUA_API) — implementations live as db* helpers
// used by ldblib; wrappers match lua_getstack / lua_getinfo / locals.
// ---------------------------------------------------------------------

// GetStack fills ar with the activation record at level (lua_getstack).
func (L *State) GetStack(level int, ar *dbAr) bool { return dbGetStack(L, level, ar) }

// GetInfo fills ar according to what (lua_getinfo).
func (L *State) GetInfo(what string, ar *dbAr) bool { return dbGetInfo(L, what, ar) }

// GetLocal pushes local n of ar and returns its name (lua_getlocal).
func (L *State) GetLocal(ar *dbAr, n int) string { return dbGetLocal(L, ar, n) }

// SetLocal pops into local n of ar (lua_setlocal).
func (L *State) SetLocal(ar *dbAr, n int) string { return dbSetLocal(L, ar, n) }
