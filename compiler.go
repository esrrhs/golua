package fakego

type compiler struct {
	mf *myflexer
}

func (c *compiler) compile(mf *myflexer) error {
	c.mf = mf
	err := c.compile_const_head()
	if err != nil {
		log_error("[compiler] compile_const_head fail")
		return err
	}

	err = c.compile_body()
	if err != nil {
		log_error("[compiler] compile_body fail")
		return err
	}

	return nil
}

func (c *compiler) compile_const_head() error {

	log_debug("[compiler] compile_const_head")
	mf := c.mf

	// 注册全局常量表
	evm := mf.get_const_map()
	evm.Range(func(key, value interface{}) bool {
		name := key.(string)
		ev := value.(*explicit_value_node)

		v, err := c.compile_explicit_value_node_to_variant(ev)
		if err != nil {
			log_error("[compiler] compile_explicit_value_node_to_variant %s %s fail", name, ev.str)
			return false
		}

		constname := fkgen_package_name(mf.get_package(), name)
		gfs.pa.reg_const_define(constname, &v, ev.lineno())
		log_debug("[compiler] reg_const_define %s %s", constname, vartostring(&v))
		return true
	})

	return nil
}

func (c *compiler) compile_body() error {

	return nil
}

func (c *compiler) compile_explicit_value_node_to_variant(node *explicit_value_node) (variant, error) {
	var ret variant
	return ret, nil
}
