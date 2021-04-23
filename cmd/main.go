package main

import (
	"fmt"
	"github.com/esrrhs/fakego"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("eg: ./fakego test.fk param...")
		return
	}
	err := fakego.Parse(os.Args[1])
	if err != nil {
		fmt.Printf("Parse fail %v\n", err)
		return
	}

	var params []string
	params = append(params, os.Args[2:]...)
	ret, err := fakego.Run("mypackage.test_return_value", params)
	if err != nil {
		fmt.Printf("Run fail %v\n", err)
		return
	}
	fmt.Println(ret)
}
