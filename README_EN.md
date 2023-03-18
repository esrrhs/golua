# fakego

[<img src="https://img.shields.io/github/license/esrrhs/fakego">](https://github.com/esrrhs/fakego)
[<img src="https://img.shields.io/github/languages/top/esrrhs/fakego">](https://github.com/esrrhs/fakego)
[![Go Report Card](https://goreportcard.com/badge/github.com/esrrhs/fakego)](https://goreportcard.com/report/github.com/esrrhs/fakego)
[<img src="https://img.shields.io/github/v/release/esrrhs/fakego">](https://github.com/esrrhs/fakego/releases)
[<img src="https://img.shields.io/github/actions/workflow/status/esrrhs/fakego/go.yml?branch=master">](https://github.com/esrrhs/fakego/actions)

Lightweight embedded scripting language

## Brief introduction
**fake** is a lightweight embedded scripting language , using Go language, grammar lessons from lua, golang, erlang, based on nex, goyacc generative grammar tree , compiled into byte code interpreted. 

[fake for C++](https://github.com/esrrhs/fake)

[fake for Java](https://github.com/esrrhs/fakejava)

## Script features
* The syntax is similar to lua
* All are functions
* Support array, map, unlimited nesting
* Support function binding
* Comes with interpreter
* Support multiple return values
* With its own profile, you can get the running time of each function of the script
* Support hot update
* Support Int64
* Support const definition
* Support package
* Support struct

## Sample

```


-- Current package name
package mypackage.test

-- include file
include "common.fk"

-- struct define
struct teststruct
	sample_a
	sample_b
	sample_c
end

-- const define
const hellostring = "hello"
const helloint = 1234
const hellomap = {1 : "a" 2 : "b" 3 : [1 2 3]}

-- func1 comment
func myfunc1(arg1, arg2)
	
	-- C function calls and class member functions
	arg3 := cfunc1(helloint) + arg2:memfunc1(arg1)
	
	-- Branch
	if arg1 < arg2 then	
		-- create routine
		fake myfunc2(arg1, arg2)
	elseif arg1 == arg2 then	
		print("elseif")
	else
		print("else")
	end
	
	-- for loop
	for var i = 0, i < arg2, i++ then
		print("i = ", i)
	end
	
	-- array use
	var a = array()
	a[1] = 3
	
	-- map use
	var b = map()
	b[a] = 1
	b[1] = a
	
	-- Int64
	var uid = 1241515236123614u
	log("uid = ", uid)

	-- sub func call
	var ret1, var ret2 = myfunc2()

	-- other package call
	ret1 = otherpackage.test.myfunc1(arg1, arg2)
	
	-- struct use
	var tt = teststruct()
	tt->sample_a = 1
	tt->sample_b = teststruct()
	tt->sample_b->sample_a = 10

	-- switch branch
	switch arg1
		case 1 then
			print("1")
		case "a" then
			print("a")
		default
			print("default")
	end

	-- multi return value
	return arg1, arg3
	
end
```

## Go Sample
```
fakego.Parse("test.fk")
ret, err := fakego.Run("mypackage.test_return_value", 1, "2")
fmt.Println(ret)
```
