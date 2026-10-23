// Translation of lbaselib.c (PUC-Lua 5.3.4): the basic library.
//
// Every luaB_* function keeps its C name prefixed with 'base', and the
// control flow follows the C source.  Three things are necessarily
// different from C: there is no collector of our own (so
// 'collectgarbage' only implements collect+count over Go's GC -- see
// baseCollectGarbage), the error mechanism is panic/recover instead of
// setjmp/longjmp (which is already handled by luaD_pcall), and every C
// function is a GoFunc.
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
	/* Lua 5.3.4: call global tostring (5.4 uses luaL_tolstring directly). */
	n := L.GetTop() /* number of arguments */
	L.GetGlobal("tostring")
	for i := 1; i <= n; i++ {
		L.PushValue(-1) /* function to be called */
		L.PushValue(i)  /* value to print */
		L.Call(1, 1)
		s, ok := L.ToString(-1) /* get result */
		if !ok {
			return L.Error2("'tostring' must return a string to 'print'")
		}
		if i > 1 {
			os.Stdout.WriteString("\t")
		}
		os.Stdout.WriteString(s)
		L.Pop(1) /* pop result */
	}
	os.Stdout.WriteString("\n")
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
	L.ArgCheck(t == LUA_TNIL || t == LUA_TTABLE, 2, "nil or table expected")
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
	/* lbaselib.c 5.3.4: luaL_argcheck with fixed message (no ", got TYPE"). */
	L.ArgCheck(t == LUA_TTABLE || t == LUA_TSTRING, 1, "table or string expected")
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

// The PUC 5.3.4 option names, in C's order.  Kept so scripts that pass
// known names do not get 'invalid option'; only "collect" and "count"
// do real work (see baseCollectGarbage).
var baseGCOpts = []string{"stop", "restart", "collect", "count", "step",
	"setpause", "setstepmul", "isrunning"}

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
// Intentional Go-GC simplification: only "collect" and "count" are
// implemented.  Every other known PUC option name is accepted (so
// CheckOption does not raise 'invalid option') but is a no-op that
// pushes integer 0 — no gcCycle on "step", no gcStopped toggle on
// stop/restart, no pause/stepmul mutation.  Unknown names still error.
//
// Note: the C API lua_gc (cdl_gc_cb) may still drive a real cycle and
// mutate knobs; Lua collectgarbage and C lua_gc can therefore diverge.
//
// "collect" sweeps weak tables and runs '__gc' of unreachable objects,
// then pushes 0.  "count" is Kbytes of the whole Go heap (not per-state).
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
	case baseGCCollect:
		L.fullCollect()
		L.PushInteger(0)
		return 1
	case baseGCCount:
		L.PushNumber(gcCount())
		return 1
	default:
		/* stop, restart, step, setpause, setstepmul, isrunning: ignore. */
		L.PushInteger(0)
		return 1
	}
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

/* basePairsMeta is pairsmeta (lbaselib.c): metamethod or default iterator. */
func basePairsMeta(L *State, method string, iszero bool, iter func(*State) int) int {
	L.CheckAny(1)
	if L.GetMetaField(1, method) == LUA_TNIL { /* no metamethod? */
		L.PushGoFunc(iter) /* will return generator, */
		L.PushValue(1)     /* state, */
		if iszero {
			L.PushInteger(0) /* and initial value */
		} else {
			L.PushNil()
		}
	} else {
		L.PushValue(1) /* argument 'self' to metamethod */
		/* 5.3.4 pairsmeta uses lua_call (not yieldable). */
		L.Call(1, 3) /* get 3 values from metamethod */
	}
	return 3
}

func basePairs(L *State) int {
	return basePairsMeta(L, "__pairs", false, baseNext)
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

/* baseIPairs: LUA_COMPAT_IPAIRS (default via LUA_COMPAT_5_2) uses __ipairs. */
func baseIPairs(L *State) int {
	return basePairsMeta(L, "__ipairs", true, baseIPairsAux)
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
	/* LUA_COMPAT_LOADSTRING (default-compat alias of load) */
	{Name: "loadstring", Func: baseLoad},
	{Name: "next", Func: baseNext},
	{Name: "pairs", Func: basePairs},
	{Name: "pcall", Func: basePCall},
	{Name: "print", Func: basePrint},
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
