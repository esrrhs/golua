package fakego

import (
	"errors"
)

func Run(fun string, p ...interface{}) (ret []interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				panic(x)
			case FakeErr:
				err = &x
			default:
				err = errors.New("unknown panic error")
			}
		}
	}()

	log_debug("start run " + fun)

	ps := &paramstack{}
	ps.pushs(p)

	var funcv variant
	funcv.V_SET_STRING(fun)

	inter := &interpreter{}
	inter.call(&funcv, ps, nil)
	inter.run()

	ret = ps.pops()
	return
}

func DebugRun(fun string, p ...interface{}) (ret interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				panic(x)
			case FakeErr:
				err = &x
			default:
				err = errors.New("unknown panic error")
			}
		}
	}()

	log_debug("start debug run " + fun)

	ps := &paramstack{}
	ps.pushs(p)

	var funcv variant
	funcv.V_SET_STRING(fun)

	inter := &interpreter{}
	inter.call(&funcv, ps, nil)

	dbg := &debuging{}
	dbg.debug()

	ret = ps.pop()
	return
}

/**************
[ret pos] .. [ret pos] [ret num] [old ip] [call time] [old fb] [old bp]
**************/
const (
	BP_SIZE = 5
)

type interpreter struct {
	stack []variant
	ip    int
	bp    int
	sp    int
	fb    *func_binary
	ret   []variant
	isend bool
}

func (inter *interpreter) call(fun *variant, ps *paramstack, retpos []int) {
	f := gfs.fm.get_func(fun)
	if f == nil {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "run no func %s fail", vartostring(fun))
	}

	retnum := len(retpos)

	if f.havefb {

		fb := f.fb

		// 准备栈大小
		needsize := inter.sp + BP_SIZE + retnum + fb.maxstack
		if needsize >= len(inter.stack) {
			oldsize := len(inter.stack)
			inter.stack = append(inter.stack, make([]variant, needsize-oldsize)...)
		}

		// 老的bp
		oldbp := inter.bp
		inter.bp = inter.sp

		// 记录返回位置
		for i := 0; i < retnum; i++ {
			inter.stack[inter.bp].ty = NIL
			inter.stack[inter.bp].data = retpos[i]
			inter.bp++
		}

		// 记录老的ip
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = inter.ip
		inter.bp++

		// 记录profile
		if gfs.cfg.OpenProfile {
			inter.stack[inter.bp].data = fkgetmstick()
		}
		inter.stack[inter.bp].ty = NIL
		inter.bp++

		// 记录老的fb
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = inter.fb
		inter.bp++

		// 记录老的bp
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = oldbp
		inter.bp++

		// 设置sp
		inter.sp = inter.bp + fb.maxstack

		if ps.size() != fb.paramnum {
			seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "call func %s param not match", vartostring(fun))
		}

		// 分配入参
		copy(inter.stack[inter.bp:], ps.vlist)
		ps.clear()

		// 清空栈区
		for i := 0; i < fb.maxstack-fb.paramnum; i++ {
			inter.stack[inter.bp+fb.paramnum+i].V_SET_NIL()
		}

		// 重置ret
		if len(inter.ret) <= 0 {
			inter.ret = make([]variant, 1)
		}
		for i, _ := range inter.ret {
			inter.ret[i].V_SET_NIL()
		}

		// 新函数
		inter.fb = fb
		inter.ip = 0

	} else if f.havebif {
		// TODO
	} else if f.haveff {
		// TODO
	} else {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "run no func %s fail", vartostring(fun))
	}
}

func (inter *interpreter) run() {

	// 栈溢出检查
	if len(inter.stack) > gfs.cfg.StackMax {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "stack too big %d", len(inter.stack))
	}

	if inter.isend {
		return
	}

	for {
		// 当前函数走完
		if inter.ip >= inter.fb.binary_size() {
			// TODO
		}
	}
}

func (inter *interpreter) getcurfile() string {
	return "TODO"
}

func (inter *interpreter) getcurline() int {
	return 0
}

func (inter *interpreter) getcurfunc() string {
	return "TODO"
}
