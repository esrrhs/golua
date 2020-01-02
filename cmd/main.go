package main

import (
	"github.com/esrrhs/fakego"
	"github.com/esrrhs/go-engine/src/loggo"
)

func main() {
	loggo.Ini(loggo.Config{
		Level:  loggo.LEVEL_DEBUG,
		Prefix: "fakego",
		MaxDay: 1,
	})
	fakego.OpenLog(true)
	err := fakego.Parse("./test.fk")
	if err != nil {
		loggo.Error("parse %s", err)
	}
}
