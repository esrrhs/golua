// Translation of lmathlib.c (PUC-Lua 5.3.4): the mathematical library.
//
// math.random uses POSIX random()/srandom() (l_rand/l_srand), not the
// 5.4 xoshiro256** generator — sort.lua comparison counts and any
// seeded sequence must match PUC.
package core

import (
	"math"
)

const mathPI = 3.141592653589793238462643383279502884

func mathAbs(L *State) int {
	if L.IsInteger(1) {
		n, _ := L.ToInteger(1)
		if n < 0 {
			n = -n
		}
		L.PushInteger(n)
	} else {
		L.PushNumber(math.Abs(L.CheckNumber(1)))
	}
	return 1
}

func mathSin(L *State) int {
	L.PushNumber(libmSin(L.CheckNumber(1)))
	return 1
}

func mathCos(L *State) int {
	L.PushNumber(libmCos(L.CheckNumber(1)))
	return 1
}

func mathTan(L *State) int {
	L.PushNumber(libmTan(L.CheckNumber(1)))
	return 1
}

func mathAsin(L *State) int {
	L.PushNumber(libmAsin(L.CheckNumber(1)))
	return 1
}

func mathAcos(L *State) int {
	L.PushNumber(libmAcos(L.CheckNumber(1)))
	return 1
}

func mathAtan(L *State) int {
	y := L.CheckNumber(1)
	x := L.OptNumber(2, 1)
	L.PushNumber(libmAtan2(y, x))
	return 1
}

func mathToInt(L *State) int {
	if n, valid := L.ToInteger(1); valid {
		L.PushInteger(n)
	} else {
		L.CheckAny(1)
		L.PushNil() /* value is not convertible to integer */
	}
	return 1
}

// mathNumberToInteger converts a float with an integral value to an
// integer (luaconf.h lua_numbertointeger).
func mathNumberToInteger(d float64, p *int64) bool {
	if d >= float64(LUA_MININTEGER) && d < -float64(LUA_MININTEGER) {
		*p = int64(d)
		return true
	}
	return false
}

// mathPushNumInt pushes 'd' as an integer when it fits, as a float
// otherwise (lmathlib.c pushnumint).
func mathPushNumInt(L *State, d float64) {
	var n int64
	if mathNumberToInteger(d, &n) {
		L.PushInteger(n) /* result is integer */
	} else {
		L.PushNumber(d) /* result is float */
	}
}

func mathLibFloor(L *State) int {
	if L.IsInteger(1) {
		L.SetTop(1) /* integer is its own floor */
	} else {
		mathPushNumInt(L, mathFloor(L.CheckNumber(1)))
	}
	return 1
}

func mathLibCeil(L *State) int {
	if L.IsInteger(1) {
		L.SetTop(1) /* integer is its own ceil */
	} else {
		mathPushNumInt(L, mathCeil(L.CheckNumber(1)))
	}
	return 1
}

func mathFmod(L *State) int {
	if L.IsInteger(1) && L.IsInteger(2) {
		d, _ := L.ToInteger(2)
		if uint64(d)+1 <= 1 { /* special cases: -1 or 0 */
			L.ArgCheck(d != 0, 2, "zero")
			L.PushInteger(0) /* avoid overflow with 0x80000... / -1 */
		} else {
			n, _ := L.ToInteger(1)
			L.PushInteger(n % d)
		}
	} else {
		L.PushNumber(libmFmod(L.CheckNumber(1), L.CheckNumber(2)))
	}
	return 1
}

/*
** This function does not use 'modf', avoiding problems with 'float64'
** (the equivalent of the C remark about 'double*').
 */
func mathModf(L *State) int {
	if L.IsInteger(1) {
		L.SetTop(1)     /* number is its own integer part */
		L.PushNumber(0) /* no fractional part */
	} else {
		n := L.CheckNumber(1)
		/* integer part (rounds toward zero) */
		var ip float64
		if n < 0 {
			ip = math.Ceil(n)
		} else {
			ip = math.Floor(n)
		}
		mathPushNumInt(L, ip)
		/* fractional part (test needed for inf/-inf) */
		var f float64
		if n == ip {
			f = 0
		} else {
			f = n - ip
		}
		L.PushNumber(f)
	}
	return 2
}

func mathSqrt(L *State) int {
	L.PushNumber(libmSqrt(L.CheckNumber(1)))
	return 1
}

func mathUlt(L *State) int {
	a := L.CheckInteger(1)
	b := L.CheckInteger(2)
	L.PushBoolean(uint64(a) < uint64(b))
	return 1
}

func mathLog(L *State) int {
	x := L.CheckNumber(1)
	var res float64
	if L.IsNoneOrNil(2) {
		res = mathLogOf(x)
	} else {
		base := L.CheckNumber(2)
		if base == 2.0 {
			res = mathLog2Of(x)
		} else if base == 10.0 {
			res = mathLog10Of(x)
		} else {
			res = mathLogOf(x) / mathLogOf(base)
		}
	}
	L.PushNumber(res)
	return 1
}

func mathExp(L *State) int {
	x := L.CheckNumber(1)
	if haveLibm {
		L.PushNumber(libmExp(x)) /* the host libm, same as PUC-Lua */
		return 1
	}
	/* Not math.Exp on its own: see mathExpScaled -- on amd64 it answers
	   inf for exp(709.5), which is 1.3549863193146328e+308. */
	L.PushNumber(mathExpScaled(x, 0))
	return 1
}

func mathDeg(L *State) int {
	L.PushNumber(L.CheckNumber(1) * (180.0 / mathPI))
	return 1
}

func mathRad(L *State) int {
	L.PushNumber(L.CheckNumber(1) * (mathPI / 180.0))
	return 1
}

func mathMin(L *State) int {
	n := L.GetTop() /* number of arguments */
	imin := 1       /* index of current minimum value */
	L.ArgCheck(n >= 1, 1, "value expected")
	for i := 2; i <= n; i++ {
		if L.LessThan(i, imin) {
			imin = i
		}
	}
	L.PushValue(imin)
	return 1
}

func mathMax(L *State) int {
	n := L.GetTop() /* number of arguments */
	imax := 1       /* index of current maximum value */
	L.ArgCheck(n >= 1, 1, "value expected")
	for i := 2; i <= n; i++ {
		if L.LessThan(imax, i) {
			imax = i
		}
	}
	L.PushValue(imax)
	return 1
}

func mathType(L *State) int {
	if L.Type(1) == LUA_TNUMBER {
		if L.IsInteger(1) {
			L.PushString("integer")
		} else {
			L.PushString("float")
		}
	} else {
		L.CheckAny(1)
		L.PushNil()
	}
	return 1
}

// ---------------------------------------------------------------------
// math.random / math.randomseed (5.3.4 l_rand/l_srand)
// ---------------------------------------------------------------------

// figs is the number of binary digits in the mantissa of a float64.
const mathFigs = 53

/*
** 5.3.4 math.random / math.randomseed (lmathlib.c).
** Uses l_rand/l_srand (POSIX random/srandom when cgo is on).
*/

func mathRandom(L *State) int {
	low, up := int64(0), int64(0)
	r := float64(lRand()) * (1.0 / (float64(lRandMax) + 1.0))
	switch L.GetTop() {
	case 0:
		L.PushNumber(r)
		return 1
	case 1:
		low = 1
		up = L.CheckInteger(1)
	case 2:
		low = L.CheckInteger(1)
		up = L.CheckInteger(2)
	default:
		return L.Error2("wrong number of arguments")
	}
	L.ArgCheck(low <= up, 1, "interval is empty")
	L.ArgCheck(low >= 0 || up <= LUA_MAXINTEGER+low, 1, "interval too large")
	r *= float64(up-low) + 1.0
	L.PushInteger(int64(r) + low)
	return 1
}

func mathRandomSeed(L *State) int {
	/* 5.3: one number argument; discard first value after srand. */
	seed := L.CheckNumber(1)
	lSrand(uint(int64(seed)))
	_ = lRand()
	return 0
}

func mathSetRandFunc(L *State) {
	/* 5.3: no per-state xoshiro userdata; random is process-global. */
	L.SetFuncs([]Reg{
		{Name: "random", Func: mathRandom},
		{Name: "randomseed", Func: mathRandomSeed},
	}, 0)
}

// ---------------------------------------------------------------------
// deprecated functions (for compatibility only)
// ---------------------------------------------------------------------
//
// luaconf.h defines 'LUA_COMPAT_MATHLIB' and the reference build does
// not turn it off, so 'math.sinh', 'math.cosh', 'math.tanh', 'math.pow',
// 'math.frexp', 'math.ldexp', 'math.log10' and 'math.atan2' are all
// there even though Lua 5.4 dropped them from the manual.  A program
// that calls them cannot be run by a port that leaves them out, so
// they are all here.

const (
	// mathMaxLog is log(MaxFloat64): the argument at which exp(x)
	// overflows.  exp(x)/2 holds out for another ln 2 beyond it, up to
	// log(2 * MaxFloat64) = 710.47586007394386.
	mathMaxLog  = 7.09782712893383973096e+02
	mathMaxLog2 = 7.10475860073943863429e+02
)

// mathLn2Hi and mathLn2Lo are ln 2 split in two, so that 'n * ln 2'
// keeps the low bits that a single float64 would drop.
const (
	mathLn2Hi = 6.93147180369123816490e-01
	mathLn2Lo = 1.90821492927058770002e-10
)

// mathExpScaled is exp(x) * 2^shift, worked out so that nothing
// overflows before the answer itself has to.
//
// It cannot simply ask math.Exp, because Go's math.Exp is not one
// function but a hand-written assembly version per architecture, and
// they do not agree at the edge of what a float can hold: on amd64
// math.Exp(709.5) answers +Inf, where exp(709.5) is
// 1.3549863193146328e+308, a perfectly ordinary float, and that is what
// arm64 answers, what C's libm answers, and what Lua programs have been
// getting for as long as math.exp has existed.  Asking it for exp(709.5)
// and halving therefore loses -- which is how math.sinh(709.5) came out
// as inf on amd64 and as 6.7749315965731642e+307 everywhere else.
//
// So exp is only ever asked about an argument it cannot get wrong: the
// argument is reduced modulo ln 2 first, with both halves of the
// constant so that the remainder keeps its low bits, and the answer is
// scaled by a power of two afterwards, which is exact.  Folding the
// halving into the argument as exp(x - ln 2) instead would be simpler
// and wrong: subtracting ln 2 from a number this large rounds, and the
// rounding shows -- that answers 9.0542048151528e+307 for sinh(709.79)
// where C answers 9.0542048151523e+307.
func mathExpScaled(x float64, shift int) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return math.Exp(x) /* no amount of scaling changes these */
	}
	if e := math.Exp(x); !math.IsInf(e, 1) {
		return math.Ldexp(e, shift)
	}
	n := int(math.Round(x / math.Ln2))
	r := (x - float64(n)*mathLn2Hi) - float64(n)*mathLn2Lo
	return math.Ldexp(math.Exp(r), n+shift)
}

// mathHalfExp is exp(x) / 2 for a non-negative x: the value both 'sinh'
// and 'cosh' tend to as x grows, and one that is representable long
// after exp(x) itself is not.  Where even the half does not fit,
// scaling by 2^-1 overflows and the answer is +Inf, which is right.
func mathHalfExp(x float64) float64 { return mathExpScaled(x, -1) }

// mathLogScaled is C's 'log'.  It cannot simply ask math.Log, for the
// same reason mathExpScaled cannot ask math.Exp: Go's math.Log is an
// assembly routine on amd64 and s390x, and that routine loses several
// digits on a subnormal argument.  1e-308 is one -- the smallest normal
// float64 is 2.2250738585072014e-308 -- and log(1e-308) is
// -709.19620864217, which the routine answers as -708.71839997088.
// The pure Go version, the one arm64 gets, is right.
//
// So a subnormal is scaled up into the normal range first, by a power of
// two, which is exact, and the exponent is taken back off afterwards.
// The answer is never small enough for that to cost anything worth
// having: a subnormal's log is below -708, where the fourteenth
// significant digit -- one more than Lua prints -- is worth 1e-11, and
// this subtraction is worth about 1e-13.
const (
	mathSmallestNormal = 1 << 52 * math.SmallestNonzeroFloat64
	mathLogScale       = 1 << 54
	mathLogScaleLn     = 54 * math.Ln2
)

func mathLogScaled(x float64) float64 {
	if x > 0 && x < mathSmallestNormal {
		return math.Log(x*mathLogScale) - mathLogScaleLn
	}
	return math.Log(x)
}

// mathLog10Scaled is C's 'log10'.  Go's own is 'math.Log(x) * (1/ln 10)'
// -- see log10.go -- so it is the same correction with the factor
// applied afterwards.
func mathLog10Scaled(x float64) float64 {
	return mathLogScaled(x) * (1 / math.Ln10)
}

// mathLogOf / mathLog2Of / mathLog10Of are the C library's log, log2 and
// log10 when this build can call them, and the scaled Go versions when
// it cannot.
func mathLogOf(x float64) float64 {
	if haveLibm {
		return libmLog(x)
	}
	return mathLogScaled(x)
}

func mathLog2Of(x float64) float64 {
	if haveLibm {
		return libmLog2(x)
	}
	return mathLog2Scaled(x)
}

func mathLog10Of(x float64) float64 {
	if haveLibm {
		return libmLog10(x)
	}
	return mathLog10Scaled(x)
}

// mathLog2Scaled is C's 'log2', which is what math.log(x, 2) asks for --
// not math.log(x)/math.log(2), which is what every other base gets.
//
// It cannot simply ask math.Log2.  Go's Log2 splits the argument with
// Frexp and adds the exponent back afterwards:
//
//	Log(frac) * (1/Ln2) + exp
//
// and both of those round.  When x is near 1 the product is near -1 and
// the exponent is 1, so the answer is the difference of two numbers a
// million times bigger than it is, and what survives of it is the
// rounding: math.log(1.0000000000000002, 2) is 3.203426503810071e-16 to
// C's log2 and 3.681659806145121e-16 to math.Log2, fifteen per cent
// away -- and every other argument in this port is within an ulp or two.
//
// In the band [sqrt(2)/2, sqrt(2)) -- the band in which Frexp's exponent
// is the 0 that math.Log would have to add back -- math.Log needs no
// such subtraction, so it keeps the low bits of an answer near zero, and
// log(x)/ln(2) is accurate there.  Outside it the answer is at least 0.5
// in magnitude, where the rounding above is worth less than an ulp and
// math.Log2 is the better of the two anyway: it is exact for a power of
// two, and its error does not grow with the exponent the way a
// quotient's does.
func mathLog2Scaled(x float64) float64 {
	if x > 0 && x < mathSmallestNormal {
		return mathLogScaled(x) * (1 / math.Ln2)
	}
	if x >= math.Sqrt2/2 && x < math.Sqrt2 {
		return math.Log(x) * (1 / math.Ln2)
	}
	return math.Log2(x)
}

func mathSinh(L *State) int {
	x := L.CheckNumber(1)
	if haveLibm {
		L.PushNumber(libmSinh(x))
		return 1
	}
	s := math.Sinh(x)
	if math.IsInf(s, 0) && !math.IsInf(x, 0) {
		/* exp(|x|) overflowed; exp(|x|) / 2 may not have */
		y := mathHalfExp(math.Abs(x))
		if !math.IsInf(y, 1) {
			if x < 0 {
				L.PushNumber(-y)
			} else {
				L.PushNumber(y)
			}
			return 1
		}
	}
	L.PushNumber(s)
	return 1
}

func mathCosh(L *State) int {
	x := L.CheckNumber(1)
	if haveLibm {
		L.PushNumber(libmCosh(x))
		return 1
	}
	s := math.Cosh(x)
	if math.IsInf(s, 0) && !math.IsInf(x, 0) {
		/* 'cosh' tends to the same value as 'sinh', and is never negative */
		if y := mathHalfExp(math.Abs(x)); !math.IsInf(y, 1) {
			L.PushNumber(y)
			return 1
		}
	}
	L.PushNumber(s)
	return 1
}

func mathTanh(L *State) int {
	L.PushNumber(libmTanh(L.CheckNumber(1)))
	return 1
}

func mathPow(L *State) int {
	x := L.CheckNumber(1)
	y := L.CheckNumber(2)
	L.PushNumber(libmPow(x, y))
	return 1
}

// mathFrexp is math.frexp (lmathlib.c).  It returns the fraction and
// the exponent, in that order.
func mathFrexp(L *State) int {
	f, e := math.Frexp(L.CheckNumber(1))
	L.PushNumber(f)
	L.PushInteger(int64(e))
	return 2
}

// mathLdexp is math.ldexp (lmathlib.c).  C truncates the exponent to
// an 'int', so a huge one wraps here as it does there.
func mathLdexp(L *State) int {
	x := L.CheckNumber(1)
	L.PushNumber(math.Ldexp(x, int(int32(L.CheckInteger(2)))))
	return 1
}

func mathLog10(L *State) int {
	L.PushNumber(mathLog10Of(L.CheckNumber(1)))
	return 1
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

var mathLib = []Reg{
	{Name: "abs", Func: mathAbs},
	{Name: "acos", Func: mathAcos},
	{Name: "asin", Func: mathAsin},
	{Name: "atan", Func: mathAtan},
	{Name: "ceil", Func: mathLibCeil},
	{Name: "cos", Func: mathCos},
	{Name: "deg", Func: mathDeg},
	{Name: "exp", Func: mathExp},
	{Name: "tointeger", Func: mathToInt},
	{Name: "floor", Func: mathLibFloor},
	{Name: "fmod", Func: mathFmod},
	{Name: "ult", Func: mathUlt},
	{Name: "log", Func: mathLog},
	{Name: "max", Func: mathMax},
	{Name: "min", Func: mathMin},
	{Name: "modf", Func: mathModf},
	{Name: "rad", Func: mathRad},
	{Name: "sin", Func: mathSin},
	{Name: "sqrt", Func: mathSqrt},
	{Name: "tan", Func: mathTan},
	{Name: "type", Func: mathType},
	/* the ones 'LUA_COMPAT_MATHLIB' adds (lmathlib.c) */
	{Name: "atan2", Func: mathAtan},
	{Name: "cosh", Func: mathCosh},
	{Name: "sinh", Func: mathSinh},
	{Name: "tanh", Func: mathTanh},
	{Name: "pow", Func: mathPow},
	{Name: "frexp", Func: mathFrexp},
	{Name: "ldexp", Func: mathLdexp},
	{Name: "log10", Func: mathLog10},
}

// OpenMath opens the math library (luaopen_math).
func OpenMath(L *State) int {
	L.NewLib(mathLib)
	L.PushNumber(mathPI)
	L.SetField(-2, "pi")
	L.PushNumber(math.Inf(1))
	L.SetField(-2, "huge")
	L.PushInteger(LUA_MAXINTEGER)
	L.SetField(-2, "maxinteger")
	L.PushInteger(LUA_MININTEGER)
	L.SetField(-2, "mininteger")
	mathSetRandFunc(L)
	return 1
}
