package fakego

import (
	"strconv"
	"strings"
)

const (
	est_nil = iota
	est_func_desc
	est_arglist
	est_block
	est_while_stmt
	est_cmp_stmt
	est_if_stmt
	est_else_stmt
	est_for_stmt
	est_for_loop_stmt
	est_explicit_value
	est_return_stmt
	est_return_value_list
	est_assign_stmt
	est_math_assign_stmt
	est_variable
	est_var
	est_function_call
	est_call_arglist
	est_math_expr
	est_break
	est_identifier
	est_multi_assign_stmt
	est_var_list
	est_container_get
	est_struct_memlist
	est_struct_pointer
	est_continue
	est_sleep
	est_yield
	est_switch_stmt
	est_switch_caselist
	est_switch_case_node
	est_elseif_stmt
	est_elseif_stmt_list
	est_constmaplist
	est_constmapvalue
	est_constarraylist
)

var syntree_TYPE_name = map[int]string{
	est_nil:               "est_nil",
	est_func_desc:         "est_func_desc",
	est_arglist:           "est_arglist",
	est_block:             "est_block",
	est_while_stmt:        "est_while_stmt",
	est_cmp_stmt:          "est_cmp_stmt",
	est_if_stmt:           "est_if_stmt",
	est_else_stmt:         "est_else_stmt",
	est_for_stmt:          "est_for_stmt",
	est_for_loop_stmt:     "est_for_loop_stmt",
	est_explicit_value:    "est_explicit_value",
	est_return_stmt:       "est_return_stmt",
	est_return_value_list: "est_return_value_list",
	est_assign_stmt:       "est_assign_stmt",
	est_math_assign_stmt:  "est_math_assign_stmt",
	est_variable:          "est_variable",
	est_var:               "est_var",
	est_function_call:     "est_function_call",
	est_call_arglist:      "est_call_arglist",
	est_math_expr:         "est_math_expr",
	est_break:             "est_break",
	est_identifier:        "est_identifier",
	est_multi_assign_stmt: "est_multi_assign_stmt",
	est_var_list:          "est_var_list",
	est_container_get:     "est_container_get",
	est_struct_memlist:    "est_struct_memlist",
	est_struct_pointer:    "est_struct_pointer",
	est_continue:          "est_continue",
	est_sleep:             "est_sleep",
	est_yield:             "est_yield",
	est_switch_stmt:       "est_switch_stmt",
	est_switch_caselist:   "est_switch_caselist",
	est_switch_case_node:  "est_switch_case_node",
	est_elseif_stmt:       "est_elseif_stmt",
	est_elseif_stmt_list:  "est_elseif_stmt_list",
	est_constmaplist:      "est_constmaplist",
	est_constmapvalue:     "est_constmapvalue",
	est_constarraylist:    "est_constarraylist",
}

var syntree_TYPE_value = map[string]int32{
	"est_nil":               est_nil,
	"est_func_desc":         est_func_desc,
	"est_arglist":           est_arglist,
	"est_block":             est_block,
	"est_while_stmt":        est_while_stmt,
	"est_cmp_stmt":          est_cmp_stmt,
	"est_if_stmt":           est_if_stmt,
	"est_else_stmt":         est_else_stmt,
	"est_for_stmt":          est_for_stmt,
	"est_for_loop_stmt":     est_for_loop_stmt,
	"est_explicit_value":    est_explicit_value,
	"est_return_stmt":       est_return_stmt,
	"est_return_value_list": est_return_value_list,
	"est_assign_stmt":       est_assign_stmt,
	"est_math_assign_stmt":  est_math_assign_stmt,
	"est_variable":          est_variable,
	"est_var":               est_var,
	"est_function_call":     est_function_call,
	"est_call_arglist":      est_call_arglist,
	"est_math_expr":         est_math_expr,
	"est_break":             est_break,
	"est_identifier":        est_identifier,
	"est_multi_assign_stmt": est_multi_assign_stmt,
	"est_var_list":          est_var_list,
	"est_container_get":     est_container_get,
	"est_struct_memlist":    est_struct_memlist,
	"est_struct_pointer":    est_struct_pointer,
	"est_continue":          est_continue,
	"est_sleep":             est_sleep,
	"est_yield":             est_yield,
	"est_switch_stmt":       est_switch_stmt,
	"est_switch_caselist":   est_switch_caselist,
	"est_switch_case_node":  est_switch_case_node,
	"est_elseif_stmt":       est_elseif_stmt,
	"est_elseif_stmt_list":  est_elseif_stmt_list,
	"est_constmaplist":      est_constmaplist,
	"est_constmapvalue":     est_constmapvalue,
	"est_constarraylist":    est_constarraylist,
}

type syntree_node interface {
	gettype() int
	dump(indent int) string
	lineno() int
}

type syntree_node_base struct {
	lno int
}

func (sn *syntree_node_base) lineno() int {
	return sn.lno
}
func (sn *syntree_node_base) gentab(n int) string {
	ret := ""
	ret += "LINE:"
	ret += strconv.Itoa(sn.lineno())
	ret += " "
	ret += strings.Repeat("\t", n)
	return ret
}

//////////////////////////////////////////////////////////////////
type struct_desc_memlist_node struct {
	syntree_node_base
	memlist []string
}

func (sn *struct_desc_memlist_node) gettype() int {
	return est_struct_memlist
}
func (sn *struct_desc_memlist_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[struct_desc_memlist_node]:\n"
	for i := range sn.memlist {
		ret += sn.gentab(indent + 1)
		ret += sn.memlist[i]
		ret += "\n"
	}
	return ret
}
func (sn *struct_desc_memlist_node) add_arg(mem string) {
	sn.memlist = append(sn.memlist, mem)
}

//////////////////////////////////////////////////////////////////

type func_desc_arglist_node struct {
	syntree_node_base
	arglist []string
}

func (sn *func_desc_arglist_node) gettype() int {
	return est_arglist
}
func (sn *func_desc_arglist_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[func_desc_arglist]:\n"
	for i := range sn.arglist {
		ret += sn.gentab(indent + 1)
		ret += sn.arglist[i]
		ret += "\n"
	}
	return ret
}
func (sn *func_desc_arglist_node) add_arg(mem string) {
	sn.arglist = append(sn.arglist, mem)
}

//////////////////////////////////////////////////////////////////

type block_node struct {
	syntree_node_base
	stmtlist []syntree_node
}

func (sn *block_node) gettype() int {
	return est_block
}
func (sn *block_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[block]:\n"
	for i := range sn.stmtlist {
		ret += sn.gentab(indent + 1)
		ret += "[stmt"
		ret += strconv.Itoa(i)
		ret += "]:\n"
		ret += sn.stmtlist[i].dump(indent + 2)
	}
	return ret
}
func (sn *block_node) add_stmt(stmt syntree_node) {
	sn.stmtlist = append(sn.stmtlist, stmt)
}

//////////////////////////////////////////////////////////////////

type func_desc_node struct {
	syntree_node_base
	memlist []string

	funcname string
	arglist  *func_desc_arglist_node
	block    *block_node
}

func (sn *func_desc_node) gettype() int {
	return est_func_desc
}
func (sn *func_desc_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[func_desc]:\n"
	ret += sn.funcname
	ret += "\n"
	if sn.arglist != nil {
		ret += sn.arglist.dump(indent + 1)
	}
	if sn.block != nil {
		ret += sn.block.dump(indent + 1)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type identifier_node struct {
	syntree_node_base
	str string
}

func (sn *identifier_node) gettype() int {
	return est_identifier
}
func (sn *identifier_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[identifier]:\n"
	ret += sn.str
	ret += "\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type function_call_arglist_node struct {
	syntree_node_base
	arglist []syntree_node
}

func (sn *function_call_arglist_node) gettype() int {
	return est_call_arglist
}
func (sn *function_call_arglist_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[func_call_arglist]:\n"
	for i := range sn.arglist {
		ret += sn.gentab(indent + 1)
		ret += "[arg"
		ret += strconv.Itoa(i)
		ret += "]:\n"
		ret += sn.arglist[i].dump(indent + 2)
	}
	return ret
}
func (sn *function_call_arglist_node) add_arg(arg syntree_node) {
	sn.arglist = append(sn.arglist, arg)
}

//////////////////////////////////////////////////////////////////

type function_call_node struct {
	syntree_node_base
	str           string
	fakecall      bool
	classmem_call bool
	fuc           string
	prefunc       syntree_node
	arglist       *function_call_arglist_node
}

func (sn *function_call_node) gettype() int {
	return est_function_call
}
func (sn *function_call_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	if sn.fakecall {
		ret += "[func_fake_call]:"
	} else if sn.classmem_call {
		ret += "[class_mem_call]:"
	} else {
		ret += "[func_call]:"
	}
	if sn.prefunc != nil {
		ret += sn.prefunc.dump(indent + 1)
	} else {
		ret += sn.fuc
	}
	ret += "\n"
	if sn.arglist != nil {
		ret += sn.arglist.dump(indent + 1)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type for_stmt struct {
	syntree_node_base
	str        string
	beginblock *block_node
	cmp        *cmp_stmt
	endblock   *block_node
	block      *block_node
}

func (sn *for_stmt) gettype() int {
	return est_for_stmt
}
func (sn *for_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[for]:\n"
	ret += sn.gentab(indent + 1)
	ret += "[beginblock]:\n"
	if sn.beginblock != nil {
		ret += sn.beginblock.dump(indent + 2)
	}
	ret += sn.gentab(indent + 1)
	ret += "[cmp]:\n"
	if sn.cmp != nil {
		ret += sn.cmp.dump(indent + 2)
	}
	ret += sn.gentab(indent + 1)
	ret += "[endblock]:\n"
	if sn.endblock != nil {
		ret += sn.endblock.dump(indent + 2)
	}
	ret += sn.gentab(indent + 1)
	ret += "[block]:\n"
	if sn.block != nil {
		ret += sn.block.dump(indent + 2)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type cmp_stmt struct {
	syntree_node_base
	cmp   string
	left  syntree_node
	right syntree_node
}

func (sn *cmp_stmt) gettype() int {
	return est_cmp_stmt
}
func (sn *cmp_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[cmp]:"
	ret += sn.cmp
	ret += "\n"
	if sn.left != nil {
		ret += sn.gentab(indent + 1)
		ret += "[left]:\n"
		ret += sn.left.dump(indent + 2)
	}
	if sn.right != nil {
		ret += sn.gentab(indent + 1)
		ret += "[right]:\n"
		ret += sn.right.dump(indent + 2)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type variable_node struct {
	syntree_node_base
	str string
}

func (sn *variable_node) gettype() int {
	return est_variable
}
func (sn *variable_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[variable]:"
	ret += sn.str
	ret += "\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type var_node struct {
	syntree_node_base
	str string
}

func (sn *var_node) gettype() int {
	return est_var
}
func (sn *var_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[var]:"
	ret += sn.str
	ret += "\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type assign_stmt struct {
	syntree_node_base
	vr    syntree_node
	value syntree_node
	isnew bool
}

func (sn *assign_stmt) gettype() int {
	return est_assign_stmt
}
func (sn *assign_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[assign]:\n"
	ret += sn.gentab(indent + 1)
	ret += "[var]:\n"
	ret += sn.vr.dump(indent + 2)
	ret += sn.gentab(indent + 1)
	ret += "[value]:\n"
	ret += sn.value.dump(indent + 2)
	return ret
}

//////////////////////////////////////////////////////////////////

type math_assign_stmt struct {
	syntree_node_base
	vr    syntree_node
	oper  string
	value syntree_node
}

func (sn *math_assign_stmt) gettype() int {
	return est_math_assign_stmt
}
func (sn *math_assign_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[mathassign]:"
	ret += sn.oper
	ret += "\n"
	ret += sn.gentab(indent + 1)
	ret += "[var]:\n"
	ret += sn.vr.dump(indent + 2)
	ret += sn.gentab(indent + 1)
	ret += "[value]:\n"
	ret += sn.value.dump(indent + 2)
	return ret
}

//////////////////////////////////////////////////////////////////

type while_stmt struct {
	syntree_node_base
	cmp   *cmp_stmt
	block *block_node
}

func (sn *while_stmt) gettype() int {
	return est_while_stmt
}
func (sn *while_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[while]:\n"
	ret += sn.cmp.dump(indent + 1)
	if sn.block != nil {
		ret += sn.block.dump(indent + 1)
	} else {
		ret += sn.gentab(indent + 1)
		ret += "nil\n"
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type elseif_stmt_list struct {
	syntree_node_base
	stmtlist []syntree_node
}

func (sn *elseif_stmt_list) gettype() int {
	return est_elseif_stmt_list
}
func (sn *elseif_stmt_list) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[elseif_stmt_list]:\n"
	for i := range sn.stmtlist {
		ret += sn.gentab(indent + 1)
		ret += "[stmt"
		ret += strconv.Itoa(i)
		ret += "]:\n"
		ret += sn.stmtlist[i].dump(indent + 2)
	}
	return ret
}
func (sn *elseif_stmt_list) add_stmt(stmt syntree_node) {
	sn.stmtlist = append(sn.stmtlist, stmt)
}

//////////////////////////////////////////////////////////////////

type else_stmt struct {
	syntree_node_base
	block *block_node
}

func (sn *else_stmt) gettype() int {
	return est_else_stmt
}
func (sn *else_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[else]:\n"
	if sn.block != nil {
		ret += sn.block.dump(indent + 1)
	} else {
		ret += sn.gentab(indent + 1)
		ret += "nil\n"
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type if_stmt struct {
	syntree_node_base
	cmp     *cmp_stmt
	block   *block_node
	elseifs *elseif_stmt_list
	elses   *else_stmt
}

func (sn *if_stmt) gettype() int {
	return est_if_stmt
}
func (sn *if_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[if]:\n"
	ret += sn.cmp.dump(indent + 1)
	if sn.block != nil {
		ret += sn.block.dump(indent + 1)
	}
	if sn.elseifs != nil {
		ret += sn.elseifs.dump(indent + 1)
	}
	if sn.elses != nil {
		ret += sn.elses.dump(indent)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type elseif_stmt struct {
	syntree_node_base
	cmp   *cmp_stmt
	block *block_node
}

func (sn *elseif_stmt) gettype() int {
	return est_elseif_stmt
}
func (sn *elseif_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[elseif_stmt]:\n"
	ret += sn.cmp.dump(indent + 1)
	if sn.block != nil {
		ret += sn.block.dump(indent + 1)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type return_stmt struct {
	syntree_node_base
	returnlist *return_value_list_node
}

func (sn *return_stmt) gettype() int {
	return est_return_stmt
}
func (sn *return_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[return]:"
	ret += "\n"
	if sn.returnlist != nil {
		ret += sn.returnlist.dump(indent + 1)
	}
	return ret
}

//////////////////////////////////////////////////////////////////

type return_value_list_node struct {
	syntree_node_base
	returnlist []syntree_node
}

func (sn *return_value_list_node) gettype() int {
	return est_return_value_list
}
func (sn *return_value_list_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[return_value_list]:\n"
	for i := range sn.returnlist {
		ret += sn.gentab(indent + 1)
		ret += sn.returnlist[i].dump(indent + 1)
		ret += "\n"
	}
	return ret
}
func (sn *return_value_list_node) add_arg(arg syntree_node) {
	sn.returnlist = append(sn.returnlist, arg)
}

//////////////////////////////////////////////////////////////////

type var_list_node struct {
	syntree_node_base
	varlist []syntree_node
}

func (sn *var_list_node) gettype() int {
	return est_var_list
}
func (sn *var_list_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[return_value_list]:\n"
	for i := range sn.varlist {
		ret += sn.varlist[i].dump(indent + 1)
	}
	return ret
}
func (sn *var_list_node) add_arg(arg syntree_node) {
	sn.varlist = append(sn.varlist, arg)
}

//////////////////////////////////////////////////////////////////

type multi_assign_stmt struct {
	syntree_node_base
	varlist *var_list_node
	value   syntree_node
	isnew   bool
}

func (sn *multi_assign_stmt) gettype() int {
	return est_multi_assign_stmt
}
func (sn *multi_assign_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[multi_assign]:\n"
	ret += sn.gentab(indent + 1)
	ret += "[var]:\n"
	ret += sn.varlist.dump(indent + 2)
	ret += sn.gentab(indent + 1)
	ret += "[value]:\n"
	ret += sn.value.dump(indent + 2)
	return ret
}

//////////////////////////////////////////////////////////////////

const (
	EVT_TRUE = iota
	EVT_FALSE
	EVT_NUM
	EVT_STR
	EVT_FLOAT
	EVT_UUID
	EVT_MAP
	EVT_ARRAY
	EVT_NULL
)

type explicit_value_node struct {
	syntree_node_base
	str string
	ty  int
	v   syntree_node
}

func (sn *explicit_value_node) gettype() int {
	return est_explicit_value
}
func (sn *explicit_value_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[explicit_value]:"
	ret += sn.str
	ret += "\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type container_get_node struct {
	syntree_node_base
	container string
	key       syntree_node
}

func (sn *container_get_node) gettype() int {
	return est_container_get
}
func (sn *container_get_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[container_get]:\n"
	ret += sn.gentab(indent + 1)
	ret += "[container]:"
	ret += sn.container
	ret += "\n"
	ret += sn.gentab(indent + 1)
	ret += "[key]:\n"
	ret += sn.key.dump(indent + 2)
	return ret
}

//////////////////////////////////////////////////////////////////

type struct_pointer_node struct {
	syntree_node_base
	str string
}

func (sn *struct_pointer_node) gettype() int {
	return est_struct_pointer
}
func (sn *struct_pointer_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[struct_pointer_node]:"
	ret += sn.str
	ret += "\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type math_expr_node struct {
	syntree_node_base
	oper  string
	left  syntree_node
	right syntree_node
}

func (sn *math_expr_node) gettype() int {
	return est_math_expr
}
func (sn *math_expr_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[math_expr]:"
	ret += sn.oper
	ret += "\n"
	ret += sn.gentab(indent + 1)
	ret += "[left]:\n"
	ret += sn.left.dump(indent + 2)
	ret += sn.gentab(indent + 1)
	ret += "[right]:\n"
	ret += sn.right.dump(indent + 2)
	return ret
}

//////////////////////////////////////////////////////////////////

type const_map_list_value_node struct {
	syntree_node_base
	map_ele_node_list []syntree_node
}

func (sn *const_map_list_value_node) gettype() int {
	return est_constmaplist
}
func (sn *const_map_list_value_node) dump(indent int) string {
	ret := ""
	for i := range sn.map_ele_node_list {
		ret += sn.gentab(indent + 1)
		ret += strconv.Itoa(i)
		ret += "]:\n"
		ret += sn.map_ele_node_list[i].dump(indent + 2)
	}
	return ret
}
func (sn *const_map_list_value_node) add_ele(ele syntree_node) {
	sn.map_ele_node_list = append(sn.map_ele_node_list, ele)
}

//////////////////////////////////////////////////////////////////

type const_map_value_node struct {
	syntree_node_base
	k syntree_node
	v syntree_node
}

func (sn *const_map_value_node) gettype() int {
	return est_constmapvalue
}
func (sn *const_map_value_node) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "("
	ret += sn.k.dump(indent + 1)
	ret += ":\n"
	ret += sn.v.dump(indent + 1)
	ret += sn.gentab(indent)
	ret += ")\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type const_array_list_value_node struct {
	syntree_node_base
	array_ele_node_list []syntree_node
}

func (sn *const_array_list_value_node) gettype() int {
	return est_constarraylist
}
func (sn *const_array_list_value_node) dump(indent int) string {
	ret := ""
	for i := range sn.array_ele_node_list {
		ret += sn.gentab(indent + 1)
		ret += strconv.Itoa(i)
		ret += "]:\n"
		ret += sn.array_ele_node_list[i].dump(indent + 2)
	}
	return ret
}
func (sn *const_array_list_value_node) add_ele(ele syntree_node) {
	sn.array_ele_node_list = append(sn.array_ele_node_list, ele)
}

//////////////////////////////////////////////////////////////////

type break_stmt struct {
	syntree_node_base
}

func (sn *break_stmt) gettype() int {
	return est_break
}
func (sn *break_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[break]:\n"
	return ret
}

//////////////////////////////////////////////////////////////////

type continue_stmt struct {
	syntree_node_base
}

func (sn *continue_stmt) gettype() int {
	return est_break
}
func (sn *continue_stmt) dump(indent int) string {
	ret := ""
	ret += sn.gentab(indent)
	ret += "[continue]:\n"
	return ret
}

//////////////////////////////////////////////////////////////////
