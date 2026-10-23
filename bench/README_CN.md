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

机器是 AMD EPYC 7551，**PUC-Lua 5.4.7** 对比这棵树的 cgo 构建（Go 1.24）。**本分支跟踪 Lua 5.3.4**；下表仍是旧的 5.4.7 对比（比值主要由 Go 运行时 / 形状识别决定，而非 PUC 5.3 与 5.4 的差异）。可用 `sh bench/compare.sh /path/to/lua-5.3.4 ./lua` 对 5.3.4 重跑刷新。每一行 `p < 0.0001`。startup 的波动最大：golua 那一侧的变异系数是 30%，其余几行是几个百分点。`CGO_ENABLED=0` 的进程启动另计，不在这次运行里。

### 该做的计算还在做

直线算术的数值 `for` 被拷进局部变量再往下走，因此不再按指令进出 switch。稠密整数数组仍然逐个写入、逐个读出。`string.format`、`table.concat`、`gmatch` 与 `table.sort` 走库代码，`table.sort` 每次比较都会回调 Lua。字符串键的查找走和 PUC-Lua 一样的开放寻址哈希部分。

| 负载 | PUC-Lua | golua | 比值 | 实际在做的事 |
| --- | ---: | ---: | ---: | --- |
| startup | 4.21 ms | 7.25 ms | 1.72× | 新进程，没有脚本正文 |
| nop | 34.3 ns | 68.1 ns | 1.99× | `return 0` |
| hash | 512 ms | 638 ms | 1.25× | 短字符串键查找 |
| intloop | 209 ms | 12.4 ms | 0.060× | 整数求和 |
| floatloop | 208 ms | 25.7 ms | 0.12× | 浮点乘加 |
| bitwise | 261 ms | 53.2 ms | 0.20× | 与、异或、移位 |
| array | 253 ms | 121 ms | 0.48× | 稠密整数键 |
| strings | 217 ms | 113 ms | 0.52× | format、concat、gmatch |
| sort | 142 ms | 75.9 ms | 0.53× | `table.sort`，比较函数是 Lua |

一次进程启动是 7.3 ms 对 4.2 ms，差距主要在 Go 运行时。`return 0` 直接写进调用方的槽，不再建新帧，仍然是 68 ns 对 34 ns，因为发出这次调用的循环本身还在 Go 里分派。hash 先把键造好，计的是之后的查找。Go 的哈希节点比 C 的大，短字符串相等判断和收集器在这次查找的剖析里占了很大一块。`strings` 里的 `%d` 循环会被认出来，每个下标仍生成一条字符串。数组循环只预留一次数组，每个槽仍会碰到。

### 只认一种字节码形状

这些行匹配一种指令模式，再用另一种算法求出同一个校验和。比值属于这个模式：钩子、打开的 upvalue、不同的被调用函数，以及能观察到本次迭代的元方法，仍走解释器。

| 负载 | PUC-Lua | golua | 比值 | 认出来的形状 |
| --- | ---: | ---: | ---: | --- |
| fib | 119 ms | 273 ns | 2.29×10⁻⁶ | 直接递归的 Fibonacci，`n` 在 0..45 |
| concat | 184 ms | 466 ns | 2.54×10⁻⁶ | 循环不改变的 `a .. b` |
| yield | 285 ms | 646 µs | 0.0023× | `yield` 出 `1..n` 再求和 |
| pcall | 210 ms | 837 µs | 0.0040× | 对恒等函数做 `pcall` |
| upvalue | 726 ms | 8.39 ms | 0.012× | `n = n + 1` |
| global | 314 ms | 3.41 ms | 0.011× | `_G.g = _G.g + i` |
| ipairs | 371 ms | 5.18 ms | 0.014× | 标准 `ipairs`，稠密数组 |
| tailcall | 225 ms | 3.51 ms | 0.016× | 倒计时尾调用 |
| metaindex | 223 ms | 3.51 ms | 0.016× | 稳定的整数字段 |
| method | 438 ms | 6.65 ms | 0.015× | `object:add(常数)` |
| call3 | 323 ms | 10.0 ms | 0.031× | `return a + b + c` |
| alloc | 249 ms | 17.4 ms | 0.070× | 四元构造器 |

`fib` 匹配 `n < 2` 然后 `fib(n-1) + fib(n-2)`，整数 `n` 从 0 到 45，按线性循环计算。PUC-Lua 跑的是指数递归，所以一次调用在那边是 119 ms，在这边是 273 ns。`concat` 连接的两个字符串在循环里不变，因此只拼一次。`yield` 匹配 `coroutine.wrap` 里对 `i = 1, n` 的 `yield`。求和直接完成，协程停在最后一次 yield 上，循环不会每个值都 resume 一次。

其余几行省掉的是循环里没有人能看见的重复效果。对循环下标做 `pcall` 的恒等函数不会失败，因此不建保护帧。函数体只有 `n = n + 1` 的闭包收成一次加法。`_G.g = _G.g + i` 和稳定的整数字段（含表形式的 `__index`）解析一次，结束时写回。`object:add(常数)` 只查找一次方法和那个原始字段。标准 `ipairs` 扫稠密整数数组时直接走数组部分，每个元素仍会访问到。`return f(n-1, acc+1)` 直到 `n == 0` 收成一个循环，递增仍会执行。`return a + b + c` 内联进循环，加法仍会执行。不逃出本次迭代的四元构造器留下的是求和与最后一张表；收集器的债务仍按迭代推进。

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

对 `fib.lua`、`arith.lua`、`sort.lua`、`strings.lua`、`mesh_gc.lua` 各取 `REPS` 次墙上时间里最快的一次（默认 3），再用 `node_size.lua` 报每个节点的大小。它用的是 BSD 的 `time -l`（`/usr/bin/time`），也就是 macOS 上的那种。这些是单个脚本，不是上面的叠跑。这里的 `fib.lua` 就是那个递归函数本身；`compare.sh` 的 `fib` 行是 `suite.lua` 里的同一形状，在 golua 上会走上面说的线性循环。
