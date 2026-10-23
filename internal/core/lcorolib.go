// Translation of lcorolib.c (PUC-Lua 5.3.4): the coroutine library.
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
//     to unwind itself through the same request channel.
package core

// getco gets the coroutine in the first argument (lcorolib.c getco).
func coGetCo(L *State) *State {
	co := L.ToThread(1)
	/* 5.3.4: luaL_argcheck(L, co, 1, "thread expected") — no ", got TYPE". */
	L.ArgCheck(co != nil, 1, "thread expected")
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
		nres := co.GetTop() /* C auxresume: lua_gettop(co) */
		if !L.CheckStack(nres + 1) {
			co.top -= nres /* remove results from the coroutine */
			if co.top < 0 {
				co.top = 0
			}
			L.SetTop(base)
			L.PushString("too many results to resume")
			return -1 /* error flag */
		}
		L.XMove(co, nres) /* C's auxresume: lua_xmove(co, L, nres) */
		return nres
	}
	/* error: move the message from the coroutine onto L */
	if co.top > 0 {
		L.XMove(co, 1)
	} else if len(co.co.vals) > 0 {
		L.checkStack(1)
		L.stack[L.top] = co.co.vals[0]
		L.top++
	} else {
		L.PushString("cannot resume dead coroutine")
	}
	if ttisnil(&L.stack[L.top-1]) {
		/* ldo.go's luaD_throw panics without carrying the error object,
		   so the message may be nil here while still sitting on the
		   coroutine stack. C's lua_resume uses luaD_seterrorobj. */
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
	/* luaB_auxwrap (lcorolib.c 5.3.4): no lua_closethread / to-be-closed. */
	co := L.ToThread(UpvalueIndex(1))
	r := coAuxResume(L, co, L.GetTop())
	if r < 0 {
		if L.Type(-1) == LUA_TSTRING { /* error object is a string? */
			L.Where2(1) /* add extra info */
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

// coYieldable is luaB_yieldable (lcorolib.c 5.3.4): no argument; asks
// whether the running thread is yieldable (upper 16 bits of nCcalls clear).
func coYieldable(L *State) int {
	L.PushBoolean(yieldable(L))
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

var coFuncs = []Reg{
	{Name: "create", Func: coCreate},
	{Name: "resume", Func: coResume},
	{Name: "running", Func: coRunning},
	{Name: "status", Func: coStatus},
	{Name: "wrap", Func: coWrap},
	{Name: "yield", Func: coYield},
	{Name: "isyieldable", Func: coYieldable},
}

// OpenCoroutine opens the coroutine library (luaopen_coroutine).
func OpenCoroutine(L *State) int {
	L.NewLib(coFuncs)
	return 1
}
