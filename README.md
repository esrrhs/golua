# fakescript_go → Lua 5.4 in pure Go (line-by-line port of PUC-Lua, GC removed)

[![License](https://img.shields.io/github/license/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Language](https://img.shields.io/github/languages/top/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Release](https://img.shields.io/github/v/release/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go/releases)

> **This repository is being rewritten.** It used to host *fakescript*, a self-designed scripting
> language. It is now a **pure-Go implementation of Lua 5.4**, produced by **translating the
> PUC-Lua 5.4.9 C sources file by file into Go**. The only substantial deletion is
> **`lgc.c` (1748 lines of garbage collector)** — object lifetime is entirely delegated to Go's
> concurrent GC.
>
> [中文](./README_CN.md)

---

## 1. Goals

| # | Goal | Detail |
|---|------|---|
| G1 | **Existing Lua scripts run unmodified** | `.lua` business code and pure-Lua third-party libraries load and run as-is |
| G2 | **Full Lua 5.4 language** | integer/float subtypes, `//`, bitwise ops, `goto`/labels, `<const>`/`<close>`, varargs, coroutines, metatables and all metamethods, weak tables, to-be-closed |
| G3 | **Complete standard library** | base, string (full pattern matching), table, math, io, os, coroutine, utf8, debug, package |
| G4 | **API mirrors the C API** | `lua_*` / `luaL_*` semantics preserved, so cgo-based hosts port with minimal edits |
| G5 | **Zero deps, no cgo, pure Go** | stdlib only; trivial cross-compilation and static linking on Linux/macOS/Windows |

**Non-goals:** ABI byte-compatibility with `liblua` (that would be a separate c-shared layer);
loading C dynamic libraries (replaced by Go-side module registration).

---

## 2. Strategy: port, don't rewrite

### 2.1 Three rules

1. **One-to-one file mapping** — `lvm.c → lvm.go`, `ltable.c → ltable.go`, `lstrlib.c → lstrlib.go`, so every line can be reviewed against the C source.
2. **One-to-one functions and control flow** — `luaV_execute` stays `luaV_execute`; no opportunistic "cleanups" or re-abstractions.
3. **Rewrite in exactly three places:**
   - **GC** — `lgc.c` deleted; all `luaC_*` gone (see §3).
   - **Errors** — `setjmp/longjmp` → Go `panic/recover` (`luaD_throw` = panic, `luaD_rawrunprotected` = recover).
   - **Allocation** — `lua_Alloc` / `luaM_*` / `lua_newstate(f,ud)` → plain Go heap, `NewState()`.

### 2.2 Source map

| C file | Go file | Status | Note |
|---|---|---|---|
| `lobject.h/.c` | `lobject.go` | ⬜ | `TValue`, `Table`, `Proto`, arithmetic/compare primitives |
| `lstate.h/.c` | `lstate.go` | ⬜ | `lua_State` / `global_State` / `CallInfo`, GC fields removed |
| `lstring.c` | `lstring.go` | ⬜ | string interning (Go map; runtime short strings not interned to avoid leaks) |
| `ltable.c` | `ltable.go` | ⬜ | array part + open-addressing hash; `luaH_next` needs stable order, cannot use Go map |
| `lfunc.c` | `lfunc.go` | ⬜ | `Proto` / `UpVal` / `CClosure` / `LClosure` |
| `ltm.c` | `ltm.go` | ⬜ | metamethod names, `luaT_gettm` |
| `lopcodes.h/.c` | `lopcodes.go` | ⬜ | opcode set, encode/decode |
| `ldo.c` | `ldo.go` | ⬜ | calls, protected calls, stack growth |
| `lvm.c` | `lvm.go` | ⬜ | `luaV_execute` main loop |
| `ldebug.c` | `ldebug.go` | ⬜ | line info, tracebacks, `luaG_*` messages |
| `lapi.c` | `lapi.go` | ⬜ | public `lua_*` API |
| `lauxlib.c` | `lauxlib.go` | ⬜ | `luaL_*` helpers |
| `llex.c` / `lparser.c` / `lcode.c` | same names `.go` | ⬜ | lexer, recursive-descent parser, codegen |
| `ldump.c` / `lundump.c` | same names `.go` | ⬜ | self-consistent binary format |
| `lbaselib` `lcorolib` `ltablib` `lmathlib` `lstrlib` `lutf8lib` `liolib` `loslib` `ldblib` `loadlib` `linit` | same names `.go` | ⬜ | standard libraries |
| `lua.c` | `cmd/lua/main.go` | ⬜ | CLI + REPL |
| **`lgc.c/.h`** | **(deleted)** | ✅ | **replaced by Go's concurrent GC** |

### 2.3 Type mapping

| C | Go |
|---|---|
| `TValue` (tagged union) | `type Value struct { tt int; i int64; n float64; p any }` |
| `GCObject *` / `CommonHeader` | removed; `*Table`, `*LClosure`, `*TString`, `*UserData`, `*State` |
| `lua_State *` | `*State` (a coroutine is another `*State`) |
| `StkId` | `int` stack index |
| `Instruction` (uint32) | `type Instruction struct{ Op, A, B, C int }` |
| `lua_Alloc` / `luaM_realloc` | gone (`make` / `append`) |
| `lua_CFunction` | `type GoFunc func(L *State) int` |
| `setjmp/longjmp` | `panic(*LuaError)` / `recover()` |

---

## 3. Removing the GC

| Lua side | C | Here |
|---|---|---|
| Object lifetime | tri-color incremental GC | Go's concurrent tri-color GC; cycles collected naturally |
| `luaC_barrier*` write barriers | manual | removed (Go has a hybrid write barrier) |
| `collectgarbage("collect")` | full GC | `runtime.GC()` |
| `collectgarbage("count")` | memory stats | `runtime.ReadMemStats()` |
| `collectgarbage("step"/"setpause"/"setstepmul"/…)` | tuning | compatible no-ops returning sane values |
| `__gc` | GC-driven finalization | `runtime.SetFinalizer` (timing not deterministic) |
| Weak tables (`__mode`) | GC clearing | `weak.Pointer` for table/userdata/function; **string keys are never weakened** |
| `lua_newstate(alloc, ud)` | custom allocator | `NewState()` |

---

## 4. Usage (target shape, available after P4)

```go
package main

import (
	"fmt"
	lua "github.com/esrrhs/fakescript_go"
)

func main() {
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	if err := L.DoString(`print("hello from lua", _VERSION)`); err != nil {
		panic(err)
	}

	_ = L.LoadString(`function add(a, b) return a + b end`, "=chunk")
	_ = L.PCall(0, 0, 0)
	L.GetGlobal("add")
	L.PushInteger(3)
	L.PushInteger(4)
	L.Call(2, 1)
	fmt.Println(L.ToInteger(-1)) // 7

	L.PushGoFunc(func(L *lua.State) int {
		n := L.CheckInteger(1)
		L.PushInteger(n * n)
		return 1
	})
	L.SetGlobal("square")
	_ = L.DoString(`print(square(9))`) // 81
}
```

```bash
go build -o lua ./cmd/lua
./lua script.lua   # run a script
./lua              # REPL
```

---

## 5. Roadmap

| Phase | Work | Status |
|---|---|---|
| **P0** | Docs; wipe old fakescript sources (kept in git history) | ✅ |
| **P1** | Core objects: `lobject` `lstate` `lstring` `ltable` `lfunc` `ltm` `lopcodes` | ✅ |
| **P2** | Execution engine: `ldo` `lvm` `ldebug` | ✅ |
| **P3** | Front end: `llex` `lparser` `lcode` | ✅ |
| **P4** | API layer: `lapi` `lauxlib` `linit` | ✅ |
| **P5** | Standard libraries | ✅ |
| **P6** | `cmd/lua` CLI + REPL, `ldump`/`lundump` | ✅ |
| **P7** | Official Lua 5.4 test suite, benchmarks, CI on 3 OSes | 🔄 |

### 5.1 Conformance: the official test suite

The 33 files of PUC-Lua 5.4.9's `testes/` are run through the reference
interpreter and through `./cmd/lua`, and the two outputs are normalised
(program path, `0x…` addresses, timings, `os.tmpname` results, PRNG seeds)
and compared. **`all.lua` is driven file by file**, because it needs the C
extension libraries pre-compiled; `main.lua` spawns the interpreter as a
subprocess and is therefore included.

**28 of the 31 comparable files produce byte-identical output.**
(`all.lua` needs the C extension libraries pre-compiled and `heavy.lua`
times out on both sides, so neither is part of the count.) The remaining
3 differ only for reasons that follow from the design — there is no
collector of our own — or from the test environment:

| File | Why it differs |
|---|---|
| `gc` | `collectgarbage("count")` accounting, and the granularity of `collectgarbage("step", n)`: PUC-Lua's step does a bounded amount of work, so `dosteps(10) < dosteps(2)`; Go's collector is all or nothing, so a step is a whole cycle and both take one |
| `literals` | `os.setlocale("pt_BR")` is unavailable here (see §6), so the decimal-point tests are skipped |
| `attrib` | needs the pre-compiled C extension (`package.loadlib`) |

Weak tables and `__gc` are supported and pass their own tests —
`closure`, `db`, `coroutine`, `gengc`, `cstack` and `locals`, the last
two of which count collections with `require"tracegc"` — see §6 for how
they are done.

The suite is also mirrored inside `go test`: `testdata/diff/*.lua` (23
cases) is compared byte for byte against golden files captured from
PUC-Lua 5.4.9, and `cli_test.go` pins the observable behaviour of the
interpreter itself — `-v`/`-e`/`-l`/`-E`/`-W`/`--`, the `arg` table, the
script varargs, the usage messages and the exit status.

### 5.2 Performance

`go test -bench=. -benchmem -run=XXX` runs the benchmarks in
`bench_test.go`. The table below is the same work measured end to end
through the two interpreters (Apple M1 Pro, PUC-Lua 5.4.9 built with
`-O2`), best of three runs:

| workload | PUC-Lua 5.4.9 | this port | ratio |
|---|---|---|---|
| `fib(26)`, recursive calls | 17.7 ms | 33.0 ms | 1.9× |
| arithmetic loop, 10⁷ iterations | 84.2 ms | 415.0 ms | 4.9× |
| integer-key table, 10⁶ insert+lookup | 25.1 ms | 71.1 ms | 2.8× |
| `table.sort`, 10⁵ elements | 38.4 ms | 58.4 ms | 1.5× |
| closure upvalue bump, 2×10⁶ calls | 51.5 ms | 128.3 ms | 2.5× |
| coroutine yield/resume, 2×10⁵ | 25.7 ms | 140.7 ms | 5.5× |
| `pcall` of a Lua function, 5×10⁵ | 60.8 ms | 80.3 ms | 1.3× |
| `string.gmatch` + `gsub` over 1 MB | 26.8 ms | 41.1 ms | 1.5× |
| `verybig.lua`: write a 120 KB chunk, then compile it | 110 ms | 250 ms | 2.3× |

Where the time goes: `Instruction` is a struct instead of a packed
uint32 (decoded on the fly), every `TValue` access goes through a tag
switch, and each coroutine runs on a goroutine, so a yield/resume pair is
a channel round trip rather than a swap of stack pointers.

---

## 6. Known differences from PUC-Lua 5.4.9

1. **Binary chunks are not compatible** — `string.dump`/`load` use this implementation's own format.
2. **No C dynamic library loading** — `package.cpath` / `package.loadlib` exist but report unsupported; port C extensions to Go modules (`RegisterModule` / `package.preload`).
3. **The collector is Go's, so its observable timing is not PUC-Lua's** — `collectgarbage("count")` reports the Go heap rather than PUC-Lua's own allocator, and `__gc` runs where this port runs a cycle (a collector step, `collectgarbage`, or `Close()`) rather than where PUC-Lua's step would have run it. Objects with a `__gc` are held between cycles so that Go cannot finalise them at a moment of its own choosing, and the metamethod of every object that still has one runs when the state closes, reachable or not — which is what C does and what makes `<close>`, `Close()` and weak tables deterministic: a sweep removes an entry once nothing but the table refers to it.
4. **Weak tables need two collector cycles** — a Go `weak.Pointer` is cleared in the cycle after the one that frees its object, so an entry survives one cycle longer than PUC-Lua would have kept it. Strings, numbers and booleans are never weakened, exactly as in PUC-Lua.
5. **Only the `"C"` locale is available** — `os.setlocale` has no libc locale database behind it, so any other name returns `nil` (as PUC-Lua does when the locale is not installed). Number formatting and parsing always use `.` as the decimal point.
6. **Performance** — `Instruction` is a struct rather than a packed uint32; may be re-packed after P7 benchmarks.

---

## 7. Build & test

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .                                  # must print nothing
go test -bench=. -benchmem -run=XXX ./...   # benchmarks
go build -o lua ./cmd/lua
```

CI runs the whole matrix on Linux, macOS and Windows (`.github/workflows/go.yml`):
`go vet`, `go build`, `go test` everywhere, `go test -race` on Linux and
macOS (the race detector needs cgo), plus a `gofmt` check and a single
pass over the benchmarks on Linux.

Reference sources for review (not vendored): `lua-5.4.9`, <https://www.lua.org/ftp/lua-5.4.9.tar.gz>.

## 8. License

MIT.
