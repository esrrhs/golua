// Copyright 2026 The golua Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// lvm_helpers.go — number conversion / compare / objlen helpers that live
// in C's lvm.c/.h. Kept in a separate file so the 5.4.9 line-faithful
// lvm.go / ldebug.go split stays clean (Apply debt from align-wip-54/ldebug).

package core

/*
** 'l_intfitsf' checks whether a given integer is in the range that
** can be converted to a float without rounding. Used in comparisons.
 */

/* number of bits in the mantissa of a float (IEEE-754 double) */
const NBM = 53

/* limit for integers that fit in a float */
const MAXINTFITSF = uint64(1) << NBM

/* check whether 'i' is in the interval [-MAXINTFITSF, MAXINTFITSF] */
func l_intfitsf(i int64) bool {
	return MAXINTFITSF+uint64(i) <= 2*MAXINTFITSF
}

/*
** Try to convert a value from string to a number value.
** If the value is not a string or is a string not representing
** a valid numeral (or if coercions from strings to numbers
** are disabled via macro 'cvt2num'), do not modify 'result'
** and return false.
 */
func l_strton(obj *Value, result *Value) bool {
	/* lua_assert(obj != result) */
	if !cvt2num(obj) { /* is object not a string? */
		return false
	} else {
		st := tsvalue(obj)
		/* C: return (luaO_str2num(getstr(st), result) == tsslen(st) + 1);
		   Go's luaO_str2num already requires a full-numeral parse. */
		return luaO_str2num(getstr(st), result)
	}
}

/*
** Try to convert a value to a float. The float case is already handled
** by the macro 'tonumber'.
**
** C signature has no lua_State*; the unused L is kept so existing
** call sites that still pass a state (and that should eventually call
** tonumber) keep compiling.
**
** DEVIATION: also accepts floats. C's luaV_tonumber_ does not — the
** tonumber macro does. Several siblings still call luaV_tonumber_
** where C uses tonumber; until those switch, float handling stays here.
 */
func luaV_tonumber_(L *State, obj *Value, n *float64) bool {
	_ = L
	var v Value
	if ttisinteger(obj) {
		*n = float64(ivalue(obj))
		return true
	} else if l_strton(obj, &v) { /* string coercible to number? */
		*n = nvalue(&v) /* convert result of 'luaO_str2num' to a float */
		return true
	} else if ttisfloat(obj) { /* see DEVIATION above */
		*n = fltvalue(obj)
		return true
	}
	return false /* conversion failed */
}

/*
** try to convert a float to an integer, rounding according to 'mode'.
 */
func luaV_flttointeger(n float64, p *int64, mode F2Imod) bool {
	f := math2Floor(n) /* l_floor(n) */
	if n != f {        /* not an integral value? */
		if mode == F2Ieq {
			return false /* fails if mode demands integral value */
		} else if mode == F2Iceil { /* needs ceil? */
			f += 1 /* convert floor to ceil (remember: n != f) */
		}
	}
	return lua_numbertointeger(f, p)
}

/* lua_numbertointeger (luaconf.h) — also used as floatToInt below. */
func lua_numbertointeger(n float64, p *int64) bool {
	if n >= float64(LUA_MININTEGER) && n < -float64(LUA_MININTEGER) {
		*p = int64(n)
		return true
	}
	return false
}

/*
** try to convert a value to an integer, rounding according to 'mode',
** without string coercion.
** ("Fast track" handled by macro 'tointegerns'.)
 */
func luaV_tointegerns(obj *Value, p *int64, mode F2Imod) bool {
	if ttisfloat(obj) {
		return luaV_flttointeger(fltvalue(obj), p, mode)
	} else if ttisinteger(obj) {
		*p = ivalue(obj)
		return true
	} else {
		return false
	}
}

/*
** try to convert a value to an integer.
**
** C has no lua_State*; L is kept for call-site compatibility.
 */
func luaV_tointeger(L *State, obj *Value, p *int64, mode F2Imod) bool {
	_ = L
	var v Value
	if l_strton(obj, &v) { /* does 'obj' point to a numerical string? */
		obj = &v /* change it to point to its corresponding number */
	}
	return luaV_tointegerns(obj, p, mode)
}

// ---------------------------------------------------------------------
// lvm.h macros / types
// ---------------------------------------------------------------------

// cvt2num: always convert strings to numbers in this port (lvm.h; LUA_NOCVTS2N unset).
func cvt2num(o *Value) bool { return ttisstring(o) }

// F2Imod is the rounding mode for float -> integer conversion (lvm.h).
type F2Imod int

const (
	F2Ieq    F2Imod = iota /* no rounding; accepts only integral values */
	F2Ifloor               /* takes the floor of the number */
	F2Iceil                /* takes the ceil of the number */
)

/*
** You can define LUA_FLOORN2I if you want to convert floats to integers
** by flooring them (instead of raising an error if they are not
** integral values)
 */
const LUA_FLOORN2I = F2Ieq

/* convert an object to a float (including string coercion) */
func tonumber(o *Value, n *float64) bool {
	if ttisfloat(o) {
		*n = fltvalue(o)
		return true
	}
	return luaV_tonumber_(nil, o, n)
}

/* convert an object to a float (without string coercion) */
func tonumberns(o *Value, n *float64) bool {
	if ttisfloat(o) {
		*n = fltvalue(o)
		return true
	}
	if ttisinteger(o) {
		*n = float64(ivalue(o))
		return true
	}
	return false
}

/* convert an object to an integer (including string coercion) */
func tointeger(L *State, o *Value, i *int64) bool {
	if ttisinteger(o) { /* l_likely */
		*i = ivalue(o)
		return true
	}
	return luaV_tointeger(L, o, i, LUA_FLOORN2I)
}

/* convert an object to an integer (without string coercion) */
func tointegerns(o *Value, i *int64) bool {
	if ttisinteger(o) { /* l_likely */
		*i = ivalue(o)
		return true
	}
	return luaV_tointegerns(o, i, LUA_FLOORN2I)
}

// ---------------------------------------------------------------------
// floatToInt / mathFloor / mathCeil — kept for siblings (ltable, lmathlib).
// floatToInt ≡ luaV_flttointeger(..., F2Ieq). mathFloor/Ceil are l_floor /
// ceil wrappers used by lmathlib.go (not part of lvm.c).
// ---------------------------------------------------------------------

func floatToInt(n float64, p *int64) bool {
	return luaV_flttointeger(n, p, F2Ieq)
}

func mathFloor(n float64) float64 {
	if n == math2Trunc(n) {
		return n
	}
	return math2Floor(n)
}

func mathCeil(n float64) float64 {
	if n == math2Trunc(n) {
		return n
	}
	return math2Ceil(n)
}

/*
** Check whether integer 'i' is less than float 'f'. If 'i' has an
** exact representation as a float ('l_intfitsf'), compare numbers as
** floats. Otherwise, use the equivalence 'i < f <=> i < ceil(f)'.
** If 'ceil(f)' is out of integer range, either 'f' is greater than
** all integers or less than all integers.
** When 'f' is NaN, comparisons must result in false.
 */
func LTintfloat(i int64, f float64) bool {
	if l_intfitsf(i) {
		return float64(i) < f /* luai_numlt */
	} else { /* i < f <=> i < ceil(f) */
		var fi int64
		if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
			return i < fi /* compare them as integers */
		} else { /* 'f' is either greater or less than all integers */
			return f > 0 /* greater? */
		}
	}
}

/*
** Check whether integer 'i' is less than or equal to float 'f'.
 */
func LEintfloat(i int64, f float64) bool {
	if l_intfitsf(i) {
		return float64(i) <= f /* luai_numle */
	} else { /* i <= f <=> i <= floor(f) */
		var fi int64
		if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
			return i <= fi /* compare them as integers */
		} else { /* 'f' is either greater or less than all integers */
			return f > 0 /* greater? */
		}
	}
}

/*
** Check whether float 'f' is less than integer 'i'.
 */
func LTfloatint(f float64, i int64) bool {
	if l_intfitsf(i) {
		return f < float64(i) /* luai_numlt */
	} else { /* f < i <=> floor(f) < i */
		var fi int64
		if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
			return fi < i /* compare them as integers */
		} else { /* 'f' is either greater or less than all integers */
			return f < 0 /* less? */
		}
	}
}

/*
** Check whether float 'f' is less than or equal to integer 'i'.
 */
func LEfloatint(f float64, i int64) bool {
	if l_intfitsf(i) {
		return f <= float64(i) /* luai_numle */
	} else { /* f <= i <=> ceil(f) <= i */
		var fi int64
		if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
			return fi <= i /* compare them as integers */
		} else { /* 'f' is either greater or less than all integers */
			return f < 0 /* less? */
		}
	}
}

/* aliases retained for covermore_test.go (C names are LEintfloat / LEfloatint) */
func leIntFloat(i int64, f float64) bool { return LEintfloat(i, f) }
func leFloatInt(f float64, i int64) bool { return LEfloatint(f, i) }

/*
** Return 'l < r', for numbers.
 */
func LTnum(l, r *Value) bool {
	/* lua_assert(ttisnumber(l) && ttisnumber(r)) */
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li < ivalue(r) /* both are integers */
		} else { /* 'l' is int and 'r' is float */
			return LTintfloat(li, fltvalue(r)) /* l < r ? */
		}
	} else {
		lf := fltvalue(l) /* 'l' must be float */
		if ttisfloat(r) {
			return lf < fltvalue(r) /* both are float; luai_numlt */
		} else { /* 'l' is float and 'r' is int */
			return LTfloatint(lf, ivalue(r))
		}
	}
}

/*
** Return 'l <= r', for numbers.
 */
func LEnum(l, r *Value) bool {
	/* lua_assert(ttisnumber(l) && ttisnumber(r)) */
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li <= ivalue(r) /* both are integers */
		} else { /* 'l' is int and 'r' is float */
			return LEintfloat(li, fltvalue(r)) /* l <= r ? */
		}
	} else {
		lf := fltvalue(l) /* 'l' must be float */
		if ttisfloat(r) {
			return lf <= fltvalue(r) /* both are float; luai_numle */
		} else { /* 'l' is float and 'r' is int */
			return LEfloatint(lf, ivalue(r))
		}
	}
}

/*
** return 'l < r' for non-numbers.
 */
func lessthanothers(L *State, l, r *Value) bool {
	/* lua_assert(!ttisnumber(l) || !ttisnumber(r)) */
	if ttisstring(l) && ttisstring(r) { /* both are strings? */
		return l_strcmp(tsvalue(l), tsvalue(r)) < 0
	} else {
		return luaT_callorderTM(L, l, r, TM_LT)
	}
}

/*
** Main operation less than; return 'l < r'.
 */
func luaV_lessthan(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) { /* both operands are numbers? */
		return LTnum(l, r)
	} else {
		return lessthanothers(L, l, r)
	}
}

/*
** return 'l <= r' for non-numbers.
 */
func lessequalothers(L *State, l, r *Value) bool {
	/* lua_assert(!ttisnumber(l) || !ttisnumber(r)) */
	if ttisstring(l) && ttisstring(r) { /* both are strings? */
		return l_strcmp(tsvalue(l), tsvalue(r)) <= 0
	} else {
		return luaT_callorderTM(L, l, r, TM_LE)
	}
}

/*
** Main operation less than or equal to; return 'l <= r'.
 */
func luaV_lessequal(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) { /* both operands are numbers? */
		return LEnum(l, r)
	} else {
		return lessequalothers(L, l, r)
	}
}

/* l_strcmp (lvm.c) — locale-aware string order; body lives in locale.go. */
func l_strcmp(ts1, ts2 *TString) int {
	return luaV_strcmp(getstr(ts1), getstr(ts2))
}

/*
** Main operation for equality of Lua values; return 't1 == t2'.
** L == nil means raw equality (no metamethods)
 */
func luaV_equalobj(L *State, t1, t2 *Value) bool {
	var tm *Value
	if ttypetag(t1) != ttypetag(t2) { /* not the same variant? */
		if ttype(t1) != ttype(t2) || ttype(t1) != LUA_TNUMBER {
			return false /* only numbers can be equal with different variants */
		} else { /* two numbers with different variants */
			/* One of them is an integer. If the other does not have an
			   integer value, they cannot be equal; otherwise, compare their
			   integer values. */
			var i1, i2 int64
			return luaV_tointegerns(t1, &i1, F2Ieq) &&
				luaV_tointegerns(t2, &i2, F2Ieq) &&
				i1 == i2
		}
	}
	/* values have same type and same variant */
	switch ttypetag(t1) {
	case LUA_VNIL, LUA_VFALSE, LUA_VTRUE:
		return true
	case LUA_VNUMINT:
		return ivalue(t1) == ivalue(t2)
	case LUA_VNUMFLT:
		return fltvalue(t1) == fltvalue(t2) /* luai_numeq */
	case LUA_VLIGHTUSERDATA:
		return pvalue(t1) == pvalue(t2)
	case LUA_VLCF:
		return pvalue(t1) == pvalue(t2) /* C: fvalue(t1) == fvalue(t2); Go funcs are not comparable */
	case LUA_VSHRSTR:
		return luaS_eqshrstr(tsvalue(t1), tsvalue(t2)) /* eqshrstr */
	case LUA_VLNGSTR:
		return luaS_eqlngstr(tsvalue(t1), tsvalue(t2))
	case LUA_VUSERDATA:
		if uvalue(t1) == uvalue(t2) {
			return true
		} else if L == nil {
			return false
		}
		tm = fasttm(L, uvalue(t1).metatable, TM_EQ)
		if tm == nil {
			tm = fasttm(L, uvalue(t2).metatable, TM_EQ)
		}
		/* will try TM */
	case LUA_VTABLE:
		if hvalue(t1) == hvalue(t2) {
			return true
		} else if L == nil {
			return false
		}
		tm = fasttm(L, hvalue(t1).metatable, TM_EQ)
		if tm == nil {
			tm = fasttm(L, hvalue(t2).metatable, TM_EQ)
		}
		/* will try TM */
	default:
		return gcvalue(t1) == gcvalue(t2)
	}
	if tm == nil { /* no TM? */
		return false /* objects are different */
	} else {
		luaT_callTMres(L, tm, t1, t2, L.top) /* call TM */
		return !l_isfalse(&L.stack[L.top])
	}
}

/*
** Main operation 'ra = #rb'.
 */
func luaV_objlen(L *State, ra int, rb *Value) {
	var tm *Value
	switch ttypetag(rb) {
	case LUA_VTABLE:
		h := hvalue(rb)
		tm = fasttm(L, h.metatable, TM_LEN)
		if tm != nil {
			break /* metamethod? break switch to call it */
		}
		setivalue(&L.stack[ra], int64(luaH_getn(h))) /* else primitive len */
		return
	case LUA_VSHRSTR:
		setivalue(&L.stack[ra], int64(tsslen(tsvalue(rb))))
		return
	case LUA_VLNGSTR:
		setivalue(&L.stack[ra], int64(tsslen(tsvalue(rb))))
		return
	default: /* try metamethod */
		tm = luaT_gettmbyobj(L, rb, TM_LEN)
		if notm(tm) { /* no metamethod? */
			luaG_typeerror(L, rb, "get length of")
			return
		}
	}
	luaT_callTMres(L, tm, rb, rb, ra)
}
