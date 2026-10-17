//go:build cgo && unix

package lua

// luaposix, the fifth real C extension run here.  Where LuaSocket owns
// memory and LuaFileSystem reaches into a file handle, this one is a
// *survey*: thirty small modules that between them call most of the C
// API, and whose answers are the host's own -- errno numbers, struct tm,
// struct stat, a directory listing.
//
// It found two things, and both were in the headers rather than in the
// port:
//
//   include/lauxlib.h had no LUA_FILEHANDLE, the name a file handle's
//   metatable is registered under.  posix.stdio could not be compiled
//   at all without it, and an extension that tests whether a value is a
//   file handle -- or makes one for io to use -- needs it too.
//
//   A dozen more macros of lua.h and lauxlib.h were missing: LUA_SIGNATURE,
//   LUA_GNAME, LUA_NUMTAGS, LUA_RIDX_LAST, LUA_VERSION_RELEASE_NUM,
//   LUA_AUTHORS, LUA_COPYRIGHT, luaL_intop, luaL_opt, lua_writestring and
//   its two companions, and lua_getextraspace.  The last one is the only
//   interesting one: it reaches *behind* the lua_State* C was handed, so
//   the block behind that pointer now really has LUA_EXTRASPACE bytes in
//   front of the handle, the way PUC-Lua's does.
//
//   The deprecated integer casts (luaL_checkint and friends) are provided
//   too, but behind LUA_COMPAT_APIINTCASTS the way PUC-Lua's luaconf.h
//   has it -- off unless asked for, so that what an extension sees by
//   default is exactly what it would see against PUC-Lua.
//
// Nine of the modules are run -- the ones whose answers do not depend on
// which machine or which second they were asked on.  posix.unistd and
// posix.sys.socket do not build on this host (they name _POSIX_PATH_MAX
// and IFNAMSIZ, which macOS does not define), and posix.stdio needs a
// FILE* the port does not keep; the rest are left out because they answer
// with the hostname, the clock, or the size of a disk.
//
// The suite makes files, a directory and a link, so it runs in a
// temporary directory of its own.  Every line it prints is PUC-Lua
// 5.4.9's, kept in testdata/luaposix/expected.txt.  MIT-licensed,
// redistributed under testdata/luaposix/LICENSE.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lpModules are the modules built and run, as "file:subdir": every one
// is a separate .so, because that is how luaposix ships them.
var lpModules = []struct{ src, sub string }{
	{"ctype.c", ""}, {"dirent.c", ""}, {"errno.c", ""}, {"fcntl.c", ""},
	{"fnmatch.c", ""},
	{"glob.c", ""}, {"libgen.c", ""}, {"time.c", ""}, {"utime.c", ""},
	{"stat.c", "sys"},
}

func TestLuaPosix(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("no C compiler in PATH")
	}
	src, err := filepath.Abs("testdata/luaposix")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "_helpers.c")); err != nil {
		t.Skipf("testdata/luaposix is not there (%v)", err)
	}
	inc, err := filepath.Abs("include")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	build := filepath.Join(dir, "build")
	work := filepath.Join(dir, "test")
	for _, d := range []string{build, filepath.Join(build, "sys"),
		work, filepath.Join(work, "posix"), filepath.Join(work, "posix", "sys")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{"_helpers.c", "test.lua"}
	for _, m := range lpModules {
		files = append(files, filepath.Join(m.sub, m.src))
	}
	for _, f := range files {
		if err := copyFile(filepath.Join(src, f), filepath.Join(build, f)); err != nil {
			t.Fatal(err)
		}
	}

	/* Each module is compiled on its own: they share nothing but
	   '_helpers.c', which they #include rather than link. */
	for _, m := range lpModules {
		name := strings.TrimSuffix(m.src, ".c")
		args := []string{"-O2", "-std=c99", "-shared", "-fPIC", "-I" + build,
			"-I" + inc, "-DPACKAGE=\"luaposix\"", "-DVERSION=\"36.3\"",
			"-o", filepath.Join(work, "posix", m.sub, name+".so"),
			filepath.Join(build, m.sub, m.src)}
		if hasUndefinedDynamicLookup() {
			args = append(args, "-undefined", "dynamic_lookup")
		}
		if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
			t.Skipf("cannot build posix.%s here (%v): %s", name, err, out)
		}
	}

	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()

	var got []string
	L.PushGoFunc(func(L *State) int {
		n := L.GetTop()
		var b strings.Builder
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

	/* It makes files, a directory and a link where it runs, and one of
	   its error messages names the file it could not open, so it runs
	   in a temporary directory of its own. */
	t.Chdir(work)
	if err := L.DoString("package.cpath = " +
		quoteLuaString(filepath.Join(work, "?.so")) + ";"); err != nil {
		t.Fatalf("setting the search path: %v", err)
	}
	/* Loaded from the bytes with 'test.lua' as the chunk name, which is
	   what 'lua test.lua' calls it: DoFile would put the whole temporary
	   directory into that message. */
	data, err := os.ReadFile(filepath.Join(build, "test.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if err := L.LoadBytes(data, "@test.lua"); err != nil {
		t.Fatalf("loading the suite: %v", err)
	}
	if status := L.PCall(0, LUA_MULTRET, 0); status != LUA_OK {
		t.Fatalf("running the suite: %s; got %q", valueToStr(&L.stack[L.top-1]), got)
	}

	b, err := os.ReadFile(filepath.Join(src, "expected.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Split(string(b), "\n")
	if n := len(want); n > 0 && want[n-1] == "" {
		want = want[:n-1]
	}
	compareLines(t, got, want)
}
