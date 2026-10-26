// Differential conformance suite.
//
// Every testdata/diff/*.lua case is run through this port and its output is
// compared byte for byte against a golden file captured from the reference
// interpreter (PUC-Lua 5.3.4, built from source).  The cases cover the
// virtual machine, the standard libraries, error messages and their
// positions -- the things a "drop-in Lua" has to get exactly right.
//
// Two host-dependent details are normalised away:
//
//   - argv[0] in "prog: file:line: msg" (the two binaries have
//     different paths);
//   - heap addresses in "table: 0x...", "function: 0x...".
//
// The cases are run through the standalone interpreter built from
// ./cmd/lua rather than through the API directly: the golden files were
// captured from the reference 'lua' binary, whose observable output
// includes the "prog: " prefix of uncaught errors and the trailing
// "[C]: in ?" host frame of every traceback. Running the same program
// makes the comparison meaningful.
package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Heap addresses differ between the two implementations and cannot be
// compared; everything else must match byte for byte.
var diffAddrRe = regexp.MustCompile(`0x[0-9a-f]+`)

// diffNormalise removes what the host decides, on both sides alike.
//
// Two of these are not per-platform but per-C-library, which the
// per-GOOS golden names cannot sort out: the reference and this port run
// on the same machine and still disagree.
func diffNormalise(s string) string {
	s = diffAddrRe.ReplaceAllString(s, "<ADDR>")
	/* The sign of a NaN.  C asks printf, which on glibc writes "-nan"
	   when the sign bit is set and "nan" when it is not, while Go's
	   strconv writes neither and fmtNaN (lobject.go) has to guess the
	   library's spelling from GOOS.  Which of the two a NaN *is* then
	   depends on what produced it: 'math.huge - math.huge' is the
	   negative one on every side, and Go's math.Mod hands back a
	   positive one where C's fmod hands back the CPU's indefinite
	   QNaN.  Not one of the cases is about that. */
	s = strings.ReplaceAll(s, "-nan", "nan")
	/* The name of the UTC zone.  TZ is pinned (see diffEnv), so the
	   only thing os.date's '%Z' can answer is what the host's C
	   library calls UTC: "UTC" on macOS, "GMT" on glibc. */
	s = diffZoneRe.ReplaceAllString(s, "UTC")
	/* a checkout that rewrites line endings would otherwise make every
	   line of every case differ, which is not a difference in Lua */
	return strings.ReplaceAll(s, "\r\n", "\n")
}

var diffZoneRe = regexp.MustCompile(`\bGMT\b`)

var (
	diffBin    string
	diffBinDir string
	diffBinErr error
	diffBinOne sync.Once
)

// diffBuild builds ./cmd/lua once for the whole suite.
func diffBuild(t *testing.T) string {
	t.Helper()
	diffBinOne.Do(func() {
		dir, err := os.MkdirTemp("", "luadiff")
		if err != nil {
			diffBinErr = err
			return
		}
		bin := filepath.Join(dir, "lua")
		cmd := exec.Command("go", "build", "-o", bin, "github.com/esrrhs/golua/cmd/lua")
		if out, err := cmd.CombinedOutput(); err != nil {
			diffBinErr = &diffBuildError{string(out), err}
			return
		}
		/* Windows will not execute a file whose name has no
		   extension, and os/exec says so as "executable file not
		   found in %PATH%" -- which is a long way from the truth,
		   since the file is right there.  Go writes lua.exe there
		   whatever -o said; make sure of it either way. */
		if runtime.GOOS == "windows" {
			if _, err := os.Stat(bin); err == nil {
				if err := os.Rename(bin, bin+".exe"); err == nil {
					bin += ".exe"
				}
			} else if _, err := os.Stat(bin + ".exe"); err == nil {
				bin += ".exe"
			}
		}
		diffBin = bin
		/* the binary is shared by every test in the package (the CLI
		   tests run it too), so it must outlive the test that happened
		   to build it: TestMain removes it at the end of the run. */
		diffBinDir = dir
	})
	if diffBinErr != nil {
		t.Fatalf("cannot build ./cmd/lua: %v", diffBinErr)
	}
	return diffBin
}

// TestMain removes the interpreter built by 'diffBuild' once every test
// in the package has finished with it.
func TestMain(m *testing.M) {
	code := m.Run()
	if diffBinDir != "" {
		os.RemoveAll(diffBinDir)
	}
	os.Exit(code)
}

type diffBuildError struct {
	out   string
	cause error
}

func (e *diffBuildError) Error() string {
	return e.cause.Error() + "\n" + e.out
}

// diffEnv is the environment every case runs under, on both sides.
//
// TZ is pinned because 'os.date' and 'os.time' ask the host -- the C
// library on the reference's side, Go's time package on this port's --
// and every answer that is not explicitly UTC carries the runner's
// offset.  A golden captured in one timezone therefore cannot match a
// runner in another, which is a worse problem than the platform one
// the per-GOOS golden names solve: two machines of the same GOOS
// disagree.  UTC is what both sides agree on anywhere, so that is what
// the cases see.  Local time is still compared -- by the official
// suite, which captures its goldens from LUA_REF seconds before it
// compares them.
func diffEnv() []string { return append(os.Environ(), "TZ=UTC") }

// diffRunCase runs one case file the way the reference interpreter was
// invoked ("lua <file>" with the case directory as the working directory,
// so that chunk names and 'arg' match) and returns stdout+stderr merged,
// as "2>&1" does.
func diffRunCase(t *testing.T, path string) string {
	t.Helper()
	bin := diffBuild(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, filepath.Base(path))
	cmd.Args[0] = "lua" /* argv[0] decides the message prefix */
	cmd.Dir = filepath.Dir(path)
	cmd.Env = diffEnv()
	out, _ := cmd.CombinedOutput() /* the exit status is not part of the golden output */
	if ctx.Err() == context.DeadlineExceeded {
		t.Errorf("%s: timed out after 15s (possible infinite loop)", filepath.Base(path))
	}
	return string(out)
}

// diffGolden is where a case's expected output lives.  It carries the
// platform, because a good many of these answers are the platform's and
// not Lua's: 'os.date' asks the C library, and the NaN that 0/0 produces
// carries the sign the CPU gave it, so 'nan' here is '-nan' there.
// The timezone is not in the name because it does not need to be: see
// diffEnv, which pins it.
func diffGolden(path string) string {
	return strings.TrimSuffix(path, ".lua") + "." + runtime.GOOS + ".expected"
}

// diffCapture writes every golden afresh from the reference interpreter
// LUA_REF points at.  It is how the goldens came to be, and how they are
// renewed; the committed ones are darwin, and a platform that has none
// skips the suite rather than comparing itself to somebody else's
// machine.  CI sets LUA_REF, so there every run is a real differential
// one, captured seconds before it is compared.
func diffCapture(t *testing.T, ref string, files []string) {
	t.Helper()
	n := 0
	for _, path := range files {
		cmd := &exec.Cmd{
			Path: ref,
			Args: []string{"lua", filepath.Base(path)},
			Dir:  filepath.Dir(path),
			Env:  diffEnv(), /* see diffEnv: TZ has to agree on both sides */
		}
		out, err := cmd.CombinedOutput()
		if err != nil && len(out) == 0 {
			t.Fatalf("%s: the reference interpreter failed: %v", filepath.Base(path), err)
		}
		/* Normalised on the way in, not only on the way out: a heap
		   address is a different one every run, so a golden that kept
		   them raw would show up as 82 changed files every time
		   anybody re-captured, and a change nobody can read is noise
		   that hides the real ones.  They are compared normalised
		   either way -- see diffNormalise. */
		if err := os.WriteFile(diffGolden(path), []byte(diffNormalise(string(out))), 0o644); err != nil {
			t.Fatal(err)
		}
		n++
	}
	t.Logf("captured %d golden files for GOOS=%s from %s", n, runtime.GOOS, ref)
}

// diffKnownDiffBuild are the cases that cannot match until the build has
// what the golden was captured with: the same idea as
// officialKnownDiffBuild, and the same probe answers for both, since it
// is one binary being asked.
//
// A golden here comes from the reference interpreter, which always has
// the C library.  A build with CGO_ENABLED=0 has none, so 'os.setlocale'
// answers from the decimal-point table instead of asking, and every name
// is accepted -- where C refuses a locale the machine does not have.
// The answers to three of these cases turn on exactly that, and turn on
// nothing else.
var diffKnownDiffBuild = map[string]struct {
	needs []string
	why   string
}{
	"24_errmsg": {[]string{"clocale"}, "asks os.setlocale(1), a locale this machine may not have"},
	"42_os":     {[]string{"clocale"}, "asks os.setlocale(42), a locale this machine may not have"},
	"61_ospkg":  {[]string{"clocale"}, `asks os.setlocale for "xx_XX" and for 5, locales this machine may not have`},
}

func TestDifferentialSuite(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "diff", "*.lua"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no differential cases found: %v", err)
	}
	sort.Strings(files)
	if ref := os.Getenv("LUA_REF"); ref != "" {
		diffCapture(t, ref, files)
	}
	/* What the binary under test can answer, asked of the binary rather
	   than guessed from CGO_ENABLED, which is a setting of a build no
	   test can see.  Asked lazily, so that a platform with no goldens
	   never runs it at all -- it is only ever needed to explain a
	   difference that is not a mistake. */
	var caps map[string]bool
	can := func(name string) bool {
		if caps == nil {
			caps = officialProbe(t, diffBuild(t))
		}
		return caps[name]
	}
	same, differ := 0, 0
	for _, path := range files {
		golden := diffGolden(path)
		want, err := os.ReadFile(golden)
		if err != nil {
			/* no golden for this platform: say so once and move on */
			t.Logf("no golden for GOOS=%s (%s); skipping the suite -- point LUA_REF at a reference interpreter to capture one",
				runtime.GOOS, filepath.Base(golden))
			return
		}
		got := diffRunCase(t, path)
		w, g := diffNormalise(string(want)), diffNormalise(got)
		if w == g {
			same++
			continue
		}
		/* Is a difference here expected?  The design one this suite can
		   have is the build's: see diffKnownDiffBuild. */
		name := strings.TrimSuffix(filepath.Base(path), ".lua")
		if b, ok := diffKnownDiffBuild[name]; ok {
			var missing []string
			for _, n := range b.needs {
				if !can(n) {
					missing = append(missing, n)
				}
			}
			if len(missing) > 0 {
				differ++
				t.Logf("%-12s known difference: %s (no %s)", name, b.why, strings.Join(missing, ", "))
				continue
			}
		}
		/* normalised on the way out as well: a heap address in the
		   message reads as a difference that is not one */
		t.Errorf("%s: output differs from PUC-Lua 5.3.4\n--- want ---\n%s\n--- got ---\n%s",
			filepath.Base(path), w, g)
	}
	t.Logf("differential suite: %d identical, %d known difference, of %d", same, differ, len(files))
}

// ---------------------------------------------------------------------
// regressions found by the differential suite
// ---------------------------------------------------------------------

// TestHexDigitValue pins luaO_hexavalue: C lower-cases with
// 'c | ('A' ^ 'a')', and getting that wrong corrupts every hexadecimal
// literal, '\x..' escape and '\u{..}' escape.
func TestHexDigitValue(t *testing.T) {
	for c, want := range map[byte]int{
		'0': 0, '9': 9, 'A': 10, 'F': 15, 'a': 10, 'f': 15,
	} {
		if got := luaO_hexavalue(c); got != want {
			t.Errorf("luaO_hexavalue(%q) = %d, want %d", c, got, want)
		}
	}
}

// TestCeilLog2 pins luaO_ceillog2 and its 256-entry table. A single missing
// entry used to make 'luaH_newkey' panic on large integer keys.
func TestCeilLog2(t *testing.T) {
	for x, want := range map[uint32]int{
		1: 0, 2: 1, 3: 2, 127: 7, 128: 7, 129: 8,
		255: 8, 256: 8, 257: 9, 0x7FFFFFFF: 31, 0x80000000: 31,
	} {
		if got := luaO_ceillog2(x); got != want {
			t.Errorf("luaO_ceillog2(%d) = %d, want %d", x, got, want)
		}
	}
}

// TestBigHexConstant loads a chunk whose constant table has to grow past
// 2^31/2 entries of numbering, the case that used to panic in 'rehash'.
func TestBigHexConstant(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, "return 0x7fffffff"); err != nil {
		t.Fatal(err)
	}
	if n, _ := L.ToInteger(-1); n != 0x7fffffff {
		t.Fatalf("0x7fffffff = %d", n)
	}
}

// TestLoadMode checks that 'load' honours its mode string the way C's
// 'f_parser' does: the first byte decides whether 'b' or 't' is required.
func TestLoadMode(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local ok, err = load("return 1", "=x", "b")
		assert(not ok and err == "attempt to load a text chunk (mode is 'b')", err)
		local ok2, err2 = load(string.dump(function() return 7 end), "=y", "t")
		assert(not ok2 and err2 == "attempt to load a binary chunk (mode is 't')", err2)
		return true
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("mode checks failed")
	}
}

// TestCloseDeadCoroutine checks 'coroutine.close' on a coroutine that ended
// with an error: it reports that error once again, and only once.
func TestCloseDeadCoroutine(t *testing.T) {
	t.Skip("Lua 5.3.4: no <close>/<const>/coroutine.close (5.4 features)")
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local co = coroutine.create(function() error("x") end)
		assert(select(1, coroutine.resume(co)) == false)
		local ok, msg = coroutine.close(co)
		assert(ok == false and msg ~= nil, "first close must report the error")
		assert(coroutine.close(co) == true, "second close must succeed")
		return true
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("coroutine.close semantics differ")
	}
}

// TestErrorInErrorHandling checks that a message handler which itself
// raises stops with LUA_ERRERR instead of recursing forever.
func TestErrorInErrorHandling(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local ok, msg = xpcall(function() error("x") end,
		                      function(e) error("in handler") end)
		return ok == false and msg == "error in error handling"
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("expected 'error in error handling'")
	}
}

// runLua loads and runs 'src', returning a Go error (not a Lua one).
func runLua(t *testing.T, L *State, src string) error {
	t.Helper()
	if err := L.LoadBytes([]byte(src), "=test"); err != nil {
		return err
	}
	if status := L.PCall(0, LUA_MULTRET, 0); status != LUA_OK {
		s, _ := L.ToString(-1)
		return &LuaError{Status: status, Message: s}
	}
	return nil
}
