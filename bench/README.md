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

**PUC-Lua 5.3.4** against a build of this tree. `MIN_MS=400 REPS=5`; every stacked row had `p < 0.02`. Startup CV on the golua side was high in this run; other rows a few percent.

### Work that still runs on the interpreter

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 1.16 ms | 9.19 ms | 7.92× | a new process, no script body |
| nop | 23.4 ns | 31.0 ns | 1.32× | `return 0` |
| floatloop | 228 ms | 514 ms | 2.26× | float multiply-add |
| bitwise | 242 ms | 703 ms | 2.90× | and, xor, shift |
| call3 | 231 ms | 485 ms | 2.10× | `return a + b + c` |
| upvalue | 384 ms | 804 ms | 2.09× | `n = n + 1` |
| method | 227 ms | 503 ms | 2.22× | `object:add(constant)` |
| pcall | 93.8 ms | 184 ms | 1.96× | `pcall` of an identity |
| hash | 207 ms | 315 ms | 1.52× | short-string key lookup |
| metaindex | 144 ms | 278 ms | 1.93× | stable integer field |
| ipairs | 154 ms | 356 ms | 2.31× | base `ipairs`, dense array |
| strings | 82.2 ms | 76.9 ms | 0.94× | format, concat, gmatch |
| sort | 72.5 ms | 39.9 ms | 0.55× | `table.sort`, Lua comparator |
| yield | 139 ms | 201 ms | 1.45× | `yield` of `1..n`, then sum |
| alloc | 130 ms | 275 ms | 2.13× | four-element constructor |

### Recognised bytecode shapes

These rows match one 5.3.4 instruction pattern and evaluate it outside the dispatch loop. The checksum stays the same. A hook, open upvalue, different callee, or metamethod declines to the interpreter.

| workload | PUC-Lua | golua | ratio | recognised shape |
| --- | ---: | ---: | ---: | --- |
| intloop | 161 ms | 59.0 ms | 0.37× | integer `s = s + i` |
| fib | 64.3 ms | 100 ns | 1.56×10⁻⁶ | direct Fibonacci, `n` in 0..45 |
| tailcall | 159 ms | 2.48 ms | 0.02× | countdown tail call |
| global | 172 ms | 6.20 ms | 0.04× | `_G.g = _G.g + i` |
| array | 194 ms | 86.2 ms | 0.44× | dense integer fill / sum |
| concat | 88.4 ms | 2.26 ms | 0.03× | loop-invariant `a .. b` |

`fib` matches `n < 2` followed by `fib(n-1) + fib(n-2)` for an integer from 0 to 45 and runs a linear loop. `concat` joins two unchanged strings once. `tailcall` folds `return f(n-1, acc+1)` into one loop. `_G.g = _G.g + i` and dense array fill/sum resolve without per-iteration dispatch.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
