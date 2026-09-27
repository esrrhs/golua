# fakescript_go

[![License](https://img.shields.io/github/license/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Language](https://img.shields.io/github/languages/top/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go)
[![Release](https://img.shields.io/github/v/release/esrrhs/fakescript_go)](https://github.com/esrrhs/fakescript_go/releases)
[![Build Status](https://github.com/esrrhs/fakescript_go/actions/workflows/go.yml/badge.svg?branch=master)](https://github.com/esrrhs/fakescript_go/actions)

A lightweight embedded scripting language written in pure Go with zero external dependencies (Go 1.22+).

[简体中文](./README_CN.md)

---

## Overview

**fakescript_go** is a lightweight, embeddable scripting language implemented in pure Go without any third-party dependencies. Its syntax draws inspiration from Lua, Go, and Erlang. The compiler utilizes `nex` and `goyacc` to generate syntax trees, compiles source code into bytecode, and executes it via an internal virtual machine interpreter.

## Key Features

* **Clean Syntax**: Lua-inspired, concise, and clean grammar.
* **Zero External Dependencies**: Pure Go implementation; only relies on the Go standard library.
* **Pure Functional**: Everything is modeled as functions; supports multiple return values.
* **Rich Containers**: Built-in dynamic arrays (`array()`) and maps (`map()`) with arbitrary nesting.
* **Struct Support**: Lightweight user-defined data structures (`struct`).
* **Modular System**: Namespace separation using `package` and `include`.
* **Types & Constants**: Built-in support for Int64 (`UUID`), `const` definitions, numbers, and strings.
* **Seamless Go Interoperability**: Direct and easy binding for Go native functions.
* **Built-in Profiler**: Measure runtime execution time per script function.
* **Sandbox Safety**: Call stack depth protection against infinite loops and stack overflow.

---

## FakeScript Syntax Example

```fakescript
-- Define package namespace
package mypackage.test

-- Include external script files
include "common.fk"

-- Struct definition
struct User
    id
    name
    extra
end

-- Constants
const MAX_COUNT = 100
const DEFAULT_NAME = "Guest"
const CONFIG_MAP = {1 : "Alpha" 2 : "Beta"}

-- Function definition
func process_user(arg1, arg2)

    -- Calling bound Go native functions
    var sum = add(arg1, 10)

    -- Conditional statements
    if arg1 < arg2 then
        print("arg1 < arg2")
    elseif arg1 == arg2 then
        print("Values are equal")
    else
        print("arg1 is greater than arg2")
    end

    -- For loop
    for var i = 0, i < arg2, i++ then
        print("Loop index: ", i)
    end

    -- Dynamic Array
    var arr = array()
    arr[0] = 100
    arr[1] = 200

    -- Dynamic Map
    var m = map()
    m["key"] = "value"
    m[1] = arr

    -- Int64 (UUID)
    var uid = 1241515236123614u
    print("uid = ", uid)

    -- Struct usage
    var u = User()
    u->id = 1001
    u->name = "Alice"

    -- Switch case
    switch arg1
        case 1 then
            print("case 1")
        case "a" then
            print("case a")
        default
            print("default")
    end

    -- Return multiple values
    return sum, m["key"]
end
```

---

## Go Integration Example

### 1. Basic Execution

```go
package main

import (
	"fmt"
	"github.com/esrrhs/fakescript_go"
)

func main() {
	// 1. Parse script file
	if err := fakescript_go.Parse("test.fk"); err != nil {
		panic(err)
	}

	// 2. Run script function
	ret, err := fakescript_go.Run("mypackage.test.process_user", 1, 2)
	if err != nil {
		panic(err)
	}
	fmt.Println("Result:", ret) // [11 value]
}
```

### 2. Binding Go Functions

```go
// Register a Go function to be called within scripts
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

---

## Command-Line Tool (CLI)

```bash
# Build CLI binary
go build -o fakescript_go ./cmd

# Execute script and call function
./fakescript_go script.fk mypackage.myfunc arg1 arg2
```

---

## Building from Source & Code Generation

Standard `//go:generate` and cross-platform generator scripts are supported:

```bash
# Run tests
go test -v -race ./...

# Regenerate lexer and parser (requires nex and goyacc)
# Linux / macOS
./gen.sh
# or
go generate ./...

# Windows
gen.bat
```
