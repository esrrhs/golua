//go:build cgo && unix

// The rest of the C API: coroutines, hooks, the debug API, userdata
// values, lua_dump, and the pieces of the auxiliary library that an
// ordinary extension reaches for -- metatables and type names, options,
// loading a file, and the two 'result' helpers.
//
// cdlopen.go carries the core (stack, types, conversion, push, get/set,
// call, and the rest of luaL_*); this file carries what an extension
// needs less often but still resolves at load time, because the loader
// opens with RTLD_NOW and a name it cannot find keeps the whole
// library from loading.
//
// The four variadic functions are not here or in cdlopen.go -- neither
// direction of the cgo boundary can express '...' -- but in
// cdlbridge.c, which reaches back through the callbacks at the bottom
// of this file.

package lua

/*
#include "cdlbridge.h"
*/
import "C"

import (
	"runtime/cgo"
	"sync"
	"unsafe"
)

// The lua_gc options and the version number, which PUC-Lua defines in
// lua.h.  There is no lua.h on the Go side, so they live here next to
// the one function that switches on them.
const (
	luaGCStop       = 0
	luaGCRestart    = 1
	luaGCCollect    = 2
	luaGCCount      = 3
	luaGCCountB     = 4
	luaGCStep       = 5
	luaGCSetPause   = 6
	luaGCSetStepMul = 7
	luaGCIsRunning  = 9
	luaGCGen        = 10
	luaGCInc        = 11

	luaVersionNum = 504
)

// ---------------------------------------------------------------------
// states and threads
// ---------------------------------------------------------------------

//export lua_close
func lua_close(p *C.lua_State) { stateOf(p).Close() }

//export lua_newthread
func lua_newthread(p *C.lua_State) *C.lua_State { return stateOf(p).NewThread().statePtr() }

//export lua_resetthread
func lua_resetthread(p *C.lua_State) C.int { return C.int(stateOf(p).ResetThread()) }

//export lua_closethread
func lua_closethread(p *C.lua_State, from *C.lua_State) C.int {
	L := stateOf(p)
	var f *State
	if from != nil {
		f = stateOf(from)
	}
	/* lua_closethread closes the pending to-be-closed slots of 'L' and
	   answers whether it is done; a thread with nothing pending, or one
	   that is not suspended, is done. */
	if L.Status() != LUA_YIELD {
		return C.int(L.ResetThread())
	}
	_ = f
	return 0
}

//export lua_atpanic
func lua_atpanic(p *C.lua_State, f C.lua_CFunction) C.lua_CFunction {
	L := stateOf(p)
	/* C answers the previous panic function, and an extension installs
	   its own precisely so that it can put the host's back when it is
	   done. The state holds a Go value, which C cannot name, so the
	   pointer each state's handler came from is remembered here; when
	   the handler did not come from C the answer is NULL, which is also
	   what a state that never had one answers. */
	panicMu.Lock()
	prev := panicCPtr[L.g]
	if unsafe.Pointer(f) != nil {
		panicCPtr[L.g] = unsafe.Pointer(f)
	} else {
		delete(panicCPtr, L.g)
	}
	panicMu.Unlock()
	L.AtPanic(cFunc(unsafe.Pointer(f)))
	return C.lua_CFunction(prev)
}

var (
	panicMu   sync.Mutex
	panicCPtr = map[*globalState]unsafe.Pointer{}
)

//export lua_version
func lua_version(p *C.lua_State) C.lua_Number {
	/* the version is a constant in PUC-Lua too; the state is not asked */
	return C.lua_Number(luaVersionNum)
}

//export lua_newstate
func lua_newstate(f C.lua_Alloc, ud unsafe.Pointer) *C.lua_State {
	/* an extension that builds its own state is asking for a second
	   interpreter inside this one; there is nothing sensible to answer */
	return nil
}

//export luaL_newstate
func luaL_newstate() *C.lua_State { return nil }

// ---------------------------------------------------------------------
// stack, access, get and set
// ---------------------------------------------------------------------

//export lua_tocfunction
func lua_tocfunction(p *C.lua_State, idx C.int) C.lua_CFunction {
	/* a Go function has no address to hand back, so C is told there is
	   none; the value is still callable from Lua */
	if stateOf(p).ToGoFunction(idxInt(idx)) == nil {
		return nil
	}
	return nil
}

//export lua_tothread
func lua_tothread(p *C.lua_State, idx C.int) *C.lua_State {
	L := stateOf(p).ToThread(idxInt(idx))
	if L == nil {
		return nil
	}
	return L.statePtr()
}

//export lua_stringtonumber
func lua_stringtonumber(p *C.lua_State, s *C.char) C.size_t {
	L := stateOf(p)
	src := C.GoString(s)
	if !strStringToNumber(L, src) {
		return 0 /* not a number: nothing pushed, as in C */
	}
	/* C answers one more than the characters it used, because 0 means
	   "not a number" and a numeral is never empty.  This port's parser
	   takes the whole string or nothing, so every character was used. */
	return C.size_t(len(src) + 1)
}

//export lua_rawgetp
func lua_rawgetp(p *C.lua_State, idx C.int, k unsafe.Pointer) C.int {
	return C.int(stateOf(p).RawGetP(idxInt(idx), ptrKey(k)))
}

//export lua_rawsetp
func lua_rawsetp(p *C.lua_State, idx C.int, k unsafe.Pointer) {
	stateOf(p).RawSetP(idxInt(idx), ptrKey(k))
}

//export lua_getiuservalue
func lua_getiuservalue(p *C.lua_State, idx, n C.int) C.int {
	return C.int(stateOf(p).GetUserValue(idxInt(idx), int(n)))
}

//export lua_setiuservalue
func lua_setiuservalue(p *C.lua_State, idx, n C.int) C.int {
	return C.int(stateOf(p).SetUserValue(idxInt(idx), int(n)))
}

//export lua_pushthread
func lua_pushthread(p *C.lua_State) C.int { return cBool(stateOf(p).PushThread()) }

// ptrKey answers the value a C pointer is used as a table key: the
// pointer itself, so that the same address is the same key.
func ptrKey(k unsafe.Pointer) any {
	if k == nil {
		return nil
	}
	return k
}

// ---------------------------------------------------------------------
// calls, coroutines, dumps
// ---------------------------------------------------------------------

//export lua_callk
func lua_callk(p *C.lua_State, nargs, nresults C.int, ctx C.lua_KContext, k C.lua_KFunction) {
	L := stateOf(p)
	/* C's lua_callk: a continuation is only possible in a thread that
	   can yield, and asking for one is what makes the call yieldable.
	   It is saved on the caller's own frame, because that is the frame
	   the continuation belongs to. */
	if k != nil && L.IsYieldable() {
		ci := L.ci
		ci.cK = unsafe.Pointer(k)
		ci.cCtx = int64(ctx)
		L.CallK(int(nargs), int(nresults))
		/* The call returned, so it did not yield: the C function goes
		   on from here itself, and no continuation is due.  (C leaves
		   'ci->u.c.k' set; this port must not, because the frame stays
		   alive and a later yield would find a stale one.) */
		ci.cK = nil
		return
	}
	L.Call(int(nargs), int(nresults))
}

//export lua_pcallk
func lua_pcallk(p *C.lua_State, nargs, nresults, msgh C.int, ctx C.lua_KContext,
	k C.lua_KFunction) C.int {
	L := stateOf(p)
	if k != nil && L.IsYieldable() {
		/* C's lua_pcallk with a continuation: the call is already
		   protected by 'resume', so it is made yieldable and left
		   unprotected -- an error is recovered right here, which is
		   what ldo.c's 'findpcall' + 'finishpcallk' do for the same
		   case.  C never comes back from the yield; here it does, and
		   the C function calls the continuation itself with the status
		   this returns. */
		ci := L.ci
		funcIdx := L.top - (int(nargs) + 1)
		var ef int
		if msgh != 0 {
			ef = L.indexToAbs(int(msgh))
		}
		ci.cK = unsafe.Pointer(k)
		ci.cCtx = int64(ctx)
		ci.oldErrFunc = L.errfunc
		L.errfunc = ef
		ci.ypcall = true
		status := LUA_OK
		n0 := L.nYields
		func() {
			defer func() {
				r := recover()
				if r == nil {
					return
				}
				e, ok := r.(*luaError)
				if !ok {
					panic(r) /* not a Lua error: let it out */
				}
				status = e.status
			}()
			luaD_call(L, funcIdx, int(nresults))
		}()
		ci.ypcall = false
		L.errfunc = ci.oldErrFunc
		if status == LUA_OK && L.nYields != n0 {
			/* C's finishpcallk: the call was interrupted by a yield,
			   and that is the status the continuation hears about --
			   even though the callee then went on to finish normally. */
			status = LUA_YIELD
		}
		if status != LUA_OK && status != LUA_YIELD {
			/* ldo.c's finishpcallk: unwind to this frame, close the
			   to-be-closed variables the call opened, and leave the
			   error object where the callee's results would have been,
			   so that the continuation sees it. */
			L.ci = ci
			status = luaD_closeprotected(L, funcIdx, status)
			luaD_seterrorobj(L, status, funcIdx)
			L.shrinkStack()
		} else {
			ci.cK = nil /* the call did not yield */
		}
		return C.int(status)
	}
	return C.int(L.PCall(int(nargs), int(nresults), int(msgh)))
}

//export lua_yieldk
func lua_yieldk(p *C.lua_State, nresults C.int, ctx C.lua_KContext, k C.lua_KFunction) C.int {
	L := stateOf(p)
	/* C's lua_yieldk: 'k' is saved on the frame that is yielding, so
	   that the resume calls it instead of handing the resume arguments
	   back to the C function. */
	if k != nil {
		L.ci.cK = unsafe.Pointer(k)
		L.ci.cCtx = int64(ctx)
	} else {
		L.ci.cK = nil
	}
	/* When there is no continuation, this is the number of values the
	   resumer passed to 'coroutine.resume' and they are already on the
	   stack: the results of the yield.  With one, it is what the
	   continuation left there. */
	return C.int(L.Yield(int(nresults)))
}

//export lua_yield
func lua_yield(p *C.lua_State, nresults C.int) C.int {
	return C.int(stateOf(p).Yield(int(nresults)))
}

func init() {
	/* The only place a C function pointer can be called from. */
	callContinuation = func(L *State, ci *CallInfo, status int) int {
		k := C.lua_KFunction(ci.cK)
		return int(C.cdl_call_kfunction(k, L.statePtr(), C.int(status),
			C.lua_KContext(ci.cCtx)))
	}
}

//export lua_resume
func lua_resume(p *C.lua_State, from *C.lua_State, nargs C.int, nres *C.int) C.int {
	L := stateOf(p)
	var f *State
	if from != nil {
		f = stateOf(from)
	}
	stat := L.Resume(f, int(nargs))
	n := len(L.co.vals)
	if nres != nil {
		*nres = C.int(n)
	}
	if stat == LUA_YIELD && n > 0 {
		/* C's lua_resume leaves the yielded values on the coroutine's
		   own stack and reports how many there are; the caller moves
		   them across with lua_xmove when it wants them, which is what
		   auxresume does. Taking them off in (*State).Yield is what
		   keeps a continuation's lua_gettop honest, so they go back on
		   here for whoever asked through the API. */
		L.checkStack(n)
		copy(L.stack[L.top:], L.co.vals)
		L.top += n
	}
	return C.int(stat)
}

//export lua_status
func lua_status(p *C.lua_State) C.int {
	L := stateOf(p)
	/* A thread that has never been resumed is LUA_YIELD here, which is
	   what makes coroutine.status call it "suspended" -- the same thing
	   PUC-Lua calls it -- but C's lua_status answers LUA_OK for it, and
	   so does lua_resetthread. An extension that asks can tell a new
	   thread from a suspended one, so it has to be able to. */
	if L.status == LUA_YIELD && !L.started {
		return C.int(LUA_OK)
	}
	return C.int(L.Status())
}

//export lua_isyieldable
func lua_isyieldable(p *C.lua_State) C.int { return cBool(stateOf(p).IsYieldable()) }

//export lua_xmove
func lua_xmove(from, to *C.lua_State, n C.int) {
	stateOf(to).XMove(stateOf(from), int(n))
}

//export lua_dump
func lua_dump(p *C.lua_State, writer C.lua_Writer, data unsafe.Pointer, strip C.int) C.int {
	L := stateOf(p)
	if writer == nil {
		return 1 /* nowhere to write: as C, which would crash instead */
	}
	status := luaDump(L, func(_ *State, b []byte) int {
		if len(b) == 0 {
			return 0
		}
		cbytes := C.CBytes(b)
		defer C.free(cbytes)
		return int(C.cdl_call_writer(writer, p, cbytes, C.size_t(len(b)), data))
	}, strip != 0)
	/* C pops nothing; the function stays on the stack */
	return C.int(status)
}

// ---------------------------------------------------------------------
// warnings
// ---------------------------------------------------------------------

//export lua_warning
func lua_warning(p *C.lua_State, msg *C.char, tocont C.int) {
	stateOf(p).Warning(C.GoString(msg), tocont != 0)
}

//export lua_setwarnf
func lua_setwarnf(p *C.lua_State, f C.lua_WarnFunction, ud unsafe.Pointer) {
	L := stateOf(p)
	if f == nil {
		L.SetWarnFunction(nil, nil)
		return
	}
	L.SetWarnFunction(func(L *State, msg string, tocont bool) {
		cs := C.CString(msg)
		defer C.free(unsafe.Pointer(cs))
		C.cdl_call_warnf(f, cs, cBool(tocont), nil)
	}, nil)
}

// ---------------------------------------------------------------------
// hooks and the debug API
// ---------------------------------------------------------------------

//export lua_sethook
func lua_sethook(p *C.lua_State, f C.lua_Hook, mask, count C.int) {
	L := stateOf(p)
	if f == nil {
		L.SetHook(nil, int(mask), int(count))
		return
	}
	L.SetHook(func(L *State, ar *dbAr) {
		/* the hook is called with a debug record C can read: ours is
		   filled in from the activation record and copied out */
		var cAr C.lua_Debug
		fillDebug(&cAr, ar)
		C.cdl_call_hook(f, L.statePtr(), &cAr)
	}, int(mask), int(count))
}

//export lua_gethook
func lua_gethook(p *C.lua_State) C.lua_Hook { return nil }

//export lua_gethookmask
func lua_gethookmask(p *C.lua_State) C.int { return C.int(stateOf(p).GetHookMask()) }

//export lua_gethookcount
func lua_gethookcount(p *C.lua_State) C.int { return C.int(stateOf(p).GetHookCount()) }

// cgoArs ties a C lua_Debug to the port's own activation record. C
// hands the same pointer to lua_getstack and then to lua_getinfo and
// lua_getlocal, so the address of the record is the key.
var (
	cgoArMu sync.Mutex
	cgoArs  = map[uintptr]*dbAr{}
)

func putAr(c *C.lua_Debug, ar *dbAr) {
	cgoArMu.Lock()
	cgoArs[uintptr(unsafe.Pointer(c))] = ar
	cgoArMu.Unlock()
}

func getAr(c *C.lua_Debug) *dbAr {
	if c == nil {
		return nil
	}
	cgoArMu.Lock()
	ar := cgoArs[uintptr(unsafe.Pointer(c))]
	cgoArMu.Unlock()
	return ar
}

// fillDebug copies the port's activation record into the fields of a C
// lua_Debug, which is what an extension reads.
func fillDebug(c *C.lua_Debug, ar *dbAr) {
	if c == nil || ar == nil {
		return
	}
	c.event = C.int(ar.event)
	c.name = keepCStr(ar.name)
	c.namewhat = keepCStr(ar.namewhat)
	c.what = keepCStr(ar.what)
	c.source = keepCStr(ar.source)
	c.srclen = C.size_t(len(ar.source))
	c.currentline = C.int(ar.currentline)
	c.linedefined = C.int(ar.linedefined)
	c.lastlinedefined = C.int(ar.lastlinedefined)
	c.nups = C.uchar(ar.nups)
	c.nparams = C.uchar(ar.nparams)
	c.isvararg = boolChar(ar.isvararg)
	c.istailcall = boolChar(ar.istailcall)
	c.ftransfer = C.ushort(ar.ftransfer)
	c.ntransfer = C.ushort(ar.ntransfer)
	copyCStr(&c.short_src[0], ar.shortSrc, CDL_IDSIZE)
	c.i_ci = ptrOf(uintptr(cgo.NewHandle(ar)))
}

// keepCStr answers a NUL-terminated copy of s that outlives this call.
// The C record holds the pointer, and C is free to read it later, so it
// cannot be freed here; the copies are kept for the life of the process.
func keepCStr(s string) *C.char {
	if s == "" {
		return nil
	}
	return C.CString(s)
}

func boolChar(b bool) C.char {
	if b {
		return 1
	}
	return 0
}

func copyCStr(dst *C.char, s string, size int) {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.cdl_copy_str(dst, cs, C.size_t(len(s)), C.size_t(size))
}

//export lua_getstack
func lua_getstack(p *C.lua_State, level C.int, ar *C.lua_Debug) C.int {
	if ar == nil {
		return 0
	}
	var db dbAr
	if !dbGetStack(stateOf(p), int(level), &db) {
		return 0
	}
	putAr(ar, &db)
	fillDebug(ar, &db)
	return 1
}

//export lua_getinfo
func lua_getinfo(p *C.lua_State, what *C.char, ar *C.lua_Debug) C.int {
	if ar == nil {
		return 0
	}
	db := getAr(ar)
	if db == nil {
		return 0
	}
	if !dbGetInfo(stateOf(p), C.GoString(what), db) {
		return 0
	}
	fillDebug(ar, db)
	return 1
}

//export lua_getlocal
func lua_getlocal(p *C.lua_State, ar *C.lua_Debug, n C.int) *C.char {
	db := getAr(ar)
	if db == nil {
		return nil
	}
	/* dbGetLocal pushes the value and answers its name */
	name := dbGetLocal(stateOf(p), db, int(n))
	return keepCStr(name)
}

//export lua_setlocal
func lua_setlocal(p *C.lua_State, ar *C.lua_Debug, n C.int) *C.char {
	db := getAr(ar)
	if db == nil {
		return nil
	}
	return keepCStr(dbSetLocal(stateOf(p), db, int(n)))
}

//export lua_upvalueid
func lua_upvalueid(p *C.lua_State, fidx, n C.int) unsafe.Pointer {
	L := stateOf(p)
	v := dbUpvalueID(L, idxInt(fidx), int(n))
	if v == nil {
		return nil
	}
	return ptrOf(ptrID(v))
}

//export lua_upvaluejoin
func lua_upvaluejoin(p *C.lua_State, fidx1, n1, fidx2, n2 C.int) {
	L := stateOf(p)
	dbUpValueJoin2(L, idxInt(fidx1), int(n1), idxInt(fidx2), int(n2))
}

// ---------------------------------------------------------------------
// the rest of the auxiliary library
// ---------------------------------------------------------------------

//export luaL_checkoption
func luaL_checkoption(p *C.lua_State, arg C.int, def *C.char, lst **C.char) C.int {
	var list []string
	if lst != nil {
		/* the list is NULL-terminated, as in C */
		for i := 0; ; i++ {
			entry := C.cdl_lst_at(lst, C.int(i))
			if entry == nil || *entry == nil {
				break
			}
			list = append(list, C.GoString(*entry))
		}
	}
	d := ""
	if def != nil {
		d = C.GoString(def)
	}
	return C.int(stateOf(p).CheckOption(int(arg), d, list))
}

//export luaL_checkudata
func luaL_checkudata(p *C.lua_State, arg C.int, tname *C.char) unsafe.Pointer {
	return cdlTestUserData(stateOf(p), int(arg), C.GoString(tname), true)
}

//export luaL_testudata
func luaL_testudata(p *C.lua_State, arg C.int, tname *C.char) unsafe.Pointer {
	return cdlTestUserData(stateOf(p), int(arg), C.GoString(tname), false)
}

//export luaL_newmetatable
func luaL_newmetatable(p *C.lua_State, tname *C.char) C.int {
	L := stateOf(p)
	name := C.GoString(tname)
	if L.GetField(LUA_REGISTRYINDEX, name) != LUA_TNIL {
		return 0 /* name already in use */
	}
	L.Pop(1)
	L.CreateTable(0, 2)
	L.PushString(name)
	L.SetField(-2, "__name")
	L.PushValue(-1)
	L.SetField(LUA_REGISTRYINDEX, name)
	return 1
}

//export luaL_setmetatable
func luaL_setmetatable(p *C.lua_State, tname *C.char) {
	L := stateOf(p)
	L.GetField(LUA_REGISTRYINDEX, C.GoString(tname))
	L.SetMetaTable(-2)
}

//export luaL_getmetatable
func luaL_getmetatable(p *C.lua_State, tname *C.char) {
	stateOf(p).GetField(LUA_REGISTRYINDEX, C.GoString(tname))
}

//export luaL_loadfilex
func luaL_loadfilex(p *C.lua_State, filename, mode *C.char) C.int {
	L := stateOf(p)
	m := ""
	if mode != nil {
		m = C.GoString(mode)
	}
	err := L.LoadFileMode(C.GoString(filename), m)
	return C.int(cdlStatus(err))
}

//export luaL_loadfile
func luaL_loadfile(p *C.lua_State, filename *C.char) C.int {
	return luaL_loadfilex(p, filename, nil)
}

//export luaL_fileresult
func luaL_fileresult(p *C.lua_State, stat C.int, fname *C.char) C.int {
	L := stateOf(p)
	if stat != 0 {
		L.PushBoolean(true)
		return 1
	}
	/* errno is not carried across the boundary, so the message is what
	   the port's file layer said, or 'fname: unknown error' */
	L.PushNil()
	L.PushString(C.GoString(fname) + ": " + "unknown error")
	L.PushInteger(0)
	return 3
}

//export luaL_execresult
func luaL_execresult(p *C.lua_State, stat C.int) C.int {
	L := stateOf(p)
	/* there is no waitpid status to decode, so the answer is the
	   outcome of 'io.close' on a popen handle: true, or nil plus why */
	L.PushBoolean(stat != 0)
	return 1
}

//export luaL_gsub
func luaL_gsub(p *C.lua_State, s, patt, repl *C.char) *C.char {
	L := stateOf(p)
	L.GetGlobal("string")
	L.GetField(-1, "gsub")
	L.PushString(C.GoString(s))
	L.PushString(C.GoString(patt))
	L.PushString(C.GoString(repl))
	L.Call(3, 1)
	/* PUC-Lua's luaL_gsub leaves the result on the stack -- it is
	   luaL_addgsub into a buffer followed by luaL_pushresult -- and
	   answers a pointer into it. */
	L.Remove(-2) /* the 'string' table is under the result */
	return lua_tolstring(p, -1, nil)
}

//export luaL_openlibs
func luaL_openlibs(p *C.lua_State) { stateOf(p).OpenLibs() }

// The luaopen_* of the standard libraries.  They belong to the exported
// surface for the same reason luaL_openlibs does: an extension that is
// linked into the host instead of dlopen'd, or a host that opens one
// library by hand with luaL_requiref, reaches them by name.  Each one
// is the Go opener of the same library, which is what OpenLibs calls.
//
//export luaopen_base
func luaopen_base(p *C.lua_State) C.int { return C.int(OpenBase(stateOf(p))) }

//export luaopen_coroutine
func luaopen_coroutine(p *C.lua_State) C.int { return C.int(OpenCoroutine(stateOf(p))) }

//export luaopen_table
func luaopen_table(p *C.lua_State) C.int { return C.int(OpenTable(stateOf(p))) }

//export luaopen_io
func luaopen_io(p *C.lua_State) C.int { return C.int(OpenIO(stateOf(p))) }

//export luaopen_os
func luaopen_os(p *C.lua_State) C.int { return C.int(OpenOS(stateOf(p))) }

//export luaopen_string
func luaopen_string(p *C.lua_State) C.int { return C.int(OpenString(stateOf(p))) }

//export luaopen_utf8
func luaopen_utf8(p *C.lua_State) C.int { return C.int(OpenUTF8(stateOf(p))) }

//export luaopen_math
func luaopen_math(p *C.lua_State) C.int { return C.int(OpenMath(stateOf(p))) }

//export luaopen_debug
func luaopen_debug(p *C.lua_State) C.int { return C.int(OpenDebug(stateOf(p))) }

//export luaopen_package
func luaopen_package(p *C.lua_State) C.int { return C.int(OpenPackage(stateOf(p))) }

// ---------------------------------------------------------------------
// the callbacks cdlbridge.c uses: the four variadic functions, once
// their arguments have been collected
// ---------------------------------------------------------------------

//export cdl_pushlstring_cb
func cdl_pushlstring_cb(p *C.lua_State, s *C.char, n C.size_t) {
	stateOf(p).PushBytes([]byte(C.GoStringN(s, C.int(n))))
}

//export cdl_error_cb
func cdl_error_cb(p *C.lua_State, s *C.char) C.int {
	L := stateOf(p)
	/* luaL_error is 'luaL_where(1)' plus the message, concatenated */
	L.Where2(1)
	L.PushString(C.GoString(s))
	L.Concat(2)
	L.Error()
	return 0
}

//export cdl_runerror_cb
func cdl_runerror_cb(p *C.lua_State, s *C.char) C.int {
	/* luaG_runerror for the "invalid option" of lua_pushfstring */
	stateOf(p).Error2("%s", C.GoString(s))
	return 0
}

//export cdl_gc_cb
func cdl_gc_cb(p *C.lua_State, what, a1, a2, a3 C.int) C.int {
	L := stateOf(p)
	switch what {
	case luaGCStop:
		L.g.gcStopped = true
		return 0
	case luaGCRestart:
		L.g.gcStopped = false
		return 0
	case luaGCCollect:
		L.fullCollect()
		return 0
	case luaGCCount:
		return C.int(gcCount())
	case luaGCCountB:
		/* the fractional part of "count", in bytes */
		return C.int(uint64(gcCount()*1024) % 1024)
	case luaGCStep:
		L.gcCycle()
		return 1 /* a step here is a whole cycle, so it finished */
	case luaGCSetPause:
		prev := L.g.gcPause
		if a1 != 0 {
			L.g.gcPause = int(a1)
		}
		return C.int(prev)
	case luaGCSetStepMul:
		prev := L.g.gcStepMul
		if a1 != 0 {
			L.g.gcStepMul = int(a1)
		}
		return C.int(prev)
	case luaGCIsRunning:
		return cBool(!L.g.gcStopped)
	case luaGCGen:
		/* C answers the previous mode; 0 is "incremental", 1 "generational" */
		prev := cBool(L.g.gcMode != gcModeGen)
		L.g.gcMode = gcModeGen
		return prev
	case luaGCInc:
		prev := cBool(L.g.gcMode == gcModeGen)
		L.g.gcMode = gcModeInc
		return prev
	}
	return 0
}

// ---------------------------------------------------------------------
// the rest of lua.h: allocator, upvalues, to-be-closed slots
// ---------------------------------------------------------------------

//export lua_getallocf
func lua_getallocf(p *C.lua_State, ud *unsafe.Pointer) C.lua_Alloc {
	L := stateOf(p)
	if ud != nil {
		*ud = L.callocUD
	}
	if L.callocF != nil { /* one set by lua_setallocf */
		return C.lua_Alloc(L.callocF)
	}
	/* The state itself has no allocator -- the Go heap is it -- but one
	   has to be handed out all the same, because an extension calls the
	   pointer it gets: lpeg compiles a pattern through it. cdl_alloc is
	   PUC-Lua's own l_alloc, written in C so that it can be called. */
	return C.lua_Alloc(C.cdl_allocfn())
}

//export lua_setallocf
func lua_setallocf(p *C.lua_State, f C.lua_Alloc, ud unsafe.Pointer) {
	L := stateOf(p)
	L.callocF = unsafe.Pointer(f)
	L.callocUD = ud
}

//export lua_setcstacklimit
func lua_setcstacklimit(p *C.lua_State, limit C.uint) C.int {
	/* the C stack is the Go stack here, and its limit is not ours to
	   set; PUC-Lua answers the previous limit */
	return C.int(LUAI_MAXCCALLS)
}

//export lua_toclose
func lua_toclose(p *C.lua_State, idx C.int) { stateOf(p).CloseSlotAt(idxInt(idx)) }

//export lua_getupvalue
func lua_getupvalue(p *C.lua_State, funcindex, n C.int) *C.char {
	name, ok := dbGetUpvalue(stateOf(p), idxInt(funcindex), int(n))
	if !ok {
		return nil
	}
	/* C answers "" for a C closure, which has no upvalue names */
	return keepCStr(name)
}

//export lua_setupvalue
func lua_setupvalue(p *C.lua_State, funcindex, n C.int) *C.char {
	name, ok := dbSetUpvalue(stateOf(p), idxInt(funcindex), int(n))
	if !ok {
		return nil
	}
	return keepCStr(name)
}

// ---------------------------------------------------------------------
// the rest of lauxlib.h: luaL_addgsub and luaL_buffinitsize
// ---------------------------------------------------------------------

//export luaL_addgsub
func luaL_addgsub(b *C.luaL_Buffer, s, patt, repl *C.char) {
	buf := bufOf(b)
	if buf == nil {
		return
	}
	/* lauxlib.c does this with 'string.gsub' on the buffer's own state,
	   which is the one the buffer was made with */
	L := stateOf(C.cdl_buf_L(b))
	L.GetGlobal("string")
	L.GetField(-1, "gsub")
	L.PushString(C.GoString(s))
	L.PushString(C.GoString(patt))
	L.PushString(C.GoString(repl))
	L.Call(3, 1)
	L.Remove(-2) /* the 'string' table */
	bufEnter(b).AddValue()
	bufLeave(b, buf)
}

//export luaL_buffinitsize
func luaL_buffinitsize(p *C.lua_State, b *C.luaL_Buffer, sz C.size_t) *C.char {
	luaL_buffinit(p, b)
	if b == nil {
		return nil
	}
	return luaL_prepbuffsize(b, sz) /* the block to copy into */
}
