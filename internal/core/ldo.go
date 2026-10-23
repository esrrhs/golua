// Translation of ldo.h / ldo.c (PUC-Lua 5.3.4), line-by-line.
//
// Intentional adaptations only:
//   - panic/recover instead of setjmp/longjmp (rawRunProtected in lstate.go)
//   - stack slots are []Value indices, not TValue* pointers
//   - Go GC (no luaC_checkGC / mark-sweep)
//   - goroutine coroutines: precallC may migrate/sync-yield (migrateAbort,
//     fastYield); resume/yield entry points stay coordinated with lstate.go
//   - no lua_Alloc / lua_lock / lua_unlock
package core

const (
	LUAI_MAXCCALLS = 200
)

func errorstatus(s int) bool { return s > LUA_YIELD }

// ---------------------------------------------------------------------
// Error-recovery (ldo.c)
// ---------------------------------------------------------------------

// seterrorobj is seterrorobj (ldo.c).
func seterrorobj(L *State, errcode, oldtop int) {
	switch errcode {
	case LUA_ERRMEM: /* memory error? */
		setsvalue(&L.stack[oldtop], L.g.memErrMsg) /* reuse preregistered msg. */
	case LUA_ERRERR:
		setsvalue(&L.stack[oldtop], luaS_new(L, "error in error handling"))
	default:
		L.stack[oldtop] = L.stack[L.top-1] /* error message on current top */
	}
	L.top = oldtop + 1
}

// luaD_seterrorobj is the exported name used by call sites (same as seterrorobj).
func luaD_seterrorobj(L *State, errcode, oldtop int) {
	seterrorobj(L, errcode, oldtop)
}

// luaD_throw raises an error with the given status (ldo.c).
//
// C longjmps via L->errorJmp or rethrows on the main thread / calls panic.
// Go adaptation: always panic with the error object copied into luaError
// so rawRunProtected (setjmp stand-in) can recover it after defers run.
func luaD_throw(L *State, errcode int) {
	var v Value
	if L.top > 0 {
		v = L.stack[L.top-1]
	} else {
		setnilvalue(&v)
	}
	/* If there is no recover point, mirror C's last-chance panic hook. */
	if L.g != nil && L.g.panic != nil {
		/* Only invoke when we appear unprotected; rawRunProtected recovers
		   luaError, so the panic function is for truly uncaught throws. */
	}
	_ = L
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

// correctstack is a no-op with index-based stacks (ldo.c correctstack
// rebases pointers after realloc; indices stay valid).
func correctstack(L *State, oldstack []Value) {
	_ = L
	_ = oldstack
}

// ERRORSTACKSIZE is in lstate.go (shared with grow paths).

// luaD_reallocstack is luaD_reallocstack (ldo.c).
// 'newsize' matches C: total vector length including EXTRA_STACK margin
// is newsize; usable last index is newsize - EXTRA_STACK.
func luaD_reallocstack(L *State, newsize int) {
	/* Go adaptation: reallocStack takes usable size (stack_last - stack). */
	usable := newsize - EXTRA_STACK
	if usable < 0 {
		usable = newsize
	}
	L.reallocStack(usable)
}

// luaD_growstack is luaD_growstack (ldo.c).
func luaD_growstack(L *State, n int) {
	size := L.stackSize()
	if size > LUAI_MAXSTACK { /* error after extra size? */
		luaD_throw(L, LUA_ERRERR)
		return
	}
	needed := L.top + n + EXTRA_STACK
	newsize := 2 * size
	if newsize > LUAI_MAXSTACK {
		newsize = LUAI_MAXSTACK
	}
	if newsize < needed {
		newsize = needed
	}
	if newsize > LUAI_MAXSTACK { /* stack overflow? */
		L.reallocStack(ERRORSTACKSIZE)
		luaG_runerror(L, "stack overflow")
		return
	}
	L.reallocStack(newsize)
}

// stackinuse is stackinuse (ldo.c).
func stackinuse(L *State) int {
	lim := L.top
	for ci := L.ci; ci != nil; ci = ci.prev {
		if lim < ci.top {
			lim = ci.top
		}
	}
	return lim + 1 /* part of stack in use (C: lim - stack + 1) */
}

// luaD_shrinkstack is luaD_shrinkstack (ldo.c).
func luaD_shrinkstack(L *State) {
	inuse := stackinuse(L)
	goodsize := inuse + (inuse / 8) + 2*EXTRA_STACK
	if goodsize > LUAI_MAXSTACK {
		goodsize = LUAI_MAXSTACK
	}
	if L.stackSize() > LUAI_MAXSTACK { /* had been handling stack overflow? */
		luaE_freeCI(L) /* free all CIs (list grew because of an error) */
	} else {
		luaE_shrinkCI(L) /* shrink list */
	}
	if inuse <= (LUAI_MAXSTACK-EXTRA_STACK) && goodsize < L.stackSize() {
		L.reallocStack(goodsize)
	}
}

// luaD_inctop is luaD_inctop (ldo.c).
func luaD_inctop(L *State) {
	L.checkStack(1)
	L.top++
}

// luaE_freeCI frees all CallInfo structures not in use (lstate.c).
func luaE_freeCI(L *State) {
	ci := L.ci
	if ci == nil {
		return
	}
	next := ci.next
	ci.next = nil
	for ci = next; ci != nil; ci = next {
		next = ci.next
		L.freeCI(ci)
	}
}

// luaE_shrinkCI frees half of the unused CallInfo structures (lstate.c).
func luaE_shrinkCI(L *State) {
	ci := L.ci
	if ci == nil {
		return
	}
	for ci.next != nil && ci.next.next != nil {
		next2 := ci.next.next
		L.freeCI(ci.next) /* free next */
		ci.next = next2   /* remove 'next' from the list */
		next2.prev = ci
		ci = next2 /* keep next's next */
	}
}

// ---------------------------------------------------------------------
// Hooks (ldo.c)
// ---------------------------------------------------------------------

// luaD_hook is luaD_hook (ldo.c 5.3.4): (L, event, line) only — no transfer info.
func luaD_hook(L *State, event, line int) {
	hook := L.hook
	if hook != nil && L.allowhook != 0 { /* make sure there is a hook */
		ci := L.ci
		top := L.top
		ciTop := ci.top
		ar := &dbAr{event: event, currentline: line, ci: ci}
		L.checkStack(LUA_MINSTACK) /* ensure minimum stack size */
		ci.top = L.top + LUA_MINSTACK
		L.allowhook = 0 /* cannot call hooks inside a hook */
		ci.callstatus |= CIST_HOOKED
		hook(L, ar)
		L.allowhook = 1
		ci.top = ciTop
		L.top = top
		ci.callstatus &= ^uint16(CIST_HOOKED)
	}
}

// callhook is callhook (ldo.c): call/tailcall hook for a Lua frame in precall.
func callhook(L *State, ci *CallInfo) {
	hook := hookCall
	ci.savedpc++ /* hooks assume 'pc' is already incremented */
	if prev := ci.prev; prev != nil && isLua(prev) {
		/* previous instruction is the CALL/TAILCALL that entered us */
		pc := prev.savedpc - 1
		if pc >= 0 {
			p := clLvalue(&L.stack[prev.funcIdx]).p
			if pc < len(p.code) && p.code[pc].Op == OP_TAILCALL {
				ci.callstatus |= CIST_TAIL
				hook = hookTailCall
			}
		}
	}
	luaD_hook(L, hook, -1)
	ci.savedpc-- /* correct 'pc' */
}

// setTraps marks every Lua frame so the VM rechecks hooks after lua_sethook
// (Go trap optimization; 5.3.4 checks hookmask every instruction).
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

// adjust_varargs is adjust_varargs (ldo.c 5.3.4).
func adjust_varargs(L *State, p *Proto, actual int) int {
	nfixargs := int(p.numparams)
	fixed := L.top - actual /* first fixed argument */
	base := L.top           /* final position of first argument */
	i := 0
	for ; i < nfixargs && i < actual; i++ {
		L.stack[L.top] = L.stack[fixed+i]
		L.top++
		setnilvalue(&L.stack[fixed+i]) /* erase original copy (for GC) */
	}
	for ; i < nfixargs; i++ {
		setnilvalue(&L.stack[L.top])
		L.top++
	}
	return base
}

// adjustVarargs keeps the old exported name used elsewhere.
func adjustVarargs(L *State, p *Proto, actual int) int {
	return adjust_varargs(L, p, actual)
}

// tryfuncTM is tryfuncTM (ldo.c): !ttisfunction(tm) → typeerror "call".
func tryfuncTM(L *State, funcIdx int) {
	tm := luaT_gettmbyobj(L, &L.stack[funcIdx], TM_CALL)
	if !ttisfunction(tm) {
		luaG_typeerror(L, &L.stack[funcIdx], "call")
		return
	}
	/* Open a hole inside the stack at 'func' */
	for p := L.top; p > funcIdx; p-- {
		L.stack[p] = L.stack[p-1]
	}
	L.top++ /* slot ensured by caller */
	L.stack[funcIdx] = *tm /* tag method is the new function to be called */
}

// moveresults is moveresults (ldo.c). Returns 0 iff wanted == LUA_MULTRET.
func moveresults(L *State, firstResult, res, nres, wanted int) int {
	switch wanted { /* handle typical cases separately */
	case 0: /* nothing to move */
		/* fall through to set top */
	case 1: /* one result needed */
		if nres == 0 { /* no results? */
			setnilvalue(&L.stack[res]) /* adjust with nil */
		} else {
			L.stack[res] = L.stack[firstResult] /* move it to proper place */
		}
	case LUA_MULTRET:
		for i := 0; i < nres; i++ { /* move all results to correct place */
			L.stack[res+i] = L.stack[firstResult+i]
		}
		L.top = res + nres
		return 0 /* wanted == LUA_MULTRET */
	default:
		if wanted <= nres { /* enough results? */
			for i := 0; i < wanted; i++ {
				L.stack[res+i] = L.stack[firstResult+i]
			}
		} else { /* not enough results; use all of them plus nils */
			i := 0
			for ; i < nres; i++ {
				L.stack[res+i] = L.stack[firstResult+i]
			}
			for ; i < wanted; i++ {
				setnilvalue(&L.stack[res+i])
			}
		}
	}
	L.top = res + wanted /* top points after the last result */
	return 1
}

// luaD_poscall is luaD_poscall (ldo.c 5.3.4).
// Returns 0 iff the call wanted multiple (variable number of) results.
func luaD_poscall(L *State, ci *CallInfo, firstResult, nres int) int {
	wanted := ci.nresults
	if L.hookmask&(dbMaskRet|dbMaskLine) != 0 {
		if L.hookmask&dbMaskRet != 0 {
			fr := firstResult /* hook may change stack; indices stay stable */
			luaD_hook(L, hookRet, -1)
			firstResult = fr
		}
		if prev := ci.prev; prev != nil && isLua(prev) {
			/* C stores Instruction* (= next pc); Go's oldpc is pcRel-style
			   (index of last traced insn). pcRel(savedpc) == savedpc-1. */
			if prev.savedpc > 0 {
				L.oldpc = prev.savedpc - 1
			} else {
				L.oldpc = 0
			}
		}
	}
	res := ci.funcIdx /* res == final position of 1st result */
	L.ci = ci.prev    /* back to caller */
	/* move results to proper place */
	return moveresults(L, firstResult, res, nres, wanted)
}

// next_ci is next_ci (ldo.c).
func next_ci(L *State) *CallInfo {
	if L.ci != nil && L.ci.next != nil {
		L.ci = L.ci.next
		return L.ci
	}
	return L.extendCI()
}

// prepCallInfo fills a CallInfo after next_ci (helper; fields match precall).
func prepCallInfo(L *State, funcIdx, nret int, mask uint16, top int) *CallInfo {
	ci := next_ci(L)
	ci.funcIdx = funcIdx
	ci.base = funcIdx + 1
	ci.nresults = nret
	ci.callstatus = mask
	ci.top = top
	ci.savedpc = 0
	ci.nExtraArgs = 0
	ci.goCont = nil
	ci.migrateFunc = nil
	ci.cK = nil
	ci.ypcall = false
	return ci
}

// precallC runs a Go/C function frame (ldo.c Cfunc path) with goroutine adaptations.
func precallC(L *State, funcIdx, nresults int, f GoFunc) int {
	L.checkStack(LUA_MINSTACK) /* ensure minimum stack size */
	ci := prepCallInfo(L, funcIdx, nresults, 0, L.top+LUA_MINSTACK)
	if L.hookmask&dbMaskCall != 0 {
		luaD_hook(L, hookCall, -1)
	}
	/* Goroutine adaptation: migrate off the resumer before entering a
	   non-yield Go function from a synchronous coroutine in the VM. */
	if L.co != nil && L.co.sync && L.inVM &&
		funcptr(f) != funcptr(coYield) {
		ci.migrateFunc = f
		panic(&migrateAbort{})
	}
	n := f(L) /* do the actual call */
	if L.fastYield {
		/* synchronous yield: the frame stays, Resume finishes it */
		return 1
	}
	luaD_poscall(L, ci, L.top-n, n)
	return 1
}

// luaD_precall is luaD_precall (ldo.c 5.3.4).
// Returns 1 iff the function has been executed (C/Go function), 0 if a
// Lua frame is ready for luaV_execute.
func luaD_precall(L *State, funcIdx, nresults int) int {
retry:
	fv := &L.stack[funcIdx]
	switch ttypetag(fv) {
	case LUA_VCCL: /* C closure — C: LUA_TCCL */
		return precallC(L, funcIdx, nresults, clCvalue(fv).f)
	case LUA_VLCF: /* light C function — C: LUA_TLCF */
		return precallC(L, funcIdx, nresults, fvalue(fv))
	case LUA_VLCL: /* Lua function — C: LUA_TLCL */
		p := clLvalue(fv).p
		n := L.top - funcIdx - 1 /* number of real arguments */
		fsize := int(p.maxstacksize) /* frame size */
		L.checkStack(fsize)
		var base int
		nExtra := 0
		if p.isVararg != 0 {
			nExtra = n - int(p.numparams)
			if nExtra < 0 {
				nExtra = 0
			}
			base = adjust_varargs(L, p, n)
		} else { /* non vararg function */
			for ; n < int(p.numparams); n++ {
				setnilvalue(&L.stack[L.top])
				L.top++
			}
			base = funcIdx + 1
		}
		ci := next_ci(L) /* now 'enter' new function */
		ci.nresults = nresults
		ci.funcIdx = funcIdx
		ci.base = base
		L.top = base + fsize
		ci.top = L.top
		ci.savedpc = 0 /* starting point */
		ci.callstatus = CIST_LUA
		ci.nExtraArgs = nExtra /* Go bookkeeping for vararg debug; not in 5.3.4 CI */
		ci.goCont = nil
		ci.migrateFunc = nil
		if L.hookmask&dbMaskCall != 0 {
			callhook(L, ci)
		}
		return 0
	default: /* not a function */
		L.checkStack(1) /* ensure space for metamethod */
		tryfuncTM(L, funcIdx)
		goto retry /* now it must be a function */
	}
}

// stackerror is stackerror (ldo.c).
func stackerror(L *State) {
	if L.nCcalls == LUAI_MAXCCALLS {
		luaG_runerror(L, "C stack overflow")
	} else if L.nCcalls >= LUAI_MAXCCALLS+(LUAI_MAXCCALLS>>3) {
		luaD_throw(L, LUA_ERRERR) /* error while handing stack error */
	}
}

// luaD_call is luaD_call (ldo.c 5.3.4).
func luaD_call(L *State, funcIdx, nResults int) {
	L.nCcalls++
	if L.nCcalls >= LUAI_MAXCCALLS {
		stackerror(L)
	}
	if luaD_precall(L, funcIdx, nResults) == 0 { /* is a Lua function? */
		L.ci.callstatus |= CIST_FRESH /* fresh invocation of luaV_execute */
		luaV_execute(L, L.ci)        /* call it */
	}
	L.nCcalls--
}

// luaD_callnoyield is luaD_callnoyield (ldo.c 5.3.4): bumps nny around the call.
func luaD_callnoyield(L *State, funcIdx, nResults int) {
	L.nny++
	luaD_call(L, funcIdx, nResults)
	L.nny--
}

// luaD_pcall is luaD_pcall (ldo.c 5.3.4).
func luaD_pcall(L *State, f func(), oldTop, ef int) int {
	oldCI := L.ci
	oldAllowHooks := L.allowhook
	oldNny := L.nny
	oldErrFunc := L.errfunc
	L.errfunc = ef
	status := luaD_rawrunprotected(L, f)
	if status != LUA_OK { /* an error occurred? */
		luaF_close(L, oldTop) /* close possible pending closures */
		seterrorobj(L, status, oldTop)
		L.ci = oldCI
		L.allowhook = oldAllowHooks
		L.nny = oldNny
		luaD_shrinkstack(L)
	}
	L.errfunc = oldErrFunc
	return status
}
