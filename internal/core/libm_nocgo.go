//go:build !cgo

package core

import "math"

// Without cgo there is no libm to ask. The wrappers are Go's math
// package, and mathExpScaled / mathLogScaled cover the arguments where
// that package and libm disagree by more than a rounding.

const haveLibm = false

func libmSin(x float64) float64   { return math.Sin(x) }
func libmCos(x float64) float64   { return math.Cos(x) }
func libmTan(x float64) float64   { return math.Tan(x) }
func libmAsin(x float64) float64  { return math.Asin(x) }
func libmAcos(x float64) float64  { return math.Acos(x) }
func libmSinh(x float64) float64  { return math.Sinh(x) }
func libmCosh(x float64) float64  { return math.Cosh(x) }
func libmTanh(x float64) float64  { return math.Tanh(x) }
func libmExp(x float64) float64   { return math.Exp(x) }
func libmLog(x float64) float64   { return math.Log(x) }
func libmLog10(x float64) float64 { return math.Log10(x) }
func libmLog2(x float64) float64  { return math.Log2(x) }
func libmSqrt(x float64) float64  { return math.Sqrt(x) }
func libmFmod(x, y float64) float64 {
	return math.Mod(x, y)
}
func libmPow(x, y float64) float64 { return math.Pow(x, y) }
func libmAtan2(y, x float64) float64 {
	return math.Atan2(y, x)
}
