package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/esrrhs/golua"
)

func TestCollectArgsAndOptChar(t *testing.T) {
	check := func(argv []string, wantMask, wantScript int) {
		t.Helper()
		args, script := collectArgs(argv)
		if args != wantMask || script != wantScript {
			t.Errorf("%v -> mask=%d script=%d want mask=%d script=%d",
				argv, args, script, wantMask, wantScript)
		}
	}
	check([]string{"lua"}, 0, 1)
	check([]string{"lua", "-v"}, hasV, 2)
	check([]string{"lua", "-e", "print(1)"}, hasE, 3)
	check([]string{"lua", "-l", "math"}, 0, 3) // -l alone does not set hasE
	check([]string{"lua", "-i"}, hasI|hasV, 2)
	check([]string{"lua", "-E"}, hasEnv, 2)
	check([]string{"lua", "--", "s.lua"}, 0, 2)
	check([]string{"lua", "s.lua", "a"}, 0, 1)
	check([]string{"lua", "-e", "1", "s.lua"}, hasE, 3)
	check([]string{"lua", "-z"}, hasError, 1)
	check([]string{"lua", "-e"}, hasError, 1)
	check([]string{"lua", "-"}, 0, 1)

	if optChar("-e") != 'e' || optChar("--") != '-' || optChar("-") != 0 || optChar("x") != 0 {
		t.Fatal("optChar")
	}
}

func TestUsageAndVersionHelpers(t *testing.T) {
	u := usage()
	if !strings.Contains(u, "usage:") || !strings.Contains(u, "-e") {
		t.Fatalf("usage %q", u)
	}
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	printUsage("-z")
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	if !strings.Contains(buf.String(), "unrecognized option") {
		t.Fatalf("printUsage %q", buf.String())
	}
	oldOut := os.Stdout
	r2, w2, _ := os.Pipe()
	os.Stdout = w2
	printVersion()
	_ = w2.Close()
	os.Stdout = oldOut
	var out bytes.Buffer
	_, _ = io.Copy(&out, r2)
	_ = r2.Close()
	if !strings.Contains(out.String(), "Lua") {
		t.Fatalf("version %q", out.String())
	}
	_ = incomplete(errors.New("other"))
	_ = incomplete(&lua.LuaError{Status: lua.LUA_ERRSYNTAX, Message: "x near '<eof>'"})
	_ = incomplete(&lua.LuaError{Status: lua.LUA_ERRSYNTAX, Message: "nope"})
	_ = incomplete(&lua.LuaError{Status: lua.LUA_ERRRUN, Message: "x near '<eof>'"})
}

func TestReplPromptNeedsState(t *testing.T) {
	if code := pmainDriver([]string{"lua", "-e", `
local p1 = _PROMPT or "> "
assert(type(p1)=="string")
`}); code != 0 {
		t.Fatalf("prompt via -e => %d", code)
	}
	L = lua.NewState()
	defer func() { L.Close(); L = nil }()
	L.OpenLibs()
	if replPrompt(true) == "" || replPrompt(false) == "" {
		t.Fatal("prompt")
	}
}

func TestPmainDriverPaths(t *testing.T) {
	if code := pmainDriver([]string{"lua", "-e", "print(1+1)"}); code != 0 {
		t.Fatalf("-e => %d", code)
	}
	if code := pmainDriver([]string{"lua", "-v"}); code != 0 {
		t.Fatalf("-v => %d", code)
	}
	if code := pmainDriver([]string{"lua", "-e", "error('x')"}); code == 0 {
		t.Fatal("expected error exit")
	}
	if code := pmainDriver([]string{"lua", "-z"}); code == 0 {
		t.Fatal("bad option should fail")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "s.lua")
	if err := os.WriteFile(path, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := pmainDriver([]string{"lua", path}); code != 0 {
		t.Fatalf("script => %d", code)
	}
	if code := pmainDriver([]string{"lua", "--", path, "a", "b"}); code != 0 {
		t.Fatalf("-- script => %d", code)
	}
	if code := pmainDriver([]string{"lua", "-l", "math", "-e", "assert(math.pi)"}); code != 0 {
		t.Fatalf("-l => %d", code)
	}
	if code := pmainDriver([]string{"lua", "-E", "-e", "assert(true)"}); code != 0 {
		t.Fatalf("-E => %d", code)
	}
	if code := pmainDriver([]string{"lua", "-e", "assert(arg~=nil)"}); code != 0 {
		t.Fatalf("arg => %d", code)
	}
}

func TestSkipStdinCommentAndReport(t *testing.T) {
	if got := string(skipStdinComment([]byte("#!/bin/lua\nreturn 1"))); got != "\nreturn 1" {
		t.Fatalf("shebang %q", got)
	}
	if string(skipStdinComment([]byte("return 1"))) != "return 1" {
		t.Fatal("no shebang")
	}
	_ = stdinIsTTY()
	report(nil)
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	report(errors.New("boom"))
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	if !strings.Contains(buf.String(), "boom") {
		t.Fatalf("report %q", buf.String())
	}
}

func TestStdinScript(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("print(7)\n"))
		_ = w.Close()
	}()
	code := pmainDriver([]string{"lua", "-"})
	os.Stdin = old
	_ = r.Close()
	if code != 0 {
		t.Fatalf("stdin script => %d", code)
	}
}

func TestREPLAndInit(t *testing.T) {
	// LUA_INIT=@file
	dir := t.TempDir()
	init := filepath.Join(dir, "init.lua")
	if err := os.WriteFile(init, []byte("INIT_OK=true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUA_INIT", "@"+init)
	if code := pmainDriver([]string{"lua", "-e", "assert(INIT_OK)"}); code != 0 {
		t.Fatalf("LUA_INIT @file => %d", code)
	}
	t.Setenv("LUA_INIT", "INIT2=true")
	if code := pmainDriver([]string{"lua", "-e", "assert(INIT2)"}); code != 0 {
		t.Fatalf("LUA_INIT string => %d", code)
	}
	// -i with piped commands then EOF
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = r
	go func() {
		_, _ = w.Write([]byte("print(1+1)\n"))
		_ = w.Close()
	}()
	code := pmainDriver([]string{"lua", "-i", "-e", "x=1"})
	os.Stdin = old
	_ = r.Close()
	if code != 0 {
		t.Fatalf("-i => %d", code)
	}
	// printUsage for -e missing arg
	oldErr := os.Stderr
	r2, w2, _ := os.Pipe()
	os.Stderr = w2
	_ = pmainDriver([]string{"lua", "-e"})
	_ = w2.Close()
	os.Stderr = oldErr
	_ = r2.Close()
}
