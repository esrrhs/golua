// Command lua is the standalone Lua interpreter, a Go port of lua.c
// (PUC-Rio Lua 5.4.9).
//
// Usage:
//
//	lua [options] [script [args]]
//
// Options:
//
//	-e stat   execute string 'stat'
//	-i        enter interactive mode after executing 'script'
//	-l mod    require library 'mod' into global 'mod'
//	-l g=mod  require library 'mod' into global 'g'
//	-v        show version information
//	-E        ignore environment variables
//	-W        turn warnings on
//	--        stop handling options
//	-         stop handling options and execute stdin
//
// Platform signal_* companions (SIGINT → hook) are intentional FLAG; do not
// delete them when present.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"

	lua "github.com/esrrhs/golua"
)

// progName is progname of lua.c (LUA_PROGNAME). Empty string stands for
// C's NULL: no prefix on error messages (REPL clears it).
var progName = "lua"

// globalL is globalL of lua.c: visible to laction for SIGINT→hook.
// FLAG: set in docall when signal_* companions are present.
var globalL *lua.State

// pmainArgv is the argv handed to pmain. lua.c passes it as a light
// userdata; this port has no useful light-userdata argv, so it is a
// package variable (FLAG).
var pmainArgv []string

const (
	luaPrompt  = "> "  // LUA_PROMPT
	luaPrompt2 = ">> " // LUA_PROMPT2
)

// version is LUA_COPYRIGHT (print_version).
const version = lua.LUA_COPYRIGHT

const (
	luaInitVar        = "LUA_INIT"     // LUA_INIT_VAR
	luaInitVarVersion = "LUA_INIT_5_4" // LUA_INITVARVERSION
	luaIgMark         = '-'            // LUA_IGMARK
	eofMark           = "<eof>"        // EOFMARK
)

func main() {
	os.Exit(luaMain(os.Args))
}

// luaMain is main of lua.c.
func luaMain(argv []string) int {
	pmainArgv = argv
	L := lua.NewState() /* create state */
	if L == nil {
		/* FLAG: Go NewState does not return nil; kept for lua.c shape. */
		name := "lua"
		if len(argv) > 0 {
			name = argv[0]
		}
		lMessage(name, "cannot create state: not enough memory")
		return 1 /* EXIT_FAILURE */
	}
	/* lua_gc(L, LUA_GCSTOP): FLAG Go GC — no-op (intentional). */
	L.PushGoFunc(pmain)                      /* to call 'pmain' in protected mode */
	L.PushInteger(int64(len(argv)))          /* 1st argument */
	L.PushLightUserData(unsafe.Pointer(nil)) /* 2nd argument (argv; see pmainArgv) */
	status := L.PCall(2, 1, 0)               /* do the call */
	result := L.ToBoolean(-1)                /* get result */
	report(L, status)
	L.Close()
	if result && status == lua.LUA_OK {
		return 0 /* EXIT_SUCCESS */
	}
	return 1 /* EXIT_FAILURE */
}

/*
** Hook set by signal function to stop the interpreter (lstop).
** C: lua_sethook(L, NULL, 0, 0); luaL_error(L, "interrupted!");
** FLAG: live SIGINT→SetHook belongs in signal_*.go when present —
** SetHook's *dbAr is not exported to cmd/lua, so this stub only keeps
** the error string and lua.c shape.
 */
func lstop(L *lua.State) int {
	/* lua_sethook(L, NULL, 0, 0) — FLAG: hook API not usable from cmd/lua */
	L.Error2("interrupted!")
	return 0
}

/*
** Function to be called at a C signal (laction).
** C: setsignal(i, SIG_DFL); lua_sethook(globalL, lstop, MASK*, 1);
** FLAG: see signal_*.go companions when present (do not delete).
 */
func laction(_ int) {
	_ = lstop
	_ = globalL
	/* flag := LUA_MASKCALL|LUA_MASKRET|LUA_MASKLINE|LUA_MASKCOUNT */
}

// printUsage is print_usage.
func printUsage(badoption string) {
	fmt.Fprintf(os.Stderr, "%s: ", progName)
	/* C: badoption[1] — bare "-" has [1]=='\0', not e/l */
	if optChar(badoption) == 'e' || optChar(badoption) == 'l' {
		fmt.Fprintf(os.Stderr, "'%s' needs argument\n", badoption)
	} else {
		fmt.Fprintf(os.Stderr, "unrecognized option '%s'\n", badoption)
	}
	fmt.Fprintf(os.Stderr,
		"usage: %s [options] [script [args]]\n"+
			"Available options are:\n"+
			"  -e stat   execute string 'stat'\n"+
			"  -i        enter interactive mode after executing 'script'\n"+
			"  -l mod    require library 'mod' into global 'mod'\n"+
			"  -l g=mod  require library 'mod' into global 'g'\n"+
			"  -v        show version information\n"+
			"  -E        ignore environment variables\n"+
			"  -W        turn warnings on\n"+
			"  --        stop handling options\n"+
			"  -         stop handling options and execute stdin\n",
		progName)
}

// lMessage is l_message: pname empty means C's NULL (no prefix).
func lMessage(pname, msg string) {
	if pname != "" {
		fmt.Fprintf(os.Stderr, "%s: ", pname)
	}
	fmt.Fprintf(os.Stderr, "%s\n", msg)
}

// report is report: on error, print the message on top of the stack and pop it.
func report(L *lua.State, status int) int {
	if status != lua.LUA_OK {
		msg, ok := L.ToString(-1)
		if !ok {
			msg = "(error message not a string)"
		}
		lMessage(progName, msg)
		L.Pop(1) /* remove message */
	}
	return status
}

// msghandler is msghandler.
func msghandler(L *lua.State) int {
	msg, ok := L.ToString(1)
	if !ok { /* is error object not a string? */
		if L.CallMeta(1, "__tostring") && /* does it have a metamethod */
			L.Type(-1) == lua.LUA_TSTRING { /* that produces a string? */
			return 1 /* that is the message */
		}
		msg = L.PushFString("(error object is a %s value)", L.TypeName(1))
	}
	L.Traceback(L, msg, true, 1) /* append a standard traceback */
	return 1                     /* return the traceback */
}

// docall is docall: lua_pcall with msghandler and C-signal handler.
//
// FLAG signal: globalL = L; setsignal(SIGINT, laction); ...; setsignal(SIGINT, SIG_DFL).
// Companions live in signal_*.go when present (do not delete).
func docall(L *lua.State, narg, nres int) int {
	base := L.GetTop() - narg /* function index */
	L.PushGoFunc(msghandler)  /* push message handler */
	L.Insert(base)            /* put it under function and args */
	globalL = L               /* to be available to 'laction' */
	_ = laction               /* FLAG signal: setsignal(SIGINT, laction) — signal_*.go */
	status := L.PCall(narg, nres, base)
	_ = laction    /* FLAG signal: setsignal(SIGINT, SIG_DFL) */
	L.Remove(base) /* remove message handler from the stack */
	return status
}

// printVersion is print_version.
func printVersion() {
	fmt.Println(version) /* lua_writestring + lua_writeline */
}

// createArgTable is createargtable.
func createArgTable(L *lua.State, argv []string, argc, script int) {
	narg := argc - (script + 1) /* number of positive indices */
	L.CreateTable(narg, script+1)
	for i := 0; i < argc; i++ {
		L.PushString(argv[i])
		L.RawSetI(-2, int64(i-script))
	}
	L.SetGlobal("arg")
}

// dochunk is dochunk.
func dochunk(L *lua.State, status int) int {
	if status == lua.LUA_OK {
		status = docall(L, 0, 0)
	}
	return report(L, status)
}

// loadErrStatus pushes a Load* error message (Go Load* clears the stack
// on failure; C's luaL_load* leaves it) and returns its status.
func loadErrStatus(L *lua.State, err error) int {
	if err == nil {
		return lua.LUA_OK
	}
	if le, ok := err.(*lua.LuaError); ok {
		L.PushString(le.Message)
		return le.Status
	}
	L.PushString(err.Error())
	return lua.LUA_ERRFILE
}

// loadFile is luaL_loadfile: empty name means stdin (NULL in C).
func loadFile(L *lua.State, name string) int {
	if name == "" {
		return loadStdin(L)
	}
	return loadErrStatus(L, L.LoadFile(name))
}

// loadStdin is luaL_loadfile(L, NULL): chunk named "=stdin", with
// skipBOM/skipcomment (same rules as LoadFile on disk).
func loadStdin(L *lua.State) int {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		L.PushString(err.Error())
		return lua.LUA_ERRFILE
	}
	return loadErrStatus(L, L.LoadBytes(skipFileComment(data), "=stdin"))
}

// skipFileComment is skipBOM/skipcomment from lauxlib.c (kept local:
// core.skipFileComment is unexported; same algorithm as LoadFile).
//
// When a shebang line is present, C's loadfilex always inserts a fresh
// '\n' (for line numbers) and then the first byte after the shebang.
// If the shebang runs to EOF with no newline, that is just '\n' — not an
// empty chunk.
func skipFileComment(data []byte) []byte {
	const bom = "\xEF\xBB\xBF"
	if len(data) >= len(bom) && string(data[:len(bom)]) == bom {
		data = data[len(bom):] /* BOM is discarded only when complete */
	}
	if len(data) > 0 && data[0] == '#' { /* Unix exec. file? */
		i := 0
		for i < len(data) && data[i] != '\n' {
			i++
		}
		if i < len(data) {
			return data[i:] /* keep the newline + rest (≡ C's synthetic \n + rest) */
		}
		return []byte{'\n'} /* shebang to EOF: C inserts \n only */
	}
	return data
}

// dofile is dofile.
func dofile(L *lua.State, name string) int {
	return dochunk(L, loadFile(L, name))
}

// dostring is dostring.
func dostring(L *lua.State, s, name string) int {
	return dochunk(L, loadErrStatus(L, L.LoadBytes([]byte(s), name)))
}

// dolibrary is dolibrary: globname[=modname] → globname = require(modname).
func dolibrary(L *lua.State, globname string) int {
	var status int
	suffix := -1
	modname := globname
	if i := strings.IndexByte(globname, '='); i >= 0 {
		modname = globname[i+1:] /* module name starts after the '=' */
		globname = globname[:i]  /* global name ends here */
	} else {
		modname = globname /* module name is equal to global name */
		if i := strings.IndexByte(modname, luaIgMark); i >= 0 {
			suffix = i /* look for a suffix mark */
		}
	}
	L.GetGlobal("require")
	L.PushString(modname)
	status = docall(L, 1, 1) /* call 'require(modname)' */
	if status == lua.LUA_OK {
		if suffix >= 0 { /* is there a suffix mark? */
			globname = globname[:suffix] /* remove suffix from global name */
		}
		L.SetGlobal(globname) /* globname = require(modname) */
	}
	return report(L, status)
}

// pushArgs is pushargs.
func pushArgs(L *lua.State) int {
	if L.GetGlobal("arg") != lua.LUA_TTABLE {
		L.Error2("'arg' is not a table")
	}
	n := int(L.Len2(-1))
	L.CheckStack2(n+3, "too many arguments to script")
	for i := 1; i <= n; i++ {
		L.RawGetI(-i, int64(i))
	}
	L.Remove(-(n + 1)) /* remove table from the stack (C: -i after loop) */
	return n
}

// handleScript is handle_script. argv[0] is the script; argv[-1] is the
// preceding argument (checked for "--"), mirrored via fullArgv/script.
func handleScript(L *lua.State, fullArgv []string, script int) int {
	fname := fullArgv[script]
	if fname == "-" && !(script > 0 && fullArgv[script-1] == "--") {
		fname = "" /* stdin */
	}
	status := loadFile(L, fname)
	if status == lua.LUA_OK {
		n := pushArgs(L) /* push arguments to script */
		status = docall(L, n, lua.LUA_MULTRET)
	}
	return report(L, status)
}

/* bits of various argument indicators in 'args' */
const (
	hasError = 1  /* has_error — bad option */
	hasI     = 2  /* has_i — -i */
	hasV     = 4  /* has_v — -v */
	hasE     = 8  /* has_e — -e */
	hasEnv   = 16 /* has_E — -E */
)

// optChar is argv[i][1]: the NUL of a bare "-" reads as 0 in C.
func optChar(a string) byte {
	if len(a) < 2 {
		return 0
	}
	return a[1]
}

// collectArgs is collectargs. Returns (args mask, first). first is -1 if
// there is no program name, 0 if there is no script, or the script index.
func collectArgs(argv []string) (args, first int) {
	if len(argv) == 0 { /* C: argv[0] == NULL */
		return 0, -1
	}
	if argv[0] != "" { /* not empty? */
		progName = argv[0] /* save it */
	}
	for i := 1; i < len(argv); i++ { /* handle arguments */
		first = i
		a := argv[i]
		if len(a) == 0 || a[0] != '-' { /* not an option? */
			return args, first /* stop handling options */
		}
		switch optChar(a) { /* else check option */
		case '-': /* '--' */
			if len(a) > 2 { /* extra characters after '--'? */
				return hasError, first /* invalid option */
			}
			/* if there is a script name, it comes after '--' */
			if i+1 < len(argv) {
				first = i + 1
			} else {
				first = 0
			}
			return args, first
		case 0: /* '-' */
			return args, first /* script "name" is '-' */
		case 'E':
			if len(a) > 2 { /* extra characters? */
				return hasError, first
			}
			args |= hasEnv
		case 'W':
			if len(a) > 2 {
				return hasError, first
			}
		case 'i':
			args |= hasI /* (-i implies -v) */
			fallthrough  /* FALLTHROUGH */
		case 'v':
			if len(a) > 2 {
				return hasError, first
			}
			args |= hasV
		case 'e':
			args |= hasE /* FALLTHROUGH */
			fallthrough
		case 'l': /* both options need an argument */
			if len(a) == 2 { /* no concatenated argument? */
				i++ /* try next 'argv' */
				if i >= len(argv) || (len(argv[i]) > 0 && argv[i][0] == '-') {
					return hasError, first /* no next argument or it is another option */
				}
			}
		default: /* invalid option */
			return hasError, first
		}
	}
	first = 0 /* no script name */
	return args, first
}

// runArgs is runargs: process -e, -l, and -W up to (not including) n.
func runArgs(L *lua.State, argv []string, n int) int {
	for i := 1; i < n; i++ {
		option := optChar(argv[i])
		/* lua_assert(argv[i][0] == '-') — already checked */
		switch option {
		case 'e', 'l':
			extra := argv[i][2:] /* both options need an argument */
			if extra == "" {
				i++
				extra = argv[i]
			}
			var status int
			if option == 'e' {
				status = dostring(L, extra, "=(command line)")
			} else {
				status = dolibrary(L, extra)
			}
			if status != lua.LUA_OK {
				return 0
			}
		case 'W':
			L.Warning("@on", false) /* warnings on */
		}
	}
	return 1
}

// handleLuaInit is handle_luainit.
func handleLuaInit(L *lua.State) int {
	name := "=" + luaInitVarVersion
	init, ok := os.LookupEnv(luaInitVarVersion)
	if !ok {
		name = "=" + luaInitVar
		init, ok = os.LookupEnv(luaInitVar) /* try alternative name */
	}
	if !ok {
		return lua.LUA_OK
	} else if len(init) > 0 && init[0] == '@' {
		return dofile(L, init[1:])
	}
	return dostring(L, init, name)
}

/*
** {==================================================================
** Read-Eval-Print Loop (REPL)
** ===================================================================
 */

// stdinIsTTY is lua_stdin_is_tty (POSIX isatty(0) / Windows _isatty).
// ISO C default in lua.c is 1; this port follows the POSIX sense.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

/*
** lua_readline / lua_saveline / lua_freeline / lua_initreadline
** (non-LUA_USE_READLINE path). FLAG: no libreadline; bufio.Scanner
** instead of fgets(LUA_MAXINPUT). Line length is not capped at 512;
** ScanLines also strips a trailing \r (C's fgets+strip keeps \r on
** \r\n), which is an intentional platform divergence.
 */
const luaMaxInput = 512 // LUA_MAXINPUT — not enforced (Scanner FLAG)

var replIn *bufio.Scanner

func initReadline(L *lua.State) {
	_ = L           /* lua_initreadline */
	_ = luaMaxInput /* not enforced — Scanner FLAG (C uses fgets(LUA_MAXINPUT)) */
	replIn = bufio.NewScanner(os.Stdin)
	replIn.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
}

func saveLine(L *lua.State, line string) { _, _ = L, line /* lua_saveline no-op */ }

func freeLine(L *lua.State, b string) { _, _ = L, b /* lua_freeline no-op */ }

// getPrompt is get_prompt: leave the string (or nil for default) on the
// stack to keep it anchored.
func getPrompt(L *lua.State, firstline bool) string {
	name, def := "_PROMPT2", luaPrompt2
	if firstline {
		name, def = "_PROMPT", luaPrompt
	}
	if L.GetGlobal(name) == lua.LUA_TNIL {
		return def /* use the default */
	}
	/* apply 'tostring' over the value (luaL_tolstring) */
	p := L.ToLString(-1)
	L.Remove(-2) /* remove original value */
	return p
}

// incomplete is incomplete: syntax error whose message ends with <eof>.
func incomplete(L *lua.State, status int) bool {
	if status == lua.LUA_ERRSYNTAX {
		msg, ok := L.ToString(-1)
		if ok && len(msg) >= len(eofMark) && strings.HasSuffix(msg, eofMark) {
			return true
		}
	}
	return false
}

// pushLine is pushline: show prompt, read a line, push it.
func pushLine(L *lua.State, firstline bool) bool {
	prmt := getPrompt(L, firstline)
	fmt.Print(prmt)
	readstatus := replIn.Scan()
	L.Pop(1) /* remove prompt */
	if !readstatus {
		return false /* no input */
	}
	b := replIn.Text()                          /* Scanner already strips the newline */
	if firstline && len(b) > 0 && b[0] == '=' { /* for compatibility with 5.2, ... */
		L.PushFString("return %s", b[1:]) /* change '=' to 'return' */
	} else {
		L.PushString(b)
	}
	freeLine(L, b)
	return true
}

// addReturn is addreturn: try to compile as 'return <line>;'.
func addReturn(L *lua.State) int {
	line, _ := L.ToString(-1) /* original line */
	retline := L.PushFString("return %s;", line)
	status := loadErrStatus(L, L.LoadBytes([]byte(retline), "=stdin"))
	if status == lua.LUA_OK {
		L.Remove(-2)    /* remove modified line */
		if line != "" { /* non empty? */
			saveLine(L, line) /* keep history */
		}
	} else {
		L.Pop(2) /* pop result from loadbuffer and modified line */
	}
	return status
}

// multiline is multiline: read continuation lines until a complete statement.
func multiline(L *lua.State) int {
	for { /* repeat until gets a complete statement */
		line, _ := L.ToString(1)                                        /* get what it has */
		status := loadErrStatus(L, L.LoadBytes([]byte(line), "=stdin")) /* try it */
		if !incomplete(L, status) || !pushLine(L, false) {
			saveLine(L, line) /* keep history */
			return status     /* should not or cannot try to add continuation line */
		}
		L.Remove(-2)       /* remove error message (from incomplete line) */
		L.PushString("\n") /* add newline... */
		L.Insert(-2)       /* ...between the two lines */
		L.Concat(3)        /* join them */
	}
}

// loadLine is loadline.
func loadLine(L *lua.State) int {
	L.SetTop(0)
	if !pushLine(L, true) {
		return -1 /* no input */
	}
	status := addReturn(L)
	if status != lua.LUA_OK { /* 'return ...' did not work? */
		status = multiline(L) /* try as command, maybe with continuation lines */
	}
	L.Remove(1) /* remove line from the stack */
	/* lua_assert(lua_gettop(L) == 1) */
	return status
}

// lPrint is l_print.
func lPrint(L *lua.State) {
	n := L.GetTop()
	if n > 0 { /* any result to be printed? */
		L.CheckStack2(lua.LUA_MINSTACK, "too many results to print")
		L.GetGlobal("print")
		L.Insert(1)
		if L.PCall(n, 0, 0) != lua.LUA_OK {
			msg, _ := L.ToString(-1)
			lMessage(progName, L.PushFString("error calling 'print' (%s)", msg))
		}
	}
}

// doREPL is doREPL.
func doREPL(L *lua.State) {
	oldProgName := progName
	progName = "" /* no 'progname' on errors in interactive mode */
	/* C assigns the saved name back only after the loop. An error that
	   escapes (prompt __tostring, for one) longjmps past that assignment,
	   so report prints no program name. Restoring in every defer would put
	   "lua: " in front of that message. */
	defer func() {
		if e := recover(); e != nil {
			panic(e)
		}
		progName = oldProgName
	}()
	initReadline(L)
	for {
		status := loadLine(L)
		if status == -1 {
			break
		}
		if status == lua.LUA_OK {
			status = docall(L, 0, lua.LUA_MULTRET)
		}
		if status == lua.LUA_OK {
			lPrint(L)
		} else {
			report(L, status)
		}
	}
	L.SetTop(0)   /* clear stack */
	fmt.Println() /* lua_writeline */
	progName = oldProgName
}

/* }================================================================== */

// pmain is pmain: main body of the stand-alone interpreter (protected mode).
func pmain(L *lua.State) int {
	argc64, _ := L.ToInteger(1)
	argc := int(argc64)
	_ = L.ToUserData(2) /* argv lightuserdata stand-in; real argv is pmainArgv */
	argv := pmainArgv
	if argc > len(argv) {
		argc = len(argv)
	}
	args, script := collectArgs(argv)
	optlim := argc /* first argv not an option */
	if script > 0 {
		optlim = script
	}
	L.CheckVersion()      /* check that interpreter has correct version */
	if args == hasError { /* bad arg? */
		printUsage(argv[script]) /* 'script' has index of bad arg */
		return 0
	}
	if args&hasV != 0 { /* option '-v'? */
		printVersion()
	}
	if args&hasEnv != 0 { /* option '-E'? */
		L.PushBoolean(true) /* signal for libraries to ignore env. vars. */
		L.SetField(lua.LUA_REGISTRYINDEX, "LUA_NOENV")
	}
	L.OpenLibs()                          /* open standard libraries */
	createArgTable(L, argv, argc, script) /* create table 'arg' */
	/* lua_gc(L, LUA_GCRESTART); lua_gc(L, LUA_GCGEN, 0, 0);
	   FLAG: Go GC — no-op (intentional). */
	if args&hasEnv == 0 { /* no option '-E'? */
		if handleLuaInit(L) != lua.LUA_OK { /* run LUA_INIT */
			return 0 /* error running LUA_INIT */
		}
	}
	if runArgs(L, argv, optlim) == 0 { /* execute arguments -e and -l */
		return 0 /* something failed */
	}
	if script > 0 { /* execute main script (if there is one) */
		if handleScript(L, argv, script) != lua.LUA_OK {
			return 0 /* interrupt in case of error */
		}
	}
	if args&hasI != 0 { /* -i option? */
		doREPL(L) /* do read-eval-print loop */
	} else if script < 1 && args&(hasE|hasV) == 0 { /* no active option? */
		if stdinIsTTY() { /* running in interactive mode? */
			printVersion()
			doREPL(L) /* do read-eval-print loop */
		} else {
			dofile(L, "") /* executes stdin as a file; status ignored (lua.c) */
		}
	}
	L.PushBoolean(true) /* signal no errors */
	return 1
}
