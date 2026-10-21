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
package core

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

// The queue is per state, not global.
//
// It was global once, and that was a bug with a long reach.  A Go
// finalizer runs on a goroutine of its own and only hands the
// metamethod over; which state owns it is known, so it went into one
// list and a counter said how long the list was.  But a state that
// closes can still have finalizers arrive for it afterwards -- nothing
// will ever call them -- and they stayed in the list forever, readable
// as "a finalizer is waiting".  The VM asks exactly that before every
// collector step (lvm.go, after OP_NEWTABLE and OP_CONCAT): if the
// answer is yes it runs the finalizers and goes round again, and never
// reaches the step.  So two undeliverable leftovers from a state that
// had closed were enough to stop every cycle for every state that came
// after -- no '__gc' ran, no weak entry was ever swept, and the only
// symptom was a handful of tests failing on one platform.
//
// Per state, an item nobody can deliver sits in a queue nobody reads
// and costs nothing.
var (
	gcMu      sync.Mutex // guards every state's 'pendingRun'
	gcWatched atomic.Int32
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

// gcDeadWatched counts the watched objects Go has collected but whose
// '__gc' has not been handed over yet (see gcAwaitPending).  It is zero
// for as long as every watched object is still alive, so it is what lets
// the wait below cost nothing when there is nothing to wait for.
func gcDeadWatched() int {
	gcWatchMu.Lock()
	n := 0
	for _, w := range gcWatch {
		if w.dead() { /* collected: its finalizer is on its way */
			n++
		}
	}
	gcWatchMu.Unlock()
	return n
}

// gcAwaitPending gives the Go finalizer goroutine the moment it needs to
// hand over the metamethods of what this cycle has collected.
//
// 'runtime.GC' returns as soon as the sweep is over, which can be before
// the queue is filled: the handover happens on another goroutine.  A
// cycle that does not wait therefore leaves a '__gc' that is due now to
// be run by the next one, which is one cycle later than PUC-Lua runs it
// -- visible as 'collectgarbage("collect")' sometimes not running the
// metamethod the script asked it to run.
//
// Waiting is not free, so it is bounded and it is only entered when
// there is something to wait for: the loop yields rather than sleeps
// first, and gives up after a few hundred microseconds, which leaves the
// behaviour as it was on a machine too busy to schedule the goroutine.
func (L *State) gcAwaitPending() {
	if gcDeadWatched() == 0 { /* nothing was collected: nothing is on its way */
		return
	}
	for i := 0; i < 1000 && L.g.pendN.Load() == 0; i++ {
		runtime.Gosched()
	}
	for i := 0; i < 6 && L.g.pendN.Load() == 0; i++ {
		time.Sleep(50 * time.Microsecond)
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

// gcReleaseFor drops the holds of one state so Go can collect its
// unreachable watched objects, and keeps every other state's holds.
// A global nil-out would let Close or collectgarbage on L1 finalize L2.
//
// gcHold is rebuilt under gcWatchMu together with the watch list so a
// concurrent luaC_checkFinalizer cannot append a hold that this replace
// then loses.
func gcReleaseFor(g *globalState) {
	gcWatchMu.Lock()
	defer gcWatchMu.Unlock()
	hold := make([]Value, 0, len(gcWatch))
	for _, w := range gcWatch {
		if watchOwner(w) == g {
			continue
		}
		if w.dead() {
			continue
		}
		hold = append(hold, w.value())
	}
	gcHold = hold
}

// gcRehold takes them back, for the time until the next cycle.
func gcRehold() {
	gcWatchMu.Lock()
	defer gcWatchMu.Unlock()
	hold := make([]Value, 0, len(gcWatch))
	for _, w := range gcWatch {
		if w.dead() { /* collected, and so already finalised */
			continue
		}
		hold = append(hold, w.value())
	}
	gcHold = hold
}

func gcHasWatchFor(g *globalState) bool {
	gcWatchMu.Lock()
	defer gcWatchMu.Unlock()
	for _, w := range gcWatch {
		if watchOwner(w) == g {
			return true
		}
	}
	return false
}

// gcCycle is where PUC-Lua would have finished a collection: the objects
// held for '__gc' are let go, the stale stack slots are cleared, Go
// collects, and the metamethods of what has become unreachable run.
func (L *State) gcCycle() {
	gcPruneWatch()
	gcReleaseFor(L.g)
	gcClearStacks() /* see gcClearStacks: stale slots keep objects alive */
	L.gcSweepWeak()
	if L.g.strt != nil {
		L.g.strt.flush() /* the intern table must not pin dead strings */
	}
	runtime.GC() /* lets Go find the unreachable objects */
	L.gcAwaitPending()
	L.runFinalizers()
	gcRehold()
}

// gcStep is what PUC-Lua's 'luaC_checkGC' does after an allocation: it
// runs a collector step, and with it any finalizer that is due.  Here the
// step is Go's own collector, which is what finds objects unreachable and
// so is what queues the metamethods.  It is only ever run while some
// object is being watched for '__gc', and at most once every
// 'gcStepDebt' allocations.
//
// It is never run from inside a '__gc', nor while the state is closing,
// which is C's 'gcstp & GCSTPGC': GCTM sets that bit, and 'luaC_step'
// then does nothing but push the debt back down, so allocating inside a
// finalizer cannot start a collection there -- and, what matters more,
// cannot run *another* finalizer in the middle of the one that is
// running.  See gcAfterAlloc, the only caller.
func (L *State) gcStep() {
	if L.g.gcInFinalizer || L.g.closing { /* 'g->gcstp & GCSTPGC' */
		return
	}
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

// gcAfterAlloc is PUC-Lua's 'luaC_checkGC' at the end of an allocation:
// run whatever '__gc' is due, and a collector step.
//
// It answers true when a metamethod ran, because that is what the caller
// needs to know: a '__gc' is arbitrary Lua code, so the frame, the stack
// and the instruction pointer have to be re-read -- the caller does that
// with 'continue', which is what C's 'Protect' re-entering the loop
// does.
//
// An allocation inside a finalizer runs neither: see gcStep.
func (L *State) gcAfterAlloc() bool {
	if L.g.gcInFinalizer || L.g.closing {
		return false
	}
	if L.hasPending() {
		L.runFinalizers()
		return true
	}
	L.gcStep()
	return false
}

// gcQueueFinalizer is the Go finalizer: it runs on the finalizer
// goroutine, so all it may do is hand the metamethod over to the state
// that owns it.  Keeping 'obj' in the queue revives the object, which is
// what makes it reachable from the metamethod.
func gcQueueFinalizer(it gcFinalizer) {
	g := it.L.g
	gcMu.Lock()
	g.pendingRun = append(g.pendingRun, it)
	gcMu.Unlock()
	g.pendN.Add(1)
}

// hasPending tells whether a finalizer is waiting for this state.  It is
// read on the allocation path, so it must stay cheap -- and it must ask
// about *this* state, which is why the queue is per state: see the note
// above the queue.
func (L *State) hasPending() bool { return L.g.pendN.Load() > 0 }

// takeOnePending takes the next finalizer off this state's queue.
//
// Every access to the queue goes through 'gcMu', including this one,
// because 'pendingRun' has two goroutines on it: this one, which is
// running Lua, and Go's finalizer goroutine, which appends to it from
// 'gcQueueFinalizer'.  Doing the pop in two steps -- read the slice,
// store back a reslice of it -- is a race even when each step looks
// harmless: the finalizer goroutine can read the slice header between
// them, see the item that is being popped still at the front, and
// 'append' a copy of the whole queue with it back in place.  The
// metamethod then runs twice, which is what 'main.lua' caught -- one run
// in a hundred, and only on a machine with more than one core.
func (L *State) takeOnePending() (gcFinalizer, bool) {
	gcMu.Lock()
	if len(L.g.pendingRun) == 0 {
		gcMu.Unlock()
		return gcFinalizer{}, false
	}
	it := L.g.pendingRun[0]
	if len(L.g.pendingRun) == 1 {
		L.g.pendingRun = nil /* let the backing array go */
	} else {
		L.g.pendingRun = L.g.pendingRun[1:]
	}
	gcMu.Unlock()
	L.g.pendN.Add(-1)
	return it, true
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
		/* lua_close from inside a '__gc' -- which is what
		   'os.exit(0, true)' does -- runs the finalizers that are left:
		   C's close_state goes straight back to
		   luaC_freeallobjects, and 'callallpendingfinalizers' picks up
		   whatever the outer loop had not reached yet.  So does this:
		   the objects still waiting are on the state, not in a local,
		   precisely so that this call can finish them. */
		L.drainCloseQueue()
		return
	}
	L.g.closing = true
	if !gcHasWatchFor(L.g) {
		L.runFinalizers() /* nothing watched here: drain whatever Go queued */
		return
	}
	gcPruneWatch()
	/* Every watched object of this state is on gcWatch, held alive until
	   its '__gc' runs. The walk below is that list, so a collection is
	   not what finds them. runtime.GC here marks the whole Go heap and
	   is most of the time a short script spends exiting, and waiting for
	   a finalizer queue that never fills sleeps on top of that. Drain
	   anything already queued, then finalize the rest in order. Holds stay until
	   the walk finishes, so a concurrent Go collection cannot run the
	   same metamethod a second time. */
	L.runFinalizers()
	/* What is still reachable was never queued, so it is found the way
	   C finds it: by walking the list of watched objects.  C's
	   'separatetobefnz(g, 1)' moves them all to 'tobefnz' first and
	   'callallpendingfinalizers' then takes them off that list one at a
	   time; 'closeQueue' is that list, and it lives on the state so
	   that a nested lua_close can take from it too. */
	/* Only this state's watched objects. gcWatch is process-wide, and
	   taking every entry would run another state's __gc and free its
	   userdata C blocks while that state is still live. */
	gcWatchMu.Lock()
	mine := make([]weakRef, 0, len(gcWatch))
	rest := make([]weakRef, 0, len(gcWatch))
	for _, w := range gcWatch {
		if watchOwner(w) == L.g {
			mine = append(mine, w)
		} else {
			rest = append(rest, w)
		}
	}
	/* Newest first. PUC-Lua prepends onto 'tobefnz' while scanning
	   allgc from the most recently allocated object, and a '__gc'
	   that closes the state must still see the older objects
	   waiting. Watch order is oldest first. */
	for i := len(mine) - 1; i >= 0; i-- {
		L.g.closeQueue = append(L.g.closeQueue, mine[i])
	}
	gcWatch = rest
	gcWatchMu.Unlock()
	L.drainCloseQueue()
	gcRehold() /* restore holds for every state that is still open */
}

// drainCloseQueue is C's 'callallpendingfinalizers': while there is an
// object left to finalize, take it off the list and run its '__gc'.
//
// It is re-entrant on purpose.  A '__gc' may call 'os.exit(0, true)',
// which closes the state from inside the finalizer, and PUC-Lua answers
// that with another 'luaC_freeallobjects': the rest of the list is
// finalized there and then, before 'exit'.  Taking one object at a time
// from a list on the state -- rather than holding the whole list in a
// local -- is what lets the nested call see and finish what is left.
func (L *State) drainCloseQueue() {
	for {
		v, ok := L.takeCloseObject()
		if !ok {
			break
		}
		L.callFinalizer(v)
		if ttypetag(&v) == LUA_VUSERDATA {
			discardUdataC(uvalue(&v))
		}
		L.runFinalizers() /* a '__gc' may have queued more */
	}
	L.runFinalizers()
}

// takeCloseObject takes the next object that still waits for its '__gc',
// C's 'udata2finalize' off 'g->tobefnz'.
func (L *State) takeCloseObject() (Value, bool) {
	gcWatchMu.Lock()
	for len(L.g.closeQueue) > 0 {
		w := L.g.closeQueue[0]
		L.g.closeQueue = L.g.closeQueue[1:]
		if w.dead() { /* gone: Go has already queued its metamethod */
			continue
		}
		v := w.value()
		if !gcIsWatched(&v) { /* already finalised, or unmarked again */
			continue
		}
		gcWatchMu.Unlock()
		gcUnwatch(&v)
		return v, true
	}
	gcWatchMu.Unlock()
	return Value{}, false
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
		gcWatch = append(gcWatch, weakTableRef{w: weak.Make(t), g: L.g})
		gcHold = append(gcHold, *v) /* see gcHold */
		gcWatchMu.Unlock()
	case LUA_VUSERDATA:
		u := uvalue(v)
		if u.finalized {
			return
		}
		u.finalized = true
		gcWatched.Add(1)
		setUdataFinalizer(L, u)
		gcWatchMu.Lock()
		gcWatch = append(gcWatch, weakUdataRef{w: weak.Make(u), g: L.g})
		gcHold = append(gcHold, *v) /* see gcHold */
		gcWatchMu.Unlock()
	}
}

// setUdataFinalizer sets the Go finalizer of a userdata, at most once.
//
// Go allows one finalizer per object, and a userdata can need one for two
// unrelated reasons: a '__gc' metamethod, and a C block behind it (see
// cgoNewUserBlock, which is how lua_newuserdata is served across cgo).
// Both go through here so that whichever comes second does not find the
// slot taken -- 'runtime.SetFinalizer' throws if it does -- and the one
// finalizer does whichever of the two applies when it runs.
func setUdataFinalizer(L *State, u *Udata) {
	if u.finSet {
		return
	}
	u.finSet = true
	u.finL = L
	runtime.SetFinalizer(u, func(o *Udata) {
		/* Nothing is captured here on purpose. A finalizer that
		   captured L -- which is what this started as -- keeps the
		   state reachable, and with it every object on its stack,
		   including this one: the collector would never see the
		   userdata as garbage and this would never run, so the C
		   block behind it would never be freed. */
		if o.finalized { /* a '__gc' is waiting: it must see its memory */
			if L := o.finL; L != nil {
				gcQueueFinalizer(gcFinalizer{L: L, obj: makeUdataValue(o)})
			}
			return /* the block stays until the object is really dead */
		}
		discardUdataC(o)
	})
}

// discardUdataC gives back the C memory tied to a userdata that is not
// coming back: the block lua_newuserdata handed out, and the luaL_Buffer
// whose box this is. Both are idempotent.
func discardUdataC(u *Udata) {
	if u == nil {
		return
	}
	if freeUdataCBlock != nil {
		freeUdataCBlock(u)
	}
	if releaseUdataBuf != nil {
		releaseUdataBuf(u)
	}
}

// releaseUdataBuf drops the luaL_Buffer whose box is u. Set by the cgo
// backend; nil otherwise.
var releaseUdataBuf func(u *Udata)

// freeUdataCBlock releases the C block behind a userdata. Only the cgo
// bridge ever allocates one, so without cgo this is nil and a userdata
// has nothing to release.
var freeUdataCBlock func(u *Udata)

// runFinalizers drains the queue, calling each metamethod with its
// object.  It is called where PUC-Lua runs a collector step.
//
// The queue being drained lives on the global state, and not in a local,
// because a '__gc' can reach it: 'os.exit(0, true)' calls lua_close from
// inside a finalizer, and close_state then runs the finalizers that are
// left -- which are the ones still in this very list.
func (L *State) runFinalizers() {
	for {
		it, ok := L.takeOnePending()
		if !ok {
			return
		}
		/* Every item in the queue belongs to this state: see the note
		   above the queue.  C takes the object out of the "to be
		   finalized" list before it calls the metamethod
		   ('udata2finalize' clears FINALIZEDBIT), so that '__gc' can
		   put it back with 'setmetatable' -- which is how a module such
		   as 'tracegc' keeps being told about collections:

		       function mt.__gc (o)
		         stderr:write'.'
		         if active then setmetatable(o, mt) end
		       end
		*/
		gcUnwatch(&it.obj)
		L.callFinalizer(it.obj)
		if ttypetag(&it.obj) == LUA_VUSERDATA {
			u := uvalue(&it.obj)
			if L.g.closing {
				/* the state is going away: nothing may resurrect it */
				discardUdataC(u)
			} else if u.finL != nil {
				/* Go runs a finalizer once. Arm another, so a userdata
				   that __gc stored somewhere keeps its C block, and one
				   that nothing kept is freed the next time it is
				   unreachable -- not while __gc can still see it. */
				u.finSet = false
				setUdataFinalizer(u.finL, u)
			}
		}
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
	/* PUC-Lua stops the collector for the duration of a finalizer, and
	   that is observable: 'collectgarbage' answers nil inside one.
	   GCTM saves the previous state and restores it
	   ('oldgcstp = g->gcstp'), so a finalizer that runs inside another
	   one -- which is what 'os.exit(0, true)' from a '__gc' does --
	   does not turn the collector back on for the outer one. */
	oldIn := L.g.gcInFinalizer
	L.g.gcInFinalizer = true
	status := L.PCall(1, 0, 0)
	L.g.gcInFinalizer = oldIn
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
