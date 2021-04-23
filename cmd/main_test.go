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

func Test_return_value1(t *testing.T) {
	load(t, "./test/test_return_value.fk")
	ret, err := fakego.Run("mypackage.test_return_value1", 1, "2")
	if err != nil {
		panic(err)
	}
	if len(ret) != 1 {
		t.Fatalf("fail")
	}
	if ret[0] != 1 {
		t.Fatalf("fail")
	}
}

func Test_return_value2(t *testing.T) {
	load(t, "./test/test_return_value.fk")
	ret, err := fakego.Run("mypackage.test_return_value2", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value1", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value1", 2, "2")
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
}

func Test_if_value2(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	{
		ret, err := fakego.Run("mypackage.test_if_value2", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value2", 2, "2")
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
}

func Test_if_value3(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	{
		ret, err := fakego.Run("mypackage.test_if_value3", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value3", 0, "2")
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
}

func Test_if_value4(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	{
		ret, err := fakego.Run("mypackage.test_if_value4", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value4", 0, "2")
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
}

func Test_if_value5(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	{
		ret, err := fakego.Run("mypackage.test_if_value5", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value5", 2, "2")
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
}

func Test_if_value6(t *testing.T) {
	load(t, "./test/test_if_value.fk")
	{
		ret, err := fakego.Run("mypackage.test_if_value6", 1, "2")
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
	{
		ret, err := fakego.Run("mypackage.test_if_value6", 2, "2")
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
}
