// Translation of ldblib.c (PUC-Lua 5.4.9): the debug library.
//
// C calls the debug API of ldebug.c / lapi.c (lua_getstack, lua_getinfo,
// lua_getlocal, lua_getupvalue, ...); counterparts are translated here
// (dbGetStack, dbGetInfo, dbGetLocal, dbGetUpvalue, ...) on top of
// currentLine, luaG_getfuncline, luaF_getlocalname, luaO_chunkid,
// upvalValue, getFuncName. Upvalue bodies delegate to lapi
// (GetUpvalue/SetUpvalue/UpvalueId). FLAG intentional diffs: see
// /workspace/mafang/golua-align/reports2/ldblib.md.
package core

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// dbHookKey is the registry key of the hook table (HOOKKEY).
const dbHookKey = "_HOOKKEY"

// hook masks (lua.h)
const (
	dbMaskCall  = 1 << 0 // LUA_MASKCALL
	dbMaskRet   = 1 << 1 // LUA_MASKRET
	dbMaskLine  = 1 << 2 // LUA_MASKLINE
	dbMaskCount = 1 << 3 // LUA_MASKCOUNT
)

// hook events (lua.h)
const (
	hookCall = iota
	hookRet
	hookLine
	hookCount
	hookTailCall
)

// dbAr collects the fields of lua_Debug that the library fills in.
type dbAr struct {
	event           int
	source          string
	shortSrc        string
	linedefined     int
	lastlinedefined int
	what            string
	currentline     int
	nups            int
	nparams         int
	isvararg        bool
	name            string
	namewhat        string
	ftransfer       int
	ntransfer       int
	istailcall      bool
	ci              *CallInfo
}

// ---------------------------------------------------------------------
// counterparts of the debug API (ldebug.c / lapi.c)
// ---------------------------------------------------------------------

// dbGetThread checks for an optional thread as the function's first
// argument (getthread).
func dbGetThread(L *State, arg *int) *State {
	if L.IsThread(1) {
		*arg = 1
		return L.ToThread(1)
	}
	*arg = 0
	return L /* function will operate over current thread */
}

// dbCheckStack is the 'checkstack' of ldblib.c: L1 may be in any state,
// so pushes on it must be checked.
func dbCheckStack(L, L1 *State, n int) {
	if L != L1 && !L1.CheckStack(n) {
		L.Error2("stack overflow")
	}
}

// dbGetStack is lua_getstack: it fills 'ar' with the frame at 'level'.
func dbGetStack(L *State, level int, ar *dbAr) bool {
	if level < 0 {
		return false /* invalid (negative) level */
	}
	ci := L.ci
	for level > 0 && ci != &L.baseCI {
		level--
		ci = ci.prev
	}
	if level == 0 && ci != &L.baseCI { /* level found? */
		ar.ci = ci
		return true
	}
	return false /* no such level */
}

// dbPush pushes a value that is not reachable through an API index
// (there is no lua_pushvalue for a *Value in this port).
func dbPush(L *State, v *Value) {
	L.checkStack(1)
	L.stack[L.top] = *v
	L.top++
}

// dbFindLocal is luaG_findlocal with a stack-slot return (C: StkId *pos).
// Control flow matches ldebug.c; vararg names/slots via findvararg;
// PC via currentpc. ci.next nil-guard is defensive (C assumes ci->next
// when ci != L->ci). Name-only callers use ldebug.luaG_findlocal.
func dbFindLocal(L *State, ci *CallInfo, n int) (string, int) {
	base := ci.funcIdx + 1
	var name string
	if isLua(ci) {
		if n < 0 { /* access to vararg values? */
			var pos int
			name = findvararg(L, ci, n, &pos)
			if name == "" {
				return "", 0 /* no such vararg */
			}
			return name, pos
		}
		name = luaF_getlocalname(clLvalue(&L.stack[ci.funcIdx]).p, n, currentpc(ci))
	}
	if name == "" { /* no 'standard' name? */
		limit := L.top
		if ci != L.ci && ci.next != nil {
			limit = ci.next.funcIdx
		}
		if limit-base >= n && n > 0 { /* is 'n' inside 'ci' stack? */
			/* generic name for any valid slot */
			if isLua(ci) {
				name = "(temporary)"
			} else {
				name = "(C temporary)"
			}
		} else {
			return "", 0 /* no name */
		}
	}
	return name, base + (n - 1) /* C: if (pos) *pos = base + (n - 1) */
}

// dbGetLocal is lua_getlocal(L, ar, n): it pushes the value of the local
// variable and returns its name ("" when there is no such variable).
// ar == nil is the non-active function path (name only; value not pushed).
func dbGetLocal(L *State, ar *dbAr, n int) string {
	if ar == nil { /* information about non-active function? */
		fv := &L.stack[L.top-1]
		if !ttisLclosure(fv) { /* not a Lua function? */
			return ""
		}
		/* consider live variables at function start (parameters) */
		return luaF_getlocalname(clLvalue(fv).p, n, 0)
	}
	ci := ar.ci
	name, slot := dbFindLocal(L, ci, n)
	if name == "" {
		return ""
	}
	dbPush(L, &L.stack[slot])
	return name
}

// dbSetLocal is lua_setlocal(L, ar, n): it pops a value from the stack
// and stores it in the local variable, returning its name.
func dbSetLocal(L *State, ar *dbAr, n int) string {
	ci := ar.ci
	name, slot := dbFindLocal(L, ci, n)
	if name == "" {
		return ""
	}
	L.stack[slot] = L.stack[L.top-1]
	L.top-- /* pop value */
	return name
}

// dbFuncInfo is funcinfo: it fills the source information of 'ar'.
func dbFuncInfo(ar *dbAr, fv *Value) {
	if fv == nil || !ttisLclosure(fv) {
		ar.source = "=[C]"
		ar.linedefined = -1
		ar.lastlinedefined = -1
		ar.what = "C"
	} else {
		p := clLvalue(fv).p
		if p.source != nil {
			ar.source = p.source.val
		} else {
			ar.source = "=?"
		}
		ar.linedefined = p.linedefined
		ar.lastlinedefined = p.lastlinedefined
		if ar.linedefined == 0 {
			ar.what = "main"
		} else {
			ar.what = "Lua"
		}
	}
	/* FLAG: C passes srclen into luaO_chunkid; Go takes the whole
	   string — equivalent short_src. */
	ar.shortSrc = luaO_chunkid(ar.source)
}

// dbCollectValidLines is collectvalidlines: it pushes a table with the
// active lines of a function (or nil for a non-Lua function).
//
// FLAG: lineinfo already holds absolute lines, so there is no
// 'nextline' / ABSLINEINFO walk; each entry is used as the line key
// (equivalent result for protos with debug info).
func dbCollectValidLines(L *State, fv *Value) {
	if fv == nil || !ttisLclosure(fv) {
		L.PushNil()
		return
	}
	p := clLvalue(fv).p
	L.NewTable()
	if len(p.lineinfo) == 0 { /* proto without debug information? */
		return
	}
	i := 0
	if p.isVararg != 0 {
		i = 1 /* skip first instruction (OP_VARARGPREP) */
	}
	for ; i < len(p.lineinfo); i++ {
		line := luaG_getfuncline(p, i)
		L.PushBoolean(true)
		L.SetI(-2, int64(line)) /* table[line] = true */
	}
}

// dbAuxGetInfo is auxgetinfo: it fills 'ar' according to 'what'.
func dbAuxGetInfo(L *State, what string, ar *dbAr, fv *Value, ci *CallInfo) bool {
	status := true
	for i := 0; i < len(what); i++ {
		switch what[i] {
		case 'S':
			dbFuncInfo(ar, fv)
		case 'l':
			if ci != nil && isLua(ci) {
				ar.currentline = currentLine(L, ci)
			} else {
				ar.currentline = -1
			}
		case 'u':
			ar.nups = 0
			if fv != nil {
				if ttisLclosure(fv) {
					ar.nups = clLvalue(fv).nupvalues
				} else if ttisCclosure(fv) {
					ar.nups = clCvalue(fv).nupvalues
				}
			}
			if fv == nil || !ttisLclosure(fv) {
				ar.isvararg = true
				ar.nparams = 0
			} else {
				p := clLvalue(fv).p
				ar.isvararg = p.isVararg != 0
				ar.nparams = int(p.numparams)
			}
		case 't':
			ar.istailcall = ci != nil && ci.callstatus&CIST_TAIL != 0
		case 'n':
			var name string
			ar.namewhat = getFuncName(L, ci, &name)
			if ar.namewhat == "" { /* not found */
				ar.namewhat = ""
				ar.name = ""
			} else {
				ar.name = name
			}
		case 'r':
			/* io->u2.transferinfo: set by luaD_hook while a call or
			   return hook is running (ldebug.c). */
			if ci == nil || ci.callstatus&CIST_TRAN == 0 {
				ar.ftransfer = 0
				ar.ntransfer = 0
			} else {
				ar.ftransfer = ci.ftransfer
				ar.ntransfer = ci.ntransfer
			}
		case 'L', 'f': /* handled by dbGetInfo */
		default:
			status = false /* invalid option */
		}
	}
	return status
}

// dbGetInfo is lua_getinfo. With a leading '>' the function is taken
// from (and popped off) the top of the stack; otherwise it is the
// function of the frame in 'ar'.
func dbGetInfo(L *State, what string, ar *dbAr) bool {
	var fv *Value
	var ci *CallInfo
	if len(what) > 0 && what[0] == '>' {
		ci = nil
		fv = &L.stack[L.top-1] /* get function from stack */
		what = what[1:]        /* skip the '>' */
		L.top--                /* pop function */
	} else {
		ci = ar.ci
		fv = &L.stack[ci.funcIdx]
	}
	status := dbAuxGetInfo(L, what, ar, fv, ci)
	if strings.ContainsRune(what, 'f') {
		dbPush(L, fv)
	}
	if strings.ContainsRune(what, 'L') {
		dbCollectValidLines(L, fv)
	}
	return status
}

// dbGetUpvalue is lua_getupvalue (body in lapi.GetUpvalue / aux_upvalue).
func dbGetUpvalue(L *State, idx, n int) (string, bool) {
	return L.GetUpvalue(idx, n)
}

// dbSetUpvalue is lua_setupvalue (body in lapi.SetUpvalue / aux_upvalue).
// Barrier omitted in lapi (Go GC).
func dbSetUpvalue(L *State, idx, n int) (string, bool) {
	return L.SetUpvalue(idx, n)
}

// dbUpvalueID is lua_upvalueid (body in lapi.UpvalueId). Soft-nil for
// non-functions so cdlopen exports do not panic on a bad index tag;
// C would api_check. Closure cases match getupvalref / sizeupvalues.
func dbUpvalueID(L *State, idx, n int) unsafe.Pointer {
	fv := index2value(L, idx)
	switch ttypetag(fv) {
	case LUA_VLCL, LUA_VCCL, LUA_VLCF:
		return L.UpvalueId(idx, n)
	default:
		return nil
	}
}

// ---------------------------------------------------------------------
// library functions
// ---------------------------------------------------------------------

func dbGetRegistry(L *State) int {
	L.PushValue(LUA_REGISTRYINDEX)
	return 1
}

func dbGetMetaTable(L *State) int {
	L.CheckAny(1)
	if !L.GetMetaTable(1) {
		L.PushNil() /* no metatable */
	}
	return 1
}

func dbSetMetaTable(L *State) int {
	t := L.Type(2)
	L.ArgExpected(t == LUA_TNIL || t == LUA_TTABLE, 2, "nil or table")
	L.SetTop(2)
	L.SetMetaTable(1)
	return 1 /* return 1st argument */
}

func dbGetUserValue(L *State) int {
	n := int(L.OptInteger(2, 1))
	if L.Type(1) != LUA_TUSERDATA {
		L.PushFail() /* luaL_pushfail */
		return 1
	}
	if L.GetUserValue(1, n) != LUA_TNONE {
		L.PushBoolean(true)
		return 2
	}
	return 1
}

func dbSetUserValue(L *State) int {
	n := int(L.OptInteger(3, 1))
	L.CheckType(1, LUA_TUSERDATA)
	L.CheckAny(2)
	L.SetTop(2)
	if L.SetUserValue(1, n) == 0 {
		L.PushFail() /* luaL_pushfail */
	}
	return 1
}

// dbSetTabSS is settabss: t[k] = v. Note that C pushes an empty string
// as an empty string; only a NULL (the absent 'name') becomes nil.
func dbSetTabSS(L *State, k, v string) {
	L.PushString(v)
	L.SetField(-2, k)
}

// dbSetTabSSNull is settabss for 'ar.name', which C stores as NULL (and
// therefore pushes as nil) when the function has no name.
func dbSetTabSSNull(L *State, k, v string) {
	if v == "" {
		L.PushNil()
	} else {
		L.PushString(v)
	}
	L.SetField(-2, k)
}

func dbSetTabSI(L *State, k string, v int) {
	L.PushInteger(int64(v))
	L.SetField(-2, k)
}

func dbSetTabSB(L *State, k string, v bool) {
	L.PushBoolean(v)
	L.SetField(-2, k)
}

// dbTreatStackOption moves a result of dbGetInfo from L1 to the result
// table of L (treatstackoption).
func dbTreatStackOption(L, L1 *State, fname string) {
	if L == L1 {
		L.Rotate(-2, 1) /* exchange object and table */
	} else {
		L.XMove(L1, 1) /* move object to the "main" stack */
	}
	L.SetField(-2, fname) /* put object into table */
}

func dbGetInfoFunc(L *State) int {
	var ar dbAr
	arg := 0
	L1 := dbGetThread(L, &arg)
	options := L.OptString(arg+2, "flnSrtu")
	dbCheckStack(L, L1, 3)
	L.ArgCheck(len(options) == 0 || options[0] != '>', arg+2, "invalid option '>'")
	if L.IsFunction(arg + 1) { /* info about a function? */
		options = ">" + options /* add '>' to 'options' */
		L.PushValue(arg + 1)    /* move function to 'L1' stack */
		L1.XMove(L, 1)
	} else { /* stack level */
		level := int(L.CheckInteger(arg + 1))
		if !dbGetStack(L1, level, &ar) {
			L.PushFail() /* luaL_pushfail: level out of range */
			return 1
		}
	}
	if !dbGetInfo(L1, options, &ar) {
		return L.ArgError(arg+2, "invalid option")
	}
	L.NewTable() /* table to collect results */
	if strings.ContainsRune(options, 'S') {
		L.PushString(ar.source)
		L.SetField(-2, "source")
		dbSetTabSS(L, "short_src", ar.shortSrc)
		dbSetTabSI(L, "linedefined", ar.linedefined)
		dbSetTabSI(L, "lastlinedefined", ar.lastlinedefined)
		dbSetTabSS(L, "what", ar.what)
	}
	if strings.ContainsRune(options, 'l') {
		dbSetTabSI(L, "currentline", ar.currentline)
	}
	if strings.ContainsRune(options, 'u') {
		dbSetTabSI(L, "nups", ar.nups)
		dbSetTabSI(L, "nparams", ar.nparams)
		dbSetTabSB(L, "isvararg", ar.isvararg)
	}
	if strings.ContainsRune(options, 'n') {
		dbSetTabSSNull(L, "name", ar.name) /* nil when there is no name */
		/* 'namewhat' is "" when there is no name, which is a real
		   (empty) string and must not be turned into nil */
		L.PushString(ar.namewhat)
		L.SetField(-2, "namewhat")
	}
	if strings.ContainsRune(options, 'r') {
		dbSetTabSI(L, "ftransfer", ar.ftransfer)
		dbSetTabSI(L, "ntransfer", ar.ntransfer)
	}
	if strings.ContainsRune(options, 't') {
		dbSetTabSB(L, "istailcall", ar.istailcall)
	}
	if strings.ContainsRune(options, 'L') {
		dbTreatStackOption(L, L1, "activelines")
	}
	if strings.ContainsRune(options, 'f') {
		dbTreatStackOption(L, L1, "func")
	}
	return 1 /* return table */
}

func dbGetLocalFunc(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	nvar := int(L.CheckInteger(arg + 2)) /* local-variable index */
	if L.IsFunction(arg + 1) {           /* function argument? */
		L.PushValue(arg + 1) /* push function */
		/* C: lua_pushstring(L, lua_getlocal(L, NULL, nvar));
		   NULL name → push nil (not an empty string). */
		name := dbGetLocal(L, nil, nvar)
		if name == "" {
			L.PushNil()
		} else {
			L.PushString(name)
		}
		return 1 /* return only name (there is no value) */
	}
	var ar dbAr
	level := int(L.CheckInteger(arg + 1))
	if !dbGetStack(L1, level, &ar) { /* out of range? */
		return L.ArgError(arg+1, "level out of range")
	}
	dbCheckStack(L, L1, 1)
	name := dbGetLocal(L1, &ar, nvar)
	if name != "" {
		L.XMove(L1, 1)     /* move local value */
		L.PushString(name) /* push name */
		L.Rotate(-2, 1)    /* re-order */
		return 2
	}
	L.PushFail() /* luaL_pushfail: no name (nor value) */
	return 1
}

func dbSetLocalFunc(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	var ar dbAr
	level := int(L.CheckInteger(arg + 1))
	nvar := int(L.CheckInteger(arg + 2))
	if !dbGetStack(L1, level, &ar) { /* out of range? */
		return L.ArgError(arg+1, "level out of range")
	}
	L.CheckAny(arg + 3)
	L.SetTop(arg + 3)
	dbCheckStack(L, L1, 1)
	L1.XMove(L, 1) /* move value to L1 */
	name := dbSetLocal(L1, &ar, nvar)
	if name == "" {
		L1.Pop(1) /* pop value (if not popped by dbSetLocal) */
	}
	/* C: lua_pushstring(L, name) — NULL → nil */
	if name == "" {
		L.PushNil()
	} else {
		L.PushString(name)
	}
	return 1
}

// dbAuxUpValue gets (when 'get') or sets an upvalue of a closure
// (auxupvalue).
func dbAuxUpValue(L *State, get bool) int {
	n := int(L.CheckInteger(2))   /* upvalue index */
	L.CheckType(1, LUA_TFUNCTION) /* closure */
	var name string
	var ok bool
	if get {
		name, ok = dbGetUpvalue(L, 1, n)
	} else {
		name, ok = dbSetUpvalue(L, 1, n)
	}
	if !ok {
		return 0 /* no such upvalue: no results at all */
	}
	L.PushString(name)
	if get {
		L.Insert(-2) /* no-op if get is false */
	}
	if get {
		return 2
	}
	return 1
}

func dbGetUpValueFunc(L *State) int { return dbAuxUpValue(L, true) }
func dbSetUpValueFunc(L *State) int { L.CheckAny(3); return dbAuxUpValue(L, false) }

// dbCheckUpVal checks whether an upvalue exists and returns its index
// (checkupval); 'id' is nil when the index is invalid.
func dbCheckUpVal(L *State, argf, argnup int, pnup *int) unsafe.Pointer {
	n := int(L.CheckInteger(argnup)) /* upvalue index */
	L.CheckType(argf, LUA_TFUNCTION) /* closure */
	id := dbUpvalueID(L, argf, n)
	if pnup != nil {
		L.ArgCheck(id != nil, argnup, "invalid upvalue index")
		*pnup = n
	}
	return id
}

func dbUpValueIDFunc(L *State) int {
	id := dbCheckUpVal(L, 1, 2, nil)
	if id != nil {
		L.PushLightUserData(id)
	} else {
		L.PushFail() /* luaL_pushfail */
	}
	return 1
}

func dbUpValueJoin(L *State) int {
	var n1, n2 int
	dbCheckUpVal(L, 1, 2, &n1)
	dbCheckUpVal(L, 3, 4, &n2)
	L.ArgCheck(!L.IsGoFunction(1), 1, "Lua function expected")
	L.ArgCheck(!L.IsGoFunction(3), 3, "Lua function expected")
	L.UpvalueJoin(1, n1, 3, n2) /* lua_upvaluejoin; barrier omitted in lapi (Go GC) */
	return 0
}

// ---------------------------------------------------------------------
// hooks
// ---------------------------------------------------------------------

// Nothing is remembered about a hook outside the thread it was set on.
//
// C keeps the whole of it in three fields of the lua_State -- 'hook',
// 'hookmask' and 'basehookcount' -- and 'db_gethook' reads those, for
// the thread the question was asked about.  It has to be that way for
// one reason a map of its own cannot reproduce: 'lua_newthread' copies
// all three into the new thread, so a coroutine made while a hook is
// running inherits it, and 'debug.gethook(co)' then answers the main
// thread's mask and count -- for a coroutine whose own hook was never
// set.
//
// A map keyed by the thread, which is how this used to be done, gets
// that case wrong (it answers nil for a coroutine that does have a
// hook) and pins every thread that ever had one in memory besides,
// since the map's keys are strong while C's hook table is weak ("k").

// dbHookNames are the event names passed to the hook function (ldblib.c).
var dbHookNames = []string{"call", "return", "line", "count", "tail call"}

// dbHookFunc is the hookf of ldblib.c: it calls the Lua function stored
// in the hook table for the running thread.
func dbHookFunc(L *State, ar *dbAr) {
	L.GetField(LUA_REGISTRYINDEX, dbHookKey) /* hook table */
	L.PushThread()
	if L.RawGet(-2) == LUA_TFUNCTION { /* is there a hook function? */
		L.PushString(dbHookNames[ar.event]) /* push event name */
		if ar.currentline >= 0 {
			L.PushInteger(int64(ar.currentline)) /* push current line */
		} else {
			L.PushNil()
		}
		/* C: lua_assert(lua_getinfo(L, "lS", ar)); release assert is
		   ((void)0) — skipped. Stack still holds hooktable under the
		   call; luaD_hook restores top after return. */
		/* lua_call (not pcall): errors propagate; yield across hook is a
		   C-call boundary. */
		L.Call(2, 0) /* call hook function */
	}
	/* leftover hook table (+ non-function) discarded when luaD_hook restores top */
}

// dbMakeMask converts a string mask into a bit mask (makemask).
func dbMakeMask(smask string, count int) int {
	mask := 0
	if strings.ContainsRune(smask, 'c') {
		mask |= dbMaskCall
	}
	if strings.ContainsRune(smask, 'r') {
		mask |= dbMaskRet
	}
	if strings.ContainsRune(smask, 'l') {
		mask |= dbMaskLine
	}
	if count > 0 {
		mask |= dbMaskCount
	}
	return mask
}

// dbUnmakeMask converts a bit mask into a string mask (unmakemask).
func dbUnmakeMask(mask int) string {
	var smask []byte
	if mask&dbMaskCall != 0 {
		smask = append(smask, 'c')
	}
	if mask&dbMaskRet != 0 {
		smask = append(smask, 'r')
	}
	if mask&dbMaskLine != 0 {
		smask = append(smask, 'l')
	}
	return string(smask)
}

// dbCreateHookTable creates (and initializes) the hook table.
func dbCreateHookTable(L *State) {
	if !L.GetSubTable(LUA_REGISTRYINDEX, dbHookKey) {
		/* table just created; initialize it */
		L.PushString("k")
		L.SetField(-2, "__mode") /* hooktable.__mode = "k" */
		L.PushValue(-1)
		L.SetMetaTable(-2) /* metatable(hooktable) = hooktable */
	}
}

func dbSetHook(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	var (
		hook  func(*State, *dbAr)
		mask  int
		count int
	)
	if L.IsNoneOrNil(arg + 1) { /* no hook? */
		L.SetTop(arg + 1)
		hook = nil
		mask = 0
		count = 0 /* turn off hooks */
	} else {
		smask := L.CheckString(arg + 2)
		L.CheckType(arg+1, LUA_TFUNCTION)
		count = int(L.OptInteger(arg+3, 0))
		hook = dbHookFunc
		mask = dbMakeMask(smask, count)
	}
	/* Whether a hook was installed is what 'gethook' asks about, and
	   'lua_sethook' installs none when the mask is empty (ldebug.c):
	   "turn off hooks" sets both the mask and the function to zero.  A
	   mask with no recognised character in it -- 'debug.sethook(f, "z")'
	   -- is such an empty mask, so the function is stored in the hook
	   table but the hook itself is off, and 'debug.gethook()' answers
	   nil rather than the function.  Nothing else needs recording: the
	   mask and the count are on the thread, as in C. */
	dbCreateHookTable(L)
	dbCheckStack(L, L1, 1)
	L1.PushThread()      /* key (thread) */
	L.XMove(L1, 1)       /* move it to the stack of L */
	L.PushValue(arg + 1) /* value (hook function) */
	L.RawSet(-3)         /* hooktable[L1] = new Lua hook */
	L1.setHook(hook, mask, count)
	return 0
}

func dbGetHook(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	mask := int(L1.hookmask) /* lua_gethookmask(L1) */
	if L1.hook == nil {      /* no hook? */
		L.PushFail() /* luaL_pushfail */
		return 1
	} else if L1.hookExternal { /* external hook (C: hook != hookf) */
		L.PushString("external hook")
	} else { /* hook table must exist */
		L.GetField(LUA_REGISTRYINDEX, dbHookKey)
		dbCheckStack(L, L1, 1)
		L1.PushThread()
		L.XMove(L1, 1)
		L.RawGet(-2) /* 1st result = hooktable[L1] */
		L.Remove(-2) /* remove hook table */
	}
	L.PushString(dbUnmakeMask(mask))       /* 2nd result = mask */
	L.PushInteger(int64(L1.basehookcount)) /* 3rd result = count */
	return 3
}

// dbDebugFunc is 'db_debug': a prompt on stderr, one command per line
// from stdin, and 'cont' -- or the end of input -- to leave.  Nothing
// about it is interactive in the terminal sense: it is a loop over
// standard input, which is why it can be driven from a pipe, and why
// leaving it out was only ever a missing function and not a design
// decision.
func dbDebugFunc(L *State) int {
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(os.Stderr, dbDebugPrompt)
		line, err := dbReadCommand(in)
		if err != nil || line == "cont\n" {
			return 0
		}
		/* C: luaL_loadbuffer || lua_pcall → luaL_tolstring on error.
		   LoadBytes returns a Go error (message not left on stack). */
		if lerr := L.LoadBytes([]byte(line), "=(debug command)"); lerr != nil {
			fmt.Fprintf(os.Stderr, "%s\n", lerr.Error())
		} else if L.PCall(0, 0, 0) != LUA_OK {
			fmt.Fprintf(os.Stderr, "%s\n", L.ToLString(-1))
		}
		L.SetTop(0) /* remove eventual returns */
	}
}

const (
	dbDebugPrompt = "lua_debug> "
	/* C's fgets reads into a 250-byte buffer, so a command longer
	   than 249 characters arrives in pieces and each piece is a
	   command of its own. */
	dbDebugLine = 250
)

// dbReadCommand is the 'fgets' of db_debug: at most dbDebugLine-1 bytes,
// stopping after the newline, which is part of what it returns.
func dbReadCommand(in *bufio.Reader) (string, error) {
	var b []byte
	for len(b) < dbDebugLine-1 {
		c, err := in.ReadByte()
		if err != nil {
			if len(b) == 0 {
				return "", err
			}
			break /* a last line with no newline in it */
		}
		b = append(b, c)
		if c == '\n' {
			break
		}
	}
	return string(b), nil
}

func dbTraceback(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	msg, ok := L.ToString(arg + 1)
	if ok || L.IsNoneOrNil(arg+1) {
		level := int64(1)
		if L != L1 {
			level = 0
		}
		L.Traceback(L1, msg, ok, int(L.OptInteger(arg+2, level)))
	} else {
		L.PushValue(arg + 1) /* return it untouched */
	}
	return 1
}

// FLAG: no mutable C-stack limit in this port; answer with the fixed
// LUAI_MAXCCALLS (same as cdlopen lua_setcstacklimit stub).
func dbSetCStackLimit(L *State) int {
	L.CheckInteger(1)
	L.PushInteger(int64(LUAI_MAXCCALLS))
	return 1
}

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

var dbFuncs = []Reg{
	{Name: "debug", Func: dbDebugFunc},
	{Name: "getuservalue", Func: dbGetUserValue},
	{Name: "gethook", Func: dbGetHook},
	{Name: "getinfo", Func: dbGetInfoFunc},
	{Name: "getlocal", Func: dbGetLocalFunc},
	{Name: "getregistry", Func: dbGetRegistry},
	{Name: "getmetatable", Func: dbGetMetaTable},
	{Name: "getupvalue", Func: dbGetUpValueFunc},
	{Name: "upvaluejoin", Func: dbUpValueJoin},
	{Name: "upvalueid", Func: dbUpValueIDFunc},
	{Name: "setuservalue", Func: dbSetUserValue},
	{Name: "sethook", Func: dbSetHook},
	{Name: "setlocal", Func: dbSetLocalFunc},
	{Name: "setmetatable", Func: dbSetMetaTable},
	{Name: "setupvalue", Func: dbSetUpValueFunc},
	{Name: "traceback", Func: dbTraceback},
	{Name: "setcstacklimit", Func: dbSetCStackLimit},
}

// OpenDebug opens the debug library (luaopen_debug).
func OpenDebug(L *State) int {
	L.NewLib(dbFuncs)
	return 1
}
