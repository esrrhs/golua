// Translation of ldump.c (PUC-Lua 5.4.9): save precompiled Lua chunks.
//
// Format matches PUC-Lua 5.4.9 on little-endian hosts: 7-bit size encoding,
// header without sizeof(int)/sizeof(size_t), upvalue 'kind', ttypetag
// constants, and debug lineinfo as signed-byte deltas + abslineinfo.
//
// Intentional Go specializations (FLAG):
//  1. No lua_lock / lua_unlock (Go has no VM locks).
//  2. lua_Writer void* data closed over by dumpWriter (no DumpState.data).
//  3. dumpInteger / dumpNumber / dumpCode write fixed little-endian
//     (C dumpVar uses host endianness via dumpVector).
//  4. Instruction is a struct — packInstruction before writing (see lopcodes).
//  5. Proto.lineinfo holds absolute lines (lobject.go FLAG); dumpDebug
//     converts to on-disk deltas + abslineinfo (C dumps those fields as-is).
//     When undump retained abslineinfo, deltas are reconstructed from it for
//     bit-identical round-trip; otherwise savelineinfo rules regenerate both.
//  6. luaDump (lua_dump from lapi.c) colocated as a thin wrapper.
package core

import (
	"encoding/binary"
	"math"
)

/*
** All high-level dumps go through dumpVector; you can change it to
** change the endianness of the result
 */
/* FLAG: Go has no dumpVector macro; dumpBlock + explicit LE packers. */

// dumpWriter is the Go counterpart of lua_Writer: it returns 0 on
// success and anything else to stop the dump. C also passes a void*
// data pointer; Go closes over that in the func value.
type dumpWriter func(L *State, b []byte) int

// DumpState is the Go counterpart of the C struct of the same name
// (no 'data' field; carried by dumpWriter).
type DumpState struct {
	L      *State
	writer dumpWriter
	strip  bool /* C: int */
	status int
}

// dumpBlock writes raw bytes (dumpBlock).
func (D *DumpState) dumpBlock(b []byte) {
	if D.status == 0 && len(b) > 0 {
		/* FLAG: no lua_unlock / lua_lock around the writer call */
		D.status = D.writer(D.L, b)
	}
}

// dumpByte writes a single byte (dumpByte).
func (D *DumpState) dumpByte(y int) {
	x := byte(y) /* lu_byte */
	D.dumpBlock([]byte{x})
}

/*
** 'dumpSize' buffer size: each byte can store up to 7 bits. (The "+6"
** rounds up the division.)
** DIBS = ((sizeof(size_t) * CHAR_BIT + 6) / 7) = ((8*8+6)/7) = 10
 */
const dmpDibs = 10

// dumpSize writes a size_t in 7-bits-per-byte form (dumpSize).
func (D *DumpState) dumpSize(x uint64) {
	var buff [dmpDibs]byte
	n := 0
	for {
		buff[dmpDibs-(n+1)] = byte(x & 0x7f) /* fill buffer in reverse order */
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	buff[dmpDibs-1] |= 0x80 /* mark last byte */
	D.dumpBlock(buff[dmpDibs-n : dmpDibs])
}

// dumpInt writes an int via dumpSize (dumpInt).
func (D *DumpState) dumpInt(x int) {
	D.dumpSize(uint64(x))
}

// dumpNumber writes a lua_Number (dumpNumber) — fixed little-endian.
func (D *DumpState) dumpNumber(x float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
	D.dumpBlock(b[:])
}

// dumpInteger writes a lua_Integer (dumpInteger) — fixed little-endian.
func (D *DumpState) dumpInteger(x int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(x))
	D.dumpBlock(b[:])
}

// dumpString writes a nullable string (dumpString): size 0 means NULL;
// otherwise size+1 then the bytes (no trailing NUL on disk).
func (D *DumpState) dumpString(s *TString) {
	if s == nil {
		D.dumpSize(0)
	} else {
		size := uint64(tsslen(s))
		str := getstr(s)
		D.dumpSize(size + 1)
		D.dumpBlock([]byte(str))
	}
}

// dumpCode writes the bytecode of a function (dumpCode).
func (D *DumpState) dumpCode(f *Proto) {
	n := f.sizecode /* C: f->sizecode */
	D.dumpInt(n)
	/* FLAG: pack Instruction structs into 32-bit LE words (C dumpVector). */
	if n <= 0 {
		return
	}
	b := make([]byte, 4*n)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(b[4*i:], packInstruction(f.code[i]))
	}
	D.dumpBlock(b)
}

// dumpFunction is forward-declared in C; Go methods need no forward decl.

// dumpConstants writes the constant table (dumpConstants).
func (D *DumpState) dumpConstants(f *Proto) {
	n := f.sizek /* C: f->sizek */
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		o := &f.k[i]
		tt := ttypetag(o)
		D.dumpByte(tt)
		switch tt {
		case LUA_VNUMFLT:
			D.dumpNumber(fltvalue(o))
		case LUA_VNUMINT:
			D.dumpInteger(ivalue(o))
		case LUA_VSHRSTR, LUA_VLNGSTR:
			D.dumpString(tsvalue(o))
		default:
			/* C: lua_assert(tt == LUA_VNIL || tt == LUA_VFALSE || tt == LUA_VTRUE); */
		}
	}
}

// dumpProtos writes nested prototypes (dumpProtos).
func (D *DumpState) dumpProtos(f *Proto) {
	n := f.sizep /* C: f->sizep */
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpFunction(f.p[i], f.source)
	}
}

// dumpUpvalues writes upvalue descriptions (dumpUpvalues), including
// the 5.4 'kind' byte.
func (D *DumpState) dumpUpvalues(f *Proto) {
	n := f.sizeupvalues /* C: f->sizeupvalues */
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpByte(int(f.upvalues[i].instack))
		D.dumpByte(int(f.upvalues[i].idx))
		D.dumpByte(int(f.upvalues[i].kind))
	}
}

/*
** encodeLineInfo converts Proto.lineinfo (absolute lines; see lobject.go /
** lcode.go) into the on-disk form C stores natively: signed-byte deltas
** plus AbsLineInfo entries — same algorithm as lcode.c savelineinfo.
** Used when Proto has no retained abslineinfo (compiled functions).
 */
func encodeLineInfo(f *Proto, n int) (deltas []byte, abs []AbsLineInfo) {
	deltas = make([]byte, n)
	previousline := f.linedefined
	iwthabs := 0
	for pc := 0; pc < n; pc++ {
		line := int(f.lineinfo[pc])
		linedif := line - previousline
		/* C savelineinfo: abs(linedif) >= LIMLINEDIFF || iwthabs++ >= MAXIWTHABS */
		if linedif >= limLineDiff || linedif <= -limLineDiff ||
			iwthabs >= maxIwthAbs {
			abs = append(abs, AbsLineInfo{pc: pc, line: line})
			linedif = absLineInfo /* signal that there is absolute info */
			iwthabs = 1           /* restart counter */
		} else {
			iwthabs++
		}
		deltas[pc] = byte(int8(linedif))
		previousline = line
	}
	return deltas, abs
}

/*
** deltasFromAbsolute rebuilds on-disk signed-byte deltas so they match a
** retained abslineinfo table (from lundump). C dumpDebug dumps f->lineinfo
** (already deltas) and f->abslineinfo as-is; Go only keeps absolute lines
** in lineinfo, so this is the undump round-trip path.
 */
func deltasFromAbsolute(f *Proto, n int, abs []AbsLineInfo) []byte {
	deltas := make([]byte, n)
	previousline := f.linedefined
	ai := 0
	for pc := 0; pc < n; pc++ {
		line := int(f.lineinfo[pc])
		if ai < len(abs) && abs[ai].pc == pc {
			dif := absLineInfo /* ABSLINEINFO; via int to avoid const→byte overflow */
			deltas[pc] = byte(int8(dif))
			ai++
		} else {
			deltas[pc] = byte(int8(line - previousline))
		}
		previousline = line
	}
	return deltas
}

// lineInfoForDump returns on-disk deltas + abslineinfo for dumpDebug.
// Prefer retained abslineinfo (lundump round-trip); else savelineinfo regen.
func lineInfoForDump(f *Proto, n int) (deltas []byte, abs []AbsLineInfo) {
	if n <= 0 {
		return nil, nil
	}
	if f.sizeabslineinfo > 0 && len(f.abslineinfo) >= f.sizeabslineinfo {
		abs = f.abslineinfo[:f.sizeabslineinfo]
		return deltasFromAbsolute(f, n, abs), abs
	}
	return encodeLineInfo(f, n)
}

// dumpDebug writes debug information (dumpDebug).
func (D *DumpState) dumpDebug(f *Proto) {
	n := 0
	if !D.strip {
		n = f.sizelineinfo /* C: n = (D->strip) ? 0 : f->sizelineinfo */
	}
	deltas, abs := lineInfoForDump(f, n)
	D.dumpInt(n)
	D.dumpBlock(deltas) /* C: dumpVector(D, f->lineinfo, n) */
	n = 0
	if !D.strip {
		n = len(abs) /* C: f->sizeabslineinfo */
	}
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpInt(abs[i].pc)
		D.dumpInt(abs[i].line)
	}
	n = 0
	if !D.strip {
		n = f.sizelocvars /* C: f->sizelocvars */
	}
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpString(f.locvars[i].varname)
		D.dumpInt(f.locvars[i].startpc)
		D.dumpInt(f.locvars[i].endpc)
	}
	n = 0
	if !D.strip {
		n = f.sizeupvalues /* C: f->sizeupvalues */
	}
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpString(f.upvalues[i].name)
	}
}

// dumpFunction writes one prototype (dumpFunction); psource is the
// parent's source so a child with the same source omits it.
func (D *DumpState) dumpFunction(f *Proto, psource *TString) {
	if D.strip || f.source == psource {
		D.dumpString(nil) /* no debug info or same source as its parent */
	} else {
		D.dumpString(f.source)
	}
	D.dumpInt(f.linedefined)
	D.dumpInt(f.lastlinedefined)
	D.dumpByte(int(f.numparams))
	D.dumpByte(int(f.isVararg)) /* C: f->is_vararg */
	D.dumpByte(int(f.maxstacksize))
	D.dumpCode(f)
	D.dumpConstants(f)
	D.dumpUpvalues(f)
	D.dumpProtos(f)
	D.dumpDebug(f)
}

// dumpHeader writes the chunk header (dumpHeader). 5.4 omits
// sizeof(int) and sizeof(size_t) that 5.3 wrote.
func (D *DumpState) dumpHeader() {
	D.dumpBlock([]byte(dmpSignature)) /* LUA_SIGNATURE — C: dumpLiteral */
	D.dumpByte(dmpVersion)            /* LUAC_VERSION */
	D.dumpByte(dmpFormat)             /* LUAC_FORMAT */
	D.dumpBlock([]byte(dmpData))      /* LUAC_DATA — C: dumpLiteral */
	D.dumpByte(4)                     /* sizeof(Instruction) */
	D.dumpByte(8)                     /* sizeof(lua_Integer) */
	D.dumpByte(8)                     /* sizeof(lua_Number) */
	D.dumpInteger(dmpInt)             /* LUAC_INT */
	D.dumpNumber(dmpNum)              /* LUAC_NUM */
}

/*
** dump Lua function as precompiled chunk (luaU_dump).
** No writer 'data' arg — closed over by dumpWriter.
 */
func luaU_dump(L *State, f *Proto, w dumpWriter, strip bool) int {
	var D DumpState
	D.L = L
	D.writer = w
	D.strip = strip
	D.status = 0
	D.dumpHeader()
	D.dumpByte(f.sizeupvalues) /* C: DumpByte(f->sizeupvalues) */
	D.dumpFunction(f, nil)
	return D.status
}

// luaDump is lua_dump (lapi.c), colocated here as a thin wrapper.
// Non-L-function → 1. No api_checknelems / locks (FLAG).
func luaDump(L *State, w dumpWriter, strip bool) int {
	o := index2value(L, -1) /* C: s2v(L->top.p - 1) */
	if !isLfunction(o) {    /* C: isLfunction(o) */
		return 1
	}
	return luaU_dump(L, getproto(o), w, strip) /* C: getproto(o) */
}
