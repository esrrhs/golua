// Translation of lparser.h / lparser.c (PUC-Lua 5.4.9).
//
// Adaptations:
//   - the block/function states are heap allocated (Go has no stack objects,
//     and the collector reclaims them);
//   - luaC_objbarrier / luaC_checkGC are gone;
//   - luaM_growvector becomes append;
//   - enterlevel/leavelevel guard the recursion depth directly.
package lua

import "fmt"

// MAXVARS is the maximum number of local variables per function (must be
// smaller than 250, due to the bytecode format).
const MAXVARS = 200

// SHRT_MAX limits the size of the debug-information arrays.
const SHRT_MAX = 32767

// UNARY_PRIORITY is the priority of unary operators.
const UNARY_PRIORITY = 12

// kinds of variables
const (
	VDKREG     = 0 // regular
	RDKCONST   = 1 // constant
	RDKTOCLOSE = 2 // to-be-closed
	RDKCTC     = 3 // compile-time constant
)

// hasmultret reports whether an expression kind has multiple returns.
func hasmultret(k expkind) bool { return k == VCALL || k == VVARARG }

// eqstr compares two strings. Because all strings are unified by the
// scanner, the parser can use pointer equality.
func eqstr(a, b *TString) bool { return a == b }

// ---------------------------------------------------------------------
// Expression descriptor (lparser.h)
// ---------------------------------------------------------------------

// expkind enumerates the kinds of variables/expressions.
type expkind int

const (
	VVOID expkind = iota // no value
	VNIL
	VTRUE
	VFALSE
	VK        // constant in 'k'; info = index of constant in 'k'
	VKFLT     // floating constant; nval = numerical float value
	VKINT     // integer constant; ival = numerical integer value
	VKSTR     // string constant; strval = TString address
	VNONRELOC // expression has its value in a fixed register
	VLOCAL    // local variable; varRidx = register, varVidx = compiler index
	VUPVAL    // upvalue variable; info = index of upvalue in 'upvalues'
	VCONST    // compile-time <const> variable; info = absolute index in actvar
	VINDEXED  // indexed variable; indT = table register, indIdx = key's R index
	VINDEXUP  // indexed upvalue; indT = table upvalue, indIdx = key's K index
	VINDEXI   // indexed variable with constant integer
	VINDEXSTR // indexed variable with literal string
	VJMP      // expression is a test/comparison; info = pc of corresponding jump
	VRELOC    // expression can put result in any register; info = instruction pc
	VCALL     // expression is a function call; info = instruction pc
	VVARARG   // vararg expression; info = instruction pc
)

func vkisvar(k expkind) bool     { return VLOCAL <= k && k <= VINDEXSTR }
func vkisindexed(k expkind) bool { return VINDEXED <= k && k <= VINDEXSTR }

// expdesc describes a potentially-delayed variable/expression. In C the
// payload is a union; here the overlapping fields are kept separate.
type expdesc struct {
	k       expkind
	ival    int64    // for VKINT
	nval    float64  // for VKFLT
	strval  *TString // for VKSTR
	info    int      // for generic use
	indT    int      // for indexed variables: table (register or upvalue)
	indIdx  int      // for indexed variables: key index
	varRidx int      // for local variables: register holding the variable
	varVidx int      // for local variables: compiler index (in actvar.arr)
	t       int      // patch list of 'exit when true'
	f       int      // patch list of 'exit when false'
}

// ---------------------------------------------------------------------
// Parser state (lparser.h)
// ---------------------------------------------------------------------

// Vardesc describes an active local variable.
type Vardesc struct {
	k    Value // constant value (if it is a compile-time constant)
	kind byte
	ridx int // register holding the variable
	pidx int // index of the variable in the Proto's 'locvars' array
	name *TString
}

// Labeldesc describes a pending goto or a label statement.
type Labeldesc struct {
	name    *TString
	pc      int
	line    int
	nactvar int
	close   bool // goto that escapes upvalues
}

// Labellist is a list of labels or gotos.
type Labellist struct {
	arr []Labeldesc
	n   int // number of entries in use
}

// Dyndata holds the dynamic structures used by the parser.
type Dyndata struct {
	actvar struct {
		arr []Vardesc
		n   int
	}
	gt    Labellist // list of pending gotos
	label Labellist // list of active labels
}

// BlockCnt is a node of the list of active blocks.
type BlockCnt struct {
	previous   *BlockCnt
	firstlabel int
	firstgoto  int
	nactvar    int
	upval      bool // true if some variable in the block is an upvalue
	isloop     bool
	insidetbc  bool // true if inside the scope of a to-be-closed variable
}

// FuncState is the state needed to generate code for a given function.
type FuncState struct {
	f            *Proto
	prev         *FuncState
	ls           *LexState
	h            *Table /* table to find (and reuse) elements in 'k' */
	bl           *BlockCnt
	pc           int
	lasttarget   int
	previousline int
	nk           int
	np           int
	firstlocal   int
	firstlabel   int
	ndebugvars   int
	nactvar      int
	nups         int
	freereg      int
	needclose    bool
}

// ---------------------------------------------------------------------
// error handling
// ---------------------------------------------------------------------

// enterlevel is luaE_incCstack.  Only the lower 16 bits of 'nCcalls'
// count C levels; the upper 16 bits only say "not yieldable", and must
// not make the parser think the stack overflowed.
func enterlevel(ls *LexState) {
	ls.L.nCcalls++
	if ls.L.nCcalls&0xffff >= LUAI_MAXCCALLS {
		luaX_syntaxerror(ls, "chunk has too many syntax levels")
	}
}

func leavelevel(ls *LexState) { ls.L.nCcalls-- }

func error_expected(ls *LexState, token int) {
	luaX_syntaxerror(ls, fmt.Sprintf("%s expected", luaX_token2str(ls, token)))
}

func errorlimit(fs *FuncState, limit int, what string) {
	line := fs.f.linedefined
	where := "main function"
	if line != 0 {
		where = fmt.Sprintf("function at line %d", line)
	}
	msg := fmt.Sprintf("too many %s (limit is %d) in %s", what, limit, where)
	luaX_syntaxerror(fs.ls, msg)
}

func checklimit(fs *FuncState, v, l int, what string) {
	if v > l {
		errorlimit(fs, l, what)
	}
}

// ---------------------------------------------------------------------
// token helpers
// ---------------------------------------------------------------------

func testnext(ls *LexState, c int) bool {
	if ls.t.token == c {
		luaX_next(ls)
		return true
	}
	return false
}

func check(ls *LexState, c int) {
	if ls.t.token != c {
		error_expected(ls, c)
	}
}

func checknext(ls *LexState, c int) {
	check(ls, c)
	luaX_next(ls)
}

func check_condition(ls *LexState, c bool, msg string) {
	if !c {
		luaX_syntaxerror(ls, msg)
	}
}

func check_match(ls *LexState, what, who, where int) {
	if testnext(ls, what) {
		return
	}
	if where == ls.linenumber { /* all in the same line? */
		error_expected(ls, what)
	}
	luaX_syntaxerror(ls, fmt.Sprintf("%s expected (to close %s at line %d)",
		luaX_token2str(ls, what), luaX_token2str(ls, who), where))
}

func str_checkname(ls *LexState) *TString {
	check(ls, TK_NAME)
	ts := ls.t.seminfo.ts
	luaX_next(ls)
	return ts
}

func init_exp(e *expdesc, k expkind, i int) {
	e.f = NO_JUMP
	e.t = NO_JUMP
	e.k = k
	e.info = i
}

func codestring(e *expdesc, s *TString) {
	e.f = NO_JUMP
	e.t = NO_JUMP
	e.k = VKSTR
	e.strval = s
}

func codename(ls *LexState, e *expdesc) {
	codestring(e, str_checkname(ls))
}

// ---------------------------------------------------------------------
// variables
// ---------------------------------------------------------------------

// registerlocalvar registers a new local variable for debug information.
func registerlocalvar(ls *LexState, fs *FuncState, varname *TString) int {
	fs.f.locvars = append(fs.f.locvars, LocVar{varname: varname, startpc: fs.pc})
	idx := fs.ndebugvars
	fs.ndebugvars++
	return idx
}

// new_localvar creates a new local variable, returning its compiler index.
func new_localvar(ls *LexState, name *TString) int {
	fs := ls.fs
	dyd := ls.dyd
	checklimit(fs, dyd.actvar.n+1-fs.firstlocal, MAXVARS, "local variables")
	for len(dyd.actvar.arr) <= dyd.actvar.n {
		dyd.actvar.arr = append(dyd.actvar.arr, Vardesc{})
	}
	v := &dyd.actvar.arr[dyd.actvar.n]
	v.kind = VDKREG /* default */
	v.name = name
	dyd.actvar.n++
	return dyd.actvar.n - 1 - fs.firstlocal
}

func new_localvarliteral(ls *LexState, v string) {
	new_localvar(ls, luaX_newstring(ls, v))
}

// getlocalvardesc returns the description of the variable 'vidx'.
func getlocalvardesc(fs *FuncState, vidx int) *Vardesc {
	return &fs.ls.dyd.actvar.arr[fs.firstlocal+vidx]
}

// reglevel converts a compiler index level to its corresponding register.
func reglevel(fs *FuncState, nvar int) int {
	for nvar--; nvar >= 0; nvar-- {
		vd := getlocalvardesc(fs, nvar) /* get previous variable */
		if vd.kind != RDKCTC {          /* is in a register? */
			return vd.ridx + 1
		}
	}
	return 0 /* no variables in registers */
}

// luaY_nvarstack returns the number of variables in the register stack.
func luaY_nvarstack(fs *FuncState) int {
	return reglevel(fs, fs.nactvar)
}

// localdebuginfo returns the debug-information entry of variable 'vidx'.
func localdebuginfo(fs *FuncState, vidx int) *LocVar {
	vd := getlocalvardesc(fs, vidx)
	if vd.kind == RDKCTC {
		return nil /* no debug info. for constants */
	}
	idx := vd.pidx
	return &fs.f.locvars[idx]
}

func init_var(fs *FuncState, e *expdesc, vidx int) {
	e.f = NO_JUMP
	e.t = NO_JUMP
	e.k = VLOCAL
	e.varVidx = vidx
	e.varRidx = getlocalvardesc(fs, vidx).ridx
}

// check_readonly raises an error if the variable described by 'e' is const.
func check_readonly(ls *LexState, e *expdesc) {
	fs := ls.fs
	var varname *TString /* to be set if variable is const */
	switch e.k {
	case VCONST:
		varname = ls.dyd.actvar.arr[e.info].name
	case VLOCAL:
		vardesc := getlocalvardesc(fs, e.varVidx)
		if vardesc.kind != VDKREG { /* not a regular variable? */
			varname = vardesc.name
		}
	case VUPVAL:
		up := &fs.f.upvalues[e.info]
		if up.kind != VDKREG {
			varname = up.name
		}
	default:
		return /* other cases cannot be read-only */
	}
	if varname != nil {
		msg := fmt.Sprintf("attempt to assign to const variable '%s'", getstr(varname))
		luaK_semerror(ls, msg)
	}
}

// adjustlocalvars starts the scope for the last 'nvars' created variables.
func adjustlocalvars(ls *LexState, nvars int) {
	fs := ls.fs
	reglevel := luaY_nvarstack(fs)
	for i := 0; i < nvars; i++ {
		vidx := fs.nactvar
		fs.nactvar++
		v := getlocalvardesc(fs, vidx)
		v.ridx = reglevel
		reglevel++
		v.pidx = registerlocalvar(ls, fs, v.name)
	}
}

// removevars closes the scope for all variables up to level 'tolevel'.
func removevars(fs *FuncState, tolevel int) {
	fs.ls.dyd.actvar.n -= (fs.nactvar - tolevel)
	for fs.nactvar > tolevel {
		fs.nactvar--
		if v := localdebuginfo(fs, fs.nactvar); v != nil {
			v.endpc = fs.pc
		}
	}
}

// ---------------------------------------------------------------------
// upvalues
// ---------------------------------------------------------------------

func searchupvalue(fs *FuncState, name *TString) int {
	for i := 0; i < fs.nups; i++ {
		if eqstr(fs.f.upvalues[i].name, name) {
			return i
		}
	}
	return -1 /* not found */
}

func allocupvalue(fs *FuncState) *Upvaldesc {
	checklimit(fs, fs.nups+1, MAXUPVAL, "upvalues")
	fs.f.upvalues = append(fs.f.upvalues, Upvaldesc{})
	fs.nups++
	return &fs.f.upvalues[fs.nups-1]
}

func newupvalue(fs *FuncState, name *TString, v *expdesc) int {
	up := allocupvalue(fs)
	prev := fs.prev
	if v.k == VLOCAL {
		up.instack = 1
		up.idx = byte(v.varRidx)
		up.kind = getlocalvardesc(prev, v.varVidx).kind
	} else {
		up.instack = 0
		up.idx = byte(v.info)
		up.kind = prev.f.upvalues[v.info].kind
	}
	up.name = name
	return fs.nups - 1
}

// searchvar looks for an active local variable with the given name.
func searchvar(fs *FuncState, n *TString, v *expdesc) int {
	for i := fs.nactvar - 1; i >= 0; i-- {
		vd := getlocalvardesc(fs, i)
		if eqstr(n, vd.name) { /* found? */
			if vd.kind == RDKCTC { /* compile-time constant? */
				init_exp(v, VCONST, fs.firstlocal+i)
			} else { /* real variable */
				init_var(fs, v, i)
			}
			return int(v.k)
		}
	}
	return -1 /* not found */
}

// markupval marks the block where the variable at 'level' was defined.
func markupval(fs *FuncState, level int) {
	bl := fs.bl
	for bl.nactvar > level {
		bl = bl.previous
	}
	bl.upval = true
	fs.needclose = true
}

// marktobeclosed marks that the current block has a to-be-closed variable.
func marktobeclosed(fs *FuncState) {
	bl := fs.bl
	bl.upval = true
	bl.insidetbc = true
	fs.needclose = true
}

func singlevaraux(fs *FuncState, n *TString, v *expdesc, base int) {
	if fs == nil { /* no more levels? */
		init_exp(v, VVOID, 0) /* default is global */
		return
	}
	if r := searchvar(fs, n, v); r >= 0 { /* found? */
		if r == int(VLOCAL) && base == 0 {
			markupval(fs, v.varVidx) /* local will be used as an upval */
		}
		return
	}
	/* not found as local at current level; try upvalues */
	idx := searchupvalue(fs, n) /* try existing upvalues */
	if idx < 0 {                /* not found? */
		singlevaraux(fs.prev, n, v, 0) /* try upper levels */
		if v.k != VLOCAL && v.k != VUPVAL {
			/* it is a global or a constant: nothing to do at this level */
			return
		}
		idx = newupvalue(fs, n, v) /* will be a new upvalue */
	}
	init_exp(v, VUPVAL, idx) /* new or old upvalue */
}

func singlevar(ls *LexState, v *expdesc) {
	varname := str_checkname(ls)
	fs := ls.fs
	singlevaraux(fs, varname, v, 1)
	if v.k == VVOID { /* global name? */
		var key expdesc
		singlevaraux(fs, ls.envn, v, 1) /* get environment variable */
		luaK_exp2anyregup(fs, v)        /* but could be a constant */
		codestring(&key, varname)       /* key is variable name */
		luaK_indexed(fs, v, &key)       /* env[varname] */
	}
}

// adjust_assign adjusts the number of results of an expression list.
func adjust_assign(ls *LexState, nvars, nexps int, e *expdesc) {
	fs := ls.fs
	needed := nvars - nexps /* extra values needed */
	if hasmultret(e.k) {    /* last expression has multiple returns? */
		extra := needed + 1 /* discount last expression itself */
		if extra < 0 {
			extra = 0
		}
		luaK_setreturns(fs, e, extra) /* last exp. provides the difference */
	} else {
		if e.k != VVOID { /* at least one expression? */
			luaK_exp2nextreg(fs, e) /* close last expression */
		}
		if needed > 0 { /* missing values? */
			luaK_nil(fs, fs.freereg, needed) /* complete with nils */
		}
	}
	if needed > 0 {
		luaK_reserveregs(fs, needed) /* registers for extra values */
	} else { /* adding 'needed' is actually a subtraction */
		fs.freereg += needed /* remove extra values */
	}
}

// ---------------------------------------------------------------------
// labels and gotos
// ---------------------------------------------------------------------

func jumpscopeerror(ls *LexState, gt *Labeldesc) {
	varname := getstr(getlocalvardesc(ls.fs, gt.nactvar).name)
	msg := fmt.Sprintf("<goto %s> at line %d jumps into the scope of local '%s'",
		getstr(gt.name), gt.line, varname)
	luaK_semerror(ls, msg)
}

func solvegoto(ls *LexState, g int, label *Labeldesc) {
	gl := &ls.dyd.gt /* list of gotos */
	gt := &gl.arr[g] /* goto to be resolved */
	if gt.nactvar < label.nactvar {
		jumpscopeerror(ls, gt)
	}
	luaK_patchlist(ls.fs, gt.pc, label.pc)
	for i := g; i < gl.n-1; i++ { /* remove goto from pending list */
		gl.arr[i] = gl.arr[i+1]
	}
	gl.n--
}

func findlabel(ls *LexState, name *TString) *Labeldesc {
	dyd := ls.dyd
	/* check labels in current function for a match */
	for i := ls.fs.firstlabel; i < dyd.label.n; i++ {
		lb := &dyd.label.arr[i]
		if eqstr(lb.name, name) { /* correct label? */
			return lb
		}
	}
	return nil /* label not found */
}

func newlabelentry(ls *LexState, l *Labellist, name *TString, line, pc int) int {
	n := l.n
	for len(l.arr) <= n {
		l.arr = append(l.arr, Labeldesc{})
	}
	l.arr[n] = Labeldesc{
		name:    name,
		line:    line,
		nactvar: ls.fs.nactvar,
		close:   false,
		pc:      pc,
	}
	l.n = n + 1
	return n
}

func newgotoentry(ls *LexState, name *TString, line, pc int) int {
	return newlabelentry(ls, &ls.dyd.gt, name, line, pc)
}

// solvegotos solves the forward jumps to the new label 'lb'.
func solvegotos(ls *LexState, lb *Labeldesc) bool {
	gl := &ls.dyd.gt
	i := ls.fs.bl.firstgoto
	needsclose := false
	for i < gl.n {
		if eqstr(gl.arr[i].name, lb.name) {
			if gl.arr[i].close {
				needsclose = true
			}
			solvegoto(ls, i, lb) /* will remove 'i' from the list */
		} else {
			i++
		}
	}
	return needsclose
}

// createlabel creates a new label, solving the pending gotos to it.
func createlabel(ls *LexState, name *TString, line int, last bool) bool {
	fs := ls.fs
	ll := &ls.dyd.label
	l := newlabelentry(ls, ll, name, line, luaK_getlabel(fs))
	if last { /* label is last no-op statement in the block? */
		/* assume that locals are already out of scope */
		ll.arr[l].nactvar = fs.bl.nactvar
	}
	if solvegotos(ls, &ll.arr[l]) { /* need close? */
		luaK_codeABC(fs, OP_CLOSE, luaY_nvarstack(fs), 0, 0)
		return true
	}
	return false
}

// movegotosout adjusts the pending gotos to the outer level of a block.
func movegotosout(fs *FuncState, bl *BlockCnt) {
	gl := &fs.ls.dyd.gt
	for i := bl.firstgoto; i < gl.n; i++ { /* for each pending goto */
		gt := &gl.arr[i]
		/* leaving a variable scope? */
		if reglevel(fs, gt.nactvar) > reglevel(fs, bl.nactvar) {
			if bl.upval {
				gt.close = true /* jump may need a close */
			}
		}
		gt.nactvar = bl.nactvar /* update goto level */
	}
}

func enterblock(fs *FuncState, bl *BlockCnt, isloop bool) {
	bl.isloop = isloop
	bl.nactvar = fs.nactvar
	bl.firstlabel = fs.ls.dyd.label.n
	bl.firstgoto = fs.ls.dyd.gt.n
	bl.upval = false
	bl.insidetbc = fs.bl != nil && fs.bl.insidetbc
	bl.previous = fs.bl
	fs.bl = bl
}

func undefgoto(ls *LexState, gt *Labeldesc) {
	var msg string
	if eqstr(gt.name, luaS_new(ls.L, "break")) {
		msg = fmt.Sprintf("break outside loop at line %d", gt.line)
	} else {
		msg = fmt.Sprintf("no visible label '%s' for <goto> at line %d",
			getstr(gt.name), gt.line)
	}
	luaK_semerror(ls, msg)
}

func leaveblock(fs *FuncState) {
	bl := fs.bl
	ls := fs.ls
	hasclose := false
	stklevel := reglevel(fs, bl.nactvar) /* level outside the block */
	removevars(fs, bl.nactvar)           /* remove block locals */
	if bl.isloop {                       /* has to fix pending breaks? */
		hasclose = createlabel(ls, luaS_new(ls.L, "break"), 0, false)
	}
	if !hasclose && bl.previous != nil && bl.upval { /* still need a 'close'? */
		luaK_codeABC(fs, OP_CLOSE, stklevel, 0, 0)
	}
	fs.freereg = stklevel          /* free registers */
	ls.dyd.label.n = bl.firstlabel /* remove local labels */
	fs.bl = bl.previous            /* current block now is previous one */
	if bl.previous != nil {        /* was it a nested block? */
		movegotosout(fs, bl) /* update pending gotos to enclosing block */
	} else if bl.firstgoto < ls.dyd.gt.n { /* still pending gotos? */
		undefgoto(ls, &ls.dyd.gt.arr[bl.firstgoto]) /* error */
	}
}

// ---------------------------------------------------------------------
// functions
// ---------------------------------------------------------------------

// addprototype adds a new prototype into the list of prototypes.
func addprototype(ls *LexState) *Proto {
	fs := ls.fs
	f := fs.f
	clp := luaF_newproto(ls.L)
	for len(f.p) <= fs.np {
		f.p = append(f.p, nil)
	}
	f.p[fs.np] = clp
	fs.np++
	return clp
}

// codeclosure codes the instruction to create a new closure.
func codeclosure(ls *LexState, v *expdesc) {
	fs := ls.fs.prev
	init_exp(v, VRELOC, luaK_codeABx(fs, OP_CLOSURE, 0, fs.np-1))
	luaK_exp2nextreg(fs, v) /* fix it at the last register */
}

func open_func(ls *LexState, fs *FuncState, bl *BlockCnt) {
	f := fs.f
	fs.prev = ls.fs /* linked list of funcstates */
	fs.ls = ls
	ls.fs = fs
	fs.pc = 0
	fs.previousline = f.linedefined
	fs.lasttarget = 0
	fs.freereg = 0
	fs.nk = 0
	fs.np = 0
	fs.nups = 0
	fs.ndebugvars = 0
	fs.nactvar = 0
	fs.needclose = false
	fs.firstlocal = ls.dyd.actvar.n
	fs.firstlabel = ls.dyd.label.n
	fs.bl = nil
	fs.h = luaH_new(ls.L) /* create table for constants */
	f.source = ls.source
	f.maxstacksize = 2 /* registers 0/1 are always valid */
	enterblock(fs, bl, false)
}

func close_func(ls *LexState) {
	fs := ls.fs
	f := fs.f
	luaK_ret(fs, luaY_nvarstack(fs), 0) /* final return */
	leaveblock(fs)
	luaK_finish(fs)
	f.code = f.code[:fs.pc]
	f.lineinfo = f.lineinfo[:fs.pc]
	f.k = f.k[:fs.nk]
	f.p = f.p[:fs.np]
	f.locvars = f.locvars[:fs.ndebugvars]
	f.upvalues = f.upvalues[:fs.nups]
	// (lparser.c keeps these sizes in the Proto via luaM_growvector; we
	// keep the Go slices and mirror the effective sizes here.)
	f.sizecode = len(f.code)
	f.sizek = len(f.k)
	f.sizeupvalues = len(f.upvalues)
	f.sizep = len(f.p)
	f.sizelocvars = len(f.locvars)
	f.sizelineinfo = len(f.lineinfo)
	ls.fs = fs.prev
}

// ---------------------------------------------------------------------
// GRAMMAR RULES
// ---------------------------------------------------------------------

// block_follow checks whether the current token is in the follow set of a
// block.
func block_follow(ls *LexState, withuntil bool) bool {
	switch ls.t.token {
	case TK_ELSE, TK_ELSEIF, TK_END, TK_EOS:
		return true
	case TK_UNTIL:
		return withuntil
	}
	return false
}

func statlist(ls *LexState) {
	/* statlist -> { stat [';'] } */
	for !block_follow(ls, true) {
		if ls.t.token == TK_RETURN {
			statement(ls)
			return /* 'return' must be last statement */
		}
		statement(ls)
	}
}

func fieldsel(ls *LexState, v *expdesc) {
	/* fieldsel -> ['.' | ':'] NAME */
	fs := ls.fs
	var key expdesc
	luaK_exp2anyregup(fs, v)
	luaX_next(ls) /* skip the dot or colon */
	codename(ls, &key)
	luaK_indexed(fs, v, &key)
}

func yindex(ls *LexState, v *expdesc) {
	/* index -> '[' expr ']' */
	luaX_next(ls) /* skip the '[' */
	expr(ls, v)
	luaK_exp2val(ls.fs, v)
	checknext(ls, ']')
}

// ---------------------------------------------------------------------
// constructors
// ---------------------------------------------------------------------

// ConsControl keeps the state of a constructor being parsed.
type ConsControl struct {
	v       expdesc  // last list item read
	t       *expdesc // table descriptor
	nh      int      // total number of 'record' elements
	na      int      // number of array elements already stored
	tostore int      // number of array elements pending to be stored
}

func recfield(ls *LexState, cc *ConsControl) {
	/* recfield -> (NAME | '['exp']') = exp */
	fs := ls.fs
	reg := fs.freereg
	var tab, key, val expdesc
	if ls.t.token == TK_NAME {
		codename(ls, &key)
	} else { /* ls->t.token == '[' */
		yindex(ls, &key)
	}
	checklimit(fs, cc.nh, maxInt, "items in a constructor")
	cc.nh++
	checknext(ls, '=')
	tab = *cc.t
	luaK_indexed(fs, &tab, &key)
	expr(ls, &val)
	luaK_storevar(fs, &tab, &val)
	fs.freereg = reg /* free registers */
}

func closelistfield(fs *FuncState, cc *ConsControl) {
	if cc.v.k == VVOID {
		return /* there is no list item */
	}
	luaK_exp2nextreg(fs, &cc.v)
	cc.v.k = VVOID
	if cc.tostore == LFIELDS_PER_FLUSH {
		luaK_setlist(fs, cc.t.info, cc.na, cc.tostore) /* flush */
		cc.na += cc.tostore
		cc.tostore = 0 /* no more items pending */
	}
}

func lastlistfield(fs *FuncState, cc *ConsControl) {
	if cc.tostore == 0 {
		return
	}
	if hasmultret(cc.v.k) {
		luaK_setmultret(fs, &cc.v)
		luaK_setlist(fs, cc.t.info, cc.na, LUA_MULTRET)
		cc.na-- /* do not count last expression (unknown number of elements) */
	} else {
		if cc.v.k != VVOID {
			luaK_exp2nextreg(fs, &cc.v)
		}
		luaK_setlist(fs, cc.t.info, cc.na, cc.tostore)
	}
	cc.na += cc.tostore
}

func listfield(ls *LexState, cc *ConsControl) {
	/* listfield -> exp */
	expr(ls, &cc.v)
	cc.tostore++
}

func field(ls *LexState, cc *ConsControl) {
	/* field -> listfield | recfield */
	switch ls.t.token {
	case TK_NAME: /* may be 'listfield' or 'recfield' */
		if luaX_lookahead(ls) != '=' { /* expression? */
			listfield(ls, cc)
		} else {
			recfield(ls, cc)
		}
	case '[':
		recfield(ls, cc)
	default:
		listfield(ls, cc)
	}
}

func constructor(ls *LexState, t *expdesc) {
	/* constructor -> '{' [ field { sep field } [sep] ] '}' */
	fs := ls.fs
	line := ls.linenumber
	pc := luaK_codeABC(fs, OP_NEWTABLE, 0, 0, 0)
	luaK_code(fs, Instruction{}) /* space for extra arg. */
	var cc ConsControl
	cc.na = 0
	cc.nh = 0
	cc.tostore = 0
	cc.t = t
	init_exp(t, VNONRELOC, fs.freereg) /* table will be at stack top */
	luaK_reserveregs(fs, 1)
	init_exp(&cc.v, VVOID, 0) /* no value (yet) */
	checknext(ls, '{')
	for {
		if ls.t.token == '}' {
			break
		}
		closelistfield(fs, &cc)
		field(ls, &cc)
		checklimit(fs, cc.tostore+cc.na+cc.nh, maxInt/2, "items in a constructor")
		if !(testnext(ls, ',') || testnext(ls, ';')) {
			break
		}
	}
	check_match(ls, '}', '{', line)
	lastlistfield(fs, &cc)
	luaK_settablesize(fs, pc, t.info, cc.na, cc.nh)
}

// ---------------------------------------------------------------------
// expressions
// ---------------------------------------------------------------------

func setvararg(fs *FuncState, nparams int) {
	fs.f.isVararg = 1
	luaK_codeABC(fs, OP_VARARGPREP, nparams, 0, 0)
}

func parlist(ls *LexState) {
	/* parlist -> [ {NAME ','} (NAME | '...') ] */
	fs := ls.fs
	f := fs.f
	nparams := 0
	isvararg := false
	if ls.t.token != ')' { /* is 'parlist' not empty? */
		for {
			switch ls.t.token {
			case TK_NAME:
				new_localvar(ls, str_checkname(ls))
				nparams++
			case TK_DOTS:
				luaX_next(ls)
				isvararg = true
			default:
				luaX_syntaxerror(ls, "<name> or '...' expected")
			}
			if isvararg || !testnext(ls, ',') {
				break
			}
		}
	}
	adjustlocalvars(ls, nparams)
	f.numparams = byte(fs.nactvar)
	if isvararg {
		setvararg(fs, int(f.numparams)) /* declared vararg */
	}
	luaK_reserveregs(fs, fs.nactvar) /* reserve registers for parameters */
}

func body(ls *LexState, e *expdesc, ismethod bool, line int) {
	/* body ->  '(' parlist ')' block END */
	var new_fs FuncState
	var bl BlockCnt
	new_fs.f = addprototype(ls)
	new_fs.f.linedefined = line
	open_func(ls, &new_fs, &bl)
	checknext(ls, '(')
	if ismethod {
		new_localvarliteral(ls, "self") /* create 'self' parameter */
		adjustlocalvars(ls, 1)
	}
	parlist(ls)
	checknext(ls, ')')
	statlist(ls)
	new_fs.f.lastlinedefined = ls.linenumber
	check_match(ls, TK_END, TK_FUNCTION, line)
	codeclosure(ls, e)
	close_func(ls)
}

func explist(ls *LexState, v *expdesc) int {
	/* explist -> expr { ',' expr } */
	n := 1 /* at least one expression */
	expr(ls, v)
	for testnext(ls, ',') {
		luaK_exp2nextreg(ls.fs, v)
		expr(ls, v)
		n++
	}
	return n
}

func funcargs(ls *LexState, f *expdesc) {
	fs := ls.fs
	var args expdesc
	line := ls.linenumber
	switch ls.t.token {
	case '(': /* funcargs -> '(' [ explist ] ')' */
		luaX_next(ls)
		if ls.t.token == ')' { /* arg list is empty? */
			args.k = VVOID
		} else {
			explist(ls, &args)
			if hasmultret(args.k) {
				luaK_setmultret(fs, &args)
			}
		}
		check_match(ls, ')', '(', line)
	case '{': /* funcargs -> constructor */
		constructor(ls, &args)
	case TK_STRING: /* funcargs -> STRING */
		codestring(&args, ls.t.seminfo.ts)
		luaX_next(ls) /* must use 'seminfo' before 'next' */
	default:
		luaX_syntaxerror(ls, "function arguments expected")
	}
	base := f.info /* base register for call */
	var nparams int
	if hasmultret(args.k) {
		nparams = LUA_MULTRET /* open call */
	} else {
		if args.k != VVOID {
			luaK_exp2nextreg(fs, &args) /* close last argument */
		}
		nparams = fs.freereg - (base + 1)
	}
	init_exp(f, VCALL, luaK_codeABC(fs, OP_CALL, base, nparams+1, 2))
	luaK_fixline(fs, line)
	/* call removes function and arguments and leaves one result */
	fs.freereg = base + 1
}

func primaryexp(ls *LexState, v *expdesc) {
	/* primaryexp -> NAME | '(' expr ')' */
	switch ls.t.token {
	case '(':
		line := ls.linenumber
		luaX_next(ls)
		expr(ls, v)
		check_match(ls, ')', '(', line)
		luaK_dischargevars(ls.fs, v)
	case TK_NAME:
		singlevar(ls, v)
	default:
		luaX_syntaxerror(ls, "unexpected symbol")
	}
}

func suffixedexp(ls *LexState, v *expdesc) {
	/* suffixedexp ->
	   primaryexp { '.' NAME | '[' exp ']' | ':' NAME funcargs | funcargs } */
	fs := ls.fs
	primaryexp(ls, v)
	for {
		switch ls.t.token {
		case '.': /* fieldsel */
			fieldsel(ls, v)
		case '[': /* '[' exp ']' */
			var key expdesc
			luaK_exp2anyregup(fs, v)
			yindex(ls, &key)
			luaK_indexed(fs, v, &key)
		case ':': /* ':' NAME funcargs */
			var key expdesc
			luaX_next(ls)
			codename(ls, &key)
			luaK_self(fs, v, &key)
			funcargs(ls, v)
		case '(', TK_STRING, '{': /* funcargs */
			luaK_exp2nextreg(fs, v)
			funcargs(ls, v)
		default:
			return
		}
	}
}

func simpleexp(ls *LexState, v *expdesc) {
	/* simpleexp -> FLT | INT | STRING | NIL | TRUE | FALSE | ... |
	   constructor | FUNCTION body | suffixedexp */
	switch ls.t.token {
	case TK_FLT:
		init_exp(v, VKFLT, 0)
		v.nval = ls.t.seminfo.r
	case TK_INT:
		init_exp(v, VKINT, 0)
		v.ival = ls.t.seminfo.i
	case TK_STRING:
		codestring(v, ls.t.seminfo.ts)
	case TK_NIL:
		init_exp(v, VNIL, 0)
	case TK_TRUE:
		init_exp(v, VTRUE, 0)
	case TK_FALSE:
		init_exp(v, VFALSE, 0)
	case TK_DOTS: /* vararg */
		fs := ls.fs
		check_condition(ls, fs.f.isVararg != 0,
			"cannot use '...' outside a vararg function")
		init_exp(v, VVARARG, luaK_codeABC(fs, OP_VARARG, 0, 0, 1))
	case '{': /* constructor */
		constructor(ls, v)
		return
	case TK_FUNCTION:
		luaX_next(ls)
		body(ls, v, false, ls.linenumber)
		return
	default:
		suffixedexp(ls, v)
		return
	}
	luaX_next(ls)
}

func getunopr(op int) UnOpr {
	switch op {
	case TK_NOT:
		return OPR_NOT
	case '-':
		return OPR_MINUS
	case '~':
		return OPR_BNOT
	case '#':
		return OPR_LEN
	}
	return OPR_NOUNOPR
}

func getbinopr(op int) BinOpr {
	switch op {
	case '+':
		return OPR_ADD
	case '-':
		return OPR_SUB
	case '*':
		return OPR_MUL
	case '%':
		return OPR_MOD
	case '^':
		return OPR_POW
	case '/':
		return OPR_DIV
	case TK_IDIV:
		return OPR_IDIV
	case '&':
		return OPR_BAND
	case '|':
		return OPR_BOR
	case '~':
		return OPR_BXOR
	case TK_SHL:
		return OPR_SHL
	case TK_SHR:
		return OPR_SHR
	case TK_CONCAT:
		return OPR_CONCAT
	case TK_NE:
		return OPR_NE
	case TK_EQ:
		return OPR_EQ
	case '<':
		return OPR_LT
	case TK_LE:
		return OPR_LE
	case '>':
		return OPR_GT
	case TK_GE:
		return OPR_GE
	case TK_AND:
		return OPR_AND
	case TK_OR:
		return OPR_OR
	}
	return OPR_NOBINOPR
}

// priority is the priority table for binary operators (ORDER OPR).
var priority = [OPR_NOBINOPR][2]int{
	{10, 10}, {10, 10}, /* '+' '-' */
	{11, 11}, {11, 11}, /* '*' '%' */
	{14, 13},           /* '^' (right associative) */
	{11, 11}, {11, 11}, /* '/' '//' */
	{6, 6}, {4, 4}, {5, 5}, /* '&' '|' '~' */
	{7, 7}, {7, 7}, /* '<<' '>>' */
	{9, 8},                 /* '..' (right associative) */
	{3, 3}, {3, 3}, {3, 3}, /* ==, <, <= */
	{3, 3}, {3, 3}, {3, 3}, /* ~=, >, >= */
	{2, 2}, {1, 1}, /* and, or */
}

func subexpr(ls *LexState, v *expdesc, limit int) BinOpr {
	enterlevel(ls)
	uop := getunopr(ls.t.token)
	if uop != OPR_NOUNOPR { /* prefix (unary) operator? */
		line := ls.linenumber
		luaX_next(ls) /* skip operator */
		subexpr(ls, v, UNARY_PRIORITY)
		luaK_prefix(ls.fs, uop, v, line)
	} else {
		simpleexp(ls, v)
	}
	/* expand while operators have priorities higher than 'limit' */
	op := getbinopr(ls.t.token)
	for op != OPR_NOBINOPR && priority[op][0] > limit {
		var v2 expdesc
		line := ls.linenumber
		luaX_next(ls) /* skip operator */
		luaK_infix(ls.fs, op, v)
		/* read sub-expression with higher priority */
		nextop := subexpr(ls, &v2, priority[op][1])
		luaK_posfix(ls.fs, op, v, &v2, line)
		op = nextop
	}
	leavelevel(ls)
	return op /* return first untreated operator */
}

func expr(ls *LexState, v *expdesc) {
	subexpr(ls, v, 0)
}

// ---------------------------------------------------------------------
// statements
// ---------------------------------------------------------------------

func block(ls *LexState) {
	/* block -> statlist */
	fs := ls.fs
	var bl BlockCnt
	enterblock(fs, &bl, false)
	statlist(ls)
	leaveblock(fs)
}

// LHS_assign chains the variables in the left-hand side of an assignment.
type LHS_assign struct {
	prev *LHS_assign
	v    expdesc /* variable (global, local, upvalue, or indexed) */
}

// check_conflict saves the value of an upvalue/local that is also used by a
// previous assignment to a table.
func check_conflict(ls *LexState, lh *LHS_assign, v *expdesc) {
	fs := ls.fs
	extra := fs.freereg /* eventual position to save local variable */
	conflict := false
	for ; lh != nil; lh = lh.prev { /* check all previous assignments */
		if vkisindexed(lh.v.k) { /* assignment to table field? */
			if lh.v.k == VINDEXUP { /* is table an upvalue? */
				if v.k == VUPVAL && lh.v.indT == v.info {
					conflict = true /* table is the upvalue being assigned now */
					lh.v.k = VINDEXSTR
					lh.v.indT = extra /* assignment will use safe copy */
				}
			} else { /* table is a register */
				if v.k == VLOCAL && lh.v.indT == v.varRidx {
					conflict = true /* table is the local being assigned now */
					lh.v.indT = extra
				}
				/* is index the local being assigned? */
				if lh.v.k == VINDEXED && v.k == VLOCAL &&
					lh.v.indIdx == v.varRidx {
					conflict = true
					lh.v.indIdx = extra /* previous assignment will use safe copy */
				}
			}
		}
	}
	if conflict {
		/* copy upvalue/local value to a temporary (in position 'extra') */
		if v.k == VLOCAL {
			luaK_codeABC(fs, OP_MOVE, extra, v.varRidx, 0)
		} else {
			luaK_codeABC(fs, OP_GETUPVAL, extra, v.info, 0)
		}
		luaK_reserveregs(fs, 1)
	}
}

func restassign(ls *LexState, lh *LHS_assign, nvars int) {
	var e expdesc
	check_condition(ls, vkisvar(lh.v.k), "syntax error")
	check_readonly(ls, &lh.v)
	if testnext(ls, ',') { /* restassign -> ',' suffixedexp restassign */
		nv := &LHS_assign{prev: lh}
		suffixedexp(ls, &nv.v)
		if !vkisindexed(nv.v.k) {
			check_conflict(ls, lh, &nv.v)
		}
		enterlevel(ls) /* control recursion depth */
		restassign(ls, nv, nvars+1)
		leavelevel(ls)
	} else { /* restassign -> '=' explist */
		checknext(ls, '=')
		nexps := explist(ls, &e)
		if nexps != nvars {
			adjust_assign(ls, nvars, nexps, &e)
		} else {
			luaK_setoneret(ls.fs, &e) /* close last expression */
			luaK_storevar(ls.fs, &lh.v, &e)
			return /* avoid default */
		}
	}
	init_exp(&e, VNONRELOC, ls.fs.freereg-1) /* default assignment */
	luaK_storevar(ls.fs, &lh.v, &e)
}

func cond(ls *LexState) int {
	/* cond -> exp */
	var v expdesc
	expr(ls, &v) /* read condition */
	if v.k == VNIL {
		v.k = VFALSE /* 'falses' are all equal here */
	}
	luaK_goiftrue(ls.fs, &v)
	return v.f
}

func gotostat(ls *LexState) {
	fs := ls.fs
	line := ls.linenumber
	name := str_checkname(ls) /* label's name */
	if lb := findlabel(ls, name); lb == nil {
		/* forward jump; will be resolved when the label is declared */
		newgotoentry(ls, name, line, luaK_jump(fs))
	} else { /* found a label */
		/* backward jump; will be resolved here */
		lblevel := reglevel(fs, lb.nactvar) /* label level */
		if luaY_nvarstack(fs) > lblevel {   /* leaving the scope of a variable? */
			luaK_codeABC(fs, OP_CLOSE, lblevel, 0, 0)
		}
		/* create jump and link it to the label */
		luaK_patchlist(fs, luaK_jump(fs), lb.pc)
	}
}

// breakstat is semantically equivalent to "goto break".
func breakstat(ls *LexState) {
	line := ls.linenumber
	luaX_next(ls) /* skip break */
	newgotoentry(ls, luaS_new(ls.L, "break"), line, luaK_jump(ls.fs))
}

func checkrepeated(ls *LexState, name *TString) {
	if lb := findlabel(ls, name); lb != nil { /* already defined? */
		msg := fmt.Sprintf("label '%s' already defined on line %d",
			getstr(name), lb.line)
		luaK_semerror(ls, msg)
	}
}

func labelstat(ls *LexState, name *TString, line int) {
	/* label -> '::' NAME '::' */
	checknext(ls, TK_DBCOLON) /* skip double colon */
	for ls.t.token == ';' || ls.t.token == TK_DBCOLON {
		statement(ls) /* skip other no-op statements */
	}
	checkrepeated(ls, name) /* check for repeated labels */
	createlabel(ls, name, line, block_follow(ls, false))
}

func whilestat(ls *LexState, line int) {
	/* whilestat -> WHILE cond DO block END */
	fs := ls.fs
	luaX_next(ls) /* skip WHILE */
	whileinit := luaK_getlabel(fs)
	condexit := cond(ls)
	var bl BlockCnt
	enterblock(fs, &bl, true)
	checknext(ls, TK_DO)
	block(ls)
	luaK_jumpto(fs, whileinit)
	check_match(ls, TK_END, TK_WHILE, line)
	leaveblock(fs)
	luaK_patchtohere(fs, condexit) /* false conditions finish the loop */
}

func repeatstat(ls *LexState, line int) {
	/* repeatstat -> REPEAT block UNTIL cond */
	fs := ls.fs
	repeat_init := luaK_getlabel(fs)
	var bl1, bl2 BlockCnt
	enterblock(fs, &bl1, true)  /* loop block */
	enterblock(fs, &bl2, false) /* scope block */
	luaX_next(ls)               /* skip REPEAT */
	statlist(ls)
	check_match(ls, TK_UNTIL, TK_REPEAT, line)
	condexit := cond(ls) /* read condition (inside scope block) */
	leaveblock(fs)       /* finish scope */
	if bl2.upval {       /* upvalues? */
		exit := luaK_jump(fs)          /* normal exit must jump over fix */
		luaK_patchtohere(fs, condexit) /* repetition must close upvalues */
		luaK_codeABC(fs, OP_CLOSE, reglevel(fs, bl2.nactvar), 0, 0)
		condexit = luaK_jump(fs)   /* repeat after closing upvalues */
		luaK_patchtohere(fs, exit) /* normal exit comes to here */
	}
	luaK_patchlist(fs, condexit, repeat_init) /* close the loop */
	leaveblock(fs)                            /* finish loop */
}

// exp1 reads an expression, putting its results in the next stack slot.
func exp1(ls *LexState) {
	var e expdesc
	expr(ls, &e)
	luaK_exp2nextreg(ls.fs, &e)
}

// fixforjump fixes the instruction at 'pc' to jump to 'dest'.
func fixforjump(fs *FuncState, pc, dest int, back bool) {
	offset := dest - (pc + 1)
	if back {
		offset = -offset
	}
	if offset > MAXARG_Bx {
		luaX_syntaxerror(fs.ls, "control structure too long")
	}
	fs.f.code[pc].B = offset
}

func forbody(ls *LexState, base, line, nvars, isgen int) {
	/* forbody -> DO block */
	forprep := [2]int{OP_FORPREP, OP_TFORPREP}
	forloop := [2]int{OP_FORLOOP, OP_TFORLOOP}
	var bl BlockCnt
	fs := ls.fs
	checknext(ls, TK_DO)
	prep := luaK_codeABx(fs, forprep[isgen], base, 0)
	enterblock(fs, &bl, false) /* scope for declared variables */
	adjustlocalvars(ls, nvars)
	luaK_reserveregs(fs, nvars)
	block(ls)
	leaveblock(fs) /* end of scope for declared variables */
	fixforjump(fs, prep, luaK_getlabel(fs), false)
	if isgen != 0 { /* generic for? */
		luaK_codeABC(fs, OP_TFORCALL, base, 0, nvars)
		luaK_fixline(fs, line)
	}
	endfor := luaK_codeABx(fs, forloop[isgen], base, 0)
	fixforjump(fs, endfor, prep+1, true)
	luaK_fixline(fs, line)
}

func fornum(ls *LexState, varname *TString, line int) {
	/* fornum -> NAME = exp,exp[,exp] forbody */
	fs := ls.fs
	base := fs.freereg
	new_localvarliteral(ls, "(for state)")
	new_localvarliteral(ls, "(for state)")
	new_localvarliteral(ls, "(for state)")
	new_localvar(ls, varname)
	checknext(ls, '=')
	exp1(ls) /* initial value */
	checknext(ls, ',')
	exp1(ls) /* limit */
	if testnext(ls, ',') {
		exp1(ls) /* optional step */
	} else { /* default step = 1 */
		luaK_int(fs, fs.freereg, 1)
		luaK_reserveregs(fs, 1)
	}
	adjustlocalvars(ls, 3) /* control variables */
	forbody(ls, base, line, 1, 0)
}

func forlist(ls *LexState, indexname *TString) {
	/* forlist -> NAME {,NAME} IN explist forbody */
	fs := ls.fs
	var e expdesc
	nvars := 5 /* gen, state, control, toclose, 'indexname' */
	base := fs.freereg
	/* create control variables */
	new_localvarliteral(ls, "(for state)")
	new_localvarliteral(ls, "(for state)")
	new_localvarliteral(ls, "(for state)")
	new_localvarliteral(ls, "(for state)")
	/* create declared variables */
	new_localvar(ls, indexname)
	for testnext(ls, ',') {
		new_localvar(ls, str_checkname(ls))
		nvars++
	}
	checknext(ls, TK_IN)
	line := ls.linenumber
	adjust_assign(ls, 4, explist(ls, &e), &e)
	adjustlocalvars(ls, 4) /* control variables */
	marktobeclosed(fs)     /* last control var. must be closed */
	luaK_checkstack(fs, 3) /* extra space to call generator */
	forbody(ls, base, line, nvars-4, 1)
}

func forstat(ls *LexState, line int) {
	/* forstat -> FOR (fornum | forlist) END */
	fs := ls.fs
	var bl BlockCnt
	enterblock(fs, &bl, true)    /* scope for loop and control variables */
	luaX_next(ls)                /* skip 'for' */
	varname := str_checkname(ls) /* first variable name */
	switch ls.t.token {
	case '=':
		fornum(ls, varname, line)
	case ',', TK_IN:
		forlist(ls, varname)
	default:
		luaX_syntaxerror(ls, "'=' or 'in' expected")
	}
	check_match(ls, TK_END, TK_FOR, line)
	leaveblock(fs) /* loop scope ('break' jumps to this point) */
}

func test_then_block(ls *LexState, escapelist *int) {
	/* test_then_block -> [IF | ELSEIF] cond THEN block */
	var bl BlockCnt
	fs := ls.fs
	var v expdesc
	luaX_next(ls) /* skip IF or ELSEIF */
	expr(ls, &v)  /* read condition */
	checknext(ls, TK_THEN)
	var jf int                  /* instruction to skip 'then' code (if condition is false) */
	if ls.t.token == TK_BREAK { /* 'if x then break' ? */
		line := ls.linenumber
		luaK_goiffalse(ls.fs, &v)  /* will jump if condition is true */
		luaX_next(ls)              /* skip 'break' */
		enterblock(fs, &bl, false) /* must enter block before 'goto' */
		newgotoentry(ls, luaS_new(ls.L, "break"), line, v.t)
		for testnext(ls, ';') {
		} /* skip semicolons */
		if block_follow(ls, false) { /* jump is the entire block? */
			leaveblock(fs)
			return /* and that is it */
		}
		/* else must skip over 'then' part if condition is false */
		jf = luaK_jump(fs)
	} else { /* regular case (not a break) */
		luaK_goiftrue(ls.fs, &v) /* skip over block if condition is false */
		enterblock(fs, &bl, false)
		jf = v.f
	}
	statlist(ls) /* 'then' part */
	leaveblock(fs)
	if ls.t.token == TK_ELSE || ls.t.token == TK_ELSEIF {
		/* followed by 'else'/'elseif'? */
		luaK_concat(fs, escapelist, luaK_jump(fs)) /* must jump over it */
	}
	luaK_patchtohere(fs, jf)
}

func ifstat(ls *LexState, line int) {
	/* ifstat -> IF cond THEN block {ELSEIF cond THEN block} [ELSE block] END */
	fs := ls.fs
	escapelist := NO_JUMP            /* exit list for finished parts */
	test_then_block(ls, &escapelist) /* IF cond THEN block */
	for ls.t.token == TK_ELSEIF {
		test_then_block(ls, &escapelist) /* ELSEIF cond THEN block */
	}
	if testnext(ls, TK_ELSE) {
		block(ls) /* 'else' part */
	}
	check_match(ls, TK_END, TK_IF, line)
	luaK_patchtohere(fs, escapelist) /* patch escape list to 'if' end */
}

func localfunc(ls *LexState) {
	var b expdesc
	fs := ls.fs
	fvar := fs.nactvar /* function's variable index */
	new_localvar(ls, str_checkname(ls))
	adjustlocalvars(ls, 1)             /* enter its scope */
	body(ls, &b, false, ls.linenumber) /* function created in next register */
	/* debug information will only see the variable after this point! */
	if v := localdebuginfo(fs, fvar); v != nil {
		v.startpc = fs.pc
	}
}

// getlocalattribute parses an optional attribute ('<const>' or '<close>').
func getlocalattribute(ls *LexState) int {
	/* ATTRIB -> ['<' Name '>'] */
	if testnext(ls, '<') {
		attr := getstr(str_checkname(ls))
		checknext(ls, '>')
		switch attr {
		case "const":
			return RDKCONST /* read-only variable */
		case "close":
			return RDKTOCLOSE /* to-be-closed variable */
		}
		luaK_semerror(ls, fmt.Sprintf("unknown attribute '%s'", attr))
	}
	return VDKREG /* regular variable */
}

func checktoclose(fs *FuncState, level int) {
	if level != -1 { /* is there a to-be-closed variable? */
		marktobeclosed(fs)
		luaK_codeABC(fs, OP_TBC, reglevel(fs, level), 0, 0)
	}
}

func localstat(ls *LexState) {
	/* stat -> LOCAL NAME ATTRIB { ',' NAME ATTRIB } ['=' explist] */
	fs := ls.fs
	toclose := -1 /* index of to-be-closed variable (if any) */
	nvars := 0
	var e expdesc
	var vidx, kind int
	for {
		vidx = new_localvar(ls, str_checkname(ls))
		kind = getlocalattribute(ls)
		getlocalvardesc(fs, vidx).kind = byte(kind)
		if kind == RDKTOCLOSE { /* to-be-closed? */
			if toclose != -1 { /* one already present? */
				luaK_semerror(ls, "multiple to-be-closed variables in local list")
			}
			toclose = fs.nactvar + nvars
		}
		nvars++
		if !testnext(ls, ',') {
			break
		}
	}
	var nexps int
	if testnext(ls, '=') {
		nexps = explist(ls, &e)
	} else {
		e.k = VVOID
		nexps = 0
	}
	v := getlocalvardesc(fs, vidx) /* get last variable */
	if nvars == nexps &&           /* no adjustments? */
		v.kind == RDKCONST && /* last variable is const? */
		luaK_exp2const(fs, &e, &v.k) { /* compile-time constant? */
		v.kind = RDKCTC              /* variable is a compile-time constant */
		adjustlocalvars(ls, nvars-1) /* exclude last variable */
		fs.nactvar++                 /* but count it */
	} else {
		adjust_assign(ls, nvars, nexps, &e)
		adjustlocalvars(ls, nvars)
	}
	checktoclose(fs, toclose)
}

func funcname(ls *LexState, v *expdesc) int {
	/* funcname -> NAME {fieldsel} [':' NAME] */
	ismethod := 0
	singlevar(ls, v)
	for ls.t.token == '.' {
		fieldsel(ls, v)
	}
	if ls.t.token == ':' {
		ismethod = 1
		fieldsel(ls, v)
	}
	return ismethod
}

func funcstat(ls *LexState, line int) {
	/* funcstat -> FUNCTION funcname body */
	var v, b expdesc
	luaX_next(ls) /* skip FUNCTION */
	ismethod := funcname(ls, &v)
	body(ls, &b, ismethod != 0, line)
	check_readonly(ls, &v)
	luaK_storevar(ls.fs, &v, &b)
	luaK_fixline(ls.fs, line) /* definition "happens" in the first line */
}

func exprstat(ls *LexState) {
	/* stat -> func | assignment */
	fs := ls.fs
	var v LHS_assign
	suffixedexp(ls, &v.v)
	if ls.t.token == '=' || ls.t.token == ',' { /* stat -> assignment ? */
		v.prev = nil
		restassign(ls, &v, 1)
	} else { /* stat -> func */
		check_condition(ls, v.v.k == VCALL, "syntax error")
		fs.f.code[v.v.info].C = 1 /* call statement uses no results */
	}
}

func retstat(ls *LexState) {
	/* stat -> RETURN [explist] [';'] */
	fs := ls.fs
	var e expdesc
	nret := 0                   /* number of values being returned */
	first := luaY_nvarstack(fs) /* first slot to be returned */
	if block_follow(ls, true) || ls.t.token == ';' {
		nret = 0 /* return no values */
	} else {
		nret = explist(ls, &e) /* optional return values */
		if hasmultret(e.k) {
			luaK_setmultret(fs, &e)
			if e.k == VCALL && nret == 1 && !fs.bl.insidetbc { /* tail call? */
				fs.f.code[e.info].Op = OP_TAILCALL
			}
			nret = LUA_MULTRET /* return all values */
		} else {
			if nret == 1 { /* only one single value? */
				first = luaK_exp2anyreg(fs, &e) /* can use original slot */
			} else { /* values must go to the top of the stack */
				luaK_exp2nextreg(fs, &e)
			}
		}
	}
	luaK_ret(fs, first, nret)
	testnext(ls, ';') /* skip optional semicolon */
}

func statement(ls *LexState) {
	line := ls.linenumber /* may be needed for error messages */
	enterlevel(ls)
	switch ls.t.token {
	case ';': /* stat -> ';' (empty statement) */
		luaX_next(ls) /* skip ';' */
	case TK_IF:
		ifstat(ls, line)
	case TK_WHILE:
		whilestat(ls, line)
	case TK_DO: /* stat -> DO block END */
		luaX_next(ls) /* skip DO */
		block(ls)
		check_match(ls, TK_END, TK_DO, line)
	case TK_FOR:
		forstat(ls, line)
	case TK_REPEAT:
		repeatstat(ls, line)
	case TK_FUNCTION:
		funcstat(ls, line)
	case TK_LOCAL:
		luaX_next(ls)                  /* skip LOCAL */
		if testnext(ls, TK_FUNCTION) { /* local function? */
			localfunc(ls)
		} else {
			localstat(ls)
		}
	case TK_DBCOLON: /* stat -> label */
		luaX_next(ls) /* skip double colon */
		labelstat(ls, str_checkname(ls), line)
	case TK_RETURN:
		luaX_next(ls) /* skip RETURN */
		retstat(ls)
	case TK_BREAK:
		breakstat(ls)
	case TK_GOTO: /* stat -> 'goto' NAME */
		luaX_next(ls) /* skip 'goto' */
		gotostat(ls)
	default: /* stat -> func | assignment */
		exprstat(ls)
	}
	ls.fs.freereg = luaY_nvarstack(ls.fs) /* free registers */
	leavelevel(ls)
}

// ---------------------------------------------------------------------
// main function
// ---------------------------------------------------------------------

// mainfunc compiles the main function, a regular vararg function with an
// upvalue named LUA_ENV.
func mainfunc(ls *LexState, fs *FuncState) {
	var bl BlockCnt
	open_func(ls, fs, &bl)
	setvararg(fs, 0) /* main function is always declared vararg */
	env := allocupvalue(fs)
	env.instack = 1
	env.idx = 0
	env.kind = VDKREG
	env.name = ls.envn
	luaX_next(ls) /* read first token */
	statlist(ls)  /* parse main body */
	check(ls, TK_EOS)
	close_func(ls)
}

// luaY_parser compiles a chunk, returning the closure for its main function.
func luaY_parser(L *State, src []byte, dyd *Dyndata, name string) *LClosure {
	var lexstate LexState
	var funcstate FuncState
	cl := luaF_newLclosure(L, 1) /* create main closure */
	lexstate.h = luaH_new(L)     /* create table for scanner */
	funcstate.f = luaF_newproto(L)
	cl.p = funcstate.f
	funcstate.f.source = luaS_new(L, name)
	lexstate.dyd = dyd
	dyd.actvar.n = 0
	dyd.gt.n = 0
	dyd.label.n = 0
	firstchar := EOZ
	if len(src) > 0 {
		firstchar = int(src[0])
		src = src[1:]
	}
	luaX_setinput(L, &lexstate, src, funcstate.f.source, firstchar)
	mainfunc(&lexstate, &funcstate)
	luaF_initupvals(L, cl)
	return cl
}
