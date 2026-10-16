# fakescript_go → 纯 Go 版 Lua 5.4（逐文件直译 PUC-Lua，去掉 GC）

[![License](https://img.shields.io/github/license/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Language](https://img.shields.io/github/languages/top/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Release](https://img.shields.io/github/v/release/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go/releases)

> **本仓库正在重构**：从自研脚本语言 fakescript 改为 **Lua 5.4.9 的纯 Go 实现**。
> 做法不是"照着语义重写一遍"，而是 **把 PUC-Lua 5.4.9 的 C 源码逐文件、逐函数直译成 Go**，
> 唯一实质性删改是 **移除 `lgc.c`（1748 行 GC）**，对象生命周期全部交给 Go 的并发 GC。
>
> [English](./README.md)

---

## 1. 设计目标

| # | 目标 | 说明 |
|---|------|------|
| G1 | **Lua 脚本一行不改** | 现有 `.lua` 业务代码、第三方 Lua 库（纯 Lua 实现的）无需任何修改即可加载运行 |
| G2 | **覆盖 Lua 5.4 全部语言特性** | 整数/浮点子类型、`//` 整除、位运算、goto/label、`<const>`/`<close>`、变参、协程、元表与全部元方法、弱表、to-be-closed |
| G3 | **标准库完整** | base / string（含完整模式匹配）/ table / math / io / os / coroutine / utf8 / debug / package |
| G4 | **宿主 API 与 C API 一一对应** | `lua_*` / `luaL_*` 语义原样保留，原先用 cgo 调 Lua C 库的 Go 项目改造成本极低 |
| G5 | **零依赖、无 cgo、纯 Go** | 只依赖 Go 标准库；交叉编译、静态链接、Windows/Linux/macOS 一条 `go build` 搞定 |

**非目标**：不追求与 `liblua.a/.so` 的 ABI 字节兼容（那是另一层 c-shared 的事，不在本阶段）；
不加载 C 动态库扩展（改用 Go 原生模块注册替代）。

---

## 2. 核心策略：直译，而非重写

### 2.1 三条铁律

1. **文件名一一对应**：`lvm.c → lvm.go`、`ltable.c → ltable.go`、`lstrlib.c → lstrlib.go` …，审校时可逐行对照 C 源码。
2. **函数名与控制流一一对应**：`luaV_execute` 还是 `luaV_execute`，`luaH_get` 还是 `luaH_get`，`switch` 分支顺序、`#define` 常量名尽量保留；不做"顺手优化"和"重新抽象"。
3. **只在三个地方改写**（其余照抄）：
   - **GC**：`lgc.c` 整体删除，`luaC_*` 全部消失（第 3 节）。
   - **错误处理**：`setjmp/longjmp` → Go `panic/recover`（`luaD_throw` = panic，`luaD_rawrunprotected` = recover）。
   - **内存分配**：`lua_Alloc` / `luaM_*` / `lua_newstate(f,ud)` → 无，直接 Go 堆分配，`NewState()` 即可。

### 2.2 源码对照表

| C 源文件 | Go 目标文件 | 状态 | 备注 |
|---|---|---|---|
| `lobject.h/.c` | `lobject.go` | ⬜ | `TValue`、`Table`、`Proto`、`Upvaldesc`、算术与比较原语 |
| `lstate.h/.c` | `lstate.go` | ⬜ | `lua_State`/`global_State`/`CallInfo`（GC 字段删除） |
| `lstring.c` | `lstring.go` | ⬜ | 字符串驻留表（改为 Go map + 非驻留短串，避免永久泄漏） |
| `ltable.c` | `ltable.go` | ⬜ | 数组部 + 开放寻址哈希部，`luaH_next` 保序（不能用 Go map 替代） |
| `lfunc.c` | `lfunc.go` | ⬜ | `Proto`/`UpVal`/`CClosure`/`LClosure`（GC 链表删除） |
| `ltm.c` | `ltm.go` | ⬜ | 元方法名表与 `luaT_gettm` |
| `lopcodes.h/.c` | `lopcodes.go` | ⬜ | opcode 定义与编码解码（指令内部格式自持） |
| `ldo.c` | `ldo.go` | ⬜ | 调用/保护调用/栈增长（longjmp→panic） |
| `lvm.c` | `lvm.go` | ⬜ | `luaV_execute` 主循环，指令分派 |
| `ldebug.c` | `ldebug.go` | ⬜ | 行号、栈回溯、`luaG_*` 错误信息 |
| `lapi.c` | `lapi.go` | ⬜ | `lua_*` 对外 API |
| `lauxlib.c` | `lauxlib.go` | ⬜ | `luaL_*` 辅助库 |
| `llex.c` | `llex.go` | ⬜ | 词法分析 |
| `lparser.c` | `lparser.go` | ⬜ | 递归下降语法分析 |
| `lcode.c` | `lcode.go` | ⬜ | 代码生成、寄存器分配、跳转回填 |
| `ldump.c` / `lundump.c` | `ldump.go` / `lundump.go` | ⬜ | 自有二进制格式（不与 PUC 字节兼容） |
| `lbaselib.c` | `lbaselib.go` | ⬜ | 基础库 + `_G` |
| `lcorolib.c` | `lcorolib.go` | ⬜ | 协程 |
| `ltablib.c` / `lmathlib.c` / `lstrlib.c` / `lutf8lib.c` | 同名 `.go` | ⬜ | 表/数学/字符串（含模式匹配）/UTF-8 |
| `liolib.c` / `loslib.c` | 同名 `.go` | ⬜ | IO / 系统调用 |
| `ldblib.c` | `ldblib.go` | ⬜ | 调试库（hook 部分做最小实现） |
| `loadlib.c` | `loadlib.go` | ⬜ | `require` + searcher；C 加载器换成 Go 模块注册表 |
| `linit.c` | `linit.go` | ⬜ | `OpenLibs` |
| `lua.c` | `cmd/lua/main.go` | ⬜ | 命令行解释器 + REPL |
| **`lgc.c/.h`** | **（删除）** | ✅ | **由 Go 并发 GC 接管** |

### 2.3 关键类型映射

| C | Go |
|---|---|
| `TValue`（tagged union） | `type Value struct { tt int; i int64; n float64; p any }` |
| `GCObject *` / `CommonHeader` | 删除，直接用 `*Table`/`*LClosure`/`*TString`/`*UserData`/`*State` |
| `lua_State *` | `*State`（协程即新 `*State`） |
| `StkId`（栈上指针） | `int` 栈下标（避免指针失效） |
| `Instruction`（uint32） | `type Instruction struct{ Op int; A, B, C int }`（内部格式，不做位压缩） |
| `lua_Alloc` / `luaM_realloc` | 无（`append`/`make`） |
| `lua_CFunction` | `type GoFunc func(L *State) int` |
| `setjmp/longjmp` | `panic(*LuaError)` / `recover()` |

---

## 3. 移除 GC：交给 Go 并发 GC

`lgc.c` 是唯一被整体删掉的模块。相关语义按下表处理：

| Lua 侧 | C 实现 | 本实现 |
|---|---|---|
| 对象生命周期 | 引用计数无关的三色增量 GC | Go 并发三色 GC，循环引用天然回收 |
| `luaC_barrier*` 写屏障 | 手工维护 | 删除（Go GC 自带混合写屏障） |
| `collectgarbage("collect")` | 完整 GC | `runtime.GC()` |
| `collectgarbage("count")` | 内存统计 | `runtime.ReadMemStats()` |
| `collectgarbage("step"/"setpause"/"setstepmul"/"incremental"/"generational")` | GC 调参 | 语义兼容的空实现（返回合理值，不报错） |
| `collectgarbage("collect")` | 强制回收 | 真的做一次完整收集：清栈上残留槽、清扫弱表、执行待跑的 `__gc` |
| `__gc` 元方法 | GC 触发终结 | `runtime.SetFinalizer` 发现不可达并入队，VM 在分配点（与 PUC-Lua 跑收集步进的位置相同）取出执行 |
| 弱表 `__mode = "k"/"v"/"kv"` | GC 清除 | 清扫时把弱引用换成 `weak.Pointer`（Go 1.24+）并清空槽位，让 Go 判断谁真的不可达；table/userdata/function/thread 生效，字符串/数字/布尔同 PUC-Lua 一样不参与弱化 |
| `lua_newstate(alloc, ud)` | 自定义分配器 | `NewState()` |

> 影响面可控：GC 只影响"什么时候回收"，不影响 Lua 的可观测语义（除了 `__gc` 时机、`collectgarbage("count")` 口径与弱表多存活一轮）。

---

## 4. 架构分层

```
          ┌──────────────────────────────────────────────┐
          │  Go 宿主代码 / cmd/lua（REPL、脚本执行）        │
          └───────────────────┬──────────────────────────┘
                              │  lapi.go (lua_*) / lauxlib.go (luaL_*)
          ┌───────────────────▼──────────────────────────┐
          │  State 栈 · CallInfo 帧 · 注册表 · 全局表       │  lstate.go
          ├──────────────────────────────────────────────┤
          │  lvm.go 指令循环 ──► ltm.go 元方法 ──► ltable  │  ldo.go 调用/保护调用
          ├──────────────────────────────────────────────┤
          │  lobject.go Value │ lstring.go │ lfunc.go     │
          ├──────────────────────────────────────────────┤
          │  前端：llex.go → lparser.go → lcode.go → Proto│
          └──────────────────────────────────────────────┘
                    Go runtime：并发 GC / goroutine / os
```

---

## 5. 实施计划

| 阶段 | 内容 | 产出验收 | 状态 |
|---|---|---|---|
| **P0** | 文档定稿；清空 fakescript 旧实现（保留 git 历史） | `go build` 只剩新代码 | ✅ |
| **P1 内核对象** | `lobject` `lstate` `lstring` `ltable` `lfunc` `ltm` `lopcodes` | 表增删查改、`Next` 保序、字符串驻留单测通过 | ✅ |
| **P2 执行引擎** | `ldo` `lvm` `ldebug` | 直接构造 Proto 跑通算术/调用/返回/元方法/错误回溯 | ✅ |
| **P3 前端** | `llex` `lparser` `lcode` | `Load("return 1+2")` → `Call` → `3`；goto/for/vararg/闭包 | ✅ |
| **P4 API 层** | `lapi` `lauxlib` `linit` | 宿主注册 Go 函数、userdata、registry 引用 | ✅ |
| **P5 标准库** | base/coroutine/table/math/string/utf8/os/io/debug/package | 官方 `lua-5.4.9-tests` 子集全绿 | ✅ |
| **P6 工具链** | `cmd/lua`（脚本执行 + REPL）、`ldump`/`lundump` | `go run ./cmd/lua test.lua` | ✅ |
| **P7 验收** | 官方测试集 + 自建用例 + 基准 + CI（Linux/macOS/Windows） | 全平台 `go test -race` 通过 | 🔄 |

### 5.1 一致性实测（官方测试集）

把 PUC-Lua 5.4.9 `testes/` 的 33 个用例分别喂给参考解释器与 `./cmd/lua`，
归一化掉程序路径、`0x…` 地址、耗时、`os.tmpname` 结果、随机数种子后逐字节比对
（`all.lua` 需要预编译 C 扩展，改为按文件驱动；`main.lua` 会派生子进程，也包含在内）。

**31 个可比用例中 28 个输出完全一致**（`all.lua` 需要预编译 C 扩展，`heavy.lua`
两边都超时，都不计入）。其余 3 个的差异全部来自设计取舍（没有自己的收集器）或测试环境：

| 用例 | 差异原因 |
|---|---|
| `gc` | 两处口径：`collectgarbage("count")` 报的是 Go 堆（PUC-Lua 报自己的分配器）；`collectgarbage("step", n)` 的粒度——PUC-Lua 的 step 只做定量工作，故 `dosteps(10) < dosteps(2)`，而 Go 的收集器要么全做要么不做，一次 step 就是一整轮，两边都是 1 次 |
| `literals` | `os.setlocale("pt_BR")` 在本实现不可用（见 §7），跳过了小数点用例 |
| `attrib` | 需要预编译的 C 扩展（`package.loadlib`） |

弱表与 `__gc` 均已实现并通过各自用例——`closure`、`db`、`coroutine`、`gengc`、
`cstack`、`locals`（后两个用 `require"tracegc"` 数回收次数），做法见 §7。

`go test` 内也镜像了这套比对：`testdata/diff/*.lua`（52 个用例）与从 PUC-Lua 5.4.9
采集的 golden 文件逐字节比对；`cli_test.go` 另外钉住解释器自身的可观察行为——
`-v`/`-e`/`-l`/`-E`/`-W`/`--`、`arg` 表、脚本可变参数、usage 文案与退出码。

### 5.2 性能

`go test -bench=. -benchmem -run=XXX` 跑 `bench_test.go` 里的基准。
下表是同一份工作分别交给两个解释器端到端跑出来的耗时
（Apple M1 Pro，PUC-Lua 5.4.9 以 `-O2` 编译，取三次最好成绩）：

| 负载 | PUC-Lua 5.4.9 | 本实现 | 倍率 |
|---|---|---|---|
| `fib(26)`，递归调用 | 17.7 ms | 33.0 ms | 1.9× |
| 算术循环，10⁷ 次 | 84.2 ms | 415.0 ms | 4.9× |
| 整数键表，10⁶ 次写入+读取 | 25.1 ms | 71.1 ms | 2.8× |
| `table.sort`，10⁵ 个元素 | 38.4 ms | 58.4 ms | 1.5× |
| 闭包 upvalue 自增，2×10⁶ 次 | 51.5 ms | 128.3 ms | 2.5× |
| 协程 yield/resume，2×10⁵ 次 | 25.7 ms | 140.7 ms | 5.5× |
| `pcall` 一个 Lua 函数，5×10⁵ 次 | 60.8 ms | 80.3 ms | 1.3× |
| `string.gmatch` + `gsub`，1 MB | 26.8 ms | 41.1 ms | 1.5× |
| `verybig.lua`：写 120 KB 源码再编译 | 110 ms | 250 ms | 2.3× |

慢在哪：`Instruction` 是结构体而非位压缩的 uint32（每条指令要现解码），
每次 `TValue` 访问都要过一遍类型分支，协程跑在独立的 goroutine 上，
一次 yield/resume 是一次 channel 往返而不是换栈指针。

---

## 6. 用法（目标形态，P4 后可用）

```go
package main

import (
	"fmt"
	lua "github.com/esrrhs/fakescript_go"
)

func main() {
	L := lua.NewState()
	defer L.Close()
	L.OpenLibs() // 打开全部标准库

	// 1) 直接执行 Lua 代码
	if err := L.DoString(`print("hello from lua", _VERSION)`); err != nil {
		panic(err)
	}

	// 2) 加载并调用 Lua 函数
	_ = L.LoadString(`function add(a, b) return a + b end`, "=chunk")
	_ = L.PCall(0, 0, 0)
	L.GetGlobal("add")
	L.PushInteger(3)
	L.PushInteger(4)
	L.Call(2, 1)
	fmt.Println(L.ToInteger(-1)) // 7

	// 3) 注册 Go 函数给 Lua（等价于 lua_pushcfunction）
	L.PushGoFunc(func(L *lua.State) int {
		n := L.CheckInteger(1)
		L.PushInteger(n * n)
		return 1
	})
	L.SetGlobal("square")
	_ = L.DoString(`print(square(9))`) // 81
}
```

命令行：

```bash
go build -o lua ./cmd/lua
./lua script.lua      # 执行脚本
./lua                 # REPL
```

---

## 7. 已知差异与限制（相对 PUC-Lua 5.4.9）

1. **`file:read(n)` 的计数超过 2 GiB 时按文件末尾处理**：C 把计数直接交给分配器，荒谬的计数是被分配器拒绝的（报 "not enough memory"；负数计数必然如此，本实现已还原这一点）。本实现不会做那个量级的分配，所以过大的计数读到空。二进制 chunk 不受影响：`string.dump` 产出的字节与 PUC-Lua 5.4.9 **完全一致**（strip 与否都一样），两边互相都能加载对方的产物。
2. **不加载 C 动态库**：`package.cpath` / `package.loadlib` 保留符号但返回不支持；C 扩展请用 Go 模块注册（`RegisterModule` / `package.preload`）改写。
3. **收集器是 Go 的，可观测时机与 PUC-Lua 不同**：`collectgarbage("count")` 报的是 Go 堆而非 PUC-Lua 分配器的口径；`__gc` 在本实现自己跑一轮的地方执行（收集步进、`collectgarbage`、`Close()`），而不是 PUC-Lua 步进的位置。带 `__gc` 的对象在两轮之间被强引用兜住，避免 Go 在自己挑的时刻终结它们；`Close()` 时会把**所有**还带 `__gc` 的对象跑一遍，可达的也算——这正是 C 的做法，也正是 `<close>`、`Close()` 与弱表确定的原因：没有别处引用就会被清掉。
4. **弱表条目要多存活一轮**：Go 的 `weak.Pointer` 在释放对象的**下一轮** GC 才失效，因此条目比 PUC-Lua 多活一个周期。字符串、数字、布尔不参与弱化，与 PUC-Lua 一致。
5. **错误信息文案**：与 PUC-Lua 保持一致（直译带来的好处），但 `ldebug` 的源码片段回显在部分路径可能省略。
6. **只支持 `"C"` locale**：`os.setlocale` 背后没有 libc 的 locale 数据库，其它名字一律返回 `nil`（与 PUC-Lua 在该 locale 未安装时的行为一致）；数字格式化与解析始终用 `.` 作小数点。
7. **`os.time` 接受宿主 `mktime` 拒绝的日期**：PUC-Lua 用 C 库的 `mktime` 构造时间，返回 -1 时报 “time result cannot be represented in this installation”。哪些日期会被拒绝由 C 库决定：macOS 上所有归一化后落在 1900 年之前的都被拒，glibc 则接受。Go 的 `time.Date` 没有这个上限，所以本移植返回一个数值。PUC-Lua 自己判定的部分全部复现——字段进入 `struct tm` 前必须满足的边界（"field 'year' is out-of-bound"）、缺失字段与非整数字段，以及结果恰好为 -1 的情形。
8. **数学库是 Go 的，不是平台 libm**：`math.sin`、`cos`、`tan`、`exp`、`asin`、`acos`、`atan` 用的是 Go 的实现，与 libc 链接的那份不同。16 位有效数字一致，约四分之一的入参最后一位差一个 ulp；`sqrt`、`log` 与所有算术运算完全一致。NaN 按平台 C 库的写法拼写（macOS 恒为 `"nan"`，glibc 符号位为 1 时是 `"-nan"`），因为 C 的 `printf` 就是这么做的。
9. **性能**：`Instruction` 使用结构体而非位压缩 uint32，分派有额外开销；P7 阶段做基准后再决定是否改回位编码。

---

## 8. 构建与测试

```bash
go build ./...            # 构建
go test ./...             # 单元测试（含 Lua 语义用例）
go test -race ./...       # 竞态检测
go vet ./...
gofmt -l .                # 必须无输出
go test -bench=. -benchmem -run=XXX ./...   # 基准
go build -o lua ./cmd/lua # 命令行解释器
```

CI 在 Linux / macOS / Windows 上跑完整矩阵（`.github/workflows/go.yml`）：
全平台 `go vet` + `go build` + `go test`，Linux 与 macOS 加跑 `go test -race`
（竞态检测需要 cgo），Linux 上另有 `gofmt` 检查与一轮 `benchtime=1x` 的基准自检。

参考源码用于对照审校（不入库）：`lua-5.4.9`，<https://www.lua.org/ftp/lua-5.4.9.tar.gz>。

---

## 9. 许可证

MIT（与原仓库一致）。
