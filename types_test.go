package fakego

import (
	"fmt"
	"testing"
)

func Test0002(t *testing.T) {
	fmt.Println(fkxtoa(124, 16))
	fmt.Println(fkxtoa(0x2E3D7F3F9, 16))
}
