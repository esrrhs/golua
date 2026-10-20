// Translation of lauxlib.h / lauxlib.c (PUC-Lua 5.4).
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
	if extramsg == "" {
		return L.Error2("bad argument #%d", arg)
	}
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

// Error2 raises a Lua error with a formatted message (luaL_error).
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

// CheckAny raises an error when the argument is absent (luaL_checkany).
// C only asks whether there *is* a value -- not what it is -- so the
// message is "bad argument #n to 'f' (value expected)".
func (L *State) CheckAny(arg int) { L.checkNElems(arg) }

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

// CheckTable checks that an argument is a table.
func (L *State) CheckTable(arg int) {
	if !L.IsTable(arg) {
		L.tagError(arg, LUA_TTABLE)
	}
}

// CheckFunction checks that an argument is a function.
func (L *State) CheckFunction(arg int) {
	if !L.IsFunction(arg) {
		L.tagError(arg, LUA_TFUNCTION)
	}
}

// CheckUserData checks that an argument is a full userdata.
func (L *State) CheckUserData(arg int) *Udata {
	v := index2value(L, arg)
	if !ttisfulluserdata(v) {
		L.tagError(arg, LUA_TUSERDATA)
	}
	return uvalue(v)
}

// CheckOption checks a string argument against a list of options
// (luaL_checkoption).
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
	// use a description of the actual value for a better message
	var typearg string
	named := false
	if L.GetMetaField(arg, "__name") == LUA_TSTRING {
		/* C's luaL_getmetafield leaves the value pushed only when it
		   finds one, and luaL_typeerror then reads it from there as a
		   'const char *', which is where a __name with a zero byte in
		   it is cut short. */
		typearg = luaCStr(L.ToStringX(-1))
		named = true
	} else if L.Type(arg) == LUA_TLIGHTUSERDATA {
		typearg = "light userdata"
	} else {
		typearg = TypeName(L.Type(arg))
	}
	msg := fmt.Sprintf("%s expected, got %s", tname, typearg)
	if named {
		L.Pop(1)
	}
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
	if L.GetMetaField(obj, name) == LUA_TNIL { /* no metafield? */
		return false
	}
	L.PushValue(obj)
	L.Call(1, 1)
	return true
}

func (L *State) checkNElems(n int) {
	if L.GetTop() < n {
		L.ArgError(n, "value expected")
	}
}

// ArgCheck raises an error when cond is false (luaL_argcheck).
func (L *State) ArgCheck(cond bool, arg int, extramsg string) {
	if !cond {
		L.ArgError(arg, extramsg)
	}
}

// ArgExpected raises an error when cond is false, naming the type that
// was expected and the type that came (luaL_argexpected).  It is what
// 'setmetatable' and 'debug.setmetatable' use for a second argument that
// may be one of two types: "nil or table expected, got number".
func (L *State) ArgExpected(cond bool, arg int, tname string) {
	if !cond {
		L.typeError(arg, tname)
	}
}

// Where pushes a string with the current position (luaL_where).
func (L *State) Where2(level int) {
	s := L.Where(level)
	L.PushString(s)
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

// luaLFreeList is the slot luaL_ref keeps its free list in: 'freelist'
// in lauxlib.c, which is LUA_RIDX_LAST + 1 -- not 0, which is where 5.3
// kept it. It is a plain key of the table the caller handed in, so an
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
// stack (luaL_setfuncs).
func (L *State) SetFuncs(l []Reg, nup int) {
	L.checkStack(nup)
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

// NewLib creates a new table and registers the functions of l into it
// (luaL_newlib).
func (L *State) NewLib(l []Reg) {
	L.CreateTable(0, len(l))
	L.SetFuncs(l, 0)
}

// ---------------------------------------------------------------------
// string buffer (lauxlib.c luaL_Buffer)
// ---------------------------------------------------------------------

// Buffer is the Go counterpart of luaL_Buffer.
//
// The content lives in one block, the way it does in PUC-Lua: 'b' is its
// address, 'n' how much of it is used, 'size' how much it holds, and
// luaL_addchar is a macro in <lauxlib.h> that writes into 'b[n++]'
// without calling anything at all. So do luaL_addsize and luaL_buffsub,
// and so are luaL_bufflen and luaL_buffaddr macros over 'n' and 'b'.
// An extension compiled against PUC-Lua's own header uses those macros
// as they are, and they have to describe the content however it was
// built -- which rules out keeping what luaL_addstring added somewhere
// of its own: luaL_bufflen would then answer only what was written
// through the address, and luaL_buffaddr an address with none of it in.
//
// The block has to be memory C may hold, because luaL_prepbuffsize hands
// out its address and the extension writes into it long after the call
// returned; that is what 'reallocBlock' is for, and in a build without
// cgo -- where nothing can ask for the address -- a Go slice stands in.
type Buffer struct {
	L      *State
	boxIdx int     // index of the "box" (a userdata marking the buffer)
	boxPtr uintptr // address of the box, not a reference the GC follows

	scratch unsafe.Pointer // the block, when there is one
	slen    int            // how much it holds
	n       int            // how much of it is used
	buf     []byte         // the stand-in, when there is no block

	// cblock is whether this buffer's content has to live in the block.
	// Only luaL_buffinit says so: it is the one that hands the address
	// out to C. A buffer the port itself makes -- table.concat's,
	// utf8.char's -- never does, and it would pay for a C allocation
	// and a finalizer on every call if it did.
	cblock bool
}

// bufInitSize is the size of the first block: LUAL_BUFFERSIZE.
const bufInitSize = 8192

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
func (L *State) NewBuffer() *Buffer {
	b := &Buffer{L: L}
	L.checkStack(2)
	u := luaS_newudata(L, 0, nil)
	setuvalue(&L.stack[L.top], u)
	L.top++
	b.boxIdx = L.AbsIndex(-1)
	b.boxPtr = uintptr(unsafe.Pointer(u))
	return b
}

// room answers the 'sz' bytes just past the content, making the block
// bigger if it has to. It does not advance 'n': that is AddString's and
// AddSize's business, because an extension that asked for room says how
// much of it it used.
func (b *Buffer) room(sz int) []byte {
	need := b.n + sz
	if !b.cblock || reallocBlock == nil {
		/* no C memory in this build, and nothing to hand an address to */
		if len(b.buf) < need {
			grown := make([]byte, (len(b.buf)/2)*3)
			if len(grown) < need {
				grown = make([]byte, need)
			}
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
	p := reallocBlock(b.scratch, newsz)
	if p == nil && newsz > 0 {
		panic("lua: out of memory for a string buffer")
	}
	if b.scratch == nil && p != nil {
		runtime.SetFinalizer(b, bufFinalize)
	}
	b.scratch = p
	b.slen = newsz
	return unsafe.Slice((*byte)(p), newsz)[b.n:need]
}

func (b *Buffer) AddString(s string) {
	if len(s) > 0 {
		copy(b.room(len(s)), s)
		b.n += len(s)
	}
}
func (b *Buffer) AddBytes(p []byte) {
	if len(p) > 0 {
		copy(b.room(len(p)), p)
		b.n += len(p)
	}
}
func (b *Buffer) AddChar(c byte) {
	b.room(1)[0] = c
	b.n++
}
func (b *Buffer) AddRune(r rune) { b.AddString(string(r)) }

// AddValue adds the value on top of the stack, converted to a string
// (luaL_addvalue).
func (b *Buffer) AddValue() {
	s, ok := b.L.ToString(-1)
	if !ok {
		b.L.Error2("cannot convert value to string")
	}
	b.L.Pop(1)
	b.AddString(s)
}

func (b *Buffer) Len() int { return b.n }

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
// (luaL_pushresult).
func (b *Buffer) PushResult() {
	b.L.PushString(b.String())
	b.n = 0
}

// AddSize says that 'n' more bytes were written into the room
// PrepBuffer handed out (luaL_addsize).
func (b *Buffer) AddSize(n int) {
	if n > 0 {
		b.n += n
	}
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
