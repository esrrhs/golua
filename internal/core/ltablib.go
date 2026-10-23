// Translation of ltablib.c (PUC-Lua 5.4.9): the table manipulation
// library, including the quicksort used by table.sort.
package core

import (
	"math"
	"time"
)

/*
** Operations that an object must define to mimic a table
** (some functions only need some of them)
 */
const (
	tabR  = 1           /* read */
	tabW  = 2           /* write */
	tabL  = 4           /* length */
	tabRW = tabR | tabW /* read/write */
)

// tableAuxGetn checks 'n' as a table-like argument and returns its
// length (aux_getn).
func tableAuxGetn(L *State, arg, what int) int64 {
	tableCheckTab(L, arg, what|tabL)
	return L.Len2(arg)
}

func tableCheckField(L *State, key string, n int) bool {
	L.PushString(key)
	return L.RawGet(-n) != LUA_TNIL
}

/*
** Check that 'arg' either is a table or can behave like one (that is,
** has a metatable with the required metamethods)
 */
func tableCheckTab(L *State, arg, what int) {
	if L.Type(arg) != LUA_TTABLE { /* is it not a table? */
		n := 1                    /* number of elements to pop */
		if L.GetMetaTable(arg) && /* must have metatable */
			(what&tabR == 0 || tableCheckField(L, "__index", tableIncr(&n))) &&
			(what&tabW == 0 || tableCheckField(L, "__newindex", tableIncr(&n))) &&
			(what&tabL == 0 || tableCheckField(L, "__len", tableIncr(&n))) {
			L.Pop(n) /* pop metatable and tested metamethods */
		} else {
			L.CheckType(arg, LUA_TTABLE) /* force an error */
		}
	}
}

// tableIncr is the Go spelling of the pre-increment used by checktab.
func tableIncr(n *int) int {
	*n++
	return *n
}

func tableInsert(L *State) int {
	var pos int64 /* where to insert new element */
	e := tableAuxGetn(L, 1, tabRW)
	e = e + 1 /* first empty element (luaL_intop(+, e, 1)) */
	switch L.GetTop() {
	case 2: /* called with only 2 arguments */
		pos = e /* insert new element at the end */
	case 3:
		pos = L.CheckInteger(2) /* 2nd argument is the position */
		/* check whether 'pos' is in [1, e] */
		L.ArgCheck(uint64(pos)-1 < uint64(e), 2, "position out of bounds")
		for i := e; i > pos; i-- { /* move up elements */
			L.GetI(1, i-1)
			L.SetI(1, i) /* t[i] = t[i - 1] */
		}
	default:
		return L.Error2("wrong number of arguments to 'insert'")
	}
	L.SetI(1, pos) /* t[pos] = v */
	return 0
}

func tableRemove(L *State) int {
	size := tableAuxGetn(L, 1, tabRW)
	pos := L.OptInteger(2, size)
	if pos != size { /* validate 'pos' if given */
		/* check whether 'pos' is in [1, size + 1] */
		L.ArgCheck(uint64(pos)-1 <= uint64(size), 2, "position out of bounds")
	}
	L.GetI(1, pos) /* result = t[pos] */
	for ; pos < size; pos++ {
		L.GetI(1, pos+1)
		L.SetI(1, pos) /* t[pos] = t[pos + 1] */
	}
	L.PushNil()
	L.SetI(1, pos) /* remove entry t[pos] */
	return 1
}

/*
** Copy elements (1[f], ..., 1[e]) into (tt[t], tt[t+1], ...). Whenever
** possible, copy in increasing order, which is better for rehashing.
** "possible" means destination after original range, or smaller
** than origin, or copying to another table.
 */
func tableMove(L *State) int {
	f := L.CheckInteger(2)
	e := L.CheckInteger(3)
	t := L.CheckInteger(4)
	tt := 1 /* destination table */
	if !L.IsNoneOrNil(5) {
		tt = 5
	}
	tableCheckTab(L, 1, tabR)
	tableCheckTab(L, tt, tabW)
	if e >= f { /* otherwise, nothing to move */
		var n, i int64
		L.ArgCheck(f > 0 || e < LUA_MAXINTEGER+f, 3,
			"too many elements to move")
		n = e - f + 1 /* number of elements to move */
		L.ArgCheck(t <= LUA_MAXINTEGER-n+1, 4,
			"destination wrap around")
		if t > e || t <= f || (tt != 1 && !L.Equal(1, tt)) {
			for i = 0; i < n; i++ {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		} else {
			for i = n - 1; i >= 0; i-- {
				L.GetI(1, f+i)
				L.SetI(tt, t+i)
			}
		}
	}
	L.PushValue(tt) /* return destination table */
	return 1
}

func tableAddField(L *State, b *Buffer, i int64) {
	L.GetI(1, i)
	if !L.IsString(-1) {
		L.Error2("invalid value (%s) at index %I in table for 'concat'",
			L.TypeName(-1), i)
	}
	b.AddValue()
}

func tableConcat(L *State) int {
	last := tableAuxGetn(L, 1, tabR)
	sep := L.OptString(2, "")
	i := L.OptInteger(3, 1)
	last = L.OptInteger(4, last)
	b := L.NewBuffer()
	for ; i < last; i++ {
		tableAddField(L, b, i)
		b.AddString(sep)
	}
	if i == last { /* add last value (if interval was not empty) */
		tableAddField(L, b, i)
	}
	b.PushResult()
	return 1
}

/*
** {======================================================
** Pack/unpack
** =======================================================
 */

func tablePack(L *State) int {
	n := L.GetTop()           /* number of elements to pack */
	L.CreateTable(n, 1)       /* create result table */
	L.Insert(1)               /* put it at index 1 */
	for i := n; i >= 1; i-- { /* assign elements */
		L.SetI(1, int64(i))
	}
	L.PushInteger(int64(n))
	L.SetField(1, "n") /* t.n = number of elements */
	return 1           /* return table */
}

func tableUnpack(L *State) int {
	i := L.OptInteger(2, 1)
	/* Not 'L.OptInteger(3, L.Len2(1))', although that is what
	   'luaL_opt(L, luaL_checkinteger, 3, luaL_len(L, 1))' looks like.
	   'luaL_opt' is a macro and its default is an expression that is
	   evaluated only in the branch that uses it; a Go call evaluates
	   every argument before it is entered, so the length is asked for
	   even when the third argument is there.  The two are different
	   programs: 'table.unpack(nil, 1, 3)' is "attempt to index a nil
	   value" in PUC-Lua, because it never gets as far as the length,
	   and was "attempt to get length of a nil value" here.  An absent
	   third argument is what 'lua_isnoneornil' says it is, which is
	   an explicit nil as well as nothing at all. */
	var e int64
	if L.IsNoneOrNil(3) {
		e = L.Len2(1)
	} else {
		e = L.CheckInteger(3)
	}
	if i > e {
		return 0 /* empty range */
	}
	n := uint64(e) - uint64(i) /* number of elements minus 1 (avoid overflows) */
	if n >= uint64(math.MaxInt32) || !L.CheckStack(int(n)+1) {
		return L.Error2("too many results to unpack")
	}
	n++
	for ; i < e; i++ { /* push arg[i..e - 1] (to avoid overflows) */
		L.GetI(1, i)
	}
	L.GetI(1, e) /* push last element */
	return int(n)
}

/* }====================================================== */

/*
** {======================================================
** Quicksort
** (based on 'Algorithms in MODULA-3', Robert Sedgewick;
**  Addison-Wesley, 1993.)
** =======================================================
 */

/*
** Produce a "random" 'unsigned int' to randomize pivot choice. This
** is used only when 'sort' detects a big imbalance in the result of
** a partition. (C: clock()+time() into unsigned int buff[]; Go: UnixNano.)
 */
func tableRandomizePivot() uint32 {
	now := time.Now().UnixNano()
	return uint32(now) ^ uint32(now>>32)
}

/* arrays larger than 'tabRanLimit' may use randomized pivots */
const tabRanLimit = 100

// tableSet2 stores the two values on top of the stack into t[i] and
// t[j] (set2).
func tableSet2(L *State, i, j uint32) {
	L.SetI(1, int64(i))
	L.SetI(1, int64(j))
}

/*
** Return true iff value at stack index 'a' is less than the value at
** index 'b' (according to the order of the sort).
 */
func tableSortComp(L *State, a, b int) bool {
	if L.IsNil(2) { /* no function? */
		return L.LessThan(a, b) /* a < b */
	}
	/* function */
	L.PushValue(2)         /* push function */
	L.PushValue(a - 1)     /* -1 to compensate function */
	L.PushValue(b - 2)     /* -2 to compensate function and 'a' */
	L.Call(2, 1)           /* call function */
	res := L.ToBoolean(-1) /* get result */
	L.Pop(1)               /* pop result */
	return res
}

/*
** Does the partition: Pivot P is at the top of the stack.
** precondition: a[lo] <= P == a[up-1] <= a[up],
** so it only needs to do the partition from lo + 1 to up - 2.
** Pos-condition: a[lo .. i - 1] <= a[i] == P <= a[i + 1 .. up]
** returns 'i'.
 */
func tablePartition(L *State, lo, up uint32) uint32 {
	i := lo     /* will be incremented before first use */
	j := up - 1 /* will be decremented before first use */
	/* loop invariant: a[lo .. i] <= P <= a[j .. up] */
	for {
		/* next loop: repeat ++i while a[i] < P */
		for {
			i++
			L.GetI(1, int64(i))
			if !tableSortComp(L, -1, -2) {
				break
			}
			if i == up-1 { /* a[i] < P  but a[up - 1] == P  ?? */
				L.Error2("invalid order function for sorting")
			}
			L.Pop(1) /* remove a[i] */
		}
		/* after the loop, a[i] >= P and a[lo .. i - 1] < P */
		/* next loop: repeat --j while P < a[j] */
		for {
			j--
			L.GetI(1, int64(j))
			if !tableSortComp(L, -3, -1) {
				break
			}
			if j < i { /* j < i  but  a[j] > P ?? */
				L.Error2("invalid order function for sorting")
			}
			L.Pop(1) /* remove a[j] */
		}
		/* after the loop, a[j] <= P and a[j + 1 .. up] >= P */
		if j < i { /* no elements out of place? */
			/* a[lo .. i - 1] <= P <= a[j + 1 .. i .. up] */
			L.Pop(1) /* pop a[j] */
			/* swap pivot (a[up - 1]) with a[i] to satisfy pos-condition */
			tableSet2(L, up-1, i)
			return i
		}
		/* otherwise, swap a[i] - a[j] to restore invariant and repeat */
		tableSet2(L, i, j)
	}
}

/*
** Choose an element in the middle (2nd-3th quarters) of [lo,up]
** "randomized" by 'rnd'
 */
func tableChoosePivot(lo, up, rnd uint32) uint32 {
	r4 := (up - lo) / 4 /* range/4 */
	p := rnd%(r4*2) + (lo + r4)
	/* lua_assert(lo + r4 <= p && p <= up - r4) — C debug-only */
	return p
}

/*
** Quicksort algorithm (recursive function)
 */
func tableAuxSort(L *State, lo, up, rnd uint32) {
	for lo < up { /* loop for tail recursion */
		var p uint32 /* Pivot index */
		var n uint32 /* to be used later */
		/* sort elements 'lo', 'p', and 'up' */
		L.GetI(1, int64(lo))
		L.GetI(1, int64(up))
		if tableSortComp(L, -1, -2) { /* a[up] < a[lo]? */
			tableSet2(L, lo, up) /* swap a[lo] - a[up] */
		} else {
			L.Pop(2) /* remove both values */
		}
		if up-lo == 1 { /* only 2 elements? */
			return /* already sorted */
		}
		if up-lo < tabRanLimit || rnd == 0 { /* small interval or no randomize? */
			p = (lo + up) / 2 /* middle element is a good pivot */
		} else { /* for larger intervals, it is worth a random pivot */
			p = tableChoosePivot(lo, up, rnd)
		}
		L.GetI(1, int64(p))
		L.GetI(1, int64(lo))
		if tableSortComp(L, -2, -1) { /* a[p] < a[lo]? */
			tableSet2(L, p, lo) /* swap a[p] - a[lo] */
		} else {
			L.Pop(1) /* remove a[lo] */
			L.GetI(1, int64(up))
			if tableSortComp(L, -1, -2) { /* a[up] < a[p]? */
				tableSet2(L, p, up) /* swap a[up] - a[p] */
			} else {
				L.Pop(2)
			}
		}
		if up-lo == 2 { /* only 3 elements? */
			return /* already sorted */
		}
		L.GetI(1, int64(p))    /* get middle element (Pivot) */
		L.PushValue(-1)        /* push Pivot */
		L.GetI(1, int64(up-1)) /* push a[up - 1] */
		tableSet2(L, p, up-1)  /* swap Pivot (a[p]) with a[up - 1] */
		p = tablePartition(L, lo, up)
		/* a[lo .. p - 1] <= a[p] == P <= a[p + 1 .. up] */
		if p-lo < up-p { /* lower interval is smaller? */
			tableAuxSort(L, lo, p-1, rnd) /* call recursively for lower interval */
			n = p - lo                    /* size of smaller interval */
			lo = p + 1                    /* tail call for [p + 1 .. up] (upper interval) */
		} else {
			tableAuxSort(L, p+1, up, rnd) /* call recursively for upper interval */
			n = up - p                    /* size of smaller interval */
			up = p - 1                    /* tail call for [lo .. p - 1]  (lower interval) */
		}
		if (up-lo)/128 > n { /* partition too imbalanced? */
			rnd = tableRandomizePivot() /* try a new randomization */
		}
	} /* tail call tableAuxSort(L, lo, up, rnd) */
}

func tableSort(L *State) int {
	n := tableAuxGetn(L, 1, tabRW)
	if n > 1 { /* non-trivial interval? */
		L.ArgCheck(n < int64(math.MaxInt32), 1, "array too big")
		if !L.IsNoneOrNil(2) { /* is there a 2nd argument? */
			L.CheckType(2, LUA_TFUNCTION) /* must be a function */
		}
		L.SetTop(2) /* make sure there are two arguments */
		tableAuxSort(L, 1, uint32(n), 0)
	}
	return 0
}

/* }====================================================== */

var tableFuncs = []Reg{
	{Name: "concat", Func: tableConcat},
	{Name: "insert", Func: tableInsert},
	{Name: "pack", Func: tablePack},
	{Name: "unpack", Func: tableUnpack},
	{Name: "remove", Func: tableRemove},
	{Name: "move", Func: tableMove},
	{Name: "sort", Func: tableSort},
}

// OpenTable opens the table library (luaopen_table).
func OpenTable(L *State) int {
	L.NewLib(tableFuncs)
	return 1
}
