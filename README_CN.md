# golua — 纯 Go 版 Lua 5.3.4

[![License](https://img.shields.io/github/license/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Go Version](https://img.shields.io/github/go-mod/go-version/esrrhs/golua)](https://github.com/esrrhs/golua)
[![Release](https://img.shields.io/github/v/release/esrrhs/golua)](https://github.com/esrrhs/golua/releases)

`golua` 是 **PUC-Lua 5.3.4 的逐行 Go 直译**。词法分析、语法分析、代码生成、
虚拟机、标准库、C API 以及 `lua`/`luac` 两个命令行工具都按 C 源码逐文件对应，
行为则与同一版本源码编译出的参考解释器逐项比对。

唯一没有直译的子系统是垃圾收集器：`lgc.c` 被删除，对象生命周期交给 Go 的
并发 GC。

[English](README.md)

## 特性

- **完整的 Lua 5.3.4** —— 整数/浮点子类型、`//`、位运算、`goto`、
  `bit32`、变参、协程、元表、弱表，以及全部标准库（无 5.4 的
  `<const>`/`<close>`）。
- **沿用 C API 形状的 Go API** —— `NewState`、`PushInteger`、`PCall`、
  `GetGlobal`……根包用类型别名重导出 API，全部实现位于 `internal/core`。
- **字节码兼容** —— 与 PUC-Lua 5.3.4 相同的二进制 chunk 格式：
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
├── bench/                基准；说明见 bench/README_CN.md
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

- **官方套件** —— PUC-Lua 5.3.4 `testes/` 的全部 **28** 个文件与参考解释器
  逐字节比对。在带 cgo 的构建上 **25** 个完全一致；`gc` 是记录在案的差异
  （Go GC 与 PUC 收集器的 pace/debt/ephemeron）。`main` 与 `all` 不跑：
  `main.lua` 的 Ctrl-C / SIGINT 子进程 harness 尚无安全自动化（没有
  `laction` 钩子；套件超时下会留下未回收的子进程），而 `all.lua` 会驱动
  `main.lua` 并带几行环境相关输出。`api.lua` 能对上，但一条断言也没执行：
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
并编译 Lua **5.3.4**，然后用 `LUA_REF` 指向它，跑官方、差分、第三方与 fuzz
套件。

## 性能

一次运行，机器是 AMD EPYC 7551：下表比值测自 **PUC-Lua 5.4.7**（Go 1.24 cgo 构建），而本分支跟踪的是 **5.3.4**；差距主要来自 Go 运行时启动与字节码形状识别，而非 PUC 小版本差异。可用 `sh bench/compare.sh /path/to/lua-5.3.4 ./lua` 重跑刷新。比值是 golua / PUC-Lua，小于 1 表示更快。每一行 `p < 0.0001`。细节见 [bench/README_CN.md](bench/README_CN.md)。

| 负载 | 比值 | 负载 | 比值 |
| --- | ---: | --- | ---: |
| startup | 1.72× | fib | 2.29×10⁻⁶ |
| nop | 1.99× | concat | 2.54×10⁻⁶ |
| hash | 1.25× | yield | 0.0023× |
| intloop | 0.060× | pcall | 0.0040× |
| floatloop | 0.12× | upvalue | 0.012× |
| bitwise | 0.20× | global | 0.011× |
| array | 0.48× | ipairs | 0.014× |
| strings | 0.52× | tailcall | 0.016× |
| sort | 0.53× | metaindex | 0.016× |
|  |  | method | 0.015× |
|  |  | call3 | 0.031× |
|  |  | alloc | 0.070× |

`startup`、`nop`、`hash` 更慢。`intloop` 到 `sort` 该做的计算还在做。`fib` 到 `alloc` 只匹配一种字节码形状；别的程序仍走解释器，校验和相同。

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
- **性能。** 见上一节。对上的形状给出同一个校验和。

## 许可证

MIT，与 Lua 本身一致。Lua **5.3.4** `testes/` 套件与随附的第三方库保留各自的原始
许可证。
