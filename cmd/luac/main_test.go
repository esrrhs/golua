package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestLuacMainViaExec covers main by running the package binary under test.
func TestLuacMainViaExec(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	if err := os.WriteFile(src, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "luac-test")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := exec.Command(bin, "-v")
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("-v: %v\n%s", err, out)
	}
	outPath := filepath.Join(dir, "a.out")
	run = exec.Command(bin, "-o", outPath, src)
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
}
