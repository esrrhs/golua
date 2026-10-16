// Translation of ldblib.c (PUC-Lua 5.4): the debug library.
//
// The C code calls the debug API of ldebug.c / lapi.c (lua_getstack,
// lua_getinfo, lua_getlocal, ...); this port has none of it, so the
// counterparts of those functions are translated here (dbGetStack,
// dbGetInfo, ...) on top of what the core already offers: currentLine,
// luaG_getfuncline, luaF_getlocalname, luaO_chunkid, upvalValue.
//
// Deviations are marked with "deviation" in the comments; the short
// list: no hook machinery (sethook records the hook but nothing ever
// calls it), no name/namewhat (the port does not track how a function
// was called), no transfer info (ftransfer/ntransfer are always 0), and
// no interactive 'debug.debug'.
package lua

import (
	"strings"
	"sync"
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

// dbFindLocal is luaG_findlocal: it returns the name and the stack slot
// of the local variable at index 'n' (1-based) of a frame. An empty name
// means "no such variable".
//
// deviation: the core's luaG_findlocal uses 'ci.savedpc' where C uses
// 'currentpc(ci)' (savedpc - 1); the lookup here follows C.
func dbFindLocal(L *State, ci *CallInfo, n int) (string, int) {
	base := ci.funcIdx + 1
	if isLua(ci) && n < 0 { /* access to vararg values? */
		cl := clLvalue(&L.stack[ci.funcIdx])
		if cl.p.isVararg != 0 && n >= -ci.nExtraArgs {
			return "(vararg)", ci.funcIdx - ci.nExtraArgs - (n + 1)
		}
		return "", 0 /* no such vararg */
	}
	var name string
	if isLua(ci) {
		cl := clLvalue(&L.stack[ci.funcIdx])
		name = luaF_getlocalname(cl.p, n, ci.savedpc-1)
	}
	if name == "" { /* no 'standard' name? */
		limit := L.top
		if ci != L.ci && ci.next != nil {
			limit = ci.next.funcIdx
		}
		if limit-base >= n && n > 0 { /* is 'n' inside 'ci' stack? */
			/* generic name for any valid slot */
			if isLua(ci) {
				return "(temporary)", base + (n - 1)
			}
			return "(C temporary)", base + (n - 1)
		}
		return "", 0 /* no name */
	}
	return name, base + (n - 1)
}

// dbGetLocal is lua_getlocal(L, ar, n): it pushes the value of the local
// variable and returns its name ("" when there is no such variable).
func dbGetLocal(L *State, ar *dbAr, n int) string {
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
	/* deviation: C stores the length of 'source' and calls
	   luaO_chunkid on it; the port's luaO_chunkid takes the whole
	   string, which is equivalent. */
	ar.shortSrc = luaO_chunkid(ar.source)
}

// dbCollectValidLines is collectvalidlines: it pushes a table with the
// active lines of a function (or nil for a non-Lua function).
//
// deviation: lineinfo already holds absolute lines, so there is no
// 'nextline' accumulation (and no 'abslineinfo').
func dbCollectValidLines(L *State, fv *Value) {
	if fv == nil || !ttisLclosure(fv) {
		L.PushNil()
		return
	}
	p := clLvalue(fv).p
	L.NewTable()
	i := 0
	if p.isVararg != 0 {
		i = 1 /* skip first instruction (OP_VARARGPREP) */
	}
	for ; i < len(p.lineinfo); i++ {
		line := luaG_getfuncline(p, i)
		if line > 0 {
			L.PushBoolean(true)
			L.SetI(-2, int64(line))
		}
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

// dbGetUpvalue is lua_getupvalue: it pushes the value of upvalue 'n' of
// the closure at 'idx' and returns its name.
func dbGetUpvalue(L *State, idx, n int) (string, bool) {
	fv := index2value(L, idx)
	switch {
	case ttisCclosure(fv):
		f := clCvalue(fv)
		if n < 1 || n > f.nupvalues {
			return "", false
		}
		dbPush(L, &f.upvalue[n-1])
		return "", true /* C closures have no upvalue names */
	case ttisLclosure(fv):
		f := clLvalue(fv)
		p := f.p
		if n < 1 || n > len(p.upvalues) {
			return "", false
		}
		name := "(no name)"
		if p.upvalues[n-1].name != nil {
			name = p.upvalues[n-1].name.val
		}
		dbPush(L, upvalValue(f.upvals[n-1]))
		return name, true
	}
	return "", false /* not a closure */
}

// dbSetUpvalue is lua_setupvalue: it pops a value from the stack and
// stores it in upvalue 'n' of the closure at 'idx'.
func dbSetUpvalue(L *State, idx, n int) (string, bool) {
	fv := index2value(L, idx)
	val := L.stack[L.top-1]
	switch {
	case ttisCclosure(fv):
		f := clCvalue(fv)
		if n < 1 || n > f.nupvalues {
			return "", false
		}
		f.upvalue[n-1] = val
		L.top--
		return "", true
	case ttisLclosure(fv):
		f := clLvalue(fv)
		p := f.p
		if n < 1 || n > len(p.upvalues) {
			return "", false
		}
		name := "(no name)"
		if p.upvalues[n-1].name != nil {
			name = p.upvalues[n-1].name.val
		}
		*upvalValue(f.upvals[n-1]) = val
		L.top--
		return name, true
	}
	return "", false /* not a closure */
}

// dbUpvalueID is lua_upvalueid: it returns an identity for an upvalue
// (a light userdata in C, any pointer here).
func dbUpvalueID(L *State, idx, n int) any {
	fv := index2value(L, idx)
	switch {
	case ttisCclosure(fv):
		f := clCvalue(fv)
		if n < 1 || n > f.nupvalues {
			return nil
		}
		return &f.upvalue[n-1]
	case ttisLclosure(fv):
		f := clLvalue(fv)
		if n < 1 || n > f.nupvalues {
			return nil
		}
		return f.upvals[n-1]
	}
	return nil
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
		L.PushNil() /* not a userdata */
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
		L.PushNil() /* deviation: C pops the value and pushes fail */
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
			L.PushNil() /* level out of range */
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
		/* information about a non-active function: the name of a
		   parameter, as it is at the start of the function */
		name := ""
		if ttisLclosure(&L.stack[L.top-1]) {
			cl := clLvalue(&L.stack[L.top-1])
			name = luaF_getlocalname(cl.p, nvar, 0)
		}
		/* 'lua_pushstring(L, NULL)' pushes nil, so an out-of-range index
		   yields nil here -- not an empty string. */
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
	L.PushNil() /* no name (nor value) */
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
func dbCheckUpVal(L *State, argf, argnup int, pnup *int) any {
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
		L.PushNil()
	}
	return 1
}

func dbUpValueJoin(L *State) int {
	var n1, n2 int
	dbCheckUpVal(L, 1, 2, &n1)
	dbCheckUpVal(L, 3, 4, &n2)
	L.ArgCheck(!L.IsGoFunction(1), 1, "Lua function expected")
	L.ArgCheck(!L.IsGoFunction(3), 3, "Lua function expected")
	f1 := clLvalue(index2value(L, 1))
	f2 := clLvalue(index2value(L, 3))
	f1.upvals[n1-1] = f2.upvals[n2-1]
	return 0
}

// ---------------------------------------------------------------------
// hooks
// ---------------------------------------------------------------------

// dbHookInfo remembers the mask and the count given to 'sethook'.
//
// The mask and the count also live on the state itself (as in C); this
// table is only used to answer 'gethook' for a thread whose state is not
// reachable from the hook function.
type dbHookInfo struct {
	mask  int
	count int
}

var dbHooks = struct {
	sync.Mutex
	m map[*State]dbHookInfo
}{m: make(map[*State]dbHookInfo)}

// dbHookNames are the event names passed to the hook function (ldblib.c).
var dbHookNames = []string{"call", "return", "line", "count", "tail call"}

// dbHookFunc is the hookf of ldblib.c: it calls the Lua function stored
// in the hook table for the running thread.
func dbHookFunc(L *State, ar *dbAr) {
	L.GetField(LUA_REGISTRYINDEX, dbHookKey) /* hook table */
	L.PushThread()
	L.RawGet(-2) /* get hooktable[thread] */
	L.Remove(-2) /* remove hook table */
	if !L.IsFunction(-1) {
		L.Pop(1) /* no hook function */
		return
	}
	L.PushString(dbHookNames[ar.event]) /* hook event */
	if ar.currentline >= 0 {
		L.PushInteger(int64(ar.currentline)) /* current line */
	} else {
		L.PushNil()
	}
	L.PCall(2, 0, 0) /* call hook function */
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
		smask string
		count int
	)
	if L.IsNoneOrNil(arg + 1) { /* no hook? */
		L.SetTop(arg + 1)
	} else {
		smask = L.CheckString(arg + 2)
		L.CheckType(arg+1, LUA_TFUNCTION)
		count = int(L.OptInteger(arg+3, 0))
		hook = dbHookFunc
	}
	mask := dbMakeMask(smask, count)
	L1.setHook(hook, mask, count)
	dbHooks.Lock()
	if hook == nil {
		delete(dbHooks.m, L1)
	} else {
		dbHooks.m[L1] = dbHookInfo{mask: mask, count: count}
	}
	dbHooks.Unlock()
	dbCreateHookTable(L)
	dbCheckStack(L, L1, 1)
	L1.PushThread()      /* key (thread) */
	L.XMove(L1, 1)       /* move it to the stack of L */
	L.PushValue(arg + 1) /* value (hook function) */
	L.RawSet(-3)         /* hooktable[L1] = new Lua hook */
	return 0
}

func dbGetHook(L *State) int {
	arg := 0
	L1 := dbGetThread(L, &arg)
	dbHooks.Lock()
	info, ok := dbHooks.m[L1]
	dbHooks.Unlock()
	if !ok { /* no hook? */
		L.PushNil()
		return 1
	}
	/* hook table must exist */
	L.GetField(LUA_REGISTRYINDEX, dbHookKey)
	dbCheckStack(L, L1, 1)
	L1.PushThread()
	L.XMove(L1, 1)
	L.RawGet(-2)                          /* 1st result = hooktable[L1] */
	L.Remove(-2)                          /* remove hook table */
	L.PushString(dbUnmakeMask(info.mask)) /* 2nd result = mask */
	L.PushInteger(int64(info.count))      /* 3rd result = count */
	return 3
}

// deviation: C's 'debug.debug' reads commands from stdin in a loop;
// there is no interactive mode here, so it returns immediately.
func dbDebugFunc(L *State) int {
	return 0
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

// deviation: there is no C stack limit to change; the current limit is
// reported back, as C does for a limit it cannot honour.
func dbSetCStackLimit(L *State) int {
	L.CheckInteger(1)
	L.PushInteger(LUAI_MAXCCALLS)
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
