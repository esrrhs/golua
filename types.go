package fakego

import (
	"github.com/esrrhs/go-engine/src/loggo"
	"sync/atomic"
)

var gOpenLog bool

func OpenLog(b bool) {
	gOpenLog = b
}

func isOpenLog() bool {
	return gOpenLog
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

func fkgen_package_name(p string, n string) string {
	if len(p) <= 0 {
		return n
	} else {
		return p + "." + n
	}
}
