package main

import (
	"github.com/esrrhs/fakego"
	"testing"
)

func load(t *testing.T, file string) {
	fakego.SetConfig(fakego.FakeConfig{OpenLog: true})
	err := fakego.Parse(file)
	if err != nil {
		t.Fatalf("load fail %v", file)
	}
}

func Test_return_value(t *testing.T) {
	load(t, "./test/test_return_value.fk")
	ret, err := fakego.Run("mypackage.test_return_value", 1, "2")
	if err != nil {
		panic(err)
	}
	if len(ret) != 2 {
		t.Fatalf("fail")
	}
	if ret[0] != 1 {
		t.Fatalf("fail")
	}
	if ret[1] != "a" {
		t.Fatalf("fail")
	}
}

func Test_if_value1(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	ret, err := fakego.Run("mypackage.test_if_value", 1, "2")
	if err != nil {
		panic(err)
	}
	if len(ret) != 1 {
		t.Fatalf("fail")
	}
	if ret[0] != "yes" {
		t.Fatalf("fail")
	}
}

func Test_if_value2(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	ret, err := fakego.Run("mypackage.test_if_value", 2, "2")
	if err != nil {
		panic(err)
	}
	if len(ret) != 1 {
		t.Fatalf("fail")
	}
	if ret[0] != "no" {
		t.Fatalf("fail")
	}
}
