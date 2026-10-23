// Translation of lauxlib.h / lauxlib.c (PUC-Lua 5.4.9).
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
		arg-- /* do not count 'self' */
		if arg == 0 { /* error is in the self argument itself? */
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

// luaVersionNumGo is LUA_VERSION_NUM (5.4 → 504). Kept here so non-cgo builds
// can check versions the same way as the C export in cdlopen.go.
const luaVersionNumGo = 504

// CheckVersion_ checks that the running core matches the version and
// numeric type sizes the caller was compiled against (luaL_checkversion_,
// lauxlib.c 5.4.9). A mismatch raises via Error2 — it must not be a silent
// no-op. 5.4.9 no longer compares lua_version(L) against lua_version(NULL)
// for multi-VM detection (that check existed in 5.3).
func (L *State) CheckVersion_(ver float64, sz uintptr) {
	if sz != luaLNumSizes { /* check numeric types */
		L.Error2("core and library have incompatible numeric types")
	} else if float64(luaVersionNumGo) != ver {
		L.Error2("version mismatch: app. needs %f, Lua core provides %f",
			ver, float64(luaVersionNumGo))
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
	/* luaL_typeerror (lauxlib.c): use a description of the actual value.
	   C leaves __name on the stack; ArgError never returns. */
	var typearg string
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		/* lua_tostring / %s cuts a __name that contains a zero byte. */
		typearg = luaCStr(L.ToStringX(-1))
	} else if L.Type(arg) == LUA_TLIGHTUSERDATA {
		typearg = "light userdata" /* special name for messages */
	} else {
		typearg = TypeName(L.Type(arg)) /* standard name */
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

// Buffer is the Go counterpart of luaL_Buffer.
//
// The content lives in one block, the way it does in PUC-Lua: 'b' is its
// address, 'n' how much of it is used, 'size' how much it holds, and
// luaL_addchar is a macro in <lauxlib.h> that writes into 'b[n++]'
// without calling anything at all. So do luaL_addsize and luaL_buffsub,
// and so are luaL_bufflen and luaL_buffaddr macros over 'n' and 'b'.
//
// C 5.4 buffinit always pushes a lightuserdata placeholder; on growth it
// replaces that with a UBox userdata marked to-be-closed. This port keeps
// an intentional dual path (see NOTES): pure-Go buffers use a []byte
// stand-in with no stack placeholder; cgo (cblock) allocates addressable
// memory via reallocBlock and a finalizer instead of UBox.__gc/__close.
type Buffer struct {
	L      *State
	boxIdx int     // index of the box (userdata) when one is on the stack
	boxPtr uintptr // address of the box, not a reference the GC follows

	scratch unsafe.Pointer // the block, when there is one
	slen    int            // how much it holds
	n       int            // how much of it is used
	buf     []byte         // the stand-in, when there is no block

	// cblock is whether this buffer's content has to live in the block.
	// Only luaL_buffinit via the cgo export says so: it hands the address
	// out to C. A buffer the port itself makes — table.concat's,
	// utf8.char's — never does.
	cblock bool
}

// bufInitSize is LUAL_BUFFERSIZE for 5.4.9:
// ((int)(16 * sizeof(void*) * sizeof(lua_Number))) → 1024 on LP64.
const bufInitSize = int(16 * unsafe.Sizeof(uintptr(0)) * unsafe.Sizeof(float64(0)))

// reallocBlock replaces a buffer's block with one of 'sz' bytes, copying
// the content across; 'old' is nil on the first call. It is set by the
// cgo layer, which is the only place that can make memory C may hold.
var reallocBlock func(old unsafe.Pointer, sz int) unsafe.Pointer

// bufFree frees a buffer's block once the buffer is unreachable. Like
// reallocBlock, only the cgo layer can do it.
var bufFree func(b *Buffer)

func bufFinalize(b *Buffer) {
	if bufFree != nil {
		bufFree(b)
	}
}

// NewBuffer creates a new string buffer (luaL_buffinit).
// Pure-Go path: starts on an empty []byte with nothing on the Lua stack
// (FLAG — C 5.4 pushes a lightuserdata placeholder; see NOTES). The cgo
// export sets cblock and calls room so addressable memory exists before
// C writes through luaL_prepbuffsize.
func (L *State) NewBuffer() *Buffer {
	return &Buffer{L: L}
}

// ensureBox pushes the UBox userdata C creates in newbox(), and records
// its address so PushResult / AddValue can find it under the top.
// Go-GC: finalizer stands in for UBox __gc / __close; no toclose mark.
func (b *Buffer) ensureBox() {
	if b.boxPtr != 0 {
		return
	}
	L := b.L
	L.checkStack(1)
	u := luaS_newudata(L, 0, nil)
	setuvalue(&L.stack[L.top], u)
	L.top++
	b.boxIdx = L.AbsIndex(-1)
	b.boxPtr = uintptr(unsafe.Pointer(u))
}

// room answers the 'sz' bytes just past the content, making the block
// bigger if it has to. It does not advance 'n': that is AddString's and
// AddSize's business, because an extension that asked for room says how
// much of it it used. Growth matches 5.4 newbuffsize: size*1.5.
func (b *Buffer) room(sz int) []byte {
	need := b.n + sz
	/* C newbuffsize: if (MAX_SIZET - sz < B->n) → "buffer too large" */
	if sz < 0 || need < b.n {
		b.L.Error2("buffer too large")
	}
	if !b.cblock || reallocBlock == nil {
		/* no C memory in this build, and nothing to hand an address to */
		if len(b.buf) < need {
			newsize := (len(b.buf) / 2) * 3 /* buffer size * 1.5 */
			if newsize < need {
				newsize = need
			}
			if newsize < bufInitSize {
				newsize = bufInitSize
			}
			grown := make([]byte, newsize)
			copy(grown, b.buf[:b.n])
			b.buf = grown
		}
		return b.buf[b.n:need]
	}
	if b.scratch != nil && need <= b.slen {
		return unsafe.Slice((*byte)(b.scratch), b.slen)[b.n:need]
	}
	newsz := (b.slen / 2) * 3 /* as lauxlib.c grows it: one and a half */
	if newsz < need {
		newsz = need
	}
	if newsz < bufInitSize {
		newsz = bufInitSize
	}
	b.ensureBox() /* first box, like C newbox() when leaving initb */
	p := reallocBlock(b.scratch, newsz)
	if p == nil && newsz > 0 {
		/* C 5.4 resizebox: "not enough memory" then lua_error */
		b.L.Error2("not enough memory")
	}
	if b.scratch == nil && p != nil {
		runtime.SetFinalizer(b, bufFinalize) /* Go GC stand-in for UBox.__gc */
	}
	b.scratch = p
	b.slen = newsz
	return unsafe.Slice((*byte)(p), newsz)[b.n:need]
}

// AddString is luaL_addstring.
func (b *Buffer) AddString(s string) {
	b.AddBytes([]byte(s))
}

// AddBytes is luaL_addlstring (length-explicit; Go []byte stand-in).
func (b *Buffer) AddBytes(p []byte) {
	if len(p) > 0 { /* avoid copy when p can be empty/nil */
		copy(b.room(len(p)), p)
		b.AddSize(len(p))
	}
}

// AddChar is luaL_addchar.
func (b *Buffer) AddChar(c byte) {
	b.room(1)[0] = c
	b.n++
}

// AddValue adds the value on top of the stack, converted to a string
// (luaL_addvalue). Matches C: lua_tolstring then addlstring into a prep
// that treats the box (if any) as being under the value (-2). A
// non-coercible value contributes zero bytes (no Go-only error).
func (b *Buffer) AddValue() {
	L := b.L
	s, _ := L.ToString(-1)
	onBox := false
	if b.boxPtr != 0 && L.GetTop() >= 2 {
		v := &L.stack[L.top-2]
		if ttisfulluserdata(v) && uintptr(unsafe.Pointer(uvalue(v))) == b.boxPtr {
			onBox = true
			/* C 5.4 prepbuffsize(..., -2): box stays under the string */
		}
	}
	b.AddString(s)
	if onBox {
		L.Pop(1) /* pop string; box remains */
	} else {
		L.Pop(1)
	}
}

func (b *Buffer) Len() int { return b.n }

// BuffSub shrinks the used length (luaL_buffsub).
func (b *Buffer) BuffSub(s int) {
	if s > 0 && s <= b.n {
		b.n -= s
	}
}

// String answers what the buffer holds, without touching it.
func (b *Buffer) String() string {
	if b.n == 0 {
		return ""
	}
	if b.scratch != nil {
		return string(unsafe.Slice((*byte)(b.scratch), b.n))
	}
	return string(b.buf[:b.n])
}

// PushResult finishes the buffer and pushes the resulting string
// (luaL_pushresult). When a box is on the stack it is removed, matching
// C's remove of box or placeholder.
func (b *Buffer) PushResult() {
	L := b.L
	L.PushString(b.String())
	b.n = 0
	if b.boxPtr != 0 && L.GetTop() >= 2 {
		v := &L.stack[L.top-2]
		if ttisfulluserdata(v) && uintptr(unsafe.Pointer(uvalue(v))) == b.boxPtr {
			/* C: if (buffonstack) lua_closeslot(-2); then lua_remove(-2).
			   Go-GC finalizer owns the block; just remove the box. */
			L.Remove(-2)
		}
	}
	b.boxPtr = 0
	b.boxIdx = 0
}

// PushResultSize is luaL_pushresultsize: addsize then pushresult.
func (b *Buffer) PushResultSize(sz int) {
	b.AddSize(sz)
	b.PushResult()
}

// PrepBuffSize is luaL_prepbuffsize: returns a slice of at least sz free bytes.
func (b *Buffer) PrepBuffSize(sz int) []byte { return b.room(sz) }

// BuffInitSize is luaL_buffinitsize: buffinit then prepbuffsize.
func (L *State) BuffInitSize(sz int) (*Buffer, []byte) {
	b := L.NewBuffer()
	return b, b.PrepBuffSize(sz)
}

// AddSize says that 'n' more bytes were written into the room
// PrepBuffSize handed out (luaL_addsize).
func (b *Buffer) AddSize(n int) {
	if n > 0 {
		b.n += n
	}
}

// AddGSub replaces every occurrence of p in s with r into the buffer
// (luaL_addgsub — new in 5.4).
func (b *Buffer) AddGSub(s, p, r string) {
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

// Traceback pushes a traceback of the current stack (luaL_traceback).
// 'hasMsg' is C's 'msg != NULL': when a message is given it is written
// first, followed by a newline — and that happens even for the empty
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

// findField is 'findfield' (lauxlib.c 5.4.9): it searches 'obj' in the
// table on top of the stack, recursively up to 'level' levels. On success
// it returns the name, which is left pushed on the stack. Uses the 5.4
// choreography: push "." / replace(-3) / concat(3).
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
				/* stack: lib_name, lib_table, field_name (top) */
				L.PushString(".") /* place '.' between the two names */
				L.Replace(-3)     /* (in the slot occupied by the table) */
				L.Concat(3)       /* lib_name.field_name */
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
	   it searches — which can reallocate the stack and leave the
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
			fmt.Fprintf(&sb, "\n\t...\t(skipping %d levels)", n)
			level += n /* and skip to the last levels */
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
			if !L.RawEqual(-1, -2) { /* not the same? */
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
