package core

import (
	"sync"
	"sync/atomic"
	"unsafe"
	"weak"
)

// Weak tables (lgc.c's 'cleartable' and 'luaC_checkfinalizer' for
// '__mode'), delegated directly to the Go runtime via weak.Pointer.
//
// Weak keys and weak values of collectable object types (Table, Udata,
// LClosure, CClosure, State) are stored inside Table nodes and array
// slots as weak.Pointer handles tagged with BIT_WEAK. Because the table
// never holds strong Go references to its weak keys or weak values,
// Go's runtime garbage collector naturally reclaims unreachable entries
// on its own schedule without artificial runtime.GC() cycles.

// weakKey / weakVal are the two halves of a table's '__mode'.
const (
	weakKey = 1 << 0
	weakVal = 1 << 1
)

// weakRef is a weak reference to a collectable object; 'dead' says the
// object is gone. Only tables, userdata, closures and threads can be
// weak: numbers, strings and booleans are values, not objects.
type weakRef interface {
	dead() bool
	value() Value
}

type weakTableRef struct {
	w weak.Pointer[Table]
	g *globalState // set for gcWatch entries; nil for weak-table keys
}

func (r weakTableRef) dead() bool {
	return weakPtrDead(*(*unsafe.Pointer)(unsafe.Pointer(&r.w)))
}
func (r weakTableRef) value() Value {
	return makeTableValue(r.w.Value())
}

type weakUdataRef struct {
	w weak.Pointer[Udata]
	g *globalState // set for gcWatch entries; nil for weak-table keys
}

func (r weakUdataRef) dead() bool {
	return weakPtrDead(*(*unsafe.Pointer)(unsafe.Pointer(&r.w)))
}
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

// makeWeakPtr converts a strong collectable object pointer into the
// underlying weak.Pointer handle word. Returns (p, false) for types
// that are never weak in Lua (numbers, strings, booleans, light
// userdata, light C functions).
func makeWeakPtr(tt uint8, p unsafe.Pointer) (unsafe.Pointer, bool) {
	if p == nil {
		return nil, false
	}
	switch int(tt) & 0x3F {
	case LUA_VTABLE:
		wp := weak.Make((*Table)(p))
		return *(*unsafe.Pointer)(unsafe.Pointer(&wp)), true
	case LUA_VUSERDATA:
		wp := weak.Make((*Udata)(p))
		return *(*unsafe.Pointer)(unsafe.Pointer(&wp)), true
	case LUA_VLCL:
		wp := weak.Make((*LClosure)(p))
		return *(*unsafe.Pointer)(unsafe.Pointer(&wp)), true
	case LUA_VCCL:
		wp := weak.Make((*CClosure)(p))
		return *(*unsafe.Pointer)(unsafe.Pointer(&wp)), true
	case LUA_VTHREAD:
		wp := weak.Make((*State)(p))
		return *(*unsafe.Pointer)(unsafe.Pointer(&wp)), true
	}
	return p, false
}

// resolveWeakPtr resolves a weak.Pointer handle word back to a strong
// object pointer, or returns nil if Go's GC has reclaimed the object.
func resolveWeakPtr(tt uint8, wp unsafe.Pointer) unsafe.Pointer {
	if wp == nil {
		return nil
	}
	switch int(tt) & 0x3F {
	case LUA_VTABLE:
		w := *(*weak.Pointer[Table])(unsafe.Pointer(&wp))
		return unsafe.Pointer(w.Value())
	case LUA_VUSERDATA:
		w := *(*weak.Pointer[Udata])(unsafe.Pointer(&wp))
		return unsafe.Pointer(w.Value())
	case LUA_VLCL:
		w := *(*weak.Pointer[LClosure])(unsafe.Pointer(&wp))
		return unsafe.Pointer(w.Value())
	case LUA_VCCL:
		w := *(*weak.Pointer[CClosure])(unsafe.Pointer(&wp))
		return unsafe.Pointer(w.Value())
	case LUA_VTHREAD:
		w := *(*weak.Pointer[State])(unsafe.Pointer(&wp))
		return unsafe.Pointer(w.Value())
	}
	return wp
}

// weakPtrDead checks whether a weak.Pointer handle word has been cleared
// by Go's GC without converting it to a strong pointer (which would shade
// the object during concurrent mark phase).
func weakPtrDead(wp unsafe.Pointer) bool {
	return wp == nil || (*atomic.Uintptr)(wp).Load() == 0
}

// cleanNodeKey checks whether a weak node key has been collected by Go's GC
// without shading it. Returns true if the node key is nil or dead.
func cleanNodeKey(n *node) bool {
	if n.keytt == LUA_VNIL || n.keytt == LUA_TDEADKEY {
		return true
	}
	if n.keytt&BIT_WEAK != 0 && weakPtrDead(n.kp) {
		setnilvalue(&n.val)
		setdeadkey(n, nil)
		return true
	}
	return false
}

// getnodekeyWeak is the slow path of getnodekey for BIT_WEAK keys.
func getnodekeyWeak(dst *Value, n *node) {
	p := resolveWeakPtr(n.keytt, n.kp)
	if p == nil {
		setnilvalue(&n.val)
		setdeadkey(n, nil)
		*dst = Value{tt: LUA_TDEADKEY}
		return
	}
	dst.p = p
	dst.i = n.ki
	dst.tt = n.keytt &^ BIT_WEAK
}

// checkSlot checks whether a table value slot holding a weak reference
// has been collected by Go's GC (without shading a live weak target).
func checkSlot(v *Value) *Value {
	if v.tt&BIT_WEAK != 0 {
		return checkSlotSlow(v)
	}
	return v
}

func checkSlotSlow(v *Value) *Value {
	if weakPtrDead(v.p) {
		setnilvalue(v)
	}
	return v
}

// slotVal reads a Value from a table slot and returns a strong Value
// suitable for placing on the Lua stack.
func slotVal(v *Value) Value {
	if v.tt&BIT_WEAK != 0 {
		return slotValSlow(v)
	}
	return *v
}

func slotValSlow(v *Value) Value {
	p := resolveWeakPtr(v.tt, v.p)
	if p == nil {
		setnilvalue(v)
		return luaO_nilobject
	}
	return Value{tt: v.tt &^ BIT_WEAK, p: p, i: v.i}
}

func weakenVal(v *Value) {
	if v.tt&BIT_WEAK != 0 {
		if weakPtrDead(v.p) {
			setnilvalue(v)
		}
		return
	}
	if wp, ok := makeWeakPtr(v.tt, v.p); ok {
		v.p = wp
		v.tt |= BIT_WEAK
	}
}

func strengthenVal(v *Value) {
	if v.tt&BIT_WEAK == 0 {
		return
	}
	p := resolveWeakPtr(v.tt, v.p)
	if p == nil {
		setnilvalue(v)
		return
	}
	v.p = p
	v.tt &^= BIT_WEAK
}

func weakenNodeKey(n *node) {
	if n.keytt&BIT_WEAK != 0 {
		if weakPtrDead(n.kp) {
			setnilvalue(&n.val)
			setdeadkey(n, nil)
		}
		return
	}
	if wp, ok := makeWeakPtr(n.keytt, n.kp); ok {
		n.kp = wp
		n.keytt |= BIT_WEAK
	}
}

func strengthenNodeKey(n *node) {
	if n.keytt&BIT_WEAK == 0 {
		return
	}
	p := resolveWeakPtr(n.keytt, n.kp)
	if p == nil {
		setnilvalue(&n.val)
		setdeadkey(n, nil)
		return
	}
	n.kp = p
	n.keytt &^= BIT_WEAK
}

func storeValMode(dst, src *Value, mode byte) {
	sv := slotVal(src)
	*dst = sv
	if mode&weakVal != 0 {
		weakenVal(dst)
	}
}

func setnodekeyMode(n *node, k *Value, mode byte) {
	sk := slotVal(k)
	setnodekey(n, &sk)
	if mode&weakKey != 0 {
		weakenNodeKey(n)
	}
}

const gcWeakPruneMin = 256

var (
	gcWeakMu        sync.Mutex
	gcWeakTables    []weak.Pointer[Table] // the weak tables themselves, weakly
	gcWeakNextPrune = gcWeakPruneMin
	gcWeakCount     atomic.Int32 // nonzero while any weak table exists
)

// parseWeakModeTable reads TM_MODE from mt without needing a State,
// caching the result on mt until invalidateTMcache clears modeCached.
func parseWeakModeTable(mt *Table) byte {
	if mt == nil {
		return 0
	}
	if mt.modeCached {
		return mt.cachedMode
	}
	if (mt.flags & (1 << uint(TM_MODE))) != 0 {
		mt.cachedMode = 0
		mt.modeCached = true
		return 0
	}
	var tm *Value
	for i := range mt.nodes {
		nd := &mt.nodes[i]
		if nd.keytt == ctb(LUA_VSHRSTR) && (*TString)(nd.kp).val == "__mode" {
			tm = checkSlot(&nd.val)
			break
		}
	}
	if tm == nil || ttisnil(tm) {
		mt.flags |= byte(1 << uint(TM_MODE))
		mt.cachedMode = 0
		mt.modeCached = true
		return 0
	}
	var mode byte
	if ttisstring(tm) {
		s := tsvalue(tm).val
		for i := 0; i < len(s); i++ {
			switch s[i] {
			case 'k':
				mode |= weakKey
			case 'v':
				mode |= weakVal
			}
		}
	}
	mt.cachedMode = mode
	mt.modeCached = true
	return mode
}

// parseWeakMode reads TM_MODE from mt the way traversetable does each mark.
// Returns 0 for nil mt, non-string __mode, or a mode without 'k'/'v'.
func parseWeakMode(L *State, mt *Table) byte {
	_ = L
	return parseWeakModeTable(mt)
}

func gcTrackWeakTable(t *Table) {
	if t.weakTracked {
		gcWeakCount.Store(1)
		return
	}
	t.weakTracked = true
	gcWeakMu.Lock()
	if len(gcWeakTables) >= gcWeakNextPrune {
		alive := gcWeakTables[:0]
		for i := range gcWeakTables {
			p := *(*unsafe.Pointer)(unsafe.Pointer(&gcWeakTables[i]))
			if !weakPtrDead(p) {
				alive = append(alive, gcWeakTables[i])
			}
		}
		gcWeakTables = alive
		gcWeakNextPrune = len(gcWeakTables)*2 + gcWeakPruneMin
	}
	gcWeakTables = append(gcWeakTables, weak.Make(t))
	gcWeakMu.Unlock()
	gcWeakCount.Store(1)
}

// syncWeakMode ensures t.weakMode and its stored entries match its
// current metatable's __mode. Fast inlineable check when metatable is nil.
func (t *Table) syncWeakMode() byte {
	if t.metatable == nil && t.weakMode == 0 {
		return 0
	}
	return t.syncWeakModeSlow()
}

func (t *Table) syncWeakModeSlow() byte {
	mt := t.metatable
	if mt == nil {
		if t.weakMode != 0 {
			t.applyWeakMode(0)
		}
		return 0
	}
	if mt.modeCached && mt.cachedMode == t.weakMode {
		return t.weakMode
	}
	mode := parseWeakModeTable(mt)
	if mode != t.weakMode {
		t.applyWeakMode(mode)
	}
	return mode
}

// applyWeakMode converts existing entries in t to weak or strong
// references according to mode, and cleans any dead weak entries.
func (t *Table) applyWeakMode(mode byte) {
	t.weakMode = mode
	if mode != 0 {
		gcTrackWeakTable(t)
	}
	for i := range t.array {
		v := &t.array[i]
		if v.tt&BIT_WEAK != 0 && weakPtrDead(v.p) {
			setnilvalue(v)
			continue
		}
		if mode&weakVal != 0 {
			weakenVal(v)
		} else {
			strengthenVal(v)
		}
	}
	for i := range t.nodes {
		nd := &t.nodes[i]
		kdead := cleanNodeKey(nd)
		v := checkSlot(&nd.val)
		if ttisnil(v) {
			if !kdead {
				weakenNodeKey(nd)
			}
			continue
		}
		if mode&weakKey != 0 {
			weakenNodeKey(nd)
		} else {
			strengthenNodeKey(nd)
		}
		if mode&weakVal != 0 {
			weakenVal(&nd.val)
		} else {
			strengthenVal(&nd.val)
		}
	}
}

// luaC_checkWeakMode is the '__mode' half of setmetatable: a table whose
// metatable has a weak '__mode' stores its weak keys/values as weak.Pointer.
func luaC_checkWeakMode(L *State, t *Table, mt *Table) {
	mode := parseWeakModeTable(mt)
	if mode != 0 && L != nil && L.g != nil {
		L.g.gcDebt = 0
	}
	t.applyWeakMode(mode)
}

// gcSweepWeak cleans dead weak entries (already collected by Go's runtime
// GC) across all tracked weak tables and prunes dead/non-weak tables from
// gcWeakTables. It never calls runtime.GC().
func (L *State) gcSweepWeak() {
	_ = L
	if gcWeakCount.Load() == 0 {
		return
	}
	gcWeakMu.Lock()
	snap := append([]weak.Pointer[Table](nil), gcWeakTables...)
	alive := gcWeakTables[:0]
	for i := range gcWeakTables {
		p := *(*unsafe.Pointer)(unsafe.Pointer(&gcWeakTables[i]))
		if !weakPtrDead(p) {
			alive = append(alive, gcWeakTables[i])
		}
	}
	gcWeakTables = alive
	n := len(alive)
	gcWeakMu.Unlock()
	if n == 0 {
		gcWeakCount.Store(0)
		return
	}
	anyWeak := false
	for i := range snap {
		p := *(*unsafe.Pointer)(unsafe.Pointer(&snap[i]))
		if weakPtrDead(p) {
			continue
		}
		t := snap[i].Value()
		if t == nil {
			continue
		}
		mode := parseWeakModeTable(t.metatable)
		t.applyWeakMode(mode)
		if mode != 0 {
			anyWeak = true
		}
	}
	if !anyWeak {
		gcWeakMu.Lock()
		keep := gcWeakTables[:0]
		for i := range gcWeakTables {
			p := *(*unsafe.Pointer)(unsafe.Pointer(&gcWeakTables[i]))
			if !weakPtrDead(p) {
				if t := gcWeakTables[i].Value(); t != nil {
					if t.weakMode != 0 {
						keep = append(keep, gcWeakTables[i])
					} else {
						t.weakTracked = false
					}
				}
			}
		}
		gcWeakTables = keep
		if len(keep) == 0 {
			gcWeakCount.Store(0)
		}
		gcWeakMu.Unlock()
	}
}

// setdeadkey is C's setdeadvalue on gkey after removeentry: mark the key
// dead.
func setdeadkey(n *node, gc unsafe.Pointer) {
	n.keytt = LUA_TDEADKEY
	n.kp = gc
	n.ki = 0
}
