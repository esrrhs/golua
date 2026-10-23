// Translation of lundump.c / lundump.h (PUC-Lua 5.3.4): load precompiled
// chunks. Format matches PUC-Lua 5.3.4 (fixed ints, 5.3 strings/tags).
package core

import (
	"encoding/binary"
	"math"
	"strings"
)

const (
	dmpSignature = "\x1bLua"
	dmpData      = "\x19\x93\r\n\x1a\n"
	dmpInt       = 0x5678
	dmpNum       = 370.5
	dmpVersion   = 0x53
	dmpFormat    = 0
)

type LoadState struct {
	L    *State
	data []byte
	pos  int
	name string
}

func (S *LoadState) loadError(why string) {
	ts := pushfString(S.L, "%s: %s precompiled chunk", S.name, why)
	S.L.checkStack(1)
	setsvalue(&S.L.stack[S.L.top], ts)
	S.L.top++
	luaD_throw(S.L, LUA_ERRSYNTAX)
}

func (S *LoadState) loadBlock(n int) []byte {
	if n < 0 || S.pos+n > len(S.data) {
		S.loadError("truncated")
	}
	b := S.data[S.pos : S.pos+n]
	S.pos += n
	return b
}

func (S *LoadState) loadByte() byte { return S.loadBlock(1)[0] }

// loadInt reads a fixed 4-byte little-endian int (5.3 LoadInt).
func (S *LoadState) loadInt() int {
	return int(int32(binary.LittleEndian.Uint32(S.loadBlock(4))))
}

func (S *LoadState) loadSizeT() uint64 {
	return binary.LittleEndian.Uint64(S.loadBlock(8))
}

func (S *LoadState) loadInteger() int64 {
	return int64(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

func (S *LoadState) loadNumber() float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

// loadString is 5.3 LoadString.
func (S *LoadState) loadString() *TString {
	size := uint64(S.loadByte())
	if size == 0xFF {
		size = S.loadSizeT()
	}
	if size == 0 {
		return nil
	}
	size--
	if size > math.MaxInt32 {
		S.loadError("truncated")
	}
	b := S.loadBlock(int(size))
	if int(size) <= LUAI_MAXSHORTLEN {
		return luaS_newlstr(S.L, string(b))
	}
	return luaS_createlngstrobj(S.L, string(b))
}

func (S *LoadState) loadCode(f *Proto) {
	n := S.loadInt()
	if n < 0 || n > (len(S.data)-S.pos)/4 {
		S.loadError("truncated")
	}
	f.code = make([]Instruction, n)
	f.sizecode = n
	for i := 0; i < n; i++ {
		f.code[i] = unpackInstruction(binary.LittleEndian.Uint32(S.loadBlock(4)))
	}
}

func (S *LoadState) loadConstants(f *Proto) {
	n := S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	f.k = make([]Value, n)
	f.sizek = n
	for i := 0; i < n; i++ {
		o := &f.k[i]
		switch S.loadByte() {
		case dumpTagNil:
			setnilvalue(o)
		case dumpTagBoolean:
			if S.loadByte() != 0 {
				setbtvalue(o)
			} else {
				setbfvalue(o)
			}
		case dumpTagNumFlt:
			setfltvalue(o, S.loadNumber())
		case dumpTagNumInt:
			setivalue(o, S.loadInteger())
		case dumpTagShrStr, dumpTagLngStr:
			setsvalue(o, S.loadString())
		default:
			S.loadError("corrupted")
		}
	}
}

func (S *LoadState) loadProtos(f *Proto) {
	n := S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	f.p = make([]*Proto, n)
	f.sizep = n
	for i := 0; i < n; i++ {
		f.p[i] = luaF_newproto(S.L)
		S.loadFunction(f.p[i], f.source)
	}
}

func (S *LoadState) loadUpvalues(f *Proto) {
	n := S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	f.upvalues = make([]Upvaldesc, n)
	f.sizeupvalues = n
	for i := 0; i < n; i++ {
		f.upvalues[i].instack = S.loadByte()
		f.upvalues[i].idx = S.loadByte()
	}
}

func (S *LoadState) loadDebug(f *Proto) {
	n := S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	f.lineinfo = make([]int32, n)
	f.sizelineinfo = n
	for i := 0; i < n; i++ {
		f.lineinfo[i] = int32(S.loadInt())
	}
	n = S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	f.locvars = make([]LocVar, n)
	f.sizelocvars = n
	for i := 0; i < n; i++ {
		f.locvars[i].varname = S.loadString()
		f.locvars[i].startpc = S.loadInt()
		f.locvars[i].endpc = S.loadInt()
	}
	n = S.loadInt()
	if n < 0 {
		S.loadError("truncated")
	}
	if n != 0 && n != len(f.upvalues) {
		/* debug info present: names for each upvalue */
		if n != len(f.upvalues) {
			S.loadError("corrupted")
		}
	}
	for i := 0; i < n; i++ {
		f.upvalues[i].name = S.loadString()
	}
}

func (S *LoadState) loadFunction(f *Proto, psource *TString) {
	f.source = S.loadString()
	if f.source == nil {
		f.source = psource
	}
	f.linedefined = S.loadInt()
	f.lastlinedefined = S.loadInt()
	f.numparams = S.loadByte()
	f.isVararg = S.loadByte()
	f.maxstacksize = S.loadByte()
	S.loadCode(f)
	S.loadConstants(f)
	S.loadUpvalues(f)
	S.loadProtos(f)
	S.loadDebug(f)
}

func (S *LoadState) checkLiteral(s, msg string) {
	if string(S.loadBlock(len(s))) != s {
		S.loadError(msg)
	}
}

func (S *LoadState) checkSize(size int, tname string) {
	if int(S.loadByte()) != size {
		S.loadError(pushfString(S.L, "%s size mismatch in", tname).val)
	}
}

func (S *LoadState) checkHeader() {
	S.checkLiteral(dmpSignature[1:], "not a")
	if S.loadByte() != dmpVersion {
		S.loadError("version mismatch in")
	}
	if S.loadByte() != dmpFormat {
		S.loadError("format mismatch in")
	}
	S.checkLiteral(dmpData, "corrupted")
	S.checkSize(4, "int")
	S.checkSize(8, "size_t")
	S.checkSize(4, "Instruction")
	S.checkSize(8, "lua_Integer")
	S.checkSize(8, "lua_Number")
	if S.loadInteger() != dmpInt {
		S.loadError("endianness mismatch in")
	}
	if S.loadNumber() != dmpNum {
		S.loadError("float format mismatch in")
	}
}

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
	if len(data) == 0 || data[0] != dmpSignature[0] {
		S.loadError("not a")
	}
	S.pos = 1
	S.checkHeader()
	cl := luaF_newLclosure(L, int(S.loadByte()))
	cl.p = luaF_newproto(L)
	S.loadFunction(cl.p, nil)
	return cl
}

func checkLoadMode(L *State, mode, x string) {
	if !strings.ContainsRune(mode, rune(x[0])) {
		ts := pushfString(L, "attempt to load a %s chunk (mode is '%s')", x, mode)
		L.checkStack(1)
		setsvalue(&L.stack[L.top], ts)
		L.top++
		luaD_throw(L, LUA_ERRSYNTAX)
	}
}

func luaU_loadBinary(L *State, data []byte, chunkname, mode string) error {
	oldtop := L.top
	status := L.rawRunProtected(func() {
		checkLoadMode(L, mode, "binary")
		cl := luaU_undump(L, data, chunkname)
		luaF_initupvals(L, cl)
		if cl.nupvalues >= 1 {
			var gt Value
			sethvalue(&gt, L.rawGetRegistryGlobals())
			setobj(&cl.upvals[0].v, &gt)
		}
		L.checkStack(1)
		setclLvalue(&L.stack[L.top], cl)
		L.top++
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

// Instruction formats used by pack/unpack (5.3: dmpIABC, dmpIABx, dmpIAsBx, dmpIAx — no isJ).
const (
	dmpIABC = iota
	dmpIABx
	dmpIAsBx
	dmpIAx
)

// dmpOpModes is getOpMode for each opcode (5.3.4 order).
var dmpOpModes = [NUM_OPCODES]int{
	dmpIABC,  /* MOVE */
	dmpIABx,  /* LOADK */
	dmpIABx,  /* LOADKX */
	dmpIABC,  /* LOADBOOL */
	dmpIABC,  /* LOADNIL */
	dmpIABC,  /* GETUPVAL */
	dmpIABC,  /* GETTABUP */
	dmpIABC,  /* GETTABLE */
	dmpIABC,  /* SETTABUP */
	dmpIABC,  /* SETUPVAL */
	dmpIABC,  /* SETTABLE */
	dmpIABC,  /* NEWTABLE */
	dmpIABC,  /* SELF */
	dmpIABC,  /* ADD */
	dmpIABC,  /* SUB */
	dmpIABC,  /* MUL */
	dmpIABC,  /* MOD */
	dmpIABC,  /* POW */
	dmpIABC,  /* DIV */
	dmpIABC,  /* IDIV */
	dmpIABC,  /* BAND */
	dmpIABC,  /* BOR */
	dmpIABC,  /* BXOR */
	dmpIABC,  /* SHL */
	dmpIABC,  /* SHR */
	dmpIABC,  /* UNM */
	dmpIABC,  /* BNOT */
	dmpIABC,  /* NOT */
	dmpIABC,  /* LEN */
	dmpIABC,  /* CONCAT */
	dmpIAsBx, /* JMP */
	dmpIABC,  /* EQ */
	dmpIABC,  /* LT */
	dmpIABC,  /* LE */
	dmpIABC,  /* TEST */
	dmpIABC,  /* TESTSET */
	dmpIABC,  /* CALL */
	dmpIABC,  /* TAILCALL */
	dmpIABC,  /* RETURN */
	dmpIAsBx, /* FORLOOP */
	dmpIAsBx, /* FORPREP */
	dmpIABC,  /* TFORCALL */
	dmpIAsBx, /* TFORLOOP */
	dmpIABC,  /* SETLIST */
	dmpIABx,  /* CLOSURE */
	dmpIABC,  /* VARARG */
	dmpIAx,   /* EXTRAARG */
}

// 5.3 bit layout (lopcodes.h): OP:6 A:8 C:9 B:9
const (
	instrPosOp  = 0
	instrPosA   = 6
	instrPosC   = 14
	instrPosB   = 23
	instrPosBx  = 14
	instrPosAx  = 6
	instrSizeOp = 6
	instrSizeA  = 8
	instrSizeB  = 9
	instrSizeC  = 9
	instrSizeBx = 18
	instrSizeAx = 26
)

func instrMask(size int) uint32 { return uint32(1)<<uint(size) - 1 }

// packInstruction encodes an instruction as a 32-bit word (5.3 CREATE_*).
// For dmpIAsBx, B already holds the biased Bx (sbx + OFFSET_sBx).
func packInstruction(i Instruction) uint32 {
	if i.Op < 0 || i.Op >= NUM_OPCODES {
		return uint32(i.Op) << instrPosOp
	}
	var w uint32
	switch dmpOpModes[i.Op] {
	case dmpIABC:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.A)<<instrPosA |
			uint32(i.B)<<instrPosB |
			uint32(i.C)<<instrPosC
	case dmpIABx, dmpIAsBx:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.A)<<instrPosA |
			uint32(i.B)<<instrPosBx
	case dmpIAx:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.A)<<instrPosAx
	}
	return w
}

// unpackInstruction decodes a 32-bit word (5.3 GET_OPCODE / GETARG_*).
func unpackInstruction(w uint32) Instruction {
	op := int(w>>instrPosOp) & int(instrMask(instrSizeOp))
	if op < 0 || op >= NUM_OPCODES {
		return Instruction{Op: op}
	}
	switch dmpOpModes[op] {
	case dmpIABC:
		return Instruction{
			Op: op,
			A:  int(w>>instrPosA) & int(instrMask(instrSizeA)),
			B:  int(w>>instrPosB) & int(instrMask(instrSizeB)),
			C:  int(w>>instrPosC) & int(instrMask(instrSizeC)),
		}
	case dmpIABx, dmpIAsBx:
		return Instruction{
			Op: op,
			A:  int(w>>instrPosA) & int(instrMask(instrSizeA)),
			B:  int(w>>instrPosBx) & int(instrMask(instrSizeBx)),
		}
	default: /* dmpIAx */
		return Instruction{
			Op: op,
			A:  int(w>>instrPosAx) & int(instrMask(instrSizeAx)),
		}
	}
}
