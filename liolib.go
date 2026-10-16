// Translation of liolib.c (PUC-Lua 5.4): the standard I/O (and popen)
// library.
//
// A file handle is a full userdata wrapping a *ioStream (the Go
// counterpart of luaL_Stream); its metatable is registered in the
// registry under ioFileHandle ("FILE*"), exactly as in C.
package lua

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"weak"
)

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
type ioStream struct {
	f       *os.File       // the file; nil for a popen handle
	rd      io.ReadCloser  // popen: the child's standard output
	wr      io.WriteCloser // popen: the child's standard input
	cmd     *exec.Cmd      // popen: the child process
	tmpname string         // io.tmpfile: removed when the handle is closed
	vbuf    string         // file:setvbuf mode (informational)
	closef  GoFunc         // close function; nil marks a closed handle
	buf     *bufio.Reader  // read buffer (Go files are unbuffered)
	wbuf    *bufio.Writer  // write buffer: a FILE has one, and so must we
	nobuf   bool           // a standard stream: see writer()
	rderr   error          // the error flag of the FILE ('ferror')
}

// ---------------------------------------------------------------------
// ioStream helpers
// ---------------------------------------------------------------------

func (s *ioStream) isClosed() bool { return s.closef == nil }

func (s *ioStream) reader() io.Reader {
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
		s.wbuf = bufio.NewWriterSize(raw, ioBufSize)
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
	w := s.writer()
	if w == nil {
		return ioWriteError
	}
	_, err := w.Write([]byte(str))
	return err
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
	err := s.flush() /* 'fclose' flushes what is still in the buffer */
	if cerr := s.f.Close(); err == nil {
		err = cerr
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

func ioErrno(err error) int64 {
	var en syscall.Errno
	if errors.As(err, &en) {
		return int64(en)
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
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "No such file or directory"
	case errors.Is(err, fs.ErrPermission):
		return "Permission denied"
	case errors.Is(err, fs.ErrExist):
		return "File exists"
	}
	return ioCapitalise(err.Error())
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
)

// ioOpenMax is how long the list is allowed to get before the handles
// that have gone are dropped from it.
const ioOpenMax = 1024

func ioTrackStream(s *ioStream) {
	ioOpenStreamsMu.Lock()
	ioOpenStreams = append(ioOpenStreams, weak.Make(s))
	n := len(ioOpenStreams)
	ioOpenStreamsMu.Unlock()
	if n >= ioOpenMax {
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
	s := &ioStream{}
	ioTrackStream(s)
	L.NewUserData(s, 0)
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

// ioCheckMode checks whether mode matches '[rwa]%+?[bt]*' (l_checkmode).
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
		if rest[i] != 'b' && rest[i] != 't' {
			return false
		}
	}
	return true
}

// ioCheckModeP checks a mode for 'popen': only "r" or "w" are valid.
func ioCheckModeP(mode string) bool {
	return (mode == "r" || mode == "w")
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
	return os.OpenFile(fname, ioFlags(mode), 0666)
}

// ioOpenCheck opens a file, raising an error when it fails (opencheck).
func ioOpenCheck(L *State, fname, mode string) {
	s := ioNewFile(L)
	f, err := ioOpenFile(fname, mode)
	if err != nil {
		L.Error2("cannot open file '%s' (%s)", fname, ioStrError(err))
	}
	s.f = f
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
	decp := ".." /* the decimal point; always accept a dot */
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
	if n > int64(1)<<31 || s.reader() == nil {
		/* deviation: a count that large is read as end of file instead
		   of being handed to the allocator, which C would do. */
		L.PushString("")
		return false
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
		status = status && err == nil
		arg++
	}
	if status {
		return 1 /* file handle already on stack top */
	}
	return ioFileResult(L, false, nil, ioWriteError)
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
func ioGetIOFile(L *State, findex string) *ioStream {
	L.GetField(LUA_REGISTRYINDEX, findex)
	s := ioToLStreamAt(L, L.AbsIndex(-1))
	if s.isClosed() {
		L.Error2("default %s file is closed", findex[len(ioPrefix):])
	}
	return s
}

// ioGIOFile is g_iofile: the engine behind io.input and io.output.
func ioGIOFile(L *State, f, mode string) int {
	if !L.IsNoneOrNil(1) {
		if filename, ok := L.ToString(1); ok {
			ioOpenCheck(L, filename, mode)
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
		L.PushNil() /* not a file */
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
		L.PushString(fmt.Sprintf("file (%p)", s))
	}
	return 1
}

// ioAuxClose calls the close function of a file handle (aux_close).
func ioAuxClose(L *State) int {
	s := ioToLStream(L)
	cf := s.closef
	s.closef = nil /* mark stream as closed */
	if cf == nil {
		return 0
	}
	return cf(L)
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
	if !s.isClosed() {
		ioAuxClose(L) /* ignore closed and incompletely open files */
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
	filename := L.CheckString(1)
	mode := L.OptString(2, "r")
	L.ArgCheck(ioCheckMode(mode), 2, "invalid mode")
	s := ioNewFile(L)
	f, err := ioOpenFile(filename, mode)
	if err != nil {
		return ioFileResult(L, false, &filename, err)
	}
	s.f = f
	return 1
}

func ioPOpen(L *State) int {
	filename := L.CheckString(1)
	mode := L.OptString(2, "r")
	L.ArgCheck(ioCheckModeP(mode), 2, "invalid mode")
	s := ioNewPreFile(L)
	cmd := ioShellCommand(filename)
	var err error
	if mode == "r" {
		s.rd, err = cmd.StdoutPipe()
		cmd.Stderr = os.Stderr
	} else {
		s.wr, err = cmd.StdinPipe()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		return ioFileResult(L, false, &filename, err)
	}
	s.cmd = cmd
	s.closef = ioPClose
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
	return 1
}

func ioFlush(L *State) int {
	s := ioGetIOFile(L, ioOutputKey)
	return ioFileResult(L, s.flush() == nil, nil, nil)
}

func ioFFlush(L *State) int {
	s := ioToFile(L)
	return ioFileResult(L, s.flush() == nil, nil, nil)
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
		filename := L.CheckString(1)
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
	offset := L.OptInteger(3, 0)
	if s.f == nil {
		return ioFileResult(L, false, nil, errors.New("not a seekable file"))
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
	if s.buf != nil {
		buffered := int64(s.buf.Buffered())
		s.buf = nil /* 'fseek' discards whatever has been buffered */
		if op == 1 {
			pos, err := s.f.Seek(0, io.SeekCurrent)
			if err != nil {
				return 0, err
			}
			offset += pos - buffered
			whence = io.SeekStart
		}
	}
	return s.f.Seek(offset, whence)
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
	L.OptInteger(3, ioBufSize)
	s.vbuf = modenames[op]
	if s.vbuf == "no" {
		s.buf = nil /* disabling buffering drops the read buffer */
	}
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

var ioMeth = []Reg{
	{"read", ioFRead},
	{"write", ioFWrite},
	{"lines", ioFLines},
	{"flush", ioFFlush},
	{"seek", ioFSeek},
	{"close", ioFClose2},
	{"setvbuf", ioFSetVBuf},
}

// ioMetaMeth are the metamethods of file handles; "__index" is filled
// below with the method table (createmeta).
var ioMetaMeth = []Reg{
	{"__gc", ioFGc},
	{"__close", ioFGc},
	{"__tostring", ioFToString},
}

// ioCreateMeta creates the metatable of file handles (createmeta).
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
	L.PushValue(-1)
	L.SetGlobal("io")
	return 1
}
