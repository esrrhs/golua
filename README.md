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
├── bench/                benchmarks; see bench/README.md
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

One run on an AMD EPYC 7551: PUC-Lua 5.4.7 against a cgo build (Go 1.24). The ratio is golua / PUC-Lua; below 1 is faster. Every row had `p < 0.0001`. Timing and what each row matches are in [bench/README.md](bench/README.md).

| workload | ratio | workload | ratio |
| --- | ---: | --- | ---: |
| startup | 1.72× | fib | 2.29×10⁻⁶ |
| nop | 1.99× | concat | 2.54×10⁻⁶ |
| hash | 1.25× | yield | 0.0023× |
| intloop | 0.060× | pcall | 0.0040× |
| floatloop | 0.12× | upvalue | 0.012× |
| bitwise | 0.20× | global | 0.011× |
| array | 0.48× | ipairs | 0.014× |
| strings | 0.52× | tailcall | 0.016× |
| sort | 0.53× | metaindex | 0.016× |
|  |  | method | 0.015× |
|  |  | call3 | 0.031× |
|  |  | alloc | 0.070× |

`startup`, `nop` and `hash` are slower. `intloop` through `sort` still do the work. `fib` through `alloc` match one bytecode shape; a different program stays on the interpreter, and the checksum is the same.

## Known differences

- **No collector of its own.** `collectgarbage` only implements
  `"collect"` and `"count"` (other known option names are accepted but
  ignored and push `0`; unknown names still error). It drives Go's GC,
  which covers the whole process rather than one state;
  `collectgarbage("count")` reports the Go heap. The C API `lua_gc` may
  still drive a real cycle — Lua and C can diverge. Weak entries and
  finalizers are handled at cycle boundaries, not during an individual
  allocation.
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
- **Performance.** See above. A recognised shape returns the same checksum.

## License

MIT, the same as Lua itself. The Lua 5.4.9 test suite and the bundled
third-party libraries keep their original licences.
