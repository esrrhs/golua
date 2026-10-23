// Translation of ltable.h / ltable.c (PUC-Lua 5.4.9).
//
// The array part and the open-addressing hash part are kept, because
// 'next' must be able to resume a traversal from a given key; a Go map
// cannot do that (its iteration order is randomized).
//
// 5.4.9 surface: alimit + BITRAS (isrealasize / setrealasize /
// setnorealasize), equalkey+deadok, luaH_finishset, hashint=hashmod,
// Xmilia trick in luaH_getint, boundary search in luaH_getn.
package core

import (
	"math"
	"unsafe"
)

/*
** MAXABITS is the largest integer such that MAXASIZE fits in an
** unsigned int. (sizeof(int)*CHAR_BIT-1 on LP64 → 31.)
 */
const (
	MAXABITS = 31
	MAXASIZE = 1 << MAXABITS
	MAXHBITS = MAXABITS - 1
	MAXHSIZE = 1 << MAXHBITS
)

// absentKey is the value returned for a key not found in a table
// (ltable.c's 'absentkey' / ABSTKEYCONSTANT).
var absentKey = Value{tt: LUA_VABSTKEY}

func hashpow2(t *Table, n uint32) int { return int(lmod(n, sizenode(t))) }

// hashmod hashes by remainder against an odd modulus (ltable.c).
func hashmod(t *Table, n uint32) int {
	return int(n % uint32((sizenode(t)-1)|1))
}

func hashstr(t *Table, ts *TString) int  { return hashpow2(t, ts.hash) }
func hashboolean(t *Table, p uint32) int { return hashpow2(t, p) }
func hashpointer(t *Table, p unsafe.Pointer) int {
	return hashmod(t, uint32(uintptr(p)))
}

/*
** Hash for integers. To allow a good hash, use the remainder operator
** ('%'). If integer fits as a non-negative int, compute an int
** remainder, which is faster. Otherwise, use an unsigned-integer
** remainder, which uses all bits and ensures a non-negative result.
** (ltable.c hashint — 5.4.9 uses hashmod, not 5.3's hashpow2.)
 */
func hashint(t *Table, i int64) int {
	ui := uint64(i)
	if ui <= uint64(math.MaxInt32) {
		return hashmod(t, uint32(ui)) /* C: hashmod(t, cast_int(ui)) */
	}
	/* C: hashmod(t, ui) with full lua_Unsigned width */
	return int(ui % uint64((sizenode(t)-1)|1))
}

/*
** Hash for floating-point numbers.
** The main computation should be just
**     n = frexp(n, &i); return (n * INT_MAX) + i
** but there are numerical subtleties (ltable.c l_hashfloat).
 */
func l_hashfloat(n float64) int {
	frac, exp := math.Frexp(n)
	/* frexp * -INT_MIN (C: -cast_num(INT_MIN)); INT_MIN exact as float. */
	v := frac * -float64(math.MinInt32)
	var ni int64
	/* C uses lua_numbertointeger (truncates); NOT luaV_flttointeger/F2Ieq. */
	if !lua_numbertointeger(v, &ni) { /* is 'n' inf/-inf/NaN? */
		return 0
	}
	u := uint32(int32(exp)) + uint32(ni)
	if u <= uint32(math.MaxInt32) {
		return int(u)
	}
	return int(^u)
}

/*
** returns the 'main' position of an element in a table (that is,
** the index of its hash value). (ltable.c mainpositionTV)
 */
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
	case LUA_VLCF:
		// C: hashpointer(t, fvalue(key)); bits live in v.p
		return hashpointer(t, pvalue(key))
	default:
		return hashpointer(t, gcvalue(key))
	}
}

func mainpositionfromnode(L *State, t *Table, nd *node) int {
	var key Value
	getnodekey(&key, nd)
	return mainpositionTV(L, t, &key)
}

// keyisdead is 'keyisdead(n)' of lobject.h / ltable.c: true when the
// node's key has been turned into a dead key (lgc clearkey / removeentry).
//
// It must look at the key's type and nothing else. A node's 'next' is a
// chain offset; a negative one is ordinary (free slot below main position).
func keyisdead(n *node) bool { return n.keytt == LUA_TDEADKEY }

/*
** Check whether key 'k1' is equal to the key in node 'n2'. This
** equality is raw, so there are no metamethods. Floats with integer
** values have been normalized, so integers cannot be equal to floats.
** Short strings: C uses pointer identity in the default case (eqshrstr);
** Go compares via luaS_eqshrstr (content fallback for the map intern).
** A true 'deadok' means to accept dead keys as equal to their original
** values. (ltable.c equalkey)
 */
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
	case LUA_VLCF:
		return pvalue(k1) == n.kp
	case ctb(LUA_VLNGSTR):
		return luaS_eqlngstr(tsvalue(k1), (*TString)(n.kp))
	case ctb(LUA_VSHRSTR):
		// C: default (pointer); Go: content-safe eq for map intern (FLAG).
		return luaS_eqshrstr(tsvalue(k1), (*TString)(n.kp))
	default:
		return gcvalue(k1) == n.kp
	}
}

/*
** True if value of 'alimit' is equal to the real size of the array
** part of table 't'. (Otherwise, the array part must be larger than
** 'alimit'.)
 */
func limitequalsasize(t *Table) bool { return isrealasize(t) || ispow2u(t.alimit) }

// ispow2u is C llimits.h ispow2: ((x)&((x)-1))==0. True for 0 (unsigned).
func ispow2u(x uint32) bool { return (x & (x - 1)) == 0 }

/*
** Returns the real size of the 'array' array (ltable.c luaH_realasize).
 */
func luaH_realasize(t *Table) uint32 {
	if limitequalsasize(t) {
		return t.alimit /* this is the size */
	}
	size := t.alimit
	/* compute the smallest power of 2 not smaller than 'size' */
	size |= (size >> 1)
	size |= (size >> 2)
	size |= (size >> 4)
	size |= (size >> 8)
	size |= (size >> 16)
	size++
	return size
}

/*
** Check whether real size of the array is a power of 2.
** (If it is not, 'alimit' cannot be changed to any other value
** without changing the real size.)
 */
func ispow2realasize(t *Table) bool {
	return !isrealasize(t) || ispow2u(t.alimit)
}

func setlimittosize(t *Table) uint32 {
	t.alimit = luaH_realasize(t)
	setrealasize(t)
	return t.alimit
}

// limitasasize is C's limitasasize(t): alimit when isrealasize.
func limitasasize(t *Table) uint32 { return t.alimit }

/*
** "Generic" get version. (Not that generic: not valid for integers,
** which may be in array part, nor for floats with integral values.)
** See explanation about 'deadok' in equalkey. (ltable.c getgeneric)
 */
func getgeneric(L *State, t *Table, key *Value, deadok bool) *Value {
	if i := getGenericNode(L, t, key, deadok); i >= 0 {
		return &t.nodes[i].val
	}
	return &absentKey
}

// getGenericNode looks a key up in the hash part, returning its node
// index (-1 if absent). Go helper also used by luaH_getstrkey.
func getGenericNode(L *State, t *Table, key *Value, deadok bool) int {
	i := mainpositionTV(L, t, key)
	for {
		if equalkey(key, &t.nodes[i], deadok) {
			return i /* that's it */
		}
		nx := t.nodes[i].next
		if nx == 0 {
			return -1 /* not found */
		}
		i += int(nx)
	}
}

/*
** returns the index for 'k' if 'k' is an appropriate key to live in
** the array part of a table, 0 otherwise.
 */
func arrayindex(k int64) uint32 {
	if uint64(k)-1 < MAXASIZE { /* 'k' in [1, MAXASIZE]? */
		return uint32(k) /* 'key' is an appropriate array index */
	}
	return 0
}

/*
** returns the index of a 'key' for table traversals. First goes all
** elements in the array part, then elements in the hash part. The
** beginning of a traversal is signaled by 0. (ltable.c findindex)
 */
func findIndex(L *State, t *Table, key *Value, asize uint32) uint32 {
	if ttisnil(key) {
		return 0 /* first iteration */
	}
	var i uint32
	if ttisinteger(key) {
		i = arrayindex(ivalue(key))
	}
	if i != 0 && i-1 < asize { /* is 'key' inside array part? */
		return i /* yes; that's the index */
	}
	n := getGenericNode(L, t, key, true)
	if n < 0 {
		luaG_runerror(L, "invalid key to 'next'") /* key not found */
	}
	/* hash elements are numbered after array ones */
	return uint32(n+1) + asize
}

// luaH_next traverses a table (ltable.c). The key is at 'keyIdx' and the
// value is written at 'keyIdx + 1' (index-stack adaptation of StkId).
func luaH_next(L *State, t *Table, keyIdx int) bool {
	asize := luaH_realasize(t)
	i := findIndex(L, t, &L.stack[keyIdx], asize) /* find original key */
	for ; i < asize; i++ {                        /* try first array part */
		if !isempty(&t.array[i]) { /* a non-empty entry? */
			setivalue(&L.stack[keyIdx], int64(i)+1)
			L.stack[keyIdx+1] = t.array[i]
			return true
		}
	}
	for j := int(i) - int(asize); j < sizenode(t); j++ { /* hash part */
		if !isempty(&t.nodes[j].val) { /* a non-empty entry? */
			getnodekey(&L.stack[keyIdx], &t.nodes[j])
			L.stack[keyIdx+1] = t.nodes[j].val
			return true
		}
	}
	return false /* no more elements */
}

// dummyNode is the counterpart of PUC-Lua's static 'dummynode': a one-slot
// hash part shared by every table that has no hash entries, so that the
// search loops can always touch at least one node.
var dummyNode = []node{{keytt: LUA_VNIL, val: Value{tt: LUA_VEMPTY}, next: 0}}

// isdummy reports whether the table is using the shared dummy node.
// C signals with 'lastfree == NULL'; Go uses lastfree < 0.
func isdummy(t *Table) bool { return t.lastfree < 0 }

func setdummy(t *Table) {
	t.nodes = dummyNode
	t.lsizenode = 0
	t.lastfree = -1 /* signal that it is using dummy node */
}

// allocsizenode is C's allocsizenode(t): 0 when dummy, else sizenode.
func allocsizenode(t *Table) int {
	if isdummy(t) {
		return 0
	}
	return sizenode(t)
}

/*
** Creates an array for the hash part of a table with the given
** size, or reuses the dummy node if size is zero. (ltable.c setnodevector)
 */
func setnodevector(L *State, t *Table, size uint32) {
	if size == 0 { /* no elements to hash part? */
		setdummy(t) /* use common 'dummynode' */
	} else {
		lsize := luaO_ceillog2(size)
		if lsize > MAXHBITS || (uint32(1)<<lsize) > MAXHSIZE {
			luaG_runerror(L, "table overflow")
		}
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

/*
** (Re)insert all elements from the hash part of 'ot' into table 't'.
 */
func reinsert(L *State, ot, t *Table) {
	size := sizenode(ot)
	for j := 0; j < size; j++ {
		old := &ot.nodes[j]
		if !isempty(&old.val) {
			/* doesn't need barrier/invalidate cache, as entry was
			   already present in the table */
			var k Value
			getnodekey(&k, old)
			v := old.val
			luaH_set(L, t, &k, &v)
		}
	}
}

/*
** Exchange the hash part of 't1' and 't2'. (ltable.c exchangehashpart)
 */
func exchangehashpart(t1, t2 *Table) {
	lsizenode := t1.lsizenode
	nodes := t1.nodes
	lastfree := t1.lastfree
	t1.lsizenode = t2.lsizenode
	t1.nodes = t2.nodes
	t1.lastfree = t2.lastfree
	t2.lsizenode = lsizenode
	t2.nodes = nodes
	t2.lastfree = lastfree
}

/*
** Resize table 't' for the new given sizes. (ltable.c luaH_resize)
** Go GC: no alloc-failure unwind; make never returns nil for size>0.
** Structure mirrors C: setlimittosize → new hash → shrink vanishing
** slice via exchange → grow/shrink array → permanent exchange →
** clear new slice → reinsert old hash.
 */
func luaH_resize(L *State, t *Table, newasize uint32, nhsize uint32) {
	oldasize := setlimittosize(t)
	newt := &Table{} /* to keep the new hash part */
	/* create new hash part with appropriate size into 'newt' */
	setnodevector(L, newt, nhsize)
	if newasize < oldasize { /* will array shrink? */
		t.alimit = newasize       /* pretend array has new size... */
		exchangehashpart(t, newt) /* and new hash */
		/* re-insert into the new hash the elements from vanishing slice */
		for i := newasize; i < oldasize; i++ {
			if !isempty(&t.array[i]) {
				luaH_setint(L, t, int64(i)+1, &t.array[i])
			}
		}
		t.alimit = oldasize       /* restore current size... */
		exchangehashpart(t, newt) /* and hash (in case of errors) */
	}
	/* allocate new array (Go: make+copy; C: luaM_reallocvector) */
	newarray := make([]Value, newasize)
	if oldasize < newasize {
		copy(newarray, t.array)
	} else {
		copy(newarray, t.array[:newasize])
	}
	/* allocation ok; initialize new part of the array */
	exchangehashpart(t, newt) /* 't' has the new hash ('newt' has the old) */
	t.array = newarray        /* set new array part */
	t.alimit = newasize
	/* setrealasize already done by setlimittosize at entry (C same) */
	for i := oldasize; i < newasize; i++ { /* clear new slice of the array */
		setempty(&t.array[i])
	}
	/* re-insert elements from old hash part into new parts */
	reinsert(L, newt, t) /* 'newt' now has the old hash */
	/* freehash omitted (Go GC) */
}

// luaH_resizearray resizes only the array part (ltable.c).
func luaH_resizearray(L *State, t *Table, nasize uint32) {
	nsize := allocsizenode(t)
	luaH_resize(L, t, nasize, uint32(nsize))
}

/*
** Compute the optimal size for the array part of table 't'. 'nums' is a
** "count array" where 'nums[i]' is the number of integers in the table
** between 2^(i - 1) + 1 and 2^i. (ltable.c computesizes)
** twotoi > 0 stops uint32 overflow (C relies on unsigned wrap of *2).
 */
func computesizes(nums *[MAXABITS + 1]uint32, pna *uint32) uint32 {
	var a, na, optimal uint32
	/* loop while keys can fill more than half of total size */
	for i, twotoi := 0, uint32(1); twotoi > 0 && *pna > twotoi/2; i, twotoi = i+1, twotoi*2 {
		a += nums[i]
		if a > twotoi/2 { /* more than half elements present? */
			optimal = twotoi /* optimal size (till now) */
			na = a           /* all elements up to 'optimal' will go to array part */
		}
	}
	*pna = na
	return optimal
}

func countint(key int64, nums *[MAXABITS + 1]uint32) int {
	k := arrayindex(key)
	if k != 0 { /* is 'key' an appropriate array index? */
		nums[luaO_ceillog2(k)]++ /* count as such */
		return 1
	}
	return 0
}

/*
** Count keys in array part of table 't': Fill 'nums[i]' with
** number of keys that will go into corresponding slice and return
** total number of non-nil keys. (ltable.c numusearray)
 */
func numusearray(t *Table, nums *[MAXABITS + 1]uint32) uint32 {
	var ause uint32          /* summation of 'nums' */
	i := uint32(1)           /* count to traverse all array keys */
	asize := limitasasize(t) /* real array size (after setlimittosize) */
	/* traverse each slice */
	for lg, ttlg := 0, uint32(1); lg <= MAXABITS; lg, ttlg = lg+1, ttlg*2 {
		var lc uint32 /* counter */
		lim := ttlg
		if lim > asize {
			lim = asize /* adjust upper limit */
			if i > lim {
				break /* no more elements to count */
			}
		}
		/* count elements in range (2^(lg - 1), 2^lg] */
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
	totaluse := 0 /* total number of elements */
	ause := 0     /* elements added to 'nums' (can go to array part) */
	i := sizenode(t)
	for i > 0 {
		i--
		n := &t.nodes[i]
		if !isempty(&n.val) {
			if keyisinteger(n) {
				ause += countint(keyival(n), nums)
			}
			totaluse++
		}
	}
	*pna += uint32(ause)
	return totaluse
}

/*
** nums[i] = number of keys 'k' where 2^(i - 1) < k <= 2^i
 */
func rehash(L *State, t *Table, ek *Value) {
	var nums [MAXABITS + 1]uint32
	var na uint32 /* number of keys in the array part */
	setlimittosize(t)
	na = numusearray(t, &nums)            /* count keys in array part */
	totaluse := int(na)                   /* all those keys are integer keys */
	totaluse += numusehash(t, &nums, &na) /* count keys in hash part */
	/* count extra key */
	if ttisinteger(ek) {
		na += uint32(countint(ivalue(ek), &nums))
	}
	totaluse++
	/* compute new size for array part */
	asize := computesizes(&nums, &na)
	/* resize the table to new computed sizes */
	luaH_resize(L, t, asize, uint32(totaluse)-na)
}

// maskflags is the set of flag bits used for the metamethod cache (ltm.h).
// Bit 7 is reserved for 'isrealasize' (BITRAS), so a new table gets the
// metamethod bits set ("no metamethods cached as absent yet") but bit 7 clear.
// C: maskflags = ~(~0u << (TM_EQ + 1)) → 0x3F when TM_EQ == 5.
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
// alive. FLAG: Go-only specialization (not in C ltable).
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

// invalidateTMcache invalidates the metamethod cache of a table
// (ltable.h: flags &= ~maskflags — preserves BITRAS).
func invalidateTMcache(t *Table) { t.flags &= ^maskflags }

// luaH_free is a no-op: the Go GC reclaims the table (ltable.c IGNORE).
// The blank assignment is an executable statement so coverage can see a call.
func luaH_free(L *State, t *Table) { _, _ = L, t }

// getfreepos returns a free position in the hash part (ltable.c).
// Returns -1 when none (C returns NULL).
func getfreepos(t *Table) int {
	if !isdummy(t) {
		for t.lastfree > 0 {
			t.lastfree--
			if keyisnil(&t.nodes[t.lastfree]) {
				return t.lastfree
			}
		}
	}
	return -1 /* could not find a free place */
}

/*
** inserts a new key into a hash table; first, check whether key's main
** position is free. If not, check whether colliding node is in its main
** position or not: if it is not, move colliding node to an empty place and
** put new key in its main position; otherwise (colliding node is in its main
** position), new key goes to an empty position. (ltable.c luaH_newkey)
**
** 5.4.9: do not insert nil values (early return). Barriers omitted (Go GC).
 */
func luaH_newkey(L *State, t *Table, key, value *Value) {
	if ttisnil(key) {
		luaG_runerror(L, "table index is nil")
		return
	}
	if ttisfloat(key) {
		f := fltvalue(key)
		if k, ok := floatToIntEq(f); ok { /* does key fit in an integer? */
			var aux Value
			setivalue(&aux, k)
			key = &aux /* insert it as an integer */
		} else if math.IsNaN(f) {
			luaG_runerror(L, "table index is NaN")
			return
		}
	}
	if ttisnil(value) {
		return /* do not insert nil values */
	}
	mp := mainpositionTV(L, t, key)
	if !isempty(&t.nodes[mp].val) || isdummy(t) { /* main position is taken? */
		f := getfreepos(t) /* get a free place */
		if f < 0 {         /* cannot find a free place? */
			rehash(L, t, key) /* grow table */
			/* whatever called 'newkey' takes care of TM cache */
			luaH_set(L, t, key, value) /* insert key into grown table */
			return
		}
		othern := mainpositionfromnode(L, t, &t.nodes[mp])
		if othern != mp { /* is colliding node out of its main position? */
			/* yes; move colliding node into free position */
			for othern+int(t.nodes[othern].next) != mp { /* find previous */
				othern += int(t.nodes[othern].next)
			}
			t.nodes[othern].next = int32(f - othern) /* rechain to point to 'f' */
			t.nodes[f] = t.nodes[mp]                 /* copy colliding node into free pos. (mp->next also goes) */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next += int32(mp - f) /* correct 'next' */
				t.nodes[mp].next = 0             /* now 'mp' is free */
			}
			setempty(&t.nodes[mp].val)
		} else { /* colliding node is in its own main position */
			/* new node will go into free position */
			if t.nodes[mp].next != 0 {
				t.nodes[f].next = int32((mp + int(t.nodes[mp].next)) - f) /* chain new position */
			}
			t.nodes[mp].next = int32(f - mp)
			mp = f
		}
	}
	setnodekey(&t.nodes[mp], key)
	/* luaC_barrierback omitted (Go GC) */
	t.nodes[mp].val = *value
}

/*
** Search function for integers. If integer is inside 'alimit', get it
** directly from the array part. Otherwise, if 'alimit' is not
** the real size of the array, the key still can be in the array part.
** In this case, do the "Xmilia trick" to check whether 'key-1' is
** smaller than the real size. (ltable.c luaH_getint)
 */
func luaH_getint(t *Table, key int64) *Value {
	alimit := t.alimit
	if uint64(key)-1 < uint64(alimit) { /* 'key' in [1, t->alimit]? */
		return &t.array[key-1]
	} else if !isrealasize(t) && /* key still may be in the array part? */
		(((uint64(key) - 1) &^ uint64(alimit-1)) < uint64(alimit)) {
		t.alimit = uint32(key) /* probably '#t' is here now */
		return &t.array[key-1]
	}
	/* key is not in the array part; check the hash */
	n := hashint(t, key)
	for { /* check whether 'key' is somewhere in the chain */
		if keyisinteger(&t.nodes[n]) && keyival(&t.nodes[n]) == key {
			return &t.nodes[n].val /* that's it */
		}
		nx := t.nodes[n].next
		if nx == 0 {
			break
		}
		n += int(nx)
	}
	return &absentKey
}

/*
** search function for short strings (ltable.c luaH_getshortstr)
 */
func luaH_getshortstr(t *Table, key *TString) *Value {
	n := hashstr(t, key)
	for { /* check whether 'key' is somewhere in the chain */
		if keyisshrstr(&t.nodes[n]) &&
			luaS_eqshrstr(keystrval(&t.nodes[n]), key) { /* C: eqshrstr; Go content FLAG */
			return &t.nodes[n].val /* that's it */
		}
		nx := t.nodes[n].next
		if nx == 0 {
			return &absentKey /* not found */
		}
		n += int(nx)
	}
}

// luaH_getstrkey answers the string a string key is *stored under*, or
// nil when the key is absent (llex.c: keystrval(nodefromval(o))).
//
// FLAG: Go-only. It is not the same as the value the key maps to, and
// llex.c needs the difference: the scanner's table anchors every string
// of a chunk by mapping it to itself, but 'addk' overwrites that value
// with the index of the constant as soon as the string is used as one
// (lcode.c). C obtains the stored key via keyfromval / node key pointer;
// Go's flattened node keys are not addressable as TValue*, so this helper
// returns the interned *TString identity.
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
	/* for long strings, use generic case */
	var ko Value
	setsvalue(&ko, key)
	return getgeneric(L, t, &ko, false)
}

/*
** main search function (ltable.c luaH_get)
 */
func luaH_get(L *State, t *Table, key *Value) *Value {
	switch ttypetag(key) {
	case LUA_VSHRSTR:
		return luaH_getshortstr(t, tsvalue(key))
	case LUA_VNUMINT:
		return luaH_getint(t, ivalue(key))
	case LUA_VNIL:
		return &absentKey
	case LUA_VNUMFLT:
		if k, ok := floatToIntEq(fltvalue(key)); ok { /* integral index? */
			return luaH_getint(t, k) /* use specialized version */
		}
		/* else... FALLTHROUGH */
	}
	return getgeneric(L, t, key, false)
}

/*
** Finish a raw "set table" operation, where 'slot' is where the value
** should have been (the result of a previous "get table").
** Beware: when using this function you probably need to check a GC
** barrier and invalidate the TM cache. (ltable.c luaH_finishset — 5.4.9)
 */
func luaH_finishset(L *State, t *Table, key, slot, value *Value) {
	if isabstkey(slot) {
		luaH_newkey(L, t, key, value)
	} else {
		*slot = *value
	}
}

/*
** beware: when using this function you probably need to check a GC
** barrier and invalidate the TM cache. (ltable.c luaH_set)
 */
func luaH_set(L *State, t *Table, key, value *Value) {
	slot := luaH_get(L, t, key)
	luaH_finishset(L, t, key, slot, value)
}

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

/*
** Try to find a boundary in the hash part of table 't'. (ltable.c hash_search)
 */
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
			if isempty(luaH_getint(t, int64(j))) { /* t[j] not present? */
				break /* 'j' now is an absent index */
			}
			/* weird case */
			return j /* well, max integer is a boundary... */
		}
		if isempty(luaH_getint(t, int64(j))) { /* repeat until an absent t[j] */
			break
		}
	}
	/* i < j  &&  t[i] present  &&  t[j] absent */
	for j-i > 1 { /* do a binary search between them */
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

/*
** Try to find a boundary in table 't'. (A 'boundary' is an integer index
** such that t[i] is present and t[i+1] is absent, or 0 if t[1] is absent
** and 'maxinteger' if t[maxinteger] is present.)
** Uses alimit as a hint and may shrink/grow it via setnorealasize.
** (ltable.c luaH_getn — 5.4.9)
 */
func luaH_getn(t *Table) uint64 {
	limit := t.alimit
	if limit > 0 && isempty(&t.array[limit-1]) { /* (1)? */
		/* there must be a boundary before 'limit' */
		if limit >= 2 && !isempty(&t.array[limit-2]) {
			/* 'limit - 1' is a boundary; can it be a new limit? */
			if ispow2realasize(t) && !ispow2u(limit-1) {
				t.alimit = limit - 1
				setnorealasize(t) /* now 'alimit' is not the real size */
			}
			return uint64(limit - 1)
		}
		/* must search for a boundary in [0, limit] */
		boundary := binsearch(t.array, 0, limit)
		/* can this boundary represent the real size of the array? */
		if ispow2realasize(t) && boundary > luaH_realasize(t)/2 {
			t.alimit = boundary /* use it as the new limit */
			setnorealasize(t)
		}
		return uint64(boundary)
	}
	/* 'limit' is zero or present in table */
	if !limitequalsasize(t) { /* (2)? */
		/* 'limit' > 0 and array has more elements after 'limit' */
		if isempty(&t.array[limit]) { /* 'limit + 1' is empty? */
			return uint64(limit) /* this is the boundary */
		}
		/* else, try last element in the array */
		limit = luaH_realasize(t)
		if isempty(&t.array[limit-1]) { /* empty? */
			/* there must be a boundary in the array after old limit,
			   and it must be a valid new limit */
			boundary := binsearch(t.array, t.alimit, limit)
			t.alimit = boundary
			return uint64(boundary)
		}
		/* else, new limit is present in the table; check the hash part */
	}
	/* (3) 'limit' is the last element and either is zero or present in table */
	if isdummy(t) || isempty(luaH_getint(t, int64(limit)+1)) {
		return uint64(limit) /* 'limit + 1' is absent */
	}
	/* 'limit + 1' is also present */
	return hashSearch(t, uint64(limit))
}

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
// FLAG: trivial field write; not a C ltable export.
func luaH_setmetatable(t *Table, mt *Table) { t.metatable = mt }
