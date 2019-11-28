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
	for i, _ := range sn.memlist {
		ret += sn.gentab(indent + 1)
		ret += sn.memlist[i]
		ret += "\n"
	}
	return ret
}
func (sn *struct_desc_memlist_node) add_arg(mem string) {
	sn.memlist = append(sn.memlist, mem)
}
