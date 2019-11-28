package fake

import "fmt"

func Debug(format string, param ...interface{}) {
	fmt.Printf(format, param...)
	fmt.Println()
}
