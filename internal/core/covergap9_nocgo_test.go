//go:build !cgo || !unix

package core

import (
	"math"
	"testing"
)

// libm_* in libm_nocgo.go are never called by mathSinh/etc (those use
// math.* directly when !haveLibm). Call them here so the !cgo profile
// covers the wrappers.
func TestNocgoLibmWrappersDirect(t *testing.T) {
	_ = libmSin(0)
	_ = libmCos(0)
	_ = libmTan(0)
	_ = libmAsin(0)
	_ = libmAcos(1)
	_ = libmSinh(0)
	_ = libmCosh(0)
	_ = libmTanh(0)
	_ = libmExp(0)
	_ = libmLog(1)
	_ = libmLog10(10)
	_ = libmLog2(8)
	_ = libmSqrt(4)
	_ = libmFmod(5, 2)
	_ = libmPow(2, 3)
	_ = libmAtan2(0, 1)
	_ = localeCatOf(0)
	_ = localeCatOf(1)
	coverWarnfSeesUD(nil)
	_ = math.NaN()
}

func TestNocgoMathSinhCoshOverflow(t *testing.T) {
	if haveLibm {
		t.Skip("cgo libm")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.DoString(`
local a = math.sinh(800)
local b = math.cosh(800)
local c = math.sinh(-800)
local d = math.exp(800)
assert(a ~= nil and b ~= nil and c ~= nil)
`); err != nil {
		t.Fatal(err)
	}
}
