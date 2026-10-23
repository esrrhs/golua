// Translation of ldo.h / ldo.c (PUC-Lua 5.4.9), line-faithful.
//
// Intentional adaptations only (see NOTES.md):
//   - panic/recover instead of setjmp/longjmp (rawRunProtected in lstate.go)
//   - index stack ([]Value indices), not TValue* / StkId pointers
//   - Go GC (no luaC_checkGC / emergency GC stop around realloc)
//   - goroutine coroutines: precallC may migrateAbort / fastYield;
//     resume / yield / unroll / finishCcall / precover / protectedparser
//     live in lstate.go / lbaselib.go / lauxlib.go (colocated SPEC)
//   - no lua_Alloc / lua_lock / lua_unlock
//   - luaD_closeprotected / luaD_pcall take extra yy (yieldable close)
package core

const (
	LUAI_MAXCCALLS = 200
)

// errorstatus reports whether 's' is a real error (not OK / YIELD).
// C: #define errorstatus(s) ((s) > LUA_YIELD)
func errorstatus(s int) bool { return s > LUA_YIELD }

// ---------------------------------------------------------------------
// Error-recovery functions (ldo.c)
// ---------------------------------------------------------------------

// luaD_seterrorobj puts the error object at 'oldtop' (ldo.c 5.4.9).
//
// 5.4 dropped the LUA_ERRERR case: luaD_errerr already pushes the
// message before throw, so the default arm copies it.
func luaD_seterrorobj(L *State, errcode, oldtop int) {
	switch errcode {
	case LUA_ERRMEM: /* memory error? */
		var v Value
		setsvalue(&v, L.g.memErrMsg) /* reuse preregistered msg. */
		L.stack[oldtop] = v
	case LUA_OK: /* special case only for closing upvalues */
		setnilvalue(&L.stack[oldtop]) /* no error message */
	default:
		/* real error: message is on current top */
		L.stack[oldtop] = L.stack[L.top-1]
	}
	L.top = oldtop + 1
}

// luaD_throw raises an error with the given status (ldo.c 5.4.9).
//
// C: if L->errorJmp → set status + LUAI_THROW; else resetthread, then
// either copy to mainthread and rethrow, or g->panic + abort().
// Go: panic/recover (rawRunProtected) stands in for the longjmp chain.
// The error object is copied into the panic value because a Go defer
// between throw and recover can pop the Lua stack (C longjmp does not
// run those). Mainthread rethrow omitted — coroutine errors recover on
// the running thread (goroutine SPEC). Unprotected atpanic: Apply debt
// (needs errorJmp nesting counter in State; see NOTES).
func luaD_throw(L *State, errcode int) {
	var v Value
	if L.top > 0 {
		v = L.stack[L.top-1]
	} else {
		setnilvalue(&v)
	}
	/* Protected path: rawRunProtected recovers this panic.
	   Unprotected: unrecovered panic ≡ C abort().
	   Apply debt: with an errorJmp nesting counter on State, the
	   unprotected arm should luaE_resetthread, call g.panic(L), then
	   panic (and omit mainthread rethrow — goroutine SPEC). */
	panic(&luaError{status: errcode, value: v})
}

// luaD_rawrunprotected is luaD_rawrunprotected (ldo.c).
// Delegates to State.rawRunProtected (panic/recover adaptation of setjmp).
func luaD_rawrunprotected(L *State, f func()) int {
	return L.rawRunProtected(f)
}

// ---------------------------------------------------------------------
// Stack reallocation (ldo.c)
// ---------------------------------------------------------------------
//
// Index-stack SPEC: C's relstack/correctstack convert pointers ↔ offsets
// across realloc. Go indices stay valid — relstack is a no-op; correctstack
// still sets trap bits. Runtime grow/check also live on *State in lstate.go;
// luaD_* forms below match C 5.4.9 (realloc → correctstack/setTraps).
// Apply debt: point State.growStackNoRaise / checkStack at these.

// relstack: no-op with index-based stacks (C converts pointers → offsets).
func relstack(L *State) { _ = L }

// correctstack restores stack pointers after realloc (ldo.c).
// Index-stack: pointer restore is a no-op; still set trap bits like C.
func correctstack(L *State) {
	setTraps(L) /* C: if (isLua(ci)) ci->u.l.trap = 1 */
}

// ERRORSTACKSIZE is defined in lstate.go (LUAI_MAXSTACK + 200).

// luaD_errerr raises an error while running the message handler (ldo.c).
func luaD_errerr(L *State) {
	ts := luaS_new(L, "error in error handling")
	if L.top >= len(L.stack) {
		L.reallocStack(len(L.stack) + EXTRA_STACK) /* assume EXTRA_STACK */
	}
	setsvalue(&L.stack[L.top], ts)
	L.top++
	luaD_throw(L, LUA_ERRERR)
}

// luaD_checkminstack checks whether stacks have enough space to run a
// simple function such as a finalizer (ldo.c 5.4.9): BASIC_STACK_SIZE
// free Lua slots, two available CallInfos, two C-stack slots.
//
// FLAG: Go CI extend always succeeds (heap alloc); C's luaE_extendCI(L,0)
// can fail under lua_Alloc. We only enforce the C-call and Lua-stack arms.
func luaD_checkminstack(L *State) int {
	if getCcalls(L) >= LUAI_MAXCCALLS-2 {
		return 0 /* not enough C-stack slots */
	}
	/* C also requires two free CallInfos; Go heap-extend cannot fail. */
	if L.stackLast-L.top >= BASIC_STACK_SIZE {
		return 1 /* enough free slots */
	}
	if luaD_growstack(L, BASIC_STACK_SIZE, false) {
		return 1
	}
	return 0
}

// getCcalls is the lower 16 bits of nCcalls (lstate.h).

// luaD_reallocstack reallocates the stack to 'newsize' usable slots
// (ldo.c 5.4.9). 'raiseerror' selects throw vs soft failure.
// Index-stack: relstack is a no-op; correctstack still sets traps.
// No emergency-GC stop (Go GC SPEC).
func luaD_reallocstack(L *State, newsize int, raiseerror bool) bool {
	/* C: assert newsize <= LUAI_MAXSTACK || newsize == ERRORSTACKSIZE */
	relstack(L) /* change pointers to offsets (no-op on index stack) */
	/* C: gcstopem = 1 around realloc — omitted (Go GC) */
	L.reallocStack(newsize)
	correctstack(L) /* restore pointers + set traps on Lua CIs */
	_ = raiseerror  /* Go reallocStack does not fail; soft-fail N/A */
	return true
}

// luaD_growstack grows the stack by at least 'n' elements (ldo.c 5.4.9).
// When raiseerror is false, returns false on failure instead of throwing.
func luaD_growstack(L *State, n int, raiseerror bool) bool {
	size := L.stackSize()
	if size > LUAI_MAXSTACK {
		/* already on ERRORSTACKSIZE / handling a stack error */
		if raiseerror {
			luaD_errerr(L) /* error inside message handler */
		}
		return false
	} else if n < LUAI_MAXSTACK { /* avoids arithmetic overflows */
		newsize := 2 * size          /* tentative new size */
		needed := L.top + n          /* C: (top - stack) + n */
		if newsize > LUAI_MAXSTACK { /* cannot cross the limit */
			newsize = LUAI_MAXSTACK
		}
		if newsize < needed { /* but must respect what was asked for */
			newsize = needed
		}
		if newsize <= LUAI_MAXSTACK {
			return luaD_reallocstack(L, newsize, raiseerror)
		}
	}
	/* else stack overflow — grow to ERRORSTACKSIZE for the message */
	luaD_reallocstack(L, ERRORSTACKSIZE, raiseerror)
	if raiseerror {
		luaG_runerror(L, "stack overflow")
	}
	return false
}

// stackinuse computes how much of the stack is in use (ldo.c).
func stackinuse(L *State) int {
	lim := L.top
	for ci := L.ci; ci != nil; ci = ci.prev {
		if lim < ci.top {
			lim = ci.top
		}
	}
	res := lim + 1 /* part of stack in use (C: lim - stack + 1) */
	if res < LUA_MINSTACK {
		res = LUA_MINSTACK /* ensure a minimum size */
	}
	return res
}

// luaD_shrinkstack shrinks an oversized stack (ldo.c 5.4.9).
//
// State.shrinkStack uses a different heuristic (halve when used < size/4);
// Apply debt: make that a thin wrapper of this.
func luaD_shrinkstack(L *State) {
	inuse := stackinuse(L)
	max := inuse * 3
	if inuse > LUAI_MAXSTACK/3 {
		max = LUAI_MAXSTACK
	}
	size := L.stackSize() /* usable = C stacksize(L) */
	if inuse <= LUAI_MAXSTACK && size > max {
		nsize := inuse * 2
		if inuse > LUAI_MAXSTACK/2 {
			nsize = LUAI_MAXSTACK
		}
		luaD_reallocstack(L, nsize, false) /* ok if that fails */
	}
	/* else: C condmovestack debug only — omitted */
	luaE_shrinkCI(L) /* shrink CI list */
}

// luaD_inctop is luaD_inctop (ldo.c).
func luaD_inctop(L *State) {
	L.checkStack(1)
	L.top++
}

// luaE_shrinkCI frees half of the unused CallInfo nodes (lstate.c).
// C lives in lstate.c; kept here because luaD_shrinkstack calls it.
// Distinct from State.shrinkCI in lstate.go (that one unwinds to baseCI).
func luaE_shrinkCI(L *State) {
	ci := L.ci
	if ci == nil {
		return
	}
	for ci.next != nil && ci.next.next != nil {
		next2 := ci.next.next
		L.freeCI(ci.next) /* free next */
		ci.next = next2
		next2.prev = ci
		ci = next2
	}
}

// ---------------------------------------------------------------------
// Hooks (ldo.c)
// ---------------------------------------------------------------------

// luaD_hook calls the hook for the given event (ldo.c 5.4.9).
// ftransfer/ntransfer are 5.4 transferinfo for call/return hooks.
func luaD_hook(L *State, event, line, ftransfer, ntransfer int) {
	hook := L.hook
	if hook == nil || L.allowhook == 0 {
		return
	}
	mask := uint16(CIST_HOOKED)
	ci := L.ci
	top := L.top    /* preserve original 'top' */
	ciTop := ci.top /* idem for 'ci->top' */
	ar := &dbAr{event: event, currentline: line, ci: ci}
	if ntransfer != 0 {
		mask |= CIST_TRAN /* 'ci' has transfer information */
		ci.ftransfer = ftransfer
		ci.ntransfer = ntransfer
	}
	if isLua(ci) && L.top < ci.top {
		L.top = ci.top /* protect entire activation register */
	}
	L.checkStack(LUA_MINSTACK) /* ensure minimum stack size */
	if ci.top < L.top+LUA_MINSTACK {
		ci.top = L.top + LUA_MINSTACK
	}
	L.allowhook = 0 /* cannot call hooks inside a hook */
	ci.callstatus |= mask
	hook(L, ar)
	L.allowhook = 1
	ci.top = ciTop
	L.top = top
	ci.callstatus &= ^mask
}

// luaD_hookcall executes a call hook for Lua functions (ldo.c).
// Called whenever 'hookmask' is not zero; checks whether call hooks are active.
func luaD_hookcall(L *State, ci *CallInfo) {
	L.oldpc = 0                     /* set 'oldpc' for new function */
	if L.hookmask&dbMaskCall != 0 { /* is call hook on? */
		event := hookCall
		if ci.callstatus&CIST_TAIL != 0 {
			event = hookTailCall
		}
		p := clLvalue(&L.stack[ci.funcIdx]).p
		ci.savedpc++ /* hooks assume 'pc' is already incremented */
		luaD_hook(L, event, -1, 1, int(p.numparams))
		ci.savedpc-- /* correct 'pc' */
	}
}

// rethook executes a return hook for Lua and Go functions and sets /
// corrects 'oldpc' (ldo.c). Correction is needed by the line hook, so
// it is done even when return hooks are off.
func rethook(L *State, ci *CallInfo, nres int) {
	if L.hookmask&dbMaskRet != 0 { /* is return hook on? */
		firstres := L.top - nres /* index of first result */
		delta := 0               /* correction for vararg functions */
		if isLua(ci) {
			p := clLvalue(&L.stack[ci.funcIdx]).p
			if p.isVararg != 0 {
				delta = ci.nExtraArgs + int(p.numparams) + 1
			}
		}
		ci.funcIdx += delta /* if vararg, back to virtual 'func' */
		ftransfer := firstres - ci.funcIdx
		luaD_hook(L, hookRet, -1, ftransfer, nres)
		ci.funcIdx -= delta
	}
	if prev := ci.prev; prev != nil && isLua(prev) {
		/* C: L->oldpc = pcRel(ci->u.l.savedpc, ...); Go savedpc is already
		   an instruction index, so pcRel ≡ savedpc-1 when savedpc > 0. */
		L.oldpc = prev.savedpc - 1
	}
}

// setTraps marks every Lua frame as needing a hook check (ldebug.c
// lua_sethook side effect; colocated here for the Go VM).
//
// C 5.4 sets ci->u.l.trap when the stack is corrected after realloc and
// when hooks are installed; the interpreter tests trap each instruction.
// FLAG: Go VM SPEC — same trap bit, used so a mid-call lua_sethook is
// seen without polling hookmask every opcode.
func setTraps(L *State) {
	for ci := L.ci; ci != nil; ci = ci.prev {
		if isLua(ci) {
			ci.trap = true
		}
	}
}

// ---------------------------------------------------------------------
// Call preparation / results (ldo.c)
// ---------------------------------------------------------------------

// tryfuncTM puts the __call metamethod below the original function so
// that the call can proceed (ldo.c). Raises if there is no __call.
func tryfuncTM(L *State, funcIdx int) {
	L.checkStack(1) /* space for metamethod (C: checkstackGCp) */
	tm := luaT_gettmbyobj(L, &L.stack[funcIdx], TM_CALL)
	if ttisnil(tm) {
		luaG_callerror(L, &L.stack[funcIdx])
		return
	}
	for p := L.top; p > funcIdx; p-- { /* open space for metamethod */
		L.stack[p] = L.stack[p-1]
	}
	L.top++
	L.stack[funcIdx] = *tm /* metamethod is the new function to be called */
}

// hastocloseCfunc, codeNresults and decodeNresults are lapi.h's: whether
// a C frame has to-be-closed variables pending, which PUC-Lua encodes in
// the sign of the frame's 'nresults'.
func hastocloseCfunc(n int) bool { return n < LUA_MULTRET }
func codeNresults(n int) int     { return -n - 3 }
func decodeNresults(n int) int   { return -n - 3 }

// moveresults moves nres results at the top of the stack to 'res' (ldo.c).
//
// Matches C 5.4.9: hastocloseCfunc is handled *inside* here (CIST_CLSRET,
// luaF_close at res==func, rethook after closes, then decodeNresults).
// u2.nres is stored in ci.ntransfer (C union with transferinfo).
// Yielding __close across a C return still needs finishCcall re-entry
// (colocated in lstate.go) — Apply debt if that path is incomplete.
func moveresults(L *State, res, nres, wanted int) {
	switch wanted { /* handle typical cases separately */
	case 0: /* no values needed */
		L.top = res
		return
	case 1: /* one value needed */
		if nres == 0 { /* no results? */
			setnilvalue(&L.stack[res]) /* adjust with nil */
		} else {
			L.stack[res] = L.stack[L.top-nres] /* move it to proper place */
		}
		L.top = res + 1
		return
	case LUA_MULTRET:
		wanted = nres /* we want all results */
	default: /* two/more results and/or to-be-closed variables */
		if hastocloseCfunc(wanted) { /* to-be-closed variables? */
			ci := L.ci
			ci.callstatus |= CIST_CLSRET /* in case of yields */
			ci.ntransfer = nres          /* C: ci->u2.nres */
			luaF_close(L, res, closeKTOP, true)
			ci.callstatus &^= CIST_CLSRET
			if L.hookmask != 0 { /* call hook after '__close's */
				rethook(L, ci, nres)
				/* C: restore res after hook may move stack; index stack OK */
			}
			wanted = decodeNresults(wanted)
			if wanted == LUA_MULTRET {
				wanted = nres /* we want all results */
			}
		}
	}
	/* generic case */
	firstresult := L.top - nres /* index of first result */
	if nres > wanted {          /* extra results? */
		nres = wanted /* don't need them */
	}
	i := 0
	for ; i < nres; i++ { /* move all results to correct place */
		L.stack[res+i] = L.stack[firstresult+i]
	}
	for ; i < wanted; i++ { /* complete wanted number of results */
		setnilvalue(&L.stack[res+i])
	}
	L.top = res + wanted /* top points after the last result */
}

// luaD_poscall finishes a function call (ldo.c 5.4.9).
//
// C: rethook only when hookmask && !hastocloseCfunc; close lives in
// moveresults. Does not mutate ci.nresults after decode (coded value kept).
func luaD_poscall(L *State, ci *CallInfo, nres int) {
	wanted := ci.nresults
	if L.hookmask != 0 && !hastocloseCfunc(wanted) {
		rethook(L, ci, nres)
	}
	/* move results to proper place (may close tbc vars) */
	moveresults(L, ci.funcIdx, nres, wanted)
	/* C asserts !(HOOKED|YPCALL|FIN|TRAN|CLSRET) */
	L.ci = ci.prev /* back to caller (after closing variables) */
}

// next_ci is next_ci (ldo.c): reuse free CI or extend.
func next_ci(L *State) *CallInfo {
	if L.ci != nil && L.ci.next != nil {
		L.ci = L.ci.next
		return L.ci
	}
	return L.extendCI()
}

// prepCallInfo prepares the CallInfo for a new call (ldo.c).
//
// FLAG: also clears Go-only fields (goCont, migrateFunc, nExtraArgs,
// savedpc) so a recycled CI from the pool cannot leak a continuation.
func prepCallInfo(L *State, funcIdx, nret int, mask uint16, top int) *CallInfo {
	ci := next_ci(L) /* new frame */
	ci.funcIdx = funcIdx
	ci.nresults = nret
	ci.callstatus = mask
	ci.top = top
	ci.savedpc = 0
	ci.nExtraArgs = 0
	ci.goCont = nil
	ci.migrateFunc = nil
	return ci
}

// precallC calls a Go/C function (ldo.c).
//
// FLAG goroutine SPEC:
//   - migrateAbort when a sync coroutine in the VM is about to enter a
//     non-yield Go function (must keep C/Go stack for later yield).
//   - fastYield early return skips poscall; Resume finishes the frame.
func precallC(L *State, funcIdx, nresults int, f GoFunc) int {
	L.checkStack(LUA_MINSTACK) /* ensure minimum stack size (C: checkstackGCp) */
	ci := prepCallInfo(L, funcIdx, nresults, CIST_C, L.top+LUA_MINSTACK)
	if L.hookmask&dbMaskCall != 0 { /* is call hook on? */
		narg := L.top - funcIdx - 1
		luaD_hook(L, hookCall, -1, 1, narg)
	}
	/* The synchronous coroutine path may only unwind pure Lua and
	   coroutine.yield. Any other Go function can call back into Lua,
	   establish an error boundary, or expose the live stack through the
	   debug API. Move its already-pushed frame before entering it. */
	if L.co != nil && L.co.sync && L.inVM &&
		funcptr(f) != funcptr(coYield) {
		ci.migrateFunc = f
		panic(&migrateAbort{})
	}
	n := f(L) /* do the actual call */
	if L.fastYield {
		/* synchronous yield: the frame stays, Resume finishes it */
		return 0
	}
	luaD_poscall(L, ci, n)
	return n
}

// luaD_precall prepares a call (ldo.c 5.4.9).
// Returns the CallInfo when the callee is a Lua function (caller must
// then run luaV_execute), or nil when the callee was a Go function and
// the results are already on the stack.
//
// 5.4: Lua path only pads missing fixed params with nil — no 5.3
// adjust_varargs reshape (varargs are OP_VARARG*).
func luaD_precall(L *State, funcIdx, nresults int) *CallInfo {
retry:
	fv := &L.stack[funcIdx]
	switch ttypetag(fv) {
	case LUA_VCCL: /* C closure */
		precallC(L, funcIdx, nresults, clCvalue(fv).f)
		return nil
	case LUA_VLCF: /* light C function */
		precallC(L, funcIdx, nresults, fvalue(fv))
		return nil
	case LUA_VLCL: /* Lua function */
		cl := clLvalue(fv)
		p := cl.p
		narg := L.top - funcIdx - 1
		nfixparams := int(p.numparams)
		fsize := int(p.maxstacksize)
		L.checkStack(fsize) /* C: checkstackGCp */
		ci := prepCallInfo(L, funcIdx, nresults, 0, funcIdx+1+fsize)
		/* ci->u.l.savedpc = p->code; Go: savedpc index 0 */
		for ; narg < nfixparams; narg++ {
			setnilvalue(&L.stack[L.top])
			L.top++
		}
		return ci
	default: /* not a function: retry with '__call' */
		tryfuncTM(L, funcIdx)
		goto retry
	}
}

// luaD_pretailcall prepares a tail call (ldo.c 5.4.9).
//
// Returns -1 when the callee is a Lua function: no new CallInfo — the
// caller's frame is reused (proper tail call). Returns the number of
// results when the callee was a Go function (caller still needs poscall).
func luaD_pretailcall(L *State, ci *CallInfo, funcIdx, narg1, delta int) int {
retry:
	fv := &L.stack[funcIdx]
	switch ttypetag(fv) {
	case LUA_VCCL: /* C closure */
		return precallC(L, funcIdx, LUA_MULTRET, clCvalue(fv).f)
	case LUA_VLCF: /* light C function */
		return precallC(L, funcIdx, LUA_MULTRET, fvalue(fv))
	case LUA_VLCL: /* Lua function */
		p := clLvalue(fv).p
		fsize := int(p.maxstacksize)
		nfixparams := int(p.numparams)
		L.checkStack(fsize - delta)
		ci.funcIdx -= delta /* restore 'func' (if vararg) */
		/* move down function and arguments */
		for i := 0; i < narg1; i++ {
			L.stack[ci.funcIdx+i] = L.stack[funcIdx+i]
		}
		funcIdx = ci.funcIdx /* moved-down function */
		for ; narg1 <= nfixparams; narg1++ {
			setnilvalue(&L.stack[funcIdx+narg1]) /* complete missing arguments */
		}
		ci.top = funcIdx + 1 + fsize /* top for the new function */
		ci.savedpc = 0               /* starting point */
		ci.callstatus |= CIST_TAIL
		L.top = funcIdx + narg1
		return -1
	default: /* not a function */
		tryfuncTM(L, funcIdx) /* try to get '__call' metamethod */
		narg1++
		goto retry
	}
}

// ---------------------------------------------------------------------
// ccall / luaD_call / luaD_callnoyield (ldo.c)
// ---------------------------------------------------------------------

// luaE_incCstack counts one more C level and raises when exhausted (lstate.c).
// Only the lower 16 bits of nCcalls count levels; the upper 16 mark
// "not yieldable" and must not look like depth.
func luaE_incCstack(L *State) {
	L.nCcalls++
	if L.nCcalls&0xffff >= LUAI_MAXCCALLS {
		luaE_checkcstack(L)
	}
}

// luaE_checkcstack raises when the C stack is exhausted (lstate.c).
func luaE_checkcstack(L *State) {
	calls := L.nCcalls & 0xffff
	if calls == LUAI_MAXCCALLS {
		luaG_runerror(L, "C stack overflow")
	} else if calls >= LUAI_MAXCCALLS/10*11 {
		luaD_errerr(L) /* error while handling stack error */
	}
}

// ccall is the common part of luaD_call / luaD_callnoyield (ldo.c).
// 'inc' is 1 or nyci (also bumps the non-yieldable upper half).
func ccall(L *State, funcIdx, nresults int, inc int) {
	L.nCcalls += uint32(inc)
	if L.nCcalls&0xffff >= LUAI_MAXCCALLS {
		/* C: checkstackp(L, 0, func) frees EXTRA_STACK use before raise */
		L.checkStack(0)
		luaE_checkcstack(L)
	}
	if ci := luaD_precall(L, funcIdx, nresults); ci != nil {
		ci.callstatus = CIST_FRESH /* mark that it is a "fresh" execute */
		luaV_execute(L, ci)
	}
	L.nCcalls -= uint32(inc)
}

func luaD_call(L *State, funcIdx, nresults int) { ccall(L, funcIdx, nresults, 1) }

// nyci is the increment used by luaD_callnoyield: bumps the upper 16
// bits of nCcalls, which makes yieldable false inside the call (lstate.h).
const nyci = 0x10000 | 1

func luaD_callnoyield(L *State, funcIdx, nresults int) { ccall(L, funcIdx, nresults, nyci) }

// ---------------------------------------------------------------------
// Protected close / pcall (ldo.c)
// ---------------------------------------------------------------------
//
// C's finishpcallk / finishCcall / unroll / findpcall / precover /
// resume / lua_resume / lua_yieldk / luaD_protectedparser are NOT in
// this file — goroutine/API split places them in lstate.go and load
// paths. Documented in NOTES (colocated yield/resume).

// closeP carries the arguments of closepaux (ldo.c).
type closeP struct {
	level  int
	status int
}

// luaD_closeprotected closes to-be-closed variables under a protected
// call (ldo.c).
//
// FLAG: extra 'yy' — C's closepaux always passes 0 (non-yieldable);
// finishpcallk closes with yy=1. This port's single pcall path takes
// yy as an argument instead of splitting the two C entry points.
func luaD_closeprotected(L *State, level, status int, yy bool) int {
	oldCI := L.ci
	oldAllowHooks := L.allowhook
	for { /* keep closing upvalues until no more errors */
		pcl := &closeP{level: level, status: status}
		st := L.rawRunProtected(func() { luaF_close(L, pcl.level, pcl.status, yy) })
		if st == LUA_OK { /* no more errors? */
			return pcl.status
		}
		/* an error occurred; restore saved state and repeat */
		L.ci = oldCI
		L.allowhook = oldAllowHooks
		status = st
	}
}

// luaD_pcall runs f in protected mode (ldo.c).
//
// FLAG: extra 'yy' for the close path — see luaD_closeprotected.
func luaD_pcall(L *State, f func(), oldTop, ef int, yy bool) int {
	oldCI := L.ci
	oldAllowHooks := L.allowhook
	oldErrFunc := L.errfunc
	L.errfunc = ef
	status := L.rawRunProtected(f)
	if status != LUA_OK { /* an error occurred? */
		L.ci = oldCI
		L.allowhook = oldAllowHooks
		status = luaD_closeprotected(L, oldTop, status, yy)
		luaD_seterrorobj(L, status, oldTop)
		luaD_shrinkstack(L) /* restore stack size in case of overflow */
	}
	L.errfunc = oldErrFunc
	return status
}

// ---------------------------------------------------------------------
// Colocated elsewhere (C ldo.c → Go other files) — see NOTES.md
// ---------------------------------------------------------------------
// finishpcallk, finishCcall, unroll, findpcall, resume_error, resume,
// precover, lua_resume, lua_isyieldable, lua_yieldk  →  lstate.go
//   (Resume / Yield / unroll / fastYield / migrateAbort / coMain)
// luaD_protectedparser / f_parser / checkmode / SParser  →  load path
//   (lbaselib.go Load* / lauxlib helpers; mode checks in load)
