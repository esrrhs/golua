// Translation of lcorolib.c (PUC-Lua 5.4): the coroutine library.
//
// Two things differ from the C source, both forced by the way this port
// implements coroutines (see lstate.go: each coroutine runs on its own
// goroutine):
//
//  1. 'lua_yield' is a blocking channel handshake instead of a longjmp.
//     C throws out of the coroutine and 'lua_resume' picks it up again
//     later; ported straight, that would unwind the goroutine and the
//     next resume would run the body again from the beginning.  (*State).
//     Yield instead hands the yielded values to the resumer and then
//     waits for the next resume request, which keeps the goroutine (and
//     therefore the Lua stack and the saved program counter) alive at
//     the yield point -- exactly the "a metamethod that yields simply
//     blocks the coroutine's goroutine" behaviour described at the top
//     of lvm.go.  It is also what makes a C continuation possible: the
//     C frame that asked for one is still there when the resume comes.
//
//  2. There is no lua_closethread; coClose asks the coroutine goroutine
//     to unwind itself through the same request channel.
package core

// getco gets the coroutine in the first argument (lcorolib.c getco).
func coGetCo(L *State) *State {
	co := L.ToThread(1)
	if co == nil {
		L.CheckType(1, LUA_TTHREAD) /* raises "thread expected, got ..." */
	}
	return co
}

// coCanResume reports whether 'co' can be resumed; it mirrors the checks
// at the top of (*State).Resume, which (unlike C's lua_resume) does not
// push an error message when it refuses.
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
 */
func coAuxResume(L *State, co *State, narg int) int {
	if ok, msg := coCanResume(co); !ok {
		L.SetTop(L.GetTop() - narg) /* remove the arguments */
		L.PushString(msg)
		return -1 /* error flag */
	}
	if !co.CheckStack(narg) {
		L.SetTop(L.GetTop() - narg)
		L.PushString("too many arguments to resume")
		return -1 /* error flag */
	}
	base := L.GetTop() - narg /* stack level below the arguments */
	co.XMove(L, narg)         /* C's auxresume: lua_xmove(L, co, narg) */
	status := co.Resume(L, narg)
	if status == LUA_OK || status == LUA_YIELD {
		nres := L.GetTop() - base /* values moved back from the coroutine */
		if !L.CheckStack(1) {     /* room for the boolean below them */
			L.SetTop(base) /* remove results anyway */
			L.PushString("too many results to resume")
			return -1 /* error flag */
		}
		return nres
	}
	/* error: Resume should have moved the error message onto L */
	if L.GetTop() == base {
		L.PushString("cannot resume dead coroutine")
	} else if ttisnil(&L.stack[L.top-1]) {
		/* ldo.go's luaD_throw panics without carrying the error object,
		   so the message never reaches (*State).Resume and the slot is
		   left nil.  C's lua_resume picks it up from the top of the
		   coroutine's stack with luaD_seterrorobj, which is still
		   intact here -- do the same.  Delete this branch once
		   luaD_throw stores L.stack[L.top-1] in the panic value. */
		if co.top > 0 {
			L.stack[L.top-1] = co.stack[co.top-1]
		}
	}
	return -1 /* error flag */
}

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

func coAuxWrap(L *State) int {
	co := L.ToThread(UpvalueIndex(1))
	r := coAuxResume(L, co, L.GetTop())
	if r < 0 { /* error? */
		stat := co.Status()
		if stat != LUA_OK && stat != LUA_YIELD { /* error in the coroutine? */
			/* C calls lua_closethread(co, L) here: that is what closes
			   the to-be-closed variables of the dead coroutine, and it
			   also hands the C-call depth of L down to 'co'.  A
			   '__close' that raises replaces the original error, and
			   C moves *that* message (in slot 1 of 'co') to the
			   caller. */
			co.nCcalls = L.nCcalls & 0xffff
			st := co.resetThreadStatus(int(stat))
			if st != LUA_OK {
				L.stack[L.top-1] = co.stack[1] /* move error message */
			}
			stat = LUA_ERRRUN
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

func coCreate(L *State) int {
	L.CheckType(1, LUA_TFUNCTION)
	co := L.NewThread() /* pushes the new thread on L */
	L.PushValue(1)      /* move function to top */
	/* The coroutine body must end up in slot 0 of the coroutine stack
	   (that is where coRun looks for it), so make it the first slot
	   before moving the value. */
	co.top = 0
	co.XMove(L, 1)
	co.prevFunc = co.stack[0] /* so that it also survives a reset */
	return 1
}

func coWrap(L *State) int {
	coCreate(L)
	L.PushGoClosure(coAuxWrap, 1)
	return 1
}

/*
** coYield is luaB_yield (and, as explained above, the port's
** lua_yield): it takes the values on top of the stack, hands them to
** the resumer and waits for the next resume, whose arguments become the
** results of the yield.
 */
func coYield(L *State) int {
	/* the whole of it is now (*State).Yield: the park-and-wait
	   handshake, the continuation, and the two error messages */
	return L.Yield(L.GetTop())
}

// status names (lcorolib.c COS_RUN, COS_DEAD, COS_YIELD, COS_NORM)
const (
	cosRun = iota
	cosDead
	cosYield
	cosNorm
)

var coStatNames = []string{"running", "dead", "suspended", "normal"}

func coAuxStatus(L *State, co *State) int {
	if L == co {
		return cosRun
	}
	switch co.Status() {
	case LUA_YIELD:
		return cosYield
	case LUA_OK:
		if co.dead { /* does it have frames? the port records this in 'dead' */
			return cosDead
		}
		if co.ci != &co.baseCI { /* lua_getstack(co, 0): it is running */
			return cosNorm
		}
		if co.GetTop() == 0 {
			return cosDead
		}
		return cosYield /* initial state */
	default: /* some error occurred */
		return cosDead
	}
}

func coStatus(L *State) int {
	co := coGetCo(L)
	L.PushString(coStatNames[coAuxStatus(L, co)])
	return 1
}

// coYieldable is luaB_yieldable (lcorolib.c): with no argument it asks
// about the running thread, otherwise about the given thread.  The
// answer is C's 'yieldable', i.e. whether the upper 16 bits of
// 'nCcalls' are clear: the main thread and any call made through
// luaD_callnoyield (a metamethod, a 'for' iterator, 'table.sort', a
// __close handler, ...) are not yieldable.
func coYieldable(L *State) int {
	co := L
	if !L.IsNone(1) {
		co = coGetCo(L)
	}
	L.PushBoolean(yieldable(co))
	return 1
}

func coRunning(L *State) int {
	ismain := L.PushThread()
	L.PushBoolean(ismain)
	return 2
}

func coPushValue(L *State, v Value) {
	L.checkStack(1)
	L.stack[L.top] = v
	L.top++
}

// coCloseThread is the port's lua_closethread(co, from): it asks the
// coroutine goroutine to unwind itself. A coroutine that has already
// finished has no goroutine left to talk to, and is simply reported as
// closed (which is what C does too).
//
// Like C, the thread being closed inherits the C-call depth of the
// closer (getCcalls(from)), so that a chain of 'coroutine.close' calls
// is reported as a C stack overflow instead of recursing forever.
func coCloseThread(co, from *State) (bool, Value) {
	if co.isMain {
		return false, strValue(co, "cannot close a running coroutine")
	}
	if from != nil {
		co.nCcalls = from.nCcalls & 0xffff
	}
	if co.dead || co.co == nil {
		/* The coroutine already terminated, so there is no goroutine
		   left to unwind. C runs luaE_resetthread(co, co->status), which
		   closes any pending variable and reports the thread's status:
		   a coroutine that ended with an error reports that error once
		   again (false + message), and the report is then consumed --
		   the second close returns true. */
		st := co.resetThreadStatus(int(co.status))
		if st != LUA_OK {
			co.hasErr = false
			if !ttisnil(&co.lastErr) {
				return false, co.lastErr
			}
			return false, co.stack[1]
		}
		co.hasErr = false
		return true, Value{} /* already closed */
	}
	co.co.req <- resumeReq{kind: reqClose}
	resp := <-co.co.resp
	co.dead = true
	if resp.status == LUA_OK {
		return true, Value{}
	}
	if len(resp.vals) > 0 {
		return false, resp.vals[0]
	}
	return false, strValue(co, "error closing coroutine")
}

func coClose(L *State) int {
	co := coGetCo(L)
	status := coAuxStatus(L, co)
	switch status {
	case cosDead, cosYield:
		ok, errval := coCloseThread(co, L)
		if ok {
			L.PushBoolean(true)
			return 1
		}
		L.PushBoolean(false)
		coPushValue(L, errval) /* move error message */
		return 2
	default: /* normal or running coroutine */
		return L.Error2("cannot close a %s coroutine", coStatNames[status])
	}
}

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
