// Translation of lstate.h / lstate.c (PUC-Lua 5.4.9).
//
// Intentional adaptations (see NOTES.md):
//   - Go GC replaces lgc (keep finalizer/warn bridge knobs; ignore mark/sweep).
//   - Error handling uses panic/recover instead of setjmp/longjmp.
//   - Index stack (funcIdx / int tops) instead of StkIdRel / TValue*.
//   - Goroutine (+ sync fast-path) coroutines instead of C-stack longjmp resume.
package core

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
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

// CallInfo status bits (lstate.h 5.4.9).
const (
	CIST_OAH       = 1 << 0 /* original value of 'allowhook' */
	CIST_C         = 1 << 1 /* call is running a C function */
	CIST_FRESH     = 1 << 2 /* call is on a fresh "luaV_execute" frame */
	CIST_HOOKED    = 1 << 3 /* call is running a debug hook */
	CIST_YPCALL    = 1 << 4 /* doing a yieldable protected call */
	CIST_TAIL      = 1 << 5 /* call was tail called */
	CIST_HOOKYIELD = 1 << 6 /* last hook called yielded */
	CIST_FIN       = 1 << 7 /* function "called" a finalizer */
	CIST_TRAN      = 1 << 8 /* 'ci' has transfer information */
	CIST_CLSRET    = 1 << 9 /* function is closing tbc variables */
	/* Bits 10-12 are used for CIST_RECST (see below) */
	CIST_RECST = 10
)

// setoah / getoah store/load allowhook in CIST_OAH (lstate.h).
// assume CIST_OAH has offset 0 and that v is strictly 0/1.
func setoah(st *uint16, v byte) {
	*st = (*st &^ CIST_OAH) | uint16(v&1)
}

func getoah(st uint16) byte {
	return byte(st & CIST_OAH)
}

// getcistrecst / setcistrecst: recover status while closing tbc (lstate.h).
func getcistrecst(ci *CallInfo) int {
	return int((ci.callstatus >> CIST_RECST) & 7)
}

func setcistrecst(ci *CallInfo, st int) {
	ci.callstatus = (ci.callstatus &^ (7 << CIST_RECST)) | (uint16(st&7) << CIST_RECST)
}

// incnny/decnny manipulate the "non-yieldable" part of nCcalls, which
// lives in the upper 16 bits (lstate.h).
//
// nCcalls is l_uint32 there -- an *unsigned* 32-bit count -- and it has
// to be: 0xffff0000 does not fit in a signed 32-bit int, so on a 386
// build the mask itself does not compile.  Unsigned is also what C's
// own arithmetic on it assumes ('nCcalls += 0x10000' wraps).
func incnny(L *State) { L.nCcalls += 0x10000 }

func decnny(L *State) { L.nCcalls -= 0x10000 }

// getCcalls is the real number of C calls (lower 16 bits).
func getCcalls(L *State) int { return int(L.nCcalls & 0xffff) }

// yieldable reports whether the thread has no non-yieldable call in the
// stack (lstate.h).
func yieldable(L *State) bool { return L.nCcalls&0xffff0000 == 0 }

func isLua(ci *CallInfo) bool     { return ci.callstatus&CIST_C == 0 }
func isLuacode(ci *CallInfo) bool { return ci.callstatus&(CIST_C|CIST_HOOKED) == 0 }

// completestate is C's completestate(g): g.nilvalue being a nil flags that
// the state was completely built (lstate.h). Incomplete states use a
// non-nil sentinel (setivalue 0) until f_luaopen finishes.
func completestate(g *globalState) bool { return ttisnil(&g.nilValue) }

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

	// cK and cCtx are ci->u.c.k and ci->u.c.ctx: the continuation a C
	// function handed to 'lua_callk', 'lua_pcallk' or 'lua_yieldk', and
	// the context to pass back to it (ldo.c finishCcall).  cK is nil
	// when the frame has none.  It is a C function pointer, which is
	// why it is an 'unsafe.Pointer' rather than a Go func: only
	// cdlopen.go can call it.
	cK   unsafe.Pointer
	cCtx int64
	// goCont is the continuation of a Go library function that a
	// synchronous yield may have unwound (pcall, xpcall, dofile, pairs).
	// The frame stays on the call chain; the resume finishes it by
	// calling goCont instead of re-entering the Go function. Nil when
	// the frame has none.
	goCont func(L *State, status int) int
	// migrateFunc is the Go function whose already-pushed C frame asked
	// to move onto the coroutine goroutine before entering its body.
	// Tail calls may no longer leave a usable closure in funcIdx, so the
	// function is recorded explicitly for finishMigrated.
	migrateFunc GoFunc
	// ypcall is CIST_YPCALL: this frame is an interrupted 'lua_pcallk'
	// with a continuation, so an error raised below it is recovered
	// here instead of killing the coroutine (ldo.c findpcall).
	ypcall bool
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

	// pendingRun is the queue of finalizers handed over by Go and not
	// run yet -- see runFinalizers, which a nested lua_close has to be
	// able to finish.  It is per state on purpose: see the note above
	// the queue in lgc.go.
	pendingRun []gcFinalizer
	pendN      atomic.Int32 // len(pendingRun), so that the hot path needs no lock
	// closeQueue is C's 'g->tobefnz' while the state is closing: the
	// objects whose '__gc' has not run yet.  It is on the state, and
	// not in a local of closeFinalizers, because a '__gc' may close the
	// state again -- 'os.exit(0, true)' from a finalizer -- and that
	// second close has to be able to finish what is left.  See
	// drainCloseQueue.
	closeQueue []weakRef
	// The four fields below are the collector's knobs.  They are what
	// 'collectgarbage' and the C API's 'lua_gc' (see cdl_gc_cb) read and
	// write, and Go's collector pays no attention to any of them --
	// except 'gcStopped', which has an effect of its own: it gates the
	// step an allocation drives.
	gcMode    string // "incremental" or "generational"
	gcStopped bool   // 'gcstp & GCSTPUSR': the program stopped the collector
	gcPause   int    // 'gcpause': LUAI_GCPAUSE, answered by 'setpause'
	gcStepMul int    // 'gcstepmul': LUAI_GCMUL, answered by 'setstepmul'

	// gcInFinalizer is 'gcstp & GCSTPGC' while a '__gc' metamethod is
	// running: PUC-Lua stops the collector for the duration of one, and
	// 'lua_gc' then answers -1 for every option, which is what makes
	// 'collectgarbage' return nil inside a finalizer.  It is a global
	// flag there, as it is here -- a finalizer that asks is asking from
	// whichever state is closing.
	gcInFinalizer bool

	// cstates are the malloc'd lua_State handles of every thread C has
	// been shown. Closing the main state gives them all back. Unused
	// without cgo.
	cstateMu sync.Mutex
	cstates  []*cstateBlock
}

// State is the Go equivalent of lua_State.
type State struct {
	g *globalState

	status    byte
	allowhook byte

	// nYields counts the yields this thread has made.  C can tell from
	// the stack that a call was interrupted by one (the CallInfo it
	// left behind is 'CIST_YPCALL'); here the call simply returns, so
	// 'lua_pcallk' compares the count before and after to know whether
	// to report LUA_OK or LUA_YIELD to the continuation (ldo.c
	// finishpcallk).
	nYields int

	// hook machinery (lstate.h). 'hook' is always a Go function: the
	// debug library installs 'dbHookFunc', which then calls the Lua
	// function stored in the hook table.
	hook          func(L *State, ar *dbAr)
	hookmask      byte
	basehookcount int
	hookcount     int
	oldpc         int
	// hookExternal is C's "hook != hookf": whether the hook was
	// installed through the C API (lua_sethook) rather than by
	// debug.sethook.  Go cannot compare two functions, so the
	// distinction is recorded when the hook is installed instead of
	// being asked of it afterwards.  'debug.gethook' answers
	// "external hook" for one and the hook table's entry for the
	// other.
	hookExternal bool

	top       int // first free slot in the stack
	stack     []Value
	stackLast int // last usable slot (len(stack) - EXTRA_STACK)
	ci        *CallInfo
	baseCI    CallInfo

	openupval []*UpVal // open upvalues of this stack
	tbclist   []int    // to-be-closed variables (stack indices)

	// Last two-string concatenation. Tight loops commonly join the same
	// pair on every iteration; short-string interning finds the same
	// result but only after Go has allocated a temporary a+b string.
	concatA, concatB, concatR *TString

	errfunc int    // current error handling function (stack index)
	nCcalls uint32 // l_uint32: see the note on incnny
	nci     int

	ciPool []*CallInfo

	// coroutine machinery (there is no equivalent in C)
	co     *coRuntime
	parent *State
	isMain bool
	dead   bool
	// started is whether this thread has ever been resumed. A thread
	// that has not been is 'LUA_YIELD' here -- that is what makes
	// coroutine.status call it "suspended", which is what PUC-Lua calls
	// it too -- but C's lua_status answers LUA_OK for it, and it is the
	// only way to tell "start it" from "wake it up" in lua_resume.
	started  bool
	prevFunc Value // function to run on the next resume after a reset
	// fastYield is set when a synchronous yield has unwound back to
	// Resume. The yielding C frame is still L.ci; the next resume
	// finishes it and continues the caller.
	fastYield bool
	// inVM is true while luaV_execute is on the stack. A nested
	// luaD_call from there (a metamethod, a __close) cannot stay on
	// the resumer's goroutine: the yield would have to come back into
	// that Go frame. The call migrates the coroutine instead.
	inVM bool
	// vmNest is how many luaV_execute frames are active. A synchronous
	// yield from the outermost one returns normally; a deeper one has
	// Go frames above it and still unwinds.
	vmNest int

	// lastErr is the error object of a coroutine that ended with an
	// error. C keeps it on the coroutine's own stack (lua_resume leaves
	// it there and luaE_resetthread feeds L->status back through
	// luaD_closeprotected), so 'coroutine.close' can report it again.
	lastErr Value
	hasErr  bool

	// cstate is this state as the `lua_State *` C sees.  The
	// dynamic-loading backend (cdlopen.go, cgo only) fills it in the
	// first time C is handed this state: it is the address of a small
	// malloc'd block -- C never dereferences it, and cgo will not let a
	// Go pointer be left in C memory -- whose one word is an integer
	// naming the state in a weak registry.  It is nil until then, and
	// nil for good in a build without cgo.
	cstate unsafe.Pointer
	// cstateBase is the block 'cstate' was carved out of: 'lua_getextraspace'
	// reaches behind the pointer C was handed, so the malloc'd block starts
	// LUA_EXTRASPACE bytes earlier and that is what has to be freed.
	cstateBase unsafe.Pointer
	// cstateTok is the same block, so Close and the state's cleanup
	// agree on freeing it once.
	cstateTok *cstateBlock

	// The allocator lua_setallocf has set, for lua_getallocf to hand
	// back. Both are C-side only: an extension calls what it is given.
	callocF  unsafe.Pointer
	callocUD unsafe.Pointer
}

// coRuntime is the channel pair that drives a coroutine goroutine.
//
// The channels carry the request and the response as *values*, and the
// value slices are scratch buffers owned by whoever is running: the two
// sides take strict turns -- one is always parked while the other works
// -- so a buffer is never touched by both. That matters because a
// resume/yield pair is already two goroutine switches, roughly 280 ns
// on this hardware, which is the larger part of its cost; allocating a
// request, a response and a slice of values on top of that, twice per
// pair, is the part that can be given back.
type coRuntime struct {
	req  chan resumeReq
	resp chan resumeResp
	done chan struct{} // closed when coMain returns
	vals []Value       // scratch for the values handed to the resumer
	args []Value       // scratch for the arguments of a resume
	// sync means this coroutine still runs on the resumer's goroutine.
	// Pure Lua yield unwinds to Resume instead of parking. A Go body or
	// a call into C promotes the coroutine onto its own goroutine,
	// because that yield has to keep the Go or C stack it returns into.
	sync bool
	// live means coMain is running. Close uses it so a coroutine that
	// was never given a goroutine is not mistaken for one whose
	// goroutine has simply not reached the receive yet.
	live bool
}

func newCoRuntime() *coRuntime {
	return &coRuntime{
		req:  make(chan resumeReq),
		resp: make(chan resumeResp),
		done: make(chan struct{}),
	}
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
	reqResume   = 0
	reqClose    = 1
	reqContinue = 2 // finish a C call that was migrated onto the goroutine
	reqReexec   = 3 // re-execute the current Lua frame on the goroutine
)

// closeSignal unwinds a coroutine being closed by coroutine.close.
type closeSignal struct{}

// yieldAbort unwinds a synchronous yield back to Resume. It is not an
// error: the Lua stack and the call chain stay as they are, and the next
// resume continues them.
type yieldAbort struct{}

// migrateAbort unwinds a synchronous resume back to Resume so the rest
// of it can run on the coroutine's goroutine. reexec means the current
// opcode has to be run again (a nested call had not started); otherwise
// a C frame is already on the stack and only that call has to be made.
type migrateAbort struct {
	reexec bool
}

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

// allocCI gets a CallInfo from the pool or the heap.
func (L *State) allocCI() *CallInfo {
	if n := len(L.ciPool); n > 0 {
		ci := L.ciPool[n-1]
		L.ciPool = L.ciPool[:n-1]
		*ci = CallInfo{}
		return ci
	}
	return new(CallInfo)
}

// luaE_extendCI is luaE_extendCI (lstate.c). C takes err and may return
// NULL on OOM; Go allocation cannot fail (err is ignored). Returns the
// new CI without assigning L.ci — C semantics. next_ci / extendCI then
// advance L.ci for the VM.
func luaE_extendCI(L *State, err bool) *CallInfo {
	_ = err
	ci := L.allocCI()
	ci.next = nil
	if L.ci != nil {
		ci.next = L.ci.next
		ci.prev = L.ci
		L.ci.next = ci
	}
	if ci.next != nil {
		ci.next.prev = ci
	}
	ci.trap = false /* C: ci->u.l.trap = 0 */
	L.nci++
	return ci
}

// extendCI advances L.ci to a new frame (used by next_ci in ldo.go).
func (L *State) extendCI() *CallInfo {
	ci := luaE_extendCI(L, true)
	L.ci = ci
	return ci
}

// freeCI returns a CallInfo to the pool (Go recycle; C luaM_free).
func (L *State) freeCI(ci *CallInfo) {
	*ci = CallInfo{}
	if len(L.ciPool) < 64 {
		L.ciPool = append(L.ciPool, ci)
	}
	L.nci--
}

// freeUnusedCI is C's static freeCI: free every CallInfo after L.ci.
func freeUnusedCI(L *State) {
	if L.ci == nil {
		return
	}
	next := L.ci.next
	L.ci.next = nil
	for next != nil {
		n := next.next
		L.freeCI(next)
		next = n
	}
}

// unwindCIToBase frees every active CallInfo back to baseCI.
// NOT luaE_shrinkCI (that frees half of the *unused* CI tail — lives in
// ldo.go because luaD_shrinkstack calls it; cannot move without editing
// that file). Prior name shrinkCI collided with C.
func (L *State) unwindCIToBase() {
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

// stack_init is stack_init (lstate.c).
func stack_init(L1, L *State) {
	_ = L /* C allocates via L; Go uses the heap */
	L1.reallocStack(BASIC_STACK_SIZE)
	L1.top = 0
	ci := &L1.baseCI
	ci.next, ci.prev = nil, nil
	ci.callstatus = CIST_C
	ci.funcIdx = 0
	ci.cK = nil
	ci.nresults = 0
	setnilvalue(&L1.stack[0]) /* 'function' entry for this 'ci' */
	L1.top = 1
	ci.top = L1.top + LUA_MINSTACK
	L1.ci = ci
}

// freestack is freestack (lstate.c). Frees unused CallInfos after L.ci.
//
// SAFE vs stubbed os.exit: C freestack also frees the stack array, then
// frealloc's the whole LG — and exit() never returns. Tests replace
// osExitHook so Close returns into a live coroutine (see
// TestCloseFromCoroutineDoesNotHang). Releasing L.stack (or a child's)
// there yields "attempt to call a nil value". Only free unused CIs;
// leave the slice for Go GC when the State becomes unreachable.
// close_state also skips freestack entirely when Close was entered from
// a child coroutine (main's live CallInfo must survive resume).
func freestack(L *State) {
	if L.stack == nil {
		return /* stack not completely built yet */
	}
	L.ci = &L.baseCI /* free the entire 'ci' list beyond base */
	freeUnusedCI(L)
}

// preinit_thread is preinit_thread (lstate.c): consistent values, no alloc.
func preinit_thread(L *State, g *globalState) {
	L.g = g
	L.stack = nil
	L.ci = nil
	L.nci = 0
	L.nCcalls = 0
	L.hook = nil
	L.hookmask = 0
	L.basehookcount = 0
	L.allowhook = 1
	L.hookcount = 0 /* resethookcount(L) */
	L.openupval = nil
	L.tbclist = nil
	L.status = LUA_OK
	L.errfunc = 0
	L.oldpc = 0
}

// init_registry is init_registry (lstate.c).
func init_registry(L *State, g *globalState) {
	sethvalue(&g.registry, luaH_new(L))
	luaH_resize(L, hvalue(&g.registry), LUA_RIDX_LAST, 0)
	var th Value
	setthvalue(&th, L)
	luaH_setint(L, hvalue(&g.registry), LUA_RIDX_MAINTHREAD, &th)
	var gl Value
	sethvalue(&gl, luaH_new(L))
	luaH_setint(L, hvalue(&g.registry), LUA_RIDX_GLOBALS, &gl)
}

// f_luaopen is f_luaopen (lstate.c): parts that may allocate.
func f_luaopen(L *State) {
	g := L.g
	stack_init(L, L)
	init_registry(L, g)
	luaS_init(L)
	luaT_init(L)
	luaX_init(L)
	g.gcStopped = false      /* C: g->gcstp = 0; allow gc */
	setnilvalue(&g.nilValue) /* now state is complete */
}

// close_state is close_state (lstate.c) plus Go coroutine shutdown.
// caller is the thread that entered Close (may be a coroutine inside
// os.exit); it is skipped by closeAllThreads and keeps a usable stack
// when osExitHook is stubbed and returns into Lua.
func close_state(L *State, caller *State) {
	g := L.g
	/* Stubbed osExitHook returns into a live coroutine while main is
	   still inside coroutine.resume. C's close_state then exit() never
	   returns; wiping main's CI/top here would leave resume returning
	   into a destroyed CallInfo ("attempt to call a table/nil value"). */
	fromChild := caller != nil && caller != L && !caller.isMain
	if !completestate(g) { /* closing a partially built state? */
		L.closeFinalizers() /* just collect its objects (Go-GC) */
	} else if fromChild {
		/* Close tbc/upvalues without unwinding main's live frames. */
		_ = luaD_closeprotected(L, 0, closeKTOP, false)
		L.closeFinalizers()
	} else { /* closing a fully built state (C close_state shape) */
		L.ci = &L.baseCI                             /* unwind CallInfo list */
		L.errfunc = 0                                /* stack unwind can "throw away" the error function */
		_ = luaD_closeprotected(L, 1, LUA_OK, false) /* close all upvalues */
		L.top = 1                                    /* empty the stack to run finalizers */
		L.closeFinalizers()                          /* Go-GC stand-in for luaC_freeallobjects */
	}
	L.status = LUA_OK
	L.dead = true
	/* Stop child goroutines. With g.closing set, forceClose skips child
	   __close (C does not run child __close on lua_close; a suspended
	   wrap whose __close is os.exit must stay collectable). */
	if g != nil {
		g.closeAllThreads(caller)
	}
	if fromChild {
		caller.dead = true
	}
	if !fromChild {
		freestack(L)
	}
	if releaseCState != nil {
		releaseCState(L)
	}
}

// NewState creates a new Lua state (lua_newstate). May return nil if
// f_luaopen fails (C returns NULL); Go alloc rarely fails.
func NewState() *State {
	g := &globalState{}
	L := &State{}
	preinit_thread(L, g)
	g.mainthread = L
	L.isMain = true
	incnny(L) /* main thread is always non yieldable */
	g.seed = uint32(rand.Uint64())
	atomic.AddUint64(&stateIDSeed, 1)
	g.panic = nil
	/* PUC-Lua declares incremental mode in lua_newstate, but what
	   collectgarbage("generational") reports is isdecGCmodegen, which
	   is also true while lastatomic != 0 — and collections of a freshly
	   opened state are always judged "bad", which sets it. A fresh state
	   therefore answers "generational". */
	g.gcPause = 200   /* LUAI_GCPAUSE */
	g.gcStepMul = 100 /* LUAI_GCMUL */
	g.gcMode = gcModeGen
	g.gcStopped = true        /* C: gcstp = GCSTPGC while building */
	setivalue(&g.nilValue, 0) /* signal state not yet built */
	g.strt = newStringTable()
	if L.rawRunProtected(func() { f_luaopen(L) }) != LUA_OK {
		close_state(L, L)
		return nil
	}
	gcTrackThread(L)
	/* FLAG: C's lua_newstate leaves warnf NULL; luaL_newstate installs
	   the default. This NewState folds that lauxlib last act in. */
	g.setWarnf(warnfOff)
	return L
}

// Close closes a state (lua_close → close_state).
func (L *State) Close() {
	/* os.exit(…, true) and lua_close may run on a coroutine. That
	   caller cannot be asked to exit via co.req — it is the goroutine
	   running Close — so close_state skips it in closeAllThreads. */
	caller := L
	L = L.g.mainthread /* only the main thread can be closed */
	close_state(L, caller)
}

// closeAllThreads wakes every non-main thread of this state so its
// goroutine can exit, before Close frees the malloc'd lua_State
// handles those frames may still name. It rescans until none remain,
// so a '__close' that creates another thread is still caught, and so a
// child that is mid-coRun is retried until it waits or finishes.
//
// skip is the thread that is running Close itself (when Close was
// entered from a coroutine). Sending reqClose to it would deadlock.
func (g *globalState) closeAllThreads(skip *State) {
	if g == nil {
		return
	}
	for {
		var kids []*State
		gcThrMu.Lock()
		for i := range gcThrs {
			th := gcThrs[i].Value()
			if th == nil || th.g != g || th.isMain || th.dead || th == skip {
				continue
			}
			kids = append(kids, th)
		}
		gcThrMu.Unlock()
		if len(kids) == 0 {
			return
		}
		progress := false
		for _, th := range kids {
			if th.forceCloseThread() {
				progress = true
			}
		}
		if !progress {
			runtime.Gosched()
		}
	}
}

// forceCloseThread asks a child coroutine's goroutine to unwind. It
// returns true when the thread is dead afterwards. A running child is
// not waiting on co.req, so the send is skipped and the caller retries;
// a child that has already exited is noticed via co.done so Close cannot
// block forever on a send nobody will read.
func (L *State) forceCloseThread() bool {
	if L == nil || L.isMain || L.dead {
		return true
	}
	co := L.co
	if co == nil {
		L.dead = true
		return true
	}
	// A synchronous coroutine has no goroutine to signal. lua_close does
	// not run a child's __close, so marking it dead is the whole of it.
	// A bare coRuntime with sync still false (a test double) keeps the
	// channel attempt below.
	if co.sync && !co.live {
		L.dead = true
		return true
	}
	select {
	case <-co.done:
		L.dead = true
		return true
	default:
	}
	select {
	case co.req <- resumeReq{kind: reqClose}:
		select {
		case <-co.resp:
		case <-co.done:
		}
		L.dead = true
		return true
	case <-co.done:
		L.dead = true
		return true
	default:
		return false
	}
}

// G returns the global state (G(L)).
func (L *State) G() *globalState { return L.g }

// luaE_setdebt stores debt (lstate.c). Go GC ignores totalbytes; only
// gcDebt is kept for lua_gc / collectgarbage knobs.
func luaE_setdebt(g *globalState, debt int) {
	if g == nil {
		return
	}
	g.gcDebt = debt
}

// SetCStackLimit is lua_setcstacklimit (lstate.c): stub returns LUAI_MAXCCALLS.
func (L *State) SetCStackLimit(limit uint) int {
	_ = L
	_ = limit
	return LUAI_MAXCCALLS
}

// CloseThread is lua_closethread (lstate.c): reset the thread with the
// closer's C-call depth. Coroutine.close / cgo use coCloseThread, which
// also signals the goroutine; this matches C's API shape for direct calls.
func (L *State) CloseThread(from *State) int {
	if from != nil {
		L.nCcalls = uint32(getCcalls(from))
	} else {
		L.nCcalls = 0
	}
	return L.resetThreadStatus(int(L.status))
}

// luaE_freethread is luaE_freethread (lstate.c): close upvalues and free
// the thread's stack/CI. Go GC reclaims the State; freestack is CI-only
// (see freestack comment).
func luaE_freethread(L, L1 *State) {
	_ = L
	luaF_closeupval(L1, 0) /* close all upvalues */
	freestack(L1)
}

// NewThread creates a new coroutine (lua_newthread).
func (L *State) NewThread() *State {
	g := L.g
	L1 := &State{}
	preinit_thread(L1, g)
	L1.isMain = false
	/* Go coroutine status: not-yet-started threads answer LUA_YIELD so
	   coroutine.status says "suspended" (C's lua_status is LUA_OK). */
	L1.status = LUA_YIELD
	L1.started = false
	L1.hookmask = L.hookmask
	L1.basehookcount = L.basehookcount
	L1.hook = L.hook
	L1.hookcount = L.basehookcount /* resethookcount(L1) */
	L1.hookExternal = L.hookExternal
	/* FLAG: C memcpy's LUA_EXTRASPACE from mainthread; Go allocates
	   cstate lazily on first C hand-off, so extraspace inherit is omitted. */
	stack_init(L1, L)
	gcTrackThread(L1)
	L1.co = newCoRuntime()
	L1.co.sync = true
	L.checkStack(1)
	var th Value
	setthvalue(&th, L1)
	L.stack[L.top] = th
	L.top++
	return L1
}

// coMain is the body of a coroutine goroutine.
func (L *State) coMain() {
	co := L.co
	defer close(co.done)
	for {
		req := <-co.req
		if req.kind == reqClose {
			L.status = LUA_OK
			L.dead = true
			co.resp <- resumeResp{status: LUA_OK}
			return
		}
		/* 'coRun' returns only when the coroutine is through: a yield
		   parks the goroutine inside (*State).Yield, which sends the
		   resume response itself and waits there for the next resume
		   request. A continuation request finishes the C call that was
		   migrated here, then the Lua that called it. */
		var status int
		var closed bool
		if req.kind == reqContinue {
			L.nCcalls = uint32(req.ccalls)
			status, closed = L.finishMigrated()
		} else if req.kind == reqReexec {
			L.nCcalls = uint32(req.ccalls)
			status, closed = L.reexec()
		} else {
			status, closed = L.coRun(req.args, req.ccalls)
		}
		L.dead = true
		if closed {
			/* luaE_resetthread leaves the thread with status LUA_OK,
			   so the error from '__close' is reported only once: a
			   second 'coroutine.close' succeeds. */
			L.status = LUA_OK
			co.resp <- resumeResp{status: status, vals: L.co.vals}
			return
		}
		L.status = byte(status)
		if status != LUA_OK && len(L.co.vals) > 0 {
			L.lastErr = L.co.vals[0]
			L.hasErr = true
		}
		co.resp <- resumeResp{status: status, vals: L.co.vals}
		return
	}
}

// coRun runs the coroutine body with the given arguments. The results
// are left in L.co.vals, the coroutine's scratch slice. The second
// result tells whether the run ended because the thread was closed.
func (L *State) coRun(args []Value, ccalls int) (status int, closed bool) {
	status = LUA_OK
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case *yieldAbort, *migrateAbort:
				panic(x)
			case *closeSignal:
				/* C's lua_closethread -> luaE_resetthread: unwind the
				   CallInfo list and close every to-be-closed variable
				   still pending on the coroutine's stack.  A '__close'
				   that raises must not stop the others: C keeps going
				   and reports the *newest* error.

				   When the whole state is closing, C does not run child
				   __close at all — skip luaD_closeprotected so a
				   suspended wrap whose __close is os.exit is not run. */
				closed = true
				if L.g.closing {
					status = LUA_OK
					L.top = 1
					L.co.vals = L.co.vals[:0]
					return
				}
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
				st = luaD_closeprotected(L, 1, st, false)
				if st != LUA_OK {
					status = st
					luaD_seterrorobj(L, st, 1)
					L.co.vals = append(L.co.vals[:0], L.stack[1])
				} else {
					status = LUA_OK
					L.top = 1
					/* no results at all: 'L.co.vals' is a scratch slice
					   that the last yield filled, and a stale value
					   handed to the resumer would be a real one. */
					L.co.vals = L.co.vals[:0]
				}
				L.baseCI.top = L.top + LUA_MINSTACK /* luaE_resetthread */
				return
			case *luaError:
				status = x.status
				L.co.vals = append(L.co.vals[:0], x.value)
				return
			default:
				status = LUA_ERRRUN
				var v Value
				setsvalue(&v, luaS_newlstr(L, fmt.Sprintf("%v", r)))
				L.co.vals = append(L.co.vals[:0], v)
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
	L.nCcalls = uint32(ccalls)
	if L.top == 1 {
		// function was reset: use the pending function
		L.stack[0] = L.prevFunc
	}
	luaD_call(L, 0, LUA_MULTRET)
	L.co.vals = append(L.co.vals[:0], L.stack[:L.top]...)
	return status, false
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
	status = luaD_closeprotected(L, 1, status, false)
	if status != LUA_OK { /* errors? */
		luaD_seterrorobj(L, status, 1)
	} else {
		L.top = 1
	}
	ci.top = L.top + LUA_MINSTACK
	luaD_reallocstack(L, ci.top, false) /* C: luaD_reallocstack(L, ci->top - stack, 0) */
	return status
}

// ResetThread resets a coroutine so that it can be reused.
// Not C's deprecated lua_resetthread (= lua_closethread); that is CloseThread /
// coCloseThread. This rebuilds the goroutine machinery for reuse.
func (L *State) ResetThread() int {
	if L.isMain {
		return LUA_ERRRUN
	}
	if !L.dead && L.status == LUA_YIELD && L.co != nil {
		if L.co.live {
			// still suspended on its goroutine: unwind it there
			select {
			case L.co.req <- resumeReq{kind: reqClose}:
				select {
				case <-L.co.resp:
				case <-L.co.done:
				}
			case <-L.co.done:
			}
		} else {
			// synchronous, and nothing is listening on the channel
			_ = L.unwindForClose()
		}
	}
	L.fastYield = false
	L.dead = false
	L.hasErr = false
	L.lastErr = Value{}
	L.started = false
	L.status = LUA_YIELD
	L.top = 1
	L.ci = &L.baseCI
	L.baseCI.next = nil
	L.tbclist = nil
	L.openupval = nil
	L.errfunc = 0
	L.nCcalls = 0
	L.co = newCoRuntime()
	L.co.live = true
	go L.coMain()
	return LUA_OK
}

// Resume resumes a coroutine with nargs values on top of L.
func (L *State) Resume(from *State, nargs int) int {
	co := L
	if co.isMain {
		return resumeError(co, "cannot resume non-suspended coroutine")
	}
	switch co.status {
	case LUA_OK:
		if co.dead {
			return resumeError(co, "cannot resume dead coroutine")
		}
		return resumeError(co, "cannot resume non-suspended coroutine")
	case LUA_YIELD:
	default:
		return resumeError(co, "cannot resume dead coroutine")
	}
	if !co.started {
		/* Starting the coroutine. C finds the body at 'ci->func + 1',
		   which is the first slot above the base frame -- where
		   lua_newthread plus a push (or coroutine.create's xmove) put
		   it -- and calls it; here it has to end up in slot 0, which is
		   where coRun looks for it. */
		if co.top < 1 {
			co.top = 1
		}
		/* The arguments sit above the body, because that is where
		   C's auxresume puts them (lua_xmove(L, co, narg)) and where
		   an extension that calls lua_resume itself puts them. */
		fnIdx := co.top - nargs - 1
		var fn Value
		if fnIdx >= 0 {
			fn = co.stack[fnIdx]
		}
		co.co.args = append(co.co.args[:0], co.stack[fnIdx+1:co.top]...)
		co.prevFunc = fn
		co.stack[0] = fn
		co.top = 1
		co.started = true
	} else {
		/* resuming from a yield: the arguments are on top of the
		   coroutine's own stack, and they leave it as they are taken */
		co.co.args = append(co.co.args[:0], co.stack[co.top-nargs:co.top]...)
		co.top -= nargs
	}
	co.parent = from
	co.status = LUA_OK
	ccalls := 0
	if from != nil {
		ccalls = getCcalls(from)
	}
	if ccalls >= LUAI_MAXCCALLS {
		/* C's resume_error: message goes on the resumed thread */
		return resumeError(co, "C stack overflow")
	}
	var resp resumeResp
	if co.co.sync {
		resp = co.resumeSync(ccalls)
	} else {
		co.co.req <- resumeReq{kind: reqResume, args: co.co.args, ccalls: ccalls}
		resp = <-co.co.resp
	}
	co.status = byte(resp.status)
	if resp.status == LUA_OK || resp.status == LUA_YIELD {
		/* C's lua_resume leaves results on the coroutine. The caller
		   (auxresume / lua_xmove) moves them. Yield took them off this
		   stack; a normal return left them on slot 0 (the body was
		   called there), which would make lua_gettop under-count — lift
		   them one slot above the base CI when they already fill [0:n].
		   Only one extra slot is needed; checkStack(n+1) would ask for
		   another full copy of a large MULTRET and abort. */
		n := len(resp.vals)
		if resp.status == LUA_YIELD {
			co.checkStack(n)
			copy(co.stack[co.top:], resp.vals)
			co.top += n
		} else if n > 0 && co.top == n && co.ci != nil && co.ci.funcIdx == 0 {
			co.checkStack(1)
			copy(co.stack[1:1+n], resp.vals)
			setnilvalue(&co.stack[0])
			co.top = 1 + n
		}
		return int(resp.status)
	}
	/* error object on the coroutine, as C's lua_resume does. Do not
	   rewrite the stack: a pending <close> variable may still sit in a
	   lower slot and must survive until coroutine.close / resetthread. */
	if len(resp.vals) > 0 {
		if co.top == 0 || !luaV_rawequalobj(&co.stack[co.top-1], &resp.vals[0]) {
			co.checkStack(1)
			co.stack[co.top] = resp.vals[0]
			co.top++
		}
		co.co.vals = append(co.co.vals[:0], resp.vals[0])
	} else if co.co != nil {
		co.co.vals = co.co.vals[:0]
	}
	return resp.status
}

// resumeError is 'resume_error' (ldo.c): it pushes the message on the
// resumed thread and reports LUA_ERRRUN. co.vals is set so lua_resume's
// nresults out-parameter is not left holding a prior successful count.
// The stack below top is left alone so a dead coroutine's pending
// to-be-closed slots are not destroyed.
func resumeError(L *State, msg string) int {
	if L == nil {
		return LUA_ERRRUN
	}
	v := strValue(L, msg)
	L.checkStack(1)
	L.stack[L.top] = v
	L.top++
	if L.co != nil {
		L.co.vals = append(L.co.vals[:0], v)
	}
	return LUA_ERRRUN
}

// migrateNested moves a synchronous coroutine onto its goroutine and
// re-executes the opcode that was about to do something a yield must
// come back into. The opcode has already saved the next pc; stepping
// back one lands on the opcode itself. No effect once the coroutine
// has been promoted, or outside the interpreter.
func (L *State) migrateNested() {
	if L.co == nil || !L.co.sync || !L.inVM {
		return
	}
	if ci := L.ci; ci != nil && isLua(ci) && ci.savedpc > 0 {
		ci.savedpc--
	}
	panic(&migrateAbort{reexec: true})
}

// ensureAsync moves a synchronous coroutine onto its own goroutine.
// The resumer is blocked in Resume, so the goroutine can take the next
// request without the two of them running the body at once.
func (L *State) ensureAsync() {
	if L.co == nil || !L.co.sync {
		return
	}
	L.co.sync = false
	if !L.co.live {
		L.co.live = true
		go L.coMain()
	}
}

// prepareFastYield suspends a synchronous coroutine and returns. The
// yielding C frame stays current; the interpreter returns to Resume
// instead of finishing the call. fastYieldNow is the same suspension
// when Go frames above the interpreter have to be unwound.
func (L *State) prepareFastYield(nresults int) {
	if L.isMain || L.co == nil {
		luaG_runerror(L, "attempt to yield from outside a coroutine")
	}
	if !yieldable(L) {
		luaG_runerror(L, "attempt to yield across a C-call boundary")
	}
	L.nYields++
	if nresults > 0 {
		L.co.vals = append(L.co.vals[:0], L.stack[L.top-nresults:L.top]...)
	} else {
		L.co.vals = L.co.vals[:0]
	}
	L.top -= nresults
	if L.top < 0 {
		L.top = 0
	}
	L.fastYield = true
}

// fastYieldNow suspends a synchronous coroutine. It does not return:
// Resume's recover continues the caller on the next resume.
func (L *State) fastYieldNow(nresults int) {
	L.prepareFastYield(nresults)
	panic(&yieldAbort{})
}

// unroll finishes whatever call is current, the way ldo.c's unroll does
// after a yield. A Lua frame is executed. A Go frame with a continuation
// runs that; any other Go frame is finished with the values already on
// the stack. It returns when the coroutine has yielded again or when
// only the base frame is left.
func (L *State) unroll() {
	for {
		ci := L.ci
		if ci == nil || ci == &L.baseCI {
			return
		}
		if isLua(ci) {
			luaV_execute(L, ci)
			if L.fastYield {
				return
			}
			continue
		}
		var n int
		if ci.goCont != nil {
			n = ci.goCont(L, LUA_OK)
			ci.goCont = nil
		} else {
			n = L.top - ci.funcIdx - 1
			if n < 0 {
				n = 0
			}
		}
		luaD_poscall(L, ci, n)
	}
}

// resumeSync runs one synchronous resume. A yield comes back as
// LUA_YIELD with the values in co.vals; entering C migrates the rest of
// the resume onto the coroutine goroutine and returns that goroutine's
// response.
func (L *State) resumeSync(ccalls int) (resp resumeResp) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch x := r.(type) {
		case *yieldAbort:
			resp.status = LUA_YIELD
			resp.vals = L.co.vals
		case *migrateAbort:
			if x.reexec {
				resp = L.reexecOnGoroutine(ccalls)
			} else {
				resp = L.continueOnGoroutine(ccalls)
			}
		default:
			st, _ := L.recoverCo(r)
			L.dead = true
			if st != LUA_OK && len(L.co.vals) > 0 {
				L.lastErr = L.co.vals[0]
				L.hasErr = true
			}
			L.status = byte(st)
			resp.status = st
			resp.vals = L.co.vals
		}
	}()
	if !L.fastYield && (L.hookmask != 0 || !ttisLclosure(&L.stack[0])) {
		// A Go body has to keep its stack across a yield. A hooked Lua
		// body also stays asynchronous: hook calls expose every frame
		// and saved program counter, which the synchronous unwind does
		// not preserve.
		resp = L.promoteBody(ccalls)
		return
	}
	if L.fastYield {
		L.fastYield = false
		L.nCcalls = uint32(ccalls + 1)
		narg := len(L.co.args)
		L.checkStack(narg)
		copy(L.stack[L.top:], L.co.args)
		L.top += narg
		L.unroll()
		if L.fastYield {
			resp.status = LUA_YIELD
			resp.vals = L.co.vals
			return
		}
		L.dead = true
		L.co.vals = append(L.co.vals[:0], L.stack[:L.top]...)
		resp.status = LUA_OK
		resp.vals = L.co.vals
		return
	}
	n := len(L.co.args)
	L.checkStack(n + 1)
	copy(L.stack[1:], L.co.args)
	L.top = 1 + n
	L.nCcalls = uint32(ccalls)
	luaD_call(L, 0, LUA_MULTRET)
	if L.fastYield {
		resp.status = LUA_YIELD
		resp.vals = L.co.vals
		return
	}
	L.dead = true
	L.co.vals = append(L.co.vals[:0], L.stack[:L.top]...)
	resp.status = LUA_OK
	resp.vals = L.co.vals
	return
}

// promoteBody runs a Go coroutine body on its own goroutine. The
// arguments are already in co.args and the function is in stack[0],
// which is where coRun expects them.
func (L *State) promoteBody(ccalls int) resumeResp {
	L.ensureAsync()
	L.co.req <- resumeReq{kind: reqResume, args: L.co.args, ccalls: ccalls}
	return <-L.co.resp
}

// reexecOnGoroutine runs the current Lua frame again on the coroutine
// goroutine. ccall rewound savedpc to the opcode that wanted the nested
// call, so the opcode and the call both happen where a yield can park.
func (L *State) reexecOnGoroutine(ccalls int) resumeResp {
	L.ensureAsync()
	L.co.req <- resumeReq{kind: reqReexec, ccalls: ccalls}
	return <-L.co.resp
}

// reexec is the goroutine side of reexecOnGoroutine.
func (L *State) reexec() (status int, closed bool) {
	defer func() {
		if r := recover(); r != nil {
			status, closed = L.recoverCo(r)
		}
	}()
	L.nCcalls++
	if L.ci != nil && isLua(L.ci) {
		luaV_execute(L, L.ci)
	}
	L.co.vals = append(L.co.vals[:0], L.stack[:L.top]...)
	return LUA_OK, false
}

// continueOnGoroutine retries the C call that asked to migrate, on the
// coroutine goroutine, and waits for that resume to yield or finish.
func (L *State) continueOnGoroutine(ccalls int) resumeResp {
	L.ensureAsync()
	L.co.req <- resumeReq{kind: reqContinue, ccalls: ccalls}
	return <-L.co.resp
}

// finishMigrated runs the C frame a synchronous resume had already
// pushed, then the Lua that called it. Yields inside it park this
// goroutine the ordinary way.
func (L *State) finishMigrated() (status int, closed bool) {
	defer func() {
		if r := recover(); r != nil {
			status, closed = L.recoverCo(r)
		}
	}()
	// The migrating ccall had already counted this C frame.
	L.nCcalls++
	ci := L.ci
	f := ci.migrateFunc
	if f == nil {
		f = clCvalue(&L.stack[ci.funcIdx]).f
	}
	ci.migrateFunc = nil
	n := f(L)
	luaD_poscall(L, ci, n)
	L.unroll()
	L.co.vals = append(L.co.vals[:0], L.stack[:L.top]...)
	return LUA_OK, false
}

// recoverCo is the panic recover shared by the coroutine runner. A
// synchronous yield or a migration is not its business: those have to
// reach Resume, so they are re-panicked.
func (L *State) recoverCo(r any) (status int, closed bool) {
	switch x := r.(type) {
	case *yieldAbort, *migrateAbort:
		panic(x)
	case *closeSignal:
		closed = true
		if L.g.closing {
			status = LUA_OK
			L.top = 1
			L.co.vals = L.co.vals[:0]
			return
		}
		return L.unwindForClose(), true
	case *luaError:
		status = x.status
		L.co.vals = append(L.co.vals[:0], x.value)
		return
	default:
		status = LUA_ERRRUN
		var v Value
		setsvalue(&v, luaS_newlstr(L, fmt.Sprintf("%v", r)))
		L.co.vals = append(L.co.vals[:0], v)
		return
	}
}

// unwindForClose closes a synchronous coroutine in place. It is what
// the goroutine's closeSignal branch does, without the goroutine.
func (L *State) unwindForClose() int {
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
	L.status = LUA_OK
	st = luaD_closeprotected(L, 1, st, false)
	if st != LUA_OK {
		luaD_seterrorobj(L, st, 1)
		L.co.vals = append(L.co.vals[:0], L.stack[1])
		L.baseCI.top = L.top + LUA_MINSTACK /* luaE_resetthread */
		return st
	}
	L.top = 1
	L.co.vals = L.co.vals[:0]
	L.baseCI.top = L.top + LUA_MINSTACK /* luaE_resetthread */
	return LUA_OK
}

// Yield yields nresults values (lua_yieldk / lua_yield / luaB_yield), and
// answers how many values the yield returns: what the resumer passed to
// 'coroutine.resume' when the frame has no continuation, and what the
// continuation pushed when it has one.
//
// C's 'lua_yieldk' throws, and 'lua_resume' picks the coroutine up again
// where it stopped.  This port instead parks the coroutine's goroutine
// right here: the yielded values go to the resumer through 'co.resp', and
// the goroutine then waits for the next resume request.  Parking keeps
// the Lua stack, the saved program counter and -- for a C caller -- the C
// stack itself alive at the yield point, so on resume execution simply
// goes on where it left off.  That is also what makes a continuation
// possible: the C frame that asked for one is still there to receive the
// values the continuation leaves for it.
func (L *State) Yield(nresults int) int {
	if L.isMain || L.co == nil {
		/* C's lua_yieldk: 'luaG_runerror', so no source:line prefix */
		luaG_runerror(L, "attempt to yield from outside a coroutine")
		return 0 /* not reached */
	}
	if !yieldable(L) {
		luaG_runerror(L, "attempt to yield across a C-call boundary")
		return 0 /* not reached */
	}
	ci := L.ci
	L.nYields++
	L.co.vals = append(L.co.vals[:0], L.stack[L.top-nresults:L.top]...)
	/* The yielded values leave the stack, because that is what C does:
	   'auxresume' hands them to the resumer with 'lua_xmove', which
	   takes them off this thread's stack.  A continuation that counts
	   with 'lua_gettop' -- PUC-Lua's own 'dofilecont' does -- must see
	   the same stack here as there (test 'v2' below pins it). */
	L.top -= nresults
	L.co.resp <- resumeResp{status: LUA_YIELD, vals: L.co.vals}
	req := <-L.co.req /* wait for the next resume (or a close) */
	if req.kind == reqClose {
		panic(&closeSignal{}) /* unwind: the coroutine is being closed */
	}
	/* C's lua_resume runs on every resume, so the thread takes the C-call
	   depth of whoever resumed it *now*: the resumer may be deeper in the
	   stack than it was when this coroutine started.  'coRun' only runs
	   once, so the depth is refreshed here. */
	L.nCcalls = uint32(req.ccalls + 1)
	narg := len(req.args)
	L.checkStack(narg)
	copy(L.stack[L.top:], req.args)
	L.top += narg
	if ci.cK != nil {
		/* C's lua_resume: the continuation takes over from the yield,
		   and what it returns are the results of the function that
		   yielded. */
		return L.runContinuation(ci, LUA_YIELD)
	}
	return narg /* the resume arguments are the results of the yield */
}

// callContinuation calls the C continuation the frame saved, the way
// ldo.c's 'finishCcall' does, and answers how many values it pushed.
// Only cdlopen.go -- the cgo half -- can call a C function pointer, so
// that is where the real one is installed; no other build can ever see a
// frame with a continuation.
var callContinuation func(L *State, ci *CallInfo, status int) int

func (L *State) runContinuation(ci *CallInfo, status int) int {
	if callContinuation == nil {
		panic("lua: a continuation was saved without the C API")
	}
	return callContinuation(L, ci, status)
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
			/* The error object was copied into the panic because a
			   Go defer between the throw and here — replPrompt's
			   SetTop, for one — can pop it off the Lua stack before
			   luaD_seterrorobj reads it. C's longjmp does not run
			   those defers, so the object is still on top. Put the
			   copy back; an object that was never popped is then
			   on top twice, and seterrorobj keeps the top one. */
			if L.top >= len(L.stack) {
				L.reallocStack(len(L.stack) + EXTRA_STACK)
			}
			L.stack[L.top] = x.value
			L.top++
		case *closeSignal, *yieldAbort, *migrateAbort:
			panic(x)
		default:
			/* A Go panic -- an index out of range, a failed type
			   assertion -- is a bug in this port, not something a Lua
			   program can cause, but it still has to come out as a Lua
			   error, and one that says what happened: an empty message
			   hides the only clue there is.

			   The message goes on the stack as well as into the panic,
			   because that is where the next 'luaD_seterrorobj' looks
			   for it: C's 'luaG_runerror' pushes before it throws, and
			   nothing else here does. */
			if L.top >= len(L.stack) {
				L.reallocStack(len(L.stack) + EXTRA_STACK)
			}
			var v Value
			setsvalue(&v, luaS_newlstr(L, fmt.Sprintf("%v", r)))
			L.stack[L.top] = v
			L.top++
			status = LUA_ERRRUN
		}
	}()
	f()
	return LUA_OK
}

// ---------------------------------------------------------------------
// warnings (lstate.c)
// ---------------------------------------------------------------------

// luaE_warning emits one piece of a warning (luaE_warning).
//
// The piece is a C string in PUC-Lua: 'lua_warning' takes a
// 'const char *', and where a warning ends up is a '%s' or a 'strcmp'
// inside one of the three functions below.  Both stop at the first
// zero, so 'warn("a\0b")' warns "a" and 'warn("@on\0x")' turns the
// warnings on — the control is compared as a C string too.
func luaE_warning(L *State, msg string, tocont bool) {
	g := L.g
	if g.warnf != nil {
		g.warnf(L, luaCStr(msg), tocont)
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

// luaE_warnerror generates a warning from an error message (lstate.c).
// Produces "error in %s (%s)" from where + the string on top of the stack.
func luaE_warnerror(L *State, where string) {
	errobj := &L.stack[L.top-1] /* error object */
	msg := "error object is not a string"
	if ttisstring(errobj) {
		msg = tsvalue(errobj).val
	}
	luaE_warning(L, "error in ", true)
	luaE_warning(L, where, true)
	luaE_warning(L, " (", true)
	luaE_warning(L, msg, true)
	luaE_warning(L, ")", false)
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
