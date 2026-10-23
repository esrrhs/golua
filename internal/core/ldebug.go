// Translation of ldebug.h / ldebug.c (PUC-Lua 5.4.9).
// Hook install/get and getstack/getinfo wrappers; luaG_traceexec /
// luaG_tracecall live in lvm.go. LUA_API bodies for getstack/getinfo/
// getlocal/setlocal live in ldblib.go (db*).
package core

import (
	"fmt"
	"strings"
)

// ABSLINEINFO / MAXIWTHABS (ldebug.h): unused at runtime here — Proto
// stores absolute lines in lineinfo (lobject.go). Constants live in
// lundump.go for dump/undump round-trip of C delta+abslineinfo form.

// Kind strings returned by symbolic-exec helpers (C's strlocal / strupval).
const (
	strLocal = "local"
	strUpval = "upvalue"
)

// luaG_getfuncline is luaG_getfuncline (ldebug.c): source line of
// instruction pc. Absolute lineinfo; C walks deltas from getbaseline.
// Out-of-range pc returns -1 (Go safety).
func luaG_getfuncline(f *Proto, pc int) int {
	if len(f.lineinfo) == 0 { /* no debug information? */
		return -1
	}
	if pc < 0 || pc >= len(f.lineinfo) {
		return -1
	}
	return int(f.lineinfo[pc])
}

// currentLine is getcurrentline / currentpc (ldebug.c): line of the
// instruction being executed (currentpc = savedpc-1). Extra !isLua → -1
// is Go safety.
func currentLine(L *State, ci *CallInfo) int {
	if !isLua(ci) {
		return -1
	}
	cl := clLvalue(&L.stack[ci.funcIdx])
	return luaG_getfuncline(cl.p, ci.savedpc-1)
}

// pushMessage pushes a string without ever going through checkStack.
// Used for error messages: 'stack overflow' is raised from growStack, and
// checkStack -> growStack -> luaG_runerror would recurse until the Go stack
// blows up. (Intentional Go specialization; not in C ldebug.c.)
func (L *State) pushMessage(ts *TString) {
	if L.top >= len(L.stack) {
		L.reallocStack(len(L.stack) + EXTRA_STACK)
	}
	setsvalue(&L.stack[L.top], ts)
	L.top++
}

// luaG_addinfo prefixes a message with "chunk:line: " (ldebug.c).
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
	if L.errfunc != 0 { /* is there an error handling function? */
		errfunc := L.errfunc
		L.checkStack(2)
		L.stack[L.top] = L.stack[L.top-1] /* move argument */
		L.stack[L.top-1] = L.stack[errfunc]
		L.top++                         /* assume EXTRA_STACK */
		luaD_callnoyield(L, L.top-2, 1) /* call it */
	}
	luaD_throw(L, LUA_ERRRUN)
}

// luaG_runerror raises a runtime error with a formatted message (ldebug.c).
// pushfString (lobject) builds the TString without stacking it; this path
// pushes via addinfo / pushMessage so luaD_seterrorobj sees the message.
// C: luaC_checkGC + luaO_pushvfstring then optionally addinfo and shrink.
func luaG_runerror(L *State, format string, args ...any) {
	ts := pushfString(L, format, args...)
	if L.ci != nil && isLua(L.ci) { /* if Lua function, add source:line */
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
// Hooks (ldebug.c lua_sethook / get*)
// ---------------------------------------------------------------------

// SetHook installs a hook (lua_sethook). hook is the Go function the VM
// calls; mask is a combination of dbMaskCall, dbMaskRet, dbMaskLine and
// dbMaskCount; count is the instruction count between count hooks.
func (L *State) SetHook(hook func(L *State, ar *dbAr), mask, count int) {
	L.installHook(hook, mask, count, true)
}

// setHook is the unexported counterpart used by the debug library: the
// hook it installs is the debug library's own, so debug.gethook names
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
		setTraps(L) /* to trace inside luaV_execute (C: settraps) */
	}
}

// HookExternal reports whether the current hook was installed through
// the C API (lua_sethook) rather than by debug.sethook — C's
// "hook != hookf".
func (L *State) HookExternal() bool { return L.hookExternal }

// GetHook returns the current hook function (lua_gethook).
func (L *State) GetHook() func(L *State, ar *dbAr) { return L.hook }

// GetHookMask returns the current hook mask (lua_gethookmask).
func (L *State) GetHookMask() int { return int(L.hookmask) }

// GetHookCount returns the current hook count (lua_gethookcount).
func (L *State) GetHookCount() int { return L.basehookcount }

// ---------------------------------------------------------------------
// Debug API wrappers (ldebug.c LUA_API) — bodies in ldblib.go (db*).
// No CallInfo.extra / swapextra (index-stack intentional).
// ---------------------------------------------------------------------

// GetStack fills ar with the activation record at level (lua_getstack).
func (L *State) GetStack(level int, ar *dbAr) bool { return dbGetStack(L, level, ar) }

// GetInfo fills ar according to what (lua_getinfo).
func (L *State) GetInfo(what string, ar *dbAr) bool { return dbGetInfo(L, what, ar) }

// GetLocal pushes local n of ar and returns its name (lua_getlocal).
func (L *State) GetLocal(ar *dbAr, n int) string { return dbGetLocal(L, ar, n) }

// SetLocal pops into local n of ar (lua_setlocal).
func (L *State) SetLocal(ar *dbAr, n int) string { return dbSetLocal(L, ar, n) }

// ---------------------------------------------------------------------
// Locals (ldebug.c upvalname / findvararg / luaG_findlocal)
// ---------------------------------------------------------------------

// upvalName is upvalname (ldebug.c).
func upvalName(p *Proto, uv int) string {
	if uv < 0 || uv >= len(p.upvalues) || p.upvalues[uv].name == nil {
		return "?"
	}
	return p.upvalues[uv].name.val
}

// luaG_findlocal returns the name of local/vararg/temporary at 1-based
// index n (ldebug.c). Slot selection for getlocal/setlocal is in
// ldblib.dbFindLocal (same logic + pos). Used by lfunc.checkclosemth.
func luaG_findlocal(L *State, ci *CallInfo, n int) string {
	base := ci.funcIdx + 1
	if isLua(ci) {
		if n < 0 { /* access to vararg values? */
			cl := clLvalue(&L.stack[ci.funcIdx])
			if cl.p.isVararg != 0 && n >= -ci.nExtraArgs {
				return "(vararg)"
			}
			return ""
		}
		name := luaF_getlocalname(clLvalue(&L.stack[ci.funcIdx]).p, n, ci.savedpc-1)
		if name != "" {
			return name
		}
	}
	/* no 'standard' name? */
	limit := L.top
	if ci != L.ci && ci.next != nil {
		limit = ci.next.funcIdx
	}
	if limit-base >= n && n > 0 { /* is 'n' inside 'ci' stack? */
		if isLua(ci) {
			return "(temporary)"
		}
		return "(C temporary)"
	}
	return ""
}

// ---------------------------------------------------------------------
// Symbolic Execution (ldebug.c)
// ---------------------------------------------------------------------

// testAMode reports whether the opcode sets register A (lopcodes.h).

// testMMMode reports whether the opcode is a metamethod helper
// (lopcodes.h: testMMMode).

// filterpc is filterpc (ldebug.c).
func filterpc(pc, jmptarget int) int {
	if pc < jmptarget { /* is code conditional (inside a jump)? */
		return -1 /* cannot know who sets that register */
	}
	return pc /* current position sets that register */
}

// findsetreg tries to find the last instruction before lastpc that
// modified register reg (ldebug.c 5.4.9).
func findsetreg(p *Proto, lastpc, reg int) int {
	setreg := -1   /* keep last instruction that changed 'reg' */
	jmptarget := 0 /* any code before this address is conditional */
	if lastpc < 0 || lastpc >= len(p.code) {
		return -1 /* Go bounds; C assumes valid lastpc */
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
			dest := pc + 1 + (i.B - OFFSET_sJ) /* GETARG_sJ */
			/* jump does not skip 'lastpc' and is larger than current one? */
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

// kname finds a "name" for the constant at index (ldebug.c).
// Returns "" (C's NULL) when the constant is not a string.
func kname(p *Proto, index int, name *string) string {
	if index < 0 || index >= len(p.k) || !ttisstring(&p.k[index]) {
		*name = "?"
		return ""
	}
	*name = tsvalue(&p.k[index]).val
	return "constant"
}

// basicgetobjname is basicgetobjname (ldebug.c). ppc is both the pc to
// start from and, on return, the pc of the instruction that set reg.
func basicgetobjname(p *Proto, ppc *int, reg int, name *string) string {
	pc := *ppc
	*name = luaF_getlocalname(p, reg+1, pc)
	if *name != "" { /* is a local? */
		return strLocal
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
			return strUpval
		case OP_LOADK:
			return kname(p, i.B, name) /* Bx in i.B */
		case OP_LOADKX:
			if pc+1 < len(p.code) {
				return kname(p, p.code[pc+1].A, name) /* Ax of EXTRAARG */
			}
		}
	}
	return "" /* could not find reasonable name */
}

// rname finds a "name" for the register c (ldebug.c).
func rname(p *Proto, pc, c int, name *string) {
	what := basicgetobjname(p, &pc, c, name) /* search for 'c' */
	if len(what) == 0 || what[0] != 'c' {    /* did not find a constant name? */
		*name = "?"
	}
}

// rkname finds a "name" for a C value in an RK instruction (ldebug.c).
func rkname(p *Proto, pc int, i Instruction, name *string) {
	c := i.C      /* key index */
	if i.K != 0 { /* is 'c' a constant? */
		kname(p, c, name)
	} else { /* 'c' is a register */
		rname(p, pc, c, name)
	}
}

// isEnv checks whether the table indexed by instruction i is the
// environment '_ENV' (ldebug.c).
func isEnv(p *Proto, pc int, i Instruction, isup bool) string {
	t := i.B /* table index */
	var name string
	if isup { /* is 't' an upvalue? */
		name = upvalName(p, t)
	} else { /* 't' is a register */
		what := basicgetobjname(p, &pc, t, &name)
		if what != strLocal && what != strUpval {
			name = "" /* cannot be the variable _ENV */
		}
	}
	if name != "" && name == LUA_ENV {
		return "global"
	}
	return "field"
}

// getobjname is getobjname (ldebug.c): extends basicgetobjname to handle
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

// funcNameFromCode is funcnamefromcode (ldebug.c): inspects the calling
// instruction to find a name for the function being called.
// Signature takes Proto+pc (caller computed currentpc); C takes CallInfo.
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
	/* C: getshrstr(G(L)->tmname[tm]) + 2  (skip "__") */
	_ = L
	*name = strings.TrimPrefix(luaT_eventname[tm], "__")
	return "metamethod"
}

// funcNameFromCall is funcnamefromcall (ldebug.c).
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

// getFuncName is getfuncname (ldebug.c): name of the function of frame
// ci as it appears in the calling code.
func getFuncName(L *State, ci *CallInfo, name *string) string {
	/* calling function is a known function? */
	if ci != nil && ci.callstatus&CIST_TAIL == 0 {
		return funcNameFromCall(L, ci.prev, name)
	}
	return "" /* no way to find a name */
}

// getUpvalName is getupvalname (ldebug.c): checks whether o is an
// upvalue of the running closure.
func getUpvalName(L *State, ci *CallInfo, o *Value, name *string) string {
	c := clLvalue(&L.stack[ci.funcIdx])
	for i := 0; i < c.nupvalues && i < len(c.upvals); i++ {
		if upvalValue(c.upvals[i]) == o {
			*name = upvalName(c.p, i)
			return strUpval
		}
	}
	return ""
}

// instack returns the register index of o in frame ci, or -1 (ldebug.c).
// C 5.4.9: base = ci->func.p + 1 (not 5.3's ci->u.l.base).
func instack(L *State, ci *CallInfo, o *Value) int {
	base := ci.funcIdx + 1
	for pos := 0; base+pos < ci.top && base+pos < len(L.stack); pos++ {
		if &L.stack[base+pos] == o {
			return pos
		}
	}
	return -1 /* not found */
}

// formatvarinfo is formatvarinfo (ldebug.c). Returns a Go string (C
// pushes via luaO_pushfstring). NUL truncation matches C printf "%s".
func formatvarinfo(kind, name string) string {
	if kind == "" {
		return "" /* no information */
	}
	/* C writes this with "%s", and a Lua string may hold a NUL — the
	   constant in '"a\0b" & 1' is one — which C's printf stops at.
	   PUC-Lua therefore reports that constant as 'a', and so does this. */
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return fmt.Sprintf(" (%s '%s')", kind, name)
}

// varinfo returns " (global 'print')" style extra information for a
// value (ldebug.c).
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

// ---------------------------------------------------------------------
// Error helpers (ldebug.c)
// ---------------------------------------------------------------------

// typeerror raises a type error (ldebug.c private).
func typeerror(L *State, o *Value, op, extra string) {
	t := luaT_objtypename(L, o)
	luaG_runerror(L, "attempt to %s a %s value%s", op, t, extra)
}

// luaG_typeerror raises "attempt to <op> a <type> value" (ldebug.c).
func luaG_typeerror(L *State, o *Value, op string) {
	typeerror(L, o, op, varinfo(L, o))
}

// luaG_callerror raises "attempt to call a <type> value" (ldebug.c).
// Prefers funcnamefromcall; falls back to varinfo.
func luaG_callerror(L *State, o *Value) {
	var name string
	kind := funcNameFromCall(L, L.ci, &name)
	extra := ""
	if kind != "" {
		extra = formatvarinfo(kind, name)
	} else {
		extra = varinfo(L, o)
	}
	typeerror(L, o, "call", extra)
}

// luaG_forerror raises a bad 'for' type error (ldebug.c).
func luaG_forerror(L *State, o *Value, what string) {
	luaG_runerror(L, "bad 'for' %s (number expected, got %s)",
		what, luaT_objtypename(L, o))
}

func luaG_concaterror(L *State, p1, p2 *Value) {
	if ttisstring(p1) || cvt2str(p1) {
		p1 = p2
	}
	luaG_typeerror(L, p1, "concatenate")
}

func luaG_opinterror(L *State, p1, p2 *Value, msg string) {
	if !ttisnumber(p1) { /* first operand is wrong? (5.4: ttisnumber) */
		p2 = p1 /* now second is wrong */
	}
	luaG_typeerror(L, p2, msg)
}

// luaG_tointerror reports that both operands are convertible to numbers
// but not to integers (ldebug.c). Uses tointegerns (5.4), not string coerce.
func luaG_tointerror(L *State, p1, p2 *Value) {
	var temp int64
	if !tointegerns(p1, &temp) { /* luaV_tointegerns(..., LUA_FLOORN2I) */
		p2 = p1
	}
	luaG_runerror(L, "number%s has no integer representation", varinfo(L, p2))
}

func luaG_ordererror(L *State, p1, p2 *Value) {
	t1 := luaT_objtypename(L, p1)
	t2 := luaT_objtypename(L, p2)
	if t1 == t2 {
		luaG_runerror(L, "attempt to compare two %s values", t1)
	} else {
		luaG_runerror(L, "attempt to compare %s with %s", t1, t2)
	}
}

// luaG_tracecall / luaG_traceexec / changedline / getbaseline: see lvm.go
// (trap fastpath; absolute lineinfo; hooks cannot yield).
