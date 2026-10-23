//go:build !cgo

package core

import "testing"

// Cover !cgo stub wrappers that mathCosh/Sinh bypass when !haveLibm.
func TestCoverGapNocgoLibmStubs(t *testing.T) {
	_ = libmSin(0)
	_ = libmCos(0)
	_ = libmTan(0)
	_ = libmAsin(0)
	_ = libmAcos(0)
	_ = libmSinh(0)
	_ = libmCosh(0)
	_ = libmTanh(0)
	_ = libmExp(0)
	_ = libmLog(1)
	_ = libmLog10(1)
	_ = libmLog2(1)
	_ = libmSqrt(4)
	_ = libmFmod(5, 2)
	_ = libmPow(2, 3)
	_ = libmAtan2(1, 1)
	capiResetChecks()
	_ = capiHasChecks()
	_ = exerciseCAPI(t.TempDir())
	_ = localeCatOf(0)
}
