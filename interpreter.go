package fakego

import (
	"errors"
)

func Run(fun string, p ...interface{}) (ret []interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				panic(x)
			case FakeErr:
				err = &x
			default:
				err = errors.New("unknown panic error")
			}
		}
	}()

	log_debug("start run " + fun)

	ps := &paramstack{}
	ps.pushs(p)

	var funcv variant
	funcv.V_SET_STRING(fun)

	inter := &interpreter{}
	inter.call(funcv, ps, nil)
	inter.run()

	ps.vlist = inter.ret
	ret = ps.trans()
	return
}

func DebugRun(fun string, p ...interface{}) (ret interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch x := r.(type) {
			case string:
				err = errors.New(x)
			case error:
				panic(x)
			case FakeErr:
				err = &x
			default:
				err = errors.New("unknown panic error")
			}
		}
	}()

	log_debug("start debug run " + fun)

	ps := &paramstack{}
	ps.pushs(p)

	var funcv variant
	funcv.V_SET_STRING(fun)

	inter := &interpreter{}
	inter.call(funcv, ps, nil)

	dbg := &debuging{}
	dbg.debug()

	ret = ps.pop()
	return
}

/**************
[ret pos] .. [ret pos] [ret num] [old ip] [call time] [old fb] [old bp]
**************/
const (
	BP_SIZE = 5
)

type interpreter struct {
	stack []variant
	ip    int
	bp    int
	sp    int
	fb    *func_binary
	ret   []variant
	isend bool
}

func (inter *interpreter) call(fun variant, ps *paramstack, retpos []int) {
	f := gfs.fm.get_func(fun)
	if f == nil {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "run no func %s fail", vartostring(fun))
	}

	retnum := len(retpos)

	if f.havefb {

		fb := f.fb

		// 准备栈大小
		needsize := inter.sp + BP_SIZE + retnum + fb.maxstack
		if needsize >= len(inter.stack) {
			oldsize := len(inter.stack)
			inter.stack = append(inter.stack, make([]variant, needsize-oldsize)...)
		}

		// 老的bp
		oldbp := inter.bp
		inter.bp = inter.sp

		// 记录返回位置
		for i := 0; i < retnum; i++ {
			inter.stack[inter.bp].ty = NIL
			inter.stack[inter.bp].data = retpos[i]
			inter.bp++
		}

		// 记录返回值数目
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = retnum
		inter.bp++

		// 记录老的ip
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = inter.ip
		inter.bp++

		// 记录profile
		if gfs.cfg.OpenProfile {
			inter.stack[inter.bp].data = fkgetmstick()
		}
		inter.stack[inter.bp].ty = NIL
		inter.bp++

		// 记录老的fb
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = inter.fb
		inter.bp++

		// 记录老的bp
		inter.stack[inter.bp].ty = NIL
		inter.stack[inter.bp].data = oldbp
		inter.bp++

		// 设置sp
		inter.sp = inter.bp + fb.maxstack

		if ps.size() != fb.paramnum {
			seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "call func %s param not match", vartostring(fun))
		}

		// 分配入参
		copy(inter.stack[inter.bp:], ps.vlist)

		// 清空栈区
		for i := 0; i < fb.maxstack-fb.paramnum; i++ {
			inter.stack[inter.bp+fb.paramnum+i].V_SET_NIL()
		}

		// 重置ret
		if len(inter.ret) <= 0 {
			inter.ret = make([]variant, 1)
		}
		for i, _ := range inter.ret {
			inter.ret[i].V_SET_NIL()
		}

		// 新函数
		inter.fb = fb
		inter.ip = 0

	} else if f.havebif {
		// TODO
	} else if f.haveff {
		// TODO
	} else {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "run no func %s fail", vartostring(fun))
	}
}

func (inter *interpreter) run() {

	// 栈溢出检查
	if len(inter.stack) > gfs.cfg.StackMax {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "stack too big %d", len(inter.stack))
	}

	if inter.isend {
		return
	}

	for {
		// 当前函数走完
		if inter.ip >= inter.fb.binary_size() {
			// 记录profile
			if gfs.cfg.OpenProfile {
				calltime := inter.BP_GET_CALLTIME(inter.bp)
				gfs.pf.add_func_sample(inter.fb.name, fkgetmstick()-calltime)
			}

			// 出栈
			oldretnum := inter.BP_GET_RETNUM(inter.bp)
			callbp := inter.BP_GET_BP(inter.bp)
			inter.fb = inter.BP_GET_FB(inter.bp)
			inter.ip = inter.BP_GET_IP(inter.bp)
			oldbp := inter.bp
			inter.sp = inter.bp - BP_SIZE - oldretnum
			inter.bp = callbp

			// 所有都完
			if inter.BP_END(inter.bp) {
				inter.isend = true
				break
			} else { // 塞返回值
				for i := 0; i < oldretnum; i++ {
					oldretpos := inter.BP_GET_RETPOS(oldbp, oldretnum, i)

					ret := inter.GET_VARIANT(inter.fb, inter.bp, oldretpos)

					if i < len(inter.ret) {
						*ret = inter.ret[i]
					} else {
						(*ret).V_SET_NIL()
					}
				}
			}
			continue
		}

		code := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))

		//log_debug("next %d %d %s", _COMMAND_TYPE(inter.GET_CMD(inter.fb, inter.ip)), code, opcodeStr(code))

		inter.ip++

		if gfs.cfg.OpenProfile {
			gfs.pf.add_code_sample(code)
		}

		// 执行对应命令，放一起switch效率更高，cpu有缓存
		switch code {
		case OPCODE_ASSIGN:
			// 赋值dest，必须为栈上或容器内
			if !(inter.CHECK_STACK_POS(inter.fb, inter.ip) || inter.CHECK_CONTAINER_POS(inter.fb, inter.ip)) {
				seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter assign error, dest is not stack or container, type %s", inter.POS_TYPE_NAME(inter.fb, inter.ip))
			}

			varv := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			if inter.CHECK_CONST_MAP_POS(*varv) || inter.CHECK_CONST_ARRAY_POS(*varv) {
				seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter assign error, dest is const container")
			}
			inter.ip++

			// 赋值来源
			valuev := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			// 赋值
			*varv = *valuev
			//log_debug("assign %s to %s", vartostring(valuev), vartostring(varv))
		case OPCODE_PLUS:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_PLUS(left, right, dest)
		case OPCODE_MINUS:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_MINUS(left, right, dest)
		case OPCODE_MULTIPLY:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_MULTIPLY(left, right, dest)
		case OPCODE_DIVIDE:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_DIVIDE(left, right, dest)
		case OPCODE_DIVIDE_MOD:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_DIVIDE_MOD(left, right, dest)
		case OPCODE_STRING_CAT:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_STRING_CAT(left, right, dest)
		case OPCODE_AND:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_AND(left, right, dest)
		case OPCODE_OR:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_OR(left, right, dest)
		case OPCODE_LESS:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_LESS(left, right, dest)
		case OPCODE_MORE:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_MORE(left, right, dest)
		case OPCODE_EQUAL:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_EQUAL(left, right, dest)
		case OPCODE_MOREEQUAL:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_MOREEQUAL(left, right, dest)
		case OPCODE_LESSEQUAL:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_LESSEQUAL(left, right, dest)
		case OPCODE_NOTEQUAL:
			left, right, dest := inter.MATH_OPER(inter.fb, inter.bp)
			inter.V_NOTEQUAL(left, right, dest)
		case OPCODE_NOT:
			left, dest := inter.MATH_SINGLE_OPER(inter.fb, inter.bp)
			inter.V_NOT(left, dest)
		case OPCODE_AND_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_AND_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_OR_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_OR_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_LESS_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_LESS_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_MORE_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_MORE_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_EQUAL_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_EQUAL_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_MOREEQUAL_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_MOREEQUAL_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_LESSEQUAL_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_LESSEQUAL_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_NOTEQUAL_JNE:
			left, right, destip := inter.MATH_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_NOTEQUAL_JNE(left, right) {
				inter.ip = destip
			}
		case OPCODE_NOT_JNE:
			left, destip := inter.MATH_SINGLE_OPER_JNE(inter.fb, inter.bp)
			if !inter.V_NOT_JNE(left) {
				inter.ip = destip
			}
		case OPCODE_JNE:
			cmp := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			ip := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++

			if !cmp.V_ISBOOL() {
				inter.ip = ip
			}
		case OPCODE_JMP:
			ip := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++
			inter.ip = ip
		case OPCODE_PLUS_ASSIGN:
			right, dest := inter.MATH_ASSIGN_OPER(inter.fb, inter.bp)
			inter.V_PLUS(*dest, right, dest)
		case OPCODE_MINUS_ASSIGN:
			right, dest := inter.MATH_ASSIGN_OPER(inter.fb, inter.bp)
			inter.V_MINUS(*dest, right, dest)
		case OPCODE_MULTIPLY_ASSIGN:
			right, dest := inter.MATH_ASSIGN_OPER(inter.fb, inter.bp)
			inter.V_MULTIPLY(*dest, right, dest)
		case OPCODE_DIVIDE_ASSIGN:
			right, dest := inter.MATH_ASSIGN_OPER(inter.fb, inter.bp)
			inter.V_DIVIDE(*dest, right, dest)
		case OPCODE_DIVIDE_MOD_ASSIGN:
			right, dest := inter.MATH_ASSIGN_OPER(inter.fb, inter.bp)
			inter.V_DIVIDE_MOD(*dest, right, dest)
		case OPCODE_CALL:
			calltype := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++

			callpos := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			retnum := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++

			retpos := make([]int, retnum)

			for i := 0; i < retnum; i++ {
				retpos[i] = inter.ip
				inter.ip++
			}

			argnum := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++

			ps := &paramstack{}

			for i := 0; i < argnum; i++ {
				arg := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
				inter.ip++

				ps.vlist = append(ps.vlist, *arg)
			}

			if calltype == CALL_NORMAL {
				inter.call(*callpos, ps, retpos)
			} else {
				// TODO
			}
		case OPCODE_FOR:
			iter := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			end := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			step := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
			inter.ip++

			// tmp dest
			inter.ip++

			ip := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			inter.ip++

			inter.V_PLUS(*iter, *step, iter)

			b := false
			inter.V_FOR_LESS(*iter, *end, &b)

			if b {
				inter.ip = ip
			}
		case OPCODE_RETURN:
			returnnum := _COMMAND_CODE(inter.GET_CMD(inter.fb, inter.ip))
			if returnnum == 0 {
				inter.ip = inter.fb.binary_size()
				break
			}
			inter.ip++

			// 塞给ret
			if len(inter.ret) < returnnum {
				inter.ret = make([]variant, returnnum)
			}
			for i := 0; i < returnnum; i++ {
				ret := inter.GET_VARIANT(inter.fb, inter.bp, inter.ip)
				inter.ip++

				inter.ret[i] = *ret
			}

			inter.ip = inter.fb.binary_size()
		}
	}
}

func (inter *interpreter) getcurfile() string {
	return "TODO"
}

func (inter *interpreter) getcurline() int {
	return 0
}

func (inter *interpreter) getcurfunc() string {
	return "TODO"
}

func (inter *interpreter) BP_END(bp int) bool {
	return bp == 0
}

func (inter *interpreter) BP_GET_CALLTIME(bp int) int64 {
	return inter.stack[bp-3].data.(int64)
}

func (inter *interpreter) BP_GET_RETNUM(bp int) int {
	return inter.stack[bp-5].data.(int)
}

func (inter *interpreter) BP_GET_BP(bp int) int {
	return inter.stack[bp-1].data.(int)
}

func (inter *interpreter) BP_GET_FB(bp int) *func_binary {
	return inter.stack[bp-2].data.(*func_binary)
}

func (inter *interpreter) BP_GET_IP(bp int) int {
	return inter.stack[bp-4].data.(int)
}

func (inter *interpreter) BP_GET_RETPOS(bp int, retnum int, i int) int {
	return inter.stack[bp-5-retnum+i].data.(int)
}

func (inter *interpreter) GET_VARIANT(fb *func_binary, bp int, pos int) *variant {
	cmd := inter.GET_CMD(fb, pos)
	return inter.GET_VARIANT_BY_CMD(fb, bp, cmd)
}
func (inter *interpreter) GET_VARIANT_BY_CMD(fb *func_binary, bp int, cmd command) *variant {
	addrtype := _ADDR_TYPE(_COMMAND_CODE(cmd))
	addrpos := _ADDR_POS(_COMMAND_CODE(cmd))
	switch addrtype {
	case ADDR_STACK:
		return inter.GET_STACK(bp, addrpos)
	case ADDR_CONST:
		return inter.GET_CONST(fb, addrpos)
	case ADDR_CONTAINER:
		return inter.GET_CONTAINER(fb, bp, addrpos)
	default:
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "addrtype cannot be %d %d", addrtype, addrpos)
	}
	return nil
}

func (inter *interpreter) GET_CMD(fb *func_binary, pos int) command {
	return fb.buff[pos]
}

func (inter *interpreter) GET_CMD_ADDR_TYPE(fb *func_binary, pos int) int {
	return _ADDR_TYPE(_COMMAND_CODE(inter.GET_CMD(fb, pos)))
}

func (inter *interpreter) CHECK_STACK_POS(fb *func_binary, pos int) bool {
	return _ADDR_TYPE(_COMMAND_CODE(inter.GET_CMD(fb, pos))) == ADDR_STACK
}

func (inter *interpreter) CHECK_CONTAINER_POS(fb *func_binary, pos int) bool {
	return _ADDR_TYPE(_COMMAND_CODE(inter.GET_CMD(fb, pos))) == ADDR_CONTAINER
}

func (inter *interpreter) POS_TYPE_NAME(fb *func_binary, pos int) string {
	return vartypetostring(inter.GET_CMD_ADDR_TYPE(fb, pos))
}

func (inter *interpreter) GET_STACK(bp int, pos int) *variant {
	return &inter.stack[bp+pos]
}

func (inter *interpreter) GET_CONST(fb *func_binary, pos int) *variant {
	return &fb.const_list[pos]
}

func (inter *interpreter) GET_CONTAINER(fb *func_binary, bp int, conpos int) *variant {
	ca := fb.container_addr_list[conpos]
	conv := inter.GET_VARIANT_BY_CMD(fb, bp, ca.con)
	keyv := inter.GET_VARIANT_BY_CMD(fb, bp, ca.key)

	switch conv.ty {
	case MAP:
		return conv.data.(*variant_map).con_map_get(*keyv)
	case ARRAY:
		return conv.data.(*variant_array).con_array_get(*keyv)
	default:
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter get container variant fail, container type error, type %s", vartypetostring(conv.ty))
	}
	return nil
}

func (inter *interpreter) CHECK_CONST_MAP_POS(v variant) bool {
	if v.ty == MAP {
		d := v.data.(*variant_map)
		return d.isconst
	}
	return false
}

func (inter *interpreter) CHECK_CONST_ARRAY_POS(v variant) bool {
	if v.ty == ARRAY {
		d := v.data.(*variant_array)
		return d.isconst
	}
	return false
}

func (inter *interpreter) MATH_ASSIGN_OPER(fb *func_binary, bp int) (variant, *variant) {
	if !(inter.CHECK_STACK_POS(fb, inter.ip) || inter.CHECK_CONTAINER_POS(fb, inter.ip)) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter math oper error, dest is not stack, type %s", inter.POS_TYPE_NAME(fb, inter.ip))
	}
	dest := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++
	if !(inter.CHECK_CONST_ARRAY_POS(*dest) || inter.CHECK_CONST_MAP_POS(*dest)) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter assign error, dest is const container")
	}

	right := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	return *right, dest
}

func (inter *interpreter) MATH_OPER(fb *func_binary, bp int) (variant, variant, *variant) {

	left := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	right := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	if !(inter.CHECK_STACK_POS(fb, inter.ip) || inter.CHECK_CONTAINER_POS(fb, inter.ip)) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter math oper error, dest is not stack, type %s", inter.POS_TYPE_NAME(fb, inter.ip))
	}
	dest := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	//log_debug("math left %s right %s", vartostring(left), vartostring(right))
	return *left, *right, dest
}

func (inter *interpreter) V_ASSERT_CAN_CAL(v variant) {
	if !(v.ty == REAL) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "variant can not calculate, the variant is %s %s", vartypetostring(v.ty), vartostring(v))
	}
}

func (inter *interpreter) V_ASSERT_CAN_DIVIDE(v variant) {
	if !(v.data.(float64) == 0) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "variant can not be divide, the variant is %s %s", vartypetostring(v.ty), vartostring(v))
	}
}

func (inter *interpreter) V_PLUS(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	dest.data = left.data.(float64) + right.data.(float64)
}

func (inter *interpreter) V_MINUS(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	dest.data = left.data.(float64) - right.data.(float64)
}

func (inter *interpreter) V_MULTIPLY(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	dest.data = left.data.(float64) * right.data.(float64)
}

func (inter *interpreter) V_DIVIDE(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	inter.V_ASSERT_CAN_DIVIDE(right)
	dest.data = left.data.(float64) / right.data.(float64)
}

func (inter *interpreter) V_DIVIDE_MOD(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	inter.V_ASSERT_CAN_DIVIDE(right)
	dest.data = float64(int64(left.data.(float64)) % int64(right.data.(float64)))
}

func (inter *interpreter) V_STRING_CAT(left variant, right variant, dest *variant) {
	dest.V_SET_STRING(left.String() + right.String())
}

func (inter *interpreter) V_AND(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) != 0 && right.data.(float64) != 0 {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_OR(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) != 0 || right.data.(float64) != 0 {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_FOR_LESS(left variant, right variant, dest *bool) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) < right.data.(float64) {
		*dest = true
	} else {
		*dest = false
	}
}

func (inter *interpreter) V_LESS(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) < right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_MORE(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) > right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_EQUAL(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) == right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_MOREEQUAL(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) >= right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_LESSEQUAL(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) <= right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) V_NOTEQUAL(left variant, right variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	if left.data.(float64) != right.data.(float64) {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) MATH_SINGLE_OPER(fb *func_binary, bp int) (variant, *variant) {
	left := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	if !(inter.CHECK_STACK_POS(fb, inter.ip) || inter.CHECK_CONTAINER_POS(fb, inter.ip)) {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "interpreter math oper error, dest is not stack, type %s", inter.POS_TYPE_NAME(fb, inter.ip))
	}
	dest := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	//log_debug("math left %s ", vartostring(left))
	return *left, dest
}

func (inter *interpreter) V_NOT(left variant, dest *variant) {
	inter.V_ASSERT_CAN_CAL(left)
	if left.data.(float64) == 0 {
		dest.data = float64(1)
	} else {
		dest.data = float64(0)
	}
	dest.ty = REAL
}

func (inter *interpreter) MATH_OPER_JNE(fb *func_binary, bp int) (variant, variant, int) {
	left := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	right := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	/*dest*/
	inter.ip++
	destip := _COMMAND_CODE(inter.GET_CMD(fb, inter.ip))
	inter.ip++

	//log_debug("math left %s right %s ", vartostring(left), vartostring(right))
	return *left, *right, destip
}

func (inter *interpreter) V_AND_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) != 0 && right.data.(float64) != 0
}

func (inter *interpreter) V_OR_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) != 0 || right.data.(float64) != 0
}

func (inter *interpreter) V_LESS_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) < right.data.(float64)
}

func (inter *interpreter) V_MORE_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) > right.data.(float64)
}

func (inter *interpreter) V_EQUAL_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) == right.data.(float64)
}

func (inter *interpreter) V_MOREEQUAL_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) >= right.data.(float64)
}

func (inter *interpreter) V_LESSEQUAL_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) <= right.data.(float64)
}

func (inter *interpreter) V_NOTEQUAL_JNE(left variant, right variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	inter.V_ASSERT_CAN_CAL(right)
	return left.data.(float64) != right.data.(float64)
}

func (inter *interpreter) MATH_SINGLE_OPER_JNE(fb *func_binary, bp int) (variant, int) {
	left := inter.GET_VARIANT(fb, bp, inter.ip)
	inter.ip++

	/*dest*/
	inter.ip++
	destip := _COMMAND_CODE(inter.GET_CMD(fb, inter.ip))
	inter.ip++

	//log_debug("math left %s ", vartostring(left))
	return *left, destip
}

func (inter *interpreter) V_NOT_JNE(left variant) bool {
	inter.V_ASSERT_CAN_CAL(left)
	return left.data.(float64) == 0
}
