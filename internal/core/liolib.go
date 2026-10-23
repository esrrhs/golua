// Translation of liolib.c (PUC-Lua 5.4.9): the standard I/O (and popen)
// library. Line-faithful to PUC 5.4.9 including __close and io.lines
// to-be-closed multi-return.
//
// A file handle is a full userdata wrapping a *ioStream (the Go
// counterpart of luaL_Stream); its metatable is registered in the
// registry under ioFileHandle ("FILE*"), exactly as in C.
//
// Platform / Go-only FILE* emulation (bufio, weak uw, cgo luaL_Stream
// block, popen via os/exec, canR/canW, nobuf std streams) lives here and
// in liolib_cgo.go — see NOTES.md.
package core

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
	"weak"
)

// ioStreamClosed is called when the handle is closed: the FILE* is
// released and the block cleared, which is what lets an extension see a
// closed handle the way PUC-Lua shows it one.  It is nil without cgo.
var ioStreamClosed func(s *ioStream)

// ioIsWindows tells whether the host runs Windows (the shell used by
// io.popen and os.execute depends on it).
func ioIsWindows() bool { return runtime.GOOS == "windows" }

const (
	ioPrefix     = "_IO_"
	ioInputKey   = ioPrefix + "input"  // registry key of the default input
	ioOutputKey  = ioPrefix + "output" // registry key of the default output
	ioFileHandle = "FILE*"             // metatable name of file handles
	ioMaxArgLine = 250                 // max number of arguments to 'lines'
	ioMaxLenNum  = 200                 // maximum length of a numeral
	ioBufSize    = 4096                // LUAL_BUFFERSIZE
)

// ioStream is the Go counterpart of luaL_Stream.
//
// 'u' and 'cfp' are what make the handle work for an extension: C is
// handed the address of a block laid out the way luaL_Stream is (see
// cdlbridge.h), with a real FILE* in it, so that an extension which
// casts a file handle to a luaL_Stream -- LuaFileSystem's setmode does --
// finds what it expects instead of a zeroed block or, worse, nothing.
type ioStream struct {
	/* 'u' is weak, and that is the whole of it.

	   Go's collector never finalises an object that is part of a
	   cycle.  Two objects that point at each other, one of them with
	   a finalizer, are never collected at all -- not after one
	   cycle, not after ten (checked on go1.27, where the pair below
	   prints nothing:

	       a.b = b; b.a = a; runtime.SetFinalizer(a, ...); a, b = nil

	   ).  A handle that pointed back at its userdata strongly would
	   therefore be exactly such a pair: 'u.val' is this stream and
	   this stream's 'u' is the userdata.  '__gc' would never run for
	   a file, and a handle dropped with bytes still in its buffer
	   would lose them instead of flushing them the way 'fclose'
	   does.  Weak is what makes the cycle a tree. */
	uw      weak.Pointer[Udata] // the userdata behind this handle
	cfp     unsafe.Pointer      // the FILE* in it, nil if there is none
	f       *os.File            // the file; nil for a popen handle
	rd      io.ReadCloser       // popen: the child's standard output
	wr      io.WriteCloser      // popen: the child's standard input
	cmd     *exec.Cmd           // popen: the child process
	tmpname string              // io.tmpfile: removed when the handle is closed
	vbuf    string              // file:setvbuf mode ("no"/"full"/"line")
	vbufsz  int                 // setvbuf size; 0 → ioBufSize
	canR    bool                // C's __SRD: the FILE was opened for reading
	canW    bool                // C's __SWR: the FILE was opened for writing
	closef  GoFunc              // close function; nil marks a closed handle
	buf     *bufio.Reader       // read buffer (Go files are unbuffered)
	wbuf    *bufio.Writer       // write buffer: a FILE has one, and so must we
	nobuf   bool                // a standard stream: see writer()
	rderr   error               // the error flag of the FILE ('ferror')
}

// ---------------------------------------------------------------------
// ioStream helpers
// ---------------------------------------------------------------------

func (s *ioStream) isClosed() bool { return s.closef == nil }

// ioBadFdReader is a FILE that was not opened for reading.  C's
// 'cantread' fails before a byte moves and leaves EBADF behind, so the
// read errors out the way a read error does -- through the error flag
// of the FILE -- rather than reaching the descriptor, which for a
// 'popen' handle writing to a child is a pipe that would answer
// something else entirely.
type ioBadFdReader struct{}

func (ioBadFdReader) Read(p []byte) (int, error) { return 0, ioErrBadFd }

func (s *ioStream) reader() io.Reader {
	if !s.canR {
		return ioBadFdReader{}
	}
	if s.f != nil {
		return s.f
	}
	if s.rd != nil {
		return s.rd
	}
	return nil
}

// writer is the stream buffered for writing, which is what C's 'fwrite'
// writes to: a FILE accumulates up to LUAL_BUFFERSIZE bytes and only then
// reaches the descriptor.  Without it every 'io.write' of a small piece is
// a system call of its own, and a program that writes a large file one
// field at a time spends nearly all of its time in the kernel.
func (s *ioStream) writer() io.Writer {
	var raw io.Writer
	switch {
	case s.f != nil:
		raw = s.f
	case s.wr != nil:
		raw = s.wr
	default:
		return nil
	}
	/* A standard stream is not buffered here.  In C 'print' and
	   'io.write' share one FILE, so their output interleaves in program
	   order however that FILE is buffered; here 'print' writes to the
	   descriptor itself, so buffering this stream on its own would send
	   what 'io.write' wrote out of order. */
	if s.nobuf {
		return raw
	}
	if s.wbuf == nil {
		sz := s.vbufsz
		if sz <= 0 {
			sz = ioBufSize
		}
		s.wbuf = bufio.NewWriterSize(raw, sz)
	}
	return s.wbuf
}

// flush empties the write buffer (io.fclose and 'flush' do this).
func (s *ioStream) flush() error {
	if s.wbuf == nil {
		return nil
	}
	return s.wbuf.Flush()
}

// writeString writes a piece of text to the stream.
func (s *ioStream) writeString(str string) error {
	/* 'cantwrite(f)' first: the FILE knows it was not opened for
	   writing, so 'io.open("x", "rb"):write("y")' fails at once with
	   EBADF.  A Go file only fails at the system call, and the write
	   buffer swallows a short piece before it gets there. */
	if !s.canW {
		return ioErrBadFd
	}
	w := s.writer()
	if w == nil {
		return ioWriteError
	}
	_, err := w.Write([]byte(str))
	if err != nil {
		return err
	}
	/* Approximate _IOLBF: flush when the piece contains a newline. */
	if s.vbuf == "line" && !s.nobuf && strings.IndexByte(str, '\n') >= 0 {
		return s.flush()
	}
	return nil
}

// ioErrWatch is the reader the read buffer is filled from, with the
// error flag of the FILE beside it.  A FILE remembers that a read failed
// and 'g_read' asks it once every format has been read (liolib.c):
//
//	clearerr(f);
//	errno = 0;
//	...
//	if (ferror(f))
//	  return luaL_fileresult(L, 0, NULL);
//
// so reading a file that was opened for writing only answers nil, "Bad
// file descriptor", 9 -- not the empty string a readable file at its end
// answers.  Reaching the end of the file is not an error.
type ioErrWatch struct {
	r    io.Reader
	flag *error
}

func (w *ioErrWatch) Read(p []byte) (int, error) {
	n, err := w.r.Read(p)
	if err != nil && err != io.EOF {
		*w.flag = err
	}
	return n, err
}

// bufferedReader returns the read buffer of the stream, creating it when
// needed (in C this is the buffer of the FILE object).
func (s *ioStream) bufferedReader() *bufio.Reader {
	if s.buf == nil {
		s.buf = bufio.NewReaderSize(&ioErrWatch{r: s.reader(), flag: &s.rderr}, ioBufSize)
	}
	return s.buf
}

// doClose closes the underlying file (io.fclose).
func (s *ioStream) doClose() error {
	s.buf = nil
	if s.f == nil {
		return nil
	}
	/* The FILE* has a descriptor of its own, so releasing it cannot come
	   before the file itself is closed: the flush goes through the Go
	   file, and closing the FILE first would close the descriptor the
	   flush is about to write to. */
	err := s.flush() /* 'fclose' flushes what is still in the buffer */
	if cerr := s.f.Close(); err == nil {
		err = cerr
	}
	if ioStreamClosed != nil {
		ioStreamClosed(s) /* the FILE* C was shown goes with the file */
	}
	if s.tmpname != "" {
		os.Remove(s.tmpname)
		s.tmpname = ""
	}
	return err
}

// waitPipe closes a popen handle and waits for the child (io.pclose).
func (s *ioStream) waitPipe() (what string, code int, err error) {
	if s.wr != nil {
		/* 'pclose' closes a FILE and 'fclose' flushes what is still in
		   its buffer, so the writing end has to be emptied before it is
		   closed: closing it on its own drops the last bufferful, and
		   'io.popen("wc -c", "w")' then counts nothing at all.  A
		   flush that fails is what C reports through 'pclose', so it
		   is answered as an error rather than as the child's status. */
		ferr := s.flush()
		s.wr.Close()
		if ferr != nil {
			s.cmd.Wait()
			return "", 0, ferr
		}
	} else if s.rd != nil {
		s.rd.Close()
	}
	werr := s.cmd.Wait()
	what, code = ioProcessStatus(werr)
	if werr != nil {
		var ee *exec.ExitError
		if !errors.As(werr, &ee) { /* not an exit status: an I/O error */
			return what, code, werr
		}
	}
	return what, code, nil
}

// ---------------------------------------------------------------------
// auxlib helpers missing from lauxlib.go (luaL_fileresult and friends)
// ---------------------------------------------------------------------

// ioErrno extracts a C-like errno from an error (0 when there is none).
// ioFileError renders a failed file operation the way lauxlib.c's
// 'errfile' does, for the messages of 'loadfile' and 'dofile':
// "cannot open x: No such file or directory".  A Go error would read
// "open x: no such file or directory", which is neither.
func ioFileError(what, fname string, err error) string {
	if ioErrno(err) == 0 { /* C leaves the reason out when errno is 0 */
		return "cannot " + what + " " + fname
	}
	return "cannot " + what + " " + fname + ": " + ioStrError(err)
}

// ioCErrno is an error that carries the errno C would have set, for the
// failures that a C library raises itself and a Go one never reaches.
type ioCErrno struct {
	en  int64
	msg string
}

func (e ioCErrno) Error() string { return e.msg }

// ioErrBadFd is what a FILE answers when it is used the wrong way
// round: 'cantwrite' and 'cantread' fail before a single byte moves, and
// both leave EBADF behind.  The number is 9 on every platform we build
// for, and 'strerror' spells it the same way too.
var ioErrBadFd = ioCErrno{en: 9, msg: "bad file descriptor"}

func ioErrno(err error) int64 {
	var en syscall.Errno
	if errors.As(err, &en) {
		return int64(en)
	}
	var ce ioCErrno
	if errors.As(err, &ce) {
		return ce.en
	}
	return 0
}

// ioCapitalise raises the first letter of a message: Go spells a system
// error the way 'strerror' does, only in lower case ("no such file or
// directory" instead of "No such file or directory").
func ioCapitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ioStrError renders an error of a file operation the way C does, with
// 'strerror(errno)'.  Go wraps the system error in a *fs.PathError whose
// text reads "open /x: no such file or directory": only the errno part
// is what C reports, and it is what must reach the Lua program.
func ioStrError(err error) string {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	/* The errno's own spelling first, because it is the only one that
	   is right for the errnos Go groups together.  'Errno.Is' answers
	   that ENOTEMPTY *is* fs.ErrExist and that EPERM *is*
	   fs.ErrPermission, which is true of what a Go program is asking
	   and false of what 'strerror' says:
	     strerror(ENOTEMPTY)  "Directory not empty"
	     strerror(EPERM)     "Operation not permitted"
	   A rename of one directory onto another that is not empty, or a
	   'remove' of a directory with something in it, and the port used
	   to answer "File exists" for both.

	   Go spells an errno exactly as C does, with the first letter in
	   lower case, so raising that one letter is all it takes. */
	var en syscall.Errno
	if errors.As(err, &en) {
		return ioCapitalise(en.Error())
	}
	/* An error with no errno in it.  That is EBADF, which a FILE
	   answers on its own for a handle used the wrong way round and
	   which comes with its spelling already.  It used to be asked
	   with 'errors.Is' against 'fs.ErrNotExist' and the other two,
	   and those three were also the ones that could never be
	   reached: an error that satisfied any of them had an errno
	   inside it, and the branch above had already answered. */
	return ioCapitalise(err.Error())
}

// ioRename is C's 'rename' (loslib.c's os_rename), not Go's os.Rename.
//
// Go's os.Rename answers EEXIST for a rename whose new name is a
// directory *before* it ever calls rename(2): it stats the new name,
// sees a directory, and gives up (os/file_unix.go, and deliberately --
// "if ofi, err := Lstat(oldname); err != nil ... else if newname ==
// oldname || !SameFile(fi, ofi) { return &LinkError{... EEXIST} }").
// Three answers of PUC-Lua's depend on the call it decided not to
// make, and two of them are not errors at all:
//
//	os.rename("f", "d")    PUC-Lua: nil, "Is a directory", 21
//	                       (Go:     nil, "File exists", 17)
//	os.rename("d1", "d2")  PUC-Lua: true -- the empty d2 is replaced
//	                       (Go:     nil, "File exists", 17)
//	os.rename("d1", "d1")  PUC-Lua: true
//
// The last is the case-only rename the short-circuit exists for, and it
// is why the call has to be made here rather than answered: what the
// two do about a rename onto a directory is not a difference in a
// number, it is a difference in whether the file moved.
func ioRename(oldpath, newpath string) error {
	if runtime.GOOS == "windows" {
		/* no rename(2) there: MoveFileEx is what a move is, and this
		   port keeps Go's, which does replace the target. */
		return os.Rename(oldpath, newpath)
	}
	return syscall.Rename(oldpath, newpath)
}

// ioFileResult is luaL_fileresult: true, or nil + message + errno.
//
// 'fname' is a pointer, because C asks whether it is NULL and not
// whether it is empty (lauxlib.c):
//
//	if (fname != NULL) lua_pushfstring(L, "%s: %s", fname, strerror(en));
//	else               lua_pushfstring(L, "%s", strerror(en));
//
// so 'io.open("")' reports ": No such file or directory", with the
// colon and the space and nothing in front of them, where a nil here
// used to drop the prefix entirely and answer without it.
func ioFileResult(L *State, stat bool, fname *string, err error) int {
	if stat {
		L.PushBoolean(true)
		return 1
	}
	en := ioErrno(err)
	msg := "(no extra info)" /* C: 'en != 0 ? strerror(en) : ...' */
	if en != 0 {
		msg = ioStrError(err)
	}
	if fname != nil {
		msg = *fname + ": " + msg
	}
	L.PushNil()
	L.PushString(msg)
	L.PushInteger(en)
	return 3
}

// ioExecResult is luaL_execresult: true/fail, "exit"/"signal", code.
func ioExecResult(L *State, what string, code int) int {
	if what == "exit" && code == 0 {
		L.PushBoolean(true)
	} else {
		L.PushNil()
	}
	L.PushString(what)
	L.PushInteger(int64(code))
	return 3
}

// ioProcessStatus interprets the result of a finished child process
// (l_inspectstat in loslib.c).
func ioProcessStatus(err error) (what string, code int) {
	if err == nil {
		return "exit", 0
	}
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return "exit", -1
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if ok && ws.Signaled() {
		return "signal", int(ws.Signal())
	}
	return "exit", ee.ExitCode()
}

// ioNewMetaTable is luaL_newmetatable: it leaves the (possibly new)
// metatable registered under name on the stack.
func ioNewMetaTable(L *State, name string) bool {
	L.GetField(LUA_REGISTRYINDEX, name)
	if !L.IsNil(-1) {
		return false /* name already in use */
	}
	L.Pop(1)
	L.NewTable()
	/* luaL_newmetatable also names the metatable: '__name' is what
	   makes 'getmetatable(io.input()).__name' be "FILE*" and is used
	   in error messages about file handles. */
	L.PushString(name)
	L.SetField(-2, "__name")
	L.PushValue(-1)
	L.SetField(LUA_REGISTRYINDEX, name)
	return true
}

// ioSetMetaTableByName is luaL_setmetatable: it sets the metatable
// registered under name on the value at the top of the stack.
func ioSetMetaTableByName(L *State, name string) {
	L.GetField(LUA_REGISTRYINDEX, name)
	L.SetMetaTable(-2)
}

// ioTestUData is luaL_testudata: it returns the Go value carried by the
// userdata at idx when it has the metatable name, nil otherwise.
func ioTestUData(L *State, idx int, name string) any {
	p := L.ToUserData(idx)
	if p == nil {
		return nil
	}
	if L.GetMetaTable(idx) {
		L.GetField(LUA_REGISTRYINDEX, name)
		if !L.RawEqual(-1, -2) {
			p = nil /* userdata with the wrong metatable */
		}
		L.Pop(2)
	}
	return p
}

// ioToLStreamAt is tolstream for an arbitrary index: it returns the
// stream at idx, raising an error when the value is not a file handle.
func ioToLStreamAt(L *State, idx int) *ioStream {
	s, _ := ioTestUData(L, idx, ioFileHandle).(*ioStream)
	if s == nil {
		/* C's tolstream calls luaL_checkudata, whose error goes
		   through luaL_typeerror -- which is what reports '__name'
		   ("FILE* expected, got My Type") instead of "table". */
		L.typeError(idx, ioFileHandle) /* does not return */
	}
	return s
}

// ioToLStream is tolstream: the file handle at index 1.
func ioToLStream(L *State) *ioStream { return ioToLStreamAt(L, 1) }

// ioToFile is tofile: like ioToLStream, but also rejects closed handles.
func ioToFile(L *State) *ioStream {
	s := ioToLStream(L)
	if s.isClosed() {
		L.Error2("attempt to use a closed file")
	}
	return s
}

// ---------------------------------------------------------------------
// creation of file handles
// ---------------------------------------------------------------------

// ioOpenStreams is every file handle that has been created, held weakly.
//
// C's 'exit' flushes the buffer of every stdio stream that is still
// open, so 'os.exit' has to do the same: a script that writes to a file
// and then calls 'os.exit' without closing it keeps what it wrote in C,
// and would lose it here.  The handles are held weakly so that the list
// does not keep them -- and their buffers -- alive itself.
var (
	ioOpenStreamsMu sync.Mutex
	ioOpenStreams   []weak.Pointer[ioStream]
	ioOpenNextPrune = ioOpenMax
)

// ioOpenMax is how long the list is allowed to get before the handles
// that have gone are dropped from it.
const ioOpenMax = 1024

func ioTrackStream(s *ioStream) {
	ioOpenStreamsMu.Lock()
	ioOpenStreams = append(ioOpenStreams, weak.Make(s))
	n := len(ioOpenStreams)
	prune := n >= ioOpenNextPrune
	if prune {
		// Live handles cannot be pruned. Back off before releasing the
		// lock so a run that creates many states does not copy the whole
		// live list again on every subsequent handle.
		ioOpenNextPrune = n * 2
	}
	ioOpenStreamsMu.Unlock()
	if prune {
		ioPruneOpen()
	}
}

// ioPruneOpen drops the entries whose handle has gone.
func ioPruneOpen() {
	ioOpenStreamsMu.Lock()
	old := ioOpenStreams
	ioOpenStreams = nil
	ioOpenStreamsMu.Unlock()
	keep := make([]weak.Pointer[ioStream], 0, len(old))
	for i := range old {
		if old[i].Value() == nil {
			continue
		}
		keep = append(keep, old[i])
	}
	ioOpenStreamsMu.Lock()
	ioOpenStreams = append(ioOpenStreams, keep...)
	if n := len(ioOpenStreams); n < ioOpenNextPrune/4 {
		ioOpenNextPrune = ioOpenMax
		if n*2 > ioOpenNextPrune {
			ioOpenNextPrune = n * 2
		}
	}
	ioOpenStreamsMu.Unlock()
}

// ioFlushAll writes out what is still in the buffers of the open
// handles.  It is what C's 'exit' does on the way out.
func ioFlushAll() {
	ioOpenStreamsMu.Lock()
	old := ioOpenStreams
	ioOpenStreams = nil
	ioOpenStreamsMu.Unlock()
	keep := make([]weak.Pointer[ioStream], 0, len(old))
	for i := range old {
		if s := old[i].Value(); s != nil {
			s.flush()
			keep = append(keep, old[i])
		}
	}
	ioOpenStreamsMu.Lock()
	ioOpenStreams = append(ioOpenStreams, keep...)
	ioOpenStreamsMu.Unlock()
}

// ioNewPreFile creates a closed file handle (newprefile).
func ioNewPreFile(L *State) *ioStream {
	/* Both ways round until the mode says otherwise: a handle an
	   extension hands over carries a real FILE*, and what that FILE
	   was opened for is not ours to guess. */
	s := &ioStream{canR: true, canW: true}
	ioTrackStream(s)
	L.NewUserData(s, 0) /* 5.4.9: lua_newuserdatauv(L, sizeof(LStream), 0) */
	/* weak: see the note on the field */
	s.uw = weak.Make(uvalue(&L.stack[L.top-1])) /* the handle C is handed */
	ioSetMetaTableByName(L, ioFileHandle)
	return s
}

// ioNewFile creates a file handle whose close function is io.fclose
// (newfile).
func ioNewFile(L *State) *ioStream {
	s := ioNewPreFile(L)
	s.closef = ioFClose
	return s
}

// ioCheckMode checks whether mode matches '[rwa]%+?b*' (l_checkmode).
//
// The only extension C accepts is 'b' (L_MODEEXT): "rt" and "r+t" are
// refused with "invalid mode" even though every C library takes them,
// because 'l_checkmode' does not ask the library, it spells the grammar
// out.  It is what makes 'io.open(f, "rt")' fail here the way it fails
// in PUC-Lua rather than succeeding the way it would in C.
func ioCheckMode(mode string) bool {
	if len(mode) == 0 {
		return false
	}
	switch mode[0] {
	case 'r', 'w', 'a':
	default:
		return false
	}
	rest := mode[1:]
	if len(rest) > 0 && rest[0] == '+' {
		rest = rest[1:]
	}
	for i := 0; i < len(rest); i++ { /* extensions */
		if rest[i] != 'b' {
			return false
		}
	}
	return true
}

// ioCStr is luaCStr: a name or a mode is handed to 'fopen', which is a
// C function and so sees no more than the bytes before the first zero.
// A mode is the same: 'l_checkmode' stops at the end of the string, so
// "r\0b" is a valid mode.
func ioCStr(s string) string { return luaCStr(s) }

// ioSetDirection records what a FILE was opened for, the way 'fopen'
// sets __SRD and __SWR (ioSetFlags is given the flags of ioFlags).
func ioSetDirection(s *ioStream, flag int) {
	s.canR = flag&os.O_RDWR != 0 || flag&os.O_WRONLY == 0
	s.canW = flag&(os.O_RDWR|os.O_WRONLY) != 0
}

// ioCheckModeP is l_checkmodep. Default (POSIX / ISO) accepts only "r"
// or "w". Under LUA_USE_WINDOWS, C also accepts "[rw][bt]?".
func ioCheckModeP(mode string) bool {
	if len(mode) == 0 {
		return false
	}
	if mode[0] != 'r' && mode[0] != 'w' {
		return false
	}
	if ioIsWindows() {
		/* Windows: (m[0]=='r'||m[0]=='w') && (m[1]=='\0' || ((m[1]=='b'||m[1]=='t') && m[2]=='\0')) */
		if len(mode) == 1 {
			return true
		}
		return len(mode) == 2 && (mode[1] == 'b' || mode[1] == 't')
	}
	return len(mode) == 1
}

// ioFlags translates a Lua open mode into os.OpenFile flags.
func ioFlags(mode string) int {
	var flag int
	switch mode[0] {
	case 'r':
		flag = os.O_RDONLY
	case 'w':
		flag = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	case 'a':
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	if len(mode) > 1 && mode[1] == '+' {
		flag &^= os.O_RDONLY | os.O_WRONLY
		flag |= os.O_RDWR
		switch mode[0] {
		case 'w':
			flag |= os.O_CREATE | os.O_TRUNC
		case 'a':
			flag |= os.O_CREATE | os.O_APPEND
		}
	}
	return flag
}

// ioOpenFile opens a file honouring the Lua mode string.
func ioOpenFile(fname, mode string) (*os.File, error) {
	f, err := os.OpenFile(fname, ioFlags(mode), 0666)
	if err != nil {
		return nil, err
	}
	/* Where an append mode leaves the file position is the C
	   library's business, and it is not the same for the two of them:
	   see ioAppendAtEnd. */
	if ioAppendAtEnd(mode) {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// ioAppendAtEnd says whether opening a file in this mode leaves the
// file position at the end, which 'O_APPEND' never does -- it decides
// where a *write* goes and leaves the position at zero.
//
// "a" both libraries answer the same way: at the end, so
// 'io.open(f, "ab"):seek()' is the size of f.
//
// "a+" they do not agree on, and C asks only that a write land at the
// end -- where the position sits *before* anything has been written is
// the library's own business. Apple's puts it at the end, so
// 'io.open(f, "a+"):read("a")' answers nothing at all; glibc leaves it
// at zero, and the same line answers the whole of f. Which of the two
// this port answers is therefore the host's, exactly as PUC-Lua's is,
// and the grid -- which asks the reference on the machine it runs on --
// is answered alike on both. Only the platforms that have been
// measured are named; the rest are left at zero, which is glibc's.
func ioAppendAtEnd(mode string) bool {
	if len(mode) == 0 || mode[0] != 'a' {
		return false
	}
	if len(mode) > 1 && mode[1] == '+' {
		return runtime.GOOS == "darwin"
	}
	return true
}

// ioOpenCheck opens a file, raising an error when it fails (opencheck).
func ioOpenCheck(L *State, fname, mode string) {
	s := ioNewFile(L)
	f, err := ioOpenFile(fname, mode)
	if err != nil {
		L.Error2("cannot open file '%s' (%s)", fname, ioStrError(err))
	}
	s.f = f
	ioSetDirection(s, ioFlags(mode)) /* match fopen mode (canR/canW) */
}

// ---------------------------------------------------------------------
// READ
// ---------------------------------------------------------------------

// ioNumReader reads the prefix of a numeral out of a stream (RN in
// liolib.c).
type ioNumReader struct {
	r    *bufio.Reader
	c    int    // current character (look ahead); -1 at end of file
	n    int    // number of characters in buff
	buff []byte // the numeral being built
}

func ioNewNumReader(r *bufio.Reader) *ioNumReader {
	rn := &ioNumReader{r: r, buff: make([]byte, 0, ioMaxLenNum+1)}
	b, err := r.ReadByte()
	if err != nil {
		rn.c = -1
	} else {
		rn.c = int(b)
	}
	return rn
}

// next adds the current character to the buffer and reads the next one
// (nextc).
func (rn *ioNumReader) next() bool {
	if rn.n >= ioMaxLenNum { /* buffer overflow? */
		rn.buff = rn.buff[:0] /* invalidate result */
		return false
	}
	rn.buff = append(rn.buff, byte(rn.c))
	b, err := rn.r.ReadByte()
	if err != nil {
		rn.c = -1
	} else {
		rn.c = int(b)
	}
	return true
}

// test2 accepts the current character when it is one of the two given
// ones (test2).
func (rn *ioNumReader) test2(set string) bool {
	if rn.c >= 0 && (rn.c == int(set[0]) || rn.c == int(set[1])) {
		return rn.next()
	}
	return false
}

// digits reads a sequence of (hexa)decimal digits (readdigits).
func (rn *ioNumReader) digits(hex bool) int {
	count := 0
	for rn.c >= 0 {
		if hex {
			if !lisxdigit(byte(rn.c)) {
				break
			}
		} else if rn.c < '0' || rn.c > '9' {
			break
		}
		if !rn.next() {
			break
		}
		count++
	}
	return count
}

// ioReadNumber reads a number from the stream (read_number).
func ioReadNumber(L *State, s *ioStream) bool {
	if s.reader() == nil {
		L.PushNil()
		return false
	}
	rn := ioNewNumReader(s.bufferedReader())
	for rn.c >= 0 && lisspace(byte(rn.c)) { /* skip spaces */
		b, err := rn.r.ReadByte()
		if err != nil {
			rn.c = -1
		} else {
			rn.c = int(b)
		}
	}
	count := 0
	hex := false
	/* C: decp[0]=lua_getlocaledecpoint(); decp[1]='.' */
	decp := string([]byte{localeDecPoint[0], '.'})
	rn.test2("-+")
	if rn.test2("00") {
		if rn.test2("xX") {
			hex = true /* numeral is hexadecimal */
		} else {
			count = 1 /* count initial '0' as a valid digit */
		}
	}
	count += rn.digits(hex) /* integral part */
	if rn.test2(decp) {     /* decimal point? */
		count += rn.digits(hex) /* fractional part */
	}
	if count > 0 && rn.test2(ioExpMark(hex)) { /* exponent mark? */
		rn.test2("-+")   /* exponent sign */
		rn.digits(false) /* exponent digits */
	}
	if rn.c >= 0 {
		rn.r.UnreadByte() /* unread look-ahead char */
	}
	var v Value
	if luaO_str2num(string(rn.buff), &v) {
		L.checkStack(1)
		L.stack[L.top] = v
		L.top++
		return true
	}
	L.PushNil() /* "result" to be removed */
	return false
}

func ioExpMark(hex bool) string {
	if hex {
		return "pP"
	}
	return "eE"
}

// ioTestEOF returns false at the end of the file (test_eof).
func ioTestEOF(L *State, s *ioStream) bool {
	L.PushString("")
	if s.reader() == nil {
		return false
	}
	br := s.bufferedReader()
	if _, err := br.ReadByte(); err != nil {
		return false
	}
	br.UnreadByte() /* no-op when the read failed */
	return true
}

// ioReadLine reads a line, chopping (or keeping) the end-of-line
// (read_line).
func ioReadLine(L *State, s *ioStream, chop bool) bool {
	var sb strings.Builder
	c := -1
	if s.reader() != nil {
		br := s.bufferedReader()
		for {
			b, err := br.ReadByte()
			if err != nil {
				c = -1
				break
			}
			if b == '\n' {
				c = int(b)
				break
			}
			sb.WriteByte(b)
		}
	}
	if !chop && c == '\n' { /* want a newline and have one? */
		sb.WriteByte('\n')
	}
	L.PushString(sb.String())
	/* return ok if read something (either a newline or something else) */
	return c == '\n' || sb.Len() > 0
}

// ioReadAll reads the whole (remaining) file (read_all).
func ioReadAll(L *State, s *ioStream) {
	if s.reader() == nil {
		L.PushString("")
		return
	}
	b, _ := io.ReadAll(s.bufferedReader())
	s.buf = nil
	L.PushString(string(b))
}

// ioReadChars reads n characters (read_chars).
func ioReadChars(L *State, s *ioStream, n int64) bool {
	if n < 0 {
		/* C hands 'n' to the allocator as a 'size_t', so a negative
		   count asks for SIZE_MAX bytes and no allocator has those:
		   'file:read(-1)' reports "not enough memory". */
		luaG_runerror(L, "not enough memory")
	}
	if s.reader() == nil {
		L.PushString("")
		return false
	}
	/* C: luaL_prepbuffsize(&b, n) then fread of n. Counts too large to
	   allocate fail with the same OOM message (closer than empty+fail). */
	if n > int64(1)<<30 {
		luaG_runerror(L, "not enough memory")
	}
	buf := make([]byte, n)
	nr, _ := io.ReadFull(s.bufferedReader(), buf)
	L.PushString(string(buf[:nr]))
	return nr > 0 /* true iff read something */
}

// ioGRead is g_read: the engine behind io.read and file:read.
func ioGRead(L *State, s *ioStream, first int) int {
	nargs := L.GetTop() - 1
	n := first
	success := true
	s.rderr = nil   /* clearerr(f) ... errno = 0 */
	if nargs == 0 { /* no arguments? */
		success = ioReadLine(L, s, true)
		n = first + 1 /* to return 1 result */
	} else {
		/* ensure stack space for all results and for auxlib's buffer */
		L.CheckStack2(nargs+LUA_MINSTACK, "too many arguments")
		for ; nargs > 0 && success; nargs-- {
			if L.Type(n) == LUA_TNUMBER {
				l := L.CheckInteger(n)
				if l == 0 {
					success = ioTestEOF(L, s)
				} else {
					success = ioReadChars(L, s, l)
				}
			} else {
				p := L.CheckString(n)
				if len(p) > 0 && p[0] == '*' { /* skip optional '*' */
					p = p[1:]
				}
				if len(p) == 0 {
					return L.ArgError(n, "invalid format")
				}
				switch p[0] {
				case 'n': /* number */
					success = ioReadNumber(L, s)
				case 'l': /* line */
					success = ioReadLine(L, s, true)
				case 'L': /* line with end-of-line */
					success = ioReadLine(L, s, false)
				case 'a': /* file */
					ioReadAll(L, s) /* read entire file */
					success = true  /* always success */
				default:
					return L.ArgError(n, "invalid format")
				}
			}
			n++
		}
	}
	if s.rderr != nil { /* ferror(f) */
		return ioFileResult(L, false, nil, s.rderr)
	}
	if !success {
		L.Pop(1)    /* remove last result */
		L.PushNil() /* push fail instead */
	}
	return n - first
}

func ioRead(L *State) int {
	return ioGRead(L, ioGetIOFile(L, ioInputKey), 1)
}

func ioFRead(L *State) int {
	return ioGRead(L, ioToFile(L), 2)
}

// ---------------------------------------------------------------------
// WRITE
// ---------------------------------------------------------------------

// ioGWrite is g_write: the engine behind io.write and file:write.
func ioGWrite(L *State, s *ioStream, arg int) int {
	nargs := L.GetTop() - arg
	status := true
	var werr error
	for ; nargs > 0; nargs-- {
		var str string
		if L.Type(arg) == LUA_TNUMBER {
			if L.IsInteger(arg) {
				v, _ := L.ToInteger(arg)
				str = fmt.Sprintf(LUA_INTEGER_FMT, v)
			} else {
				v, _ := L.ToNumber(arg)
				str = fmtFloat(v)
			}
		} else {
			str = L.CheckString(arg)
		}
		err := s.writeString(str)
		if err != nil && werr == nil {
			werr = err /* C reports the errno of the first failure */
		}
		status = status && err == nil
		arg++
	}
	if status {
		return 1 /* file handle already on stack top */
	}
	return ioFileResult(L, false, nil, werr)
}

var ioWriteError = errors.New("error writing to file")

func ioWriteString(w io.Writer, s string) (int, error) {
	if w == nil {
		return 0, ioWriteError
	}
	return w.Write([]byte(s))
}

func ioWrite(L *State) int {
	return ioGWrite(L, ioGetIOFile(L, ioOutputKey), 1)
}

func ioFWrite(L *State) int {
	s := ioToFile(L)
	L.PushValue(1) /* push file at the stack top (to be returned) */
	return ioGWrite(L, s, 2)
}

// ---------------------------------------------------------------------
// default files (input/output)
// ---------------------------------------------------------------------

// ioGetIOFile pushes the default file registered under findex and
// returns its stream (getiofile). The pushed value is left on the stack,
// which is what the callers expect (see ioWrite/ioRead).
// C uses lua_touserdata (no metatable check); registry entries are ours.
func ioGetIOFile(L *State, findex string) *ioStream {
	L.GetField(LUA_REGISTRYINDEX, findex)
	s := L.ToUserData(-1).(*ioStream) /* C: (LStream *)lua_touserdata(L, -1) */
	if s.isClosed() {
		/* findex + IOPREF_LEN → "input" / "output" */
		L.Error2("default %s file is closed", findex[len(ioPrefix):])
	}
	return s
}

// ioGIOFile is g_iofile: the engine behind io.input and io.output.
func ioGIOFile(L *State, f, mode string) int {
	if !L.IsNoneOrNil(1) {
		if filename, ok := L.ToString(1); ok {
			ioOpenCheck(L, ioCStr(filename), mode)
		} else {
			ioToFile(L) /* check that it's a valid file handle */
			L.PushValue(1)
		}
		L.SetField(LUA_REGISTRYINDEX, f)
	}
	L.GetField(LUA_REGISTRYINDEX, f)
	return 1
}

func ioInput(L *State) int  { return ioGIOFile(L, ioInputKey, "r") }
func ioOutput(L *State) int { return ioGIOFile(L, ioOutputKey, "w") }

// ---------------------------------------------------------------------
// library functions
// ---------------------------------------------------------------------

func ioType(L *State) int {
	L.CheckAny(1)
	p := ioTestUData(L, 1, ioFileHandle)
	if p == nil {
		L.PushNil() /* not a file; luaL_pushfail ≡ nil by default */
	} else if p.(*ioStream).isClosed() {
		L.PushString("closed file")
	} else {
		L.PushString("file")
	}
	return 1
}

func ioFToString(L *State) int {
	s := ioToLStream(L)
	if s.isClosed() {
		L.PushString("file (closed)")
	} else {
		/* C: lua_pushfstring(L, "file (%p)", p->f) */
		var p any
		switch {
		case s.f != nil:
			p = s.f
		case s.cfp != nil:
			p = s.cfp
		default:
			p = s /* popen / incomplete: no FILE* */
		}
		L.PushString(fmt.Sprintf("file (%p)", p))
	}
	return 1
}

// ioAuxClose calls the close function of a file handle (aux_close).
func ioAuxClose(L *State) int {
	s := ioToLStream(L)
	cf := s.closef /* C: volatile lua_CFunction cf = p->closef */
	s.closef = nil /* mark stream as closed */
	if cf == nil {
		/* C would call NULL; callers (f_close/f_gc) reject closed handles */
		return 0
	}
	return cf(L) /* close it */
}

func ioFClose2(L *State) int {
	ioToFile(L) /* make sure argument is an open stream */
	return ioAuxClose(L)
}

func ioClose(L *State) int {
	if L.IsNone(1) { /* no argument? */
		L.GetField(LUA_REGISTRYINDEX, ioOutputKey) /* use default output */
	}
	return ioFClose2(L)
}

func ioFGc(L *State) int {
	s := ioToLStream(L)
	/* C: !isclosed(p) && p->f != NULL — ignore closed and incompletely
	   open files (closef set, f still NULL). Go popen has no FILE* f;
	   rd/wr are the open-stream stand-in (not cmd alone: a failed
	   Start before pipes would otherwise GC-close unlike C). */
	if !s.isClosed() && (s.f != nil || s.rd != nil || s.wr != nil) {
		ioAuxClose(L)
	}
	return 0
}

// ioFClose is the close function of regular files (io_fclose).
func ioFClose(L *State) int {
	s := ioToLStream(L)
	err := s.doClose()
	return ioFileResult(L, err == nil, nil, err)
}

// ioPClose is the close function of 'popen' files (io_pclose).
func ioPClose(L *State) int {
	s := ioToLStream(L)
	what, code, err := s.waitPipe()
	if err != nil {
		return ioFileResult(L, false, nil, err)
	}
	return ioExecResult(L, what, code)
}

// ioNoClose is the "close" function of the standard files.
func ioNoClose(L *State) int {
	s := ioToLStream(L)
	s.closef = ioNoClose /* keep file opened */
	L.PushNil()
	L.PushString("cannot close standard file")
	return 2
}

func ioOpen(L *State) int {
	filename := ioCStr(L.CheckString(1))
	mode := ioCStr(L.OptString(2, "r"))
	s := ioNewFile(L)
	md := mode /* to traverse/check mode */
	L.ArgCheck(ioCheckMode(md), 2, "invalid mode")
	f, err := ioOpenFile(filename, mode)
	if err != nil {
		return ioFileResult(L, false, &filename, err)
	}
	s.f = f
	ioSetDirection(s, ioFlags(mode))
	return 1
}

func ioPOpen(L *State) int {
	/* 'popen' takes a C string, so it is the bytes before the first
	   zero that are handed to the shell. */
	filename := luaCStr(L.CheckString(1))
	mode := ioCStr(L.OptString(2, "r"))
	/* C: newprefile then l_checkmodep then l_popen then closef=io_pclose */
	s := ioNewPreFile(L)
	L.ArgCheck(ioCheckModeP(mode), 2, "invalid mode")
	cmd := ioShellCommand(filename)
	var err error
	/* Direction from mode[0]: Windows may pass "rb"/"rt"/"wb"/"wt". */
	if mode[0] == 'r' {
		s.rd, err = cmd.StdoutPipe()
		cmd.Stderr = os.Stderr
		s.canW = false /* a pipe to the child's output: read only */
	} else { /* 'w' — ArgCheck already rejected other modes */
		s.wr, err = cmd.StdinPipe()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		s.canR = false /* a pipe to the child's input: write only */
	}
	if err == nil {
		/* POSIX l_popen: (fflush(NULL), popen(...)) — flush every open
		   stdio write buffer before the child inherits descriptors.
		   Windows _popen has no fflush(NULL); harmless extra flush. */
		ioFlushAll()
		err = cmd.Start()
	}
	/* C sets closef even when l_popen returns NULL; f_gc skips f==NULL. */
	s.cmd = cmd
	s.closef = ioPClose
	if err != nil {
		return ioFileResult(L, false, &filename, err)
	}
	return 1
}

// ioShellCommand builds the command run by io.popen and os.execute.
func ioShellCommand(command string) *exec.Cmd {
	if ioIsWindows() {
		return exec.Command("cmd", "/c", command)
	}
	return exec.Command("sh", "-c", command)
}

func ioTmpFile(L *State) int {
	s := ioNewFile(L)
	f, err := os.CreateTemp("", "lua_")
	if err != nil {
		return ioFileResult(L, false, nil, err)
	}
	s.f = f
	s.tmpname = f.Name()
	/* os.CreateTemp opens for reading and writing, which is what
	   'tmpfile' does ("w+b"). */
	s.canR, s.canW = true, true
	return 1
}

func ioFlush(L *State) int {
	s := ioGetIOFile(L, ioOutputKey)
	err := s.flush()
	return ioFileResult(L, err == nil, nil, err)
}

func ioFFlush(L *State) int {
	s := ioToFile(L)
	err := s.flush()
	return ioFileResult(L, err == nil, nil, err)
}

// ---------------------------------------------------------------------
// 'lines'
// ---------------------------------------------------------------------

// ioAuxLines creates the iteration function for 'lines' (aux_lines).
func ioAuxLines(L *State, toclose bool) {
	n := L.GetTop() - 1 /* number of arguments to read */
	L.ArgCheck(n <= ioMaxArgLine, ioMaxArgLine+2, "too many arguments")
	L.PushValue(1)          /* file */
	L.PushInteger(int64(n)) /* number of arguments to read */
	L.PushBoolean(toclose)  /* close/not close file when finished */
	L.Rotate(2, 3)          /* move the three values to their positions */
	L.PushGoClosure(ioReadline, 3+n)
}

func ioFLines(L *State) int {
	ioToFile(L) /* check that it's a valid file handle */
	ioAuxLines(L, false)
	return 1
}

func ioLines(L *State) int {
	toclose := false
	if L.IsNone(1) {
		L.PushNil() /* at least one argument */
	}
	if L.IsNil(1) { /* no file name? */
		L.GetField(LUA_REGISTRYINDEX, ioInputKey) /* get default input */
		L.Replace(1)                              /* put it at index 1 */
		ioToFile(L)                               /* check that it's a valid file handle */
	} else { /* open a new file */
		filename := ioCStr(L.CheckString(1))
		ioOpenCheck(L, filename, "r")
		L.Replace(1)   /* put file at index 1 */
		toclose = true /* close it after iteration */
	}
	ioAuxLines(L, toclose) /* push iteration function */
	if toclose {
		L.PushNil()    /* state */
		L.PushNil()    /* control */
		L.PushValue(1) /* file is the to-be-closed variable (4th result) */
		return 4
	}
	return 1
}

// ioReadline is the iteration function of 'lines'.
func ioReadline(L *State) int {
	s, _ := L.ToUserData(UpvalueIndex(1)).(*ioStream)
	n64, _ := L.ToInteger(UpvalueIndex(2))
	n := int(n64)
	if s == nil || s.isClosed() { /* file is already closed? */
		return L.Error2("file is already closed")
	}
	L.SetTop(1)
	L.CheckStack2(n, "too many arguments")
	for i := 1; i <= n; i++ { /* push arguments to 'g_read' */
		L.PushValue(UpvalueIndex(3 + i))
	}
	n = ioGRead(L, s, 2) /* 'n' is number of results */
	if L.ToBoolean(-n) { /* read at least one value? */
		return n /* return them */
	}
	if n > 1 { /* is there error information? */
		/* 2nd result is error message */
		return L.Error2("%s", L.ToStringX(-n+1))
	}
	if L.ToBoolean(UpvalueIndex(3)) { /* generator created file? */
		L.SetTop(0)                  /* clear stack */
		L.PushValue(UpvalueIndex(1)) /* push file at index 1 */
		ioAuxClose(L)                /* close it */
	}
	return 0
}

// ---------------------------------------------------------------------
// file methods
// ---------------------------------------------------------------------

func ioFSeek(L *State) int {
	modenames := []string{"set", "cur", "end"}
	s := ioToFile(L)
	op := L.CheckOption(2, "cur", modenames)
	p3 := L.OptInteger(3, 0)
	offset := p3 /* l_seeknum: int64 here (POSIX off_t / MSVC __int64) */
	L.ArgCheck(offset == p3, 3, "not an integer in proper range")
	if s.f == nil {
		/* A pipe: 'fseek' asks the descriptor, which answers ESPIPE
		   ("Illegal seek", 29) -- the same on every platform. */
		return ioFileResult(L, false, nil,
			ioCErrno{en: 29, msg: "illegal seek"})
	}
	pos, err := s.seek(offset, op)
	if err != nil {
		return ioFileResult(L, false, nil, err)
	}
	L.PushInteger(pos)
	return 1
}

// seek moves the stream the way C's 'fseek' does and returns the new
// position the way 'ftell' does.  The base of a "cur" seek is the
// *logical* position, which stops short of the descriptor's: bytes that
// a 'read' already pulled into the buffer but that the caller has not
// consumed yet are not part of it, while the descriptor has moved past
// them.  Any seek drops the buffer, as 'fseek' does.
func (s *ioStream) seek(offset int64, op int) (int64, error) {
	whence := []int{io.SeekStart, io.SeekCurrent, io.SeekEnd}[op]
	if err := s.flush(); err != nil { /* 'fseek' writes out the buffer first */
		return 0, err
	}
	/* A read that went through the buffer has left the descriptor
	   ahead of the file position the Lua program is at, by however
	   much is still in the buffer: 'f:read("l"); f:seek("cur", 0)'
	   answers the position after the line, not after what the
	   descriptor has read.  Asking the descriptor to move by less
	   is how the two are brought back together. */
	if n := s.buffered(); n > 0 && op == 1 {
		if adj := offset - n; adj <= offset {
			offset = adj
		} else { /* 'math.mininteger', which would wrap */
			offset = math.MinInt64 /* C answers EINVAL for it as well */
		}
	}
	n, err := s.f.Seek(offset, whence)
	if err != nil {
		/* A seek that fails changes nothing, and the buffer is part
		   of what it does not change: 'f:read("l"); f:seek("set", -1)'
		   fails and the rest of the file is still there to read. */
		return n, err
	}
	s.buf = nil /* 'fseek' discards whatever has been buffered */
	return n, nil
}

// buffered is how much of the file the read buffer is holding.
func (s *ioStream) buffered() int64 {
	if s.buf == nil {
		return 0
	}
	return int64(s.buf.Buffered())
}

func ioFSetVBuf(L *State) int {
	modenames := []string{"no", "full", "line"}
	s := ioToFile(L)
	/* C passes NULL as the default (liolib.c), and 'luaL_checkoption'
	   asks for the argument outright when the default is NULL:
	      const char *name = (def != NULL) ? luaL_optstring(L, arg, def)
	                                       : luaL_checkstring(L, arg);
	   so 'f:setvbuf()' reports "string expected, got no value" rather
	   than "invalid option ''". */
	op := L.CheckOption(2, L.CheckString(2), modenames)
	sz := L.OptInteger(3, ioBufSize)
	if sz < 0 {
		sz = 0
	}
	s.vbuf = modenames[op]
	s.vbufsz = int(sz)
	switch s.vbuf {
	case "no": /* _IONBF */
		s.nobuf = true
		s.buf = nil
		s.wbuf = nil
	case "full", "line": /* _IOFBF / _IOLBF */
		s.nobuf = false
		s.buf = nil
		s.wbuf = nil /* recreate with vbufsz on next write */
	}
	/* C setvbuf returns non-zero on failure; Go's bufio always succeeds. */
	return ioFileResult(L, true, nil, nil)
}

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

var ioLib = []Reg{
	{"close", ioClose},
	{"flush", ioFlush},
	{"input", ioInput},
	{"lines", ioLines},
	{"open", ioOpen},
	{"output", ioOutput},
	{"popen", ioPOpen},
	{"read", ioRead},
	{"tmpfile", ioTmpFile},
	{"type", ioType},
	{"write", ioWrite},
}

// ioMeth is meth[] in liolib.c (5.4.9): methods for file handles.
var ioMeth = []Reg{
	{"read", ioFRead},
	{"write", ioFWrite},
	{"lines", ioFLines},
	{"flush", ioFFlush},
	{"seek", ioFSeek},
	{"close", ioFClose2},
	{"setvbuf", ioFSetVBuf},
}

// ioMetaMeth is metameth[] in liolib.c (5.4.9): __index placeholder
// overwritten below; __gc and __close both call f_gc.
var ioMetaMeth = []Reg{
	{"__index", nil}, /* placeholder */
	{"__gc", ioFGc},
	{"__close", ioFGc},
	{"__tostring", ioFToString},
}

// ioCreateMeta creates the metatable of file handles (createmeta, Lua 5.4.9).
// Metamethods on the metatable; methods on a separate table as __index.
func ioCreateMeta(L *State) {
	ioNewMetaTable(L, ioFileHandle) /* metatable for file handles */
	L.SetFuncs(ioMetaMeth, 0)       /* add metamethods to new metatable */
	L.CreateTable(0, len(ioMeth))   /* create method table */
	L.SetFuncs(ioMeth, 0)           /* add file methods to method table */
	L.SetField(-2, "__index")       /* metatable.__index = method table */
	L.Pop(1)                        /* pop metatable */
}

// ioCreateStdFile adds a standard file to the module and, when k is not
// empty, to the registry (createstdfile).
func ioCreateStdFile(L *State, f *os.File, k, fname string) {
	s := ioNewPreFile(L)
	s.f = f
	s.closef = ioNoClose
	if f == os.Stdout || f == os.Stderr {
		s.nobuf = true /* see writer() */
		s.canR = false /* __SWR only, as C's stdout and stderr are */
	} else {
		s.canW = false /* __SRD only, as C's stdin is */
	}
	if k != "" {
		L.PushValue(-1)
		L.SetField(LUA_REGISTRYINDEX, k) /* add file to registry */
	}
	L.SetField(-2, fname) /* add file to module */
}

// OpenIO opens the I/O library (luaopen_io).
func OpenIO(L *State) int {
	L.NewLib(ioLib) /* new module */
	ioCreateMeta(L)
	/* create (and set) default files */
	ioCreateStdFile(L, os.Stdin, ioInputKey, "stdin")
	ioCreateStdFile(L, os.Stdout, ioOutputKey, "stdout")
	ioCreateStdFile(L, os.Stderr, "", "stderr")
	/* C luaopen_io leaves the module on the stack only; the global is
	   set by luaL_requiref(..., 1) / OpenLibs — not here. */
	return 1
}
