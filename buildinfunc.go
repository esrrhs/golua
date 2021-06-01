package fakego

import (
	"fmt"
	"strings"
)

type buildinfunc struct {
}

func buildin_print(inter *interpreter, ps *paramstack) {
	var strs []string
	for _, v := range ps.vlist {
		strs = append(strs, vartostring(v))
	}
	str := strings.Join(strs, " ")
	// printf
	if gfs.cfg.FakePrint != nil {
		gfs.cfg.FakePrint(str)
	} else {
		fmt.Printf("%s\n", str)
	}
	ps.clear()
	// ret
	ps.push(str)
}

func (bi *buildinfunc) reg(name string, bif bifunc) {
	var kv variant
	kv.V_SET_STRING(name)
	gfs.fm.add_buildin_func(kv, bif)
}

func (bi *buildinfunc) openbasefunc() {
	bi.reg("print", buildin_print)
}
