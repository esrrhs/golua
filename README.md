# fakescript_go

[<img src="https://img.shields.io/github/license/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[![Go Report Card](https://goreportcard.com/badge/github.com/esrrhs/fakescript_go)](https://goreportcard.com/report/github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/v/release/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fakescript_go/go.yml?branch=master">](https://github.com/esrrhs/fakescript_go/actions)

轻量级嵌入式脚本语言 (Go 1.22+)

[README_EN](./README_EN.md)

## 简介
**fakescript_go** 是一款轻量级的嵌入式脚本语言，纯 Go 实现，无第三方外部依赖。语法吸取自 Lua、Golang、Erlang，基于 nex、goyacc 生成语法树，编译成字节码解释执行。

* [C++ 版本 fake](https://github.com/esrrhs/fake)
* [Java 版本 fake](https://github.com/esrrhs/fakejava)

## 脚本特性
* **语法类似 Lua**：简洁易学，兼具动态语言灵活性与表达力
* **零外部依赖**：仅依赖 Go 标准库
* **丰富数据类型**：支持 array、map（可无限嵌套）、struct、Int64、const 常量
* **函数与多返回值**：支持函数定义、多返回值返回与接收
* **Go 双向绑定**：支持将 Go 原生函数直接注册到脚本中调用
* **控制流完善**：支持 if-elseif-else、while、for、switch-case
* **性能与统计**：自带 profile 功能，可获取脚本各函数的执行耗时
* **安全沙箱**：内置执行调用栈深保护，杜绝死循环导致内存耗尽

## 脚本示例

```lua
-- 当前包名
package mypackage.test

-- 引入的文件
include "common.fk"

-- 结构体定义
struct teststruct
	sample_a
	sample_b
	sample_c
end

-- 常量值
const hellostring = "hello"
const helloint = 1234
const hellomap = {1 : "a" 2 : "b" 3 : [1 2 3]}

-- 函数定义
func myfunc1(arg1, arg2)

	-- 分支
	if arg1 < arg2 then
		print("arg1 < arg2")
	elseif arg1 == arg2 then
		print("elseif")
	else
		print("else")
	end

	-- for 循环
	for var i = 0, i < arg2, i++ then
		print("i = ", i)
	end

	-- 数组
	var a = array()
	a[1] = 3

	-- 集合 (map)
	var b = map()
	b[a] = 1
	b[1] = a

	-- Int64 (UUID)
	var uid = 1241515236123614u
	print("uid = ", uid)

	-- 结构体
	var tt = teststruct()
	tt->sample_a = 1
	tt->sample_b = teststruct()
	tt->sample_b->sample_a = 10

	-- switch 分支
	switch arg1
		case 1 then
			print("1")
		case "a" then
			print("a")
		default
			print("default")
	end

	-- 多返回值
	return arg1, arg2 + 10
end
```

## Go 中使用

### 1. 快速执行
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

### 2. 绑定 Go 原生函数
```go
// 注册 Go 函数供脚本调用
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

## CLI 工具
```bash
# 编译命令行工具
go build -o fakescript_go ./cmd

# 执行脚本并调用函数
./fakescript_go script.fk mypackage.myfunc arg1 arg2
```

## 重新生成词法/语法解析器
项目已配置标准 `//go:generate` 及跨平台生成脚本：
```bash
# Linux / macOS
./gen.sh
# 或
go generate ./...

# Windows
gen.bat
```
