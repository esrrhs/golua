// Third-party Lua corpus.
//
// testdata/third holds five single-file Lua libraries that nobody wrote
// for this port -- a JSON encoder and decoder (dkjson), a smaller one
// (rxi's json.lua), an object-orientation library (middleclass), a
// functional-iterator library (Lua Fun) and a pretty-printer (inspect)
// -- 2 800 lines between them -- plus drive.lua, which asks them
// questions and prints the answers.  What comes out is compared
// against PUC-Lua line for line.
//
// This is a different kind of question from the one testdata/diff asks.
// Every case there was written to test one thing: a function, a corner
// of a function, a message.  A real library does not do that.  It
// builds a scanner out of 'string.find' and a position, escapes a
// string with 'gsub' and a table of replacements, walks a metatable
// chain three deep, and runs every iterator as a coroutine -- and it
// does all of it in the same few thousand lines, so what is under test
// is the machine as a whole rather than any single part of it.  A bug
// that needs three features to line up before it shows is the kind
// neither a per-function case nor a fuzz over one library's arguments
// is looking for.
//
// Two things keep the output comparable, and both are the sort of thing
// that only shows up once a real library is involved:
//
//   - a table is printed with its keys sorted (see 'dump' in
//     drive.lua).  PUC-Lua seeds its string hashes from the time and
//     an address, so 'pairs' hands out the keys of the same table in a
//     different order on every run -- of the reference as much as of
//     this port.  A library that iterates a table to print it, which
//     is what a JSON encoder or a pretty-printer is, is uncomparable
//     until that is taken out.
//   - a function is reported by 'type', not by 'tostring': the address
//     a function prints with is the host's, and differs between the
//     two binaries by gigabytes.
//
// Both are properties of the *harness*, not of Lua, and neither is
// something this port can be wrong about -- which is why they are
// normalised away rather than compared.

package core

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// thirdGlob is which files in testdata/third are cases.  The libraries
// themselves are 'require'd by drive.lua, not run as scripts, so they
// are not in it; a second driver can be dropped in next to this one
// and is picked up without touching the test.
const thirdGlob = "drive*.lua"

func TestThirdPartySuite(t *testing.T) {
	dir := filepath.Join("testdata", "third")
	files, err := filepath.Glob(filepath.Join(dir, thirdGlob))
	if err != nil || len(files) == 0 {
		t.Fatalf("no third-party cases found under %s: %v", dir, err)
	}
	sort.Strings(files)
	if ref := os.Getenv("LUA_REF"); ref != "" {
		diffCapture(t, ref, files)
	}
	for _, path := range files {
		golden := diffGolden(path)
		want, err := os.ReadFile(golden)
		if err != nil {
			/* as in TestDifferentialSuite: a platform with no golden
			   of its own is not compared against another one's */
			t.Logf("no golden for GOOS=%s (%s); skipping the corpus -- point LUA_REF at a reference interpreter to capture one",
				runtime.GOOS, filepath.Base(golden))
			return
		}
		got := diffRunCase(t, path)
		w, g := diffNormalise(string(want)), diffNormalise(got)
		if w != g {
			t.Errorf("%s: output differs from PUC-Lua 5.4.9\n--- want ---\n%s\n--- got ---\n%s",
				filepath.Base(path), w, g)
			continue
		}
		/* the last line of every driver is how many cases it asked */
		if line := lastLine(w); line != "" && !strings.Contains(line, "0\tcases") {
			t.Logf("%s: %s", filepath.Base(path), strings.Replace(line, "\t", " ", 1))
		}
	}
}

// lastLine is the final non-empty line of 's'.
func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		s = s[i+1:]
	}
	return strings.TrimSpace(s)
}
