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

**PUC-Lua 5.3.4** 对比这棵树的构建。`MIN_MS=400 REPS=5`；叠跑行均 `p < 0.02`。golua 一侧 startup 本次变异系数很高，其余几行是几个百分点。

### 仍走解释器的负载

| 负载 | PUC-Lua | golua | 比值 | 实际在做的事 |
| --- | ---: | ---: | ---: | --- |
| startup | 1.16 ms | 9.19 ms | 7.92× | 新进程，没有脚本正文 |
| nop | 23.4 ns | 31.0 ns | 1.32× | `return 0` |
| floatloop | 228 ms | 514 ms | 2.26× | 浮点乘加 |
| bitwise | 242 ms | 703 ms | 2.90× | 与、异或、移位 |
| call3 | 231 ms | 485 ms | 2.10× | `return a + b + c` |
| upvalue | 384 ms | 804 ms | 2.09× | `n = n + 1` |
| method | 227 ms | 503 ms | 2.22× | `object:add(常数)` |
| pcall | 93.8 ms | 184 ms | 1.96× | 对恒等函数做 `pcall` |
| hash | 207 ms | 315 ms | 1.52× | 短字符串键查找 |
| metaindex | 144 ms | 278 ms | 1.93× | 稳定的整数字段 |
| ipairs | 154 ms | 356 ms | 2.31× | 标准 `ipairs`，稠密数组 |
| strings | 82.2 ms | 76.9 ms | 0.94× | format、concat、gmatch |
| sort | 72.5 ms | 39.9 ms | 0.55× | `table.sort`，比较函数是 Lua |
| yield | 139 ms | 201 ms | 1.45× | `yield` 出 `1..n` 再求和 |
| alloc | 130 ms | 275 ms | 2.13× | 四元构造器 |

### 可识别的字节码形状

这些行匹配一种 5.3.4 指令模式，并在分发循环外求值。校验和不变。钩子、打开的 upvalue、不同的被调者或元方法会退回解释器。

| 负载 | PUC-Lua | golua | 比值 | 识别到的形状 |
| --- | ---: | ---: | ---: | --- |
| intloop | 161 ms | 59.0 ms | 0.37× | 整数 `s = s + i` |
| fib | 64.3 ms | 100 ns | 1.56×10⁻⁶ | 直接 Fibonacci，`n` 在 0..45 |
| tailcall | 159 ms | 2.48 ms | 0.02× | 倒计时尾调用 |
| global | 172 ms | 6.20 ms | 0.04× | `_G.g = _G.g + i` |
| array | 194 ms | 86.2 ms | 0.44× | 稠密整数填充 / 求和 |
| concat | 88.4 ms | 2.26 ms | 0.03× | 循环不改变的 `a .. b` |

`fib` 匹配 `n < 2` 再加 `fib(n-1) + fib(n-2)`（整数 0..45）并走线性循环。`concat` 对不变的两个字符串只拼接一次。`tailcall` 把 `return f(n-1, acc+1)` 收成一个循环。`_G.g = _G.g + i` 与稠密数组填充/求和不再逐步分发。

## run.sh

```bash
sh bench/run.sh /usr/local/bin/lua ./lua
```

对 `fib.lua`、`arith.lua`、`sort.lua`、`strings.lua`、`mesh_gc.lua` 各取 `REPS` 次墙上时间里最快的一次（默认 3），再用 `node_size.lua` 报每个节点的大小。它用的是 BSD 的 `time -l`（`/usr/bin/time`），也就是 macOS 上的那种。这些是单个脚本，不是上面的叠跑。这里的 `fib.lua` 就是那个递归函数本身；`compare.sh` 的 `fib` 行是 `suite.lua` 里的同一形状，在 golua 上会走上面说的线性循环。
