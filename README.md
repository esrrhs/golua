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
| `lstring.c` | `lstring.go` | ✅ | string interning (Go map; short strings interned for the life of the state, as in C) |
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
| `luac.c` | `luac.go` + `cmd/luac/main.go` | ⬜ | the standalone compiler: dump, strip, parse-only, listing |
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
| `collectgarbage("count")` | Kbytes handed out by Lua's allocator | Kbytes of the Go heap (`runtime.ReadMemStats`) |
| `collectgarbage("collect" / "step")` | starts a collection | a cycle of Go's own: what has become unreachable is finalized, and the weak entries are swept |
| `collectgarbage("step")` answers | whether the step ended a cycle — `false` for a basic step | `false` for a basic step, as in C, and `true` for one given work to do, because here any step is a whole cycle. It cannot answer `false` to both: `repeat until collectgarbage("step", siz)` — the shape `gc.lua` forces a collection with — would never end |
| `collectgarbage("stop" / "restart" / "isrunning")` | stops, restarts, reports the collector | kept on the state and answered, so `stop` really does stop the cycle an allocation would drive |
| `collectgarbage("setpause" / "setstepmul")` | tunes it, answering the value replaced | remembered and answered (200 and 100, as in C); Go's collector pays no attention to them |
| `collectgarbage("generational" / "incremental")` | switches mode, answering the one replaced | remembered and answered; a fresh state answers `generational`, as 5.4's does |
| `lua_gc` (the C API, `cdl_gc_cb`) | the same options from C | the same cycle, so Lua and C reach the same collector |
| `__gc` | GC-driven finalization | `runtime.SetFinalizer` finds the object unreachable and queues the metamethod; the VM runs the queue at a cycle boundary — the step an allocation drives, `collectgarbage`, or `Close()` |
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

static const luaL_Reg funcs[] = { {"add", l_add}, {NULL, NULL} };

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
| **P6** | `cmd/lua` CLI + REPL, `cmd/luac` compiler, `ldump`/`lundump` | ✅ |
| **P7** | Official Lua 5.4 test suite, benchmarks, CI on 3 OSes | ✅ |

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
| `gc` | the file is a test *of* the collector, and there is none here. Its assertions measure an incremental one: that `gcinfo()` falls after a step, that a bigger step size needs fewer steps, that one huge step finishes a cycle, that a weak table is swept. They fail on the first one that needs a collection to have happened. After them the file measures the collector's *pace* by allocating tables until `gcinfo()` has tripled — a loop that does not terminate when nothing is ever reclaimed |
| `all` | the environment, in four lines: the second PRNG seed (PUC-Lua's is truncated to 32 bits, this one is a 64-bit address), `total memory: 44.6K` against `619.2K` (the same accounting), the elapsed time, and the interpreter's own path. Everything else — including the C extension libraries `attrib`, `files`, `gengc` and `locals` load — matches |
| `heavy` | times out on both sides (it builds 2 MB of tables per iteration, and 2 GB overall); the two agree as far as the timeout lets them |

Weak tables and `__gc` are supported: an entry goes when nothing but the
table refers to it, and a `__gc` runs, both at a cycle boundary — the step
an allocation drives, `collectgarbage`, or `Close()` — which is what
`closure`, `db`, `cstack`, `events`, `tracegc` and the 80 golden cases
exercise. See §6 for how they are done, and §5.1 above for what a cycle
here cannot answer.

A `__gc` also keeps PUC-Lua's ordering rules, and `main.lua` is what checks
them. An allocation inside a finalizer cannot drive a cycle — C's `GCTM`
sets `gcstp & GCSTPGC` for the duration — so one finalizer never starts in
the middle of another, and `collectgarbage` answers `nil` there rather
than `0`. Closing the state from inside a finalizer, which is what
`os.exit(0, true)` does, runs the finalizers that are left *there and
then*, before `exit`: `main.lua`'s "bug since 5.2" case creates an object
inside a `__gc`, asks `collectgarbage`, closes the state, and expects the
other object's metamethod after all of that — `creating 1 / creating 2 / 2
/ creating 3 / nil / 1` — and the object created inside the finalizer is
never finalized at all, the state being already on its way out.

**The 33 official files are in the repository and are first-class `go
test` cases.** `testdata/official/` holds all of `testes/` (redistributed
under the same MIT licence as Lua itself) beside a
`<name>.<goos>.expected`: what the reference interpreter, built from
source, printed for that file. `official_test.go` runs one side and then
the other, normalises, and compares — so `go test ./...` now carries 30
identical / 2 known differences / 1 not run. A few things about it are
deliberate:

- `testdata/official/libs/*.c` — the five C libraries Lua's own makefile
  builds — are compiled into that copy, so `attrib.lua` and `main.lua`
  exercise the C half of the suite for real: versioned module names
  (`lib2-v2`), `luaopen_` chains, and a library that borrows a symbol
  from one opened before it. Without them both files stop at "cannot
  load dynamic library" and none of that is tested.

- The files run in a copy made in `t.TempDir()`. `attrib.lua` writes a
  dozen chunks under `libs/` and requires them; `main.lua` spawns
  subprocesses. Nothing may be written into `testdata/`, a checkout is
  often read-only, and a leftover file would change the next run's answer.
- The exit status is not part of the comparison: several of these files
  *end* in a failed assert, which is the point of them.
- `heavy` is not run (it times out on both sides); `all`, `gc`,
  `coroutine`, `gengc` and `locals` are registered differences, and the
  test says so out loud if one of them ever starts matching — that is
  progress and it should not go unnoticed.
- `attrib` and `literals` differ depending on *how the binary was built*:
  without cgo there is no dlopen and no `setlocale`, and those two files
  test exactly those. The test does not guess at `CGO_ENABLED`; it asks
  the binary it just built (the third value `package.loadlib` returns,
  and whether `os.setlocale` answers), so `CGO_ENABLED=0 go test ./...`
  passes too.
- To re-capture after a Lua release, or on a platform that has no golden
  yet: `LUA_REF=/path/to/lua go test -run TestOfficialSuite` (and
  `-run TestDifferentialSuite` for `testdata/diff`). The
  reference has to be built the way CI builds it — `make posix` with
  `-DLUA_USE_DLOPEN`, on macOS with `SYSLIBS=` — because `posix` is the
  one target that is the same everywhere *and* leaves readline out, and
  readline matters: with it, the reference prints no REPL prompt when its
  input is a pipe, and `main.lua` tests the prompt.
- Goldens are per-GOOS — `all.lua.darwin.expected`, and so on — in both
  suites, and a platform with none of its own skips the case and says so.
  A good many answers belong to the machine and not to Lua: `os.date`
  asks the C library and answers in the runner's timezone, and the NaN
  that `0/0` produces carries whatever sign the CPU gave it (`nan` here,
  `-nan` there). One golden per case could only ever be green on the
  machine that produced it.
- `api.lua` matches but asserts nothing, and the test says so every run.
  Its whole body sits behind `if T==nil then return end`, and `T` comes
  from Lua's own `ltests.c`, which is `LUA_CORE`: it includes `lstate.h`,
  `ltable.h`, `lcode.h` and `lopcodes.h` and walks `GCObject`, `CallInfo`
  and `TValue` by hand (`L->ci->func.p`, `s2v`, `setobj2s`). There is no
  C `lua_State` in this port to hand it, so it cannot be compiled against
  one. Writing `T` in Go would not finish the job either: a third of what
  `api.lua` asks `T` for is the debug allocator — `T.alloccount` and
  `T.totalmem` set a limit after which allocations fail — and the
  collector's own state (`T.gcstate`, `T.gccolor`), none of which exists
  here. It is the same situation as `gc.lua`, not a gap that can be
  closed; the C API is covered by §5.6–5.8, by the five real C
  extensions, and by `capi_test.go`.

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

#### A shared object built for PUC-Lua loads as it is

LPeg, CJSON and LuaFileSystem were also built **against the reference
interpreter's own headers** (`-I /tmp/lua-5.4.9/src`, not `include/`) and
loaded without being rebuilt: the output is PUC-Lua's line for line. So
an existing `.so` — a LuaRocks module, say — does not need recompiling,
on three conditions, each measured:

| Condition | Why | How to check |
|---|---|---|
| built for Lua **5.4** | the names are the ABI: a 5.1–5.3 object asks for `lua_objlen`, `luaL_register`, `lua_setfenv`, which do not exist here, and fails to load | `nm -u x.so \| grep lua_` |
| built **without linking liblua** | the usual way — symbols left for the host to fill. One that has `liblua` in its `DT_NEEDED` resolves `lua_*` to the real interpreter and then dereferences *this* port's `lua_State`, which is an opaque handle: measured `SIGSEGV` | `otool -L x.so` / `ldd x.so` |
| the host binary was built **with cgo** | `CGO_ENABLED=0` exports no `lua_*` at all, and dyld answers `symbol not found in flat namespace '_lua_pushlstring'` | `nm -gU luabin \| grep _lua` |

The exported set is every name in `include/lua.h` and `include/lauxlib.h`
except the four that take `...`: 177 symbols, against the 55 LPeg asks
for. Nothing else about the build has to agree — the headers an extension
was compiled against can be PUC-Lua's own, because what has to match is
the ABI, not the text.

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

### 5.8 The C API, end to end

LPeg, CJSON, LuaSocket, LuaFileSystem and luaposix between them call a
great deal of the C API, but they are programs with a job to do, not a
survey of it: whole groups of functions are never reached. So
`testdata/capi/capi.c` is the survey. It is an ordinary extension —
compiled against `include/`, opened by `luaopen_capi`, found by `require`
— that walks the API group by group and answers with numbers, booleans
and fixed strings, which is what makes it comparable:

| Group | What it calls |
|---|---|
| the stack | `lua_rotate`, `lua_absindex`, `lua_copy`, `lua_checkstack`, `lua_remove`, `lua_insert`, `lua_replace` |
| arithmetic | `lua_arith` for all fourteen `LUA_OP*`, `lua_compare`, `lua_rawequal`, `lua_concat`, `lua_len`, `lua_rawlen` |
| tables | every get/set pair — `getfield`/`setfield`, `geti`/`seti`, `rawget`/`rawset`, `rawgeti`/`rawseti` — plus `lua_next`, `lua_createtable` and the globals |
| conversions | `lua_tointegerx`/`lua_tonumberx` with the `isnum` out parameter, and `lua_type`/`lua_typename` and every `lua_is*` on one value of each of the nine kinds |
| lauxlib | `luaL_ref`/`luaL_unref`, `luaL_newmetatable`, `luaL_getsubtable`, `luaL_loadbufferx` with a mode, `luaL_fileresult`, `luaL_traceback`, `luaL_tolstring`, `luaL_gsub`, `luaL_addgsub`, `luaL_check*`/`luaL_opt*`, `luaL_argerror`, `luaL_typeerror`, `luaL_where`, and `luaL_requiref(L, "string", luaopen_string, 1)` |
| userdata | `lua_newuserdatauv` with several user values, `lua_getiuservalue`/`lua_setiuservalue`, and `LUA_TNONE` past the end |
| to-be-closed | `lua_toclose` and `lua_closeslot` |
| the debug API | `lua_sethook` with a real C hook, `lua_getstack`/`lua_getinfo` for every field, `lua_getlocal`/`lua_setlocal`, `lua_getupvalue`/`lua_setupvalue`, `lua_upvalueid`/`lua_upvaluejoin` |
| coroutines | `lua_newthread`, `lua_resume` twice, `lua_status`, `lua_isyieldable`, `lua_xmove`, `lua_resetthread` |
| buffers | the whole API: `luaL_addstring`/`addchar`/`addlstring`/`addvalue`, `luaL_prepbuffsize`, `luaL_addsize`, `luaL_bufflen`/`buffaddr`/`buffsub`, `luaL_pushresult`, `luaL_buffinitsize`, `luaL_pushresultsize` |
| a file handle | `luaL_checkudata(L, 1, LUA_FILEHANDLE)`, cast to `luaL_Stream` |
| limits | `lua_setcstacklimit`, `lua_atpanic` |

All 35 lines it prints are PUC-Lua 5.4.9's, kept in
`testdata/capi/expected.txt`, and `TestCApiSurface` compares them. The
driver is loaded under the chunk name `cext` — `testdata/capi/run.lua`
does that for the reference interpreter — so the `cext:35` inside a
`luaL_where` answer is the same on both sides.

It found ten things, none of which any of the five published
extensions had reached:

| What it does | What was wrong |
|---|---|
| `luaL_requiref(L, "string", luaopen_string, 1)` | `luaopen_*` was declared in `lualib.h` but never put in the dynamic symbol table, so an extension linked into the host — or a host opening one library by hand — could not resolve the name |
| read a value with `luaL_tolstring`, then pop it | `luaL_tolstring` answered a pointer but left nothing on the stack, which is what "tostring" means in the middle of building one. The stack came out one short and the next absolute index named the wrong value |
| `luaL_gsub` | the same: PUC-Lua leaves the result behind, and the port took it off |
| `luaL_buffinitsize` and `luaL_pushresultsize` | `luaL_pushresultsize` ignored the size, which is the whole point of it: the extension fills the room itself, and this is where it says how much it wrote |
| `luaL_bufflen`, `luaL_buffaddr`, `luaL_buffsub` | macros over `n` and `b` in `<lauxlib.h>`, so they have to describe the content however it was built. What `luaL_addstring` added lived in a Go builder instead, and all three answered as if it were not there. The content now lives in one block that grows, the way PUC-Lua's does — C memory, since `luaL_prepbuffsize` hands out its address and the extension writes into it after the call has returned |
| `luaL_ref`, then `luaL_unref`, then read the slot | the free list lived in slot 0, where 5.3 kept it; 5.4's is `LUA_RIDX_LAST + 1`, and that slot is a plain key of the table the caller handed in |
| `lua_toclose` and `lua_closeslot` | both were stubs, so `<close>` from C never ran its `__close`. The mark now lives in the sign of the frame's `nresults`, as in C, and `luaD_poscall` closes what is pending before it moves the results |
| `lua_newthread`, then `lua_status`, then `lua_resume` | a thread that has never run is `LUA_YIELD` here, which is what `coroutine.status` needs, but C's `lua_status` answers `LUA_OK` for it and `lua_resume` has to tell "start it" from "wake it up". A thread now carries whether it has ever been resumed |
| `lua_resume` on a thread that yields | C takes the arguments from the coroutine's own stack, leaves the yielded values there and reports how many there are, for the caller to `lua_xmove` across. Neither was true here: the arguments came from the resumer and the values had been taken off |
| `lua_atpanic` | answered NULL instead of the previous handler, which is the one thing an extension installing its own needs in order to put the host's back |

### 5.9 The standalone compiler: luac, and what its listing proves

`cmd/luac` is luac.c, and it is the last thing PUC-Lua ships that was
missing: compiling a script to a binary chunk, checking one without
running it (`-p`), stripping debug information (`-s`), and listing the
bytecode (`-l`, twice for the full listing). The work is done by
`lua.Luac`, which lives in the library rather than in `cmd/luac` because
the listing has to read prototypes — luac.c says so itself by including
`lobject.h`, `lopcodes.h` and `lstate.h`.

The listing is worth having for a second reason: it is a far stricter
conformance check than any output comparison. A script can print the
right thing and still have been compiled differently; a listing cannot.
Every instruction, operand, comment, constant, local variable and
upvalue has to be PUC-Lua's, in PUC-Lua's order, with PUC-Lua's line
numbers. Run over the 80 scripts of the differential suite (§5.10) —
some 81,000 lines of listing — the two agree line for line.

Which is how it found the last real difference in the compiler:

**`addk` was using the wrong table.** `lcode.c` looks a constant up in
the *scanner's* table (`fs->ls->h`), which is shared by every function
of the chunk, while this port had given each function a table of its
own. The consequence is not that one of them shares constants and the
other does not — PUC-Lua's entry holds an index into whichever
function's constant table was being filled at the time, and it is only
reused if that index still names the same value in the function being
compiled now. So PUC-Lua *misses* some reuse that a per-function table
would find, and a chunk like

```lua
local function f() return print end
print(f)
```

gives `print` two constant slots in PUC-Lua and one here. Nothing Lua
can observe depends on it, which is why 80 scripts of golden output
never showed it — but the constant indices are part of the bytecode, so
the bytes `string.dump` writes were not always PUC-Lua's. The table is
now the scanner's, and the occasional duplicate constant is reproduced.

`testdata/luac/` pins it: three sources, their 683-line combined listing
(`expected.txt`, taken from PUC-Lua 5.4.9 with the prototype addresses
blanked out, since those cannot match across processes), and the two
binary chunks `luac -o` writes for them, compared byte for byte and then
loaded back. Several files are combined the way luac.c does it — by
compiling `"(function()end)();\n"` once per file and grafting each
file's prototype onto the nested ones — so that is covered too.

Sharing that table has a consequence, and it is the other half of the
same translation: once a string has been used as a constant, the entry
`addk` wrote for it holds an *integer*, not the string. `luaX_newstring`
therefore cannot read the value back out of the entry to find the saved
copy of a literal — C reads the **key** (`llex.c`:
`keystrval(nodefromval(o))`), and this port has `luaH_getstrkey` for
exactly that. Reading the value instead makes every later occurrence of
a literal that has already been a constant a fresh object, so two equal
long literals in one chunk stop being the same string, which is what
PUC-Lua's own `literals.lua` and `verybig.lua` assert. Both of those
regressed the moment constants started sharing the scanner's table, and
both are green again; `testdata/diff/80_strid.lua` pins it.

### 5.10 Coverage inventory

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

Two things the inventory cannot show about the debug library are worth
saying here, because they are the reason it has a fuzz of its own
(§5.13). Four of its 43 functions — `debug.upvalueid`,
`debug.upvaluejoin`, `debug.setuservalue` and the helper they share —
had never been entered by any test in this repository, and they are
entered now. And an error raised in a hook escapes every `pcall` there
is, which is PUC-Lua's behaviour too and not a place this port could
have been right about on its own.

The same is true of `string.pack`, and it is the same four-plus-seven
functions: `str_pack`, `str_unpack`, `str_packsize` and the seven
helpers behind them — `strGetOption`, `strGetDetails`, `strPackInt`,
`strUnpackInt`, `strPackBits`, `strUnpackBits` and the one that cuts a
format at its zero byte — were all at 0%, the last of `lstrlib.go` no
test had ever reached. A fuzz cannot change that, because it runs the
two interpreters as subprocesses and a subprocess does not answer
`go test -cover`; `TestPackInProcess` is what does, driving the same
functions through the API in this process. They are between 92% and
100% now, and the two things the grid found are pinned in it.

The same was true of the eleven functions that take a name (§5.13):
`osRename` and the `ioRename` behind it were both at 0%, and
`ioStrError` at 54%, for the same reason — a grid cannot reach them
from outside the process. `TestNameInProcess` drives them in this
process, and the two things the name half found are pinned in it: the
`os.rename` onto a directory, and the ENOTEMPTY whose spelling Go's
`errors.Is` had been grouping with EEXIST. `osRename` and `ioStrError`
are at 100% now; `ioRename` is at 67%, its uncovered branch being the
one for Windows, where `rename(2)` is not what a move is.

The same was true of the two functions behind `os.date`, and it is the
same reason: `osFormatSpec` was at 18.4% and `osFormatYear2` at 0%,
every case that entered them being a case in another process.
`TestDateInProcess` asks all thirty-seven specifiers, the 19 `E` and
`O` modifiers, the nine fields of `*t`, the refusals and their
messages, and `os.time`, in this process and in the UTC form — which
is what lets one written answer be the right one on every machine, the
two letters in front of a format being the part the C library decides
(§6.15). `osFormatSpec` is at 95.9% now and `osCheckOption` and
`osStrftime` are at 100%. What the grid cannot hold, this holds
instead: `os.date("!%Y", -65307340800)`, which PUC-Lua answers `-100`,
is a line in it, and it is a line the grid does not have.

The same was true of four functions of `ltablib.go`, and it is four
this time because the table library has more small helpers than the
others do: `tableCheckField`, `tableIncr`, `tableRandomizePivot` and
`tableChoosePivot` were all at 0%, and `tableCheckTab` at 20%, every
case that entered them being a case in another process.
`TestTableInProcess` asks them in this process, and every function of
`ltablib.go` is at 100% now. It is not a second copy of the grid: the
two things it asks that the grid cannot ask outside a subprocess are
the two places `partition` says the comparison function is not an
order, and since both of them say the same thing, what tells them
apart is how many pairs the comparison function was asked about before
it gave up — ten for the one that walks up from `lo`, twenty-six for
the one that walks down from `up`. What it holds besides those is the
thing the table half found (§5.13), the two directions of a `move` and
its two refusals, the positions `remove` and `insert` will take, the
kinds `concat` names when it refuses them, the 28 pairs a sort of ten
elements compares in the order it compares them — the seventeenth of
which is `9<9`, the pivot being compared with itself — a userdata,
which is the only kind of value that is not a table and can be asked
for a metamethod, and the lengths `sort` refuses, which need no array
at all but only a `__len` answering with a number belonging to none.
Every refusal is pinned with the name in front of it, and
the names are not the ones the C source spells: "bad argument #3 to
'**table.move**' (too many elements to move)" comes from
`pushglobalfuncname` searching `package.loaded`, which is the road C
takes when the caller is a C function and there is no calling
instruction to read a name out of, while "wrong number of arguments to
'**insert**'" is a literal in `ltablib.c` and is spelled the short way
in every case. A caller that is Lua code gets the short name too, and a
position in front of the message with it.

Of the 24 things this half has been broken with to see whether the
in-process test can see them, 22 turn it red. The two that stay green
are the two that are §6.18's: a `tableChoosePivot` that ignores the
random number it is handed, and a `tableRandomizePivot` that always
answers 0, are the same thing to an array that comes out sorted either
way, and the two functions are only asked so that they are entered.

It was true four times over of `lmathlib.go`, and there the reason is
that the grid asks its questions in a subprocess for the excellent
reason that the reference interpreter *is* one. `mathFrexp`,
`mathLdexp`, `mathPow` and `mathTanh` were at 0% and a dozen more were
half covered. `TestMathInProcess` asks all 31 names of the library in
this process and every function of `lmathlib.go` is at 100% but one,
and the one is `mathI2d`'s correction for a two's-complement negative:
it is a line of `lmathlib.c` that exists for a `lua_Number` wider than
64 bits, and since this port's is 64, `x >> 11` has 53 bits and `sx` is
never negative — as C says, "a good compiler will remove the
correction". It is kept, and it is the only statement in the package
that is uncovered on purpose.

What the in-process test holds that the grid cannot is the two things
in §5.13: the draws of a generator seeded by the clock, of which only
the shape can be asked — two integers, and the first of them at least
as new as `os.time()` — and the six draws from a seeded one written to
seventeen digits, which is more than `tostring` uses and one more than
a fifty-second bit would survive. What it holds besides is what the
grid would hold too if the grid were not a question about a subprocess:
the shortcut in `floor` and `ceil`, which 2^53+1 is the whole point
of; the two signs a square root keeps, -0.0 having one and a NaN
having neither, seen as the reciprocal of the one and as `x ~= x` of
the other; the four rules of `fmod`, the third of which is that a
divisor of 0.0 is not one; the two positions of `log`'s base; `ldexp`
with an exponent of 2^31, which C cannot hold and so wraps to -2^31;
and `random` with three arguments, the one arity error in the library
whose message names nothing at all.

### 5.11 Performance

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
* **short strings are interned permanently, like C's.** The string
  table holds every short string the state has interned for its whole
  life, so two equal short strings are the same object and
  `lua_topointer` answers the same address for both — which is what
  PUC-Lua does and what lets a short string be a `lua_rawsetp` key. The
  price is that interned strings are not collected until the state
  closes, the same price PUC-Lua pays; Go's collector does not sweep
  the table because it holds a strong reference to every entry.

`Value` is 32 bytes rather than 40 for the same reason C's `TValue` is a
union: a float shares the integer's field, stored as its IEEE bits.

### 5.12 The third-party corpus

Everything above is a case somebody wrote — somebody who was, in every
instance, thinking about one thing. `testdata/third` is the other kind:
five single-file Lua libraries nobody wrote for this port, 2 759 lines
between them, run unmodified and asked to print.

| | Lines | What it is |
|---|---|---|
| `dkjson.lua` | 714 | a JSON encoder and decoder built out of `string.find` and a position |
| `fun.lua` (Lua Fun) | 1 087 | 60 iterators, every one of them a coroutine |
| `json.lua` (rxi) | 388 | a smaller JSON library, asked the same questions |
| `inspect.lua` | 377 | a pretty-printer, which is a table walked and printed |
| `middleclass.lua` | 193 | object orientation, which is metatables all the way down |

What is under test is not any of them. It is the machine they run on,
asked the way a real program asks it: a scanner that keeps a position
and `find`s the next token, a string escaped through `gsub` and a table
of replacements, a metatable chain three deep with `__index` on every
level, an iterator chain that is five coroutines suspended inside one
another. A bug that needs three features to line up before it shows is
the kind neither a per-function case nor a fuzz over one library's
arguments is looking for. `drive.lua` (517 lines) asks 264 questions and
prints the answers; `TestThirdPartySuite` compares them line for line
against PUC-Lua, golden captured from `LUA_REF` as everywhere else.

Two things had to be taken out before any of that was comparable, and
both are the sort of thing only a real library brings out:

* **A table is printed with its keys sorted.** PUC-Lua seeds its string
  hashes from the time and an address, so `pairs` hands out the keys of
  the same table in a different order on every run — of the reference
  as much as of this port. Three runs of the reference over `{a..f}`
  answered `cdefab`, `fedcba`, `afdebc`; three of this port answered
  `badcfe`, `fabcde`, `facbed`. A JSON encoder and a pretty-printer
  *are* a loop over `pairs`, so without this the corpus would be
  comparing the seed.
* **A function is reported by `type`, not `tostring`.** The address a
  function prints with is the host's, and differs between the two
  binaries by gigabytes. `inspect` prints `<function 1>` — a counter
  per process — so that one is fine, but `dkjson.use_lpeg` printed a
  raw address and had to be asked differently.

The answer it gives today is: **no difference**, in all 264. That is
worth less than it sounds unless the corpus can be shown to be able to
say otherwise, so it was checked the way every fuzz here is checked, by
breaking something on purpose and watching it go red:

* `fmtFloat`'s `%.14g` cut to `%.13g` — 40 lines red, naming
  `dkjson/encode 0.33333333333333`.
* `%q`'s special case for `\n` removed from `addquoted` — 86 lines red,
  starting at the case whose subject contains a newline.

The first of those is also a small lesson in how to read a green run.
The corpus as first written — 228 cases, none of them a number with
more than three significant digits — did **not** go red under that
mutation. It was not evidence that number-to-string is right; it was
evidence that nothing had asked. Twelve long values (`1/3`, `math.pi`,
`0.1`, `2^53`, `1e-15`, `12345678901234`, …) went in because of it, and
the corpus has been sensitive to that digit ever since. "No difference
found" is only as strong as the questions that were asked.

The five libraries are also in the codegen fuzz's source list, so what
is compared is not only what they print but what they compile to: 122
sources, 17 197 581 bytes of byte code, PUC-Lua's on both sides.

All five are MIT, and each one's license is checked in beside it
(`LICENSE_*.txt`). None of them is vendored into the library itself —
they live under `testdata` and nothing outside the tests can see them.

### 5.13 The differential fuzz

Everything above is a case somebody wrote. This is the other kind: cases
built in the test, none of them chosen, run through the reference
interpreter and through `./cmd/lua` and compared line by line. It needs
`LUA_REF`, so it runs in CI rather than on every machine, and it is
exhaustive rather than random, so the same cases come back every run and
a difference is always reproducible.

| | Built | Answers compared |
|---|---|---|
| `fuzz_test.go` | 72 463 expressions — 43 operands against every operator, and each of those again in a form the VM has to run at runtime instead of folding while it compiles, plus the calls that take a base or a format — and 201 written-out snippets for what combination cannot reach: syntax errors, metamethods, coroutines, hooks, patterns, non-string errors, stack overflow | 72 664 |
| `patternfuzz_test.go` | 6 991 patterns — every pattern atom against every atom, plus the triples that need three pieces to mean anything — against 25 subjects | 1 048 650 |
| `formatfuzz_test.go` | 11 340 format strings — every flag against every width, precision and conversion — against 22 values, plus one line each with the value left out | 260 820 |
| `numfuzz_test.go` | 1 834 strings read as numbers — every sign, integer part, fraction and exponent in combination, then the same for hexadecimal, then each wrapped in nine different kinds of space — and 49 values written back by `tostring`, `%q`, `math.tointeger`, `%g` and `%e` | 11 053 |
| `forfuzz_test.go` | 16 277 numeric `for` loops — 25 values in each of the three positions: zeroes of both signs, integers and floats, both ends of the integer range, the floats just outside it, the point where a float stops counting by ones, and the three values that are not comparable at all — each asked with the step written out and with it left to default to 1, plus each of 9 values that are not numbers in each of the three positions | 16 277 |
| `codegenfuzz_test.go` | 101 356 snippets — every literal, every operator against every shape of operand, every statement form both bare and inside a function, every shape a closure can capture, 40 things that must not compile, and then every expression of depth two over 20 atoms and 18 operators plus a seventh of depth three, each in 22 statement positions — plus all 122 sources in `testdata` — official, ours, and the five third-party libraries of §5.12. What is compared is not what they print but **what they compile to**: 17 109 752 bytes of byte code | 17 109 752 |
| `iofuzz_test.go` | 3 166 things the `io` library can be asked — every mode of length 0 to 3 over `r w a + b t x` against a file that exists, one that does not and a directory; every format `read` takes (each letter of the alphabet in both cases, the four C asks for, their `* ` spellings, and the counts and the values that are not formats) against 10 fixtures, and the same three at a time; what `write` puts in the file, byte for byte, for each kind of value and for two of them; `seek` from each of 8 whence by each of 17 offsets, before and after a read; `setvbuf`; `lines` and `f:lines`; 13 kinds of handle — the three standard ones, a file opened each way round, a closed one, `tmpfile`, both ends of a `popen` — used 17 ways each; `io.input` and `io.output`; and the errors, `<close>`, and dropping a handle without closing it. Both interpreters get an identical copy of the fixtures in a directory of their own | 3 166 |
| `debugfuzz_test.go` | 1 333 questions about the debug library — every option string of length 0 to 2 over the eight letters `getinfo` recognises and the one it does not, asked about each of 8 kinds of function; every stack level from each of 7 shapes of stack, which is the only way to reach a `name`, a `namewhat`, a `currentline` that moves and an `istailcall`; a coroutine in each of 5 states; the same function called 10 ways, asked about itself and about its caller; every local index in every shape of frame, on the main thread and on a coroutine, and `setlocal` for each; every upvalue got, set, identified and joined — including the 25 ways two closures can be joined and the 8 ways two upvalues can be the same one; 56 hook masks, 6 counts, `gethook` on the main thread, on a coroutine, and on a coroutine made before or after the hook was set; every `traceback` message at every level, from every depth; every metatable, the registry, and every user value — plus every debug function asked the wrong thing. And 12 hook errors, each in a process of its own, because an error in a hook escapes every `pcall` and ends the run | 1 333 + 12 |
| `packfuzz_test.go` | 3 733 questions about `string.pack`, `string.unpack` and `string.packsize` — every byte of the alphabet as a format of its own, all 256 of them and not the 26 C looks up, because a byte that is not an option is an option refused and the refusal is printed with the byte in it; every size from 0 to 17 of `i`, `I`, `s` and `c` — the sweep running past 16 in both directions, because only the sizes that are refused can say where the edge is — each with 12 values and with the three that sit on the edge of that size; every alignment `!` can ask for, over 20 things to align and then over every pair of 6; every endianness over every option; every pair of 16 options, and an `X` between every pair of 12; `unpack` at 15 positions of 12 formats; every option given 8 wrong things, and given nothing; 138 round trips; 53 written-out formats; a zero byte at every position of 9 formats; a zero byte in a value; and every size read out of 6 buffers of raw bytes | 5 337 |
| `namefuzz_test.go` | 1 328 questions about a name with a zero byte in it — every name a script can hand to the OS, with a zero at every position of five of them (`a`, `a.txt`, `sub`, `no`, `m.lua`), asked of `io.open` in each of its 6 modes, of `io.lines`, `loadfile`, `dofile`, `os.remove`, `os.rename` (in the name moved, in the name it is moved to, and in both), `require` and `package.searchpath`, each against a file that is there, a directory, and nothing; then the same renames and removals with no zero in them, which is where a directory meets a directory; a name with a slash on the end of it, which is not a name a regular file can be given; and 62 commands with a zero in them, run by `io.popen` and `os.execute`. 1 218 names and 62 commands, each with a number of its own so that one case's file is never another case's, and both interpreters get an empty directory of their own | 1 328 |
| `corofuzz_test.go` | 396 coroutines — six things a coroutine can do (suspend, fail, finish, suspend and then either, suspend twice) in each of 22 places it can do them: bare, under one and two `pcall`s, under `xpcall`, in every metamethod Lua itself calls, in a `for` iterator, in the callbacks of `table.sort` and `string.gsub`, in the reader `load` asks, in a coroutine of its own, and with a to-be-closed variable still pending — each driven three ways: resumed four times, driven through `coroutine.wrap`, and closed with `coroutine.close` | 396 |
| `tablefuzz_test.go` | 1 005 questions about the six functions of the table library — every `f`, `e` and `t` of a `table.move` over a table of four, to itself and to another table; every position a `table.remove` and a `table.insert` can be given, at every size up to four, and the number of arguments they take; every kind of value in every position of a `table.concat`, every kind of separator and every pair of ends; every pair of ends of a `table.unpack` and four that are past the end of anything, and `table.pack`; 218 `table.sort`s — 52 with a comparison function that writes down every pair it is asked about in the order it is asked about them, 69 with one that claims a different element is the pivot, 45 with one that says so of an element there are two copies of, 15 of a `__len` that answers with a length belonging to no array — 13 of them on a table and two on a userdata — which is the only way to reach "array too big", and the ones that raise, the ones that move the array while it is being sorted, and the ones that are not orders; and 142 things that behave like a table and things that do not — 64 of them userdata with each of the eight combinations of the three metamethods, in each of the six functions and as both ends of a `move`, 30 of them tables with a metatable on them, and 48 that are not tables at all | 1 005 |
| `mathfuzz_test.go` | 4 787 questions about the maths library — every one of 41 values there is a name for, the 31 numbers and the 9 strings that read as them: both zeroes, the two ends of the integers, the three sizes at which a float stops counting by ones, both infinities, the NaN and π — through each of `abs`, `floor`, `ceil`, `sqrt`, `tointeger` and `type`; every pair of 15 dividends and 15 divisors of a `fmod`; every pair of the 41 through each of `max` and `min`, and every triple of three near 1 through each; every pair of 8 through `ult`; every one of the 41 through `modf`, and through `frexp` besides the 18 powers of two that are asked of it on their own; every one of the 41 through each of `deg` and `rad`; 18 mantissas through `ldexp` with each of 18 exponents, the last four of which do not fit in the `int` C truncates to; 151 draws from a generator seeded with two integers, in each of 15 shapes and after each of 5 single-integer seeds; every form of `randomseed` and a draw after each of 7; 160 things that are not numbers — a nil, a boolean, a string, a table and a function — asked of every one of the 31 names in the library, and of `log` in both of its positions; and 28 of those names given nothing at all, `fmod` being given one argument instead of none | 4 787 |
| `datefuzz_test.go` | 2 517 questions about `os.date` and `os.time` — every conversion specifier of C99's `strftime`, all 37 of them, asked of every day of 2000 and of 2001 and of every noon of 2000, of every hour and every five minutes of one day, and of the instants at and past the ends of a `struct tm`; the 19 `E` and `O` modifiers at four instants; the nine fields of `*t`; the 51 formats that are not specifiers, 18 of them with a zero byte in them; the format that is not there at all; and 124 tables for `os.time`, two of them only on a host whose `mktime` reads `isdst` the way this port does. Every one of them asked in each of four zones — one with no summer time, one west of Greenwich, one east, and Dublin, whose standard time is the smaller of its two — because four of the specifiers are questions the zone answers | 2 517 × 4, or 2 523 × 4 where the C library is glibc's |

The pattern half exists because `lstrlib.c`'s `match` is two hundred
lines of exactly the kind of logic nobody writes down the combinations
of: a frontier followed by a set, a quantifier on a position capture,
`%b` with a delimiter that is also a magic character. `32_pattern.lua`
and `72_gsubbit.lua` are 366 lines of cases somebody *did* think of, and
they pass; this is the rest.

The format half exists because `string.format` is the one place here
that is not a translation of C at all. Everything else hands a format to
Go's own `fmt` or reads a number the way Lua does; `str_format` has to
reproduce **C's** `printf`, including the parts of it that are decided
by the library rather than by the standard — and it is therefore
hand-written: `strParseFormat`, `strPadField`, `strFormatUnsigned`,
`strHexFloat`. It found two real differences, both now fixed and pinned
in `lstrlib_test.go`:

* **The value is checked before the specification.** In C, `d`, `i`,
  `u`, `o`, `x`, `X` all go through `intcase`, which is
  `luaL_checkinteger(L, arg); checkformat(...)`, and `e`, `E`, `f`, `g`,
  `G` read the number the same way round — so `%#d` with `1.5` answers
  "number has no integer representation", and only `%#d` with a real
  integer answers "invalid conversion specification". Here the
  specification was being judged first, so every one of those reported
  the wrong complaint. `c` is the other way round and stays that way:
  `%#c` with a string complains about the specification.
* **`%a` rounds on more than the digit it drops.** Half of the last digit
  kept is 8, so a first dropped digit above 8 rounds up and one below
  does not — but a first dropped digit of exactly 8 is a tie, and the
  tie is only a tie if everything behind it is zero. Deciding from that
  digit alone made `%.8a` of `1e-300` answer `0x1.56e1fc2fp-997` where C
  answers `0x1.56e1fc30p-997`: the digits are `56e1fc2f|8f359`.

Three things the format half cannot ask, and says so rather than quietly
passing them:

* `%s` of a value whose `print` is an address. Padding is decided from
  the address's real length, and the normalisation that makes addresses
  comparable rewrites it afterwards, so a table is not in the value
  list — not because it differs, but because no answer could be right.
* **A `%a` tie on an odd digit**, which is the one rounding the C
  libraries do differently: glibc hands the last digit kept to
  `round_away`, which leaves an even digit alone and steps an odd one,
  while Apple's library rounds a tie *down* whatever that digit is.
  (§6.11)
  `%.0A` of `1.5` is `0X2P+0` under the first and `0X1P+0` under the
  second, and no port can answer both. This port answers `0X2P+0` — what
  glibc answers, what the C library's own `%f` answers on every platform
  (`%.0f` of `1.5` is `2`), and what makes `%a` agree with `%f` inside a
  single call. The fuzz therefore asks the reference that one question
  first, and on a reference that answers `0X1P+0` it leaves the 720
  grid cases that turn on it out of the comparison, on both sides. A tie
  on an *even* digit is where both rules land anyway, so those are
  compared like any other case, and `TestFormatHexFloatTieRounding`
  pins both halves against the reference.
* **A subnormal number**, whose leading digit C leaves the library to
  choose: `5e-324` is `0x1p-1074` here and `0x0.0000000000001p-1022`
  under glibc. (§6.12) The two are handled by the same question put to
  the reference at the top of the run — one probe each, each with its
  two possible answers written out, and a reference that answers
  anything else fails the test rather than being compared anyway.

The number half is the same comparison over the other way round a number
becomes a string and back, and it is where a Go detail had been showing
through in three separate places. `tonumber("1_000")` answered `1000`
here and `nil` in PUC-Lua, because Go's `strconv.ParseFloat` accepts the
underscores of a Go literal and C's `strtod` does not.
`tonumber("\u{3000}1")` answered `1` here and `nil` there, because
`strings.TrimSpace` trims the Unicode spaces and C's `isspace` does not —
and the hexadecimal path, a second parser for the numerals Go will not
take without an exponent, had its own copy of that second trim. All
three are fixed and pinned in `lobject_test.go`. A sixty-string probe
found the first two, which is why the grid was built at all. What the
number half deliberately does not ask is a *computed* value: `math.pi`
is a constant and is fine, `math.exp(1)` is not, because Go's `math` and
the platform's libm disagree in the last ulp (§6.7) and the fuzz would
then be reporting the maths library rather than the conversion.

The expression half asks every operation twice, and the second form is
the one that matters. `A op B` where both operands are numerals is not
a runtime operation at all: `lcode.c`'s `constfolding` folds it while it
compiles, so what runs is a constant, and `OP_ADD`, `OP_IDIV` and their
neighbours are never reached. The second form —

```lua
(function() local a, b = A, B return a op b end)()
```

— is the same operation arrived at with two values in registers, which
is where a string is coerced to a number, where an integer division by
zero raises, and where the two would answer differently if only one of
them were right. A grid of one is a grid of the compiler. This half was
probed on its own first, over 30 258 runtime expressions, and did not
differ; it is in the grid so that it stays that way.

The `for` half is the same comparison over the one loop Lua decides
before it runs. `OP_FORPREP` and `OP_FORLOOP` say whether a loop runs at
all, what its control variable is, and when it stops — in integers,
where the counter can wrap past `maxinteger`, and in floats, where a
step can round the index past the limit — and nothing about that is
written down anywhere but in `lvm.c`. A probe of 16 250 loops found one
case in 60 where the two disagreed:

```lua
for i = 0, 1, 0/0   -- PUC-Lua does not run it; this port ran it once
```

C decides whether to skip the loop with
`luai_numlt(0, step) ? luai_numlt(limit, init) : luai_numlt(init, limit)`,
which is a **ternary**, and this port had written it as the pair of
conjunctions that reads the same — `(step > 0 && limit < init) ||
(step <= 0 && init < limit)`. They are the same only while `step` is a
number comparable with zero. A NaN is neither greater nor less than
zero, so C takes the *else* branch and asks `init < limit`: `0 < 1`,
and the loop is skipped. `step <= 0` is false too, so the port took
neither branch and ran it. `floatforloop` had the same rewrite of the
same ternary.

It is not a rule about NaNs so much as a rule about *translating*: a
ternary whose condition is a comparison is not the same as the
conjunction it looks like, because a comparison that is false does not
mean its negation is true. Both are fixed and pinned in
`TestForNaNStepPinsTheTernary` — twelve answers, not all one number,
because a NaN *limit* with an integer start and step is not the float
path at all: `forlimit` cannot coerce it and truncates it to
`mininteger`, or skips the loop, depending on the step's sign.

The io half was written last and found the most. It was written because
`liolib.go` was the one file whose coverage stayed under 55% — 79
functions, 28 of them never entered — and it went red six times, all
six in the same way: C asks the FILE a question, and the port asked the
descriptor behind it.

* **`io.open` with a mode C's grammar refuses.** `l_checkmode` is
  `[rwa]%+?b*` — `L_MODEEXT` is `"b"` and nothing else — so `"rt"`,
  `"wt"`, `"at"` and `"r+t"` are "invalid mode" in PUC-Lua however
  willing every C library is to take them. The port accepted `t`.
* **Writing to a handle that was not opened for writing.** C's
  `cantwrite` reads the flags `fopen` set in the FILE and fails with
  EBADF before a byte moves, so `io.open(f, "rb"):write("x")` answers
  `nil, "Bad file descriptor", 9`. A Go file fails only at the system
  call, and the write buffer swallows a short piece on its way there.
  `cantread` is the same the other way round: `io.popen(cmd, "w"):read("a")`
  is not the empty string and `io.stdin:write("z")` is not a write.
* **A failed write reported the wrong errno.** `g_write` threw the real
  error away and handed `luaL_fileresult` a made-up one, so every
  failure answered "(no extra info)" with an errno of 0 — including the
  one above, which was therefore invisible.
* **A name with a zero byte in it.** `fopen` is handed a C string, so
  `io.open("a\0b", "w")` makes a file called `a`; the port passed the
  whole name to Go, which answers "Invalid argument" for it.
* **A mode that appends.** `fopen` with `"a"` leaves the file position
  at the end, which `O_APPEND` does not — it decides where a *write*
  goes and leaves the position at zero — so `io.open(f, "ab"):seek()`
  is the size of `f`. With `"a+"` the two C libraries disagree, and C
  asks only that a write land at the end: Apple's puts the position
  there too, so `io.open(f, "a+"):read("a")` answers nothing at all,
  while glibc leaves it at zero and answers the whole of `f`. The port
  follows the host's library, which is what PUC-Lua on that host does,
  and the grid — which asks the reference on the machine it runs on —
  is answered alike on both.
* **`fseek` that fails, and `fseek` that wraps.** A seek that fails
  changes nothing, and the read-ahead sitting in the buffer is part of
  what it does not change: `f:read("l"); f:seek("set", -1)` fails, and
  the rest of the file is still there to read. A `"cur"` offset of
  `math.maxinteger` is handed to `lseek` as it stands, so the kernel is
  what answers EOVERFLOW — the port added the buffer's read-ahead to
  it first, wrapped to a negative number and answered EINVAL. And a
  pipe is ESPIPE ("Illegal seek", 29), not a made-up error whose errno
  is 0.

The one that is not about a question C asks the FILE is the one that
cost the most to find, and it is the one worth remembering: **a handle
that was never closed**. `f:close` was the only thing that closed a
file, so a handle that was dropped kept whatever was still in its write
buffer, and those bytes were lost. `__gc` was never running for a
file, and the reason is Go's: `runtime.SetFinalizer` does not fire for
an object in a reference cycle, and the stream behind a handle pointed
back at its userdata, which made the pair one. Two objects that point
at each other, one of them with a finalizer, are never collected at
all — not after one cycle, not after ten. The pointer back is weak
now, which turns the cycle into a tree, and the grid asks it: a handle
dropped inside a `do ... end` writes its bytes out.

The debug half found the two things below, one in
the library and one outside it — in the interpreter's own 'main', which
is the one place a fuzz is not expected to reach. `ldblib.go` had four
of its 43 functions that no test had ever entered — `debug.upvalueid`,
`debug.upvaluejoin`, `debug.setuservalue` and the helper they share —
and a probe of 258 cases over the whole library went red in one place.

* **A coroutine made while a hook is on inherits it.** `lua_newthread`
  copies `hook`, `hookmask` and `basehookcount` into the new thread, so
  `debug.sethook(f, "crl", 2)` and then
  `debug.gethook(coroutine.create(...))` answers `nil, "crl", 2` — the
  mask and the count of a hook nobody ever set on that coroutine. The
  port kept the mask and the count in a map of its own, keyed by the
  thread, and a coroutine that had never been given one was not in it.
  The map was also a leak C's design does not have: its keys were
  strong, so every coroutine that ever had a hook stayed reachable for
  as long as the process did, while C's hook table is weak
  (`__mode = "k"`). All three are on the thread now.
* **An error that escapes `pmain` printed as `lua: ` with nothing after
  it.** `lua.c`'s `main` reads the result with `lua_toboolean` — which
  is a *read* — and then calls `report`, and `report` is what removes
  the error object. This port popped it first and then reported, so
  `report` was holding whatever sat underneath. Only a hook can make an
  error reach that far: a `return` hook fires on the return of `pmain`
  itself, which is outside every protected call the interpreter makes,
  so the raise escapes that one too and `main` reports it a second
  time, bare. Which is why this is also the one way to see a property
  of hooks that is easy to get wrong: **an error raised in a hook
  escapes every `pcall`, every `xpcall` and `coroutine.resume` alike.**
  PUC-Lua does the same, so `TestDebugHookErrors` asks those twelve in
  twelve processes and compares the exit status as well as the output —
  they cannot be lines in the grid, because one of them would be the
  last thing the run ever printed.

The pack half is over `string.pack`, `string.unpack` and
`string.packsize` — the last C mini-language in the standard library
without a grid of its own — and it found the two things below. A format
string is a program
in sizes, alignments, endianness and options, and `lstrlib.c` is an
interpreter for it. A probe of 907 hand-chosen cases over the three went
**0 lines red**, and by the rule of this project that is not yet an
answer, so the same corpus was asked twice with `lstrlib.go` broken on
purpose and it saw the break both times: the sign extension in `packint`
taken out, 36 lines red; `*ntoalign` in `getdetails` forced to 0, 4
lines red. Four lines out of 907 is what a hand-picked probe misses, and
the grid is what was built for them. It found two.

* **A format ends at its first zero byte, not at the end of the string.**
  `str_pack`, `str_packsize` and `str_unpack` walk a format with
  `while (*fmt != '\0')`, so `string.packsize("i4\0i4")` is 4 and
  `string.packsize("\0i4")` is 0, and a zero byte on its own is an empty
  format rather than a refused one. A Go string is a length and some
  bytes, so this port walked past the zero into the bytes after it and
  found a format option that is the zero byte, which is not one.
  `string.format` is not like this — C walks *its* format with
  `strfrmt < strfrmt_end`, the end being the length — so that one keeps
  its zero bytes, and the difference between the two is now one line in
  one place.
* **A length read out of the data is compared as a `size_t`.**
  `str_unpack`'s `Kstring` case is
  `size_t len = (size_t)unpackint(...)` and then
  `len <= ld - pos - size`, all three of them unsigned. Read out of
  eight bytes of `0xff` that length is 2^64−1 there and −1 here, and −1
  is less than whatever is left of the string: the comparison was the
  wrong way round, and the slice behind it went out of range.
  `string.unpack("s", ("\xff"):rep(16))` is refused with "data string
  too short" in PUC-Lua and was a Go runtime panic in this port.

The second of those is also the lesson of the half, and it is the one
the third-party corpus had already taught once. The grid only ever
unpacked what it had just packed, and what `pack` writes is always
well-formed: the bytes it puts above the eighth are the sign extension,
so the check that there is one can never fail. Deleting that check
turned the grid **0 lines red** — not because the check is invisible but
because nothing had asked. A group that reads every size from 9 to 16
out of six buffers of raw bytes took it to 56.

The name half is over the one thing every one of those functions has in
common: a name a script hands to the OS. A C
string ends at its first zero byte, and every one of these names is a C
string by the time it reaches anything — `fopen`, `open`, `remove`,
`rename`, `system` and `popen` are C functions, and the strings
`require` and `package.searchpath` put together are built with
`luaL_addgsub` and `luaL_addstring`, which are `strlen`. So
`io.open("a\0b", "w")` makes a file called "a" and `os.remove("a\0b")`
removes it and says so, the name in the message being "a" because `%s`
is `strlen` too.

A Go string has no such end, and what this port used to hand the whole
thing to does not truncate it — it refuses it. Every path through
`syscall` runs through `BytePtrFromString`, which is EINVAL for a
string with a zero in it, so the port answered "Invalid argument" for
every one of them while PUC-Lua went on and did what the truncated
name asked for. A probe of 165 cases went red in seven of the eleven
functions; `io.open` and `io.lines` were already right, the io half
having found the same thing in the eighth fuzz. The grid found two
more, and neither of them has a zero byte in it.

* **`os.rename` onto a directory.** Go's `os.Rename` asks whether the
  name it was given is free before it calls `rename(2)` at all, so a
  directory it is told to move onto a directory that is there answers
  EEXIST — and, worse, the rename does not happen. C's `rename` is
  asked nothing and answers EISDIR, or ENOTDIR, or ENOTEMPTY. `ioRename`
  in `liolib.go` calls `syscall.Rename`, which is `rename(2)`.
* **The spelling of an errno Go groups with another one.** `ioStrError`
  used `errors.Is(err, fs.ErrExist)`, and `Errno.Is` answers that
  ENOTEMPTY *is* `fs.ErrExist`: true of what a Go program is asking,
  false of what `strerror` says. A rename of one directory onto another
  that is not empty, and a `remove` of a directory with something in
  it, both answered "File exists" where PUC-Lua answers "Directory not
  empty". The same grouping says EPERM is `fs.ErrPermission`, so a
  permission failure would have been "Permission denied" where C says
  "Operation not permitted". Go spells every errno exactly as C does
  with the first letter in lower case, so the errno's own text, raised
  by that one letter, is what `ioStrError` answers now.

Two things the half does not do, and says so rather than quietly
passing them. It does not ask `io.popen` or `os.execute` to run a
command that says anything on the standard error: C's standard output
is a FILE with a buffer of its own and this port writes straight to the
descriptor, so the two would differ in the interleaving alone. Every
command is run as `exec 2>/dev/null; ...`, which swallows the "command
not found" most truncated commands produce, and the exit status still
tells the truncated names apart — "" is 0, "t" is 127, "tr" is 1,
"tru" is 127 and "true" is 0 — which is the whole point of the cases.
And it does not ask `os.rename` to move a name onto one the grid has
not made, because a rename that succeeds is one the next case would
otherwise see.

Every one of the twelve things the half fixed has been taken back out
and the grid asked again, and it saw all twelve: the shared cut, 1 280
lines; `ioCStr`, 483; `ioStrError`'s errno, 364; `pkSearchPath`, 231;
`osRename`, 126; `baseLoadFile`, 105; `baseDoFile`, 105; `pkRequire`,
66; `osRemove`, 63; `ioPOpen`, 44; `osExecute`, 18; `ioRename`, 8.

The codegen half is the only one that does not ask what a program
prints. It compiles every snippet and every source in `testdata` on
both sides and compares the **bytes**. Byte code is a far stricter
answer than output: a difference in register allocation, in whether a
jump is threaded, in whether two constants are folded, in the order of
the constant table, in the line info, or in an upvalue's index changes
the bytes while leaving what the program prints exactly the same — and
each of those is somewhere a translation can be wrong and still look
right. Where a snippet does not compile at all, the complaint is
compared instead, which covers the lexer's and the parser's error
messages over the same grid. It is cheap for what it covers: one
process per side, six seconds, for seventeen million bytes. A failure
names which snippet's bytes part and where.

The one thing it cannot ask is `'^'` on two numerals. Folding turns it
into a constant, and `pow` is the one folded operation whose answer is
not decided by IEEE arithmetic — it is `pow`, and §6.7's disagreement
between Go's and libc's then lands in the byte code: `2 ^ 2.5` folds to
`0x1.6a09e667f3bcdp+2` under C and `0x1.6a09e667f3bccp+2` here. It was
the single difference in the 103 786-snippet probe this corpus grew
out of, and it is the only way two identical programs can compile to
different bytes. Everything else that folds is exact: the arithmetic
operations are IEEE, `//` and `%` are exactly defined, and the integer
and string conversions are decided by the representation.

The date half is the newest, and it is over `os.date` and `os.time` —
the last C mini-language in the standard library without a grid of its
own, and the one part of it where a good many of the answers are not
PUC-Lua's to give. A conversion specifier is a question about an
instant, and six of the things it can be asked are things this project
is measured against two C libraries to answer, and the two disagree.
Four of the six are conversion specifiers and two are `os.time`'s; they
are written up in §6.15 and §6.16 with both answers, and none of them
is asked where it is a question no port could answer.

Two of the four specifiers are asked nowhere: `%Z` and `%z` under `!`,
and `%z` at an instant the timezone database does not reach. A third
never comes up, because every instant in the grid is in a year of four
digits. The fourth, the year at the very top of a `struct tm`, is
asked on a host whose C library keeps the year in 64 bits and not on
one that does not — and the table the same instant breaks down into is
asked on both, because PUC-Lua writes that one itself. Of `os.time`'s
two, the tables for 1899 are asked only where `mktime` accepts a year
before 1900, and `isdst = true` only where `mktime` steps by an hour.
The grid therefore asks the reference three questions before it builds
itself, to find out which C library it is running over, and it is 2 517
questions in each of four zones on macOS and 2 523 on Linux — the
difference is the twelve 1899 tables, which are asked there and not
here, less the six that are asked here and not there. What is
left is asked in each of four zones — one with no summer time at all,
one west of Greenwich, one east, and Dublin, whose standard time is
the smaller of its two — because a specifier that is about the zone is
a different question in each of them.

A probe of 1 643 cases went **0 lines red**, and thirteen things were
then taken out of `loslib.go` one at a time and it stayed at 0 for all
thirteen — verified afterwards by hand, because `%U` at the first of
January 2001 is 01 in one of those and 00 in the other. A probe that
asks nothing is not a probe that found nothing, so the grid was built:
every day of two years, the one whose first of January is the Monday
`%U` and `%W` disagree about, at midnight and at noon; every hour and
every five minutes of one day; the ends of a `struct tm`; and 124
tables for `os.time`.

It found four things, and they are all about how wide something is or
where it stops.

* **`%y` and `%g` carried the minus sign of a year.** They are the year
  without its century, which is the remainder of a signed division by a
  hundred, and C takes the absolute value of it before it writes it two
  digits wide: for the year -2147481748, PUC-Lua answers 48 and this
  port answered -48. Go's `%y` keeps the sign. (There is a second half
  to this one, and it is the reason the grid does not ask it: for the
  year -27 the two C libraries say 27 and 73.)
* **`%Y` is four characters wide and not four digits wide.** The sign
  is one of the characters, so the year -101 is `-101` and the year 1
  is `0001`; Go's layout pads the digits and leaves the sign outside
  them, which is `-0101` for the first of those. The grid cannot see
  this either, for the same reason as the one above — but
  `TestDateInProcess` can, because it holds a written answer instead of
  comparing two, and it asks four instants in years that divide by a
  hundred, which is exactly where the two C libraries agree.
* **The table is recognised with `strcmp`, and a format is walked by
  its length.** Those are two separate rules in C and this port had one
  of them: `os.date("*t\0junk", t)` is the table in PUC-Lua, because
  what `strcmp` sees is `*t`, and it was an error here.
* **The message of a refused specifier is built with `strlen`.**
  `lua_pushfstring`'s `%s` is `strlen`, and what it is handed is a
  pointer into the middle of the format, so what comes out is the
  bytes up to the first zero and not the rest behind them:
  `os.date("%\0Y")` is "invalid conversion specifier '%'" and was
  "invalid conversion specifier '%\0Y'".

It also took one thing out. PUC-Lua's `LUA_STRFTIMEOPTIONS` is
`aAbBcCdDeFgGhHIjmMnprRStTuUVwWxXyYzZ%` and there is no `s` in it, so
`%s` is a refused specifier and the branch that would have answered it
was dead code.

Of the 30 things the half has been broken with — `%U`'s shift, `%j`,
`%V`, `%G`, `%g`, `%u`, `%e`, `%c`, `%p`, `%r`, `%b`, `%H`, `%I`, `%Y`,
`%d`, `%x`, `%n`, `%z`, the two fields of `*t`, both of the two rules
above, the range a `struct tm` holds, the format that is not there,
and the four ways `os.time` can read `isdst` — 28 turn it red: `%U` and `%W`'s shift, 2 926 lines; `%j`, 8 884;
`%n` and `%t`, 8 884; `%x` and `%D`, 8 884; `%V`, 8 900; `%g`, 8 884;
`%G`, 8 884; `%Y`, 8 924; `%p`, 9 124; `%b` and `%h`, 8 140; `%H` and
`%I`, 5 482 each; `%r`, 5 471; `%z`, 2 847; `%u`, 1 265; `%e` and
`%c`, 2 624 each; `%d`, 2 624; the two `*t` rules, 12 each; the range,
16; the missing format, 16; the two `*t` fields, 152 each; and the four
`isdst` ones, 5, 5, 2 and 3. The two
that stay green are `%y`'s absolute value and `%C`'s two characters,
and they stay green for the same reason the grid stops where it does:
the only instants that could tell them apart are instants the two C
libraries answer differently. `%y` is held by `TestDateInProcess` at
the four it can hold it at, and `%C` is not held by anything — it is
`%02d`, which is what the C library this project is developed against
prints, and it is the one thing in this repository no test covers.

The table half is newer still, and it is over the six functions of
`ltablib.c` — the one standard library that every program uses and
nobody thinks about. They are small enough to hold in your head, which
is exactly how a port gets one of them wrong without a test noticing:
`table.move` copies a range in one of two directions and only one of
them is right when the range overlaps the place it is going;
`table.remove` and `table.insert` keep the same rule about which
positions are in bounds, and the one thing they disagree about is an
argument they were not asked for — `remove` ignores it and `insert`
refuses it, in the only message that spells a name the short way;
`table.concat` names the type of the value it refuses; and
`table.sort` is a quicksort whose pivot and whose order of comparisons
a caller can watch, with a comparison function that keeps a tally.

A probe of 209 hand-picked cases went **1 line red**, and that one is a
Go habit that is not a C habit: `tunpack` reads its last argument with
the macro `luaL_opt(L, luaL_checkinteger, 3, luaL_len(L, 1))`, whose
default is an expression that is evaluated only in the branch that
needs it, while a Go call evaluates every argument before it is
entered. `table.unpack(nil, 1, 3)` is therefore "attempt to index a nil
value" in PUC-Lua, which never gets as far as the length, and was
"attempt to get length of a nil value" here. It is the only place in
the package where that shape occurs, and the grid holds it now: with
the fix out, 16 lines go red.

Two things the grid does not ask, and it used to be three. A
`table.move` that would move a huge number of elements hangs **PUC-Lua
itself** — `table.move({}, 1, math.maxinteger, 1)` passes both of its
checks and then copies for the rest of time, on every machine — so
there is no answer to compare against; what the two checks say is asked
instead. And a `table.sort` of more than 128 elements whose comparison
function cares about the order it is called in is left alone for the
reason in §6.18.

The third was "array too big", and it turned out to be askable after
all. What the check is on is the *number*, asked for before a single
element is looked at, so what it needs is not an array but a length,
and a `__len` metamethod can answer with one that belongs to no array:
`setmetatable({}, {__len = function() return math.maxinteger end})` is
refused without anything being built. It is `<` and not `<=`, so
`INT_MAX` itself is refused too; and the lengths that are let through
reach whatever is there, which is nothing — `2` is "attempt to compare
two nil values" and `1.5` is "object length is not an integer", the
latter from the `luaL_len` behind `aux_getn` and not from the sort at
all. Fifteen of the 1 005 cases are those lengths, two of them on a
userdata, which `checktab` has to let through before the length is even
looked at.

Of the 38 things this half has been broken with — both directions and
all three conditions of `table.move`, its fifth argument and its two
refusals; the two position rules and the default position of `remove`
and `insert` and the number of arguments they take; the kind, the
index, the separator and the last field of `concat`; `unpack`'s
eagerly-evaluated default, its stack check and its ceiling, and
`pack`'s `n`; the pivot, the initial swap, both of the two "invalid
order function" checks, the order the comparison function is called in
and whether its answer is turned into a boolean; and all four ways
`checktab` can be wrong about a metamethod, and the five ways the
length `sort` refuses can be wrong — 37 turn it red. The one
that stays green is the randomized pivot, and it stays green because
the grid asks no array that could reach one: it is §6.18's.

Two of them are worth naming, because they are what the corpus was
built around after the mutation test said the grid could not see them.
The things that behave like a table have to be **userdata**: a table
with a metatable on it is a table, and `checktab` asks it nothing, so
the 48 cases that were meant to exercise it were asking 48 questions
that never reached it. And the two "invalid order function" checks are
two different checks, and only one of them is easy to reach: the one
that walks up from `lo` fires on a comparison function that says
everything is less than the pivot, but the one that walks down from
`up` needs the pivot to say it is less than *every* element it meets,
which needs a comparison function whose answer does not depend on the
other element — `function(a, b) return a == 4 end` — and an array that
holds that value in two places at once, since the pivot is moved out
of the position it was picked from. Forty-five of the 1 005 cases are
those arrays. They are asked a second time by `TestTableInProcess`,
which asks both of the two checks in this process and with the number
of pairs the comparison function was asked about before it gave up —
ten and twenty-six — because both of them say the same thing and only
the count can tell them apart (§5.10).

The maths half is the newest, and it is the one that can be wrong in
two different ways at once. Half of `lmathlib.c` is arithmetic a port
writes for itself — `floor` and `ceil` have to answer an *integer* for a
float that fits in one, which is what the shortcut at the top of each of
them is for, since `floor(2^53+1)` would otherwise be `2^53`; `fmod`
keeps a rule of its own for a divisor of -1 and refuses a divisor of 0
only when both arguments are integers, so `math.fmod(7.5, 0.0)` is a NaN
and nothing is said; and `abs` of the smallest integer is the smallest
integer. The other half is a pseudo-random generator: `random` is
xoshiro256\*\* seeded by two integers, so the sequence it gives is a
thing two programs can be asked about and either agree on or not.

The first thing the half had to settle was which functions are in which
half, and two probes did it. The first asked 4 173 questions — every
function over every value there is a name for — and **10** answers came
back different, all of them in the last bit of a transcendental. The
second asked 44 478 over the five that were left — every power of two
from 2^-1074 to 2^1023, the subnormals, both infinities and the NaN,
into `frexp`, `ldexp`, `modf`, `deg` and `rad` — and **not one** did.

That second probe is worth more than it looks, because it moved five
functions out of §6.7's column. They are arithmetic, not the C
library's: `frexp` and `ldexp` are a shift, `modf` subtracts the part it
has just taken, and `deg` and `rad` multiply by 180/π, a constant
written into both libraries in the same 37 digits. One rounding each,
and two implementations that follow IEEE 754 do it the same way. So the
five are asked 470 times over in the grid, and §6.7 says so. The
fourteen that are left — `sin`, `cos`, `tan`, `asin`, `acos`, `atan`,
`atan2`, `exp`, `log`, `log10`, `cosh`, `sinh`, `tanh` and `pow` — are
asked only what they say about a table, a boolean and nothing at all,
which is theirs and not the library's whatever they say about 0.5; their
answers to a number, and the three places in which this port had to
leave Go's implementations, are pinned in `lmathlib_test.go`.

Four things the half does not hold, and three of them nobody can:

* `math.randomseed()` with no arguments. PUC-Lua takes its seed from
  `time(NULL)` and the address of a local, so what it answers is two
  numbers no two runs share: §6.8. There is nothing to compare. Every
  other form of `randomseed` is asked, and so is every draw that follows
  one. What this one answers *is* two integers and the first is the
  time, which is what `TestMathInProcess` asks of it instead.
* The answer of any of the fourteen to a number. It is a thing this port
  gets right to fourteen digits and cannot get right to fifteen, and a
  grid that asked it would be red on one machine and green on another.
* Which position `math.fmod` complains about when it is handed
  nothing at all: §6.20. It is the only place in the whole of PUC-Lua
  where two argument checkers sit in one argument list, so which of
  them is reached first is the compiler's business — clang says `#1`
  and gcc says `#2`. One argument is asked instead, which has only one
  position that can be wrong.
* Which of the two names `atan` and `atan2` an error message says:
  §6.19. They are one function, and the name is not the one the source
  spells but the one `findfield` walks into first, which the hash seed
  decides. Both sides answer it at random and disagree half the time, so
  the ten messages that could say either are asked, and then made to say
  one.

Of the 43 things this half has been broken with — both halves of `abs`,
`floor` and `ceil`, the shortcut at the top of each of the last two, and
the float that does not fit in an integer they exist for; `sqrt` reached
by way of `pow`, which loses the sign of -0.0 and answers `inf` where
the answer is a NaN; `tointeger` at the smallest integer and the
integer-or-float of what it pushes; both halves of `type`; all four
rules of `fmod`; the direction of `max` and of `min`, the tie that keeps
the first of two equal values, and the check that a value was given at
all; `ult` read as signed; the sign, the infinity and the integer of
`modf`; the order and the reduction of `frexp`; `ldexp` by way of `pow`
and `ldexp` without the truncation to an `int` that puts 2^31 at -2^31;
both of `deg` and `rad` and the digits of the constant they share; the
four constants and the two shifts and the two multiplications of
xoshiro256\*\*, the 16 draws its seed discards and the word in it that
avoids a zero state; the default second seed; `random(0)`; the width of
the interval and the projection into it, the Mersenne mask and the draw
that follows a value outside it; `random` with three arguments; and
`randomseed` that forgets to answer, or that is given nothing and does
not ask the clock for a seed — 40 turn the grid red and 42 turn
`TestMathInProcess` red, and 42 of the 43 turn one or the other.

The two the grid misses, the in-process test catches. A float in [0,1)
built from 52 bits instead of 53 differs from the right one by 2^-53,
which `tostring`'s fourteen digits hide; so the in-process test writes
its six draws to seventeen instead. And `randomseed` that answers
`randomseed()`, which the grid cannot ask at all, is caught there —
because the mutated one raises "number expected, got no value" instead
of answering two integers. The one that stays green in both is a
thirty-first digit of π: C parses the literal as a double and so does
Go, and both round the same 37 digits to the same one of the 2^64 there
are. It is the one mutant here that is not a mutation, and it is kept,
because a grid that caught it would be a grid over-fitted to a
constant.

Two of them are worth naming, because they are what the corpus grew
after the mutation test said the grid could not see them. The shortcut
in `floor` and `ceil` exists for an integer a float cannot hold, and
2^53+1 is the first of those; it was not in the value list. And the
sixteen hundred and eighty pairs of `max` and `min` are what makes the
tie worth asking: `math.max(1.0, 1)` is `1.0` and `math.max(1, 1.0)` is
`1`, which are the same number and two different values, and which one
comes back is which one went in first — the loop only moves when the
extreme is not the first argument, so `min` is asked the one order in
which it does move.

It is worth saying what makes a fuzz like this worthless, because one
round of it was. A generated chunk that does not *load* prints the same
syntax error on both sides, and identical output is what success looks
like — all 37 615 expressions of the first round were being compared
without a single one of them running. Both halves now assert that the
chunk ran to the end
(`count` answered) before anything is compared, and each is checked by
breaking something on purpose and watching it go red. The values are
quoted byte by byte rather than with `%q` for a related reason: `%q`
escapes a newline as a backslash followed by a real newline, so a value
that prints as two lines quietly breaks the one-line-per-case the
comparison rests on — the lines still line up, both sides being equally
shifted, but the case named in a failure is then the wrong one. The
escape is written `\%03d`, padded to three digits, because Lua's `\ddd`
takes as many digits as it is given: an unpadded `\9` followed by the
byte `1` reads back as `\91`, one character, and two different values
would then print alike.

---

## 6. Known differences from PUC-Lua 5.4.9

1. **`file:read(n)` with a count above 2 GiB is treated as end of file** — C hands the count straight to the allocator, and it is the allocator that rejects an absurd one ("not enough memory", which a negative count always is and this port reproduces). This port will not allocate on that scale, so a count that large reads nothing. Binary chunks are not affected: `string.dump` produces the same bytes PUC-Lua 5.4.9 produces — stripped or not — and each loads the other's. The one exception is at the end of §6.7: a float constant folded from `^` can differ in the last ulp, because `pow` is not decided by IEEE arithmetic.
2. **C extension modules load only when the host is built with cgo** — `cdlopen.go` is the `lsys_load`/`lsys_sym` half of `loadlib.c` and dlopens the library for real, so `package.cpath`, the four searchers and `require` work on an ordinary `.so` written against `lua.h`, and `package.loadlib` answers `dlerror()`'s own message with `"open"` or `"init"`, byte for byte what PUC-Lua answers. It needs cgo for one reason: a Go binary has to export the `lua_*`/`luaL_*` names into its **dynamic symbol table** for the module to resolve them, and only the cgo path can do that. Build with `CGO_ENABLED=1` (the default where a C toolchain is present) and compile the extension the way its platform allows the names to stay unresolved until load time — `-undefined dynamic_lookup` on macOS; on Linux, where the host's symbols are resolved from the executable's dynamic table, `-shared -fPIC` is enough. No linker flag is asked of whoever embeds the library: the exporting is the package's own business and travels with it through `#cgo LDFLAGS` (`//go:cgo_ldflag`), which reaches the link of anything built against it — `--export-dynamic` on Linux, where an ELF executable exports nothing it is not told to and `dlopen` looks nowhere else. That one matters more than it looks: Go's linker adds `--export-dynamic-symbol` per `//export`-ed Go function, which is most of the API, but the four variadic ones are C (`cdlbridge.c`) and are never mentioned to the linker at all, so without it every PUC-Lua shared object stops at `undefined symbol: lua_pushfstring`. macOS needs nothing — its executables export their globals by default. `go test` covers it: `cdlopen_test.go` writes a C extension with `luaL_newlib`, compiles it, `require`s it and calls into it; `cdlopen_api_test.go` compiles a larger one and compares all 25 lines it prints against the same extension run under PUC-Lua 5.4.9 — userdata with a registered metatable, `luaL_checkudata`, `luaL_checkoption`, the buffer API, user values, `lua_rawgetp`/`lua_rawsetp`, `lua_dump`, `lua_gc`, `lua_getstack`/`lua_getinfo` through a C `lua_Debug`, `lua_stringtonumber`, and every conversion `lua_pushfstring` has (including the "invalid option" error for one it does not).

The four functions that take `...` — `lua_pushfstring`, `lua_pushvfstring`, `luaL_error`, `lua_gc` — are written in C (`cdlbridge.c`), because neither direction of the cgo boundary can express a variadic call. They reproduce `luaO_pushvfstring` rather than delegating to `printf`: `%d` is a Lua integer, `%f` is `%.14g` with the `.0` C adds when the result looks like an integer, `%p` is a bare `%p`, `%U` is a UTF-8 sequence, and anything else is the error PUC-Lua gives. `printf` would have accepted `%.2f` (which Lua rejects) and printed `%d` as C's `int`.

With `CGO_ENABLED=0` the backend is absent, `package.loadlib` answers `nil, "dynamic libraries not enabled", "absent"` — word for word what PUC-Lua answers when built without `LUA_USE_DLOPEN` — and a C module found in `package.cpath` is the error PUC-Lua gives there. So it is on Windows, for the same reason from the other end: there is no `dlfcn.h` to include, so the backend is built on unix only and a Windows host answers the same thing.

Continuations — `lua_callk`, `lua_pcallk` and `lua_yieldk` with a non-NULL `k` — are implemented: see §5.6. The one thing about them that is *not* PUC-Lua's is the status a `lua_callk` continuation is told: C always passes `LUA_YIELD` there, and here the C function calls the continuation itself, having watched the call return normally. Nothing observable depends on it, because for `lua_callk` that status is a constant.

`lua_error` is worth a note, because "it raises instead of longjmping" reads like a gap and is not one. A Go panic crosses a cgo frame: the runtime unwinds to the `recover` of the call that was protecting the call, and the extension's frame is left behind exactly as `longjmp` would leave it. Three levels deep — Lua → C → Lua → C → raise — the answer is PUC-Lua's, line for line, and that is what `TestCModuleErrors` pins. `longjmp` across the boundary is the thing that would be unsafe, since it would skip the runtime's own bookkeeping for the cgo call, so there is no `setjmp` here on purpose. What is lost is what `longjmp` loses too: a C frame holding something that needs cleanup does not get to clean it up. `TestCModuleErrorStress` is the other half of the evidence: 30 000 raises across C frames, and one from the bottom of a 50-level nest of them, none of which run the C stack out — which is the only way abandoning those frames could actually be seen to go wrong.

The C block behind a userdata is freed when the userdata is collected: `C.malloc` is not memory Go's collector knows about, so without a finalizer an extension making userdata in a loop would grow without bound. Anything written in Lua, or ported to Go, needs none of this: `L.RequireF(modname, openf, true)` adds one to `package.loaded` (the port's `luaL_requiref`), or set `package.preload[name]` to a Go function and let `require` find it the usual way.
3. **`collectgarbage` delegates to Go's collector, which is the whole process's collector** — the semantics of each option are C's own: `collect` and `step` run a cycle, so a `__gc` runs and the weak entries go; `stop` and `restart` set a flag the step an allocation drives obeys, and `isrunning` answers it; `setpause`, `setstepmul` and the two modes remember what they were given and answer what they replaced. But PUC-Lua's collector only ever walks the objects of one `lua_State`, so a script calling `collectgarbage()` costs the interpreter that owns it and nothing else. Here the call is `runtime.GC()`, which stops the world and marks everything the *process* has allocated — the host application's data included — and `collectgarbage("count")` likewise answers the Kbytes of the whole heap rather than of this state, because the Go heap is not accounted per state. For a host embedding this library the cost is therefore the host's: a script that calls `collectgarbage()` often — in a loop, or at the end of every request — pauses the whole process, and it pauses for longer the bigger the host's heap is. That is worth knowing before handing a state to a script you did not write. The step an allocation drives (see `gcStep`) runs the same cycle and is therefore the same cost, but it only fires once every 65 536 table allocations and only while something is being watched for `__gc`.

Three tests of the official suite — `coroutine`, `gengc`, `locals` — assert precisely that a collection happened right after a `collectgarbage`, and all three are byte-identical. An object with a `__gc` is held between cycles so that Go cannot finalise it at a moment of its own choosing, and the metamethod of every object that still has one runs when the state closes, reachable or not.

One thing Go's finalizers will not do at all is fire for an object in a reference cycle: two objects that point at each other, one of them with a finalizer, are never collected — not after one cycle, not after ten. A `__gc` that seems never to run is worth checking for a pointer back before anything else (§5.13: the stream behind a file handle had one, and every dropped handle was keeping its buffer).

   What a cycle here cannot answer is only what measures an allocator that is not there — `gc.lua` — the one file of the official suite that is a test *of* a collector — asks how much memory a collection freed and in what order it ran the finalizers of a long list, which is a question about PUC-Lua's own tri-color marking and not about Go's.

Inside a `__gc` the collector counts as stopped, exactly as in C: `collectgarbage` answers `nil` for every option there (`lua_gc` returns -1, and `lbaselib.c`'s `checkvalres` turns that into a fail), and an allocation in a finalizer drives no cycle, so finalizers do not nest. A `__gc` that closes the state — `os.exit(0, true)` — has the rest of them run from that close, which is `main.lua`'s finalizer-order case and now matches PUC-Lua line for line.
4. **A weak entry disappears in one cycle, not part way through allocating** — a Go
`weak.Pointer` is cleared in the cycle *after* the one that frees its object, so
one cycle here runs Go's collector twice: the first frees what only a weak table
referred to, the second clears its pointer. Where that can be measured it
agrees: an entry is gone after two million allocations, exactly as in PUC-Lua.
What differs is the granularity — PUC-Lua's collector interleaves with
allocation, so an entry vanishes during some particular allocation, while this
port clears at a cycle boundary (`Close()`, or allocation crossing a threshold),
so "after which allocation" has no answer. Strings, numbers and
booleans are never weakened, exactly as in PUC-Lua.
5. **`os.setlocale` asks the C library when the build has cgo, and answers from a built-in table of decimal points when it does not** — the decimal point is the one part of a locale Lua observes, and it is observed in three places: `tostring` of a number, `string.format`'s `%f`/`%e`/`%g` (but not `%a` or `%q`, which is what lets `%q` round-trip), and `tonumber`, which accepts either separator while a source literal always uses `.`. With cgo the C library's `setlocale` is the authority and answers what it would in PUC-Lua, including `nil` for a locale the host has not installed. Without cgo there is no C library to ask, so the port accepts the name and reads the decimal point from the language of the locale — `pt_BR`, `fr_FR`, `de_DE` and the rest of the languages that write a comma get `,`, everything else and `"C"`/`"POSIX"` get `.` — and `os.setlocale("")` reads `LC_ALL`/`LC_NUMERIC`/`LANG` the way C's would. Which locales are installed is the host's business when cgo is on; when it is off, every name is accepted, because the only thing Lua can see of a locale is that one character.
6. **`os.date` refuses a slightly different range of times, `os.time` accepts dates the host's `mktime` refuses, and `os.date("!%z")` answers the UTC offset rather than the local one** — `struct tm` keeps the year in an `int`, and the host's `gmtime`/`localtime` answer NULL once it no longer fits, which is where PUC-Lua's "date result cannot be represented in this installation" comes from. This port asks the same question against `tm_year` and reproduces the top of the range exactly — 67768036191676799 is the last representable second (year 2147485547, `tm_year` `INT_MAX`) and one more is refused — but the bottom of the host's range stops a little short of `INT_MIN`, so a window of some 2×10¹⁴ seconds at the far negative end is answered here and refused there; no second beyond that is answered by either. For `os.time`, PUC-Lua builds a time with the C library's `mktime` and reports "time result cannot be represented in this installation" when it answers -1; which dates those are is up to the C library (on macOS, anything that normalises to a year before 1900; glibc accepts those). Go's `time.Date` has no such limit, so this port answers a number. Everything PUC-Lua itself decides is reproduced — the bounds each field must satisfy before it reaches `struct tm` ("field 'year' is out-of-bound"), the missing and non-integer fields, a result of exactly -1, and every other specifier of `os.date`. Six of the thirty-seven conversion specifiers are not theirs in every place they could be asked — `%Z`, `%z`, `%Y`, `%y`, `%C` and `%G`, and the `%F` and `%g` that are spelled out of the year. They ask the C library, and where the two C libraries disagree they are in §6.15.
7. **The maths library is Go's, not the platform's libm** — `math.sin`, `cos`, `tan`, `exp`, `log`, `log10`, `sqrt`, `asin`, `acos`, `atan`, `atan2`, `pow`, `sinh`, `cosh` and `tanh` are Go's implementations rather than the ones libc links. They agree to 15 significant digits and differ in the last one or two for a fraction of the arguments: measured over 120 arguments spread across fifteen magnitudes, `sqrt` never differs, `exp` and `log` differ for a handful (below one ulp), `log10`, `tanh`, `asin`, `acos` and `atan` for a few dozen (around one ulp), and `sin`, `cos` and `tan` for a quarter to a half. Five functions that used to be on that list are not on it: `frexp`, `ldexp`, `modf`, `deg` and `rad` are arithmetic and not the library's, and a probe of 44 478 answers over them — every power of two from 2^-1074 to 2^1023, the subnormals, both infinities and the NaN — found **not one** that differs, which is why they are 457 cases of the maths half of the fuzz (§5.13). Above 2^53, where the argument has to be reduced before the function can be asked, the gap widens — `tan` strays by six ulp and `sin` by seven — because neither implementation is correctly rounded there and they reduce differently — and an ulp is a small absolute error when the answer itself is near zero, which is why `cos` of a large argument can read as tens of ulp away while the two agree to within one rounding of the reduced argument. Every arithmetic operation, `sqrt`, and both integer conversions are exact. A NaN is spelled the way the platform's C library spells it — `"nan"` on macOS, `"-nan"` on glibc when the sign bit is set — because that is what C's `printf` does with it. There are three places where the port deliberately steps outside Go's implementations. The first is the band in which `exp(|x|)` approaches the largest float: `math.exp`, `math.sinh` and `math.cosh` are computed there by reducing the argument modulo ln 2 and scaling the answer by a power of two (`mathExpScaled`) rather than by asking `math.Exp` itself, because `math.Exp` is not one function but a hand-written assembly version per architecture and the copies disagree at that edge. On amd64 `math.Exp(709.5)` answers `inf`, where `exp(709.5)` is 1.3549863193146328e+308 — an ordinary float, and what arm64, the generic Go implementation and C's libm all answer — because `exp_amd64.s` truncates its overflow constant to `7.09782712893384e+02` instead of `7.09782712893383973096e+02`. The same threshold makes Go's `math.Sinh(710)` an `inf` whose true value, `exp(710) / 2` = 1.1e308, is representable. 25 answers across that border — 709, 709.5, 709.78, 709.79, 710, 710.5, 800 and their negatives — are pinned against PUC-Lua in `lmathlib_test.go`. The second is the other end of the same range: `log_amd64.s` loses several digits on a **subnormal** argument, and `1e-308` is one — the smallest normal float is 2.2250738585072014e-308 — answering `-708.71839997088` where `log(1e-308)` is `-709.19620864217`. So `math.log` and `math.log10` scale a subnormal up into the normal range first, by a power of two, which is exact, and take the exponent back off afterwards (`mathLogScaled`): `math.log10(1e-308)` is `-308` again. Fourteen answers down there — `1e-308`, `1e-320`, `5e-324` and the smallest normal, in every base — are pinned in the same file. The third is `math.log(x, 2)`, which asks C for `log2(x)` and not for `math.log(x)/math.log(2)`: Go's `math.Log2` splits the argument with `Frexp` and adds the exponent back afterwards, so an answer that is near zero — which is what the log of a number near 1 is — is left made of the roundings of two numbers a million times bigger than it is. `math.log(1.0000000000000002, 2)` is 3.2034265038149e-16 to C's `log2` and 3.6816598061451e-16 to `math.Log2`: fifteen per cent, and unlike anything else on this list. So in the band `[sqrt(2)/2, sqrt(2))`, where `math.Log` has no exponent to add back and keeps those bits, `mathLog2Scaled` asks `math.Log` and divides by ln 2; outside it — and for every power of two, where `math.Log2` is exact — it asks `math.Log2`, whose error does not grow with the exponent the way a quotient's does. 26 answers across that border, from `1 + 2^-52` to `5e-324`, are pinned in the same file.

One consequence is worth naming, because it reaches further than a
number that is off by one rounding. `A op B` with two numerals is
folded by the compiler, not computed at run time — so an inexact `pow`
does not merely make `2^2.5` print wrong, it makes it a *constant in
the byte code*: `0x1.6a09e667f3bcdp+2` under C, `0x1.6a09e667f3bccp+2`
here. It is the only way two identical programs can compile to
different bytes, and it was the single difference in the 103 786-snippet
probe the codegen fuzz grew out of (§5.13). Every other operator that
folds is exact: the arithmetic ones are IEEE, `//` and `%` are exactly
defined and both libraries answer identically, and the integer and
string conversions are decided by the representation. Measured against
`Exp2(y*Log2(x))` and a split-exponent variant over 922 pairs, Go's
`math.Pow` is the closest of the three — 150 differ against 456 and 473
— so there is nothing here to switch to.
8. **`math.randomseed()` with no arguments returns a different second seed** — PUC-Lua returns the current time and the *address of the `lua_State`* as its two seeds (lmathlib.c: `randseed`), and this port returns the same two quantities, so the first seed — the time — matches and the second is a Go heap address rather than a C one; the numbers are both host addresses and are not reproducible in either implementation. `math.randomseed(n)` and `math.randomseed(n, m)` return their arguments unchanged, and the generator itself is the same xoshiro256**-based one: for a given seed it produces the identical sequence, integer or float, which the suite pins across 20 draws of `math.random()`, `math.random(6)` and `math.random(10, 20)`.
9. **A coroutine suspends by parking its goroutine, and a C frame that suspended is still there when it wakes** — C's `lua_yield` long-jumps to the `lua_resume` that will pick the thread up again, and everything between is discarded; this port instead hands the yielded values to the resumer through a channel and then blocks on the next resume request, so the Lua stack, the saved program counter and — for a C caller — the C stack itself all survive at the yield point. That is what lets a metamethod or a Go function yield from the middle of a call, and it is what makes continuations work (§5.6). Two consequences follow. The C-call depth a thread inherits from its resumer is refreshed at every yield rather than only at the first resume, which is what C does too, since `lua_resume` runs on every resume. And a C function that yielded does not get to notice: `lua_callk` returns to it as if the call had simply finished, so the status a `lua_callk` continuation hears is `LUA_OK` where PUC-Lua's is always `LUA_YIELD` — a constant, so nothing can depend on the difference. `lua_pcallk`'s status does vary, and that one *is* PUC-Lua's: the port counts the yields the thread has made and reports `LUA_YIELD` when the call was interrupted by one.
10. **A `%a` tie on an odd digit rounds to even, which is what glibc does and what Apple's library does not** — PUC-Lua hands every conversion to the platform's `sprintf`, so what `string.format("%.0A", 1.5)` answers is what that library answers, and the libraries do not agree: `1.5` is `0x1.8p+0`, so a precision of zero drops exactly half of the last digit kept. glibc's `__printf_fp` hands the digit being kept to `round_away`, which leaves an even digit alone and steps an odd one, and answers `0X2P+0`; Apple's library rounds a tie *down* whatever that digit is, and answers `0X1P+0`. This port answers `0X2P+0`: it is what the C library's own `%f` answers on every platform (`%.0f` of `1.5` is `2`, in both libraries), and it is what makes `%a` agree with `%f` inside a single call. So on a host whose library is Apple's, `%.0a` of `1.5` is `0x2p+0` here and `0x1p+0` under PUC-Lua — the only case in the whole library where a difference is the C library's rather than this port's. A tie on an *even* digit is where both rules land anyway, so nothing differs there; the fuzz asks the reference which library it was built against and leaves the rest out (§5.13).

11. **A subnormal number is written normalised, which is what Apple's library does and what glibc does not** — of a normalised float the leading hexadecimal digit of `%a` is 1; of one too small to have a 1 — below 2.2250738585072014e-308 — C leaves it unspecified, because there is nothing to normalise to. glibc keeps the exponent of the smallest normal float and writes the raw 52-bit fraction after the point, so `string.format("%a", 5e-324)` is `0x0.0000000000001p-1022`; Apple's library normalises it, and so does this port, for which it is `0x1p-1074` — what Go's own hexadecimal conversion writes, and one number rather than two spellings of one. Unlike the tie above, this one the standard allows both ways, so neither is the more correct. It reaches further than `%a`: `%q` writes a float the same way, and that is the conversion a program uses to write a number back out. Both spellings are numerals Lua reads, so a value written by one side is read by the other either way. The fuzz asks the reference which library it was built against, and on a reference that answers `0x0.0000000000001p-1022` the grid cases that turn on it are not asked on either side.

12. **`file:setvbuf(mode, size)` with a negative size always succeeds** — C hands `size` to `setvbuf` as a `size_t`, so `-1` is `SIZE_MAX` there, and what the library does with that is the library's business: Apple's asks the allocator for a buffer that large, fails, and leaves `errno` alone, so PUC-Lua answers `nil, "(no extra info)", 0`; glibc pays no attention to the size when it is given no buffer of its own and answers `true`. No portable answer exists, so the port answers `true` and the grid does not ask the case (§5.13). Every other size — including 0, which glibc's `setvbuf` treats as "no buffer asked for" — is PUC-Lua's.
13. **The registry has no `_UBOX*` key.** `lauxlib.c` holds a `luaL_Buffer` in a userdata on the Lua stack once the buffer grows past `LUAL_BUFFERSIZE`, and it makes the metatable for that userdata the first time it happens: `string.rep("x", 9000)` is enough, and the key `"_UBOX*"` is then in `debug.getregistry()` for the rest of the run. This port's buffers are Go slices and are never stack objects, so there is no moment at which such a key would be created. Every other key is PUC-Lua's and is compared — the main thread, `_LOADED`, `_PRELOAD`, `_CLIBS`, `_IO_input`, `_IO_output`, `FILE*` and `_HOOKKEY` — and the grid leaves this one out (§5.13).
14. **Performance** — the port is slower than C by a constant factor that depends on the workload: 1.4× to 3.6× for ordinary code (§5.11), and 6.0× for coroutines, where a yield/resume pair is two goroutine switches rather than a swap of stack pointers. This is a difference in kind from PUC-Lua but not one in behaviour: no program answers differently because of it. The one place it can be observed is a script that is also a deadline — a workload timed out on one side and finished on the other — which is why the official suite's `heavy` test is a known difference.

15. **Four of the questions `os.date` can be asked are the C library's, and the two C libraries this project is measured against do not agree** — the rest of the thirty-seven conversion specifiers are PUC-Lua's and are reproduced, but these four are the machine's, so the date fuzz does not ask them (§5.13) and this port answers the way one of the libraries does:

    * **`%Z` and `%z` under `!`.** Under `!` PUC-Lua breaks the time down with `gmtime`, and what it then prints is the library's: glibc's `gmtime` leaves `tm_gmtoff` at zero and `tm_zone` at `"GMT"`, so PUC-Lua prints `+0000` and `GMT`; Apple's `%z` is taken from the C library's `timezone` variable and not from the broken-down time at all, so the same format prints the zone's nominal offset — `-0500` in New York — and `UTC`. This port prints `+0000` and `UTC`: the offset and the name of the instant it was asked about, and what glibc says.
    * **`%z` at an instant the timezone database does not cover.** Before its first transition the offset is a local mean time, and a zone with negative daylight saving — Dublin — also has instants whose historical standard time is not the one its current rules describe: New York before 1883 is `-0456` here and `-0500` under Apple's C library, and Dublin in 1901 is `-0025` and `+0000`. Both name the type alike, so `%Z` is asked there and `%z` is not.
    * **The year, when it is below a thousand.** A year is written with a width, and the two libraries pick different ones: for the year 1, `%Y` is `0001` to Apple's and `1` to glibc's, and for the year -27, `%y` is `27` to one and `73` to the other. Every instant the date fuzz asks about is in a year of four digits or more.
    * **The year at the very top of a `struct tm`.** 67768036191676799 is the last second a `struct tm` can hold, and its `tm_year` is exactly `INT_MAX` — 2147483647. What `strftime` prints is not `tm_year` but `tm_year + TM_YEAR_BASE`, and glibc does that addition in `int`, so it wraps: `%Y` is `-2147481749` there, and `%C` is `-21474818`, which is the century of the negative year. Apple's prints `2147485547` and `21474855`, and so does this port. The same instant is still asked to break down into a table on both, because that table is PUC-Lua's own and its `setfield` casts before it adds — `loslib.c`: `lua_pushinteger(L, (lua_Integer)value + delta)` — so `os.date("!*t", 67768036191676799).year` is 2147485547 under both libraries. Only the string is each library's own, and it is the only thing the two instants at the top are not asked for on glibc.

    The one instant on which they agree about a year before the Common Era is a year that divides by a hundred, and there this port agrees with both — which is where `TestDateInProcess` holds `%y` and `%Y`, since the grid cannot.

16. **What `os.time` does with `isdst` is what the C library's `mktime` does, and the two libraries do it by opposite rules** — `mktime` reads `tm_isdst` as an instruction: 1 is "read the wall clock as a summer-time one", 0 is "read it as a standard-time one", and -1, which is the field absent, is "work it out". When the instruction and the zone disagree, the two part company. glibc's `mktime` goes looking through the timezone database for a nearby instant whose `tm_isdst` is the one it was told, and borrows its offset: for 1970 in Dublin it borrows 1972's, which is the offset Dublin kept the whole year round in 1970, so the instant does not move at all. When it finds nothing anywhere near — in UTC, where nothing is ever summer time — it gives up the search and assumes an hour, which is the last thing `mktime.c` does: `t += 60 * 60 * dst_difference`. macOS's steps by an hour in any zone that has a summer time and does nothing in one that has none. The two answer `-3600` and `0` to `os.time{year=1970,month=1,day=1,hour=0,min=0,sec=0,isdst=true}` in UTC, and `-3600` and `-7200` in Dublin. This port steps by an hour and ignores the field in a zone with no summer-time rule, which is the macOS answer; the two tables that turn on it are asked on macOS and not on Linux (§5.13). Everything else about `os.time` is PUC-Lua's: the bounds on each field, the missing and non-integer ones, the result of exactly -1, the twelve fields written back, and the month, day, hour, minute and second that were out of range and came back in it.

17. **The pivot `table.sort` picks is a random one, once the array is big enough to have a badly balanced partition, and the randomness comes from `clock()` and `time(NULL)`** — `ltablib.c` starts its quicksort with a randomization of 0, which its `choosePivot` is not asked to use, so the pivot is the middle element. It reaches for a real one only after a partition lands badly, which is what `if ((up - lo) / 128 > n)` says, where `n` is the smaller of the two intervals the partition left; and what it reaches for is `l_randomizePivot`, which copies `clock()` and `time(NULL)` into an array of `unsigned int` and adds them up. Since `up - lo` is one less than the size of the interval, an interval of 128 or fewer elements can never set it: 127 / 128 is 0, and 0 is not more than anything. So an array of 128 or fewer elements sorts by a fixed route and compares a fixed sequence of pairs, and a bigger one may not, from one second to the next. The array it ends up with is the same either way — only the pairs it compares can differ, and only a comparison function that notices the order it is called in can see them. The table fuzz asks such a function of arrays of up to 100 elements and of nothing bigger (§5.13), and this port carries the same randomization for the arrays that can reach it.

18. **Which of the two names `math.atan` and `math.atan2` an error message says is nobody's to decide** — they are one function, `atan2` being the name `LUA_COMPAT_MATHLIB` adds for it, and an error message does not name a C function as the source spells it but as `pushglobalfuncname` found it, which is `findfield` walking `package.loaded.math` with `next` and taking the first key whose value is that function. Which key that is depends on the order of the table's hash part, and the order depends on the seed every state makes for its string hashes, which is the clock and a heap address. So the name is drawn at random in every process, and two processes draw independently: PUC-Lua answered `math.atan` in 8 of 12 runs and `math.atan2` in the other 4, and this port answered 7 and 5. There is no answer a port could give that would be right, because there is no answer PUC-Lua itself will repeat — it is not the C library's, not the clock's alone and not a translation's. The maths fuzz asks the ten messages that could say either and then makes them say one (§5.13), which is the third way a difference here is kept out of a corpus: not by leaving the case out, but by taking the part of the answer that was not anybody's out of it. Every other name in the library is unique, so every other message is one thing — including the one that says `'insert'` and `'remove'` the short way, which is a literal in `ltablib.c` and not a name anybody searched for.
19. **`math.fmod` with no arguments complains about whichever position its compiler reached first** — it is the only place in the whole of PUC-Lua where two argument checkers sit in the same argument list: `l_mathop(fmod)(luaL_checknumber(L, 1), luaL_checknumber(L, 2))`. C leaves the order the arguments of a call are evaluated in unspecified, and the two compilers this project builds the reference with do not agree: clang on macOS answers "bad argument #1 to 'math.fmod' (number expected, got no value)" and gcc on Ubuntu answers "#2", from the same source on the same day. Every other function in every library of PUC-Lua reads its arguments into locals first, so every other message names the position the source names. This port answers "#1" on both, which is what the reference it was written against answers — a Go call evaluates its arguments in the order they are written. So on a gcc-built reference there is one message in the whole language that differs, and there is no answer that would be right there, because PUC-Lua's own answer changes with the compiler it was built by. The maths fuzz asks `math.fmod(1)` instead, which has only one position that can be wrong, and not `math.fmod()` (§5.13) — the fourth way an answer that is not Lua's is kept out of a corpus, and the only one that is the compiler's rather than the clock's, the hash seed's or a library's.
---

## 7. Build & test

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .                                  # must print nothing
go test -bench=. -benchmem -run=XXX ./...   # benchmarks
go build -o lua  ./cmd/lua    # the standalone interpreter
go build -o luac ./cmd/luac   # the standalone compiler
```

Two of those need more than Go: `cdlopen_test.go` and
`cdlopen_api_test.go` compile C extensions and `require` them, and skip
themselves when there is no cgo or no C compiler, so
`CGO_ENABLED=0 go test ./...` is a supported configuration that simply
does not exercise the dlopen path.

CI runs the whole matrix on Linux, macOS and Windows
(`.github/workflows/go.yml`) — four jobs:

| Job | Platforms | What it does |
|---|---|---|
| **build** | 3 OS × `oldstable` + `stable` | `go vet`, `go build`, `go test`; `go test -race` on Linux and macOS (the race detector needs cgo); a `gofmt` check and one pass over the benchmarks on Linux |
| **nocgo** | 3 OS | `CGO_ENABLED=0` build and test, plus cross-compiles for five targets — this is what keeps the pure-Go promise true |
| **PUC-Lua suites** | Linux + macOS | downloads Lua 5.4.9 from lua.org, builds a reference interpreter from source, and runs both suites with `LUA_REF` pointed at it: `TestOfficialSuite` (Lua's own 33 files, in `testdata/official`) and `TestDifferentialSuite` (this port's 82 cases, in `testdata/diff`) and `TestThirdPartySuite` (§5.12's five third-party libraries). Goldens are captured fresh for that platform and then compared byte for byte. That makes both suites real gates everywhere and stops the checked-in darwin goldens from rotting unnoticed. (No Windows: Lua's own makefile is a Unix one.) |

Reference sources for review (not vendored): `lua-5.4.9`, <https://www.lua.org/ftp/lua-5.4.9.tar.gz>.

## 8. License

MIT.
