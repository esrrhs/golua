package goid

import (
	"runtime"
	"sync"
	"testing"
)

func TestGetNonZero(t *testing.T) {
	if id := Get(); id == 0 {
		t.Fatal("Get() returned 0")
	}
}

//go:noinline
func growStack(depth int) uintptr {
	var pad [1024]byte
	pad[0] = byte(depth)
	if depth <= 0 {
		return Get() ^ uintptr(pad[0])
	}
	return growStack(depth - 1)
}

func TestGetSameGoroutineStable(t *testing.T) {
	id0 := Get()
	for i := 0; i < 100; i++ {
		if got := Get(); got != id0 {
			t.Fatalf("iteration %d: Get() = %#x, want %#x", i, got, id0)
		}
		runtime.Gosched()
	}
	if got := growStack(64); got != id0 {
		t.Fatalf("after stack growth: Get() = %#x, want %#x", got, id0)
	}
}

func TestGetDifferentGoroutines(t *testing.T) {
	const n = 64
	ids := make([]uintptr, n)
	ready := make(chan struct{}, n)
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			ids[idx] = Get()
			ready <- struct{}{}
			<-release
		}(i)
	}
	for i := 0; i < n; i++ {
		<-ready
	}
	mainID := Get()
	close(release)
	wg.Wait()

	seen := map[uintptr]int{mainID: -1}
	for i, id := range ids {
		if id == 0 {
			t.Fatalf("goroutine %d: Get() returned 0", i)
		}
		if prev, dup := seen[id]; dup {
			t.Fatalf("goroutine %d shared id %#x with %d", i, id, prev)
		}
		seen[id] = i
	}
}

func BenchmarkGet(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Get()
	}
}
