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

Intel Xeon (8 cores), **PUC-Lua 5.3.4** against a cgo build of this tree (Go 1.27). `MIN_MS=600 REPS=7`; every row had `p < 0.01`. Startup CV on the golua side was about 15%; other rows a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.27 ms | 3.38 ms | 2.67× | a new process, no script body |
| nop | 24.2 ns | 40.1 ns | 1.66× | `return 0` |
| intloop | 162 ms | 347 ms | 2.15× | integer sum |
| floatloop | 226 ms | 520 ms | 2.31× | float multiply-add |
| bitwise | 229 ms | 698 ms | 3.05× | and, xor, shift |
| fib | 60.8 ms | 99.6 ms | 1.64× | recursive Fibonacci |
| tailcall | 154 ms | 278 ms | 1.80× | countdown tail call |
| call3 | 219 ms | 386 ms | 1.76× | `return a + b + c` |
| upvalue | 385 ms | 693 ms | 1.80× | `n = n + 1` |
| method | 232 ms | 403 ms | 1.74× | `object:add(constant)` |
| pcall | 95.5 ms | 170 ms | 1.78× | `pcall` of an identity |
| global | 187 ms | 366 ms | 1.96× | `_G.g = _G.g + i` |
| array | 189 ms | 435 ms | 2.30× | dense integer keys |
| hash | 219 ms | 400 ms | 1.83× | short-string key lookup |
| metaindex | 140 ms | 259 ms | 1.85× | stable integer field |
| ipairs | 146 ms | 361 ms | 2.47× | base `ipairs`, dense array |
| concat | 89.8 ms | 230 ms | 2.55× | loop-invariant `a .. b` |
| strings | 81.5 ms | 545 ms | 6.69× | format, concat, gmatch |
| sort | 72.2 ms | 114 ms | 1.58× | `table.sort`, Lua comparator |
| yield | 138 ms | 180 ms | 1.30× | `yield` of `1..n`, then sum |
| alloc | 127 ms | 266 ms | 2.10× | four-element constructor |

Most rows are interpreter-bound and land between about 1.6× and 2.5×. `yield` is closest to C; `bitwise` is about 3×. `strings` is the outlier (~6.7×): almost all of the gap is `string.format` allocating an 8 KiB heap buffer on every call (C keeps `LUAL_BUFFERSIZE` on the stack). Process startup is dominated by the Go runtime.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
