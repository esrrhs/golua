package fakego

import (
	"fmt"
	"strconv"
	"strings"
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

	// break必须为空
	if len(c.loop_break_pos_stack) > 0 {
		c.compile_seterror(funcnode, "compile extra break error")
	}

	// 编译成功
	funcname := fkgen_package_name(mf.get_package(), funcnode.funcname)
	cg.output(mf.getfilename(), mf.get_package(), funcname, &bin)

	var fv variant
	fv.V_SET_STRING(funcname)
	gfs.bin.add_func(fv, &bin)

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
	case est_switch_stmt:
		ss := node.(*switch_stmt)
		c.compile_switch_stmt(cg, ss)
	default:
		c.compile_seterror(node, "compile node type error %d", ty)
	}

	log_debug("[compiler] compile_node %p %s OK", node, get_syntree_typename(node))
}

func (c *compiler) compile_explicit_value_node_to_variant(ev *explicit_value_node) variant {
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
				vm.con_map_set(kv, &vv)
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
				va.con_array_set(kv, &vv)
			}
			v.V_SET_ARRAY(va)
		}
	default:
		c.compile_seterror(ev, "compile_explicit_value_node_to_variant type error "+strconv.Itoa(ev.getvaluetype()))
	}

	return v
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
			pos := cg.getconst(v)
			c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)

			c.cmp_deps--
			c.cmp_jne = false

			return
		} else if cs.cmp == "false" {
			var v variant
			v.V_SET_REAL(0)
			pos := cg.getconst(v)
			c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)

			c.cmp_deps--
			c.cmp_jne = false

			return
		} else if cs.cmp == "is" {
			// left
			c.compile_node(cg, cs.left)

			c.cmp_deps--
			c.cmp_jne = false

			return
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

func (c *compiler) compile_if_stmt(cg *codegen, is *if_stmt) {

	log_debug("[compiler] compile_if_stmt %p", is)

	jnepos := 0
	var jmpifpos []int

	// 条件
	cg.push_stack_identifiers()
	c.compile_node(cg, is.cmp)
	cg.pop_stack_identifiers()

	// cmp与jne结合
	if c.cmp_jne {
		cg.push(EMPTY_CMD, is.cmp.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	} else {
		cg.push(_MAKE_OPCODE(OPCODE_JNE), is.lineno())
		cg.push(c.cur_addr, is.lineno())
		cg.push(EMPTY_CMD, is.lineno()) // 先塞个位置
		jnepos = cg.byte_code_size() - 1
	}
	c.cmp_deps = 0
	c.cmp_jne = false

	// if块
	if is.block != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, is.block)
		cg.pop_stack_identifiers()
	}

	// 跳出if块
	if is.elseifs != nil || (is.elses != nil && is.elses.block != nil) {
		cg.push(_MAKE_OPCODE(OPCODE_JMP), is.lineno())
		cg.push(EMPTY_CMD, is.lineno()) // 先塞个位置
		jmpifpos = append(jmpifpos, cg.byte_code_size()-1)
	}

	// 开始处理elseif的
	if is.elseifs != nil {
		list := is.elseifs.stmtlist
		for i, _ := range list {
			eis := list[i].(*elseif_stmt)

			// 跳转到else if
			cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))

			// 条件
			cg.push_stack_identifiers()
			c.compile_node(cg, eis.cmp)
			cg.pop_stack_identifiers()

			// cmp与jne结合
			if c.cmp_jne {
				cg.push(EMPTY_CMD, eis.cmp.lineno()) // 先塞个位置
				jnepos = cg.byte_code_size() - 1
			} else {
				cg.push(_MAKE_OPCODE(OPCODE_JNE), eis.lineno())
				cg.push(c.cur_addr, eis.lineno())
				cg.push(EMPTY_CMD, eis.lineno()) // 先塞个位置
				jnepos = cg.byte_code_size() - 1
			}
			c.cmp_deps = 0
			c.cmp_jne = false

			// else if块
			if eis.block != nil {
				cg.push_stack_identifiers()
				c.compile_node(cg, eis.block)
				cg.pop_stack_identifiers()
			}

			// 跳出if块
			cg.push(_MAKE_OPCODE(OPCODE_JMP), eis.lineno())
			cg.push(EMPTY_CMD, eis.lineno()) // 先塞个位置
			jmpifpos = append(jmpifpos, cg.byte_code_size()-1)
		}
	}

	// 跳转到else
	cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))

	// else块
	if is.elses != nil && is.elses.block != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, is.elses.block)
		cg.pop_stack_identifiers()
	}

	// 跳转到结束
	for i, _ := range jmpifpos {
		cg.set(jmpifpos[i], _MAKE_POS(cg.byte_code_size()))
	}

	log_debug("[compiler] compile_if_stmt %p OK", is)
}

func (c *compiler) compile_explicit_value(cg *codegen, ev *explicit_value_node) {

	log_debug("[compiler] compile_explicit_value %p %s", ev, ev.str)

	v := c.compile_explicit_value_node_to_variant(ev)

	pos := cg.getconst(v)
	c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)

	log_debug("[compiler] compile_explicit_value %p OK", ev)
}

func (c *compiler) compile_return_stmt(cg *codegen, rs *return_stmt) {

	log_debug("[compiler] compile_return_stmt %p", rs)

	if rs.returnlist != nil {
		c.compile_node(cg, rs.returnlist)

		cg.push(_MAKE_OPCODE(OPCODE_RETURN), rs.lineno())
		cg.push(_MAKE_POS(len(rs.returnlist.returnlist)), rs.lineno())
		for i, _ := range rs.returnlist.returnlist {
			cg.push(c.cur_addrs[i], rs.lineno())
		}
	} else {
		cg.push(_MAKE_OPCODE(OPCODE_RETURN), rs.lineno())
		cg.push(_MAKE_POS(0), rs.lineno())
	}

	log_debug("[compiler] compile_return_stmt %p OK", rs)
}

func (c *compiler) compile_return_value_list(cg *codegen, rn *return_value_list_node) {

	log_debug("[compiler] compile_return_value_list %p", rn)

	tmp := make([]command, len(rn.returnlist))
	for i, _ := range rn.returnlist {
		c.compile_node(cg, rn.returnlist[i])
		tmp[i] = c.cur_addr
	}
	c.cur_addrs = tmp
	c.cur_addr = c.cur_addrs[0]

	log_debug("[compiler] compile_return_value_list %p OK", rn)
}

func (c *compiler) compile_assign_stmt(cg *codegen, as *assign_stmt) {

	log_debug("[compiler] compile_assign_stmt %p", as)

	var vr command
	var value command

	c.compile_node(cg, as.value)
	value = c.cur_addr
	log_debug("[compiler] compile_assign_stmt value = %d", c.cur_addr)

	c.new_var = as.isnew
	c.compile_node(cg, as.vr)
	c.new_var = false
	vr = c.cur_addr
	log_debug("[compiler] compile_assign_stmt var = %d", c.cur_addr)

	cg.push(_MAKE_OPCODE(OPCODE_ASSIGN), as.lineno())
	cg.push(vr, as.lineno())
	cg.push(value, as.lineno())
	log_debug("[compiler] compile_assign_stmt %p OK", as)
}

func (c *compiler) compile_math_assign_stmt(cg *codegen, ms *math_assign_stmt) {

	log_debug("[compiler] compile_math_assign_stmt %p", ms)

	var oper command
	var vr command
	var value command

	if ms.oper == "+=" {
		oper = _MAKE_OPCODE(OPCODE_PLUS_ASSIGN)
	} else if ms.oper == "-=" {
		oper = _MAKE_OPCODE(OPCODE_MINUS_ASSIGN)
	} else if ms.oper == "*=" {
		oper = _MAKE_OPCODE(OPCODE_MULTIPLY_ASSIGN)
	} else if ms.oper == "/=" {
		oper = _MAKE_OPCODE(OPCODE_DIVIDE_ASSIGN)
	} else if ms.oper == "%=" {
		oper = _MAKE_OPCODE(OPCODE_DIVIDE_MOD_ASSIGN)
	} else {
		c.compile_seterror(ms, "compile math assign oper type %s error", ms.oper)
	}

	// value
	c.compile_node(cg, ms.value)
	value = c.cur_addr
	log_debug("[compiler] compile_math_assign_stmt value = %d", c.cur_addr)

	// var
	c.compile_node(cg, ms.vr)
	vr = c.cur_addr
	log_debug("[compiler] compile_math_assign_stmt var = %d", c.cur_addr)

	cg.push(oper, ms.lineno())
	cg.push(vr, ms.lineno())
	cg.push(value, ms.lineno())

	log_debug("[compiler] compile_math_assign_stmt %p OK", ms)
}

func (c *compiler) compile_variable_node(cg *codegen, vn *variable_node) {

	log_debug("[compiler] compile_variable_node %p", vn)

	// 看看是否是全局常量定义
	constname := fkgen_package_name(c.mf.get_package(), vn.str)
	find, gcv, _ := gfs.pa.get_const_define(constname)
	if find {
		pos := cg.getconst(gcv)
		c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)
		log_debug("[compiler] compile_variable_node %p OK", vn)
		return
	}

	find, gcv, _ = gfs.pa.get_const_define(vn.str)
	if find {
		pos := cg.getconst(gcv)
		c.cur_addr = _MAKE_ADDR(ADDR_CONST, pos)
		log_debug("[compiler] compile_variable_node %p OK", vn)
		return
	}

	// 从当前堆栈往上找
	pos := cg.getvariable(vn.str)
	if pos == -1 {
		// 是不是需要new出来
		if c.new_var {
			var tmp var_node
			tmp.str = vn.str
			c.compile_var_node(cg, &tmp)
			return
		} else {
			c.compile_seterror(vn, "variable %s not found", vn.str)
		}
	}
	c.cur_addr = _MAKE_ADDR(ADDR_STACK, pos)

	log_debug("[compiler] compile_variable_node %p OK", vn)
}

func (c *compiler) compile_var_node(cg *codegen, vn *var_node) {

	log_debug("[compiler] compile_var_node %p", vn)

	// 确保当前block没有
	if cg.get_cur_variable_pos(vn.str) != -1 {
		c.compile_seterror(vn, "variable %s has define", vn.str)
	}

	// 看看是否是常量定义
	mf := c.mf
	evm := mf.get_const_map()
	_, ok := evm.Load(vn.str)
	if ok {
		c.compile_seterror(vn, "variable %s has defined const", vn.str)
	}

	// 看看是否是全局常量定义
	find, _, _ := gfs.pa.get_const_define(vn.str)
	if find {
		c.compile_seterror(vn, "variable %s has defined global const", vn.str)
	}

	// 申请栈上空间
	pos := cg.add_stack_identifier(vn.str, vn.lineno())
	if pos == -1 {
		c.compile_seterror(vn, "double %s identifier error", vn.str)
	}
	c.cur_addr = _MAKE_ADDR(ADDR_STACK, pos)

	log_debug("[compiler] compile_var_node %p OK", vn)
}

func (c *compiler) compile_function_call_node(cg *codegen, fn *function_call_node) {

	log_debug("[compiler] compile_function_call_node %p", fn)

	mf := c.mf

	ret_num := c.func_ret_num
	c.func_ret_num = 1

	// 参数
	var arglist []command
	if fn.arglist != nil {
		for i, _ := range fn.arglist.arglist {
			sn := fn.arglist.arglist[i]
			c.compile_node(cg, sn)
			arglist = append(arglist, c.cur_addr)
		}
	}

	// 调用位置
	var callpos command
	if fn.prefunc == nil {
		funcn := fn.fuc
		// 1 检查变量
		pos := cg.getvariable(funcn)
		if pos != -1 {
			// 是用变量来调用函数
			callpos = _MAKE_ADDR(ADDR_STACK, pos)
		} else if mf.is_have_struct(funcn) {
			// 2 检查struct
			// 直接替换成map
			var v variant
			v.V_SET_STRING(MAP_FUNC_NAME)
			pos = cg.getconst(v)
			callpos = _MAKE_ADDR(ADDR_CONST, pos)
		} else if mf.is_have_func(funcn) {
			// 3 检查本地函数
			// 申请字符串变量
			var v variant
			// 拼上包名
			pname := fkgen_package_name(mf.get_package(), funcn)
			v.V_SET_STRING(pname)
			pos = cg.getconst(v)
			callpos = _MAKE_ADDR(ADDR_CONST, pos)
		} else {
			// 4 直接字符串使用
			// 申请字符串变量
			var v variant
			v.V_SET_STRING(funcn)
			pos = cg.getconst(v)
			callpos = _MAKE_ADDR(ADDR_CONST, pos)
		}
	} else {
		c.compile_node(cg, fn.prefunc)
		callpos = c.cur_addr
	}

	// oper
	oper := _MAKE_OPCODE(OPCODE_CALL)

	// 调用类型
	var calltype command
	if fn.fakecall {
		calltype = _MAKE_POS(CALL_FAKE)
	} else if fn.classmem_call {
		calltype = _MAKE_POS(CALL_CLASSMEM)
	} else {
		calltype = _MAKE_POS(CALL_NORMAL)
	}

	// 参数个数
	argnum := _MAKE_POS(len(arglist))

	// 返回值个数
	retnum := _MAKE_POS(ret_num)

	// 返回值
	var ret []command
	c.cur_addrs = make([]command, ret_num)
	for i := 0; i < ret_num; i++ {
		retpos := cg.alloc_stack_identifier()
		ret = append(ret, _MAKE_ADDR(ADDR_STACK, retpos))
		c.cur_addrs[i] = ret[i]
	}
	if ret_num > 0 {
		c.cur_addr = ret[0]
	} else {
		c.cur_addr = 0
	}

	cg.push(oper, fn.lineno())
	cg.push(calltype, fn.lineno())
	cg.push(callpos, fn.lineno())
	cg.push(retnum, fn.lineno())
	for i := 0; i < ret_num; i++ {
		cg.push(ret[i], fn.lineno())
	}
	cg.push(argnum, fn.lineno())
	for i, _ := range arglist {
		cg.push(arglist[i], fn.lineno())
	}

	log_debug("[compiler] compile_function_call_node %p OK", fn)
}

func (c *compiler) compile_break_stmt(cg *codegen, bs *break_stmt) {

	log_debug("[compiler] compile_break_stmt %p", bs)

	cg.push(_MAKE_OPCODE(OPCODE_JMP), bs.lineno())
	cg.push(EMPTY_CMD, bs.lineno()) // 先塞个位置

	jmppos := cg.byte_code_size() - 1

	c.loop_break_pos_stack[len(c.loop_break_pos_stack)-1] = append(c.loop_break_pos_stack[len(c.loop_break_pos_stack)-1], jmppos)

	log_debug("[compiler] compile_break_stmt %p OK", bs)
}

func (c *compiler) compile_continue_stmt(cg *codegen, cs *continue_stmt) {

	log_debug("[compiler] compile_continue_stmt %p", cs)

	if len(c.loop_continue_pos_stack) <= 0 {
		c.compile_seterror(cs, "no loop to continue")
	}

	continuepos := c.loop_continue_pos_stack[len(c.loop_continue_pos_stack)-1]

	cg.push(_MAKE_OPCODE(OPCODE_JMP), cs.lineno())
	cg.push(_MAKE_POS(continuepos), cs.lineno())

	if continuepos == -1 {
		// 一会统一设置
		pos := cg.byte_code_size() - 1
		c.continue_end_pos_stack[len(c.continue_end_pos_stack)-1] = append(c.continue_end_pos_stack[len(c.continue_end_pos_stack)-1], pos)
	}

	log_debug("[compiler] compile_continue_stmt %p OK", cs)
}

func (c *compiler) compile_math_expr_node(cg *codegen, mn *math_expr_node) {

	log_debug("[compiler] compile_math_expr_node %p", mn)

	var oper command
	var left command
	var right command
	var dest command

	if mn.oper == "+" {
		oper = _MAKE_OPCODE(OPCODE_PLUS)
	} else if mn.oper == "-" {
		oper = _MAKE_OPCODE(OPCODE_MINUS)
	} else if mn.oper == "*" {
		oper = _MAKE_OPCODE(OPCODE_MULTIPLY)
	} else if mn.oper == "/" {
		oper = _MAKE_OPCODE(OPCODE_DIVIDE)
	} else if mn.oper == "%" {
		oper = _MAKE_OPCODE(OPCODE_DIVIDE_MOD)
	} else if mn.oper == ".." {
		oper = _MAKE_OPCODE(OPCODE_STRING_CAT)
	} else {
		c.compile_seterror(mn, "compile math oper type %s error", mn.oper)
	}

	// left
	c.compile_node(cg, mn.left)
	left = c.cur_addr

	// right
	c.compile_node(cg, mn.right)
	right = c.cur_addr

	// result
	despos := cg.alloc_stack_identifier()
	dest = _MAKE_ADDR(ADDR_STACK, despos)
	c.cur_addr = dest

	cg.push(oper, mn.lineno())
	cg.push(left, mn.lineno())
	cg.push(right, mn.lineno())
	cg.push(dest, mn.lineno())

	log_debug("[compiler] compile_math_expr_node %p OK", mn)
}

func (c *compiler) compile_container_get(cg *codegen, cn *container_get_node) {

	log_debug("[compiler] compile_container_get %p", cn)

	// 编译con
	var con command

	// 看看是否是全局常量定义
	find, gcv, _ := gfs.pa.get_const_define(cn.container)
	if find {
		pos := cg.getconst(gcv)
		con = _MAKE_ADDR(ADDR_CONST, pos)
	} else {
		pos := cg.getvariable(cn.container)
		if pos == -1 {
			c.compile_seterror(cn, "variable %s not found", cn.container)
		}
		con = _MAKE_ADDR(ADDR_STACK, pos)
	}

	// 编译key
	var key command
	c.compile_node(cg, cn.key)
	key = c.cur_addr

	// 返回
	addrpos := cg.getcontaineraddr(con, key)
	c.cur_addr = _MAKE_ADDR(ADDR_CONTAINER, addrpos)

	log_debug("[compiler] compile_container_get %p OK", cn)
}

func (c *compiler) compile_struct_pointer(cg *codegen, sn *struct_pointer_node) {

	log_debug("[compiler] compile_struct_pointer %p", sn)

	name := sn.str
	var tmp []string
	for {
		pos := strings.Index(name, "->")
		if pos == -1 {
			tmp = append(tmp, name)
			break
		}
		tmp = append(tmp, name[0:pos])
		name = name[pos+2:]
	}

	if len(tmp) < 2 {
		c.compile_seterror(sn, "compile struct pointer %s failed", sn.str)
	}

	connname := tmp[0]

	// 编译con
	var con command
	// 看看是否是全局常量定义
	find, gcv, _ := gfs.pa.get_const_define(connname)
	if find {
		pos := cg.getconst(gcv)
		con = _MAKE_ADDR(ADDR_CONST, pos)
	} else {
		pos := cg.getvariable(connname)
		if pos == -1 {
			c.compile_seterror(sn, "variable %s not found", connname)
		}
		con = _MAKE_ADDR(ADDR_STACK, pos)
	}

	for i, _ := range tmp {
		keystr := tmp[i]

		// 编译key
		var v variant
		v.V_SET_STRING(keystr)
		pos := cg.getconst(v)
		key := _MAKE_ADDR(ADDR_CONST, pos)

		// 获取容器的位置
		addrpos := cg.getcontaineraddr(con, key)
		c.cur_addr = _MAKE_ADDR(ADDR_CONTAINER, addrpos)
		con = c.cur_addr
	}

	log_debug("[compiler] compile_struct_pointer %p OK", sn)
}

func (c *compiler) compile_switch_stmt(cg *codegen, ss *switch_stmt) {

	log_debug("[compiler] compile_switch_stmt %p", ss)

	var caseleft command
	var caseresult command

	cg.push_stack_identifiers()

	// caseleft
	c.compile_node(cg, ss.cmp)
	caseleft = c.cur_addr

	// caseresult
	despos := cg.alloc_stack_identifier()
	caseresult = _MAKE_ADDR(ADDR_STACK, despos)

	scln := ss.caselist.(*switch_caselist_node)

	var jmpswitchposlist []int

	// 挨个和case的比较
	for i, _ := range scln.list {
		oper := _MAKE_OPCODE(OPCODE_EQUAL)
		left := caseleft
		right := command(0)
		dest := caseresult

		scn := scln.list[i].(*switch_case_node)

		// right
		c.compile_node(cg, scn.cmp)
		right = c.cur_addr

		// push case
		cg.push(oper, scn.lineno())
		cg.push(left, scn.lineno())
		cg.push(right, scn.lineno())
		cg.push(dest, scn.lineno())

		// push jmp
		cg.push(_MAKE_OPCODE(OPCODE_JNE), scn.lineno())
		cg.push(dest, scn.lineno())
		cg.push(EMPTY_CMD, scn.lineno()) // 先塞个位置
		jnepos := cg.byte_code_size() - 1

		// build block
		if scn.block != nil {
			cg.push_stack_identifiers()
			c.compile_node(cg, scn.block)
			cg.pop_stack_identifiers()
		}

		// 跳出switch块
		cg.push(_MAKE_OPCODE(OPCODE_JMP), scn.lineno())
		cg.push(EMPTY_CMD, scn.lineno()) // 先塞个位置
		jmpswitchpos := cg.byte_code_size() - 1
		jmpswitchposlist = append(jmpswitchposlist, jmpswitchpos)

		// 跳转出case块
		cg.set(jnepos, _MAKE_POS(cg.byte_code_size()))
	}

	// default
	if ss.def != nil {
		cg.push_stack_identifiers()
		c.compile_node(cg, ss.def)
		cg.pop_stack_identifiers()
	}

	cg.pop_stack_identifiers()

	// 塞跳出的
	for i, _ := range jmpswitchposlist {
		cg.set(jmpswitchposlist[i], _MAKE_POS(cg.byte_code_size()))
	}

	log_debug("[compiler] compile_switch_stmt %p OK", ss)
}
