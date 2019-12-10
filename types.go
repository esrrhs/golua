package fakego

import "sync/atomic"

func vartostring(v *variant) string {
	return v.String()
}

func fkmaptoa(vm *variant_map) string {
	if vm.recurflag != 0 {
		return "MAP IN RECUR"
	}
	atomic.AddInt32(&vm.recurflag, 1)
	defer atomic.AddInt32(&vm.recurflag, -1)

	ret := ""
	ret += "{"
	i := 0
	for key, value := range vm.vm {
		if i == 0 {
			ret += "("
		} else {
			ret += ",("
		}
		ret += vartostring(&key)
		ret += ","
		ret += vartostring(&value)
		ret += ")"
		i++
	}
	ret += "}"

	return ret
}

func fkarraytoa(va *variant_array) string {
	if va.recurflag != 0 {
		return "ARRAY IN RECUR"
	}
	atomic.AddInt32(&va.recurflag, 1)
	defer atomic.AddInt32(&va.recurflag, -1)

	ret := ""
	ret += "["
	for _, n := range va.va {
		ret += vartostring(&n)
		ret += ","
	}
	ret += "]"

	return ret
}
