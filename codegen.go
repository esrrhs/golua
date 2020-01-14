package fakego

type block_identifiers struct {
	name string
	pos  int
}

type block_identifiers_list []block_identifiers
type block_identifiers_stack []block_identifiers_list
type byte_code_list []command
type byte_lineno_list []int
type const_list []variant
type containeraddr_list []container_addr
type debug_block_identifiers_list []stack_variant_info

type codegen struct {
	stackpos                     int
	maxstackpos                  int
	block_identifiers_stack      block_identifiers_stack
	byte_code_list               byte_code_list
	byte_lineno_list             byte_lineno_list
	const_list                   const_list
	containeraddr_list           containeraddr_list
	debug_block_identifiers_list debug_block_identifiers_list
}

func (cg *codegen) push_stack_identifiers() {
	cg.block_identifiers_stack = append(cg.block_identifiers_stack, block_identifiers_list{})
}

func (cg *codegen) get_cur_variable_pos(name string) int {
	list := cg.block_identifiers_stack[len(cg.block_identifiers_stack)-1]
	for i := range list {
		if name == list[i].name {
			return list[i].pos
		}
	}
	return -1
}

func (cg *codegen) add_stack_identifier(name string, lineno int) int {

	if cg.get_cur_variable_pos(name) != -1 {
		return -1
	}
	list := cg.block_identifiers_stack[len(cg.block_identifiers_stack)-1]
	list = append(list, block_identifiers{name: name, pos: cg.stackpos})
	ret := cg.stackpos
	cg.stackpos++
	if cg.stackpos > cg.maxstackpos {
		cg.maxstackpos = cg.stackpos
	}

	var tmp stack_variant_info
	tmp.name = name
	tmp.line = lineno
	tmp.pos = ret
	cg.debug_block_identifiers_list = append(cg.debug_block_identifiers_list, tmp)
	return ret
}

func (cg *codegen) byte_code_size() int {
	return len(cg.byte_code_list)
}

func (cg *codegen) pop_stack_identifiers() {
	list := cg.block_identifiers_stack[len(cg.block_identifiers_stack)-1]
	stacksize := len(list)
	cg.block_identifiers_stack = cg.block_identifiers_stack[0 : len(cg.block_identifiers_stack)-1]
	cg.stackpos -= stacksize
}

func (cg *codegen) push(code command, lineno int) {
	cg.byte_code_list = append(cg.byte_code_list, code)
	cg.byte_lineno_list = append(cg.byte_lineno_list, lineno)
}

func (cg *codegen) set(pos int, code command) {
	cg.byte_code_list[pos] = code
}
