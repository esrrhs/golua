package fakego

import (
	"log"
	"math"
	"os"
	"strconv"
	"sync/atomic"
)

func isOpenLog() bool {
	return gfs.cfg.OpenLog
}

func log_debug(format string, a ...interface{}) {
	if isOpenLog() {
		f, err := os.OpenFile("fakego.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		log.SetOutput(f)
		log.SetPrefix("[DEBUG]:")
		log.Printf(format, a...)
	}
}

func log_error(format string, a ...interface{}) {
	if isOpenLog() {
		f, err := os.OpenFile("fakego.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		log.SetOutput(f)
		log.SetPrefix("[ERROR]:")
		log.Printf(format, a...)
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

func _MAKEINT64(high int32, low int32) int64 {
	return (int64)(((int64)(low)) | ((int64)((int32)(high)))<<32)
}
func _HIINT32(I int64) int32 {
	return (int32)(((int64)(I) >> 32) & 0xFFFFFFFF)
}
func _LOINT32(l int64) int32 {
	return (int32)(l)
}

func _MAKEINT32(high int16, low int16) int32 {
	return (int32)(((int32)(low)) | ((int32)((int16)(high)))<<16)
}
func _HIINT16(I int32) int16 {
	return (int16)(((int32)(I) >> 16) & 0xFFFF)
}
func _LOINT16(l int32) int16 {
	return (int16)(l)
}

func isInt(r float64) bool {
	return (r - math.Floor(r)) == 0
}
