package fk

import (
	"fmt"
	"os"
	"bufio"
)

func Parse(file string) {
	fmt.Println("parse " + file)

	f, err := os.OpenFile(file, os.O_RDONLY, os.ModeType)
	if err != nil {
		panic(err)
	}
	fr := bufio.NewReader(f)
	lex := NewLexer(fr)

	yyParse(lex)
}
