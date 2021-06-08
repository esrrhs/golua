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

func (ff *fkfunctor) fromKind(i interface{}, k reflect.Kind) variant {
	var ret variant
	ret.from(i)
	return ret
}

func (ff *fkfunctor) toKind(v variant, k reflect.Kind) interface{} {
	var ret variant
	i := ret.to()
	switch k {
	case reflect.Bool:
		return i.(int) > 0
	case reflect.Int:
		return i.(int)
	case reflect.Int8:
		return i.(int8)
	case reflect.Int16:
		return i.(int16)
	case reflect.Int32:
		return i.(int32)
	case reflect.Int64:
		return i.(int64)
	case reflect.Uint:
		return i.(uint)
	case reflect.Uint8:
		return i.(uint8)
	case reflect.Uint16:
		return i.(uint16)
	case reflect.Uint32:
		return i.(uint32)
	case reflect.Uint64:
		return i.(uint64)
	case reflect.Float32:
		return i.(float32)
	case reflect.Float64:
		return i.(float64)
	default:
		return i
	}
}

func (ff *fkfunctor) Invoke(inter *interpreter, ps *paramstack) {

}

func RegFunc(name string, f interface{}) error {
	t := reflect.TypeOf(f)
	if t.Kind() != reflect.Func {
		return errors.New("not func")
	}

	log_debug("RegFunc %v, %v", name, t.String())

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
