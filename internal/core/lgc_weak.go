package core

import (
	"runtime"
	"sync"
	"sync/atomic"
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

type weakTableRef struct{ w weak.Pointer[Table] }

func (r weakTableRef) dead() bool { return r.w.Value() == nil }
func (r weakTableRef) value() Value {
	return makeTableValue(r.w.Value())
}

type weakUdataRef struct{ w weak.Pointer[Udata] }

func (r weakUdataRef) dead() bool { return r.w.Value() == nil }
func (r weakUdataRef) value() Value {
	return makeUdataValue(r.w.Value())
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
		return weakTableRef{weak.Make(hvalue(v))}
	case LUA_VUSERDATA:
		return weakUdataRef{weak.Make(uvalue(v))}
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

// luaC_checkWeakMode is the '__mode' half of 'luaC_checkfinalizer': a
// table whose metatable has a '__mode' field starts to be swept.
func luaC_checkWeakMode(L *State, t *Table, mt *Table) {
	if mt == nil {
		return
	}
	tm := luaH_getshortstr(mt, L.g.tmNames[TM_MODE])
	if !ttisstring(tm) { /* no mode (or not a string) */
		return
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
	t.weakMode = byte(mode)
	if mode == 0 {
		return
	}
	gcWeakMu.Lock()
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

// gcSweepWeak is 'cleartable' (lgc.c): it drops the entries of the weak
// tables whose object is no longer reachable from anywhere else.  It runs
// from the collector step, as it does in C.
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
	for _, t := range tables {
		mode := t.weakMode
		if mode == 0 {
			continue
		}
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
				}
			}
			if mode&weakVal != 0 {
				if w := makeWeakRef(&nd.val); w != nil {
					slots = append(slots, weakSlot{n: nd, v: &nd.val, w: w})
					setnilvalue(&nd.val)
				}
			}
		}
		/* the array part too: 'cache[1] = o' lands there, and PUC-Lua
		   drops it just the same */
		if mode&weakVal != 0 {
			for i := range t.array {
				if w := makeWeakRef(&t.array[i]); w != nil {
					slots = append(slots, weakSlot{v: &t.array[i], w: w})
					setnilvalue(&t.array[i])
				}
			}
		}
	}
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
				*s.v = sv
			}
			continue
		}
		if s.n == nil { /* the array part: the entry simply goes */
			setnilvalue(s.v)
			continue
		}
		/* gone: remove the entry the way lgc.c's 'removeentry' does --
		   the value goes, and the key is marked dead so that 'next'
		   steps over the hole without losing the collision chain */
		setnilvalue(&s.n.val)
		setdeadkey(s.n)
	}
}

// setdeadkey marks a node's key as dead: 'removeentry' leaves one behind
// so that 'next' can step over the hole while the collision chain stays
// intact.
func setdeadkey(n *node) { n.keytt = LUA_TDEADKEY; n.kp = nil; n.ki = 0 }
