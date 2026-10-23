// Translation of lundump.c / lundump.h (PUC-Lua 5.4.9): load precompiled
// chunks.
//
// The file format is *the official one*, byte for byte, so chunks dumped
// by this package can be loaded by PUC-Lua (and vice versa, as long as
// the host is little-endian, which is what 'luac' produces on x86/ARM).
//
// Intentional Go specializations (see NOTES.md FLAG items):
//
//  1. Go GC — no luaC_objbarrier; no stack-anchor of long strings while
//     reading (C does setsvalue2s + luaD_inctop around loadVector).
//
//  2. panic/recover — C luaD_throw (setjmp); caught by rawRunProtected
//     in luaU_loadBinary → nil, message like lua_load.
//
//  3. Instruction is a struct, not a packed 32-bit word — every on-disk
//     word is unpacked with unpackInstruction (bit layout from lopcodes.h).
//
//  4. Proto.lineinfo holds *absolute* lines (see lobject.go). The file
//     format stores signed byte deltas plus abslineinfo; loadDebug
//     converts on load (mirrors luaG_getfuncline), encodeLineInfo (ldump.go)
//     converts back.
//
//  5. LoadState uses []byte+pos instead of ZIO*; fixed LE widths for
//     lua_Integer / lua_Number / Instruction in the header check.
//
//  6. checkLoadMode + luaU_loadBinary colocated here (C: ldo.c checkmode /
//     f_parser + lapi.c lua_load _ENV setup) — not in C lundump.c.
package core

import (
	"encoding/binary"
	"math"
	"strings"
)

/* lundump.h — data to catch conversion errors */
const (
	dmpSignature = "\x1bLua"            /* LUA_SIGNATURE (lua.h) */
	dmpData      = "\x19\x93\r\n\x1a\n" /* LUAC_DATA */
	dmpInt       = 0x5678               /* LUAC_INT */
	dmpNum       = 370.5                /* LUAC_NUM cast_num(370.5) */
	/* LUAC_VERSION: ((LUA_VERSION_NUM/100)*16) + LUA_VERSION_NUM%100 → 0x54 */
	dmpVersion = 0x54
	dmpFormat  = 0 /* LUAC_FORMAT: the official format */
)

/* ldebug.h / lcode.c — line-info constants (shared with ldump.go) */
const (
	absLineInfo = -0x80 /* ABSLINEINFO */
	limLineDiff = 0x80  /* LIMLINEDIFF */
	maxIwthAbs  = 128   /* MAXIWTHABS */
)

/*
** LoadState — C: { lua_State *L; ZIO *Z; const char *name; }
** FLAG: ZIO* → in-memory []byte + pos (no streaming reader on this path).
 */
type LoadState struct {
	L    *State
	data []byte
	pos  int
	name string
}

/* error (lundump.c): "%s: bad binary format (%s)" + LUA_ERRSYNTAX */
func (S *LoadState) loadError(why string) {
	ts := pushfString(S.L, "%s: bad binary format (%s)", S.name, why)
	S.L.checkStack(1)
	setsvalue(&S.L.stack[S.L.top], ts)
	S.L.top++
	luaD_throw(S.L, LUA_ERRSYNTAX)
}

/*
** loadBlock — C loadBlock via loadVector.
** All high-level loads go through this; endianness of multi-byte values
** is fixed little-endian in the typed loaders (FLAG).
 */
func (S *LoadState) loadBlock(n int) []byte {
	if n < 0 || S.pos+n > len(S.data) {
		S.loadError("truncated chunk")
	}
	b := S.data[S.pos : S.pos+n]
	S.pos += n
	return b
}

/* loadByte */
func (S *LoadState) loadByte() byte {
	return S.loadBlock(1)[0]
}

/*
** loadUnsigned — 7-bits-per-byte integer (lundump.c loadUnsigned).
** Continuation while (b & 0x80) == 0; last byte has high bit set.
 */
func (S *LoadState) loadUnsigned(limit int64) int64 {
	var x int64
	limit >>= 7
	for {
		b := int64(S.loadByte())
		if x >= limit {
			S.loadError("integer overflow")
		}
		x = (x << 7) | (b & 0x7f)
		if b&0x80 != 0 { /* last byte */
			break
		}
	}
	return x
}

/* loadSize — loadUnsigned(S, MAX_SIZET); Go uses MaxInt64 (FLAG overflow ceiling) */
func (S *LoadState) loadSize() int64 { return S.loadUnsigned(math.MaxInt64) }

/* loadInt — cast_int(loadUnsigned(S, INT_MAX)) */
func (S *LoadState) loadInt() int { return int(S.loadUnsigned(math.MaxInt32)) }

/*
** loadCount — Go hardening around loadInt: reject n<0 and sizes that
** cannot fit 'unit' bytes per element in the remaining buffer. C would
** fail inside luaM_newvectorchecked / ZIO. Valid dumps unchanged. (FLAG)
 */
func (S *LoadState) loadCount(unit int) int {
	n := S.loadInt()
	if n < 0 {
		S.loadError("integer overflow")
	}
	if n > (len(S.data)-S.pos)/unit {
		S.loadError("truncated chunk")
	}
	return n
}

/* loadNumber — loadVar of lua_Number (8 bytes, LE) */
func (S *LoadState) loadNumber() float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

/* loadInteger — loadVar of lua_Integer (8 bytes, LE) */
func (S *LoadState) loadInteger() int64 {
	return int64(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

/*
** loadStringN — Load a nullable string into prototype 'p'.
** Size encoding: loadSize; 0 → NULL; else --size, then bytes.
** Short (size <= LUAI_MAXSHORTLEN) via luaS_newlstr; else createlngstrobj.
** FLAG: no stack anchor / luaC_objbarrier (Go GC).
 */
func (S *LoadState) loadStringN(p *Proto) *TString {
	size := S.loadSize()
	if size == 0 { /* no string? */
		return nil
	}
	size-- /* C: --size in the else-if condition; dump writes len+1 */
	/* Go hardening: reject absurd size (C trusts size_t / ZIO). */
	if size < 0 || size > math.MaxInt32 {
		S.loadError("integer overflow")
	}
	var ts *TString
	if size <= LUAI_MAXSHORTLEN { /* short string? */
		b := S.loadBlock(int(size))       /* load string into buffer */
		ts = luaS_newlstr(S.L, string(b)) /* create string */
	} else { /* long string */
		/* C: createlngstrobj, setsvalue2s+inctop anchor, loadVector, pop.
		   FLAG: load then createlngstrobj (no stack anchor / Go GC). */
		b := S.loadBlock(int(size))
		ts = luaS_createlngstrobj(S.L, string(b))
	}
	_ = p /* C: luaC_objbarrier(L, p, ts) — Go GC */
	return ts
}

/* loadString — non-nullable; NULL → "bad format for constant string" */
func (S *LoadState) loadString(p *Proto) *TString {
	st := S.loadStringN(p)
	if st == nil {
		S.loadError("bad format for constant string")
	}
	return st
}

/*
** loadCode — loadInt n; allocate code[n]; loadVector of Instructions.
** FLAG: each 32-bit word is unpacked into Instruction struct fields.
** Uses loadCount(4) hardening (C: loadInt + luaM_newvectorchecked).
 */
func (S *LoadState) loadCode(f *Proto) {
	n := S.loadCount(4)
	f.code = make([]Instruction, n)
	f.sizecode = n
	for i := 0; i < n; i++ {
		f.code[i] = unpackInstruction(binary.LittleEndian.Uint32(S.loadBlock(4)))
	}
}

/* loadFunction forward decl equivalent — Go methods need no forward decl */

/*
** loadConstants — tags LUA_VNIL / VFALSE / VTRUE / VNUMFLT / VNUMINT /
** VSHRSTR / VLNGSTR (5.4 variant tags). C first setnilvalue's the vector
** for GC; Go make zeros Values (nil). Unknown tag → loadError (C lua_assert).
 */
func (S *LoadState) loadConstants(f *Proto) {
	n := S.loadCount(1)
	f.k = make([]Value, n)
	f.sizek = n
	for i := 0; i < n; i++ {
		setnilvalue(&f.k[i]) /* make array valid for GC (C) */
	}
	for i := 0; i < n; i++ {
		o := &f.k[i]
		switch S.loadByte() {
		case LUA_VNIL:
			setnilvalue(o)
		case LUA_VFALSE:
			setbfvalue(o)
		case LUA_VTRUE:
			setbtvalue(o)
		case LUA_VNUMFLT:
			setfltvalue(o, S.loadNumber())
		case LUA_VNUMINT:
			setivalue(o, S.loadInteger())
		case LUA_VSHRSTR, LUA_VLNGSTR:
			setsvalue(o, S.loadString(f)) /* C: setsvalue2n */
		default:
			S.loadError("bad constant type") /* C: lua_assert(0) */
		}
	}
}

/*
** loadProtos — nested prototypes; each luaF_newproto then loadFunction
** with parent source. FLAG: no luaC_objbarrier.
 */
func (S *LoadState) loadProtos(f *Proto) {
	n := S.loadCount(1)
	f.p = make([]*Proto, n)
	f.sizep = n
	for i := 0; i < n; i++ {
		f.p[i] = nil /* make array valid for GC (C) */
	}
	for i := 0; i < n; i++ {
		f.p[i] = luaF_newproto(S.L)
		/* luaC_objbarrier(S->L, f, f->p[i]); — Go GC */
		S.loadFunction(f.p[i], f.source)
	}
}

/*
** loadUpvalues — names filled first as NULL (make zeros); then
** instack, idx, kind (5.4; 5.3 had no kind). Order matters in C so an
** error mid-read leaves GC-consistent protos.
 */
func (S *LoadState) loadUpvalues(f *Proto) {
	n := S.loadCount(3)
	f.upvalues = make([]Upvaldesc, n)
	f.sizeupvalues = n
	for i := 0; i < n; i++ { /* make array valid for GC */
		f.upvalues[i].name = nil
	}
	for i := 0; i < n; i++ { /* following calls can raise errors */
		f.upvalues[i].instack = S.loadByte()
		f.upvalues[i].idx = S.loadByte()
		f.upvalues[i].kind = S.loadByte()
	}
}

/*
** loadDebug — lineinfo/abslineinfo, locvars, upvalue names (lundump.c).
** FLAG #4: on-disk signed-byte deltas + abslineinfo are converted into
** Proto.lineinfo absolute lines (mirrors luaG_getfuncline); abslineinfo
** retained for dump round-trip. C stores ls_byte deltas natively.
 */
func (S *LoadState) loadDebug(f *Proto) {
	n := S.loadCount(1)
	raw := S.loadBlock(n) /* C: loadVector into f->lineinfo (ls_byte) */
	nabs := S.loadCount(2)
	abs := make([]AbsLineInfo, nabs)
	for i := 0; i < nabs; i++ {
		abs[i].pc = S.loadInt()
		abs[i].line = S.loadInt()
	}
	/* convert deltas → absolute lines (FLAG #4); C keeps ls_byte deltas.
	   ABSLINEINFO markers + abslineinfo[] mirror luaG_getfuncline. */
	lines := make([]int32, n)
	baseline := f.linedefined
	ai := 0
	for pc := 0; pc < n; pc++ {
		linedif := int8(raw[pc])
		if linedif == int8(absLineInfo) { /* ABSLINEINFO */
			if ai < nabs && abs[ai].pc == pc {
				baseline = abs[ai].line
				ai++
			}
			lines[pc] = int32(baseline)
			continue
		}
		baseline += int(linedif)
		lines[pc] = int32(baseline)
	}
	f.lineinfo = lines
	f.sizelineinfo = n
	f.abslineinfo = abs
	f.sizeabslineinfo = nabs /* C: f->sizeabslineinfo = n */

	n = S.loadCount(1)
	f.locvars = make([]LocVar, n)
	f.sizelocvars = n
	for i := 0; i < n; i++ {
		f.locvars[i].varname = nil
	}
	for i := 0; i < n; i++ {
		f.locvars[i].varname = S.loadStringN(f)
		f.locvars[i].startpc = S.loadInt()
		f.locvars[i].endpc = S.loadInt()
	}
	n = S.loadInt()
	if n != 0 { /* does it have debug information? */
		n = f.sizeupvalues /* must be this many */
	}
	for i := 0; i < n; i++ {
		f.upvalues[i].name = S.loadStringN(f)
	}
}

/* loadFunction — source, lines, params, vararg, maxstack, then body */
func (S *LoadState) loadFunction(f *Proto, psource *TString) {
	f.source = S.loadStringN(f)
	if f.source == nil { /* no source in dump? */
		f.source = psource /* reuse parent's source */
	}
	f.linedefined = S.loadInt()
	f.lastlinedefined = S.loadInt()
	f.numparams = S.loadByte()
	f.isVararg = S.loadByte() /* C: is_vararg */
	f.maxstacksize = S.loadByte()
	S.loadCode(f)
	S.loadConstants(f)
	S.loadUpvalues(f)
	S.loadProtos(f)
	S.loadDebug(f)
}

/* checkliteral */
func (S *LoadState) checkLiteral(s, msg string) {
	if string(S.loadBlock(len(s))) != s {
		S.loadError(msg)
	}
}

/* fchecksize — sizeof recorded in header must match */
func (S *LoadState) checkSize(size int, tname string) {
	if int(S.loadByte()) != size {
		S.loadError(pushfString(S.L, "%s size mismatch", tname).val)
	}
}

/*
** checkHeader — skip 1st char (already read/checked by caller), then
** signature[1:], version, format, LUAC_DATA, sizeof Instruction /
** lua_Integer / lua_Number, LUAC_INT, LUAC_NUM.
** FLAG: sizes hardcoded 4/8/8 (LE fixed width).
 */
func (S *LoadState) checkHeader() {
	/* skip 1st char (already read and checked) */
	S.checkLiteral(dmpSignature[1:], "not a binary chunk")
	if S.loadByte() != dmpVersion {
		S.loadError("version mismatch")
	}
	if S.loadByte() != dmpFormat {
		S.loadError("format mismatch")
	}
	S.checkLiteral(dmpData, "corrupted chunk")
	S.checkSize(4, "Instruction") /* checksize(S, Instruction) */
	S.checkSize(8, "lua_Integer")
	S.checkSize(8, "lua_Number")
	if S.loadInteger() != dmpInt {
		S.loadError("integer format mismatch")
	}
	if S.loadNumber() != dmpNum {
		S.loadError("float format mismatch")
	}
}

/*
** luaU_undump — Load precompiled chunk (lundump.c).
** Pushes the closure on the stack (setclLvalue2s + luaD_inctop), matching C.
** FLAG: no luaC_objbarrier; luai_verifycode empty by default in C.
** _ENV setup lives in luaU_loadBinary (C: lua_load).
 */
func luaU_undump(L *State, data []byte, name string) *LClosure {
	var S LoadState
	switch {
	case len(name) > 0 && (name[0] == '@' || name[0] == '='):
		S.name = name[1:]
	case len(name) > 0 && name[0] == dmpSignature[0]:
		S.name = "binary string"
	default:
		S.name = name
	}
	S.L = L
	S.data = data
	/* first byte is what lua_load / f_parser looked at; checkHeader skips it */
	if len(data) == 0 || data[0] != dmpSignature[0] {
		S.loadError("not a binary chunk")
	}
	S.pos = 1
	S.checkHeader()
	cl := luaF_newLclosure(L, int(S.loadByte()))
	setclLvalue(&L.stack[L.top], cl) /* setclLvalue2s */
	luaD_inctop(L)
	cl.p = luaF_newproto(L)
	/* luaC_objbarrier(L, cl, cl->p); — Go GC */
	S.loadFunction(cl.p, nil)
	/* lua_assert(cl->nupvalues == cl->p->sizeupvalues); */
	/* luai_verifycode(L, cl.p) — empty by default in C */
	return cl
}

/*
** checkLoadMode — checkmode from ldo.c (colocated; FLAG #6).
** C: if (mode && strchr(mode, x[0]) == NULL). Empty mode ≈ NULL (skip).
 */
func checkLoadMode(L *State, mode, x string) {
	if mode != "" && !strings.ContainsRune(mode, rune(x[0])) {
		ts := pushfString(L, "attempt to load a %s chunk (mode is '%s')", x, mode)
		L.checkStack(1)
		setsvalue(&L.stack[L.top], ts)
		L.top++
		luaD_throw(L, LUA_ERRSYNTAX)
	}
}

/*
** luaU_loadBinary — precompiled path of lua_load / f_parser (colocated).
** checkmode → undump (pushes cl) → luaF_initupvals → set _ENV upvalue.
 */
func luaU_loadBinary(L *State, data []byte, chunkname, mode string) error {
	oldtop := L.top
	status := L.rawRunProtected(func() {
		checkLoadMode(L, mode, "binary")
		cl := luaU_undump(L, data, chunkname) /* closure already on stack */
		luaF_initupvals(L, cl)
		if cl.nupvalues >= 1 {
			/* set the globals table as the first upvalue (the _ENV) */
			var gt Value
			sethvalue(&gt, L.rawGetRegistryGlobals())
			setobj(&cl.upvals[0].v, &gt)
		}
	})
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

/* ---------------------------------------------------------------------
** instruction (un)packing (lopcodes.h) — shared with ldump.go
** Uses POS_/SIZE_ constants and getOpMode from lopcodes.go (luaP_opmodes).
** --------------------------------------------------------------------- */

/* instrMask — MASK1(size) in lopcodes.h (bit macros otherwise omitted). */
func instrMask(size int) uint32 { return uint32(1)<<uint(size) - 1 }

/*
** packInstruction — CREATE_ABCk / CREATE_ABx / CREATE_Ax / CREATE_sJ.
** Go fields already hold encoded operands: B is Bx for iABx/iAsBx
** (already biased by OFFSET_sBx for sBx) and raw sJ for isJ; A is Ax
** for iAx.
 */
func packInstruction(i Instruction) uint32 {
	if i.Op < 0 || i.Op >= NUM_OPCODES {
		return uint32(i.Op) << POS_OP
	}
	var w uint32
	switch getOpMode(i.Op) {
	case iABC:
		w = uint32(i.Op)<<POS_OP |
			uint32(i.A)<<POS_A |
			uint32(i.B)<<POS_B |
			uint32(i.C)<<POS_C |
			uint32(i.K)<<POS_k
	case iABx, iAsBx:
		w = uint32(i.Op)<<POS_OP |
			uint32(i.A)<<POS_A |
			uint32(i.B)<<POS_Bx
	case iAx:
		w = uint32(i.Op)<<POS_OP |
			uint32(i.A)<<POS_Ax
	case isJ:
		w = uint32(i.Op)<<POS_OP |
			uint32(i.B)<<POS_sJ |
			uint32(i.K)<<POS_k
	}
	return w
}

/* unpackInstruction — GET_OPCODE / GETARG_A / …; unknown op → iABC fields. */
func unpackInstruction(w uint32) Instruction {
	op := int(w>>POS_OP) & int(instrMask(SIZE_OP))
	mode := iABC /* C stores raw Instruction words; tolerate out-of-range op */
	if op >= 0 && op < NUM_OPCODES {
		mode = getOpMode(op)
	}
	switch mode {
	case iABC:
		return Instruction{
			Op: op,
			A:  int(w>>POS_A) & int(instrMask(SIZE_A)),
			B:  int(w>>POS_B) & int(instrMask(SIZE_B)),
			C:  int(w>>POS_C) & int(instrMask(SIZE_C)),
			K:  int(w>>POS_k) & int(instrMask(1)),
		}
	case iABx, iAsBx:
		return Instruction{
			Op: op,
			A:  int(w>>POS_A) & int(instrMask(SIZE_A)),
			B:  int(w>>POS_Bx) & int(instrMask(SIZE_Bx)),
		}
	case iAx:
		return Instruction{
			Op: op,
			A:  int(w>>POS_Ax) & int(instrMask(SIZE_Ax)),
		}
	}
	/* isJ */
	return Instruction{
		Op: op,
		B:  int(w>>POS_sJ) & int(instrMask(SIZE_sJ)),
		K:  int(w>>POS_k) & int(instrMask(1)),
	}
}
