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

**30 of the 33 files produce byte-identical output.** The remaining 3
differ only for reasons that follow from the design — there is no
collector of our own — or from the test environment:

| File | Why it differs |
|---|---|
| `gc` | the file is a test *of* the collector, and there is none here. Three assertions measure an incremental one: that `gcinfo()` falls after a step, that a bigger step size needs fewer steps, and that one huge step finishes a cycle. A step here is a whole cycle and reclaims nothing of Lua's, so all three fail. After them the file measures the collector's *pace* by allocating tables until `gcinfo()` has tripled — a loop that does not terminate when nothing is ever reclaimed |
| `all` | the environment, in four lines: the second PRNG seed (PUC-Lua's is truncated to 32 bits, this one is a 64-bit address), `total memory: 44.6K` against `619.2K` (the same accounting), the elapsed time, and the interpreter's own path. Everything else — including the C extension libraries `attrib`, `files`, `gengc` and `locals` load — matches |
| `heavy` | times out on both sides (it builds 2 MB of tables per iteration, and 2 GB overall); the two agree as far as the timeout lets them |

Weak tables and `__gc` are supported and pass their own tests —
`closure`, `db`, `coroutine`, `gengc`, `cstack` and `locals`, the last
two of which count collections with `require"tracegc"` — see §6 for how
they are done.

The suite is also mirrored inside `go test`: `testdata/diff/*.lua` (79
cases) is compared byte for byte against golden files captured from
PUC-Lua 5.4.9, and `cli_test.go` pins the observable behaviour of the
interpreter itself — `-v`/`-e`/`-l`/`-E`/`-W`/`--`, the `arg` table, the
script varargs, the usage messages and the exit status.

### 5.2 A real C extension: LPeg

`go test` also builds [LPeg](http://www.inf.puc-rio.br/~roberto/lpeg/) 1.1.0
— six translation units, published and used far away from here — from its
own sources in `testdata/lpeg/`, and runs LPeg's own test suite to
completion. That is worth more than another extension written for the
test, because LPeg reaches for the parts of the C API a small extension
never touches, and found four real bugs that nothing else had:

| What LPeg does | What was wrong |
|---|---|
| `lua_rawlen` on a userdata, to read back how big the block it was given is | the size was never recorded; a userdata answered with its number of user values, so LPeg derived a negative size, allocated nothing, and copied past the end |
| `lua_getallocf`, and then a call through the pointer it gets | there was no allocator to hand out, so LPeg called address zero |
| `luaL_Buffer`, written through `luaL_addchar` and through the address `luaL_prepbuffsize` returns | the address was NULL, and the result left its placeholder on the stack — which shifted the stack under the `lua_call` that followed |
| `lua_tostring`, to tell a rule named by a string from one named by a table | an empty string where PUC-Lua answers NULL, so LPeg took the wrong branch |

Every line its suite prints is PUC-Lua 5.4.9's, kept in
`testdata/lpeg/expected.txt`, so the test needs neither the reference
interpreter nor the network. Both are MIT-licensed and redistributed
under `testdata/lpeg/LICENSE`.

### 5.3 A second one: Lua CJSON, which found the locale

[Lua CJSON](https://kyne.com.au/~mark/software/lua-cjson.php) 2.1.0.10 is
built the same way from `testdata/cjson/` — three translation units — and
runs its own 105-case suite, all of which pass. Where LPeg exercises
patterns, userdata and the buffer API, CJSON exercises the other half of
what an extension does: walking tables with `lua_next`, converting numbers
and strings in both directions, and building error messages with
`luaL_error`. And it found something LPeg did not:

```lua
os.setlocale("cs_CZ")     -- the suite forces a decimal comma on purpose,
print(1.5)                -- to check what a number looks like then
```

`os.setlocale` used to answer `nil` for everything but `"C"`. It now goes
to the C library's `setlocale` and remembers the decimal point, which is
the one part of a locale Lua actually observes — and it is observed in
three places: `tostring` of a number, `string.format`'s `%f`/`%e`/`%g`
(but not `%a` or `%q`, which is what lets `%q` round-trip), and `tonumber`,
which accepts either separator. That is also what turned the official
suite's `literals.lua` green: its decimal-point cases had been skipped for
want of it.

Two kinds of line are left out of the comparison, both because PUC-Lua
does not fix them: a line printing a table with string keys (`pairs` walks
in hash order, and the hash is seeded afresh in every process — the
reference interpreter prints these two ways on two runs of its own), and
the one line naming the module's version. The suite wants a 4 MB
`utf8.dat`, which its `genutf8.pl` builds with perl; the test writes the
same bytes in Go, so that a C compiler is the only thing it needs.

### 5.4 A third one: LuaSocket, which owns memory and sockets

[LuaSocket](https://lunarmodules.github.io/luasocket/) is the third real
extension built and run here, and the one that reaches furthest. Where
LPeg and CJSON stay inside the interpreter, LuaSocket owns a socket: it
puts a `t_socket` in a userdata, gives that userdata the metatable it
registered in the registry, and expects to be handed back exactly the
pointer it put there — through `luaL_testudata`, once per method call.
It found two real bugs, neither of which the smaller extensions touch:

| What LuaSocket does | What was wrong |
|---|---|
| `lua_setmetatable`, to class a socket as a `tcp{server}` | the call pushed the *old* metatable to answer whether there was one, and then set the metatable from the top of the stack — which by then was that answer. A socket kept the metatable it already had, so `luaL_testudata` stopped recognising a server, and every method call on it raised "bad self" |
| `luaL_optlstring(L, 2, NULL, &size)`, and then a test of the pointer for NULL, to tell an empty chunk from the end of the stream | NULL could not be handed back: an absent argument answered `""`, and so did an argument that *was* the empty string, because the two were told apart by comparing with `""`. `luaL_checklstring` leaked besides — a fresh `C.CString` per call, and mime's filters call it once per chunk |

Its suites run over loopback, so the machine needs no network: TCP
connect, send and receive (including a 100 KB chunk that takes several
`write`s), a `receive` that has to time out rather than block, `select`,
UDP, `getaddrinfo`, and both of mime's filters — base64 and
quoted-printable — through `ltn12`. One loopback round trip of our own,
`socket_e2e.lua`, ties them together. Every line is PUC-Lua 5.4.9's, kept
in `testdata/luasocket/expected.txt`. MIT-licensed, redistributed under
`testdata/luasocket/LICENSE`.

### 5.5 A fourth one: LuaFileSystem, which reaches into a file handle

[LuaFileSystem](https://lunarmodules.github.io/luafilesystem/) is the
fourth, and the first that reaches *into* something the interpreter made
rather than something it made itself: `lfs.setmode` takes a file handle
from `io.open`, casts the userdata to a `luaL_Stream` — a `FILE*` and a
close function, in that order — and reads straight out of it. It found
two things, and the first is not really about files:

| What LuaFileSystem does | What was wrong |
|---|---|
| `luaL_testudata` on a file handle, then read `closef` out of the result | a userdata made in Go — which is what a file handle is — answered NULL, so C was handed nothing and read 8 bytes past it. It did not fail, it died. Every userdata now has a block to stand for it, made the first time C asks, because PUC-Lua answers with an address for any full userdata and never with NULL |
| read the `FILE*` out of that block | the block was zeroed, so every handle looked closed to C. An `io` file is a Go `*os.File` and has no `FILE*`, so the block is now filled in the way `luaL_Stream` is laid out — a real `FILE*` on a *duplicate* of the descriptor, since two owners of one descriptor would have whichever closed first close the file out from under the other — and cleared when the file is closed, which is how C sees a closed handle in PUC-Lua too |

It also could not be compiled: `include/lualib.h` did not exist, and an
extension includes it as a matter of course. Nor did `lua.h` define
`LUA_API`, `LUALIB_API` or `LUAMOD_API`, which PUC-Lua's `luaconf.h`
does and an extension declaring its own `luaopen_` uses. Both are there
now.

Its suite makes files, links and directories, so it runs in a temporary
directory of its own. Every line it prints is PUC-Lua 5.4.9's, kept in
`testdata/luafilesystem/expected.txt`. MIT-licensed, redistributed under
`testdata/luafilesystem/LICENSE`.

### 5.6 Continuations: the last part of the C API

`lua_callk`, `lua_pcallk` and `lua_yieldk` take a `lua_KFunction`, which
C calls when the coroutine is resumed *instead of* returning into the
frame that suspended — because in PUC-Lua that frame is gone: the yield
long-jumps out of it and its C stack is thrown away.

This port cannot throw a C frame away, and does not need to. A yield
parks the coroutine's goroutine exactly where it stands (§6, item 10),
so the C frame that asked for a continuation is still there when the
resume comes and simply goes on — which is what makes the continuation
reachable at all. `CallInfo` carries `k` and its context, `(*State).Yield`
calls the continuation on resume and answers how many values it pushed,
and `lua_pcallk` recovers an error raised below it the way `ldo.c`'s
`findpcall` + `finishpcallk` do.

`testdata/continuation/k.c` is a C extension built to exercise every
path — a `lua_yieldk` with and without a continuation, a `lua_callk`
whose callee yields, a `lua_callk` with no continuation (which must
*not* be yieldable), a `lua_pcallk` that yields, one that raises, one
with a message handler, a continuation that counts the results with
`lua_gettop` the way PUC-Lua's own `dofilecont` does (for both
`lua_pcallk` and `lua_yieldk`, which leave different stacks), one C
function that suspends three times in a row through a single
continuation, a yield of no values, and `coroutine.close` on a coroutine
parked inside a C yield. All 31 lines it prints are PUC-Lua 5.4.9's,
kept in `testdata/continuation/expected.txt`, and `TestContinuation`
compares them. Written for this port; MIT, like the rest of the
testdata.

### 5.7 A fifth one: luaposix, which surveys the whole C API

[luaposix](https://github.com/luaposix/luaposix) is not one module but
thirty small ones, and between them they call most of the C API — and
answer with the host's own numbers: `errno`, `struct tm`, `struct stat`,
a directory listing. Nine of them are run here, the ones whose answers
do not depend on which machine or which second they were asked on.

It found two things, and both were in the headers rather than in the
port:

| What luaposix needs | What was missing |
|---|---|
| `LUA_FILEHANDLE` | the name a file handle's metatable is registered under. `posix.stdio` would not compile at all, and an extension that asks whether a value is a file handle — or makes one for `io` to use — needs it |
| `LUA_SIGNATURE`, `LUA_GNAME`, `LUA_NUMTAGS`, `LUA_RIDX_LAST`, `LUA_VERSION_RELEASE_NUM`, `LUA_AUTHORS`, `LUA_COPYRIGHT`, `luaL_intop`, `luaL_opt`, `lua_writestring` and its two companions, `lua_getextraspace` | a dozen more macros of `lua.h` and `lauxlib.h`. The last is the only interesting one: it reaches *behind* the `lua_State*` C was handed, so the block behind that pointer now really has `LUA_EXTRASPACE` bytes in front of the handle, the way PUC-Lua's does |

The deprecated integer casts (`luaL_checkint` and friends) are there too,
but behind `LUA_COMPAT_APIINTCASTS` as PUC-Lua's `luaconf.h` has it — off
unless asked for, so that what an extension sees by default is exactly
what it would see against PUC-Lua. Every other macro, and every function
declaration, of the three public headers is present; the dozen that
PUC-Lua spells as a macro and this port as a function (`lua_call`,
`lua_pcall`, `lua_yield`, the eight `lua_is*`) behave the same either way.

All 30 lines the suite prints are PUC-Lua 5.4.9's, kept in
`testdata/luaposix/expected.txt`, and `TestLuaPosix` compares them. It
makes files, a directory and a link, so it runs in a temporary directory
of its own. MIT-licensed, redistributed under
`testdata/luaposix/LICENSE`.

### 5.8 Coverage inventory

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

### 5.9 Performance

`go test -bench=. -benchmem -run=XXX` runs the benchmarks in
`bench_test.go`. The table below is the same work measured end to end
through the two interpreters (Apple M1 Pro, PUC-Lua 5.4.9 built with
`-O2`), best of three runs:

| workload | PUC-Lua 5.4.9 | this port | ratio |
|---|---|---|---|
| `fib(26)`, recursive calls | 6.5 ms | 16.1 ms | 2.5× |
| arithmetic loop, 10⁷ iterations | 63.6 ms | 229.3 ms | 3.6× |
| integer-key table, 10⁶ insert+lookup | 12.6 ms | 35.7 ms | 2.8× |
| string-key table, 10⁵ insert+lookup | 64.7 ms | 92.7 ms | 1.4× |
| `table.sort`, 10⁵ elements | 27.6 ms | 39.3 ms | 1.4× |
| closure upvalue bump, 2×10⁶ calls | 31.5 ms | 86.2 ms | 2.7× |
| coroutine yield/resume, 2×10⁵ | 16.8 ms | 100.2 ms | 6.0× |
| `pcall` of a Lua function, 5×10⁵ | 15.9 ms | 32.8 ms | 2.1× |
| `string.gmatch` + `gsub` over 1 MB | 34.4 ms | 54.4 ms | 1.6× |
| compile a 120 KB chunk | 5.0 ms | 10.8 ms | 2.2× |

Where the time goes: `Instruction` is a struct instead of a packed
uint32 (decoded on the fly), every `TValue` access goes through a tag
switch, and each coroutine runs on a goroutine, so a yield/resume pair is
two goroutine switches — about 280 ns of them, measured on this machine
with a bare channel round trip — rather than a swap of stack pointers.
That is the whole of the coroutine gap and the one number here that
cannot be optimised away.

The rest has been shrinking, and the two changes that mattered were both
about the Go runtime rather than about Lua:

* **the arithmetic opcodes have an integer fast path.** Two integers can
  never reach a metamethod and always produce an integer, so
  `luaV_execute` does the arithmetic itself instead of walking
  `opCodeToArith` → `luaO_rawarith` → `intarith`. It is written out in
  the switch rather than moved to a helper, because a helper that can
  raise (`%` and `//`) is over the compiler's inline budget and the call
  costs about as much as the operation.
* **short strings are interned in a two-generation cache, not weakly.**
  Holding them weakly, so that Go's collector drops the dead ones, is
  the obvious translation of a table C's collector sweeps — and it cost
  *half* the time of a string-key table workload, because every interned
  string then carries a runtime weak handle and the handles are exactly
  what the collector walks during a cycle. The cache holds at most two
  generations' worth and registers nothing with the runtime. Its one
  consequence is that interning is no longer permanent, so short strings
  are compared by hash and bytes (`luaS_eqshrstr`), never by pointer.

`Value` is 32 bytes rather than 40 for the same reason C's `TValue` is a
union: a float shares the integer's field, stored as its IEEE bits.

---

## 6. Known differences from PUC-Lua 5.4.9

1. **`file:read(n)` with a count above 2 GiB is treated as end of file** — C hands the count straight to the allocator, and it is the allocator that rejects an absurd one ("not enough memory", which a negative count always is and this port reproduces). This port will not allocate on that scale, so a count that large reads nothing. Binary chunks are not affected: `string.dump` produces the same bytes PUC-Lua 5.4.9 produces — stripped or not — and each loads the other's.
2. **C extension modules load only when the host is built with cgo** — `cdlopen.go` is the `lsys_load`/`lsys_sym` half of `loadlib.c` and dlopens the library for real, so `package.cpath`, the four searchers and `require` work on an ordinary `.so` written against `lua.h`, and `package.loadlib` answers `dlerror()`'s own message with `"open"` or `"init"`, byte for byte what PUC-Lua answers. It needs cgo for one reason: a Go binary has to export the `lua_*`/`luaL_*` names into its **dynamic symbol table** for the module to resolve them, and only the cgo path can do that. Build with `CGO_ENABLED=1` (the default where a C toolchain is present) and compile the extension the way its platform allows the names to stay unresolved until load time — `-undefined dynamic_lookup` on macOS; on Linux, where the host's symbols are resolved from the executable's dynamic table, `-shared -fPIC` is enough. `go test` covers it: `cdlopen_test.go` writes a C extension with `luaL_newlib`, compiles it, `require`s it and calls into it; `cdlopen_api_test.go` compiles a larger one and compares all 25 lines it prints against the same extension run under PUC-Lua 5.4.9 — userdata with a registered metatable, `luaL_checkudata`, `luaL_checkoption`, the buffer API, user values, `lua_rawgetp`/`lua_rawsetp`, `lua_dump`, `lua_gc`, `lua_getstack`/`lua_getinfo` through a C `lua_Debug`, `lua_stringtonumber`, and every conversion `lua_pushfstring` has (including the "invalid option" error for one it does not).

The four functions that take `...` — `lua_pushfstring`, `lua_pushvfstring`, `luaL_error`, `lua_gc` — are written in C (`cdlbridge.c`), because neither direction of the cgo boundary can express a variadic call. They reproduce `luaO_pushvfstring` rather than delegating to `printf`: `%d` is a Lua integer, `%f` is `%.14g` with the `.0` C adds when the result looks like an integer, `%p` is a bare `%p`, `%U` is a UTF-8 sequence, and anything else is the error PUC-Lua gives. `printf` would have accepted `%.2f` (which Lua rejects) and printed `%d` as C's `int`.

With `CGO_ENABLED=0` the backend is absent, `package.loadlib` answers `nil, "dynamic libraries not enabled", "absent"` — word for word what PUC-Lua answers when built without `LUA_USE_DLOPEN` — and a C module found in `package.cpath` is the error PUC-Lua gives there.

Continuations — `lua_callk`, `lua_pcallk` and `lua_yieldk` with a non-NULL `k` — are implemented: see §5.6. The one thing about them that is *not* PUC-Lua's is the status a `lua_callk` continuation is told: C always passes `LUA_YIELD` there, and here the C function calls the continuation itself, having watched the call return normally. Nothing observable depends on it, because for `lua_callk` that status is a constant.

`lua_error` is worth a note, because "it raises instead of longjmping" reads like a gap and is not one. A Go panic crosses a cgo frame: the runtime unwinds to the `recover` of the call that was protecting the call, and the extension's frame is left behind exactly as `longjmp` would leave it. Three levels deep — Lua → C → Lua → C → raise — the answer is PUC-Lua's, line for line, and that is what `TestCModuleErrors` pins. `longjmp` across the boundary is the thing that would be unsafe, since it would skip the runtime's own bookkeeping for the cgo call, so there is no `setjmp` here on purpose. What is lost is what `longjmp` loses too: a C frame holding something that needs cleanup does not get to clean it up.

The C block behind a userdata is freed when the userdata is collected: `C.malloc` is not memory Go's collector knows about, so without a finalizer an extension making userdata in a loop would grow without bound. Anything written in Lua, or ported to Go, needs none of this: `L.RequireF(modname, openf, true)` adds one to `package.loaded` (the port's `luaL_requiref`), or set `package.preload[name]` to a Go function and let `require` find it the usual way.
3. **The collector is Go's, so its observable timing is not PUC-Lua's** — `collectgarbage("count")` reports the Go heap rather than PUC-Lua's own allocator, and `__gc` runs where this port runs a cycle (a collector step, `collectgarbage`, or `Close()`) rather than where PUC-Lua's step would have run it. Objects with a `__gc` are held between cycles so that Go cannot finalise them at a moment of its own choosing, and the metamethod of every object that still has one runs when the state closes, reachable or not — which is what C does and what makes `<close>`, `Close()` and weak tables deterministic:  a sweep removes an entry once nothing but the table refers to it. `collectgarbage("step")` is the one answer that cannot be matched: PUC-Lua answers true only when the step brought the collector back to the pause state, which a single step almost never does — it answers false for hundreds of calls in a row, even right after a full collection — while a step here is a whole cycle and answers true. Answering false instead would leave the documented idiom `while not collectgarbage("step") do end` spinning forever, which is the worse of the two.
4. **Weak tables need two collector cycles** — a Go `weak.Pointer` is cleared in the cycle after the one that frees its object, so an entry survives one cycle longer than PUC-Lua would have kept it. Strings, numbers and booleans are never weakened, exactly as in PUC-Lua.
5. **`os.setlocale` asks the C library, so it needs cgo** — the decimal point is the one part of a locale Lua observes, and it is observed in three places: `tostring` of a number, `string.format`'s `%f`/`%e`/`%g` (but not `%a` or `%q`, which is what lets `%q` round-trip), and `tonumber`, which accepts either separator while a source literal always uses `.`. Which locales are installed is the host's business, exactly as in PUC-Lua, and a name the C library refuses answers `nil`. Built with `CGO_ENABLED=0` there is no C library to ask, so every name but `"C"` answers `nil` and the decimal point stays `.`.
6. **`os.date` refuses a slightly different range of times, `os.time` accepts dates the host's `mktime` refuses, and `os.date("!%z")` answers the UTC offset rather than the local one** — `struct tm` keeps the year in an `int`, and the host's `gmtime`/`localtime` answer NULL once it no longer fits, which is where PUC-Lua's "date result cannot be represented in this installation" comes from. This port asks the same question against `tm_year` and reproduces the top of the range exactly — 67768036191676799 is the last representable second (year 2147485547, `tm_year` `INT_MAX`) and one more is refused — but the bottom of the host's range stops a little short of `INT_MIN`, so a window of some 2×10¹⁴ seconds at the far negative end is answered here and refused there; no second beyond that is answered by either. For `os.time`, PUC-Lua builds a time with the C library's `mktime` and reports "time result cannot be represented in this installation" when it answers -1; which dates those are is up to the C library (on macOS, anything that normalises to a year before 1900; glibc accepts those). Go's `time.Date` has no such limit, so this port answers a number. The opposite happens for `%z` under `!`: the time is UTC, but the host's `gmtime` leaves its offset at the local value, so PUC-Lua prints the local offset where this port prints `+0000`. Everything PUC-Lua itself decides is reproduced — the bounds each field must satisfy before it reaches `struct tm` ("field 'year' is out-of-bound"), the missing and non-integer fields, a result of exactly -1, and every other specifier of `os.date`.
7. **The maths library is Go's, not the platform's libm** — `math.sin`, `cos`, `tan`, `exp`, `log`, `log10`, `sqrt`, `asin`, `acos`, `atan`, `atan2`, `pow`, `sinh`, `cosh`, `tanh`, `frexp` and `ldexp` are Go's implementations rather than the ones libc links. They agree to 15 significant digits and differ in the last one or two for a fraction of the arguments: measured over 120 arguments spread across fifteen magnitudes, `sqrt` never differs, `exp` and `log` differ for a handful (below one ulp), `log10`, `tanh`, `asin`, `acos` and `atan` for a few dozen (around one ulp), and `sin`, `cos` and `tan` for a quarter to a half. Above 2^53, where the argument has to be reduced before the function can be asked, the gap widens — `tan` strays by six ulp and `sin` by seven — because neither implementation is correctly rounded there and they reduce differently. Every arithmetic operation, `sqrt`, and both integer conversions are exact. A NaN is spelled the way the platform's C library spells it — `"nan"` on macOS, `"-nan"` on glibc when the sign bit is set — because that is what C's `printf` does with it. The one place the port deliberately steps outside Go's implementations is the band in which `exp(|x|)` overflows while `sinh(x)` and `cosh(x)` do not (709.78 < |x| < 710.48): Go's `math.Sinh` answers `inf` for `math.sinh(710)`, whose true value is `exp(710) / 2` = 1.1e308 and representable, so the halving is folded into the argument there instead.
8. **`math.randomseed()` with no arguments returns a different second seed** — PUC-Lua returns the current time and the *address of the `lua_State`* as its two seeds (lmathlib.c: `randseed`), and this port returns the same two quantities, so the first seed — the time — matches and the second is a Go heap address rather than a C one; the numbers are both host addresses and are not reproducible in either implementation. `math.randomseed(n)` and `math.randomseed(n, m)` return their arguments unchanged, and the generator itself is the same xoshiro256**-based one: for a given seed it produces the identical sequence, integer or float, which the suite pins across 20 draws of `math.random()`, `math.random(6)` and `math.random(10, 20)`.
9. **A coroutine suspends by parking its goroutine, and a C frame that suspended is still there when it wakes** — C's `lua_yield` long-jumps to the `lua_resume` that will pick the thread up again, and everything between is discarded; this port instead hands the yielded values to the resumer through a channel and then blocks on the next resume request, so the Lua stack, the saved program counter and — for a C caller — the C stack itself all survive at the yield point. That is what lets a metamethod or a Go function yield from the middle of a call, and it is what makes continuations work (§5.6). Two consequences follow. The C-call depth a thread inherits from its resumer is refreshed at every yield rather than only at the first resume, which is what C does too, since `lua_resume` runs on every resume. And a C function that yielded does not get to notice: `lua_callk` returns to it as if the call had simply finished, so the status a `lua_callk` continuation hears is `LUA_OK` where PUC-Lua's is always `LUA_YIELD` — a constant, so nothing can depend on the difference. `lua_pcallk`'s status does vary, and that one *is* PUC-Lua's: the port counts the yields the thread has made and reports `LUA_YIELD` when the call was interrupted by one.
10. **Two short strings that are equal may not be the same object** — PUC-Lua interns short strings for as long as they live, so `lua_topointer` answers the same address for two equal ones and a C extension can use that address as a key. Here interning is a two-generation cache (§5.9), so a string can be interned again once it has aged out of both generations, and two live strings with the same bytes may then sit at different addresses. Nothing in Lua can see it: strings compare by content, which is why `luaS_eqshrstr` compares the hash and then the bytes where C's `eqshrstr` would compare pointers, and `rawequal`, `==`, table keys and `next` all behave as before. Only `lua_topointer` — and a `lua_rawsetp` keyed on it — can tell the difference.

11. **Performance** — `Instruction` is a struct rather than a packed uint32; may be re-packed after P7 benchmarks.

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
