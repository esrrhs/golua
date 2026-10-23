// Command lua is the standalone Lua interpreter, a Go port of lua.c
// (PUC-Lua 5.3.4). Translation target: full CLI/REPL/docall alignment.
// Platform SIGINT lives in signal_unix.go / signal_windows.go (see NOTES).
//
// Usage:
//
//	lua [options] [script [args]]
//
// Options:
//
//	-e stat  execute string 'stat'
//	-i       enter interactive mode after executing 'script'
//	-l name  require library 'name'
//	-v       show version information
//	-E       ignore environment variables
//	--       stop handling options
//	-        stop handling options and execute stdin
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	lua "github.com/esrrhs/golua"
)

// progName is argv[0], as in lua.c (which uses it to prefix the messages
// of uncaught errors).
var progName = "lua"

const (
	luaPrompt  = "> "
	luaPrompt2 = ">> "
)

// version is LUA_COPYRIGHT, which is what lua.c's 'print_version' writes
// for '-v'.  This port is a translation of PUC-Lua 5.3.4, whose licence
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
	L = lua.NewState() /* create state */
	if L == nil {
		/* lua.c: l_message(argv[0], "cannot create state: not enough memory") */
		pname := "lua"
		if len(argv) > 0 && argv[0] != "" {
			pname = argv[0]
		}
		fmt.Fprintf(os.Stderr, "%s: cannot create state: not enough memory\n", pname)
		return 1
	}
	defer L.Close()
	/* the libraries are opened inside 'pmain', after '-E' has had its
	   say about the environment (see pmain) */

	L.PushGoClosure(pmain, 0)
	L.PushInteger(int64(len(argv))) /* 1st argument: argc */
	L.PushInteger(0)                /* 2nd argument: argv (lightuserdata in C) */
	status := L.PCall(2, 1, 0)      /* do the call */
	result := L.ToBoolean(-1)       /* get result */
	/* 'report' is what removes the error object, as in C: it reads the
	   message off the top of the stack and pops it.  Popping it here
	   first -- which is what this used to do, before deciding whether
	   there was anything to report -- leaves 'report' holding whatever
	   happened to be underneath, so an error that escaped 'pmain'
	   printed as "lua: " with nothing after it.  The order in lua.c is
	   'toboolean' (a read), then 'report' (the pop). */
	if status != lua.LUA_OK {
		report(lastError()) /* an error outside 'pmain': report it too */
		return 1
	}
	L.Pop(1)
	/* 'pmain' returns false when a chunk it ran failed: the message has
	   already been reported, only the exit status is left to decide.
	   C: return (result && status == LUA_OK) ? EXIT_SUCCESS : EXIT_FAILURE */
	if !result {
		return 1
	}
	return 0
}

// pmain is 'pmain' of lua.c: everything the interpreter does, run inside a
// protected call so that an uncaught error becomes the single result of
// that call.
func pmain(L *lua.State) int {
	argcN, _ := L.ToInteger(1)
	argc := int(argcN) /* lua_tointeger(L, 1) */
	argv := pmainArgv           /* C: (char **)lua_touserdata(L, 2) */
	args, script := collectArgs(argv)
	L.CheckVersion() /* luaL_checkversion: interpreter has correct version */
	if len(argv) > 0 && argv[0] != "" { /* argv[0] && argv[0][0] */
		progName = argv[0]
	}
	if args == hasError { /* bad arg? */
		printUsage(argv[script]) /* 'script' has the index of the bad arg */
		L.PushBoolean(false)
		return 1
	}
	if args&hasV != 0 { /* option '-v'? */
		printVersion()
	}
	if args&hasEnv != 0 { /* option '-E'? */
		L.PushBoolean(true) /* signal for libraries to ignore env. vars. */
		L.SetField(lua.LUA_REGISTRYINDEX, "LUA_NOENV")
	}
	/* lua.c opens the libraries here, not before 'pmain': the packages
	   read LUA_PATH from the environment, and '-E' must already have
	   signalled that they should not. */
	L.OpenLibs() /* open standard libraries */
	createArgTable(L, argv, argc, script) /* create table 'arg' */
	if args&hasEnv == 0 { /* no option '-E'? */
		if !handleLuaInit() { /* run LUA_INIT */
			L.PushBoolean(false)
			return 1 /* error running LUA_INIT */
		}
	}
	if !runArgs(argv, script) { /* execute arguments -e and -l */
		L.PushBoolean(false)
		return 1 /* something failed */
	}
	if script < argc && /* execute main script (if there is one) */
		!handleScript(argv[script], script > 0 && argv[script-1] == "--") {
		L.PushBoolean(false)
		return 1
	}
	if args&hasI != 0 { /* -i option? */
		doREPL() /* do read-eval-print loop */
	} else if script == argc && args&(hasE|hasV) == 0 { /* no arguments? */
		if stdinIsTTY() { /* running in interactive mode? */
			printVersion()
			doREPL() /* do read-eval-print loop */
		} else {
			/* lua.c dofile(NULL): report errors but still signal success. */
			if err := runStdin(); err != nil {
				report(err)
			}
		}
	}
	L.PushBoolean(true) /* signal no errors */
	return 1
}

// ---------------------------------------------------------------------
// option handling
// ---------------------------------------------------------------------

// The option flags of lua.c (has_error / has_i / has_v / has_e / has_E).
const (
	hasError = 1  /* has_error — bad option */
	hasI     = 2  /* has_i — -i */
	hasV     = 4  /* has_v — -v */
	hasE     = 8  /* has_e — -e */
	hasEnv   = 16 /* has_E — -E */
)

func usage() string {
	return "usage: " + progName + " [options] [script [args]]\n" +
		"Available options are:\n" +
		"  -e stat  execute string 'stat'\n" +
		"  -i       enter interactive mode after executing 'script'\n" +
		"  -l name  require library 'name'\n" +
		"  -v       show version information\n" +
		"  -E       ignore environment variables\n" +
		"  --       stop handling options\n" +
		"  -        stop handling options and execute stdin\n"
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

// collectArgs is 'collectargs' of lua.c. It walks the options and returns
// the option mask plus 'first' (script index). Like C, 'script' is the
// index of the script name, or len(argv) (== argc) when there is none —
// not 0. createArgTable remaps script==argc to 0, matching C.
// When an option is invalid it returns hasError and 'script' is the
// index of the offending argument.
func collectArgs(argv []string) (args, script int) {
	for i := 1; i < len(argv); i++ {
		a := argv[i]
		script = i /* *first = i */
		if len(a) == 0 || a[0] != '-' { /* not an option? */
			return args, script /* stop handling options */
		}
		/* C reads argv[i][1], which is the NUL of a bare "-"; Go would
		   panic on that index, so the option character is fetched with
		   the same "zero if absent" meaning. */
		switch optChar(a) {
		case '-': /* '--' */
			if len(a) > 2 { /* extra characters after '--'? */
				return hasError, i /* invalid option */
			}
			script = i + 1 /* *first = i + 1; may equal argc */
			return args, script
		case 0: /* '-' */
			return args, script /* script "name" is '-' */
		case 'E':
			if len(a) > 2 { /* extra characters after 1st? */
				return hasError, i /* invalid option */
			}
			args |= hasEnv
		case 'i':
			args |= hasI /* (-i implies -v) */
			fallthrough
		case 'v':
			if len(a) > 2 { /* extra characters after 1st? */
				return hasError, i /* invalid option */
			}
			args |= hasV
		case 'e':
			args |= hasE /* FALLTHROUGH */
			fallthrough
		case 'l': /* both options need an argument */
			if len(a) == 2 { /* no concatenated argument? */
				i++ /* try next 'argv' */
				if i >= len(argv) || (len(argv[i]) > 0 && argv[i][0] == '-') {
					return hasError, script /* no next argument or it is another option */
				}
			}
		default: /* invalid option */
			return hasError, i
		}
	}
	script = len(argv) /* *first = i; no script name */
	return args, script
}

// runArgs is 'runargs' of lua.c: it executes the -e statements and -l
// libraries, in the order they appear, up to (but not including) the
// script (index n). It stops at the first failure, whose message has
// already been reported. Returns false if some code raises an error.
func runArgs(argv []string, n int) bool {
	for i := 1; i < n; i++ {
		option := optChar(argv[i])
		/* C: lua_assert(argv[i][0] == '-'); already checked by collectargs */
		if option == 'e' || option == 'l' {
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
		}
	}
	return true
}

// createArgTable builds the global 'arg' table the way lua.c's
// 'createargtable' does: arg[i - script] = argv[i] for every argument, so
// arg[0] is the script name, arg[1..] the script arguments and the
// negative indices hold the interpreter and the options. 'script' is the
// index of the script in argv, or argc when there is none (remapped to 0).
func createArgTable(L *lua.State, argv []string, argc, script int) {
	if script == argc { /* no script name? */
		script = 0
	}
	narg := argc - (script + 1) /* number of positive indices */
	L.CreateTable(narg, script+1) /* lua_createtable(narg, script+1) */
	for i := 0; i < argc; i++ {
		L.PushString(argv[i])
		L.RawSetI(-2, int64(i-script))
	}
	L.SetGlobal("arg")
}

// handleLuaInit is 'handle_luainit': it runs LUA_INIT_5_3 (or LUA_INIT),
// as a file when the value starts with '@' and as a chunk otherwise.
func handleLuaInit() bool {
	name := "=LUA_INIT_5_3"
	init, ok := os.LookupEnv("LUA_INIT_5_3")
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

// doLibrary is 'dolibrary' of lua.c 5.3.4: require(name) then setglobal(name).
func doLibrary(name string) error {
	L.GetGlobal("require")
	L.PushString(name)
	if err := docall(1, 1); err != nil { /* call 'require(name)' */
		return err
	}
	L.SetGlobal(name) /* global[name] = require return */
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
	n := pushArgs() /* may luaL_error; caught by pmain's pcall */
	return docall(n, lua.LUA_MULTRET)
}

// pushArgs is 'pushargs': it pushes arg[1..n] -- the arguments given to
// the script -- and returns how many there are.
func pushArgs() int {
	/* pushargs of lua.c: raise via luaL_error / luaL_checkstack (not Go error) */
	if L.GetGlobal("arg") != lua.LUA_TTABLE {
		L.Error2("'arg' is not a table")
	}
	n := int(L.Len2(-1)) /* luaL_len */
	L.CheckStack2(n+3, "too many arguments to script")
	for i := 1; i <= n; i++ {
		L.RawGetI(-i, int64(i)) /* the table moves down as values are pushed */
	}
	L.Remove(-(n + 1)) /* remove the table from the stack */
	return n
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
	n := pushArgs() /* may luaL_error; caught by pmain's pcall */
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
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	/* luaL_loadfile(NULL) runs skipBOM and skipcomment before compiling.
	   The newline that ends a shebang has to stay, or every later line
	   number is one too small. */
	return L.LoadBytes(skipStdinComment(data), name)
}

// skipStdinComment is skipBOM/skipcomment from lauxlib.c, the same rules
// LoadFile uses for a script on disk.
func skipStdinComment(data []byte) []byte {
	const bom = "\xEF\xBB\xBF"
	if len(data) >= len(bom) && string(data[:len(bom)]) == bom {
		data = data[len(bom):]
	}
	if len(data) > 0 && data[0] == '#' {
		i := 0
		for i < len(data) && data[i] != '\n' {
			i++
		}
		return data[i:]
	}
	return data
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
	enableSIGINT() /* signal(SIGINT, laction) — lua.c docall */
	status := L.PCall(narg, nres, base)
	disableSIGINT() /* signal(SIGINT, SIG_DFL) */
	L.Remove(base)  /* remove the message handler */
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
	/* lua_tostring, not luaL_tolstring: a table (or any other
	   non-string) is not converted again. C prints a fixed sentence
	   when the pointer comes back NULL. */
	s, ok := L.ToString(-1)
	L.Pop(1)
	if !ok {
		return fmt.Errorf("(error message not a string)")
	}
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
	/* C assigns the saved name back only after the loop. An error
	   that escapes — the prompt's __tostring, for one — longjmps
	   past that assignment, so report prints no program name.
	   Restoring it in every defer would put "lua: " in front of
	   that message. */
	defer func() {
		if e := recover(); e != nil {
			panic(e)
		}
		progName = oldProgName
	}()

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
		/* pushline (lua.c): '=' prefix → "return …" (5.2 compat); line is
		   then handed to loadline = addreturn + multiline. */
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

// replPrompt is get_prompt (lua.c): lua_tostring on _PROMPT/_PROMPT2.
func replPrompt(first bool) string {
	name, def := "_PROMPT2", luaPrompt2
	if first {
		name, def = "_PROMPT", luaPrompt
	}
	top := L.GetTop()
	defer L.SetTop(top)
	L.GetGlobal(name)
	if s, ok := L.ToString(-1); ok {
		return s /* even empty string is a valid prompt */
	}
	return def
}

// incomplete is lua.c incomplete: status==LUA_ERRSYNTAX and message ends with <eof>.
func incomplete(err error) bool {
	le, ok := err.(*lua.LuaError)
	if !ok || le.Status != lua.LUA_ERRSYNTAX {
		return false
	}
	return strings.HasSuffix(le.Message, "<eof>")
}

// loadLine is loadline of lua.c (addreturn + multiline), after pushline has
// produced the first line in 'chunk' (and applied the '=' → 'return' rewrite).
// Try "return <line>;" first; on failure, compile as a statement with
// continuation lines while incomplete.
func loadLine(chunk *string, in *bufio.Scanner) error {
	/* addreturn: load "return %s;" — trailing semicolon matches C pushfstring */
	if err := L.LoadBytes([]byte("return "+*chunk+";"), "=stdin"); err == nil {
		return nil
	}
	L.SetTop(0) /* discard failed addreturn attempt (LoadBytes already restored top) */
	for {
		err := L.LoadBytes([]byte(*chunk), "=stdin")
		if err == nil {
			return nil
		}
		if !incomplete(err) {
			return err /* cannot continue: report and go on */
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
	L.CheckStack2(lua.LUA_MINSTACK, "too many results to print")
	L.GetGlobal("print")
	L.Insert(1)
	if L.PCall(n, 0, 0) != lua.LUA_OK {
		msg, _ := L.ToString(-1)
		/* l_message(progname, ...): progname is NULL in REPL */
		if progName == "" {
			fmt.Fprintf(os.Stderr, "error calling 'print' (%s)\n", msg)
		} else {
			fmt.Fprintf(os.Stderr, "%s: error calling 'print' (%s)\n", progName, msg)
		}
		L.Pop(1)
	}
}
