// Translation of lbaselib.c (PUC-Lua 5.4.9): the basic library.
//
// Every luaB_* function is named base*; control flow follows the C source.
// Intentional port exceptions: Go-GC collectgarbage subset (collect+count
// only; other known opts no-op→0), panic/recover instead of setjmp,
// index-stack (not TValue*), goroutine coroutines (goCont only on sync
// co). Every C function is a GoFunc.
package core

import (
	"io"
	"os"
	"runtime"
	"strings"
)

// LUA_GNAME / LUA_VERSION (lua.h / lauxlib.h) — local to the base library.
const (
	baseGName   = "_G"
	baseVersion = "Lua 5.4" /* LUA_VERSION */
)

// ---------------------------------------------------------------------
// lTolstring (lauxlib.c luaL_tolstring) — thin wrapper; body in lstrlib
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
// print / warn  (luaB_print / luaB_warn)
// ---------------------------------------------------------------------

// baseWriteString is lua_writestring (luaconf.h): fwrite(s,1,l,stdout).
// l is the byte length so embedded NULs are written.
func baseWriteString(s string, l int) {
	if l > 0 {
		os.Stdout.WriteString(s[:l]) /* fwrite((s), sizeof(char), (l), stdout) */
	}
}

// baseWriteLine is lua_writeline: newline + fflush(stdout).
func baseWriteLine() {
	baseWriteString("\n", 1)
	os.Stdout.Sync() /* fflush(stdout) */
}

func basePrint(L *State) int {
	n := L.GetTop()           /* number of arguments */
	for i := 1; i <= n; i++ { /* for each argument */
		s := lTolstring(L, i) /* convert it to string (luaL_tolstring) */
		if i > 1 {            /* not the first element? */
			baseWriteString("\t", 1) /* add a tab before it */
		}
		baseWriteString(s, len(s)) /* print it */
		L.Pop(1)                   /* pop result */
	}
	baseWriteLine()
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
		L.Warning(s, true) /* lua_warning(..., 1) */
	}
	s, _ := L.ToString(n)
	L.Warning(s, false) /* close warning */
	return 0
}

// ---------------------------------------------------------------------
// tonumber  (SPACECHARS / b_str2int / luaB_tonumber)
// ---------------------------------------------------------------------

const baseSpaceChars = " \f\n\r\t\v" /* SPACECHARS */

// baseStr2Int converts 's' in the given base; it returns the value, the
// index where the conversion stopped, and whether it succeeded
// (b_str2int).  The conversion is only valid when it consumes the
// whole string (caller checks end == len(s), matching C "s + l").
func baseStr2Int(s string, base int) (int64, int, bool) {
	i := 0
	for i < len(s) && strings.IndexByte(baseSpaceChars, s[i]) >= 0 {
		i++ /* skip initial spaces */
	}
	neg := false
	if i < len(s) && s[i] == '-' {
		i++
		neg = true /* handle sign */
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
		} else {
			/* C: s = lua_tolstring; if (s && lua_stringtonumber(L,s) == l+1) */
			s, ok := L.ToString(1)             /* lua_tolstring (also converts number→string) */
			if ok && strStringToNumber(L, s) { /* whole-string parse ≡ l+1 */
				return 1 /* successful conversion to number */
			}
			/* else not a number */
			L.CheckAny(1) /* (but there must be some parameter) */
		}
	} else {
		base := L.CheckInteger(2)
		L.CheckType(1, LUA_TSTRING) /* no numbers as strings */
		s, _ := L.ToString(1)       /* lua_tolstring → s, l */
		L.ArgCheck(2 <= base && base <= 36, 2, "base out of range")
		/* C: if (b_str2int(s, base, &n) == s + l) */
		if n, end, ok := baseStr2Int(s, int(base)); ok && end == len(s) {
			L.PushInteger(n)
			return 1
		} /* else not a number */
	} /* else not a number */
	L.PushNil() /* luaL_pushfail — not a number */
	return 1
}

// ---------------------------------------------------------------------
// error  (luaB_error)
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
// metatables  (luaB_getmetatable / luaB_setmetatable)
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
// raw*  (luaB_rawequal / rawlen / rawget / rawset)
// ---------------------------------------------------------------------

func baseRawEqual(L *State) int {
	L.CheckAny(1)
	L.CheckAny(2)
	L.PushBoolean(L.RawEqual(1, 2))
	return 1
}

func baseRawLen(L *State) int {
	t := L.Type(1)
	/* luaL_argexpected → "table or string expected, got …" */
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
// collectgarbage  (pushmode / luaB_collectgarbage) — intentional Go-GC
// ---------------------------------------------------------------------

// The PUC option names (including 5.4 mode names), in C's opts[] order.
// Kept so scripts that pass known names do not get 'invalid option';
// only "collect" and "count" do real work (see baseCollectGarbage).
var baseGCOpts = []string{"stop", "restart", "collect", "count", "step",
	"setpause", "setstepmul", "isrunning", "generational", "incremental"}

// indices into baseGCOpts — same order as C's optsnum[] → LUA_GC*
const (
	baseGCStop         = iota // LUA_GCSTOP
	baseGCRestart             // LUA_GCRESTART
	baseGCCollect             // LUA_GCCOLLECT
	baseGCCount               // LUA_GCCOUNT
	baseGCStep                // LUA_GCSTEP
	baseGCSetPause            // LUA_GCSETPAUSE
	baseGCSetStepMul          // LUA_GCSETSTEPMUL
	baseGCIsRunning           // LUA_GCISRUNNING
	baseGCGenerational        // LUA_GCGEN
	baseGCIncremental         // LUA_GCINC
)

// gcModeInc / gcModeGen are the two collector modes the C API's lua_gc
// still understands (see cdl_gc_cb).  collectgarbage no longer switches
// them; the names stay for the C side and as accepted no-op options.
const (
	gcModeInc = "incremental"
	gcModeGen = "generational"
)

// LUA_GCINC / LUA_GCGEN numeric values (lua.h) — used by basePushMode.
const (
	baseGCModeInc = 11 // LUA_GCINC
	baseGCModeGen = 10 // LUA_GCGEN
)

// gcCount is 'lua_gc(L, LUA_GCCOUNT)' + LUA_GCCOUNTB expressed the way
// lbaselib.c does it: Kbytes with a fractional part (bytes/1024 as float).
func gcCount() float64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.HeapAlloc>>10) + float64(ms.HeapAlloc&1023)/1024
}

// basePushMode is pushmode: push the previous collector mode name, or
// fail if oldmode == -1 (invalid lua_gc inside a finalizer).
func basePushMode(L *State, oldmode int) int {
	if oldmode == -1 {
		L.PushNil() /* luaL_pushfail — invalid call to 'lua_gc' */
	} else {
		if oldmode == baseGCModeInc {
			L.PushString("incremental")
		} else {
			L.PushString("generational")
		}
	}
	return 1
}

// baseCollectGarbage is luaB_collectgarbage.
//
// Intentional Go-GC simplification (not a mark-sweep port of lgc.c):
// only "collect" and "count" are implemented.  Every other known PUC
// option name (including mode names) is accepted but is a no-op that
// pushes integer 0 — no OptInteger validation of extra args (so
// collectgarbage("step","x") stays valid), no gcCycle on "step", no
// gcStopped toggle, no pause/stepmul/mode mutation, and mode cases do
// not call basePushMode.  Unknown names still error via CheckOption.
//
// Note: the C API lua_gc (cdl_gc_cb) may still drive a real cycle and
// mutate knobs; Lua collectgarbage and C lua_gc can therefore diverge.
//
// "collect" sweeps weak tables and runs '__gc' of unreachable objects,
// then pushes 0.  "count" is Kbytes of the whole Go heap (not per-state).
// Inside a finalizer (gcInFinalizer), after CheckOption, push fail/nil
// for every option (C: lua_gc → -1 → checkvalres → luaL_pushfail).
func baseCollectGarbage(L *State) int {
	/* o = optsnum[luaL_checkoption(...)] — index stands in for LUA_GC*
	   C runs checkoption *before* lua_gc; inside a finalizer lua_gc
	   returns -1 and checkvalres breaks to luaL_pushfail. */
	o := L.CheckOption(1, "collect", baseGCOpts)
	if L.g.gcInFinalizer {
		L.PushNil() /* luaL_pushfail — invalid call (inside a finalizer) */
		return 1
	}
	switch o {
	case baseGCCount: /* LUA_GCCOUNT (+ COUNTB fractional) */
		L.PushNumber(gcCount())
		return 1
	case baseGCStep: /* LUA_GCSTEP — Go-GC no-op (C: pushboolean) */
		L.PushInteger(0)
		return 1
	case baseGCSetPause, baseGCSetStepMul: /* setpause / setstepmul */
		L.PushInteger(0)
		return 1
	case baseGCIsRunning: /* LUA_GCISRUNNING — Go-GC no-op (C: pushboolean) */
		L.PushInteger(0)
		return 1
	case baseGCGenerational: /* LUA_GCGEN — Go-GC no-op (C: pushmode) */
		L.PushInteger(0)
		return 1
	case baseGCIncremental: /* LUA_GCINC — Go-GC no-op (C: pushmode) */
		L.PushInteger(0)
		return 1
	default: /* stop, restart, collect */
		if o == baseGCCollect {
			L.fullCollect()
		}
		L.PushInteger(0)
		return 1
	}
}

// gcSetParam is 'setgcparam' inside 'lua_gc': the value it is handed
// replaces the knob — a 0 is a value like any other — and the value the
// knob had is what is answered. Used by cdlopen lua_gc bridge.
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
	} else {
		L.PushNil()
		return 1
	}
}

// basePairsCont is pairscont (lua_Kfunction); unused args; returns 3.
func basePairsCont(L *State, status int) int {
	_, _ = L, status /* (void)L; (void)status; (void)k */
	return 3
}

func basePairs(L *State) int {
	L.CheckAny(1)
	if L.GetMetaField(1, "__pairs") == LUA_TNIL { /* no metamethod? */
		L.PushGoFunc(baseNext) /* will return generator, */
		L.PushValue(1)         /* state, */
		L.PushNil()            /* and initial value */
	} else {
		L.PushValue(1) /* argument 'self' to metamethod */
		/* C: lua_callk(..., pairscont) — yieldable. Sync co installs goCont. */
		if L.co != nil && L.co.sync {
			L.ci.goCont = basePairsCont
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
	i++ /* luaL_intop(+, i, 1) */
	L.PushInteger(i)
	if L.GetI(1, i) == LUA_TNIL {
		return 1
	} else {
		return 2
	}
}

/*
** 'ipairs' function. Returns 'ipairsaux', given "table", 0.
** (The given "table" may not be a table.)
** 5.4: no LUA_COMPAT_IPAIRS / __ipairs path.
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

// baseLoadAux finishes a 'load': on success it returns the chunk (with
// its environment set), on failure it returns fail plus the error
// message (load_aux).
//
// C passes a status and finds the error message already on the stack;
// the Go load helpers return an error and restore top, so the failure
// path re-pushes the message (same Lua results).
func baseLoadAux(L *State, err error, env int) int {
	if err == nil {
		if env != 0 { /* 'env' parameter? */
			L.PushValue(env) /* environment for loaded function */
			/* lua_setupvalue(L, -2, 1) */
			if _, ok := dbSetUpvalue(L, -2, 1); !ok {
				L.Pop(1) /* remove 'env' if not used by previous call */
			}
		}
		return 1
	} else { /* error (C: message already on stack; Go rebuilds it) */
		L.PushString(err.Error()) /* error message */
		L.PushNil()               /* luaL_pushfail */
		L.Insert(-2)              /* put fail before error message */
		return 2                  /* return fail plus error message */
	}
}

// baseReadStdin reads a whole chunk from the standard input
// (stand-in for luaL_loadfilex with NULL filename).
func baseReadStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}

func baseLoadFile(L *State) int {
	/* luaL_optstring(L, 1, NULL): absent/nil → stdin; "" opens empty name. */
	stdin := L.IsNoneOrNil(1)
	/* fopen-like: bytes before first NUL. */
	fname := luaCStr(L.OptString(1, ""))
	/* luaL_optstring(L, 2, NULL): "bt" stands in for NULL mode
	   (checkmode skip ≡ allow b+t). */
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

/*
** {======================================================
** Generic Read function
** =======================================================
 */

// RESERVEDSLOT (lbaselib.c): above all arguments, holds a copy of the
// reader string so the parser can keep a stable pointer (C GC). Go still
// copies to []byte for the stream API; the slot mirrors C stack layout.
const baseReservedSlot = 5 /* RESERVEDSLOT */

// baseLoadReader is generic_reader.
//
// Called from inside protected LoadStream / rawRunProtected (as C's
// reader runs inside protected lua_load), so L.Call errors become the
// load status. Empty string → size 0 for that fill (EOZ that fill);
// nil → end. Re-call after empty is an llex/ZIO concern (port debt).
func baseLoadReader(L *State) func() []byte {
	return func() []byte {
		L.CheckStack2(2, "too many nested functions")
		L.PushValue(1) /* get function */
		L.Call(0, 1)   /* call it (unprotected; outer load is protected) */
		if L.IsNil(-1) {
			L.Pop(1) /* pop result */
			return nil
		} else if !L.IsString(-1) {
			L.Error2("reader function must return a string")
			return nil /* not reached */
		}
		L.Replace(baseReservedSlot) /* save string in reserved slot */
		s, _ := L.ToString(baseReservedSlot)
		return []byte(s)
	}
}

func baseLoad(L *State) int {
	var status error
	s, ok := L.ToString(1)
	mode := L.OptString(3, "bt")
	env := 0
	if !L.IsNone(4) { /* 'env' index or 0 if no 'env' */
		env = 4
	}
	if ok { /* loading a string? */
		chunkname := L.OptString(2, s)
		status = L.LoadBytesMode([]byte(s), chunkname, mode) /* luaL_loadbufferx */
	} else { /* loading from a reader function */
		chunkname := L.OptString(2, "=(load)")
		L.CheckType(1, LUA_TFUNCTION)
		L.SetTop(baseReservedSlot) /* create reserved slot */
		more := baseLoadReader(L)
		/* C: lua_load(generic_reader) — first byte inside protected f_parser.
		   Go LoadStream is text-oriented; peek first piece (protected) to
		   dispatch binary undump vs text stream (port debt). */
		var first []byte
		top := L.top
		if st := L.rawRunProtected(func() { first = more() }); st != LUA_OK {
			msg := ""
			if L.top > top {
				msg = valueToStr(&L.stack[L.top-1])
			}
			L.top = top
			return baseLoadAux(L, &LuaError{Status: st, Message: msg}, env)
		}
		if len(first) > 0 && first[0] == dmpSignature[0] {
			data := append([]byte{}, first...)
			for {
				piece := more()
				if piece == nil || len(piece) == 0 { /* EOZ: nil or empty */
					break
				}
				data = append(data, piece...)
			}
			status = luaU_loadBinary(L, data, chunkname, mode)
		} else {
			status = L.LoadStream(first, more, chunkname, mode)
		}
	}
	return baseLoadAux(L, status, env)
}

/* }====================================================== */

// baseDoFileCont is dofilecont.
func baseDoFileCont(L *State, status int) int {
	_ = status
	return L.GetTop() - 1
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
	/* C: lua_callk(..., dofilecont). Sync co installs goCont. */
	if L.co != nil && L.co.sync {
		L.ci.goCont = baseDoFileCont
	}
	L.callK(0, LUA_MULTRET, true)
	if L.ci != nil {
		L.ci.goCont = nil
	}
	return baseDoFileCont(L, 0)
}

// ---------------------------------------------------------------------
// assert / select
// ---------------------------------------------------------------------

func baseAssert(L *State) int {
	if L.ToBoolean(1) { /* condition is true? */
		return L.GetTop() /* return all arguments */
	} else { /* error */
		L.CheckAny(1)                     /* there must be a condition */
		L.Remove(1)                       /* remove it */
		L.PushString("assertion failed!") /* default message */
		L.SetTop(1)                       /* leave only message (default if no other one) */
		return baseError(L)               /* call 'error' */
	}
}

func baseSelect(L *State) int {
	n := L.GetTop()
	if L.Type(1) == LUA_TSTRING {
		s, _ := L.ToString(1)
		if len(s) > 0 && s[0] == '#' { /* *lua_tostring(L, 1) == '#' */
			L.PushInteger(int64(n - 1))
			return 1
		}
	}
	/* else */
	{
		i := L.CheckInteger(1)
		if i < 0 {
			i = int64(n) + i
		} else if i > int64(n) {
			i = int64(n)
		}
		L.ArgCheck(1 <= i, 1, "index out of range")
		return n - int(i)
	}
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
	} else {
		return L.GetTop() - extra /* return all results */
	}
}

func basePCall(L *State) int {
	L.CheckAny(1)
	L.PushBoolean(true) /* first result if no errors */
	L.Insert(1)         /* put it in place */
	/* finishpcall continuation: sync co may yield across pcall. */
	if L.co != nil && L.co.sync {
		extra := 0
		L.ci.goCont = func(L *State, status int) int {
			return baseFinishPCall(L, status, extra)
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
// library registration  (base_funcs / luaopen_base)
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
