// Translation of lmathlib.c (PUC-Lua 5.4): the mathematical library.
//
// Every lua_CFunction becomes a func(*State) int; the xoshiro256**
// pseudo-random generator uses uint64 (the port always has 64-bit
// integers), so only the "standard implementation" branch of the C
// source is needed.
package lua

import (
	"math"
	"time"
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
	L.PushNumber(math.Sin(L.CheckNumber(1)))
	return 1
}

func mathCos(L *State) int {
	L.PushNumber(math.Cos(L.CheckNumber(1)))
	return 1
}

func mathTan(L *State) int {
	L.PushNumber(math.Tan(L.CheckNumber(1)))
	return 1
}

func mathAsin(L *State) int {
	L.PushNumber(math.Asin(L.CheckNumber(1)))
	return 1
}

func mathAcos(L *State) int {
	L.PushNumber(math.Acos(L.CheckNumber(1)))
	return 1
}

func mathAtan(L *State) int {
	y := L.CheckNumber(1)
	x := L.OptNumber(2, 1)
	L.PushNumber(math.Atan2(y, x))
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
		L.PushNumber(math.Mod(L.CheckNumber(1), L.CheckNumber(2)))
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
	L.PushNumber(math.Sqrt(L.CheckNumber(1)))
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
		res = math.Log(x)
	} else {
		base := L.CheckNumber(2)
		if base == 2.0 {
			res = math.Log2(x)
		} else if base == 10.0 {
			res = math.Log10(x)
		} else {
			res = math.Log(x) / math.Log(base)
		}
	}
	L.PushNumber(res)
	return 1
}

func mathExp(L *State) int {
	L.PushNumber(math.Exp(L.CheckNumber(1)))
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
// Pseudo-Random Number Generator based on 'xoshiro256**'
// ---------------------------------------------------------------------

// figs is the number of binary digits in the mantissa of a float64.
const mathFigs = 53

// mathRanState is the state of the generator: four 64-bit words
// (RanState in lmathlib.c).
type mathRanState struct {
	s [4]uint64
}

// mathRotl rotates left 'x' by 'n' bits (the trim64 of the C code is
// implicit: uint64 already has exactly 64 bits).
func mathRotl(x uint64, n int) uint64 {
	return (x << n) | (x >> (64 - n))
}

func mathNextRand(state *[4]uint64) uint64 {
	state0 := state[0]
	state1 := state[1]
	state2 := state[2] ^ state0
	state3 := state[3] ^ state1
	res := mathRotl(state1*5, 7) * 9
	state[0] = state0 ^ state3
	state[1] = state1 ^ state2
	state[2] = state2 ^ (state1 << 17)
	state[3] = mathRotl(state3, 45)
	return res
}

/*
** Convert bits from a random integer into a float in the interval
** [0,1), getting the higher 'mathFigs' bits from the random unsigned
** integer and converting that to a float.
 */
func mathI2d(x uint64) float64 {
	sx := int64(x >> (64 - mathFigs))
	res := float64(sx) * (0.5 / float64(uint64(1)<<(mathFigs-1)))
	if sx < 0 {
		res += 1.0 /* correct the two's complement if negative */
	}
	return res
}

/*
** Project the random integer 'ran' into the interval [0, n].
** Because 'ran' has 2^B possible values, the projection can only be
** uniform when the size of the interval is a power of 2 (exact
** division). Otherwise, to get a uniform projection into [0, n], we
** first compute 'lim', the smallest Mersenne number not smaller than
** 'n'. We then project 'ran' into the interval [0, lim].  If the result
** is inside [0, n], we are done. Otherwise, we try with another 'ran',
** until we have a result inside the interval.
 */
func mathProject(ran, n uint64, state *mathRanState) uint64 {
	if n&(n+1) == 0 { /* is 'n + 1' a power of 2? */
		return ran & n /* no bias */
	}
	lim := n
	/* compute the smallest (2^b - 1) not smaller than 'n' */
	lim |= lim >> 1
	lim |= lim >> 2
	lim |= lim >> 4
	lim |= lim >> 8
	lim |= lim >> 16
	lim |= lim >> 32
	for {
		ran &= lim
		if ran <= n { /* project 'ran' into [0..lim] */
			return ran
		}
		ran = mathNextRand(&state.s) /* not inside [0..n]? try again */
	}
}

func mathRandom(L *State) int {
	var low, up int64
	state, _ := L.ToUserData(UpvalueIndex(1)).(*mathRanState)
	rv := mathNextRand(&state.s) /* next pseudo-random value */
	switch L.GetTop() {          /* check number of arguments */
	case 0: /* no arguments */
		L.PushNumber(mathI2d(rv)) /* float between 0 and 1 */
		return 1
	case 1: /* only upper limit */
		low = 1
		up = L.CheckInteger(1)
		if up == 0 { /* single 0 as argument? */
			L.PushInteger(int64(rv)) /* full random integer */
			return 1
		}
	case 2: /* lower and upper limits */
		low = L.CheckInteger(1)
		up = L.CheckInteger(2)
	default:
		return L.Error2("wrong number of arguments")
	}
	/* random integer in the interval [low, up] */
	L.ArgCheck(low <= up, 1, "interval is empty")
	/* project random integer into the interval [0, up - low] */
	p := mathProject(rv, uint64(up)-uint64(low), state)
	L.PushInteger(int64(p + uint64(low)))
	return 1
}

func mathSetSeed(L *State, state *[4]uint64, n1, n2 uint64) {
	state[0] = n1
	state[1] = 0xff /* avoid a zero state */
	state[2] = n2
	state[3] = 0
	for i := 0; i < 16; i++ {
		mathNextRand(state) /* discard initial values to "spread" seed */
	}
	L.PushInteger(int64(n1))
	L.PushInteger(int64(n2))
}

/*
** Set a "random" seed. To get some randomness, use the current time
** and the address of 'L' (here, the identity of the state object).
 */
func mathRandSeed(L *State, state *mathRanState) {
	seed1 := uint64(time.Now().Unix())
	seed2 := uint64(ptrID(L))
	mathSetSeed(L, &state.s, seed1, seed2)
}

func mathRandomSeed(L *State) int {
	state, _ := L.ToUserData(UpvalueIndex(1)).(*mathRanState)
	if L.IsNone(1) {
		mathRandSeed(L, state)
	} else {
		n1 := L.CheckInteger(1)
		n2 := L.OptInteger(2, 0)
		mathSetSeed(L, &state.s, uint64(n1), uint64(n2))
	}
	return 2 /* return seeds */
}

var mathRandFuncs = []Reg{
	{Name: "random", Func: mathRandom},
	{Name: "randomseed", Func: mathRandomSeed},
}

/*
** Register the random functions and initialize their state.
 */
func mathSetRandFunc(L *State) {
	state := &mathRanState{}
	L.NewUserData(state, 0)
	mathRandSeed(L, state) /* initialize with a "random" seed */
	L.Pop(2)               /* remove pushed seeds */
	L.SetFuncs(mathRandFuncs, 1)
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
