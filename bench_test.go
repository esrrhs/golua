package lua

// Benchmarks for the interpreter.  They are here so that a change to the
// VM, the table code or the string library can be measured against a
// baseline instead of guessed at, and so that the numbers quoted in the
// README can be reproduced:
//
//	go test -bench=. -benchmem -run=XXX
//
// Every benchmark compiles its chunk once (outside the timed loop) and
// then calls it repeatedly, so that the parser is not measured together
// with the VM -- except in BenchmarkCompile, which is about the parser.
import (
	"testing"
)

// benchPrepare opens a state, runs 'setup' once and leaves 'chunk'
// compiled on the stack, ready to be called.
func benchPrepare(b *testing.B, setup, chunk string) *State {
	b.Helper()
	L := NewState()
	L.OpenLibs()
	if setup != "" {
		if err := L.DoString(setup); err != nil {
			b.Fatalf("setup: %v", err)
		}
	}
	if err := L.LoadString(chunk, "=bench"); err != nil {
		b.Fatalf("load: %v", err)
	}
	return L
}

// benchCall runs the chunk left on the stack by 'benchPrepare' once.
func benchCall(b *testing.B, L *State) {
	b.Helper()
	L.PushValue(-1) /* the compiled chunk */
	if status := L.PCall(0, 0, 0); status != LUA_OK {
		s, _ := L.ToString(-1)
		b.Fatalf("chunk failed: %s", s)
	}
}

func BenchmarkFib(b *testing.B) {
	L := benchPrepare(b, `
		function fib(n)
			if n < 2 then return n end
			return fib(n - 1) + fib(n - 2)
		end
		N = 24
	`, `local x = fib(N)`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkArithLoop(b *testing.B) {
	L := benchPrepare(b, "", `
		local s = 0
		for i = 1, 1000000 do s = s + i * i % 7 end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkStringKeyTable(b *testing.B) {
	L := benchPrepare(b, "", `
		local t = {}
		for i = 1, 100000 do t["key" .. i] = i end
		local s = 0
		for i = 1, 100000 do s = s + t["key" .. i] end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkIntegerKeyTable(b *testing.B) {
	L := benchPrepare(b, "", `
		local t = {}
		for i = 1, 200000 do t[i] = i end
		local s = 0
		for i = 1, 200000 do s = s + t[i] end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkStringOps(b *testing.B) {
	L := benchPrepare(b, "", `
		local s = ""
		for i = 1, 20000 do s = string.format("%s%d;", s, i) end
		local n = 0
		for w in string.gmatch(s, "%d+") do n = n + 1 end
		string.rep("x", 100000)
		string.upper(s:sub(1, 1000))
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkTableSort(b *testing.B) {
	L := benchPrepare(b, `
		t = {}
		for i = 1, 20000 do t[i] = (i * 2654435761) % 100003 end
	`, `table.sort(t)`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkClosureUpvalues(b *testing.B) {
	L := benchPrepare(b, "", `
		local function make()
			local n = 0
			return function() n = n + 1; return n end
		end
		local f = make()
		for i = 1, 500000 do f() end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkCoroutine(b *testing.B) {
	L := benchPrepare(b, "", `
		local co = coroutine.wrap(function()
			for i = 1, 200000 do coroutine.yield(i) end
		end)
		for i = 1, 200000 do co() end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

func BenchmarkPCall(b *testing.B) {
	L := benchPrepare(b, "", `
		for i = 1, 100000 do pcall(function() return i end) end
	`)
	defer L.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkGoCall measures the cost of crossing from Lua into Go, which
// is the operation a host program pays on every registered function.
func BenchmarkGoCall(b *testing.B) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushGoFunc(func(L *State) int {
		n, _ := L.ToInteger(1)
		L.PushInteger(n + 1)
		return 1
	})
	L.SetGlobal("gofunc")
	if err := L.LoadString(`
		local s = 0
		for i = 1, 200000 do s = gofunc(s) end
	`, "=bench"); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchCall(b, L)
	}
}

// BenchmarkCompile measures the front end: lexer, parser and code
// generator, with no execution at all.
func BenchmarkCompile(b *testing.B) {
	src := `
		local function f(a, b)
			local t = {}
			for i = 1, 100 do t[i] = a * i + b end
			return t
		end
		local x = 0
		for i = 1, 100 do x = x + #f(i, i * 2) end
		return x
	`
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := L.LoadString(src, "=bench"); err != nil {
			b.Fatal(err)
		}
		L.Pop(1)
	}
}
