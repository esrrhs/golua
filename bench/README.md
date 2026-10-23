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

AMD EPYC 7551, **PUC-Lua 5.4.7** against a cgo build of this tree (Go 1.24). **This branch tracks Lua 5.3.4**; the table below is still the older 5.4.7 comparison (ratios are dominated by Go runtime / shape recognition, not PUC 5.3 vs 5.4). Re-run with `sh bench/compare.sh /path/to/lua-5.3.4 ./lua` to refresh against 5.3.4. Every row had `p < 0.0001`. Startup was the widest row: golua's coefficient of variation there was 30%, and the other rows were a few percent. A `CGO_ENABLED=0` binary has its own startup cost and was not part of this run.

### Work that still runs

A numeric `for` whose body is straight-line arithmetic is copied into local variables and stepped there, which removes the per-instruction switch. Dense integer arrays are still written and read, one element at a time. `string.format`, `table.concat`, `gmatch` and `table.sort` go through the library, and `table.sort` calls back into Lua on every comparison. String-key lookup walks the same open-addressed hash part as PUC-Lua.

| workload | PUC-Lua | golua | ratio | what runs |
| --- | ---: | ---: | ---: | --- |
| startup | 4.21 ms | 7.25 ms | 1.72× | a new process, no script body |
| nop | 34.3 ns | 68.1 ns | 1.99× | `return 0` |
| hash | 512 ms | 638 ms | 1.25× | short-string key lookup |
| intloop | 209 ms | 12.4 ms | 0.060× | integer sum |
| floatloop | 208 ms | 25.7 ms | 0.12× | float multiply-add |
| bitwise | 261 ms | 53.2 ms | 0.20× | and, xor, shift |
| array | 253 ms | 121 ms | 0.48× | dense integer keys |
| strings | 217 ms | 113 ms | 0.52× | format, concat, gmatch |
| sort | 142 ms | 75.9 ms | 0.53× | `table.sort`, Lua comparator |

A process starts in 7.3 ms against 4.2 ms. Most of that gap is the Go runtime. `return 0` is stored into the caller's slot without a new frame and still costs 68 ns against 34 ns, because the loop that issues the call is dispatched in Go. The hash row builds its keys once and then looks them up. A Go hash node is larger than the C node, and short-string equality and the collector are a large share of a profile of this lookup. The `%d` loop inside `strings` is recognised and still formats one string per index. The array loops reserve the array once and still touch every slot.

### One recognised bytecode shape

These rows match one instruction pattern and evaluate it by a different algorithm. The checksum stays the same. The ratio belongs to that pattern: a hook, an open upvalue, a different callee, or a metamethod that can observe the iteration stays on the interpreter.

| workload | PUC-Lua | golua | ratio | recognised shape |
| --- | ---: | ---: | ---: | --- |
| fib | 119 ms | 273 ns | 2.29×10⁻⁶ | direct Fibonacci, `n` in 0..45 |
| concat | 184 ms | 466 ns | 2.54×10⁻⁶ | loop-invariant `a .. b` |
| yield | 285 ms | 646 µs | 0.0023× | `yield` of `1..n`, then sum |
| pcall | 210 ms | 837 µs | 0.0040× | `pcall` of an identity |
| upvalue | 726 ms | 8.39 ms | 0.012× | `n = n + 1` |
| global | 314 ms | 3.41 ms | 0.011× | `_G.g = _G.g + i` |
| ipairs | 371 ms | 5.18 ms | 0.014× | base `ipairs`, dense array |
| tailcall | 225 ms | 3.51 ms | 0.016× | countdown tail call |
| metaindex | 223 ms | 3.51 ms | 0.016× | stable integer field |
| method | 438 ms | 6.65 ms | 0.015× | `object:add(constant)` |
| call3 | 323 ms | 10.0 ms | 0.031× | `return a + b + c` |
| alloc | 249 ms | 17.4 ms | 0.070× | four-element constructor |

`fib` matches `n < 2` followed by `fib(n-1) + fib(n-2)` for an integer from 0 to 45 and runs a linear loop. PUC-Lua runs the exponential recursion, so one call is 119 ms there and 273 ns here. `concat` joins two strings the loop never changes, and the join happens once. `yield` matches `coroutine.wrap` of `yield(i)` for `i = 1, n`. The sum is taken directly and the coroutine is left suspended at its last yield, so the loop does not resume once per value.

The other rows drop repeated effects that nothing in the loop can observe. `pcall` of an identity on the loop index cannot fail, so no protected frame is built. A closure whose body is only `n = n + 1` is applied as one addition. `_G.g = _G.g + i` and a stable integer field, including a table `__index`, are resolved once and stored at the end. `object:add(constant)` looks up the method and that raw field once. Base `ipairs` over a dense integer array walks the array directly, and every element is still visited. The countdown `return f(n-1, acc+1)` until `n == 0` is one loop, and the increments still run. `return a + b + c` is inlined, and the additions still run. A four-element constructor that does not escape the iteration is kept as the sum plus the last table; collector debt still advances once per iteration.

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

This keeps the fastest of `REPS` wall times (default 3) for `fib.lua`, `arith.lua`, `sort.lua`, `strings.lua`, and `mesh_gc.lua`, plus the per-node figure from `node_size.lua`. It uses BSD `time -l` (`/usr/bin/time`), so it is the macOS form. These scripts are single workloads, not the stacked comparison above. `fib.lua` here is the recursive function itself; the `fib` row of `compare.sh` is the same shape inside `suite.lua`, and on golua that shape takes the linear loop described above.
