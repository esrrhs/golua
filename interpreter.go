package fakego

import (
	"errors"
)

func Run(fun string, p ...interface{}) (ret interface{}, err error) {
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
	inter.call(funcv, ps)
	inter.run()

	ret = ps.pop()
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
	inter.call(funcv, ps)

	dbg := &debuging{}
	dbg.debug()

	ret = ps.pop()
	return
}

type interpreter struct {
}

func (inter *interpreter) call(v variant, ps *paramstack) {

}

func (inter *interpreter) run() {

}
