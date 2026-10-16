// Translation of the parts of lgc.c that Go's collector cannot express.
//
// The port has no collector of its own: objects live on the Go heap and
// die when nothing refers to them any more, which is what lets the
// interpreter be a plain Go library.  Two things PUC-Lua's collector does
// are nevertheless observable from a Lua program, and Go's collector does
// not do them.  This file supplies them.
//
//   - '__gc' metamethods (here).  'runtime.SetFinalizer' tells us that an
//     object has become unreachable, but the function it runs does so on
//     another goroutine, where Lua code cannot be called -- a Lua state
//     is not safe to touch from more than one goroutine.  So the Go
//     finalizer only queues the metamethod, and the VM runs the queue at
//     a point where calling back into Lua is safe.  PUC-Lua runs
//     finalizers from its collector step, which an allocation triggers;
//     the queue is drained in exactly the same places.
//
//   - weak tables (lgc_weak.go).
//
// What Go's collector cannot be told is *when* to run any of this.  It
// collects whenever the heap grows, so an object would be finalised at a
// moment of its own choosing and '__gc' would run a different number of
// times than it does in C.  The objects are therefore held (see 'gcHold')
// until the interpreter decides it is time for a cycle: a collector step,
// 'collectgarbage', or the closing of the state.
package lua

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"weak"
)

// gcFinalizer is an '__gc' metamethod waiting to be run: Go has found the
// object unreachable, and the interpreter has to be told about it on the
// thread that owns the state.
type gcFinalizer struct {
	L   *State
	obj Value // the object; keeping it alive is what Lua calls resurrection
}

var (
	gcMu      sync.Mutex
	gcQueue   []gcFinalizer
	gcWaiting atomic.Int32 // len(gcQueue), so that the hot path needs no lock
	gcWatched atomic.Int32 // objects being watched for '__gc'
)

// gcStepDebt is how many table allocations PUC-Lua would have paid for
// with a collector step.  A program that never sets '__gc' never pays:
// 'gcWatched' is zero and the counter is never even read.
const gcStepDebt = 65536

// gcWatchList is every object that is being watched for '__gc', held
// weakly.  PUC-Lua keeps such objects on a list of its own ('finobj'),
// which does not keep them alive; a weak reference is the only way to
// say the same thing here.  The list is what lets 'lua_close' find them
// again: C's 'separatetobefnz(g, 1)' finalises every object that has a
// '__gc' metamethod, reachable or not.
var (
	gcWatchMu sync.Mutex
	gcWatch   []weakRef
)

// gcThrs is every thread of every state, held weakly for the same
// reason.  See gcClearStacks.
var (
	gcThrMu sync.Mutex
	gcThrs  []weak.Pointer[State]
)

// gcTrackThread notes a new thread (threads are created in lstate.go).
func gcTrackThread(L *State) {
	gcThrMu.Lock()
	gcThrs = append(gcThrs, weak.Make(L))
	gcThrMu.Unlock()
}

// gcClearStacks empties the stack slots above the top of every thread.
//
// Go's collector scans the whole stack slice, while Lua's only ever sees
// the slots below 'top'.  What sits above 'top' is garbage as far as the
// program is concerned -- a slot a call has left behind still holds the
// value it had -- but Go cannot know that, and one stale reference is
// enough to keep an object alive forever.  It is worse for a coroutine
// than for the main thread: a coroutine that has finished leaves its
// whole stack behind, and everything that stack ever touched stays
// reachable until the coroutine itself goes.
//
// The slots are cleared so that Go's idea of what is reachable matches
// Lua's.
func gcClearStacks() {
	gcThrMu.Lock()
	old := gcThrs
	gcThrs = nil
	gcThrMu.Unlock()
	keep := make([]weak.Pointer[State], 0, len(old))
	for i := range old {
		th := old[i].Value()
		if th == nil { /* the thread itself is gone */
			continue
		}
		for j := th.top; j < len(th.stack); j++ {
			setnilvalue(&th.stack[j])
		}
		keep = append(keep, old[i])
	}
	gcThrMu.Lock()
	gcThrs = append(gcThrs, keep...)
	gcThrMu.Unlock()
}

// gcPruneWatch drops the list entries whose object has gone.  Without it
// the list would grow for as long as the program runs.
func gcPruneWatch() {
	gcWatchMu.Lock()
	old := gcWatch
	gcWatch = nil
	gcWatchMu.Unlock()
	keep := make([]weakRef, 0, len(old))
	for _, w := range old {
		if w.dead() {
			continue /* collected, and so already finalised */
		}
		keep = append(keep, w)
	}
	gcWatchMu.Lock()
	gcWatch = append(gcWatch, keep...)
	gcWatchMu.Unlock()
}

// gcWatchEmpty says whether any object is being watched at all.  A
// program that never sets '__gc' never has to pay for a collection when
// its state closes.
func gcWatchEmpty() bool {
	gcWatchMu.Lock()
	n := len(gcWatch)
	gcWatchMu.Unlock()
	return n == 0
}

// gcWaitFinalizers waits for the Go finalizer goroutine to hand over what
// it has found.  'runtime.GC' returns as soon as the sweep is over, which
// can be before the queue is filled.
//
// Only 'closeFinalizers' can afford to wait.  A collector step must not:
// a script that calls 'collectgarbage()' in a loop -- 'calls.lua' does,
// hundreds of times, from a reader function -- would pay milliseconds
// every time for a queue that is empty simply because there was nothing
// to finalise.  What turns up after the step is picked up at the next
// allocation anyway (see the drain in lvm.go).
func gcWaitFinalizers() {
	for i := 0; i < 2 && !gcHasPending(); i++ {
		runtime.GC()
		runtime.Gosched()
	}
	for i := 0; i < 5 && !gcHasPending(); i++ {
		time.Sleep(time.Millisecond)
	}
}

// gcHold keeps every object that is being watched for '__gc' reachable
// until the interpreter says it is time for a collection.
//
// Go's collector runs whenever the heap grows, and an object that only
// Lua's collector would have freed would then be finalised at a moment
// of Go's choosing -- so '__gc' would run a different number of times,
// at different times, than it does in C.  Holding the objects is what
// makes the metamethod run where PUC-Lua runs it: at a collector step,
// at 'collectgarbage', and when the state closes.
var gcHold []Value

// gcRelease lets the held objects go, so that Go can see which of them
// are really unreachable.
func gcRelease() { gcHold = nil }

// gcRehold takes them back, for the time until the next cycle.
func gcRehold() {
	gcWatchMu.Lock()
	watch := gcWatch
	gcWatchMu.Unlock()
	hold := make([]Value, 0, len(watch))
	for _, w := range watch {
		if w.dead() { /* collected, and so already finalised */
			continue
		}
		hold = append(hold, w.value())
	}
	gcHold = hold
}

// gcCycle is where PUC-Lua would have finished a collection: the objects
// held for '__gc' are let go, the stale stack slots are cleared, Go
// collects, and the metamethods of what has become unreachable run.
func (L *State) gcCycle() {
	gcPruneWatch()
	gcRelease()
	gcClearStacks() /* see gcClearStacks: stale slots keep objects alive */
	L.gcSweepWeak()
	runtime.GC() /* lets Go find the unreachable objects */
	L.runFinalizers()
	gcRehold()
}

// gcStep is what PUC-Lua's 'luaC_checkGC' does after an allocation: it
// runs a collector step, and with it any finalizer that is due.  Here the
// step is Go's own collector, which is what finds objects unreachable and
// so is what queues the metamethods.  It is only ever run while some
// object is being watched for '__gc', and at most once every
// 'gcStepDebt' allocations.
func (L *State) gcStep() {
	if gcWatched.Load() == 0 && gcWeakCount.Load() == 0 {
		return
	}
	if L.g.gcStopped { /* 'collectgarbage("stop")' */
		return
	}
	L.g.gcDebt++
	if L.g.gcDebt < gcStepDebt {
		return
	}
	L.g.gcDebt = 0
	L.gcCycle()
}

// gcQueueFinalizer is the Go finalizer: it runs on the finalizer
// goroutine, so all it may do is hand the metamethod over to the state
// that owns it.  Keeping 'obj' in the queue revives the object, which is
// what makes it reachable from the metamethod.
func gcQueueFinalizer(it gcFinalizer) {
	gcMu.Lock()
	gcQueue = append(gcQueue, it)
	gcMu.Unlock()
	gcWaiting.Add(1)
}

// gcHasPending tells whether a finalizer is waiting.  It is read on the
// allocation path, so it must stay cheap.
func gcHasPending() bool { return gcWaiting.Load() > 0 }

// gcTakePending moves the queue out.  Only the thread running Lua calls
// it, so the items it returns can be called on safely.
func gcTakePending() []gcFinalizer {
	gcMu.Lock()
	items := gcQueue
	gcQueue = nil
	gcMu.Unlock()
	gcWaiting.Store(0)
	return items
}

// fullCollect runs a complete collection, the way
// 'collectgarbage("collect")' does: the slots above the stack top are
// cleared (see gcStep), weak tables are swept, and the '__gc' metamethods
// of what has become unreachable are run.
func (L *State) fullCollect() {
	L.g.gcDebt = 0
	L.gcCycle()
}

// closeFinalizers is 'luaC_freeallobjects' (lgc.c): when a state closes,
// the '__gc' metamethod of every object that has one runs, whether the
// object is still reachable or not.
//
// 'closing' is C's 'gcstp |= GCSTPCLS'.  It is what stops a '__gc' that
// remarks its own object -- the trick 'tracegc' uses to be told about
// every collection -- from keeping 'callallpendingfinalizers' looping
// forever: while the state is closing, 'setmetatable' cannot register a
// new finalizer.
func (L *State) closeFinalizers() {
	if L.g.closing {
		return
	}
	L.g.closing = true
	if gcWatchEmpty() {
		L.runFinalizers() /* nothing watched: drain whatever Go queued */
		return
	}
	gcPruneWatch()
	gcRelease() /* nothing is held any more: the state is going away */
	gcClearStacks()
	runtime.GC()       /* lets Go find the unreachable objects */
	gcWaitFinalizers() /* nothing more will allocate: wait for the queue */
	L.runFinalizers()
	/* What is still reachable was never queued, so it is found the way
	   C finds it: by walking the list of watched objects. */
	gcWatchMu.Lock()
	watch := gcWatch
	gcWatch = nil
	gcWatchMu.Unlock()
	for _, w := range watch {
		if w.dead() { /* gone: Go has already queued its metamethod */
			continue
		}
		v := w.value()
		if !gcIsWatched(&v) { /* already finalised */
			continue
		}
		gcUnwatch(&v)
		L.callFinalizer(v)
	}
	L.runFinalizers() /* a '__gc' may have queued more */
}

// gcIsWatched says whether an object still waits for its '__gc'.
func gcIsWatched(v *Value) bool {
	switch ttypetag(v) {
	case LUA_VTABLE:
		return hvalue(v).finalized
	case LUA_VUSERDATA:
		return uvalue(v).finalized
	}
	return false
}

// gcMetatable is the metatable of a collectable object.
func gcMetatable(v *Value) *Table {
	switch ttypetag(v) {
	case LUA_VTABLE:
		return hvalue(v).metatable
	case LUA_VUSERDATA:
		return uvalue(v).metatable
	}
	return nil
}

// luaC_checkFinalizer is 'luaC_checkfinalizer' (lgc.c): an object that
// gets a metatable with a '__gc' field starts to be watched, so that the
// metamethod runs when the object becomes unreachable.
func luaC_checkFinalizer(L *State, v *Value, mt *Table) {
	if mt == nil || L.g.closing { /* 'g->gcstp & GCSTPCLS': closing state? */
		return
	}
	tm := luaH_getshortstr(mt, L.g.tmNames[TM_GC])
	if !ttisfunction(tm) { /* no finalizer (or not callable) */
		return
	}
	switch ttypetag(v) {
	case LUA_VTABLE:
		t := hvalue(v)
		if t.finalized { /* already watched: Go runs each finalizer once */
			return
		}
		t.finalized = true
		gcWatched.Add(1)
		runtime.SetFinalizer(t, func(o *Table) {
			gcQueueFinalizer(gcFinalizer{L: L, obj: makeTableValue(o)})
		})
		gcWatchMu.Lock()
		gcWatch = append(gcWatch, weakTableRef{weak.Make(t)})
		gcWatchMu.Unlock()
		gcHold = append(gcHold, *v) /* see gcHold */
	case LUA_VUSERDATA:
		u := uvalue(v)
		if u.finalized {
			return
		}
		u.finalized = true
		gcWatched.Add(1)
		runtime.SetFinalizer(u, func(o *Udata) {
			gcQueueFinalizer(gcFinalizer{L: L, obj: makeUdataValue(o)})
		})
		gcWatchMu.Lock()
		gcWatch = append(gcWatch, weakUdataRef{weak.Make(u)})
		gcWatchMu.Unlock()
		gcHold = append(gcHold, *v) /* see gcHold */
	}
}

// runFinalizers drains the queue, calling each metamethod with its
// object.  It is called where PUC-Lua runs a collector step.
func (L *State) runFinalizers() {
	if !gcHasPending() {
		return
	}
	items := gcTakePending()
	for i := range items {
		it := &items[i]
		if it.L.g != L.g { /* another state: leave it for its own thread */
			gcQueueFinalizer(*it)
			continue
		}
		/* C takes the object out of the "to be finalized" list before it
		   calls the metamethod ('udata2finalize' clears FINALIZEDBIT), so
		   that '__gc' can put it back with 'setmetatable' -- which is how
		   a module such as 'tracegc' keeps being told about collections:

		       function mt.__gc (o)
		         stderr:write'.'
		         if active then setmetatable(o, mt) end
		       end
		*/
		gcUnwatch(&it.obj)
		L.callFinalizer(it.obj)
	}
}

// callFinalizer runs the '__gc' metamethod of one object.  It is what
// lgc.c's 'GCTM' does around 'luaD_pcall'.
func (L *State) callFinalizer(obj Value) {
	mt := gcMetatable(&obj)
	if mt == nil {
		return
	}
	tm := luaH_getshortstr(mt, L.g.tmNames[TM_GC])
	if !ttisfunction(tm) { /* the metatable has changed its mind */
		return
	}
	L.CheckStack2(4, "finalizer")
	top := L.GetTop()
	L.pushValue(*tm) /* the finalizer... */
	L.pushValue(obj) /* ... and its argument */
	/* PUC-Lua marks the running frame, so that 'debug.getinfo' of the
	   finalizer reports it as the '__gc' metamethod, and it stops the
	   hooks while the finalizer runs (lgc.c's 'GCTM'). */
	L.ci.callstatus |= CIST_FIN
	oldah := L.allowhook
	L.allowhook = 0
	status := L.PCall(1, 0, 0)
	L.allowhook = oldah
	L.ci.callstatus &= ^uint16(CIST_FIN)
	if status != LUA_OK {
		msg := "error object is not a string"
		if s, ok := L.ToString(-1); ok {
			msg = s
		}
		L.Warning("error in __gc ("+msg+")", false) /* luaE_warnerror */
	}
	L.SetTop(top)
}

// gcUnwatch clears the "watched" mark of an object, so that a later
// 'setmetatable' can start watching it again.
func gcUnwatch(v *Value) {
	switch ttypetag(v) {
	case LUA_VTABLE:
		hvalue(v).finalized = false
	case LUA_VUSERDATA:
		uvalue(v).finalized = false
	}
}

// pushValue puts a value on top of the stack (the stack must have room).
func (L *State) pushValue(v Value) {
	L.stack[L.top] = v
	L.top++
}

// makeTableValue / makeUdataValue build the tagged value of an object,
// which is what the finalizer has to hand back to the interpreter.
func makeTableValue(t *Table) Value {
	var v Value
	sethvalue(&v, t)
	return v
}

func makeUdataValue(u *Udata) Value {
	var v Value
	setuvalue(&v, u)
	return v
}

func makeLClosureValue(c *LClosure) Value {
	var v Value
	setclLvalue(&v, c)
	return v
}

func makeCClosureValue(c *CClosure) Value {
	var v Value
	setclCvalue(&v, c)
	return v
}

func makeThreadValue(th *State) Value {
	var v Value
	setthvalue(&v, th)
	return v
}
