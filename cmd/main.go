package main

import (
	"fmt"
	"os"

	"github.com/esrrhs/fakescript_go"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: fakescript_go <file.fk> [function_name] [args...]")
		fmt.Println("  example: fakescript_go test.fk main 1 2")
		return
	}

	file := os.Args[1]
	err := fakescript_go.Parse(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse failed: %v\n", err)
		os.Exit(1)
	}

	funcName := "main"
	argStartIndex := 2
	if len(os.Args) > 2 {
		funcName = os.Args[2]
		argStartIndex = 3
	}

	var params []interface{}
	for _, arg := range os.Args[argStartIndex:] {
		params = append(params, arg)
	}

	ret, err := fakescript_go.Run(funcName, params...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Run failed: %v\n", err)
		os.Exit(1)
	}
	if len(ret) > 0 {
		fmt.Println(ret)
	}
}
