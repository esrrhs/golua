// Translation of lundump.c / lundump.h (PUC-Lua 5.4.9): load precompiled
// chunks.
//
// The file format is *the official one*, byte for byte, so chunks dumped
// by this package can be loaded by PUC-Lua (and vice versa, as long as
// the host is little-endian, which is what 'luac' produces on x86/ARM).
// Three things are necessarily different from C:
//
//  1. There is no GC, so the luaC_objbarrier calls and the "anchor the
//     string on the stack while reading" dance are gone (Go's GC sees
//     the TString through the slice).
//
//  2. C raises read errors with luaD_throw (setjmp); here the panic
//     unwinds the Go stack and is caught by rawRunProtected in
//     luaU_loadBinary, which turns it into 'nil, message' exactly like
//     'lua_load' does.
//
//  3. An Instruction here is a struct, not a packed 32-bit word, so
//     every instruction is unpacked with 'unpackInstruction' (see the
//     bit layout from lopcodes.h at the bottom of this file).
//
// The fourth difference is not in this file but matters here: Proto.lineinfo
// holds *absolute* lines (see lobject.go), while the file format stores
// signed byte deltas plus an 'abslineinfo' array. loadLineInfo converts
// the on-disk form back into absolute lines, mirroring luaG_getfuncline.
package lua

import (
	"encoding/binary"
	"math"
	"strings"
)

// Constants from lua.h / lundump.h.
const (
	dmpSignature = "\x1bLua"            // LUA_SIGNATURE
	dmpData      = "\x19\x93\r\n\x1a\n" // LUAC_DATA
	dmpInt       = 0x5678               // LUAC_INT
	dmpNum       = 370.5                // LUAC_NUM
	// LUAC_VERSION for Lua 5.4: ((504 / 100) * 16) + 504 % 100
	dmpVersion = 0x54
	dmpFormat  = 0 // LUAC_FORMAT: the official format
)

// Line-info constants (ldebug.h / lcode.c).
const (
	absLineInfo = -0x80 // ABSLINEINFO
	limLineDiff = 0x80  // LIMLINEDIFF
	maxIwthAbs  = 128   // MAXIWTHABS
)

// LoadState is the Go counterpart of the C struct of the same name.
type LoadState struct {
	L    *State
	data []byte
	pos  int
	name string
}

// loadError raises "name: bad binary format (why)" (lundump.c error).
func (S *LoadState) loadError(why string) {
	ts := pushfString(S.L, "%s: bad binary format (%s)", S.name, why)
	S.L.checkStack(1)
	setsvalue(&S.L.stack[S.L.top], ts)
	S.L.top++
	luaD_throw(S.L, LUA_ERRSYNTAX)
}

// loadBlock reads 'n' bytes (loadBlock in lundump.c).
func (S *LoadState) loadBlock(n int) []byte {
	if n < 0 || S.pos+n > len(S.data) {
		S.loadError("truncated chunk")
	}
	b := S.data[S.pos : S.pos+n]
	S.pos += n
	return b
}

// loadByte reads a single byte (loadByte).
func (S *LoadState) loadByte() byte {
	return S.loadBlock(1)[0]
}

// loadUnsigned reads a 7-bits-per-byte integer (loadUnsigned).
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

// loadSize reads a size_t (loadSize).
func (S *LoadState) loadSize() int64 { return S.loadUnsigned(math.MaxInt64) }

// loadInt reads an int (loadInt).
func (S *LoadState) loadInt() int { return int(S.loadUnsigned(math.MaxInt32)) }

// loadCount reads a count and checks that 'unit' bytes per element are
// still available, so that a corrupted size cannot make us allocate an
// absurd slice (C would fail inside luaM_newvectorchecked instead).
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

// loadInteger reads a lua_Integer (8 bytes, host byte order).
func (S *LoadState) loadInteger() int64 {
	return int64(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

// loadNumber reads a lua_Number (8 bytes, host byte order).
func (S *LoadState) loadNumber() float64 {
	return math.Float64frombits(binary.LittleEndian.Uint64(S.loadBlock(8)))
}

// loadStringN loads a nullable string (loadStringN).
func (S *LoadState) loadStringN(p *Proto) *TString {
	size := S.loadSize()
	if size == 0 { /* no string? */
		return nil
	}
	size--
	if size < 0 || size > math.MaxInt32 {
		S.loadError("integer overflow")
	}
	b := S.loadBlock(int(size))
	if size <= LUAI_MAXSHORTLEN { /* short string? */
		return luaS_newlstr(S.L, string(b))
	}
	return luaS_createlngstrobj(S.L, string(b)) /* long string */
}

// loadString loads a non-nullable string (loadString).
func (S *LoadState) loadString(p *Proto) *TString {
	ts := S.loadStringN(p)
	if ts == nil {
		S.loadError("bad format for constant string")
	}
	return ts
}

// loadCode loads the bytecode of a function (loadCode).
func (S *LoadState) loadCode(f *Proto) {
	n := S.loadCount(4)
	f.code = make([]Instruction, n)
	f.sizecode = n
	for i := 0; i < n; i++ {
		f.code[i] = unpackInstruction(binary.LittleEndian.Uint32(S.loadBlock(4)))
	}
}

// loadConstants loads the constant table (loadConstants).
func (S *LoadState) loadConstants(f *Proto) {
	n := S.loadCount(1)
	f.k = make([]Value, n)
	f.sizek = n
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
			setsvalue(o, S.loadString(f))
		default:
			S.loadError("bad constant type")
		}
	}
}

// loadProtos loads the nested prototypes (loadProtos).
func (S *LoadState) loadProtos(f *Proto) {
	n := S.loadCount(1)
	f.p = make([]*Proto, n)
	f.sizep = n
	for i := 0; i < n; i++ {
		f.p[i] = luaF_newproto(S.L)
		S.loadFunction(f.p[i], f.source)
	}
}

// loadUpvalues loads the upvalue descriptions (loadUpvalues).
func (S *LoadState) loadUpvalues(f *Proto) {
	n := S.loadCount(3)
	f.upvalues = make([]Upvaldesc, n)
	f.sizeupvalues = n
	for i := 0; i < n; i++ {
		f.upvalues[i].instack = S.loadByte()
		f.upvalues[i].idx = S.loadByte()
		f.upvalues[i].kind = S.loadByte()
	}
}

// loadLineInfo reads the line information and converts the on-disk
// (delta + abslineinfo) form into the absolute lines used by Proto.
func (S *LoadState) loadLineInfo(f *Proto) {
	n := S.loadCount(1)
	raw := S.loadBlock(n)
	nabs := S.loadCount(2)
	abs := make([]AbsLineInfo, nabs)
	for i := 0; i < nabs; i++ {
		abs[i].pc = S.loadInt()
		abs[i].line = S.loadInt()
	}
	/* convert to absolute lines, the way luaG_getfuncline does it */
	lines := make([]int32, n)
	baseline := f.linedefined
	ai := 0
	for pc := 0; pc < n; pc++ {
		if ai < nabs && abs[ai].pc <= pc {
			/* the delta here is ABSLINEINFO; the line is absolute */
			baseline = abs[ai].line
			ai++
			lines[pc] = int32(baseline)
			continue
		}
		baseline += int(int8(raw[pc]))
		lines[pc] = int32(baseline)
	}
	f.lineinfo = lines
	f.sizelineinfo = n
	f.abslineinfo = abs
}

// loadDebug loads the debug information (loadDebug).
func (S *LoadState) loadDebug(f *Proto) {
	S.loadLineInfo(f)
	n := S.loadCount(1)
	f.locvars = make([]LocVar, n)
	f.sizelocvars = n
	for i := 0; i < n; i++ {
		f.locvars[i].varname = S.loadStringN(f)
		f.locvars[i].startpc = S.loadInt()
		f.locvars[i].endpc = S.loadInt()
	}
	n = S.loadInt()
	if n != 0 { /* does it have debug information? */
		n = len(f.upvalues) /* must be this many */
	}
	for i := 0; i < n; i++ {
		f.upvalues[i].name = S.loadStringN(f)
	}
}

// loadFunction loads one function prototype (loadFunction).
func (S *LoadState) loadFunction(f *Proto, psource *TString) {
	f.source = S.loadStringN(f)
	if f.source == nil { /* no source in dump? */
		f.source = psource /* reuse parent's source */
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

// checkLiteral compares the next bytes with a literal (checkliteral).
func (S *LoadState) checkLiteral(s, msg string) {
	if string(S.loadBlock(len(s))) != s {
		S.loadError(msg)
	}
}

// checkSize checks the size of a C type recorded in the header
// (fchecksize).
func (S *LoadState) checkSize(size int, tname string) {
	if int(S.loadByte()) != size {
		S.loadError(pushfString(S.L, "%s size mismatch", tname).val)
	}
}

// checkHeader validates the chunk header (checkHeader).
func (S *LoadState) checkHeader() {
	/* skip 1st char (already checked by the caller) */
	S.checkLiteral(dmpSignature[1:], "not a binary chunk")
	if S.loadByte() != dmpVersion {
		S.loadError("version mismatch")
	}
	if S.loadByte() != dmpFormat {
		S.loadError("format mismatch")
	}
	S.checkLiteral(dmpData, "corrupted chunk")
	S.checkSize(4, "Instruction")
	S.checkSize(8, "lua_Integer")
	S.checkSize(8, "lua_Number")
	if S.loadInteger() != dmpInt {
		S.loadError("integer format mismatch")
	}
	if S.loadNumber() != dmpNum {
		S.loadError("float format mismatch")
	}
}

// luaU_undump loads a precompiled chunk (lundump.c luaU_undump).
// The returned closure has no upvalues yet: the caller must call
// luaF_initupvals, as ldo.c f_parser does.
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
	/* the first byte is the one 'lua_load' looked at to recognize a
	   binary chunk (ldo.c f_parser); checkHeader skips it */
	if len(data) == 0 || data[0] != dmpSignature[0] {
		S.loadError("not a binary chunk")
	}
	S.pos = 1
	S.checkHeader()
	cl := luaF_newLclosure(L, int(S.loadByte()))
	cl.p = luaF_newproto(L)
	S.loadFunction(cl.p, nil)
	return cl
}

// checkLoadMode is checkmode from ldo.c: the 'mode' argument of 'load'
// must allow the kind of chunk being loaded ("binary" needs 'b', "text"
// needs 't').
func checkLoadMode(L *State, mode, x string) {
	if !strings.ContainsRune(mode, rune(x[0])) {
		ts := pushfString(L, "attempt to load a %s chunk (mode is '%s')", x, mode)
		L.checkStack(1)
		setsvalue(&L.stack[L.top], ts)
		L.top++
		luaD_throw(L, LUA_ERRSYNTAX)
	}
}

// luaU_loadBinary is what C does for a precompiled chunk inside
// lua_load: undump it, initialise its upvalues, set the globals table
// as its first upvalue (the _ENV) and push it on the stack. Errors are
// turned into a Go error, so that 'load' returns nil plus a message.
func luaU_loadBinary(L *State, data []byte, chunkname, mode string) error {
	oldtop := L.top
	status := L.rawRunProtected(func() {
		checkLoadMode(L, mode, "binary")
		cl := luaU_undump(L, data, chunkname)
		luaF_initupvals(L, cl)
		if cl.nupvalues >= 1 {
			/* set the globals table as the first upvalue (the _ENV) */
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

// ---------------------------------------------------------------------
// instruction (un)packing (lopcodes.h)
// ---------------------------------------------------------------------

// Instruction modes (lopcodes.c luaP_opmodes, only the mode field).
const (
	dmpIABC = iota
	dmpIABx
	dmpIAsBx
	dmpIAx
	dmpIsJ
)

// dmpOpModes is the 'mode' column of luaP_opmodes, in opcode order.
// (lopcodes.go does not export it, so it is repeated here.)
var dmpOpModes = [NUM_OPCODES]int{
	dmpIABC,  /* OP_MOVE */
	dmpIAsBx, /* OP_LOADI */
	dmpIAsBx, /* OP_LOADF */
	dmpIABx,  /* OP_LOADK */
	dmpIABx,  /* OP_LOADKX */
	dmpIABC,  /* OP_LOADFALSE */
	dmpIABC,  /* OP_LFALSESKIP */
	dmpIABC,  /* OP_LOADTRUE */
	dmpIABC,  /* OP_LOADNIL */
	dmpIABC,  /* OP_GETUPVAL */
	dmpIABC,  /* OP_SETUPVAL */
	dmpIABC,  /* OP_GETTABUP */
	dmpIABC,  /* OP_GETTABLE */
	dmpIABC,  /* OP_GETI */
	dmpIABC,  /* OP_GETFIELD */
	dmpIABC,  /* OP_SETTABUP */
	dmpIABC,  /* OP_SETTABLE */
	dmpIABC,  /* OP_SETI */
	dmpIABC,  /* OP_SETFIELD */
	dmpIABC,  /* OP_NEWTABLE */
	dmpIABC,  /* OP_SELF */
	dmpIABC,  /* OP_ADDI */
	dmpIABC,  /* OP_ADDK */
	dmpIABC,  /* OP_SUBK */
	dmpIABC,  /* OP_MULK */
	dmpIABC,  /* OP_MODK */
	dmpIABC,  /* OP_POWK */
	dmpIABC,  /* OP_DIVK */
	dmpIABC,  /* OP_IDIVK */
	dmpIABC,  /* OP_BANDK */
	dmpIABC,  /* OP_BORK */
	dmpIABC,  /* OP_BXORK */
	dmpIABC,  /* OP_SHRI */
	dmpIABC,  /* OP_SHLI */
	dmpIABC,  /* OP_ADD */
	dmpIABC,  /* OP_SUB */
	dmpIABC,  /* OP_MUL */
	dmpIABC,  /* OP_MOD */
	dmpIABC,  /* OP_POW */
	dmpIABC,  /* OP_DIV */
	dmpIABC,  /* OP_IDIV */
	dmpIABC,  /* OP_BAND */
	dmpIABC,  /* OP_BOR */
	dmpIABC,  /* OP_BXOR */
	dmpIABC,  /* OP_SHL */
	dmpIABC,  /* OP_SHR */
	dmpIABC,  /* OP_MMBIN */
	dmpIABC,  /* OP_MMBINI */
	dmpIABC,  /* OP_MMBINK */
	dmpIABC,  /* OP_UNM */
	dmpIABC,  /* OP_BNOT */
	dmpIABC,  /* OP_NOT */
	dmpIABC,  /* OP_LEN */
	dmpIABC,  /* OP_CONCAT */
	dmpIABC,  /* OP_CLOSE */
	dmpIABC,  /* OP_TBC */
	dmpIsJ,   /* OP_JMP */
	dmpIABC,  /* OP_EQ */
	dmpIABC,  /* OP_LT */
	dmpIABC,  /* OP_LE */
	dmpIABC,  /* OP_EQK */
	dmpIABC,  /* OP_EQI */
	dmpIABC,  /* OP_LTI */
	dmpIABC,  /* OP_LEI */
	dmpIABC,  /* OP_GTI */
	dmpIABC,  /* OP_GEI */
	dmpIABC,  /* OP_TEST */
	dmpIABC,  /* OP_TESTSET */
	dmpIABC,  /* OP_CALL */
	dmpIABC,  /* OP_TAILCALL */
	dmpIABC,  /* OP_RETURN */
	dmpIABC,  /* OP_RETURN0 */
	dmpIABC,  /* OP_RETURN1 */
	dmpIABx,  /* OP_FORLOOP */
	dmpIABx,  /* OP_FORPREP */
	dmpIABx,  /* OP_TFORPREP */
	dmpIABC,  /* OP_TFORCALL */
	dmpIABx,  /* OP_TFORLOOP */
	dmpIABC,  /* OP_SETLIST */
	dmpIABx,  /* OP_CLOSURE */
	dmpIABC,  /* OP_VARARG */
	dmpIABC,  /* OP_VARARGPREP */
	dmpIAx,   /* OP_EXTRAARG */
}

// Bit positions and sizes of the packed instruction (lopcodes.h):
//
//	Op: bits 0-6    A: bits 7-14    k: bit 15    B: bits 16-23    C: bits 24-31
//	Bx: bits 15-31 (17 bits)        Ax: bits 7-31 (25 bits)        sJ: bits 7-31
const (
	instrPosOp  = 0
	instrPosA   = 7
	instrPosK   = 15
	instrPosB   = 16
	instrPosC   = 24
	instrPosBx  = instrPosK // 15
	instrPosAx  = instrPosA // 7
	instrPosSJ  = instrPosA // 7
	instrSizeOp = 7
	instrSizeA  = 8
	instrSizeB  = 8
	instrSizeC  = 8
	instrSizeK  = 1
	instrSizeBx = 17
	instrSizeAx = 25
	instrSizeSJ = 25
)

// instrMask returns a mask with 'size' one bits (MASK1 in lopcodes.h).
func instrMask(size int) uint32 { return uint32(1)<<uint(size) - 1 }

// packInstruction encodes an instruction as a 32-bit word, the way
// PUC-Lua stores it (CREATE_ABCk / CREATE_ABx / CREATE_Ax / CREATE_sJ).
//
// The Go fields already hold the *encoded* operands: 'B' is the Bx
// value for iABx/iAsBx (already biased by OFFSET_sBx for sBx) and the
// raw sJ for isJ; 'A' is the Ax value for iAx.
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
			uint32(i.C)<<instrPosC |
			uint32(i.K)<<instrPosK
	case dmpIABx, dmpIAsBx:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.A)<<instrPosA |
			uint32(i.B)<<instrPosBx
	case dmpIAx:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.A)<<instrPosAx
	case dmpIsJ:
		w = uint32(i.Op)<<instrPosOp |
			uint32(i.B)<<instrPosSJ |
			uint32(i.K)<<instrPosK
	}
	return w
}

// unpackInstruction decodes a 32-bit word (GET_OPCODE / GETARG_A ...).
func unpackInstruction(w uint32) Instruction {
	op := int(w>>instrPosOp) & int(instrMask(instrSizeOp))
	switch dmpOpModes[op] {
	case dmpIABC:
		return Instruction{
			Op: op,
			A:  int(w>>instrPosA) & int(instrMask(instrSizeA)),
			B:  int(w>>instrPosB) & int(instrMask(instrSizeB)),
			C:  int(w>>instrPosC) & int(instrMask(instrSizeC)),
			K:  int(w>>instrPosK) & int(instrMask(instrSizeK)),
		}
	case dmpIABx, dmpIAsBx:
		return Instruction{
			Op: op,
			A:  int(w>>instrPosA) & int(instrMask(instrSizeA)),
			B:  int(w>>instrPosBx) & int(instrMask(instrSizeBx)),
		}
	case dmpIAx:
		return Instruction{
			Op: op,
			A:  int(w>>instrPosAx) & int(instrMask(instrSizeAx)),
		}
	}
	/* isJ */
	return Instruction{
		Op: op,
		B:  int(w>>instrPosSJ) & int(instrMask(instrSizeSJ)),
		K:  int(w>>instrPosK) & int(instrMask(instrSizeK)),
	}
}
