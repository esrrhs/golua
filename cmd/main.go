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
	ret, err := fakego.Run("json.test_run", 1, "2")
	if err != nil {
		fmt.Printf("Run %v\n", err)
		return
	}
	fmt.Printf("Run ok %v\n", ret)
}
