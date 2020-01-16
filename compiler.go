package fakego

import (
	"fmt"
	"strconv"
)

type beak_pos_list []int
type loop_break_pos_stack []beak_pos_list
type continue_pos_stack []int
type continue_end_pos_list []int
type continue_end_pos_stack []continue_end_pos_list

type compiler struct {
	mf                      *myflexer
	cur_compile_func        string
	loop_break_pos_stack    loop_break_pos_stack
	loop_continue_pos_stack continue_pos_stack
	continue_end_pos_stack  continue_end_pos_stack
	cmp_jne                 bool
	cur_addr                command
	cur_addrs               []command
	new_var                 bool
	cmp_deps                int
	func_ret_num            int
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

	mf := c.mf
	funclist := mf.get_func_list()
	log_debug("[compiler] compile_body funclist %d", len(funclist))

	for _, funcnode := range funclist {
		c.compile_func(funcnode)
	}

	if isOpenLog() {
		log_debug("[compiler] compile_body funclist %d ok dump \n%s", len(funclist), gfs.bin.dump())
		log_debug("[compiler] compile_body funcmap %d ok dump \n%s", gfs.fm.size(), gfs.fm.dump())
	}

	return nil
}

func (c *compiler) compile_func(funcnode *func_desc_node) {
	log_debug("[compiler] compile_func func %s", funcnode.funcname)

	mf := c.mf
	c.cur_compile_func = funcnode.funcname

	// 不检测重名，直接替换掉老的
	var cg codegen
	var bin func_binary

	// 压栈
	cg.push_stack_identifiers()

	// 参数入栈
	if funcnode.arglist != nil {
		arglist := funcnode.arglist.arglist
		for _, arg := range arglist {
			if cg.add_stack_identifier(arg, funcnode.arglist.lineno()) == -1 {
				c.compile_seterror(funcnode.arglist, "double %s identifier error", arg)
			}
		}
		bin.paramnum = len(arglist)
	}

	// 编译函数体
	if funcnode.block != nil {
		c.compile_block(&cg, funcnode.block)
	}

	// 编译成功
	funcname := fkgen_package_name(mf.get_package(), funcnode.funcname)

	log_debug("[compiler] compile_func func %s size = %d OK", funcname, bin.binary_size())

}

func (c *compiler) compile_block(cg *codegen, blocknode *block_node) {

	log_debug("[compiler] compile_block block_node %p", blocknode)
	for _, stmt := range blocknode.stmtlist {
		c.compile_node(cg, stmt)
	}
	log_debug("[compiler] compile_block block_node %p OK", blocknode)
}

func (c *compiler) compile_node(cg *codegen, node syntree_node) {

	log_debug("[compiler] compile_node %p %s", node, get_syntree_typename(node))
	ty := node.gettype()
	switch ty {
	case est_block:
		bn := node.(*block_node)
		c.compile_block(cg, bn)
	case est_while_stmt:
		ws := node.(*while_stmt)
		c.compile_while_stmt(cg, ws)
	case est_for_stmt:
		fs := node.(*for_stmt)
		c.compile_for_stmt(cg, fs)
	case est_for_loop_stmt:
		fs := node.(*for_loop_stmt)
		c.compile_for_loop_stmt(cg, fs)
	case est_multi_assign_stmt:
		as := node.(*multi_assign_stmt)
		c.compile_multi_assign_stmt(cg, as)
	case est_cmp_stmt:
		cs := node.(*cmp_stmt)
		c.compile_cmp_stmt(cg, cs)
	case est_if_stmt:
		is := node.(*if_stmt)
		c.compile_if_stmt(cg, is)
	case est_explicit_value:
		ev := node.(*explicit_value_node)
		c.compile_explicit_value(cg, ev)
	case est_return_stmt:
		rs := node.(*return_stmt)
		c.compile_return_stmt(cg, rs)
	case est_return_value_list:
		rn := node.(*return_value_list_node)
		c.compile_return_value_list(cg, rn)
	case est_assign_stmt:
		as := node.(*assign_stmt)
		c.compile_assign_stmt(cg, as)
	case est_math_assign_stmt:
		ms := node.(*math_assign_stmt)
		c.compile_math_assign_stmt(cg, ms)
	case est_variable:
		vn := node.(*variable_node)
		c.compile_variable_node(cg, vn)
	case est_var:
		vn := node.(*var_node)
		c.compile_var_node(cg, vn)
	case est_function_call:
		fn := node.(*function_call_node)
		c.compile_function_call_node(cg, fn)
	case est_break:
		bs := node.(*break_stmt)
		c.compile_break_stmt(cg, bs)
	case est_continue:
		cs := node.(*continue_stmt)
		c.compile_continue_stmt(cg, cs)
	case est_math_expr:
		mn := node.(*math_expr_node)
		c.compile_math_expr_node(cg, mn)
	case est_container_get:
		cn := node.(*container_get_node)
		c.compile_container_get(cg, cn)
	case est_struct_pointer:
		cn := node.(*struct_pointer_node)
		c.compile_struct_pointer(cg, cn)
	case est_sleep:
		ss := node.(*sleep_stmt)
		c.compile_sleep_stmt(cg, ss)
	case est_yield:
		ys := node.(*yield_stmt)
		c.compile_yield_stmt(cg, ys)
	case est_switch_stmt:
		ss := node.(*switch_stmt)
		c.compile_switch_stmt(cg, ss)
	default:
		c.compile_seterror(node, "compile node type error %d", ty)
	}

	log_debug("[compiler] compile_node %p %s OK", node, get_syntree_typename(node))
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
		c.compile_seterror(ev, "compile_explicit_value_node_to_variant type error "+strconv.Itoa(ev.getvaluetype()))
	}

	return &v
}

func (c *compiler) compile_seterror(node syntree_node, format string, a ...interface{}) {
	str := fmt.Sprintf(format, a...)
	seterror(c.mf.getfilename(), node.lineno(), c.cur_compile_func, "compile func(%s), %s", c.cur_compile_func, str)
}

func (c *compiler) compile_while_stmt(cg *codegen, ws *while_stmt) {

	log_debug("[compiler] compile_while_stmt %p", ws)

	startpos := 0
	jnepos := 0
	c.loop_break_pos_stack = append(c.loop_break_pos_stack, beak_pos_list{})
	startpos = cg.byte_code_size()
	c.loop_continue_pos_stack = append(c.loop_continue_pos_stack, startpos)

	// 条件
	cg.push_stack_identifiers()
	c.compile_node(cg, ws.cmp)
	cg.pop_stack_identifiers()

	// cmp与jne结合
	if c.cmp_jne {
		cg.push(EMPTY_CMD, ws.cmp.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	} else {
		cg.push(_MAKE_OPCODE(OPCODE_JNE), ws.lineno())
		cg.push(c.cur_addr, ws.lineno())
		cg.push(EMPTY_CMD, ws.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	}
	c.cmp_deps = 0
	c.cmp_jne = false

	// block块
	if ws.block != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, ws.block)
		cg.pop_stack_identifiers()
	}

	// 跳回判断地方
	cg.push(_MAKE_OPCODE(OPCODE_JMP), ws.lineno())
	cg.push(_MAKE_POS(startpos), ws.lineno())

	// 跳转出block块
	cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))

	// 替换掉break
	bplist := c.loop_break_pos_stack[len(c.loop_break_pos_stack)-1]
	for i, _ := range bplist {
		cg.set(bplist[i], _MAKE_POS(cg.byte_code_size()))
	}
	c.loop_break_pos_stack = c.loop_break_pos_stack[0 : len(c.loop_break_pos_stack)-1]
	c.loop_continue_pos_stack = c.loop_continue_pos_stack[0 : len(c.loop_continue_pos_stack)-1]

	log_debug("[compiler] compile_while_stmt %p OK", ws)
}

func (c *compiler) compile_for_stmt(cg *codegen, fs *for_stmt) {

	log_debug("[compiler] compile_for_stmt %p", fs)

	startpos := 0
	jnepos := 0
	continuepos := 0

	c.loop_break_pos_stack = append(c.loop_break_pos_stack, beak_pos_list{})
	c.continue_end_pos_stack = append(c.continue_end_pos_stack, continue_end_pos_list{})

	// 开始语句，这个作用域是全for都有效的
	cg.push_stack_identifiers()
	if fs.beginblock != nil {
		c.compile_node(cg, fs.beginblock)
	}

	startpos = cg.byte_code_size()

	// 需要continue end
	c.loop_continue_pos_stack = append(c.loop_continue_pos_stack, -1)

	// 条件
	cg.push_stack_identifiers()
	c.compile_node(cg, fs.cmp)
	cg.pop_stack_identifiers()

	// cmp与jne结合
	if c.cmp_jne {
		cg.push(EMPTY_CMD, fs.cmp.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	} else {
		cg.push(_MAKE_OPCODE(OPCODE_JNE), fs.lineno())
		cg.push(c.cur_addr, fs.lineno())
		cg.push(EMPTY_CMD, fs.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	}
	c.cmp_deps = 0
	c.cmp_jne = false

	// block块
	if fs.block != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, fs.block)
		cg.pop_stack_identifiers()
	}

	continuepos = cg.byte_code_size()

	// 结束
	if fs.endblock != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, fs.endblock)
		cg.pop_stack_identifiers()
	}

	// 跳回判断地方
	cg.push(_MAKE_OPCODE(OPCODE_JMP), fs.lineno())
	cg.push(_MAKE_POS(startpos), fs.lineno())

	// 跳转出block块
	cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))

	// 替换掉break
	bplist := c.loop_break_pos_stack[len(c.loop_break_pos_stack)-1]
	for i, _ := range bplist {
		cg.set(bplist[i], _MAKE_POS(cg.byte_code_size()))
	}
	c.loop_break_pos_stack = c.loop_break_pos_stack[0 : len(c.loop_break_pos_stack)-1]

	// 替换掉continue
	cplist := c.continue_end_pos_stack[len(c.continue_end_pos_stack)-1]
	for i, _ := range cplist {
		cg.set(cplist[i], _MAKE_POS(continuepos))
	}
	c.continue_end_pos_stack = c.continue_end_pos_stack[0 : len(c.continue_end_pos_stack)-1]

	c.loop_continue_pos_stack = c.loop_continue_pos_stack[0 : len(c.loop_continue_pos_stack)-1]

	// 离开作用域
	cg.pop_stack_identifiers()

	log_debug("[compiler] compile_for_stmt %p OK", fs)
}

func (c *compiler) compile_for_loop_stmt(cg *codegen, fs *for_loop_stmt) {

	log_debug("[compiler] compile_for_loop_stmt %p", fs)

	startpos := 0
	jnepos := 0
	continuepos := 0

	c.loop_break_pos_stack = append(c.loop_break_pos_stack, beak_pos_list{})
	c.continue_end_pos_stack = append(c.continue_end_pos_stack, continue_end_pos_list{})

	// 开始语句，这个作用域是全for都有效的
	cg.push_stack_identifiers()

	var iter command
	c.compile_node(cg, fs.iter)
	iter = c.cur_addr
	log_debug("[compiler] compile_for_loop_stmt iter = %d", c.cur_addr)

	var begin command
	c.compile_node(cg, fs.begin)
	begin = c.cur_addr
	log_debug("[compiler] compile_for_loop_stmt begin = %d", c.cur_addr)

	var end command
	c.compile_node(cg, fs.end)
	end = c.cur_addr
	log_debug("[compiler] compile_for_loop_stmt end = %d", c.cur_addr)

	var step command
	c.compile_node(cg, fs.step)
	step = c.cur_addr
	log_debug("[compiler] compile_for_loop_stmt step = %d", c.cur_addr)

	cg.push(_MAKE_OPCODE(OPCODE_ASSIGN), fs.lineno())
	cg.push(iter, fs.lineno())
	cg.push(begin, fs.lineno())

	cg.push(_MAKE_OPCODE(OPCODE_LESS_JNE), fs.lineno())
	cg.push(iter, fs.lineno())
	cg.push(end, fs.lineno())
	tmpdespos := cg.alloc_stack_identifier()
	tmpdest := _MAKE_ADDR(ADDR_STACK, tmpdespos)
	cg.push(tmpdest, fs.lineno())
	cg.push(EMPTY_CMD, fs.lineno()) // 先塞个位置
	jnepos = cg.byte_code_size() - 1

	startpos = cg.byte_code_size()

	// 需要continue end
	c.loop_continue_pos_stack = append(c.loop_continue_pos_stack, -1)

	// block块
	if fs.block != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, fs.block)
		cg.pop_stack_identifiers()
	}

	continuepos = cg.byte_code_size()

	cg.push(_MAKE_OPCODE(OPCODE_FOR), fs.lineno())
	cg.push(iter, fs.lineno())
	cg.push(end, fs.lineno())
	cg.push(step, fs.lineno())
	tmpdespos = cg.alloc_stack_identifier()
	tmpdest = _MAKE_ADDR(ADDR_STACK, tmpdespos)
	cg.push(tmpdest, fs.lineno())
	cg.push(_MAKE_POS(startpos), fs.lineno())

	// 跳转出block块
	cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))

	// 替换掉break
	bplist := c.loop_break_pos_stack[len(c.loop_break_pos_stack)-1]
	for i, _ := range bplist {
		cg.set(bplist[i], _MAKE_POS(cg.byte_code_size()))
	}
	c.loop_break_pos_stack = c.loop_break_pos_stack[0 : len(c.loop_break_pos_stack)-1]

	// 替换掉continue
	cplist := c.continue_end_pos_stack[len(c.continue_end_pos_stack)-1]
	for i, _ := range cplist {
		cg.set(cplist[i], _MAKE_POS(continuepos))
	}
	c.continue_end_pos_stack = c.continue_end_pos_stack[0 : len(c.continue_end_pos_stack)-1]

	c.loop_continue_pos_stack = c.loop_continue_pos_stack[0 : len(c.loop_continue_pos_stack)-1]

	// 离开作用域
	cg.pop_stack_identifiers()

	log_debug("[compiler] compile_for_loop_stmt %p OK", fs)
}

func (c *compiler) compile_multi_assign_stmt(cg *codegen, as *multi_assign_stmt) {

	log_debug("[compiler] compile_multi_assign_stmt %p", as)

	// 目前多重赋值只支持a,b,c = myfunc1()，需要告诉func1多返回几个值
	c.func_ret_num = len(as.varlist.varlist)

	// 编译value
	c.compile_node(cg, as.value)

	// 挨个编译var
	var varlist []command
	for i, _ := range as.varlist.varlist {
		c.new_var = as.isnew
		c.compile_node(cg, as.varlist.varlist[i])
		c.new_var = false
		varlist = append(varlist, c.cur_addr)
	}

	// 挨个赋值
	for i, _ := range as.varlist.varlist {
		var varc command
		var value command

		varc = varlist[i]
		value = c.cur_addrs[i]

		cg.push(_MAKE_OPCODE(OPCODE_ASSIGN), as.lineno())
		cg.push(varc, as.lineno())
		cg.push(value, as.lineno())
	}

	log_debug("[compiler] compile_multi_assign_stmt %p OK", as)
}

func (c *compiler) compile_cmp_stmt(cg *codegen, cs *cmp_stmt) {

	log_debug("[compiler] compile_cmp_stmt %p", cs)

	deps := c.cmp_deps
	c.cmp_deps++

	var oper command
	var left command
	var right command
	var dest command

	if cs.cmp != "not" {
		// oper
		if cs.cmp == "&&" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_AND_JNE), _MAKE_OPCODE(OPCODE_AND))
		} else if cs.cmp == "||" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_OR_JNE), _MAKE_OPCODE(OPCODE_OR))
		} else if cs.cmp == "<" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_LESS_JNE), _MAKE_OPCODE(OPCODE_LESS))
		} else if cs.cmp == ">" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_MORE_JNE), _MAKE_OPCODE(OPCODE_MORE))
		} else if cs.cmp == "==" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_EQUAL_JNE), _MAKE_OPCODE(OPCODE_EQUAL))
		} else if cs.cmp == ">=" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_MOREEQUAL_JNE), _MAKE_OPCODE(OPCODE_MOREEQUAL))
		} else if cs.cmp == "<=" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_LESSEQUAL_JNE), _MAKE_OPCODE(OPCODE_LESSEQUAL))
		} else if cs.cmp == "!=" {
			oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_NOTEQUAL_JNE), _MAKE_OPCODE(OPCODE_NOTEQUAL))
		} else if cs.cmp == "true" {
			var v variant
			v.V_SET_REAL(1)
			pos := cg.getconst(&v)
			c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)

			c.cmp_deps--
			c.cmp_jne = false
		} else if cs.cmp == "false" {
			var v variant
			v.V_SET_REAL(0)
			pos := cg.getconst(&v)
			c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)

			c.cmp_deps--
			c.cmp_jne = false
		} else if cs.cmp == "is" {
			// left
			c.compile_node(cg, cs.left)

			c.cmp_deps--
			c.cmp_jne = false
		} else {
			c.compile_seterror(cs, "cmp error %s", cs.cmp)
		}

		// left
		c.compile_node(cg, cs.left)
		left = c.cur_addr

		// right
		c.compile_node(cg, cs.right)
		right = c.cur_addr

		// result
		despos := cg.alloc_stack_identifier()
		dest = _MAKE_ADDR(ADDR_STACK, despos)
		c.cur_addr = dest

		cg.push(oper, cs.lineno())
		cg.push(left, cs.lineno())
		cg.push(right, cs.lineno())
		cg.push(dest, cs.lineno())
	} else { /* "not" */
		oper = command_ternarytesting(deps != 0, _MAKE_OPCODE(OPCODE_NOT_JNE), _MAKE_OPCODE(OPCODE_NOT))

		// left
		c.compile_node(cg, cs.left)
		left = c.cur_addr
		despos := cg.alloc_stack_identifier()
		dest = _MAKE_ADDR(ADDR_STACK, despos)
		c.cur_addr = dest
		cg.push(oper, cs.lineno())
		cg.push(left, cs.lineno())
		cg.push(dest, cs.lineno())
	}

	c.cmp_deps--
	if deps != 0 {
		c.cmp_jne = true
	}

	log_debug("[compiler] compile_cmp_stmt %p OK", cs)
}

func (c *compiler) compile_if_stmt(cg *codegen, stmt *if_stmt) {

}

func (c *compiler) compile_explicit_value(cg *codegen, node *explicit_value_node) {

}

func (c *compiler) compile_return_stmt(cg *codegen, stmt *return_stmt) {

}

func (c *compiler) compile_return_value_list(cg *codegen, node *return_value_list_node) {

}

func (c *compiler) compile_assign_stmt(cg *codegen, stmt *assign_stmt) {

}

func (c *compiler) compile_math_assign_stmt(cg *codegen, stmt *math_assign_stmt) {

}

func (c *compiler) compile_variable_node(cg *codegen, node *variable_node) {

}

func (c *compiler) compile_var_node(cg *codegen, node *var_node) {

}

func (c *compiler) compile_function_call_node(cg *codegen, node *function_call_node) {

}

func (c *compiler) compile_break_stmt(cg *codegen, stmt *break_stmt) {

}

func (c *compiler) compile_continue_stmt(cg *codegen, stmt *continue_stmt) {

}

func (c *compiler) compile_math_expr_node(cg *codegen, node *math_expr_node) {

}

func (c *compiler) compile_container_get(cg *codegen, node *container_get_node) {

}

func (c *compiler) compile_struct_pointer(cg *codegen, node *struct_pointer_node) {

}

func (c *compiler) compile_sleep_stmt(cg *codegen, stmt *sleep_stmt) {

}

func (c *compiler) compile_yield_stmt(cg *codegen, stmt *yield_stmt) {

}

func (c *compiler) compile_switch_stmt(cg *codegen, stmt *switch_stmt) {

}
