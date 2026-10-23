// Translation of lvm.h / lvm.c (PUC-Lua 5.4.9) — line-faithful C→Go.
//
// Main interpreter: luaV_execute opcode switch matches 5.4.9 (including
// OP_LOADI/LOADF/ADDI/*K/MMBIN*/EQI/LTI/…/RETURN0/RETURN1/VARARGPREP,
// forprep counter, floatforloop). ljumptab.h is reference only: Go uses
// a switch, not a computed-goto dispatch table.
//
// Intentional omissions / adaptations (BRIEF KEEP):
//   - GC barriers / checkGC → gcAfterAlloc (Go GC)
//   - luaV_finishOp absent (goroutine yield; no longjmp out of the loop)
//   - index stack / UpVal.open list; panic/recover error path
//   - number/compare/objlen helpers live in lvm_helpers.go (C: this file)
package core

import (
	"math"
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
** Integer division; return 'm // n', that is, floor(m/n).
** C division truncates its result (rounds towards zero).
** 'floor(q) == trunc(q)' when 'q >= 0' or when 'q' is integer,
** otherwise 'floor(q) == trunc(q) - 1'.
 */
func luaV_idiv(L *State, m, n int64) int64 {
	if uint64(n)+1 <= 1 { /* special cases: -1 or 0 */
		if n == 0 {
			luaG_runerror(L, "attempt to divide by zero")
			return 0
		}
		return int64(uint64(0) - uint64(m)) /* intop(-, 0, m); n == -1 */
	}
	q := m / n                 /* perform C division */
	if (m^n) < 0 && m%n != 0 { /* 'm/n' would be negative non-integer? */
		q-- /* correct result for different rounding */
	}
	return q
}

/*
** Integer modulus; return 'm % n'. (Assume that C '%' with
** negative operands follows C99 behavior. See previous comment
** about luaV_idiv.)
 */
func luaV_mod(L *State, m, n int64) int64 {
	if uint64(n)+1 <= 1 { /* special cases: -1 or 0 */
		if n == 0 {
			luaG_runerror(L, "attempt to perform 'n%%0'")
			return 0
		}
		return 0 /* m % -1 == 0; avoid overflow with 0x80000...%-1 */
	}
	r := m % n
	if r != 0 && (r^n) < 0 { /* 'm/n' would be non-integer negative? */
		r += n /* correct result for different rounding */
	}
	return r
}

/*
** Float modulus
 */
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

/* number of bits in an integer */
const NBITS = 64

/*
** Shift left operation. (Shift right just negates 'y'.)
 */
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

/* tostring: ensure element at 'o' is a string (lvm.c macro). */
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

/* isemptystr: short empty string only (lvm.c). */
func isEmptyStr(v *Value) bool {
	return ttypetag(v) == LUA_VSHRSTR && len(tsvalue(v).val) == 0
}

/* copy strings in stack from top - n up to top - 1 to buffer (lvm.c). */
func copy2buff(stack []Value, top, n int, buff *strings.Builder) {
	for ; n > 0; n-- {
		buff.WriteString(tsvalue(&stack[top-n]).val)
	}
}

/*
** Main operation for concatenation: concat 'total' values in the stack,
** from 'L.top - total' up to 'L.top - 1'.
 */
// luaV_concatMaxLen is MaxInt for the concat length-overflow check.
// Tests may lower it; production always uses math.MaxInt.
var luaV_concatMaxLen = math.MaxInt

func luaV_concat(L *State, total int) {
	if total == 1 {
		return /* "all" values already concatenated */
	}
	for {
		top := L.top
		n := 2 /* number of elements handled in this pass (at least 2) */
		o1 := &L.stack[top-2]
		o2 := &L.stack[top-1]
		if !(ttisstring(o1) || cvt2str(o1)) || !tostringInPlace(L, o2) {
			luaT_tryconcatTM(L) /* may invalidate 'top' */
		} else if isEmptyStr(o2) { /* second operand is empty? */
			tostringInPlace(L, o1) /* result is first operand */
		} else if isEmptyStr(o1) { /* first operand is empty string? */
			L.stack[top-2] = L.stack[top-1] /* result is second op. */
		} else {
			/* at least two non-empty string values; get as many as possible */
			tl := len(tsvalue(&L.stack[top-1]).val)
			for n = 1; n < total && tostringInPlace(L, &L.stack[top-n-1]); n++ {
				l := len(tsvalue(&L.stack[top-n-1]).val)
				if l >= luaV_concatMaxLen-tl { /* string length overflow? */
					L.top = top - total /* pop strings to avoid wasting stack */
					luaG_runerror(L, "string length overflow")
					return
				}
				tl += l
			}
			var buff strings.Builder
			buff.Grow(tl)
			copy2buff(L.stack, top, n, &buff)
			setsvalue(&L.stack[top-n], luaS_newlstr(L, buff.String()))
		}
		total -= n - 1 /* got 'n' strings to create one new */
		L.top -= n - 1 /* popped 'n' strings and pushed one */
		if total <= 1 {
			return
		}
	}
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

// vmFastGetShortStr mirrors luaV_fastget(..., luaH_getshortstr).
func vmFastGetShortStr(t *Value, key *TString) *Value {
	if !ttistable(t) {
		return nil
	}
	return luaH_getshortstr(hvalue(t), key)
}

// vmFastGetStr mirrors luaV_fastget(..., luaH_getstr) for OP_SELF.
func vmFastGetStr(L *State, t *Value, key *TString) *Value {
	if !ttistable(t) {
		return nil
	}
	return luaH_getstr(L, hvalue(t), key)
}

// pushclosure creates a new Lua closure and initializes its upvalues.
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
		if !tonumber(lim, &flim) { /* cannot convert to float? */
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
	if !tonumber(plimit, &limit) {
		luaG_forerror(L, plimit, "limit")
	}
	if !tonumber(pstep, &step) {
		luaG_forerror(L, pstep, "step")
	}
	if !tonumber(pinit, &init) {
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
		p    *Proto
		code []Instruction
		k    []Value
		pc   int
	)

	// 'trap' is the local copy of ci.trap (lvm.c). It tells whether the
	// loop must stop at the next instruction to run the hooks.
	var trap bool

	restart := true
	for restart {
		restart = false
		cl = clLvalue(&L.stack[ci.funcIdx])
		p = cl.p
		code = p.code
		k = p.k
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
			base := ci.funcIdx + 1
			ra := base + i.A

			switch i.Op {
			case OP_MOVE:
				L.stack[ra] = L.stack[base+i.B]
			case OP_LOADI:
				setivalue(&L.stack[ra], int64(i.B-OFFSET_sBx))
			case OP_LOADF:
				setfltvalue(&L.stack[ra], float64(i.B-OFFSET_sBx))
			case OP_LOADK:
				L.stack[ra] = k[i.B]
			case OP_LOADKX:
				L.stack[ra] = k[code[pc].A]
				pc++
			case OP_LOADFALSE:
				setbfvalue(&L.stack[ra])
			case OP_LFALSESKIP:
				setbfvalue(&L.stack[ra])
				pc++
			case OP_LOADTRUE:
				setbtvalue(&L.stack[ra])
			case OP_LOADNIL:
				for j := 0; j <= i.B; j++ {
					setnilvalue(&L.stack[ra+j])
				}
			case OP_GETUPVAL:
				L.stack[ra] = *upvalValue(cl.upvals[i.B])
			case OP_SETUPVAL:
				*upvalValue(cl.upvals[i.B]) = L.stack[ra]
			case OP_GETTABUP:
				upval := upvalValue(cl.upvals[i.B])
				key := &k[i.C]
				slot := vmFastGetShortStr(upval, tsvalue(key)) /* luaH_getshortstr */
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, upval, key, ra, slot)
				}
			case OP_GETTABLE:
				rb := &L.stack[base+i.B]
				rc := &L.stack[base+i.C]
				var slot *Value
				if ttisinteger(rc) { /* fast track for integers? */
					slot = vmFastGeti(rb, ivalue(rc))
				} else {
					slot = vmFastGet(L, rb, rc)
				}
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, rc, ra, slot)
				}
			case OP_GETI:
				rb := &L.stack[base+i.B]
				slot := vmFastGeti(rb, int64(i.C))
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					var key Value
					setivalue(&key, int64(i.C))
					savestate(L, ci, pc)
					luaV_finishget(L, rb, &key, ra, slot)
				}
			case OP_GETFIELD:
				rb := &L.stack[base+i.B]
				key := &k[i.C]
				slot := vmFastGetShortStr(rb, tsvalue(key)) /* luaH_getshortstr */
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, key, ra, slot)
				}
			case OP_SETTABUP:
				upval := upvalValue(cl.upvals[i.A])
				key := &k[i.B]
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				slot := vmFastGetShortStr(upval, tsvalue(key)) /* luaH_getshortstr */
				if slot != nil && !isempty(slot) {
					*slot = *rc /* luaV_finishfastset (barrier skipped: Go GC) */
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, upval, key, rc, slot)
				}
			case OP_SETTABLE:
				rt := &L.stack[ra]
				rb := &L.stack[base+i.B] /* key (table is in 'ra') */
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				var slot *Value
				if ttisinteger(rb) { /* fast track for integers? */
					slot = vmFastGeti(rt, ivalue(rb))
				} else {
					slot = vmFastGet(L, rt, rb)
				}
				if slot != nil && !isempty(slot) {
					*slot = *rc /* luaV_finishfastset (barrier skipped: Go GC) */
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, rt, rb, rc, slot)
				}
			case OP_SETI:
				rt := &L.stack[ra]
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				slot := vmFastGeti(rt, int64(i.B))
				if slot != nil && !isempty(slot) {
					*slot = *rc
				} else {
					var key Value
					setivalue(&key, int64(i.B))
					savestate(L, ci, pc)
					luaV_finishset(L, rt, &key, rc, slot)
				}
			case OP_SETFIELD:
				rt := &L.stack[ra]
				key := &k[i.B]
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				slot := vmFastGetShortStr(rt, tsvalue(key)) /* luaH_getshortstr */
				if slot != nil && !isempty(slot) {
					*slot = *rc /* luaV_finishfastset (barrier skipped: Go GC) */
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, rt, key, rc, slot)
				}
			case OP_NEWTABLE:
				b := i.B /* log2(hash size) + 1 */
				c := i.C /* array size */
				if b > 0 {
					b = 1 << (b - 1) /* size is 2^(b - 1) */
				}
				if i.K != 0 { /* non-zero extra argument? */
					c += code[pc].A * (MAXARG_C + 1) /* add it to size */
				}
				pc++             /* skip extra argument */
				L.top = ra + 1   /* correct top in case of emergency GC */
				t := luaH_new(L) /* memory allocation */
				sethvalue(&L.stack[ra], t)
				if b != 0 || c != 0 {
					luaH_resize(L, t, uint32(c), uint32(b)) /* idem */
				}
				/* checkGC → gcAfterAlloc (Go GC); Protect-style restart */
				if L.gcAfterAlloc() {
					continue
				}
			case OP_SELF:
				rb := &L.stack[base+i.B]
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				L.stack[ra+1] = *rb
				slot := vmFastGetStr(L, rb, tsvalue(rc)) /* luaH_getstr; key is a string */
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, rc, ra, slot)
				}
			case OP_ADDI:
				/* op_arithI(L, l_addi, luai_numadd) */
				rb := &L.stack[base+i.B]
				imm := int64(i.C - OFFSET_sC)
				if ttisinteger(rb) {
					pc++
					setivalue(&L.stack[ra], ivalue(rb)+imm)
				} else if ttisfloat(rb) {
					pc++
					setfltvalue(&L.stack[ra], fltvalue(rb)+float64(imm))
				}
			case OP_ADDK:
				opArithK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(_ *State, a, b int64) int64 { return a + b },
					func(a, b float64) float64 { return a + b }, &pc)
			case OP_SUBK:
				opArithK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(_ *State, a, b int64) int64 { return a - b },
					func(a, b float64) float64 { return a - b }, &pc)
			case OP_MULK:
				opArithK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(_ *State, a, b int64) int64 { return a * b },
					func(a, b float64) float64 { return a * b }, &pc)
			case OP_MODK:
				savestate(L, ci, pc) /* in case of division by 0 */
				opArithK(L, ra, &L.stack[base+i.B], &k[i.C],
					luaV_mod, luaV_modfWrap, &pc)
			case OP_POWK:
				opArithfK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(a, b float64) float64 { return math2Pow(a, b) }, &pc)
			case OP_DIVK:
				opArithfK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(a, b float64) float64 { return a / b }, &pc)
			case OP_IDIVK:
				savestate(L, ci, pc) /* in case of division by 0 */
				opArithK(L, ra, &L.stack[base+i.B], &k[i.C],
					luaV_idiv,
					func(a, b float64) float64 { return math2Floor(a / b) }, &pc)
			case OP_BANDK:
				opBitwiseK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(a, b int64) int64 { return a & b }, &pc)
			case OP_BORK:
				opBitwiseK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(a, b int64) int64 { return a | b }, &pc)
			case OP_BXORK:
				opBitwiseK(L, ra, &L.stack[base+i.B], &k[i.C],
					func(a, b int64) int64 { return a ^ b }, &pc)
			case OP_SHRI:
				rb := &L.stack[base+i.B]
				ic := int64(i.C - OFFSET_sC)
				var ib int64
				if tointegerns(rb, &ib) {
					pc++
					setivalue(&L.stack[ra], luaV_shiftl(ib, -ic))
				}
			case OP_SHLI:
				rb := &L.stack[base+i.B]
				ic := int64(i.C - OFFSET_sC)
				var ib int64
				if tointegerns(rb, &ib) {
					pc++
					setivalue(&L.stack[ra], luaV_shiftl(ic, ib))
				}
			case OP_ADD:
				opArith(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(_ *State, a, b int64) int64 { return a + b },
					func(a, b float64) float64 { return a + b }, &pc)
			case OP_SUB:
				opArith(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(_ *State, a, b int64) int64 { return a - b },
					func(a, b float64) float64 { return a - b }, &pc)
			case OP_MUL:
				opArith(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(_ *State, a, b int64) int64 { return a * b },
					func(a, b float64) float64 { return a * b }, &pc)
			case OP_MOD:
				savestate(L, ci, pc) /* in case of division by 0 */
				opArith(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					luaV_mod, luaV_modfWrap, &pc)
			case OP_POW:
				opArithf(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b float64) float64 { return math2Pow(a, b) }, &pc)
			case OP_DIV: /* float division (always with floats) */
				opArithf(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b float64) float64 { return a / b }, &pc)
			case OP_IDIV: /* floor division */
				savestate(L, ci, pc) /* in case of division by 0 */
				opArith(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					luaV_idiv,
					func(a, b float64) float64 { return math2Floor(a / b) }, &pc)
			case OP_BAND:
				opBitwise(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b int64) int64 { return a & b }, &pc)
			case OP_BOR:
				opBitwise(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b int64) int64 { return a | b }, &pc)
			case OP_BXOR:
				opBitwise(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b int64) int64 { return a ^ b }, &pc)
			case OP_SHR:
				opBitwise(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b int64) int64 { return luaV_shiftl(a, -b) }, &pc)
			case OP_SHL:
				opBitwise(L, ra, &L.stack[base+i.B], &L.stack[base+i.C],
					func(a, b int64) int64 { return luaV_shiftl(a, b) }, &pc)
			case OP_MMBIN:
				pi := code[pc-2]
				rb := &L.stack[base+i.B]
				result := base + pi.A
				savestate(L, ci, pc)
				luaT_trybinTM(L, &L.stack[ra], rb, result, TMS(i.C))
			case OP_MMBINI:
				pi := code[pc-2]
				imm := int64(i.B - OFFSET_sC)
				result := base + pi.A
				savestate(L, ci, pc)
				luaT_trybiniTM(L, &L.stack[ra], imm, i.K != 0, result, TMS(i.C))
			case OP_MMBINK:
				pi := code[pc-2]
				imm := &k[i.B]
				result := base + pi.A
				savestate(L, ci, pc)
				luaT_trybinassocTM(L, &L.stack[ra], imm, i.K != 0, result, TMS(i.C))
			case OP_UNM:
				rb := &L.stack[base+i.B]
				if ttisinteger(rb) {
					ib := ivalue(rb)
					setivalue(&L.stack[ra], int64(uint64(0)-uint64(ib))) /* intop(-, 0, ib) */
				} else {
					var nb float64
					if tonumberns(rb, &nb) {
						setfltvalue(&L.stack[ra], -nb)
					} else {
						savestate(L, ci, pc)
						luaT_trybinTM(L, rb, rb, ra, TM_UNM)
					}
				}
			case OP_BNOT:
				rb := &L.stack[base+i.B]
				var ib int64
				if tointegerns(rb, &ib) {
					setivalue(&L.stack[ra], int64(^uint64(0)^uint64(ib))) /* intop(^, ~0, ib) */
				} else {
					savestate(L, ci, pc)
					luaT_trybinTM(L, rb, rb, ra, TM_BNOT)
				}
			case OP_NOT:
				if isFalse(&L.stack[base+i.B]) {
					setbtvalue(&L.stack[ra])
				} else {
					setbfvalue(&L.stack[ra])
				}
			case OP_LEN:
				savestate(L, ci, pc)
				luaV_objlen(L, ra, &L.stack[base+i.B])
			case OP_CONCAT:
				n := i.B
				L.top = ra + n
				ci.savedpc = pc
				luaV_concat(L, n)
				/* a concatenation allocates a string, and a program can
				   wait for the collector on that alone */
				if L.gcAfterAlloc() {
					continue
				}
			case OP_CLOSE:
				savestate(L, ci, pc) /* Protect: a yield in __close restarts here */
				luaF_close(L, ra, LUA_OK, true)
			case OP_TBC:
				savestate(L, ci, pc) /* in case of errors */
				luaF_newtbcupval(L, ra)
			case OP_JMP:
				pc += i.B - OFFSET_sJ
				trap = ci.trap /* dojump → updatetrap */
			case OP_EQ:
				savestate(L, ci, pc)
				cond := luaV_equalobj(L, &L.stack[ra], &L.stack[base+i.B])
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_LT: /* op_order(L, l_lti, LTnum, lessthanothers) */
				rb := &L.stack[base+i.B]
				var cond bool
				if ttisinteger(&L.stack[ra]) && ttisinteger(rb) {
					cond = ivalue(&L.stack[ra]) < ivalue(rb)
				} else if ttisnumber(&L.stack[ra]) && ttisnumber(rb) {
					cond = LTnum(&L.stack[ra], rb)
				} else {
					savestate(L, ci, pc)
					cond = lessthanothers(L, &L.stack[ra], rb)
				}
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_LE: /* op_order(L, l_lei, LEnum, lessequalothers) */
				rb := &L.stack[base+i.B]
				var cond bool
				if ttisinteger(&L.stack[ra]) && ttisinteger(rb) {
					cond = ivalue(&L.stack[ra]) <= ivalue(rb)
				} else if ttisnumber(&L.stack[ra]) && ttisnumber(rb) {
					cond = LEnum(&L.stack[ra], rb)
				} else {
					savestate(L, ci, pc)
					cond = lessequalothers(L, &L.stack[ra], rb)
				}
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_EQK:
				cond := luaV_rawequalobj(&L.stack[ra], &k[i.B])
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_EQI:
				im := int64(i.B - OFFSET_sC)
				a := &L.stack[ra]
				cond := false
				if ttisinteger(a) {
					cond = ivalue(a) == im
				} else if ttisfloat(a) {
					cond = fltvalue(a) == float64(im)
				} /* else other types cannot be equal to a number */
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_LTI, OP_LEI, OP_GTI, OP_GEI: /* op_orderI */
				im := int64(i.B - OFFSET_sC)
				a := &L.stack[ra]
				cond := false
				if ttisinteger(a) {
					switch i.Op {
					case OP_LTI:
						cond = ivalue(a) < im
					case OP_LEI:
						cond = ivalue(a) <= im
					case OP_GTI:
						cond = ivalue(a) > im
					case OP_GEI:
						cond = ivalue(a) >= im
					}
				} else if ttisfloat(a) {
					fa := fltvalue(a)
					fim := float64(im)
					switch i.Op {
					case OP_LTI:
						cond = fa < fim
					case OP_LEI:
						cond = fa <= fim
					case OP_GTI:
						cond = fa > fim
					case OP_GEI:
						cond = fa >= fim
					}
				} else {
					/* 'isf' tells whether the immediate operand was a float */
					isf := i.C != 0
					savestate(L, ci, pc)
					switch i.Op {
					case OP_LTI:
						cond = luaT_callorderiTM(L, a, im, false, isf, TM_LT)
					case OP_LEI:
						cond = luaT_callorderiTM(L, a, im, false, isf, TM_LE)
					case OP_GTI:
						cond = luaT_callorderiTM(L, a, im, true, isf, TM_LT)
					case OP_GEI:
						cond = luaT_callorderiTM(L, a, im, true, isf, TM_LE)
					}
				}
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_TEST:
				cond := !isFalse(&L.stack[ra])
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → dojump → updatetrap */
				}
			case OP_TESTSET:
				rb := &L.stack[base+i.B]
				if isFalse(rb) == (i.K != 0) {
					pc++
				} else {
					L.stack[ra] = *rb
					pc += code[pc].B - OFFSET_sJ + 1
					trap = ci.trap /* donextjump → updatetrap */
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
					return /* synchronous yield: back to Resume */
				}
				trap = ci.trap /* C call; updatetrap */
			case OP_TAILCALL:
				b := i.B
				nparams1 := i.C
				delta := 0
				if nparams1 != 0 {
					delta = ci.nExtraArgs + nparams1
				}
				if b != 0 {
					L.top = ra + b
				} else {
					b = L.top - ra
				}
				ci.savedpc = pc
				if i.K != 0 {
					luaF_closeupval(L, base) /* close upvalues of this frame */
				}
				if n := luaD_pretailcall(L, ci, ra, b, delta); n < 0 {
					/* Lua function: the frame was reused in place, so
					   start the callee in it (like C's 'startfunc') */
					restart = true
					break loop
				} else if L.fastYield {
					return /* synchronous yield: back to Resume */
				} else { /* Go/C function */
					ci.funcIdx -= delta    /* restore 'func' (if vararg) */
					luaD_poscall(L, ci, n) /* finish the caller */
					trap = ci.trap         /* updatetrap */
				}
				/* ret: the caller returns after the tail call */
				if ci.callstatus&CIST_FRESH != 0 {
					return /* end this 'luaV_execute' */
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_RETURN:
				n := i.B - 1
				nparams1 := i.C
				if n < 0 {
					n = L.top - ra
				}
				ci.savedpc = pc
				if i.K != 0 {
					if L.top < ci.top {
						L.top = ci.top
					}
					luaF_close(L, base, closeKTOP, true)
				}
				if nparams1 != 0 {
					ci.funcIdx -= ci.nExtraArgs + nparams1
				}
				L.top = ra + n
				luaD_poscall(L, ci, n)
				trap = ci.trap /* updatetrap */
				if ci.callstatus&CIST_FRESH != 0 {
					return
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_RETURN0:
				if L.hookmask != 0 {
					L.top = ra /* no results */
					ci.savedpc = pc
					luaD_poscall(L, ci, 0) /* no hurry... */
					trap = true
				} else { /* do the 'poscall' here */
					nres := ci.nresults
					L.ci = ci.prev
					L.top = ci.funcIdx
					for ; nres > 0; nres-- {
						setnilvalue(&L.stack[L.top])
						L.top++
					}
				}
				if ci.callstatus&CIST_FRESH != 0 {
					return
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_RETURN1:
				if L.hookmask != 0 {
					L.top = ra + 1 /* one result */
					ci.savedpc = pc
					luaD_poscall(L, ci, 1) /* no hurry... */
					trap = true
				} else { /* do the 'poscall' here */
					nres := ci.nresults
					L.ci = ci.prev
					if nres == 0 {
						L.top = ci.funcIdx
					} else {
						L.stack[ci.funcIdx] = L.stack[ra]
						L.top = ci.funcIdx + 1
						for ; nres > 1; nres-- {
							setnilvalue(&L.stack[L.top])
							L.top++
						}
					}
				}
				if ci.callstatus&CIST_FRESH != 0 {
					return
				}
				ci = ci.prev
				restart = true
				break loop
			case OP_FORLOOP:
				if ttisinteger(&L.stack[ra+2]) { /* integer loop? */
					count := uint64(ivalue(&L.stack[ra+1]))
					if count > 0 {
						step := ivalue(&L.stack[ra+2])
						idx := ivalue(&L.stack[ra])
						chgivalue(&L.stack[ra+1], int64(count-1))
						idx = int64(uint64(idx) + uint64(step)) /* intop(+, idx, step) */
						chgivalue(&L.stack[ra], idx)
						setivalue(&L.stack[ra+3], idx)
						pc -= i.B
					}
				} else if floatforloop(L, ra) {
					pc -= i.B
				}
				trap = ci.trap /* updatetrap: allow a signal to break the loop */
			case OP_FORPREP:
				savestate(L, ci, pc)
				if forprep(L, ra) {
					pc += i.B + 1 /* skip the loop */
				}
			case OP_TFORPREP:
				savestate(L, ci, pc) /* halfProtect: create to-be-closed upvalue */
				luaF_newtbcupval(L, ra+3)
				pc += i.B
				/* go to the next instruction (OP_TFORCALL) */
				i = code[pc]
				pc++
				ra3 := ci.funcIdx + 1 + i.A
				L.checkStack(4)
				copy(L.stack[ra3+4:ra3+7], L.stack[ra3:ra3+3])
				L.top = ra3 + 7
				ci.savedpc = pc
				luaD_call(L, ra3+4, i.C)
				/* go to the next instruction (OP_TFORLOOP) */
				i = code[pc]
				pc++
				ra4 := ci.funcIdx + 1 + i.A
				if !ttisnil(&L.stack[ra4+4]) {
					L.stack[ra4+2] = L.stack[ra4+4]
					pc -= i.B
				}
			case OP_TFORCALL:
				ra3 := base + i.A
				L.checkStack(4)
				copy(L.stack[ra3+4:ra3+7], L.stack[ra3:ra3+3])
				L.top = ra3 + 7
				ci.savedpc = pc
				luaD_call(L, ra3+4, i.C)
				/* fall into OP_TFORLOOP */
				i3 := code[pc]
				pc++
				ra4 := ci.funcIdx + 1 + i3.A
				if !ttisnil(&L.stack[ra4+4]) {
					L.stack[ra4+2] = L.stack[ra4+4]
					pc -= i3.B
				}
			case OP_TFORLOOP:
				/* Unreachable on valid bytecode: OP_TFORCALL / OP_TFORPREP
				   already consume the following OP_TFORLOOP (C fallthrough).
				   Kept as an empty case so an accidental lone OP_TFORLOOP is
				   a no-op rather than an unknown-opcode panic. */
			case OP_SETLIST:
				n := i.B
				last := uint32(i.C)
				h := hvalue(&L.stack[ra])
				if n == 0 {
					n = L.top - ra - 1
				} else {
					L.top = ci.top
				}
				last += uint32(n)
				if i.K != 0 {
					last += uint32(code[pc].A) * (MAXARG_C + 1)
					pc++
				}
				if uint64(last) > uint64(luaH_realasize(h)) {
					luaH_resizearray(L, h, last)
				}
				for ; n > 0; n-- {
					h.array[last-1] = L.stack[ra+n]
					last--
				}
			case OP_CLOSURE:
				pushclosure(L, p.p[i.B], cl.upvals, base, ra)
				if L.gcAfterAlloc() { /* checkGC(L, ra + 1) */
					continue
				}
			case OP_VARARG:
				savestate(L, ci, pc) /* Protect */
				luaT_getvarargs(L, ci, ra, i.C-1)
			case OP_VARARGPREP:
				ci.savedpc = pc /* in case of errors */
				luaT_adjustvarargs(L, i.A, ci, cl.p)
				trap = ci.trap /* ProtectNT updates 'trap' */
				if trap {      /* previous "Protect" updated trap */
					luaD_hookcall(L, ci)
					L.oldpc = 1 /* next opcode will be seen as a "new" line */
				}
			case OP_EXTRAARG:
				/* nothing to do (already consumed) */
			default:
				panic("unknown opcode " + opName(i.Op))
			}
			/* Catch-all updatetrap for Protect sites that do not set trap
			   locally (finishget/set, MMBIN*, comparisons, CLOSE, ...).
			   JMP / FORLOOP / docondjump / CALL / RETURN already update
			   trap explicitly like C. Skipped when no hooks are installed. */
			if L.hookmask != 0 {
				trap = ci.trap
			}
		}
	}
}

// luaG_tracecall prepares the hook check when the VM starts (or resumes)
// a Lua function (ldebug.c). It reports whether 'trap' must stay on.
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

/*
** Arithmetic helpers mirroring lvm.c op_arith* / op_bitwise* macros.
** On success they skip the following MMBIN* instruction (pc++), as C does.
 */

func luaV_modfWrap(a, b float64) float64 {
	/* float mod used from op_arith; L is unused by luaV_modf body */
	return luaV_modf(nil, a, b)
}

func opArithfAux(L *State, ra int, v1, v2 *Value, fop func(float64, float64) float64, pc *int) {
	var n1, n2 float64
	if tonumberns(v1, &n1) && tonumberns(v2, &n2) {
		*pc++
		setfltvalue(&L.stack[ra], fop(n1, n2))
	}
}

func opArithf(L *State, ra int, v1, v2 *Value, fop func(float64, float64) float64, pc *int) {
	opArithfAux(L, ra, v1, v2, fop, pc)
}

func opArithfK(L *State, ra int, v1, v2 *Value, fop func(float64, float64) float64, pc *int) {
	opArithfAux(L, ra, v1, v2, fop, pc)
}

func opArithAux(L *State, ra int, v1, v2 *Value, iop func(*State, int64, int64) int64, fop func(float64, float64) float64, pc *int) {
	if ttisinteger(v1) && ttisinteger(v2) {
		*pc++
		setivalue(&L.stack[ra], iop(L, ivalue(v1), ivalue(v2)))
	} else {
		opArithfAux(L, ra, v1, v2, fop, pc)
	}
}

func opArith(L *State, ra int, v1, v2 *Value, iop func(*State, int64, int64) int64, fop func(float64, float64) float64, pc *int) {
	opArithAux(L, ra, v1, v2, iop, fop, pc)
}

func opArithK(L *State, ra int, v1, v2 *Value, iop func(*State, int64, int64) int64, fop func(float64, float64) float64, pc *int) {
	opArithAux(L, ra, v1, v2, iop, fop, pc)
}

func opBitwiseK(L *State, ra int, v1, v2 *Value, op func(int64, int64) int64, pc *int) {
	var i1 int64
	i2 := ivalue(v2)
	if tointegerns(v1, &i1) {
		*pc++
		setivalue(&L.stack[ra], op(i1, i2))
	}
}

func opBitwise(L *State, ra int, v1, v2 *Value, op func(int64, int64) int64, pc *int) {
	var i1, i2 int64
	if tointegerns(v1, &i1) && tointegerns(v2, &i2) {
		*pc++
		setivalue(&L.stack[ra], op(i1, i2))
	}
}
