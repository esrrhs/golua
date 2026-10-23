// lmathutil.go — math primitives used by the translated runtime.
//
// There is no C translation unit of this name. These wrappers are the Go
// stand-ins for the macros / libm calls that lmathlib.c, lvm.c, lobject.c
// and friends expand via l_mathop / luai_num* (PUC-Lua 5.4.9).
//
// Mapped C sources:
//
//	luaconf.h  — l_mathop(op), l_floor(x)  (LUA_FLOAT_DOUBLE → bare names)
//	llimits.h  — luai_numisnan, luai_numpow, luai_nummod, luai_numidiv
//	lmathlib.c — l_mathop(floor|ceil|fabs|fmod|pow|sqrt) call sites
//	lfunc.h    — CLOSEKTOP (relocated here; value unchanged)
//
// FLAG (platform, keep): companion to lmathlib.go and libm_{cgo,nocgo}.go.
// Transcendentals (fmod/pow/sqrt) go through libm_* so a cgo build asks the
// same host libm PUC-Lua does; floor/ceil/trunc/fabs/isnan/isinf use Go math
// (IEEE-identical for these ops under LUA_FLOAT_DOUBLE).
package core

import "math"

// math2Floor is l_floor / l_mathop(floor) (luaconf.h).
func math2Floor(n float64) float64 { return math.Floor(n) }

// math2Ceil is l_mathop(ceil) (luaconf.h / lmathlib.c).
func math2Ceil(n float64) float64 { return math.Ceil(n) }

// math2Trunc is C99 trunc(); used for "integral float" checks where C
// relies on lua_numbertointeger's range test after an already-integral value.
func math2Trunc(n float64) float64 { return math.Trunc(n) }

// math2Abs is l_mathop(fabs) (luaconf.h / lmathlib.c / ltable.c).
func math2Abs(n float64) float64 { return math.Abs(n) }

// math2IsNaN is luai_numisnan (llimits.h): !luai_numeq(a, a).
func math2IsNaN(n float64) bool { return n != n }

// math2IsInf is the Go stand-in for C99 isinf / HUGE_VAL comparisons.
// Not a PUC-Lua macro; callers that need IEEE infinity tests use this
// (or math.IsInf) where C would call isinf or compare against HUGE_VAL.
func math2IsInf(n float64, sign int) bool { return math.IsInf(n, sign) }

// math2Fmod is l_mathop(fmod) only (lmathlib.c math_fmod; first step of
// luai_nummod). Sign correction for Lua's float `%` lives in math2NumMod.
func math2Fmod(x, y float64) float64 { return libmFmod(x, y) }

// math2NumMod is luai_nummod (llimits.h):
//
//	m = l_mathop(fmod)(a,b);
//	if ((m > 0) ? (b < 0) : (m < 0 && b > 0)) m += b;
//
// Gives the remainder the same sign as the divisor (Lua `%`), correcting
// fmod's trunc-toward-zero quotient. luaV_modf in lvm.go still inlines the
// same test; prefer this helper for new call sites.
func math2NumMod(a, b float64) float64 {
	m := math2Fmod(a, b)
	if m > 0 {
		if b < 0 {
			m += b
		}
	} else if m < 0 && b > 0 {
		m += b
	}
	return m
}

// math2Pow is luai_numpow (llimits.h):
//
//	(b == 2) ? (a)*(a) : l_mathop(pow)(a,b)
func math2Pow(x, y float64) float64 {
	if y == 2 {
		return x * x
	}
	return libmPow(x, y)
}

// math2Idiv is luai_numidiv (llimits.h): l_floor(luai_numdiv(L,a,b)).
func math2Idiv(a, b float64) float64 { return math2Floor(a / b) }

// math2Sqrt is l_mathop(sqrt) (lmathlib.c math_sqrt).
func math2Sqrt(x float64) float64 { return libmSqrt(x) }

// closeKTOP is CLOSEKTOP from lfunc.h: close to-be-closed variables up to
// the top of the stack. Relocated here (same value -1); see lfunc.go note.
const closeKTOP = -1
