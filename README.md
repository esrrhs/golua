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

### 4.1 C extension modules (needs cgo)

A `.so` written against the usual headers loads through `require`. The
headers are this repository's `include/lua.h` and `include/lauxlib.h`,
whose names are the real `lua_*` / `luaL_*` ones, so existing C sources
compile unchanged:

```c
#include "lua.h"
#include "lauxlib.h"

static int l_add (lua_State *L) {
  lua_pushinteger(L, luaL_checkinteger(L, 1) + luaL_checkinteger(L, 2));
  return 1;
}

static const luaL_Reg funcs[] = {{"add", l_add}, {NULL, NULL}};

int luaopen_myext (lua_State *L) {
  luaL_newlib(L, funcs);
  return 1;
}
```

```bash
# macOS: let the lua_* names stay unresolved until load time
cc -shared -fPIC -I/path/to/fakescript_go/include \
   -undefined dynamic_lookup -o myext.so myext.c
# Linux: the host's symbols come from the executable's dynamic table
cc -shared -fPIC -I/path/to/fakescript_go/include -o myext.so myext.c

./lua -e 'print(require("myext").add(3, 4))'   # 7
```

`package.cpath` is the platform's, so `LUA_CPATH` or `package.cpath` put
the extension where `require` will look. See §6 item 2 for what the
build needs and what the bridge does not cover yet.

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

**29 of the 33 files produce byte-identical output.** The remaining 4
differ only for reasons that follow from the design — there is no
collector of our own — or from the test environment:

| File | Why it differs |
|---|---|
| `gc` | `collectgarbage("count")` accounting, and the granularity of `collectgarbage("step", n)`: PUC-Lua's step does a bounded amount of work, so `dosteps(10) < dosteps(2)`; Go's collector is all or nothing, so a step is a whole cycle and both take one |
| `all` | the same accounting, in the one line that prints it: `total memory: 44.7K` against `611.9K`. Everything else in the suite — including the C extension libraries `attrib`, `files`, `gengc` and `locals` load — matches |
| `literals` | `os.setlocale("pt_BR")` is unavailable here (see §6), so the decimal-point tests are skipped |
| `heavy` | times out on both sides (it builds 2 GB of tables); the two agree as far as the timeout lets them |

Weak tables and `__gc` are supported and pass their own tests —
`closure`, `db`, `coroutine`, `gengc`, `cstack` and `locals`, the last
two of which count collections with `require"tracegc"` — see §6 for how
they are done.

The suite is also mirrored inside `go test`: `testdata/diff/*.lua` (79
cases) is compared byte for byte against golden files captured from
PUC-Lua 5.4.9, and `cli_test.go` pins the observable behaviour of the
interpreter itself — `-v`/`-e`/`-l`/`-E`/`-W`/`--`, the `arg` table, the
script varargs, the usage messages and the exit status.

### 5.2 Coverage inventory

Conformance is measured by comparing outputs, but it is also worth asking
the blunter question: is *everything there*? `testdata/diff/79_inventory.lua`
enumerates every name the reference interpreter exposes — `_G` and the nine
standard libraries, all thirty metamethods, all ten `collectgarbage` options,
and the version-visible constants — and prints them sorted, so that the
order of `pairs` (which PUC-Lua randomises per run) cannot matter. The
answer is **byte-identical to PUC-Lua 5.4.9**, which is to say that nothing
is missing and nothing is extra:

| Table | Entries | New in 5.4 |
|---|---|---|
| `_G` | 36 | `warn` |
| `string` | 17 | `pack`, `unpack`, `packsize` |
| `math` | 35 | `LUA_COMPAT_MATHLIB` (`pow`, `frexp`, `ldexp`, `log10`, `atan2`) |
| `debug` | 17 | `setcstacklimit`, `getuservalue`, `setuservalue` |
| `coroutine` | 8 | `close` |
| `table` / `io` / `os` / `utf8` / `package` | 7 / 14 / 11 / 6 / 8 | — |

All thirty metamethods are recognised, including `__close`, `__gc`, `__mode`,
`__name`, `__pairs`, `__ipairs` and `__metatable`.

The behaviour that a name list cannot show is pinned by
`testdata/diff/78_new54.lua`: `warn` and its `@on`/`@off` control messages,
`debug.setcstacklimit` and the depth it actually buys, integer `math.random`
(and that a given seed produces the identical sequence), the new `%q`,
the mode argument of `load`, `table.pack`/`unpack` at their boundaries, and
integer division and modulo on negative operands.

The rest of the suite covers the areas in between — coercion and `tonumber`
in every base, `next` and traversal while a table is being modified,
`table.sort` with a comparator that misbehaves, `string.gsub`'s replacement
protocol, the bitwise library, recursion depth and the two overflow errors,
`error` objects and levels, coroutine boundaries that refuse to be crossed,
`<close>` and `<const>`, the whole debug library, `utf8` and the byte-level
string functions, the lexer's escapes and long strings, `require` and
`package.searchpath`, and `os`/`io` at their edges.

### 5.3 Performance

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

1. **`file:read(n)` with a count above 2 GiB is treated as end of file** — C hands the count straight to the allocator, and it is the allocator that rejects an absurd one ("not enough memory", which a negative count always is and this port reproduces). This port will not allocate on that scale, so a count that large reads nothing. Binary chunks are not affected: `string.dump` produces the same bytes PUC-Lua 5.4.9 produces — stripped or not — and each loads the other's.
2. **C extension modules load only when the host is built with cgo** — `cdlopen.go` is the `lsys_load`/`lsys_sym` half of `loadlib.c` and dlopens the library for real, so `package.cpath`, the four searchers and `require` work on an ordinary `.so` written against `lua.h`, and `package.loadlib` answers `dlerror()`'s own message with `"open"` or `"init"`, byte for byte what PUC-Lua answers. It needs cgo for one reason: a Go binary has to export the `lua_*`/`luaL_*` names into its **dynamic symbol table** for the module to resolve them, and only the cgo path can do that. Build with `CGO_ENABLED=1` (the default where a C toolchain is present) and compile the extension the way its platform allows the names to stay unresolved until load time — `-undefined dynamic_lookup` on macOS; on Linux, where the host's symbols are resolved from the executable's dynamic table, `-shared -fPIC` is enough. `go test` covers it: `cdlopen_test.go` writes a C extension with `luaL_newlib`, compiles it, `require`s it and calls into it; `cdlopen_api_test.go` compiles a larger one and compares all 25 lines it prints against the same extension run under PUC-Lua 5.4.9 — userdata with a registered metatable, `luaL_checkudata`, `luaL_checkoption`, the buffer API, user values, `lua_rawgetp`/`lua_rawsetp`, `lua_dump`, `lua_gc`, `lua_getstack`/`lua_getinfo` through a C `lua_Debug`, `lua_stringtonumber`, and every conversion `lua_pushfstring` has (including the "invalid option" error for one it does not).

The four functions that take `...` — `lua_pushfstring`, `lua_pushvfstring`, `luaL_error`, `lua_gc` — are written in C (`cdlbridge.c`), because neither direction of the cgo boundary can express a variadic call. They reproduce `luaO_pushvfstring` rather than delegating to `printf`: `%d` is a Lua integer, `%f` is `%.14g` with the `.0` C adds when the result looks like an integer, `%p` is a bare `%p`, `%U` is a UTF-8 sequence, and anything else is the error PUC-Lua gives. `printf` would have accepted `%.2f` (which Lua rejects) and printed `%d` as C's `int`.

With `CGO_ENABLED=0` the backend is absent, `package.loadlib` answers `nil, "dynamic libraries not enabled", "absent"` — word for word what PUC-Lua answers when built without `LUA_USE_DLOPEN` — and a C module found in `package.cpath` is the error PUC-Lua gives there.

Two things the bridge does not do (both noted in `cdlopen.go`): `lua_error` from C raises a panic rather than unwinding the extension's own C frames, so a module that throws across them is not supported yet; and a continuation (`lua_callk`, `lua_pcallk`, `lua_yieldk` with a non-NULL `k`) is dropped, because there is no C frame to return into after a yield. Anything written in Lua, or ported to Go, needs none of this: `L.RequireF(modname, openf, true)` adds one to `package.loaded` (the port's `luaL_requiref`), or set `package.preload[name]` to a Go function and let `require` find it the usual way.
3. **The collector is Go's, so its observable timing is not PUC-Lua's** — `collectgarbage("count")` reports the Go heap rather than PUC-Lua's own allocator, and `__gc` runs where this port runs a cycle (a collector step, `collectgarbage`, or `Close()`) rather than where PUC-Lua's step would have run it. Objects with a `__gc` are held between cycles so that Go cannot finalise them at a moment of its own choosing, and the metamethod of every object that still has one runs when the state closes, reachable or not — which is what C does and what makes `<close>`, `Close()` and weak tables deterministic:  a sweep removes an entry once nothing but the table refers to it. `collectgarbage("step")` is the one answer that cannot be matched: PUC-Lua answers true only when the step brought the collector back to the pause state, which a single step almost never does — it answers false for hundreds of calls in a row, even right after a full collection — while a step here is a whole cycle and answers true. Answering false instead would leave the documented idiom `while not collectgarbage("step") do end` spinning forever, which is the worse of the two.
4. **Weak tables need two collector cycles** — a Go `weak.Pointer` is cleared in the cycle after the one that frees its object, so an entry survives one cycle longer than PUC-Lua would have kept it. Strings, numbers and booleans are never weakened, exactly as in PUC-Lua.
5. **Only the `"C"` locale is available** — `os.setlocale` has no libc locale database behind it, so any other name returns `nil` (as PUC-Lua does when the locale is not installed). Number formatting and parsing always use `.` as the decimal point.
6. **`os.date` refuses a slightly different range of times, `os.time` accepts dates the host's `mktime` refuses, and `os.date("!%z")` answers the UTC offset rather than the local one** — `struct tm` keeps the year in an `int`, and the host's `gmtime`/`localtime` answer NULL once it no longer fits, which is where PUC-Lua's "date result cannot be represented in this installation" comes from. This port asks the same question against `tm_year` and reproduces the top of the range exactly — 67768036191676799 is the last representable second (year 2147485547, `tm_year` `INT_MAX`) and one more is refused — but the bottom of the host's range stops a little short of `INT_MIN`, so a window of some 2×10¹⁴ seconds at the far negative end is answered here and refused there; no second beyond that is answered by either. For `os.time`, PUC-Lua builds a time with the C library's `mktime` and reports "time result cannot be represented in this installation" when it answers -1; which dates those are is up to the C library (on macOS, anything that normalises to a year before 1900; glibc accepts those). Go's `time.Date` has no such limit, so this port answers a number. The opposite happens for `%z` under `!`: the time is UTC, but the host's `gmtime` leaves its offset at the local value, so PUC-Lua prints the local offset where this port prints `+0000`. Everything PUC-Lua itself decides is reproduced — the bounds each field must satisfy before it reaches `struct tm` ("field 'year' is out-of-bound"), the missing and non-integer fields, a result of exactly -1, and every other specifier of `os.date`.
7. **The maths library is Go's, not the platform's libm** — `math.sin`, `cos`, `tan`, `exp`, `log`, `log10`, `sqrt`, `asin`, `acos`, `atan`, `atan2`, `pow`, `sinh`, `cosh`, `tanh`, `frexp` and `ldexp` are Go's implementations rather than the ones libc links. They agree to 15 significant digits and differ in the last one or two for a fraction of the arguments: measured over 120 arguments spread across fifteen magnitudes, `sqrt` never differs, `exp` and `log` differ for a handful (below one ulp), `log10`, `tanh`, `asin`, `acos` and `atan` for a few dozen (around one ulp), and `sin`, `cos` and `tan` for a quarter to a half. Above 2^53, where the argument has to be reduced before the function can be asked, the gap widens — `tan` strays by six ulp and `sin` by seven — because neither implementation is correctly rounded there and they reduce differently. Every arithmetic operation, `sqrt`, and both integer conversions are exact. A NaN is spelled the way the platform's C library spells it — `"nan"` on macOS, `"-nan"` on glibc when the sign bit is set — because that is what C's `printf` does with it. The one place the port deliberately steps outside Go's implementations is the band in which `exp(|x|)` overflows while `sinh(x)` and `cosh(x)` do not (709.78 < |x| < 710.48): Go's `math.Sinh` answers `inf` for `math.sinh(710)`, whose true value is `exp(710) / 2` = 1.1e308 and representable, so the halving is folded into the argument there instead.
8. **`math.randomseed()` with no arguments returns a different second seed** — PUC-Lua returns the current time and the *address of the `lua_State`* as its two seeds (lmathlib.c: `randseed`), and this port returns the same two quantities, so the first seed — the time — matches and the second is a Go heap address rather than a C one; the numbers are both host addresses and are not reproducible in either implementation. `math.randomseed(n)` and `math.randomseed(n, m)` return their arguments unchanged, and the generator itself is the same xoshiro256**-based one: for a given seed it produces the identical sequence, integer or float, which the suite pins across 20 draws of `math.random()`, `math.random(6)` and `math.random(10, 20)`.
9. **Performance** — `Instruction` is a struct rather than a packed uint32; may be re-packed after P7 benchmarks.

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

Two of those need more than Go: `cdlopen_test.go` and
`cdlopen_api_test.go` compile C extensions and `require` them, and skip
themselves when there is no cgo or no C compiler, so
`CGO_ENABLED=0 go test ./...` is a supported configuration that simply
does not exercise the dlopen path.

CI runs the whole matrix on Linux, macOS and Windows (`.github/workflows/go.yml`):
`go vet`, `go build`, `go test` everywhere, `go test -race` on Linux and
macOS (the race detector needs cgo), plus a `gofmt` check and a single
pass over the benchmarks on Linux.

Reference sources for review (not vendored): `lua-5.4.9`, <https://www.lua.org/ftp/lua-5.4.9.tar.gz>.

## 8. License

MIT.
