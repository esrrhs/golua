# fakescript_go

[<img src="https://img.shields.io/github/license/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go)
[![Go Report Card](https://goreportcard.com/badge/github.com/esrrhs/fakescript_go)](https://goreportcard.com/report/github.com/esrrhs/fakescript_go)
[<img src="https://img.shields.io/github/v/release/esrrhs/fakescript_go">](https://github.com/esrrhs/fakescript_go/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fakescript_go/go.yml?branch=master">](https://github.com/esrrhs/fakescript_go/actions)

Lightweight embedded scripting language (Go 1.22+)

## Brief Introduction
**fakescript_go** is a lightweight embedded scripting language written in pure Go without any external dependencies. The grammar draws inspiration from Lua, Go, and Erlang, using nex and goyacc to generate the syntax tree and compiling to bytecode for interpretation.

* [fake for C++](https://github.com/esrrhs/fake)
* [fake for Java](https://github.com/esrrhs/fakejava)

## Script Features
* **Lua-like syntax**: Easy to learn, expressive and dynamic
* **Zero external dependencies**: Built entirely on Go standard library
* **Rich data types**: Array, map (with unlimited nesting), struct, Int64, and const definitions
* **Functions & Multi-return**: Full support for multiple return values
* **Go Interoperability**: Seamlessly register Go native functions to be called inside scripts
* **Comprehensive control flow**: `if-elseif-else`, `while`, `for`, `switch-case`
* **Performance profiling**: Built-in profiler measuring execution time per script function
* **Sandbox safety**: Call stack depth protection against infinite loops

## Script Sample

```lua
-- Current package name
package mypackage.test

-- Include file
include "common.fk"

-- Struct definition
struct teststruct
	sample_a
	sample_b
	sample_c
end

-- Const definitions
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

	-- Array
	var a = array()
	a[1] = 3

	-- Map
	var b = map()
	b[a] = 1
	b[1] = a

	-- Int64 (UUID)
	var uid = 1241515236123614u
	print("uid = ", uid)

	-- Struct
	var tt = teststruct()
	tt->sample_a = 1
	tt->sample_b = teststruct()
	tt->sample_b->sample_a = 10

	-- Switch statement
	switch arg1
		case 1 then
			print("1")
		case "a" then
			print("a")
		default
			print("default")
	end

	-- Multi return value
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

	// Run function
	ret, err := fakescript_go.Run("mypackage.test.myfunc1", 1, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println(ret) // [1 12]
}
```

### 2. Binding Go Functions
```go
// Register a Go function to be called from fakescript_go script
fakescript_go.RegFunc("add", func(a int, b int) int {
	return a + b
})
```

### 3. Engine Configuration
```go
fakescript_go.SetConfig(fakescript_go.FakeConfig{
	OpenLog:        false, // Debug logs
	ArrayGrowSpeed: 50,    // Array resize factor (%)
	StackMax:       10000, // Maximum stack depth
	OpenProfile:    false, // Performance profiling
	FakePrint: func(str string) {
		fmt.Print(str)    // Custom print handler
	},
})
```

## CLI Tool
```bash
# Build the CLI
go build -o fakescript_go ./cmd

# Execute script and call function
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
