//go:build cgo

package core

// The math library's transcendental functions are libm's in PUC-Lua.
// Go's math package is a different implementation: the same expression
// can differ in the last bit, and on amd64 its exp and log are wrong
// past the ends of the normal range. A build with cgo asks the C
// library, which is the one the reference interpreter on this machine
// asks.

/*
#cgo !windows LDFLAGS: -lm
#include <math.h>
*/
import "C"

const haveLibm = true

func libmSin(x float64) float64   { return float64(C.sin(C.double(x))) }
func libmCos(x float64) float64   { return float64(C.cos(C.double(x))) }
func libmTan(x float64) float64   { return float64(C.tan(C.double(x))) }
func libmAsin(x float64) float64  { return float64(C.asin(C.double(x))) }
func libmAcos(x float64) float64  { return float64(C.acos(C.double(x))) }
func libmSinh(x float64) float64  { return float64(C.sinh(C.double(x))) }
func libmCosh(x float64) float64  { return float64(C.cosh(C.double(x))) }
func libmTanh(x float64) float64  { return float64(C.tanh(C.double(x))) }
func libmExp(x float64) float64   { return float64(C.exp(C.double(x))) }
func libmLog(x float64) float64   { return float64(C.log(C.double(x))) }
func libmLog10(x float64) float64 { return float64(C.log10(C.double(x))) }
func libmLog2(x float64) float64  { return float64(C.log2(C.double(x))) }
func libmSqrt(x float64) float64  { return float64(C.sqrt(C.double(x))) }
func libmFmod(x, y float64) float64 {
	return float64(C.fmod(C.double(x), C.double(y)))
}
func libmPow(x, y float64) float64 {
	return float64(C.pow(C.double(x), C.double(y)))
}
func libmAtan2(y, x float64) float64 {
	return float64(C.atan2(C.double(y), C.double(x)))
}
