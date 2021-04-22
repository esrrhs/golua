package main

import (
	"fmt"
	"github.com/esrrhs/fakego"
)

func main() {
	fakego.SetConfig(fakego.FakeConfig{OpenLog: true})
	err := fakego.Parse("test.fk")
	if err != nil {
		fmt.Printf("parse %v\n", err)
		return
	}
	test_return_value()
}

func test_return_value() {
	ret, err := fakego.Run("mypackage.test_return_value", 1, "2")
	if err != nil {
		panic(err)
	}
	if len(ret) != 2 {
		panic("fail")
	}
	if ret[0] != 1 {
		panic("fail")
	}
	if ret[1] != "a" {
		panic("fail")
	}
}
