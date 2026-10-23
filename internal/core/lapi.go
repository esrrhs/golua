// Translation of lua.h / lapi.h / lapi.c (PUC-Lua 5.3.4).
//
// Every lua_* function becomes a method on *State, keeping the same
// semantics (absolute/negative indices, pseudo-indices, registry, ...).
// lua_pushcfunction/lua_pushcclosure become PushGoFunc/PushGoClosure.
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

// Insert moves the top element to position idx (lua_insert).
func (L *State) Insert(idx int) {
	p := L.indexToStack(idx)
	if p < 0 {
		panic("invalid index to 'insert'")
	}
	// (lapi.c uses the slot *at* L.top as scratch while shifting; here we
	// simply keep the old top value, which is equivalent and cannot
	// clobber itself when the shift runs.)
	top := L.stack[L.top-1]
	for i := L.top - 1; i > p; i-- {
		L.stack[i] = L.stack[i-1]
	}
	L.stack[p] = top
}

// Remove removes the element at idx (lua_remove).
func (L *State) Remove(idx int) {
	p := L.indexToStack(idx)
	if p < 0 {
		panic("invalid index to 'remove'")
	}
	for i := p; i < L.top-1; i++ {
		L.stack[i] = L.stack[i+1]
	}
	L.top--
}

// Replace moves the top element into position idx (lua_replace).
func (L *State) Replace(idx int) {
	p := index2value(L, idx)
	if !isvalid(L, p) { /* api_checkvalidindex */
		panic("invalid index")
	}
	*p = L.stack[L.top-1]
	L.top--
}

// Copy copies the value at fromIdx to toIdx (lua_copy).
func (L *State) Copy(fromIdx, toIdx int) {
	f := index2value(L, fromIdx)
	t := index2value(L, toIdx)
	if !isvalid(L, t) { /* api_checkvalidindex */
		panic("invalid index")
	}
	*t = *f
}

// Rotate rotates the stack segment [idx, top] by n (lua_rotate).
func (L *State) Rotate(idx, n int) {
	absIdx := L.indexToStack(idx)
	if absIdx < 0 {
		panic("invalid index to 'rotate'")
	}
	// simple rotation using a temporary buffer
	m := L.top - absIdx // size of the segment
	if n < 0 {
		n += m
	}
	if n < 0 || n > m {
		panic("bad rotate")
	}
	// (lua_rotate moves the last 'n' elements to the front of the
	// segment for n > 0, and the first '-n' elements to the end for
	// n < 0 -- i.e. it is a right rotation by 'n'.)
	tmp := make([]Value, m)
	copy(tmp, L.stack[absIdx:L.top])
	for i := 0; i < m; i++ {
		L.stack[absIdx+i] = tmp[(i-n+m)%m]
	}
}

// CheckStack ensures that the stack has room for n extra values.
// CheckStack is lua_checkstack (lapi.c): it makes sure there is room for
// 'n' more slots, growing the stack if needed, and reports whether that
// was possible. It never raises -- callers such as 'table.unpack' turn a
// false result into their own error message.
func (L *State) CheckStack(n int) bool {
	if L.stackLast-L.top > n { /* stack large enough? */
		return true
	}
	inuse := L.top + EXTRA_STACK
	if inuse > LUAI_MAXSTACK-n { /* can grow without overflow? */
		return false
	}
	res := L.growStackNoRaise(n, false)
	if res && L.ci != nil && L.ci.top < L.top+n {
		L.ci.top = L.top + n /* adjust frame top */
	}
	return res
}

// XMove moves n values between two states of the same global state
// (lua_xmove). Same-state is a no-op, matching C's early return.
func (L *State) XMove(from *State, n int) {
	if from == L {
		return
	}
	if L.g != from.g {
		panic("moving values across states")
	}
	L.checkStack(n)
	copy(L.stack[L.top:], from.stack[from.top-n:from.top])
	L.top += n
	from.top -= n
}

// ---------------------------------------------------------------------
// access functions
// ---------------------------------------------------------------------

// Type returns the type of the value at idx (lua_type).
func (L *State) Type(idx int) int {
	v := index2value(L, idx)
	if v == &L.g.nilValue || v == &luaO_nilobject {
		return LUA_TNONE
	}
	return ttype(v)
}

// TypeName returns the name of a type (lua_typename).
// TypeName returns the type name of the value at idx
// (luaL_typename: lua_typename(L, lua_type(L, idx))).
func (L *State) TypeName(idx int) string { return TypeName(L.Type(idx)) }

func TypeName(tp int) string {
	if tp < 0 || tp >= len(luaT_typenames)-2 {
		return "no value"
	}
	return luaT_typenames[tp+1]
}

func (L *State) IsNil(idx int) bool        { return L.Type(idx) == LUA_TNIL }
func (L *State) IsNone(idx int) bool       { return L.Type(idx) == LUA_TNONE }
func (L *State) IsNoneOrNil(idx int) bool  { t := L.Type(idx); return t == LUA_TNONE || t == LUA_TNIL }
func (L *State) IsBoolean(idx int) bool    { return L.Type(idx) == LUA_TBOOLEAN }
func (L *State) IsNumber(idx int) bool {
	var n float64
	return luaV_tonumber_(L, index2value(L, idx), &n) /* tonumber: coerce strings */
}
func (L *State) IsInteger(idx int) bool    { return ttisinteger(index2value(L, idx)) }
func (L *State) IsString(idx int) bool     { t := L.Type(idx); return t == LUA_TSTRING || t == LUA_TNUMBER }
func (L *State) IsTable(idx int) bool      { return L.Type(idx) == LUA_TTABLE }
func (L *State) IsFunction(idx int) bool   { return L.Type(idx) == LUA_TFUNCTION }
func (L *State) IsGoFunction(idx int) bool { return ttiscfunction(index2value(L, idx)) }
func (L *State) IsUserData(idx int) bool {
	t := L.Type(idx)
	return t == LUA_TUSERDATA || t == LUA_TLIGHTUSERDATA
}
func (L *State) IsThread(idx int) bool        { return L.Type(idx) == LUA_TTHREAD }
func (L *State) IsLightUserData(idx int) bool { return ttislightuserdata(index2value(L, idx)) }

func nil2() *float64 { var f float64; return &f }

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
		return luaH_getn(hvalue(v))
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
		return ptrID(gcvalue(v))
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
	setsvalue(&L.stack[L.top], luaS_newlstr(L, s))
	L.top++
}

func (L *State) PushBytes(b []byte) { L.PushString(string(b)) }

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
// get functions
// ---------------------------------------------------------------------

// GetTable pushes t[k] where t is at idx and k is on top (lua_gettable).
func (L *State) GetTable(idx int) int {
	t := index2value(L, idx)
	key := &L.stack[L.top-1]
	slot := vmFastGet(L, t, key)
	if slot != nil && !isempty(slot) {
		L.stack[L.top-1] = *slot
		return ttype(&L.stack[L.top-1])
	}
	L.stack[L.top-1] = *key
	luaV_finishget(L, t, &L.stack[L.top-1], L.top-1, slot)
	return ttype(&L.stack[L.top-1])
}

// GetField pushes t[k] for a string key (lua_getfield).
func (L *State) GetField(idx int, k string) int {
	t := index2value(L, idx)
	L.checkStack(1)
	var key Value
	setsvalue(&key, luaS_newlstr(L, k))
	slot := vmFastGet(L, t, &key)
	setsvalue(&L.stack[L.top], luaS_newlstr(L, k))
	if slot != nil && !isempty(slot) {
		L.stack[L.top] = *slot
		L.top++
		return ttype(&L.stack[L.top-1])
	}
	luaV_finishget(L, t, &L.stack[L.top], L.top, slot)
	L.top++
	return ttype(&L.stack[L.top-1])
}

// GetI pushes t[n] for an integer key (lua_geti).
func (L *State) GetI(idx int, n int64) int {
	t := index2value(L, idx)
	L.checkStack(1)
	slot := vmFastGeti(t, n)
	if slot != nil && !isempty(slot) {
		L.stack[L.top] = *slot
		L.top++
		return ttype(&L.stack[L.top-1])
	}
	var key Value
	setivalue(&key, n)
	luaV_finishget(L, t, &key, L.top, slot)
	L.top++
	return ttype(&L.stack[L.top-1])
}

// RawGet pushes t[k] without metamethods (lua_rawget).
func (L *State) RawGet(idx int) int {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	key := &L.stack[L.top-1]
	slot := luaH_get(L, hvalue(t), key)
	if isempty(slot) {
		setnilvalue(&L.stack[L.top-1])
	} else {
		L.stack[L.top-1] = *slot
	}
	return ttype(&L.stack[L.top-1])
}

// RawGetI pushes t[n] without metamethods (lua_rawgeti).
func (L *State) RawGetI(idx int, n int64) int {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	L.checkStack(1)
	slot := luaH_getint(hvalue(t), n)
	if isempty(slot) {
		setnilvalue(&L.stack[L.top])
	} else {
		L.stack[L.top] = *slot
	}
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
	slot := luaH_get(L, hvalue(t), &k)
	if isempty(slot) {
		setnilvalue(&L.stack[L.top])
	} else {
		L.stack[L.top] = *slot
	}
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

// GetUserValue is lua_getuservalue (lapi.c 5.3.4: single uservalue).
// The 'n' argument is kept for API compatibility; only n==1 is valid
// (C has no n). Out of range / not full userdata → push nil, LUA_TNONE.
func (L *State) GetUserValue(idx, n int) int {
	v := index2value(L, idx)
	L.checkStack(1)
	if !ttisfulluserdata(v) || n != 1 {
		setnilvalue(&L.stack[L.top])
		L.top++
		return LUA_TNONE
	}
	L.stack[L.top] = uvalue(v).user
	L.top++
	return ttype(&L.stack[L.top-1])
}

// ---------------------------------------------------------------------
// set functions
// ---------------------------------------------------------------------

// SetTable does t[k] = v (lua_settable).
func (L *State) SetTable(idx int) {
	t := index2value(L, idx)
	key := L.stack[L.top-2]
	val := L.stack[L.top-1]
	slot := vmFastGet(L, t, &key)
	if slot != nil && !isempty(slot) {
		*slot = val
	} else {
		luaV_finishset(L, t, &key, &val, slot)
	}
	L.top -= 2
}

// SetField does t[k] = v for a string key (lua_setfield).
func (L *State) SetField(idx int, k string) {
	t := index2value(L, idx)
	val := L.stack[L.top-1]
	var key Value
	setsvalue(&key, luaS_newlstr(L, k))
	slot := vmFastGet(L, t, &key)
	if slot != nil && !isempty(slot) {
		*slot = val
	} else {
		luaV_finishset(L, t, &key, &val, slot)
	}
	L.top--
}

// SetI does t[n] = v for an integer key (lua_seti).
func (L *State) SetI(idx int, n int64) {
	t := index2value(L, idx)
	val := L.stack[L.top-1]
	slot := vmFastGeti(t, n)
	if slot != nil && !isempty(slot) {
		*slot = val
	} else {
		var key Value
		setivalue(&key, n)
		luaV_finishset(L, t, &key, &val, slot)
	}
	L.top--
}

// RawSet does t[k] = v without metamethods (lua_rawset).
func (L *State) RawSet(idx int) {
	t := index2value(L, idx)
	if !ttistable(t) {
		panic("table expected")
	}
	key := L.stack[L.top-2]
	val := L.stack[L.top-1]
	luaH_set(L, hvalue(t), &key, &val)
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
	luaH_setint(L, hvalue(t), n, &val)
	invalidateTMcache(hvalue(t))
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
	luaH_set(L, hvalue(t), &k, &val)
	invalidateTMcache(hvalue(t))
	L.top--
}

// SetMetaTable sets the metatable of the value at idx
// (lua_setmetatable).
func (L *State) SetMetaTable(idx int) {
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
	switch ttype(v) {
	case LUA_TTABLE:
		hvalue(v).metatable = mt
		luaC_checkFinalizer(L, v, mt)
		luaC_checkWeakMode(L, hvalue(v), mt)
	case LUA_TUSERDATA:
		uvalue(v).metatable = mt
		luaC_checkFinalizer(L, v, mt)
	default:
		L.g.mt[ttype(v)] = mt
	}
	L.top--
}

// SetUserValue is lua_setuservalue (lapi.c 5.3.4: single uservalue).
// The 'n' argument is kept for API compatibility; only n==1 succeeds
// (C has no n / returns void). Returns 1 on success, 0 otherwise.
func (L *State) SetUserValue(idx, n int) int {
	v := index2value(L, idx)
	val := L.stack[L.top-1]
	if !ttisfulluserdata(v) || n != 1 {
		L.top--
		return 0
	}
	uvalue(v).user = val
	L.top--
	return 1
}

// GetGlobal pushes the value of a global (lua_getglobal).
func (L *State) GetGlobal(name string) int {
	L.checkStack(2)
	var key Value
	setsvalue(&key, luaS_newlstr(L, name))
	L.stack[L.top] = key
	L.top++
	var gv Value
	sethvalue(&gv, L.rawGetRegistryGlobals())
	slot := vmFastGet(L, &gv, &key)
	if slot != nil && !isempty(slot) {
		L.stack[L.top-1] = *slot
	} else {
		luaV_finishget(L, &gv, &key, L.top-1, slot)
	}
	return ttype(&L.stack[L.top-1])
}

// SetGlobal pops a value and assigns it to a global (lua_setglobal).
func (L *State) SetGlobal(name string) {
	var key Value
	setsvalue(&key, luaS_newlstr(L, name))
	val := L.stack[L.top-1]
	var gv Value
	sethvalue(&gv, L.rawGetRegistryGlobals())
	slot := vmFastGet(L, &gv, &key)
	if slot != nil && !isempty(slot) {
		*slot = val
	} else {
		luaV_finishset(L, &gv, &key, &val, slot)
	}
	L.top--
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
	luaH_set(L, gt, &key, &v)
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
	funcIdx := L.top - (nargs + 1)
	if k && yieldable(L) {
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
// C (yieldable path): set CIST_YPCALL, save errfunc, bare luaD_call;
// errors unwind to resume/findpcall → finishpcallk. This port runs
// coroutines on goroutines with goCont resume, so a true bare-call +
// findpcall redesign would risk breaking Yield/Resume. Safe alignment:
// mark ci.ypcall (CIST_YPCALL), keep luaD_call (not noyield) inside
// luaD_pcall so yield can cross pcall and errors stay recovered here.
func (L *State) pcallK(nargs, nresults, msgh int, k bool) int {
	funcIdx := L.top - (nargs + 1)
	var ef int
	if msgh != 0 {
		ef = L.indexToAbs(msgh)
	}
	var status int
	if k && yieldable(L) {
		ci := L.ci
		ci.ypcall = true /* CIST_YPCALL */
		defer func() { ci.ypcall = false }()
		status = luaD_pcall(L, func() {
			luaD_call(L, funcIdx, nresults)
		}, funcIdx, ef)
	} else {
		/* conventional: C's f_call via luaD_pcall (noyield) */
		status = luaD_pcall(L, func() {
			luaD_callnoyield(L, funcIdx, nresults)
		}, funcIdx, ef)
	}
	if status == LUA_OK {
		adjustresults(L, nresults)
	}
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
		return false
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

// UpvalueJoin makes upvalue n1 of f1 refer to upvalue n2 of f2 (lua_upvaluejoin).
func (L *State) UpvalueJoin(fidx1, n1, fidx2, n2 int) {
	fv1 := index2value(L, fidx1)
	fv2 := index2value(L, fidx2)
	if !ttisLclosure(fv1) || !ttisLclosure(fv2) {
		return
	}
	f1 := clLvalue(fv1)
	f2 := clLvalue(fv2)
	if n1 < 1 || n1 > f1.nupvalues || n2 < 1 || n2 > f2.nupvalues {
		return
	}
	f1.upvals[n1-1] = f2.upvals[n2-1]
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
