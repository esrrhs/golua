// Translation of lfunc.h / lfunc.c (PUC-Lua 5.3.4).
//
// Upvalues keep a stack index instead of a pointer, because the Go stack
// slice may be reallocated. Open list is a slice sorted by ascending stack
// index (C: descending TValue* linked list); close walks from the high end.
package core

// UpVal is an upvalue: either open (pointing to a stack slot) or closed.
// C (lfunc.h): TValue *v + refcount + union { open{next,touched} | value }.
// Go: index-stack specialization (intentional); no refcount / twups (Go GC).
type UpVal struct {
	open bool
	idx  int
	L    *State
	v    Value
}

// CClosure is a Go function with upvalues (the Go counterpart of a
// C closure). Plain "light C functions" are not used: every Go function
// is wrapped in a closure, because Go functions are not comparable and
// therefore cannot be used as table keys.
type CClosure struct {
	nupvalues int
	f         GoFunc
	upvalue   []Value
}

// LClosure is a Lua function with upvalues.
type LClosure struct {
	nupvalues int
	p         *Proto
	upvals    []*UpVal
}

// luaF_newproto creates a new prototype (lfunc.c).
// &Proto{} zero-value matches C's explicit NULL/0 field init.
func luaF_newproto(L *State) *Proto { return &Proto{} }

// luaF_newCclosure creates a closure for a Go function (lfunc.c).
// Go nil-fills upvalue slots (C leaves them uninitialized until caller fills).
func luaF_newCclosure(L *State, nupvals int) *CClosure {
	cl := &CClosure{nupvalues: nupvals}
	if nupvals > 0 {
		cl.upvalue = make([]Value, nupvals)
		for i := 0; i < nupvals; i++ {
			setnilvalue(&cl.upvalue[i])
		}
	}
	return cl
}

// luaF_newLclosure creates a Lua closure (lfunc.c).
func luaF_newLclosure(L *State, nupvals int) *LClosure {
	cl := &LClosure{nupvalues: nupvals}
	cl.upvals = make([]*UpVal, nupvals) /* C: while (n--) c->upvals[n] = NULL */
	return cl
}

// luaF_initupvals gives a closure fresh closed upvalues (lfunc.c).
// Omits C refcount=1 (Go GC).
func luaF_initupvals(L *State, cl *LClosure) {
	for i := 0; i < cl.nupvalues; i++ {
		uv := &UpVal{open: false} /* closed: value lives in uv.v */
		setnilvalue(&uv.v)
		cl.upvals[i] = uv
	}
}

// luaF_findupval finds (or creates) the open upvalue for a stack slot (lfunc.c).
// Open list kept sorted by ascending stack index; close walks from the high end.
// Omits C refcount/touched/twups (Go GC / index-stack Spec).
func luaF_findupval(L *State, level int) *UpVal {
	i := 0
	for i < len(L.openupval) && L.openupval[i].idx < level {
		i++
	}
	if i < len(L.openupval) && L.openupval[i].idx == level {
		return L.openupval[i] /* found a corresponding upvalue */
	}
	/* not found: create a new upvalue */
	uv := &UpVal{open: true, idx: level, L: L}
	L.openupval = append(L.openupval, nil)
	copy(L.openupval[i+1:], L.openupval[i:])
	L.openupval[i] = uv
	return uv
}

// luaF_close closes every open upvalue at or above 'level' (lfunc.c, Lua 5.3.4).
// Ascending index slice: walk from the high end ≡ C head of descending list.
// Omits C refcount==0 free and luaC_upvalbarrier (Go GC).
// No luaF_closeupval / luaF_unlinkupval — those are Lua 5.4 APIs, purged here.
func luaF_close(L *State, level int) {
	for len(L.openupval) > 0 {
		uv := L.openupval[len(L.openupval)-1]
		if uv.idx < level {
			break
		}
		uv.v = L.stack[uv.idx] /* move value to upvalue slot */
		uv.open = false        /* now current value lives here */
		uv.L = nil
		L.openupval = L.openupval[:len(L.openupval)-1]
	}
}

// luaF_getlocalname returns the name of a local variable (lfunc.c).
// "" ≡ C NULL (not found).
func luaF_getlocalname(f *Proto, localNumber, pc int) string {
	for i := 0; i < len(f.locvars) && f.locvars[i].startpc <= pc; i++ {
		if pc < f.locvars[i].endpc { /* is variable active? */
			localNumber--
			if localNumber == 0 {
				return f.locvars[i].varname.val
			}
		}
	}
	return "" /* not found */
}

// luaF_freeproto omitted — Go GC owns Proto lifetime (intentional).
