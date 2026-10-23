//go:build !cgo

package core

import (
	"math"
	"testing"
)

func TestCoverGapMathNocgoCoshSinh(t *testing.T) {
	if haveLibm {
		t.Fatal("expected !haveLibm")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	// Finite values where Sinh/Cosh overflow to Inf in Go math.
	candidates := []float64{700, 710, 720, 740, 800, 900, 1000, 1e308}
	for _, x := range candidates {
		for _, name := range []string{"cosh", "sinh"} {
			L.SetTop(0)
			L.GetGlobal("math")
			L.GetField(-1, name)
			L.PushNumber(x)
			if L.PCall(1, 1, 0) != LUA_OK {
				t.Fatal(name, x, L.ToStringX(-1))
			}
		}
		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, "sinh")
		L.PushNumber(-x)
		if L.PCall(1, 1, 0) != LUA_OK {
			t.Fatal("sinh neg", x, L.ToStringX(-1))
		}
	}
	// Inf inputs (salvage condition false)
	for _, name := range []string{"cosh", "sinh"} {
		L.SetTop(0)
		L.GetGlobal("math")
		L.GetField(-1, name)
		L.PushNumber(math.Inf(1))
		_ = L.PCall(1, 1, 0)
	}
	// Direct Go entry
	L.SetTop(0)
	L.PushNumber(710)
	_ = mathCosh(L)
	L.SetTop(0)
	L.PushNumber(710)
	_ = mathSinh(L)
	L.SetTop(0)
	L.PushNumber(-710)
	_ = mathSinh(L)
	L.SetTop(0)
	L.PushNumber(1.0) // no overflow
	_ = mathCosh(L)
	_ = mathSinh(L)
}
