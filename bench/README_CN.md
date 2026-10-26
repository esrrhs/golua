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

机器是 Linux x86_64（Intel Xeon，内核 6.12.94+），PUC-Lua 5.4.9 对比这棵树的 cgo 构建（Go 1.27.3）。移植本身跟踪 Lua 5.4.9。每一行 `p < 0.0001`。startup 的波动最大：golua 那一侧的变异系数是 18%，其余几行是几个百分点。`CGO_ENABLED=0` 的进程启动另计，不在这次运行里。

### 该做的计算还在做

稠密整数数组仍然逐个写入、逐个读出。`string.format`、`table.concat`、`gmatch` 与 `table.sort` 走库代码，`table.sort` 每次比较都会回调 Lua。字符串键的查找走和 PUC-Lua 一样的开放寻址哈希部分。数值循环仍按指令进出解释器。

| 负载 | PUC-Lua | golua | 比值 | 实际在做的事 |
| --- | ---: | ---: | ---: | --- |
| startup | 1.44 ms | 3.43 ms | 2.38× | 新进程，没有脚本正文 |
| nop | 16.6 ns | 34.9 ns | 2.11× | `return 0` |
| hash | 158 ms | 318 ms | 2.01× | 短字符串键查找 |
| intloop | 111 ms | 259 ms | 2.32× | 整数求和 |
| floatloop | 130 ms | 304 ms | 2.33× | 浮点乘加 |
| bitwise | 131 ms | 342 ms | 2.61× | 与、异或、移位 |
| array | 120 ms | 358 ms | 2.99× | 稠密整数键 |
| strings | 69.6 ms | 86.9 ms | 1.25× | format、concat、gmatch |
| sort | 73.4 ms | 114 ms | 1.55× | `table.sort`，比较函数是 Lua |

一次进程启动是 3.43 ms 对 1.44 ms，差距主要在 Go 运行时。`return 0` 仍然是 34.9 ns 对 16.6 ns，因为发出这次调用的循环本身还在 Go 里分派。hash 先把键造好，计的是之后的查找。Go 的哈希节点比 C 的大，短字符串相等判断和收集器在这次查找的剖析里占了很大一块。数组循环每个槽仍会碰到；`strings` 每个下标仍生成一条字符串。

### 调用与分配形状

这些行各练一种常见字节码模式，校验和相同。两侧都走解释器（没有 Go 侧专用捷径）。

| 负载 | PUC-Lua | golua | 比值 | 形状 |
| --- | ---: | ---: | ---: | --- |
| fib | 38.5 ms | 87.1 ms | 2.27× | 直接递归的 Fibonacci，`n` 在 0..45 |
| concat | 78.3 ms | 176 ms | 2.24× | 循环不改变的 `a .. b` |
| yield | 133 ms | 174 ms | 1.31× | `yield` 出 `1..n` 再求和 |
| pcall | 74.8 ms | 181 ms | 2.42× | 对恒等函数做 `pcall` |
| upvalue | 344 ms | 725 ms | 2.11× | `n = n + 1` |
| global | 152 ms | 343 ms | 2.25× | `_G.g = _G.g + i` |
| ipairs | 135 ms | 309 ms | 2.30× | 标准 `ipairs`，稠密数组 |
| tailcall | 114 ms | 215 ms | 1.89× | 倒计时尾调用 |
| metaindex | 120 ms | 213 ms | 1.78× | 稳定的整数字段 |
| method | 200 ms | 372 ms | 1.86× | `object:add(常数)` |
| call3 | 170 ms | 363 ms | 2.14× | `return a + b + c` |
| alloc | 96.7 ms | 206 ms | 2.13× | 四元构造器 |

`fib` 是 `n < 2` / `fib(n-1) + fib(n-2)` 的递归形状，整数 `n` 从 0 到 45。`concat` 在循环里连接两个字符串。`yield` 用 `coroutine.wrap` 对 `i = 1, n` 做 `yield` 再求和。其余几行分别练 `pcall`、upvalue、全局变量、`ipairs`、尾调用、元方法、方法调用、多参数调用和小表构造——路径与 PUC-Lua 相同，多出的是 Go 分派开销。

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

对 `fib.lua`、`arith.lua`、`sort.lua`、`strings.lua`、`mesh_gc.lua` 各取 `REPS` 次墙上时间里最快的一次（默认 3），再用 `node_size.lua` 报每个节点的大小。它用的是 BSD 的 `time -l`（`/usr/bin/time`），也就是 macOS 上的那种。这些是单个脚本，不是上面的叠跑。这里的 `fib.lua` 就是那个递归函数本身；`compare.sh` 的 `fib` 行是 `suite.lua` 里的同一形状。
