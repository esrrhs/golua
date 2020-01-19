package fakego

import (
	"fmt"
	"runtime"
)

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
	stack    []uintptr
}

func newFakeErr() *FakeErr {
	fe := &FakeErr{}
	stack := make([]uintptr, 50)
	length := runtime.Callers(2, stack[:])
	fe.stack = stack[:length]
	return fe
}

func (fe *FakeErr) Error() string {
	frames := runtime.CallersFrames(fe.stack)
	stack := ""
	for i := 0; i < len(fe.stack); i++ {
		frame, more := frames.Next()
		str := fmt.Sprintf("%s:%d %s \n", frame.File, frame.Line, frame.Function)
		stack += str
		if !more {
			break
		}
	}
	return fmt.Sprintf("%s(%s:%d):%s\ncall stack:\n%s", fe.funcname, fe.file, fe.lineno, fe.error, stack)
}
