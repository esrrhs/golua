package fakego

import "fmt"

var gfs fakeStruct

type fakeStruct struct {
	cfg FakeConfig
	pa  parser
	con container
	bin binary
	fm  funcmap
}

type FakeErr struct {
	file     string
	lineno   int
	funcname string
	error    string
}

func (fe *FakeErr) Error() string {
	return fmt.Sprintf("%s(%s:%d):%s", fe.funcname, fe.file, fe.lineno, fe.error)
}
