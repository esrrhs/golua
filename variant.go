package fakego

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

const (
	NIL     = iota
	REAL    // 参与计算的数值
	STRING  // 字符串
	POINTER // 指针
	UUID    // int64的uuid，不参与计算，为了效率
	ARRAY   // 数组
	MAP     // 集合
)

type variant struct {
	ty   int
	data interface{}
}

func (v *variant) String() string {
	switch v.ty {
	case NIL:
		return "nil"
	case REAL:
		d := v.data.(float64)
		if isInt(d) {
			return strconv.FormatInt(int64(d), 10)
		} else {
			return strconv.FormatFloat(d, 'E', -1, 64)
		}
	case STRING:
		return v.data.(string)
	case UUID:
		d := v.data.(uint64)
		return strconv.FormatUint(d, 10)
	case POINTER:
		d := v.data
		return "(" + reflect.TypeOf(d).String() + ")" + fmt.Sprintf("%x", d)
	case ARRAY:
		d := v.data.(*variant_array)
		return fkarraytoa(d)
	case MAP:
		d := v.data.(*variant_map)
		return fkmaptoa(d)
	}
	return "ERROR"
}
func (v *variant) V_SET_NIL() {
	v.ty = NIL
	v.data = nil
}
func (v *variant) V_SET_POINTER(p interface{}) {
	v.ty = POINTER
	v.data = p
}
func (v *variant) V_SET_REAL(r float64) {
	v.ty = REAL
	v.data = r
}
func (v *variant) V_SET_STRING(s string) {
	v.ty = STRING
	v.data = s
}
func (v *variant) V_SET_UUID(id uint64) {
	v.ty = UUID
	v.data = id
}
func (v *variant) V_SET_ARRAY(a *variant_array) {
	v.ty = ARRAY
	v.data = a
}
func (v *variant) V_SET_MAP(m *variant_map) {
	v.ty = MAP
	v.data = m
}

func (v *variant) V_GET_POINTER() interface{} {
	if v.ty == POINTER {
		return v.data
	} else if v.ty == NIL {
		return nil
	} else {
		panic(errors.New(fmt.Sprintf("variant get pointer fail, the variant is %s %s", vartypetostring(v.ty), vartostring(v))))
	}
}

func (v *variant) V_GET_REAL() float64 {
	if v.ty == REAL {
		return v.data.(float64)
	} else if v.ty == NIL {
		return 0
	} else {
		panic(errors.New(fmt.Sprintf("variant get real fail, the variant is %s %s", vartypetostring(v.ty), vartostring(v))))
	}
}

func (v *variant) V_GET_STRING() string {
	if v.ty == STRING {
		return v.data.(string)
	} else if v.ty == NIL {
		return ""
	} else {
		panic(errors.New(fmt.Sprintf("variant get string fail, the variant is %s %s", vartypetostring(v.ty), vartostring(v))))
	}
}

func (v *variant) V_GET_UUID() uint64 {
	if v.ty == UUID {
		return v.data.(uint64)
	} else if v.ty == NIL {
		return 0
	} else {
		panic(errors.New(fmt.Sprintf("variant get uuid fail, the variant is %s %s", vartypetostring(v.ty), vartostring(v))))
	}
}
