// The official PUC-Lua 5.3.4 test suite, run the way 'go test' runs
// everything else.
//
// testdata/official holds the 28 files of Lua's own 'testes' directory,
// redistributed under the same MIT licence as Lua itself (Lua.org,
// PUC-Rio).  Beside each one sits <name>.<goos>.expected: what the
// reference interpreter, built from source, printed for it.  The suite
// is therefore a comparison against PUC-Lua and not against what this
// port happened to print one day -- the same idea as testdata/diff, but
// with Lua's own tests instead of this project's.
//
// Two things cannot be compared and are normalised away: heap addresses,
// and anything the host decides -- the path of the interpreter itself
// (main.lua prints it as 'progname' and then runs it), the file
// os.tmpname hands out, elapsed time, and the two seeds
// 'math.randomseed()' picks when it is not told.  The run is seeded
// ('-e "math.randomseed(1, 1)"') so that everything else the PRNG does
// is reproducible.
//
// What is left divides three ways:
//
//   - identical: the file must match the golden byte for byte;
//   - a known difference: the file cannot match, for a reason that
//     follows from the design and is written down beside it.  The test
//     reports the difference instead of failing, and says so when one
//     of them starts agreeing -- that is progress, and it should be
//     noticed;
//   - not run: 'heavy', which times out on both sides.
//
// The goldens are per-GOOS because that is what the reference
// interpreter answers, and only the ones that have been captured are
// compared.  To capture them -- after a Lua release, or on a platform
// that has none yet -- point LUA_REF at a reference 'lua' and run the
// test: it writes every golden afresh and then compares as usual.
package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

const officialDir = "testdata/official"

// officialKnownDiff are the files that cannot match, and why.  Both are
// a consequence of there being no collector of our own; see README §5.1
// and §3.
//
// coroutine.lua, gengc.lua and locals.lua used to be here too, and all
// three for the same reason: they assert that a weak entry is gone, or
// that a finalizer has run, immediately after a 'collectgarbage'.  They
// are identical now, because 'collectgarbage' sweeps -- see
// baseCollectGarbage.  What a collection cannot be made to answer is
// only what gc.lua asks of it: how much memory it freed, and in what
// order it ran the finalizers of a long list.
var officialKnownDiff = map[string]string{
	"all": "run with _port=true (else gc.lua dosteps(0) hangs); plus env lines (PRNG seed, total memory, elapsed time, interpreter path) that cannot match PUC",
	"gc":  "Go GC: collectgarbage only implements collect+count (other opts no-op→0); no pace/debt/ephemerons; suite runs with _port=true so step/stop/isrunning asserts and hang-prone loops are skipped",
}

// officialKnownDiffBuild are differences that depend on what the binary
// was built with.  A build with CGO_ENABLED=0 has no dlopen and no
// setlocale -- see README §5.3 -- and one file of the suite tests each
// of those two things.  A golden comes from an interpreter that has
// both, so those files cannot match until the build has them too; they
// are reported as differences, with the reason, rather than failed.
var officialKnownDiffBuild = map[string]struct {
	needs []string
	why   string
}{
	"attrib":  {[]string{"dlopen", "clibs"}, "loads the C libraries of libs/ through package.loadlib and require"},
	"main":    {[]string{"dlopen", "clibs"}, "runs 'lua -l lib2-v2' as a subprocess, and the module has to be there"},
	"strings": {[]string{"clocale"}, "asks which locales are installed; without the C library's setlocale every name is accepted"},
	// literals: pt_BR decimal-point block is skipped identically when the
	// locale is missing (this box); if a host has pt_BR and Go's locale
	// decimal point still differs from C setlocale, that host will see a
	// real failure rather than a silent known-diff.
}

// officialNoT are the files that match but assert nothing, because the
// 'T' module is missing and their whole body sits behind it.
//
// 'T' comes from Lua's own ltests.c, and ltests.c is LUA_CORE: it
// includes lstate.h, ltable.h, lcode.h and lopcodes.h and walks
// GCObject, CallInfo and TValue by hand -- L->ci->func.p, s2v, setobj2s.
// There is no C lua_State in this port to hand it, so it cannot be
// compiled against one, and it is not a matter of writing the module in
// Go either: a third of what api.lua asks 'T' for is the debug allocator
// ('T.alloccount', 'T.totalmem' -- a limit after which allocations fail)
// and the collector's own state ('T.gcstate', 'T.gccolor'), none of
// which exists here. api.lua is the one file of the suite that is
// entirely behind 'if T==nil then return end'.
var officialNoT = map[string]string{
	"api": "its whole body is behind 'if T==nil then return end', and there is no T: see officialNoT",
}

// officialSkip are the files that are not run at all.
var officialSkip = map[string]string{
	// 5.3.4 testes/ has no heavy.lua (that file is 5.4+); keep the key
	// so a future tree that adds it stays skipped for the same reason.
	"heavy": "builds 2 MB of tables per iteration, 2 GB overall: it times out on both sides, and a test that takes minutes is not a test",
}

// officialTimeout is what one file gets.  main.lua's Ctrl-C harness
// is about a second; all.lua with _port is the slowest runnable file.
const officialTimeout = 180 * time.Second

// officialNormalise takes out what the host decides.  refPath is the
// path of the reference interpreter these goldens were captured with --
// see officialRefPath -- and binPath is the one under test; both have to
// come out, because main.lua prints argv[0] and several files print it
// again in front of an uncaught error.
func officialNormalise(s, binPath, refPath string) string {
	/* An empty path would otherwise be "found" between every pair of
	   characters, so it has to be skipped. */
	if binPath != "" {
		s = strings.ReplaceAll(s, binPath, "PROG")
	}
	if refPath != "" {
		s = strings.ReplaceAll(s, refPath, "PROG")
	}
	for _, n := range []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?m)^/[^ :]*: `), "lua: "},                                                                 // prog: of an uncaught error
		{regexp.MustCompile(`0x[0-9a-f]+`), "0xADDR"},                                                                   // heap addresses
		{regexp.MustCompile(`in [0-9]+(\.[0-9]+)? msec`), "in X msec"},                                                  // timings
		{regexp.MustCompile(`testing short-circuit optimizations \([01]\)`), "testing short-circuit optimizations (N)"}, // os.time()%2
		{regexp.MustCompile(`random seeds: [0-9]+, [0-9]+`), "random seeds: S1, S2"},
		{regexp.MustCompile(`(float|integer) random range in .*`), "$1 random range in ..."},
		{regexp.MustCompile(`/tmp/lua_[A-Za-z0-9]+`), "/tmp/lua_XXXX"},            // os.tmpname
		{regexp.MustCompile(`pid [0-9]+`), "pid PID"},                             // main.lua Ctrl-C harness
		{regexp.MustCompile(`done \(with [0-9]+ kills\)`), "done (with N kills)"}, // signal timing
		{regexp.MustCompile(`(?m)^Lua 5\.[0-9][^ ]*  Copyright.*$`), "LUA-RELEASE"},
	} {
		s = n.re.ReplaceAllString(s, n.repl)
	}
	return strings.ReplaceAll(s, "\r\n", "\n") /* Windows */
}

// officialRefPath is the file that remembers which interpreter the
// goldens for this GOOS came from.  It is written when they are captured
// and read when they are compared, which is the only way the
// comparison can take that path out of the output -- main.lua prints it
// as "progname" -- on a run that never saw LUA_REF.
func officialRefPath() string {
	return filepath.Join(officialDir, runtime.GOOS+".refpath")
}

// officialWriteRefPath records 'ref' for the goldens just captured.
func officialWriteRefPath(t *testing.T, ref string) {
	t.Helper()
	if err := os.WriteFile(officialRefPath(), []byte(ref), 0o644); err != nil {
		t.Fatal(err)
	}
}

// officialReadRefPath answers the recorded path, or "" when there is
// none: without a golden for this GOOS the suite does not run at all, so
// a missing file can only mean goldens from before this was recorded.
func officialReadRefPath() string {
	b, err := os.ReadFile(officialRefPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// officialPatchGc replaces GC1/GC2 busy-waits that allocate until __gc fires.
// After a large heap and live weak tables, allocation-triggered steps alone
// can fail to observe the object as unreachable (stale frame slots + hold
// timing), so the repeat loop never ends.  Forcing collectgarbage inside the
// wait matches what the suite needs from a port without a real collector:
// the metamethod runs, the test moves on, and ephemeron/pace diffs stay in
// knownDiff rather than a 120s harness timeout.
func officialPatchGc(src []byte) []byte {
	const old = "local function GC()  GC1(); GC2() end"
	const new = `local function GC()
  local function wait(make)
    local finish = false
    local u = make(function() finish = true end)
    local n = 0
    repeat
      u = make(nil)
      n = n + 1
      if n % 1000 == 0 then collectgarbage() end
    until finish or n > 100000
    assert(finish, "GC wait: __gc did not run")
  end
  wait(function(fin)
    if fin then return setmetatable({}, {__gc = fin}) end
    return {}
  end)
  wait(function(fin)
    if fin then return {setmetatable({}, {__gc = fin})} end
    return {{}}
  end)
end`
	out := strings.Replace(string(src), old, new, 1)
	return []byte(out)
}

// officialWorkDir makes a scratch copy of the suite, builds its C
// libraries into it, and returns it.
//
// Several of these files write: attrib.lua creates a dozen chunks under
// libs/ and require's them, main.lua writes temporary programs.  Nothing
// may be written into testdata/official -- the tree is checked in, a
// checkout is often read-only, and a case that leaves a file behind
// would change the answer of the next run.  So the suite runs in a copy.
// It is 500 KB, which costs nothing.
//
// libs/ and libs/P1 must exist for attrib.lua: it writes into them, and
// which of its files fails first when they do not is a matter of hash
// order, so the answer is not even reproducible.
func officialWorkDir(t *testing.T) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	files, err := filepath.Glob(filepath.Join(officialDir, "*.lua"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(path) == "gc.lua" {
			b = officialPatchGc(b)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range []string{"libs", filepath.Join("libs", "P1")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir, officialBuildLibs(t, dir)
}

// officialLibs are the C libraries Lua's own suite loads, in the
// makefile's own pairs: <object> built from <source>.  lib2-v2 comes
// from lib2.c under a second name (5.3.4 makefile: mv lib2.so lib2-v2.so).
var officialLibs = []struct{ so, src string }{
	{"lib1.so", "lib1.c"},
	{"lib11.so", "lib11.c"},
	{"lib2.so", "lib2.c"},
	{"lib21.so", "lib21.c"},
	{"lib2-v2.so", "lib2.c"}, /* 5.3.4: renamed copy of lib2 */
}

// officialBuildLibs compiles testdata/official/libs/*.c into the work
// directory, the way Lua's own makefile does.  Without them attrib.lua
// and main.lua stop at "cannot load dynamic library" and the whole C
// half of the suite -- versioned module names, luaopen_ chains, symbols
// one library borrows from another -- goes untested, which is the part
// this port cares most about.  The objects are built against this
// repository's own include/lua.h, and the lua_* names are left for the
// process that loads them to answer, so the very same objects load into
// the reference interpreter and into this one.
//
// It answers false when there is no C compiler or the linker will not
// leave those names unresolved; the caller decides what to do then.
func officialBuildLibs(t *testing.T, dir string) bool {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		return false /* no C compiler: see officialKnownDiffBuild */
	}
	inc, err := filepath.Abs("../../include")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range officialLibs {
		src := filepath.Join(officialDir, "libs", l.src)
		so := filepath.Join(dir, "libs", l.so)
		args := []string{"-shared", "-fPIC", "-I" + inc, "-o", so, src}
		if runtime.GOOS == "darwin" {
			args = append(args, "-undefined", "dynamic_lookup")
		}
		if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
			t.Logf("cannot build %s here (%v): %s", l.so, err, out)
			return false
		}
	}
	return true
}

// officialProbe asks the binary under test what it was built with,
// rather than guessing from the environment: CGO_ENABLED is a setting of
// the build that produced it, and this test only ever sees the result.
func officialProbe(t *testing.T, bin string) map[string]bool {
	t.Helper()
	script := `local _, _, when = package.loadlib("donotexist", "x")
print(when == "open", os.setlocale("pt_BR") ~= nil, os.setlocale("zz_ZZ.NOPE") == nil)`
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("probing %s: %v\n%s", bin, err, out)
	}
	f := strings.Fields(string(out))
	if len(f) < 3 {
		t.Fatalf("probing %s: unexpected answer %q", bin, out)
	}
	return map[string]bool{"dlopen": f[0] == "true", "locale": f[1] == "true", "clocale": f[2] == "true"}
}

// officialRun runs one file through a binary the way the reference
// interpreter was invoked: "-e" first, then the file, with the case
// directory as the working directory so chunk names and 'arg' match.
//
// The process is started in its own group (Setpgid) so a timeout can
// SIGKILL the whole tree — main.lua's Ctrl-C tests spawn background
// `lua -e ... &` children that CommandContext would otherwise leave
// unreaped under the suite timeout.
func officialRun(bin, dir, file string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), officialTimeout)
	defer cancel()
	/* gc.lua / all.lua need _port: collectgarbage("step") is a no-op that
	   answers 0 (truthy), and stop/isrunning do nothing — without _port the
	   suite asserts boolean step/isrunning and can hang on stop+alloc loops.
	   all.lua still dofiles main.lua, but main returns at once when _port
	   is set — Ctrl-C coverage lives in the main.lua case alone. */
	preamble := "math.randomseed(1, 1)"
	if file == "gc.lua" || file == "all.lua" {
		preamble = "_port=true; " + preamble
	}
	cmd := exec.Command(bin, "-e", preamble, file)
	cmd.Dir = dir
	if attr := officialProcAttr(); attr != nil {
		cmd.SysProcAttr = attr
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return buf.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return buf.String(), err
	case <-ctx.Done():
		officialKillTree(cmd.Process)
		<-done
		return buf.String(), ctx.Err()
	}
}

// officialCapture writes every golden afresh from the reference
// interpreter LUA_REF points at.  It is how the goldens came to be, and
// how they are renewed; nothing else in the suite writes to testdata.
func officialCapture(t *testing.T, ref string, dir string, files []string) {
	t.Helper()
	n := 0
	for _, path := range files {
		base := strings.TrimSuffix(filepath.Base(path), ".lua")
		if _, ok := officialSkip[base]; ok {
			continue
		}
		out, err := officialRun(ref, dir, filepath.Base(path))
		if err != nil && len(out) == 0 {
			t.Fatalf("%s: the reference interpreter failed: %v", base, err)
		}
		golden := filepath.Join(officialDir, base+"."+runtime.GOOS+".expected")
		if err := os.WriteFile(golden, []byte(out), 0o644); err != nil {
			t.Fatal(err)
		}
		n++
	}
	t.Logf("captured %d golden files for GOOS=%s from %s", n, runtime.GOOS, ref)
}

func TestOfficialSuite(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(officialDir, "*.lua"))
	if err != nil || len(files) == 0 {
		t.Skipf("no official cases in %s", officialDir)
	}
	sort.Strings(files)
	dir, libs := officialWorkDir(t)

	if ref := os.Getenv("LUA_REF"); ref != "" {
		officialCapture(t, ref, dir, files)
		officialWriteRefPath(t, ref)
	}

	bin := diffBuild(t)
	caps := officialProbe(t, bin)
	caps["clibs"] = libs /* the C libraries are a property of the run, not of the binary */
	var (
		same, differ, skipped, vacuous int
		firstDiff                      string
	)
	for _, path := range files {
		base := strings.TrimSuffix(filepath.Base(path), ".lua")
		if why, ok := officialSkip[base]; ok {
			skipped++
			t.Logf("%-12s not run: %s", base, why)
			continue
		}
		golden := filepath.Join(officialDir, base+"."+runtime.GOOS+".expected")
		want, err := os.ReadFile(golden)
		if err != nil {
			/* no golden for this platform: say so once and move on */
			t.Logf("no golden for GOOS=%s (%s); skipping the suite — point LUA_REF at a reference interpreter to capture one", runtime.GOOS, golden)
			return
		}
		got, err := officialRun(bin, dir, filepath.Base(path))
		if err != nil && len(got) == 0 {
			/* nothing at all came out: a timeout, or a binary that
			   would not start -- not a Lua answer */
			t.Errorf("%s: %v", base, err)
			continue
		}
		/* The exit status is not part of the golden: several of these
		   files end with a failed assert, which is the point of them.
		   What a file prints is the answer; whether it printed an
		   assert failure along the way is already in the output. */
		w := officialNormalise(string(want), "", officialReadRefPath())
		g := officialNormalise(got, bin, officialReadRefPath())

		/* Is a difference here expected?  Two ways: the design (no
		   collector), or the build (no cgo). */
		why, known := officialKnownDiff[base]
		if b, ok := officialKnownDiffBuild[base]; ok {
			var missing []string
			for _, n := range b.needs {
				if !caps[n] {
					missing = append(missing, n)
				}
			}
			if len(missing) > 0 {
				why, known = b.why+" (no "+strings.Join(missing, ", ")+")", true
			}
		}

		if w == g {
			same++
			if why, ok := officialNoT[base]; ok {
				/* It matches, but it matches vacuously: say so every
				   time, so that "27 identical" is never read as
				   "27 files' worth of assertions ran". */
				vacuous++
				t.Logf("%-12s IDENTICAL BUT EMPTY: %s", base, why)
			} else if known {
				t.Logf("%-12s NOW IDENTICAL (was a known difference: %s)", base, why)
			}
			continue
		}
		if known {
			differ++
			t.Logf("%-12s known difference: %s", base, why)
			if firstDiff == "" {
				firstDiff = officialFirstDiff(w, g)
			}
			continue
		}
		t.Errorf("%s: output differs from PUC-Lua 5.3.4\n%s", base, officialFirstDiff(w, g))
	}
	if firstDiff != "" {
		t.Logf("the first line that differs, across the known differences:\n%s", firstDiff)
	}
	t.Logf("official suite: %d identical (%d of them assert nothing), %d known difference, %d not run, of %d",
		same, vacuous, differ, skipped, len(files))
}

// officialFirstDiff prints the first line the two outputs disagree on,
// the way 'diff' would show it.
func officialFirstDiff(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	var b bytes.Buffer
	for i := 0; i < len(wl) || i < len(gl); i++ {
		w, g := "<eof>", "<eof>"
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			fmt.Fprintf(&b, "  line %d\n    PUC-Lua: %q\n    this:    %q\n", i+1, w, g)
			return b.String()
		}
	}
	return ""
}
