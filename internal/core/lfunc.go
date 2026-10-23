// Translation of lfunc.h / lfunc.c (PUC-Lua 5.4.9).
//
// Intentional Go diffs (KEEP — do not "fix" to C shapes):
//   - Go GC: no luaC_newobj / barriers / nw2black / twups / luaF_freeproto.
//   - Index stack: UpVal uses open/idx/L/v; open list is ascending []*UpVal
//     by stack index (C: descending TValue* doubly-linked list). Close walks
//     from the high end ≡ C head of descending list.
//   - tbclist is []int of stack indices (C: delta chain on stack slots with
//     MAXDELTA dummy nodes). Semantics equivalent.
//   - Goroutine coroutines: luaF_close may migrateNested before tbc work.
package core

// ---------------------------------------------------------------------
// Types (C: UpVal / CClosure / LClosure live in lobject.h; hosted here)
// ---------------------------------------------------------------------

// UpVal is an upvalue: either open (stack index) or closed (value in v).
// C (lobject.h): TValue *v.p + union { open{next,previous} | value }.
// Go: index-stack specialization (intentional).
type UpVal struct {
	open bool
	idx  int // stack index while open (C: uplevel → StkId)
	L    *State
	v    Value // closed slot (C: u.value); open reads via upvalValue → stack
}

// CClosure is a Go function with upvalues (counterpart of a C closure).
// Plain "light C functions" may still exist elsewhere as LUA_VLCF; every
// GoFunc pushed through this path is wrapped because Go funcs are not
// comparable and cannot be table keys.
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

// ---------------------------------------------------------------------
// Header macros from lfunc.h
// ---------------------------------------------------------------------

// Header macros from lfunc.h (values / homes):
//   CLOSEKTOP (-1) → closeKTOP in lmathutil.go (relocated; same value).
//   MAXUPVAL (255) → lopcodes.go (relocated).
//   MAXMISS (10) — defined below; unused in 5.4.9 C sources (header-only).
//   sizeCclosure / sizeLclosure: N/A under Go slice allocation.
//   isintwups / upisopen / uplevel: open bool + idx (index-stack Spec); twups IGNORE (Go GC).

// MAXMISS: maximum misses before giving up the cache of closures in
// prototypes (lfunc.h). Header-only in 5.4.9 C tree.
const MAXMISS = 10

// ---------------------------------------------------------------------
// lfunc.c
// ---------------------------------------------------------------------

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
	cl.upvals = make([]*UpVal, nupvals) /* C: while (nupvals--) c->upvals[nupvals] = NULL */
	return cl
}

/*
** fill a closure with new closed upvalues
 */
// luaF_initupvals (lfunc.c). Omits luaC_newobj / objbarrier (Go GC).
func luaF_initupvals(L *State, cl *LClosure) {
	for i := 0; i < cl.nupvalues; i++ {
		uv := &UpVal{open: false} /* make it closed: value lives in uv.v */
		setnilvalue(&uv.v)
		cl.upvals[i] = uv
	}
}

/*
** Create a new upvalue at the given level, and link it into the open list.
** C: static newupval — inserts after 'prev' in a doubly-linked list and
** may link the thread into G(L)->twups. Go: insert into ascending slice;
** twups omitted (Go GC).
 */
func newupval(L *State, level int, i int) *UpVal {
	uv := &UpVal{open: true, idx: level, L: L}
	L.openupval = append(L.openupval, nil)
	copy(L.openupval[i+1:], L.openupval[i:])
	L.openupval[i] = uv
	return uv
}

/*
** Find and reuse, or create if it does not exist, an upvalue
** at the given level.
 */
// luaF_findupval (lfunc.c). Ascending index scan ≡ C descending pointer walk.
func luaF_findupval(L *State, level int) *UpVal {
	i := 0
	for i < len(L.openupval) && L.openupval[i].idx < level {
		i++
	}
	if i < len(L.openupval) && L.openupval[i].idx == level {
		return L.openupval[i] /* corresponding upvalue */
	}
	/* not found: create a new upvalue after position i */
	return newupval(L, level, i)
}

/*
** Call closing method for object 'obj' with error message 'err'. The
** boolean 'yy' controls whether the call is yieldable.
** (C assumes EXTRA_STACK; Go checkStack(3).)
 */
func callclosemethod(L *State, obj *Value, err *Value, yy bool) {
	top := L.top
	tm := luaT_gettmbyobj(L, obj, TM_CLOSE)
	L.checkStack(3)
	L.stack[top] = *tm     /* will call metamethod... */
	L.stack[top+1] = *obj  /* with 'self' as the 1st argument */
	L.stack[top+2] = *err  /* and error msg. as 2nd argument */
	L.top = top + 3        /* add function and arguments */
	if yy {
		luaD_call(L, top, 0)
	} else {
		luaD_callnoyield(L, top, 0)
	}
}

/*
** Check whether object at given level has a close metamethod and raise
** an error if not.
 */
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

/*
** Prepare and call a closing method.
** If status is CLOSEKTOP, the call to the closing method will be pushed
** at the top of the stack. Otherwise, values can be pushed right after
** the 'level' of the upvalue being closed, as everything after that
** won't be used again.
 */
func prepcallclosemth(L *State, level int, status int, yy bool) {
	uv := &L.stack[level] /* value being closed */
	var errObj *Value
	if status == closeKTOP {
		errObj = &L.g.nilValue /* error object is nil */
	} else { /* 'luaD_seterrorobj' will set top to level + 2 */
		luaD_seterrorobj(L, status, level+1) /* set error object */
		errObj = &L.stack[level+1]           /* error object goes after 'uv' */
	}
	callclosemethod(L, uv, errObj, yy)
}

// MAXDELTA (C): dependent on sizeof(tbclist.delta) — N/A; Go tbclist is []int.

/*
** Insert a variable in the list of to-be-closed variables.
 */
// luaF_newtbcupval (lfunc.c). No MAXDELTA dummy nodes; keep []int sorted ascending.
func luaF_newtbcupval(L *State, level int) {
	if isFalse(&L.stack[level]) {
		return /* false doesn't need to be closed */
	}
	checkclosemth(L, level) /* value must have a close method */
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
// C: void luaF_unlinkupval(UpVal *uv) via previous/next pointers.
// Go needs L because the open list is a slice on State (index-stack Spec).
// Not-found path marks closed (defensive; C asserts upisopen) — FLAG.
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

/*
** Close all upvalues up to the given stack level.
 */
// luaF_closeupval (lfunc.c). Omits iswhite/nw2black/luaC_barrier (Go GC).
func luaF_closeupval(L *State, level int) {
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

// poptbclist (C static): remove first tbc + dummy nodes — inlined into
// luaF_close via slice pop (no MAXDELTA dummies on Go tbclist).

/*
** Close all upvalues and to-be-closed variables up to the given stack
** level. C returns restored StkId after savestack/restorestack around
** closing methods (stack may reallocate). Go uses absolute indices, so
** no return; migrateNested is the goroutine-coroutine Spec before tbc.
 */
func luaF_close(L *State, level int, status int, yy bool) {
	// Closing can call a metamethod that yields. On a synchronous
	// coroutine that has to happen on the coroutine's goroutine, and
	// it has to happen before the to-be-closed list is consumed:
	// the opcode is re-executed there from the start.
	if len(L.tbclist) > 0 {
		L.migrateNested()
	}
	luaF_closeupval(L, level) /* first, close the upvalues */
	for len(L.tbclist) > 0 { /* traverse tbc's down to that level */
		idx := L.tbclist[len(L.tbclist)-1] /* get variable index */
		if idx < level {
			break
		}
		L.tbclist = L.tbclist[:len(L.tbclist)-1] /* poptbclist */
		prepcallclosemth(L, idx, status, yy)    /* close variable */
	}
}

// luaF_newproto creates a new prototype (lfunc.c).
// &Proto{} zero-value matches C's explicit NULL/0 field init.
func luaF_newproto(L *State) *Proto { return &Proto{} }

// luaF_freeproto omitted — Go GC owns Proto / array lifetime (intentional).

/*
** Look for n-th local variable at line 'line' in function 'func'.
** Returns "" if not found (C: NULL).
 */
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

// getLocalNameAt is a Go-only error helper (not in C lfunc). Ignores obj;
// returns first active local at savedpc-1, else "?". FLAG — Apply debt:
// covermore_test.go; may be purged on merge.
func (L *State) getLocalNameAt(obj *Value) string {
	_ = obj
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
