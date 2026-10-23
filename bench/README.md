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

Intel Xeon (8 cores), **PUC-Lua 5.3.4** against a cgo build of this tree (Go 1.27). `MIN_MS=600 REPS=7`; every row had `p < 0.01`. Startup CV on the golua side was about 12%; other rows a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.38 ms | 3.58 ms | 2.59× | a new process, no script body |
| nop | 25.7 ns | 38.8 ns | 1.51× | `return 0` |
| intloop | 169 ms | 338 ms | 2.00× | integer sum |
| floatloop | 228 ms | 538 ms | 2.36× | float multiply-add |
| bitwise | 241 ms | 597 ms | 2.48× | and, xor, shift |
| fib | 63.1 ms | 104 ms | 1.65× | recursive Fibonacci |
| tailcall | 155 ms | 310 ms | 2.01× | countdown tail call |
| call3 | 234 ms | 390 ms | 1.67× | `return a + b + c` |
| upvalue | 405 ms | 710 ms | 1.75× | `n = n + 1` |
| method | 246 ms | 440 ms | 1.79× | `object:add(constant)` |
| pcall | 94.8 ms | 174 ms | 1.83× | `pcall` of an identity |
| global | 180 ms | 407 ms | 2.26× | `_G.g = _G.g + i` |
| array | 192 ms | 454 ms | 2.36× | dense integer keys |
| hash | 200 ms | 420 ms | 2.10× | short-string key lookup |
| metaindex | 142 ms | 269 ms | 1.90× | stable integer field |
| ipairs | 155 ms | 352 ms | 2.27× | base `ipairs`, dense array |
| concat | 89.7 ms | 276 ms | 3.08× | loop-invariant `a .. b` |
| strings | 81.5 ms | 547 ms | 6.70× | format, concat, gmatch |
| sort | 72.9 ms | 117 ms | 1.61× | `table.sort`, Lua comparator |
| yield | 139 ms | 195 ms | 1.40× | `yield` of `1..n`, then sum |
| alloc | 131 ms | 271 ms | 2.08× | four-element constructor |

Most rows are interpreter-bound and land between about 1.5× and 2.5×. `yield` is closest to C; `concat` (~3×) and `strings` (~6.7×) are the outliers. `strings` keeps that gap after a C-shaped stack-local `luaL_Buffer` (`initb`): each `string.format("%d-…")` still uses Go `fmt`/`strconv` plus short-string interning, not C's `sprintf` into the stack buffer. Process startup is dominated by the Go runtime.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
