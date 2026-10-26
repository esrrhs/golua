// Translation of ltable.h / ltable.c (PUC-Lua 5.3.4).
//
// The array part and the open-addressing hash part are kept, because
// 'next' must be able to resume a traversal from a given key; a Go map
// cannot do that (its iteration order is randomized).
//
// 5.3.4 surface: sizearray (no alimit/realasize/BITRAS), findindex with
// inline dead-key match (no equalkey/deadok), luaH_set/luaH_newkey return
// *Value (no luaH_finishset). Barriers / luaH_free are Go-GC no-ops.
package core

import (
	"math"
	"unsafe"
)

const (
	MAXABITS = 31 // sizeof(int)*CHAR_BIT-1 on LP64
	MAXASIZE = 1 << MAXABITS
	MAXHBITS = MAXABITS - 1
)

func lmod(s uint32, size int) uint32 { return s & uint32(size-1) }

func hashpow2(t *Table, n uint32) int { return int(lmod(n, sizenode(t))) }
func hashmod(t *Table, n uint32) int  { return int(n % uint32((sizenode(t)-1)|1)) }

func hashstr(t *Table, ts *TString) int  { return hashpow2(t, ts.hash) }
func hashboolean(t *Table, p uint32) int { return hashpow2(t, p) }
func hashpointer(t *Table, p unsafe.Pointer) int {
	return hashmod(t, uint32(uintptr(p)))
}

// hashint is the hash for integer keys (ltable.c 5.3.4: hashpow2, not 5.4 hashmod).
func hashint(t *Table, i int64) int { return hashpow2(t, uint32(uint64(i))) }

// l_hashfloat is the hash for float keys (ltable.c).
// Early NaN/Inf matches C's lua_numbertointeger failure → 0.
func l_hashfloat(n float64) int {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	frac, exp := math.Frexp(n)
	v := frac * 2147483648.0 // -INT_MIN
	if v != math.Trunc(v) {  /* is 'n' inf/-inf/NaN? */
		return 0
	}
	u := uint32(int64(exp)) + uint32(int64(v))
	if u <= uint32(math.MaxInt32) {
		return int(u)
	}
	return int(^u)
}

// mainpositionTV returns the main position (node index) of a key (ltable.c mainposition).
func mainpositionTV(L *State, t *Table, key *Value) int {
	switch ttypetag(key) {
	case LUA_VNUMINT:
		return hashint(t, ivalue(key))
	case LUA_VNUMFLT:
		return hashmod(t, uint32(l_hashfloat(fltvalue(key))))
	case LUA_VSHRSTR:
		return hashstr(t, tsvalue(key))
	case LUA_VLNGSTR:
		return hashpow2(t, luaS_hashlongstr(L, tsvalue(key)))
	case LUA_TBOOLEAN:
		if bvalue(key) {
			return hashboolean(t, 1)
		}
		return hashboolean(t, 0)
	case LUA_VLIGHTUSERDATA:
		return hashpointer(t, pvalue(key))
	case LUA_VLCF: // C: hashpointer(t, fvalue(key)); bits live in v.p
		return hashpointer(t, pvalue(key))
	default:
		return hashpointer(t, gcvalue(key))
	}
}

// getgeneric is the generic get version (ltable.c 5.3.4; no deadok).
func getgeneric(L *State, t *Table, key *Value) *Value {
	mode := t.syncWeakMode()
	n := mainpositionTV(L, t, key)
	for {
		var nk Value
		getnodekey(&nk, &t.nodes[n])
		if luaV_rawequalobj(&nk, key) {
			if mode&weakVal != 0 {
				return checkSlot(&t.nodes[n].val)
			}
			return &t.nodes[n].val
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &luaO_nilobject
		}
		n += int(nx)
	}
}

// getGenericNode is a Go helper: hash-chain lookup returning node index
// (-1 if absent). Used by luaH_getstrkey; not a C symbol.
func getGenericNode(L *State, t *Table, key *Value) int {
	n := mainpositionTV(L, t, key)
	for {
		var nk Value
		getnodekey(&nk, &t.nodes[n])
		if luaV_rawequalobj(&nk, key) {
			return n
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return -1
		}
		n += int(nx)
	}
}

// arrayindex returns the index for 'key' if it is an appropriate key to
// live in the array part of the table, 0 otherwise (ltable.c).
func arrayindex(key *Value) uint32 {
	if ttisinteger(key) {
		k := ivalue(key)
		if uint64(k)-1 < MAXASIZE { /* 0 < k && (lua_Unsigned)k <= MAXASIZE */
			return uint32(k) /* 'key' is an appropriate array index */
		}
	}
	return 0 /* 'key' did not match some condition */
}

// findIndex returns the traversal index of a key (ltable.c 5.3.4 findindex).
// First array part (via sizearray), then hash. Dead keys matched inline:
// rawequalobj OR (deadkey && collectable && deadvalue==gcvalue).
// No 5.4 equalkey(deadok).
func findIndex(L *State, t *Table, key *Value) uint32 {
	if ttisnil(key) {
		return 0 /* first iteration */
	}
	i := arrayindex(key)
	if i != 0 && i <= t.sizearray { /* is 'key' inside array part? */
		return i /* yes; that's the index */
	}
	n := mainpositionTV(L, t, key)
	for { /* check whether 'key' is somewhere in the chain */
		nd := &t.nodes[n]
		var nk Value
		getnodekey(&nk, nd)
		/* key may be dead already, but it is ok to use it in 'next' */
		if luaV_rawequalobj(&nk, key) ||
			(ttisdeadkey(&nk) && iscollectable(key) && deadvalue(&nk) == gcvalue(key)) {
			/* hash elements are numbered after array ones */
			return uint32(n+1) + t.sizearray
		}
		nx := nd.next
		if nx == 0 {
			luaG_runerror(L, "invalid key to 'next'") /* key not found */
		}
		n += int(nx)
	}
}

// luaH_next traverses a table (ltable.c). Key at keyIdx; value at keyIdx+1.
func luaH_next(L *State, t *Table, keyIdx int) bool {
	mode := t.syncWeakMode()
	i := findIndex(L, t, &L.stack[keyIdx]) /* find original element */
	for ; i < t.sizearray; i++ {           /* try first array part */
		v := checkSlot(&t.array[i])
		if !ttisnil(v) { /* a non-nil value? */
			sv := slotVal(v)
			if ttisnil(&sv) {
				continue
			}
			setivalue(&L.stack[keyIdx], int64(i)+1)
			L.stack[keyIdx+1] = sv
			return true
		}
	}
	for j := int(i) - int(t.sizearray); j < sizenode(t); j++ { /* hash part */
		nd := &t.nodes[j]
		_ = cleanNodeKey(nd)
		v := checkSlot(&nd.val)
		if !ttisnil(v) {
			var nk Value
			getnodekey(&nk, nd)
			if ttisnil(&nk) || ttisdeadkey(&nk) {
				continue
			}
			sv := slotVal(v)
			if ttisnil(&sv) {
				continue
			}
			L.stack[keyIdx] = nk
			L.stack[keyIdx+1] = sv
			if mode != 0 {
				/* For weak tables, clear any stale stack slots above keyIdx+2
				   so a discarded result of next(t) does not stay pinned above top. */
				for k := keyIdx + 2; k < len(L.stack); k++ {
					setnilvalue(&L.stack[k])
				}
			}
			return true
		}
	}
	return false
}

// dummyNode is C's static dummynode_: shared one-slot hash for empty tables.
var dummyNode = []node{{keytt: LUA_TNIL, val: Value{tt: LUA_TNIL}, next: 0}}

// isdummy: C signals with lastfree == NULL; Go uses lastfree < 0.
func isdummy(t *Table) bool { return t.lastfree < 0 }

func setdummy(t *Table) {
	t.nodes = dummyNode
	t.lsizenode = 0
	t.lastfree = -1
}

func allocsizenode(t *Table) int {
	if isdummy(t) {
		return 0
	}
	return sizenode(t)
}

// setnodevector creates a new hash part of the given size (ltable.c).
func setnodevector(L *State, t *Table, size uint32) {
	if size == 0 { /* no elements to hash part? */
		setdummy(t) /* use common 'dummynode' */
	} else {
		lsize := luaO_ceillog2(size)
		if lsize > MAXHBITS {
			luaG_runerror(L, "table overflow")
		}
		size = uint32(twoto(byte(lsize)))
		t.nodes = make([]node, size)
		for i := range t.nodes {
			t.nodes[i].next = 0
			setnilkey(&t.nodes[i])
			setnilvalue(&t.nodes[i].val)
		}
		t.lsizenode = byte(lsize)
		t.lastfree = int(size) /* all positions are free */
	}
}

// getfreepos returns a free position in the hash part (ltable.c).
// Free = nil key (ttisnil(gkey)); dead keys are not free.
func getfreepos(t *Table) int {
	if !isdummy(t) {
		for t.lastfree > 0 {
			t.lastfree--
			var k Value
			getnodekey(&k, &t.nodes[t.lastfree])
			if ttisnil(&k) {
				return t.lastfree
			}
		}
	}
	return -1 /* could not find a free place */
}

// luaH_new creates a new table (ltable.c).
func luaH_new(L *State) *Table {
	t := &Table{}
	t.flags = ^byte(0) /* cast_byte(~0) */
	t.array = nil
	t.sizearray = 0
	setnodevector(L, t, 0)
	return t
}

// invalidateTMcache invalidates the metamethod cache (ltable.h: flags = 0).
func invalidateTMcache(t *Table) {
	t.flags = 0
	t.modeCached = false
}

// luaH_free is a no-op: the Go GC reclaims the table (ltable.c IGNORE).
func luaH_free(L *State, t *Table) {}

// luaH_resize resizes a table (ltable.c). Uses sizearray only (no alimit).
// Go exchanges hash via a temp table (C grows/shrinks in place); reinsert
// of vanishing array slice + old hash matches C (GC/alloc ignored).
func luaH_resize(L *State, t *Table, nasize uint32, nhsize uint32) {
	mode := t.syncWeakMode()
	oldFlags := t.flags
	oldCachedMode := t.cachedMode
	oldModeCached := t.modeCached
	oldArray := t.array
	oldASize := t.sizearray
	oldNodes := t.nodes
	oldLastFree := t.lastfree
	oldLsizenode := t.lsizenode
	oldHSize := allocsizenode(t)

	if nasize > oldASize { /* array part must grow? */
		na := make([]Value, nasize)
		copy(na, oldArray)
		t.array = na
		t.sizearray = nasize
	}

	/* create new hash part with appropriate size */
	newt := &Table{}
	setnodevector(L, newt, nhsize)
	t.nodes, newt.nodes = newt.nodes, oldNodes
	t.lsizenode, newt.lsizenode = newt.lsizenode, oldLsizenode
	t.lastfree, newt.lastfree = newt.lastfree, oldLastFree
	/* Preserve self-metatable mode cache while t.nodes is being rebuilt. */
	t.flags = oldFlags
	t.cachedMode = oldCachedMode
	t.modeCached = true
	t.weakMode = mode

	if nasize < oldASize { /* array part must shrink? */
		t.sizearray = nasize
		/* re-insert elements from vanishing slice */
		for i := nasize; i < oldASize; i++ {
			v := checkSlot(&oldArray[i])
			if !ttisnil(v) {
				val := slotVal(v)
				if !ttisnil(&val) {
					luaH_setint(L, t, int64(i)+1, &val)
				}
			}
		}
		na := make([]Value, nasize)
		copy(na, oldArray[:nasize])
		t.array = na
	}

	/* re-insert elements from hash part (C: j from oldhsize-1 downto 0) */
	if oldHSize > 0 {
		for j := oldHSize - 1; j >= 0; j-- {
			var k Value
			getnodekey(&k, &newt.nodes[j])
			v := checkSlot(&newt.nodes[j].val)
			if !ttisnil(v) && !ttisnil(&k) && !ttisdeadkey(&k) {
				val := slotVal(v)
				if !ttisnil(&val) {
					luaH_setval(L, t, &k, &val)
				}
			}
		}
	}
	t.flags = oldFlags
	t.cachedMode = oldCachedMode
	t.modeCached = oldModeCached
	t.weakMode = mode
}

// luaH_resizearray resizes only the array part (ltable.c).
func luaH_resizearray(L *State, t *Table, nasize uint32) {
	nsize := allocsizenode(t)
	luaH_resize(L, t, nasize, uint32(nsize))
}

// computesizes computes the best size for the array part (ltable.c).
// twotoi > 0 stops uint32 overflow (C loops while *pna > twotoi/2 only).
func computesizes(nums *[MAXABITS + 1]uint32, pna *uint32) uint32 {
	var a, na, optimal uint32
	for i, twotoi := 0, uint32(1); twotoi > 0 && *pna > twotoi/2; i, twotoi = i+1, twotoi*2 {
		if nums[i] > 0 {
			a += nums[i]
			if a > twotoi/2 { /* more than half elements present? */
				optimal = twotoi
				na = a
			}
		}
	}
	*pna = na
	return optimal
}

func countint(key *Value, nums *[MAXABITS + 1]uint32) int {
	k := arrayindex(key)
	if k != 0 { /* is 'key' an appropriate array index? */
		nums[luaO_ceillog2(k)]++ /* count as such */
		return 1
	}
	return 0
}

func numusearray(t *Table, nums *[MAXABITS + 1]uint32) uint32 {
	var ause uint32
	i := uint32(1)
	asize := t.sizearray
	for lg, ttlg := 0, uint32(1); lg <= MAXABITS; lg, ttlg = lg+1, ttlg*2 {
		var lc uint32
		lim := ttlg
		if lim > asize {
			lim = asize
			if i > lim {
				break
			}
		}
		for ; i <= lim; i++ {
			if !ttisnil(checkSlot(&t.array[i-1])) {
				lc++
			}
		}
		nums[lg] += lc
		ause += lc
	}
	return ause
}

func numusehash(t *Table, nums *[MAXABITS + 1]uint32, pna *uint32) int {
	totaluse := 0 /* total number of elements */
	ause := 0     /* elements added to 'nums' (can go to array part) */
	i := sizenode(t)
	for i > 0 {
		i--
		n := &t.nodes[i]
		var k Value
		getnodekey(&k, n)
		v := checkSlot(&n.val)
		if !ttisnil(v) && !ttisnil(&k) && !ttisdeadkey(&k) {
			ause += countint(&k, nums)
			totaluse++
		}
	}
	*pna += uint32(ause)
	return totaluse
}

// rehash recomputes sizes of both parts (ltable.c 5.3.4; no setlimittosize).
func rehash(L *State, t *Table, ek *Value) {
	var nums [MAXABITS + 1]uint32
	for i := 0; i <= MAXABITS; i++ {
		nums[i] = 0 /* reset counts (Go make-zero already; mirror C) */
	}
	na := numusearray(t, &nums)           /* count keys in array part */
	totaluse := int(na)                   /* all those keys are integer keys */
	totaluse += numusehash(t, &nums, &na) /* count keys in hash part */
	/* count extra key */
	na += uint32(countint(ek, &nums))
	totaluse++
	/* compute new size for array part */
	asize := computesizes(&nums, &na)
	/* resize the table to new computed sizes */
	luaH_resize(L, t, asize, uint32(totaluse)-na)
}

// luaH_newkey inserts a new key into a hash table and returns its value slot
// (ltable.c 5.3.4: TValue *luaH_newkey). Caller writes the value (and TM
// cache / barrier). No nil-value early return — C always allocates the node.
func luaH_newkey(L *State, t *Table, key *Value) *Value {
	mode := t.syncWeakMode()
	if ttisnil(key) {
		luaG_runerror(L, "table index is nil")
	}
	if ttisfloat(key) {
		f := fltvalue(key)
		if k, ok := floatToIntEq(f); ok {
			var aux Value
			setivalue(&aux, k)
			key = &aux /* insert it as an integer */
		} else if math.IsNaN(f) {
			luaG_runerror(L, "table index is NaN")
		}
	}
	mp := mainpositionTV(L, t, key)
	if !isdummy(t) {
		var mpkCheck Value
		getnodekey(&mpkCheck, &t.nodes[mp])
		_ = checkSlot(&t.nodes[mp].val)
	}
	if !ttisnil(&t.nodes[mp].val) || isdummy(t) { /* main position is taken? */
		f := getfreepos(t) /* get a free place */
		if f < 0 {         /* cannot find a free place? */
			rehash(L, t, key) /* grow table */
			/* whatever called 'newkey' takes care of TM cache */
			return luaH_set(L, t, key) /* insert key into grown table */
		}
		var mpk Value
		getnodekey(&mpk, &t.nodes[mp])
		othern := mainpositionTV(L, t, &mpk)
		if othern != mp { /* is colliding node out of its main position? */
			/* yes; move colliding node into free position */
			for othern+int(t.nodes[othern].next) != mp { /* find previous */
				othern += int(t.nodes[othern].next)
			}
			t.nodes[othern].next = int32(f - othern) /* rechain to point to 'f' */
			t.nodes[f] = t.nodes[mp]                 /* copy colliding node into free pos. */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next += int32(mp - f) /* correct 'next' */
				t.nodes[mp].next = 0             /* now 'mp' is free */
			}
			setnilvalue(&t.nodes[mp].val)
		} else { /* colliding node is in its own main position */
			/* new node will go into free position */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next = int32((mp + int(t.nodes[mp].next)) - f) /* chain new position */
			}
			t.nodes[mp].next = int32(f - mp)
			mp = f
		}
	}
	setnodekeyMode(&t.nodes[mp], key, mode)
	/* luaC_barrierback omitted (Go GC) */
	return &t.nodes[mp].val
}

// luaH_getint searches an integer key (ltable.c 5.3.4; sizearray bound).
func luaH_getint(t *Table, key int64) *Value {
	/* (1 <= key && key <= t->sizearray) */
	if uint64(key)-1 < uint64(t.sizearray) {
		if t.metatable != nil && t.syncWeakMode()&weakVal != 0 {
			return checkSlot(&t.array[key-1])
		}
		return &t.array[key-1]
	}
	n := hashint(t, key)
	for {
		if t.nodes[n].keytt == LUA_VNUMINT && t.nodes[n].ki == key {
			if t.metatable != nil && t.syncWeakMode()&weakVal != 0 {
				return checkSlot(&t.nodes[n].val)
			}
			return &t.nodes[n].val
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &luaO_nilobject
		}
		n += int(nx)
	}
}

// luaH_getshortstr searches a short-string key (ltable.c).
func luaH_getshortstr(t *Table, key *TString) *Value {
	n := hashstr(t, key)
	for {
		if t.nodes[n].keytt == ctb(LUA_VSHRSTR) && luaS_eqshrstr((*TString)(t.nodes[n].kp), key) {
			if t.metatable != nil && t.syncWeakMode()&weakVal != 0 {
				return checkSlot(&t.nodes[n].val)
			}
			return &t.nodes[n].val
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &luaO_nilobject
		}
		n += int(nx)
	}
}

// luaH_getstrkey answers the string a string key is *stored under*, or nil
// when absent (llex: keystrval(nodefromval(o)) / C keyfromval).
//
// FLAG: Go-only — not in C ltable. Needed because node keys are not addressable
// as TValue*; llex interns chunk strings and addk may overwrite the value with
// a constant index while the key must remain the same TString object.
func luaH_getstrkey(L *State, t *Table, key *TString) *TString {
	if key.tt == LUA_VSHRSTR {
		n := hashstr(t, key)
		for {
			if t.nodes[n].keytt == ctb(LUA_VSHRSTR) &&
				luaS_eqshrstr((*TString)(t.nodes[n].kp), key) {
				return (*TString)(t.nodes[n].kp)
			}
			nx := t.nodes[n].next
			if nx == 0 {
				return nil
			}
			n += int(nx)
		}
	}
	var ko Value
	setsvalue(&ko, key)
	if i := getGenericNode(L, t, &ko); i >= 0 {
		return (*TString)(t.nodes[i].kp)
	}
	return nil
}

// luaH_getstr searches a string key (ltable.c). L is needed for longstr hash.
func luaH_getstr(L *State, t *Table, key *TString) *Value {
	if key.tt == LUA_VSHRSTR {
		return luaH_getshortstr(t, key)
	}
	var ko Value
	setsvalue(&ko, key)
	return getgeneric(L, t, &ko)
}

// luaH_get is the main search function (ltable.c).
func luaH_get(L *State, t *Table, key *Value) *Value {
	switch ttypetag(key) {
	case LUA_VSHRSTR:
		return luaH_getshortstr(t, tsvalue(key))
	case LUA_VNUMINT:
		return luaH_getint(t, ivalue(key))
	case LUA_VNIL:
		return &luaO_nilobject
	case LUA_VNUMFLT:
		if k, ok := floatToIntEq(fltvalue(key)); ok {
			return luaH_getint(t, k) /* use specialized version */
		}
		fallthrough
	default:
		return getgeneric(L, t, key)
	}
}

// luaH_set returns the value slot for 'key', creating it if absent
// (ltable.c 5.3.4: TValue *luaH_set). Caller writes the value and handles
// GC barrier / invalidateTMcache. No luaH_finishset (5.4 purged).
func luaH_set(L *State, t *Table, key *Value) *Value {
	p := luaH_get(L, t, key)
	if p != &luaO_nilobject {
		return p
	}
	return luaH_newkey(L, t, key)
}

// luaH_setval sets t[key] = value, applying weak-table mode to the stored
// key and value when t is a weak table, and weakening deleted collectable
// keys when value is nil so deleted keys are not pinned in strong tables.
func luaH_setval(L *State, t *Table, key, value *Value) {
	mode := t.syncWeakMode()
	slot := luaH_set(L, t, key)
	if mode == 0 && !ttisnil(value) && value.tt&BIT_WEAK == 0 {
		*slot = *value
		if key.tt&BIT_ISCOLLECTABLE != 0 && ttypetag(key) != LUA_VSHRSTR && len(t.nodes) > 0 {
			luaH_strengthenSlotKey(t, slot)
		}
		return
	}
	luaH_setvalSlow(t, slot, value, mode)
}

func luaH_strengthenSlotKey(t *Table, slot *Value) {
	first := uintptr(unsafe.Pointer(&t.nodes[0]))
	end := first + uintptr(len(t.nodes))*unsafe.Sizeof(t.nodes[0])
	sp := uintptr(unsafe.Pointer(slot))
	if sp >= first && sp < end {
		idx := (sp - first) / unsafe.Sizeof(t.nodes[0])
		strengthenNodeKey(&t.nodes[idx])
	}
}

func luaH_setvalSlow(t *Table, slot, value *Value, mode byte) {
	storeValMode(slot, value, mode)
	if ttisnil(value) && len(t.nodes) > 0 {
		first := uintptr(unsafe.Pointer(&t.nodes[0]))
		end := first + uintptr(len(t.nodes))*unsafe.Sizeof(t.nodes[0])
		sp := uintptr(unsafe.Pointer(slot))
		if sp >= first && sp < end {
			idx := (sp - first) / unsafe.Sizeof(t.nodes[0])
			weakenNodeKey(&t.nodes[idx])
		}
	} else if !ttisnil(value) && mode&weakKey == 0 && len(t.nodes) > 0 {
		luaH_strengthenSlotKey(t, slot)
	}
}

// luaH_setSlotVal writes value into an already-located non-empty slot of t.
func luaH_setSlotVal(t *Table, slot, value *Value) {
	if t.metatable == nil && uint8(value.tt-1) < BIT_WEAK-1 {
		*slot = *value
	} else {
		luaH_setSlotValSlow(t, slot, value)
	}
}

func luaH_setSlotValSlow(t *Table, slot, value *Value) {
	mode := t.syncWeakMode()
	storeValMode(slot, value, mode)
	if ttisnil(value) && len(t.nodes) > 0 {
		first := uintptr(unsafe.Pointer(&t.nodes[0]))
		end := first + uintptr(len(t.nodes))*unsafe.Sizeof(t.nodes[0])
		sp := uintptr(unsafe.Pointer(slot))
		if sp >= first && sp < end {
			idx := (sp - first) / unsafe.Sizeof(t.nodes[0])
			weakenNodeKey(&t.nodes[idx])
		}
	}
}

// luaH_setint does a raw set with an integer key (ltable.c).
func luaH_setint(L *State, t *Table, key int64, value *Value) {
	mode := t.syncWeakMode()
	p := luaH_getint(t, key)
	var cell *Value
	if p != &luaO_nilobject {
		cell = p
	} else {
		var k Value
		setivalue(&k, key)
		cell = luaH_newkey(L, t, &k)
	}
	storeValMode(cell, value, mode)
}

// unboundSearch finds a boundary in the hash part (ltable.c unbound_search).
func unboundSearch(t *Table, j uint32) uint32 {
	i := j /* i is zero or a present index */
	jj := uint64(j) + 1
	/* find 'i' and 'j' such that i is present and j is not */
	for !ttisnil(luaH_getint(t, int64(jj))) {
		i = uint32(jj)
		if jj > uint64(math.MaxInt32)/2 { /* overflow? */
			/* table was built with bad purposes: resort to linear search */
			i = 1
			for !ttisnil(luaH_getint(t, int64(i))) {
				i++
			}
			return i - 1
		}
		jj *= 2
	}
	j = uint32(jj)
	/* now do a binary search between them */
	for j-i > 1 {
		m := (i + j) / 2
		if ttisnil(luaH_getint(t, int64(m))) {
			j = m
		} else {
			i = m
		}
	}
	return i
}

// luaH_getn returns a boundary of the table (ltable.c 5.3.4).
// Uses sizearray only — no 5.4 alimit / BITRAS shrinking.
func luaH_getn(t *Table) int {
	t.syncWeakMode()
	j := t.sizearray
	if j > 0 && ttisnil(checkSlot(&t.array[j-1])) {
		/* there is a boundary in the array part: (binary) search for it */
		var i uint32
		for j-i > 1 {
			m := (i + j) / 2
			if ttisnil(checkSlot(&t.array[m-1])) {
				j = m
			} else {
				i = m
			}
		}
		return int(i)
	}
	/* else must find a boundary in hash part */
	if isdummy(t) { /* hash part is empty? */
		return int(j) /* that is easy... */
	}
	return int(unboundSearch(t, j))
}

// floatToIntEq converts a float with an exact integer value (F2Ieq /
// luaV_tointeger(..., 0)). Helper for get/newkey float→int paths.
func floatToIntEq(n float64) (int64, bool) {
	var k int64
	if floatToInt(n, &k) {
		return k, true
	}
	return 0, false
}

// luaH_setmetatable sets (or clears) the metatable of a table.
// FLAG: trivial Go helper; not a C ltable.c symbol (field write elsewhere).
func luaH_setmetatable(t *Table, mt *Table) { t.metatable = mt }
