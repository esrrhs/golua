// Package lua is a pure-Go implementation of PUC-Lua 5.4.
//
// This is the public API surface. All implementation lives in
// internal/core; this file re-exports the types, constants, and
// functions that external callers need so that the import path
// github.com/esrrhs/golua keeps working exactly as before.
package lua

import (
	"io"

	core "github.com/esrrhs/golua/internal/core"
)

// ---------------------------------------------------------------------------
// Type aliases – methods defined on the core types are inherited automatically.
// ---------------------------------------------------------------------------

type (
	State       = core.State
	Value       = core.Value
	TString     = core.TString
	Table       = core.Table
	Udata       = core.Udata
	Upvaldesc   = core.Upvaldesc
	LocVar      = core.LocVar
	AbsLineInfo = core.AbsLineInfo
	Proto       = core.Proto
	Instruction = core.Instruction
	Reg         = core.Reg
	Buffer      = core.Buffer
	LuaError    = core.LuaError
	GoFunc      = core.GoFunc
	WarnFunc    = core.WarnFunc
	CallInfo    = core.CallInfo
	LoadState   = core.LoadState
	DumpState   = core.DumpState
	SemInfo     = core.SemInfo
	Token       = core.Token
	LexState    = core.LexState
	BinOpr      = core.BinOpr
	UnOpr       = core.UnOpr
	UpVal       = core.UpVal
	CClosure    = core.CClosure
	LClosure    = core.LClosure
	Vardesc     = core.Vardesc
	Labeldesc   = core.Labeldesc
	Labellist   = core.Labellist
	Dyndata     = core.Dyndata
	BlockCnt    = core.BlockCnt
	FuncState   = core.FuncState
	ConsControl = core.ConsControl
	LHS_assign  = core.LHS_assign
	TMS         = core.TMS
	F2Imod      = core.F2Imod
)

// ---------------------------------------------------------------------------
// Package-level function wrappers
// ---------------------------------------------------------------------------

func NewState() *State                           { return core.NewState() }
func TypeName(tp int) string                     { return core.TypeName(tp) }
func UpvalueIndex(i int) int                     { return core.UpvalueIndex(i) }
func OpenBase(L *State) int                      { return core.OpenBase(L) }
func OpenCoroutine(L *State) int                 { return core.OpenCoroutine(L) }
func OpenPackage(L *State) int                   { return core.OpenPackage(L) }
func OpenIO(L *State) int                        { return core.OpenIO(L) }
func OpenOS(L *State) int                        { return core.OpenOS(L) }
func OpenString(L *State) int                    { return core.OpenString(L) }
func OpenMath(L *State) int                      { return core.OpenMath(L) }
func OpenUTF8(L *State) int                      { return core.OpenUTF8(L) }
func OpenDebug(L *State) int                     { return core.OpenDebug(L) }
func OpenTable(L *State) int                     { return core.OpenTable(L) }
func Luac(argv []string, out, err io.Writer) int { return core.Luac(argv, out, err) }

// ---------------------------------------------------------------------------
// Constants – lobject.go
// ---------------------------------------------------------------------------

const (
	LUA_TNONE          = core.LUA_TNONE
	LUA_TNIL           = core.LUA_TNIL
	LUA_TBOOLEAN       = core.LUA_TBOOLEAN
	LUA_TLIGHTUSERDATA = core.LUA_TLIGHTUSERDATA
	LUA_TNUMBER        = core.LUA_TNUMBER
	LUA_TSTRING        = core.LUA_TSTRING
	LUA_TTABLE         = core.LUA_TTABLE
	LUA_TFUNCTION      = core.LUA_TFUNCTION
	LUA_TUSERDATA      = core.LUA_TUSERDATA
	LUA_TTHREAD        = core.LUA_TTHREAD
	LUA_NUMTYPES       = core.LUA_NUMTYPES
)

const (
	LUA_OK        = core.LUA_OK
	LUA_YIELD     = core.LUA_YIELD
	LUA_ERRRUN    = core.LUA_ERRRUN
	LUA_ERRSYNTAX = core.LUA_ERRSYNTAX
	LUA_ERRMEM    = core.LUA_ERRMEM
	LUA_ERRERR    = core.LUA_ERRERR
	LUA_ERRFILE   = core.LUA_ERRFILE
)

const (
	LUA_MINSTACK    = core.LUA_MINSTACK
	LUAI_MAXSTACK   = core.LUAI_MAXSTACK
	LUA_MAXINTEGER  = core.LUA_MAXINTEGER
	LUA_MININTEGER  = core.LUA_MININTEGER
	LUA_MULTRET     = core.LUA_MULTRET
	LUA_IDSIZE      = core.LUA_IDSIZE
	MAXNUMBER2STR   = core.MAXNUMBER2STR
	LUA_INTEGER_FMT = core.LUA_INTEGER_FMT
	LUA_NUMBER_FMT  = core.LUA_NUMBER_FMT
)

const (
	LUA_REGISTRYINDEX   = core.LUA_REGISTRYINDEX
	LUA_RIDX_MAINTHREAD = core.LUA_RIDX_MAINTHREAD
	LUA_RIDX_GLOBALS    = core.LUA_RIDX_GLOBALS
	LUA_RIDX_LAST       = core.LUA_RIDX_LAST
)

const (
	BIT_ISCOLLECTABLE = core.BIT_ISCOLLECTABLE

	LUA_VNIL           = core.LUA_VNIL
	LUA_VEMPTY         = core.LUA_VEMPTY
	LUA_VABSTKEY       = core.LUA_VABSTKEY
	LUA_VFALSE         = core.LUA_VFALSE
	LUA_VTRUE          = core.LUA_VTRUE
	LUA_VLIGHTUSERDATA = core.LUA_VLIGHTUSERDATA
	LUA_VNUMINT        = core.LUA_VNUMINT
	LUA_VNUMFLT        = core.LUA_VNUMFLT
	LUA_VSHRSTR        = core.LUA_VSHRSTR
	LUA_VLNGSTR        = core.LUA_VLNGSTR
	LUA_VTABLE         = core.LUA_VTABLE
	LUA_VLCL           = core.LUA_VLCL
	LUA_VLCF           = core.LUA_VLCF
	LUA_VCCL           = core.LUA_VCCL
	LUA_VUSERDATA      = core.LUA_VUSERDATA
	LUA_VTHREAD        = core.LUA_VTHREAD
	LUA_VUPVAL         = core.LUA_VUPVAL
	LUA_VPROTO         = core.LUA_VPROTO

	LUA_TUPVAL   = core.LUA_TUPVAL
	LUA_TPROTO   = core.LUA_TPROTO
	LUA_TDEADKEY = core.LUA_TDEADKEY
)

const (
	LUA_OPADD  = core.LUA_OPADD
	LUA_OPSUB  = core.LUA_OPSUB
	LUA_OPMUL  = core.LUA_OPMUL
	LUA_OPMOD  = core.LUA_OPMOD
	LUA_OPPOW  = core.LUA_OPPOW
	LUA_OPDIV  = core.LUA_OPDIV
	LUA_OPIDIV = core.LUA_OPIDIV
	LUA_OPBAND = core.LUA_OPBAND
	LUA_OPBOR  = core.LUA_OPBOR
	LUA_OPBXOR = core.LUA_OPBXOR
	LUA_OPSHL  = core.LUA_OPSHL
	LUA_OPSHR  = core.LUA_OPSHR
	LUA_OPUNM  = core.LUA_OPUNM
	LUA_OPBNOT = core.LUA_OPBNOT
)

// ---------------------------------------------------------------------------
// Constants – lopcodes.go
// ---------------------------------------------------------------------------

const (
	OP_MOVE       = core.OP_MOVE
	OP_LOADI      = core.OP_LOADI
	OP_LOADF      = core.OP_LOADF
	OP_LOADK      = core.OP_LOADK
	OP_LOADKX     = core.OP_LOADKX
	OP_LOADFALSE  = core.OP_LOADFALSE
	OP_LFALSESKIP = core.OP_LFALSESKIP
	OP_LOADTRUE   = core.OP_LOADTRUE
	OP_LOADNIL    = core.OP_LOADNIL
	OP_GETUPVAL   = core.OP_GETUPVAL
	OP_SETUPVAL   = core.OP_SETUPVAL
	OP_GETTABUP   = core.OP_GETTABUP
	OP_GETTABLE   = core.OP_GETTABLE
	OP_GETI       = core.OP_GETI
	OP_GETFIELD   = core.OP_GETFIELD
	OP_SETTABUP   = core.OP_SETTABUP
	OP_SETTABLE   = core.OP_SETTABLE
	OP_SETI       = core.OP_SETI
	OP_SETFIELD   = core.OP_SETFIELD
	OP_NEWTABLE   = core.OP_NEWTABLE
	OP_SELF       = core.OP_SELF
	OP_ADDI       = core.OP_ADDI
	OP_ADDK       = core.OP_ADDK
	OP_SUBK       = core.OP_SUBK
	OP_MULK       = core.OP_MULK
	OP_MODK       = core.OP_MODK
	OP_POWK       = core.OP_POWK
	OP_DIVK       = core.OP_DIVK
	OP_IDIVK      = core.OP_IDIVK
	OP_BANDK      = core.OP_BANDK
	OP_BORK       = core.OP_BORK
	OP_BXORK      = core.OP_BXORK
	OP_SHRI       = core.OP_SHRI
	OP_SHLI       = core.OP_SHLI
	OP_ADD        = core.OP_ADD
	OP_SUB        = core.OP_SUB
	OP_MUL        = core.OP_MUL
	OP_MOD        = core.OP_MOD
	OP_POW        = core.OP_POW
	OP_DIV        = core.OP_DIV
	OP_IDIV       = core.OP_IDIV
	OP_BAND       = core.OP_BAND
	OP_BOR        = core.OP_BOR
	OP_BXOR       = core.OP_BXOR
	OP_SHL        = core.OP_SHL
	OP_SHR        = core.OP_SHR
	OP_MMBIN      = core.OP_MMBIN
	OP_MMBINI     = core.OP_MMBINI
	OP_MMBINK     = core.OP_MMBINK
	OP_UNM        = core.OP_UNM
	OP_BNOT       = core.OP_BNOT
	OP_NOT        = core.OP_NOT
	OP_LEN        = core.OP_LEN
	OP_CONCAT     = core.OP_CONCAT
	OP_CLOSE      = core.OP_CLOSE
	OP_TBC        = core.OP_TBC
	OP_JMP        = core.OP_JMP
	OP_EQ         = core.OP_EQ
	OP_LT         = core.OP_LT
	OP_LE         = core.OP_LE
	OP_EQK        = core.OP_EQK
	OP_EQI        = core.OP_EQI
	OP_LTI        = core.OP_LTI
	OP_LEI        = core.OP_LEI
	OP_GTI        = core.OP_GTI
	OP_GEI        = core.OP_GEI
	OP_TEST       = core.OP_TEST
	OP_TESTSET    = core.OP_TESTSET
	OP_CALL       = core.OP_CALL
	OP_TAILCALL   = core.OP_TAILCALL
	OP_RETURN     = core.OP_RETURN
	OP_RETURN0    = core.OP_RETURN0
	OP_RETURN1    = core.OP_RETURN1
	OP_FORLOOP    = core.OP_FORLOOP
	OP_FORPREP    = core.OP_FORPREP
	OP_TFORPREP   = core.OP_TFORPREP
	OP_TFORCALL   = core.OP_TFORCALL
	OP_TFORLOOP   = core.OP_TFORLOOP
	OP_SETLIST    = core.OP_SETLIST
	OP_CLOSURE    = core.OP_CLOSURE
	OP_VARARG     = core.OP_VARARG
	OP_VARARGPREP = core.OP_VARARGPREP
	OP_EXTRAARG   = core.OP_EXTRAARG
	NUM_OPCODES   = core.NUM_OPCODES
)

const (
	MAXARG_C          = core.MAXARG_C
	MAXARG_sC         = core.MAXARG_sC
	MAXARG_B          = core.MAXARG_B
	MAXARG_sB         = core.MAXARG_sB
	MAXARG_Bx         = core.MAXARG_Bx
	MAXARG_sBx        = core.MAXARG_sBx
	MAXARG_Ax         = core.MAXARG_Ax
	MAXARG_sJ         = core.MAXARG_sJ
	OFFSET_sC         = core.OFFSET_sC
	OFFSET_sB         = core.OFFSET_sB
	OFFSET_sBx        = core.OFFSET_sBx
	OFFSET_sJ         = core.OFFSET_sJ
	MAXARG_A          = core.MAXARG_A
	SIZE_A            = core.SIZE_A
	MAXINDEXRK        = core.MAXINDEXRK
	NO_REG            = core.NO_REG
	LFIELDS_PER_FLUSH = core.LFIELDS_PER_FLUSH
	MAXUPVAL          = core.MAXUPVAL
)

// ---------------------------------------------------------------------------
// Constants – lstate.go
// ---------------------------------------------------------------------------

const (
	EXTRA_STACK      = core.EXTRA_STACK
	BASIC_STACK_SIZE = core.BASIC_STACK_SIZE
	ERRORSTACKSIZE   = core.ERRORSTACKSIZE

	CIST_OAH       = core.CIST_OAH
	CIST_C         = core.CIST_C
	CIST_FRESH     = core.CIST_FRESH
	CIST_HOOKED    = core.CIST_HOOKED
	CIST_YPCALL    = core.CIST_YPCALL
	CIST_TAIL      = core.CIST_TAIL
	CIST_HOOKYIELD = core.CIST_HOOKYIELD
	CIST_FIN       = core.CIST_FIN
	CIST_TRAN      = core.CIST_TRAN
	CIST_CLSRET    = core.CIST_CLSRET
)

// ---------------------------------------------------------------------------
// Constants – lcode.go
// ---------------------------------------------------------------------------

const (
	NO_JUMP = core.NO_JUMP
	MAXREGS = core.MAXREGS

	OPR_ADD      = core.OPR_ADD
	OPR_SUB      = core.OPR_SUB
	OPR_MUL      = core.OPR_MUL
	OPR_MOD      = core.OPR_MOD
	OPR_POW      = core.OPR_POW
	OPR_DIV      = core.OPR_DIV
	OPR_IDIV     = core.OPR_IDIV
	OPR_BAND     = core.OPR_BAND
	OPR_BOR      = core.OPR_BOR
	OPR_BXOR     = core.OPR_BXOR
	OPR_SHL      = core.OPR_SHL
	OPR_SHR      = core.OPR_SHR
	OPR_CONCAT   = core.OPR_CONCAT
	OPR_EQ       = core.OPR_EQ
	OPR_LT       = core.OPR_LT
	OPR_LE       = core.OPR_LE
	OPR_NE       = core.OPR_NE
	OPR_GT       = core.OPR_GT
	OPR_GE       = core.OPR_GE
	OPR_AND      = core.OPR_AND
	OPR_OR       = core.OPR_OR
	OPR_NOBINOPR = core.OPR_NOBINOPR

	OPR_MINUS   = core.OPR_MINUS
	OPR_BNOT    = core.OPR_BNOT
	OPR_NOT     = core.OPR_NOT
	OPR_LEN     = core.OPR_LEN
	OPR_NOUNOPR = core.OPR_NOUNOPR
)

// ---------------------------------------------------------------------------
// Constants – llex.go
// ---------------------------------------------------------------------------

const (
	EOZ            = core.EOZ
	FIRST_RESERVED = core.FIRST_RESERVED
	NUM_RESERVED   = core.NUM_RESERVED
	LUA_ENV        = core.LUA_ENV

	TK_AND      = core.TK_AND
	TK_BREAK    = core.TK_BREAK
	TK_DO       = core.TK_DO
	TK_ELSE     = core.TK_ELSE
	TK_ELSEIF   = core.TK_ELSEIF
	TK_END      = core.TK_END
	TK_FALSE    = core.TK_FALSE
	TK_FOR      = core.TK_FOR
	TK_FUNCTION = core.TK_FUNCTION
	TK_GOTO     = core.TK_GOTO
	TK_IF       = core.TK_IF
	TK_IN       = core.TK_IN
	TK_LOCAL    = core.TK_LOCAL
	TK_NIL      = core.TK_NIL
	TK_NOT      = core.TK_NOT
	TK_OR       = core.TK_OR
	TK_REPEAT   = core.TK_REPEAT
	TK_RETURN   = core.TK_RETURN
	TK_THEN     = core.TK_THEN
	TK_TRUE     = core.TK_TRUE
	TK_UNTIL    = core.TK_UNTIL
	TK_WHILE    = core.TK_WHILE
	TK_IDIV     = core.TK_IDIV
	TK_CONCAT   = core.TK_CONCAT
	TK_DOTS     = core.TK_DOTS
	TK_EQ       = core.TK_EQ
	TK_GE       = core.TK_GE
	TK_LE       = core.TK_LE
	TK_NE       = core.TK_NE
	TK_SHL      = core.TK_SHL
	TK_SHR      = core.TK_SHR
	TK_DBCOLON  = core.TK_DBCOLON
	TK_EOS      = core.TK_EOS
	TK_FLT      = core.TK_FLT
	TK_INT      = core.TK_INT
	TK_NAME     = core.TK_NAME
	TK_STRING   = core.TK_STRING
)

// ---------------------------------------------------------------------------
// Constants – lparser.go
// ---------------------------------------------------------------------------

const (
	MAXVARS        = core.MAXVARS
	SHRT_MAX       = core.SHRT_MAX
	UNARY_PRIORITY = core.UNARY_PRIORITY

	VDKREG     = core.VDKREG
	RDKCONST   = core.RDKCONST
	RDKTOCLOSE = core.RDKTOCLOSE
	RDKCTC     = core.RDKCTC

	VVOID     = core.VVOID
	VNIL      = core.VNIL
	VTRUE     = core.VTRUE
	VFALSE    = core.VFALSE
	VK        = core.VK
	VKFLT     = core.VKFLT
	VKINT     = core.VKINT
	VKSTR     = core.VKSTR
	VNONRELOC = core.VNONRELOC
	VLOCAL    = core.VLOCAL
	VUPVAL    = core.VUPVAL
	VCONST    = core.VCONST
	VINDEXED  = core.VINDEXED
	VINDEXUP  = core.VINDEXUP
	VINDEXI   = core.VINDEXI
	VINDEXSTR = core.VINDEXSTR
	VJMP      = core.VJMP
	VRELOC    = core.VRELOC
	VCALL     = core.VCALL
	VVARARG   = core.VVARARG
)

// ---------------------------------------------------------------------------
// Constants – ltm.go
// ---------------------------------------------------------------------------

const (
	TM_INDEX    = core.TM_INDEX
	TM_NEWINDEX = core.TM_NEWINDEX
	TM_GC       = core.TM_GC
	TM_MODE     = core.TM_MODE
	TM_LEN      = core.TM_LEN
	TM_EQ       = core.TM_EQ
	TM_ADD      = core.TM_ADD
	TM_SUB      = core.TM_SUB
	TM_MUL      = core.TM_MUL
	TM_MOD      = core.TM_MOD
	TM_POW      = core.TM_POW
	TM_DIV      = core.TM_DIV
	TM_IDIV     = core.TM_IDIV
	TM_BAND     = core.TM_BAND
	TM_BOR      = core.TM_BOR
	TM_BXOR     = core.TM_BXOR
	TM_SHL      = core.TM_SHL
	TM_SHR      = core.TM_SHR
	TM_UNM      = core.TM_UNM
	TM_BNOT     = core.TM_BNOT
	TM_LT       = core.TM_LT
	TM_LE       = core.TM_LE
	TM_CONCAT   = core.TM_CONCAT
	TM_CALL     = core.TM_CALL
	TM_CLOSE    = core.TM_CLOSE
	TM_N        = core.TM_N
)

// ---------------------------------------------------------------------------
// Constants – misc
// ---------------------------------------------------------------------------

const (
	LUAI_MAXCCALLS   = core.LUAI_MAXCCALLS
	LUAI_MAXSHORTLEN = core.LUAI_MAXSHORTLEN
	MAXABITS         = core.MAXABITS
	MAXASIZE         = core.MAXASIZE
	MAXHBITS         = core.MAXHBITS
	MAXTAGLOOP       = core.MAXTAGLOOP
	NBITS            = core.NBITS
	LUA_NOREF        = core.LUA_NOREF
	LUA_REFNIL       = core.LUA_REFNIL
	LUA_COPYRIGHT    = core.LUA_COPYRIGHT
	LUA_FLOORN2I     = core.LUA_FLOORN2I
	F2Ieq            = core.F2Ieq
	F2Ifloor         = core.F2Ifloor
	F2Iceil          = core.F2Iceil
)
