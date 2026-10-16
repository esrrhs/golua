// Package lua is a pure-Go port of PUC-Lua 5.4.
//
// This file is the translation of lobject.h / lobject.c: the value
// representation (TValue) and the basic object operations. The garbage
// collector (lgc.c) is not translated: object lifetime is handled by the
// Go runtime.
package lua

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Basic type tags (lua.h).
const (
	LUA_TNONE          = -1
	LUA_TNIL           = 0
	LUA_TBOOLEAN       = 1
	LUA_TLIGHTUSERDATA = 2
	LUA_TNUMBER        = 3
	LUA_TSTRING        = 4
	LUA_TTABLE         = 5
	LUA_TFUNCTION      = 6
	LUA_TUSERDATA      = 7
	LUA_TTHREAD        = 8
	LUA_NUMTYPES       = 9
)

// Status codes (lua.h).
const (
	LUA_OK        = 0
	LUA_YIELD     = 1
	LUA_ERRRUN    = 2
	LUA_ERRSYNTAX = 3
	LUA_ERRMEM    = 4
	LUA_ERRERR    = 5
	LUA_ERRFILE   = LUA_ERRERR + 1 /* 'luaL_loadfilex': a file could not be opened */
)

// Stack limits (luaconf.h / lua.h).
const (
	LUA_MINSTACK    = 20
	LUAI_MAXSTACK   = 1000000
	LUA_MAXINTEGER  = int64(math.MaxInt64)
	LUA_MININTEGER  = int64(math.MinInt64)
	LUA_MULTRET     = -1
	LUA_IDSIZE      = 60
	MAXNUMBER2STR   = 44
	LUA_INTEGER_FMT = "%d"
	LUA_NUMBER_FMT  = "%.14g"
)

// Pseudo-indices (lua.h).
const (
	LUA_REGISTRYINDEX   = -LUAI_MAXSTACK - 1000
	LUA_RIDX_MAINTHREAD = 1
	LUA_RIDX_GLOBALS    = 2
	LUA_RIDX_LAST       = LUA_RIDX_GLOBALS
)

// Upvalueindex returns the pseudo-index of the i-th upvalue.
func UpvalueIndex(i int) int { return LUA_REGISTRYINDEX - i }

// Bits in a value tag (lobject.h).
const (
	BIT_ISCOLLECTABLE = 1 << 6
)

// makevariant / ctb / novariant, exactly as lobject.h.
func makevariant(t, v int) int { return t | (v << 4) }
func ctb(t int) int            { return t | BIT_ISCOLLECTABLE }
func novariant(t int) int      { return t & 0x0F }

// Variant tags (lobject.h). The values are 'makevariant(t, v)', written
// out literally because Go constants cannot call functions.
const (
	LUA_VNIL     = LUA_TNIL + 0<<4 // 0
	LUA_VEMPTY   = LUA_TNIL + 1<<4 // 16
	LUA_VABSTKEY = LUA_TNIL + 2<<4 // 32

	LUA_VFALSE = LUA_TBOOLEAN + 0<<4 // 1
	LUA_VTRUE  = LUA_TBOOLEAN + 1<<4 // 17

	LUA_VLIGHTUSERDATA = LUA_TLIGHTUSERDATA + 0<<4 // 2

	LUA_VNUMINT = LUA_TNUMBER + 0<<4 // 3
	LUA_VNUMFLT = LUA_TNUMBER + 1<<4 // 19

	LUA_VSHRSTR = LUA_TSTRING + 0<<4 // 4
	LUA_VLNGSTR = LUA_TSTRING + 1<<4 // 20

	LUA_VTABLE = LUA_TTABLE + 0<<4 // 5

	LUA_VLCL = LUA_TFUNCTION + 0<<4 // 6  Lua closure
	LUA_VLCF = LUA_TFUNCTION + 1<<4 // 22 light C function
	LUA_VCCL = LUA_TFUNCTION + 2<<4 // 38 C closure

	LUA_VUSERDATA = LUA_TUSERDATA + 0<<4 // 7
	LUA_VTHREAD   = LUA_TTHREAD + 0<<4   // 8

	LUA_VUPVAL = LUA_TUPVAL + 0<<4 // 144
	LUA_VPROTO = LUA_TPROTO + 0<<4 // 160
)

// Extra types for collectable non-values (lobject.h).
const (
	LUA_TUPVAL   = LUA_NUMTYPES
	LUA_TPROTO   = LUA_NUMTYPES + 1
	LUA_TDEADKEY = LUA_NUMTYPES + 2
)

// Arithmetic op codes (lua.h LUA_OP*).
const (
	LUA_OPADD  = 0
	LUA_OPSUB  = 1
	LUA_OPMUL  = 2
	LUA_OPMOD  = 3
	LUA_OPPOW  = 4
	LUA_OPDIV  = 5
	LUA_OPIDIV = 6
	LUA_OPBAND = 7
	LUA_OPBOR  = 8
	LUA_OPBXOR = 9
	LUA_OPSHL  = 10
	LUA_OPSHR  = 11
	LUA_OPUNM  = 12
	LUA_OPBNOT = 13
)

// Value is the translation of TValue: an actual value plus a tag.
// Fields mirror the C union: i for integers/booleans, n for floats and
// p for every collectable (or light userdata).
type Value struct {
	tt int
	i  int64
	n  float64
	p  any
}

// ---------------------------------------------------------------------
// tag tests (lobject.h macros)
// ---------------------------------------------------------------------

func ttype(v *Value) int    { return novariant(v.tt) }
func ttypetag(v *Value) int { return v.tt & 0x3F }

func ttisnil(v *Value) bool       { return ttype(v) == LUA_TNIL }
func ttisstrictnil(v *Value) bool { return v.tt == LUA_VNIL }
func isabstkey(v *Value) bool     { return v.tt == LUA_VABSTKEY }
func isempty(v *Value) bool       { return ttisnil(v) }

func ttisboolean(v *Value) bool { return ttype(v) == LUA_TBOOLEAN }
func ttisfalse(v *Value) bool   { return v.tt == LUA_VFALSE }
func ttistrue(v *Value) bool    { return v.tt == LUA_VTRUE }
func isFalse(v *Value) bool     { return ttisfalse(v) || ttisnil(v) }
func bvalue(v *Value) bool      { return v.tt == LUA_VTRUE }

func ttisnumber(v *Value) bool  { return ttype(v) == LUA_TNUMBER }
func ttisinteger(v *Value) bool { return v.tt == LUA_VNUMINT }
func ttisfloat(v *Value) bool   { return v.tt == LUA_VNUMFLT }

func ttisstring(v *Value) bool    { return ttype(v) == LUA_TSTRING }
func ttisshrstring(v *Value) bool { return v.tt == ctb(LUA_VSHRSTR) }
func ttislngstring(v *Value) bool { return v.tt == ctb(LUA_VLNGSTR) }

func ttistable(v *Value) bool { return v.tt == ctb(LUA_VTABLE) }

func ttisfunction(v *Value) bool { return ttype(v) == LUA_TFUNCTION }
func ttisLclosure(v *Value) bool { return v.tt == ctb(LUA_VLCL) }
func ttislcf(v *Value) bool      { return v.tt == LUA_VLCF }
func ttisCclosure(v *Value) bool { return v.tt == ctb(LUA_VCCL) }
func ttisclosure(v *Value) bool  { return ttisLclosure(v) || ttisCclosure(v) }

// ttiscfunction reports whether 'v' is any kind of C function (a light C
// function or a C closure), as 'lua_iscfunction' does.
func ttiscfunction(v *Value) bool { return ttislcf(v) || ttisCclosure(v) }

func ttisfulluserdata(v *Value) bool  { return v.tt == ctb(LUA_VUSERDATA) }
func ttislightuserdata(v *Value) bool { return v.tt == LUA_VLIGHTUSERDATA }
func ttisthread(v *Value) bool        { return v.tt == ctb(LUA_VTHREAD) }

// ---------------------------------------------------------------------
// value accessors
// ---------------------------------------------------------------------

func ivalue(v *Value) int64     { return v.i }
func fltvalue(v *Value) float64 { return v.n }
func nvalue(v *Value) float64 {
	if ttisinteger(v) {
		return float64(v.i)
	}
	return v.n
}
func tsvalue(v *Value) *TString   { return v.p.(*TString) }
func hvalue(v *Value) *Table      { return v.p.(*Table) }
func clLvalue(v *Value) *LClosure { return v.p.(*LClosure) }
func clCvalue(v *Value) *CClosure { return v.p.(*CClosure) }
func fvalue(v *Value) GoFunc      { return v.p.(GoFunc) }
func uvalue(v *Value) *Udata      { return v.p.(*Udata) }
func pvalue(v *Value) any         { return v.p }
func thvalue(v *Value) *State     { return v.p.(*State) }
func gcvalue(v *Value) any        { return v.p }

func iscollectable(v *Value) bool { return v.tt&BIT_ISCOLLECTABLE != 0 }

// ---------------------------------------------------------------------
// setters (lobject.h macros). Every setter clears the whole value so
// that no stale reference keeps an object alive.
// ---------------------------------------------------------------------

func setnilvalue(v *Value) { *v = Value{tt: LUA_VNIL} }
func setempty(v *Value)    { *v = Value{tt: LUA_VEMPTY} }
func setabstkey(v *Value)  { *v = Value{tt: LUA_VABSTKEY} }
func setbfvalue(v *Value)  { *v = Value{tt: LUA_VFALSE} }
func setbtvalue(v *Value)  { *v = Value{tt: LUA_VTRUE, i: 1} }
func setbvalue(v *Value, b bool) {
	if b {
		setbtvalue(v)
	} else {
		setbfvalue(v)
	}
}
func setivalue(v *Value, i int64)     { *v = Value{tt: LUA_VNUMINT, i: i} }
func setfltvalue(v *Value, f float64) { *v = Value{tt: LUA_VNUMFLT, n: f} }
func chgfltvalue(v *Value, f float64) { v.n = f }
func chgivalue(v *Value, i int64)     { v.i = i }
func setpvalue(v *Value, p any)       { *v = Value{tt: LUA_VLIGHTUSERDATA, p: p} }
func setfvalue(v *Value, f GoFunc)    { *v = Value{tt: LUA_VLCF, p: f} }

func setsvalue(v *Value, ts *TString)    { *v = Value{tt: ctb(ts.tt), p: ts} }
func sethvalue(v *Value, t *Table)       { *v = Value{tt: ctb(LUA_VTABLE), p: t} }
func setuvalue(v *Value, u *Udata)       { *v = Value{tt: ctb(LUA_VUSERDATA), p: u} }
func setthvalue(v *Value, th *State)     { *v = Value{tt: ctb(LUA_VTHREAD), p: th} }
func setclLvalue(v *Value, cl *LClosure) { *v = Value{tt: ctb(LUA_VLCL), p: cl} }
func setclCvalue(v *Value, cl *CClosure) { *v = Value{tt: ctb(LUA_VCCL), p: cl} }

func setobj(v1, v2 *Value) { *v1 = *v2 }

// ---------------------------------------------------------------------
// TString (lobject.h)
// ---------------------------------------------------------------------

// TString is the header for a Lua string value.
type TString struct {
	tt    int // LUA_VSHRSTR or LUA_VLNGSTR
	extra byte
	hash  uint32
	val   string
}

func (ts *TString) isShort() bool { return ts.tt == LUA_VSHRSTR }
func tsslen(ts *TString) int      { return len(ts.val) }
func getstr(ts *TString) string   { return ts.val }

// ---------------------------------------------------------------------
// Table (lobject.h)
// ---------------------------------------------------------------------

const BITRAS = 1 << 7

// node is a hash-table entry: a key/value pair plus a chain offset.
// next == -1 means "no next"; next == -2 marks a dead key.
type node struct {
	key  Value
	val  Value
	next int
}

const (
	noNext   = -1
	deadNext = -2
)

// Table is a Lua table: an array part plus an open-addressing hash part.
type Table struct {
	flags     byte // 1<<p means tagmethod(p) is not present
	lsizenode byte // log2 of size of 'node' array
	alimit    uint32
	array     []Value
	nodes     []node
	lastfree  int
	metatable *Table
	weakMode  byte // 0 or "k"/"v"/"kv" bits, replacing GC-driven weakness
	finalized bool // a Go finalizer has been set to run its '__gc'
}

func isrealasize(t *Table) bool { return t.flags&BITRAS == 0 }
func setrealasize(t *Table)     { t.flags &= ^byte(BITRAS) }
func setnorealasize(t *Table)   { t.flags |= BITRAS }
func sizenode(t *Table) int     { return 1 << t.lsizenode }
func twoto(x byte) int          { return 1 << x }

// ---------------------------------------------------------------------
// Udata (lobject.h)
// ---------------------------------------------------------------------

// Udata is a full userdata: a Go value plus a metatable and user values.
type Udata struct {
	nuvalue   uint16
	metatable *Table
	uv        []Value // user values
	val       any     // the Go value carried by this userdata
	finalized bool    // a Go finalizer has been set to run its '__gc'
}

// ---------------------------------------------------------------------
// Prototypes (lobject.h)
// ---------------------------------------------------------------------

// Upvaldesc describes an upvalue for function prototypes.
type Upvaldesc struct {
	name    *TString
	instack byte
	idx     byte
	kind    byte // RDKTOCLOSE etc.
}

// LocVar describes a local variable (debug information).
type LocVar struct {
	varname *TString
	startpc int
	endpc   int
}

// AbsLineInfo associates an absolute line with an instruction.
type AbsLineInfo struct {
	pc   int
	line int
}

// Proto is a function prototype.
type Proto struct {
	numparams    byte
	isVararg     byte
	maxstacksize byte

	sizeupvalues int
	sizek        int
	sizecode     int
	sizelineinfo int
	sizep        int
	sizelocvars  int

	linedefined     int
	lastlinedefined int

	k        []Value
	code     []Instruction
	p        []*Proto
	upvalues []Upvaldesc
	// lineinfo holds the absolute source line of each instruction.
	// (PUC-Lua stores line deltas plus an 'abslineinfo' array; the
	// absolute form is equivalent and much simpler.)
	lineinfo    []int32
	abslineinfo []AbsLineInfo
	locvars     []LocVar
	source      *TString
}

// ---------------------------------------------------------------------
// luaO_* functions (lobject.c)
// ---------------------------------------------------------------------

var log2Table = [256]byte{
	0, 1, 2, 2, 3, 3, 3, 3, 4, 4, 4, 4, 4, 4, 4, 4, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5, 5,
	6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6, 6,
	7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
	8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8, 8,
}

// luaO_ceillog2 computes ceil(log2(x)).
func luaO_ceillog2(x uint32) int {
	l := 0
	x--
	for x >= 256 {
		l += 8
		x >>= 8
	}
	return l + int(log2Table[x])
}

// luaO_hexavalue returns the value of a hexadecimal digit.
func luaO_hexavalue(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	/* C is '(ltolower(c) - 'a') + 10', where 'ltolower(c)' is
	   '(c | ('A' ^ 'a'))' -- a bitwise OR with 0x20, not a subtraction.
	   Doing the arithmetic on bytes would also wrap around. */
	return (int(c) | ('A' ^ 'a')) - 'a' + 10
}

func intarith(L *State, op int, v1, v2 int64) int64 {
	switch op {
	case LUA_OPADD:
		return v1 + v2
	case LUA_OPSUB:
		return v1 - v2
	case LUA_OPMUL:
		return v1 * v2
	case LUA_OPMOD:
		return luaV_mod(L, v1, v2)
	case LUA_OPIDIV:
		return luaV_idiv(L, v1, v2)
	case LUA_OPBAND:
		return v1 & v2
	case LUA_OPBOR:
		return v1 | v2
	case LUA_OPBXOR:
		return v1 ^ v2
	case LUA_OPSHL:
		return int64(luaV_shiftl(v1, v2))
	case LUA_OPSHR:
		return int64(luaV_shiftl(v1, -v2))
	case LUA_OPUNM:
		return -v1
	case LUA_OPBNOT:
		return ^v1
	}
	panic("invalid integer operation")
}

func numarith(L *State, op int, v1, v2 float64) float64 {
	switch op {
	case LUA_OPADD:
		return v1 + v2
	case LUA_OPSUB:
		return v1 - v2
	case LUA_OPMUL:
		return v1 * v2
	case LUA_OPDIV:
		return v1 / v2
	case LUA_OPPOW:
		return math.Pow(v1, v2)
	case LUA_OPIDIV:
		return math.Floor(v1 / v2)
	case LUA_OPUNM:
		return -v1
	case LUA_OPMOD:
		return luaV_modf(L, v1, v2)
	}
	panic("invalid float operation")
}

// luaO_rawarith performs the raw arithmetic operation; it returns false
// when the operands cannot be coerced to numbers (and a metamethod must
// be tried).
func luaO_rawarith(L *State, op int, p1, p2 *Value, res *Value) bool {
	switch op {
	case LUA_OPBAND, LUA_OPBOR, LUA_OPBXOR, LUA_OPSHL, LUA_OPSHR, LUA_OPBNOT:
		// operate only on integers
		var i1, i2 int64
		if tointegerns(p1, &i1) && tointegerns(p2, &i2) {
			setivalue(res, intarith(L, op, i1, i2))
			return true
		}
		return false
	case LUA_OPDIV, LUA_OPPOW:
		// operate only on floats
		var n1, n2 float64
		if tonumberns(p1, &n1) && tonumberns(p2, &n2) {
			setfltvalue(res, numarith(L, op, n1, n2))
			return true
		}
		return false
	default:
		var n1, n2 float64
		if ttisinteger(p1) && ttisinteger(p2) {
			setivalue(res, intarith(L, op, ivalue(p1), ivalue(p2)))
			return true
		} else if tonumberns(p1, &n1) && tonumberns(p2, &n2) {
			setfltvalue(res, numarith(L, op, n1, n2))
			return true
		}
		return false
	}
}

// luaO_arith performs an arithmetic operation, falling back to the
// corresponding metamethod.
func luaO_arith(L *State, op int, p1, p2 *Value, res int) {
	if !luaO_rawarith(L, op, p1, p2, &L.stack[res]) {
		luaT_trybinTM(L, p1, p2, res, TMS(op-LUA_OPADD+int(TM_ADD)))
	}
}

// luaO_utf8esc encodes x as UTF-8, returning the bytes.
func luaO_utf8esc(x uint32) []byte {
	if x < 0x80 {
		return []byte{byte(x)}
	}
	var buf [8]byte
	n := 1
	mfb := uint32(0x3f)
	for {
		buf[8-n] = byte(0x80 | (x & 0x3f))
		n++
		x >>= 6
		mfb >>= 1
		if x <= mfb {
			break
		}
	}
	buf[8-n] = byte((^mfb << 1) | x)
	return buf[8-n:]
}

// tostringbuff converts a number object to its string representation.
func tostringbuff(obj *Value) string {
	if ttisinteger(obj) {
		return strconv.FormatInt(ivalue(obj), 10)
	}
	s := fmtFloat(fltvalue(obj))
	if !strings.ContainsAny(s, ".eEnNaxi") { // looks like an integer?
		s += ".0"
	}
	return s
}

// fmtFloat formats a float with C's "%.14g" (LUAI_NUMFFORMAT).
//
// Go spells the non-finite values the way Go spells them ("NaN",
// "+Inf"), while C's printf spells them "nan"/"inf"/"-inf". Lua's
// number-to-string conversion is user visible, so C wins here.
func fmtFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "nan"
	case math.IsInf(f, +1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	}
	return strconv.FormatFloat(f, 'g', 14, 64)
}

// luaO_tostring converts a number object into a string, in place.
func luaO_tostring(L *State, obj *Value) {
	setsvalue(obj, luaS_newlstr(L, tostringbuff(obj)))
}

// luaO_chunkid formats a source into a chunk id (lobject.c).
func luaO_chunkid(source string) string {
	const rets = "..."
	const pre = "[string \""
	const pos = "\"]"
	bufflen := LUA_IDSIZE               /* free space in buffer */
	if strings.HasPrefix(source, "=") { /* 'literal' source */
		/* C's 'srclen' counts the '=' as well, so the source of a
		   literal is kept only when it is at most LUA_IDSIZE-1 long. */
		if len(source) <= bufflen { /* small enough? */
			return source[1:] /* keep it */
		}
		return source[1 : 1+bufflen-1] /* truncate it */
	}
	if strings.HasPrefix(source, "@") { /* file name */
		if len(source) <= bufflen { /* small enough? */
			return source[1:] /* keep it */
		}
		/* add '...' before rest of name.  C copies 'bufflen' bytes
		   from 'source + 1 + srclen - bufflen', but only 'bufflen - 1'
		   of them are real characters -- the rest is the terminating
		   '\0' -- so the result is one character shorter than
		   LUA_IDSIZE. */
		src := source[1:]
		n := bufflen - len(rets) - 1
		if n < 0 {
			n = 0
		}
		if n > len(src) {
			n = len(src)
		}
		return rets + src[len(src)-n:]
	}
	/* string; format as [string "source"] */
	nl := strings.IndexByte(source, '\n') /* find first new line (if any) */
	out := pre
	bufflen -= len(pre) + len(rets) + len(pos) + 1 /* save space for prefix+suffix+'\0' */
	if len(source) < bufflen && nl < 0 {           /* small one-line source? */
		return out + source + pos /* keep it */
	}
	src := source
	if nl >= 0 {
		src = src[:nl] /* stop at first newline */
	}
	if len(src) > bufflen {
		src = src[:bufflen]
	}
	return out + src + rets + pos
}

// ---------------------------------------------------------------------
// string -> number conversion (lobject.c)
// ---------------------------------------------------------------------

func lisspace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

func isneg(s *string) int {
	if len(*s) > 0 && (*s)[0] == '-' {
		*s = (*s)[1:]
		return 1
	}
	if len(*s) > 0 && (*s)[0] == '+' {
		*s = (*s)[1:]
	}
	return 0
}

func lisxdigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// l_str2int converts a string to an integer (decimal or hexadecimal).
func l_str2int(s string) (int64, bool) {
	var a uint64
	empty := true
	for len(s) > 0 && lisspace(s[0]) {
		s = s[1:]
	}
	neg := isneg(&s)
	if len(s) >= 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		s = s[2:]
		for len(s) > 0 && lisxdigit(s[0]) {
			a = a*16 + uint64(luaO_hexavalue(s[0]))
			empty = false
			s = s[1:]
		}
	} else {
		const maxby10 = uint64(LUA_MAXINTEGER / 10)
		const maxlastd = int(LUA_MAXINTEGER % 10)
		for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			d := int(s[0] - '0')
			if a >= maxby10 && (a > maxby10 || d > maxlastd+neg) {
				return 0, false // overflow
			}
			a = a*10 + uint64(d)
			empty = false
			s = s[1:]
		}
	}
	for len(s) > 0 && lisspace(s[0]) {
		s = s[1:]
	}
	if empty || len(s) != 0 {
		return 0, false
	}
	if neg != 0 {
		return int64(-a), true
	}
	return int64(a), true
}

// l_str2d converts a string to a float, accepting Lua numerals only.
func l_str2d(s string) (float64, bool) {
	mode := byte(0)
	if i := strings.IndexAny(s, ".xXnN"); i >= 0 {
		mode = s[i]
		if mode >= 'A' && mode <= 'Z' {
			mode += 'a' - 'A'
		}
	}
	if mode == 'n' { // reject 'inf' and 'nan'
		return 0, false
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return 0, false
	}
	if mode == 'x' {
		/* Go's ParseFloat is a correctly rounded hexadecimal strtod, but
		   it requires the binary exponent, which Lua makes optional:
		   "0xAA.0" is a valid numeral. Adding a zero exponent lets Go
		   do the whole job, including the very long mantissas (150 and
		   500 digits) whose exact value a hand-written parser cannot
		   keep in a 64-bit accumulator. */
		sx := t
		if !strings.ContainsAny(sx, "pP") {
			sx += "p0"
		}
		if v, err := strconv.ParseFloat(sx, 64); floatOK(v, err) {
			return v, true
		}
		if v, ok := parseHexFloat(t); ok { /* syntaxes Go rejects */
			return v, true
		}
		return 0, false
	}
	if v, err := strconv.ParseFloat(t, 64); floatOK(v, err) {
		return v, true
	}
	return 0, false
}

// floatOK reports whether a strconv.ParseFloat result should be accepted.
// Besides a clean conversion, C's 'strtod' also succeeds when the value
// overflows (returning HUGE_VAL, i.e. inf) or underflows (returning 0);
// Go reports those as ErrRange but still returns the saturated value, so
// '1e400' must be accepted as 'math.huge'.
func floatOK(v float64, err error) bool {
	if err == nil {
		return true
	}
	ne, ok := err.(*strconv.NumError)
	return ok && ne.Err == strconv.ErrRange
}

// hexToGoFloat rewrites a Lua hexadecimal numeral (0x1p4) into a form Go
// understands (0x1p4 is already valid Go syntax, so only the case of a
// plain hexadecimal integer such as "0xff" needs handling).
// parseHexFloat parses Lua hexadecimal numerals, with or without exponent.
func parseHexFloat(s string) (float64, bool) {
	neg := false
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if len(s) < 2 || s[0] != '0' || (s[1] != 'x' && s[1] != 'X') {
		return 0, false
	}
	s = s[2:]
	var mant uint64
	ndig := 0
	for len(s) > 0 && lisxdigit(s[0]) {
		mant = mant*16 + uint64(luaO_hexavalue(s[0]))
		ndig++
		s = s[1:]
	}
	var frac float64
	if len(s) > 0 && s[0] == '.' {
		s = s[1:]
		scale := 1.0
		for len(s) > 0 && lisxdigit(s[0]) {
			scale /= 16
			frac += float64(luaO_hexavalue(s[0])) * scale
			ndig++
			s = s[1:]
		}
	}
	if ndig == 0 {
		return 0, false
	}
	r := float64(mant) + frac
	if len(s) > 0 && (s[0] == 'p' || s[0] == 'P') {
		s = s[1:]
		esign := 1
		if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
			if s[0] == '-' {
				esign = -1
			}
			s = s[1:]
		}
		exp := 0
		nd := 0
		for len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
			exp = exp*10 + int(s[0]-'0')
			nd++
			s = s[1:]
		}
		if nd == 0 {
			return 0, false
		}
		r *= math.Pow(2, float64(esign*exp))
	}
	if len(strings.TrimSpace(s)) != 0 {
		return 0, false
	}
	if neg {
		r = -r
	}
	return r, true
}

// luaO_str2num converts a string to a Lua number; it returns false when
// the string is not a valid numeral.
func luaO_str2num(s string, o *Value) bool {
	if i, ok := l_str2int(s); ok {
		setivalue(o, i)
		return true
	}
	if n, ok := l_str2d(s); ok {
		setfltvalue(o, n)
		return true
	}
	return false
}

// ---------------------------------------------------------------------
// luaO_pushvfstring / luaO_pushfstring (lobject.c)
// ---------------------------------------------------------------------

// pushfString builds a string on the stack, handling Lua's '%d', '%I',
// '%f', '%s', '%c', '%p', '%U' and '%%' formats.
func pushfString(L *State, format string, args ...any) *TString {
	var b strings.Builder
	i := 0
	ai := 0
	next := func() any {
		if ai >= len(args) {
			luaG_runerror(L, "bad argument #%d to 'pushfstring'", ai+1)
		}
		a := args[ai]
		ai++
		return a
	}
	for i < len(format) {
		if format[i] != '%' {
			b.WriteByte(format[i])
			i++
			continue
		}
		// format[i] == '%'
		if i+1 >= len(format) {
			b.WriteByte('%')
			break
		}
		switch format[i+1] {
		case 's':
			a := next()
			if a == nil {
				b.WriteString("(null)")
			} else {
				b.WriteString(toStringAny(a))
			}
		case 'c':
			a := next()
			b.WriteByte(byte(toIntAny(a)))
		case 'd':
			a := next()
			b.WriteString(strconv.FormatInt(toIntAny(a), 10))
		case 'I':
			a := next()
			b.WriteString(strconv.FormatInt(toIntAny(a), 10))
		case 'f':
			a := next()
			switch v := a.(type) {
			case float64:
				b.WriteString(fmtFloat(v))
			case float32:
				b.WriteString(fmtFloat(float64(v)))
			default:
				b.WriteString(fmtFloat(float64(toIntAny(a))))
			}
		case 'p':
			a := next()
			b.WriteString("0x" + strconv.FormatUint(uint64(toPtrAny(a)), 16))
		case 'U':
			a := next()
			b.Write(luaO_utf8esc(uint32(toIntAny(a))))
		case '%':
			b.WriteByte('%')
		default:
			luaG_runerror(L, "invalid option '%%%c' to 'lua_pushfstring'", format[i+1])
		}
		i += 2
	}
	return luaS_newlstr(L, b.String())
}

func toStringAny(a any) string {
	switch v := a.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case *TString:
		return v.val
	case nil:
		return "(null)"
	}
	return fmt.Sprintf("%v", a)
}

func toIntAny(a any) int64 {
	switch v := a.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case int32:
		return int64(v)
	case uint:
		return int64(v)
	case uint64:
		return int64(v)
	case byte:
		return int64(v)
	case float64:
		return int64(v)
	}
	return 0
}

// ptrID returns a stable identity for a pointer-like value, used by
// lua_topointer and tostring(). (The C implementation uses the address;
// here we use the Go pointer value, which is stable because the Go GC
// does not move heap objects.)
func ptrID(v any) uintptr {
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.UnsafePointer, reflect.Map, reflect.Slice,
		reflect.Chan, reflect.Func, reflect.Interface:
		if rv.IsNil() {
			return 0
		}
		return rv.Pointer()
	}
	return 0
}

func toPtrAny(a any) uintptr {
	switch v := a.(type) {
	case *Table:
		return ptrID(v)
	case *TString:
		return ptrID(v)
	case *LClosure:
		return ptrID(v)
	case *CClosure:
		return ptrID(v)
	case *Udata:
		return ptrID(v)
	case *State:
		return ptrID(v)
	}
	return 0
}

// ---------------------------------------------------------------------
// misc helpers used by several modules
// ---------------------------------------------------------------------

// luaV_rawequalobj is the raw (non-metamethod) equality.
//
// lvm.h defines it as 'luaV_equalobj(NULL, t1, t2)': a nil state disables
// the __eq metamethod, but numbers of different variants are still
// compared with the exact integer conversion, which a plain float
// comparison is not (9007199254740993 ~= 9007199254740993.0, whose float
// value is ...992). Getting this wrong also merges those two constants
// into one in lcode.c's constant table.
func luaV_rawequalobj(t1, t2 *Value) bool {
	return luaV_equalobj(nil, t1, t2)
}

// utf8 decode helper used by the lexer and libraries.
func utf8Decode(s string) (rune, int) { return utf8.DecodeRuneInString(s) }
