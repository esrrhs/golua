# 基准

[English](README.md)

这里有两套工具。根 README 里的比值来自 `compare.sh`。`run.sh` 计的是另一组更小的脚本，不是那张表。

## compare.sh

```bash
go build -o lua ./cmd/lua
sh bench/compare.sh /usr/local/bin/lua ./lua
MIN_MS=500 REPS=9 sh bench/compare.sh /usr/local/bin/lua ./lua
ONLY="fib hash" sh bench/compare.sh /usr/local/bin/lua ./lua
```

时钟在进程外部读。PUC-Lua 的 `os.clock` 是 CPU 时间，golua 的 `os.clock` 是自进程启动起的墙上时间，所以 `suite.lua` 只打印校验和。

每个负载是 `suite.lua` 里的一个函数。单次调用常常只有几毫秒，其中大部分是进程启动，因此那一次不是测量。每个解释器各自把调用叠起来，直到一个进程至少跑满 `MIN_MS`（默认 1000）。次数按解释器分开选。这个进程再重复 `REPS` 次（默认 9，最多 25）。报出的时间是单次调用的平均墙上时间。比值是 golua / PUC-Lua，小于 1 表示更快。`compare.awk` 还打印变异系数、绝对差和相对差，以及对这些重复做的精确双侧 Mann–Whitney U 检验。`noise` 表示 `p >= ALPHA`（默认 0.05）：差距落在波动里。`short` 表示那一侧从未达到 `MIN_MS`，启动时间仍然占主导。解释器一次只跑一个。`startup` 是进程本身，不叠调用。`ONLY` 只留点名的行。调用多于一次时，校验和折成一个和，跳过迭代的运行对不上。

## 根 README 里的那一次

机器是 Intel Xeon（8 核），**PUC-Lua 5.3.4** 对比这棵树的 cgo 构建（Go 1.27）。`MIN_MS=600 REPS=7`；每一行 `p < 0.01`。golua 一侧 startup 的变异系数大约 12%，其余几行是几个百分点。`CGO_ENABLED=0` 的进程启动另计，不在这次运行里。

| 负载 | PUC-Lua | golua | 比值 | 实际在做的事 |
| --- | ---: | ---: | ---: | --- |
| startup | 1.38 ms | 3.58 ms | 2.59× | 新进程，没有脚本正文 |
| nop | 25.7 ns | 38.8 ns | 1.51× | `return 0` |
| intloop | 169 ms | 338 ms | 2.00× | 整数求和 |
| floatloop | 228 ms | 538 ms | 2.36× | 浮点乘加 |
| bitwise | 241 ms | 597 ms | 2.48× | 与、异或、移位 |
| fib | 63.1 ms | 104 ms | 1.65× | 递归 Fibonacci |
| tailcall | 155 ms | 310 ms | 2.01× | 倒计时尾调用 |
| call3 | 234 ms | 390 ms | 1.67× | `return a + b + c` |
| upvalue | 405 ms | 710 ms | 1.75× | `n = n + 1` |
| method | 246 ms | 440 ms | 1.79× | `object:add(常数)` |
| pcall | 94.8 ms | 174 ms | 1.83× | 对恒等函数做 `pcall` |
| global | 180 ms | 407 ms | 2.26× | `_G.g = _G.g + i` |
| array | 192 ms | 454 ms | 2.36× | 稠密整数键 |
| hash | 200 ms | 420 ms | 2.10× | 短字符串键查找 |
| metaindex | 142 ms | 269 ms | 1.90× | 稳定的整数字段 |
| ipairs | 155 ms | 352 ms | 2.27× | 标准 `ipairs`，稠密数组 |
| concat | 89.7 ms | 276 ms | 3.08× | 循环不改变的 `a .. b` |
| strings | 81.5 ms | 547 ms | 6.70× | format、concat、gmatch |
| sort | 72.9 ms | 117 ms | 1.61× | `table.sort`，比较函数是 Lua |
| yield | 139 ms | 195 ms | 1.40× | `yield` 出 `1..n` 再求和 |
| alloc | 131 ms | 271 ms | 2.08× | 四元构造器 |

多数字节码密集行大约 1.5–2.5×。`yield` 最接近 C；`concat`（约 3×）与 `strings`（约 6.7×）是离群值。`strings` 在栈上 `luaL_Buffer`（`initb`）对齐后仍保持该差距：每次 `string.format("%d-…")` 仍走 Go 的 `fmt`/`strconv` 与短串 intern，而不是 C 那样 `sprintf` 写进栈缓冲。进程启动主要是 Go 运行时的成本。

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

对 `fib.lua`、`arith.lua`、`sort.lua`、`strings.lua`、`mesh_gc.lua` 各取 `REPS` 次墙上时间里最快的一次（默认 3），再用 `node_size.lua` 报每个节点的大小。它用的是 BSD 的 `time -l`（`/usr/bin/time`），也就是 macOS 上的那种。这些是单个脚本，不是上面的叠跑。这里的 `fib.lua` 就是那个递归函数本身；`compare.sh` 的 `fib` 行是 `suite.lua` 里的同一形状，在 golua 上会走上面说的线性循环。
