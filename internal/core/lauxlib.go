// Translation of lauxlib.h / lauxlib.c (PUC-Lua 5.3.4).
package core

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"
)

// Reg is the Go counterpart of luaL_Reg.
type Reg struct {
	Name string
	Func GoFunc
}

// ArgError raises an error for a bad argument (luaL_argerror,
// lauxlib.c:176).
//
// The function name in "bad argument #%d to '%s'" comes from
// 'lua_getinfo(L, "n", &ar)' -- which digs the name out of the calling
// instruction -- and, when that fails, from 'pushglobalfuncname', which
// searches 'package.loaded' for a name. Both paths matter: the standard
// library is only reachable through the second one.
func (L *State) ArgError(arg int, extramsg string) int {
	var ar dbAr
	if !dbGetStack(L, 0, &ar) { /* no stack frame? */
		return L.Error2("bad argument #%d (%s)", arg, extramsg)
	}
	dbGetInfo(L, "n", &ar)
	if ar.namewhat == "method" {
		arg-- /* do not count 'self' */
		if arg == 0 {
			return L.Error2("calling '%s' on bad self (%s)", ar.name, extramsg)
		}
	}
	name := ar.name
	if name == "" {
		/* ar.name == NULL: try a global name for the function */
		if fn, ok := L.globalFuncName(&L.stack[ar.ci.funcIdx]); ok {
			name = fn
		} else {
			name = "?"
		}
	}
	return L.Error2("bad argument #%d to '%s' (%s)", arg, name, extramsg)
}

// Error2 raises an error with a formatted message (luaL_error).
//
// C is not 'luaG_runerror' here: luaL_error first pushes
// 'luaL_where(L, 1)' -- the position of the *caller* of the C function,
// hence level 1 -- then the message, concatenates them and raises with
// 'lua_error'. Going through 'luaG_runerror' instead loses the position
// entirely, because the current frame is the C function itself and
// 'isLua(ci)' is false for it.
func (L *State) Error2(format string, args ...any) int {
	L.Where2(1)
	L.PushString(pushfString(L, format, args...).val)
	L.Concat(2)
	return L.Error()
}

// luaLNumSizes is LUAL_NUMSIZES for this port: sizeof(lua_Integer)*16 +
// sizeof(lua_Number). lua_Integer is int64 and lua_Number is float64, matching
// include/lua.h.
const luaLNumSizes = unsafe.Sizeof(int64(0))*16 + unsafe.Sizeof(float64(0))

// luaVersionNumGo is LUA_VERSION_NUM (5.3 → 503). Kept here so non-cgo builds
// can check versions the same way as the C export in cdlopen.go.
const luaVersionNumGo = 503

// CheckVersion_ checks that the running core matches the version and
// numeric type sizes the caller was compiled against (luaL_checkversion_,
// lauxlib.c). A mismatch raises via Error2 — it must not be a silent no-op.
//
// C also compares lua_version(L) != lua_version(NULL) to detect multiple
// VMs in one process. This port keeps one immortal version number, so that
// pointer-identity check cannot fire; see NOTES.md.
func (L *State) CheckVersion_(ver float64, sz uintptr) {
	if sz != luaLNumSizes { /* check numeric types */
		L.Error2("core and library have incompatible numeric types")
	}
	/* C: if (v != lua_version(NULL)) → "multiple Lua VMs detected"
	   Go: single version constant; no multi-VM pointer split. */
	if float64(luaVersionNumGo) != ver {
		L.Error2("version mismatch: app. needs %f, Lua core provides %f",
			ver, float64(luaVersionNumGo))
	}
}

// CheckAny raises an error when the argument is absent (luaL_checkany).
// C only asks whether there *is* a value -- not what it is -- so the
// message is "bad argument #n to 'f' (value expected)". Matches C:
// if (lua_type(L, arg) == LUA_TNONE) luaL_argerror(...).
func (L *State) CheckAny(arg int) {
	if L.Type(arg) == LUA_TNONE {
		L.ArgError(arg, "value expected")
	}
}

// CheckType checks the type of an argument (luaL_checktype).
func (L *State) CheckType(arg, t int) {
	if L.Type(arg) != t {
		L.tagError(arg, t)
	}
}

// CheckNumber checks that an argument is a number (luaL_checknumber).
func (L *State) CheckNumber(arg int) float64 {
	d, ok := L.ToNumber(arg)
	if !ok {
		L.tagError(arg, LUA_TNUMBER)
	}
	return d
}

// OptNumber returns an optional number argument (luaL_optnumber).
func (L *State) OptNumber(arg int, def float64) float64 {
	if L.IsNoneOrNil(arg) {
		return def
	}
	return L.CheckNumber(arg)
}

// CheckInteger checks that an argument is an integer (luaL_checkinteger).
func (L *State) CheckInteger(arg int) int64 {
	d, ok := L.ToInteger(arg)
	if !ok {
		L.intError(arg)
	}
	return d
}

// OptInteger returns an optional integer argument (luaL_optinteger).
func (L *State) OptInteger(arg int, def int64) int64 {
	if L.IsNoneOrNil(arg) {
		return def
	}
	return L.CheckInteger(arg)
}

// CheckString checks that an argument is a string (luaL_checkstring /
// luaL_checklstring).
func (L *State) CheckString(arg int) string {
	s, ok := L.ToString(arg)
	if !ok {
		L.tagError(arg, LUA_TSTRING)
	}
	return s
}

// OptString returns an optional string argument (luaL_optlstring).
func (L *State) OptString(arg int, def string) string {
	if L.IsNoneOrNil(arg) {
		return def
	}
	return L.CheckString(arg)
}

// CheckOption checks a string argument against a list of options
// (luaL_checkoption). When def is non-empty it is the luaL_optstring
// default; callers that need a required option pass the already-checked
// string as def (see liolib), matching C's (def ? opt : check) without a
// NULL pointer in Go.
func (L *State) CheckOption(arg int, def string, lst []string) int {
	name := L.OptString(arg, def)
	for i, s := range lst {
		if s == name {
			return i
		}
	}
	return L.ArgError(arg, fmt.Sprintf("invalid option '%s'", name))
}

func (L *State) intError(arg int) {
	if L.IsNumber(arg) {
		L.ArgError(arg, "number has no integer representation")
	} else {
		L.tagError(arg, LUA_TNUMBER)
	}
}

func (L *State) tagError(arg, tag int) {
	L.typeError(arg, TypeName(tag))
}

func (L *State) typeError(arg int, tname string) {
	/* use a description of the actual value for a better message
	   (static typeerror in lauxlib.c). C leaves __name on the stack;
	   ArgError never returns. */
	var typearg string
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		/* lua_tostring / %s cuts a __name that contains a zero byte. */
		typearg = luaCStr(L.ToStringX(-1))
	} else if L.Type(arg) == LUA_TLIGHTUSERDATA {
		typearg = "light userdata"
	} else {
		typearg = TypeName(L.Type(arg))
	}
	msg := fmt.Sprintf("%s expected, got %s", tname, typearg)
	L.ArgError(arg, msg)
}

// GetMetaField pushes obj[name] where obj is the value at idx and name a
// field of its metatable (luaL_getmetafield).
func (L *State) GetMetaField(obj int, name string) int {
	if !L.GetMetaTable(obj) { /* no metatable? */
		return LUA_TNIL
	}
	L.PushString(name)
	tt := L.RawGet(-2)
	if tt == LUA_TNIL {
		L.Pop(2)
	} else {
		L.Remove(-2)
	}
	return tt
}

// CallMeta calls a metamethod of the value at obj (luaL_callmeta).
func (L *State) CallMeta(obj int, name string) bool {
	obj = L.AbsIndex(obj)                      /* getmetafield may grow the stack */
	if L.GetMetaField(obj, name) == LUA_TNIL { /* no metafield? */
		return false
	}
	L.PushValue(obj)
	L.Call(1, 1)
	return true
}

// ToLString converts the value at idx the way luaL_tolstring does
// (__tostring, then the default formatting) and leaves that string on
// the stack.
func (L *State) ToLString(idx int) string { return strToLString(L, idx) }

// ArgCheck raises an error when cond is false (luaL_argcheck).
func (L *State) ArgCheck(cond bool, arg int, extramsg string) {
	if !cond {
		L.ArgError(arg, extramsg)
	}
}

// Where2 pushes a string with the current position (luaL_where).
// Uses getstack+getinfo("Sl") like C, not a manual ci walk.
func (L *State) Where2(level int) {
	var ar dbAr
	if dbGetStack(L, level, &ar) { /* check function at level */
		dbGetInfo(L, "Sl", &ar) /* get info about it */
		if ar.currentline > 0 { /* is there info? */
			L.PushString(fmt.Sprintf("%s:%d: ", ar.shortSrc, ar.currentline))
			return
		}
	}
	L.PushString("") /* else, no information available... */
}

// Len2 returns the length of the value at idx as an integer (luaL_len).
func (L *State) Len2(idx int) int64 {
	L.Len(idx)
	l, ok := L.ToInteger(-1)
	if !ok {
		L.Error2("object length is not an integer")
	}
	L.Pop(1)
	return l
}

// ---------------------------------------------------------------------
// references
// ---------------------------------------------------------------------

// luaLFreeList is the freelist header slot for luaL_ref (lauxlib.c 5.3.4:
// #define freelist 0). Slot 0 of the caller's table holds the free list.
const luaLFreeList = 0

// LUA_REFNIL and LUA_NOREF are the two predefined references (lauxlib.h).
const (
	LUA_NOREF  = -2
	LUA_REFNIL = -1
)

// Ref creates a reference to the value on top of the stack in the table
// at 't' and pops it (luaL_ref).
func (L *State) Ref(t int) int {
	if L.IsNil(-1) {
		L.Pop(1)          /* remove nil */
		return LUA_REFNIL /* 'nil' has a unique fixed reference */
	}
	t = L.AbsIndex(t)
	L.RawGetI(t, luaLFreeList) /* get first free element */
	ref, _ := L.ToInteger(-1)  /* ref = t[freelist] */
	L.Pop(1)                   /* remove it from stack */
	if ref != 0 {              /* any free element? */
		L.RawGetI(t, ref)          /* remove it from list */
		L.RawSetI(t, luaLFreeList) /* (t[freelist] = t[ref]) */
	} else { /* no free elements */
		ref = int64(L.RawLen(t)) + 1 /* get a new reference */
	}
	L.RawSetI(t, ref)
	return int(ref)
}

// Unref releases a reference (luaL_unref).
func (L *State) Unref(t, ref int) {
	if ref >= 0 {
		t = L.AbsIndex(t)
		L.RawGetI(t, luaLFreeList)
		L.RawSetI(t, int64(ref)) /* t[ref] = t[freelist] */
		L.PushInteger(int64(ref))
		L.RawSetI(t, luaLFreeList) /* t[freelist] = ref */
	}
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

// SetFuncs registers all functions in l into the table on top of the
// stack (luaL_setfuncs).
func (L *State) SetFuncs(l []Reg, nup int) {
	L.CheckStack2(nup, "too many upvalues")
	for _, r := range l {
		for i := 0; i < nup; i++ { /* copy upvalues to the top */
			L.PushValue(-nup)
		}
		L.PushGoClosure(r.Func, nup)
		L.SetField(-(nup + 2), r.Name)
	}
	L.Pop(nup) /* remove upvalues */
}

// GetSubTable pushes t[name] where t is the table at idx, creating the
// subtable if needed (luaL_getsubtable).
func (L *State) GetSubTable(idx int, fname string) bool {
	if L.GetField(idx, fname) == LUA_TTABLE {
		return true /* table already there */
	}
	L.Pop(1) /* remove previous result */
	idx = L.AbsIndex(idx)
	L.NewTable()
	L.PushValue(-1)        /* copy to be left at top */
	L.SetField(idx, fname) /* assign new table to field */
	return false           /* false, because did not find table there */
}

// CheckVersion is luaL_checkversion: verify core version and numeric sizes.
func (L *State) CheckVersion() {
	L.CheckVersion_(float64(luaVersionNumGo), luaLNumSizes)
}

// NewLib creates a new table and registers the functions of l into it
// (luaL_newlib). C's macro always runs luaL_checkversion first.
func (L *State) NewLib(l []Reg) {
	L.CheckVersion()
	L.CreateTable(0, len(l))
	L.SetFuncs(l, 0)
}

// ---------------------------------------------------------------------
// string buffer (lauxlib.c luaL_Buffer / lauxlib.h)
// ---------------------------------------------------------------------

// LUAL_BUFFERSIZE is the initial buffer size (luaconf.h).
// Non-longdouble formula: (int)(0x80 * sizeof(void*) * sizeof(lua_Integer)).
// On this port (64-bit pointers, int64 Integer) that is 8192, matching
// include/lauxlib.h and the longdouble branch's literal 8192.
const LUAL_BUFFERSIZE = 8192

// bufInitSize is an alias kept for older call sites / cgo comments.
const bufInitSize = LUAL_BUFFERSIZE

// Buffer is luaL_Buffer (lauxlib.h):
//
//	typedef struct luaL_Buffer {
//	  char *b;  size_t size;  size_t n;  lua_State *L;
//	  char initb[LUAL_BUFFERSIZE];
//	} luaL_Buffer;
//
// buffinit points b at initb; a userdata "box" appears only when
// prepbuffsize outgrows initb (buffonstack). Pure-Go hot paths should
// prefer a stack-local Buffer + BuffInit, like C's luaL_Buffer b on the
// C stack — NewBuffer heap-allocates for convenience of existing callers.
type Buffer struct {
	b     []byte // B->b (aliases initb or grown box storage)
	size  int    // B->size
	n     int    // B->n
	L     *State
	initb [LUAL_BUFFERSIZE]byte // B->initb

	// When buffonstack (b no longer aliases initb): UBox on the Lua stack
	// plus grown storage. boxBuf is the pure-Go block; scratch/slen are the
	// C-addressable block when cblock is set (cgo luaL_buffinit).
	boxPtr  uintptr
	boxBuf  []byte
	scratch unsafe.Pointer
	slen    int

	// cblock: cgo path needs memory C may hold across calls (FLAG:
	// platform/cgo companion). Pure-Go buffers never set this — they use
	// initb then a Go []byte box, matching C's initb→UBox choreography
	// without a C allocator. Not a "fast path" that skips initb.
	cblock bool
}

// reallocBlock replaces a buffer's C block with one of 'sz' bytes, copying
// the content across; 'old' is nil on the first call. Set by the cgo layer.
var reallocBlock func(old unsafe.Pointer, sz int) unsafe.Pointer

// bufFree frees a buffer's C block once the buffer is unreachable.
var bufFree func(b *Buffer)

func bufFinalize(b *Buffer) {
	if bufFree != nil {
		bufFree(b)
	}
}

// buffonstack is C's '#define buffonstack(B) ((B)->b != (B)->initb)'.
func (B *Buffer) buffonstack() bool {
	if len(B.b) == 0 {
		return false
	}
	return unsafe.Pointer(&B.b[0]) != unsafe.Pointer(&B.initb[0])
}

// BuffInit is luaL_buffinit: B starts on embedded initb, nothing on the
// Lua stack. Prefer this with a stack-local Buffer for hot callers.
func (L *State) BuffInit(B *Buffer) {
	B.L = L
	B.b = B.initb[:]
	B.n = 0
	B.size = LUAL_BUFFERSIZE
	B.boxPtr = 0
	B.boxBuf = nil
	/* scratch/cblock left for the cgo export to configure after BuffInit */
}

// NewBuffer heap-allocates a Buffer and runs BuffInit (luaL_buffinit).
// Equivalent to C's 'luaL_Buffer b; luaL_buffinit(L, &b);' when the
// Buffer must outlive the current frame; hot stdlib sites may switch to
// stack-local Buffer + BuffInit (Apply debt).
func (L *State) NewBuffer() *Buffer {
	B := &Buffer{}
	L.BuffInit(B)
	return B
}

// ensureBox pushes the UBox userdata C creates in newbox(), and records
// its address so PushResult / AddValue can find it under the top.
func (B *Buffer) ensureBox() {
	if B.boxPtr != 0 {
		return
	}
	L := B.L
	L.checkStack(1)
	u := luaS_newudata(L, nil)
	setuvalue(&L.stack[L.top], u)
	L.top++
	B.boxPtr = uintptr(unsafe.Pointer(u))
}

// newBox is C newbox + memcpy of current content (leaving initb).
func (B *Buffer) newBox(newsize int) {
	B.ensureBox()
	if B.cblock && reallocBlock != nil {
		p := reallocBlock(nil, newsize)
		if p == nil && newsize > 0 {
			B.L.Error2("not enough memory for buffer allocation")
		}
		if p != nil {
			runtime.SetFinalizer(B, bufFinalize) /* Go GC stand-in for UBox.__gc */
			if B.n > 0 {
				copy(unsafe.Slice((*byte)(p), newsize)[:B.n], B.b[:B.n])
			}
		}
		B.scratch = p
		B.slen = newsize
		if p != nil {
			B.b = unsafe.Slice((*byte)(p), newsize)
		} else {
			B.b = nil
		}
		return
	}
	grown := make([]byte, newsize)
	copy(grown, B.b[:B.n])
	B.boxBuf = grown
	B.b = grown
}

// resizeBox is C resizebox(L, -1, newsize) for an existing on-stack box.
func (B *Buffer) resizeBox(newsize int) {
	if B.cblock && reallocBlock != nil {
		p := reallocBlock(B.scratch, newsize)
		if p == nil && newsize > 0 {
			reallocBlock(B.scratch, 0) /* free buffer, like C */
			B.scratch = nil
			B.slen = 0
			B.b = nil
			B.L.Error2("not enough memory for buffer allocation")
		}
		B.scratch = p
		B.slen = newsize
		if p != nil {
			B.b = unsafe.Slice((*byte)(p), newsize)
		} else {
			B.b = nil
		}
		return
	}
	grown := make([]byte, newsize)
	copy(grown, B.b[:B.n])
	B.boxBuf = grown
	B.b = grown
}

// growTo expands the live block to newsize (newbox or resizebox).
func (B *Buffer) growTo(newsize int) {
	if B.buffonstack() {
		B.resizeBox(newsize)
	} else {
		B.newBox(newsize)
	}
	B.size = newsize
}

// freeBox is resizebox(L, idx, 0): drop grown storage after pushresult.
func (B *Buffer) freeBox() {
	if B.cblock && reallocBlock != nil && B.scratch != nil {
		reallocBlock(B.scratch, 0)
		B.scratch = nil
		B.slen = 0
	}
	B.boxBuf = nil
	B.b = nil
	B.size = 0
}

// room is luaL_prepbuffsize: returns a slice of at least sz free bytes
// at &B.b[B.n], growing via size*2 / n+sz like C.
func (B *Buffer) room(sz int) []byte {
	/* cgo: Go initb is not stable for C to hold; leave initb on first prep
	   so buffonstack becomes true and scratch is C memory (cdlopen room(0)). */
	if B.cblock && !B.buffonstack() && reallocBlock != nil {
		B.growTo(B.size)
	}
	if B.size-B.n < sz { /* not enough space? */
		newsize := B.size * 2 /* double buffer size */
		if newsize-B.n < sz { /* not big enough? */
			newsize = B.n + sz
		}
		if newsize < B.n || newsize-B.n < sz {
			B.L.Error2("buffer too large")
		}
		B.growTo(newsize)
	}
	return B.b[B.n : B.n+sz]
}

// AddString is luaL_addstring.
func (B *Buffer) AddString(s string) {
	B.AddBytes([]byte(s))
}

// AddBytes is luaL_addlstring (length-explicit; Go []byte stand-in).
func (B *Buffer) AddBytes(p []byte) {
	if len(p) > 0 { /* avoid copy when p can be empty/nil — C: if (l > 0) */
		copy(B.room(len(p)), p)
		B.AddSize(len(p))
	}
}

// AddChar is luaL_addchar.
func (B *Buffer) AddChar(c byte) {
	B.room(1)[0] = c
	B.n++
}

// AddValue adds the value on top of the stack, converted to a string
// (luaL_addvalue). Matches C: lua_tolstring then addlstring; a
// non-coercible value contributes zero bytes (no Go-only error).
// buffonstack is checked before insert and again after addlstring
// (growth may push a box mid-call) — same as C.
func (B *Buffer) AddValue() {
	L := B.L
	s, _ := L.ToString(-1)
	if B.buffonstack() {
		L.Insert(-2) /* put value below buffer */
	}
	B.AddString(s)
	if B.buffonstack() {
		L.Remove(-2) /* remove value */
	} else {
		L.Pop(1) /* remove value */
	}
}

// Len is luaL_bufflen (macro over n).
func (B *Buffer) Len() int { return B.n }

// String answers what the buffer holds, without touching it.
func (B *Buffer) String() string {
	if B.n == 0 {
		return ""
	}
	return string(B.b[:B.n])
}

// PushResult finishes the buffer and pushes the resulting string
// (luaL_pushresult). When buffonstack, free the box storage and remove
// the UBox — one value out where the box was.
func (B *Buffer) PushResult() {
	L := B.L
	L.PushString(B.String())
	if B.buffonstack() {
		B.freeBox() /* resizebox(L, -2, 0) */
		if B.boxPtr != 0 && L.GetTop() >= 2 {
			v := &L.stack[L.top-2]
			if ttisfulluserdata(v) && uintptr(unsafe.Pointer(uvalue(v))) == B.boxPtr {
				L.Remove(-2) /* remove its header from the stack */
			}
		}
	}
	B.n = 0
	B.boxPtr = 0
}

// PushResultSize is luaL_pushresultsize: addsize then pushresult.
func (B *Buffer) PushResultSize(sz int) {
	B.AddSize(sz)
	B.PushResult()
}

// PrepBuffSize is luaL_prepbuffsize: returns a slice of at least sz free bytes.
func (B *Buffer) PrepBuffSize(sz int) []byte { return B.room(sz) }

// BuffInitSize is luaL_buffinitsize: buffinit then prepbuffsize.
// Heap form for callers that need a *Buffer; stack-local sites can
// BuffInit(&B) then B.PrepBuffSize(sz).
func (L *State) BuffInitSize(sz int) (*Buffer, []byte) {
	B := L.NewBuffer()
	return B, B.PrepBuffSize(sz)
}

// AddSize is luaL_addsize: n more bytes were written into the prep room.
func (B *Buffer) AddSize(n int) {
	B.n += n
}

// ---------------------------------------------------------------------
// traceback (lauxlib.c)
// ---------------------------------------------------------------------

// Traceback pushes a traceback of the current stack (luaL_traceback).
// 'hasMsg' is C's 'msg != NULL': when a message is given it is written
// first, followed by a newline -- and that happens even for the empty
// string, which is why 'debug.traceback("", 1)' starts with a newline
// while 'debug.traceback(nil, 1)' does not.
func (L *State) Traceback(L1 *State, msg string, hasMsg bool, level int) {
	L.PushString(L.stackTraceback(L1, msg, hasMsg, level))
}

// tracebackLevels1 / tracebackLevels2 are LEVELS1 / LEVELS2 of lauxlib.c.
const (
	tracebackLevels1 = 10 /* size of the first part of the stack */
	tracebackLevels2 = 11 /* size of the second part of the stack */
)

// findField is 'findfield' (lauxlib.c): it searches 'obj' in the table on
// top of the stack, recursively up to 'level' levels. On success it
// returns the name, which is left pushed on the stack.
func (L *State) findField(obj *Value, level int) (string, bool) {
	if level == 0 || !L.IsTable(-1) {
		return "", false /* not found */
	}
	L.PushNil()      /* start 'next' loop */
	for L.Next(-2) { /* for each pair in table */
		if L.Type(-2) == LUA_TSTRING { /* ignore non-string keys */
			if luaV_rawequalobj(obj, &L.stack[L.top-1]) { /* found object? */
				L.Pop(1) /* remove value (but keep name) */
				s, _ := L.ToString(-1)
				return s, true
			}
			if _, ok := L.findField(obj, level-1); ok { /* try recursively */
				/* stack: lib_name, lib_table, field_name (top) — C: */
				L.Remove(-2)      /* remove table (but keep name) */
				L.PushString(".") /* place '.' between the two names */
				L.Insert(-2)
				L.Concat(3) /* lib_name.field_name */
				s, _ := L.ToString(-1)
				return s, true
			}
		}
		L.Pop(1) /* remove value */
	}
	return "", false /* not found */
}

// globalFuncName is 'pushglobalfuncname' (lauxlib.c): it searches a name
// for 'obj' among all loaded modules.
func (L *State) globalFuncName(obj *Value) (string, bool) {
	/* 'obj' points into the stack, and 'findField' pushes values while
	   it searches -- which can reallocate the stack and leave the
	   pointer dangling. C passes a stack index (so 'lua_rawequal' always
	   re-reads it); copy the value to get the same safety. */
	objCopy := *obj
	top := L.GetTop()
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.CheckStack(6) /* slots for 'findField' */
	if name, ok := L.findField(&objCopy, 2); ok {
		if strings.HasPrefix(name, "_G.") { /* name starts with '_G.'? */
			name = name[3:] /* use the name without the prefix */
		}
		L.SetTop(top)
		return name, true
	}
	L.SetTop(top) /* remove the loaded table */
	return "", false
}

// funcName is 'pushfuncname' (lauxlib.c): it describes the function of
// 'ar' the way a traceback line does.
func (L *State) funcName(ar *dbAr, obj *Value) string {
	if name, ok := L.globalFuncName(obj); ok { /* try a global name first */
		return fmt.Sprintf("function '%s'", name)
	}
	if ar.namewhat != "" { /* is there a name from the code? */
		return fmt.Sprintf("%s '%s'", ar.namewhat, ar.name)
	}
	switch ar.what {
	case "main":
		return "main chunk"
	case "C": /* nothing left... */
		return "?"
	}
	return fmt.Sprintf("function <%s:%d>", ar.shortSrc, ar.linedefined)
}

// lastLevel is 'lastlevel' (lauxlib.c): the deepest level of the stack.
func (L *State) lastLevel() int {
	var ar dbAr
	li, le := 1, 1
	for dbGetStack(L, le, &ar) { /* find an upper bound */
		li = le
		le *= 2
	}
	for li < le { /* do a binary search */
		m := (li + le) / 2
		if dbGetStack(L, m, &ar) {
			li = m + 1
		} else {
			le = m
		}
	}
	return le - 1
}

// stackTraceback is 'luaL_traceback' (lauxlib.c). 'L' is the state that
// receives the message, 'L1' the one whose stack is walked.
func (L *State) stackTraceback(L1 *State, msg string, hasMsg bool, level int) string {
	var sb strings.Builder
	last := L1.lastLevel()
	limit2show := -1
	if last-level > tracebackLevels1+tracebackLevels2 {
		limit2show = tracebackLevels1
	}
	if hasMsg { /* 'if (msg)' in C: a NULL pointer, not an empty string */
		sb.WriteString(msg)
		sb.WriteByte('\n')
	}
	sb.WriteString("stack traceback:")
	var ar dbAr
	for dbGetStack(L1, level, &ar) {
		level++
		skip := limit2show == 0 /* too many levels? */
		limit2show--
		if skip {
			n := last - level - tracebackLevels2 + 1 /* levels to skip */
			sb.WriteString("\n\t...")                /* 5.3.4: no level count */
			level += n                               /* and skip to the last levels */
			continue
		}
		fv := &L1.stack[ar.ci.funcIdx]
		/* C does 'lua_getinfo(L1, "Slnt", &ar)': the information is
		   collected on the thread being traced, which is not
		   necessarily the one building the message. */
		dbAuxGetInfo(L1, "Slnt", &ar, fv, ar.ci)
		if ar.currentline <= 0 {
			fmt.Fprintf(&sb, "\n\t%s: in ", ar.shortSrc)
		} else {
			fmt.Fprintf(&sb, "\n\t%s:%d: in ", ar.shortSrc, ar.currentline)
		}
		sb.WriteString(L.funcName(&ar, fv))
		if ar.istailcall {
			sb.WriteString("\n\t(...tail calls...)")
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------
// userdata metatable helpers (lauxlib.c)
// ---------------------------------------------------------------------

// NewMetaTable creates a new metatable registered under tname
// (luaL_newmetatable). Returns false if the name was already in use
// (previous value left on top).
func (L *State) NewMetaTable(tname string) bool {
	if L.GetField(LUA_REGISTRYINDEX, tname) != LUA_TNIL { /* name already in use? */
		return false /* leave previous value on top, but return false */
	}
	L.Pop(1)
	L.CreateTable(0, 2) /* create metatable */
	L.PushString(tname)
	L.SetField(-2, "__name") /* metatable.__name = tname */
	L.PushValue(-1)
	L.SetField(LUA_REGISTRYINDEX, tname) /* registry[tname] = metatable */
	return true
}

// SetMetaTableByName sets the metatable registered under tname on the
// value at the top of the stack (luaL_setmetatable).
func (L *State) SetMetaTableByName(tname string) {
	L.GetField(LUA_REGISTRYINDEX, tname)
	L.SetMetaTable(-2)
}

// TestUserData returns the userdata at ud when it has the metatable
// registered under tname, otherwise nil (luaL_testudata).
func (L *State) TestUserData(ud int, tname string) any {
	p := L.ToUserData(ud)
	if p != nil { /* value is a userdata? */
		if L.GetMetaTable(ud) { /* does it have a metatable? */
			L.GetField(LUA_REGISTRYINDEX, tname) /* get correct metatable */
			if !L.RawEqual(-1, -2) {             /* not the same? */
				p = nil /* userdata with wrong metatable */
			}
			L.Pop(2) /* remove both metatables */
			return p
		}
	}
	return nil /* not a userdata with a metatable */
}

// CheckUserData is luaL_checkudata: TestUserData or typeError(tname).
func (L *State) CheckUserData(ud int, tname string) any {
	p := L.TestUserData(ud, tname)
	if p == nil {
		L.typeError(ud, tname)
	}
	return p
}

// ---------------------------------------------------------------------
// LUA_COMPAT_MODULE (enabled in PUC 5.3.4 luaconf.h)
// ---------------------------------------------------------------------

// FindTable finds or creates a dotted table path starting at idx
// (luaL_findtable). On success it leaves the table on top and returns "".
// On conflict it returns the problematic name fragment.
func (L *State) FindTable(idx int, fname string, szhint int) string {
	if idx != 0 {
		L.PushValue(idx)
	}
	for {
		e := len(fname)
		for i := 0; i < len(fname); i++ {
			if fname[i] == '.' {
				e = i
				break
			}
		}
		L.PushString(fname[:e])
		if L.RawGet(-2) == LUA_TNIL { /* no such field? */
			L.Pop(1) /* remove this nil */
			hint := szhint
			if e < len(fname) && fname[e] == '.' {
				hint = 1
			}
			L.CreateTable(0, hint) /* new table for field */
			L.PushString(fname[:e])
			L.PushValue(-2)
			L.SetTable(-4) /* set new table into field */
		} else if !L.IsTable(-1) { /* field has a non-table value? */
			L.Pop(2)     /* remove table and value */
			return fname /* return problematic part of the name */
		}
		L.Remove(-2) /* remove previous table */
		if e >= len(fname) || fname[e] != '.' {
			return ""
		}
		fname = fname[e+1:]
	}
}

// PushModule finds or creates a module table with the given name
// (luaL_pushmodule). Leaves the module table on the stack.
func (L *State) PushModule(modname string, sizehint int) {
	L.FindTable(LUA_REGISTRYINDEX, pkLoadedTable, 1)
	if L.GetField(-1, modname) != LUA_TTABLE { /* no LOADED[modname]? */
		L.Pop(1) /* remove previous result */
		/* try global variable (and create one if it does not exist) */
		L.PushGlobalTable()
		if L.FindTable(0, modname, sizehint) != "" {
			L.Error2("name conflict for module '%s'", modname)
		}
		L.PushValue(-1)
		L.SetField(-3, modname) /* LOADED[modname] = new table */
	}
	L.Remove(-2) /* remove LOADED table */
}

// OpenLib opens a library (luaL_openlib / luaL_register).
func (L *State) OpenLib(libname string, l []Reg, nup int) {
	L.CheckVersion()
	if libname != "" {
		L.PushModule(libname, len(l)) /* get/create library table */
		L.Insert(-(nup + 1))          /* move library table below upvalues */
	}
	if l != nil {
		L.SetFuncs(l, nup)
	} else {
		L.Pop(nup) /* remove upvalues */
	}
}

// ---------------------------------------------------------------------
// GSub (luaL_gsub)
// ---------------------------------------------------------------------

// GSub replaces every occurrence of p in s with r, pushes the result,
// and returns it (luaL_gsub).
func (L *State) GSub(s, p, r string) string {
	b := L.NewBuffer()
	for {
		i := strings.Index(s, p)
		if i < 0 {
			break
		}
		b.AddString(s[:i]) /* push prefix */
		b.AddString(r)     /* push replacement in place of pattern */
		s = s[i+len(p):]   /* continue after p */
	}
	b.AddString(s) /* push last suffix */
	b.PushResult()
	out, _ := L.ToString(-1)
	return out
}

// ---------------------------------------------------------------------
// misc helpers used by the standard libraries
// ---------------------------------------------------------------------

// CheckStack2 raises an error when there is not enough stack space
// (luaL_checkstack).
func (L *State) CheckStack2(sz int, msg string) {
	if !L.CheckStack(sz) {
		if msg != "" {
			L.Error2("stack overflow (%s)", msg)
		} else {
			L.Error2("stack overflow")
		}
	}
}
