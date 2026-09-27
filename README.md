# fakescript_go

[<img src="https://img.shields.io/github/license/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[![Go Report Card](https://goreportcard.com/badge/github.com/esrrhs/fakescript_go)](https://goreportcard.com/report/github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/v/release/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fakescript_go/go.yml?branch=master">](https://github.com/esrrhs/fakescript_go/actions)

> **A lightweight, embeddable scripting language implemented in pure Go with zero external dependencies (Go 1.22+).**

[English](#english) | [中文说明](#中文说明)

---

<span id="english"></span>
## Introduction

**fakescript_go** is a lightweight, embeddable scripting language written in pure Go without any external dependencies. Its syntax draws inspiration from Lua, Go, and Erlang. The engine generates syntax trees via `nex` and `goyacc`, compiling scripts into bytecode for virtual machine interpretation.

* [fake for C++](https://github.com/esrrhs/fake)
* [fake for Java](https://github.com/esrrhs/fakejava)

## Key Features

* **Lua-like Syntax**: Simple, expressive, and easy to learn.
* **Zero External Dependencies**: Pure Go implementation; only relies on the Go standard library.
* **Rich Data Types**: Built-in support for array, map (with unlimited nesting), struct, Int64, and constants.
* **Multi-Return Values**: First-class support for functions with multiple return values.
* **Seamless Go Interoperability**: Easily bind Go native functions for scripts to invoke.
* **Comprehensive Control Flow**: Supports `if-elseif-else`, `while`, `for`, and `switch-case`.
* **Built-in Profiler**: Measure runtime execution time per script function.
* **Execution Safety**: Call stack depth protection against infinite loops and memory exhaustion.

## Script Example

```lua
-- Current package name
package mypackage.test

-- Include other scripts
include "common.fk"

-- Struct definition
struct teststruct
	sample_a
	sample_b
	sample_c
end

-- Constants
const hellostring = "hello"
const helloint = 1234
const hellomap = {1 : "a" 2 : "b" 3 : [1 2 3]}

-- Function definition
func myfunc1(arg1, arg2)

	-- Branching
	if arg1 < arg2 then
		print("arg1 < arg2")
	elseif arg1 == arg2 then
		print("elseif")
	else
		print("else")
	end

	-- For loop
	for var i = 0, i < arg2, i++ then
		print("i = ", i)
	end

	-- Arrays
	var a = array()
	a[1] = 3

	-- Maps
	var b = map()
	b[a] = 1
	b[1] = a

	-- Int64 (UUID)
	var uid = 1241515236123614u
	print("uid = ", uid)

	-- Structs
	var tt = teststruct()
	tt->sample_a = 1
	tt->sample_b = teststruct()
	tt->sample_b->sample_a = 10

	-- Switch statements
	switch arg1
		case 1 then
			print("1")
		case "a" then
			print("a")
		default
			print("default")
	end

	-- Multiple return values
	return arg1, arg2 + 10
end
```

## Go Usage

### 1. Basic Execution
```go
package main

import (
	"fmt"
	"github.com/esrrhs/fakescript_go"
)

func main() {
	// Parse script file
	if err := fakescript_go.Parse("test.fk"); err != nil {
		panic(err)
	}

	// Run script function
	ret, err := fakescript_go.Run("mypackage.test.myfunc1", 1, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println(ret) // [1 12]
}
```

### 2. Binding Go Functions
```go
// Register a Go function for scripts to call
fakescript_go.RegFunc("add", func(a int, b int) int {
	return a + b
})
```

### 3. Engine Configuration
```go
fakescript_go.SetConfig(fakescript_go.FakeConfig{
	OpenLog:        false, // Enable debug logging
	ArrayGrowSpeed: 50,    // Array resize factor (%)
	StackMax:       10000, // Maximum call stack depth
	OpenProfile:    false, // Enable performance profiler
	FakePrint: func(str string) {
		fmt.Print(str)    // Custom print handler
	},
})
```

## Command-Line Tool (CLI)
```bash
# Build CLI binary
go build -o fakescript_go ./cmd

# Execute script with arguments
./fakescript_go script.fk mypackage.myfunc arg1 arg2
```

## Regenerating Parser / Lexer
Standard `//go:generate` and cross-platform generator scripts are supported:
```bash
# Linux / macOS
./gen.sh
# or
go generate ./...

# Windows
gen.bat
```

---

<span id="中文说明"></span>
## 中文说明

**fakescript_go** 是一款纯 Go 实现的轻量级嵌入式脚本语言，不依赖任何第三方外部库。语法借鉴自 Lua、Golang 和 Erlang，通过 `nex` 与 `goyacc` 构建抽象语法树，编译生成字节码并在虚拟机中解释执行。

### 脚本特性
* **语法类似 Lua**：语法简洁明了，易于上手，兼具动态语言的灵活性。
* **零外部依赖**：仅使用 Go 原生标准库实现。
* **丰富数据结构**：原生支持 array、map（可无限级嵌套）、struct、Int64、const 常量。
* **多返回值机制**：完整支持多参数返回与解构接收。
* **Go 双向绑定**：支持将 Go 语言函数直接注入到脚本中透明调用。
* **完善流程控制**：支持 `if-elseif-else`、`while`、`for` 与 `switch-case`。
* **内置 Profile 采样**：自带函数级性能度量与耗时统计。
* **安全沙箱保护**：内置执行调用栈深度限制，防范死循环与内存溢出。

### Go 快速上手

```go
package main

import (
	"fmt"
	"github.com/esrrhs/fakescript_go"
)

func main() {
	// 解析脚本
	if err := fakescript_go.Parse("test.fk"); err != nil {
		panic(err)
	}

	// 运行指定函数
	ret, err := fakescript_go.Run("mypackage.test.myfunc1", 1, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println(ret) // 输出返回值列表 [1 12]
}
```

#### 注册 Go 函数
```go
fakescript_go.RegFunc("add", func(a int, b int) int {
	return a + b
})
```

#### 配置参数
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

### 命令行工具 (CLI)
```bash
# 编译命令行工具
go build -o fakescript_go ./cmd

# 执行脚本并调用函数
./fakescript_go script.fk mypackage.myfunc arg1 arg2
```

### 代码生成
```bash
# Linux / macOS
./gen.sh
# 或
go generate ./...

# Windows
gen.bat
```
