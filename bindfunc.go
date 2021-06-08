package fakego

import (
	"errors"
	"reflect"
)

type fkfunctor struct {
	argnum int
	retnum int
	args   []reflect.Kind
	rets   []reflect.Kind
}

func (ff *fkfunctor) Invoke(inter *interpreter, ps *paramstack) {

}

func Reg(name string, f interface{}) error {
	t := reflect.TypeOf(f)
	if t.Kind() != reflect.Func {
		return errors.New("not func")
	}

	log_debug("Reg %v, %v", name, t.String())

	argnum := t.NumIn()
	retnum := t.NumOut()
	var args []reflect.Kind
	var rets []reflect.Kind

	for i := 0; i < argnum; i++ {
		inV := t.In(i)
		in_Kind := inV.Kind()
		log_debug("Arg %v %v %v", i, in_Kind, inV.Name())
		args = append(args, in_Kind)
	}

	for i := 0; i < retnum; i++ {
		outV := t.Out(i)
		out_Kind := outV.Kind()
		log_debug("Ret %v %v %v", i, out_Kind, outV.Name())
		rets = append(rets, out_Kind)
	}

	var k variant
	k.from(name)

	ff := fkfunctor{
		argnum: argnum,
		retnum: retnum,
		args:   args,
		rets:   rets,
	}

	gfs.fm.add_bind_func(k, ff)

	log_debug("Reg ok %v, %v", name, t.String())

	return nil
}
