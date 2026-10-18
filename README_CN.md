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
| `luac.c` | `luac.go` + `cmd/luac/main.go` | ⬜ | 独立编译器：dump / strip / 只解析 / listing |
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
| `collectgarbage("count")` | Lua 分配器发出的 Kbytes | Go 堆的 Kbytes（`runtime.ReadMemStats`） |
| `collectgarbage("collect"/"step")` | 启动一次收集 | 跑一轮 Go 自己的收集：不可达者被终结，弱表条目被清扫 |
| `collectgarbage("step")` 的回答 | 这一步是否跑完了一整轮——基本步进为 `false` | 基本步进（不带参数）回答 `false`，与 C 一致；给了工作量的回答 `true`，因为这里任何一步都是一整轮。不能两个都答 `false`：`repeat until collectgarbage("step", siz)` 这个 `gc.lua` 用来强制收集的写法会永远出不来 |
| `collectgarbage("stop"/"restart"/"isrunning")` | 停止/重启/报告收集器 | 状态上记下并如实回答，`stop` 真的会停掉分配驱动的那次步进 |
| `collectgarbage("setpause"/"setstepmul")` | 调参，并返回被替换的旧值 | 记住并回答（与 C 相同的 200 与 100）；Go 的收集器不理会它们 |
| `collectgarbage("generational"/"incremental")` | 切换模式，并返回被替换的旧模式 | 记住并回答；新开的 state 回答 `generational`，与 5.4 一致 |
| `lua_gc`（C API，见 `cdl_gc_cb`） | 同上，来自 C | 同一轮周期，Lua 侧与 C 侧走到的是同一个收集器 |
| `__gc` 元方法 | GC 触发终结 | `runtime.SetFinalizer` 发现不可达并入队，VM 在周期边界取出执行：分配驱动的步进（与 PUC-Lua 跑收集步进的位置相同）、`collectgarbage`，或 `Close()` |
| 弱表 `__mode = "k"/"v"/"kv"` | GC 清除 | 清扫时把弱引用换成 `weak.Pointer`（Go 1.24+）并清空槽位，让 Go 判断谁真的不可达；table/userdata/function/thread 生效，字符串/数字/布尔同 PUC-Lua 一样不参与弱化 |
| `lua_newstate(alloc, ud)` | 自定义分配器 | `NewState()` |

> 影响面可控：GC 只影响"什么时候回收"，以及"回收了多少"这两个数字。
> `collectgarbage` 现在与 C 一样真的跑一轮（`collect`/`step`），所以 `__gc` 与弱表随叫随到；
> 只有 `count` 的口径（整个 Go 堆，而非本 state）和"释放了多少内存"这类对分配器本身的度量对不上。

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
| **P6 工具链** | `cmd/lua`（脚本执行 + REPL）、`cmd/luac`（编译器）、`ldump`/`lundump` | `go run ./cmd/lua test.lua` | ✅ |
| **P7 验收** | 官方测试集（33 个入库，`go test` 一等公民）+ 自建用例 + 基准 + CI（4 个作业） | 全平台 `go test -race` 通过 | ✅ |

### 5.1 一致性实测（官方测试集）

把 PUC-Lua 5.4.9 `testes/` 的 33 个用例分别喂给参考解释器与 `./cmd/lua`，
归一化掉程序路径、`0x…` 地址、耗时、`os.tmpname` 结果、随机数种子后逐字节比对
（`all.lua` 需要预编译 C 扩展，改为按文件驱动；`main.lua` 会派生子进程，也包含在内）。

**33 个用例中 30 个输出完全一致**。其余 3 个的差异全部来自设计取舍
（没有自己的收集器）或测试环境：

| 用例 | 差异原因 |
|---|---|
| `gc` | 这个文件测的**就是**收集器，而这里没有。它的断言量的是增量式收集器：step 之后 `gcinfo()` 会下降、步长越大所需步数越少、一次超大的 step 能跑完一整轮、弱表会被清扫——第一条需要"真的收过一次"的断言就挂。之后该文件靠「一直分配表直到 `gcinfo()` 翻三倍」来量收集器的**速度**——没有回收器时这个循环不终止 |
| `all` | 环境差异，集中在四行：第二个随机数种子（PUC-Lua 截成 32 位，这里是 64 位地址）、`total memory: 44.6K` 对 `619.2K`（同一处口径）、耗时、以及解释器自身路径。套件其余部分全部吻合——包括 `attrib`、`files`、`gengc`、`locals` 加载的 C 扩展库 |
| `heavy` | 两边都超时（每轮要构造 2 MB 的表，总量 2 GB）；在超时截断之前两边一致 |

弱表与 `__gc` 均已实现：条目在只有表引用它时被清掉，`__gc` 会跑，两者都发生在
周期边界——分配驱动的步进、`collectgarbage`，或 `Close()`。`closure`、`db`、
`cstack`、`events`、`tracegc` 与 80 个 golden 用例走的都是这条路，做法见 §7。

`coroutine`、`gengc`、`locals` 三个用例断言的正是"`collectgarbage` 之后立刻查
一次收集结果"——该消失的弱表条目（`C[1] == undef`、`t[1] == nil`），或该跑过的
终结器（`locals` 用 `require"tracegc"` 数次数）。`collectgarbage` 现在真的跑一轮，
三者均已逐字节一致。

`__gc` 还保留了 PUC-Lua 的**顺序**规则，而 `main.lua` 正是检查它的那个用例：
终结器内部的分配不会驱动一个周期（C 的 `GCTM` 期间置了 `gcstp & GCSTPGC`），
所以一个终结器绝不会在另一个跑到一半时插进来；那里的 `collectgarbage` 回答
`nil` 而不是 `0`。在终结器里关闭状态——`os.exit(0, true)` 就是这么做的——会在
`exit` **之前**、就地跑完剩下的终结器：`main.lua` 的 "bug since 5.2" 用例在
`__gc` 里创建对象、问 `collectgarbage`、关状态，并要求另一个对象的元方法在
这一切之后才跑（`creating 1 / creating 2 / 2 / creating 3 / nil / 1`），
而且在终结器里创建的那个对象根本不会被终结——状态已经在退场了。

`go test` 内也镜像了这套比对：`testdata/diff/*.lua`（80 个用例）与从 PUC-Lua 5.4.9
采集的 golden 文件逐字节比对；`cli_test.go` 另外钉住解释器自身的可观察行为——
`-v`/`-e`/`-l`/`-E`/`-W`/`--`、`arg` 表、脚本可变参数、usage 文案与退出码。

**这 33 个官方用例现在就躺在仓库里，是 `go test` 的一等公民**：`testdata/official/`
存着 `testes/` 的全部 33 个 `.lua`（同 MIT 许可一起再分发），旁边是
`<name>.<goos>.expected`——参考解释器源码编译出来后对该文件打印的内容。
`official_test.go` 一边跑参考、一边跑本实现，归一化后逐字节比对，于是
`go test ./...` 里就带上了 30 个全等 / 2 个已知差异 / 1 个不跑。它有几个刻意的安排：

- **`testdata/official/libs/*.c`（Lua 自带 makefile 里那五个 C 库）会在副本里
  现编出来**，于是 `attrib.lua` 与 `main.lua` 真的跑了套件的 C 那一半：带版本号的
  模块名（`lib2-v2`）、`luaopen_` 链、以及一个库借用在它之前打开的那个库的符号。
  没有它们，这两个文件都停在「cannot load dynamic library」，上面这些一行也没测到。
- 用例跑在 `t.TempDir()` 里的一份副本上。`attrib.lua` 会往 `libs/` 写十几个
  chunk 再 `require` 它们，`main.lua` 会派生子进程——不能写进 `testdata/`，
  而且一次残留会改变下一次的答案。
- 退出码不算在比对里。这些文件里有好几个**就是**以一个失败的 `assert` 收尾的。
- `heavy` 不跑（两边都超时）；`all` / `gc` / `coroutine` / `gengc` / `locals`
  是登记在案的差异，哪个哪天变得全等了，测试会专门说一句——那是进展，不该被埋掉。
- `attrib` 与 `literals` 的差异取决于**构建**：没有 cgo 就没有 dlopen 与
  `setlocale`，而这两个文件测的正是这两样。测试不猜 `CGO_ENABLED`，它去问
  刚编出来的那个二进制（`package.loadlib` 返回的第三个值、`os.setlocale`
  是否答得上来），因此 `CGO_ENABLED=0 go test ./...` 也照样通过。
- 换 Lua 版本或换平台后重新采集：`LUA_REF=/path/to/lua go test -run TestOfficialSuite`。
  参考解释器必须按 CI 的编法来（`make posix` 加 `-DLUA_USE_DLOPEN`，macOS 上再加
  `SYSLIBS=`）：`posix` 是唯一在各平台一致的构建目标，而且它不带 readline——
  带 readline 时参考解释器碰到管道输入**一个提示符都不打**，而 `main.lua` 是要测提示符的。
- **`api.lua` 是全等的，但它一条断言也没跑**，测试每次都会专门说一句。它的正文整个
  在 `if T==nil then return end` 后面，而 `T` 来自 Lua 自带的 `ltests.c`——
  那个文件是 `LUA_CORE`：它 include 了 `lstate.h` `ltable.h` `lcode.h`
  `lopcodes.h`，手工摆弄 `GCObject`、`CallInfo`、`TValue`（`L->ci->func.p`、
  `s2v`、`setobj2s`）。这里根本没有 C 的 `lua_State` 可以交给它，所以它编译不过来。
  用 Go 重写 `T` 也补不完：`api.lua` 向 `T` 要的东西里有三分之一是**调试分配器**
  （`T.alloccount`、`T.totalmem`：设一个上限，之后的分配就失败）和收集器自身的状态
  （`T.gcstate`、`T.gccolor`），这些在这里都不存在。这跟 `gc.lua` 是同一种情况，
  不是能补上的缺口；C API 的覆盖由 §5.7–§5.9、五个真实 C 扩展和 `capi_test.go` 承担。

### 5.2 一个真实的 C 扩展：LPeg

`go test` 还会用 `testdata/lpeg/` 里的源码编译 [LPeg](http://www.inf.puc-rio.br/~roberto/lpeg/)
1.1.0（六个编译单元，是发布在外、被广泛使用的第三方扩展），并跑 LPeg 自带的测试套件到
最后一行。这比「再写一个为测试而生的扩展」有价值得多，因为 LPeg 会去碰小扩展永远
碰不到的那部分 C API，也因此找出了四个别的测试都没发现的真 bug：

| LPeg 做的事 | 错在哪 |
|---|---|
| 对 userdata 调 `lua_rawlen`，读回它拿到的块有多大 | 大小从来没记录；userdata 回答的是「用户值个数」，于是 LPeg 推出负长度、分配 0 字节、越界拷贝 |
| 调 `lua_getallocf`，然后**直接调用**拿到的函数指针 | 无分配器可交出，LPeg 跳到了 0 地址 |
| 用 `luaL_Buffer`：既走 `luaL_addchar`，也往 `luaL_prepbuffsize` 返回的地址里写 | 那个地址是 NULL，且结果把占位符留在了栈上——栈被顶偏，随后的 `lua_call` 调错了东西 |
| 用 `lua_tostring` 区分「用字符串命名的规则」和「用表命名的规则」 | PUC-Lua 回答 NULL 的地方我们回答了空串，LPeg 因此走了错分支 |

它打印的每一行都取自 PUC-Lua 5.4.9 的实跑结果，存在 `testdata/lpeg/expected.txt`，
所以这个测试既不需要参照解释器，也不需要联网。二者均为 MIT 许可，随
`testdata/lpeg/LICENSE` 一并再分发。

#### 为 PUC-Lua 编好的 .so 可以直接用

LPeg、CJSON、LuaFileSystem 另外还**用参照解释器自己的头文件**（`-I /tmp/lua-5.4.9/src`，
不是 `include/`）各编了一份，不重新编译直接加载，输出与 PUC-Lua 逐行一致。
所以现成的 `.so`（比如 LuaRocks 装的模块）不用重编，但有三个条件，每条都实测过：

| 条件 | 原因 | 怎么查 |
|---|---|---|
| 按 Lua **5.4** 编的 | 函数名就是 ABI：5.1–5.3 的产物会要 `lua_objlen`、`luaL_register`、`lua_setfenv`，这些这里没有，直接加载失败 | `nm -u x.so \| grep lua_` |
| 编译时**没有链接 liblua** | 常规做法就是留空符号交给宿主填。若 `DT_NEEDED` 里有 `liblua`，它的 `lua_*` 会解析到真解释器，然后拿本实现的 `lua_State`（一个不透明句柄）当真的用——实测 `SIGSEGV` | `otool -L x.so` / `ldd x.so` |
| 宿主二进制是**开 cgo** 编出来的 | `CGO_ENABLED=0` 一个 `lua_*` 都不导出，dyld 直接报 `symbol not found in flat namespace '_lua_pushlstring'` | `nm -gU luabin \| grep _lua` |

导出集合是 `include/lua.h` 与 `include/lauxlib.h` 里除四个 `...` 之外的全部名字：
177 个符号，LPeg 只用到 55 个。除此之外不需要别的一致——扩展当初用谁的头文件编的无所谓，
要对齐的是 ABI 而不是文本。

### 5.3 第二个扩展：Lua CJSON，它找出了 locale

[Lua CJSON](https://kyne.com.au/~mark/software/lua-cjson.php) 2.1.0.10 用同样的方式从
`testdata/cjson/` 编译（三个编译单元），并跑完它自带的 105 个用例——全部通过。LPeg
练的是模式、userdata 与 buffer API，CJSON 练的是另一半：用 `lua_next` 遍历表、数字与
字符串的双向转换、以及用 `luaL_error` 造错误消息。它还找出了一件 LPeg 没碰到的事：

```lua
os.setlocale("cs_CZ")     -- 套件故意切到小数逗号的老 locale，
print(1.5)                -- 然后检查数字长什么样
```

`os.setlocale` 过去对 `"C"` 以外的名字一律返回 `nil`。现在它真的去问 C 库的
`setlocale` 并记住小数点——这是 locale 里 Lua 唯一真正会观察的部分，体现在三处：
数字转字符串、`string.format` 的 `%f`/`%e`/`%g`（不含 `%a` 与 `%q`，这正是 `%q`
能往返的原因）、以及 `tonumber`（两种分隔符都收）。这也是官方套件 `literals.lua`
转绿的原因：它那些小数点用例此前一直是跳过的。

比对时剔除了两类行，都因为 PUC-Lua 自己就不固定：打印带字符串键的表的行（`pairs`
按哈希顺序走，而哈希种子每个进程都重新生成——参考解释器连跑两次也会给出两种顺序），
以及那行带着模块版本号的行。套件还需要一个 4 MB 的 `utf8.dat`，它的 `genutf8.pl`
用 perl 生成；测试里用 Go 写出同样的字节，这样它只需要一台有 C 编译器的机器。

### 5.4 第三个扩展：LuaSocket，它自己掌管内存与套接字

[LuaSocket](https://lunarmodules.github.io/luasocket/) 是这里编译并运行的第三个真实扩展，
也是走得最远的一个。LPeg 与 CJSON 都待在解释器内部，LuaSocket 则要掌管一个套接字：
它把 `t_socket` 放进 userdata，给这个 userdata 挂上自己注册在 registry 里的 metatable，
并要求每次方法调用都能通过 `luaL_testudata` 拿回当初放进去的那个指针。它找出两个
前两个扩展都没碰到的真 bug：

| LuaSocket 做的事 | 错在哪 |
|---|---|
| 用 `lua_setmetatable` 把一个套接字标成 `tcp{server}` | 为了回答「原来有没有 metatable」先 push 了一次，随后又从栈顶取 metatable 来设置——而此时栈顶已经是那个回答。套接字因此保留了原来的 metatable，`luaL_testudata` 不再认得 server，于是它上面的每次方法调用都报 "bad self" |
| 用 `luaL_optlstring(L, 2, NULL, &size)`，然后测指针是否为 NULL，以此来区分「空块」与「流结束」 | NULL 交不出去：参数缺失时回答 `""`，而参数**就是**空串时也回答 `""`，因为两者是靠与 `""` 比较来区分的。此外 `luaL_checklstring` 还泄漏——每次调用都新分配一个 `C.CString`，而 mime 的过滤器每个分块都要调一次 |

它的用例全部跑在回环地址上，所以机器不需要联网：TCP 连接、发送与接收（含一个要分多次
`write` 的 100 KB 块）、一个必须超时而非阻塞的 `receive`、`select`、UDP、`getaddrinfo`，
以及 mime 的两个过滤器——base64 与 quoted-printable——经 `ltn12` 走一遍。另有一个我们
自己写的回环往返脚本 `socket_e2e.lua` 把它们串起来。它打印的每一行都取自 PUC-Lua 5.4.9
的实跑结果，存在 `testdata/luasocket/expected.txt`。MIT 许可，随
`testdata/luasocket/LICENSE` 一并再分发。

### 5.5 第四个扩展：LuaFileSystem，它伸手进了解释器造的东西

[LuaFileSystem](https://lunarmodules.github.io/luafilesystem/) 是第四个，也是第一个「伸手进
解释器造的东西」而不是自己造的东西的扩展：`lfs.setmode` 接过 `io.open` 给的文件句柄，把
userdata 强转成 `luaL_Stream`（一个 `FILE*` 加一个关闭函数，就这个顺序），然后直接读。
它找出两件事，第一件其实与文件无关：

| LuaFileSystem 做的事 | 错在哪 |
|---|---|
| 对文件句柄调 `luaL_testudata`，然后读结果的 `closef` | Go 造的 userdata（文件句柄正是）回答 NULL，C 拿到「什么都没有」再去读它之后 8 字节。它不是报错，是**崩**。现在每个 userdata 都有一个代表它的块，在 C 第一次索要时生成——因为 PUC-Lua 对任何 full userdata 都回答一个地址，从不回答 NULL |
| 从那个块里读 `FILE*` | 块是零值的，于是每个句柄在 C 看来都是「已关闭」。`io` 的文件是 Go 的 `*os.File`，没有 `FILE*`，所以现在按 `luaL_Stream` 的布局填块：一个**副本描述符**上的真 `FILE*`（同一个描述符两个主人，先关的那个会把文件从另一个手下关掉），文件关闭时清空——这正是 C 在 PUC-Lua 里看到已关闭句柄的方式 |

它此前还根本编译不过：`include/lualib.h` 不存在，而扩展包含它是理所当然的事。`lua.h`
也没定义 `LUA_API`/`LUALIB_API`/`LUAMOD_API`，而 PUC-Lua 的 `luaconf.h` 定义了，扩展声明
自己的 `luaopen_` 时要用。两者现在都在。

它的用例会创建文件、链接与目录，所以跑在自己的临时目录里。它打印的每一行都取自
PUC-Lua 5.4.9 的实跑结果，存在 `testdata/luafilesystem/expected.txt`。MIT 许可，随
`testdata/luafilesystem/LICENSE` 一并再分发。

### 5.6 延续：C API 的最后一块

`lua_callk`、`lua_pcallk`、`lua_yieldk` 都收一个 `lua_KFunction`：协程恢复时，C 调它，
**而不是**返回到挂起的那一帧——因为在 PUC-Lua 里那一帧已经没了：yield 从里面
longjmp 出去，它的 C 栈被整个丢掉。

本实现丢不掉 C 帧，也不需要丢。yield 是把协程所在的 goroutine **就地停住**（见第 7 节第 11 条），
所以要延续的那一帧在恢复时还在，接着往下走就是了——这正是延续可达的前提。
`CallInfo` 带上 `k` 与它的上下文，`(*State).Yield` 在恢复时调延续并回答它压了几个值，
`lua_pcallk` 则按 `ldo.c` 的 `findpcall` + `finishpcallk` 那样回收下面抛出的错误。

`testdata/continuation/k.c` 是专为这件事写的 C 扩展，覆盖每一条路径：带延续与不带延续的
`lua_yieldk`、被调方 yield 的 `lua_callk`、没有延续的 `lua_callk`（必须**不可**yield）、
yield 的 `lua_pcallk`、抛错的 `lua_pcallk`、带 message handler 的、用 `lua_gettop` 数结果的
延续（PUC-Lua 自己的 `dofilecont` 就是这么写的，`lua_pcallk` 与 `lua_yieldk` 留下的栈还不一样）、
一个 C 函数靠同一个延续连挂三次、零值 yield、以及对停在 C yield 里的协程调 `coroutine.close`。
它打印的 31 行全部取自 PUC-Lua 5.4.9，存在 `testdata/continuation/expected.txt`，
由 `TestContinuation` 逐行比对。为本仓库所写，MIT 许可，与其他 testdata 一致。

### 5.7 第五个扩展：luaposix，它把整个 C API 走了一遍

[luaposix](https://github.com/luaposix/luaposix) 不是一个模块，而是三十个小模块，
它们合起来几乎把 C API 调了个遍——而且回答的是**宿主自己的**数字：`errno`、
`struct tm`、`struct stat`、目录列表。这里跑其中九个，即答案不依赖「哪台机器、哪一秒」
的那九个。

它找出两件事，而且都在头文件里，不在实现里：

| luaposix 需要的 | 缺什么 |
|---|---|
| `LUA_FILEHANDLE` | 文件句柄元表注册所用的名字。缺了它 `posix.stdio` 根本编译不过；而一个要判断「某值是不是文件句柄」、或想造一个给 `io` 用的扩展，都需要它 |
| `LUA_SIGNATURE`、`LUA_GNAME`、`LUA_NUMTAGS`、`LUA_RIDX_LAST`、`LUA_VERSION_RELEASE_NUM`、`LUA_AUTHORS`、`LUA_COPYRIGHT`、`luaL_intop`、`luaL_opt`、`lua_writestring` 及另外两个、`lua_getextraspace` | `lua.h` 与 `lauxlib.h` 里另外十来个宏。只有最后一个有点意思：它要摸 `lua_State*` **之前**的字节，所以那个指针背后的块现在真的在句柄前面留了 `LUA_EXTRASPACE` 字节，与 PUC-Lua 一致 |

被废弃的整数转换宏（`luaL_checkint` 等）也补上了，但像 PUC-Lua 的 `luaconf.h` 那样
藏在 `LUA_COMPAT_APIINTCASTS` 后面——默认不开，所以扩展默认看到的与它在 PUC-Lua 下
看到的完全一致。三个公开头文件的其余宏与全部函数声明都已齐备；PUC-Lua 写成宏、
本实现写成函数的那十来个（`lua_call`、`lua_pcall`、`lua_yield`、八个 `lua_is*`）
两种写法行为相同。

用例打印的 30 行全部取自 PUC-Lua 5.4.9，存在 `testdata/luaposix/expected.txt`，
由 `TestLuaPosix` 逐行比对。它会创建文件、目录与链接，所以跑在自己的临时目录里。
MIT 许可，随 `testdata/luaposix/LICENSE` 一并再分发。

### 5.8 把 C API 从头到尾走一遍

LPeg、CJSON、LuaSocket、LuaFileSystem 与 luaposix 合起来调用了 C API 的很大一部分，
但它们是「有活要干的程序」，不是普查：整组整组的函数从未被触及。于是
`testdata/capi/capi.c` 就是那份普查。它是一个普通扩展——对着 `include/` 编译、
由 `luaopen_capi` 打开、被 `require` 找到——按组把 C API 走一遍，并用数字、布尔
与固定字符串作答，这正是它可被比对的原因：

| 分组 | 调用了什么 |
|---|---|
| 栈 | `lua_rotate`、`lua_absindex`、`lua_copy`、`lua_checkstack`、`lua_remove`、`lua_insert`、`lua_replace` |
| 算术 | 全部十四个 `LUA_OP*` 的 `lua_arith`、以及 `lua_compare`、`lua_rawequal`、`lua_concat`、`lua_len`、`lua_rawlen` |
| 表 | 每一对 get/set——`getfield`/`setfield`、`geti`/`seti`、`rawget`/`rawset`、`rawgeti`/`rawseti`——外加 `lua_next`、`lua_createtable` 与全局表 |
| 转换 | 带 `isnum` 出参的 `lua_tointegerx`/`lua_tonumberx`，以及对九种类型各取一个值来问 `lua_type`/`lua_typename` 与全部 `lua_is*` |
| lauxlib | `luaL_ref`/`luaL_unref`、`luaL_newmetatable`、`luaL_getsubtable`、带 mode 的 `luaL_loadbufferx`、`luaL_fileresult`、`luaL_traceback`、`luaL_tolstring`、`luaL_gsub`、`luaL_addgsub`、`luaL_check*`/`luaL_opt*`、`luaL_argerror`、`luaL_typeerror`、`luaL_where`，以及 `luaL_requiref(L, "string", luaopen_string, 1)` |
| userdata | 带多个 user value 的 `lua_newuserdatauv`、`lua_getiuservalue`/`lua_setiuservalue`，以及越界时的 `LUA_TNONE` |
| to-be-closed | `lua_toclose` 与 `lua_closeslot` |
| 调试 API | 装一个真正的 C hook 的 `lua_sethook`、取全部字段的 `lua_getstack`/`lua_getinfo`、`lua_getlocal`/`lua_setlocal`、`lua_getupvalue`/`lua_setupvalue`、`lua_upvalueid`/`lua_upvaluejoin` |
| 协程 | `lua_newthread`、两次 `lua_resume`、`lua_status`、`lua_isyieldable`、`lua_xmove`、`lua_resetthread` |
| 缓冲区 | 整个 API：`luaL_addstring`/`addchar`/`addlstring`/`addvalue`、`luaL_prepbuffsize`、`luaL_addsize`、`luaL_bufflen`/`buffaddr`/`buffsub`、`luaL_pushresult`、`luaL_buffinitsize`、`luaL_pushresultsize` |
| 文件句柄 | `luaL_checkudata(L, 1, LUA_FILEHANDLE)`，再转成 `luaL_Stream` |
| 限额 | `lua_setcstacklimit`、`lua_atpanic` |

它打印的 35 行全部取自 PUC-Lua 5.4.9，存在 `testdata/capi/expected.txt`，
由 `TestCApiSurface` 逐行比对。驱动是以 chunk 名 `cext` 加载的（参考解释器那侧由
`testdata/capi/run.lua` 来做），所以 `luaL_where` 答案里的那个 `cext:35` 两边一样。

它找出十处问题，五个已发布的扩展一个都没碰到：

| 它做了什么 | 原来错在哪 |
|---|---|
| `luaL_requiref(L, "string", luaopen_string, 1)` | `luaopen_*` 在 `lualib.h` 里声明了，却从未进动态符号表，于是「静态链进宿主」的扩展、或想手工打开某个库的宿主，解析不到这个名字 |
| 用 `luaL_tolstring` 读一个值，再把它弹出 | `luaL_tolstring` 只返回指针，没往栈上留东西；而「在构造栈的过程中做一次 tostring」正是它的语义。栈因此少了一个，下一个绝对索引就指错了值 |
| `luaL_gsub` | 同样的问题：PUC-Lua 会把结果留在栈上，本实现把它弹掉了 |
| `luaL_buffinitsize` 与 `luaL_pushresultsize` | `luaL_pushresultsize` 把长度忽略了，而这正是它的全部意义：扩展自己往那块空间里写，然后在这里告诉缓冲区写了多少 |
| `luaL_bufflen`、`luaL_buffaddr`、`luaL_buffsub` | 它们是 `<lauxlib.h>` 里基于 `n` 和 `b` 的宏，所以无论内容是怎么攒起来的，它们都得描述它。而 `luaL_addstring` 加进去的内容原先放在一个 Go builder 里，这三个于是当它不存在。现在内容放在一整块会长大的内存里，与 PUC-Lua 一致——而且是 C 内存，因为 `luaL_prepbuffsize` 会把地址交出去，扩展在调用返回之后才往里写 |
| `luaL_ref`，再 `luaL_unref`，再读那个槽 | 空闲链表原先放在槽 0，那是 5.3 的做法；5.4 用的是 `LUA_RIDX_LAST + 1`，而这个槽就是调用者传进来的那张表的普通键 |
| `lua_toclose` 与 `lua_closeslot` | 两个都是空壳，于是从 C 里用 `<close>` 时 `__close` 根本不会跑。现在标记放在帧的 `nresults` 的符号位上（与 C 一致），由 `luaD_poscall` 在搬运返回值之前先把待关闭的关掉 |
| `lua_newthread`，再 `lua_status`，再 `lua_resume` | 从未运行过的线程在这里是 `LUA_YIELD`（`coroutine.status` 需要这个），但 C 的 `lua_status` 对它回答 `LUA_OK`，而 `lua_resume` 必须能区分「启动它」和「唤醒它」。现在线程自带「是否曾被 resume 过」 |
| 对会 yield 的线程调 `lua_resume` | C 从协程自己的栈上取参数，把 yield 出来的值留在那里，并报告有几个，好让调用者用 `lua_xmove` 搬走。这两条这里原先都不成立：参数取自恢复者，值也被弹掉了 |
| `lua_atpanic` | 返回 NULL 而不是上一个处理函数——而「把宿主的处理函数放回去」正是安装自己的处理函数的扩展唯一需要它做的事 |

### 5.9 独立编译器 luac——以及它的 listing 证明了什么

`cmd/luac` 就是 luac.c，它补上了 PUC-Lua 里最后一件缺失的东西：把脚本编译成二进制
chunk、只做语法检查而不运行（`-p`）、去掉调试信息（`-s`）、以及把字节码列出来
（`-l`，两次是完整 listing）。真正的活由 `lua.Luac` 干，它放在库里而不是
`cmd/luac`，因为 listing 必须读原型——luac.c 自己就是这么说的：它 include 了
`lobject.h`、`lopcodes.h` 和 `lstate.h`。

listing 值得一做的第二个理由：它是比任何输出比对都严格得多的**一致性检查**。一个
脚本可以打印出正确的结果，而编译出来的东西并不一样；listing 不行。每条指令、每个
操作数、每行注释、每个常量、每个局部变量与每个 upvalue 都必须是 PUC-Lua 的，顺序、
行号也要一样。在差分套件的 80 个脚本上跑下来——约 **81000 行 listing**——两侧逐行
一致。

也正因如此，它挖出了编译器里最后一个真差异：

**`addk` 用错了表。**`lcode.c` 是在**扫描器**的那张表（`fs->ls->h`）里查常量的，
整个 chunk 的所有函数共用一张；而本移植给每个函数单独建了一张。后果不是「一个共享、
一个不共享」——PUC 的表项里存的是**当初填它时那个函数**的常量下标，只有当这个下标
在当前正在编译的函数里仍然指向同一个值时才会被复用。所以 PUC-Lua 会**漏掉**一些
「每函数一张表」能找到的复用，像

```lua
local function f() return print end
print(f)
```

这样的 chunk，PUC-Lua 会给 `print` 两个常量槽，本移植只给一个。Lua 层面没有东西能
观察这件事，所以 80 个脚本的 golden 从来没露出来——但常量下标是字节码的一部分，
于是 `string.dump` 写出的字节并不总是 PUC-Lua 的。现在那张表就是扫描器的，偶尔
重复的常量也被复现出来了。

`testdata/luac/` 把它钉住：三个源文件、它们合起来 683 行的 listing
（`expected.txt`，取自 PUC-Lua 5.4.9，原型地址抹掉——跨进程不可能相同）、
以及 `luac -o` 为它们写出的两个二进制 chunk（逐字节比对，再加载回来跑）。
多文件合并用的也是 luac.c 的做法——把 `"(function()end)();\n"` 按文件数重复编译一遍，
再把每个文件的原型接到嵌套原型上——所以那条路径也覆盖到了。

共用这张表有一个后果，而它是同一次直译的另一半：一个串一旦被当作常量用过，`addk`
为它写的表项里存的就是**整数**，不再是那个串。于是 `luaX_newstring` 不能靠读**值**
把「这个字面量上次存的那份」取回来——C 读的是**键**（`llex.c`：
`keystrval(nodefromval(o))`），本移植为此加了 `luaH_getstrkey`。若读值，那么每个
「已经当过常量的字面量」之后再出现一次都会变成新对象，同一个 chunk 里两个相等的长串
字面量就不再是同一个串——这正是 PUC-Lua 自己的 `literals.lua` 与 `verybig.lua`
断言的东西。常量一开始共用扫描器的表，这两个用例立刻挂了；改回读键之后两者转绿，
并由 `testdata/diff/80_strid.lua` 钉住。

### 5.10 覆盖盘点

一致性靠输出比对来测，但还有个更粗暴的问题值得单独回答：**东西是不是都在？**
`testdata/diff/79_inventory.lua` 把参考解释器暴露的每一个名字都枚举出来——
`_G` 与九个标准库、全部三十个元方法、`collectgarbage` 的十个选项、以及版本可见的常量——
排序后打印，这样 `pairs` 的顺序（PUC-Lua 每次运行都随机化）就不会造成影响。
答案是**与 PUC-Lua 5.4.9 逐字节一致**，也就是说没有缺项，也没有多项：

| 表 | 条目数 | 5.4 新增 |
|---|---|---|
| `_G` | 36 | `warn` |
| `string` | 17 | `pack`、`unpack`、`packsize` |
| `math` | 35 | `LUA_COMPAT_MATHLIB`（`pow`、`frexp`、`ldexp`、`log10`、`atan2`） |
| `debug` | 17 | `setcstacklimit`、`getuservalue`、`setuservalue` |
| `coroutine` | 8 | `close` |
| `table` / `io` / `os` / `utf8` / `package` | 7 / 14 / 11 / 6 / 8 | — |

三十个元方法全部被识别，含 `__close`、`__gc`、`__mode`、`__name`、`__pairs`、
`__ipairs`、`__metatable`。

名字列表看不出来的行为，由 `testdata/diff/78_new54.lua` 钉住：`warn` 及其
`@on`/`@off` 控制消息、`debug.setcstacklimit` 与它实际换来的栈深、整数版
`math.random`（以及同一种子产出完全相同的序列）、新的 `%q`、`load` 的 mode 参数、
`table.pack`/`unpack` 的边界、以及负数上的整数除法与取模。

其余用例覆盖夹在中间的地带：各种进制下的强制转换与 `tonumber`、表被修改时的
`next` 与遍历、比较器不规矩时的 `table.sort`、`string.gsub` 的替换协议、位库、
递归深度与两种溢出错误、`error` 对象与 level、跨不过去的协程边界、`<close>` 与
`<const>`、整个 debug 库、`utf8` 与字节级字符串函数、词法的转义与长字符串、
`require` 与 `package.searchpath`、以及 `os`/`io` 的边界。

关于 debug 库，有两件清单列不出来的事值得在这里说，因为它们是它有自己那份 fuzz
（§5.13）的理由。它的 43 个函数里有 4 个——`debug.upvalueid`、`debug.upvaluejoin`、
`debug.setuservalue` 以及它们共用的那个辅助函数——在本仓库里从未被任何测试进入过，
现在进入了。而**在钩子里抛出的错误会挣脱每一个 `pcall`**——这也是 PUC-Lua 的行为，
不是本移植可以自行判断对错的地方。

`string.pack` 这一族也一样，而且是同样四个加七个函数：`str_pack`、`str_unpack`、
`str_packsize` 以及它们背后的七个辅助函数——`strGetOption`、`strGetDetails`、
`strPackInt`、`strUnpackInt`、`strPackBits`、`strUnpackBits`，以及把格式串在零字节
处切断的那一个——全部是 0%，是 `lstrlib.go` 里最后一批从未被测试进入的函数。fuzz
改变不了这件事：它把两个解释器当子进程跑，而子进程不回答 `go test -cover`；
`TestPackInProcess` 才是，它在本进程里经 API 驱动同样的函数。它们现在在 92% 与
100% 之间，网格找到的那两处也钉在里面。

### 5.11 性能

`go test -bench=. -benchmem -run=XXX` 跑 `bench_test.go` 里的基准。
下表是同一份工作分别交给两个解释器端到端跑出来的耗时
（Apple M1 Pro，PUC-Lua 5.4.9 以 `-O2` 编译，取三次最好成绩）：

| 负载 | PUC-Lua 5.4.9 | 本实现 | 倍率 |
|---|---|---|---|
| `fib(26)`，递归调用 | 6.5 ms | 16.1 ms | 2.5× |
| 算术循环，10⁷ 次 | 63.6 ms | 229.3 ms | 3.6× |
| 整数键表，10⁶ 次写入+读取 | 12.6 ms | 35.7 ms | 2.8× |
| 字符串键表，10⁵ 次写入+读取 | 64.7 ms | 92.7 ms | 1.4× |
| `table.sort`，10⁵ 个元素 | 27.6 ms | 39.3 ms | 1.4× |
| 闭包 upvalue 自增，2×10⁶ 次 | 31.5 ms | 86.2 ms | 2.7× |
| 协程 yield/resume，2×10⁵ 次 | 16.8 ms | 100.2 ms | 6.0× |
| `pcall` 一个 Lua 函数，5×10⁵ 次 | 15.9 ms | 32.8 ms | 2.1× |
| `string.gmatch` + `gsub`，1 MB | 34.4 ms | 54.4 ms | 1.6× |
| 编译 120 KB 源码 | 5.0 ms | 10.8 ms | 2.2× |

慢在哪：`Instruction` 是结构体而非位压缩的 uint32（每条指令要现解码），
每次 `TValue` 访问都要过一遍类型分支，协程跑在独立的 goroutine 上——
一次 yield/resume 是**两次 goroutine 切换**，在本机用一个裸 channel 往返
量出来约 280 ns。协程这一项的差距全是它，也是这张表里唯一优化不掉的数字。

其余各项一直在缩小，其中两处改动起的作用最大，而且都不是 Lua 本身的问题，
而是 Go 运行时的问题：

* **算术指令有了整数快路径。** 两个整数永远不可能走到元方法，结果也一定是
  整数，所以 `luaV_execute` 自己就把算术做了，不再走
  `opCodeToArith` → `luaO_rawarith` → `intarith`。这段是写在 switch 里而不是
  抽成辅助函数的：能抛错的辅助函数（`%` 与 `//`）超过编译器的内联预算，
  而那次调用的开销和运算本身差不多。
* **短字符串改用两代缓存做驻留，不再用弱引用。** 用弱引用持有，让 Go 的收集器
  把死掉的串清掉，是「C 的收集器扫那张表」最直白的翻译——而它吃掉字符串键表
  负载**一半**的时间：那样每个驻留串都挂着一个 runtime 的 weak handle，
  而 handle 正是收集器一轮里要逐个走的东西。缓存最多持有两代，且完全不向
  运行时注册任何东西。它带来的唯一后果是驻留不再是永久的，所以短字符串按
  「哈希再比字节」比较（`luaS_eqshrstr`），绝不比指针。

`Value` 是 32 字节而不是 40，理由和 C 的 `TValue` 是 union 一样：
浮点数与整数共用那个字段，以 IEEE 位模式存放。

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

### 6.1 加载 C 扩展模块（需要 cgo）

按常规头文件写的 `.so` 可以经 `require` 直接加载。头文件用本仓库的
`include/lua.h` 与 `include/lauxlib.h`，里面的名字就是真的 `lua_*` / `luaL_*`，
所以现成的 C 源码无需改动即可编译：

```c
#include "lua.h"
#include "lauxlib.h"

static int l_add (lua_State *L) {
  lua_pushinteger(L, luaL_checkinteger(L, 1) + luaL_checkinteger(L, 2));
  return 1;
}

static const luaL_Reg funcs[] = { {"add", l_add}, {NULL, NULL} };

int luaopen_myext (lua_State *L) {
  luaL_newlib(L, funcs);
  return 1;
}
```

```bash
# macOS：让 lua_* 这些名字留到加载时再解析
cc -shared -fPIC -I/path/to/fakescript_go/include \
   -undefined dynamic_lookup -o myext.so myext.c
# Linux：宿主符号从可执行文件的动态表里来
cc -shared -fPIC -I/path/to/fakescript_go/include -o myext.so myext.c

./lua -e 'print(require("myext").add(3, 4))'   # 7
```

`package.cpath` 与平台一致，用 `LUA_CPATH` 或 `package.cpath` 把扩展放到
`require` 会找的地方即可。构建前提与桥接尚未覆盖的部分见 §7 第 2 条。

### 5.12 第三方语料

上面所有用例都是**有人写出来**的——而且每一个写的人，当时都在想某一件具体的事。
`testdata/third` 是另一种：五个单文件 Lua 库，没有一个是为这个移植写的，共
2 759 行，原样跑起来，然后让它们打印。

| | 行数 | 它是什么 |
|---|---|---|
| `dkjson.lua` | 714 | 用 `string.find` 加一个位置写出来的 JSON 编解码器 |
| `fun.lua`（Lua Fun） | 1 087 | 60 个迭代器，每一个都是一个协程 |
| `json.lua`（rxi） | 388 | 更小的一份 JSON 库，被问同样的问题 |
| `inspect.lua` | 377 | 一个漂亮打印器，本质是把表走一遍再打出来 |
| `middleclass.lua` | 193 | 面向对象，从头到尾都是元表 |

受测的不是它们中的任何一个，而是它们跑在上面的那台机器，而且是按**真实程序**
的方式去问：一个靠位置往前 `find` 的扫描器、一个经 `gsub` 与替换表转义的串、
一条三层深且每层都有 `__index` 的元表链、一条五个协程互相挂起的迭代器链。要三
个特性凑齐才露面的 bug，既不是「一个函数一个用例」那一类找的，也不是「穷举某
个库的参数」那一类找的。`drive.lua`（517 行）问 264 个问题并把答案打出来，
`TestThirdPartySuite` 拿它跟 PUC-Lua 逐行比对，黄金文件与别处一样从 `LUA_REF`
现场抓。

有两件东西必须先拿掉，否则什么都比不了——而这两件都只有真库才逼得出来：

* **表打印时按键排序。** PUC-Lua 用时间与一个地址给字符串哈希播种，所以同一个
  表的键，`pairs` 每次给出的顺序都不同——参考解释器是这样，本移植也一样。参考
  解释器在 `{a..f}` 上跑三遍，答案是 `cdefab`、`fedcba`、`afdebc`；本移植跑三遍
  是 `badcfe`、`fabcde`、`facbed`。JSON 编码器和漂亮打印器**本身就是**一遍
  `pairs`，不排序的话，比的就不是 Lua 而是那颗种子。
* **函数用 `type` 报告，不用 `tostring`。** 函数打印出来的那个地址属于宿主，两
  个二进制之间差着几个 GB。`inspect` 打的是 `<function 1>`——每进程一个计数器
  ——所以那个没问题；但 `dkjson.use_lpeg` 打的是裸地址，只能换一种问法。

今天它给出的答案是：**264 个问题，零差异**。这句话的价值取决于这份语料能不能
说出相反的话，所以它按这里每个 fuzz 的老办法验证过——故意弄坏一处，看它是否
变红：

* 把 `fmtFloat` 的 `%.14g` 砍成 `%.13g`——40 行变红，点名
  `dkjson/encode 0.33333333333333`。
* 从 `addquoted` 里去掉 `%q` 对 `\n` 的特例——86 行变红，第一个就是那个主题里
  带换行的用例。

第一条还附带一个教训，关于怎么读一次「全绿」。最初那份语料——228 个问题，没
有一个数字的有效位超过三位——在那个变异下**没有**变红。那并不证明数字转字符
串是对的，只证明没人问过。因为这件事，后来加了 12 个长值（`1/3`、`math.pi`、
`0.1`、`2^53`、`1e-15`、`12345678901234`……），从那以后这份语料就对那一位敏感
了。「没发现差异」的力度，只等于被问过的问题。

这五个库也在 codegen fuzz 的源文件清单里，所以比对的还不只是它们打印什么，还
有它们编译成什么：122 个源文件、17 197 581 字节字节码，两边都是 PUC-Lua 的。

五个库都是 MIT，各自的许可证就放在旁边（`LICENSE_*.txt`）。它们并没有被打进
这个库本身——只存在于 `testdata` 下，测试之外的任何代码都看不见它们。

### 5.13 差分 fuzz

上面所有用例——包括第三方库被问的那 264 个——都是**有人写出来**的。这一节是另
一种：用例在测试里生成，没有一个是挑出来的，同样过一遍参考解释器和 `./cmd/lua`
再逐行比对。它需要 `LUA_REF`，
所以只在 CI 上跑；而且是穷举而非随机，每次跑的都是同一批，差异必然可复现。

| | 生成方式 | 比对答案数 |
|---|---|---|
| `fuzz_test.go` | 72 463 个表达式（43 个操作数 × 全部运算符，且每一个再生成一份"VM 必须运行时算、编译时折不了"的形式，外加带进制/格式的调用），再加 201 条手写 snippet 覆盖组合达不到的东西：语法错误、元方法、协程、钩子、模式、非字符串错误、栈溢出 | 72 664 |
| `patternfuzz_test.go` | 6 991 个模式（每个模式原子 × 每个原子，外加必须三片才有意义的三元组）× 25 个主题串 | 1 048 650 |
| `formatfuzz_test.go` | 11 340 个格式串（每个标志 × 每个宽度、精度、转换）× 22 个值，外加每个格式串一次"不给值" | 260 820 |
| `numfuzz_test.go` | 1 834 个待读的字符串（符号 × 整数部分 × 小数部分 × 指数的全部组合，再加十六进制同样一遍，再各自套九种"空格"），以及 49 个值被 `tostring`、`%q`、`math.tointeger`、`%g`、`%e` 写回 | 11 053 |
| `forfuzz_test.go` | 16 277 个数值 `for` 循环——25 个值各在 start / limit / step 三个位置上：两种符号的零、整数与浮点、整数区间的两端、刚好越界的浮点、浮点不再能逐一点下去的那个点，以及三个根本不可比较的值——每个都问两遍（写出 step / 省略 step 让它默认为 1），外加 9 个「不是数字」的值各放在三个位置上 | 16 277 |
| `codegenfuzz_test.go` | 101 356 个片段——每一种字面量、每个运算符 × 每种操作数形状、每种语句形式（裸写一次、放进函数里再一次）、闭包能捕获变量的每种形态、40 个不该编译通过的东西，再往后是 20 个原子 × 18 个运算符的全部深度二表达式加上七分之一的深度三，各放进 22 种语句位置——外加 `testdata` 下全部 122 个源文件（官方的、我们自己的，以及 §5.12 那五个第三方库）。比对的**不是它们打印什么，而是它们编译成什么**：17 109 752 字节字节码 | 17 109 752 |
| `iofuzz_test.go` | 3 166 件 `io` 库能被问到的事——`r w a + b t x` 上长度 0 到 3 的**每一种**模式，各问一个已存在的文件、一个不存在的文件和一个目录；`read` 能接的每一种格式（大小写各 26 个字母、C 认的四个及其 `* ` 写法、以及根本不是格式的计数与值）× 10 个样本文件，还有三个一组的组合；`write` 对每种值与两种值的组合真正写进文件的字节；`seek` 的 8 种 whence × 17 种偏移（读之前与读过一行之后各一遍）；`setvbuf`；`lines` 与 `f:lines`；13 种句柄——三个标准流、每种方式打开的文件、已关闭的、`tmpfile`、`popen` 的两端——各用 17 种方式；`io.input` 与 `io.output`；以及报错、`<close>`，和「丢掉一个句柄而不关闭它」。两边的样本文件各自一份、内容完全相同 | 3 166 |
| `debugfuzz_test.go` | 1 333 个关于 debug 库的问题——长度 0 到 2 的**每一种** `getinfo` 选项串（八个 C 认的字母与一个它不认的）× 8 种函数；每种栈深度下的每个层次，那是唯一能拿到 `name`、`namewhat`、会动的 `currentline` 和 `istailcall` 的路径；5 种状态下的协程；同一个函数用 10 种方式被调用，各问一遍自己与调用者；每种栈帧形态下的每个局部变量下标（主线程上与协程上），以及各自的 `setlocal`；每个 upvalue 的取、设、取标识与接合——包括两个闭包接合的 25 种方式、两个 upvalue 是同一个的 8 种问法；56 种钩子掩码、6 种计数，以及对主线程、对协程、对「在设钩子之前 / 之后造出来的协程」各问一遍 `gethook`；每种 `traceback` 消息 × 每个层次 × 每种嵌套深度；每种元表、注册表、每种 user value——外加每个 debug 函数被问错的样子。另有 12 个钩子报错，各自开一个进程问，因为钩子里的错误会挣脱每一个 `pcall` 并结束整个进程 | 1 333 + 12 |
| `packfuzz_test.go` | 3 733 个关于 `string.pack`、`string.unpack` 与 `string.packsize` 的问题——字母表里**每一个字节**单独当格式串，是全部 256 个而不是 C 认的那 26 个，因为一个不是选项的字节就是一个被拒绝的选项，而拒绝消息里就带着那个字节；`i`、`I`、`s`、`c` 的每一个尺寸 0 到 17——要越过 16 两侧都扫，因为只有被拒绝的那些尺寸能说出边界在哪——每个尺寸 12 个值，外加正落在它边界上的 3 个；`!` 能要的每一种对齐，配 20 种被对齐的对象，再配 6 种对象两两配对的全部 36 种；每一种字节序配每一种选项；16 种选项两两配对的 256 种，以及在 12 种两两配对中间插一个 `X` 的 144 种；12 种格式 × 15 个位置上的 `unpack`；每种选项被给 8 个错的东西，以及什么都不给；138 次往返；53 个写出来的格式串；9 个格式串里每个位置上的零字节；值里的零字节；以及 6 个原始字节缓冲区里读出来的每种尺寸 | 5 337 |
| `corofuzz_test.go` | 396 个协程：协程能做的六件事（挂起、出错、结束、先挂起再二者之一、挂起两次）× 能做的 22 个位置——裸调用、一层与两层 `pcall`、`xpcall`、Lua 自己调用的每一个元方法、`for` 迭代器、`table.sort` 与 `string.gsub` 的回调、`load` 要的 reader、自己的子协程，以及"尚有待关闭变量"——每个再用三种方式驱动：连续 resume 四次、经 `coroutine.wrap`、用 `coroutine.close` 关闭 | 396 |

模式这一半存在的理由：`lstrlib.c` 的 `match` 是两百行"没人会把组合写下来"的
逻辑——frontier 后面跟一个集合、位置捕获上加量词、`%b` 的定界符本身又是魔法字符。
`32_pattern.lua` 与 `72_gsubbit.lua` 是 366 行**有人想得到**的用例，它们通过；
这一半是剩下的。

格式这一半存在的理由：`string.format` 是这里**唯一不是翻译 C** 的地方。别处要么
把格式交给 Go 自己的 `fmt`，要么按 Lua 的方式读数字；`str_format` 必须复现
**C 的** `printf`，包括那些由库而非标准决定的部分——所以它是手写的：
`strParseFormat`、`strPadField`、`strFormatUnsigned`、`strHexFloat`。它找出两处
真差异，均已修好并钉在 `lstrlib_test.go` 里：

* **先检查值，再检查格式串。** C 里 `d`、`i`、`u`、`o`、`x`、`X` 都走 `intcase`，
  也就是 `luaL_checkinteger(L, arg); checkformat(...)`，`e`、`E`、`f`、`g`、`G`
  读数字的顺序同样如此——所以 `%#d` 配 `1.5` 报的是 "number has no integer
  representation"，只有配真正的整数才报 "invalid conversion specification"。
  这里原本先判格式串，于是每一条都报错了错的那一条。`c` 的顺序反过来，且保持
  相反：`%#c` 配字符串报的是格式串的问题。
* **`%a` 的进位不只看被丢掉的那一位。** 保留的最后一位的一半是 8，所以被丢掉的
  首位大于 8 就进位、小于就不进位——但恰好等于 8 是**平局**，而只有在它后面
  全是 0 时才真是平局。只看那一位，会让 `%.8a` 格式化 `1e-300` 时给出
  `0x1.56e1fc2fp-997`，而 C 给的是 `0x1.56e1fc30p-997`：各位是
  `56e1fc2f|8f359`。

格式这一半有三类问题问不了，它会明说，而不是悄悄放过：

* `%s` 作用在一个 `print` 出来是地址的值上：填充是按地址的**真实长度**决定的，而让
  地址可比的归一化发生在填充之后，所以值列表里没有 table——不是因为它有差异，而是
  因为任何答案都不可能对。
* **落在奇数位上的 `%a` 平局**——这是各 C 库唯一取整不同的地方：glibc 把保留的最后
  一位交给 `round_away`，偶数就留着、奇数就进一位；Apple 的库则无论奇偶一律**向下**。
  `%.0A` 格式化 1.5 在前者下是 `0X2P+0`、在后者下是 `0X1P+0`，任何移植都不可能两个都
  对。本实现回答 `0X2P+0`——glibc 的答案，也是各平台 C 库自己的 `%f` 的答案
  （`%.0f` 格式化 1.5 是 `2`），也是让同一个调用里 `%a` 与 `%f` 自相一致的那一个。
  于是 fuzz 先拿这一个问题问参考解释器：若它答 `0X1P+0`，就把网格里依赖它的 720 个
  用例**两边都不问**。落在**偶数**位上的平局则两种规则殊途同归，照常比对；
  `TestFormatHexFloatTieRounding` 把两半都对着参考解释器钉住。
* **次正规数**，其首位数字由 C 留给库自己定：`5e-324` 在这里是 `0x1p-1074`，
  在 glibc 下是 `0x0.0000000000001p-1022`（§7.13）。这两类问题由同一套机制处理：
  跑之前各问参考解释器**一个问题**，两个可能答案都写在测试里，参考解释器若答了别的，
  测试直接失败而不是照旧比对。

数字这一半是同一套比对，只是对着「数字 ↔ 字符串」的另一端，而正是在这里，Go 的细节
在**三个地方**漏了出来。`tonumber("1_000")` 这里答 `1000`、PUC-Lua 答 `nil`，因为
Go 的 `strconv.ParseFloat` 接受 Go 字面量里的下划线，而 C 的 `strtod` 从来没听说过
它；`tonumber("\u{3000}1")` 这里答 `1`、那里答 `nil`，因为 `strings.TrimSpace` 会
剪掉 Unicode 空格，而 C 的 `isspace` 不会——十六进制那条路（Go 不给指数就不收的那些
数字，由第二个解析器负责）还自带了第二份同样的剪裁。三处都已修好并钉在
`lobject_test.go`。六十个字符串的探针先找出前两条，这正是网格被建起来的原因。数字
这一半**故意不问**算出来的值：`math.pi` 是常量、没问题，`math.exp(1)` 就不行——Go 的
`math` 与平台 libm 在最后一个 ulp 上不一致（§7.8），那样 fuzz 报的就是数学库而不是
转换本身。

表达式这一半把每个运算问两遍，而第二遍才是要紧的那一遍。`A op B` 的两个操作数
**都是数字字面量**时，它根本不是运行时运算：`lcode.c` 的 `constfolding` 在编译时
就把它折掉了，跑起来的是一个常量，`OP_ADD`、`OP_IDIV` 以及它们的邻居一次也没被
走到。第二遍——

```lua
(function() local a, b = A, B return a op b end)()
```

——是同一个运算，只是两个值躺在寄存器里：字符串正是在这里被转成数字，整数除零
正是在这里抛错，两边只要有一边错了也正是在这里给出不同的答案。**只问一种形式的
网格，是编译器的网格。** 这一半先单独探过 30 258 个运行时表达式，没有差异；放进
网格是为了让它一直没差异。

`for` 这一半是同一套比对，只不过对着「Lua 在跑之前就先决定好的那个循环」。
`OP_FORPREP` 与 `OP_FORLOOP` 决定一个循环**跑不跑**、控制变量是什么、什么时候停
——整数里计数器会绕过 `maxinteger`，浮点里一个 step 会把索引舍入到越过 limit——
这些没有写在别处，只写在 `lvm.c` 里。16 250 个循环的探针找出 60 分之一的差异：

```lua
for i = 0, 1, 0/0   -- PUC-Lua 一次也不跑；本实现跑了一次
```

C 用 `luai_numlt(0, step) ? luai_numlt(limit, init) : luai_numlt(init, limit)`
决定是否跳过循环，这是个**三元表达式**；本实现写成了「读起来一样」的那对合取式
`(step > 0 && limit < init) || (step <= 0 && init < limit)`。二者只在 step 是个
能和零比较的数时才一样。NaN 既不大于也不小于零，于是 C 走 **else 分支**去问
`init < limit`：`0 < 1`，循环被跳过。而 `step <= 0` 同样为假，本实现两条都不走，
就跑了一次。`floatforloop` 里同一个三元表达式被同样地改写过。

这与其说是关于 NaN 的规矩，不如说是关于**翻译**的规矩：条件是个比较的三元表达式，
并不等于它看起来像的那对合取式——一个比较为假，不代表它的否定为真。两处都已修好
并钉在 `TestForNaNStepPinsTheTernary` 里——十二个答案，并非同一个数，因为 NaN 作
**limit**、而 start 与 step 都是整数时，根本不走浮点那条路：`forlimit` 无法把它
转成整数，于是按 step 的符号把它截成 `mininteger`，或者干脆跳过循环。

io 这一半写得最晚，找到的也最多。写它是因为 `liolib.go` 是唯一覆盖率一直低于
55% 的文件——79 个函数，28 个从未被进入过——而它红了六次，六次都是同一个形状：
**C 问的是 FILE，移植问的是 FILE 后面的那个描述符**。

* **`io.open` 拿到 C 语法不认的模式**：`l_checkmode` 是 `[rwa]%+?b*`——`L_MODEEXT`
  只有 `"b"`，没有别的——所以 `"rt"`、`"wt"`、`"at"`、`"r+t"` 在 PUC-Lua 里都是
  "invalid mode"，无论每个 C 库多么愿意收下它们。移植版收下了 `t`。
* **往没有为写打开的句柄里写**：C 的 `cantwrite` 看的是 `fopen` 在 FILE 里设下的
  标志，一个字节还没动就已经失败并返回 EBADF，于是 `io.open(f, "rb"):write("x")`
  答 `nil, "Bad file descriptor", 9`。Go 的文件只在系统调用处失败，而写缓冲会在
  到达那里之前先把这一小段吞掉。`cantread` 是同一件事的另一面：
  `io.popen(cmd, "w"):read("a")` 不是空串，`io.stdin:write("z")` 也不是一次成功的写。
* **写失败时报告了错的 errno**：`g_write` 把真正的错误丢掉，交给 `luaL_fileresult`
  一个编造的，于是每次失败都答 "(no extra info)"、errno 为 0——上面那一条因此
  一直看不见。
* **名字里有零字节**：`fopen` 拿到的是 C 字符串，所以 `io.open("a\0b", "w")` 造出
  的是名为 `a` 的文件；移植版把整个名字交给了 Go，后者答 "Invalid argument"。
* **追加模式**：`fopen` 的 `"a"` 把文件位置留在末尾，而 `O_APPEND` 不——它只决定
  **写**落在哪里，位置仍停在 0——所以 `io.open(f, "ab"):seek()` 是 `f` 的大小。`"a+"`
  上两个 C 库不一致，而 C 只要求写落在末尾：Apple 那份把位置也放在那里，于是
  `io.open(f, "a+"):read("a")` 答什么也没有；glibc 留在 0，答整个 `f`。本实现跟随
  宿主的那份库——也就是该机器上 PUC-Lua 的做法——而网格在跑的那台机器上问参考解释器，
  两边因此一致。
* **失败的 `fseek`，以及会绕回的 `fseek`**：一次失败的 seek 什么也不改变，缓冲里
  预读的内容也属于「什么也不改变」——`f:read("l"); f:seek("set", -1)` 失败，而文件
  剩下的部分还在那里可读。`"cur"` 上 `math.maxinteger` 的偏移是原样交给 `lseek`
  的，所以答 EOVERFLOW 的是内核；移植版先加上了缓冲的预读量，绕回成负数，答了
  EINVAL。管道上则是 ESPIPE（"Illegal seek"，29），而不是一个 errno 为 0 的编造错误。

唯一一件**不是**「C 问 FILE」的事，也是找起来最费劲、最值得记住的一件：
**从来没人关闭的句柄**。`f:close` 是唯一会关闭文件的东西，于是被随手丢掉的句柄
把还留在写缓冲里的字节一起留下了——那些字节就丢了。文件句柄的 `__gc` 从来没有
跑过，原因是 Go 的：`runtime.SetFinalizer` **对处在引用循环里的对象永远不触发**，
而句柄背后那个流又指回它自己的 userdata，于是两者成了一个环。两个互相指着的对象，
其中一个带着 finalizer，是**根本不会被回收**的——一轮之后不会，十轮之后也不会。
指回去的那个指针现在改成弱引用，环于是变成了树；网格也问这件事：在 `do ... end`
里丢掉的句柄会把字节写出来。

debug 这一半找到的两件事，一件在库里，一件在库外——在解释器自己的 `main` 里，
那是 fuzz 本不该够得着的地方。`ldblib.go` 有 43 个函数，其中 4 个从未被任何测试进入
过——`debug.upvalueid`、`debug.upvaluejoin`、`debug.setuservalue` 以及它们共用的那个
辅助函数——而一份 258 例的全库探针只红了**一处**。

* **钩子开着时造出来的协程会继承这个钩子**：`lua_newthread` 会把 `hook`、
  `hookmask` 与 `basehookcount` 一起拷进新线程，所以 `debug.sethook(f, "crl", 2)`
  之后再 `debug.gethook(coroutine.create(...))`，答案是 `nil, "crl", 2`——一个
  从没有人在这个协程上设过的钩子的掩码与计数。移植版把掩码与计数放在自己那张
  以线程为键的表里，而一个从未被设过的协程不在表里。那张表还带着一个 C 的设计
  里没有的泄漏：它的键是强引用，于是每个曾经有过钩子的协程在整个进程生命期内
  都可达，而 C 的钩子表是弱表（`__mode = "k"`）。这三样东西现在都在线程上。
* **一个逃出 `pmain` 的错误打印成 `lua: `，后面什么也没有**：`lua.c` 的 `main`
  先用 `lua_toboolean` 读结果——那是**读**，不是弹栈——然后才调 `report`，而真正
  移除错误对象的是 `report`。移植版先弹栈再报告，于是 `report` 拿到的是下面那一个。
  只有钩子能让错误跑到那么远：`return` 钩子会在 `pmain` 自己返回时触发，那在解释器
  所做的每一个受保护调用之外，于是那次抛出连这一层也挣脱了，`main` 会**再**报告一次，
  不带回溯。这也顺带暴露了钩子的一个很容易弄错的性质：**在钩子里抛出的错误会挣脱
  每一个 `pcall`、每一个 `xpcall`，以及 `coroutine.resume`**。PUC-Lua 同样如此，
  所以 `TestDebugHookErrors` 把那 12 个用例各自开一个进程来问，并且连退出码一起比对——
  它们不能成为网格里的行，因为其中任何一个都会是这次运行打印的最后一样东西。

pack 这一半最新，问的是 `string.pack`、`string.unpack` 与 `string.packsize`——标准库里
最后一套没有自己网格的 C 小语言。格式串是尺寸、对齐、字节序与选项构成的一段程序，
`lstrlib.c` 就是它的解释器。一份 907 例的手选探针在三者之上**一行没红**，而按本项目的
规矩，那还不是答案：于是把 `lstrlib.go` 故意弄坏两遍，用同一份语料各问一次——两遍都
看见了——去掉 `packint` 里的符号扩展，红 36 行；把 `getdetails` 里的 `*ntoalign` 强行
置 0，红 4 行。907 例里只红 4 行，正是手选探针会漏掉的东西，网格就是为它们建的。
它找到两处。

* **格式串在第一个零字节处结束，而不是在字符串末尾**：`str_pack`、`str_packsize`
  与 `str_unpack` 都用 `while (*fmt != '\0')` 遍历格式串，所以
  `string.packsize("i4\0i4")` 是 4，`string.packsize("\0i4")` 是 0，而单独一个零
  字节是**空格式**而不是被拒绝的格式。Go 的字符串是「长度 + 若干字节」，于是移植版
  会越过那个零，走到它后面的字节上，然后发现一个「零字节」这样的格式选项——并不存在。
  `string.format` 不这样：C 遍历**它的**格式串用的是 `strfrmt < strfrmt_end`，
  末尾是长度——所以它保留零字节，两者的差别现在只有一处一行代码。
* **从数据里读出来的长度是按 `size_t` 比较的**：`str_unpack` 的 `Kstring` 分支是
  `size_t len = (size_t)unpackint(...)`，然后是 `len <= ld - pos - size`，三者都
  无符号。从八个字节的 `0xff` 里读出来，那个长度在 C 里是 2^64−1、在这里是 −1，而
  −1 小于字符串剩下的任何长度：比较方向整个反了，它后面那个切片就越界了。
  `string.unpack("s", ("\xff"):rep(16))` 在 PUC-Lua 里是「data string too short」
  被拒绝，在移植版里则是一次 Go 运行时 panic。

第二处也正是这一半的教训，而且是第三方语料库已经教过一次的那一课：网格此前只解包
**它自己刚刚打包的东西**，而 `pack` 写出来的永远格式良好——它在第八个字节以上放的
就是符号扩展，所以「那里有一个符号扩展」这个检查永远不会失败。把那个检查删掉，网格
**一行没红**——不是因为它看不见，而是因为没有任何问题问到它。新增的一组「把 9 到 16
的每种尺寸从 6 个原始字节缓冲区里读出来」把它推到了 56 行。

codegen 这一半是唯一**不问程序打印什么**的。它把每个片段和 `testdata` 下每个源文件
在两边各编译一遍，比对**字节**。字节码是比输出严格得多的答案：寄存器分配的差异、
跳转有没有被串起来、两个常量有没有被折叠、常量表的顺序、行号信息、某个 upvalue
的索引——每一处都会改变字节，却让程序打印的东西完全不变，而每一处又都是"翻译错了
但看着没错"的藏身之地。片段根本编译不过时，比对的换成它的报错，于是同一套网格顺带
覆盖了 lexer 与 parser 的错误消息。就覆盖面而言它很便宜：每边一个进程、6 秒、
一千七百万字节；出错时会点名是哪个片段的字节从哪里开始分叉。

它唯一不能问的是「两个数字字面量做 `^`」。折叠会把它变成常量，而 `pow` 是折叠运算里
唯一**不由 IEEE 算术决定答案**的一个——它调用 `pow`，于是 §7.8 里 Go 与 libc 的分歧就
落进了字节码：`2 ^ 2.5` 在 C 下折成 `0x1.6a09e667f3bcdp+2`，在这里是
`0x1.6a09e667f3bccp+2`。它是这个语料库所由生长的那次 103 786 片段探针里**唯一的**差异，
也是两个相同的程序能编译出不同字节的**唯一**途径。其余能折叠的运算都是精确的：算术
运算是 IEEE 的，`//` 与 `%` 是精确可定义的，整数与字符串的转换由表示法本身决定。

值得说清楚这类 fuzz 什么时候一文不值，因为真的发生过一次：生成的 chunk 如果
**加载不了**，两边打印的是同一个语法错误，而"输出一致"看起来就是成功——第一轮那
37 615 个表达式一个也没跑，测试却是绿的。现在两半都在比对之前断言 chunk 跑到了最后
（`count` 有回答），并且都用"故意改坏一处、看它是否变红"验证过。值改成逐字节
转义而不是用 `%q` 也是同一个道理：`%q` 会把换行写成反斜杠加**真换行**，一个打印
成两行的值会悄悄破坏"一行一个用例"这个比对前提——两边行照样对齐（位移相同），
但报错时点出来的用例是错的那个。转义写成 `\%03d`、补齐三位，是因为 Lua 的 `\ddd`
**有多少位吃多少位**：不补零的 `\9` 后面跟字节 `1` 会被读成 `\91`（一个字符），
于是两个不同的值会打印成同一个样子。

---

## 7. 已知差异与限制（相对 PUC-Lua 5.4.9）

1. **`file:read(n)` 的计数超过 2 GiB 时按文件末尾处理**：C 把计数直接交给分配器，荒谬的计数是被分配器拒绝的（报 "not enough memory"；负数计数必然如此，本实现已还原这一点）。本实现不会做那个量级的分配，所以过大的计数读到空。二进制 chunk 不受影响：`string.dump` 产出的字节与 PUC-Lua 5.4.9 **完全一致**（strip 与否都一样），两边互相都能加载对方的产物——唯一的例外见 §7.8 末尾：`^` 折叠出的浮点常量会差最后一个 ulp，因为 `pow` 不是 IEEE 决定的。
2. **加载 C 扩展模块需要宿主带 cgo 构建**：`cdlopen.go` 就是 `loadlib.c` 里 `lsys_load`/`lsys_sym` 那一半，会真的 dlopen 目标库，所以 `package.cpath`、四个 searcher 与 `require` 能直接吃按 `lua.h` 写的普通 `.so`，`package.loadlib` 返回的是 `dlerror()` 自己的消息加 `"open"` 或 `"init"`——与 PUC-Lua 逐字节相同。它必须要 cgo，原因只有一个：Go 二进制得把 `lua_*`/`luaL_*` 这些名字导出到**动态符号表**里，模块才能解析到，而只有走 cgo 的链接路径才做得到。用 `CGO_ENABLED=1` 构建（有 C 工具链时本就是默认），扩展按平台允许「名字留到加载时再解析」的方式编译——macOS 上加 `-undefined dynamic_lookup`；Linux 上宿主符号从可执行文件的动态表里解析，`-shared -fPIC` 就够了。`go test` 里有覆盖：`cdlopen_test.go` 写一个用 `luaL_newlib` 的 C 扩展、编译、`require`、再调进去；
`cdlopen_api_test.go` 编译一个更大的扩展，把它打印的 **25 行输出与同一个扩展在 PUC-Lua 5.4.9 下的输出逐行比对**——
带注册元表的 userdata、`luaL_checkudata`、`luaL_checkoption`、buffer API、user value、
`lua_rawgetp`/`lua_rawsetp`、`lua_dump`、`lua_gc`、经 C 的 `lua_Debug` 读到的
`lua_getstack`/`lua_getinfo`、`lua_stringtonumber`，以及 `lua_pushfstring` 的每一种转换
（含 Lua 不认识的选项所报的 "invalid option"）。

四个带 `...` 的函数——`lua_pushfstring`、`lua_pushvfstring`、`luaL_error`、`lua_gc`——
写在 C 里（`cdlbridge.c`）：cgo 边界的**两个方向**都表达不了变参调用。它们复刻的是
`luaO_pushvfstring` 而不是 `printf`：`%d` 是 Lua 整数，`%f` 是 `%.14g` 且结果看着像整数时补 `.0`，
`%p` 是裸的 `%p`，`%U` 是 UTF-8 序列，其余一律报 PUC-Lua 的那个错。
用 `printf` 会放过 `%.2f`（Lua 是拒绝的），也会把 `%d` 按 C 的 `int` 打印。

`CGO_ENABLED=0` 时后端缺失，`package.loadlib` 返回 `nil, "dynamic libraries not enabled", "absent"`——
正是 PUC-Lua 在没有 `LUA_USE_DLOPEN` 时的逐字回答——而 `package.cpath` 里找到的 C 模块
会给出 PUC-Lua 在那里的报错。

延续——`lua_callk`、`lua_pcallk`、带非 NULL `k` 的 `lua_yieldk`——已经实现，见 §5.6。
唯一与 PUC-Lua 不同的是 `lua_callk` 的延续被告知的 status：C 那里恒为 `LUA_YIELD`，
而本实现里是 C 函数自己调延续，它看着调用「正常返回」了，于是传 `LUA_OK`。
没有可观测的东西依赖它，因为对 `lua_callk` 而言那个 status 本就是个常量。

`lua_error` 值得单独说一句：「用 panic 而不是 longjmp」看着像缺口，其实不是。
Go 的 panic **能穿过 cgo 帧**：runtime 展开到「保护该次调用的那层」的 recover，
扩展自己的帧被丢下，与 longjmp 的效果一致。三层深——Lua → C → Lua → C → 抛错——
输出与 PUC-Lua 逐行相同，`TestCModuleErrors` 钉住了这一点。反过来，
**跨边界 longjmp 才是不安全的**（会跳过 runtime 为这次 cgo 调用维护的状态），
所以这里刻意没有 setjmp。真正丢掉的东西 longjmp 也一样丢：持锁或需要析构的 C 帧不会被清理。
另一半证据是 `TestCModuleErrorStress`：跨 C 帧抛错 30000 次，再从 50 层嵌套的底部抛一次，
都没有把 C 栈耗尽——那是「丢下这些帧」唯一可能真的出错的方式。

userdata 背后的 C 内存会在 userdata 被回收时释放：`C.malloc` 不受 Go 收集器管辖，
没有 finalizer 的话，一个在循环里造 userdata 的扩展会无上限地涨。
用 Lua 写的、或已改成 Go 的模块完全不需要这些：`L.RequireF(modname, openf, true)` 把模块放进
`package.loaded`（对应 C 的 `luaL_requiref`），或把 `package.preload[name]` 设成 Go 函数，
让 `require` 照常找到它。
3. **`collectgarbage` 委托给 Go 的收集器，而那是整个进程的收集器** —— 每个选项的语义都是 C 自己的：`collect` 与 `step` 跑一轮，`__gc` 会执行、弱表条目会消失；`stop`/`restart` 置一个「分配驱动的步进」会遵守的标志，`isrunning` 如实回答它；`setpause`、`setstepmul` 与两个模式记住收到的值、回答被替换掉的值。

   但 PUC-Lua 的收集器只遍历**一个** `lua_State` 的对象，脚本调 `collectgarbage()` 的代价由拥有它的解释器承担，与别人无关；这里那一句是 `runtime.GC()`——它会 stop the world 并标记整个**进程**分配过的东西，宿主应用的数据也在内；`collectgarbage("count")` 同理报的是整个堆的 Kbytes 而非本 state 的，因为 Go 的堆不按 state 记账。把本库嵌进自己的程序时，这个代价是宿主的：一个频繁调用 `collectgarbage()` 的脚本（放在循环里，或每次请求结束时）会暂停整个进程，宿主堆越大停得越久——在把 state 交给一个不是自己写的脚本之前，这件事值得知道。分配驱动的那次步进（见 `gcStep`）跑的是同一轮、因而同样的代价，但它每 65536 次表分配才触发一次，而且只有在真的有对象被 `__gc` 监视时才触发。

   官方套件里 `coroutine`、`gengc`、`locals` 三个断言的正是「`collectgarbage` 之后立刻收过一次」，三者现在都逐字节一致。带 `__gc` 的对象在两轮之间被强引用兜住，避免 Go 在自己挑的时刻终结它们；`Close()` 时会把**所有**还带 `__gc` 的对象跑一遍，可达的也算——这正是 C 的做法。

有一件 Go 的 finalizer **根本不会做**的事：处在引用循环里的对象，它永远不触发——两个互相指着的对象，其中一个带着 finalizer，是根本不会被回收的，一轮之后不会，十轮之后也不会。一个看起来从不运行的 `__gc`，在查别的原因之前，值得先查一查有没有指回去的指针（§5.13：文件句柄背后的那个流就有，于是每个被丢掉的句柄都留着它的缓冲）。

   一轮收集在这里答不上来的，只有那些在度量一个并不存在的分配器的东西：`gc.lua`——官方套件里唯一一个测**收集器本身**的文件——问的是一次收集释放了多少内存、以及一长串对象的终结器按什么顺序跑，那是 PUC-Lua 三色标记的问题，不是 Go 的。

   在 `__gc` 内部，收集器按 C 的规则算作「已停止」：那里的 `collectgarbage` 对每个选项都回答 `nil`（`lua_gc` 返回 -1，`lbaselib.c` 的 `checkvalres` 把它变成 fail），终结器里的分配也不驱动任何周期，因此终结器不会嵌套。在终结器里关闭状态（`os.exit(0, true)`）会由那次关闭把剩下的终结器跑完——这正是 `main.lua` 的终结器顺序用例，现在与 PUC-Lua 逐行一致。

4. **弱表条目在「一轮」里被清掉，而不是随着分配被逐步清掉**：Go 的 `weak.Pointer` 在释放对象的**下一轮** GC 才失效，所以本实现的一次 Lua 周期内部跑**两遍** `runtime.GC()`——第一遍让只剩弱引用的对象真的死掉，第二遍让它的弱指针失效。结果在能测到的地方是一致的：不调 `collectgarbage` 而分配两百万个表之后条目就没了，与 PUC-Lua 相同。真正的区别在**粒度**：PUC-Lua 的收集器与分配交织，条目是在某一次分配中途消失的；本实现只在周期边界（`Close()`，或分配越过阈值时）清除，所以「第几次分配之后条目消失」这个问题没有答案。字符串、数字、布尔不参与弱化，与 PUC-Lua 一致。
5. **错误信息的文案与位置**：与 PUC-Lua 一致，由 `24_errmsg`、`36_errors`、`66_errmsg`、`67_meta` 四个 golden 共数百行钉住——算术/位运算/连接/取长/比较/索引/调用各自的措辞、`error` 的 level、`assert`/`pcall`/`xpcall`/`select`、以及 `ldebug` 推断出的变量名（local/upvalue/global/field/常量/调用结果）。唯一无法相同的是未捕获错误行首的 `prog:`——那是 argv[0]，两个程序路径不同，测试里已归一化。
6. **`os.setlocale` 去问 C 库，所以需要 cgo**：小数点（decimal point）是 locale 里 Lua 唯一真正观察的部分，体现在三处——数字转字符串、`string.format` 的 `%f`/`%e`/`%g`（不含 `%a` 与 `%q`，这正是 `%q` 能往返的原因）、以及 `tonumber`（两种分隔符都收，而源码里的数字字面量始终用 `.`）。装了哪些 locale 由宿主机决定，与 PUC-Lua 完全一致，C 库不认的名字返回 `nil`。以 `CGO_ENABLED=0` 构建时没有 C 库可问，于是除 `"C"` 以外的名字一律返回 `nil`，小数点固定为 `.`。
7. **`os.date` 拒绝的时间范围略有出入，`os.time` 接受宿主 `mktime` 拒绝的日期，`os.date("!%z")` 给出 UTC 偏移而非本地偏移**：`struct tm` 的年份字段是 `int`，宿主 `gmtime`/`localtime` 一旦放不下就返回 NULL，PUC-Lua 的 “date result cannot be represented in this installation” 正来源于此。本移植按 `tm_year` 判同一条件，上界**完全吻合**——67768036191676799 是最后一个可表示的秒（2147485547 年，`tm_year` 恰为 `INT_MAX`），再加一秒即被拒；宿主的下界离 `INT_MIN` 还差一点，所以最负端约 2×10¹⁴ 秒的窗口内本移植给出答案而参考实现拒绝，再往外两边都不给。至于 `os.time`：PUC-Lua 用 C 库的 `mktime` 构造时间，返回 -1 时报 “time result cannot be represented in this installation”。哪些日期会被拒绝由 C 库决定：macOS 上所有归一化后落在 1900 年之前的都被拒，glibc 则接受。Go 的 `time.Date` 没有这个上限，所以本移植返回一个数值。PUC-Lua 自己判定的部分全部复现——字段进入 `struct tm` 前必须满足的边界（"field 'year' is out-of-bound"）、缺失字段与非整数字段，以及结果恰好为 -1 的情形。`%z` 则相反：加了 `!` 之后时间是 UTC，但宿主的 `gmtime` 把偏移量留在本地值上，于是 PUC-Lua 打印本地偏移，而本移植打印 `+0000`。
8. **数学库是 Go 的，不是平台 libm**：`math.sin`、`cos`、`tan`、`exp`、`log`、`log10`、`sqrt`、`asin`、`acos`、`atan`、`atan2`、`pow`、`sinh`、`cosh`、`tanh`、`frexp`、`ldexp` 用的是 Go 的实现，与 libc 链接的那份不同。15 位有效数字一致，最后一位或两位在一部分入参上不同：在横跨 15 个数量级的 120 个入参上实测，`sqrt` 从不出现差异，`exp`、`log` 只有个位数（小于 1 ulp），`log10`、`tanh`、`asin`、`acos`、`atan` 有几十个（约 1 ulp），`sin`、`cos`、`tan` 占四分之一到一半。超过 2^53 之后需要先做参数归约，差距会拉大——`tan` 偏 6 ulp、`sin` 偏 7 ulp——因为两边都不是正确舍入，归约方式也不同；而且当结果本身趋近 0 时，一个 ulp 只是一点点绝对误差，所以大入参的 `cos` 读起来能差出几十 ulp，实际上两者相差不过归约后的一次舍入。所有算术运算、`sqrt` 与两种整数转换都是精确的。NaN 按平台 C 库的写法拼写（macOS 恒为 `"nan"`，glibc 符号位为 1 时是 `"-nan"`），因为 C 的 `printf` 就是这么做的。刻意绕开 Go 实现的地方有三处。一是 `exp(|x|)` 溢出而 `sinh(x)`、`cosh(x)` 尚未溢出的那条带（709.78 < |x| < 710.48）：Go 的 `math.Sinh` 对 `math.sinh(710)` 返回 `inf`，而其真值是 `exp(710)/2` = 1.1e308、完全可表示，所以那里把「减半」折进了参数里。二是同一段范围的另一头：`log_amd64.s` 在**次正规数**（subnormal）入参上会丢掉好几位——`1e-308` 就是一个，最小正规数是 2.2250738585072014e-308——把 `log(1e-308)` 算成 `-708.71839997088`，而真值是 `-709.19620864217`。所以 `math.log`、`math.log10` 先把次正规数乘 2 的幂放大到正规区间（这一步是精确的），再把指数减回去（`mathLogScaled`），`math.log10(1e-308)` 于是又是 `-308`。三是 `math.log(x, 2)`：它要的是 C 的 `log2(x)`，而不是 `math.log(x)/math.log(2)`。Go 的 `math.Log2` 先用 `Frexp` 拆分入参、再把指数加回去，于是当答案趋近 0 时——1 附近的数的对数正是这样——它只剩下两个比它大一百万倍的数字的舍入残差：`math.log(1.0000000000000002, 2)` 在 C 的 `log2` 里是 3.2034265038149e-16，在 `math.Log2` 里是 3.6816598061451e-16，差 15%，与本列表里其他任何一项都不同。所以在 `[sqrt(2)/2, sqrt(2))` 这段里——`math.Log` 在此无需加回指数，低位得以保留——`mathLog2Scaled` 改问 `math.Log` 再除以 ln 2；在这段之外，以及所有 2 的幂上（`math.Log2` 在那里是精确的），它问 `math.Log2`，因为它的误差不像商那样随指数增长。从 `1 + 2^-52` 到 `5e-324` 共 26 个答案钉在同一个文件里。

有一个后果值得单独点名，因为它比"一个数字差了一次舍入"走得更远：两个数字字面量做
`A op B` 是**编译器折叠**的，不在运行时计算——所以不精确的 `pow` 不只是让 `2^2.5` 打印
错，而是让它成为**字节码里的一个常量**：C 下是 `0x1.6a09e667f3bcdp+2`，这里是
`0x1.6a09e667f3bccp+2`。这是两个相同的程序能编译出不同字节的**唯一**途径，也是
codegen fuzz 所由生长的那次 103 786 片段探针里**唯一的**差异（§6.5）。其余能折叠的运算
都是精确的：算术运算是 IEEE 的，`//` 与 `%` 精确可定义、两个库答得一样，整数与字符串的
转换由表示法决定。拿 `Exp2(y*Log2(x))` 与"拆分指数"的变体在 922 组上实测，Go 的
`math.Pow` 是三者中最接近的——150 处不同，另两者是 456 与 473——所以没有可换的东西。
9. **`math.randomseed()` 不带参数时第二个种子不同**：PUC-Lua 返回的两个种子是「当前时间」与「`lua_State` 的地址」（lmathlib.c 的 `randseed`），本移植返回的也是这两个量，所以第一个种子（时间）一致，第二个是 Go 堆地址而非 C 堆地址——两者都是宿主地址，在任一实现里都不可复现。`math.randomseed(n)` 与 `math.randomseed(n, m)` 原样返回入参；生成器本身仍是同一个基于 xoshiro256** 的实现：同一种子下产出的序列完全相同，整数与浮点都是，用例已钉住 `math.random()`、`math.random(6)`、`math.random(10, 20)` 各 20 次取值。
10. **协程是「停住 goroutine」而不是 longjmp，挂起的 C 帧醒来时还在**：C 的 `lua_yield` 是 longjmp 到「将来会把它接起来的那个 `lua_resume`」，中间的一切都被丢弃；本实现则把 yield 的值经 channel 交给恢复者，然后阻塞在下一个 resume 请求上，于是 Lua 栈、保存的程序计数器、乃至 C 调用者自己的栈，全都活在 yield 的那个点上。这正是元方法与 Go 函数能从调用中途 yield 的原因，也是延续能成立的原因（§5.6）。随之而来两点：协程从恢复者继承的 C 调用深度是**每次 yield 都刷新**，而非只在第一次 resume 时刷新——这与 C 一致，因为 `lua_resume` 每次 resume 都跑一遍。另外 yield 过的 C 函数察觉不到这件事：`lua_callk` 会像调用正常结束那样返回它，所以 `lua_callk` 的延续听到的 status 是 `LUA_OK`，而 PUC-Lua 那里恒为 `LUA_YIELD`——是个常量，没有东西能依赖这个差别。`lua_pcallk` 的 status 是会变的，那个**与 PUC-Lua 一致**：本实现数着该线程 yield 了几次，被 yield 打断时报告 `LUA_YIELD`。
11. **两个相等的短字符串不一定是同一个对象**：PUC-Lua 对短字符串的驻留贯穿其整个生命周期，所以 `lua_topointer` 对两个相等的串返回同一个地址，C 扩展也能拿这个地址当键。本实现的驻留是两代缓存（§5.11），一个串在两代都过期后可能被重新驻留，于是两个内容相同且都活着的串可能位于不同地址。Lua 层面看不到这件事：字符串按内容比较，所以 `luaS_eqshrstr` 是「比哈希再比字节」，而 C 那里的 `eqshrstr` 只比指针；`rawequal`、`==`、表的键与 `next` 的行为都和以前一样。只有 `lua_topointer`——以及拿它当键的 `lua_rawsetp`——能看出差别。

12. **落在奇数位上的 `%a` 平局按「取偶」处理——这是 glibc 的做法，不是 Apple 库的做法**：PUC-Lua 把每个转换都交给平台的 `sprintf`，所以 `string.format("%.0A", 1.5)` 的答案就是那个库的答案，而各库的答案并不一致——`1.5` 是 `0x1.8p+0`，精度为 0 时丢掉的正好是保留位的一半。glibc 的 `__printf_fp` 把保留的那一位交给 `round_away`：偶数留着、奇数进一，于是答 `0X2P+0`；Apple 的库则无论奇偶一律向下，答 `0X1P+0`。本实现答 `0X2P+0`：这是各平台 C 库自己的 `%f` 的答案（两个库里 `%.0f` 格式化 1.5 都是 `2`），也是让同一个调用中 `%a` 与 `%f` 自相一致的那一个。所以在库是 Apple 那一份的机器上，`%.0a` 格式化 1.5 在本实现里是 `0x2p+0`、在 PUC-Lua 里是 `0x1p+0`——这是全库唯一一处「差异属于 C 库而非本移植」的情形。落在**偶数**位上的平局两种规则殊途同归，那里没有差异；fuzz 会先问参考解释器它链接的是哪一份库，再把其余的排除掉（§5.13）。

13. **次正规数按**规范化**写法输出——这是 Apple 库的做法，不是 glibc 的做法**：规范化浮点在 `%a` 下的首位十六进制数字是 1；小到没有 1 可规范化的数（小于
2.2250738585072014e-308）C 把这一位留给实现自己定，因为本来就没有 1 可规范。
glibc 保留最小正规数的指数、把 52 位尾数原样写在小数点后，于是
`string.format("%a", 5e-324)` 是 `0x0.0000000000001p-1022`；Apple 的库会规范化，
本实现也规范化，那里是 `0x1p-1074`——Go 自己的十六进制转换就是这个写法，也是一个数
而不是一个数的两种写法。与上面的平局不同，**这一处标准允许两种写法**，所以没有谁更
正确。它的影响不止 `%a`：`%q` 写浮点用的是同一套，而 `%q` 正是程序把一个数写回去时
用的转换。两种写法都是 Lua 读得进的数字字面量，所以一边写出来的值另一边都读得回。
fuzz 会问参考解释器它链接的是哪一份库；若它答 `0x0.0000000000001p-1022`，网格里依赖
它的用例两边都不问。

14. **负的 `file:setvbuf(mode, size)` 一律成功**：C 把 `size` 当作 `size_t` 交给 `setvbuf`，所以 `-1` 在那里是 `SIZE_MAX`，而库拿它做什么就是库自己的事了——Apple 那份向分配器要一个那么大的缓冲，失败，并且不动 `errno`，于是 PUC-Lua 答 `nil, "(no extra info)", 0`；glibc 在没有给缓冲时根本不看这个大小，答 `true`。不存在可移植的答案，于是本实现答 `true`，网格也不问这个用例（§5.13）。其余每个大小都是 PUC-Lua 的答案——包括 0，glibc 的 `setvbuf` 把它当成「没有要求缓冲」。
15. **注册表里没有 `_UBOX*` 这个键**：`lauxlib.c` 的 `luaL_Buffer` 一旦超过 `LUAL_BUFFERSIZE` 就要搬到栈上的一个 userdata 里，而为它准备元表这件事只发生一次——`string.rep("x", 9000)` 就够了，此后 `"_UBOX*"` 就一直待在 `debug.getregistry()` 里。本实现的缓冲是 Go 切片，从来不是栈上的对象，所以不存在「该造这个键」的时刻。其余每个键都与 PUC-Lua 一致并被逐条比对——主线程、`_LOADED`、`_PRELOAD`、`_CLIBS`、`_IO_input`、`_IO_output`、`FILE*` 与 `_HOOKKEY`——网格把这一个排除在外（§5.13）。
16. **性能**：本实现比 C 慢一个随负载而变的常数倍——普通代码 1.4× 到 3.6×（§5.11），协程 6.0×，因为一次 yield/resume 是两次 goroutine 切换而非两个栈指针互换。这是与 PUC-Lua 的**性质**差异，不是**行为**差异：没有任何程序会因此给出不同的答案。唯一能观察到它的地方是「脚本本身就是截止时间」——一边超时、另一边跑完的负载——官方套件里的 `heavy` 之所以是已知差异，就是这个原因。

---

## 8. 构建与测试

```bash
go build ./...            # 构建
go test ./...             # 单元测试（含 Lua 语义用例）
go test -race ./...       # 竞态检测
go vet ./...
gofmt -l .                # 必须无输出
go test -bench=. -benchmem -run=XXX ./...   # 基准
go build -o lua  ./cmd/lua    # 命令行解释器
go build -o luac ./cmd/luac   # 命令行编译器
```

其中两项需要的不止 Go：`cdlopen_test.go` 与 `cdlopen_api_test.go` 会编译 C 扩展
并 `require` 它们，在没有 cgo 或没有 C 编译器时自行跳过；因此
`CGO_ENABLED=0 go test ./...` 也是受支持的配置，只是不走 dlopen 那条路径。

CI 在 Linux / macOS / Windows 上跑完整矩阵（`.github/workflows/go.yml`），四个作业：

| 作业 | 平台 | 内容 |
|---|---|---|
| **build** | 3 OS × `oldstable`+`stable` | `go vet` + `go build` + `go test`；Linux/macOS 加 `go test -race`；Linux 另加 `gofmt` 与一轮 `benchtime=1x` 的基准自检 |
| **nocgo** | 3 OS | `CGO_ENABLED=0` 下的 `go build` + `go test`，外加 5 个目标的交叉编译——纯 Go 这条路的承诺由它守住 |
| **official** | Linux + macOS | 从 lua.org 拉 5.4.9 源码现编一个参考解释器，`LUA_REF` 指向它跑 `TestOfficialSuite`、`TestDifferentialSuite` 与 `TestThirdPartySuite`（§5.12 那五个第三方库）：先采集本平台的 golden，再逐字节比对。这样这些套件在每个平台上都是真闸门，已入库的 darwin golden 也不会悄悄烂掉（Windows 不在此列：Lua 自带的 makefile 是 Unix 的） |

参考源码用于对照审校（不入库）：`lua-5.4.9`，<https://www.lua.org/ftp/lua-5.4.9.tar.gz>。

---

## 9. 许可证

MIT（与原仓库一致）。
