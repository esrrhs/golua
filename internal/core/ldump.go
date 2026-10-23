// Translation of ldump.c (PUC-Lua 5.3.4): save precompiled chunks.
//
// Format matches PUC-Lua 5.3.4 byte-for-byte on little-endian hosts:
// fixed-size ints/size_t, 5.3 string encoding, 5.3 constant tags
// (float=3 / int=19), upvalues without 'kind', absolute int lineinfo.
package core

import (
	"encoding/binary"
	"math"
)

type dumpWriter func(L *State, b []byte) int

type DumpState struct {
	L      *State
	writer dumpWriter
	strip  bool
	status int
}

func (D *DumpState) dumpBlock(b []byte) {
	if D.status == 0 && len(b) > 0 {
		D.status = D.writer(D.L, b)
	}
}

func (D *DumpState) dumpByte(y int) { D.dumpBlock([]byte{byte(y)}) }

// dumpInt writes a fixed-size int (4 bytes LE) — 5.3 DumpInt.
func (D *DumpState) dumpInt(x int) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(int32(x)))
	D.dumpBlock(b[:])
}

// dumpSizeT writes a size_t (8 bytes LE).
func (D *DumpState) dumpSizeT(x uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], x)
	D.dumpBlock(b[:])
}

func (D *DumpState) dumpInteger(x int64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(x))
	D.dumpBlock(b[:])
}

func (D *DumpState) dumpNumber(x float64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], math.Float64bits(x))
	D.dumpBlock(b[:])
}

// dumpString writes a 5.3 string: size byte, optional size_t if 0xFF,
// size includes the trailing NUL that is not written.
func (D *DumpState) dumpString(s *TString) {
	if s == nil {
		D.dumpByte(0)
		return
	}
	size := uint64(len(s.val)) + 1
	if size < 0xFF {
		D.dumpByte(int(size))
	} else {
		D.dumpByte(0xFF)
		D.dumpSizeT(size)
	}
	D.dumpBlock([]byte(s.val))
}

func (D *DumpState) dumpCode(f *Proto) {
	n := len(f.code)
	D.dumpInt(n)
	b := make([]byte, 4*n)
	for i, ins := range f.code {
		binary.LittleEndian.PutUint32(b[4*i:], packInstruction(ins))
	}
	D.dumpBlock(b)
}

// 5.3 on-disk constant tags (lobject.h) — float/int variants are
// swapped relative to our internal 5.4-style LUA_VNUM* tags.
const (
	dumpTagNil     = LUA_TNIL               // 0
	dumpTagBoolean = LUA_TBOOLEAN           // 1
	dumpTagNumFlt  = LUA_TNUMBER | (0 << 4) // 3  LUA_TNUMFLT
	dumpTagNumInt  = LUA_TNUMBER | (1 << 4) // 19 LUA_TNUMINT
	dumpTagShrStr  = LUA_TSTRING | (0 << 4) // 4  LUA_TSHRSTR
	dumpTagLngStr  = LUA_TSTRING | (1 << 4) // 20 LUA_TLNGSTR
)

func (D *DumpState) dumpConstants(f *Proto) {
	n := len(f.k)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		o := &f.k[i]
		switch {
		case ttisnil(o):
			D.dumpByte(dumpTagNil)
		case ttisboolean(o):
			D.dumpByte(dumpTagBoolean)
			if ttistrue(o) {
				D.dumpByte(1)
			} else {
				D.dumpByte(0)
			}
		case ttisfloat(o):
			D.dumpByte(dumpTagNumFlt)
			D.dumpNumber(fltvalue(o))
		case ttisinteger(o):
			D.dumpByte(dumpTagNumInt)
			D.dumpInteger(o.i)
		case ttisshrstring(o):
			D.dumpByte(dumpTagShrStr)
			D.dumpString(tsvalue(o))
		case ttisstring(o):
			D.dumpByte(dumpTagLngStr)
			D.dumpString(tsvalue(o))
		default:
			D.dumpByte(dumpTagNil)
		}
	}
}

func (D *DumpState) dumpProtos(f *Proto) {
	n := len(f.p)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpFunction(f.p[i], f.source)
	}
}

func (D *DumpState) dumpUpvalues(f *Proto) {
	n := len(f.upvalues)
	D.dumpInt(n)
	for i := 0; i < n; i++ {
		D.dumpByte(int(f.upvalues[i].instack))
		D.dumpByte(int(f.upvalues[i].idx))
		/* 5.3 has no 'kind' byte */
	}
}

func (D *DumpState) dumpDebug(f *Proto) {
	n := 0
	if !D.strip {
		n = f.sizelineinfo /* C: f->sizelineinfo (combine may zero this) */
	}
	D.dumpInt(n)
	if n > 0 {
		b := make([]byte, 4*n)
		for i := 0; i < n; i++ {
			binary.LittleEndian.PutUint32(b[4*i:], uint32(f.lineinfo[i]))
		}
		D.dumpBlock(b)
	}
	n = 0
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

func (D *DumpState) dumpFunction(f *Proto, psource *TString) {
	if D.strip || f.source == psource {
		D.dumpString(nil)
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

func (D *DumpState) dumpHeader() {
	D.dumpBlock([]byte(dmpSignature))
	D.dumpByte(dmpVersion)
	D.dumpByte(dmpFormat)
	D.dumpBlock([]byte(dmpData))
	D.dumpByte(4) /* sizeof(int) */
	D.dumpByte(8) /* sizeof(size_t) */
	D.dumpByte(4) /* sizeof(Instruction) */
	D.dumpByte(8) /* sizeof(lua_Integer) */
	D.dumpByte(8) /* sizeof(lua_Number) */
	D.dumpInteger(dmpInt)
	D.dumpNumber(dmpNum)
}

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

func luaDump(L *State, w dumpWriter, strip bool) int {
	o := index2value(L, -1)
	if !ttisLclosure(o) {
		return 1
	}
	return luaU_dump(L, clLvalue(o).p, w, strip)
}
