//go:build cgo && unix

package lua

// LuaSocket, the third real C extension run here after LPeg and CJSON,
// and the one that reaches furthest: where LPeg and CJSON stay inside
// the interpreter, LuaSocket owns sockets, and so it has to be given
// memory of its own that outlives a call -- a t_socket inside a userdata,
// reached through a metatable registered in the registry -- and it has to
// be handed back exactly the pointer it put there.
//
// It found two real bugs, neither of which the smaller extensions touch:
//
//   lua_setmetatable pushed the old metatable to answer whether there was
//   one, and then set the metatable from the top of the stack -- which by
//   then was the answer.  A socket ended up carrying the metatable it had
//   before, so luaL_testudata stopped recognising a server as a
//   tcp{server} and every method call on it raised "bad self".
//
//   luaL_optlstring could not hand back NULL.  LuaSocket's filters call
//   luaL_optlstring(L, 2, NULL, &size) and test the pointer to learn that
//   the stream is over, because '' is an empty chunk and nil is the end.
//   Returning "" for NULL ended the stream at the first empty chunk, and
//   the same test -- comparing the result with "" -- also mistook an
//   argument that *was* the empty string for one that had been left out.
//
// The tests run over loopback, so the machine needs no network: TCP
// connect/send/receive, a timeout, select, UDP, getaddrinfo, and the two
// filters of mime (base64 and quoted-printable) through ltn12.  Every
// line is PUC-Lua 5.4.9's, kept in testdata/luasocket/expected.txt.

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// sockSources builds socket.core; mimeSources builds mime.core.  They are
// the split luasocket's own makefile makes, and the two halves of what
// 'require "socket"' and 'require "mime"' ask for.
var sockSources = []string{
	"luasocket.c", "auxiliar.c", "buffer.c", "compat.c", "except.c",
	"inet.c", "io.c", "options.c", "select.c", "tcp.c", "timeout.c",
	"udp.c", "usocket.c", "unix.c", "unixdgram.c", "unixstream.c",
}

var mimeSources = []string{"mime.c"}

// sockHeaders is every header the sources above include, wsocket.h among
// them: it is the Windows one, and a source picks it with an #if.
var sockHeaders = []string{
	"auxiliar.h", "buffer.h", "compat.h", "except.h", "inet.h", "io.h",
	"luasocket.h", "mime.h", "options.h", "pierror.h", "select.h",
	"socket.h", "tcp.h", "timeout.h", "udp.h", "unix.h", "unixdgram.h",
	"unixstream.h", "usocket.h", "wsocket.h",
}

// sockLua is the Lua half of the module: socket.lua requires
// socket.core, so the .so has to be found as 'socket/core.so'.
var sockLua = []string{"socket.lua", "mime.lua", "ltn12.lua"}

// sockSupport is testsupport.lua, which the suites require: it is part of
// LuaSocket's test/ and not of the module.
var sockSupport = []string{"testsupport.lua"}

// sockTests is every suite in LuaSocket's own test/ that needs neither
// the network nor a server of another kind: the ones that do (ftp, http,
// smtp, dict, tftp) are left out.
var sockTests = []string{
	"mimetest.lua", "ltn12test.lua", "stufftest.lua", "excepttest.lua",
	"test_bind.lua", "test_socket_error.lua", "test_getaddrinfo.lua",
	"test_tcp_receive_zero.lua", "test_udp_receive_zero.lua",
	"socket_e2e.lua",
}

func TestLuaSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("luasocket builds from wsocket.c there and needs winsock, which is not set up for this test")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/luasocket")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "src", "luasocket.c")); err != nil {
		t.Skipf("testdata/luasocket is not there (%v)", err)
	}
	inc, err := filepath.Abs("include")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	work := filepath.Join(dir, "test")
	for _, d := range []string{build, work,
		filepath.Join(dir, "socket"), filepath.Join(dir, "mime")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range append(append([]string{}, sockHeaders...),
		append(append([]string{}, sockSources...), mimeSources...)...) {
		if err := copyFile(filepath.Join(src, "src", f), filepath.Join(build, f)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range sockLua {
		if err := copyFile(filepath.Join(src, "src", f), filepath.Join(dir, f)); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range append(append([]string{}, sockTests...), sockSupport...) {
		if err := copyFile(filepath.Join(src, "test", f), filepath.Join(work, f)); err != nil {
			t.Fatal(err)
		}
	}

	/* socket.core and mime.core, each where 'require' wants it */
	if !buildSockModule(t, cc, inc, build, "socket", sockSources) {
		return
	}
	if !buildSockModule(t, cc, inc, build, "mime", mimeSources) {
		return
	}

	/* The suites read nothing by relative name, but running them from
	   their own directory is what their runtests.sh does. */
	t.Chdir(work)

	/* Each suite gets its own state, the way each one gets its own
	   process when the reference interpreter runs them: some of them
	   leave globals behind, and 'stufftest' ends with os.exit. */
	var got []string
	for _, name := range sockTests {
		got = append(got, "== "+name)
		out, err := runSockSuite(dir, filepath.Join(work, name))
		got = append(got, out...)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	compareLines(t, stableSockLines(got), stableSockLines(sockWant(t, src)))
}

// runSockSuite runs one suite in a state of its own and answers with the
// lines it printed.
func runSockSuite(dir, script string) ([]string, error) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		/* mimetest writes the name of the case with io.write and its
		   verdict with print, so the two meet on one line of the real
		   output: 'testing identity: ok'.  print is captured here, so
		   io.write is made to leave what it is given in __pend for
		   print to pick up -- a line is only complete when print ends
		   it, exactly as the newline is what ends it on a terminal. */
		const pendName = "__pend"
		L.GetGlobal(pendName)
		pend, _ := L.ToString(-1)
		L.Pop(1)
		L.PushString("")
		L.SetGlobal(pendName)
		n := L.GetTop()
		var b strings.Builder
		b.WriteString(pend)
		for i := 1; i <= n; i++ {
			if i > 1 {
				b.WriteByte('\t')
			}
			b.WriteString(lTolstring(L, i))
		}
		got = append(got, strings.Split(b.String(), "\n")...)
		return 0
	})
	L.SetGlobal("print")

	/* 'arg' is the interpreter's, not the language's.  Two of the suites
	   end with os.exit -- stufftest with 0, and test_socket_error with 0
	   when it passes and 1 when it does not -- so os.exit is made to
	   raise carrying the status: a test binary cannot end, and a no-op
	   would let test_socket_error run on into the "Fail!" it prints
	   after the loop it exits from. */
	if err := L.DoString("arg = {}\n" +
		"__pend = ''\n" +
		"io.write = function(...)\n" +
		"  local t = {}\n" +
		"  for i = 1, select('#', ...) do t[i] = tostring((select(i, ...))) end\n" +
		"  __pend = __pend .. table.concat(t)\n" +
		"end\n" +
		"os.exit = function(code)\n" +
		"  error('" + sockExit + "' .. tostring(code or 0))\n" +
		"end\n" +
		"package.cpath = " + quoteLuaString(filepath.Join(dir, "?.so")) +
		"\npackage.path = " + quoteLuaString(filepath.Join(dir, "?.lua")) + ";"); err != nil {
		return got, err
	}
	err := L.DoFile(script)
	if err != nil && strings.Contains(err.Error(), sockExit+"0") {
		err = nil /* the suite ended the way it was meant to */
	}
	return got, err
}

// sockExit is the marker the raised os.exit carries, so that a status of
// 0 can be told from one that is not.
const sockExit = "__luasocket_exit_"

// buildSockModule compiles one of the two cores into dir/name/core.so.
func buildSockModule(t *testing.T, cc, inc, build, name string, files []string) bool {
	t.Helper()
	args := []string{"-O2", "-std=c99", "-shared", "-fPIC", "-I" + inc,
		"-o", filepath.Join(filepath.Dir(build), name, "core.so")}
	for _, f := range files {
		args = append(args, filepath.Join(build, f))
	}
	if hasUndefinedDynamicLookup() {
		args = append(args, "-undefined", "dynamic_lookup")
	}
	if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
		t.Skipf("cannot build %s here (%v): %s", name, err, out)
		return false
	}
	return true
}

// stableSockLines drops what a run cannot pin down: the one line of
// mimetest that prints how long it took.
func stableSockLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "done in ") {
			continue
		}
		out = append(out, l)
	}
	return out
}

func sockWant(t *testing.T, src string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}
