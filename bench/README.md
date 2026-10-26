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
| startup | 1.18 ms | 2.89 ms | 2.44× | a new process, no script body |
| nop | 22.3 ns | 40.2 ns | 1.80× | `return 0` |
| intloop | 158 ms | 359 ms | 2.27× | integer sum |
| floatloop | 237 ms | 604 ms | 2.55× | float multiply-add |
| bitwise | 233 ms | 606 ms | 2.60× | and, xor, shift |
| fib | 67.9 ms | 107 ms | 1.57× | recursive Fibonacci |
| tailcall | 154 ms | 272 ms | 1.77× | countdown tail call |
| call3 | 221 ms | 386 ms | 1.75× | `return a + b + c` |
| upvalue | 395 ms | 677 ms | 1.71× | `n = n + 1` |
| method | 219 ms | 459 ms | 2.09× | `object:add(constant)` |
| pcall | 90.7 ms | 204 ms | 2.24× | `pcall` of an identity |
| global | 180 ms | 412 ms | 2.29× | `_G.g = _G.g + i` |
| array | 198 ms | 473 ms | 2.39× | dense integer keys |
| hash | 192 ms | 331 ms | 1.73× | short-string key lookup |
| metaindex | 138 ms | 272 ms | 1.96× | stable integer field |
| ipairs | 156 ms | 311 ms | 1.99× | base `ipairs`, dense array |
| concat | 86.4 ms | 182 ms | 2.10× | loop-invariant `a .. b` |
| strings | 71.8 ms | 93.1 ms | 1.30× | format, concat, gmatch |
| sort | 71.0 ms | 125 ms | 1.76× | `table.sort`, Lua comparator |
| yield | 134 ms | 191 ms | 1.43× | `yield` of `1..n`, then sum |
| alloc | 121 ms | 234 ms | 1.94× | four-element constructor |

Most rows are interpreter-bound and land between about 1.3× and 2.5×. `strings` (1.30×) and `yield` (1.43×) are closest to C: `string.format` writes directly into a non-escaping stack-local `luaL_Buffer` via `PrepBuffSize` + `strconv.AppendInt` and checks the short-string intern table before allocating. Process startup is dominated by the Go runtime.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
