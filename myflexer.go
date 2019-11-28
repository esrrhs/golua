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
