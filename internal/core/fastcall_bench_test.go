//go:build cgo && unix && amd64

package core

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/esrrhs/golua/internal/fastcall"
)

func BenchmarkFFIEmptyCgo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if ffiEmptyC() != 42 {
			b.Fatal("bad")
		}
	}
}

func BenchmarkFFIEmptyPurego(b *testing.B) {
	fn := ffiEmptyFn()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r1, _, _ := purego.SyscallN(fn)
		if int(r1) != 42 {
			b.Fatal("bad")
		}
	}
}

func BenchmarkFFIEmptyFastCall(b *testing.B) {
	fn := ffiEmptyFn()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if int(fastcall.Call0(fn)) != 42 {
			b.Fatal("bad")
		}
	}
}

func BenchmarkFFIAdd1Cgo(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if ffiAdd1C(i) != i+1 {
			b.Fatal("bad")
		}
	}
}

func BenchmarkFFIAdd1Purego(b *testing.B) {
	fn := ffiAdd1Fn()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r1, _, _ := purego.SyscallN(fn, uintptr(i))
		if int(r1) != i+1 {
			b.Fatal("bad")
		}
	}
}

func BenchmarkFFIAdd1FastCall(b *testing.B) {
	fn := ffiAdd1Fn()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if int(fastcall.Call1(fn, uintptr(i))) != i+1 {
			b.Fatal("bad")
		}
	}
}

func TestFastCallDirect(t *testing.T) {
	fn := ffiEmptyFn()
	if got := int(fastcall.Call0(fn)); got != 42 {
		t.Fatalf("Call0: got %d", got)
	}
	fn1 := ffiAdd1Fn()
	if got := int(fastcall.Call1(fn1, 41)); got != 42 {
		t.Fatalf("Call1: got %d", got)
	}
}
