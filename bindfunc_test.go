package fakego

import (
	"fmt"
	"testing"
)

func testReg(a int, b string) int {
	fmt.Println(a, b)
	return a + 1
}

func Test0003(t *testing.T) {
	gfs.cfg.OpenLog = true
	Reg("testReg", testReg)
}
