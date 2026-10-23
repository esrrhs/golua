// Translation of lauxlib.h / lauxlib.c (PUC-Lua 5.4.9).
package core

import (
	"runtime"
	"strings"
	"unsafe"
)

// LUA_GNAME is the global table name (lauxlib.h).
const LUA_GNAME = "_G"

// Reg is the Go counterpart of luaL_Reg.
type Reg struct {
	Name string
	Func GoFunc
}

// PushFail pushes the value used to represent failure (luaL_pushfail).
// In 5.4 the default is nil.
func (L *State) PushFail() { L.PushNil() }

// ArgError raises an error for a bad argument (luaL_argerror,
// lauxlib.c).
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
		arg--         /* do not count 'self' */
		if arg == 0 { /* error is in the self argument itself? */
			return L.Error2("calling '%s' on bad self (%s)", ar.name, extramsg)
		}
	}
	if ar.name == "" {
		/* ar.name == NULL: try a global name for the function */
		if L.pushGlobalFuncName(&ar) {
			ar.name = L.ToStringX(-1)
		} else {
			ar.name = "?"
		}
	}
	return L.Error2("bad argument #%d to '%s' (%s)", arg, ar.name, extramsg)
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
	L.PushFString(format, args...) /* lua_pushvfstring */
	L.Concat(2)
	return L.Error()
}

// luaLNumSizes is LUAL_NUMSIZES for this port: sizeof(lua_Integer)*16 +
// sizeof(lua_Number). lua_Integer is int64 and lua_Number is float64, matching
// include/lua.h.
const luaLNumSizes = unsafe.Sizeof(int64(0))*16 + unsafe.Sizeof(float64(0))

// luaVersionNumGo is LUA_VERSION_NUM (5.4 → 504). Kept here so non-cgo builds
// can check versions the same way as the C export in cdlopen.go.
const luaVersionNumGo = 504

// CheckVersion_ checks that the running core matches the version and
// numeric type sizes the caller was compiled against (luaL_checkversion_,
// lauxlib.c 5.4.9). A mismatch raises via Error2 — it must not be a silent
// no-op. 5.4.9 no longer compares lua_version(L) against lua_version(NULL)
// for multi-VM detection (that check existed in 5.3).
func (L *State) CheckVersion_(ver float64, sz uintptr) {
	v := Version(L)         /* C: lua_version(L) */
	if sz != luaLNumSizes { /* check numeric types */
		L.Error2("core and library have incompatible numeric types")
	} else if v != ver {
		L.Error2("version mismatch: app. needs %f, Lua core provides %f",
			ver, v)
	}
}

// CheckVersion is luaL_checkversion: verify core version and numeric sizes.
func (L *State) CheckVersion() {
	L.CheckVersion_(float64(luaVersionNumGo), luaLNumSizes)
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
	return L.ArgError(arg, L.PushFString("invalid option '%s'", name))
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

func (L *State) typeError(arg int, tname string) int {
	/* luaL_typeerror (lauxlib.c). Public TypeError in cdlopen_aux is the
	   C-export twin (same algorithm + luaCStr); prefer consolidating later. */
	var typearg string
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		typearg = luaCStr(L.ToStringX(-1)) /* %s cuts at a zero byte */
	} else if L.Type(arg) == LUA_TLIGHTUSERDATA {
		typearg = "light userdata" /* special name for messages */
	} else {
		typearg = L.TypeName(arg) /* standard name */
	}
	msg := L.PushFString("%s expected, got %s", tname, typearg)
	return L.ArgError(arg, msg)
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
		L.Pop(2) /* remove metatable and metafield */
	} else {
		L.Remove(-2) /* remove only metatable */
	}
	return tt /* return metafield type */
}

// CallMeta calls a metamethod of the value at obj (luaL_callmeta).
func (L *State) CallMeta(obj int, name string) bool {
	obj = L.AbsIndex(obj)
	if L.GetMetaField(obj, name) == LUA_TNIL { /* no metafield? */
		return false
	}
	L.PushValue(obj)
	L.Call(1, 1)
	return true
}

// ToLString converts the value at idx the way luaL_tolstring does
// (__tostring, then the default formatting) and leaves that string on
// the stack. Implementation lives with the string library helpers
// (strToLString) so formatting stays in one place.
func (L *State) ToLString(idx int) string { return strToLString(L, idx) }

// ArgCheck raises an error when cond is false (luaL_argcheck).
func (L *State) ArgCheck(cond bool, arg int, extramsg string) {
	if !cond {
		L.ArgError(arg, extramsg)
	}
}

// ArgExpected raises an error when cond is false, naming the type that
// was expected (luaL_argexpected). 5.4 API used by setmetatable and
// friends for a second argument that may be one of two types.
func (L *State) ArgExpected(cond bool, arg int, tname string) {
	if !cond {
		L.typeError(arg, tname)
	}
}

// Where2 pushes a string with the current position (luaL_where).
// Uses getstack+getinfo("Sl") like C, not a manual ci walk.
func (L *State) Where2(level int) {
	var ar dbAr
	if dbGetStack(L, level, &ar) { /* check function at level */
		dbGetInfo(L, "Sl", &ar) /* get info about it */
		if ar.currentline > 0 { /* is there info? */
			L.PushFString("%s:%d: ", ar.shortSrc, ar.currentline)
			return
		}
	}
	L.PushFString("") /* else, no information available... */
}

// Len2 returns the length of the value at idx as an integer (luaL_len).
func (L *State) Len2(idx int) int64 {
	L.Len(idx)
	l, ok := L.ToInteger(-1)
	if !ok {
		L.Error2("object length is not an integer")
	}
	L.Pop(1) /* remove object */
	return l
}

// ---------------------------------------------------------------------
// references (lauxlib.c — freelist = LUA_RIDX_LAST + 1 in 5.4)
// ---------------------------------------------------------------------

// luaLFreeList is the slot luaL_ref keeps its free list in: 'freelist'
// in lauxlib.c 5.4.9, which is LUA_RIDX_LAST + 1 — not 0, which is where
// 5.3 kept it. It is a plain key of the table the caller handed in, so an
// extension that looks at that table sees the difference, and so does
// one that unrefs a reference and then reads the slot.
const luaLFreeList = 3 /* LUA_RIDX_LAST + 1 */

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
	var ref int64
	if L.RawGetI(t, luaLFreeList) == LUA_TNIL { /* first access? */
		ref = 0 /* list is empty */
		L.PushInteger(0)
		L.RawSetI(t, luaLFreeList) /* t[freelist] = 0 */
	} else { /* already initialized */
		ref, _ = L.ToInteger(-1) /* ref = t[freelist] */
	}
	L.Pop(1)      /* remove the element lua_rawgeti pushed */
	if ref != 0 { /* any free element? */
		L.RawGetI(t, ref)          /* remove it from the list */
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
// stack (luaL_setfuncs). A Reg with Func == nil is a 5.4 placeholder:
// C pushes boolean false instead of a closure (see string.__index).
func (L *State) SetFuncs(l []Reg, nup int) {
	L.CheckStack2(nup, "too many upvalues")
	for _, r := range l {
		if r.Func == nil { /* placeholder? */
			L.PushBoolean(false)
		} else {
			for i := 0; i < nup; i++ { /* copy upvalues to the top */
				L.PushValue(-nup)
			}
			L.PushGoClosure(r.Func, nup) /* closure with those upvalues */
		}
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

// NewLib creates a new table and registers the functions of l into it
// (luaL_newlib). C's macro always runs luaL_checkversion first.
func (L *State) NewLib(l []Reg) {
	L.CheckVersion()
	L.CreateTable(0, len(l)) /* luaL_newlibtable: sizeof(l)/sizeof(l[0])-1 ≈ len */
	L.SetFuncs(l, 0)
}

// ---------------------------------------------------------------------
// string buffer (lauxlib.c luaL_Buffer — 5.4 UBox / placeholder model)
// ---------------------------------------------------------------------

// Buffer is the Go counterpart of luaL_Buffer (lauxlib.h / lauxlib.c 5.4.9).
//
// Content lives in one block: 'b' is its address, 'n' how much is used,
// 'size' how much it holds. luaL_addchar / luaL_addsize / luaL_buffsub are
// macros over those fields in C; the methods here mirror them.
//
// C 5.4 buffinit always pushes a lightuserdata placeholder ((void*)B);
// on growth it replaces that with a UBox userdata marked to-be-closed.
// This port pushes the same placeholder. UBox __gc/__close + toclose are
// replaced by a Go finalizer when cblock allocates C memory (FLAG / Go GC).
type Buffer struct {
	L *State

	initb [bufInitSize]byte // B->init.b — initial on-stack (embedded) area
	b     []byte            // B->b (aliases initb or grown storage)
	size  int               // B->size
	n     int               // B->n

	boxPtr  uintptr // non-zero ⇒ UBox on the Lua stack (buffonstack)
	onStack bool    // placeholder or box was pushed by BuffInit

	scratch unsafe.Pointer // C-addressable block when cblock
	slen    int
	boxBuf  []byte // grown pure-Go storage when leaving initb without cblock

	// cblock: cgo luaL_buffinit needs memory C may hold. Pure-Go buffers
	// (table.concat, utf8.char, …) never set this.
	cblock bool
}

// bufInitSize is LUAL_BUFFERSIZE for 5.4.9:
// ((int)(16 * sizeof(void*) * sizeof(lua_Number))) → 1024 on LP64.
const bufInitSize = int(16 * unsafe.Sizeof(uintptr(0)) * unsafe.Sizeof(float64(0)))

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

// buffonstack is C's '#define buffonstack(B) ((B)->b != (B)->init.b)'.
func (B *Buffer) buffonstack() bool {
	return B.boxPtr != 0
}

// BuffInit is luaL_buffinit: start on embedded initb and push the placeholder.
func (L *State) BuffInit(B *Buffer) {
	B.L = L
	B.b = B.initb[:]
	B.n = 0
	B.size = bufInitSize
	B.boxPtr = 0
	B.boxBuf = nil
	B.scratch = nil
	B.slen = 0
	L.PushLightUserData(unsafe.Pointer(B)) /* push placeholder */
	B.onStack = true
}

// NewBuffer heap-allocates a Buffer and runs BuffInit (luaL_buffinit).
func (L *State) NewBuffer() *Buffer {
	B := &Buffer{}
	L.BuffInit(B)
	return B
}

// newbox pushes the UBox userdata C creates in newbox().
// Go-GC: no metatable __gc/__close; cblock uses a finalizer instead.
func (B *Buffer) newbox() {
	L := B.L
	L.checkStack(1)
	u := luaS_newudata(L, 0, nil)
	setuvalue(&L.stack[L.top], u)
	L.top++
	B.boxPtr = uintptr(unsafe.Pointer(u))
}

// replacePlaceholderWithBox is the else-branch of C prepbuffsize when
// leaving initb: remove placeholder at boxidx, newbox, insert, toclose.
func (B *Buffer) replacePlaceholderWithBox(boxidx int) {
	L := B.L
	L.Remove(boxidx) /* remove placeholder */
	B.newbox()
	L.Insert(boxidx) /* move box to its intended position */
	/* C: lua_toclose(L, boxidx) — FLAG: Go GC / finalizer owns the block */
}

// newbuffsize is C newbuffsize: size*1.5, at least n+sz.
func (B *Buffer) newbuffsize(sz int) int {
	/* C: if (MAX_SIZET - sz < B->n) → "buffer too large" */
	if sz < 0 || B.n+sz < B.n {
		B.L.Error2("buffer too large")
	}
	newsize := (B.size / 2) * 3 /* buffer size * 1.5 */
	if newsize < B.n+sz {
		newsize = B.n + sz
	}
	return newsize
}

// resizebox grows storage for an existing box (C resizebox + content).
func (B *Buffer) resizebox(newsize int) {
	L := B.L
	if B.cblock && reallocBlock != nil {
		p := reallocBlock(B.scratch, newsize)
		if p == nil && newsize > 0 {
			L.Error2("not enough memory")
		}
		if B.scratch == nil && p != nil {
			runtime.SetFinalizer(B, bufFinalize) /* Go GC stand-in for UBox.__gc */
		}
		B.scratch = p
		B.slen = newsize
		B.b = unsafe.Slice((*byte)(p), newsize)
		B.size = newsize
		return
	}
	grown := make([]byte, newsize)
	copy(grown, B.b[:B.n])
	B.boxBuf = grown
	B.b = grown
	B.size = newsize
}

// prepbuffsize is the static helper in lauxlib.c: free area of 'sz' bytes;
// boxidx is the relative stack slot of the placeholder/box.
func (B *Buffer) prepbuffsize(sz, boxidx int) []byte {
	/* cblock without scratch must leave initb for a C-addressable block,
	   even when sz==0 (cgo luaL_buffinit calls room(0) immediately). */
	force := B.cblock && B.scratch == nil && reallocBlock != nil
	if !force && B.size-B.n >= sz {
		return B.b[B.n : B.n+sz]
	}
	newsize := B.newbuffsize(sz)
	if force && newsize < bufInitSize {
		newsize = bufInitSize
	}
	if B.buffonstack() {
		B.resizebox(newsize)
	} else {
		B.replacePlaceholderWithBox(boxidx)
		/* copy original content out of initb into the new block */
		oldn := B.n
		old := make([]byte, oldn)
		copy(old, B.initb[:oldn])
		B.resizebox(newsize)
		copy(B.b, old)
		B.n = oldn
	}
	return B.b[B.n : B.n+sz]
}

// room is prepbuffsize with boxidx -1 (normal prep; box/placeholder on top).
func (B *Buffer) room(sz int) []byte {
	boxidx := -1
	if !B.onStack {
		/* legacy / defensive: no placeholder (should not happen after BuffInit) */
		return B.prepbuffsizeNoStack(sz)
	}
	return B.prepbuffsize(sz, boxidx)
}

func (B *Buffer) prepbuffsizeNoStack(sz int) []byte {
	need := B.n + sz
	if sz < 0 || need < B.n {
		B.L.Error2("buffer too large")
	}
	if B.size-B.n >= sz && len(B.b) >= need {
		return B.b[B.n:need]
	}
	newsize := B.newbuffsize(sz)
	if newsize < bufInitSize {
		newsize = bufInitSize
	}
	grown := make([]byte, newsize)
	copy(grown, B.b[:B.n])
	B.b = grown
	B.size = newsize
	return B.b[B.n:need]
}

// AddString is luaL_addstring.
func (B *Buffer) AddString(s string) {
	B.AddBytes([]byte(s))
}

// AddBytes is luaL_addlstring (length-explicit; Go []byte stand-in).
func (B *Buffer) AddBytes(p []byte) {
	if len(p) > 0 { /* avoid copy when p can be empty/nil */
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
// (luaL_addvalue). C uses prepbuffsize(..., -2) so the box/placeholder
// stays under the string while growth may replace the placeholder.
func (B *Buffer) AddValue() {
	L := B.L
	s, _ := L.ToString(-1)
	boxidx := -1
	if B.onStack {
		boxidx = -2
	}
	dst := B.prepbuffsize(len(s), boxidx)
	copy(dst, s)
	B.AddSize(len(s))
	L.Pop(1) /* pop string */
}

// Len is luaL_bufflen.
func (B *Buffer) Len() int { return B.n }

// BuffSub shrinks the used length (luaL_buffsub macro: B->n -= s).
func (B *Buffer) BuffSub(s int) { B.n -= s }

// String answers what the buffer holds, without touching it.
func (B *Buffer) String() string {
	if B.n == 0 {
		return ""
	}
	return string(B.b[:B.n])
}

// PushResult finishes the buffer and pushes the resulting string
// (luaL_pushresult). Always removes the placeholder or box under the
// new string, matching C's lua_remove(-2).
func (B *Buffer) PushResult() {
	L := B.L
	L.PushString(B.String())
	if B.onStack {
		/* C: if (buffonstack) lua_closeslot(-2); then always lua_remove(-2).
		   Go-GC finalizer owns cblock; skip closeslot. */
		L.Remove(-2) /* remove box or placeholder from the stack */
		B.onStack = false
	}
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
func (L *State) BuffInitSize(sz int) (*Buffer, []byte) {
	b := L.NewBuffer()
	return b, b.PrepBuffSize(sz)
}

// AddSize says that 'n' more bytes were written into the room
// PrepBuffSize handed out (luaL_addsize macro: B->n += s).
func (B *Buffer) AddSize(n int) { B.n += n }

// AddGSub replaces every occurrence of p in s with r into the buffer
// (luaL_addgsub — new in 5.4).
func (B *Buffer) AddGSub(s, p, r string) {
	for {
		i := strings.Index(s, p)
		if i < 0 {
			break
		}
		B.AddString(s[:i]) /* push prefix */
		B.AddString(r)     /* push replacement in place of pattern */
		s = s[i+len(p):]   /* continue after p */
	}
	B.AddString(s) /* push last suffix */
}

// GSub replaces every occurrence of p in s with r, pushes the result,
// and returns it (luaL_gsub).
func (L *State) GSub(s, p, r string) string {
	b := L.NewBuffer()
	b.AddGSub(s, p, r)
	b.PushResult()
	out, _ := L.ToString(-1)
	return out
}

// ---------------------------------------------------------------------
// traceback (lauxlib.c)
// ---------------------------------------------------------------------

// Traceback pushes a traceback of the call stack of L1 onto L
// (luaL_traceback). 'hasMsg' is C's 'msg != NULL': when a message is
// given it is written first, followed by a newline — even for the empty
// string, which is why 'debug.traceback("", 1)' starts with a newline
// while 'debug.traceback(nil, 1)' does not.
func (L *State) Traceback(L1 *State, msg string, hasMsg bool, level int) {
	var b Buffer
	L.BuffInit(&b)
	last := L1.lastLevel()
	limit2show := -1
	if last-level > tracebackLevels1+tracebackLevels2 {
		limit2show = tracebackLevels1
	}
	if hasMsg {
		b.AddString(msg)
		b.AddChar('\n')
	}
	b.AddString("stack traceback:")
	var ar dbAr
	for dbGetStack(L1, level, &ar) {
		level++
		if limit2show == 0 { /* too many levels? */
			n := last - level - tracebackLevels2 + 1 /* levels to skip */
			L.PushFString("\n\t...\t(skipping %d levels)", n)
			b.AddValue() /* add warning about skip */
			level += n   /* and skip to last levels */
		} else {
			fv := &L1.stack[ar.ci.funcIdx]
			/* C: lua_getinfo(L1, "Slnt", &ar) */
			dbAuxGetInfo(L1, "Slnt", &ar, fv, ar.ci)
			if ar.currentline <= 0 {
				L.PushFString("\n\t%s: in ", ar.shortSrc)
			} else {
				L.PushFString("\n\t%s:%d: in ", ar.shortSrc, ar.currentline)
			}
			b.AddValue()
			L.pushFuncName(&ar, fv)
			b.AddValue()
			if ar.istailcall {
				b.AddString("\n\t(...tail calls...)")
			}
		}
		limit2show--
	}
	b.PushResult()
}

// tracebackLevels1 / tracebackLevels2 are LEVELS1 / LEVELS2 of lauxlib.c.
const (
	tracebackLevels1 = 10 /* size of the first part of the stack */
	tracebackLevels2 = 11 /* size of the second part of the stack */
)

// findField is 'findfield' (lauxlib.c 5.4.9): search objidx in the table
// on top of the stack, recursively up to 'level' levels. On success leaves
// the name on the stack and returns true. Uses the 5.4 choreography:
// push "." / replace(-3) / concat(3).
func (L *State) findField(objidx, level int) bool {
	if level == 0 || !L.IsTable(-1) {
		return false /* not found */
	}
	L.PushNil()      /* start 'next' loop */
	for L.Next(-2) { /* for each pair in table */
		if L.Type(-2) == LUA_TSTRING { /* ignore non-string keys */
			if L.RawEqual(objidx, -1) { /* found object? */
				L.Pop(1) /* remove value (but keep name) */
				return true
			} else if L.findField(objidx, level-1) { /* try recursively */
				/* stack: lib_name, lib_table, field_name (top) */
				L.PushString(".") /* place '.' between the two names */
				L.Replace(-3)     /* (in the slot occupied by the table) */
				L.Concat(3)       /* lib_name.field_name */
				return true
			}
		}
		L.Pop(1) /* remove value */
	}
	return false /* not found */
}

// pushGlobalFuncName is 'pushglobalfuncname' (lauxlib.c): search a name
// for the function of 'ar' among all loaded modules. On success leaves
// the name on the stack and returns true.
func (L *State) pushGlobalFuncName(ar *dbAr) bool {
	top := L.GetTop()
	dbGetInfo(L, "f", ar) /* push function */
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.CheckStack2(6, "not enough stack") /* slots for 'findField' */
	if L.findField(top+1, 2) {
		name := L.ToStringX(-1)
		if strings.HasPrefix(name, LUA_GNAME+".") { /* name start with '_G.'? */
			L.PushString(name[len(LUA_GNAME)+1:]) /* push name without prefix */
			L.Remove(-2)                          /* remove original name */
		}
		L.Copy(-1, top+1) /* copy name to proper place */
		L.SetTop(top + 1) /* remove table "loaded" and name copy */
		return true
	}
	L.SetTop(top) /* remove function and loaded table */
	return false
}

// globalFuncName searches a name for 'obj' among loaded modules (used by
// traceback when L != L1, so getinfo("f") on L would read the wrong stack —
// dbGetInfo indexes L.stack[ci.funcIdx], not ci's foreign StkId).
func (L *State) globalFuncName(obj *Value) (string, bool) {
	top := L.GetTop()
	L.checkStack(1)
	setobj(&L.stack[L.top], obj) /* push a copy of the function */
	L.top++
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.CheckStack2(6, "not enough stack")
	funcIdx := top + 1
	if L.findField(funcIdx, 2) {
		name := L.ToStringX(-1)
		if strings.HasPrefix(name, LUA_GNAME+".") {
			name = name[len(LUA_GNAME)+1:]
		}
		L.SetTop(top)
		return name, true
	}
	L.SetTop(top)
	return "", false
}

// pushFuncName is 'pushfuncname' (lauxlib.c): push a description of the
// function of 'ar'. 'fv' is the function value when L may differ from the
// state that filled 'ar' (traceback).
func (L *State) pushFuncName(ar *dbAr, fv *Value) {
	/* pushfuncname (lauxlib.c). Cross-state traceback cannot use
	   pushglobalfuncname/getinfo("f") on L (dbGetInfo indexes L.stack),
	   so search via fv with globalFuncName instead. */
	if name, ok := L.globalFuncName(fv); ok { /* try first a global name */
		L.PushFString("function '%s'", name)
	} else if ar.namewhat != "" { /* is there a name from code? */
		L.PushFString("%s '%s'", ar.namewhat, ar.name) /* use it */
	} else if len(ar.what) > 0 && ar.what[0] == 'm' { /* main? */
		L.PushString("main chunk")
	} else if ar.what != "C" { /* for Lua functions, use <file:line> */
		L.PushFString("function <%s:%d>", ar.shortSrc, ar.linedefined)
	} else { /* nothing left... */
		L.PushString("?")
	}
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
		L.ArgExpected(false, ud, tname)
	}
	return p
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
