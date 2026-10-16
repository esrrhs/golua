// Translation of lbaselib.c (PUC-Lua 5.4.9): the basic library.
//
// Every luaB_* function keeps its C name prefixed with 'base', and the
// control flow follows the C source.  Three things are necessarily
// different from C: there is no GC (so 'collectgarbage' is a stub), the
// error mechanism is panic/recover instead of setjmp/longjmp (which is
// already handled by luaD_pcall), and every C function is a GoFunc.
package lua

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// LUA_GNAME and LUA_VERSION are not defined in this port; they are
// local to the base library.
const (
	baseGName   = "_G"
	baseVersion = "Lua 5.4"
)

// ---------------------------------------------------------------------
// luaL_tolstring (lauxlib.c)
// ---------------------------------------------------------------------

// luaL_tolstring converts the value at 'idx' to a string using the
// '__tostring' metamethod (or '__name'), pushes the resulting string on
// the stack and returns it (lauxlib.c luaL_tolstring).
//
// The implementation lives in lstrlib.go (strToLString), which already
// follows the C code step by step.
func luaL_tolstring(L *State, idx int) string {
	return strToLString(L, idx)
}

// ---------------------------------------------------------------------
// print / warn
// ---------------------------------------------------------------------

func basePrint(L *State) int {
	n := L.GetTop()           /* number of arguments */
	for i := 1; i <= n; i++ { /* for each argument */
		s := luaL_tolstring(L, i) /* convert it to string */
		if i > 1 {                /* not the first element? */
			os.Stdout.WriteString("\t") /* add a tab before it */
		}
		os.Stdout.WriteString(s) /* print it */
		L.Pop(1)                 /* pop result */
	}
	os.Stdout.WriteString("\n")
	return 0
}

/*
** Creates a warning with all given arguments.
** Check first for errors; otherwise an error may interrupt
** the composition of a warning, leaving it unfinished.
 */
func baseWarn(L *State) int {
	n := L.GetTop()  /* number of arguments */
	L.CheckString(1) /* at least one argument */
	for i := 2; i <= n; i++ {
		L.CheckString(i) /* make sure all arguments are strings */
	}
	for i := 1; i < n; i++ { /* compose warning */
		s, _ := L.ToString(i)
		luaE_warning(L, s, true)
	}
	s, _ := L.ToString(n)
	luaE_warning(L, s, false) /* close warning */
	return 0
}

// ---------------------------------------------------------------------
// tonumber
// ---------------------------------------------------------------------

const baseSpaceChars = " \f\n\r\t\v"

func baseIsAlnum(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func baseASCIIUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

// baseStr2Int converts 's' in the given base; it returns the value, the
// index where the conversion stopped, and whether it succeeded
// (b_str2int).  The conversion is only valid when it consumes the
// whole string.
func baseStr2Int(s string, base int) (int64, int, bool) {
	i := 0
	for i < len(s) && strings.IndexByte(baseSpaceChars, s[i]) >= 0 {
		i++ /* skip initial spaces */
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		i++
		neg = true
	} else if i < len(s) && s[i] == '+' {
		i++
	}
	if i >= len(s) || !baseIsAlnum(s[i]) { /* no digit? */
		return 0, 0, false
	}
	var n uint64
	for {
		var digit int
		if s[i] >= '0' && s[i] <= '9' {
			digit = int(s[i] - '0')
		} else {
			digit = int(baseASCIIUpper(s[i])-'A') + 10
		}
		if digit >= base {
			return 0, 0, false /* invalid numeral */
		}
		n = n*uint64(base) + uint64(digit)
		i++
		if i >= len(s) || !baseIsAlnum(s[i]) {
			break
		}
	}
	for i < len(s) && strings.IndexByte(baseSpaceChars, s[i]) >= 0 {
		i++ /* skip trailing spaces */
	}
	if neg {
		return int64(uint64(0) - n), i, true
	}
	return int64(n), i, true
}

func baseToNumber(L *State) int {
	if L.IsNoneOrNil(2) { /* standard conversion? */
		if L.Type(1) == LUA_TNUMBER { /* already a number? */
			L.SetTop(1) /* yes; return it */
			return 1
		}
		if s, ok := L.ToString(1); ok && strStringToNumber(L, s) {
			return 1 /* successful conversion to number */
		}
		L.CheckAny(1) /* (but there must be some parameter) */
	} else {
		base := L.CheckInteger(2)
		L.CheckType(1, LUA_TSTRING) /* no numbers as strings */
		s, _ := L.ToString(1)
		L.ArgCheck(2 <= base && base <= 36, 2, "base out of range")
		if n, end, ok := baseStr2Int(s, int(base)); ok && end == len(s) {
			L.PushInteger(n)
			return 1
		} /* else not a number */
	} /* else not a number */
	L.PushNil() /* not a number */
	return 1
}

// ---------------------------------------------------------------------
// error
// ---------------------------------------------------------------------

func baseError(L *State) int {
	level := int(L.OptInteger(2, 1))
	L.SetTop(1)
	if L.Type(1) == LUA_TSTRING && level > 0 {
		L.Where2(level) /* add extra information */
		L.PushValue(1)
		L.Concat(2)
	}
	return L.Error()
}

// ---------------------------------------------------------------------
// metatables
// ---------------------------------------------------------------------

func baseGetMetatable(L *State) int {
	L.CheckAny(1)
	if !L.GetMetaTable(1) {
		L.PushNil()
		return 1 /* no metatable */
	}
	L.GetMetaField(1, "__metatable")
	return 1 /* returns either __metatable field (if present) or metatable */
}

func baseSetMetatable(L *State) int {
	t := L.Type(2)
	L.CheckType(1, LUA_TTABLE)
	L.ArgCheck(t == LUA_TNIL || t == LUA_TTABLE, 2, "nil or table")
	if L.GetMetaField(1, "__metatable") != LUA_TNIL {
		return L.Error2("cannot change a protected metatable")
	}
	L.SetTop(2)
	L.SetMetaTable(1)
	return 1
}

// ---------------------------------------------------------------------
// raw functions
// ---------------------------------------------------------------------

func baseRawEqual(L *State) int {
	L.CheckAny(1)
	L.CheckAny(2)
	L.PushBoolean(L.RawEqual(1, 2))
	return 1
}

func baseRawLen(L *State) int {
	t := L.Type(1)
	L.ArgCheck(t == LUA_TTABLE || t == LUA_TSTRING, 1, "table or string")
	L.PushInteger(int64(L.RawLen(1)))
	return 1
}

func baseRawGet(L *State) int {
	L.CheckType(1, LUA_TTABLE)
	L.CheckAny(2)
	L.SetTop(2)
	L.RawGet(1)
	return 1
}

func baseRawSet(L *State) int {
	L.CheckType(1, LUA_TTABLE)
	L.CheckAny(2)
	L.CheckAny(3)
	L.SetTop(3)
	L.RawSet(1)
	return 1
}

// ---------------------------------------------------------------------
// collectgarbage
// ---------------------------------------------------------------------

// There is no GC in this port (the Go collector reclaims everything),
// so 'collectgarbage' only accepts the options and answers with
// placeholder values; it never raises an error.
var baseGCOpts = []string{"stop", "restart", "collect", "count", "step",
	"setpause", "setstepmul", "isrunning", "generational", "incremental"}

// indices into baseGCOpts
const (
	baseGCStop = iota
	baseGCRestart
	baseGCCollect
	baseGCCount
	baseGCStep
	baseGCSetPause
	baseGCSetStepMul
	baseGCIsRunning
	baseGCGenerational
	baseGCIncremental
)

// basePushMode pushes the name of a collector mode (pushmode).
func basePushMode(L *State, mode string) int {
	L.PushString(mode)
	return 1
}

// gcModeInc / gcModeGen are the two collector modes.  PUC-Lua remembers
// which one is in force and 'collectgarbage("generational")' /
// 'collectgarbage("incremental")' answer with the one it replaced, which
// is what lets a script put the mode back the way it found it.
const (
	gcModeInc = "incremental"
	gcModeGen = "generational"
)

// baseSetMode is 'lua_gc(L, LUA_GCGEN | LUA_GCINC)': it answers with the
// previous mode and switches to the new one.
func baseSetMode(L *State, mode string) int {
	previous := L.g.gcMode
	L.g.gcMode = mode
	return basePushMode(L, previous)
}

// gcCount is 'lua_gc(L, LUA_GCCOUNT)' expressed the way lbaselib.c does
// it: Kbytes with a fractional part, i.e. bytes/1024 as a float.
func gcCount() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc>>10) + float64(ms.HeapAlloc&1023)/1024
}

func baseCollectGarbage(L *State) int {
	switch L.CheckOption(1, "collect", baseGCOpts) {
	case baseGCCount:
		/* PUC-Lua answers with the number of Kbytes its own allocator
		   has handed out; here the equivalent figure is what the Go
		   heap is holding.  It is not the same number (there is no
		   per-state accounting), but scripts that print or watch
		   'collectgarbage("count")' still see a real number that
		   grows and shrinks with the data. */
		L.PushNumber(gcCount())
		return 1
	case baseGCStep:
		L.OptInteger(2, 0)
		/* PUC-Lua's step does a bounded amount of work and says whether
		   it finished a cycle; with a step size of 0 it finishes one,
		   which is what every script that calls
		   'collectgarbage("step")' and then looks at a weak table is
		   asking for.  Go's collector is all or nothing, so the step
		   is a whole cycle. */
		L.gcCycle()
		L.PushBoolean(true)
		return 1
	case baseGCSetPause:
		p := int(L.OptInteger(2, 0))
		L.PushInteger(int64(L.g.gcPause))
		if p != 0 {
			L.g.gcPause = p
		}
		return 1
	case baseGCSetStepMul:
		m := int(L.OptInteger(2, 0))
		L.PushInteger(int64(L.g.gcStepMul))
		if m != 0 {
			L.g.gcStepMul = m
		}
		return 1
	case baseGCIsRunning:
		/* C answers with 'gcstp == 0': stopped by the program counts,
		   Go's collector being unable to stop does not. */
		L.PushBoolean(!L.g.gcStopped)
		return 1
	case baseGCGenerational:
		L.OptInteger(2, 0)
		L.OptInteger(3, 0)
		return baseSetMode(L, gcModeGen)
	case baseGCIncremental:
		L.OptInteger(2, 0)
		L.OptInteger(3, 0)
		L.OptInteger(4, 0)
		return baseSetMode(L, gcModeInc)
	case baseGCCollect:
		L.fullCollect()
		L.PushInteger(0)
		return 1
	case baseGCStop:
		L.g.gcStopped = true
		L.PushInteger(0)
		return 1
	case baseGCRestart:
		L.g.gcStopped = false
		L.PushInteger(0)
		return 1
	}
	/* not reached: CheckOption accepts nothing else */
	L.PushInteger(0)
	return 1
}

// ---------------------------------------------------------------------
// type / next / pairs / ipairs
// ---------------------------------------------------------------------

func baseType(L *State) int {
	t := L.Type(1)
	L.ArgCheck(t != LUA_TNONE, 1, "value expected")
	L.PushString(TypeName(t))
	return 1
}

func baseNext(L *State) int {
	L.CheckType(1, LUA_TTABLE)
	L.SetTop(2) /* create a 2nd argument if there isn't one */
	if L.Next(1) {
		return 2
	}
	L.PushNil()
	return 1
}

func basePairs(L *State) int {
	L.CheckAny(1)
	if L.GetMetaField(1, "__pairs") == LUA_TNIL { /* no metamethod? */
		L.PushGoFunc(baseNext) /* will return generator, */
		L.PushValue(1)         /* state, */
		L.PushNil()            /* and initial value */
	} else {
		L.PushValue(1) /* argument 'self' to metamethod */
		/* C passes 'pairscont' as the continuation of lua_callk, which
		   makes the call yieldable: '__pairs' may yield. */
		L.callK(1, 3, true) /* get 3 values from metamethod */
	}
	return 3
}

// baseIPairsAux is the traversal function for 'ipairs' (ipairsaux).
func baseIPairsAux(L *State) int {
	i := L.CheckInteger(2)
	i++ /* next index */
	L.PushInteger(i)
	if L.GetI(1, i) == LUA_TNIL {
		return 1
	}
	return 2
}

/*
** 'ipairs' function. Returns 'ipairsaux', given "table", 0.
** (The given "table" may not be a table.)
 */
func baseIPairs(L *State) int {
	L.CheckAny(1)
	L.PushGoFunc(baseIPairsAux) /* iteration function */
	L.PushValue(1)              /* state */
	L.PushInteger(0)            /* initial value */
	return 3
}

// ---------------------------------------------------------------------
// load / loadfile / dofile
// ---------------------------------------------------------------------

// baseSetupValue sets the value on top of the stack as the first
// upvalue (_ENV) of the closure at 'idx', popping it; it returns false
// when there is no such upvalue (lua_setupvalue).
func baseSetupValue(L *State, idx int) bool {
	v := index2value(L, idx)
	if !ttisLclosure(v) || clLvalue(v).nupvalues < 1 {
		return false
	}
	cl := clLvalue(v)
	setobj(&cl.upvals[0].v, &L.stack[L.top-1])
	L.top--
	return true
}

// baseLoadAux finishes a 'load': on success it returns the chunk (with
// its environment set), on failure it returns fail plus the error
// message (load_aux).
func baseLoadAux(L *State, err error, env int) int {
	if err == nil {
		if env != 0 { /* 'env' parameter? */
			L.PushValue(env)            /* environment for loaded function */
			if !baseSetupValue(L, -2) { /* set it as 1st upvalue */
				L.Pop(1) /* remove 'env' if not used by previous call */
			}
		}
		return 1
	}
	/* error */
	L.PushNil()               /* return fail ... */
	L.PushString(err.Error()) /* ... plus error message */
	return 2
}

// baseReadStdin reads a whole chunk from the standard input.
func baseReadStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

func baseLoadFile(L *State) int {
	fname := L.OptString(1, "") /* NULL (stdin) when absent */
	L.OptString(2, "")          /* mode (not used by this port) */
	env := 0
	if !L.IsNone(3) { /* 'env' index or 0 if no 'env' */
		env = 3
	}
	var err error
	if fname == "" { /* no file name: read the standard input */
		var data []byte
		if data, err = baseReadStdin(); err == nil {
			err = L.LoadBytes(data, "=stdin")
		}
	} else {
		err = L.LoadFile(fname)
	}
	return baseLoadAux(L, err, env)
}

// baseLoadReader collects the pieces returned by the reader function
// (the C code feeds them to lua_load through generic_reader).
//
// In C the reader is called from inside lua_load, which is protected, so
// an error raised by the reader (or the "must return a string" check)
// becomes the status of lua_load and 'load' returns nil plus the message.
// Here the call is protected explicitly to get the same behaviour.
func baseLoadReader(L *State) ([]byte, error) {
	var sb strings.Builder
	for {
		L.PushValue(1) /* get function */
		if L.PCall(0, 1, 0) != LUA_OK {
			msg, _ := L.ToString(-1)
			L.Pop(1)
			return nil, fmt.Errorf("%s", msg)
		}
		if L.IsNil(-1) {
			L.Pop(1) /* pop result */
			break
		}
		if !L.IsString(-1) {
			L.Pop(1)
			return nil, fmt.Errorf("reader function must return a string")
		}
		s, _ := L.ToString(-1)
		L.Pop(1)
		if s == "" { /* empty string ends the chunk */
			break
		}
		sb.WriteString(s)
	}
	return []byte(sb.String()), nil
}

func baseLoad(L *State) int {
	s, ok := L.ToString(1)
	mode := L.OptString(3, "bt")
	env := 0
	if !L.IsNone(4) { /* 'env' index or 0 if no 'env' */
		env = 4
	}
	if ok { /* loading a string? */
		chunkname := L.OptString(2, s)
		if len(s) > 0 && s[0] == dmpSignature[0] {
			/* precompiled chunk: 'lua_load' calls luaU_undump instead
			   of the parser (ldo.c f_parser) */
			return baseLoadAux(L, luaU_loadBinary(L, []byte(s), chunkname, mode), env)
		}
		return baseLoadAux(L, L.LoadBytesMode([]byte(s), chunkname, mode), env)
	}
	/* loading from a reader function */
	chunkname := L.OptString(2, "=(load)")
	L.CheckType(1, LUA_TFUNCTION)
	L.SetTop(5) /* create reserved slot */
	data, err := baseLoadReader(L)
	if err != nil {
		return baseLoadAux(L, err, env)
	}
	if len(data) > 0 && data[0] == dmpSignature[0] {
		return baseLoadAux(L, luaU_loadBinary(L, data, chunkname, mode), env)
	}
	return baseLoadAux(L, L.LoadBytesMode(data, chunkname, mode), env)
}

func baseDoFile(L *State) int {
	fname := L.OptString(1, "")
	L.SetTop(1)
	var err error
	if fname == "" {
		var data []byte
		if data, err = baseReadStdin(); err == nil {
			err = L.LoadBytes(data, "=stdin")
		}
	} else {
		err = L.LoadFile(fname)
	}
	if err != nil {
		L.PushString(err.Error())
		return L.Error()
	}
	/* C passes 'dofilecont' as the continuation of lua_callk, so the
	   loaded chunk may yield when 'dofile' runs inside a coroutine. */
	L.callK(0, LUA_MULTRET, true)
	return L.GetTop() - 1 /* return all results */
}

// ---------------------------------------------------------------------
// assert / select
// ---------------------------------------------------------------------

func baseAssert(L *State) int {
	if L.ToBoolean(1) { /* condition is true? */
		return L.GetTop() /* return all arguments */
	}
	/* error */
	L.CheckAny(1)                     /* there must be a condition */
	L.Remove(1)                       /* remove it */
	L.PushString("assertion failed!") /* default message */
	L.SetTop(1)                       /* leave only message (default if no other one) */
	return baseError(L)               /* call 'error' */
}

func baseSelect(L *State) int {
	n := L.GetTop()
	if L.Type(1) == LUA_TSTRING {
		s, _ := L.ToString(1)
		if len(s) > 0 && s[0] == '#' {
			L.PushInteger(int64(n - 1))
			return 1
		}
	}
	i := L.CheckInteger(1)
	if i < 0 {
		i = int64(n) + i
	} else if i > int64(n) {
		i = int64(n)
	}
	L.ArgCheck(i >= 1, 1, "index out of range")
	return n - int(i)
}

// ---------------------------------------------------------------------
// pcall / xpcall
// ---------------------------------------------------------------------

/*
** Continuation function for 'pcall' and 'xpcall'. Both functions
** already pushed a 'true' before doing the call, so in case of success
** 'finishpcall' only has to return everything in the stack minus
** 'extra' values (where 'extra' is exactly the number of items to be
** ignored).
 */
func baseFinishPCall(L *State, status, extra int) int {
	if status != LUA_OK && status != LUA_YIELD { /* error? */
		L.PushBoolean(false) /* first result (false) */
		L.PushValue(-2)      /* error message */
		return 2             /* return false, msg */
	}
	return L.GetTop() - extra /* return all results */
}

func basePCall(L *State) int {
	L.CheckAny(1)
	L.PushBoolean(true) /* first result if no errors */
	L.Insert(1)         /* put it in place (lua_insert(L, 1)) */
	/* 'finishpcall' is the continuation: in a yieldable thread C takes
	   the yieldable path (lua_pcallk), so 'coroutine.yield' may cross
	   a 'pcall'. */
	status := L.pcallK(L.GetTop()-2, LUA_MULTRET, 0, true)
	return baseFinishPCall(L, status, 0)
}

/*
** Do a protected call with error handling. After 'lua_rotate', the
** stack will have <f, err, true, f, [args...]>; so, the function passes
** 2 to 'finishpcall' to skip the 2 first values when returning results.
 */
func baseXPCall(L *State) int {
	n := L.GetTop()
	L.CheckType(2, LUA_TFUNCTION) /* check error function */
	L.PushBoolean(true)           /* first result */
	L.PushValue(1)                /* function */
	L.Rotate(3, 2)                /* move them below function's arguments */
	status := L.pcallK(n-2, LUA_MULTRET, 2, true)
	return baseFinishPCall(L, status, 2)
}

// ---------------------------------------------------------------------
// tostring
// ---------------------------------------------------------------------

func baseToString(L *State) int {
	L.CheckAny(1)
	luaL_tolstring(L, 1)
	return 1
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

var baseFuncs = []Reg{
	{Name: "assert", Func: baseAssert},
	{Name: "collectgarbage", Func: baseCollectGarbage},
	{Name: "dofile", Func: baseDoFile},
	{Name: "error", Func: baseError},
	{Name: "getmetatable", Func: baseGetMetatable},
	{Name: "ipairs", Func: baseIPairs},
	{Name: "loadfile", Func: baseLoadFile},
	{Name: "load", Func: baseLoad},
	{Name: "next", Func: baseNext},
	{Name: "pairs", Func: basePairs},
	{Name: "pcall", Func: basePCall},
	{Name: "print", Func: basePrint},
	{Name: "warn", Func: baseWarn},
	{Name: "rawequal", Func: baseRawEqual},
	{Name: "rawlen", Func: baseRawLen},
	{Name: "rawget", Func: baseRawGet},
	{Name: "rawset", Func: baseRawSet},
	{Name: "select", Func: baseSelect},
	{Name: "setmetatable", Func: baseSetMetatable},
	{Name: "tonumber", Func: baseToNumber},
	{Name: "tostring", Func: baseToString},
	{Name: "type", Func: baseType},
	{Name: "xpcall", Func: baseXPCall},
	/* placeholders */
	{Name: baseGName, Func: nil},
	{Name: "_VERSION", Func: nil},
}

// OpenBase opens the base library into the global table (luaopen_base).
func OpenBase(L *State) int {
	/* open lib into global table */
	L.PushGlobalTable()
	L.SetFuncs(baseFuncs, 0)
	/* set global _G */
	L.PushValue(-1)
	L.SetField(-2, baseGName)
	/* set global _VERSION */
	L.PushString(baseVersion)
	L.SetField(-2, "_VERSION")
	return 1
}
