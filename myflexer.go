package fakego

import (
	"sync"
)

type myflexer struct {
	fileName    string
	packageName string
	includelist []string
	constmap    sync.Map // string->*explicit_value_node
	funclist    []*func_desc_node
	struct_list sync.Map // string->[]string
}

func (mf *myflexer) set_package(package_name string) {
	mf.packageName = package_name
}

func (mf *myflexer) get_package() string {
	return mf.packageName
}

func (mf *myflexer) add_include(include_file string) {
	log_debug("add include %s", include_file)
	mf.includelist = append(mf.includelist, include_file)
}

func (mf *myflexer) add_struct_desc(name string, p *struct_desc_memlist_node) {
	log_debug("add struct %s %d", name, p.lineno())
	mf.struct_list.Store(name, p.memlist)
}

func (mf *myflexer) add_const_desc(name string, p syntree_node) {
	log_debug("add const %s %d", name, p.lineno())
	ev := p.(*explicit_value_node)
	log_debug("add_const_desc %s = %s", name, ev.dump(0))
	mf.constmap.Store(name, ev)
}

func (mf *myflexer) get_const_map() sync.Map {
	return mf.constmap
}

func (mf *myflexer) add_func_desc(p *func_desc_node) {
	log_debug("add func %s %d", p.funcname, p.lineno())
	if isOpenLog() {
		log_debug("dump func %s \n%s", p.funcname, p.dump(0))
	}
	mf.funclist = append(mf.funclist, p)
}

func (mf *myflexer) get_func_list() []*func_desc_node {
	return mf.funclist
}

func (mf *myflexer) getfilename() string {
	return mf.fileName
}

func (mf *myflexer) is_have_struct(name string) bool {
	_, ok := mf.struct_list.Load(name)
	return ok
}

func (mf *myflexer) is_have_func(funcname string) bool {
	for i, _ := range mf.funclist {
		p := mf.funclist[i]
		if p.funcname == funcname {
			return true
		}
	}
	return false
}
