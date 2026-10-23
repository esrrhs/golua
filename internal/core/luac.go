// Translation of luac.c (PUC-Lua 5.4.9): the standalone Lua compiler.
//
// It lives in the library rather than in cmd/luac for one reason: the
// listing (-l) reads prototypes, and a prototype is not reachable
// through the C API. luac.c says so itself -- it includes lobject.h,
// lopcodes.h and lstate.h and walks f->code, f->k, f->p, f->locvars
// and f->upvalues directly. cmd/luac is a wrapper over Luac.
//
// Two things follow the C source even where it is odd:
//
//   - '-l' or '-p' with no input files makes luac compile "luac.out"
//     (doargs rewrites the last argv entry, so that is the file it
//     reads). It is what the reference compiler does.
//   - several files are combined by compiling
//     "(function()end)();\n" once per file and then replacing each
//     nested prototype with one of the files', which is how luac packs
//     them into a single main chunk.
package core

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// LUA_COPYRIGHT is what '-v' prints (lua.h: LUA_COPYRIGHT). lua.c and
// luac.c print the same line, and scripts grep it for "PUC-Rio".
const LUA_COPYRIGHT = "Lua 5.3.4  Copyright (C) 1994-2017 Lua.org, PUC-Rio"

const (
	luacProgName = "luac"
	luacOutput   = luacProgName + ".out"
	// luacFunction is FUNCTION in luac.c: the skeleton whose nested
	// prototypes are replaced by the ones being combined.
	luacFunction = "(function()end)();\n"
)

// luac is the compiler: the options doargs collected, plus where the
// listing and the diagnostics go.
type luac struct {
	progname  string
	out, err  io.Writer
	listing   int
	dumping   int
	stripping int
	output    string // "" means standard output ('-o -')
}

// Luac runs the Lua compiler and returns the process exit status. The
// listing goes to 'out', diagnostics and the usage message to 'err'.
//
// 'argv' is the process argv, argv[0] included: luac.c names the program
// after it and skips it when looking for options.
func Luac(argv []string, out, err io.Writer) int {
	/* doargs writes argv[argc-1] (see the note in the file comment), so
	   the caller's slice must not be the one it sees. */
	args := make([]string, len(argv))
	copy(args, argv)
	c := &luac{progname: luacProgName, out: out, err: err, dumping: 1, output: luacOutput}
	i, code := c.doargs(args)
	if code >= 0 {
		return code
	}
	files := args[i:]
	if len(files) <= 0 {
		c.usage("no input files given")
		return 1
	}
	return c.pmain(files)
}

// usage is luac.c's 'usage': the message, then the options, then
// EXIT_FAILURE. A message that starts with '-' is an option luac did not
// recognise.
func (c *luac) usage(message string) {
	if strings.HasPrefix(message, "-") {
		fmt.Fprintf(c.err, "%s: unrecognized option '%s'\n", c.progname, message)
	} else {
		fmt.Fprintf(c.err, "%s: %s\n", c.progname, message)
	}
	fmt.Fprintf(c.err,
		"usage: %s [options] [filenames]\n"+
			"Available options are:\n"+
			"  -l       list (use -l -l for full listing)\n"+
			"  -o name  output to file 'name' (default is %q)\n"+
			"  -p       parse only\n"+
			"  -s       strip debug information\n"+
			"  -v       show version information\n"+
			"  --       stop handling options\n"+
			"  -        stop handling options and process stdin\n",
		c.progname, luacOutput)
}

// fatal is luac.c's 'fatal'.
func (c *luac) fatal(message string) {
	fmt.Fprintf(c.err, "%s: %s\n", c.progname, message)
}

// cannot is luac.c's 'cannot': "cannot <what> <output>: <strerror>".
func (c *luac) cannot(what string) {
	name := c.output
	if name == "" {
		name = "(null)" /* C prints NULL with '%s' */
	}
	fmt.Fprintf(c.err, "%s: cannot %s %s: %s\n", c.progname, what, name, "I/O error")
}

// doargs is luac.c's 'doargs'. It returns the index of the first
// non-option argument, or a status to exit with (0 or 1) when there is
// nothing left to do or the options are wrong.
func (c *luac) doargs(argv []string) (int, int) {
	if len(argv) > 0 && argv[0] != "" {
		c.progname = argv[0]
	}
	i := 1
	version := 0
	for ; i < len(argv); i++ {
		a := argv[i]
		if len(a) == 0 || a[0] != '-' {
			break /* end of options; keep it */
		}
		switch a {
		case "--": /* end of options; skip it */
			i++
			if version > 0 {
				version++
			}
			goto done
		case "-": /* end of options; use stdin */
			goto done
		case "-l": /* list */
			c.listing++
		case "-o": /* output file */
			i++
			if i >= len(argv) || argv[i] == "" || (argv[i][0] == '-' && len(argv[i]) > 1) {
				c.usage("'-o' needs argument")
				return 0, 1
			}
			c.output = argv[i]
			if argv[i] == "-" {
				c.output = "" /* write to standard output */
			}
		case "-p": /* parse only */
			c.dumping = 0
		case "-s": /* strip debug information */
			c.stripping = 1
		case "-v": /* show version */
			version++
		default: /* unknown option */
			c.usage(a)
			return 0, 1
		}
	}
done:
	if i == len(argv) && (c.listing > 0 || c.dumping == 0) {
		/* no input files: the output file becomes the input */
		c.dumping = 0
		i--
		argv[i] = luacOutput
	}
	if version > 0 {
		fmt.Fprintf(c.out, "%s\n", LUA_COPYRIGHT)
		if version == len(argv)-1 {
			return 0, 0 /* '-v' was the only argument */
		}
	}
	return i, -1
}

// pmain is luac.c's 'pmain': load every file, combine them, list, dump.
func (c *luac) pmain(files []string) int {
	/* luac.c uses luaL_newstate, which opens no libraries: the parser
	   needs nothing from them. */
	L := NewState()
	defer L.Close()
	if !L.CheckStack(len(files)) {
		c.fatal("too many input files")
		return 1
	}
	for _, name := range files {
		var err error
		if name == "-" {
			err = c.loadStdin(L)
		} else {
			err = L.LoadFile(name)
		}
		if err != nil {
			c.fatal(err.Error())
			return 1
		}
	}
	f := c.combine(L, len(files))
	if f == nil {
		return 1
	}
	if c.listing > 0 {
		c.printFunction(f, c.listing > 1)
	}
	if c.dumping != 0 {
		if !c.dump(L, f) {
			return 1
		}
	}
	return 0
}

// loadStdin is luaL_loadfile(L, NULL): the chunk is standard input and
// is named "=stdin".
func (c *luac) loadStdin(L *State) error {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	return L.LoadBytes(skipFileComment(data), "=stdin")
}

// combine is luac.c's 'combine': one file is used as it is, several are
// grafted onto the skeleton's nested prototypes.
func (c *luac) combine(L *State, n int) *Proto {
	if n == 1 {
		return clLvalue(&L.stack[L.top-1]).p
	}
	src := strings.Repeat(luacFunction, n)
	if err := L.LoadBytes([]byte(src), "=("+luacProgName+")"); err != nil {
		c.fatal(err.Error())
		return nil
	}
	f := clLvalue(&L.stack[L.top-1]).p
	for i := 0; i < n; i++ {
		f.p[i] = clLvalue(&L.stack[L.top+i-n-1]).p
		if len(f.p[i].upvalues) > 0 {
			/* the chunk's _ENV was a register of its own frame; as a
			   nested prototype it is an upvalue of the enclosing one */
			f.p[i].upvalues[0].instack = 0
		}
	}
	return f
}

// dump writes the precompiled chunk (luac.c's 'pmain' when dumping).
func (c *luac) dump(L *State, f *Proto) bool {
	w := c.out
	var file *os.File
	if c.output != "" {
		var err error
		file, err = os.Create(c.output) /* C: fopen(output,"wb") */
		if err != nil {
			c.cannot("open")
			return false
		}
		w = file
	}
	werr := false
	luaU_dump(L, f, func(L *State, b []byte) int {
		if werr {
			return 1
		}
		if _, err := w.Write(b); err != nil {
			werr = true
			return 1
		}
		return 0
	}, c.stripping != 0)
	if file != nil {
		if err := file.Close(); err != nil { /* C checks ferror and fclose */
			c.cannot("close")
			return false
		}
	}
	if werr {
		c.cannot("write")
		return false
	}
	return true
}

// ---------------------------------------------------------------------
// print bytecodes
// ---------------------------------------------------------------------

// upvalName is UPVALNAME(x).
func (c *luac) upvalName(f *Proto, x int) string {
	if f.upvalues[x].name != nil {
		return f.upvalues[x].name.val
	}
	return "-"
}

// printString is PrintString: a string the way a Lua literal would
// spell it.
func (c *luac) printString(ts *TString) {
	s := ts.val
	fmt.Fprint(c.out, "\"")
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '"':
			fmt.Fprint(c.out, "\\\"")
		case '\\':
			fmt.Fprint(c.out, "\\\\")
		case '\a':
			fmt.Fprint(c.out, "\\a")
		case '\b':
			fmt.Fprint(c.out, "\\b")
		case '\f':
			fmt.Fprint(c.out, "\\f")
		case '\n':
			fmt.Fprint(c.out, "\\n")
		case '\r':
			fmt.Fprint(c.out, "\\r")
		case '\t':
			fmt.Fprint(c.out, "\\t")
		case '\v':
			fmt.Fprint(c.out, "\\v")
		default:
			if ch >= 0x20 && ch < 0x7f { /* isprint, in the C locale */
				fmt.Fprintf(c.out, "%c", ch)
			} else {
				fmt.Fprintf(c.out, "\\%03d", ch)
			}
		}
	}
	fmt.Fprint(c.out, "\"")
}

// printType is PrintType: the one-letter type of a constant.
func (c *luac) printType(f *Proto, i int) {
	o := &f.k[i]
	switch ttypetag(o) {
	case LUA_VNIL:
		fmt.Fprint(c.out, "N")
	case LUA_VFALSE, LUA_VTRUE:
		fmt.Fprint(c.out, "B")
	case LUA_VNUMFLT:
		fmt.Fprint(c.out, "F")
	case LUA_VNUMINT:
		fmt.Fprint(c.out, "I")
	case LUA_VSHRSTR, LUA_VLNGSTR:
		fmt.Fprint(c.out, "S")
	default: /* cannot happen */
		fmt.Fprintf(c.out, "?%d", ttypetag(o))
	}
	fmt.Fprint(c.out, "\t")
}

// printConstant is PrintConstant.
func (c *luac) printConstant(f *Proto, i int) {
	o := &f.k[i]
	switch ttypetag(o) {
	case LUA_VNIL:
		fmt.Fprint(c.out, "nil")
	case LUA_VFALSE:
		fmt.Fprint(c.out, "false")
	case LUA_VTRUE:
		fmt.Fprint(c.out, "true")
	case LUA_VNUMFLT:
		s := fmtFloat(fltvalue(o)) /* LUA_NUMBER_FMT: "%.14g" */
		fmt.Fprint(c.out, s)
		if strspn(s, "-0123456789") == len(s) { /* looks like an integer? */
			fmt.Fprint(c.out, ".0")
		}
	case LUA_VNUMINT:
		fmt.Fprintf(c.out, "%d", ivalue(o))
	case LUA_VSHRSTR, LUA_VLNGSTR:
		c.printString(tsvalue(o))
	default: /* cannot happen */
		fmt.Fprintf(c.out, "?%d", ttypetag(o))
	}
}

// strspn is C's strspn: the length of the prefix of 's' made of
// characters from 'set'.
func strspn(s, set string) int {
	n := 0
	for n < len(s) && strings.IndexByte(set, s[n]) >= 0 {
		n++
	}
	return n
}

// ss is SS(x): the plural 's'.
func ss(x int) string {
	if x == 1 {
		return ""
	}
	return "s"
}

// printHeader is PrintHeader.
func (c *luac) printHeader(f *Proto) {
	s := "=?"
	if f.source != nil {
		s = f.source.val
	}
	if len(s) > 0 && (s[0] == '@' || s[0] == '=') {
		s = s[1:]
	} else if len(s) > 0 && s[0] == dmpSignature[0] {
		s = "(bstring)"
	} else {
		s = "(string)"
	}
	what := "function"
	if f.linedefined == 0 {
		what = "main"
	}
	fmt.Fprintf(c.out, "\n%s <%s:%d,%d> (%d instruction%s at %p)\n",
		what, s, f.linedefined, f.lastlinedefined, len(f.code), ss(len(f.code)), f)
	fmt.Fprintf(c.out, "%d%s param%s, %d slot%s, %d upvalue%s, ",
		int(f.numparams), varargMark(f), ss(int(f.numparams)),
		int(f.maxstacksize), ss(int(f.maxstacksize)),
		len(f.upvalues), ss(len(f.upvalues)))
	fmt.Fprintf(c.out, "%d local%s, %d constant%s, %d function%s\n",
		len(f.locvars), ss(len(f.locvars)),
		len(f.k), ss(len(f.k)),
		len(f.p), ss(len(f.p)))
}

// myk is MYK from luac.c: constant indices print as -1-k.
func myk(x int) int { return -1 - x }

// printCode is PrintCode (luac.c, Lua 5.3.4).
func (c *luac) printCode(f *Proto) {
	code := f.code
	for pc := 0; pc < len(code); pc++ {
		i := code[pc]
		op := i.Op
		a := i.A
		b := i.B
		carg := i.C
		ax := i.A /* Ax stored in A for iAx */
		bx := i.B
		sbx := sBx(i.B)
		line := luaG_getfuncline(f, pc)
		fmt.Fprintf(c.out, "\t%d\t", pc+1)
		if line > 0 {
			fmt.Fprintf(c.out, "[%d]\t", line)
		} else {
			fmt.Fprintf(c.out, "[-]\t")
		}
		fmt.Fprintf(c.out, "%-9s\t", opName(op))
		if op < 0 || op >= NUM_OPCODES {
			fmt.Fprint(c.out, "\n")
			continue
		}
		switch getOpMode(op) {
		case iABC:
			fmt.Fprintf(c.out, "%d", a)
			if getBMode(op) != OpArgN {
				if ISK(b) {
					fmt.Fprintf(c.out, " %d", myk(INDEXK(b)))
				} else {
					fmt.Fprintf(c.out, " %d", b)
				}
			}
			if getCMode(op) != OpArgN {
				if ISK(carg) {
					fmt.Fprintf(c.out, " %d", myk(INDEXK(carg)))
				} else {
					fmt.Fprintf(c.out, " %d", carg)
				}
			}
		case iABx:
			fmt.Fprintf(c.out, "%d", a)
			if getBMode(op) == OpArgK {
				fmt.Fprintf(c.out, " %d", myk(bx))
			}
			if getBMode(op) == OpArgU {
				fmt.Fprintf(c.out, " %d", bx)
			}
		case iAsBx:
			fmt.Fprintf(c.out, "%d %d", a, sbx)
		case iAx:
			fmt.Fprintf(c.out, "%d", myk(ax))
		}
		switch op {
		case OP_LOADK:
			fmt.Fprint(c.out, "\t; ")
			c.printConstant(f, bx)
		case OP_GETUPVAL, OP_SETUPVAL:
			fmt.Fprintf(c.out, "\t; %s", c.upvalName(f, b))
		case OP_GETTABUP:
			fmt.Fprintf(c.out, "\t; %s", c.upvalName(f, b))
			if ISK(carg) {
				fmt.Fprint(c.out, " ")
				c.printConstant(f, INDEXK(carg))
			}
		case OP_SETTABUP:
			fmt.Fprintf(c.out, "\t; %s", c.upvalName(f, a))
			if ISK(b) {
				fmt.Fprint(c.out, " ")
				c.printConstant(f, INDEXK(b))
			}
			if ISK(carg) {
				fmt.Fprint(c.out, " ")
				c.printConstant(f, INDEXK(carg))
			}
		case OP_GETTABLE, OP_SELF:
			if ISK(carg) {
				fmt.Fprint(c.out, "\t; ")
				c.printConstant(f, INDEXK(carg))
			}
		case OP_SETTABLE, OP_ADD, OP_SUB, OP_MUL, OP_POW, OP_DIV, OP_IDIV,
			OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR, OP_EQ, OP_LT, OP_LE:
			if ISK(b) || ISK(carg) {
				fmt.Fprint(c.out, "\t; ")
				if ISK(b) {
					c.printConstant(f, INDEXK(b))
				} else {
					fmt.Fprint(c.out, "-")
				}
				fmt.Fprint(c.out, " ")
				if ISK(carg) {
					c.printConstant(f, INDEXK(carg))
				} else {
					fmt.Fprint(c.out, "-")
				}
			}
		case OP_JMP, OP_FORLOOP, OP_FORPREP, OP_TFORLOOP:
			fmt.Fprintf(c.out, "\t; to %d", sbx+pc+2)
		case OP_CLOSURE:
			fmt.Fprintf(c.out, "\t; %p", f.p[bx])
		case OP_SETLIST:
			if carg == 0 {
				pc++
				fmt.Fprintf(c.out, "\t; %d", code[pc].A)
			} else {
				fmt.Fprintf(c.out, "\t; %d", carg)
			}
		case OP_EXTRAARG:
			fmt.Fprint(c.out, "\t; ")
			c.printConstant(f, ax)
		}
		fmt.Fprint(c.out, "\n")
	}
}

// printDebug is PrintDebug.
func (c *luac) printDebug(f *Proto) {
	fmt.Fprintf(c.out, "constants (%d) for %p:\n", len(f.k), f)
	for i := range f.k {
		fmt.Fprintf(c.out, "\t%d\t", i)
		c.printType(f, i)
		c.printConstant(f, i)
		fmt.Fprint(c.out, "\n")
	}
	fmt.Fprintf(c.out, "locals (%d) for %p:\n", len(f.locvars), f)
	for i := range f.locvars {
		fmt.Fprintf(c.out, "\t%d\t%s\t%d\t%d\n", i,
			f.locvars[i].varname.val, f.locvars[i].startpc+1, f.locvars[i].endpc+1)
	}
	fmt.Fprintf(c.out, "upvalues (%d) for %p:\n", len(f.upvalues), f)
	for i := range f.upvalues {
		fmt.Fprintf(c.out, "\t%d\t%s\t%d\t%d\n", i,
			c.upvalName(f, i), f.upvalues[i].instack, f.upvalues[i].idx)
	}
}

// printFunction is PrintFunction, which luac.c calls through its
// 'luaU_print'.
func (c *luac) printFunction(f *Proto, full bool) {
	c.printHeader(f)
	c.printCode(f)
	if full {
		c.printDebug(f)
	}
	for i := range f.p {
		c.printFunction(f.p[i], full)
	}
}

// varargMark is the '+' luac.c writes after the parameter count of a
// vararg function.
func varargMark(f *Proto) string {
	if f.isVararg != 0 {
		return "+"
	}
	return ""
}
