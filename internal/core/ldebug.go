// Translation of ldebug.h / ldebug.c (PUC-Lua 5.3.4).
// Hook install/get and getstack/getinfo wrappers; luaG_traceexec is in lvm.go.
// LUA_API bodies for getstack/getinfo live in ldblib.go (db*); getlocal/setlocal
// are implemented here (findlocal/findvararg) to match C ldebug.c placement.
package core

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------
// ldebug.h macros
// ---------------------------------------------------------------------

// pcRel is pcRel (ldebug.h). Go stores savedpc as the index of the next
// instruction (C: Instruction* into p->code); pcRel is that index minus 1.
func pcRel(savedpc int) int { return savedpc - 1 }

// luaG_getfuncline is getfuncline (ldebug.h): absolute lineinfo[pc], or -1.
// (This port stores absolute lines; C 5.3.4 is the same shape when lineinfo
// is present. Out-of-range pc returns -1 — Go safety; not 5.4 getbaseline.)
func luaG_getfuncline(f *Proto, pc int) int {
	if len(f.lineinfo) == 0 { /* no debug information? */
		return -1
	}
	if pc < 0 || pc >= len(f.lineinfo) {
		return -1
	}
	return int(f.lineinfo[pc])
}

// currentpc is currentpc (ldebug.c).
func currentpc(ci *CallInfo) int {
	/* lua_assert(isLua(ci)); */
	return pcRel(ci.savedpc)
}

// currentLine is currentline (ldebug.c): line of the instruction being
// executed. Extra !isLua → -1 is Go safety (C assumes isLua).
func currentLine(L *State, ci *CallInfo) int {
	if !isLua(ci) {
		return -1
	}
	cl := clLvalue(&L.stack[ci.funcIdx])
	return luaG_getfuncline(cl.p, currentpc(ci))
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
// (ldebug.c). C assumes EXTRA_STACK — no checkStack (would recurse into
// runerror on stack overflow).
func luaG_errormsg(L *State) {
	if L.errfunc != 0 { /* is there an error handling function? */
		errfunc := L.errfunc
		L.stack[L.top] = L.stack[L.top-1]   /* move argument */
		L.stack[L.top-1] = L.stack[errfunc] /* push function */
		L.top++                             /* assume EXTRA_STACK */
		luaD_callnoyield(L, L.top-2, 1)     /* call it */
	}
	luaD_throw(L, LUA_ERRRUN)
}

// luaG_runerror raises a runtime error with a formatted message (ldebug.c).
// pushfString (lobject) builds the TString without stacking it; this path
// pushes via addinfo / pushMessage so luaD_seterrorobj sees the message.
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
// Symbolic Execution (ldebug.c)
//
// Helpers behind varinfo — the "(global 'print')" suffix on type errors.
// ---------------------------------------------------------------------

// filterpc is filterpc (ldebug.c).
func filterpc(pc, jmptarget int) int {
	if pc < jmptarget { /* is code conditional (inside a jump)? */
		return -1 /* cannot know who sets that register */
	}
	return pc /* current position sets that register */
}

// findsetreg tries to find the last instruction before lastpc that
// modified register reg (ldebug.c 5.3.4).
func findsetreg(p *Proto, lastpc, reg int) int {
	setreg := -1 /* keep last instruction that changed 'reg' */
	jmptarget := 0 /* any code before this address is conditional */
	/* Go bounds: C assumes a valid lastpc (loop is pc < lastpc). */
	if lastpc < 0 {
		return -1
	}
	if lastpc > len(p.code) {
		lastpc = len(p.code)
	}
	for pc := 0; pc < lastpc; pc++ {
		i := p.code[pc]
		op := i.Op
		a := i.A
		switch op {
		case OP_LOADNIL:
			b := i.B
			if a <= reg && reg <= a+b { /* set registers from 'a' to 'a+b' */
				setreg = filterpc(pc, jmptarget)
			}
		case OP_TFORCALL:
			if reg >= a+2 { /* affect all regs above its base */
				setreg = filterpc(pc, jmptarget)
			}
		case OP_CALL, OP_TAILCALL:
			if reg >= a { /* affect all registers above base */
				setreg = filterpc(pc, jmptarget)
			}
		case OP_JMP:
			b := sBx(i.B)
			dest := pc + 1 + b
			/* jump is forward and do not skip 'lastpc'? */
			if pc < dest && dest <= lastpc {
				if dest > jmptarget {
					jmptarget = dest /* update 'jmptarget' */
				}
			}
		default:
			if testAMode(op) && reg == a { /* any instruction that set A */
				setreg = filterpc(pc, jmptarget)
			}
		}
	}
	return setreg
}

// upvalName is upvalname (ldebug.c).
func upvalName(p *Proto, uv int) string {
	if uv < 0 || uv >= len(p.upvalues) {
		return "?"
	}
	s := p.upvalues[uv].name
	if s == nil {
		return "?"
	}
	return s.val
}

// kname finds a "name" for the constant at plain K index (split from C's
// kname RK helper for clarity; see knameRK). Returns "constant" or "".
func kname(p *Proto, index int, name *string) string {
	if index < 0 || index >= len(p.k) || !ttisstring(&p.k[index]) {
		*name = "?"
		return ""
	}
	*name = tsvalue(&p.k[index]).val
	return "constant"
}

// knameRK is kname (ldebug.c): find a name for RK operand c.
func knameRK(p *Proto, pc, c int, name *string) {
	if ISK(c) { /* is 'c' a constant? */
		idx := INDEXK(c)
		if idx >= 0 && idx < len(p.k) {
			kvalue := &p.k[idx]
			if ttisstring(kvalue) { /* literal constant? */
				*name = tsvalue(kvalue).val /* it is its own name */
				return
			}
		}
		/* else no reasonable name found */
	} else { /* 'c' is a register */
		what := getobjname(p, pc, c, name) /* search for 'c' */
		if len(what) > 0 && what[0] == 'c' { /* found a constant name? */
			return /* 'name' already filled */
		}
		/* else no reasonable name found */
	}
	*name = "?" /* no reasonable name found */
}

// getobjname is getobjname (ldebug.c, Lua 5.3.4).
func getobjname(p *Proto, lastpc, reg int, name *string) string {
	*name = luaF_getlocalname(p, reg+1, lastpc)
	if *name != "" { /* is a local? */
		return "local"
	}
	/* else try symbolic execution */
	pc := findsetreg(p, lastpc, reg)
	if pc != -1 { /* could find instruction? */
		i := p.code[pc]
		op := i.Op
		switch op {
		case OP_MOVE:
			b := i.B /* move from 'b' to 'a' */
			if b < i.A {
				return getobjname(p, pc, b, name) /* get name for 'b' */
			}
		case OP_GETTABUP, OP_GETTABLE:
			k := i.C /* key index */
			t := i.B /* table index */
			var vn string
			if op == OP_GETTABLE { /* name of indexed variable */
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
		case OP_LOADK, OP_LOADKX:
			/* C: Bx for LOADK; Ax of next EXTRAARG for LOADKX.
			   Do not use kname() here — on non-string C leaves *name and
			   returns NULL (kname helper would set "?"). */
			b := i.B /* Bx in i.B (Instruction encoding) */
			if op == OP_LOADKX {
				if pc+1 >= len(p.code) {
					break
				}
				b = p.code[pc+1].A /* Ax of EXTRAARG */
			}
			if b >= 0 && b < len(p.k) && ttisstring(&p.k[b]) {
				*name = tsvalue(&p.k[b]).val
				return "constant"
			}
		case OP_SELF:
			knameRK(p, pc, i.C, name)
			return "method"
		default:
			/* go through to return NULL */
		}
	}
	return "" /* could not find reasonable name */
}

// funcNameFromCode is funcnamefromcode (ldebug.c, Lua 5.3.4).
// Takes Proto+pc (caller already computed currentpc); C takes CallInfo.
// CIST_HOOKED is checked in getFuncName before calling here (equivalent).
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
	/* 5.3.4: getstr(G(L)->tmname[tm]) keeps the "__" prefix. */
	*name = L.g.tmNames[tm].val
	return "metamethod"
}

// getFuncName is getfuncname (ldebug.c, Lua 5.3.4).
//
// CIST_FIN is set on the *caller* of the finalizer (GCTM), so querying
// that frame yields namewhat "metamethod" / name "__gc". CIST_HOOKED is
// checked here (C checks inside funcnamefromcode) — equivalent.
func getFuncName(L *State, ci *CallInfo, name *string) string {
	if ci == nil { /* no 'ci'? */
		return "" /* no info */
	}
	if ci.callstatus&CIST_FIN != 0 { /* is this a finalizer? */
		*name = "__gc"
		return "metamethod" /* report it as such */
	}
	/* calling function is a known Lua function? */
	if ci.callstatus&CIST_TAIL == 0 && ci.prev != nil && isLua(ci.prev) {
		prev := ci.prev
		/* 5.3.4 funcnamefromcode: hooked frame reports as hook '?'. */
		if prev.callstatus&CIST_HOOKED != 0 {
			*name = "?"
			return "hook"
		}
		return funcNameFromCode(L, clLvalue(&L.stack[prev.funcIdx]).p,
			currentpc(prev), name)
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
			return "upvalue"
		}
	}
	return ""
}

// instack returns the register index of o in frame ci, or -1
// (ldebug.c isinstack + cast_int(o - ci->u.l.base)).
// Must use Lua frame base (ci.base), not funcIdx+1: with varargs
// ci.base > funcIdx+1, so getobjname / varinfo indices are relative
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

// formatvarinfo builds the " (kind 'name')" suffix used by varinfo.
// Not a separate C 5.3.4 function (C inlines luaO_pushfstring in varinfo);
// Go returns a string instead of stacking. NUL truncation matches C printf.
func formatvarinfo(kind, name string) string {
	if kind == "" {
		return "" /* no information */
	}
	/* C writes this with "%s", and a Lua string may hold a NUL — the
	   constant in '"a\0b" & 1' is one — which C's printf stops at.
	   PUC-Lua therefore reports that constant as 'a', and so does this:
	   without the cut the message would name bytes it never shows. */
	if i := strings.IndexByte(name, 0); i >= 0 {
		name = name[:i]
	}
	return fmt.Sprintf(" (%s '%s')", kind, name)
}

// varinfo returns " (global 'print')" style extra information for a value
// (ldebug.c). Returns a Go string (C pushes via luaO_pushfstring).
func varinfo(L *State, o *Value) string {
	ci := L.ci
	if ci == nil || !isLua(ci) || o == nil {
		return ""
	}
	var name string
	kind := getUpvalName(L, ci, o, &name) /* check whether 'o' is an upvalue */
	if kind == "" {                       /* no? try a register */
		if reg := instack(L, ci, o); reg >= 0 {
			p := clLvalue(&L.stack[ci.funcIdx]).p
			kind = getobjname(p, currentpc(ci), reg, &name)
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
// Message is "number%s has no integer representation" with varinfo
// interpolated — *not* the offending number itself.
func luaG_tointerror(L *State, p1, p2 *Value) {
	var temp int64
	/* 5.3: tointeger coerces strings; 5.4 used tointegerns */
	if !tointeger(L, p1, &temp) {
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

// ---------------------------------------------------------------------
// findlocal / findvararg / lua_getlocal / lua_setlocal (ldebug.c)
//
// Index-stack: pos is a stack index (C: StkId). No swapextra (no
// CallInfo.extra — FLAG intentional).
// ---------------------------------------------------------------------

// findvararg is findvararg (ldebug.c). n is 1-based vararg index.
// Index-stack: needs L to resolve ci.funcIdx (C: StkId arithmetic on ci->func).
func findvararg(L *State, ci *CallInfo, n int) (string, int) {
	nparams := int(clLvalue(&L.stack[ci.funcIdx]).p.numparams)
	if n >= (ci.base-ci.funcIdx)-nparams {
		return "", 0 /* no such vararg */
	}
	return "(*vararg)", ci.funcIdx + nparams + n
}

// findlocal is findlocal (ldebug.c). Returns name and stack index of
// local/vararg/temporary n (1-based; n<0 → vararg). Empty name = none.
func findlocal(L *State, ci *CallInfo, n int) (string, int) {
	var name string
	var base int
	if isLua(ci) {
		if n < 0 { /* access to vararg values? */
			return findvararg(L, ci, -n)
		}
		base = ci.base
		name = luaF_getlocalname(clLvalue(&L.stack[ci.funcIdx]).p, n, currentpc(ci))
	} else {
		base = ci.funcIdx + 1
	}
	if name == "" { /* no 'standard' name? */
		limit := L.top
		if ci != L.ci && ci.next != nil {
			limit = ci.next.funcIdx
		}
		if limit-base >= n && n > 0 { /* is 'n' inside 'ci' stack? */
			name = "(*temporary)" /* generic name for any valid slot */
		} else {
			return "", 0 /* no name */
		}
	}
	return name, base + (n - 1)
}

// ---------------------------------------------------------------------
// hooks (ldebug.c)
// ---------------------------------------------------------------------

// SetHook installs a hook (lua_sethook). hook is the Go function the VM
// calls; mask is a combination of dbMaskCall, dbMaskRet, dbMaskLine and
// dbMaskCount; count is the instruction count between count hooks.
func (L *State) SetHook(hook func(L *State, ar *dbAr), mask, count int) {
	L.installHook(hook, mask, count, true)
}

// SetInterruptHook installs the stand-alone interpreter's SIGINT hook
// (lua.c's lstop): the next call, return or count event raises
// "interrupted!" and clears the hook. (Host specialization; not in
// ldebug.c — lives with lua_sethook plumbing.)
func (L *State) SetInterruptHook() {
	L.installHook(func(L *State, ar *dbAr) {
		L.installHook(nil, 0, 0, false)
		/* luaL_error(L, "interrupted!") */
		L.Error2("interrupted!")
	}, dbMaskCall|dbMaskRet|dbMaskCount, 1, true)
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
	/* ldebug.c lua_sethook: if (isLua(L->ci)) L->oldpc = L->ci->u.l.savedpc;
	   Go oldpc is pcRel-style, so savedpc-1. */
	if ci := L.ci; ci != nil && isLua(ci) {
		if ci.savedpc > 0 {
			L.oldpc = pcRel(ci.savedpc)
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
		setTraps(L) /* Go VM trap fastpath (vs per-insn mask in 5.3.4) */
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
// Debug API (ldebug.c LUA_API)
// getstack / getinfo: wrappers → db* in ldblib.go
// getlocal / setlocal: implemented here (C placement)
// swapextra omitted: no CallInfo.extra (index-stack intentional).
// ---------------------------------------------------------------------

// GetStack fills ar with the activation record at level (lua_getstack).
func (L *State) GetStack(level int, ar *dbAr) bool { return dbGetStack(L, level, ar) }

// GetInfo fills ar according to what (lua_getinfo).
func (L *State) GetInfo(what string, ar *dbAr) bool { return dbGetInfo(L, what, ar) }

// GetLocal is lua_getlocal. ar == nil → name of parameter n of the
// non-active Lua function on the stack top (no value pushed), matching C.
func (L *State) GetLocal(ar *dbAr, n int) string {
	if ar == nil { /* information about non-active function? */
		if L.top < 1 || !ttisLclosure(&L.stack[L.top-1]) { /* not a Lua function? */
			return ""
		}
		/* consider live variables at function start (parameters) */
		return luaF_getlocalname(clLvalue(&L.stack[L.top-1]).p, n, 0)
	}
	name, pos := findlocal(L, ar.ci, n)
	if name != "" {
		L.checkStack(1)
		L.stack[L.top] = L.stack[pos]
		L.top++
	}
	return name
}

// SetLocal is lua_setlocal: pops into local n of ar.
func (L *State) SetLocal(ar *dbAr, n int) string {
	name, pos := findlocal(L, ar.ci, n)
	if name != "" {
		L.stack[pos] = L.stack[L.top-1]
		L.top-- /* pop value */
	}
	return name
}
