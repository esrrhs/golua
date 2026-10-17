// Command lua is the standalone Lua interpreter, a Go port of lua.c.
//
// Usage:
//
//	lua [options] [script [args]]
//
// Options:
//
//	-e stat  execute string 'stat'
//	-l name  require library 'name'
//	-i       enter interactive mode after executing 'script'
//	-v       show version information
//	-        stop handling options and execute stdin as a file
//	--       stop handling options
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	lua "github.com/esrrhs/fakescript_go"
)

// progName is argv[0], as in lua.c (which uses it to prefix the messages
// of uncaught errors).
var progName = "lua"

const (
	luaPrompt  = "> "
	luaPrompt2 = ">> "
)

// version is LUA_COPYRIGHT, which is what lua.c's 'print_version' writes
// for '-v'.  This port is a translation of PUC-Lua 5.4.9, whose licence
// (MIT) requires the notice to be kept, and the standalone interpreter
// has to be indistinguishable from the reference one anyway: scripts
// spawn 'lua -v' and look for "PUC-Rio" in its output.
const version = lua.LUA_COPYRIGHT

var L *lua.State

// pmainArgv is the argv handed down to 'pmain'. lua.c passes it as a light
// userdata; this port has no light userdata, so it is a package variable.
var pmainArgv []string

func main() {
	os.Exit(pmainDriver(os.Args))
}

// pmainDriver is 'main' of lua.c. It creates the state and runs 'pmain'
// under a protected call, just like lua.c does with
// 'lua_pushcfunction(L, &pmain); lua_pcall(L, 2, 1, 0)'.
//
// That indirection is not cosmetic: it is what makes the host show up as
// the trailing "[C]: in ?" frame of every traceback, which is part of the
// reference interpreter's observable behaviour.
func pmainDriver(argv []string) int {
	pmainArgv = argv
	if len(argv) > 0 && argv[0] != "" {
		progName = argv[0]
	}
	L = lua.NewState()
	defer L.Close()
	/* the libraries are opened inside 'pmain', after '-E' has had its
	   say about the environment (see pmain) */

	L.PushGoClosure(pmain, 0)
	L.PushInteger(int64(len(argv))) /* 1st argument: argc */
	L.PushInteger(0)                /* 2nd argument: argv (a pointer in C) */
	status := L.PCall(2, 1, 0)
	result := L.ToBoolean(-1) /* get result */
	L.Pop(1)
	if status != lua.LUA_OK {
		report(lastError()) /* an error outside 'pmain': report it too */
		return 1
	}
	/* 'pmain' returns false when a chunk it ran failed: the message has
	   already been reported, only the exit status is left to decide. */
	if !result {
		return 1
	}
	return 0
}

// pmain is 'pmain' of lua.c: everything the interpreter does, run inside a
// protected call so that an uncaught error becomes the single result of
// that call.
func pmain(L *lua.State) int {
	argv := pmainArgv
	args, script := collectArgs(argv)
	if args == hasError { /* bad arg? */
		printUsage(argv[script]) /* 'script' has the index of the bad arg */
		L.PushBoolean(false)
		return 1
	}
	if args&hasV != 0 { /* option '-v'? */
		printVersion()
	}
	if args&hasEnv != 0 { /* option '-E'? */
		L.PushBoolean(true)
		L.SetField(lua.LUA_REGISTRYINDEX, "LUA_NOENV")
	}
	/* lua.c opens the libraries here, not before 'pmain': the packages
	   read LUA_PATH from the environment, and '-E' must already have
	   signalled that they should not. */
	L.OpenLibs()
	optlim := len(argv) /* first argv that is not an option */
	if script > 0 {
		optlim = script
	}
	createArgTable(L, argv, script) /* create table 'arg' */
	if args&hasEnv == 0 {           /* no option '-E'? */
		if !handleLuaInit() { /* run LUA_INIT */
			L.PushBoolean(false)
			return 1
		}
	}
	if !runArgs(argv, optlim) { /* execute arguments -e and -l */
		L.PushBoolean(false)
		return 1
	}
	if script > 0 { /* execute main script (if there is one) */
		if !handleScript(argv[script], script > 1 && argv[script-1] == "--") {
			L.PushBoolean(false)
			return 1 /* interrupt in case of error */
		}
	}
	if args&hasI != 0 { /* -i option? */
		doREPL()
	} else if script < 1 && args&(hasE|hasV) == 0 { /* no active option? */
		if stdinIsTTY() { /* running in interactive mode? */
			printVersion()
			doREPL()
		} else if err := runStdin(); err != nil { /* executes stdin as a file */
			report(err)
			L.PushBoolean(false)
			return 1
		}
	}
	L.PushBoolean(true) /* signal no errors */
	return 1
}

// ---------------------------------------------------------------------
// option handling
// ---------------------------------------------------------------------

// The option flags of lua.c.
const (
	hasError = 1  /* bad option */
	hasI     = 2  /* -i */
	hasV     = 4  /* -v */
	hasE     = 8  /* -e */
	hasEnv   = 16 /* -E */
)

func usage() string {
	return "usage: " + progName + " [options] [script [args]]\n" +
		"Available options are:\n" +
		"  -e stat   execute string 'stat'\n" +
		"  -i        enter interactive mode after executing 'script'\n" +
		"  -l mod    require library 'mod' into global 'mod'\n" +
		"  -l g=mod  require library 'mod' into global 'g'\n" +
		"  -v        show version information\n" +
		"  -E        ignore environment variables\n" +
		"  -W        turn warnings on\n" +
		"  --        stop handling options\n" +
		"  -         stop handling options and execute stdin\n"
}

// printUsage is print_usage: it names the offending option and then the
// usage message, all on stderr.
func printUsage(badoption string) {
	fmt.Fprintf(os.Stderr, "%s: ", progName)
	if len(badoption) > 1 && (badoption[1] == 'e' || badoption[1] == 'l') {
		fmt.Fprintf(os.Stderr, "'%s' needs argument\n", badoption)
	} else {
		fmt.Fprintf(os.Stderr, "unrecognized option '%s'\n", badoption)
	}
	fmt.Fprint(os.Stderr, usage())
}

// printVersion is print_version: LUA_COPYRIGHT and a newline on stdout.
func printVersion() { fmt.Println(version) }

// optChar is argv[i][1] of lua.c: the NUL terminator of a bare "-" reads
// as a zero byte there, which is the option that means "read stdin".
func optChar(a string) byte {
	if len(a) < 2 {
		return 0
	}
	return a[1]
}

// collectArgs is 'collectargs' of lua.c.  It walks the options, saving
// the program name in 'progName', and returns the option flags plus the
// index of the script in argv (0 when there is no script).  When an
// option is invalid it returns hasError and 'script' is the index of the
// offending argument.
func collectArgs(argv []string) (args, script int) {
	if len(argv) > 0 && argv[0] != "" { /* is there a program name? */
		progName = argv[0] /* save it */
	}
	for i := 1; i < len(argv); i++ { /* handle arguments */
		a := argv[i]
		script = i
		if len(a) == 0 || a[0] != '-' {
			return args, script /* not an option: this is the script */
		}
		/* C reads argv[i][1], which is the NUL of a bare "-"; Go would
		   panic on that index, so the option character is fetched with
		   the same "zero if absent" meaning. */
		switch optChar(a) {
		case '-': /* '--' */
			if len(a) > 2 { /* extra characters after '--'? */
				return hasError, i /* invalid option */
			}
			/* if there is a script name, it comes after '--' */
			if i+1 < len(argv) {
				script = i + 1
			} else {
				script = 0
			}
			return args, script
		case 0: /* '-' */
			return args, script /* script "name" is '-' */
		case 'E': /* ignore environment */
			if len(a) > 2 {
				return hasError, i
			}
			args |= hasEnv
		case 'W': /* turn warnings on */
			if len(a) > 2 {
				return hasError, i
			}
		case 'i':
			args |= hasI /* (-i implies -v) */
			if len(a) > 2 {
				return hasError, i
			}
			args |= hasV
		case 'v':
			if len(a) > 2 {
				return hasError, i
			}
			args |= hasV
		case 'e':
			args |= hasE /* FALLTHROUGH */
			fallthrough
		case 'l': /* both options need an argument */
			if len(a) == 2 { /* no concatenated argument? */
				i++ /* try next argv */
				if i >= len(argv) || (len(argv[i]) > 0 && argv[i][0] == '-') {
					return hasError, script /* no arg, or another option */
				}
			}
		default: /* invalid option */
			return hasError, i
		}
	}
	script = 0 /* no script name */
	return args, script
}

// runArgs is 'runargs' of lua.c: it executes the -e statements and -l
// libraries, in the order they appear, up to (but not including) the
// script.  It stops at the first failure, whose message has already been
// reported.
func runArgs(argv []string, optlim int) bool {
	for i := 1; i < optlim; i++ {
		option := optChar(argv[i])
		switch option {
		case 'e', 'l':
			extra := argv[i][2:] /* both options need an argument */
			if extra == "" {
				i++
				extra = argv[i]
			}
			var err error
			if option == 'e' {
				err = doString(extra, "=(command line)")
			} else {
				err = doLibrary(extra)
			}
			if err != nil {
				report(err)
				return false
			}
		case 'W':
			L.Warning("@on", false) /* warnings on */
		}
	}
	return true
}

// createArgTable builds the global 'arg' table the way lua.c's
// 'createargtable' does: arg[i - script] = argv[i] for every argument, so
// arg[0] is the script name, arg[1..] the script arguments and the
// negative indices hold the interpreter and the options.  'script' is the
// index of the script in argv, or 0 when there is none.
func createArgTable(L *lua.State, argv []string, script int) {
	L.NewTable()
	for i, a := range argv {
		L.PushString(a)
		L.RawSetI(-2, int64(i-script))
	}
	L.SetGlobal("arg")
}

// handleLuaInit is 'handle_luainit': it runs LUA_INIT_5_4 (or LUA_INIT),
// as a file when the value starts with '@' and as a chunk otherwise.
func handleLuaInit() bool {
	name := "=LUA_INIT_5_4"
	init, ok := os.LookupEnv("LUA_INIT_5_4")
	if !ok {
		name = "=LUA_INIT"
		init, ok = os.LookupEnv("LUA_INIT")
	}
	if !ok {
		return true
	}
	var err error
	if len(init) > 0 && init[0] == '@' {
		err = runFile(init[1:])
	} else {
		err = doString(init, name)
	}
	if err != nil {
		report(err)
		return false
	}
	return true
}

// doLibrary is 'dolibrary': it requires 'mod' and stores the result in
// the global 'globname'.  A '-l g=mod' argument names the global
// explicitly; without a suffix mark the global is the module name.
func doLibrary(globname string) error {
	modname := globname
	suffix := -1
	if i := strings.IndexByte(globname, '='); i >= 0 {
		modname = globname[i+1:] /* module name starts after the '=' */
		globname = globname[:i]  /* global name ends here */
	} else if i := strings.IndexByte(modname, '-'); i >= 0 {
		suffix = i /* the ignore mark starts a suffix */
	}
	L.GetGlobal("require")
	L.PushString(modname)
	if err := docall(1, 1); err != nil { /* call 'require(modname)' */
		return err
	}
	if suffix >= 0 { /* is there a suffix mark? */
		globname = globname[:suffix] /* remove suffix from global name */
	}
	L.SetGlobal(globname) /* globname = require(modname) */
	return nil
}

// stdinIsTTY is lua_stdin_is_tty: without it the interpreter would start
// a REPL on a redirected standard input.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------------------------------------------------------------------
// running chunks
// ---------------------------------------------------------------------

// handleScript is 'handle_script': it loads the script and runs it with
// its arguments (the positive entries of 'arg') as varargs.  A '-' means
// standard input, unless it was quoted with '--'.
func handleScript(fname string, afterDashDash bool) bool {
	var err error
	if fname == "-" && !afterDashDash {
		err = runStdinScript()
	} else {
		err = runScript(fname)
	}
	if err != nil {
		report(err)
		return false
	}
	return true
}

// runScript loads 'path' and runs it, pushing the script arguments first
// the way lua.c's 'pushargs' does.
func runScript(path string) error {
	if err := L.LoadFile(path); err != nil {
		return err
	}
	n, err := pushArgs()
	if err != nil {
		return err
	}
	return docall(n, lua.LUA_MULTRET)
}

// pushArgs is 'pushargs': it pushes arg[1..n] -- the arguments given to
// the script -- and returns how many there are.
func pushArgs() (int, error) {
	if L.GetGlobal("arg") != lua.LUA_TTABLE {
		return 0, fmt.Errorf("'arg' is not a table")
	}
	n := int(L.Len2(-1)) /* luaL_len */
	if !L.CheckStack(n + 3) {
		return 0, fmt.Errorf("too many arguments to script")
	}
	for i := 1; i <= n; i++ {
		L.RawGetI(-i, int64(i)) /* the table moves down as values are pushed */
	}
	L.Remove(-(n + 1)) /* remove the table from the stack */
	return n, nil
}

func runFile(path string) error {
	if err := L.LoadFile(path); err != nil {
		return err
	}
	return callChunk()
}

// runStdin is 'dofile(L, NULL)' of lua.c: standard input run when there
// is no script to run.  It gets no arguments -- 'arg' holds the
// interpreter's own, and lua.c's dochunk calls the chunk with none.
func runStdin() error { return doReader(os.Stdin, "=stdin") }

// runStdinScript is what 'handle_script' does when the script is named
// "-": standard input *is* the script, and a script runs with the
// positive entries of 'arg' as its varargs.  That is not the same thing
// as runStdin, and mixing the two up shows: 'echo "print(...)" | lua -e
// "arg[1] = 100" -' prints 100 in PUC-Lua and nothing here.
func runStdinScript() error {
	if err := loadStdin(); err != nil {
		return err
	}
	n, err := pushArgs()
	if err != nil {
		return err
	}
	return docall(n, lua.LUA_MULTRET)
}

func loadStdin() error { return loadReader(os.Stdin, "=stdin") }

func doReader(f *os.File, name string) error {
	if err := loadReader(f, name); err != nil {
		return err
	}
	return callChunk()
}

func loadReader(f *os.File, name string) error {
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		/* fall back to reading the file handle directly */
		buf := make([]byte, 0, 4096)
		tmp := make([]byte, 4096)
		for {
			n, e := f.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if e != nil {
				break
			}
		}
		data = buf
	}
	if len(data) > 0 && data[0] == '#' { /* skip exec. file comment */
		if i := strings.IndexByte(string(data), '\n'); i >= 0 {
			data = data[i+1:]
		}
	}
	return L.LoadBytes(data, name)
}

func doString(src, name string) error {
	if err := L.LoadBytes([]byte(src), name); err != nil {
		return err
	}
	return callChunk()
}

// msghandler is the message handler that lua.c installs under every
// chunk it runs: it turns the error object into a string (honouring
// '__tostring') and appends a standard traceback.
func msghandler(L *lua.State) int {
	msg, ok := L.ToString(1) /* strings and numbers are messages as they are */
	if !ok {
		if L.CallMeta(1, "__tostring") && L.Type(-1) == lua.LUA_TSTRING {
			return 1 /* that is the message */
		}
		msg = fmt.Sprintf("(error object is a %s value)", L.TypeName(1))
		ok = true
	}
	L.Traceback(L, msg, ok, 1)
	return 1
}

// docall is 'docall' of lua.c: it runs the function that sits below
// 'narg' arguments with 'msghandler' installed under it, so that an
// uncaught error comes back with a traceback.
func docall(narg, nres int) error {
	base := L.GetTop() - narg /* function index */
	L.PushGoClosure(msghandler, 0)
	L.Insert(base) /* put the handler under the function and its arguments */
	status := L.PCall(narg, nres, base)
	L.Remove(base) /* remove the message handler */
	if status != lua.LUA_OK {
		return lastError()
	}
	return nil
}

// callChunk runs a chunk the way lua.c's 'dochunk' does: no arguments and
// *no* results, so that the stack is left exactly where it was before the
// chunk was pushed.  That matters: 'lua -e "..." script.lua' runs the
// chunk from a deeper stack when '-e' leaves anything behind, and the
// depth of the stack is observable (the official test suite counts the
// calls it takes to overflow it).
func callChunk() error {
	return docall(0, 0)
}

func lastError() error {
	s, _ := L.ToString(-1)
	L.Pop(1)
	return fmt.Errorf("%s", s)
}

// report is 'report' plus 'l_message' of lua.c: the message on stderr,
// prefixed with progname -- except inside the REPL, where lua.c clears
// progname precisely so that an error in a line has no prefix at all.
func report(err error) {
	if progName == "" {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", progName, err)
}

// ---------------------------------------------------------------------
// REPL
// ---------------------------------------------------------------------

func doREPL() {
	oldProgName := progName
	progName = "" /* no 'progname' on errors in interactive mode */
	defer func() { progName = oldProgName }()

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for {
		/* loadline starts by emptying the stack: a line is read against
		   nothing but what the REPL itself has put there, and the
		   results of the previous line are gone.  Without this the
		   REPL would hand 'print' the arguments pmain was called with
		   -- 'lua -i' and then '1+1' would print three numbers. */
		L.SetTop(0)
		fmt.Print(replPrompt(true))
		if !in.Scan() {
			break
		}
		chunk := in.Text()
		/* lua.c's 'pushline': a line starting with '=' means "print the
		   value of what follows", kept from 5.0. */
		if strings.HasPrefix(chunk, "=") {
			chunk = "return " + chunk[1:]
		}
		loadErr := loadLine(&chunk, in)
		if loadErr != nil {
			report(loadErr)
		} else if err := docall(0, lua.LUA_MULTRET); err != nil {
			report(err) /* with a traceback: docall installs msghandler */
		} else {
			printResults()
		}
	}
	L.SetTop(0)
	fmt.Println() /* lua_writeline, which ends doREPL */
}

// replPrompt is lua.c's 'get_prompt': the global _PROMPT, or _PROMPT2 on
// a continuation line, with 'tostring' applied to it -- which is the only
// reason a prompt that is not a string can work at all.  A _PROMPT that
// is nil means the default; one that is the empty string is an empty
// prompt and not the default.
func replPrompt(first bool) string {
	name, def := "_PROMPT2", luaPrompt2
	if first {
		name, def = "_PROMPT", luaPrompt
	}
	top := L.GetTop()
	defer L.SetTop(top)
	L.GetGlobal(name)
	if L.Type(-1) == lua.LUA_TNIL {
		return def
	}
	if L.CallMeta(-1, "__tostring") { /* luaL_tolstring asks first */
		if s, ok := L.ToString(-1); ok {
			return s /* a number counts: 'lua_isstring' says it does */
		}
		return def /* C would raise "'__tostring' must return a string" */
	}
	switch L.Type(-1) {
	case lua.LUA_TNUMBER, lua.LUA_TSTRING:
		if s, ok := L.ToString(-1); ok {
			return s
		}
	case lua.LUA_TBOOLEAN:
		if L.ToBoolean(-1) {
			return "true"
		}
		return "false"
	}
	return def
}

// loadLine compiles one REPL line, reading continuation lines from 'in'
// while the line is unfinished, and leaves the chunk on the stack.  It
// answers the error to report, if any: the syntax error of a finished
// line, or -- when input runs out in the middle of one -- the error of
// the last try, which is what lua.c's 'multiline' returns and 'report'
// then prints.
//
// It is 'addreturn' and 'multiline' of lua.c together: "return <line>"
// first, so that an expression prints its value, and the line on its own
// when that does not compile.
func loadLine(chunk *string, in *bufio.Scanner) error {
	for {
		if err := L.LoadBytes([]byte("return "+*chunk), "=stdin"); err == nil {
			return nil
		}
		L.SetTop(0)
		err := L.LoadBytes([]byte(*chunk), "=stdin")
		if err == nil {
			return nil
		}
		/* lua.c's 'incomplete': the message ends with <eof>. */
		if !strings.HasSuffix(err.Error(), "<eof>") {
			return err /* a real syntax error: report it and go on */
		}
		fmt.Print(replPrompt(false))
		if !in.Scan() {
			return err /* input ran out mid-line: lua.c reports this too */
		}
		*chunk += "\n" + in.Text()
	}
}

// printResults is 'l_print' of lua.c: the results of a line are handed
// to the global 'print' -- the whole point being that 'print' is a
// global and not a built-in, so a REPL whose 'print' has been replaced
// prints with the replacement and complains when it cannot call it.
// Building the line here from 'tostring' would silently swallow that,
// which is what 'lua -e "print=nil" -i' is testing.
func printResults() {
	n := L.GetTop()
	if n == 0 { /* any result to be printed? */
		return
	}
	L.CheckStack(lua.LUA_MINSTACK) /* "too many results to print" */
	L.GetGlobal("print")
	L.Insert(1)
	if L.PCall(n, 0, 0) != lua.LUA_OK {
		msg, _ := L.ToString(-1)
		L.Pop(1)
		fmt.Fprintf(os.Stderr, "error calling 'print' (%s)\n", msg)
	}
}
