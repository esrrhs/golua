// Translation of ltable.h / ltable.c (PUC-Lua 5.4).
//
// The array part and the open-addressing hash part are kept, because
// 'next' must be able to resume a traversal from a given key; a Go map
// cannot do that (its iteration order is randomized).
package core

import (
	"math"
	"unsafe"
)

const (
	MAXABITS = 31
	MAXASIZE = 1 << MAXABITS
	MAXHBITS = MAXABITS - 1
)

// absentKey is the value returned for a key not found in a table
// (ltable.c's 'absentkey').
var absentKey = Value{tt: LUA_VABSTKEY}

func lmod(s uint32, size int) uint32 { return s & uint32(size-1) }

func hashpow2(t *Table, n uint32) int { return int(lmod(n, sizenode(t))) }
func hashmod(t *Table, n uint32) int  { return int(n % uint32((sizenode(t)-1)|1)) }

func hashstr(t *Table, ts *TString) int  { return hashpow2(t, ts.hash) }
func hashboolean(t *Table, p uint32) int { return hashpow2(t, p) }
func hashpointer(t *Table, p unsafe.Pointer) int {
	return hashmod(t, uint32(uintptr(p)))
}

// hashint is the hash for integer keys (ltable.c).
func hashint(t *Table, i int64) int { return hashmod(t, uint32(uint64(i))) }

// l_hashfloat is the hash for float keys (ltable.c).
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

// mainpositionTV returns the main position (node index) of a key.
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
	case LUA_VFALSE:
		return hashboolean(t, 0)
	case LUA_VTRUE:
		return hashboolean(t, 1)
	case LUA_VLIGHTUSERDATA:
		return hashpointer(t, pvalue(key))
	default:
		return hashpointer(t, gcvalue(key))
	}
}

// keyisdead is 'keyisdead(n)' of ltable.c: true when the node's key has
// been turned into a dead key (lgc.c's 'clearkey', and 'removeentry' for
// a weak table).
//
// It must look at the key's type and nothing else.  A node's 'next' is a
// *chain offset*, and a negative one is ordinary: 'f - mp' is negative
// whenever the free slot found for a colliding key sits below its main
// position, which is the common case.  Testing 'next' for a sentinel
// made any node whose offset happened to be -2 look dead, and 'equalkey'
// then compared a key of one type against a node holding another -- a
// closure against a long string, for one -- which is a Go panic inside
// 'next'.
func keyisdead(n *node) bool { return n.keytt == LUA_TDEADKEY }

// equalkey reports whether k1 equals the key stored in node n.
func equalkey(k1 *Value, n *node, deadok bool) bool {
	if (k1.tt != n.keytt) && !(deadok && keyisdead(n) && iscollectable(k1)) {
		return false /* cannot be same key */
	}
	switch n.keytt {
	case LUA_VNIL, LUA_VFALSE, LUA_VTRUE:
		return true
	case LUA_VNUMINT:
		return ivalue(k1) == n.ki
	case LUA_VNUMFLT:
		return fltvalue(k1) == math.Float64frombits(uint64(n.ki))
	case LUA_VLIGHTUSERDATA:
		return pvalue(k1) == n.kp
	case ctb(LUA_VLNGSTR):
		return luaS_eqlngstr(tsvalue(k1), (*TString)(n.kp))
	case ctb(LUA_VSHRSTR):
		return luaS_eqshrstr(tsvalue(k1), (*TString)(n.kp))
	default:
		return gcvalue(k1) == n.kp
	}
}

// luaH_realasize returns the real size of the array part (ltable.c).
func luaH_realasize(t *Table) uint32 {
	if isrealasize(t) || ispow2u(t.alimit) {
		return t.alimit
	}
	size := t.alimit
	size |= (size >> 1)
	size |= (size >> 2)
	size |= (size >> 4)
	size |= (size >> 8)
	size |= (size >> 16)
	size++
	return size
}

func ispow2u(x uint32) bool { return x != 0 && (x&(x-1)) == 0 }

func limitequalsasize(t *Table) bool { return isrealasize(t) || ispow2u(t.alimit) }

func setlimittosize(t *Table) uint32 {
	t.alimit = luaH_realasize(t)
	setrealasize(t)
	return t.alimit
}

// getGenericNode looks a key up in the hash part, returning its node index.
func getGenericNode(L *State, t *Table, key *Value, deadok bool) int {
	if sizenode(t) == 0 {
		return -1
	}
	i := mainpositionTV(L, t, key)
	for {
		if equalkey(key, &t.nodes[i], deadok) {
			return i
		}
		nx := t.nodes[i].next
		if nx == 0 {
			return -1
		}
		i += int(nx)
	}
}

// getgeneric is the generic get version (ltable.c).
func getgeneric(L *State, t *Table, key *Value, deadok bool) *Value {
	if i := getGenericNode(L, t, key, deadok); i >= 0 {
		return &t.nodes[i].val
	}
	return &absentKey
}

// arrayindex returns the index for 'k' if it may live in the array part.
func arrayindex(k int64) uint32 {
	if uint64(k)-1 < MAXASIZE {
		return uint32(k)
	}
	return 0
}

// luaH_next traverses a table (ltable.c). The key is at 'keyIdx' and the
// value is written at 'keyIdx + 1'.
func luaH_next(L *State, t *Table, keyIdx int) bool {
	asize := luaH_realasize(t)
	i := findIndex(L, t, &L.stack[keyIdx], asize)
	for ; i < asize; i++ {
		if !isempty(&t.array[i]) { /* a non-nil entry? */
			setivalue(&L.stack[keyIdx], int64(i)+1)
			L.stack[keyIdx+1] = t.array[i]
			return true
		}
	}
	for j := int(i) - int(asize); j < sizenode(t); j++ {
		if !isempty(&t.nodes[j].val) {
			getnodekey(&L.stack[keyIdx], &t.nodes[j])
			L.stack[keyIdx+1] = t.nodes[j].val
			return true
		}
	}
	return false
}

// findIndex returns the traversal index of a key (ltable.c).
func findIndex(L *State, t *Table, key *Value, asize uint32) uint32 {
	if ttisnil(key) {
		return 0 /* first iteration */
	}
	var i uint32
	if ttisinteger(key) {
		i = arrayindex(ivalue(key))
	}
	if i != 0 && i-1 < asize {
		return i /* 'key' is inside the array part */
	}
	n := getGenericNode(L, t, key, true)
	if n < 0 {
		luaG_runerror(L, "invalid key to 'next'") /* key not found */
	}
	return uint32(n+1) + asize /* hash elements come after array ones */
}

// dummyNode is the counterpart of PUC-Lua's static 'dummynode': a one-slot
// hash part shared by every table that has no hash entries, so that the
// search loops can always touch at least one node.
var dummyNode = []node{{keytt: LUA_VNIL, val: Value{tt: LUA_VEMPTY}, next: 0}}

// isdummy reports whether the table is using the shared dummy node. PUC-Lua
// signals this with 'lastfree == NULL'.
func isdummy(t *Table) bool { return t.lastfree < 0 }

func setdummy(t *Table) {
	t.nodes = dummyNode
	t.lsizenode = 0
	t.lastfree = -1
}

// setnodevector creates a new hash part of the given size (ltable.c).
func setnodevector(L *State, t *Table, size uint32) {
	if size == 0 { /* no elements to hash part? */
		setdummy(t) /* use common 'dummyNode' */
	} else {
		lsize := luaO_ceillog2(size)
		size = uint32(twoto(byte(lsize)))
		t.nodes = make([]node, size)
		for i := range t.nodes {
			t.nodes[i].next = 0
			setnilkey(&t.nodes[i])
			setempty(&t.nodes[i].val)
		}
		t.lsizenode = byte(lsize)
		t.lastfree = int(size) /* all positions are free */
	}
}

// getfreepos returns a free position in the hash part (ltable.c).
func getfreepos(t *Table) int {
	if !isdummy(t) { /* is there a real node array? */
		for t.lastfree > 0 {
			t.lastfree--
			if t.nodes[t.lastfree].keytt == LUA_VNIL {
				return t.lastfree
			}
		}
	}
	return -1 /* could not find a free place */
}

// maskflags is the set of flag bits used for the metamethod cache (ltm.h).
// Bit 7 is reserved for 'isrealasize', so a new table gets the metamethod
// bits set ("no metamethods cached as absent yet") but bit 7 clear.
const maskflags = byte(0x3F)

// luaH_new creates a new table (ltable.c).
func luaH_new(L *State) *Table {
	t := &Table{}
	t.flags = maskflags /* table has no metamethod fields */
	t.array = nil
	t.alimit = 0
	setnodevector(L, t, 0)
	return t
}

// tableArray4 stores the header and the common four-element literal in
// one Go allocation. A regular table followed by luaH_resize needs one
// object for the Table and another for []Value; an interior pointer to
// table keeps this whole object (including its pointer-scanned array)
// alive.
type tableArray4 struct {
	table Table
	array [4]Value
}

func luaH_newarray4(L *State) *Table {
	x := &tableArray4{}
	t := &x.table
	t.flags = maskflags
	t.array = x.array[:]
	t.alimit = 4
	setrealasize(t)
	setnodevector(L, t, 0)
	return t
}

// invalidateTMcache invalidates the metamethod cache of a table.
func invalidateTMcache(t *Table) { t.flags &= ^maskflags }

// luaH_free is a no-op: the Go GC reclaims the table (ltable.c).
func luaH_free(L *State, t *Table) {}

// luaH_resize resizes a table (ltable.c).
func luaH_resize(L *State, t *Table, newasize uint32, nhsize uint32) {
	oldArray := t.array
	oldASize := uint32(len(oldArray))
	oldNodes := t.nodes
	oldLastFree := t.lastfree
	oldLsizenode := t.lsizenode

	/* create the new hash part */
	newt := &Table{}
	setnodevector(L, newt, nhsize)

	if newasize < oldASize { /* will array shrink? */
		na := make([]Value, newasize)
		copy(na, oldArray[:newasize])
		t.array = na
		t.alimit = newasize
		/* exchange hash parts: the new (empty) one goes to 't' */
		t.nodes, newt.nodes = newt.nodes, oldNodes
		t.lsizenode, newt.lsizenode = newt.lsizenode, oldLsizenode
		t.lastfree, newt.lastfree = newt.lastfree, oldLastFree
		/* re-insert the elements of the vanishing slice */
		for i := newasize; i < oldASize; i++ {
			if !isempty(&oldArray[i]) {
				luaH_setint(L, t, int64(i)+1, &oldArray[i])
			}
		}
		setrealasize(t)
	} else { /* will array grow? */
		na := make([]Value, newasize)
		copy(na, oldArray)
		t.array = na
		t.alimit = newasize
		setrealasize(t)
		/* exchange hash parts */
		t.nodes, newt.nodes = newt.nodes, oldNodes
		t.lsizenode, newt.lsizenode = newt.lsizenode, oldLsizenode
		t.lastfree, newt.lastfree = newt.lastfree, oldLastFree
	}
	/* re-insert the elements of the old hash part */
	reinsert(L, newt, t)
}

func reinsert(L *State, ot, t *Table) {
	for i := range ot.nodes {
		if !isempty(&ot.nodes[i].val) {
			var k Value
			getnodekey(&k, &ot.nodes[i])
			v := ot.nodes[i].val
			luaH_set(L, t, &k, &v)
		}
	}
}

// luaH_resizearray resizes only the array part (ltable.c).
func luaH_resizearray(L *State, t *Table, nasize uint32) {
	luaH_resize(L, t, nasize, uint32(sizenode(t)))
}

// computesizes computes the best size for the array part (ltable.c).
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

func countint(key int64, nums *[MAXABITS + 1]uint32) int {
	k := arrayindex(key)
	if k != 0 { /* is 'key' an appropriate array index? */
		nums[luaO_ceillog2(k)]++
		return 1
	}
	return 0
}

func numusearray(t *Table, nums *[MAXABITS + 1]uint32) uint32 {
	var ause uint32
	i := uint32(1)
	asize := t.alimit
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
			if !isempty(&t.array[i-1]) {
				lc++
			}
		}
		nums[lg] += lc
		ause += lc
	}
	return ause
}

func numusehash(t *Table, nums *[MAXABITS + 1]uint32, pna *uint32) int {
	totaluse := 0
	ause := 0
	for i := len(t.nodes) - 1; i >= 0; i-- {
		n := &t.nodes[i]
		if !isempty(&n.val) {
			if n.keytt == LUA_VNUMINT {
				ause += countint(n.ki, nums)
			}
			totaluse++
		}
	}
	*pna += uint32(ause)
	return totaluse
}

// rehash recomputes the sizes of both parts (ltable.c).
func rehash(L *State, t *Table, ek *Value) {
	var nums [MAXABITS + 1]uint32
	var na uint32
	setlimittosize(t)
	na = numusearray(t, &nums)            /* count keys in array part */
	totaluse := int(na)                   /* all those keys are integer keys */
	totaluse += numusehash(t, &nums, &na) /* count keys in hash part */
	if ttisinteger(ek) {                  /* count extra key */
		na += uint32(countint(ivalue(ek), &nums))
	}
	totaluse++
	asize := computesizes(&nums, &na) /* compute new size for array part */
	luaH_resize(L, t, asize, uint32(totaluse)-na)
}

// luaH_newkey inserts a new key into a table (ltable.c).
func luaH_newkey(L *State, t *Table, key, value *Value) {
	if ttisnil(key) {
		luaG_runerror(L, "table index is nil")
		return
	}
	if ttisfloat(key) {
		f := fltvalue(key)
		if k, ok := floatToIntEq(f); ok {
			var aux Value
			setivalue(&aux, k)
			key = &aux
		} else if math.IsNaN(f) {
			luaG_runerror(L, "table index is NaN")
			return
		}
	}
	if ttisnil(value) {
		return /* do not insert nil values */
	}
	mp := mainpositionTV(L, t, key)
	if isdummy(t) || !isempty(&t.nodes[mp].val) { /* main position is taken? */
		f := getfreepos(t) /* get a free place */
		if f < 0 {         /* cannot find a free place? */
			rehash(L, t, key) /* grow table */
			luaH_set(L, t, key, value)
			return
		}
		var mpk Value
		getnodekey(&mpk, &t.nodes[mp])
		othern := mainpositionTV(L, t, &mpk)
		if othern != mp { /* is colliding node out of its main position? */
			/* yes; move colliding node into free position */
			for othern+int(t.nodes[othern].next) != mp {
				othern += int(t.nodes[othern].next) /* find previous */
			}
			t.nodes[othern].next = int32(f - othern) /* rechain to point to 'f' */
			t.nodes[f] = t.nodes[mp]                 /* copy colliding node into free pos. */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next += int32(mp - f) /* correct 'next' */
				t.nodes[mp].next = 0             /* now 'mp' is free */
			}
			setempty(&t.nodes[mp].val)
		} else { /* colliding node is in its own main position */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next = int32((mp + int(t.nodes[mp].next)) - f) /* chain new position */
			}
			t.nodes[mp].next = int32(f - mp)
			mp = f
		}
	}
	setnodekey(&t.nodes[mp], key)
	t.nodes[mp].val = *value
}

// luaH_getint searches an integer key (ltable.c).
func luaH_getint(t *Table, key int64) *Value {
	alimit := t.alimit
	if uint64(key)-1 < uint64(alimit) { /* 'key' in [1, alimit]? */
		return &t.array[key-1]
	}
	if !isrealasize(t) &&
		(((uint64(key) - 1) &^ uint64(alimit-1)) < uint64(alimit)) {
		t.alimit = uint32(key) /* probably '#t' is here now */
		return &t.array[key-1]
	}
	/* key is not in the array part; check the hash */
	n := hashint(t, key)
	for {
		if t.nodes[n].keytt == LUA_VNUMINT && t.nodes[n].ki == key {
			return &t.nodes[n].val
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &absentKey
		}
		n += int(nx)
	}
}

// luaH_getshortstr searches a short-string key (ltable.c).
func luaH_getshortstr(t *Table, key *TString) *Value {
	if sizenode(t) == 0 {
		return &absentKey
	}
	n := hashstr(t, key)
	for {
		if t.nodes[n].keytt == ctb(LUA_VSHRSTR) && luaS_eqshrstr((*TString)(t.nodes[n].kp), key) {
			return &t.nodes[n].val
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &absentKey
		}
		n += int(nx)
	}
}

// luaH_getstrkey answers the string a string key is *stored under*, or
// nil when the key is absent (llex.c: keystrval(nodefromval(o))).
//
// It is not the same as the value the key maps to, and llex.c needs the
// difference: the scanner's table anchors every string of a chunk by
// mapping it to itself, but 'addk' overwrites that value with the index
// of the constant as soon as the string is used as one (lcode.c). An
// entry whose value has become an integer must still yield the string,
// or every later occurrence of a literal that has already been used as a
// a constant comes back as a fresh object — which is exactly what
// PUC-Lua's own 'literals.lua' checks: two equal long literals in one
// chunk have to be the same string.
func luaH_getstrkey(L *State, t *Table, key *TString) *TString {
	if sizenode(t) == 0 {
		return nil
	}
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
	if i := getGenericNode(L, t, &ko, false); i >= 0 {
		return (*TString)(t.nodes[i].kp)
	}
	return nil
}

// luaH_getstr searches a string key (ltable.c).
func luaH_getstr(L *State, t *Table, key *TString) *Value {
	if key.tt == LUA_VSHRSTR {
		return luaH_getshortstr(t, key)
	}
	var ko Value
	setsvalue(&ko, key)
	return getgeneric(L, t, &ko, false)
}

// luaH_get is the main search function (ltable.c).
func luaH_get(L *State, t *Table, key *Value) *Value {
	switch ttypetag(key) {
	case LUA_VSHRSTR:
		return luaH_getshortstr(t, tsvalue(key))
	case LUA_VNUMINT:
		return luaH_getint(t, ivalue(key))
	case LUA_VNIL:
		return &absentKey
	case LUA_VNUMFLT:
		if k, ok := floatToIntEq(fltvalue(key)); ok {
			return luaH_getint(t, k) /* use specialized version */
		}
	}
	return getgeneric(L, t, key, false)
}

// luaH_finishset completes a raw set (ltable.c).
func luaH_finishset(L *State, t *Table, key, slot, value *Value) {
	if isabstkey(slot) {
		luaH_newkey(L, t, key, value)
	} else {
		*slot = *value
	}
}

// luaH_set does a raw set (ltable.c).
func luaH_set(L *State, t *Table, key, value *Value) {
	slot := luaH_get(L, t, key)
	luaH_finishset(L, t, key, slot, value)
}

// luaH_setint does a raw set with an integer key (ltable.c).
func luaH_setint(L *State, t *Table, key int64, value *Value) {
	p := luaH_getint(t, key)
	if isabstkey(p) {
		var k Value
		setivalue(&k, key)
		luaH_newkey(L, t, &k, value)
	} else {
		*p = *value
	}
}

// hash_search finds a boundary in the hash part (ltable.c).
func hashSearch(t *Table, j uint64) uint64 {
	var i uint64
	if j == 0 {
		j++ /* the caller ensures 'j + 1' is present */
	}
	for {
		i = j /* 'i' is a present index */
		if j <= uint64(LUA_MAXINTEGER)/2 {
			j *= 2
		} else {
			j = uint64(LUA_MAXINTEGER)
			if isempty(luaH_getint(t, int64(j))) {
				break /* 'j' now is an absent index */
			}
			return j /* max integer is a boundary */
		}
		if isempty(luaH_getint(t, int64(j))) {
			break
		}
	}
	/* i < j  &&  t[i] present  &&  t[j] absent */
	for j-i > 1 { /* binary search between them */
		m := (i + j) / 2
		if isempty(luaH_getint(t, int64(m))) {
			j = m
		} else {
			i = m
		}
	}
	return i
}

func binsearch(array []Value, i, j uint32) uint32 {
	for j-i > 1 { /* binary search */
		m := (i + j) / 2
		if isempty(&array[m-1]) {
			j = m
		} else {
			i = m
		}
	}
	return i
}

// luaH_getn returns a boundary of the table (ltable.c).
func luaH_getn(t *Table) uint64 {
	limit := t.alimit
	if limit > 0 && isempty(&t.array[limit-1]) { /* (1)? */
		/* there must be a boundary before 'limit' */
		if limit >= 2 && !isempty(&t.array[limit-2]) {
			/* 'limit - 1' is a boundary; can it be a new limit? */
			if ispow2realasize(t) && !ispow2u(limit-1) {
				t.alimit = limit - 1
				setnorealasize(t)
			}
			return uint64(limit - 1)
		}
		/* must search for a boundary in [0, limit] */
		boundary := binsearch(t.array, 0, limit)
		if ispow2realasize(t) && boundary > luaH_realasize(t)/2 {
			t.alimit = boundary
			setnorealasize(t)
		}
		return uint64(boundary)
	}
	/* 'limit' is zero or present in table */
	if !limitequalsasize(t) { /* (2)? */
		if isempty(&t.array[limit]) { /* 'limit + 1' is empty? */
			return uint64(limit) /* this is the boundary */
		}
		limit = luaH_realasize(t)
		if isempty(&t.array[limit-1]) {
			boundary := binsearch(t.array, t.alimit, limit)
			t.alimit = boundary
			return uint64(boundary)
		}
		/* else, new limit is present; check the hash part */
	}
	/* (3) 'limit' is the last element and is zero or present */
	if len(t.nodes) == 0 || isempty(luaH_getint(t, int64(limit)+1)) {
		return uint64(limit)
	}
	return hashSearch(t, uint64(limit))
}

func ispow2realasize(t *Table) bool { return !isrealasize(t) || ispow2u(t.alimit) }

// floatToIntEq converts a float with an exact integer value to that
// integer (luaV_flttointeger with mode F2Ieq).
//
// The upper bound has to be exclusive. float64(math.MaxInt64) rounds up
// to 2^63, so 'n <= math.MaxInt64' accepts 9223372036854775808.0 and the
// conversion to int64 wraps it onto math.mininteger. A table then stores
// that float key on top of the minimum integer. lua_numbertointeger
// rejects it: the value must be strictly less than 2^63.
func floatToIntEq(n float64) (int64, bool) {
	var k int64
	if floatToInt(n, &k) {
		return k, true
	}
	return 0, false
}

// luaH_setmetatable sets (or clears) the metatable of a table.
func luaH_setmetatable(t *Table, mt *Table) { t.metatable = mt }
