// Small math wrappers used by the translated runtime.
package core

import "math"

func math2Trunc(n float64) float64        { return math.Trunc(n) }
func math2Floor(n float64) float64        { return math.Floor(n) }
func math2Ceil(n float64) float64         { return math.Ceil(n) }
func math2IsNaN(n float64) bool           { return math.IsNaN(n) }
func math2IsInf(n float64, sign int) bool { return math.IsInf(n, sign) }
func math2Abs(n float64) float64          { return math.Abs(n) }
func math2Fmod(x, y float64) float64      { return libmFmod(x, y) }
func math2Pow(x, y float64) float64       { return libmPow(x, y) }
func math2Sqrt(x float64) float64         { return libmSqrt(x) }

