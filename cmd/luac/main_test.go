package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLuacMainInProcess(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.lua")
	if err := os.WriteFile(src, []byte("return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldArgs := os.Args
	oldOut, oldErr := os.Stdout, os.Stderr
	defer func() {
		os.Args = oldArgs
		os.Stdout, os.Stderr = oldOut, oldErr
		exitFunc = os.Exit
	}()
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut, wErr
	var code int
	exitFunc = func(c int) { code = c }
	os.Args = []string{"luac", "-v"}
	main()
	_ = wOut.Close()
	_ = wErr.Close()
	var out, errb bytes.Buffer
	_, _ = out.ReadFrom(rOut)
	_, _ = errb.ReadFrom(rErr)
	_ = rOut.Close()
	_ = rErr.Close()
	if code != 0 {
		t.Fatalf("-v exit %d stderr=%q", code, errb.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Lua")) && !bytes.Contains(out.Bytes(), []byte("PUC")) {
		t.Fatalf("version out %q", out.String())
	}

	outPath := filepath.Join(dir, "a.out")
	code = -1
	os.Args = []string{"luac", "-o", outPath, src}
	rOut2, wOut2, _ := os.Pipe()
	rErr2, wErr2, _ := os.Pipe()
	os.Stdout, os.Stderr = wOut2, wErr2
	main()
	_ = wOut2.Close()
	_ = wErr2.Close()
	_, _ = out.ReadFrom(rOut2)
	_, _ = errb.ReadFrom(rErr2)
	_ = rOut2.Close()
	_ = rErr2.Close()
	if code != 0 {
		t.Fatalf("compile exit %d", code)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
}

// TestLuacMainViaExec still exercises the real binary path as a smoke check.
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
}
