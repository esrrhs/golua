// Translation of lfunc.h / lfunc.c (PUC-Lua 5.4).
//
// Upvalues keep a stack index instead of a pointer, because the Go stack
// slice may be reallocated. To-be-closed variables are kept in a plain
// list of stack indices on the state (in C they ride on the upvalue
// list; the semantics are the same).
package core

// UpVal is an upvalue: either open (pointing to a stack slot) or closed.
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
func luaF_newproto(L *State) *Proto { return &Proto{} }

// luaF_newCclosure creates a closure for a Go function (lfunc.c).
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
	cl.upvals = make([]*UpVal, nupvals)
	return cl
}

// luaF_initupvals gives a closure fresh closed upvalues (lfunc.c).
func luaF_initupvals(L *State, cl *LClosure) {
	for i := 0; i < cl.nupvalues; i++ {
		uv := &UpVal{open: false}
		setnilvalue(&uv.v)
		cl.upvals[i] = uv
	}
}

// luaF_findupval finds (or creates) the open upvalue for a stack slot.
func luaF_findupval(L *State, level int) *UpVal {
	i := 0
	for i < len(L.openupval) && L.openupval[i].idx < level {
		i++
	}
	if i < len(L.openupval) && L.openupval[i].idx == level {
		return L.openupval[i]
	}
	uv := &UpVal{open: true, idx: level, L: L, v: L.stack[level]}
	L.openupval = append(L.openupval, nil)
	copy(L.openupval[i+1:], L.openupval[i:])
	L.openupval[i] = uv
	return uv
}

// checkclosemth raises an error if the value at 'level' has no __close
// metamethod (lfunc.c).
func checkclosemth(L *State, level int) {
	tm := luaT_gettmbyobj(L, &L.stack[level], TM_CLOSE)
	if ttisnil(tm) { /* no metamethod? */
		idx := level - L.ci.funcIdx /* variable index */
		vname := luaG_findlocal(L, L.ci, idx)
		if vname == "" {
			vname = "?"
		}
		luaG_runerror(L, "variable '%s' got a non-closable value", vname)
	}
}

// luaF_newtbcupval marks a stack slot as to-be-closed (lfunc.c).
func luaF_newtbcupval(L *State, level int) {
	if isFalse(&L.stack[level]) {
		return /* false doesn't need to be closed */
	}
	checkclosemth(L, level) /* value must have a close method */
	// keep the list sorted by increasing index
	i := 0
	for i < len(L.tbclist) && L.tbclist[i] < level {
		i++
	}
	if i < len(L.tbclist) && L.tbclist[i] == level {
		return
	}
	L.tbclist = append(L.tbclist, 0)
	copy(L.tbclist[i+1:], L.tbclist[i:])
	L.tbclist[i] = level
}

// luaF_unlinkupval removes an upvalue from the open list (lfunc.c).
func luaF_unlinkupval(L *State, uv *UpVal) {
	for i, u := range L.openupval {
		if u == uv {
			L.openupval = append(L.openupval[:i], L.openupval[i+1:]...)
			return
		}
	}
	uv.open = false
	uv.L = nil
}

// luaF_closeupval closes every upvalue at or above 'level' (lfunc.c).
func luaF_closeupval(L *State, level int) {
	for len(L.openupval) > 0 {
		uv := L.openupval[len(L.openupval)-1]
		if uv.idx < level {
			break
		}
		uv.v = L.stack[uv.idx]
		uv.open = false
		uv.L = nil
		L.openupval = L.openupval[:len(L.openupval)-1]
	}
}

// callclosemethod calls the __close metamethod of a variable (lfunc.c).
func callclosemethod(L *State, obj *Value, err *Value, yy bool) {
	top := L.top
	tm := luaT_gettmbyobj(L, obj, TM_CLOSE)
	L.checkStack(3)
	L.stack[top] = *tm
	L.stack[top+1] = *obj
	L.stack[top+2] = *err
	L.top = top + 3
	if yy {
		luaD_call(L, top, 0)
	} else {
		luaD_callnoyield(L, top, 0)
	}
}

// prepcallclosemth prepares the error object and calls __close (lfunc.c).
func prepcallclosemth(L *State, level int, status int, yy bool) {
	uv := &L.stack[level]
	var errObj *Value
	if status == closeKTOP {
		errObj = &L.g.nilValue /* error object is nil */
	} else {
		luaD_seterrorobj(L, status, level+1)
		errObj = &L.stack[level+1]
	}
	callclosemethod(L, uv, errObj, yy)
}

// luaF_close closes tbc variables and upvalues up from 'level' (lfunc.c).
func luaF_close(L *State, level int, status int, yy bool) {
	luaF_closeupval(L, level) /* first, close the upvalues */
	for len(L.tbclist) > 0 {
		idx := L.tbclist[len(L.tbclist)-1]
		if idx < level {
			break
		}
		L.tbclist = L.tbclist[:len(L.tbclist)-1]
		/* C does *not* clear the status here: every pending variable
		   is closed with the original error as its 2nd argument. */
		prepcallclosemth(L, idx, status, yy)
	}
}

// luaF_getlocalname returns the name of a local variable (lfunc.c).
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

// getLocalNameAt is a small helper for error messages.
func (L *State) getLocalNameAt(obj *Value) string {
	if L.ci != nil && isLua(L.ci) {
		cl := clLvalue(&L.stack[L.ci.funcIdx])
		f := cl.p
		pc := L.ci.savedpc - 1
		for i := 0; i < len(f.locvars); i++ {
			if f.locvars[i].startpc <= pc && pc < f.locvars[i].endpc {
				return f.locvars[i].varname.val
			}
		}
	}
	return "?"
}
