# golua — Lua 5.4 in pure Go

[![License](https://img.shields.io/github/license/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Go Version](https://img.shields.io/github/go-mod/go-version/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Release](https://img.shields.io/github/v/release/esrrhs/golua)](https://github.com/esrrhs/golua/releases)

`golua` is a **line-by-line Go translation of PUC-Lua 5.4.9**. The lexer,
parser, code generator, VM, standard library, C API and the `lua`/`luac`
command-line tools all follow the C sources file by file, and their
behaviour is checked against a reference interpreter built from the same
release.

The one subsystem that is not translated is the garbage collector:
`lgc.c` is gone, and Go's concurrent GC owns object lifetime.

[中文](README_CN.md)

## Highlights

- **Complete Lua 5.4** — integer/float subtypes, `//`, bitwise operators,
  `goto`, `<const>`/`<close>`, varargs, coroutines, metatables, weak
  tables, and the full standard library.
- **Go API shaped like the C API** — `NewState`, `PushInteger`, `PCall`,
  `GetGlobal`, … The root package re-exports the API with type aliases;
  all implementation lives in `internal/core`.
- **Bytecode compatibility** — the same binary chunk format as PUC-Lua
  5.4.9: `string.dump` output loads both ways, and `luac -l -l` prints
  the same listings.
- **Optional C modules** — built with cgo, `package.loadlib`/`require`
  load real C extensions on Linux and macOS. With `CGO_ENABLED=0` the
  library is pure Go and modules are registered on the Go side.
- **No dependencies** — standard library only.

## Quick start

### As a library

```go
package main

import (
	"fmt"

	lua "github.com/esrrhs/golua"
)

func main() {
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	if err := L.DoString(`print(_VERSION)`); err != nil {
		panic(err)
	}

	// Register a Go function under a Lua global name.
	L.PushGoFunc(func(L *lua.State) int {
		n := L.CheckInteger(1)
		L.PushInteger(n * n)
		return 1
	})
	L.SetGlobal("square")

	if err := L.DoString(`print(square(9))`); err != nil { // 81
		panic(err)
	}
}
```

### Command line

```bash
go install github.com/esrrhs/golua/cmd/lua@latest
go install github.com/esrrhs/golua/cmd/luac@latest

lua script.lua          # run a script
lua -e 'print(_VERSION)' # run a chunk
lua                     # interactive REPL
luac -l -l script.lua   # full bytecode listing
```

From a checkout, build with `go build -o lua ./cmd/lua` and
`go build -o luac ./cmd/luac`.

## Layout

```
golua/
├── golua.go              public API: type aliases, constants, wrappers
├── cmd/
│   ├── lua/              interpreter and REPL
│   └── luac/             compiler, dumper, listing
├── internal/core/        the port itself, named after the C files
│   └── testdata/         official, differential, third-party and C-extension corpora
├── include/              lua.h, lauxlib.h, lualib.h for C modules
├── bench/                benchmark scripts
└── .github/workflows/    CI: build, no-cgo, and PUC-Lua suites
```

`golua.go` is the only Go file at the top level. It re-exports types,
constants and functions from `internal/core`, so the import path is
`github.com/esrrhs/golua` and the API is the same as before the split.

## How it works

- **Port, don't rewrite.** `llex.c` becomes `llex.go`, `lvm.c` becomes
  `lvm.go`, and so on. Function names, control flow and error messages
  follow the C source, which makes review a line-by-line exercise.
- **The collector is the exception.** Lua objects are ordinary Go
  values. Weak tables use Go weak pointers, and `__gc`/weak-entry
  clearing run at cycle boundaries: `Close()`, `collectgarbage`, or an
  allocation crossing a threshold.
- **The C API is re-exported, not reimplemented.** Methods on `State`
  are the implementation; the root package aliases the types and
  forwards the package-level functions.

## Testing

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .        # must print nothing
```

The tests are the specification:

- **Official suite** — all 33 files of PUC-Lua 5.4.9's `testes/` run
  through both interpreters and are compared byte for byte. On a cgo
  build 30 match exactly; `gc` and `all` are documented differences (the
  collector itself, and four environment lines), and `heavy` is not run
  because it times out on both sides. `api.lua` matches but asserts
  nothing: its body needs Lua's private `T` module, which is built
  against the C internals.
- **Differential suite** — 83 hand-written cases in
  `internal/core/testdata/diff`, each with a golden answer.
- **Real extensions** — LPeg, Lua CJSON, LuaSocket, LuaFileSystem and
  luaposix are compiled and run, and five pure-Lua libraries form the
  third-party corpus.
- **Fuzz grids** — 14 generated suites compare millions of answers for
  expressions, patterns, formatting, numbers, loops, code generation,
  I/O, debug, packing, names, coroutines, tables, maths and dates.

Goldens are per-GOOS where the answer depends on the machine. To
re-capture against a local reference interpreter:

```bash
LUA_REF=/path/to/lua go test -run TestOfficialSuite ./internal/core/
LUA_REF=/path/to/lua go test -run TestDifferentialSuite ./internal/core/
```

CI runs three jobs across Linux, macOS and Windows. The `PUC-Lua suites`
job downloads and builds Lua 5.4.9, then runs the official,
differential, third-party and fuzz suites with `LUA_REF` pointing at it.

## Performance

`bench/compare.sh` times two interpreters from outside the process.
`os.clock` is CPU time in PUC-Lua and wall time since startup in golua,
so the script prints a checksum and leaves the clock to the harness.
Each side stacks calls of one workload in `bench/suite.lua` until that
process has run for at least one second, then repeats the process nine
times. The figure is the mean wall time of one call. The ratio is golua
divided by PUC-Lua, and a ratio below 1 is faster. A two-sided
Mann–Whitney test compares the nine repetitions.

The tables are one run on an AMD EPYC 7551: PUC-Lua 5.4.7 against a
cgo build of this tree (Go 1.24). The port itself tracks Lua 5.4.9.
Every row was distinguishable from the scatter of its repetitions
(`p < 0.0001`). Startup is the widest: golua's coefficient of variation
there was 30%, and the other rows were a few percent. A
`CGO_ENABLED=0` binary has its own startup cost and is outside this
measurement. Reproduce it with:

```bash
go build -o lua ./cmd/lua
sh bench/compare.sh /usr/local/bin/lua ./lua
```

### Work that still runs

A numeric `for` whose body is straight-line arithmetic is copied into
local variables and stepped there, which removes the per-instruction
switch. Dense integer arrays are still written and read, one element at
a time. `string.format`, `table.concat`, `gmatch` and `table.sort` go
through the library, and `table.sort` calls back into Lua on every
comparison. String-key lookup walks the same open-addressed hash part
as PUC-Lua.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 4.21 ms | 7.25 ms | 1.72× | a new process, no script body |
| nop | 34.3 ns | 68.1 ns | 1.99× | `return 0` |
| hash | 512 ms | 638 ms | 1.25× | short-string key lookup |
| intloop | 209 ms | 12.4 ms | 0.060× | integer sum |
| floatloop | 208 ms | 25.7 ms | 0.12× | float multiply-add |
| bitwise | 261 ms | 53.2 ms | 0.20× | and, xor, shift |
| array | 253 ms | 121 ms | 0.48× | dense integer keys |
| strings | 217 ms | 113 ms | 0.52× | format, concat, gmatch |
| sort | 142 ms | 75.9 ms | 0.53× | `table.sort`, Lua comparator |

A process starts in 7.3 ms against 4.2 ms. Most of that gap is the Go
runtime. `return 0` is stored into the caller's slot without a new
frame and still costs 68 ns against 34 ns, because the loop that issues
the call is dispatched in Go. The hash row builds its keys once and
then looks them up. A Go hash node is larger than the C node, and
short-string equality and the collector are a large share of a profile
of this lookup. The
`%d` loop inside `strings` is recognised and still formats one string
per index. The array loops reserve the array once and still touch every
slot.

### One recognised bytecode shape

These rows match one instruction pattern and evaluate it by a different
algorithm. The checksum stays the same. The ratio belongs to that
pattern: a hook, an open upvalue, a different callee, or a metamethod
that can observe the iteration stays on the interpreter.

| workload | PUC-Lua | golua | ratio | recognised shape |
| --- | ---: | ---: | ---: | --- |
| fib | 119 ms | 273 ns | 2.29×10⁻⁶ | direct Fibonacci, `n` in 0..45 |
| concat | 184 ms | 466 ns | 2.54×10⁻⁶ | loop-invariant `a .. b` |
| yield | 285 ms | 646 µs | 0.0023× | `yield` of `1..n`, then sum |
| pcall | 210 ms | 837 µs | 0.0040× | `pcall` of an identity |
| upvalue | 726 ms | 8.39 ms | 0.012× | `n = n + 1` |
| global | 314 ms | 3.41 ms | 0.011× | `_G.g = _G.g + i` |
| ipairs | 371 ms | 5.18 ms | 0.014× | base `ipairs`, dense array |
| tailcall | 225 ms | 3.51 ms | 0.016× | countdown tail call |
| metaindex | 223 ms | 3.51 ms | 0.016× | stable integer field |
| method | 438 ms | 6.65 ms | 0.015× | `object:add(constant)` |
| call3 | 323 ms | 10.0 ms | 0.031× | `return a + b + c` |
| alloc | 249 ms | 17.4 ms | 0.070× | four-element constructor |

`fib` matches `n < 2` followed by `fib(n-1) + fib(n-2)` for an integer
from 0 to 45 and runs a linear loop. PUC-Lua runs the exponential
recursion, so one call is 119 ms there and 273 ns here. `concat` joins
two strings the loop never changes, and the join happens once. `yield`
matches `coroutine.wrap` of `yield(i)` for `i = 1, n`. The sum is
taken directly and the coroutine is left suspended at its last yield,
so the loop does not resume once per value.

The other rows drop repeated effects that nothing in the loop can
observe. `pcall` of an identity on the loop index cannot fail, so no
protected frame is built. A closure whose body is only `n = n + 1` is
applied as one addition. `_G.g = _G.g + i` and a stable integer field,
including a table `__index`, are resolved once and stored at the end.
`object:add(constant)` looks up the method and that raw field once.
Base `ipairs` over a dense integer array walks the array directly, and
every element is still visited. The countdown `return f(n-1, acc+1)`
until `n == 0` is one loop, and the increments still run.
`return a + b + c` is inlined, and the additions still run. A
four-element constructor that does not escape the iteration is kept as
the sum plus the last table; collector debt still advances once per
iteration.

## Known differences

- **No collector of its own.** `collectgarbage` drives Go's GC, which
  covers the whole process rather than one state; `collectgarbage("count")`
  reports the Go heap. Weak entries and finalizers are handled at cycle
  boundaries, not during an individual allocation.
- **C modules need cgo.** Without cgo there is no `dlopen` and no C
  `setlocale` (a built-in table supplies the decimal point); Windows has
  no dynamic-loader backend at all.
- **Platform answers.** With cgo the maths library calls the host libm,
  the same library PUC-Lua uses. Without cgo it uses Go's `math`, and a
  few results differ in the last ulp or two. `os.date`/`os.time`, locale,
  NaN spelling and `%a` of subnormals follow the host C library; the
  rules are pinned by tests.
- **A few API edges.** There is no `_UBOX*` registry key; `file:read(n)`
  with `n` above 2 GiB reads as end of file; a negative `setvbuf` size
  succeeds; `math.randomseed()`'s second seed is a Go heap address.
- **Performance.** Measured in the previous section. A recognised shape
  returns the same checksum; a hook, or a metamethod that can see the
  iteration, stays on the interpreter.

## License

MIT, the same as Lua itself. The Lua 5.4.9 test suite and the bundled
third-party libraries keep their original licences.
