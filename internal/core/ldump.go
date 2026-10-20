// Translation of ldump.c (PUC-Lua 5.4.9): save precompiled chunks.
//
// The bytes produced here are a standard Lua 5.4 binary chunk: the
// header (signature, version, LUAC_DATA, the sizes of Instruction /
// lua_Integer / lua_Number, LUAC_INT and LUAC_NUM), then the main
// prototype. Multi-byte values are written in little-endian order,
// which is what 'luac' produces on x86/ARM hosts.
//
// Differences from C:
//
//  1. No GC: the luaC_objbarrier calls are gone.
//
//  2. The writer is a Go func instead of a lua_Writer plus a void*;
//     memory allocation errors cannot happen, so 'status' is only
//     non-zero if the writer itself reports an error.
//
//  3. Proto.lineinfo holds absolute lines, while the format wants
//     signed byte deltas plus an 'abslineinfo' array: dumpLineInfo
//     converts them back, mirroring lcode.c savelineinfo.
package core

import (
	"encoding/binary"
	"math"
)

// dumpWriter is the Go counterpart of lua_Writer: it returns 0 on
// success and anything else to stop the dump.
type dumpWriter func(L *State, b []byte) int

// DumpState is the Go counterpart of the C struct of the same name.
type DumpState struct {
	L      *State
	writer dumpWriter
	strip  bool
	status int
}

// dumpBlock writes raw bytes (dumpBlock).
func (D *DumpState) dumpBlock(b []byte) {
	if D.status == 0 && len(b) > 0 {
		D.status = D.writer(D.L, b)
	}
}

// dumpByte writes a single byte (dumpByte).
func (D *DumpState) dumpByte(y int) { D.dumpBlock([]byte{byte(y)}) }

// dmpDibs is DIBS: the number of bytes a 64-bit size_t needs in
// dumpSize ((sizeof(size_t) * CHAR_BIT + 6) / 7).
const dmpDibs = 10

// dumpSize writes a size_t in 7-bits-per-byte form (dumpSize).
func (D *DumpState) dumpSize(x uint64) {
	var buff [dmpDibs]byte
	n := 0
	for {
		buff[dmpDibs-1-n] = byte(x & 0x7f) /* fill buffer in reverse order */
		n++
		x >>= 7
		if x == 0 {
			break
		}
	}
	buff[dmpDibs-1] |= 0x80 /* mark last byte */
	D.dumpBlock(buff[dmpDibs-n:])
}

// dumpInt writes an int (dumpInt).
func (D *DumpState) dumpInt(x int) { D.dumpSize(uint64(x)) }

// dumpInteger writes a lua_Integer (8 bytes, host byte order).
func (D *DumpState) dumpInteger(x int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(x))
	D.dumpBlock(b[:])
}

// dumpNumber writes a lua_Number (8 bytes, host byte order).
func (D *DumpState) dumpNumber(x float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
	D.dumpBlock(b[:])
}

// dumpString writes a nullable string: a size of 0 means NULL
// (dumpString).
func (D *DumpState) dumpString(s *TString) {
	if s == nil {
		D.dumpSize(0)
	} else {
		D.dumpSize(uint64(len(s.val)) + 1)
		D.dumpBlock([]byte(s.val))
	}
}

// dumpCode writes the bytecode of a function (dumpCode).
func (D *DumpState) dumpCode(f *Proto) {
	n := len(f.code)
	D.dumpInt(n)
	b := make([]byte, 4*n)
	for i, ins := range f.code {
		binary.LittleEndian.PutUint32(b[4*i:], packInstruction(ins))
	}
	D.dumpBlock(b)
}

// dumpConstants writes the constant table (dumpConstants).
func (D *DumpState) dumpConstants(f *Proto) {
	n := len(f.k)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		o := &f.k[i]
		tt := ttypetag(o)
		D.dumpByte(tt)
		switch tt {
		case LUA_VNUMFLT:
			D.dumpNumber(fltvalue(o))
		case LUA_VNUMINT:
			D.dumpInteger(o.i)
		case LUA_VSHRSTR, LUA_VLNGSTR:
			D.dumpString(tsvalue(o))
		}
	}
}

// dumpProtos writes the nested prototypes (dumpProtos).
func (D *DumpState) dumpProtos(f *Proto) {
	n := len(f.p)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpFunction(f.p[i], f.source)
	}
}

// dumpUpvalues writes the upvalue descriptions (dumpUpvalues).
func (D *DumpState) dumpUpvalues(f *Proto) {
	n := len(f.upvalues)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpByte(int(f.upvalues[i].instack))
		D.dumpByte(int(f.upvalues[i].idx))
		D.dumpByte(int(f.upvalues[i].kind))
	}
}

// dumpLineInfo converts the absolute lines of 'f.lineinfo' into the
// on-disk form (signed byte deltas plus 'abslineinfo' entries), the way
// lcode.c savelineinfo builds it.
func (D *DumpState) dumpLineInfo(f *Proto) {
	if D.strip {
		D.dumpInt(0)
		D.dumpInt(0)
		return
	}
	deltas := make([]byte, len(f.lineinfo))
	var abs []AbsLineInfo
	previousline := f.linedefined
	iwthabs := 0
	for pc, line := range f.lineinfo {
		linedif := int(line) - previousline
		if linedif >= limLineDiff || linedif <= -limLineDiff ||
			iwthabs >= maxIwthAbs {
			abs = append(abs, AbsLineInfo{pc: pc, line: int(line)})
			linedif = absLineInfo /* signal that there is absolute info */
			iwthabs = 1           /* restart counter */
		} else {
			iwthabs++
		}
		deltas[pc] = byte(int8(linedif))
		previousline = int(line)
	}
	D.dumpInt(len(deltas))
	D.dumpBlock(deltas)
	D.dumpInt(len(abs))
	for _, a := range abs {
		D.dumpInt(a.pc)
		D.dumpInt(a.line)
	}
}

// dumpDebug writes the debug information (dumpDebug).
func (D *DumpState) dumpDebug(f *Proto) {
	D.dumpLineInfo(f)
	n := 0
	if !D.strip {
		n = len(f.locvars)
	}
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpString(f.locvars[i].varname)
		D.dumpInt(f.locvars[i].startpc)
		D.dumpInt(f.locvars[i].endpc)
	}
	n = 0
	if !D.strip {
		n = len(f.upvalues)
	}
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpString(f.upvalues[i].name)
	}
}

// dumpFunction writes one prototype; 'psource' is the source of the
// parent, so that a child with the same source does not repeat it
// (dumpFunction).
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

// dumpHeader writes the chunk header (dumpHeader).
func (D *DumpState) dumpHeader() {
	D.dumpBlock([]byte(dmpSignature))
	D.dumpByte(dmpVersion)
	D.dumpByte(dmpFormat)
	D.dumpBlock([]byte(dmpData))
	D.dumpByte(4) /* sizeof(Instruction) */
	D.dumpByte(8) /* sizeof(lua_Integer) */
	D.dumpByte(8) /* sizeof(lua_Number) */
	D.dumpInteger(dmpInt)
	D.dumpNumber(dmpNum)
}

// luaU_dump dumps a prototype as a precompiled chunk (ldump.c
// luaU_dump).
func luaU_dump(L *State, f *Proto, w dumpWriter, strip bool) int {
	var D DumpState
	D.L = L
	D.writer = w
	D.strip = strip
	D.status = 0
	D.dumpHeader()
	D.dumpByte(len(f.upvalues))
	D.dumpFunction(f, nil)
	return D.status
}

// luaDump is lua_dump (lapi.c): it dumps the Lua function on the top of
// the stack, returning non-zero if it is not a Lua function.
func luaDump(L *State, w dumpWriter, strip bool) int {
	o := index2value(L, -1)
	if !ttisLclosure(o) {
		return 1
	}
	return luaU_dump(L, clLvalue(o).p, w, strip)
}
