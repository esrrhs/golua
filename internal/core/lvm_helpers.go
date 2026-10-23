// Copyright 2026 The golua Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// lvm_helpers.go — number conversion / compare / objlen helpers that live
// in C's lvm.c/.h. Kept in a separate file so the 5.4.9 line-faithful
// lvm.go / ldebug.go split stays clean (Apply debt from align-wip-54/ldebug).

package core

func luaV_tonumber_(L *State, o *Value, n *float64) bool {
	switch {
	case ttisinteger(o):
		*n = float64(ivalue(o))
		return true
	case ttisfloat(o):
		*n = fltvalue(o)
		return true
	case ttisstring(o): // string coercion
		var v Value
		return luaO_str2num(tsvalue(o).val, &v) && numberToFloat(&v, n)
	}
	return false
}

func numberToFloat(v *Value, n *float64) bool {
	if ttisinteger(v) {
		*n = float64(ivalue(v))
		return true
	}
	if ttisfloat(v) {
		*n = fltvalue(v)
		return true
	}
	return false
}

// luaV_flttointeger converts a float to an integer with a rounding mode.
func luaV_flttointeger(n float64, p *int64, mode F2Imod) bool {
	var f float64
	switch mode {
	case F2Ifloor:
		f = mathFloor(n)
	case F2Iceil:
		f = mathCeil(n)
	default:
		f = n
	}
	return floatToInt(f, p)
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

func floatToInt(n float64, p *int64) bool {
	if n == math2Trunc(n) && !math2IsNaN(n) && n >= -9.223372036854775808e18 && n < 9.223372036854775808e18 {
		*p = int64(n)
		return true
	}
	return false
}

// luaV_tointeger converts a value to an integer (with string coercion).
//
// C converts the string to a number first and *then* applies the rounding
// mode, so '"10.8"' becomes 10 under F2Ifloor and 11 under F2Iceil.
// Dropping the mode (as a plain conversion would) makes 'for i = 1,"10.8"'
// fail to convert and fall into the wrong, unbounded loop.
func luaV_tointeger(L *State, o *Value, p *int64, mode F2Imod) bool {
	if ttisstring(o) {
		var v Value
		if luaO_str2num(tsvalue(o).val, &v) {
			o = &v /* change it to point to its corresponding number */
		}
	}
	return luaV_tointegerns(o, p, mode)
}

// luaV_tointegerns converts a number to an integer without string coercion.
func luaV_tointegerns(o *Value, p *int64, mode F2Imod) bool {
	switch {
	case ttisinteger(o):
		*p = ivalue(o)
		return true
	case ttisfloat(o):
		return luaV_flttointeger(fltvalue(o), p, mode)
	}
	return false
}

// F2Imod is the rounding mode for float -> integer conversion (lvm.h).
type F2Imod int

const (
	F2Ieq F2Imod = iota
	F2Ifloor
	F2Iceil
)

// LUA_FLOORN2I is the rounding mode used to convert floats to integers
// (lvm.h). It is F2Ieq unless overridden.
const LUA_FLOORN2I = F2Ieq

// tonumberns converts to float without string coercion (lvm.h).
func tonumberns(o *Value, n *float64) bool {
	switch {
	case ttisfloat(o):
		*n = fltvalue(o)
		return true
	case ttisinteger(o):
		*n = float64(ivalue(o))
		return true
	}
	return false
}

// tointegerns converts to integer without string coercion (lvm.h).
func tointegerns(o *Value, i *int64) bool {
	if ttisinteger(o) {
		*i = ivalue(o)
		return true
	}
	return luaV_tointegerns(o, i, F2Ieq)
}

// tointeger converts to integer with string coercion (lvm.h).
func tointeger(L *State, o *Value, i *int64) bool {
	if ttisinteger(o) {
		*i = ivalue(o)
		return true
	}
	return luaV_tointeger(L, o, i, F2Ieq)
}

// luaV_equalobj / luaV_lessthan / luaV_lessequal implement comparisons
// with metamethods (lvm.c).
func luaV_equalobj(L *State, t1, t2 *Value) bool {
	var tm *Value
	if ttypetag(t1) != ttypetag(t2) { /* not the same variant? */
		if ttype(t1) != ttype(t2) || ttype(t1) != LUA_TNUMBER {
			return false /* only numbers can be equal with different variants */
		}
		/* two numbers with different variants: one of them is an
		   integer; if the other has no exact integer value they
		   cannot be equal, otherwise compare the integer values */
		var i1, i2 int64
		return tointegerns(t1, &i1) && tointegerns(t2, &i2) && i1 == i2
	}
	/* values have the same type and the same variant */
	switch ttypetag(t1) {
	case LUA_VNIL, LUA_VFALSE, LUA_VTRUE:
		return true
	case LUA_VNUMINT:
		return ivalue(t1) == ivalue(t2)
	case LUA_VNUMFLT:
		return fltvalue(t1) == fltvalue(t2)
	case LUA_VLIGHTUSERDATA:
		return pvalue(t1) == pvalue(t2)
	case LUA_VLCF: /* light C functions: identity */
		return gcvalue(t1) == gcvalue(t2)
	case LUA_VSHRSTR:
		return luaS_eqshrstr(tsvalue(t1), tsvalue(t2))
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
	default:
		return gcvalue(t1) == gcvalue(t2)
	}
	if tm == nil { /* no TM? */
		return false /* objects are different */
	}
	luaT_callTMres(L, tm, t1, t2, L.top) /* call TM */
	return !isFalse(&L.stack[L.top])
}

// maxIntFitsF is MAXINTFITSF: 1 << NBM, with NBM the number of bits in
// the mantissa of a float (53 for an IEEE-754 double).
const maxIntFitsF = uint64(1) << 53

// lIntfitsf reports whether 'i' has an exact representation as a float
// (l_intfitsf in lvm.c). The addition is unsigned and wraps, exactly as
// in C, which is what turns the test into "is i in [-2^53, 2^53]".
func lIntfitsf(i int64) bool {
	return maxIntFitsF+uint64(i) <= 2*maxIntFitsF
}

// ltIntFloat returns 'i < f' (LTintfloat). When 'i' has no exact float
// representation, the two cannot be compared as floats: use the
// equivalence 'i < f <=> i < ceil(f)'.
func ltIntFloat(i int64, f float64) bool {
	if lIntfitsf(i) {
		return float64(i) < f
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
		return i < fi
	}
	return f > 0 /* 'f' is either greater or less than all integers */
}

func leIntFloat(i int64, f float64) bool {
	if lIntfitsf(i) {
		return float64(i) <= f
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
		return i <= fi
	}
	return f > 0
}

func ltFloatInt(f float64, i int64) bool {
	if lIntfitsf(i) {
		return f < float64(i)
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Ifloor) { /* fi = floor(f) */
		return fi < i
	}
	return f < 0
}

func leFloatInt(f float64, i int64) bool {
	if lIntfitsf(i) {
		return f <= float64(i)
	}
	var fi int64
	if luaV_flttointeger(f, &fi, F2Iceil) { /* fi = ceil(f) */
		return fi <= i
	}
	return f < 0
}

// ltNum returns 'l < r' for two numbers (LTnum in lvm.c).
func ltNum(l, r *Value) bool {
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li < ivalue(r)
		}
		return ltIntFloat(li, fltvalue(r))
	}
	lf := fltvalue(l)
	if ttisfloat(r) {
		return lf < fltvalue(r)
	}
	return ltFloatInt(lf, ivalue(r))
}

// leNum returns 'l <= r' for two numbers (LEnum in lvm.c).
func leNum(l, r *Value) bool {
	if ttisinteger(l) {
		li := ivalue(l)
		if ttisinteger(r) {
			return li <= ivalue(r)
		}
		return leIntFloat(li, fltvalue(r))
	}
	lf := fltvalue(l)
	if ttisfloat(r) {
		return lf <= fltvalue(r)
	}
	return leFloatInt(lf, ivalue(r))
}

// luaV_lessthan is the main operation 'l < r' (lvm.c).
func luaV_lessthan(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) {
		return ltNum(l, r)
	}
	if ttisstring(l) && ttisstring(r) {
		return luaV_strcmp(tsvalue(l).val, tsvalue(r).val) < 0
	}
	return luaT_callorderTM(L, l, r, TM_LT)
}

// luaV_lessequal is the main operation 'l <= r' (lvm.c).
func luaV_lessequal(L *State, l, r *Value) bool {
	if ttisnumber(l) && ttisnumber(r) {
		return leNum(l, r)
	}
	if ttisstring(l) && ttisstring(r) {
		return luaV_strcmp(tsvalue(l).val, tsvalue(r).val) <= 0
	}
	return luaT_callorderTM(L, l, r, TM_LE)
}

// luaV_objlen is the main operation 'ra = #rb' (lvm.c).
func luaV_objlen(L *State, ra int, rb *Value) {
	var tm *Value
	switch ttypetag(rb) {
	case LUA_VTABLE:
		h := hvalue(rb)
		tm = fasttm(L, h.metatable, TM_LEN)
		if tm == nil {
			setivalue(&L.stack[ra], int64(luaH_getn(h)))
			return
		}
	case LUA_VSHRSTR, LUA_VLNGSTR:
		setivalue(&L.stack[ra], int64(len(tsvalue(rb).val)))
		return
	default:
		tm = luaT_gettmbyobj(L, rb, TM_LEN)
		if notm(tm) {
			luaG_typeerror(L, rb, "get length of")
			return
		}
	}
	luaT_callTMres(L, tm, rb, rb, ra)
}
