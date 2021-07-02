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

func Test0004(t *testing.T) {
	gfs.cfg.OpenLog = true
	vo := reflect.ValueOf(testReg)
	ret := vo.Call([]reflect.Value{reflect.ValueOf(2), reflect.ValueOf("2")})
	fmt.Printf("Invoke %v\n", ret)
}
