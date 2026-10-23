// Translation of lvm.h / lvm.c (PUC-Lua 5.4).
//
// The instruction dispatch loop follows lvm.c closely. Two things are
// gone: the GC steps, and 'luaV_finishOp' (there is nothing to finish,
// because a metamethod that yields simply blocks the coroutine's
// goroutine instead of longjmp-ing out of the interpreter).
package core

import (
	"math"
	"strconv"
	"strings"
)

const MAXTAGLOOP = 2000

// upvalValue returns the current value of an upvalue (open upvalues live
// in the stack of their thread).
func upvalValue(uv *UpVal) *Value {
	if uv.open {
		return &uv.L.stack[uv.idx]
	}
	return &uv.v
}

/*
luaV_modcore and luaV_idivcore are the arithmetic of Lua's '%' and '//'

	for a divisor that is neither 0 nor -1. They are separate from
	luaV_mod and luaV_idiv for one reason: those two can raise, and the
	cost of the call that raises is what keeps them over the compiler's
	inline budget -- and '%' and '//' are the slowest arithmetic
	operations, so it is exactly them that the interpreter loop wants
	inlined. The loop therefore tests the divisor itself and only falls
	back to luaV_mod / luaV_idiv for the two divisors that need care.
*/
func luaV_modcore(m, n int64) int64 {
	r := m % n
	if r != 0 && (r^n) < 0 { /* 'm/n' would be non-integer negative? */
		r += n
	}
	return r
}

func luaV_idivcore(m, n int64) int64 {
	q := m / n
	if (m^n) < 0 && m%n != 0 { /* 'm/n' would be negative non-integer? */
		q--
	}
	return q
}

func luaV_idiv(L *State, m, n int64) int64 {
	if uint64(n)+1 <= 1 { /* special cases: -1 or 0 */
		if n == 0 {
			luaG_runerror(L, "attempt to divide by zero")
			return 0
		}
		return -m /* n == -1; avoid overflow with mininteger // -1 */
	}
	return luaV_idivcore(m, n)
}

func luaV_mod(L *State, m, n int64) int64 {
	if uint64(n)+1 <= 1 { /* special cases: -1 or 0 */
		if n == 0 {
			luaG_runerror(L, "attempt to perform 'n%%0'")
			return 0
		}
		return 0 /* m % -1 == 0; avoid overflow */
	}
	return luaV_modcore(m, n)
}

// luaV_modf is the float modulus (lvm.c). Unlike the integer version it
// never raises: 'luai_nummod' is plain 'fmod', so '1.0 % 0.0' is NaN.
func luaV_modf(L *State, m, n float64) float64 {
	r := math2Fmod(m, n)
	/* luai_nummod: give the result the same sign as the divisor.  The
	   test must not multiply the two operands, or it misfires when the
	   product underflows to zero (as it does for very small numbers). */
	if (r > 0 && n < 0) || (r < 0 && n > 0) {
		r += n
	}
	return r
}

const NBITS = 64

// luaV_shiftl shifts x left by y (a negative y shifts right).
func luaV_shiftl(x, y int64) int64 {
	if y < 0 { /* shift right? */
		if y <= -NBITS {
			return 0
		}
		return int64(uint64(x) >> uint64(-y))
	}
	if y >= NBITS {
		return 0
	}
	return int64(uint64(x) << uint64(y))
}

// luaV_concat concatenates the 'total' values on top of the stack.
func luaV_concat(L *State, total int) {
	for {
		top := L.top
		n := 2
		o1 := &L.stack[top-2]
		o2 := &L.stack[top-1]
		if total == 2 && ttisstring(o1) && ttisinteger(o2) {
			/* Formatting an integer directly after the string avoids
			   interning a temporary decimal string only to copy it into
			   the concatenation. OP_CONCAT still performs its one GC
			   check after this returns. */
			var scratch [128]byte
			out := append(scratch[:0], tsvalue(o1).val...)
			out = strconv.AppendInt(out, o2.i, 10)
			setsvalue(&L.stack[top-2], luaS_newlstr(L, string(out)))
			n = 2
		} else if !(ttisstring(o1) || cvt2str(o1)) || !tostringInPlace(L, o2) {
			luaT_tryconcatTM(L)
		} else if isEmptyStr(o2) { /* second operand is empty? */
			tostringInPlace(L, o1) /* result is first operand */
		} else if isEmptyStr(o1) { /* first operand is empty string? */
			L.stack[top-2] = L.stack[top-1] /* result is second operand */
		} else if total == 2 && tostringInPlace(L, o1) {
			/* Two strings is the common opcode. Joining them directly
			   skips the slice and the builder the general path needs
			   when it is gathering an arbitrary run. Remember the last
			   pair too: interning otherwise discovers the same result
			   only after a+b has allocated its temporary string. */
			a, b := tsvalue(o1), tsvalue(o2)
			r := L.concatR
			if a != L.concatA || b != L.concatB || r == nil {
				r = luaS_newlstr(L, a.val+b.val)
				L.concatA, L.concatB, L.concatR = a, b, r
			}
			setsvalue(&L.stack[top-2], r)
			n = 2
		} else {
			/* at least two non-empty string values; get as many as possible */
			parts := make([]string, 0, total)
			totalLen := 0
			parts = append(parts, tsvalue(o2).val)
			totalLen += len(tsvalue(o2).val)
			for n = 1; n < total && tostringInPlace(L, &L.stack[top-n-1]); n++ {
				s := tsvalue(&L.stack[top-n-1]).val
				parts = append(parts, s)
				totalLen += len(s)
			}
			var sb strings.Builder
			sb.Grow(totalLen)
			for i := len(parts) - 1; i >= 0; i-- {
				sb.WriteString(parts[i])
			}
			setsvalue(&L.stack[top-n], luaS_newlstr(L, sb.String()))
		}
		total -= n - 1 /* got 'n' strings to create one new */
		L.top -= n - 1 /* popped 'n' strings and pushed one */
		if total <= 1 {
			return
		}
	}
}

func isEmptyStr(v *Value) bool {
	return ttisstring(v) && len(tsvalue(v).val) == 0
}

// tostringInPlace converts a number to a string, in place.
func tostringInPlace(L *State, o *Value) bool {
	if ttisstring(o) {
		return true
	}
	if cvt2str(o) {
		luaO_tostring(L, o)
		return true
	}
	return false
}

// luaV_finishget finishes a table access 'val = t[key]' (lvm.c).
func luaV_finishget(L *State, t, key *Value, val int, slot *Value) {
	L.migrateNested()
	for loop := 0; loop < MAXTAGLOOP; loop++ {
		var tm *Value
		if slot == nil { /* 't' is not a table? */
			tm = luaT_gettmbyobj(L, t, TM_INDEX)
			if notm(tm) {
				luaG_typeerror(L, t, "index") /* no metamethod */
				return
			}
		} else { /* 't' is a table */
			tm = fasttm(L, hvalue(t).metatable, TM_INDEX)
			if tm == nil { /* no metamethod? */
				setnilvalue(&L.stack[val]) /* result is nil */
				return
			}
		}
		if ttisfunction(tm) { /* is metamethod a function? */
			luaT_callTMres(L, tm, t, key, val)
			return
		}
		t = tm /* else try to access 'tm[key]' */
		slot = vmFastGet(L, t, key)
		if slot != nil && !isempty(slot) {
			L.stack[val] = *slot
			return
		}
	}
	luaG_runerror(L, "'__index' chain too long; possible loop")
}

// luaV_finishset finishes a table assignment 't[key] = val' (lvm.c).
func luaV_finishset(L *State, t, key, val *Value, slot *Value) {
	L.migrateNested()
	for loop := 0; loop < MAXTAGLOOP; loop++ {
		var tm *Value
		if slot != nil { /* is 't' a table? */
			h := hvalue(t)
			tm = fasttm(L, h.metatable, TM_NEWINDEX)
			if tm == nil { /* no metamethod? */
				luaH_finishset(L, h, key, slot, val)
				invalidateTMcache(h)
				return
			}
		} else { /* not a table; check metamethod */
			tm = luaT_gettmbyobj(L, t, TM_NEWINDEX)
			if notm(tm) {
				luaG_typeerror(L, t, "index")
				return
			}
		}
		if ttisfunction(tm) { /* is metamethod a function? */
			luaT_callTM(L, tm, t, key, val)
			return
		}
		t = tm /* else repeat assignment over 'tm' */
		slot = vmFastGet(L, t, key)
		if slot != nil && !isempty(slot) {
			*slot = *val
			return
		}
	}
	luaG_runerror(L, "'__newindex' chain too long; possible loop")
}

// vmFastGet does a raw table access, returning the slot (nil when 't' is
// not a table).
func vmFastGet(L *State, t, key *Value) *Value {
	if !ttistable(t) {
		return nil
	}
	h := hvalue(t)
	switch ttypetag(key) {
	case LUA_VSHRSTR:
		return luaH_getshortstr(h, tsvalue(key))
	case LUA_VNUMINT:
		return luaH_getint(h, ivalue(key))
	case LUA_VNUMFLT:
		if k, ok := floatToIntEq(fltvalue(key)); ok {
			return luaH_getint(h, k)
		}
	}
	return luaH_get(L, h, key)
}

// vmFastGeti is the integer specialization of vmFastGet (luaV_fastgeti).
func vmFastGeti(t *Value, key int64) *Value {
	if !ttistable(t) {
		return nil
	}
	h := hvalue(t)
	if uint64(key)-1 < uint64(h.alimit) {
		return &h.array[key-1]
	}
	return luaH_getint(h, key)
}

// getcached returns p.cache if it has the same upvalues the new closure would.
func getcached(p *Proto, encup []*UpVal, L *State, base int) *LClosure {
	c := p.cache
	if c == nil {
		return nil
	}
	for i := 0; i < p.sizeupvalues; i++ {
		uv := c.upvals[i]
		if p.upvalues[i].instack != 0 {
			level := base + int(p.upvalues[i].idx)
			if uv == nil || !uv.open || uv.L != L || uv.idx != level {
				return nil
			}
		} else if uv != encup[p.upvalues[i].idx] {
			return nil
		}
	}
	return c
}

// pushclosure creates a new Lua closure, initializes upvalues, and caches it.
func pushclosure(L *State, p *Proto, encup []*UpVal, base, ra int) {
	nup := p.sizeupvalues
	uv := p.upvalues
	ncl := luaF_newLclosure(L, nup)
	ncl.p = p
	setclLvalue(&L.stack[ra], ncl)
	for i := 0; i < nup; i++ {
		if uv[i].instack != 0 { /* upvalue refers to local variable? */
			ncl.upvals[i] = luaF_findupval(L, base+int(uv[i].idx))
		} else { /* get upvalue from enclosing function */
			ncl.upvals[i] = encup[uv[i].idx]
		}
	}
	p.cache = ncl /* 5.3: reuse when upvalues match (no GC color check here) */
}

// forlimit prepares the limit of a numerical 'for' loop (lvm.c).
func forlimit(L *State, init int64, lim *Value, p *int64, step int64) bool {
	/* the limit is rounded *towards* the direction the loop runs:
	   floor for an ascending loop, ceil for a descending one */
	mode := F2Ifloor
	if step < 0 {
		mode = F2Iceil
	}
	if !luaV_tointeger(L, lim, p, mode) {
		var flim float64
		if !luaV_tonumber_(L, lim, &flim) {
			luaG_forerror(L, lim, "limit")
			return true
		}
		if flim > 0 {
			if step < 0 {
				return true
			}
			*p = LUA_MAXINTEGER
		} else {
			if step > 0 {
				return true
			}
			*p = LUA_MININTEGER
		}
	}
	if step > 0 {
		return init > *p
	}
	return init < *p
}

// forprep prepares a numerical 'for' loop; it returns true to skip it.
func forprep(L *State, ra int) bool {
	pinit := &L.stack[ra]
	plimit := &L.stack[ra+1]
	pstep := &L.stack[ra+2]
	if ttisinteger(pinit) && ttisinteger(pstep) { /* integer loop? */
		init := ivalue(pinit)
		step := ivalue(pstep)
		var limit int64
		if step == 0 {
			luaG_runerror(L, "'for' step is zero")
			return true
		}
		setivalue(&L.stack[ra+3], init) /* control variable */
		if forlimit(L, init, plimit, &limit, step) {
			return true /* skip the loop */
		}
		var count uint64
		if step > 0 { /* ascending loop? */
			count = uint64(limit) - uint64(init)
			if step != 1 {
				count /= uint64(step)
			}
		} else { /* step < 0; descending loop */
			count = uint64(init) - uint64(limit)
			count /= uint64(-(step + 1)) + 1
		}
		setivalue(plimit, int64(count))
		return false
	}
	/* else try making all values floats */
	var init, limit, step float64
	if !luaV_tonumber_(L, plimit, &limit) {
		luaG_forerror(L, plimit, "limit")
	}
	if !luaV_tonumber_(L, pstep, &step) {
		luaG_forerror(L, pstep, "step")
	}
	if !luaV_tonumber_(L, pinit, &init) {
		luaG_forerror(L, pinit, "initial value")
	}
	if step == 0 {
		luaG_runerror(L, "'for' step is zero")
		return true
	}
	/* Written the way C writes it -- '0 < step ? limit < init
	   : init < limit' -- and not as the pair of conjunctions that
	   reads the same, because the two stop being the same when step
	   is a NaN: a NaN is neither greater nor less than zero, so C
	   takes the *else* branch and 'for i = 0, 1, 0/0' does not run,
	   while 'step <= 0' takes neither branch and runs it once. */
	skip := false
	if step > 0 {
		skip = limit < init
	} else {
		skip = init < limit
	}
	if skip {
		return true /* skip the loop */
	}
	setfltvalue(plimit, limit)
	setfltvalue(pstep, step)
	setfltvalue(&L.stack[ra], init)   /* internal index */
	setfltvalue(&L.stack[ra+3], init) /* control variable */
	return false
}

// floatforloop executes a step of a float numerical 'for' loop.
func floatforloop(L *State, ra int) bool {
	step := fltvalue(&L.stack[ra+2])
	limit := fltvalue(&L.stack[ra+1])
	idx := fltvalue(&L.stack[ra])
	idx += step
	/* The same ternary as 'forprep', for the same reason: with a NaN
	   step, C asks 'limit <= idx' and not 'idx <= limit'. */
	goon := false
	if step > 0 {
		goon = idx <= limit
	} else {
		goon = limit <= idx
	}
	if goon {
		chgfltvalue(&L.stack[ra], idx)
		setfltvalue(&L.stack[ra+3], idx)
		return true /* jump back */
	}
	return false /* finish the loop */
}

// savestate saves the current pc and puts the stack top at the end of
// the current frame, exactly like the 'savestate' macro of lvm.c. Every
// operation that may push temporaries on the stack (metamethod calls,
// finishing a table access, comparisons, ...) must run with L.top above
// the frame's registers, otherwise the temporary frame would clobber the
// registers that are also the operands of the operation.
func savestate(L *State, ci *CallInfo, pc int) {
	ci.savedpc = pc
	L.top = ci.top
}

// luaV_execute is the main interpreter loop (lvm.c).
func rk(base int, k []Value, x int) *Value {
	if ISK(x) {
		return &k[INDEXK(x)]
	}
	return nil // signal register — caller uses base+x
}

func rkVal(L *State, base int, k []Value, x int) *Value {
	if ISK(x) {
		return &k[INDEXK(x)]
	}
	return &L.stack[base+x]
}

// forprep53 is the 5.3 FORPREP semantics (lvm.c).
func forprep53(L *State, ra int) {
	init := &L.stack[ra]
	plimit := &L.stack[ra+1]
	pstep := &L.stack[ra+2]
	var ilimit int64
	var stopnow bool
	if ttisinteger(init) && ttisinteger(pstep) && forlimit53(L, plimit, &ilimit, ivalue(pstep), &stopnow) {
		initv := int64(0)
		if !stopnow {
			initv = ivalue(init)
		}
		setivalue(plimit, ilimit)
		setivalue(init, initv-ivalue(pstep))
	} else {
		var ninit, nlimit, nstep float64
		if !tonumberns(plimit, &nlimit) {
			luaG_runerror(L, "'for' limit must be a number")
		}
		setfltvalue(plimit, nlimit)
		if !tonumberns(pstep, &nstep) {
			luaG_runerror(L, "'for' step must be a number")
		}
		setfltvalue(pstep, nstep)
		if !tonumberns(init, &ninit) {
			luaG_runerror(L, "'for' initial value must be a number")
		}
		setfltvalue(init, ninit-nstep)
	}
}

// forlimit53 converts the limit for a 5.3 numerical for (lvm.c forlimit).
func forlimit53(L *State, obj *Value, p *int64, step int64, stopnow *bool) bool {
	*stopnow = false
	mode := F2Ifloor
	if step < 0 {
		mode = F2Iceil
	}
	if !luaV_tointeger(L, obj, p, mode) {
		var n float64
		if !luaV_tonumber_(L, obj, &n) {
			return false
		}
		if n > 0 {
			*p = LUA_MAXINTEGER
			if step < 0 {
				*stopnow = true
			}
		} else {
			*p = LUA_MININTEGER
			if step >= 0 {
				*stopnow = true
			}
		}
	}
	return true
}

func luaV_execute(L *State, ci *CallInfo) {
	prevVM := L.inVM
	L.inVM = true
	L.vmNest++
	defer func() {
		L.vmNest--
		L.inVM = prevVM
	}()
	var (
		cl   *LClosure
		code []Instruction
		k    []Value
		pc   int
	)
	var trap bool
	restart := true
	for restart {
		restart = false
		cl = clLvalue(&L.stack[ci.funcIdx])
		code = cl.p.code
		k = cl.p.k
		pc = ci.savedpc
		trap = L.hookmask != 0
		if trap {
			trap = luaG_tracecall(L, ci)
		}
	loop:
		for {
			if trap {
				trap = luaG_traceexec(L, ci, pc)
			}
			i := code[pc]
			pc++
			base := ci.base
			ra := base + i.A

			switch i.Op {
			case OP_MOVE:
				L.stack[ra] = L.stack[base+i.B]
			case OP_LOADK:
				L.stack[ra] = k[i.B]
			case OP_LOADKX:
				L.stack[ra] = k[code[pc].A]
				pc++
			case OP_LOADBOOL:
				if i.B != 0 {
					setbtvalue(&L.stack[ra])
				} else {
					setbfvalue(&L.stack[ra])
				}
				if i.C != 0 {
					pc++
				}
			case OP_LOADNIL:
				for b := i.B; b >= 0; b-- {
					setnilvalue(&L.stack[ra])
					ra++
				}
			case OP_GETUPVAL:
				L.stack[ra] = *upvalValue(cl.upvals[i.B])
			case OP_GETTABUP:
				upval := upvalValue(cl.upvals[i.B])
				rc := rkVal(L, base, k, i.C)
				slot := vmFastGet(L, upval, rc)
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, upval, rc, ra, slot)
					base = ci.base
				}
			case OP_GETTABLE:
				rb := &L.stack[base+i.B]
				rc := rkVal(L, base, k, i.C)
				slot := vmFastGet(L, rb, rc)
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, rc, ra, slot)
					base = ci.base
				}
			case OP_SETTABUP:
				upval := upvalValue(cl.upvals[i.A])
				rb := rkVal(L, base, k, i.B)
				rc := rkVal(L, base, k, i.C)
				slot := vmFastGet(L, upval, rb)
				if slot != nil && !isempty(slot) {
					*slot = *rc
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, upval, rb, rc, slot)
					base = ci.base
				}
			case OP_SETUPVAL:
				*upvalValue(cl.upvals[i.B]) = L.stack[ra]
			case OP_SETTABLE:
				rb := rkVal(L, base, k, i.B)
				rc := rkVal(L, base, k, i.C)
				slot := vmFastGet(L, &L.stack[ra], rb)
				if slot != nil && !isempty(slot) {
					*slot = *rc
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, &L.stack[ra], rb, rc, slot)
					base = ci.base
				}
			case OP_NEWTABLE:
				b, c := i.B, i.C
				t := luaH_new(L)
				sethvalue(&L.stack[ra], t)
				if b != 0 || c != 0 {
					luaH_resize(L, t, uint32(luaO_fb2int(b)), uint32(luaO_fb2int(c)))
				}
				/* PUC luaC_checkGC after allocation; metamethod may run Lua. */
				if L.gcAfterAlloc() {
					continue
				}
			case OP_SELF:
				rb := &L.stack[base+i.B]
				L.stack[ra+1] = *rb
				rc := rkVal(L, base, k, i.C)
				slot := vmFastGet(L, rb, rc)
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, rc, ra, slot)
					base = ci.base
				}
			case OP_ADD, OP_SUB, OP_MUL, OP_MOD, OP_POW, OP_DIV, OP_IDIV,
				OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
				rb := rkVal(L, base, k, i.B)
				rc := rkVal(L, base, k, i.C)
				if !vmBinOp(L, i.Op, &L.stack[ra], rb, rc) {
					savestate(L, ci, pc)
					tm := binOpTM(i.Op)
					luaT_trybinTM(L, rb, rc, ra, tm)
					base = ci.base
				}
			case OP_UNM:
				rb := &L.stack[base+i.B]
				if ttisinteger(rb) {
					setivalue(&L.stack[ra], -ivalue(rb))
				} else {
					var nb float64
					if luaV_tonumber_(L, rb, &nb) {
						setfltvalue(&L.stack[ra], -nb)
					} else {
						savestate(L, ci, pc)
						luaT_trybinTM(L, rb, rb, ra, TM_UNM)
						base = ci.base
					}
				}
			case OP_BNOT:
				rb := &L.stack[base+i.B]
				var ib int64
				if tointeger(L, rb, &ib) {
					setivalue(&L.stack[ra], ^ib)
				} else {
					savestate(L, ci, pc)
					luaT_trybinTM(L, rb, rb, ra, TM_BNOT)
					base = ci.base
				}
			case OP_NOT:
				rb := &L.stack[base+i.B]
				if l_isfalse(rb) {
					setbtvalue(&L.stack[ra])
				} else {
					setbfvalue(&L.stack[ra])
				}
			case OP_LEN:
				savestate(L, ci, pc)
				luaV_objlen(L, ra, &L.stack[base+i.B])
				base = ci.base
			case OP_CONCAT:
				/* Like PUC 5.3 Protect / 5.4 ProtectNT: mark operand end in
				   L.top, save pc, but do NOT reset top to ci.top (savestate
				   would) before luaV_concat sees the operands. */
				b, c := i.B, i.C
				L.top = base + c + 1
				ci.savedpc = pc
				luaV_concat(L, c-b+1)
				base = ci.base
				ra = base + i.A
				L.stack[ra] = L.stack[base+b]
				L.top = ci.top
				/* concatenation allocates; programs may wait on the collector */
				if L.gcAfterAlloc() {
					continue
				}
			case OP_JMP:
				if i.A != 0 {
					luaF_closeupval(L, base+i.A-1)
				}
				pc += sBx(i.B)
			case OP_EQ:
				rb := rkVal(L, base, k, i.B)
				rc := rkVal(L, base, k, i.C)
				savestate(L, ci, pc)
				eq := luaV_equalobj(L, rb, rc)
				base = ci.base
				if (eq && i.A != 0) || (!eq && i.A == 0) {
					ji := code[pc]
					pc++
					if ji.A != 0 {
						luaF_closeupval(L, base+ji.A-1)
					}
					pc += sBx(ji.B)
				} else {
					pc++
				}
			case OP_LT, OP_LE:
				// handled below with corrected EQ — see fix
				rb := rkVal(L, base, k, i.B)
				rc := rkVal(L, base, k, i.C)
				savestate(L, ci, pc)
				var res bool
				if i.Op == OP_LT {
					res = luaV_lessthan(L, rb, rc)
				} else {
					res = luaV_lessequal(L, rb, rc)
				}
				base = ci.base
				if (res && i.A != 0) || (!res && i.A == 0) {
					ji := code[pc]
					pc++
					if ji.A != 0 {
						luaF_closeupval(L, base+ji.A-1)
					}
					pc += sBx(ji.B)
				} else {
					pc++
				}
			case OP_TEST:
				cond := !l_isfalse(&L.stack[ra])
				if (i.C != 0 && !cond) || (i.C == 0 && cond) {
					pc++
				} else {
					ji := code[pc]
					pc++
					if ji.A != 0 {
						luaF_closeupval(L, base+ji.A-1)
					}
					pc += sBx(ji.B)
				}
			case OP_TESTSET:
				rb := &L.stack[base+i.B]
				cond := !l_isfalse(rb)
				if (i.C != 0 && !cond) || (i.C == 0 && cond) {
					pc++
				} else {
					L.stack[ra] = *rb
					ji := code[pc]
					pc++
					if ji.A != 0 {
						luaF_closeupval(L, base+ji.A-1)
					}
					pc += sBx(ji.B)
				}
			case OP_CALL:
				b := i.B
				nresults := i.C - 1
				if b != 0 {
					L.top = ra + b
				}
				ci.savedpc = pc
				if newci := luaD_precall(L, ra, nresults); newci != nil {
					ci = newci
					restart = true
					break loop
				}
				if L.fastYield {
					return
				}
			case OP_TAILCALL:
				b := i.B
				if b != 0 {
					L.top = ra + b
				} else {
					b = L.top - ra
				}
				ci.savedpc = pc
				if cl.p.sizep > 0 {
					luaF_closeupval(L, base)
				}
				if n := luaD_pretailcall(L, ci, ra, b, 0); n < 0 {
					restart = true
					break loop
				} else if L.fastYield {
					return
				} else {
					luaD_poscall(L, ci, n)
				}
				if ci.callstatus&CIST_FRESH != 0 {
					return
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_RETURN:
				n := i.B - 1
				if n < 0 {
					n = L.top - ra
				}
				ci.savedpc = pc
				if cl.p.sizep > 0 {
					luaF_closeupval(L, base)
				}
				L.top = ra + n
				luaD_poscall(L, ci, n)
				if ci.callstatus&CIST_FRESH != 0 {
					return
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_FORLOOP:
				if ttisinteger(&L.stack[ra]) {
					step := ivalue(&L.stack[ra+2])
					idx := ivalue(&L.stack[ra]) + step
					limit := ivalue(&L.stack[ra+1])
					goon := false
					if step > 0 {
						goon = idx <= limit
					} else {
						goon = limit <= idx
					}
					if goon {
						pc += sBx(i.B)
						setivalue(&L.stack[ra], idx)
						setivalue(&L.stack[ra+3], idx)
					}
				} else {
					step := fltvalue(&L.stack[ra+2])
					idx := fltvalue(&L.stack[ra]) + step
					limit := fltvalue(&L.stack[ra+1])
					goon := false
					if step > 0 {
						goon = idx <= limit
					} else {
						goon = limit <= idx
					}
					if goon {
						pc += sBx(i.B)
						setfltvalue(&L.stack[ra], idx)
						setfltvalue(&L.stack[ra+3], idx)
					}
				}
			case OP_FORPREP:
				forprep53(L, ra)
				pc += sBx(i.B)
			case OP_TFORCALL:
				cb := ra + 3
				L.stack[cb+2] = L.stack[ra+2]
				L.stack[cb+1] = L.stack[ra+1]
				L.stack[cb] = L.stack[ra]
				L.top = cb + 3
				savestate(L, ci, pc)
				luaD_call(L, cb, i.C)
				base = ci.base
				L.top = ci.top
				i = code[pc]
				pc++
				ra = base + i.A
				fallthrough
			case OP_TFORLOOP:
				if !ttisnil(&L.stack[ra+1]) {
					L.stack[ra] = L.stack[ra+1]
					pc += sBx(i.B)
				}
			case OP_SETLIST:
				n := i.B
				c := i.C
				if n == 0 {
					n = L.top - ra - 1
				}
				if c == 0 {
					c = code[pc].A
					pc++
				}
				h := hvalue(&L.stack[ra])
				last := ((c - 1) * LFIELDS_PER_FLUSH) + n
				if uint32(last) > h.alimit {
					luaH_resizearray(L, h, uint32(last))
				}
				for ; n > 0; n-- {
					val := &L.stack[ra+n]
					luaH_setint(L, h, int64(last), val)
					last--
				}
				L.top = ci.top
			case OP_CLOSURE:
				p := cl.p.p[i.B]
				if ncl := getcached(p, cl.upvals, L, base); ncl != nil {
					setclLvalue(&L.stack[ra], ncl)
				} else {
					pushclosure(L, p, cl.upvals, base, ra)
				}
				if L.gcAfterAlloc() {
					continue
				}
			case OP_VARARG:
				b := i.B - 1
				n := (base - ci.funcIdx) - int(cl.p.numparams) - 1
				if n < 0 {
					n = 0
				}
				if b < 0 {
					b = n
					L.checkStack(n)
					base = ci.base
					ra = base + i.A
					L.top = ra + n
				}
				j := 0
				for ; j < b && j < n; j++ {
					L.stack[ra+j] = L.stack[base-n+j]
				}
				for ; j < b; j++ {
					setnilvalue(&L.stack[ra+j])
				}
			case OP_EXTRAARG:
				// should not happen
			default:
				luaG_runerror(L, "unknown opcode %d", i.Op)
			}
			ci.savedpc = pc
		}
	}
}

func binOpTM(op int) TMS {
	switch op {
	case OP_ADD:
		return TM_ADD
	case OP_SUB:
		return TM_SUB
	case OP_MUL:
		return TM_MUL
	case OP_MOD:
		return TM_MOD
	case OP_POW:
		return TM_POW
	case OP_DIV:
		return TM_DIV
	case OP_IDIV:
		return TM_IDIV
	case OP_BAND:
		return TM_BAND
	case OP_BOR:
		return TM_BOR
	case OP_BXOR:
		return TM_BXOR
	case OP_SHL:
		return TM_SHL
	case OP_SHR:
		return TM_SHR
	}
	return TM_ADD
}

func vmBinOp(L *State, op int, ra, rb, rc *Value) bool {
	/* Lua 5.3: tonumber/tointeger coerce strings (unlike 5.4's *ns helpers). */
	switch op {
	case OP_ADD:
		if ttisinteger(rb) && ttisinteger(rc) {
			setivalue(ra, ivalue(rb)+ivalue(rc))
			return true
		}
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, nb+nc)
			return true
		}
	case OP_SUB:
		if ttisinteger(rb) && ttisinteger(rc) {
			setivalue(ra, ivalue(rb)-ivalue(rc))
			return true
		}
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, nb-nc)
			return true
		}
	case OP_MUL:
		if ttisinteger(rb) && ttisinteger(rc) {
			setivalue(ra, ivalue(rb)*ivalue(rc))
			return true
		}
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, nb*nc)
			return true
		}
	case OP_DIV:
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, nb/nc)
			return true
		}
	case OP_IDIV:
		if ttisinteger(rb) && ttisinteger(rc) {
			setivalue(ra, luaV_idiv(L, ivalue(rb), ivalue(rc)))
			return true
		}
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, math.Floor(nb/nc))
			return true
		}
	case OP_MOD:
		if ttisinteger(rb) && ttisinteger(rc) {
			setivalue(ra, luaV_mod(L, ivalue(rb), ivalue(rc)))
			return true
		}
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, luaV_modf(L, nb, nc))
			return true
		}
	case OP_POW:
		var nb, nc float64
		if luaV_tonumber_(L, rb, &nb) && luaV_tonumber_(L, rc, &nc) {
			setfltvalue(ra, math.Pow(nb, nc))
			return true
		}
	case OP_BAND, OP_BOR, OP_BXOR, OP_SHL, OP_SHR:
		var ib, ic int64
		if tointeger(L, rb, &ib) && tointeger(L, rc, &ic) {
			switch op {
			case OP_BAND:
				setivalue(ra, ib&ic)
			case OP_BOR:
				setivalue(ra, ib|ic)
			case OP_BXOR:
				setivalue(ra, ib^ic)
			case OP_SHL:
				setivalue(ra, luaV_shiftl(ib, ic))
			case OP_SHR:
				setivalue(ra, luaV_shiftl(ib, -ic))
			}
			return true
		}
	}
	return false
}

func luaG_tracecall(L *State, ci *CallInfo) bool {
	p := clLvalue(&L.stack[ci.funcIdx]).p
	ci.trap = true       /* ensure hooks will be checked */
	if ci.savedpc == 0 { /* first instruction (not resuming)? */
		if p.isVararg != 0 {
			return false /* hooks will start at VARARGPREP instruction */
		} else if ci.callstatus&CIST_HOOKYIELD == 0 { /* not yielded? */
			luaD_hookcall(L, ci) /* check 'call' hook */
		}
	}
	return true /* keep 'trap' on */
}

// getFuncline returns the source line of an instruction (ldebug.c).
//
// deviation: Proto.lineinfo already holds absolute lines, so there is no
// baseline to walk back from.
func getFuncline(f *Proto, pc int) int {
	if f.lineinfo == nil || pc < 0 || pc >= len(f.lineinfo) {
		return -1 /* no debug information */
	}
	return int(f.lineinfo[pc])
}

// changedLine reports whether two instructions are on different lines
// (ldebug.c).
func changedLine(p *Proto, oldpc, newpc int) bool {
	if p.lineinfo == nil { /* no debug information? */
		return false
	}
	if oldpc < 0 {
		oldpc = 0
	}
	return getFuncline(p, oldpc) != getFuncline(p, newpc)
}

// luaG_traceexec checks (and runs) the line and count hooks before an
// instruction (ldebug.c). It reports whether 'trap' must stay on.
func luaG_traceexec(L *State, ci *CallInfo, pc int) bool {
	mask := L.hookmask
	p := clLvalue(&L.stack[ci.funcIdx]).p
	if mask&(dbMaskLine|dbMaskCount) == 0 { /* no hooks? */
		ci.trap = false /* don't need to stop again */
		return false    /* turn off 'trap' */
	}
	pc++            /* reference is always next instruction */
	ci.savedpc = pc /* save 'pc' */
	counthook := false
	if mask&dbMaskCount != 0 {
		L.hookcount--
		counthook = L.hookcount == 0
	}
	if counthook {
		L.hookcount = L.basehookcount /* resethookcount(L) */
	} else if mask&dbMaskLine == 0 {
		return true /* no line hook and count != 0; nothing to be done */
	}
	if ci.callstatus&CIST_HOOKYIELD != 0 { /* hook yielded last time? */
		ci.callstatus &= ^uint16(CIST_HOOKYIELD) /* erase mark */
		return true
	}
	if !isIT(p.code[pc-1]) { /* top not being used? */
		L.top = ci.top /* correct top */
	}
	if counthook {
		luaD_hook(L, hookCount, -1, 0, 0) /* call count hook */
	}
	if mask&dbMaskLine != 0 {
		/* 'L.oldpc' may be invalid; use zero in this case */
		oldpc := 0
		if L.oldpc < len(p.code) && L.oldpc >= 0 {
			oldpc = L.oldpc
		}
		npci := pc - 1 /* pcRel: 'pc' relative to the first instruction */
		if npci <= oldpc || changedLine(p, oldpc, npci) {
			/* call the hook when jumping back (loop) or on a new line */
			luaD_hook(L, hookLine, getFuncline(p, npci), 0, 0)
		}
		L.oldpc = npci /* 'pc' of last call to line hook */
	}
	/* deviation: a hook cannot yield here, so the CIST_HOOKYIELD dance
	   that ends luaG_traceexec is not needed. */
	return true /* keep 'trap' on */
}

func numIsNaN(n float64) bool { return math.IsNaN(n) }
