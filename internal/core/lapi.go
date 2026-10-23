// Translation of lua.h / lapi.h / lapi.c (PUC-Lua 5.3.4) — line-faithful C→Go.
//
// Every lua_* API becomes a method on *State (or a package helper matching
// a C static). Index model is Go's index stack (SPEC); GC barriers /
// luaC_checkGC are omitted (Go GC SPEC). Continuations are bool (no
// lua_KFunction); yieldable pcall recovers at the call site (SPEC).
// lua_pushcfunction/lua_pushcclosure → PushGoFunc/PushGoClosure.
package core

import (
	"fmt"
	"io"
	"os"
	"unsafe"
)

// ---------------------------------------------------------------------
// index translation (lapi.c)
// ---------------------------------------------------------------------

func ispseudo(i int) bool { return i <= LUA_REGISTRYINDEX }

// isupvalue reports a pseudo-index below the registry (lapi.c).
func isupvalue(i int) bool { return i < LUA_REGISTRYINDEX }

// index2value translates an API index into a stack slot.
// Invalid indices return &L.g.nilValue (C: NONVALIDVALUE / luaO_nilobject).
// Callers that write through the result must check isvalid first.
func index2value(L *State, idx int) *Value {
	ci := L.ci
	if idx > 0 {
		o := ci.funcIdx + idx
		if o < L.top {
			return &L.stack[o]
		}
		return &L.g.nilValue
	} else if !ispseudo(idx) { /* negative index */
		return &L.stack[L.top+idx]
	} else if idx == LUA_REGISTRYINDEX {
		return &L.g.registry
	} else { /* upvalues */
		i := LUA_REGISTRYINDEX - idx
		if ttislcf(&L.stack[ci.funcIdx]) {
			return &L.g.nilValue /* light C function has no upvalues */
		}
		fv := &L.stack[ci.funcIdx]
		if ttisCclosure(fv) {
			fn := clCvalue(fv)
			if i <= fn.nupvalues {
				return &fn.upvalue[i-1]
			}
			return &L.g.nilValue
		}
		return &L.g.nilValue
	}
}

// isvalid reports whether o is a real addressable slot, not the shared
// invalid-index sentinel (lapi.c: isvalid / NONVALIDVALUE).
func isvalid(L *State, o *Value) bool {
	return o != &L.g.nilValue
}

// stackIndexOf answers the 0-based stack slot an acceptable index names,
// which is what luaF_* counts in: a positive index counts from the
// frame's own base, a negative one from the top.
// It answers -1 for an index that is not a stack slot at all (the
// registry, or an upvalue).
func stackIndexOf(L *State, idx int) int {
	if idx > 0 {
		return L.ci.funcIdx + idx
	}
	if ispseudo(idx) {
		return -1
	}
	return L.top + idx
}

// AbsIndex converts an acceptable index into an absolute one.
func (L *State) AbsIndex(idx int) int {
	if idx > 0 || ispseudo(idx) {
		return idx
	}
	return L.top - L.ci.funcIdx + idx
}

// indexToAbs turns a stack index into an absolute index into L.stack
// (index2stack in lapi.c). Positive indices are relative to the current
// frame, negative ones to the current top.
func (L *State) indexToAbs(idx int) int {
	if ispseudo(idx) {
		return -1
	}
	if idx > 0 {
		return L.ci.funcIdx + idx
	}
	return L.top + idx
}

// ---------------------------------------------------------------------
// basic stack manipulation
// ---------------------------------------------------------------------

// GetTop returns the number of elements in the stack (lua_gettop).
func (L *State) GetTop() int { return L.top - (L.ci.funcIdx + 1) }

// SetTop sets the top of the stack (lua_settop).
func (L *State) SetTop(idx int) {
	if idx >= 0 {
		for L.top < L.ci.funcIdx+1+idx {
			setnilvalue(&L.stack[L.top])
			L.top++
		}
		L.top = L.ci.funcIdx + 1 + idx
	} else {
		L.top += idx + 1 /* 'subtract' index (index is negative) */
	}
}

// Pop pops n elements from the stack.
func (L *State) Pop(n int) { L.SetTop(-n - 1) }

// PushValue pushes a copy of the element at idx (lua_pushvalue).
func (L *State) PushValue(idx int) {
	v := index2value(L, idx)
	L.checkStack(1)
	L.stack[L.top] = *v
	L.top++
}

// indexToStack converts an acceptable index to an absolute stack slot.
// It returns -1 for pseudo-indices (registry, upvalues), which cannot be
// used by the stack-moving operations.
func (L *State) indexToStack(idx int) int {
	if idx > 0 {
		return L.ci.funcIdx + idx
	}
	if ispseudo(idx) {
		return -1
	}
	return L.top + idx /* negative index */
}

// Insert moves the top element to position idx (lua_insert macro → lua_rotate 1).
func (L *State) Insert(idx int) { L.Rotate(idx, 1) }

// Remove removes the element at idx (lua_remove macro → rotate -1, pop 1).
func (L *State) Remove(idx int) {
	L.Rotate(idx, -1)
	L.Pop(1)
}

// Replace moves the top element into position idx (lua_replace macro → copy, pop).
func (L *State) Replace(idx int) {
	L.Copy(-1, idx)
	L.Pop(1)
}

// Copy copies the value at fromIdx to toIdx (lua_copy).
func (L *State) Copy(fromIdx, toIdx int) {
	f := index2value(L, fromIdx)
	t := index2value(L, toIdx)
	if !isvalid(L, t) { /* api_checkvalidindex */
		panic("invalid index")
	}
	*t = *f
	if isupvalue(toIdx) {
		/* C: luaC_barrier(L, clCvalue(L->ci->func), fr); omitted (Go GC) */
	}
}

// reverse reverses stack segment [from, to] inclusive (lapi.c static reverse).
func reverse(L *State, from, to int) {
	for from < to {
		temp := L.stack[from]
		L.stack[from] = L.stack[to]
		L.stack[to] = temp
		from++
		to--
	}
}

// Rotate rotates the stack segment [idx, top] by n (lua_rotate).
// Let x = AB with |A|=n; rotate gives BA == (A^r . B^r)^r.
func (L *State) Rotate(idx, n int) {
	t := L.top - 1           /* end of stack segment being rotated */
	p := L.indexToStack(idx) /* start of segment */
	if p < 0 {
		panic("index not in the stack") /* api_checkstackindex */
	}
	seg := t - p + 1
	if n >= 0 {
		if n > seg {
			panic("invalid 'n'")
		}
	} else if -n > seg {
		panic("invalid 'n'")
	}
	var m int /* end of prefix */
	if n >= 0 {
		m = t - n
	} else {
		m = p - n - 1
	}
	reverse(L, p, m)   /* reverse the prefix with length 'n' */
	reverse(L, m+1, t) /* reverse the suffix */
	reverse(L, p, t)   /* reverse the entire segment */
}

// CheckStack is lua_checkstack (lapi.c): ensure room for n more slots.
// Never raises (C grows via luaD_rawrunprotected; Go: growStackNoRaise).
func (L *State) CheckStack(n int) bool {
	ci := L.ci
	var res bool
	if L.stackLast-L.top > n { /* stack large enough? */
		res = true
	} else {
		inuse := L.top + EXTRA_STACK
		if inuse > LUAI_MAXSTACK-n { /* can grow without overflow? */
			res = false
		} else {
			res = L.growStackNoRaise(n, false)
		}
	}
	if res && ci != nil && ci.top < L.top+n {
		ci.top = L.top + n /* adjust frame top — C does this even if already large */
	}
	return res
}

// XMove moves n values from 'from' onto L (lua_xmove).
// C: if (from == to) return; api_check same G; api_check to has room (no grow).
// Go may grow via checkStack (intentional index-stack SPEC softening).
func (L *State) XMove(from *State, n int) {
	if from == L {
		return
	}
	if L.g != from.g {
		panic("moving among independent states")
	}
	if n < 0 || n > from.GetTop() {
		panic("not enough elements in the stack") /* api_checknelems(from, n) */
	}
	L.checkStack(n) /* C: api_check(to->ci->top - to->top >= n) without grow */
	from.top -= n
	for i := 0; i < n; i++ {
		L.stack[L.top] = from.stack[from.top+i]
		L.top++
	}
}

// ---------------------------------------------------------------------
// access functions
// ---------------------------------------------------------------------

// Type returns the type of the value at idx (lua_type).
func (L *State) Type(idx int) int {
	v := index2value(L, idx)
	if !isvalid(L, v) { /* NONVALIDVALUE → LUA_TNONE */
		return LUA_TNONE
	}
	return ttype(v) /* ttnov */
}

// TypeName (package) is lua_typename(L, t): name of a type tag.
func TypeName(tp int) string {
	if tp < LUA_TNONE || tp >= LUA_NUMTAGS {
		return "no value" /* C api_check would abort; soft for Go */
	}
	if tp < 0 || tp >= len(luaT_typenames)-2 {
		return "no value"
	}
	return luaT_typenames[tp+1]
}

// TypeName (method) is luaL_typename — NOT lua_typename. Kept for
// call sites; see NOTES (lauxlib bleed living in lapi.go).
func (L *State) TypeName(idx int) string { return TypeName(L.Type(idx)) }

func (L *State) IsNil(idx int) bool       { return L.Type(idx) == LUA_TNIL }
func (L *State) IsNone(idx int) bool      { return L.Type(idx) == LUA_TNONE }
func (L *State) IsNoneOrNil(idx int) bool { t := L.Type(idx); return t == LUA_TNONE || t == LUA_TNIL }
func (L *State) IsBoolean(idx int) bool   { return L.Type(idx) == LUA_TBOOLEAN }
func (L *State) IsNumber(idx int) bool {
	var n float64
	return luaV_tonumber_(L, index2value(L, idx), &n) /* tonumber: coerce strings */
}
func (L *State) IsInteger(idx int) bool { return ttisinteger(index2value(L, idx)) }
func (L *State) IsString(idx int) bool {
	o := index2value(L, idx)
	return ttisstring(o) || cvt2str(o)
}
func (L *State) IsTable(idx int) bool      { return L.Type(idx) == LUA_TTABLE }
func (L *State) IsFunction(idx int) bool   { return L.Type(idx) == LUA_TFUNCTION }
func (L *State) IsGoFunction(idx int) bool { return ttiscfunction(index2value(L, idx)) }
func (L *State) IsUserData(idx int) bool {
	t := L.Type(idx)
	return t == LUA_TUSERDATA || t == LUA_TLIGHTUSERDATA
}
func (L *State) IsThread(idx int) bool        { return L.Type(idx) == LUA_TTHREAD }
func (L *State) IsLightUserData(idx int) bool { return ttislightuserdata(index2value(L, idx)) }

// ToNumber converts the value at idx to a float (lua_tonumberx).
func (L *State) ToNumber(idx int) (float64, bool) {
	v := index2value(L, idx)
	var n float64
	if ttisinteger(v) {
		return float64(ivalue(v)), true
	}
	if luaV_tonumber_(L, v, &n) {
		return n, true
	}
	return 0, false
}

// ToInteger converts the value at idx to an integer (lua_tointegerx).
func (L *State) ToInteger(idx int) (int64, bool) {
	v := index2value(L, idx)
	var i int64
	if luaV_tointeger(L, v, &i, F2Ieq) {
		return i, true
	}
	return 0, false
}

// ToBoolean converts the value at idx to a bool (lua_toboolean).
func (L *State) ToBoolean(idx int) bool { return !isFalse(index2value(L, idx)) }

// ToString converts the value at idx to a string (lua_tolstring /
// lua_tostring). Numbers are converted in place, exactly as in C Lua.
func (L *State) ToString(idx int) (string, bool) {
	v := index2value(L, idx)
	if ttisstring(v) {
		return tsvalue(v).val, true
	}
	if cvt2str(v) {
		luaO_tostring(L, v)
		return tsvalue(v).val, true
	}
	return "", false
}

// ToStringX is ToString without in-place conversion.
func (L *State) ToStringX(idx int) string {
	v := index2value(L, idx)
	switch ttype(v) {
	case LUA_TSTRING:
		return tsvalue(v).val
	case LUA_TNUMBER:
		return tostringbuff(v)
	}
	return ""
}

// RawLen returns the raw length of the value at idx (lua_rawlen).
func (L *State) RawLen(idx int) uint64 {
	v := index2value(L, idx)
	switch ttypetag(v) {
	case LUA_VTABLE:
		return uint64(luaH_getn(hvalue(v)))
	case LUA_VSHRSTR, LUA_VLNGSTR:
		return uint64(len(tsvalue(v).val))
	case LUA_VUSERDATA:
		/* the size in bytes of the block, which is what 'lua_rawlen'
		   answers for a userdata -- not the number of user values. An
		   extension reads it back to know how much it was given: lpeg
		   derives the size of a pattern from it. */
		return uint64(uvalue(v).len)
	}
	return 0
}

// ToGoFunction returns the Go function at idx (lua_tocfunction).
func (L *State) ToGoFunction(idx int) GoFunc {
	v := index2value(L, idx)
	if ttisCclosure(v) {
		return clCvalue(v).f
	}
	if ttislcf(v) {
		return fvalue(v)
	}
	return nil
}

// ToUserData returns the Go value carried by the userdata at idx.
func (L *State) ToUserData(idx int) any {
	v := index2value(L, idx)
	switch {
	case ttisfulluserdata(v):
		return uvalue(v).val
	case ttislightuserdata(v):
		return pvalue(v)
	}
	return nil
}

// ToPointer returns a unique identifier for the value at idx
// (lua_topointer). Matches C: NULL for strings/numbers/nil/boolean;
// TLCL / TCCL / TLCF distinguished like lapi.c.
func (L *State) ToPointer(idx int) uintptr {
	v := index2value(L, idx)
	switch ttypetag(v) {
	case LUA_TTABLE:
		return ptrID(unsafe.Pointer(hvalue(v)))
	case LUA_TLCL:
		return ptrID(unsafe.Pointer(clLvalue(v)))
	case LUA_TCCL:
		return ptrID(unsafe.Pointer(clCvalue(v)))
	case LUA_TLCF:
		/* C: cast(void *, cast(size_t, fvalue(o))) */
		return uintptr(v.p)
	case LUA_TTHREAD:
		return ptrID(unsafe.Pointer(thvalue(v)))
	case LUA_TUSERDATA:
		/* C: getudatamem(u) = (char*)u + sizeof(Udata). Go: cblock is that
		   blob when allocated for CGO; else Udata identity (Go payload SPEC). */
		u := uvalue(v)
		if u.cblock != nil {
			return ptrID(u.cblock)
		}
		return ptrID(unsafe.Pointer(u))
	case LUA_TLIGHTUSERDATA:
		return ptrID(pvalue(v))
	default:
		return 0 /* strings, numbers, nil, boolean, … */
	}
}

// ToThread returns the thread at idx (lua_tothread).
func (L *State) ToThread(idx int) *State {
	v := index2value(L, idx)
	if !ttisthread(v) {
		return nil
	}
	return thvalue(v)
}

// ---------------------------------------------------------------------
// push functions
// ---------------------------------------------------------------------

func (L *State) PushNil() {
	L.checkStack(1)
	setnilvalue(&L.stack[L.top])
	L.top++
}

func (L *State) PushNumber(n float64) {
	L.checkStack(1)
	setfltvalue(&L.stack[L.top], n)
	L.top++
}

func (L *State) PushInteger(n int64) {
	L.checkStack(1)
	setivalue(&L.stack[L.top], n)
	L.top++
}

func (L *State) PushString(s string) {
	L.checkStack(1)
	/* Go has no NULL string; C lua_pushstring(NULL) pushes nil — N/A */
	setsvalue(&L.stack[L.top], luaS_newlstr(L, s))
	L.top++
}

// PushLString is lua_pushlstring (length-explicit; empty → "").
func (L *State) PushLString(s string) string {
	L.checkStack(1)
	ts := luaS_newlstr(L, s) /* len==0 still interned empty string */
	setsvalue(&L.stack[L.top], ts)
	L.top++
	return ts.val
}

// PushBytes is a Go convenience ≈ lua_pushlstring for []byte.
func (L *State) PushBytes(b []byte) { L.PushLString(string(b)) }

// PushFString is lua_pushfstring (formats then pushes; returns the string).
func (L *State) PushFString(format string, args ...any) string {
	ts := pushfString(L, format, args...)
	L.checkStack(1)
	setsvalue(&L.stack[L.top], ts)
	L.top++
	return ts.val
}

func (L *State) PushBoolean(b bool) {
	L.checkStack(1)
	setbvalue(&L.stack[L.top], b)
	L.top++
}

func (L *State) PushLightUserData(p unsafe.Pointer) {
	L.checkStack(1)
	setpvalue(&L.stack[L.top], p)
	L.top++
}

// PushThread pushes the current thread; it returns true when it is the
// main thread (lua_pushthread).
func (L *State) PushThread() bool {
	L.checkStack(1)
	setthvalue(&L.stack[L.top], L)
	L.top++
	return L == L.g.mainthread
}

// PushGoClosure pushes a Go function with n upvalues taken from the
// stack (lua_pushcclosure).
//
// With no upvalues C does not build a closure at all: 'lua_pushcclosure'
// pushes a *light* C function, a plain value whose identity is the
// function itself. That is why 'ipairs{} == ipairs{}' is true in Lua, and
// why every standard library function compares equal to itself -- a
// per-call closure would make each of them a different object.
func (L *State) PushGoClosure(f GoFunc, n int) {
	if n == 0 {
		L.checkStack(1)
		setfvalue(&L.stack[L.top], f)
		L.top++
		return
	}
	/* C: api_checknelems(L, n); api_check(n <= MAXUPVAL) */
	if n < 0 || n > L.GetTop() {
		panic("not enough elements in the stack")
	}
	if n > MAXUPVAL {
		panic("upvalue index too large")
	}
	cl := luaF_newCclosure(L, n)
	cl.f = f
	L.checkStack(1)
	L.top -= n
	for i := 0; i < n; i++ {
		cl.upvalue[i] = L.stack[L.top+i]
	}
	setclCvalue(&L.stack[L.top], cl)
	L.top++
}

// PushGoFunc pushes a Go function with no upvalues (lua_pushcfunction).
func (L *State) PushGoFunc(f GoFunc) { L.PushGoClosure(f, 0) }

// PushGlobalTable pushes the global table (lua_pushglobaltable).
func (L *State) PushGlobalTable() {
	L.checkStack(1)
	L.RawGetI(LUA_REGISTRYINDEX, LUA_RIDX_GLOBALS)
}

// ---------------------------------------------------------------------
// get functions (Lua → stack) — lapi.c
// ---------------------------------------------------------------------

// luaV_gettable expands the C macro: fastget via luaH_get, else finishget.
func luaV_gettable(L *State, t *Value, keyIdx, valIdx int) {
	var slot *Value
	if ttistable(t) {
		slot = luaH_get(L, hvalue(t), &L.stack[keyIdx])
		if !ttisnil(slot) {
			L.stack[valIdx] = *slot
			return
		}
	} else {
		slot = nil
	}
	luaV_finishget(L, t, &L.stack[keyIdx], valIdx, slot)
}

// auxgetstr is lapi.c auxgetstr: intern k once, fastget via luaH_getstr.
func auxgetstr(L *State, t *Value, k string) int {
	str := luaS_newlstr(L, k)
	var slot *Value
	if ttistable(t) {
		slot = luaH_getstr(L, hvalue(t), str)
		if !ttisnil(slot) {
			L.checkStack(1)
			L.stack[L.top] = *slot
			L.top++
			return ttype(&L.stack[L.top-1])
		}
	} else {
		slot = nil
	}
	L.checkStack(1)
	setsvalue(&L.stack[L.top], str)
	L.top++
	luaV_finishget(L, t, &L.stack[L.top-1], L.top-1, slot)
	return ttype(&L.stack[L.top-1])
}

// GetTable pushes t[k] where t is at idx and k is on top (lua_gettable).
func (L *State) GetTable(idx int) int {
	t := index2value(L, idx)
	luaV_gettable(L, t, L.top-1, L.top-1)
	return ttype(&L.stack[L.top-1])
}

// GetField pushes t[k] for a string key (lua_getfield → auxgetstr).
func (L *State) GetField(idx int, k string) int {
	return auxgetstr(L, index2value(L, idx), k)
}

// GetI pushes t[n] for an integer key (lua_geti).
func (L *State) GetI(idx int, n int64) int {
	t := index2value(L, idx)
	var slot *Value
	if ttistable(t) {
		slot = luaH_getint(hvalue(t), n)
		if !ttisnil(slot) {
			L.checkStack(1)
			L.stack[L.top] = *slot
			L.top++
			return ttype(&L.stack[L.top-1])
		}
	} else {
		slot = nil
	}
	L.checkStack(1)
	setivalue(&L.stack[L.top], n)
	L.top++
	luaV_finishget(L, t, &L.stack[L.top-1], L.top-1, slot)
	return ttype(&L.stack[L.top-1])
}

// RawGet pushes t[k] without metamethods (lua_rawget).
func (L *State) RawGet(idx int) int {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	/* C: setobj2s(L, L->top - 1, luaH_get(...)); absent → luaO_nilobject (nil) */
	L.stack[L.top-1] = *luaH_get(L, hvalue(t), &L.stack[L.top-1])
	return ttype(&L.stack[L.top-1])
}

// RawGetI pushes t[n] without metamethods (lua_rawgeti).
func (L *State) RawGetI(idx int, n int64) int {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	L.checkStack(1)
	L.stack[L.top] = *luaH_getint(hvalue(t), n) /* absent → nilobject */
	L.top++
	return ttype(&L.stack[L.top-1])
}

// RawGetP pushes t[p] for a light userdata key (lua_rawgetp).
func (L *State) RawGetP(idx int, p unsafe.Pointer) int {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	L.checkStack(1)
	var k Value
	setpvalue(&k, p)
	L.stack[L.top] = *luaH_get(L, hvalue(t), &k) /* absent → nilobject */
	L.top++
	return ttype(&L.stack[L.top-1])
}

// CreateTable creates a new empty table (lua_createtable).
func (L *State) CreateTable(narr, nrec int) {
	t := luaH_new(L)
	L.checkStack(1)
	sethvalue(&L.stack[L.top], t)
	L.top++
	if narr > 0 || nrec > 0 {
		luaH_resize(L, t, uint32(narr), uint32(nrec))
	}
}

// NewTable creates a table with no preallocated space.
func (L *State) NewTable() { L.CreateTable(0, 0) }

// NewUserData creates a full userdata wrapping a Go value
// (lua_newuserdata). C 5.3.4 allocates a size_t byte blob with exactly
// one uservalue. Go keeps val any as the intentional port of that blob;
// nuvalue is accepted for API compatibility but must be 0 or 1 — a
// userdata always has one uservalue slot (nil until set), never a 5.4
// multi-uv array.
func (L *State) NewUserData(val any, nuvalue int) any {
	if nuvalue < 0 || nuvalue > 1 {
		panic("NewUserData: Lua 5.3.4 allows at most one uservalue")
	}
	u := luaS_newudata(L, val)
	L.checkStack(1)
	setuvalue(&L.stack[L.top], u)
	L.top++
	return val
}

// GetMetaTable pushes the metatable of the value at idx
// (lua_getmetatable); it returns false when there is none.
func (L *State) GetMetaTable(idx int) bool {
	v := index2value(L, idx)
	var mt *Table
	switch ttype(v) {
	case LUA_TTABLE:
		mt = hvalue(v).metatable
	case LUA_TUSERDATA:
		mt = uvalue(v).metatable
	default:
		mt = L.g.mt[ttype(v)]
	}
	if mt == nil {
		return false
	}
	L.checkStack(1)
	sethvalue(&L.stack[L.top], mt)
	L.top++
	return true
}

// GetUserValue is lua_getuservalue (lapi.c 5.3.4: single uservalue, no n).
// Extra 'n' is Go API bleed toward 5.4 multi-uv; only n==1 is the 5.3 path.
// C api_check(fulluserdata) — Go: n!=1 or not full → push nil, LUA_TNONE.
func (L *State) GetUserValue(idx, n int) int {
	v := index2value(L, idx)
	L.checkStack(1)
	if !ttisfulluserdata(v) || n != 1 {
		setnilvalue(&L.stack[L.top])
		L.top++
		return LUA_TNONE
	}
	L.stack[L.top] = uvalue(v).user /* getuservalue */
	L.top++
	return ttype(&L.stack[L.top-1])
}

// ---------------------------------------------------------------------
// set functions (stack → Lua) — lapi.c
// ---------------------------------------------------------------------

// luaV_settable expands the C macro: fastset via luaH_get, else finishset.
func luaV_settable(L *State, t *Value, keyIdx, valIdx int) {
	var slot *Value
	if ttistable(t) {
		slot = luaH_get(L, hvalue(t), &L.stack[keyIdx])
		if !ttisnil(slot) {
			*slot = L.stack[valIdx] /* luaC_barrierback omitted (Go GC) */
			return
		}
	} else {
		slot = nil
	}
	luaV_finishset(L, t, &L.stack[keyIdx], &L.stack[valIdx], slot)
}

// auxsetstr is lapi.c auxsetstr: intern k once, fastset via luaH_getstr.
func auxsetstr(L *State, t *Value, k string) {
	str := luaS_newlstr(L, k)
	var slot *Value
	if ttistable(t) {
		slot = luaH_getstr(L, hvalue(t), str)
		if !ttisnil(slot) {
			*slot = L.stack[L.top-1]
			L.top-- /* pop value */
			return
		}
	} else {
		slot = nil
	}
	L.checkStack(1)
	setsvalue(&L.stack[L.top], str) /* push 'str' (to make it a TValue) */
	L.top++
	luaV_finishset(L, t, &L.stack[L.top-1], &L.stack[L.top-2], slot)
	L.top -= 2 /* pop value and key */
}

// SetTable does t[k] = v (lua_settable).
func (L *State) SetTable(idx int) {
	t := index2value(L, idx)
	luaV_settable(L, t, L.top-2, L.top-1)
	L.top -= 2 /* pop index and value */
}

// SetField does t[k] = v for a string key (lua_setfield → auxsetstr).
func (L *State) SetField(idx int, k string) {
	auxsetstr(L, index2value(L, idx), k)
}

// SetI does t[n] = v for an integer key (lua_seti).
func (L *State) SetI(idx int, n int64) {
	t := index2value(L, idx)
	var slot *Value
	if ttistable(t) {
		slot = luaH_getint(hvalue(t), n)
		if !ttisnil(slot) {
			*slot = L.stack[L.top-1]
			L.top-- /* pop value */
			return
		}
	} else {
		slot = nil
	}
	L.checkStack(1)
	setivalue(&L.stack[L.top], n)
	L.top++
	luaV_finishset(L, t, &L.stack[L.top-1], &L.stack[L.top-2], slot)
	L.top -= 2 /* pop value and key */
}

// RawSet does t[k] = v without metamethods (lua_rawset).
func (L *State) RawSet(idx int) {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	key := L.stack[L.top-2]
	val := L.stack[L.top-1]
	*luaH_set(L, hvalue(t), &key) = val
	invalidateTMcache(hvalue(t))
	L.top -= 2
}

// RawSetI does t[n] = v without metamethods (lua_rawseti).
func (L *State) RawSetI(idx int, n int64) {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	val := L.stack[L.top-1]
	luaH_setint(L, hvalue(t), n, &val) /* C: no invalidateTMcache on rawseti */
	/* luaC_barrierback omitted (Go GC) */
	L.top--
}

// RawSetP does t[p] = v for a light userdata key (lua_rawsetp).
func (L *State) RawSetP(idx int, p unsafe.Pointer) {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	val := L.stack[L.top-1]
	var k Value
	setpvalue(&k, p)
	*luaH_set(L, hvalue(t), &k) = val /* C: no invalidateTMcache on rawsetp */
	/* luaC_barrierback omitted (Go GC) */
	L.top--
}

// SetMetaTable sets the metatable of the value at idx (lua_setmetatable).
// Returns 1 like C (always succeeds when api_check passes).
func (L *State) SetMetaTable(idx int) int {
	v := index2value(L, idx)
	mtv := &L.stack[L.top-1]
	var mt *Table
	if ttisnil(mtv) {
		mt = nil
	} else {
		if !ttistable(mtv) {
			panic("table expected")
		}
		mt = hvalue(mtv)
	}
	switch ttype(v) { /* ttnov */
	case LUA_TTABLE:
		hvalue(v).metatable = mt
		if mt != nil {
			luaC_checkFinalizer(L, v, mt)        /* barrier omitted (Go GC) */
			luaC_checkWeakMode(L, hvalue(v), mt) /* Go weak-table bridge */
		}
	case LUA_TUSERDATA:
		uvalue(v).metatable = mt
		if mt != nil {
			luaC_checkFinalizer(L, v, mt)
		}
	default:
		L.g.mt[ttype(v)] = mt
	}
	L.top--
	return 1
}

// SetUserValue is lua_setuservalue (lapi.c 5.3.4: void, single uv, no n).
// Extra 'n' + int return are Go/5.4-shaped; only n==1 is the 5.3 path.
// Returns 1 on success, 0 otherwise (C would api_check and not return).
func (L *State) SetUserValue(idx, n int) int {
	v := index2value(L, idx)
	val := L.stack[L.top-1]
	if !ttisfulluserdata(v) || n != 1 {
		L.top--
		return 0
	}
	uvalue(v).user = val /* setuservalue; barrier omitted (Go GC) */
	L.top--
	return 1
}

// GetGlobal pushes the value of a global (lua_getglobal → auxgetstr on _G).
func (L *State) GetGlobal(name string) int {
	reg := hvalue(&L.g.registry)
	gt := luaH_getint(reg, LUA_RIDX_GLOBALS)
	return auxgetstr(L, gt, name)
}

// SetGlobal pops a value and assigns it to a global (lua_setglobal → auxsetstr).
func (L *State) SetGlobal(name string) {
	reg := hvalue(&L.g.registry)
	gt := luaH_getint(reg, LUA_RIDX_GLOBALS)
	auxsetstr(L, gt, name)
}

func (L *State) rawGetRegistryGlobals() *Table {
	reg := hvalue(&L.g.registry)
	slot := luaH_getint(reg, LUA_RIDX_GLOBALS)
	if !ttistable(slot) {
		panic("globals table missing")
	}
	return hvalue(slot)
}

// SetGlobalValue sets a global from a Go value.
func (L *State) SetGlobalValue(name string, v Value) {
	L.checkStack(2)
	gt := L.rawGetRegistryGlobals()
	var key Value
	setsvalue(&key, luaS_newlstr(L, name))
	*luaH_set(L, gt, &key) = v
}

// ---------------------------------------------------------------------
// load and call
// ---------------------------------------------------------------------

// Call calls a function (lua_call). Like C's 'lua_call' it passes no
// continuation, so the call is made non-yieldable: a 'coroutine.yield'
// inside the called function raises "attempt to yield across a C-call
// boundary".
/* adjustresults (lapi.h): when MULTRET, raise ci.top to L.top. */
func adjustresults(L *State, nres int) {
	if nres == LUA_MULTRET && L.ci.top < L.top {
		L.ci.top = L.top
	}
}

func (L *State) Call(nargs, nresults int) {
	L.callK(nargs, nresults, false)
}

// CallK calls a function the way C's 'lua_callk' does when the caller
// supplies a continuation function: in a thread that can yield the call
// is made yieldable, so a 'coroutine.yield' inside it (even inside a Go
// function called from it) suspends the coroutine instead of raising
// "attempt to yield across a C-call boundary".  'pairs' and 'dofile'
// are the two library functions that need it.
//
// Call is the right choice for everything else: it reproduces C's
// 'lua_call', which never lets a yield escape.
func (L *State) CallK(nargs, nresults int) {
	L.callK(nargs, nresults, true)
}

// callK is lua_callk. 'k' tells whether the caller supplied a
// continuation function; that is how 'pairs' (lbaselib.c passes
// 'pairscont') and 'dofile' ('dofilecont') ask for a *yieldable* call,
// so that a 'coroutine.yield' can cross them.
func (L *State) callK(nargs, nresults int, k bool) {
	/* C: api_check(L->status == LUA_OK); api_checknelems(nargs+1); checkresults */
	if L.status != LUA_OK {
		panic("cannot do calls on non-normal thread")
	}
	if nargs+1 > L.GetTop() {
		panic("not enough elements in the stack")
	}
	funcIdx := L.top - (nargs + 1)
	if k && yieldable(L) { /* C: k != NULL && L->nny == 0 */
		luaD_call(L, funcIdx, nresults)
	} else {
		luaD_callnoyield(L, funcIdx, nresults)
	}
	adjustresults(L, nresults)
}

// PCall calls a function in protected mode (lua_pcall). Like C's
// 'lua_pcall' it passes no continuation, so the call is never yieldable.
func (L *State) PCall(nargs, nresults, msgh int) int {
	return L.pcallK(nargs, nresults, msgh, false)
}

// pcallK is lua_pcallk. 'k' tells whether the caller supplied a
// continuation function; that is how 'pcall' and 'xpcall' ask for a
// *yieldable* protected call (lbaselib.c passes 'finishpcall' as the
// continuation).
//
// C (lapi.c): when k!=NULL && nny==0, save errfunc/old allowhook on ci,
// set CIST_YPCALL, then bare luaD_call (not luaD_pcall). Errors longjmp
// to resume → findpcall → recover. Non-yieldable path uses luaD_pcall
// + f_call (noyield).
//
// This port: non-yieldable path matches C. Yieldable path now also uses
// bare luaD_call with CIST_YPCALL + saved errfunc/allowhook (same shape
// as cdlopen_ext.lua_pcallk). Errors are still recovered at the call
// site — goroutine/goCont keep the frame alive — rather than by a
// resume-time findpcall. Remaining gap: true findpcall-on-resume (errors
// do not abandon pcallK the way C longjmp does).
func (L *State) pcallK(nargs, nresults, msgh int, k bool) int {
	/* C: api_check(L->status == LUA_OK); api_checknelems(nargs+1) */
	if L.status != LUA_OK {
		panic("cannot do calls on non-normal thread")
	}
	if nargs+1 > L.GetTop() {
		panic("not enough elements in the stack")
	}
	funcIdx := L.top - (nargs + 1)
	var ef int
	if msgh != 0 {
		ef = L.indexToAbs(msgh)
	}
	var status int
	if k && yieldable(L) {
		ci := L.ci
		ci.oldErrFunc = L.errfunc
		L.errfunc = ef
		setoah(&ci.callstatus, L.allowhook)
		ci.ypcall = true /* CIST_YPCALL */
		status = LUA_OK
		oldNCcalls := L.nCcalls
		func() {
			L.errorJmp++ /* C: resume's errorJmp covers unprotected call */
			defer func() {
				L.errorJmp--
				L.nCcalls = oldNCcalls
				r := recover()
				if r == nil {
					return
				}
				e, ok := r.(*luaError)
				if !ok {
					panic(r) /* yieldAbort / migrateAbort / closeSignal */
				}
				status = e.status
				/* re-push like rawRunProtected: a Go defer may have popped it */
				if L.top >= len(L.stack) {
					L.reallocStack(len(L.stack) + EXTRA_STACK)
				}
				L.stack[L.top] = e.value
				L.top++
			}()
			luaD_call(L, funcIdx, nresults) /* bare call, not luaD_pcall */
		}()
		ci.ypcall = false
		L.errfunc = ci.oldErrFunc
		if status != LUA_OK {
			/* ldo.c recover (finish interrupted protected call) */
			L.ci = ci
			L.allowhook = getoah(ci.callstatus)
			L.nny = 0
			luaF_close(L, funcIdx)
			seterrorobj(L, status, funcIdx)
			L.shrinkStack()
		}
	} else {
		/* conventional: C's f_call via luaD_pcall (noyield) */
		status = luaD_pcall(L, func() {
			luaD_callnoyield(L, funcIdx, nresults)
		}, funcIdx, ef)
	}
	adjustresults(L, nresults) /* C always adjusts, including after error */
	return status
}

// LoadBytes compiles a chunk, pushing the resulting function on the stack
// (lua_load / luaL_loadbufferx).
func (L *State) LoadBytes(data []byte, chunkname string) error {
	return L.LoadBytesMode(data, chunkname, "bt")
}

// LoadBytesMode is LoadBytes with an explicit mode string, the way
// 'lua_load' takes one (luaL_loadbufferx). C's 'f_parser' dispatches on
// the first byte and then checks the mode: a byte equal to
// LUA_SIGNATURE[0] requires 'b', anything else requires 't'.
func (L *State) LoadBytesMode(data []byte, chunkname, mode string) error {
	if len(data) > 0 && data[0] == dmpSignature[0] {
		/* precompiled chunk: 'lua_load' calls luaU_undump instead of
		   the parser (ldo.c f_parser) */
		return luaU_loadBinary(L, data, chunkname, mode)
	}
	return L.LoadStream(data, nil, chunkname, mode)
}

// LoadStream compiles a chunk that arrives in pieces, the way 'lua_load'
// reads a reader function through a ZIO (lzio.c): 'first' is what has
// been read so far and 'more' returns the next piece, or nil at the end
// of the stream.
//
// The pieces are pulled only as the parser needs them -- which is what
// makes 'load' answer with a syntax error, instead of reading for ever,
// when the function never returns nil: C's lexer reaches the malformed
// token long before the reader runs dry.
func (L *State) LoadStream(first []byte, more func() []byte, chunkname, mode string) error {
	var dyd Dyndata
	oldtop := L.top
	incnny(L) /* cannot yield during parsing */
	status := L.rawRunProtected(func() {
		checkLoadMode(L, mode, "text")
		cl := luaY_parser(L, first, &dyd, chunkname, more)
		if cl.nupvalues >= 1 {
			/* set the globals table as the first upvalue (the _ENV) */
			var gt Value
			sethvalue(&gt, L.rawGetRegistryGlobals())
			setobj(&cl.upvals[0].v, &gt)
		}
		L.checkStack(1)
		setclLvalue(&L.stack[L.top], cl)
		L.top++
	})
	decnny(L)
	if status != LUA_OK {
		msg := ""
		if L.top > oldtop {
			msg = valueToStr(&L.stack[L.top-1])
		}
		L.top = oldtop
		return &LuaError{Status: status, Message: msg}
	}
	return nil
}

// LoadString compiles a string as a Lua chunk (luaL_loadstring).
func (L *State) LoadString(s, chunkname string) error {
	return L.LoadBytes([]byte(s), chunkname)
}

// LoadFile compiles a Lua file (luaL_loadfile).
func (L *State) LoadFile(path string) error {
	return L.LoadFileMode(path, "bt")
}

// LoadFileMode compiles a Lua file with an explicit mode, the way
// 'luaL_loadfilex' hands its 'mode' argument to 'lua_load'.  Opening a
// file is not the same as compiling it: 'loadfile(name, "b")' on a text
// file answers "attempt to load a text chunk (mode is 'b')" just as
// 'load' would, and 'loadfile(name, "t")' on a precompiled chunk answers
// the binary form of that message.
//
// 'bt', the mode 'LoadFile' uses, is what C's NULL 'mode' amounts to:
// 'checkmode' is skipped for NULL, and "bt" allows both kinds of chunk.
func (L *State) LoadFileMode(path, mode string) error {
	/* C opens the file and then reads from it, and the two can fail
	   differently -- 'fopen' on a directory succeeds on some systems
	   and it is the read that reports "Is a directory". */
	f, err := os.Open(path)
	if err != nil {
		return &LuaError{Status: LUA_ERRFILE, Message: ioFileError("open", path, err)}
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return &LuaError{Status: LUA_ERRFILE, Message: ioFileError("read", path, err)}
	}
	/* 'skipcomment' is part of loading a *file*, not of loading a chunk:
	   'load("#")' is a syntax error, while a file starting with
	   "#!/usr/bin/lua" is a valid script. */
	return L.LoadBytesMode(skipFileComment(data), "@"+path, mode)
}

// skipFileComment drops a leading UTF-8 BOM and, when what follows starts
// with '#', the rest of that first line, keeping the newline so that line
// numbers stay correct (skipBOM/skipcomment in lauxlib.c).
func skipFileComment(data []byte) []byte {
	const bom = "\xEF\xBB\xBF"
	if len(data) >= len(bom) && string(data[:len(bom)]) == bom {
		data = data[len(bom):] /* BOM is discarded only when complete */
	}
	if len(data) > 0 && data[0] == '#' { /* Unix exec. file? */
		i := 0
		for i < len(data) && data[i] != '\n' {
			i++
		}
		return data[i:] /* keep the newline, if any */
	}
	return data
}

// DoString loads and runs a chunk of Lua code (luaL_dostring).
func (L *State) DoString(s string) error {
	if err := L.LoadString(s, "=chunk"); err != nil {
		return err
	}
	if status := L.PCall(0, LUA_MULTRET, 0); status != LUA_OK {
		v := &L.stack[L.top-1]
		err := &LuaError{Status: status, Message: valueToStr(v)}
		L.Pop(1)
		return err
	}
	return nil
}

// DoFile loads and runs a Lua file (luaL_dofile).
func (L *State) DoFile(path string) error {
	/* luaL_dofile is luaL_loadfile and then a call, and loading a file
	   is not the same as loading its text: a file may start with a
	   "#!" line, which 'load' would call a syntax error and a file
	   reader skips. Reading the bytes here and handing them to
	   DoString would take the second view -- lpeg's own test suite,
	   like most Lua scripts, starts with "#!/usr/bin/env lua". */
	if err := L.LoadFile(path); err != nil {
		return err
	}
	if status := L.PCall(0, LUA_MULTRET, 0); status != LUA_OK {
		v := &L.stack[L.top-1]
		err := &LuaError{Status: status, Message: valueToStr(v)}
		L.Pop(1)
		return err
	}
	return nil
}

// LuaError is the error returned by the Go-facing helpers.
type LuaError struct {
	Status  int
	Message string
}

func (e *LuaError) Error() string { return e.Message }

// Error raises a Lua error with the value on top of the stack (lua_error).
func (L *State) Error() int {
	luaG_errormsg(L)
	return 0
}

// ---------------------------------------------------------------------
// arith / compare / stringtonumber (lapi.c)
// ---------------------------------------------------------------------

// Arith performs arithmetic op on the stack (lua_arith).
func (L *State) Arith(op int) {
	if op == LUA_OPUNM || op == LUA_OPBNOT {
		L.CheckStack(1)
		L.PushValue(-1) /* unary: fake 2nd operand */
	}
	res := L.top - 2
	luaO_arith(L, op, &L.stack[L.top-2], &L.stack[L.top-1], res)
	L.top-- /* remove second operand */
}

// Compare compares values at idx1/idx2 with op LUA_OPEQ/OPLT/OPLE (lua_compare).
func (L *State) Compare(idx1, idx2, op int) bool {
	o1 := index2value(L, idx1)
	o2 := index2value(L, idx2)
	if !isvalid(L, o1) || !isvalid(L, o2) {
		return false
	}
	switch op {
	case LUA_OPEQ:
		return luaV_equalobj(L, o1, o2)
	case LUA_OPLT:
		return luaV_lessthan(L, o1, o2)
	case LUA_OPLE:
		return luaV_lessequal(L, o1, o2)
	default:
		panic("invalid option") /* C api_check */
	}
}

// StringToNumber converts s and pushes the number (lua_stringtonumber).
// Returns the size of the string plus one on success, or 0 on failure —
// matching C's return of (e - s) + 1 from luaO_str2num.
func (L *State) StringToNumber(s string) int {
	L.checkStack(1)
	if !luaO_str2num(s, &L.stack[L.top]) {
		return 0
	}
	L.top++
	return len(s) + 1
}

// GetUpvalue pushes upvalue n of the function at funcindex (lua_getupvalue).
// Returns the upvalue name, or "" when there is none / out of range.
func (L *State) GetUpvalue(funcindex, n int) (name string, ok bool) {
	return dbGetUpvalue(L, funcindex, n)
}

// SetUpvalue pops a value into upvalue n (lua_setupvalue).
func (L *State) SetUpvalue(funcindex, n int) (name string, ok bool) {
	return dbSetUpvalue(L, funcindex, n)
}

// UpvalueId returns an identity for an upvalue (lua_upvalueid).
func (L *State) UpvalueId(funcindex, n int) unsafe.Pointer {
	return dbUpvalueID(L, funcindex, n)
}

// getupvalref is lapi.c getupvalref (Lua closure upvalue pointer).
func getupvalref(L *State, fidx, n int) **UpVal {
	fi := index2value(L, fidx)
	if !ttisLclosure(fi) {
		panic("Lua function expected")
	}
	f := clLvalue(fi)
	/* C: 1 <= n && n <= f->p->sizeupvalues */
	if f.p == nil || n < 1 || n > f.p.sizeupvalues {
		panic("invalid upvalue index")
	}
	return &f.upvals[n-1]
}

// UpvalueJoin makes upvalue n1 of f1 refer to upvalue n2 of f2 (lua_upvaluejoin).
// refcount / touched / upvalbarrier omitted (Go GC SPEC).
func (L *State) UpvalueJoin(fidx1, n1, fidx2, n2 int) {
	up1 := getupvalref(L, fidx1, n1)
	up2 := getupvalref(L, fidx2, n2)
	*up1 = *up2
}

// ---------------------------------------------------------------------
// comparison and misc
// ---------------------------------------------------------------------

// Equal compares two values (lua_compare with LUA_OPEQ).
func (L *State) Equal(idx1, idx2 int) bool {
	o1 := index2value(L, idx1)
	o2 := index2value(L, idx2)
	if !isvalid(L, o1) || !isvalid(L, o2) {
		return false
	}
	return luaV_equalobj(L, o1, o2)
}

// RawEqual compares two values without metamethods (lua_rawequal).
func (L *State) RawEqual(idx1, idx2 int) bool {
	o1 := index2value(L, idx1)
	o2 := index2value(L, idx2)
	if !isvalid(L, o1) || !isvalid(L, o2) {
		return false
	}
	return luaV_rawequalobj(o1, o2)
}

// LessThan compares two values with '<' (lua_compare with LUA_OPLT).
func (L *State) LessThan(idx1, idx2 int) bool {
	o1 := index2value(L, idx1)
	o2 := index2value(L, idx2)
	if !isvalid(L, o1) || !isvalid(L, o2) {
		return false
	}
	return luaV_lessthan(L, o1, o2)
}

// Concat concatenates the n values on top of the stack (lua_concat).
func (L *State) Concat(n int) {
	if n >= 2 {
		luaV_concat(L, n)
	} else if n == 0 {
		L.checkStack(1)
		setsvalue(&L.stack[L.top], luaS_newlstr(L, ""))
		L.top++
	}
	/* else n == 1: nothing to do */
}

// Len returns the length of the value at idx, honouring __len
// (lua_len / luaL_len).
func (L *State) Len(idx int) {
	v := index2value(L, idx)
	L.checkStack(1)
	luaV_objlen(L, L.top, v)
	L.top++
}

// Next pops a key and pushes the next key/value pair of the table at idx
// (lua_next).
func (L *State) Next(idx int) bool {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	keyIdx := L.top - 1
	if luaH_next(L, hvalue(t), keyIdx) {
		L.top++
		return true
	}
	L.top-- /* remove the key */
	return false
}

// Status returns the status of the state (lua_status).
func (L *State) Status() int { return int(L.status) }

// Dump writes the Lua function on top of the stack as a binary chunk
// (lua_dump). writer returns 0 on success; non-zero aborts. strip drops debug.
func (L *State) Dump(writer func(L *State, b []byte) int, strip bool) int {
	if L.GetTop() < 1 {
		panic("not enough elements in the stack") /* api_checknelems(L, 1) */
	}
	return luaDump(L, dumpWriter(writer), strip)
}

// Version returns LUA_VERSION_NUM (lua_version).
// C: L==NULL → &static; else G(L)->version. Go has no g.version pointer;
// both paths return the 5.3.4 constant (503).
func Version(L *State) float64 {
	_ = L
	return float64(luaVersionNumGo)
}

// IsYieldable reports whether the thread can yield (lua_isyieldable).
// The main thread and any call made through luaD_callnoyield (a
// metamethod, a 'for' iterator, 'table.sort', ...)
// are not yieldable.
func (L *State) IsYieldable() bool { return yieldable(L) }

// Where returns a string describing the current position (luaL_where).
func (L *State) Where(level int) string {
	ci := L.ci
	for level > 0 && ci != nil && ci != &L.baseCI {
		level--
		ci = ci.prev
	}
	if ci == nil || !isLua(ci) {
		return ""
	}
	cl := clLvalue(&L.stack[ci.funcIdx])
	line := currentLine(L, ci)
	if line <= 0 {
		return "" /* no information available */
	}
	/* a stripped chunk ('string.dump(f, true)') has no source, exactly
	   like C's 'funcinfo', which uses "=?" in that case */
	src := "=?"
	if cl.p.source != nil {
		src = cl.p.source.val
	}
	return fmt.Sprintf("%s:%d: ", luaO_chunkid(src), line)
}
