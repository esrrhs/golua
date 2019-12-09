package fakego

import "sync/atomic"

func vartostring(v *variant) string {
	return v.String()
}

func fkmaptoa(vm *variant_map) string {
	return ""
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
		if n != nil {
			ret += vartostring(n)
		} else {
			ret += " "
		}
		ret += ","
	}
	ret += "]"

	return ret
}
