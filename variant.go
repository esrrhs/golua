package fakego

import (
	"fmt"
	"github.com/esrrhs/go-engine/src/common"
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
		if common.IsInt(d) {
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
		d := v.data.(*pointerele)
		return "(" + d.ty + ")" + fmt.Sprintf("%x", d.ptr)
	case ARRAY:
		d := v.data.(*variant_array)
		return fkarraytoa(d)
	case MAP:
		d := v.data.(*variant_map)
		return fkmaptoa(d)
	}
	return "ERROR"
}
