// Translation of the parts of lgc.c that Go's collector cannot express
// (PUC-Lua 5.4.9 bridge: __gc finalizers + weak tables in lgc_weak.go).
// Full mark/sweep/colors/barriers/debt are INTENTIONAL omissions — Go GC.
//
// Memory management is delegated completely to the Go runtime garbage
// collector (no artificial runtime.GC() cycles or global object holds):
//
//   - '__gc' metamethods (here). 'runtime.SetFinalizer' notifies us when
//     Go's GC finds a watched table or userdata unreachable. Because a Go
//     finalizer runs on a separate goroutine and Lua states are single-
//     threaded, the finalizer queues the metamethod onto the owning state
//     and the VM drains the queue at safe points. Remaining watched
//     objects are finalized when the state closes ('closeFinalizers').
//
//   - weak tables (lgc_weak.go).
package core

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
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
	gcMu      sync.Mutex // guards every state's 'pendingRun'
	gcWatched atomic.Int32
)

// gcStepDebt is how many allocations trigger a lightweight maintenance step
// (clearing stale stack slots above top, pruning weak tracking lists, and
// cleaning dead weak-table slots).
const (
	gcStepDebt     = 65536
	gcWeakStepDebt = 2048
)

const (
	gcWatchPruneMin = 256
	gcThrPruneMin   = 256
)

// gcWatch is every object that is being watched for '__gc', held weakly
// so 'lua_close' can finalize remaining reachable objects.
var (
	gcWatchMu        sync.Mutex
	gcWatch          []weakRef
	gcWatchNextPrune = gcWatchPruneMin
)

// gcThrs is every thread of every state, held weakly. See gcClearStacks.
var (
	gcThrMu        sync.Mutex
	gcThrs         []weak.Pointer[State]
	gcThrNextPrune = gcThrPruneMin
)

// gcTrackThread notes a new thread (threads are created in lstate.go).
func gcTrackThread(L *State) {
	gcThrMu.Lock()
	if len(gcThrs) >= gcThrNextPrune {
		keep := gcThrs[:0]
		for i := range gcThrs {
			p := *(*unsafe.Pointer)(unsafe.Pointer(&gcThrs[i]))
			if !weakPtrDead(p) {
				keep = append(keep, gcThrs[i])
			}
		}
		gcThrs = keep
		gcThrNextPrune = len(gcThrs)*2 + gcThrPruneMin
	}
	gcThrs = append(gcThrs, weak.Make(L))
	gcThrMu.Unlock()
}

// gcTrackWatch records w in gcWatch, pruning dead entries when the list grows.
func gcTrackWatch(w weakRef) {
	gcWatchMu.Lock()
	if len(gcWatch) >= gcWatchNextPrune {
		keep := gcWatch[:0]
		for _, item := range gcWatch {
			if !item.dead() {
				keep = append(keep, item)
			}
		}
		gcWatch = keep
		gcWatchNextPrune = len(gcWatch)*2 + gcWatchPruneMin
	}
	gcWatch = append(gcWatch, w)
	gcWatchMu.Unlock()
}

// gcClearThreadStack clears dead stack slots on a single thread so stale
// values cannot pin unreachable objects on the Go heap.
func (th *State) gcClearThreadStack() {
	tbcFloor := 0
	if len(th.tbclist) > 0 {
		tbcFloor = th.tbclist[len(th.tbclist)-1] + 1
	}
	lim := th.top
	if th.ci != nil && isLua(th.ci) && th.ci.top > lim {
		lim = th.ci.top
	}
	var callee *CallInfo
	for ci := th.ci; ci != nil; ci = ci.prev {
		curCallee := callee
		callee = ci
		if !isLua(ci) {
			continue
		}
		base := ci.funcIdx + 1
		cl := clLvalue(&th.stack[ci.funcIdx])
		if cl == nil || cl.p == nil || len(cl.p.code) == 0 {
			continue
		}
		rawpc := ci.savedpc
		pc := rawpc - 1
		if pc < 0 || pc >= len(cl.p.code) {
			continue
		}
		if cl.p.code[pc].Op == OP_EXTRAARG && pc > 0 {
			pc--
		}
		used := base
		for _, lv := range cl.p.locvars {
			if lv.startpc <= rawpc && rawpc-1 < lv.endpc {
				used++
			}
		}
		if used < base+int(cl.p.numparams) {
			used = base + int(cl.p.numparams)
		}
		/* If debug info was stripped, locvars is empty; never clear inside the frame. */
		if len(cl.p.locvars) == 0 && int(cl.p.maxstacksize) > 0 {
			used = base + int(cl.p.maxstacksize)
		}
		if used < tbcFloor {
			used = tbcFloor
		}
		op := cl.p.code[pc].Op
		if ci == th.ci && op == OP_NEWTABLE {
			ra := base + cl.p.code[pc].A
			if ra <= used {
				hasTemp := false
				if pc > 0 {
					iop := cl.p.code[pc-1].Op
					if iop == OP_EXTRAARG && pc >= 2 {
						iop = cl.p.code[pc-2].Op
					}
					if testAMode(iop) && iop != OP_FORLOOP && iop != OP_FORPREP && iop != OP_TFORLOOP && base+cl.p.code[pc-1].A >= used {
						hasTemp = true
					}
				}
				if !hasTemp {
					end := used
					if ra+1 > end {
						end = ra + 1
					}
					if end < lim {
						lim = end
					}
				}
			}
		} else if curCallee == nil {
			hasTemp := false
			if pc > 0 {
				iop := cl.p.code[pc-1].Op
				if testAMode(iop) && iop != OP_FORLOOP && iop != OP_FORPREP && iop != OP_TFORLOOP && base+cl.p.code[pc-1].A >= used {
					hasTemp = true
				}
			}
			if !hasTemp {
				end := used
				if ra := base + cl.p.code[pc].A; ra+1 > end {
					end = ra + 1
				}
				if op == OP_CONCAT {
					if rb := base + cl.p.code[pc].A + cl.p.code[pc].B; rb > end {
						end = rb
					}
				}
				if end <= used+1 && end < lim {
					lim = end
				}
			}
		} else if (op == OP_CALL || op == OP_TFORCALL) && curCallee != nil {
			callRa := base + cl.p.code[pc].A
			if callRa == curCallee.funcIdx && pc > 0 {
				prev := cl.p.code[pc-1]
				isOuter := false
				if testAMode(prev.Op) && base+prev.A == callRa {
					isOuter = true
				} else if pc >= 2 && testAMode(prev.Op) && base+prev.A == callRa+1 {
					prev2 := cl.p.code[pc-2]
					if testAMode(prev2.Op) && base+prev2.A == callRa {
						isOuter = true
					}
				}
				if isOuter && callRa == used+1 && pc >= 2 {
					k := pc - 2
					if base+prev.A == callRa+1 && pc >= 3 {
						k = pc - 3
					}
					if k >= 0 && cl.p.code[k].Op == OP_CALL && cl.p.code[k].C == 1 && base+cl.p.code[k].A == used {
						setnilvalue(&th.stack[used])
					}
				}
			}
		} else if curCallee != nil && ci.callstatus&CIST_HOOKED == 0 && op != OP_RETURN {
			end := used
			if ra := base + cl.p.code[pc].A; ra+1 > end {
				end = ra + 1
			}
			if op == OP_CONCAT {
				if rb := base + cl.p.code[pc].A + cl.p.code[pc].B; rb > end {
					end = rb
				}
			}
			if end <= used+1 {
				for j := end; j < curCallee.funcIdx; j++ {
					setnilvalue(&th.stack[j])
				}
			}
		}
	}
	if lim < tbcFloor {
		lim = tbcFloor
	}
	for j := lim; j < len(th.stack); j++ {
		setnilvalue(&th.stack[j])
	}
}

func (L *State) gcClearStacks() {
	g := L.g
	gcThrMu.Lock()
	old := gcThrs
	gcThrs = nil
	gcThrMu.Unlock()
	keep := make([]weak.Pointer[State], 0, len(old))
	for i := range old {
		p := *(*unsafe.Pointer)(unsafe.Pointer(&old[i]))
		if weakPtrDead(p) {
			continue
		}
		th := old[i].Value()
		if th == nil {
			continue
		}
		if th.g == g {
			th.gcClearThreadStack()
		}
		if !th.dead {
			keep = append(keep, old[i])
		}
	}
	gcThrMu.Lock()
	gcThrs = append(gcThrs, keep...)
	gcThrMu.Unlock()
}

// gcPruneWatch drops the list entries whose object has gone.
func gcPruneWatch() {
	gcWatchMu.Lock()
	old := gcWatch
	gcWatch = nil
	gcWatchMu.Unlock()
	keep := make([]weakRef, 0, len(old))
	for _, w := range old {
		if w.dead() {
			continue
		}
		keep = append(keep, w)
	}
	gcWatchMu.Lock()
	gcWatch = append(gcWatch, keep...)
	gcWatchMu.Unlock()
}

// gcWatchEmpty says whether any object is being watched at all.
func gcWatchEmpty() bool {
	gcWatchMu.Lock()
	n := len(gcWatch)
	gcWatchMu.Unlock()
	return n == 0
}

// gcDeadWatchedFor counts the watched objects Go has collected whose '__gc'
// may still be in flight on the Go finalizer goroutine.
func gcDeadWatchedFor(g *globalState) int {
	gcWatchMu.Lock()
	n := 0
	for _, w := range gcWatch {
		if watchOwner(w) == g && w.dead() {
			n++
		}
	}
	gcWatchMu.Unlock()
	return n
}

var gcSpanFlushSink any

// gcAwaitPending gives the Go finalizer goroutine a brief moment to hand
// over metamethods of objects that Go's GC has already collected.
func (L *State) gcAwaitPending() {
	if gcDeadWatchedFor(L.g) == 0 {
		return
	}
	/* When weak.Pointer is already nil (gcDeadWatchedFor > 0), Go's GC has
	   marked the watched Table/Udata unreachable, but SetFinalizer is only
	   queued when the span containing the object is swept. Allocate batches
	   of Table-sized heap objects across goroutines so the allocator sweeps
	   all Table spans across all P mcaches. */
	np := runtime.GOMAXPROCS(0)
	for round := 0; round < 4 && L.g.pendN.Load() == 0 && gcDeadWatchedFor(L.g) > 0; round++ {
		var wg sync.WaitGroup
		holds := make([][]*Table, np)
		for p := 0; p < np; p++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				hold := make([]*Table, 1024)
				for j := range hold {
					hold[j] = new(Table)
				}
				holds[idx] = hold
			}(p)
		}
		wg.Wait()
		gcSpanFlushSink = holds
		runtime.Gosched()
		gcSpanFlushSink = nil
		for i := 0; i < 20 && L.g.pendN.Load() == 0 && gcDeadWatchedFor(L.g) > 0; i++ {
			time.Sleep(100 * time.Microsecond)
		}
	}
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

// gcCycle performs lightweight GC-bridge maintenance without calling
// runtime.GC(): prunes dead watch entries, clears stale stack slots above
// top, cleans dead weak-table slots, flushes the short-string intern cache,
// and drains any pending '__gc' finalizers queued by Go's runtime GC.
func (L *State) gcCycle() {
	L.gcCycleOpt(true)
}

func (L *State) gcCycleOpt(sweepWeak bool) {
	L.gcAwaitPending()
	gcPruneWatch()
	L.gcClearStacks()
	if sweepWeak {
		L.gcSweepWeak()
	}
	if L.g.strt != nil {
		L.g.strt.flush()
	}
	L.gcAwaitPending()
	L.runFinalizers()
}

// gcStep performs periodic maintenance after gcStepDebt allocations and
// drains any pending finalizers queued by Go's collector.
func (L *State) gcStep() bool {
	if L.g.gcInFinalizer || L.g.closing {
		return false
	}
	if L.g.gcStopped {
		return false
	}
	if gcWeakCount.Load() == 0 && gcWatched.Load() == 0 {
		return false
	}
	L.g.gcDebt++
	if L.g.gcDebt <= 4 {
		L.gcClearThreadStack()
	}
	if L.g.gcDebt%gcWeakStepDebt == 0 {
		if gcWeakCount.Load() != 0 {
			L.gcClearThreadStack()
		}
	}
	if L.g.gcDebt < gcStepDebt {
		return false
	}
	L.g.gcDebt = 1
	L.gcClearThreadStack()
	L.gcAwaitPending()
	gcPruneWatch()
	L.gcSweepWeak()
	if L.g.strt != nil {
		L.g.strt.flush()
	}
	if L.hasPending() {
		L.runFinalizers()
		return true
	}
	return false
}

// gcAfterAlloc is PUC-Lua's 'luaC_checkGC' at the end of an allocation:
// run whatever '__gc' is due, and a maintenance step.
func (L *State) gcAfterAlloc() bool {
	if L.g.gcInFinalizer || L.g.closing {
		return false
	}
	if L.g.gcDebt == 0 && L.hasPending() {
		L.runFinalizers()
		return true
	}
	return L.gcStep()
}

// gcQueueFinalizer is called from the Go finalizer goroutine to hand the
// metamethod over to the state that owns it. If the state has already
// finished closing, the item is dropped (and any userdata C block freed)
// so a closed state never leaks pending finalizers.
func gcQueueFinalizer(it gcFinalizer) {
	if it.L == nil || it.L.g == nil {
		if ttypetag(&it.obj) == LUA_VUSERDATA {
			discardUdataC(uvalue(&it.obj))
		}
		return
	}
	g := it.L.g
	gcMu.Lock()
	if g.closed {
		gcMu.Unlock()
		if ttypetag(&it.obj) == LUA_VUSERDATA {
			discardUdataC(uvalue(&it.obj))
		}
		return
	}
	g.pendingRun = append(g.pendingRun, it)
	g.pendN.Add(1)
	gcMu.Unlock()
}

// hasPending tells whether a finalizer is waiting for this state.
func (L *State) hasPending() bool { return L.g.pendN.Load() > 0 }

// takeOnePending takes the next finalizer off this state's queue.
func (L *State) takeOnePending() (gcFinalizer, bool) {
	gcMu.Lock()
	if len(L.g.pendingRun) == 0 {
		L.g.pendN.Store(0)
		gcMu.Unlock()
		return gcFinalizer{}, false
	}
	it := L.g.pendingRun[0]
	L.g.pendingRun[0] = gcFinalizer{}
	if len(L.g.pendingRun) == 1 {
		L.g.pendingRun = nil
	} else {
		L.g.pendingRun = L.g.pendingRun[1:]
	}
	L.g.pendN.Add(-1)
	gcMu.Unlock()
	return it, true
}

// fullCollect runs maintenance and drains any pending finalizers.
func (L *State) fullCollect() {
	L.g.gcDebt = 0
	L.gcCycle()
}

// closeFinalizers is 'luaC_freeallobjects' (lgc.c): when a state closes,
// the '__gc' metamethod of every object that has one runs, whether the
// object is still reachable or not.
func (L *State) closeFinalizers() {
	if L.g.closing {
		L.drainCloseQueue()
		return
	}
	L.g.closing = true
	if !gcHasWatchFor(L.g) {
		L.runFinalizers()
		L.markClosedQueue()
		return
	}
	L.gcAwaitPending()
	gcPruneWatch()
	L.runFinalizers()
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
	for i := len(mine) - 1; i >= 0; i-- {
		L.g.closeQueue = append(L.g.closeQueue, mine[i])
	}
	gcWatch = rest
	gcWatchMu.Unlock()
	L.drainCloseQueue()
	L.markClosedQueue()
}

func (L *State) markClosedQueue() {
	gcMu.Lock()
	L.g.closed = true
	left := L.g.pendingRun
	L.g.pendingRun = nil
	L.g.pendN.Store(0)
	gcMu.Unlock()
	for i := range left {
		if ttypetag(&left[i].obj) == LUA_VUSERDATA {
			discardUdataC(uvalue(&left[i].obj))
		}
	}
}

// drainCloseQueue is C's 'callallpendingfinalizers': while there is an
// object left to finalize, take it off the list and run its '__gc'.
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
		L.runFinalizers()
	}
	L.runFinalizers()
}

// takeCloseObject takes the next object that still waits for its '__gc',
// C's 'udata2finalize' off 'g->tobefnz'.
func (L *State) takeCloseObject() (Value, bool) {
	gcWatchMu.Lock()
	for len(L.g.closeQueue) > 0 {
		w := L.g.closeQueue[0]
		L.g.closeQueue[0] = nil
		if len(L.g.closeQueue) == 1 {
			L.g.closeQueue = nil
		} else {
			L.g.closeQueue = L.g.closeQueue[1:]
		}
		if w.dead() {
			continue
		}
		v := w.value()
		if !gcIsWatched(&v) {
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
	if v.p == nil {
		return false
	}
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
		if t.finalized {
			return
		}
		t.finalized = true
		gcWatched.Add(1)
		L.g.gcDebt = 0
		mainL := L
		if L.g != nil && L.g.mainthread != nil {
			mainL = L.g.mainthread
		}
		wL := weak.Make(mainL)
		runtime.SetFinalizer(t, func(o *Table) {
			if st := wL.Value(); st != nil {
				gcQueueFinalizer(gcFinalizer{L: st, obj: makeTableValue(o)})
			}
		})
		gcTrackWatch(weakTableRef{w: weak.Make(t), g: L.g})
	case LUA_VUSERDATA:
		u := uvalue(v)
		if u.finalized {
			return
		}
		u.finalized = true
		gcWatched.Add(1)
		L.g.gcDebt = 0
		setUdataFinalizer(L, u)
		gcTrackWatch(weakUdataRef{w: weak.Make(u), g: L.g})
	}
}

// setUdataFinalizer sets the Go finalizer of a userdata, at most once.
func setUdataFinalizer(L *State, u *Udata) {
	if u.finSet {
		return
	}
	u.finSet = true
	if L != nil {
		mainL := L
		if L.g != nil && L.g.mainthread != nil {
			mainL = L.g.mainthread
		}
		u.finL = weak.Make(mainL)
	}
	runtime.SetFinalizer(u, func(o *Udata) {
		if o.finalized {
			if st := o.finL.Value(); st != nil && !st.g.closed {
				gcQueueFinalizer(gcFinalizer{L: st, obj: makeUdataValue(o)})
				return
			}
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

// runFinalizers drains the queue, calling each metamethod with its object.
func (L *State) runFinalizers() {
	for {
		it, ok := L.takeOnePending()
		if !ok {
			return
		}
		gcUnwatch(&it.obj)
		L.callFinalizer(it.obj)
		if ttypetag(&it.obj) == LUA_VUSERDATA {
			u := uvalue(&it.obj)
			if L.g.closing {
				discardUdataC(u)
			} else if st := u.finL.Value(); st != nil {
				u.finSet = false
				setUdataFinalizer(st, u)
			} else {
				discardUdataC(u)
			}
		}
	}
}

// callFinalizer runs the '__gc' metamethod of one object. It is what
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
	savedTop := L.top
	savedCITop := 0
	if L.ci != nil {
		savedCITop = L.ci.top
		if isLua(L.ci) && L.top < L.ci.top {
			L.top = L.ci.top
		}
	}
	L.CheckStack2(4, "finalizer")
	top := L.GetTop()
	L.pushValue(*tm) /* the finalizer... */
	L.pushValue(obj) /* ... and its argument */
	L.ci.callstatus |= CIST_FIN
	oldah := L.allowhook
	L.allowhook = 0
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
	L.top = savedTop
	if L.ci != nil {
		L.ci.top = savedCITop
	}
}

// gcUnwatch clears the "watched" mark of an object, so that a later
// 'setmetatable' can start watching it again.
func gcUnwatch(v *Value) {
	switch ttypetag(v) {
	case LUA_VTABLE:
		t := hvalue(v)
		if t.finalized {
			t.finalized = false
			gcWatched.Add(-1)
		}
	case LUA_VUSERDATA:
		u := uvalue(v)
		if u.finalized {
			u.finalized = false
			gcWatched.Add(-1)
		}
	}
	gcPruneWatch()
}

// pushValue puts a value on top of the stack (the stack must have room).
func (L *State) pushValue(v Value) {
	L.stack[L.top] = slotVal(&v)
	L.top++
}

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
