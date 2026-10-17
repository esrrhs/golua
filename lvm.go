// Translation of lvm.h / lvm.c (PUC-Lua 5.4).
//
// The instruction dispatch loop follows lvm.c closely. Two things are
// gone: the GC steps, and 'luaV_finishOp' (there is nothing to finish,
// because a metamethod that yields simply blocks the coroutine's
// goroutine instead of longjmp-ing out of the interpreter).
package lua

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
		if !(ttisstring(o1) || cvt2str(o1)) || !tostringInPlace(L, o2) {
			luaT_tryconcatTM(L)
		} else if isEmptyStr(o2) { /* second operand is empty? */
			tostringInPlace(L, o1) /* result is first operand */
		} else if isEmptyStr(o1) { /* first operand is empty string? */
			L.stack[top-2] = L.stack[top-1] /* result is second operand */
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
	if (step > 0 && limit < init) || (step <= 0 && init < limit) {
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
	if (step > 0 && idx <= limit) || (step <= 0 && limit <= idx) {
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
				slot := vmFastGet(L, upval, key)
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, upval, key, ra, slot)
				}
			case OP_GETTABLE:
				rb := &L.stack[base+i.B]
				rc := &L.stack[base+i.C]
				slot := vmFastGet(L, rb, rc)
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
				slot := vmFastGet(L, rb, key)
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
				slot := vmFastGet(L, upval, key)
				if slot != nil && !isempty(slot) {
					*slot = *rc
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, upval, key, rc, slot)
				}
			case OP_SETTABLE:
				rt := &L.stack[ra]
				rb := &L.stack[base+i.B]
				var rc *Value
				if i.K != 0 {
					rc = &k[i.C]
				} else {
					rc = &L.stack[base+i.C]
				}
				slot := vmFastGet(L, rt, rb)
				if slot != nil && !isempty(slot) {
					*slot = *rc
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
				slot := vmFastGet(L, rt, key)
				if slot != nil && !isempty(slot) {
					*slot = *rc
				} else {
					savestate(L, ci, pc)
					luaV_finishset(L, rt, key, rc, slot)
				}
			case OP_NEWTABLE:
				b := i.B
				c := i.C
				if b > 0 {
					b = 1 << (b - 1)
				}
				if i.K != 0 {
					c += code[pc].A * (MAXARG_C + 1)
				}
				pc++ /* skip extra argument */
				L.top = ra + 1
				t := luaH_new(L)
				sethvalue(&L.stack[ra], t)
				if b != 0 || c != 0 {
					luaH_resize(L, t, uint32(c), uint32(b))
				}
				/* PUC-Lua runs a collector step -- and with it any
				   '__gc' metamethod -- right after an allocation; see
				   gcAfterAlloc.  A metamethod can run arbitrary Lua
				   code, so the frame, the stack and the instruction
				   pointer are re-read: 'continue' does that, as
				   'Protect' does in C. */
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
				slot := vmFastGet(L, rb, rc)
				if slot != nil && !isempty(slot) {
					L.stack[ra] = *slot
				} else {
					savestate(L, ci, pc)
					luaV_finishget(L, rb, rc, ra, slot)
				}
			case OP_ADDI, OP_ADDK, OP_SUBK, OP_MULK, OP_MODK, OP_POWK,
				OP_DIVK, OP_IDIVK, OP_BANDK, OP_BORK, OP_BXORK:
				/* Fast path, not in lvm.c: an integer register with
				   an integer immediate or constant can never reach a
				   metamethod and always produces an integer, so the
				   arithmetic is done here instead of walking
				   opCodeToArith -> luaO_rawarith -> intarith. It is
				   spelled out inside the loop rather than moved to a
				   helper because the compiler will not inline such a
				   helper (it is over the cost budget as soon as it
				   can raise), and the call would cost about as much
				   as the operation itself. */
				rb := &L.stack[base+i.B]
				if rb.tt == LUA_VNUMINT && i.Op != OP_POWK && i.Op != OP_DIVK {
					v2 := int64(i.C - OFFSET_sC) /* the OP_ADDI immediate */
					if i.Op != OP_ADDI {
						kc := &k[i.C]
						if kc.tt != LUA_VNUMINT {
							vmArithK(L, ci, i, ra, base, k, &pc)
							break
						}
						v2 = kc.i
					}
					/* '%' and '//' can raise (division by zero), so the
					   current instruction must be saved first: that is
					   what puts the right line in the error message. */
					if i.Op == OP_MODK || i.Op == OP_IDIVK {
						savestate(L, ci, pc)
					}
					pc++
					switch i.Op {
					case OP_ADDI, OP_ADDK:
						setivalue(&L.stack[ra], rb.i+v2)
					case OP_SUBK:
						setivalue(&L.stack[ra], rb.i-v2)
					case OP_MULK:
						setivalue(&L.stack[ra], rb.i*v2)
					case OP_MODK:
						if uint64(v2)+1 <= 1 {
							setivalue(&L.stack[ra], luaV_mod(L, rb.i, v2))
						} else {
							setivalue(&L.stack[ra], luaV_modcore(rb.i, v2))
						}
					case OP_IDIVK:
						if uint64(v2)+1 <= 1 {
							setivalue(&L.stack[ra], luaV_idiv(L, rb.i, v2))
						} else {
							setivalue(&L.stack[ra], luaV_idivcore(rb.i, v2))
						}
					case OP_BANDK:
						setivalue(&L.stack[ra], rb.i&v2)
					case OP_BORK:
						setivalue(&L.stack[ra], rb.i|v2)
					case OP_BXORK:
						setivalue(&L.stack[ra], rb.i^v2)
					}
					break
				}
				vmArithK(L, ci, i, ra, base, k, &pc)
			case OP_SHRI, OP_SHLI:
				rb := &L.stack[base+i.B]
				ic := int64(i.C - OFFSET_sC)
				var ib int64
				if tointegerns(rb, &ib) {
					pc++
					if i.Op == OP_SHRI {
						setivalue(&L.stack[ra], luaV_shiftl(ib, -ic))
					} else {
						setivalue(&L.stack[ra], luaV_shiftl(ic, ib))
					}
				}
			case OP_ADD, OP_SUB, OP_MUL, OP_BAND, OP_BOR, OP_BXOR,
				OP_SHL, OP_SHR:
				/* Fast path, as for the immediate and constant forms
				   above: two integers can never reach a metamethod
				   and always produce an integer. None of these eight
				   can raise, so nothing has to be saved first. */
				rb := &L.stack[base+i.B]
				rc := &L.stack[base+i.C]
				if rb.tt == LUA_VNUMINT && rc.tt == LUA_VNUMINT {
					v1, v2 := rb.i, rc.i
					var r int64
					switch i.Op {
					case OP_ADD:
						r = v1 + v2
					case OP_SUB:
						r = v1 - v2
					case OP_MUL:
						r = v1 * v2
					case OP_BAND:
						r = v1 & v2
					case OP_BOR:
						r = v1 | v2
					case OP_BXOR:
						r = v1 ^ v2
					case OP_SHL:
						r = int64(luaV_shiftl(v1, v2))
					case OP_SHR:
						r = int64(luaV_shiftl(v1, -v2))
					}
					pc++
					setivalue(&L.stack[ra], r)
					break
				}
				var res Value
				op := opCodeToArith(i.Op)
				if luaO_rawarith(L, op, rb, rc, &res) {
					pc++
					L.stack[ra] = res
				}
			case OP_MOD, OP_IDIV:
				/* these two can raise (division by zero), so the
				   current instruction must be saved before running
				   them: that is what puts the right line in the
				   error message. */
				savestate(L, ci, pc)
				rb := &L.stack[base+i.B]
				rc := &L.stack[base+i.C]
				if rb.tt == LUA_VNUMINT && rc.tt == LUA_VNUMINT {
					pc++
					if uint64(rc.i)+1 <= 1 {
						/* the divisor is -1 or 0: the two that need
						   care, one of them fatally */
						if i.Op == OP_MOD {
							setivalue(&L.stack[ra], luaV_mod(L, rb.i, rc.i))
						} else {
							setivalue(&L.stack[ra], luaV_idiv(L, rb.i, rc.i))
						}
					} else if i.Op == OP_MOD {
						setivalue(&L.stack[ra], luaV_modcore(rb.i, rc.i))
					} else {
						setivalue(&L.stack[ra], luaV_idivcore(rb.i, rc.i))
					}
					break
				}
				var res Value
				op := opCodeToArith(i.Op)
				if luaO_rawarith(L, op, rb, rc, &res) {
					pc++
					L.stack[ra] = res
				}
			case OP_POW, OP_DIV:
				/* these two always produce a float, so there is no
				   integer fast path for them */
				rb := &L.stack[base+i.B]
				rc := &L.stack[base+i.C]
				var res Value
				op := opCodeToArith(i.Op)
				if luaO_rawarith(L, op, rb, rc, &res) {
					pc++
					L.stack[ra] = res
				}
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
					setivalue(&L.stack[ra], -ivalue(rb))
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
					setivalue(&L.stack[ra], ^ib)
				} else {
					savestate(L, ci, pc)
					luaT_trybinTM(L, rb, rb, ra, TM_BNOT)
				}
			case OP_NOT:
				setbvalue(&L.stack[ra], isFalse(&L.stack[base+i.B]))
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
				luaF_close(L, ra, LUA_OK, true)
			case OP_TBC:
				savestate(L, ci, pc) /* in case of errors */
				luaF_newtbcupval(L, ra)
			case OP_JMP:
				pc += i.B - OFFSET_sJ
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
				}
			case OP_LT:
				savestate(L, ci, pc)
				cond := luaV_lessthan(L, &L.stack[ra], &L.stack[base+i.B])
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
				}
			case OP_LE:
				savestate(L, ci, pc)
				cond := luaV_lessequal(L, &L.stack[ra], &L.stack[base+i.B])
				condBool := 0
				if cond {
					condBool = 1
				}
				if condBool != i.K {
					pc++
				} else {
					pc += code[pc].B - OFFSET_sJ + 1
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
				}
			case OP_EQI, OP_LTI, OP_LEI, OP_GTI, OP_GEI:
				im := int64(i.B - OFFSET_sC)
				a := &L.stack[ra]
				cond := false
				if ttisinteger(a) {
					switch i.Op {
					case OP_EQI:
						cond = ivalue(a) == im
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
					case OP_EQI:
						cond = fa == fim
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
				}
			case OP_TESTSET:
				rb := &L.stack[base+i.B]
				if isFalse(rb) == (i.K != 0) {
					pc++
				} else {
					L.stack[ra] = *rb
					pc += code[pc].B - OFFSET_sJ + 1
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
				} else { /* Go function */
					ci.funcIdx -= delta    /* restore 'func' (if vararg) */
					luaD_poscall(L, ci, n) /* finish the caller */
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
						idx += step
						chgivalue(&L.stack[ra], idx)
						setivalue(&L.stack[ra+3], idx)
						pc -= i.B
					}
				} else if floatforloop(L, ra) {
					pc -= i.B
				}
			case OP_FORPREP:
				savestate(L, ci, pc)
				if forprep(L, ra) {
					pc += i.B + 1 /* skip the loop */
				}
			case OP_TFORPREP:
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
				ra5 := base + i.A
				if !ttisnil(&L.stack[ra5+4]) {
					L.stack[ra5+2] = L.stack[ra5+4]
					pc -= i.B
				}
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
			case OP_VARARG:
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
			/* updatetrap: the operation may have changed the hooks (or
			   moved the stack), so re-read the mark of this frame.  C
			   does this only where 'Protect'/'ProtectNT' wraps such an
			   operation; doing it on every instruction is the same
			   thing as far as the hooks are concerned (it is a strict
			   superset) but it costs a store on the hottest path of
			   the interpreter, so it is skipped while no hook is
			   installed -- which is the only situation in which
			   'ci.trap' can have changed behind our back anyway. */
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

// vmArithK handles the immediate/constant arithmetic opcodes; on success
// it skips the following MMBIN* instruction (as PUC-Lua does).
func vmArithK(L *State, ci *CallInfo, i Instruction, ra, base int, k []Value, pc *int) {
	rb := &L.stack[base+i.B]
	var res Value
	ok := false
	switch i.Op {
	case OP_ADDI:
		imm := int64(i.C - OFFSET_sC)
		if ttisinteger(rb) {
			setivalue(&res, ivalue(rb)+imm)
			ok = true
		} else if ttisfloat(rb) {
			setfltvalue(&res, fltvalue(rb)+float64(imm))
			ok = true
		}
	default:
		v2 := &k[i.C]
		op := opCodeToArith(i.Op)
		/* '%' and '//' can raise (division by zero): save the current
		   instruction first, as C does */
		if i.Op == OP_MODK || i.Op == OP_IDIVK {
			savestate(L, ci, *pc)
		}
		ok = luaO_rawarith(L, op, rb, v2, &res)
	}
	if ok {
		*pc++
		L.stack[ra] = res
	}
}

// opCodeToArith maps an opcode to its LUA_OP* code. It is only used by
// the slow (non integer/integer) paths; the integer fast paths are
// spelled out in luaV_execute.
func opCodeToArith(op int) int {
	switch op {
	case OP_ADDI, OP_ADDK, OP_ADD:
		return LUA_OPADD
	case OP_SUBK, OP_SUB:
		return LUA_OPSUB
	case OP_MULK, OP_MUL:
		return LUA_OPMUL
	case OP_MODK, OP_MOD:
		return LUA_OPMOD
	case OP_POWK, OP_POW:
		return LUA_OPPOW
	case OP_DIVK, OP_DIV:
		return LUA_OPDIV
	case OP_IDIVK, OP_IDIV:
		return LUA_OPIDIV
	case OP_BANDK, OP_BAND:
		return LUA_OPBAND
	case OP_BORK, OP_BOR:
		return LUA_OPBOR
	case OP_BXORK, OP_BXOR:
		return LUA_OPBXOR
	case OP_SHL:
		return LUA_OPSHL
	case OP_SHR:
		return LUA_OPSHR
	}
	panic("not an arithmetic opcode")
}

// math helper used by float loops
func numIsNaN(n float64) bool { return math.IsNaN(n) }
