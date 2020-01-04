package fakego

import (
	"errors"
	"strconv"
)

type compiler struct {
	mf *myflexer
}

func (c *compiler) compile(mf *myflexer) {
	c.mf = mf

	c.compile_const_head()

	c.compile_body()
}

func (c *compiler) compile_const_head() {

	log_debug("[compiler] compile_const_head")
	mf := c.mf

	// 注册全局常量表
	evm := mf.get_const_map()
	evm.Range(func(key, value interface{}) bool {
		name := key.(string)
		ev := value.(*explicit_value_node)

		v := c.compile_explicit_value_node_to_variant(ev)

		constname := fkgen_package_name(mf.get_package(), name)
		gfs.pa.reg_const_define(constname, v, ev.lineno())
		log_debug("[compiler] reg_const_define %s %s", constname, vartostring(v))
		return true
	})
}

func (c *compiler) compile_body() error {

	return nil
}

func (c *compiler) compile_explicit_value_node_to_variant(ev *explicit_value_node) *variant {
	var v variant
	switch ev.getvaluetype() {
	case EVT_NULL:
		v.V_SET_REAL(0)
	case EVT_TRUE:
		v.V_SET_REAL(1)
	case EVT_FALSE:
		v.V_SET_REAL(0)
	case EVT_NUM:
		v.V_SET_REAL(float64(fkatol(ev.str)))
	case EVT_STR:
		v.V_SET_STRING(ev.str)
	case EVT_FLOAT:
		v.V_SET_REAL(fkatof(ev.str))
	case EVT_UUID:
		v.V_SET_UUID(uint64(fkatol(ev.str)))
	case EVT_MAP:
		{
			cml := ev.v.(*const_map_list_value_node)
			vm := gfs.con.newconstmap()
			for _, j := range cml.map_ele_node_list {
				cmv := j.(*const_map_value_node)
				kn := cmv.k.(*explicit_value_node)
				vn := cmv.v.(*explicit_value_node)
				kv := c.compile_explicit_value_node_to_variant(kn)
				vv := c.compile_explicit_value_node_to_variant(vn)
				vm.con_map_set(kv, vv)
			}
			v.V_SET_MAP(vm)
		}
	case EVT_ARRAY:
		{
			cal := ev.v.(*const_array_list_value_node)
			va := gfs.con.newconstarray()
			for i, j := range cal.array_ele_node_list {
				var kv variant
				kv.V_SET_REAL(float64(i))
				vn := j.(*explicit_value_node)
				vv := c.compile_explicit_value_node_to_variant(vn)
				va.con_array_set(&kv, vv)
			}
			v.V_SET_ARRAY(va)
		}
	default:
		panic(errors.New("compile_explicit_value_node_to_variant type error " + strconv.Itoa(ev.getvaluetype())))
	}

	return &v
}
