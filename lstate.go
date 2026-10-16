// Translation of lstate.h / lstate.c (PUC-Lua 5.4).
//
// GC-related fields are gone (there is no collector); object lifetime is
// the Go runtime's job. Error handling uses panic/recover instead of
// setjmp/longjmp, and each coroutine runs on its own goroutine so that a
// Go function can yield from the middle of a call.
package lua

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
)

const (
	EXTRA_STACK      = 5
	BASIC_STACK_SIZE = 2 * LUA_MINSTACK
	// ERRORSTACKSIZE is the size the stack is grown to when it overflows,
	// so that there is still room to run the error message (ldo.c).
	ERRORSTACKSIZE = LUAI_MAXSTACK + 200
)

// GoFunc is the Go equivalent of lua_CFunction: it receives the state and
// returns the number of results left on the stack.
type GoFunc func(L *State) int

// WarnFunc is the Go equivalent of lua_WarnFunction.
type WarnFunc func(L *State, msg string, tocont bool)

// CallInfo status bits (lstate.h).
const (
	CIST_OAH       = 1 << 0
	CIST_C         = 1 << 1
	CIST_FRESH     = 1 << 2
	CIST_HOOKED    = 1 << 3
	CIST_YPCALL    = 1 << 4
	CIST_TAIL      = 1 << 5
	CIST_HOOKYIELD = 1 << 6
	CIST_FIN       = 1 << 7
	CIST_TRAN      = 1 << 8
	CIST_CLSRET    = 1 << 9
)

// incnny/decnny manipulate the "non-yieldable" part of nCcalls, which
// lives in the upper 16 bits (lstate.h).
func incnny(L *State) { L.nCcalls += 0x10000 }

func decnny(L *State) { L.nCcalls -= 0x10000 }

// yieldable reports whether the thread has no non-yieldable call in the
// stack (lstate.h).
func yieldable(L *State) bool { return L.nCcalls&0xffff0000 == 0 }

func isLua(ci *CallInfo) bool     { return ci.callstatus&CIST_C == 0 }
func isLuacode(ci *CallInfo) bool { return ci.callstatus&(CIST_C|CIST_HOOKED) == 0 }

// CallInfo holds information about a call.
type CallInfo struct {
	funcIdx    int // function index in the stack
	top        int // top for this function
	prev       *CallInfo
	next       *CallInfo
	savedpc    int
	nresults   int
	callstatus uint16
	nExtraArgs int // number of vararg extra arguments
	oldErrFunc int

	// trap is ci->u.l.trap: whether the VM must stop at the next
	// instruction to check hooks (and to correct a moved stack).
	trap bool
	// ftransfer/ntransfer are ci->u2.transferinfo: the position and the
	// number of values being transferred, for the return/call hook.
	ftransfer int
	ntransfer int
}

// globalState is the state shared by all threads (lstate.h).
type globalState struct {
	strt       *stringTable
	registry   Value
	nilValue   Value
	seed       uint32
	mainthread *State
	tmNames    [TM_N]*TString
	mt         [LUA_NUMTYPES]*Table
	panic      GoFunc
	warnf      WarnFunc
	udWarn     any
	memErrMsg  *TString
	gcDebt     int  // allocations since the last collector step
	closing    bool // 'gcstp & GCSTPCLS': no new '__gc' once the state closes
}

// State is the Go equivalent of lua_State.
type State struct {
	g *globalState

	status    byte
	allowhook byte

	// hook machinery (lstate.h). 'hook' is always a Go function: the
	// debug library installs 'dbHookFunc', which then calls the Lua
	// function stored in the hook table.
	hook          func(L *State, ar *dbAr)
	hookmask      byte
	basehookcount int
	hookcount     int
	oldpc         int

	top       int // first free slot in the stack
	stack     []Value
	stackLast int // last usable slot (len(stack) - EXTRA_STACK)
	ci        *CallInfo
	baseCI    CallInfo

	openupval []*UpVal // open upvalues of this stack
	tbclist   []int    // to-be-closed variables (stack indices)

	errfunc int // current error handling function (stack index)
	nCcalls int
	nci     int

	ciPool []*CallInfo

	// coroutine machinery (there is no equivalent in C)
	co       *coRuntime
	parent   *State
	isMain   bool
	dead     bool
	prevFunc Value // function to run on the next resume after a reset

	// lastErr is the error object of a coroutine that ended with an
	// error. C keeps it on the coroutine's own stack (lua_resume leaves
	// it there and luaE_resetthread feeds L->status back through
	// luaD_closeprotected), so 'coroutine.close' can report it again.
	lastErr Value
	hasErr  bool
}

// coRuntime is the channel pair that drives a coroutine goroutine.
type coRuntime struct {
	req  chan *resumeReq
	resp chan *resumeResp
}

type resumeReq struct {
	kind int // reqResume or reqClose
	args []Value
	// ccalls is the C-call depth of the resuming thread
	// (getCcalls(from) in C's lua_resume).  Carrying it across is what
	// lets "infinite recursion of coroutines" be reported as a C stack
	// overflow instead of recursing forever.
	ccalls int
}

type resumeResp struct {
	status int
	vals   []Value
}

const (
	reqResume = 0
	reqClose  = 1
)

// yieldSignal is what lua_yield throws; it is not a Lua error but a
// non-local exit to the resume point.
type yieldSignal struct {
	vals []Value
}

// closeSignal unwinds a coroutine being closed by coroutine.close.
type closeSignal struct{}

// luaError is the object thrown by luaD_throw.
type luaError struct {
	status int
	value  Value
}

func (e *luaError) Error() string { return valueToStr(&e.value) }

func valueToStr(v *Value) string {
	switch {
	case ttisstring(v):
		return tsvalue(v).val
	case ttisnumber(v):
		return tostringbuff(v)
	case ttisnil(v):
		return "nil"
	case ttisboolean(v):
		if bvalue(v) {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%s: %#x", typeName(v), ptrID(v.p))
}

func typeName(v *Value) string {
	switch ttype(v) {
	case LUA_TNIL:
		return "nil"
	case LUA_TBOOLEAN:
		return "boolean"
	case LUA_TNUMBER:
		return "number"
	case LUA_TSTRING:
		return "string"
	case LUA_TTABLE:
		return "table"
	case LUA_TFUNCTION:
		return "function"
	case LUA_TUSERDATA, LUA_TLIGHTUSERDATA:
		return "userdata"
	case LUA_TTHREAD:
		return "thread"
	}
	return "no value"
}

// ---------------------------------------------------------------------
// stack management (ldo.c)
// ---------------------------------------------------------------------

func (L *State) stackSize() int { return L.stackLast }

// reallocStack is 'luaD_reallocstack' (ldo.c).  'newsize' is the number of
// *usable* slots, exactly C's 'stack_last - stack': the buffer itself gets
// EXTRA_STACK more, which is the room C reserves so that an error message
// can still be pushed after a stack overflow.
func (L *State) reallocStack(newsize int) {
	ns := make([]Value, newsize+EXTRA_STACK)
	copy(ns, L.stack)
	L.stack = ns
	L.stackLast = newsize
}

// growStackNoRaise is 'luaD_growstack' (ldo.c). It grows the stack to hold
// 'n' more slots and reports whether that was possible; when 'raise' is
// false it never raises an error -- C's callers such as 'lua_checkstack'
// rely on getting a 0 back instead of a longjmp.
func (L *State) growStackNoRaise(n int, raise bool) bool {
	size := L.stackSize()
	if size > LUAI_MAXSTACK {
		/* The stack is already at ERRORSTACKSIZE, that is, the thread is
		   using the extra space reserved for errors: it is handling a
		   stack error and cannot grow any further. */
		if raise {
			luaD_errerr(L) /* error inside message handler */
		}
		return false
	}
	if n < LUAI_MAXSTACK { /* avoids arithmetic overflows */
		newsize := 2 * size          /* tentative new size */
		needed := L.top + n          /* what was asked for, in absolute slots */
		if newsize > LUAI_MAXSTACK { /* cannot cross the limit */
			newsize = LUAI_MAXSTACK
		}
		if newsize < needed { /* but must respect what was asked for */
			newsize = needed
		}
		if newsize <= LUAI_MAXSTACK {
			L.reallocStack(newsize)
			return true
		}
	}
	/* else stack overflow: add extra size to be able to handle the
	   error message, so that the message handler can still run */
	L.reallocStack(ERRORSTACKSIZE)
	if raise {
		luaG_runerror(L, "stack overflow")
	}
	return false
}

func (L *State) growStack(n int) { L.growStackNoRaise(n, true) }

func (L *State) checkStack(n int) {
	if L.stackLast-L.top <= n {
		L.growStack(n)
	}
}

func (L *State) shrinkStack() {
	size := L.stackSize()
	used := L.top + EXTRA_STACK
	if size > BASIC_STACK_SIZE && used < size/4 {
		newsize := size / 2
		if newsize < used {
			newsize = used
		}
		if newsize < BASIC_STACK_SIZE {
			newsize = BASIC_STACK_SIZE
		}
		L.reallocStack(newsize)
	}
}

// ---------------------------------------------------------------------
// CallInfo management (lstate.c)
// ---------------------------------------------------------------------

func (L *State) extendCI() *CallInfo {
	var ci *CallInfo
	if n := len(L.ciPool); n > 0 {
		ci = L.ciPool[n-1]
		L.ciPool = L.ciPool[:n-1]
	} else {
		ci = new(CallInfo)
	}
	L.nci++
	ci.prev = L.ci
	if L.ci != nil {
		L.ci.next = ci
	}
	ci.next = nil
	L.ci = ci
	return ci
}

func (L *State) freeCI(ci *CallInfo) {
	*ci = CallInfo{}
	if len(L.ciPool) < 64 {
		L.ciPool = append(L.ciPool, ci)
	}
	L.nci--
}

func (L *State) shrinkCI() {
	for L.ci != &L.baseCI && L.ci != nil {
		prev := L.ci.prev
		L.freeCI(L.ci)
		L.ci = prev
	}
	if L.ci != nil {
		L.ci.next = nil
	}
}

// ---------------------------------------------------------------------
// creation / destruction
// ---------------------------------------------------------------------

var stateIDSeed uint64

// NewState creates a new Lua state (replaces lua_newstate).
func NewState() *State {
	g := &globalState{}
	L := &State{g: g}
	g.mainthread = L
	g.seed = uint32(rand.Uint64())
	atomic.AddUint64(&stateIDSeed, 1)

	L.nCcalls = 0
	L.isMain = true
	L.status = LUA_OK
	L.allowhook = 1
	L.errfunc = 0
	incnny(L) /* main thread is always non yieldable */

	L.reallocStack(BASIC_STACK_SIZE)
	L.top = 1
	L.ci = &L.baseCI
	L.baseCI.funcIdx = 0
	L.baseCI.top = 1
	L.baseCI.callstatus = CIST_C
	L.baseCI.nresults = 0
	gcTrackThread(L) /* so that the collector can clear its stale slots */

	g.nilValue = Value{tt: LUA_VNIL}
	g.strt = newStringTable()
	g.memErrMsg = luaS_new(L, "not enough memory")
	luaT_init(L)
	luaS_init(L)
	luaX_init(L)
	// registry
	sethvalue(&g.registry, luaH_new(L))
	// registry[1] = main thread, registry[2] = globals table
	var th Value
	setthvalue(&th, L)
	luaH_setint(L, g.registry.p.(*Table), 1, &th)
	var gl Value
	gt := luaH_new(L)
	sethvalue(&gl, gt)
	luaH_setint(L, g.registry.p.(*Table), 2, &gl)
	/* luaL_newstate's last act: the default warning function keeps the
	   warning system off until 'warn("@on")' or the '-W' option. */
	g.setWarnf(warnfOff)
	return L
}

// Close closes a state (replaces lua_close).
func (L *State) Close() {
	L = L.g.mainthread /* only the main thread can be closed */
	// the '__gc' metamethods run on closing, as they do in C
	L.closeFinalizers()
	// close all coroutines of this state
	// (nothing to free: the Go GC does it)
	L.status = LUA_OK
	L.dead = true
	if L.co != nil && !L.isMain {
		go func() { L.co.req <- &resumeReq{kind: reqClose} }()
	}
}

// G returns the global state (G(L)).
func (L *State) G() *globalState { return L.g }

// NewThread creates a new coroutine (replaces lua_newthread).
func (L *State) NewThread() *State {
	g := L.g
	L1 := &State{g: g}
	L1.isMain = false
	L1.allowhook = 1
	L1.status = LUA_YIELD
	L1.nCcalls = 0
	L1.errfunc = 0
	L1.reallocStack(BASIC_STACK_SIZE)
	L1.top = 1
	L1.ci = &L1.baseCI
	L1.baseCI.funcIdx = 0
	L1.baseCI.top = 1
	L1.baseCI.callstatus = CIST_C
	L1.baseCI.nresults = 0
	gcTrackThread(L1) /* so that the collector can clear its stale slots */
	/* a new thread inherits the hook of the creating thread */
	L1.hook = L.hook
	L1.hookmask = L.hookmask
	L1.basehookcount = L.basehookcount
	L1.hookcount = L.basehookcount /* resethookcount(L1) */
	L1.co = &coRuntime{req: make(chan *resumeReq), resp: make(chan *resumeResp)}
	go L1.coMain()
	// the thread object goes on the stack of the creating state
	L.checkStack(1)
	var th Value
	setthvalue(&th, L1)
	L.stack[L.top] = th
	L.top++
	return L1
}

// coMain is the body of a coroutine goroutine.
func (L *State) coMain() {
	for {
		req := <-L.co.req
		if req.kind == reqClose {
			L.status = LUA_OK
			L.dead = true
			L.co.resp <- &resumeResp{status: LUA_OK}
			return
		}
		status, vals, closed := L.coRun(req.args, req.ccalls)
		if status == LUA_YIELD {
			L.status = LUA_YIELD
			L.co.resp <- &resumeResp{status: LUA_YIELD, vals: vals}
			continue
		}
		L.dead = true
		if closed {
			/* luaE_resetthread leaves the thread with status LUA_OK,
			   so the error from '__close' is reported only once: a
			   second 'coroutine.close' succeeds. */
			L.status = LUA_OK
			L.co.resp <- &resumeResp{status: status, vals: vals}
			return
		}
		L.status = byte(status)
		if status != LUA_OK && len(vals) > 0 {
			L.lastErr = vals[0]
			L.hasErr = true
		}
		L.co.resp <- &resumeResp{status: status, vals: vals}
		return
	}
}

// coRun runs the coroutine body with the given arguments. The third
// result tells whether the run ended because the thread was closed.
func (L *State) coRun(args []Value, ccalls int) (status int, vals []Value, closed bool) {
	status = LUA_OK
	vals = nil
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case *yieldSignal:
				status = LUA_YIELD
				vals = x.vals
				return
			case *closeSignal:
				/* C's lua_closethread -> luaE_resetthread: unwind the
				   CallInfo list and close every to-be-closed variable
				   still pending on the coroutine's stack.  A '__close'
				   that raises must not stop the others: C keeps going
				   and reports the *newest* error. */
				closed = true
				L.ci = &L.baseCI
				L.baseCI.next = nil
				L.baseCI.funcIdx = 0
				L.baseCI.top = 1
				L.baseCI.callstatus = CIST_C
				setnilvalue(&L.stack[0])
				L.errfunc = 0
				st := int(L.status)
				if st == LUA_YIELD {
					st = LUA_OK
				}
				L.status = LUA_OK /* so that it can run the metamethods */
				st = luaD_closeprotected(L, 1, st)
				if st != LUA_OK {
					status = st
					luaD_seterrorobj(L, st, 1)
					vals = []Value{L.stack[1]}
				} else {
					status = LUA_OK
					L.top = 1
				}
				return
			case *luaError:
				status = x.status
				vals = []Value{x.value}
				return
			default:
				status = LUA_ERRRUN
				var v Value
				setsvalue(&v, luaS_newlstr(L, fmt.Sprintf("%v", r)))
				vals = []Value{v}
				return
			}
		}
	}()
	// stack[0] is the function; push the arguments after it
	n := len(args)
	L.checkStack(n + 1)
	copy(L.stack[1:], args)
	L.top = 1 + n
	/* C's lua_resume: the thread inherits the C-call depth of the
	   resumer (getCcalls(from)), which is how "infinite recursion of
	   coroutines" is reported as a C stack overflow.  Only the lower
	   16 bits are copied, so the "not yieldable" mark of the resumer
	   does not leak into the coroutine.

	   C then bumps 'nCcalls' once more and calls the body with
	   'ccall(..., 0)', so a running coroutine sits at
	   getCcalls(from) + 1.  Here luaD_call supplies that +1. */
	L.nCcalls = ccalls
	if L.top == 1 {
		// function was reset: use the pending function
		L.stack[0] = L.prevFunc
	}
	luaD_call(L, 0, LUA_MULTRET)
	vals = make([]Value, L.top)
	copy(vals, L.stack[:L.top])
	return status, vals, false
}

// resetThreadStatus is luaE_resetthread: it unwinds the CallInfo list and
// closes every to-be-closed variable still pending on the stack, returning
// the (possibly new) status. C uses it both to reset a thread for reuse and
// to close a coroutine that has just died.
func (L *State) resetThreadStatus(status int) int {
	ci := &L.baseCI
	L.ci = ci
	setnilvalue(&L.stack[0]) /* 'function' entry for basic 'ci' */
	ci.funcIdx = 0
	ci.callstatus = CIST_C
	if status == LUA_YIELD {
		status = LUA_OK
	}
	L.status = LUA_OK /* so it can run __close metamethods */
	L.errfunc = 0     /* stack unwind can "throw away" the error function */
	status = luaD_closeprotected(L, 1, status)
	if status != LUA_OK { /* errors? */
		luaD_seterrorobj(L, status, 1)
	} else {
		L.top = 1
	}
	ci.top = L.top + LUA_MINSTACK
	return status
}

// ResetThread resets a coroutine so that it can be reused (lua_resetthread).
func (L *State) ResetThread() int {
	if L.isMain {
		return LUA_ERRRUN
	}
	if !L.dead && L.status == LUA_YIELD {
		// still suspended: unwind it
		L.co.req <- &resumeReq{kind: reqClose}
		<-L.co.resp
	}
	L.dead = false
	L.hasErr = false
	L.lastErr = Value{}
	L.status = LUA_YIELD
	L.top = 1
	L.ci = &L.baseCI
	L.baseCI.next = nil
	L.tbclist = nil
	L.openupval = nil
	L.errfunc = 0
	L.nCcalls = 0
	L.co = &coRuntime{req: make(chan *resumeReq), resp: make(chan *resumeResp)}
	go L.coMain()
	return LUA_OK
}

// Resume resumes a coroutine with nargs values on top of L.
func (L *State) Resume(from *State, nargs int) int {
	co := L
	if co.isMain {
		return LUA_ERRRUN
	}
	switch co.status {
	case LUA_OK:
		if co.dead {
			return LUA_ERRRUN
		}
	case LUA_YIELD:
	default:
		return LUA_ERRRUN
	}
	args := make([]Value, nargs)
	copy(args, from.stack[from.top-nargs:from.top])
	if co.status == LUA_OK {
		// starting (or restarting) the coroutine: the function is the
		// value that was pushed on the coroutine stack
		fn := co.stack[co.top-1]
		co.prevFunc = fn
		co.top = 1
		co.stack[0] = fn
	}
	from.top -= nargs
	co.parent = from
	co.status = LUA_OK
	ccalls := 0
	if from != nil {
		ccalls = from.nCcalls & 0xffff /* getCcalls(from) */
	}
	if ccalls >= LUAI_MAXCCALLS {
		/* C's resume_error: "C stack overflow" */
		return resumeError(from, "C stack overflow")
	}
	co.co.req <- &resumeReq{kind: reqResume, args: args, ccalls: ccalls}
	resp := <-co.co.resp
	co.status = byte(resp.status)
	if resp.status == LUA_OK || resp.status == LUA_YIELD {
		/* C's auxresume grows the resumer's stack with lua_checkstack,
		   which must not raise: when the results do not fit, the error
		   is "too many results to resume". */
		if !from.CheckStack(len(resp.vals)) {
			return resumeError(from, "too many results to resume")
		}
		copy(from.stack[from.top:], resp.vals)
		from.top += len(resp.vals)
		if resp.status == LUA_YIELD {
			return LUA_YIELD
		}
		return LUA_OK
	}
	if !from.CheckStack(1) {
		return LUA_ERRRUN
	}
	from.stack[from.top] = resp.vals[0]
	from.top++
	return resp.status
}

// resumeError is 'resume_error' (ldo.c): it pushes the message and
// reports the failure to 'resume'.
func resumeError(L *State, msg string) int {
	L.checkStack(1)
	L.stack[L.top] = strValue(L, msg)
	L.top++
	return LUA_ERRRUN
}

// Yield yields nresults values (lua_yieldk / lua_yield).
func (L *State) Yield(nresults int) {
	if L.isMain || L.co == nil {
		panic(&luaError{status: LUA_ERRRUN, value: strValue(L, "attempt to yield from outside a coroutine")})
	}
	vals := make([]Value, nresults)
	copy(vals, L.stack[L.top-nresults:L.top])
	panic(&yieldSignal{vals: vals})
}

func strValue(L *State, s string) Value {
	var v Value
	setsvalue(&v, luaS_newlstr(L, s))
	return v
}

// ---------------------------------------------------------------------
// errors (ldo.c)
// ---------------------------------------------------------------------

// throw is luaD_throw. The error object must be on the top of the stack;
// the message handler (if any) is applied by luaG_errormsg.
func (L *State) throw(status int) {
	panic(&luaError{status: status})
}

// rawRunProtected is luaD_rawrunprotected, translated with
// panic/recover instead of setjmp/longjmp.
func (L *State) rawRunProtected(f func()) (status int) {
	/* C saves 'nCcalls' around the setjmp: an error unwinds past the
	   decrements in 'ccall', so without this the C-call depth would stay
	   inflated and the next error would be reported as "error in error
	   handling". */
	oldNCcalls := L.nCcalls
	defer func() {
		L.nCcalls = oldNCcalls
		r := recover()
		if r == nil {
			return
		}
		switch x := r.(type) {
		case *luaError:
			status = x.status
		case *yieldSignal:
			panic(x) // yields escape protected calls
		case *closeSignal:
			panic(x)
		default:
			// a Go panic becomes a Lua error
			var v Value
			setsvalue(&v, luaS_newlstr(L, fmt.Sprintf("%v", r)))
			panic(&luaError{status: LUA_ERRRUN, value: v})
		}
	}()
	f()
	return LUA_OK
}

// ---------------------------------------------------------------------
// warnings (lstate.c)
// ---------------------------------------------------------------------

func luaE_warning(L *State, msg string, tocont bool) {
	g := L.g
	if g.warnf != nil {
		g.warnf(L, msg, tocont)
	}
}

// Warning emits a warning (lua_warning).  Messages that start with '@'
// and are not continuations are control messages ('@on' / '@off'); the
// others are written to stderr prefixed with "Lua warning: ".
func (L *State) Warning(msg string, tocont bool) {
	luaE_warning(L, msg, tocont)
}

// ---------------------------------------------------------------------
// the default warning functions (lauxlib.c)
//
// The warning system is a three-state machine: it is off until
// 'warn("@on")' (or the '-W' option) turns it on, and a message can be
// composed of several parts, which the 'tocont' flag announces.
// ---------------------------------------------------------------------

// warnCheckControl is checkcontrol: '@off' and '@on' switch the warning
// system off and on; unknown control messages are ignored.
func warnCheckControl(L *State, msg string, tocont bool) bool {
	if tocont || !strings.HasPrefix(msg, "@") { /* not a control message? */
		return false
	}
	switch msg[1:] {
	case "off":
		L.g.setWarnf(warnfOff) /* turn warnings off */
	case "on":
		L.g.setWarnf(warnfOn) /* turn warnings on */
	}
	return true /* it was a control message */
}

func (g *globalState) setWarnf(f WarnFunc) { g.warnf = f }

// warnfOff is warnfoff: the warning system is off, so nothing is written
// until a control message turns it back on.
func warnfOff(L *State, msg string, tocont bool) {
	warnCheckControl(L, msg, tocont)
}

// warnfCont is warnfcont: the previous message is to be continued.
func warnfCont(L *State, msg string, tocont bool) {
	fmt.Fprint(os.Stderr, msg) /* write message */
	if tocont {                /* not the last part? */
		L.g.setWarnf(warnfCont) /* to be continued */
	} else { /* last part */
		fmt.Fprintln(os.Stderr) /* finish message with end-of-line */
		L.g.setWarnf(warnfOn)   /* next call is a new message */
	}
}

// warnfOn is warnfon: ready to start a new message.
func warnfOn(L *State, msg string, tocont bool) {
	if warnCheckControl(L, msg, tocont) { /* control message? */
		return /* nothing else to be done */
	}
	fmt.Fprint(os.Stderr, "Lua warning: ") /* start a new warning */
	warnfCont(L, msg, tocont)              /* finish processing */
}

func luaE_warnerror(L *State, where string) {
	luaE_warning(L, fmt.Sprintf("%s: WARNING: failed to write warning: ", where), false)
}

// AtPanic sets the panic function (lua_atpanic).
func (L *State) AtPanic(f GoFunc) GoFunc {
	old := L.g.panic
	L.g.panic = f
	return old
}

// SetWarnFunction sets the warning function (lua_setwarnf).
func (L *State) SetWarnFunction(f WarnFunc, ud any) {
	L.g.warnf = f
	L.g.udWarn = ud
}
