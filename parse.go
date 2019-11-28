package fakego

import (
	"bufio"
	"os"
)

func Parse(file string) {
	Debug("start parse " + file)

	f, err := os.OpenFile(file, os.O_RDONLY, os.ModeType)
	if err != nil {
		panic(err)
	}
	fr := bufio.NewReader(f)
	lex := NewLexer(fr)

	mf := myflexer{}

	l := lexerwarpper{
		lex,
		mf,
	}

	yyParse(l)
}
