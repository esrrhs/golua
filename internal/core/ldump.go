// Translation of ldump.c (PUC-Lua 5.4.9): save precompiled Lua chunks.
//
// Format matches PUC-Lua 5.4.9 on little-endian hosts: 7-bit size encoding,
// header without sizeof(int)/sizeof(size_t), upvalue 'kind', ttypetag
// constants, and debug lineinfo as signed-byte deltas + abslineinfo.
package core

import (
	"encoding/binary"
	"math"
)

/*
** All high-level dumps go through dumpVector; you can change it to
** change the endianness of the result. Go writes fixed little-endian
** multi-byte values (see dumpInteger / dumpNumber / dumpCode).
 */

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

// dumpBlock writes raw bytes (dumpBlock). No lua_lock/lua_unlock (Go
// has no VM locks).
func (D *DumpState) dumpBlock(b []byte) {
	if D.status == 0 && len(b) > 0 {
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

// dumpInt writes an int via dumpSize (dumpInt) — 5.4 varint, not fixed width.
func (D *DumpState) dumpInt(x int) { D.dumpSize(uint64(x)) }

// dumpNumber writes a lua_Number (8 bytes, little-endian).
func (D *DumpState) dumpNumber(x float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
	D.dumpBlock(b[:])
}

// dumpInteger writes a lua_Integer (8 bytes, little-endian).
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
	if n <= 0 {
		return
	}
	b := make([]byte, 4*n)
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(b[4*i:], packInstruction(f.code[i]))
	}
	D.dumpBlock(b)
}

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
** dumpLineInfo converts Proto.lineinfo (absolute lines; see lobject.go /
** lcode.go) into the on-disk form C stores natively: signed-byte deltas
** plus AbsLineInfo entries — same algorithm as lcode.c savelineinfo.
** C dumpDebug dumps f->lineinfo / f->abslineinfo directly.
 */
func (D *DumpState) dumpLineInfo(f *Proto) {
	if D.strip {
		D.dumpInt(0) /* sizelineinfo */
		D.dumpInt(0) /* sizeabslineinfo */
		return
	}
	n := f.sizelineinfo /* C: f->sizelineinfo */
	deltas := make([]byte, n)
	var abs []AbsLineInfo
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
	D.dumpInt(n)
	D.dumpBlock(deltas)
	D.dumpInt(len(abs)) /* C: f->sizeabslineinfo; Proto has no sizeabslineinfo field */
	for i := 0; i < len(abs); i++ {
		D.dumpInt(abs[i].pc)
		D.dumpInt(abs[i].line)
	}
}

// dumpDebug writes debug information (dumpDebug).
func (D *DumpState) dumpDebug(f *Proto) {
	D.dumpLineInfo(f) /* lineinfo + abslineinfo (converted; see FLAG) */
	n := 0
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
	D.dumpByte(int(f.isVararg))
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
	D.dumpBlock([]byte(dmpSignature)) /* LUA_SIGNATURE */
	D.dumpByte(dmpVersion)            /* LUAC_VERSION */
	D.dumpByte(dmpFormat)             /* LUAC_FORMAT */
	D.dumpBlock([]byte(dmpData))      /* LUAC_DATA */
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
// Non-L-closure → 1. No api_checknelems / locks.
func luaDump(L *State, w dumpWriter, strip bool) int {
	o := index2value(L, -1)
	if !ttisLclosure(o) {
		return 1
	}
	return luaU_dump(L, clLvalue(o).p, w, strip)
}
