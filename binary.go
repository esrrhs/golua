package fakego

import (
	"github.com/esrrhs/go-engine/src/common"
	"github.com/esrrhs/go-engine/src/rbuffergo"
	"math"
)

type command uint64

var EMPTY_CMD = command(math.MaxInt64)

const (
	COMMAND_OPCODE = iota
	COMMAND_ADDR
	COMMAND_POS
)

const (
	OPCODE_ASSIGN = iota

	OPCODE_PLUS
	OPCODE_MINUS
	OPCODE_MULTIPLY
	OPCODE_DIVIDE
	OPCODE_DIVIDE_MOD
	OPCODE_STRING_CAT

	OPCODE_PLUS_ASSIGN
	OPCODE_MINUS_ASSIGN
	OPCODE_MULTIPLY_ASSIGN
	OPCODE_DIVIDE_ASSIGN
	OPCODE_DIVIDE_MOD_ASSIGN

	OPCODE_RETURN

	OPCODE_JNE
	OPCODE_JMP

	OPCODE_AND
	OPCODE_OR
	OPCODE_LESS
	OPCODE_MORE
	OPCODE_EQUAL
	OPCODE_MOREEQUAL
	OPCODE_LESSEQUAL
	OPCODE_NOTEQUAL
	OPCODE_NOT

	OPCODE_AND_JNE
	OPCODE_OR_JNE
	OPCODE_LESS_JNE
	OPCODE_MORE_JNE
	OPCODE_EQUAL_JNE
	OPCODE_MOREEQUAL_JNE
	OPCODE_LESSEQUAL_JNE
	OPCODE_NOTEQUAL_JNE
	OPCODE_NOT_JNE

	OPCODE_CALL

	OPCODE_SLEEP
	OPCODE_YIELD

	OPCODE_FOR

	OPCODE_MAX
)

const (
	ADDR_STACK = iota
	ADDR_CONST
	ADDR_CONTAINER
)

type container_addr struct {
	con command
	key command
}

const (
	CALL_NORMAL = iota
	CALL_FAKE
	CALL_CLASSMEM
)

func _MAKE_COMMAND(ty int, code int) command {
	return command(common.MAKEINT64(int32(ty), int32(code)))
}
func _MAKE_ADDR(addrtype int, pos int) command {
	return _MAKE_COMMAND(COMMAND_ADDR, int(common.MAKEINT32(int16(addrtype), int16(pos))))
}
func _MAKE_OPCODE(op int) command {
	return _MAKE_COMMAND(COMMAND_OPCODE, op)
}
func _MAKE_POS(pos int) command {
	return _MAKE_COMMAND(COMMAND_POS, pos)
}

func _COMMAND_TYPE(cmd command) int {
	return int(common.HIINT32(int64(cmd)))
}
func _COMMAND_CODE(cmd command) int {
	return int(common.LOINT32(int64(cmd)))
}

func _ADDR_TYPE(code int) int {
	return int(common.HIINT16(int32(code)))
}
func _ADDR_POS(code int) int {
	return int(common.LOINT16(int32(code)))
}

func OpCodeStr(opcode int) string {
	switch opcode {

	case OPCODE_ASSIGN:
		return "OPCODE_ASSIGN"

	case OPCODE_PLUS:
		return "OPCODE_PLUS"
	case OPCODE_MINUS:
		return "OPCODE_MINUS"
	case OPCODE_MULTIPLY:
		return "OPCODE_MULTIPLY"
	case OPCODE_DIVIDE:
		return "OPCODE_DIVIDE"
	case OPCODE_DIVIDE_MOD:
		return "OPCODE_DIVIDE_MOD"
	case OPCODE_STRING_CAT:
		return "OPCODE_STRING_CAT"

	case OPCODE_PLUS_ASSIGN:
		return "OPCODE_PLUS_ASSIGN"
	case OPCODE_MINUS_ASSIGN:
		return "OPCODE_MINUS_ASSIGN"
	case OPCODE_MULTIPLY_ASSIGN:
		return "OPCODE_MULTIPLY_ASSIGN"
	case OPCODE_DIVIDE_ASSIGN:
		return "OPCODE_DIVIDE_ASSIGN"
	case OPCODE_DIVIDE_MOD_ASSIGN:
		return "OPCODE_DIVIDE_MOD_ASSIGN"

	case OPCODE_RETURN:
		return "OPCODE_RETURN"

	case OPCODE_JNE:
		return "OPCODE_JNE"
	case OPCODE_JMP:
		return "OPCODE_JMP"

	case OPCODE_AND:
		return "OPCODE_AND"
	case OPCODE_OR:
		return "OPCODE_OR"
	case OPCODE_LESS:
		return "OPCODE_LESS"
	case OPCODE_MORE:
		return "OPCODE_MORE"
	case OPCODE_EQUAL:
		return "OPCODE_EQUAL"
	case OPCODE_MOREEQUAL:
		return "OPCODE_MOREEQUAL"
	case OPCODE_LESSEQUAL:
		return "OPCODE_LESSEQUAL"
	case OPCODE_NOTEQUAL:
		return "OPCODE_NOTEQUAL"
	case OPCODE_NOT:
		return "OPCODE_NOT"

	case OPCODE_AND_JNE:
		return "OPCODE_AND_JNE"
	case OPCODE_OR_JNE:
		return "OPCODE_OR_JNE"
	case OPCODE_LESS_JNE:
		return "OPCODE_LESS_JNE"
	case OPCODE_MORE_JNE:
		return "OPCODE_MORE_JNE"
	case OPCODE_EQUAL_JNE:
		return "OPCODE_EQUAL_JNE"
	case OPCODE_MOREEQUAL_JNE:
		return "OPCODE_MOREEQUAL_JNE"
	case OPCODE_LESSEQUAL_JNE:
		return "OPCODE_LESSEQUAL_JNE"
	case OPCODE_NOTEQUAL_JNE:
		return "OPCODE_NOTEQUAL_JNE"
	case OPCODE_NOT_JNE:
		return "OPCODE_NOT_JNE"

	case OPCODE_CALL:
		return "OPCODE_CALL"

	case OPCODE_SLEEP:
		return "OPCODE_SLEEP"
	case OPCODE_YIELD:
		return "OPCODE_YIELD"

	case OPCODE_FOR:
		return "OPCODE_FOR"

	}
	return "unknow"
}

type stack_variant_info struct {
	name string
	line int
	pos  int
}

type func_binary struct {
	// 最大栈空间
	maxstack int
	// 参数个数
	paramnum int
	// 名字
	name string
	// 文件名
	filename string
	// 包名
	packagename string
	// 二进制缓冲区
	buff []command
	// 二进制行号缓冲区
	lineno_buff []int
	// 常量
	const_list []variant
	// container地址
	container_addr_list []container_addr
	// 调试信息，栈变量
	debug_stack_variant_info []stack_variant_info
}

func (fb *func_binary) dump(pos int) string {
	return ""
}

func (fb *func_binary) save(buffer *rbuffergo.RBuffergo) error {
	return nil
}

func (fb *func_binary) load(buffer *rbuffergo.RBuffergo) error {
	return nil
}
