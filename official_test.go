// The official PUC-Lua 5.4.9 test suite, run the way 'go test' runs
// everything else.
//
// testdata/official holds the 33 files of Lua's own 'testes' directory,
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
package lua

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

// officialKnownDiff are the files that cannot match, and why.  Every
// one of them is a consequence of there being no collector of our own;
// see README §5.1 and §3.
var officialKnownDiff = map[string]string{
	"all":       "the environment, in four lines: the second PRNG seed, 'total memory' (PUC-Lua's allocator against the Go heap), the elapsed time, and the interpreter's own path",
	"coroutine": "asserts a weak entry is gone right after a collectgarbage (C[1] == undef), and collectgarbage no longer starts a collection",
	"gengc":     "asserts a weak entry is gone right after collectgarbage('step', 0) (t[1] == nil), for the same reason",
	"gc":        "the file is a test of the collector itself, and there is none here",
	"locals":    "counts finalizers with require\"tracegc\" right after a collectgarbage, and gets one fewer",
}

// officialKnownDiffBuild are differences that depend on what the binary
// was built with.  A build with CGO_ENABLED=0 has no dlopen and no
// setlocale -- see README §5.3 -- and one file of the suite tests each
// of those two things.  A golden comes from an interpreter that has
// both, so those files cannot match until the build has them too; they
// are reported as differences, with the reason, rather than failed.
var officialKnownDiffBuild = map[string]struct {
	need string
	why  string
}{
	"attrib":   {"dlopen", "tests package.loadlib; without cgo there is no dlopen, and the file takes its other branch"},
	"literals": {"locale", "tests the decimal point of the pt_BR locale; without cgo os.setlocale answers nil"},
}

// officialSkip are the files that are not run at all.
var officialSkip = map[string]string{
	"heavy": "builds 2 MB of tables per iteration, 2 GB overall: it times out on both sides, and a test that takes minutes is not a test",
}

// officialTimeout is what one file gets.  The slowest that is run,
// 'errors', takes two seconds.
const officialTimeout = 120 * time.Second

// officialNormalise takes out what the host decides.
func officialNormalise(s string, binPath string) string {
	/* main.lua prints argv[0] as "progname" and then runs it, so the
	   path of both interpreters has to become the same thing. */
	s = strings.ReplaceAll(s, binPath, "PROG")
	s = strings.ReplaceAll(s, "/tmp/lua-5.4.9/src/lua", "PROG")
	for _, n := range []struct {
		re   *regexp.Regexp
		repl string
	}{
		{regexp.MustCompile(`(?m)^/[^ :]*: `), "lua: "},                // prog: of an uncaught error
		{regexp.MustCompile(`0x[0-9a-f]+`), "0xADDR"},                  // heap addresses
		{regexp.MustCompile(`in [0-9]+(\.[0-9]+)? msec`), "in X msec"}, // timings
		{regexp.MustCompile(`random seeds: [0-9]+, [0-9]+`), "random seeds: S1, S2"},
		{regexp.MustCompile(`(float|integer) random range in .*`), "$1 random range in ..."},
		{regexp.MustCompile(`/tmp/lua_[A-Za-z0-9]+`), "/tmp/lua_XXXX"}, // os.tmpname
		{regexp.MustCompile(`(?m)^Lua 5\.4[^ ]*  Copyright.*$`), "LUA-RELEASE"},
	} {
		s = n.re.ReplaceAllString(s, n.repl)
	}
	return strings.ReplaceAll(s, "\r\n", "\n") /* Windows */
}

// officialWorkDir makes a scratch copy of the suite and returns it.
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
func officialWorkDir(t *testing.T) string {
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
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(path)), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, sub := range []string{"libs", filepath.Join("libs", "P1")} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// officialProbe asks the binary under test what it was built with,
// rather than guessing from the environment: CGO_ENABLED is a setting of
// the build that produced it, and this test only ever sees the result.
func officialProbe(t *testing.T, bin string) map[string]bool {
	t.Helper()
	script := `local _, _, when = package.loadlib("donotexist", "x")
print(when == "open", os.setlocale("pt_BR") ~= nil)`
	out, err := exec.Command(bin, "-e", script).CombinedOutput()
	if err != nil {
		t.Fatalf("probing %s: %v\n%s", bin, err, out)
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		t.Fatalf("probing %s: unexpected answer %q", bin, out)
	}
	return map[string]bool{"dlopen": f[0] == "true", "locale": f[1] == "true"}
}

// officialRun runs one file through a binary the way the reference
// interpreter was invoked: "-e" first, then the file, with the case
// directory as the working directory so chunk names and 'arg' match.
func officialRun(bin, dir, file string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), officialTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-e", "math.randomseed(1, 1)", file)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput() /* stdout+stderr, as "2>&1" does */
	return string(out), err
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
	dir := officialWorkDir(t)

	if ref := os.Getenv("LUA_REF"); ref != "" {
		officialCapture(t, ref, dir, files)
	}

	bin := diffBuild(t)
	caps := officialProbe(t, bin)
	var (
		same, differ, skipped int
		firstDiff             string
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
		w := officialNormalise(string(want), "/tmp/lua-5.4.9/src/lua")
		g := officialNormalise(got, bin)

		/* Is a difference here expected?  Two ways: the design (no
		   collector), or the build (no cgo). */
		why, known := officialKnownDiff[base]
		if b, ok := officialKnownDiffBuild[base]; ok && !caps[b.need] {
			why, known = b.why+" (no "+b.need+")", true
		}

		if w == g {
			same++
			if known {
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
		t.Errorf("%s: output differs from PUC-Lua 5.4.9\n%s", base, officialFirstDiff(w, g))
	}
	if firstDiff != "" {
		t.Logf("the first line that differs, across the known differences:\n%s", firstDiff)
	}
	t.Logf("official suite: %d identical, %d known difference, %d not run, of %d",
		same, differ, skipped, len(files))
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
