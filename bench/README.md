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

Intel Xeon (8 cores), **PUC-Lua 5.3.4** against a cgo build of this tree (Go 1.27). `MIN_MS=600 REPS=7`; every row had `p < 0.01`. Startup CV on the golua side was about 10%; other rows a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.27 ms | 3.02 ms | 2.38× | a new process, no script body |
| nop | 23.4 ns | 28.8 ns | 1.23× | `return 0` |
| intloop | 156 ms | 338 ms | 2.16× | integer sum |
| floatloop | 225 ms | 549 ms | 2.45× | float multiply-add |
| bitwise | 233 ms | 613 ms | 2.63× | and, xor, shift |
| fib | 64.8 ms | 123 ms | 1.89× | recursive Fibonacci |
| tailcall | 174 ms | 300 ms | 1.73× | countdown tail call |
| call3 | 231 ms | 445 ms | 1.93× | `return a + b + c` |
| upvalue | 385 ms | 759 ms | 1.97× | `n = n + 1` |
| method | 229 ms | 471 ms | 2.06× | `object:add(constant)` |
| pcall | 108 ms | 199 ms | 1.84× | `pcall` of an identity |
| global | 175 ms | 407 ms | 2.32× | `_G.g = _G.g + i` |
| array | 192 ms | 420 ms | 2.19× | dense integer keys |
| hash | 188 ms | 329 ms | 1.75× | short-string key lookup |
| metaindex | 141 ms | 263 ms | 1.86× | stable integer field |
| ipairs | 150 ms | 336 ms | 2.23× | base `ipairs`, dense array |
| concat | 91.4 ms | 119 ms | 1.31× | loop-invariant `a .. b` |
| strings | 81.4 ms | 69.6 ms | 0.86× | format, concat, gmatch |
| sort | 72.1 ms | 40.8 ms | 0.56× | `table.sort`, Lua comparator |
| yield | 143 ms | 205 ms | 1.44× | `yield` of `1..n`, then sum |
| alloc | 134 ms | 318 ms | 2.37× | four-element constructor |

Most rows are interpreter-bound and land between about 1.2× and 2.6×. `strings` and `sort` spend most of their time in library code and come out ahead. Process startup is dominated by the Go runtime.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
