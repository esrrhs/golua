package fakego

import (
	"github.com/esrrhs/go-engine/src/loggo"
	"strconv"
	"sync/atomic"
)

func isOpenLog() bool {
	return gfs.cfg.OpenLog
}

func log_debug(format string, a ...interface{}) {
	if isOpenLog() {
		loggo.Debug(format, a...)
	}
}

func log_error(format string, a ...interface{}) {
	if isOpenLog() {
		loggo.Error(format, a...)
	}
}

func vartostring(v *variant) string {
	return v.String()
}

func vartypetostring(ty int) string {
	switch ty {
	case NIL:
		return "NIL"
	case REAL:
		return "REAL"
	case STRING:
		return "STRING"
	case POINTER:
		return "POINTER"
	case UUID:
		return "UUID"
	case ARRAY:
		return "ARRAY"
	case MAP:
		return "MAP"
	default:
		panic("vartypetostring fail " + strconv.Itoa(ty))
	}
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
		ret += vartostring(value)
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
		ret += vartostring(n)
		ret += ","
	}
	ret += "]"

	return ret
}

func fkatol(str string) int {
	ret, err := strconv.Atoi(str)
	if err != nil {
		panic(err)
	}
	return ret
}

func fkatof(str string) float64 {
	ret, err := strconv.ParseFloat(str, 64)
	if err != nil {
		panic(err)
	}
	return ret
}

func fkgen_package_name(p string, n string) string {
	if len(p) <= 0 {
		return n
	} else {
		return p + "." + n
	}
}
