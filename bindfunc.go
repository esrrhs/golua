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
	fv     reflect.Value
}

func (ff *fkfunctor) fromKind(v reflect.Value, k reflect.Kind) interface{} {
	switch k {
	case reflect.Bool:
		return v.Bool()
	case reflect.Int:
		return float64(v.Int())
	case reflect.Int8:
		return int8(v.Int())
	case reflect.Int16:
		return int16(v.Int())
	case reflect.Int32:
		return int32(v.Int())
	case reflect.Int64:
		return int64(v.Int())
	case reflect.Uint:
		return uint(v.Int())
	case reflect.Uint8:
		return uint8(v.Int())
	case reflect.Uint16:
		return uint16(v.Int())
	case reflect.Uint32:
		return uint32(v.Int())
	case reflect.Uint64:
		return uint64(v.Int())
	case reflect.Float32:
		return float32(v.Int())
	case reflect.Float64:
		return float64(v.Int())
	case reflect.String:
		return v.String()
	default:
		return v.Pointer()
	}
}

func (ff *fkfunctor) toKind(v variant, k reflect.Kind) interface{} {
	i := v.to()
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
	case reflect.String:
		return i.(string)
	default:
		return i
	}
}

func (ff *fkfunctor) Invoke(inter *interpreter, ps *paramstack) {
	if len(ff.args) != ps.size() {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "bind func arg num not match need %v get %v", len(ff.args), ps.size())
	}
	input := make([]reflect.Value, len(ff.args))
	for index, argk := range ff.args {
		input[index] = reflect.ValueOf(ff.toKind(ps.vlist[index], argk))
	}
	output := ff.fv.Call(input)
	if len(ff.rets) != len(output) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "bind func return num not match need %v get %v", len(ff.rets), len(output))
	}
	ps.clear()
	for index, retk := range ff.rets {
		ps.push(ff.fromKind(output[index], retk))
	}
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

	fv := reflect.ValueOf(f)

	ff := fkfunctor{
		argnum: argnum,
		retnum: retnum,
		args:   args,
		rets:   rets,
		fv:     fv,
	}

	var k variant
	k.from(name)

	gfs.fm.add_bind_func(k, ff)

	log_debug("Reg ok %v, %v", name, t.String())

	return nil
}
