package fakego

import (
	"fmt"
	"reflect"
	"testing"
)

func testReg(a int, b string) int {
	fmt.Println(a, b)
	return a + 1
}

func Test0003(t *testing.T) {
	gfs.cfg.OpenLog = true
	RegFunc("testReg", testReg)
}

type TestStruct struct {
	a int
}

func (t *TestStruct) testReg(a int) int {
	return a - t.a
}

func Test0004(t *testing.T) {
	vo := reflect.ValueOf(testReg)
	ret := vo.Call([]reflect.Value{reflect.ValueOf(2), reflect.ValueOf("2")})
	fmt.Printf("Invoke %v\n", ret[0].Int())
}

func Test0005(t *testing.T) {
	ts := &TestStruct{10}
	vo := reflect.ValueOf(ts.testReg)
	ret := vo.Call([]reflect.Value{reflect.ValueOf(2)})
	fmt.Printf("Invoke %v\n", ret[0].Int())
}
