package fakego

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

const (
	NIL     = 0
	REAL    = 1 // 参与计算的数值
	STRING  = 2 // 字符串
	POINTER = 3 // 指针
	UUID    = 4 // int64的uuid，不参与计算，为了效率
	ARRAY   = 5 // 数组
	MAP     = 6 // 集合
)

var nil_variant variant

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
	panic(errors.New("type error " + strconv.Itoa(v.ty)))
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
		panic(errors.New(fmt.Sprintf("variant get pointer fail, the variant is %s %s", vartypetostring(v.ty), vartostring(*v))))
	}
}

func (v *variant) V_GET_REAL() float64 {
	if v.ty == REAL {
		return v.data.(float64)
	} else if v.ty == NIL {
		return 0
	} else {
		panic(errors.New(fmt.Sprintf("variant get real fail, the variant is %s %s", vartypetostring(v.ty), vartostring(*v))))
	}
}

func (v *variant) V_GET_STRING() string {
	if v.ty == STRING {
		return v.data.(string)
	} else if v.ty == NIL {
		return ""
	} else {
		panic(errors.New(fmt.Sprintf("variant get string fail, the variant is %s %s", vartypetostring(v.ty), vartostring(*v))))
	}
}

func (v *variant) V_GET_UUID() uint64 {
	if v.ty == UUID {
		return v.data.(uint64)
	} else if v.ty == NIL {
		return 0
	} else {
		panic(errors.New(fmt.Sprintf("variant get uuid fail, the variant is %s %s", vartypetostring(v.ty), vartostring(*v))))
	}
}

func (v *variant) V_EQUAL_V(v2 variant) bool {
	if v.ty != v2.ty {
		return false
	} else {
		return v.data != v2.data
	}
}

func (v *variant) from(i interface{}) {
	switch i.(type) {
	case nil:
		v.V_SET_NIL()
	case bool:
		d := i.(bool)
		if d {
			v.V_SET_REAL(1)
		} else {
			v.V_SET_REAL(0)
		}
	case int:
		d := i.(int)
		v.V_SET_REAL(float64(d))
	case int8:
		d := i.(int8)
		v.V_SET_REAL(float64(d))
	case uint8:
		d := i.(uint8)
		v.V_SET_REAL(float64(d))
	case int16:
		d := i.(int16)
		v.V_SET_REAL(float64(d))
	case uint16:
		d := i.(uint16)
		v.V_SET_REAL(float64(d))
	case int32:
		d := i.(int32)
		v.V_SET_REAL(float64(d))
	case uint32:
		d := i.(uint32)
		v.V_SET_REAL(float64(d))
	case float32:
		d := i.(float32)
		v.V_SET_REAL(float64(d))
	case float64:
		d := i.(float64)
		v.V_SET_REAL(d)
	case int64:
		d := i.(int64)
		v.V_SET_UUID(uint64(d))
	case uint64:
		d := i.(uint64)
		v.V_SET_UUID(d)
	case string:
		d := i.(string)
		v.V_SET_STRING(d)
	case []bool:
		d := i.([]bool)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []int:
		d := i.([]int)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []int8:
		d := i.([]int8)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []uint8:
		d := i.([]uint8)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []int16:
		d := i.([]int16)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []uint16:
		d := i.([]uint16)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []int32:
		d := i.([]int32)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []uint32:
		d := i.([]uint32)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []float32:
		d := i.([]float32)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []float64:
		d := i.([]float64)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []int64:
		d := i.([]int64)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []uint64:
		d := i.([]uint64)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []string:
		d := i.([]string)
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case []interface{}:
		d := i.([]interface{})
		va := gfs.con.newarray()
		for i, _ := range d {
			var kv variant
			kv.V_SET_REAL(float64(i))
			var vv variant
			vv.from(d[i])
			va.con_array_set(kv, &vv)
		}
		v.V_SET_ARRAY(va)
	case map[interface{}]interface{}:
		d := i.(map[interface{}]interface{})
		vm := gfs.con.newmap()
		for k, v := range d {
			var kv variant
			kv.from(k)
			var vv variant
			vv.from(v)
			vm.con_map_set(kv, &vv)
		}
		v.V_SET_MAP(vm)
	default:
		v.V_SET_POINTER(i)
	}
}

func (v *variant) to() interface{} {
	switch v.ty {
	case NIL:
		return nil
	case REAL:
		d := v.data.(float64)
		if isInt(d) {
			return int(d)
		} else {
			return d
		}
	case STRING:
		return v.data.(string)
	case UUID:
		d := v.data.(uint64)
		return d
	case POINTER:
		d := v.data
		return d
	case ARRAY:
		va := v.data.(*variant_array)
		tmp := va.con_array_to()
		d := make([]interface{}, len(tmp))
		for i, _ := range tmp {
			vv := tmp[i]
			if vv != nil {
				d[i] = vv.to()
			}
		}
		return d
	case MAP:
		vm := v.data.(*variant_map)
		tmp := vm.con_map_to()
		d := make(map[interface{}]interface{})
		for kv, vv := range tmp {
			k := kv.to()
			v := vv.to()
			d[k] = v
		}
		return d
	}
	panic(errors.New("type error " + strconv.Itoa(v.ty)))
}
