// Translation of lcorolib.c (PUC-Lua 5.4.9): the coroutine library.
//
// Intentional Go specialization (goroutine coroutines; see lstate.go):
//
//  1. (*State).Yield is a blocking channel handshake instead of a longjmp.
//     C throws out of the coroutine and lua_resume picks it up again
//     later; ported straight, that would unwind the goroutine and the
//     next resume would run the body again from the beginning. Yield
//     instead hands the yielded values to the resumer and then waits
//     for the next resume request, which keeps the goroutine (and
//     therefore the Lua stack and the saved program counter) alive at
//     the yield point — exactly the "a metamethod that yields simply
//     blocks the coroutine's goroutine" behaviour described at the top
//     of lvm.go. It is also what makes a C continuation possible: the
//     C frame that asked for one is still there when the resume comes.
//
//  2. luaB_yield (coYield) may take a sync fast-path (prepareFastYield /
//     fastYieldNow) when the coroutine is still on the resumer's
//     goroutine; otherwise it parks via Yield. Visible Lua yield/resume
//     value protocol is unchanged (yield args become resume results and
//     vice versa).
//
//  3. lua_closethread (lstate.c) is coCloseThread here: it asks the
//     coroutine goroutine to unwind itself through the same request
//     channel (or runs resetThreadStatus when the goroutine is already
//     gone). Used by luaB_close and by auxwrap's to-be-closed path (5.4).
package core

// coGetCo is getco (lcorolib.c).
func coGetCo(L *State) *State {
	co := L.ToThread(1)
	L.ArgExpected(co != nil, 1, "thread") /* luaL_argexpected */
	return co
}

// coCanResume reports whether 'co' can be resumed. Go-only preflight:
// C's auxresume always lua_xmove's args then lets lua_resume /
// resume_error scrub them; Go's resumeError (lstate.go) does not take
// narg and must not destroy pending to-be-closed slots, so it leaves
// args on co. Preflighting with the same PUC messages keeps a refused
// resume from depositing args onto a running or dead coroutine.
func coCanResume(co *State) (bool, string) {
	if co == nil || co.isMain {
		return false, "cannot resume non-suspended coroutine"
	}
	switch co.Status() {
	case LUA_YIELD:
		return true, ""
	case LUA_OK:
		if co.dead {
			return false, "cannot resume dead coroutine"
		}
		return false, "cannot resume non-suspended coroutine" /* it is running */
	default: /* some error occurred */
		return false, "cannot resume dead coroutine"
	}
}

/*
** Resumes a coroutine. Returns the number of results for non-error
** cases or -1 for errors (the error message is then on top of L).
** (lcorolib.c auxresume)
 */
func coAuxResume(L *State, co *State, narg int) int {
	/* Go: coCanResume — see comment on that helper. C has no preflight. */
	if ok, msg := coCanResume(co); !ok {
		L.SetTop(L.GetTop() - narg) /* remove the arguments */
		L.PushString(msg)
		return -1 /* error flag */
	}
	if !co.CheckStack(narg) {
		/* C leaves args on L; return count still hides them from Lua. */
		L.PushString("too many arguments to resume")
		return -1 /* error flag */
	}
	co.XMove(L, narg) /* C: lua_xmove(L, co, narg) */
	status := co.Resume(L, narg)
	if status == LUA_OK || status == LUA_YIELD {
		/* C: lua_resume(..., &nres). Go Resume has no out-param; the
		   C export uses len(co.co.vals) for the same count (GetTop can
		   include the base CI slot after an OK return). */
		nres := len(co.co.vals)
		if !L.CheckStack(nres + 1) {
			co.Pop(nres) /* remove results anyway */
			L.PushString("too many results to resume")
			return -1 /* error flag */
		}
		L.XMove(co, nres) /* C: lua_xmove(co, L, nres) — move yielded values */
		return nres
	}
	/* error: C's lua_resume does luaD_seterrorobj(L, status, L->top),
	   which leaves an extra copy on co; auxresume then lua_xmove(co, L, 1)
	   so one copy remains for lua_closethread. Go Resume leaves a single
	   copy — duplicate before xmove. When Resume left nothing on the stack
	   and no error object in co.vals (async empty-vals error), report dead. */
	if co.top <= 0 {
		L.PushString("cannot resume dead coroutine")
		return -1 /* error flag */
	}
	co.checkStack(1)
	co.stack[co.top] = co.stack[co.top-1]
	co.top++
	L.XMove(co, 1) /* C: lua_xmove(co, L, 1) — move error message */
	return -1      /* error flag */
}

// coResume is luaB_coresume (lcorolib.c).
func coResume(L *State) int {
	co := coGetCo(L)
	r := coAuxResume(L, co, L.GetTop()-1)
	if r < 0 {
		L.PushBoolean(false)
		L.Insert(-2)
		return 2 /* return false + error message */
	}
	L.PushBoolean(true)
	L.Insert(-(r + 1))
	return r + 1 /* return true + 'resume' returns */
}

// coAuxWrap is luaB_auxwrap (lcorolib.c 5.4): on error, close tbc vars.
func coAuxWrap(L *State) int {
	co := L.ToThread(UpvalueIndex(1))
	r := coAuxResume(L, co, L.GetTop())
	if r < 0 { /* error? */
		stat := co.Status()
		if stat != LUA_OK && stat != LUA_YIELD { /* error in the coroutine? */
			/* C: stat = lua_closethread(co, L); lua_xmove(co, L, 1).
			   auxresume left a copy of the error on co (see above).
			   resetThreadStatus is luaE_resetthread; nCcalls like
			   lua_closethread. Preserve returned status for ERRMEM. */
			co.nCcalls = uint32(getCcalls(L))
			stat = co.resetThreadStatus(stat)
			/* C asserts stat != LUA_OK after closethread on an erroring
			   coroutine; resetThreadStatus preserves a non-OK status when
			   __close does not replace it, so the OK→ERRRUN guard is omitted. */
			if co.top > 0 {
				L.XMove(co, 1) /* move error message to the caller */
			}
		}
		if stat != LUA_ERRMEM && /* not a memory error and ... */
			L.Type(-1) == LUA_TSTRING { /* ... error object is a string? */
			L.Where2(1) /* add extra info, if available */
			L.Insert(-2)
			L.Concat(2)
		}
		return L.Error() /* propagate error */
	}
	return r
}

// coCreate is luaB_cocreate (lcorolib.c).
func coCreate(L *State) int {
	L.CheckType(1, LUA_TFUNCTION)
	NL := L.NewThread() /* pushes the new thread on L */
	L.PushValue(1)      /* move function to top */
	NL.XMove(L, 1)      /* move function from L to NL */
	return 1
}

// coWrap is luaB_cowrap (lcorolib.c).
func coWrap(L *State) int {
	coCreate(L)
	L.PushGoClosure(coAuxWrap, 1) /* lua_pushcclosure(L, luaB_auxwrap, 1) */
	return 1
}

/*
** coYield is luaB_yield (lcorolib.c). C: return lua_yield(L, lua_gettop(L)).
** Sync fast-path is intentional goroutine adaptation (see package comment).
 */
func coYield(L *State) int {
	/* A pure Lua yield on a synchronous coroutine unwinds to Resume.
	   The goroutine park stays for a coroutine that has been promoted
	   (a Go body, or anything that has called into C), because there
	   the yield has to return into the frame that called it. */
	if L.co != nil && L.co.sync {
		/* Outermost interpreter: return through the call. Deeper than
		   that, Go frames sit above this one and have to be unwound. */
		if L.vmNest == 1 {
			L.prepareFastYield(L.GetTop())
			return 0
		}
		L.fastYieldNow(L.GetTop())
	}
	return L.Yield(L.GetTop())
}

/* status name indices — lcorolib.c COS_RUN / COS_DEAD / COS_YIELD / COS_NORM */
const (
	cosRun   = iota /* COS_RUN */
	cosDead         /* COS_DEAD */
	cosYield        /* COS_YIELD */
	cosNorm         /* COS_NORM */
)

/* statname[] (lcorolib.c) */
var coStatNames = []string{"running", "dead", "suspended", "normal"}

// coAuxStatus is auxstatus (lcorolib.c).
func coAuxStatus(L *State, co *State) int {
	if L == co {
		return cosRun
	}
	switch co.Status() {
	case LUA_YIELD:
		return cosYield
	case LUA_OK:
		var ar dbAr
		if co.dead { /* Go-only bookkeeping; finished OK agrees with C dead */
			return cosDead
		}
		if co.GetStack(0, &ar) { /* does it have frames? */
			return cosNorm /* it is running */
		} else if co.GetTop() == 0 {
			return cosDead
		}
		return cosYield /* initial state */
	default: /* some error occurred */
		return cosDead
	}
}

// coStatus is luaB_costatus (lcorolib.c).
func coStatus(L *State) int {
	co := coGetCo(L)
	L.PushString(coStatNames[coAuxStatus(L, co)])
	return 1
}

// coYieldable is luaB_yieldable (lcorolib.c 5.4): with no argument it
// asks about the running thread, otherwise about the given thread.
// Answer is lua_isyieldable (upper 16 bits of nCcalls clear).
func coYieldable(L *State) int {
	co := L
	if !L.IsNone(1) {
		co = coGetCo(L)
	}
	L.PushBoolean(co.IsYieldable())
	return 1
}

// coRunning is luaB_corunning (lcorolib.c).
func coRunning(L *State) int {
	ismain := L.PushThread()
	L.PushBoolean(ismain)
	return 2
}

// coPushValue pushes a raw Value (no lua_pushvalue for *Value in this port).
func coPushValue(L *State, v Value) {
	L.checkStack(1)
	L.stack[L.top] = v
	L.top++
}

// coCloseThread is the port's lua_closethread(co, from) (lstate.c): it
// asks the coroutine goroutine to unwind itself. A coroutine that has
// already finished has no goroutine left to talk to, and is simply
// reported as closed (which is what C does too via luaE_resetthread).
//
// Like C, the thread being closed inherits the C-call depth of the
// closer (getCcalls(from)), so that a chain of 'coroutine.close' calls
// is reported as a C stack overflow instead of recursing forever.
//
// Returns (ok, errval): ok means status LUA_OK; otherwise errval is the
// error object (C leaves it on co for lua_xmove).
func coCloseThread(co, from *State) (bool, Value) {
	if co.isMain {
		return false, strValue(co, "cannot close a running coroutine")
	}
	if from != nil {
		co.nCcalls = uint32(getCcalls(from))
	} else {
		co.nCcalls = 0
	}
	if co.co != nil && co.co.sync && !co.co.live && !co.dead {
		st := co.unwindForClose()
		co.dead = true
		if st == LUA_OK {
			return true, Value{}
		}
		/* unwindForClose always leaves the error in co.vals on failure. */
		if len(co.co.vals) > 0 {
			return false, co.co.vals[0]
		}
		return false, strValue(co, "error closing coroutine")
	}
	if co.dead || co.co == nil {
		/* The coroutine already terminated, so there is no goroutine
		   left to unwind. C runs luaE_resetthread(co, co->status), which
		   closes any pending variable and reports the thread's status:
		   a coroutine that ended with an error reports that error once
		   again (false + message), and the report is then consumed —
		   the second close returns true.

		   Put lastErr back on the stack first so seterrorobj / __close
		   see the body error (with its source:line). A raising __close
		   still replaces it. */
		if co.hasErr && !ttisnil(&co.lastErr) {
			co.checkStack(1)
			if co.top < 1 {
				setnilvalue(&co.stack[0])
				co.top = 1
			}
			co.stack[co.top] = co.lastErr
			co.top++
		}
		st := co.resetThreadStatus(int(co.status))
		if st != LUA_OK {
			co.hasErr = false
			if co.top > 1 && !ttisnil(&co.stack[1]) {
				return false, co.stack[1]
			}
			if !ttisnil(&co.lastErr) {
				return false, co.lastErr
			}
			return false, strValue(co, "error closing coroutine")
		}
		co.hasErr = false
		return true, Value{} /* already closed */
	}
	var resp resumeResp
	select {
	case <-co.co.done:
		co.dead = true
		return true, Value{}
	case co.co.req <- resumeReq{kind: reqClose}:
		select {
		case resp = <-co.co.resp:
		case <-co.co.done:
			co.dead = true
			return true, Value{}
		}
	}
	co.dead = true
	if resp.status == LUA_OK {
		return true, Value{}
	}
	if len(resp.vals) > 0 {
		return false, resp.vals[0]
	}
	return false, strValue(co, "error closing coroutine")
}

// coClose is luaB_close (lcorolib.c) — coroutine.close (5.4).
func coClose(L *State) int {
	co := coGetCo(L)
	status := coAuxStatus(L, co)
	switch status {
	case cosDead, cosYield:
		ok, errval := coCloseThread(co, L) /* lua_closethread(co, L) */
		if ok {
			L.PushBoolean(true)
			return 1
		}
		L.PushBoolean(false)
		coPushValue(L, errval) /* C: lua_xmove(co, L, 1) */
		return 2
	default: /* normal or running coroutine */
		return L.Error2("cannot close a %s coroutine", coStatNames[status])
	}
}

/* co_funcs[] (lcorolib.c) */
var coFuncs = []Reg{
	{Name: "create", Func: coCreate},
	{Name: "resume", Func: coResume},
	{Name: "running", Func: coRunning},
	{Name: "status", Func: coStatus},
	{Name: "wrap", Func: coWrap},
	{Name: "yield", Func: coYield},
	{Name: "isyieldable", Func: coYieldable},
	{Name: "close", Func: coClose},
}

// OpenCoroutine opens the coroutine library (luaopen_coroutine).
func OpenCoroutine(L *State) int {
	L.NewLib(coFuncs)
	return 1
}
