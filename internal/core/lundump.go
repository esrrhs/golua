// Translation of lundump.c / lundump.h (PUC-Lua 5.3.4): load precompiled
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
package core

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
	// LUAC_VERSION for Lua 5.3: major*16+minor = 0x53
	dmpVersion = 0x53
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

