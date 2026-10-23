// Translation of lbaselib.c (PUC-Lua 5.4.9): the basic library.
//
// Every luaB_* function keeps its C name prefixed with 'base', and the
// control flow follows the C source.  Three things are necessarily
// different from C: there is no collector of our own (so
// 'collectgarbage' drives Go's, and 'count' answers the Go heap rather
// than an allocator of ours -- see baseCollectGarbage), the error
// mechanism is panic/recover instead of setjmp/longjmp (which is
// already handled by luaD_pcall), and every C function is a GoFunc.
package core

import (
	"io"
	"os"
	"runtime"
	"strings"
)

// LUA_GNAME and LUA_VERSION are not defined in this port; they are
// local to the base library.
const (
	baseGName   = "_G"
	baseVersion = "Lua 5.3"
)

// ---------------------------------------------------------------------
// lTolstring (lauxlib.c luaL_tolstring)
// ---------------------------------------------------------------------

// lTolstring converts the value at 'idx' to a string using the
// '__tostring' metamethod (or '__name'), pushes the resulting string on
// the stack and returns it (lauxlib.c luaL_tolstring). The exported
// name luaL_tolstring belongs to the C API (see cdlopen.go).
//
// The implementation lives in lstrlib.go (strToLString), which already
// follows the C code step by step.
func lTolstring(L *State, idx int) string {
	return strToLString(L, idx)
}

// ---------------------------------------------------------------------
// print / warn
// ---------------------------------------------------------------------

func basePrint(L *State) int {
	n := L.GetTop()           /* number of arguments */
	for i := 1; i <= n; i++ { /* for each argument */
		s := lTolstring(L, i) /* convert it to string */
		if i > 1 {            /* not the first element? */
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
	if i >= len(s) || !localeIsAlnum(s[i]) { /* no digit? */
		return 0, 0, false
	}
	var n uint64
	for {
		var digit int
		if localeIsDigit(s[i]) {
			digit = int(s[i] - '0')
		} else {
			digit = int(localeToUpper(s[i])-'A') + 10
		}
		if digit >= base {
			return 0, 0, false /* invalid numeral */
		}
		n = n*uint64(base) + uint64(digit)
		i++
		if i >= len(s) || !localeIsAlnum(s[i]) {
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
	L.ArgExpected(t == LUA_TNIL || t == LUA_TTABLE, 2, "nil or table")
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
	/* lbaselib.c uses 'luaL_argexpected', so the message names the type
	   that came: "table or string expected, got boolean". */
	L.ArgExpected(t == LUA_TTABLE || t == LUA_TSTRING, 1, "table or string")
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

// The ten options, in C's order: 'luaB_collectgarbage' turns the name
// into one of these and then into a 'lua_gc' call, so the index here is
// what decides which branch of baseCollectGarbage runs.
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

// gcModeInc / gcModeGen are the two collector modes PUC-Lua knows.  A C
// library can ask for either through 'lua_gc' (see cdl_gc_cb), and the
// answer it gets is the one it replaced; 'collectgarbage' longer offers
// them, but the names stay because the C API still needs them.
const (
	gcModeInc = "incremental"
	gcModeGen = "generational"
)

// gcCount is 'lua_gc(L, LUA_GCCOUNT)' expressed the way lbaselib.c does
// it: Kbytes with a fractional part, i.e. bytes/1024 as a float.
func gcCount() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc>>10) + float64(ms.HeapAlloc&1023)/1024
}

// baseCollectGarbage is 'collectgarbage'.
//
// It is lbaselib.c's 'luaB_collectgarbage' over 'lua_gc', and it answers
// what that answers: 'count' a number, 'step' and 'isrunning' a boolean,
// 'setpause' and 'setstepmul' the value they replaced, 'generational'
// and 'incremental' the name of the mode they replaced, and the rest 0.
//
// It used to answer 0 to everything but 'count', on the grounds that
// there is no collector here to drive.  That was a translation left
// half finished rather than a decision: the C API's 'lua_gc' --
// cdl_gc_cb -- has always driven one, and the knobs it reads are the
// right ones already (pause 200, stepmul 100, generational, which is
// what a fresh state answers).  Only this function was throwing them
// away, so a program that reaches the collector from Lua and one that
// reaches it from C got two different collectors.
//
// Two answers cannot be the ones C computes, and both are said so where
// they are given: 'collect' sweeps the weak tables and runs the '__gc'
// of what has become unreachable, but the number it reports is not the
// memory C freed, because nothing was freed by an allocator of ours --
// and 'count' is the Kbytes of the whole Go heap, not of this state,
// because the heap is not accounted per state.
func baseCollectGarbage(L *State) int {
	/* From inside a '__gc' PUC-Lua answers nil for every option, and not
	   0: it has stopped the collector for the duration of the finalizer,
	   and 'lua_gc' turns that into -1, which lbaselib.c's 'checkvalres'
	   makes a fail.  So does this, for the same reason -- a finalizer is
	   the one place where "the collector cannot be asked" is something a
	   program can see. */
	if L.g.gcInFinalizer {
		L.PushNil()
		return 1
	}
	switch L.CheckOption(1, "collect", baseGCOpts) {
	case baseGCStop:
		L.g.gcStopped = true
	case baseGCRestart:
		L.g.gcStopped = false
	case baseGCCollect:
		L.fullCollect()
	case baseGCCount:
		L.PushNumber(gcCount())
		return 1
	case baseGCStep:
		/* C reads the second argument even though a step here ignores
		   what it says, so does this: 'collectgarbage("step", "x")'
		   is an error in C and has to be one here. */
		n := L.OptInteger(2, 0)
		L.gcCycle()
		/* C answers whether the step ended a cycle, and it splits on
		   the argument to decide: with no data it does a *basic*
		   step, which never ends one -- 'luaC_step' leaves the
		   collector in GCSpropagate, so 'g->gcstate == GCSpause' is
		   false and 'res' stays 0 -- while a step given 'data'
		   Kbytes of debt does enough work to finish.
		   Here a step is always a whole cycle, so the answer is the
		   one for work that was asked for, and 'false' for the basic
		   step whose answer PUC-Lua publishes.
		   It cannot answer false to both.  'repeat until
		   collectgarbage("step", siz)' -- the shape gc.lua uses to
		   force a collection -- would never end, and a script that
		   hangs is a worse answer than one that differs by a bit. */
		L.PushBoolean(n != 0)
		return 1
	case baseGCSetPause:
		L.PushInteger(int64(gcSetParam(&L.g.gcPause, L.OptInteger(2, 0))))
		return 1
	case baseGCSetStepMul:
		L.PushInteger(int64(gcSetParam(&L.g.gcStepMul, L.OptInteger(2, 0))))
		return 1
	case baseGCIsRunning:
		L.PushBoolean(!L.g.gcStopped)
		return 1
	case baseGCGenerational:
		_ = L.OptInteger(2, 0) /* minormul */
		_ = L.OptInteger(3, 0) /* majormul */
		L.PushString(gcSetMode(L, gcModeGen))
		return 1
	case baseGCIncremental:
		_ = L.OptInteger(2, 0) /* pause */
		_ = L.OptInteger(3, 0) /* stepmul */
		_ = L.OptInteger(4, 0) /* stepsize */
		L.PushString(gcSetMode(L, gcModeInc))
		return 1
	}
	L.PushInteger(0)
	return 1
}

// gcSetParam is 'setgcparam' inside 'lua_gc': the value it is handed
// replaces the knob -- a 0 is a value like any other, and 'setpause'
// with no argument really does set the pause to 0 -- and the value the
// knob had is what is answered.
func gcSetParam(p *int, v int64) int {
	prev := *p
	*p = int(v)
	return prev
}

// gcSetMode is 'lua_gc' for LUA_GCGEN and LUA_GCINC: the mode the state
// was in is answered, and the new one is what is left in it.
func gcSetMode(L *State, mode string) string {
	prev := L.g.gcMode
	L.g.gcMode = mode
	return prev
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
		if L.co != nil && L.co.sync {
			L.ci.goCont = func(L *State, status int) int { return 3 }
		}
		L.callK(1, 3, true) /* get 3 values from metamethod */
		if L.ci != nil {
			L.ci.goCont = nil
		}
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
	/* 'luaL_optstring(L, 1, NULL)': stdin is what the *absent* or nil
	   argument means, not the empty string -- 'loadfile("")' really does
	   try to open a file with no name, and fails. */
	stdin := L.IsNoneOrNil(1)
	/* 'luaL_loadfilex' hands the name to 'fopen', which is a C
	   function: it is the bytes before the first zero that are opened,
	   and the message names those and no more. */
	fname := luaCStr(L.OptString(1, ""))
	/* 'luaL_optstring(L, 2, NULL)': C gives 'lua_load' a NULL mode when
	   the argument is absent, and 'checkmode' does nothing then.  "bt"
	   allows the same two kinds of chunk, so it stands in for NULL. */
	mode := L.OptString(2, "bt")
	env := 0
	if !L.IsNone(3) { /* 'env' index or 0 if no 'env' */
		env = 3
	}
	var err error
	if stdin { /* no file name: read the standard input */
		var data []byte
		if data, err = baseReadStdin(); err == nil {
			err = L.LoadBytesMode(data, "=stdin", mode)
		}
	} else {
		err = L.LoadFileMode(fname, mode)
	}
	return baseLoadAux(L, err, env)
}

// baseLoadReader is generic_reader: it calls the reader function once
// and hands back what it returned, or nil when the chunk is over.
//
// In C the reader is called from inside the parser, which is protected,
// so an error raised by the reader -- or by the "must return a string"
// check -- becomes the status of 'lua_load' and 'load' returns nil plus
// the message.  Raising the error here does the same: the parser runs
// inside 'rawRunProtected', which turns it into the status of the load.
//
// The function is called through 'LoadStream', which pulls the pieces
// one at a time as the lexer needs them; that is what lets a chunk that
// fails to parse stop the reader instead of draining it.
func baseLoadReader(L *State) func() []byte {
	done := false /* the reader said it was over: do not ask again */
	return func() []byte {
		if done {
			return nil
		}
		L.PushValue(1) /* get function */
		if L.PCall(0, 1, 0) != LUA_OK {
			L.Error()  /* the reader's own error, with its position */
			return nil /* not reached */
		}
		if L.IsNil(-1) {
			L.Pop(1) /* pop result */
			done = true
			return nil /* end of chunk */
		}
		if !L.IsString(-1) {
			L.Pop(1)
			L.PushString("reader function must return a string")
			L.Error()
			return nil /* not reached */
		}
		s, _ := L.ToString(-1)
		L.Pop(1)
		if s == "" { /* empty string ends the chunk */
			done = true
			return nil
		}
		return []byte(s)
	}
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
	more := baseLoadReader(L)
	/* f_parser reads the first character *inside* the protected call
	   (ldo.c), so an error raised by the very first call to the reader
	   is the status of the load; it must not escape 'load' itself. */
	var first []byte
	top := L.top
	if status := L.rawRunProtected(func() { first = more() }); status != LUA_OK {
		msg := ""
		if L.top > top {
			msg = valueToStr(&L.stack[L.top-1])
		}
		L.top = top
		return baseLoadAux(L, &LuaError{Status: status, Message: msg}, env)
	}
	if len(first) > 0 && first[0] == dmpSignature[0] {
		/* a precompiled chunk: undump wants the whole thing, so the
		   rest of it is collected before it starts */
		data := append([]byte{}, first...)
		for {
			piece := more()
			if piece == nil {
				break
			}
			data = append(data, piece...)
		}
		return baseLoadAux(L, luaU_loadBinary(L, data, chunkname, mode), env)
	}
	return baseLoadAux(L, L.LoadStream(first, more, chunkname, mode), env)
}

func baseDoFile(L *State) int {
	stdin := L.IsNoneOrNil(1) /* see baseLoadFile */
	fname := luaCStr(L.OptString(1, ""))
	L.SetTop(1)
	var err error
	if stdin {
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
	   loaded chunk may yield when 'dofile' runs inside a coroutine.
	   A synchronous yield unwinds this frame and the resume finishes
	   it with the same result count. */
	if L.co != nil && L.co.sync {
		L.ci.goCont = func(L *State, status int) int { return L.GetTop() - 1 }
	}
	L.callK(0, LUA_MULTRET, true)
	if L.ci != nil {
		L.ci.goCont = nil
	}
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
	   a 'pcall'. A synchronous yield unwinds this frame; the resume
	   calls the continuation instead of returning here. */
	if L.co != nil && L.co.sync {
		L.ci.goCont = func(L *State, status int) int {
			return baseFinishPCall(L, status, 0)
		}
	}
	status := L.pcallK(L.GetTop()-2, LUA_MULTRET, 0, true)
	if L.ci != nil {
		L.ci.goCont = nil
	}
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
	if L.co != nil && L.co.sync {
		L.ci.goCont = func(L *State, status int) int {
			return baseFinishPCall(L, status, 2)
		}
	}
	status := L.pcallK(n-2, LUA_MULTRET, 2, true)
	if L.ci != nil {
		L.ci.goCont = nil
	}
	return baseFinishPCall(L, status, 2)
}

// ---------------------------------------------------------------------
// tostring
// ---------------------------------------------------------------------

func baseToString(L *State) int {
	L.CheckAny(1)
	lTolstring(L, 1)
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
