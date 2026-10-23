package core

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// Weak tables (lgc.c's 'cleartable' and 'luaC_checkfinalizer' for
// '__mode').
//
// A weak table holds its keys or its values without keeping them alive.
// Lua's collector can tell whether an object is still reachable, because
// it walks the object graph itself; Go's collector knows only what the
// Go heap says, and a weak table necessarily holds a strong Go reference
// to every entry -- so as far as Go is concerned nothing in it is ever
// garbage.
//
// The answer is to take the references away and ask Go again:
//
//  1. every weak reference in every weak table is replaced by a
//     'weak.Pointer' and its slot is emptied;
//  2. Go collects.  An object that only the table referred to is now
//     really unreachable, so it goes -- and its weak pointer goes with
//     it.  Anything else survives, and its weak pointer still resolves.
//  3. the entries whose weak pointer is gone are removed, the rest are
//     put back.
//
// Step 2 needs two cycles: a weak pointer is cleared in the cycle after
// the one that frees its object.

// weakKey / weakVal are the two halves of a table's '__mode'.
const (
	weakKey = 1 << 0
	weakVal = 1 << 1
)

// weakRef is a weak reference to a collectable object; 'dead' says the
// object is gone.  Only tables, userdata, closures and threads can be
// weak: numbers, strings and booleans are values, not objects.
type weakRef interface {
	dead() bool
	// value rebuilds the tagged value of a survivor.  Nothing may hold a
	// strong reference to the object while Go collects -- not even the
	// sweep itself -- or the object could never be seen as garbage; the
	// weak pointer is the only place it can come back from.
	value() Value
}

type weakTableRef struct {
	w weak.Pointer[Table]
	g *globalState // set for gcWatch entries; nil for weak-table keys
}

func (r weakTableRef) dead() bool { return r.w.Value() == nil }
func (r weakTableRef) value() Value {
	return makeTableValue(r.w.Value())
}

type weakUdataRef struct {
	w weak.Pointer[Udata]
	g *globalState // set for gcWatch entries; nil for weak-table keys
}

func (r weakUdataRef) dead() bool { return r.w.Value() == nil }
func (r weakUdataRef) value() Value {
	return makeUdataValue(r.w.Value())
}

// watchOwner is the globalState that put w on gcWatch. Other states must
// not steal it on Close.
func watchOwner(w weakRef) *globalState {
	switch r := w.(type) {
	case weakTableRef:
		return r.g
	case weakUdataRef:
		return r.g
	}
	return nil
}

type weakLClosureRef struct{ w weak.Pointer[LClosure] }

func (r weakLClosureRef) dead() bool { return r.w.Value() == nil }
func (r weakLClosureRef) value() Value {
	return makeLClosureValue(r.w.Value())
}

type weakCClosureRef struct{ w weak.Pointer[CClosure] }

func (r weakCClosureRef) dead() bool { return r.w.Value() == nil }
func (r weakCClosureRef) value() Value {
	return makeCClosureValue(r.w.Value())
}

type weakThreadRef struct{ w weak.Pointer[State] }

func (r weakThreadRef) dead() bool { return r.w.Value() == nil }
func (r weakThreadRef) value() Value {
	return makeThreadValue(r.w.Value())
}

func makeWeakRef(v *Value) weakRef {
	switch ttypetag(v) {
	case LUA_VTABLE:
		return weakTableRef{w: weak.Make(hvalue(v))}
	case LUA_VUSERDATA:
		return weakUdataRef{w: weak.Make(uvalue(v))}
	case LUA_VLCL:
		return weakLClosureRef{weak.Make(clLvalue(v))}
	case LUA_VCCL:
		return weakCClosureRef{weak.Make(clCvalue(v))}
	case LUA_VTHREAD:
		return weakThreadRef{weak.Make(thvalue(v))}
	}
	return nil /* numbers, strings and booleans are never weak */
}

var (
	gcWeakMu     sync.Mutex
	gcWeakTables []weak.Pointer[Table] // the weak tables themselves, weakly
	gcWeakCount  atomic.Int32          // nonzero while any weak table exists
)

// parseWeakMode reads TM_MODE from mt the way traversetable does each mark.
// Returns 0 for nil mt, non-string __mode, or a mode without 'k'/'v'.
func parseWeakMode(L *State, mt *Table) byte {
	if mt == nil {
		return 0
	}
	tm := luaH_getshortstr(mt, L.g.tmNames[TM_MODE])
	if !ttisstring(tm) {
		return 0
	}
	mode := 0
	s := tsvalue(tm).val
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case 'k':
			mode |= weakKey
		case 'v':
			mode |= weakVal
		}
	}
	return byte(mode)
}

// luaC_checkWeakMode is the '__mode' half of setmetatable: a table whose
// metatable has a weak '__mode' starts to be swept.
//
// C re-reads TM_MODE every mark (traversetable); nil mt / non-weak mode
// means a strong table. Always clear weakMode first so setmetatable(t, nil)
// or a metatable without '__mode' stops weak sweeping. Dedup list membership
// so repeated setmetatable does not sweep the same table twice per cycle.
func luaC_checkWeakMode(L *State, t *Table, mt *Table) {
	t.weakMode = parseWeakMode(L, mt)
	if t.weakMode == 0 {
		return
	}
	gcWeakMu.Lock()
	for _, wp := range gcWeakTables {
		if wp.Value() == t {
			gcWeakMu.Unlock()
			gcWeakCount.Store(1)
			return
		}
	}
	gcWeakTables = append(gcWeakTables, weak.Make(t))
	gcWeakMu.Unlock()
	gcWeakCount.Store(1)
}

// weakSlot is one weak reference taken out of a table for the duration of
// a sweep: the node it lives in, the slot to put a survivor back into,
// and the weak pointer that will say whether the object survived.  A key
// slot has no 'v': a node's key is not a full Value, so a survivor is
// put back with 'setnodekey' instead.
type weakSlot struct {
	n     *node // nil for a slot of the array part
	v     *Value
	isKey bool
	w     weakRef
}

// gcSweepWeak is clearkeys/clearvalues (lgc.c): drop weak-table entries whose
// object is no longer reachable from anywhere else. Runs from the collector
// step, as in C.
//
// Ephemeron note (__mode="k"): C marks a value only when its key is already
// marked (traverseephemeron + convergeephemerons). Leaving nd.val strong
// across runtime.GC lets a value→key cycle keep the key alive, so the entry
// never clears. During the detach window we therefore also weaken collectable
// values for weak-key-only tables.
//
// Residuals (NOT 完全对齐 — do not rubber-stamp; see NOTES FLAG):
//  - No convergeephemerons fixed point: a strong value reachable only via an
//    alive weak key may be cleared in the detach window (Go has no ephemeron map).
//  - No atomic-phase iscleared ordering: C clearvalues → separatetobefnz →
//    markbeingfnz → clearkeys keeps objects about to run __gc as weak keys
//    for that cycle; Go weak.Pointer nils as soon as the Go finalizer is
//    queued, so a finalizable weak key can vanish in the same cycle as __gc.
func (L *State) gcSweepWeak() {
	if gcWeakCount.Load() == 0 {
		return
	}
	var slots []weakSlot
	gcWeakMu.Lock()
	tables := make([]*Table, 0, len(gcWeakTables))
	alive := gcWeakTables[:0]
	for _, wp := range gcWeakTables {
		if t := wp.Value(); t != nil {
			tables = append(tables, t)
			alive = append(alive, wp)
		}
	}
	gcWeakTables = alive
	n := len(alive)
	gcWeakMu.Unlock()
	if n == 0 {
		gcWeakCount.Store(0)
		return
	}
	/* Re-parse TM_MODE each sweep (≈ traversetable); note non-weak for prune. */
	weakStill := make(map[*Table]struct{}, len(tables))
	for _, t := range tables {
		mode := parseWeakMode(L, t.metatable)
		t.weakMode = mode
		if mode == 0 {
			continue
		}
		weakStill[t] = struct{}{}
		for i := range t.nodes {
			nd := &t.nodes[i]
			if isempty(&nd.val) { /* no entry? */
				continue
			}
			if mode&weakKey != 0 {
				var kv Value
				getnodekey(&kv, nd)
				if w := makeWeakRef(&kv); w != nil {
					slots = append(slots, weakSlot{n: nd, isKey: true, w: w})
					setnilkey(nd)
					/* ephemeron approx: do not leave the value strong across GC */
					if mode&weakVal == 0 {
						if vw := makeWeakRef(&nd.val); vw != nil {
							slots = append(slots, weakSlot{n: nd, v: &nd.val, w: vw})
							setnilvalue(&nd.val)
						}
					}
				}
			}
			if mode&weakVal != 0 {
				if w := makeWeakRef(&nd.val); w != nil {
					slots = append(slots, weakSlot{n: nd, v: &nd.val, w: w})
					setnilvalue(&nd.val)
				}
			}
		}
		/* array part: clearvalues walks sizearray the same way */
		if mode&weakVal != 0 {
			for i := range t.array {
				if w := makeWeakRef(&t.array[i]); w != nil {
					slots = append(slots, weakSlot{v: &t.array[i], w: w})
					setnilvalue(&t.array[i])
				}
			}
		}
	}
	/* Prune tables that are no longer weak; keep concurrent appends. */
	gcWeakMu.Lock()
	filtered := gcWeakTables[:0]
	for _, wp := range gcWeakTables {
		t := wp.Value()
		if t == nil {
			continue
		}
		if _, ok := weakStill[t]; ok || t.weakMode != 0 {
			/* in this sweep's weak set, or registered after we unlocked */
			filtered = append(filtered, wp)
		}
	}
	gcWeakTables = filtered
	if len(filtered) == 0 {
		gcWeakCount.Store(0)
	}
	gcWeakMu.Unlock()
	if len(slots) == 0 {
		return
	}
	/* the slots above 'top' would hold the objects alive: see gcStep */
	for i := L.top; i < len(L.stack); i++ {
		setnilvalue(&L.stack[i])
	}
	runtime.GC()
	runtime.GC() /* the second cycle is the one that clears the pointer */
	for i := range slots {
		s := &slots[i]
		if !s.w.dead() { /* still reachable from somewhere: put it back */
			sv := s.w.value()
			if s.isKey {
				setnodekey(s.n, &sv)
			} else {
				/* C allweak: clearkeys nils the value when the key dies.
				   Do not resurrect a live value into a DEADKEY node. */
				if s.n != nil && s.n.keytt == LUA_TDEADKEY {
					continue
				}
				*s.v = sv
			}
			continue
		}
		if s.n == nil { /* the array part: the entry simply goes */
			setnilvalue(s.v)
			continue
		}
		if s.isKey {
			/* clearkeys: nil value + removeentry → setdeadvalue.
			   FLAG Go-GC: C keeps gkey's gc pointer; after runtime.GC the
			   object is reclaimed, so storing that address as unsafe.Pointer
			   is illegal ("marked free object"). kp=nil; findindex dead-key
			   arm stays inert (TestDeadKeyDoesNotBreakTraversal skipped). */
			setnilvalue(&s.n.val)
			setdeadkey(s.n, nil)
			continue
		}
		/* clearvalues / ephemeron value death: nil the value.
		   removeentry only deadens unmarked (still-nil) keys — a restored
		   or already-dead key stays as-is (C: valiswhite on gkey). */
		setnilvalue(&s.n.val)
		if s.n.keytt == LUA_VNIL {
			setdeadkey(s.n, nil)
		}
	}
}

// setdeadkey is C's setdeadvalue on gkey after removeentry: mark the key
// dead. C keeps the collectable pointer for findindex deadvalue==gcvalue;
// callers may pass it when the object is still live. Weak-sweep death
// must pass nil (Go GC FLAG — see gcSweepWeak).
func setdeadkey(n *node, gc unsafe.Pointer) {
	n.keytt = LUA_TDEADKEY
	n.kp = gc
	n.ki = 0
}
