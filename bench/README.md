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

Intel Xeon (8 cores), **PUC-Lua 5.3.4** against a cgo build of this tree (Go 1.27). `MIN_MS=600 REPS=7`; every row had `p < 0.01`. Startup CV on the golua side was about 21%; other rows a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.22 ms | 3.71 ms | 3.04× | a new process, no script body |
| nop | 25.4 ns | 47.1 ns | 1.85× | `return 0` |
| intloop | 172 ms | 378 ms | 2.20× | integer sum |
| floatloop | 239 ms | 593 ms | 2.48× | float multiply-add |
| bitwise | 252 ms | 730 ms | 2.89× | and, xor, shift |
| fib | 73.2 ms | 117 ms | 1.60× | recursive Fibonacci |
| tailcall | 171 ms | 347 ms | 2.03× | countdown tail call |
| call3 | 242 ms | 437 ms | 1.80× | `return a + b + c` |
| upvalue | 441 ms | 757 ms | 1.72× | `n = n + 1` |
| method | 249 ms | 485 ms | 1.95× | `object:add(constant)` |
| pcall | 98.1 ms | 176 ms | 1.79× | `pcall` of an identity |
| global | 182 ms | 413 ms | 2.27× | `_G.g = _G.g + i` |
| array | 191 ms | 443 ms | 2.32× | dense integer keys |
| hash | 213 ms | 407 ms | 1.91× | short-string key lookup |
| metaindex | 147 ms | 306 ms | 2.08× | stable integer field |
| ipairs | 158 ms | 373 ms | 2.36× | base `ipairs`, dense array |
| concat | 89.4 ms | 237 ms | 2.65× | loop-invariant `a .. b` |
| strings | 82.6 ms | 151 ms | 1.83× | format, concat, gmatch |
| sort | 76.3 ms | 125 ms | 1.64× | `table.sort`, Lua comparator |
| yield | 158 ms | 204 ms | 1.30× | `yield` of `1..n`, then sum |
| alloc | 133 ms | 300 ms | 2.27× | four-element constructor |

Most rows are interpreter-bound and land between about 1.5× and 2.5×. `yield` is closest to C; `concat` (~2.7×) may still stand out. `strings` is now ~1.8× after pooling `initb` slabs and avoiding an escaped 8 KiB `Buffer` on the hot path; `string.format` still uses Go helpers for some conversions. Process startup is dominated by the Go runtime.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
