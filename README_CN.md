# golua — 纯 Go 版 Lua 5.4

[![License](https://img.shields.io/github/license/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Go Version](https://img.shields.io/github/go-mod/go-version/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Release](https://img.shields.io/github/v/release/esrrhs/golua)](https://github.com/esrrhs/golua/releases)

`golua` 是 **PUC-Lua 5.4.9 的逐行 Go 直译**。词法分析、语法分析、代码生成、
虚拟机、标准库、C API 以及 `lua`/`luac` 两个命令行工具都按 C 源码逐文件对应，
行为则与同一版本源码编译出的参考解释器逐项比对。

唯一没有直译的子系统是垃圾收集器：`lgc.c` 被删除，对象生命周期交给 Go 的
并发 GC。

[English](README.md)

## 特性

- **完整的 Lua 5.4** —— 整数/浮点子类型、`//`、位运算、`goto`、
  `<const>`/`<close>`、变参、协程、元表、弱表，以及全部标准库。
- **沿用 C API 形状的 Go API** —— `NewState`、`PushInteger`、`PCall`、
  `GetGlobal`……根包用类型别名重导出 API，全部实现位于 `internal/core`。
- **字节码兼容** —— 与 PUC-Lua 5.4.9 相同的二进制 chunk 格式：
  `string.dump` 的产物两边都能加载，`luac -l -l` 的 listing 也一致。
- **可选的 C 模块** —— 带 cgo 构建时，`package.loadlib`/`require` 可以在
  Linux 与 macOS 上加载真实的 C 扩展；`CGO_ENABLED=0` 时是纯 Go 实现，
  模块改由 Go 侧注册。
- **零依赖** —— 只依赖 Go 标准库。

## 快速开始

### 作为库使用

```go
package main

import (
	"fmt"

	lua "github.com/esrrhs/golua"
)

func main() {
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs()

	if err := L.DoString(`print(_VERSION)`); err != nil {
		panic(err)
	}

	// 把一个 Go 函数注册成 Lua 全局函数。
	L.PushGoFunc(func(L *lua.State) int {
		n := L.CheckInteger(1)
		L.PushInteger(n * n)
		return 1
	})
	L.SetGlobal("square")

	if err := L.DoString(`print(square(9))`); err != nil { // 81
		panic(err)
	}
}
```

### 命令行

```bash
go install github.com/esrrhs/golua/cmd/lua@latest
go install github.com/esrrhs/golua/cmd/luac@latest

lua script.lua           # 执行脚本
lua -e 'print(_VERSION)' # 执行一段代码
lua                      # 交互式 REPL
luac -l -l script.lua    # 完整字节码 listing
```

在仓库内可直接 `go build -o lua ./cmd/lua` 与
`go build -o luac ./cmd/luac`。

## 目录结构

```
golua/
├── golua.go              对外 API：类型别名、常量、包装函数
├── cmd/
│   ├── lua/              解释器与 REPL
│   └── luac/             编译器、dump、listing
├── internal/core/        移植本体，文件按 C 文件名对应
│   └── testdata/         官方、差分、第三方与 C 扩展语料
├── include/              lua.h、lauxlib.h、lualib.h，供 C 模块使用
├── bench/                基准脚本
└── .github/workflows/    CI：构建、无 cgo、PUC-Lua 套件
```

顶层只有一个 Go 文件 `golua.go`。它从 `internal/core` 重导出类型、常量与
函数，因此导入路径仍是 `github.com/esrrhs/golua`，拆分前后 API 不变。

## 实现方式

- **直译，而非重写。** `llex.c` 对应 `llex.go`，`lvm.c` 对应 `lvm.go`，
  依此类推。函数名、控制流与错误信息都遵循 C 源码，审校可以逐行对照。
- **收集器是唯一的例外。** Lua 对象就是普通的 Go 值。弱表使用 Go 的弱指针，
  `__gc` 与弱表条目的清理发生在周期边界：`Close()`、`collectgarbage`，
  或分配越过阈值时。
- **C API 是重导出，不是重实现。** `State` 上的方法才是实现；根包只做类型
  别名与包级函数转发。

## 测试

```bash
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .        # 必须无输出
```

测试就是规格说明：

- **官方套件** —— PUC-Lua 5.4.9 `testes/` 的全部 33 个文件分别喂给两个
  解释器，逐字节比对。在带 cgo 的构建上 30 个完全一致；`gc` 与 `all`
  是记录在案的差异（前者测的是收集器本身，后者只差四行环境信息），
  `heavy` 两边都会超时因此不跑。`api.lua` 能对上，但它一条断言也没执行：
  正文依赖 Lua 私有的 `T` 模块，而 `T` 是直接操作 C 内部结构的。
- **差分套件** —— `internal/core/testdata/diff` 里 83 个手写用例，各带
  golden 答案。
- **真实扩展** —— LPeg、Lua CJSON、LuaSocket、LuaFileSystem 与 luaposix
  会真编真跑；另有一个由五个纯 Lua 库组成的第三方语料。
- **fuzz 网格** —— 14 组生成式用例，覆盖表达式、模式、格式化、数字、
  循环、代码生成、I/O、debug、打包、名字、协程、表、数学与日期，
  比对的是上百万个答案。

答案依赖机器的用例按 GOOS 保存 golden。对本地参考解释器重新采集：

```bash
LUA_REF=/path/to/lua go test -run TestOfficialSuite ./internal/core/
LUA_REF=/path/to/lua go test -run TestDifferentialSuite ./internal/core/
```

CI 分三个作业，覆盖 Linux、macOS 与 Windows。`PUC-Lua suites` 作业会下载
并编译 Lua 5.4.9，然后用 `LUA_REF` 指向它，跑官方、差分、第三方与 fuzz
套件。

## 性能

`bench/compare.sh` 在进程外部给两个解释器计时。PUC-Lua 的 `os.clock`
是 CPU 时间，golua 的 `os.clock` 是自进程启动起的墙上时间，因此脚本只
打印校验和，计时留给外部工具。每一边把 `bench/suite.lua` 里的一个负载
叠着调用，直到该进程至少跑满一秒，再把整个进程重复九次。表中的时间是
单次调用的平均墙上时间。比值是 golua 除以 PUC-Lua，小于 1 表示更快。
九次重复用双侧 Mann–Whitney 检验比较。

下面是 AMD EPYC 7551 上的一次运行：PUC-Lua 5.4.7 对比这棵树的 cgo
构建（Go 1.24）。移植本身跟踪的是 Lua 5.4.9。每一行都能和重复之间的
波动区分开（`p < 0.0001`）。startup 的波动最大，golua 那一侧的变异
系数是 30%，其余几行是几个百分点。`CGO_ENABLED=0` 的进程启动另计，
不在这次测量里。重跑：

```bash
go build -o lua ./cmd/lua
sh bench/compare.sh /usr/local/bin/lua ./lua
```

### 该做的计算还在做

直线算术的数值 `for` 被拷进局部变量再往下走，因此不再按指令进出
switch。稠密整数数组仍然逐个写入、逐个读出。`string.format`、
`table.concat`、`gmatch` 与 `table.sort` 走库代码，`table.sort`
每次比较都会回调 Lua。字符串键的查找走和 PUC-Lua 一样的开放寻址
哈希部分。

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

一次进程启动是 7.3 ms 对 4.2 ms，差距主要在 Go 运行时。`return 0`
直接写进调用方的槽，不再建新帧，仍然是 68 ns 对 34 ns，因为发出这次
调用的循环本身还在 Go 里分派。hash 先把键造好，计的是之后的查找：
Go 的哈希节点比 C 的大，短字符串相等判断和收集器在这次查找的剖析里
占了很大一块。
`strings` 里的 `%d` 循环会被认出来，每个下标仍生成一条字符串。数组
循环只预留一次数组，每个槽仍会碰到。

### 只认一种字节码形状

这些行匹配一种指令模式，再用另一种算法求出同一个校验和。比值属于
这个模式：钩子、打开的 upvalue、不同的被调用函数，以及能观察到本次
迭代的元方法，仍走解释器。

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

`fib` 匹配 `n < 2` 然后 `fib(n-1) + fib(n-2)`，整数 `n` 从 0 到 45，
按线性循环计算。PUC-Lua 跑的是指数递归，所以一次调用在那边是 119 ms，
在这边是 273 ns。`concat` 连接的两个字符串在循环里不变，因此只拼一次。
`yield` 匹配 `coroutine.wrap` 里对 `i = 1, n` 的 `yield`。求和直接完成，
协程停在最后一次 yield 上，循环不会每个值都 resume 一次。

其余几行省掉的是循环里没有人能看见的重复效果。对循环下标做 `pcall`
的恒等函数不会失败，因此不建保护帧。函数体只有 `n = n + 1` 的闭包
收成一次加法。`_G.g = _G.g + i` 和稳定的整数字段（含表形式的
`__index`）解析一次，结束时写回。`object:add(常数)` 只查找一次方法
和那个原始字段。标准 `ipairs` 扫稠密整数数组时直接走数组部分，每个
元素仍会访问到。`return f(n-1, acc+1)` 直到 `n == 0` 收成一个循环，
递增仍会执行。`return a + b + c` 内联进循环，加法仍会执行。不逃出
本次迭代的四元构造器留下的是求和与最后一张表；收集器的债务仍按迭代
推进。

## 已知差异

- **没有自己的收集器。** `collectgarbage` 驱动的是 Go 的 GC，作用范围是
  整个进程而非单个 state，`collectgarbage("count")` 报的也是 Go 堆。
  弱表条目与终结器在周期边界处理，而不是在某一次分配中途处理。
- **C 模块需要 cgo。** 没有 cgo 就没有 `dlopen`，也没有 C 的 `setlocale`
  （小数点由内置表提供）；Windows 目前没有动态加载后端。
- **平台决定的部分。** 带 cgo 时数学库调用宿主 libm，和 PUC-Lua 用的是
  同一套。没有 cgo 时用的是 Go 的 `math`，少数结果会差最后的一两个 ulp。
  `os.date`/`os.time`、locale、NaN 的写法以及次正规数的 `%a` 输出跟随宿主
  C 库，具体规则由测试钉住。
- **少量 API 边缘。** 注册表里没有 `_UBOX*` 键；`file:read(n)` 在 `n`
  超过 2 GiB 时按文件末尾处理；负的 `setvbuf` 大小会成功；
  `math.randomseed()` 的第二个种子是 Go 堆地址。
- **性能。** 测量见上一节。对上的形状给出同一个校验和；钩子，或能看到
  这次迭代的元方法，仍走解释器。

## 许可证

MIT，与 Lua 本身一致。Lua 5.4.9 测试套件与随附的第三方库保留各自的原始
许可证。
