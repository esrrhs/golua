package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestTableFuzz runs testdata/tablefuzz.lua under many seeds: it builds a
// table with keys of every kind, writes and deletes them at random,
// resizes both parts under them, and checks the invariants of 'next' that
// do not depend on the order keys come out in.
//
// It is a fuzz test rather than a fixed case because what it is looking
// for is a state, not an input: the bug it was written for -- treating
// chain offset -2 as a dead-key marker -- only showed up when the hash
// put a colliding key two slots below its main position, which the
// per-state seed decides.  A single traversal would pass almost every time.
func TestTableFuzz(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "tablefuzz.lua"))
	if err != nil {
		t.Fatal(err)
	}
	/* 120 seeds is what a normal run pays for.  The check that matters
	   is a property of the *state* the hash lands in, not of the seed,
	   so a deep run does find more: LUA_FUZZ_SEEDS=5000 makes one. */
	seeds := 120
	if s := os.Getenv("LUA_FUZZ_SEEDS"); s != "" {
		if v, err := strconv.Atoi(s); err == nil && v > 0 {
			seeds = v
		}
	}
	var failed []string
	for seed := 1; seed <= seeds; seed++ {
		L := NewState()
		L.OpenLibs()
		prog := "seed = " + strconv.Itoa(seed) + "\n" + string(src)
		if err := L.LoadString(prog, "=tablefuzz"); err != nil {
			L.Close()
			t.Fatalf("seed %d: load: %v", seed, err)
		}
		st := L.PCall(0, 1, 0)
		got, _ := L.ToString(-1)
		L.Close()
		if st != LUA_OK || got != "OK" {
			failed = append(failed, "seed "+strconv.Itoa(seed)+": "+describe(got))
			if len(failed) > 3 {
				break
			}
		}
	}
	if len(failed) > 0 {
		t.Fatalf("%d of %d seeds broke the traversal:\n  %s",
			len(failed), seeds, strings.Join(failed, "\n  "))
	}
}

// describe shortens a long list of complaints to its first few.
func describe(s string) string {
	const max = 4
	parts := strings.Split(s, " | ")
	if len(parts) <= max {
		return s
	}
	return strings.Join(parts[:max], " | ") + " ... (and " +
		strconv.Itoa(len(parts)-max) + " more)"
}
