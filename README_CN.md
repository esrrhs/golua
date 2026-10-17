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

**33 个用例中 30 个输出完全一致**。其余 3 个的差异全部来自设计取舍
（没有自己的收集器）或测试环境：

| 用例 | 差异原因 |
|---|---|
| `gc` | 这个文件测的**就是**收集器，而这里没有。三处断言量的是增量式收集器：step 之后 `gcinfo()` 会下降、步长越大所需步数越少、一次超大的 step 能跑完一整轮。这里的 step 就是一整轮且不回收 Lua 的对象，三条都不成立。之后该文件靠「一直分配表直到 `gcinfo()` 翻三倍」来量收集器的**速度**——没有回收器时这个循环不终止 |
| `all` | 环境差异，集中在四行：第二个随机数种子（PUC-Lua 截成 32 位，这里是 64 位地址）、`total memory: 44.6K` 对 `619.2K`（同一处口径）、耗时、以及解释器自身路径。套件其余部分全部吻合——包括 `attrib`、`files`、`gengc`、`locals` 加载的 C 扩展库 |
| `heavy` | 两边都超时（每轮要构造 2 MB 的表，总量 2 GB）；在超时截断之前两边一致 |

弱表与 `__gc` 均已实现并通过各自用例——`closure`、`db`、`coroutine`、`gengc`、
`cstack`、`locals`（后两个用 `require"tracegc"` 数回收次数），做法见 §7。

`go test` 内也镜像了这套比对：`testdata/diff/*.lua`（79 个用例）与从 PUC-Lua 5.4.9
采集的 golden 文件逐字节比对；`cli_test.go` 另外钉住解释器自身的可观察行为——
`-v`/`-e`/`-l`/`-E`/`-W`/`--`、`arg` 表、脚本可变参数、usage 文案与退出码。

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

### 5.9 覆盖盘点

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

### 5.10 性能

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

static const luaL_Reg funcs[] = {{"add", l_add}, {NULL, NULL}};

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

---

## 7. 已知差异与限制（相对 PUC-Lua 5.4.9）

1. **`file:read(n)` 的计数超过 2 GiB 时按文件末尾处理**：C 把计数直接交给分配器，荒谬的计数是被分配器拒绝的（报 "not enough memory"；负数计数必然如此，本实现已还原这一点）。本实现不会做那个量级的分配，所以过大的计数读到空。二进制 chunk 不受影响：`string.dump` 产出的字节与 PUC-Lua 5.4.9 **完全一致**（strip 与否都一样），两边互相都能加载对方的产物。
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

userdata 背后的 C 内存会在 userdata 被回收时释放：`C.malloc` 不受 Go 收集器管辖，
没有 finalizer 的话，一个在循环里造 userdata 的扩展会无上限地涨。
用 Lua 写的、或已改成 Go 的模块完全不需要这些：`L.RequireF(modname, openf, true)` 把模块放进
`package.loaded`（对应 C 的 `luaL_requiref`），或把 `package.preload[name]` 设成 Go 函数，
让 `require` 照常找到它。
3. **收集器是 Go 的，可观测时机与 PUC-Lua 不同**：`collectgarbage("count")` 报的是 Go 堆而非 PUC-Lua 分配器的口径；`__gc` 在本实现自己跑一轮的地方执行（收集步进、`collectgarbage`、`Close()`），而不是 PUC-Lua 步进的位置。带 `__gc` 的对象在两轮之间被强引用兜住，避免 Go 在自己挑的时刻终结它们；`Close()` 时会把**所有**还带 `__gc` 的对象跑一遍，可达的也算——这正是 C 的做法，也正是 `<close>`、`Close()` 与弱表确定的原因：没有别处引用就会被清掉。`collectgarbage("step")` 是唯一无法对齐的返回值：PUC-Lua 只在「这一步把收集器带回了暂停态」时才返回 true，而单步几乎从不如此——它连续几百次都返回 false，紧接在一次完整收集之后也一样——而本实现的一步就是一整轮，所以返回 true。若改成返回 false，文档里的惯用法 `while not collectgarbage("step") do end` 就会永远转下去，两害相权取其轻。
4. **弱表条目要多存活一轮**：Go 的 `weak.Pointer` 在释放对象的**下一轮** GC 才失效，因此条目比 PUC-Lua 多活一个周期。字符串、数字、布尔不参与弱化，与 PUC-Lua 一致。
5. **错误信息文案**：与 PUC-Lua 保持一致（直译带来的好处），但 `ldebug` 的源码片段回显在部分路径可能省略。
6. **`os.setlocale` 去问 C 库，所以需要 cgo**：小数点（decimal point）是 locale 里 Lua 唯一真正观察的部分，体现在三处——数字转字符串、`string.format` 的 `%f`/`%e`/`%g`（不含 `%a` 与 `%q`，这正是 `%q` 能往返的原因）、以及 `tonumber`（两种分隔符都收，而源码里的数字字面量始终用 `.`）。装了哪些 locale 由宿主机决定，与 PUC-Lua 完全一致，C 库不认的名字返回 `nil`。以 `CGO_ENABLED=0` 构建时没有 C 库可问，于是除 `"C"` 以外的名字一律返回 `nil`，小数点固定为 `.`。
7. **`os.date` 拒绝的时间范围略有出入，`os.time` 接受宿主 `mktime` 拒绝的日期，`os.date("!%z")` 给出 UTC 偏移而非本地偏移**：`struct tm` 的年份字段是 `int`，宿主 `gmtime`/`localtime` 一旦放不下就返回 NULL，PUC-Lua 的 “date result cannot be represented in this installation” 正来源于此。本移植按 `tm_year` 判同一条件，上界**完全吻合**——67768036191676799 是最后一个可表示的秒（2147485547 年，`tm_year` 恰为 `INT_MAX`），再加一秒即被拒；宿主的下界离 `INT_MIN` 还差一点，所以最负端约 2×10¹⁴ 秒的窗口内本移植给出答案而参考实现拒绝，再往外两边都不给。至于 `os.time`：PUC-Lua 用 C 库的 `mktime` 构造时间，返回 -1 时报 “time result cannot be represented in this installation”。哪些日期会被拒绝由 C 库决定：macOS 上所有归一化后落在 1900 年之前的都被拒，glibc 则接受。Go 的 `time.Date` 没有这个上限，所以本移植返回一个数值。PUC-Lua 自己判定的部分全部复现——字段进入 `struct tm` 前必须满足的边界（"field 'year' is out-of-bound"）、缺失字段与非整数字段，以及结果恰好为 -1 的情形。`%z` 则相反：加了 `!` 之后时间是 UTC，但宿主的 `gmtime` 把偏移量留在本地值上，于是 PUC-Lua 打印本地偏移，而本移植打印 `+0000`。
8. **数学库是 Go 的，不是平台 libm**：`math.sin`、`cos`、`tan`、`exp`、`log`、`log10`、`sqrt`、`asin`、`acos`、`atan`、`atan2`、`pow`、`sinh`、`cosh`、`tanh`、`frexp`、`ldexp` 用的是 Go 的实现，与 libc 链接的那份不同。15 位有效数字一致，最后一位或两位在一部分入参上不同：在横跨 15 个数量级的 120 个入参上实测，`sqrt` 从不出现差异，`exp`、`log` 只有个位数（小于 1 ulp），`log10`、`tanh`、`asin`、`acos`、`atan` 有几十个（约 1 ulp），`sin`、`cos`、`tan` 占四分之一到一半。超过 2^53 之后需要先做参数归约，差距会拉大——`tan` 偏 6 ulp、`sin` 偏 7 ulp——因为两边都不是正确舍入，归约方式也不同。所有算术运算、`sqrt` 与两种整数转换都是精确的。NaN 按平台 C 库的写法拼写（macOS 恒为 `"nan"`，glibc 符号位为 1 时是 `"-nan"`），因为 C 的 `printf` 就是这么做的。唯一刻意绕开 Go 实现的地方是 `exp(|x|)` 溢出而 `sinh(x)`、`cosh(x)` 尚未溢出的那条带（709.78 < |x| < 710.48）：Go 的 `math.Sinh` 对 `math.sinh(710)` 返回 `inf`，而其真值是 `exp(710)/2` = 1.1e308、完全可表示，所以那里把「减半」折进了参数里。
9. **`math.randomseed()` 不带参数时第二个种子不同**：PUC-Lua 返回的两个种子是「当前时间」与「`lua_State` 的地址」（lmathlib.c 的 `randseed`），本移植返回的也是这两个量，所以第一个种子（时间）一致，第二个是 Go 堆地址而非 C 堆地址——两者都是宿主地址，在任一实现里都不可复现。`math.randomseed(n)` 与 `math.randomseed(n, m)` 原样返回入参；生成器本身仍是同一个基于 xoshiro256** 的实现：同一种子下产出的序列完全相同，整数与浮点都是，用例已钉住 `math.random()`、`math.random(6)`、`math.random(10, 20)` 各 20 次取值。
10. **协程是「停住 goroutine」而不是 longjmp，挂起的 C 帧醒来时还在**：C 的 `lua_yield` 是 longjmp 到「将来会把它接起来的那个 `lua_resume`」，中间的一切都被丢弃；本实现则把 yield 的值经 channel 交给恢复者，然后阻塞在下一个 resume 请求上，于是 Lua 栈、保存的程序计数器、乃至 C 调用者自己的栈，全都活在 yield 的那个点上。这正是元方法与 Go 函数能从调用中途 yield 的原因，也是延续能成立的原因（§5.6）。随之而来两点：协程从恢复者继承的 C 调用深度是**每次 yield 都刷新**，而非只在第一次 resume 时刷新——这与 C 一致，因为 `lua_resume` 每次 resume 都跑一遍。另外 yield 过的 C 函数察觉不到这件事：`lua_callk` 会像调用正常结束那样返回它，所以 `lua_callk` 的延续听到的 status 是 `LUA_OK`，而 PUC-Lua 那里恒为 `LUA_YIELD`——是个常量，没有东西能依赖这个差别。`lua_pcallk` 的 status 是会变的，那个**与 PUC-Lua 一致**：本实现数着该线程 yield 了几次，被 yield 打断时报告 `LUA_YIELD`。
11. **两个相等的短字符串不一定是同一个对象**：PUC-Lua 对短字符串的驻留贯穿其整个生命周期，所以 `lua_topointer` 对两个相等的串返回同一个地址，C 扩展也能拿这个地址当键。本实现的驻留是两代缓存（§5.10），一个串在两代都过期后可能被重新驻留，于是两个内容相同且都活着的串可能位于不同地址。Lua 层面看不到这件事：字符串按内容比较，所以 `luaS_eqshrstr` 是「比哈希再比字节」，而 C 那里的 `eqshrstr` 只比指针；`rawequal`、`==`、表的键与 `next` 的行为都和以前一样。只有 `lua_topointer`——以及拿它当键的 `lua_rawsetp`——能看出差别。

12. **性能**：`Instruction` 使用结构体而非位压缩 uint32，分派有额外开销；P7 阶段做基准后再决定是否改回位编码。

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

其中两项需要的不止 Go：`cdlopen_test.go` 与 `cdlopen_api_test.go` 会编译 C 扩展
并 `require` 它们，在没有 cgo 或没有 C 编译器时自行跳过；因此
`CGO_ENABLED=0 go test ./...` 也是受支持的配置，只是不走 dlopen 那条路径。

CI 在 Linux / macOS / Windows 上跑完整矩阵（`.github/workflows/go.yml`）：
全平台 `go vet` + `go build` + `go test`，Linux 与 macOS 加跑 `go test -race`
（竞态检测需要 cgo），Linux 上另有 `gofmt` 检查与一轮 `benchtime=1x` 的基准自检。

参考源码用于对照审校（不入库）：`lua-5.4.9`，<https://www.lua.org/ftp/lua-5.4.9.tar.gz>。

---

## 9. 许可证

MIT（与原仓库一致）。
