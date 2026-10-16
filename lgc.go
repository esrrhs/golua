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
package lua

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// gcFinalizer is an '__gc' metamethod waiting to be run: Go has found the
// object unreachable, and the interpreter has to be told about it on the
// thread that owns the state.
type gcFinalizer struct {
	L   *State
	obj Value // the object; keeping it alive is what Lua calls resurrection
	tm  Value // the '__gc' metamethod
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
const gcStepDebt = 1024

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
	L.g.gcDebt++
	if L.g.gcDebt < gcStepDebt {
		return
	}
	L.g.gcDebt = 0
	L.gcSweepWeak()
	/* Go's collector scans the whole stack slice, while Lua's only ever
	   sees the slots below 'top'.  What sits above 'top' is garbage as
	   far as the program is concerned -- a slot a call has left behind
	   still holds the value it had -- but Go cannot know that, and one
	   stale reference is enough to keep an object alive forever.  The
	   slots are cleared so that Go's idea of what is reachable matches
	   Lua's. */
	for i := L.top; i < len(L.stack); i++ {
		setnilvalue(&L.stack[i])
	}
	runtime.GC() /* lets Go find the unreachable objects */
	L.runFinalizers()
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

// luaC_checkFinalizer is 'luaC_checkfinalizer' (lgc.c): an object that
// gets a metatable with a '__gc' field starts to be watched, so that the
// metamethod runs when the object becomes unreachable.
func luaC_checkFinalizer(L *State, v *Value, mt *Table) {
	if mt == nil {
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
		tmCopy := *tm
		runtime.SetFinalizer(t, func(o *Table) {
			gcQueueFinalizer(gcFinalizer{L: L, obj: makeTableValue(o), tm: tmCopy})
		})
	case LUA_VUSERDATA:
		u := uvalue(v)
		if u.finalized {
			return
		}
		u.finalized = true
		gcWatched.Add(1)
		tmCopy := *tm
		runtime.SetFinalizer(u, func(o *Udata) {
			gcQueueFinalizer(gcFinalizer{L: L, obj: makeUdataValue(o), tm: tmCopy})
		})
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
		if it.L != L { /* another state: leave it for its own thread */
			gcQueueFinalizer(*it)
			continue
		}
		L.CheckStack2(4, "finalizer")
		top := L.GetTop()
		L.pushValue(it.tm)  /* the finalizer... */
		L.pushValue(it.obj) /* ... and its argument */
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
