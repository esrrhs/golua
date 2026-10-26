# Benchmarks

[中文](README_CN.md)

Two harnesses live here. `compare.sh` is the one behind the ratios in the root README. `run.sh` times an older, smaller set of scripts and is not that table.

## compare.sh

```bash
go build -o lua ./cmd/lua
sh bench/compare.sh /usr/local/bin/lua ./lua
MIN_MS=500 REPS=9 sh bench/compare.sh /usr/local/bin/lua ./lua
ONLY="fib hash" sh bench/compare.sh /usr/local/bin/lua ./lua
```

The clock is read outside the process. `os.clock` is CPU time in PUC-Lua and wall time since startup in golua, so `suite.lua` only prints a checksum.

Each workload is one function in `suite.lua`. One call is often a few milliseconds, which is mostly process startup, so that call is not the measurement. For each interpreter the call is stacked until one process has run for at least `MIN_MS` (default 1000). The count is chosen per interpreter. The timed process is then repeated `REPS` times (default 9, at most 25). The reported time is the mean wall time of one call. The ratio is golua / PUC-Lua; below 1 is faster. `compare.awk` also prints the coefficient of variation, the absolute and relative difference, and an exact two-sided Mann–Whitney U test on the repetitions. `noise` means `p >= ALPHA` (default 0.05): the gap sits inside the scatter. `short` means that side never reached `MIN_MS`, so startup still dominates. Interpreters run one at a time. `startup` is the process itself and is not stacked. `ONLY` keeps named rows. More than one call folds the checksum, so a run that skipped iterations cannot pass as one that did them.

## The run in the root README

Linux x86_64 (Intel Xeon, kernel 6.12.94+), PUC-Lua 5.4.9 against a cgo build of this tree (Go 1.27.3). The port tracks Lua 5.4.9. Every row had `p < 0.0001`. Startup was the widest row: golua's coefficient of variation there was 18%, and the other rows were a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

### Work that still runs

Dense integer arrays are written and read one element at a time. `string.format`, `table.concat`, `gmatch` and `table.sort` go through the library, and `table.sort` calls back into Lua on every comparison. String-key lookup walks the same open-addressed hash part as PUC-Lua. Numeric loops stay on the per-instruction interpreter.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.44 ms | 3.43 ms | 2.38× | a new process, no script body |
| nop | 16.6 ns | 34.9 ns | 2.11× | `return 0` |
| hash | 158 ms | 318 ms | 2.01× | short-string key lookup |
| intloop | 111 ms | 259 ms | 2.32× | integer sum |
| floatloop | 130 ms | 304 ms | 2.33× | float multiply-add |
| bitwise | 131 ms | 342 ms | 2.61× | and, xor, shift |
| array | 120 ms | 358 ms | 2.99× | dense integer keys |
| strings | 69.6 ms | 86.9 ms | 1.25× | format, concat, gmatch |
| sort | 73.4 ms | 114 ms | 1.55× | `table.sort`, Lua comparator |

A process starts in 3.43 ms against 1.44 ms. Most of that gap is the Go runtime. `return 0` still costs 34.9 ns against 16.6 ns, because the loop that issues the call is dispatched in Go. The hash row builds its keys once and then looks them up. A Go hash node is larger than the C node, and short-string equality and the collector are a large share of a profile of this lookup. The array loops still touch every slot; `strings` still formats one string per index.

### Call and allocation shapes

These rows exercise one common bytecode pattern each. The checksum stays the same. Both sides run the interpreter (no Go-only specialized shortcuts).

| workload | PUC-Lua | golua | ratio | shape |
| --- | ---: | ---: | ---: | --- |
| fib | 38.5 ms | 87.1 ms | 2.27× | direct Fibonacci, `n` in 0..45 |
| concat | 78.3 ms | 176 ms | 2.24× | loop-invariant `a .. b` |
| yield | 133 ms | 174 ms | 1.31× | `yield` of `1..n`, then sum |
| pcall | 74.8 ms | 181 ms | 2.42× | `pcall` of an identity |
| upvalue | 344 ms | 725 ms | 2.11× | `n = n + 1` |
| global | 152 ms | 343 ms | 2.25× | `_G.g = _G.g + i` |
| ipairs | 135 ms | 309 ms | 2.30× | base `ipairs`, dense array |
| tailcall | 114 ms | 215 ms | 1.89× | countdown tail call |
| metaindex | 120 ms | 213 ms | 1.78× | stable integer field |
| method | 200 ms | 372 ms | 1.86× | `object:add(constant)` |
| call3 | 170 ms | 363 ms | 2.14× | `return a + b + c` |
| alloc | 96.7 ms | 206 ms | 2.13× | four-element constructor |

`fib` is the recursive `n < 2` / `fib(n-1) + fib(n-2)` shape for an integer from 0 to 45. `concat` joins two strings inside a loop. `yield` wraps `coroutine.wrap` of `yield(i)` for `i = 1, n` and sums the results. The other rows exercise `pcall`, upvalues, globals, `ipairs`, tail calls, metamethods, method calls, multi-argument calls, and small table constructors — the same paths as PUC-Lua, with Go dispatch overhead.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`.
