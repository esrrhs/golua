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

func Test_for1(t *testing.T) {
	load(t, "./test/test_for.fk")
	{
		ret, err := fakego.Run("mypackage.test_for1", 1, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 9 {
			t.Fatalf("fail")
		}
	}
}

func Test_for2(t *testing.T) {
	load(t, "./test/test_for.fk")
	{
		ret, err := fakego.Run("mypackage.test_for2", 1, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 45 {
			t.Fatalf("fail")
		}
	}
}

func Test_for3(t *testing.T) {
	load(t, "./test/test_for.fk")
	{
		ret, err := fakego.Run("mypackage.test_for3", 1, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 9 {
			t.Fatalf("fail")
		}
	}
}

func Test_for4(t *testing.T) {
	load(t, "./test/test_for.fk")
	{
		ret, err := fakego.Run("mypackage.test_for4", 1, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 45 {
			t.Fatalf("fail")
		}
	}
}

func Test_switch1(t *testing.T) {
	load(t, "./test/test_switch.fk")
	{
		ret, err := fakego.Run("mypackage.test_switch1", 1, 10)
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
	{
		ret, err := fakego.Run("mypackage.test_switch1", "a", 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 10 {
			t.Fatalf("fail")
		}
	}
	{
		ret, err := fakego.Run("mypackage.test_switch1", 2, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != 12 {
			t.Fatalf("fail")
		}
	}
}

func Test_funccall1(t *testing.T) {
	load(t, "./test/test_funccall.fk")
	{
		ret, err := fakego.Run("mypackage.test_funccall1", 2, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != -16 {
			t.Fatalf("fail")
		}
	}
}

func Test_print1(t *testing.T) {
	load(t, "./test/test_print.fk")
	{
		ret, err := fakego.Run("mypackage.test_print1", 2, 10)
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != "2 10" {
			t.Fatalf("fail")
		}
	}
	{
		ret, err := fakego.Run("mypackage.test_print1", "a", "b")
		if err != nil {
			panic(err)
		}
		if len(ret) != 1 {
			t.Fatalf("fail")
		}
		if ret[0] != "a b" {
			t.Fatalf("fail")
		}
	}
}
