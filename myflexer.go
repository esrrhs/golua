package fakego

import "github.com/esrrhs/go-engine/src/loggo"

type myflexer struct {
	f           fakeStruct
	fileName    string
	packageName string
}

func (mf *myflexer) add_include(include_file string) {
	loggo.Debug("add include %s", include_file)
}

func (mf *myflexer) add_struct_desc(name string, p *struct_desc_memlist_node) {
	loggo.Debug("add struct %s", name)
}
