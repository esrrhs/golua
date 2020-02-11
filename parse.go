package fakego

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
)

type parser struct {
	constinfo sync.Map // string->*const_parser_info
}

type parseContent struct {
	includemap  map[string]int
	includelist []string
}

func Parse(file string) error {
	ctx := &parseContent{}
	ctx.includemap = make(map[string]int)
	return gfs.pa.parse(ctx, file)
}

func (pa *parser) parse(ctx *parseContent, file string) (err error) {
	var ll *lexerwarpper
	defer func() {
		if r := recover(); r != nil {
			line := 0
			if ll != nil {
				line = ll.yyLexer.(*Lexer).Line()
			}
			switch x := r.(type) {
			case string:
				err = errors.New(file + ":" + strconv.Itoa(line) + ":" + x)
			case error:
				panic(x)
			case FakeErr:
				err = &x
			default:
				err = errors.New("unknown panic error")
			}
		}
	}()

	log_debug("start parse " + file)

	for _, s := range ctx.includelist {
		if s == file {
			seterror(file, 0, "", "include loop "+strings.Join(ctx.includelist, ",")+file)
		}
	}
	ctx.includelist = append(ctx.includelist, file)

	if ctx.includemap[file] > 0 {
		return nil
	}
	ctx.includemap[file] = 1

	file = strings.TrimSuffix(file, "\n")
	file = strings.TrimSuffix(file, "\r")
	f, err := os.OpenFile(file, os.O_RDONLY, os.ModeType)
	if err != nil {
		seterror(file, 0, "", err.Error())
	}
	fr := bufio.NewReader(f)
	lex := NewLexer(fr)

	mf := myflexer{}
	mf.fileName = file

	l := lexerwarpper{
		lex,
		&mf,
	}
	ll = &l

	ret := yyParse(l)
	if ret != 0 {
		seterror(file, 0, "", "yyParse fail "+strconv.Itoa(ret))
	}

	log_debug("yyParse ok" + file)
	f.Close()

	// parse include file
	for _, f := range mf.includelist {
		err := pa.parse(ctx, f)
		if err != nil {
			return err
		}
	}

	log_debug("include Parse ok" + file)

	mc := compiler{}
	mc.compile(&mf)

	log_debug("compile ok" + file)

	ctx.includelist = ctx.includelist[0 : len(ctx.includelist)-1]

	log_debug("start parse ok" + file)

	return nil
}

type const_parser_info struct {
	v      *variant
	lineno int
}

func (pa *parser) reg_const_define(constname string, v *variant, lineno int) {
	pa.constinfo.Store(constname, &const_parser_info{v: v, lineno: lineno})
}

func (pa *parser) get_const_define(constname string) (*variant, int) {
	v, ok := pa.constinfo.Load(constname)
	if ok {
		ci := v.(*const_parser_info)
		return ci.v, ci.lineno
	}
	return nil, 0
}
