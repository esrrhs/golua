# fakescript_go

[![License](https://img.shields.io/github/license/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Language](https://img.shields.io/github/languages/top/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Release](https://img.shields.io/github/v/release/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go/releases)
[![Build Status](https://github.com/esrrhs/fakescript_go/actions/workflows/go.yml/badge.svg?branch=master)](https://github.com/esrrhs/fakescript_go/actions)

轻量级纯 Go 编写的嵌入式脚本语言，无任何第三方依赖 (Go 1.22+)。

[English](./README.md)

---

## 项目简介

**fakescript_go** 是一款纯 Go 编写的轻量级嵌入式脚本语言，不依赖任何第三方外部库。语法设计借鉴了 Lua、Go 和 Erlang。底层基于 `nex` 与 `goyacc` 生成语法树，将其编译为高效的字节码并由内置虚拟机解释器执行。

## 核心特性

* **简洁易学**：类似 Lua 的清晰语法结构。
* **零外部依赖**：纯 Go 原生实现，仅依赖 Go 标准库。
* **纯函数式模型**：一切皆函数，原生支持多返回值。
* **丰富容器支持**：内置动态数组（`array()`）与映射表（`map()`），支持任意嵌套。
* **结构体支持**：支持轻量级自定义数据结构体（`struct`）。
* **模块化与命名空间**：支持 `package` 与 `include` 实现模块化与包隔离。
* **丰富数据类型**：支持 Int64（`UUID`）、`const` 常量定义、浮点数以及字符串。
* **无缝 Go 双向互操作**：极简方式直接注册并调用 Go 原生函数。
* **内置性能分析器**：内置 Profiler，可采集并输出脚本各函数执行耗时。
* **安全沙箱保护**：内置执行调用栈深度限制，防范死循环与内存溢出。

---

## 脚本语法示例

```fakescript
-- 定义包名
package mypackage.test

-- 引入依赖脚本
include "common.fk"

-- 结构体定义
struct User
    id
    name
    extra
end

-- 常量定义
const MAX_COUNT = 100
const DEFAULT_NAME = "Guest"
const CONFIG_MAP = {1 : "Alpha" 2 : "Beta"}

-- 函数定义
func process_user(arg1, arg2)

    -- 调用已绑定的 Go 原生函数
    var sum = add(arg1, 10)

    -- 分支控制
    if arg1 < arg2 then
        print("arg1 < arg2")
    elseif arg1 == arg2 then
        print("两值相等")
    else
        print("arg1 大于 arg2")
    end

    -- For 循环
    for var i = 0, i < arg2, i++ then
        print("当前循环索引: ", i)
    end

    -- 动态数组
    var arr = array()
    arr[0] = 100
    arr[1] = 200

    -- 动态哈希 Map
    var m = map()
    m["key"] = "value"
    m[1] = arr

    -- 结构体实例化与访问
    var u = User()
    u->id = 1001
    u->name = "Alice"

    -- Switch 语句
    switch arg1
        case 1 then
            print("分支 1")
        case "a" then
            print("分支 a")
        default
            print("默认分支")
    end

    -- 返回多个值
    return sum, m["key"]
end
```

---

## Go 嵌入示例

### 1. 快速上手

```go
package main

import (
	"fmt"
	"github.com/esrrhs/fakescript_go"
)

func main() {
	// 1. 解析脚本文件
	if err := fakescript_go.Parse("test.fk"); err != nil {
		panic(err)
	}

	// 2. 执行脚本函数
	ret, err := fakescript_go.Run("mypackage.test.process_user", 1, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println("Result:", ret) // [11 value]
}
```

### 2. 绑定 Go 原生函数

```go
// 注册 Go 原生函数供脚本调用
fakescript_go.RegFunc("add", func(a int, b int) int {
	return a + b
})
```

### 3. 配置引擎参数

```go
fakescript_go.SetConfig(fakescript_go.FakeConfig{
	OpenLog:        false, // 是否输出调试日志
	ArrayGrowSpeed: 50,    // 数组扩容步长百分比
	StackMax:       10000, // 栈深度限制
	OpenProfile:    false, // 是否开启性能采样分析
	FakePrint: func(str string) {
		fmt.Print(str)    // 自定义脚本 print 输出
	},
})
```

---

## 命令行工具 (CLI)

```bash
# 编译命令行工具
go build -o fakescript_go ./cmd

# 执行脚本并调用函数
./fakescript_go script.fk mypackage.myfunc arg1 arg2
```

---

## 源码构建与代码生成

支持标准 `//go:generate` 以及跨平台生成脚本：

```bash
# 运行单元测试
go test -v -race ./...

# 重新生成词法/语法解析器 (需要 nex 与 goyacc)
# Linux / macOS
./gen.sh
# 或
go generate ./...

# Windows
gen.bat
```
