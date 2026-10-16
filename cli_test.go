package lua

// Tests for the standalone interpreter: the parts of lua.c that scripts
// and build systems observe -- the option parser, the 'arg' table, the
// exit status and the warning switch.  They run the binary built from
// ./cmd/lua (see diffBuild), always with argv[0] == "lua", because that
// is what the reference interpreter is compared against.
import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cliRun runs the interpreter with the given arguments (argv[0] is forced
// to "lua") and returns stdout, stderr and the exit status.
func cliRun(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	bin := diffBuild(t)
	cmd := &exec.Cmd{Path: bin, Args: append([]string{"lua"}, args...)}
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	code := 0
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("running %v: %v", args, err)
		}
	}
	return out.String(), errb.String(), code
}

// cliScript writes 'src' to a file in a temporary directory and returns
// its path.
func cliScript(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.lua")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestCLIVersionOnly pins the bug that made 'lua -v' fall through to the
// REPL: with '-v' (or '-e') there is no script and no option is active,
// so lua.c must not start reading stdin.
func TestCLIVersionOnly(t *testing.T) {
	out, errb, code := cliRun(t, "-v")
	if code != 0 {
		t.Fatalf("lua -v exited %d, stderr %q", code, errb)
	}
	if !strings.HasPrefix(out, "Lua 5.4") {
		t.Errorf("lua -v printed %q", out)
	}
	if n := strings.Count(out, "\n"); n != 1 {
		t.Errorf("lua -v printed %d lines (%q): the REPL must not start", n, out)
	}
	if strings.Contains(out, "> ") {
		t.Errorf("lua -v printed a REPL prompt: %q", out)
	}
}

// TestCLIScriptArgs pins 'pushargs': the script receives its arguments as
// varargs and sees them in 'arg'.
func TestCLIScriptArgs(t *testing.T) {
	path := cliScript(t, `
		print(...)
		print(arg[1], arg[2], arg[3], arg[4])
		print(#arg, arg[-1], arg[-2])
		print(arg[0]:sub(-5))   -- the script name, without its directory
	`)
	out, errb, code := cliRun(t, path, "a", "b", "c")
	if code != 0 {
		t.Fatalf("exited %d: %s%s", code, out, errb)
	}
	want := "a\tb\tc\n" +
		"a\tb\tc\tnil\n" +
		"3\tlua\tnil\n" +
		"s.lua\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestCLIExitStatus pins that a failing script exits with 1 (lua.c
// returns EXIT_FAILURE because 'pmain' pushed false) while the message is
// reported exactly once.
func TestCLIExitStatus(t *testing.T) {
	bad := cliScript(t, "error('boom')")
	out, errb, code := cliRun(t, bad)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if n := strings.Count(errb, "boom"); n != 1 {
		t.Errorf("error message reported %d times: %q", n, errb)
	}
	if !strings.HasPrefix(errb, "lua: ") {
		t.Errorf("stderr = %q, want the 'lua: ' prefix", errb)
	}
	if out != "" {
		t.Errorf("stdout = %q, want empty", out)
	}

	good := cliScript(t, "print('ok')")
	out, _, code = cliRun(t, good)
	if code != 0 || out != "ok\n" {
		t.Errorf("lua good.lua = (%q, %d), want (\"ok\\n\", 0)", out, code)
	}
}

// TestCLIBadOptions pins the messages of lua.c's print_usage.
func TestCLIBadOptions(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-Ex"}, "unrecognized option '-Ex'"},
		{[]string{"-z"}, "unrecognized option '-z'"},
		{[]string{"-e"}, "'-e' needs argument"},
		{[]string{"-l"}, "'-l' needs argument"},
		{[]string{"-e", "-v"}, "'-e' needs argument"},
	} {
		out, errb, code := cliRun(t, tc.args...)
		if code != 1 {
			t.Errorf("lua %v exited %d", tc.args, code)
		}
		if !strings.Contains(errb, tc.want) {
			t.Errorf("lua %v stderr = %q, want %q", tc.args, errb, tc.want)
		}
		if !strings.Contains(errb, "usage: lua [options] [script [args]]") {
			t.Errorf("lua %v stderr = %q, want the usage message", tc.args, errb)
		}
		if out != "" {
			t.Errorf("lua %v wrote to stdout: %q", tc.args, out)
		}
	}
}

// TestCLIEAndL pins '-e' and '-l', including the '-l g=mod' form and the
// fact that they run before the script.
func TestCLIEAndL(t *testing.T) {
	out, errb, code := cliRun(t, "-eprint(1)", "-e", "a=3", "-e", "print(a)")
	if code != 0 || out != "1\n3\n" {
		t.Errorf("-e: (%q, %d) stderr %q", out, code, errb)
	}
	out, errb, code = cliRun(t, "-l", "string", "-e", "print(type(string.format))")
	if code != 0 || out != "function\n" {
		t.Errorf("-l string: (%q, %d) stderr %q", out, code, errb)
	}
	out, errb, code = cliRun(t, "-lm=math", "-e", "print(m.sin(0))")
	if code != 0 || out != "0.0\n" {
		t.Errorf("-l m=math: (%q, %d) stderr %q", out, code, errb)
	}
	/* a failing -e stops the interpreter before the script runs */
	path := cliScript(t, "print('script')")
	out, errb, code = cliRun(t, "-e", "error('nope')", path)
	if code != 1 {
		t.Errorf("failing -e exited %d", code)
	}
	if strings.Contains(out, "script") {
		t.Errorf("script ran after a failing -e: %q", out)
	}
}

// TestCLIDashDash pins '--' (a following '-' is a file name, not stdin)
// and the bare '-' (stdin).
func TestCLIDashDash(t *testing.T) {
	path := cliScript(t, "print('script', ...)")
	out, errb, code := cliRun(t, "--", path, "x")
	if code != 0 || out != "script\tx\n" {
		t.Errorf("-- script: (%q, %d) stderr %q", out, code, errb)
	}
	out, errb, code = cliRun(t, path, "-h")
	if code != 0 || out != "script\t-h\n" {
		t.Errorf("script -h: (%q, %d) stderr %q", out, code, errb)
	}
}

// TestCLIWarningsOn pins '-W': it turns the warning system on, which
// 'warn' is otherwise silent about.
func TestCLIWarningsOn(t *testing.T) {
	path := cliScript(t, "warn('hi', ' there')")
	out, errb, code := cliRun(t, "-W", path)
	if code != 0 {
		t.Fatalf("-W exited %d: %s%s", code, out, errb)
	}
	if errb != "Lua warning: hi there\n" {
		t.Errorf("-W stderr = %q", errb)
	}
	out, errb, code = cliRun(t, path)
	if code != 0 {
		t.Fatalf("exited %d: %s%s", code, out, errb)
	}
	if errb != "" {
		t.Errorf("without -W the warning must stay off, got %q", errb)
	}
}

// TestCLINoEnv pins '-E': the packages must ignore LUA_PATH then.
func TestCLINoEnv(t *testing.T) {
	path := cliScript(t, "print(package.path)")
	t.Setenv("LUA_PATH", "xxx")
	out, errb, code := cliRun(t, "-E", path)
	if code != 0 {
		t.Fatalf("-E exited %d: %s%s", code, out, errb)
	}
	if strings.Contains(out, "xxx") {
		t.Errorf("-E must ignore LUA_PATH, got %q", out)
	}
	out, _, code = cliRun(t, path)
	if code != 0 || !strings.Contains(out, "xxx") {
		t.Errorf("without -E LUA_PATH must be used, got (%q, %d)", out, code)
	}
}
