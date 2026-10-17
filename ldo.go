// Translation of ldo.h / ldo.c (PUC-Lua 5.4).
//
// setjmp/longjmp become panic/recover (see State.rawRunProtected in
// lstate.go) and the allocator is gone.
package lua

const (
	LUAI_MAXCCALLS = 200
)

// CIST flags for fresh frames are in lstate.go.

// tryfuncTM puts the __call metamethod below the original function so
// that the call can proceed (ldo.c).
func tryfuncTM(L *State, funcIdx int) {
	L.checkStack(1)
	tm := luaT_gettmbyobj(L, &L.stack[funcIdx], TM_CALL)
	if ttisnil(tm) {
		luaG_callerror(L, &L.stack[funcIdx])
		return
	}
	for p := L.top; p > funcIdx; p-- { /* open space for metamethod */
		L.stack[p] = L.stack[p-1]
	}
	L.top++
	L.stack[funcIdx] = *tm
}

// moveresults moves nres results at the top of the stack to 'res'
// (ldo.c).
func moveresults(L *State, res, nres, wanted int) {
	switch wanted {
	case 0: /* no values needed */
		L.top = res
		return
	case 1: /* one value needed */
		if nres == 0 {
			setnilvalue(&L.stack[res]) /* adjust with nil */
		} else {
			L.stack[res] = L.stack[L.top-nres] /* move it to proper place */
		}
		L.top = res + 1
		return
	case LUA_MULTRET:
		wanted = nres /* we want all results */
	}
	/* generic case */
	firstresult := L.top - nres
	if nres > wanted {
		nres = wanted
	}
	i := 0
	for ; i < nres; i++ { /* move all results to correct place */
		L.stack[res+i] = L.stack[firstresult+i]
	}
	for ; i < wanted; i++ { /* complete wanted number of results */
		setnilvalue(&L.stack[res+i])
	}
	L.top = res + wanted
}

// luaD_poscall finishes a function call (ldo.c).
func luaD_poscall(L *State, ci *CallInfo, nres int) {
	wanted := ci.nresults
	if hastocloseCfunc(wanted) { /* to-be-closed variables pending? */
		/* C does this inside 'moveresults', before the results are moved
		   and with the return hook saved until after the '__close's --
		   a hook cannot run with a close still pending. */
		wanted = decodeNresults(wanted)
		ci.nresults = wanted
		luaF_close(L, ci.funcIdx+1, closeKTOP, true)
		if L.hookmask != 0 {
			rethook(L, ci, nres)
		}
		moveresults(L, ci.funcIdx, nres, wanted)
		L.ci = ci.prev
		return
	}
	if L.hookmask != 0 {
		rethook(L, ci, nres)
	}
	moveresults(L, ci.funcIdx, nres, wanted)
	L.ci = ci.prev
}

// hastocloseCfunc, codeNresults and decodeNresults are lapi.h's: whether
// a C frame has to-be-closed variables pending, which PUC-Lua encodes in
// the sign of the frame's 'nresults'.
func hastocloseCfunc(n int) bool { return n < LUA_MULTRET }
func codeNresults(n int) int     { return -n - 3 }
func decodeNresults(n int) int   { return -n - 3 }

// prepCallInfo prepares the CallInfo for a new call (ldo.c).
func prepCallInfo(L *State, funcIdx, nret int, mask uint16, top int) *CallInfo {
	var ci *CallInfo
	if L.ci != nil && L.ci.next != nil {
		ci = L.ci.next
		ci.prev = L.ci
		L.ci = ci
	} else {
		ci = L.extendCI()
	}
	ci.funcIdx = funcIdx
	ci.nresults = nret
	ci.callstatus = mask
	ci.top = top
	ci.savedpc = 0
	ci.nExtraArgs = 0
	return ci
}

// precallC calls a Go function (ldo.c).
func precallC(L *State, funcIdx, nresults int, f GoFunc) int {
	L.checkStack(LUA_MINSTACK)
	ci := prepCallInfo(L, funcIdx, nresults, CIST_C, L.top+LUA_MINSTACK)
	if L.hookmask&dbMaskCall != 0 { /* is call hook on? */
		narg := L.top - funcIdx - 1
		luaD_hook(L, hookCall, -1, 1, narg)
	}
	n := f(L)
	luaD_poscall(L, ci, n)
	return n
}

// ---------------------------------------------------------------------
// hooks (ldo.c)
// ---------------------------------------------------------------------

// luaD_hook calls the hook for the given event, making sure there is one
// to be called (ldo.c).
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
//
// It is called whenever 'hookmask' is not zero, so it checks whether call
// hooks are active.
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
// corrects 'oldpc' (ldo.c). The correction is needed by the line hook, so
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
		L.oldpc = prev.savedpc - 1 /* set 'oldpc' (pcRel) */
	}
}

// setTraps marks every Lua frame as needing a hook check (ldebug.c).
//
// 'lua_sethook' sets the mark so that a hook installed in the middle of a
// call is seen by the interpreter loop of the running function.
func setTraps(L *State) {
	for ci := L.ci; ci != nil; ci = ci.prev {
		if isLua(ci) {
			ci.trap = true
		}
	}
}

// luaD_precall prepares a call; it returns the CallInfo when the callee
// is a Lua function (the caller must then run luaV_execute), or nil when
// the callee was a Go function and the results are already on the stack.
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
		L.checkStack(fsize)
		ci := prepCallInfo(L, funcIdx, nresults, 0, funcIdx+1+fsize)
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

// luaD_pretailcall prepares a tail call (ldo.c).
//
// It returns -1 when the callee is a Lua function: unlike a regular
// call, no new CallInfo is created -- the frame of the caller is
// reused in place (that is what makes the call a *proper* tail call).
// It returns the number of results when the callee was a Go function,
// in which case the caller must still be finished with luaD_poscall.
//
// Allocating an extra CallInfo here (as a regular call would) corrupts
// the 'prev' links of the call chain, because L.ci is then left behind
// pointing at the old frame while ci.next is already the new one.
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

// ccall is the common part of luaD_call / luaD_callnoyield (ldo.c).
// luaE_incCstack counts one more C level and raises when there are none
// left (lstate.c).  Only the lower 16 bits of 'nCcalls' count levels; the
// upper 16 only say "not yieldable" and must not look like depth.
func luaE_incCstack(L *State) {
	L.nCcalls++
	if L.nCcalls&0xffff >= LUAI_MAXCCALLS {
		luaE_checkcstack(L)
	}
}

func ccall(L *State, funcIdx, nresults int, inc int) {
	L.nCcalls += uint32(inc)
	if L.nCcalls&0xffff >= LUAI_MAXCCALLS {
		luaE_checkcstack(L)
	}
	if ci := luaD_precall(L, funcIdx, nresults); ci != nil {
		ci.callstatus = CIST_FRESH /* mark that it is a "fresh" execute */
		luaV_execute(L, ci)
	}
	L.nCcalls -= uint32(inc)
}

func luaD_call(L *State, funcIdx, nresults int) { ccall(L, funcIdx, nresults, 1) }

// nyci is the increment used by 'luaD_callnoyield': it also bumps the
// upper 16 bits of 'nCcalls', which is what makes 'yieldable' false
// inside the call (lstate.h).
const nyci = 0x10000 | 1

func luaD_callnoyield(L *State, funcIdx, nresults int) { ccall(L, funcIdx, nresults, nyci) }

// luaD_errerr is 'luaD_errerr' (ldo.c): it raises an error while the
// message handler is running. It throws directly instead of going through
// 'luaG_errormsg', which is what stops a handler that keeps raising from
// recursing forever.
func luaD_errerr(L *State) {
	ts := luaS_new(L, "error in error handling")
	if L.top >= len(L.stack) {
		L.reallocStack(len(L.stack) + EXTRA_STACK) /* assume EXTRA_STACK */
	}
	setsvalue(&L.stack[L.top], ts)
	L.top++
	luaD_throw(L, LUA_ERRERR)
}

// luaE_checkcstack raises an error when the C stack is exhausted
// (lstate.c). A depth of exactly LUAI_MAXCCALLS reports a plain overflow;
// going well past it means the overflow handling itself failed, which is
// reported as an error in error handling.
func luaE_checkcstack(L *State) {
	calls := L.nCcalls & 0xffff
	if calls == LUAI_MAXCCALLS {
		luaG_runerror(L, "C stack overflow")
	} else if calls >= LUAI_MAXCCALLS/10*11 {
		luaD_errerr(L) /* error while handling stack error */
	}
}

// closeP carries the arguments of 'closepaux' (ldo.c).
type closeP struct {
	level  int
	status int
}

// luaD_closeprotected closes to-be-closed variables, running the close
// methods under a protected call (ldo.c).
//
// A '__close' that raises must not escape: C keeps closing the remaining
// variables, remembering the newest error, and only then hands that error
// to the enclosing protected call. Closing unprotected instead makes an
// 'error' inside '__close' bypass the 'pcall' around the function that
// declared the variable.
func luaD_closeprotected(L *State, level, status int) int {
	oldCI := L.ci
	oldAllowHooks := L.allowhook
	for { /* keep closing upvalues until no more errors */
		pcl := &closeP{level: level, status: status}
		st := L.rawRunProtected(func() { luaF_close(L, pcl.level, pcl.status, true) })
		if st == LUA_OK { /* no more errors? */
			return pcl.status
		}
		/* an error occurred; restore saved state and repeat */
		L.ci = oldCI
		L.allowhook = oldAllowHooks
		status = st
	}
}

// luaD_seterrorobj puts the error object at 'oldtop' (ldo.c).
func luaD_seterrorobj(L *State, errcode, oldtop int) {
	switch errcode {
	case LUA_ERRMEM:
		var v Value
		setsvalue(&v, L.g.memErrMsg)
		L.stack[oldtop] = v
	case LUA_ERRERR:
		var v Value
		setsvalue(&v, luaS_new(L, "error in error handling"))
		L.stack[oldtop] = v
	case LUA_OK: /* special case only for closing upvalues */
		setnilvalue(&L.stack[oldtop]) /* no error message */
	default:
		L.stack[oldtop] = L.stack[L.top-1]
	}
	L.top = oldtop + 1
}

// luaD_pcall runs f in protected mode (ldo.c).
func luaD_pcall(L *State, f func(), oldTop, ef int) int {
	oldCI := L.ci
	oldAllowHooks := L.allowhook
	oldErrFunc := L.errfunc
	L.errfunc = ef
	status := L.rawRunProtected(f)
	if status != LUA_OK { /* an error occurred? */
		L.ci = oldCI
		L.allowhook = oldAllowHooks
		status = luaD_closeprotected(L, oldTop, status)
		luaD_seterrorobj(L, status, oldTop)
		L.shrinkStack()
	}
	L.errfunc = oldErrFunc
	return status
}

// luaD_throw raises an error with the given status.
// luaD_throw raises an error with the given status (ldo.c).
//
// In C the error object is left on the top of the stack of the throwing
// thread and picked up later by luaD_seterrorobj; because a Go panic
// unwinds without any chance to inspect the stack, the object is copied
// into the panic value here.
func luaD_throw(L *State, status int) {
	var v Value
	if L.top > 0 {
		v = L.stack[L.top-1]
	} else {
		setnilvalue(&v)
	}
	panic(&luaError{status: status, value: v})
}
