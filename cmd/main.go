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
	}
}
