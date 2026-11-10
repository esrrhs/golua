package core

import (
	"fmt"
	"testing"
)

// Exercise overlapping result moves, varargs, tail calls, and nil padding at
// multiple arities, rather than depending on one particular compiled chunk.
func TestResultMoveArities(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 16, 64, 180} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			L := NewState()
			defer L.Close()
			L.OpenLibs()
			src := fmt.Sprintf(`
    local n = %d
    local a = {}; for i = 1,n do a[i] = i end
    local function id(...) return ... end
    local function tail(...) return id(...) end
    local function capture(...) return select('#', ...), {...} end
    local count, values = capture(tail(table.unpack(a,1,n)))
    assert(count == n)
    for i = 1,n do assert(values[i] == i) end
    local x,y,z = id(17)
    assert(x == 17 and y == nil and z == nil)
    local function grow(depth, ...)
     if depth == 0 then return ... end
     local x = {grow(depth-1, ...)}
     return table.unpack(x,1,n)
    end
    count, values = capture(grow(40, table.unpack(a,1,n)))
    assert(count == n)
    for i=1,n do assert(values[i] == i) end
   `, n)
			if err := L.DoString(src); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShortKeysAcrossInternFlushAndResize(t *testing.T) {
	L := NewState()
	defer L.Close()
	tab := luaH_new(L)
	keys := make([]*TString, 2000)
	for i := range keys {
		keys[i] = luaS_new(L, fmt.Sprintf("field-%d", i))
		var k, v Value
		setsvalue(&k, keys[i])
		setivalue(&v, int64(i))
		luaH_setval(L, tab, &k, &v)
	}
	// After flushing, pointer identity cannot establish equal-key lookups.
	L.g.strt.flush()
	for i, old := range keys {
		key := luaS_new(L, old.val)
		if key == old {
			t.Fatal("expected a fresh interned object")
		}
		got := luaH_getshortstr(tab, key)
		if !ttisinteger(got) || ivalue(got) != int64(i) {
			t.Fatalf("key %d lost", i)
		}
	}
	// Resize/rechain with old and new interned identities both reachable.
	luaH_resize(L, tab, 0, 8192)
	for i, key := range keys {
		got := luaH_getshortstr(tab, key)
		if !ttisinteger(got) || ivalue(got) != int64(i) {
			t.Fatalf("resized key %d lost", i)
		}
	}
}

func BenchmarkMultiReturnVararg(b *testing.B) {
	for _, n := range []int{3, 16, 64} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			L := benchPrepare(b, fmt.Sprintf(`
    values = {}; for i=1,%d do values[i]=i end
    function relay(...) return ... end
   `, n), `
    local function take(a,b,c) return a+b+c end
    local s=0
    for i=1,10000 do s=s+take(relay(table.unpack(values))) end
    assert(s==60000)
   `)
			defer L.Close()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchCall(b, L)
			}
		})
	}
}

// Result sources and destinations share a stack, including partial overlap.
func TestMoveResultsOverlap(t *testing.T) {
	for _, nres := range []int{0, 1, 2, 3, 16, 64} {
		for _, wanted := range []int{LUA_MULTRET, 0, 1, 2, 3, 16, 64, 80} {
			for _, gap := range []int{0, 1, 2, 8, 64} {
				L := NewState()
				L.checkStack(256)
				res, first := 2, 2+gap
				for i := range L.stack {
					setivalue(&L.stack[i], int64(i+100))
				}
				L.top = first + nres
				count := wanted
				if count == LUA_MULTRET {
					count = nres
				}
				moveresults(L, first, res, nres, wanted)
				if L.top != res+count {
					t.Fatalf("nres=%d wanted=%d gap=%d: top=%d", nres, wanted, gap, L.top)
				}
				for i := 0; i < count; i++ {
					v := &L.stack[res+i]
					if i < nres {
						if !ttisinteger(v) || ivalue(v) != int64(first+i+100) {
							t.Fatalf("nres=%d wanted=%d gap=%d result=%d", nres, wanted, gap, i)
						}
					} else if !ttisnil(v) {
						t.Fatalf("result %d not nil", i)
					}
				}
				L.Close()
			}
		}
	}
}
