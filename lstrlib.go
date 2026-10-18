// Translation of lstrlib.c (PUC-Lua 5.4.9).
//
// It contains the string library: the basic string functions, the
// complete Lua pattern-matching engine, string.format, and the
// pack/unpack family.
//
// Naming: every identifier introduced here is prefixed with 'str' (or
// 'lStr') to avoid clashes with the other files of the package.  The
// control flow follows the C source closely; the C pointer arithmetic
// on 'const char *' becomes integer offsets into a Go string.
package lua

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------
// basic string functions
// ---------------------------------------------------------------------

func strLen(L *State) int {
	s := L.CheckString(1)
	L.PushInteger(int64(len(s)))
	return 1
}

// strPosRelatI translates a relative initial string position
// (negative means back from the end); the result is clipped to [1, inf)
// (posrelatI).
func strPosRelatI(pos int64, l int) int {
	if pos > 0 {
		return int(pos)
	} else if pos == 0 {
		return 1
	} else if pos < -int64(l) { /* inverted comparison */
		return 1 /* clip to 1 */
	}
	return l + int(pos) + 1
}

// strGetEndPos gets an optional ending string position from argument
// 'arg', with default value 'def' (getendpos).
func strGetEndPos(L *State, arg int, def int64, l int) int {
	pos := L.OptInteger(arg, def)
	if pos > int64(l) {
		return l
	} else if pos >= 0 {
		return int(pos)
	} else if pos < -int64(l) {
		return 0
	}
	return l + int(pos) + 1
}

func strSub(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	start := strPosRelatI(L.CheckInteger(2), l)
	end := strGetEndPos(L, 3, -1, l)
	if start <= end {
		L.PushString(s[start-1 : end])
	} else {
		L.PushString("")
	}
	return 1
}

func strReverse(L *State) int {
	s := L.CheckString(1)
	n := len(s)
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = s[n-i-1]
	}
	L.PushString(string(b))
	return 1
}

// strASCIILower/strASCIIUpper mimic the C library 'tolower'/'toupper'
// in the "C" locale: only ASCII letters are converted.
func strASCIILower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

func strASCIIUpper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - ('a' - 'A')
	}
	return c
}

func strLower(L *State) int {
	s := L.CheckString(1)
	b := []byte(s)
	for i := range b {
		b[i] = strASCIILower(b[i])
	}
	L.PushString(string(b))
	return 1
}

func strUpper(L *State) int {
	s := L.CheckString(1)
	b := []byte(s)
	for i := range b {
		b[i] = strASCIIUpper(b[i])
	}
	L.PushString(string(b))
	return 1
}

func strRep(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	n := L.CheckInteger(2)
	sep := L.OptString(3, "")
	lsep := len(sep)
	if n <= 0 {
		L.PushString("")
	} else if l+lsep > math.MaxInt32/int(n) { /* may overflow? */
		return L.Error2("resulting string too large")
	} else {
		var sb strings.Builder
		for n > 1 { /* first n-1 copies (followed by separator) */
			sb.WriteString(s)
			if lsep > 0 { /* empty copy is not that cheap */
				sb.WriteString(sep)
			}
			n--
		}
		sb.WriteString(s) /* last copy (not followed by separator) */
		L.PushString(sb.String())
	}
	return 1
}

func strByte(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	pi := L.OptInteger(2, 1)
	posi := strPosRelatI(pi, l)
	pose := strGetEndPos(L, 3, pi, l)
	if posi > pose { /* empty interval; return no values */
		return 0
	}
	if pose-posi >= math.MaxInt32 { /* arithmetic overflow? */
		return L.Error2("string slice too long")
	}
	n := pose - posi + 1
	L.CheckStack2(n, "string slice too long")
	for i := 0; i < n; i++ {
		L.PushInteger(int64(s[posi+i-1]))
	}
	return n
}

func strChar(L *State) int {
	n := L.GetTop() /* number of arguments */
	b := make([]byte, 0, n)
	for i := 1; i <= n; i++ {
		c := uint64(L.CheckInteger(i))
		L.ArgCheck(c <= 255, i, "value out of range")
		b = append(b, byte(c))
	}
	L.PushString(string(b))
	return 1
}

// strDump dumps a function as a precompiled chunk (string.dump,
// lstrlib.c). The 'writer' of the C code is a closure appending to a Go
// buffer; 'lua_dump' returns non-zero when the value is not a Lua
// function.
func strDump(L *State) int {
	strip := L.ToBoolean(2)
	L.CheckType(1, LUA_TFUNCTION)
	L.SetTop(1) /* ensure function is on the top of the stack */
	var sb strings.Builder
	status := luaDump(L, func(_ *State, b []byte) int {
		sb.Write(b)
		return 0
	}, strip)
	if status != 0 {
		return L.Error2("unable to dump given function")
	}
	L.PushString(sb.String())
	return 1
}

// ---------------------------------------------------------------------
// METAMETHODS (arithmetic on strings)
// ---------------------------------------------------------------------

// strStringToNumber converts the whole string 's' into a number, pushes
// it, and reports success.
//
// This is 'lua_stringtonumber' (lapi.c), which is exactly 'luaO_str2num':
// there is only one numeral parser in Lua, and it must be the one that
// also serves the arithmetic coercions. Keeping a private copy here used
// to make 'tonumber("0xAA.0")' fail while '"0xAA.0" + 0' worked.
func strStringToNumber(L *State, s string) bool {
	var v Value
	if !luaO_str2num(s, &v) {
		return false
	}
	if ttisinteger(&v) {
		L.PushInteger(ivalue(&v))
	} else {
		L.PushNumber(fltvalue(&v))
	}
	return true
}

// strTonum pushes the value of argument 'arg' as a number, if possible
// (tonum).
func strTonum(L *State, arg int) bool {
	if L.Type(arg) == LUA_TNUMBER { /* already a number? */
		L.PushValue(arg)
		return true
	}
	/* else check whether it is a numerical string */
	if s, ok := L.ToString(arg); ok {
		return strStringToNumber(L, s)
	}
	return false
}

// strTryMt tries a metamethod for the operation 'mtname' (trymt).
func strTryMt(L *State, mtname string) {
	L.SetTop(2) /* back to the original arguments */
	if L.Type(2) == LUA_TSTRING || L.GetMetaField(2, mtname) == LUA_TNIL {
		L.Error2("attempt to %s a '%s' with a '%s'", mtname[2:],
			TypeName(L.Type(-2)), TypeName(L.Type(-1)))
	}
	/* call the metamethod with the two original arguments */
	L.PushValue(1)
	L.PushValue(2)
	L.Call(2, 1)
	L.Copy(-1, 1) /* the single result becomes the returned value */
	L.SetTop(1)
}

// strArithOp performs 'op' on the two values on the top of the stack,
// leaving one result (lua_arith).
func strArithOp(L *State, op int) {
	if op == LUA_OPUNM || op == LUA_OPBNOT {
		L.PushValue(-1) /* add fake 2nd operand */
	}
	p1 := L.stack[L.top-2]
	p2 := L.stack[L.top-1]
	luaO_arith(L, op, &p1, &p2, L.top-2)
	L.top-- /* remove second operand */
}

func strArith(L *State, op int, mtname string) int {
	if strTonum(L, 1) && strTonum(L, 2) {
		strArithOp(L, op) /* result will be on the top */
	} else {
		strTryMt(L, mtname)
	}
	return 1
}

func strArithAdd(L *State) int  { return strArith(L, LUA_OPADD, "__add") }
func strArithSub(L *State) int  { return strArith(L, LUA_OPSUB, "__sub") }
func strArithMul(L *State) int  { return strArith(L, LUA_OPMUL, "__mul") }
func strArithMod(L *State) int  { return strArith(L, LUA_OPMOD, "__mod") }
func strArithPow(L *State) int  { return strArith(L, LUA_OPPOW, "__pow") }
func strArithDiv(L *State) int  { return strArith(L, LUA_OPDIV, "__div") }
func strArithIDiv(L *State) int { return strArith(L, LUA_OPIDIV, "__idiv") }
func strArithUnm(L *State) int  { return strArith(L, LUA_OPUNM, "__unm") }

// ---------------------------------------------------------------------
// PATTERN MATCHING
// ---------------------------------------------------------------------

const (
	strMaxCaptures   = 32
	strCapUnfinished = -1
	strCapPosition   = -2
	strMaxCCalls     = 200
	strEsc           = '%'
	strSpecials      = "^$*+?.([%-"
)

type strCapture struct {
	init int
	len  int
}

// strMatchState replaces MatchState: offsets replace the C pointers.
// 'src'/'p' are the subject and the pattern; srcEnd/pEnd are their
// lengths (i.e. the offsets of their ends).
type strMatchState struct {
	L          *State
	src        string
	srcInit    int
	srcEnd     int
	p          string
	pEnd       int
	matchdepth int
	level      int
	capture    [strMaxCaptures]strCapture
}

func strPrepState(ms *strMatchState, L *State, s string, ls int, p string, lp int) {
	ms.L = L
	ms.src = s
	ms.srcInit = 0
	ms.srcEnd = ls
	ms.p = p
	ms.pEnd = lp
}

func strReprepState(ms *strMatchState) {
	ms.matchdepth = strMaxCCalls
	ms.level = 0
}

func strCheckCapture(ms *strMatchState, l int) int {
	l -= '1'
	if l < 0 || l >= ms.level || ms.capture[l].len == strCapUnfinished {
		ms.L.Error2("invalid capture index %%%d", l+1)
	}
	return l
}

func strCaptureToClose(ms *strMatchState) int {
	level := ms.level
	for level--; level >= 0; level-- {
		if ms.capture[level].len == strCapUnfinished {
			return level
		}
	}
	ms.L.Error2("invalid pattern capture")
	return 0
}

// strClassEnd returns the offset just after the class starting at 'p'
// (classend).
func strClassEnd(ms *strMatchState, p int) int {
	var c byte
	if p < len(ms.p) {
		c = ms.p[p]
	}
	p++
	switch c {
	case strEsc:
		if p >= ms.pEnd {
			ms.L.Error2("malformed pattern (ends with '%%')")
		}
		return p + 1
	case '[':
		if p < ms.pEnd && ms.p[p] == '^' {
			p++
		}
		for { /* look for a ']' */
			if p >= ms.pEnd {
				ms.L.Error2("malformed pattern (missing ']')")
			}
			ch := ms.p[p]
			p++
			if ch == strEsc && p < ms.pEnd {
				p++ /* skip escapes (e.g. '%]') */
			}
			if p < ms.pEnd && ms.p[p] == ']' {
				break
			}
		}
		return p + 1
	default:
		return p
	}
}

// character classes, as in the "C" locale
func strIsAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func strIsDigit(c byte) bool { return c >= '0' && c <= '9' }
func strIsAlnum(c byte) bool { return strIsAlpha(c) || strIsDigit(c) }
func strIsCntrl(c byte) bool { return c < 0x20 || c == 0x7f }
func strIsGraph(c byte) bool { return c > 0x20 && c < 0x7f }
func strIsLower(c byte) bool { return c >= 'a' && c <= 'z' }
func strIsUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func strIsPunct(c byte) bool { return strIsGraph(c) && !strIsAlnum(c) }
func strIsXDigit(c byte) bool {
	return strIsDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func strIsSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}

// strMatchClass tells whether 'c' belongs to the class 'cl' (match_class).
func strMatchClass(c, cl byte) bool {
	var res bool
	switch strASCIILower(cl) {
	case 'a':
		res = strIsAlpha(c)
	case 'c':
		res = strIsCntrl(c)
	case 'd':
		res = strIsDigit(c)
	case 'g':
		res = strIsGraph(c)
	case 'l':
		res = strIsLower(c)
	case 'p':
		res = strIsPunct(c)
	case 's':
		res = strIsSpace(c)
	case 'u':
		res = strIsUpper(c)
	case 'w':
		res = strIsAlnum(c)
	case 'x':
		res = strIsXDigit(c)
	case 'z':
		res = (c == 0) /* deprecated option */
	default:
		return cl == c
	}
	if strIsLower(cl) {
		return res
	}
	return !res
}

// strMatchBracketClass matches a set/class starting at 'p' and ending
// at 'ec' (matchbracketclass).
func strMatchBracketClass(c byte, p, ec int, pat string) bool {
	sig := true
	if p+1 < len(pat) && pat[p+1] == '^' {
		sig = false
		p++ /* skip the '^' */
	}
	for p++; p < ec; p++ {
		if pat[p] == strEsc {
			p++
			var cl byte
			if p < len(pat) {
				cl = pat[p]
			}
			if strMatchClass(c, cl) {
				return sig
			}
		} else if p+1 < len(pat) && pat[p+1] == '-' && p+2 < ec {
			p += 2
			if pat[p-2] <= c && c <= pat[p] {
				return sig
			}
		} else if pat[p] == c {
			return sig
		}
	}
	return !sig
}

// strSingleMatch tells whether the character at 's' matches the single
// class at 'p'..'ep' (singlematch).
func strSingleMatch(ms *strMatchState, s, p, ep int) bool {
	if s >= ms.srcEnd {
		return false
	}
	c := ms.src[s]
	var pc byte
	if p < len(ms.p) {
		pc = ms.p[p]
	}
	switch pc {
	case '.':
		return true /* matches any char */
	case strEsc:
		var cl byte
		if p+1 < len(ms.p) {
			cl = ms.p[p+1]
		}
		return strMatchClass(c, cl)
	case '[':
		return strMatchBracketClass(c, p, ep-1, ms.p)
	default:
		return pc == c
	}
}

// strMatchBalance handles '%b' (matchbalance).
func strMatchBalance(ms *strMatchState, s, p int) (int, bool) {
	if p >= ms.pEnd-1 {
		ms.L.Error2("malformed pattern (missing arguments to '%%b')")
	}
	if s >= ms.srcEnd || ms.src[s] != ms.p[p] {
		return 0, false
	}
	b := ms.p[p]
	e := byte(0)
	if p+1 < len(ms.p) {
		e = ms.p[p+1]
	}
	cont := 1
	for s++; s < ms.srcEnd; s++ {
		if ms.src[s] == e {
			cont--
			if cont == 0 {
				return s + 1, true
			}
		} else if ms.src[s] == b {
			cont++
		}
	}
	return 0, false /* string ends out of balance */
}

func strMaxExpand(ms *strMatchState, s, p, ep int) (int, bool) {
	i := 0 /* counts maximum expand for item */
	for strSingleMatch(ms, s+i, p, ep) {
		i++
	}
	/* keeps trying to match with the maximum repetitions */
	for i >= 0 {
		if res, ok := strMatch(ms, s+i, ep+1); ok {
			return res, true
		}
		i-- /* else didn't match; reduce 1 repetition to try again */
	}
	return 0, false
}

func strMinExpand(ms *strMatchState, s, p, ep int) (int, bool) {
	for {
		if res, ok := strMatch(ms, s, ep+1); ok {
			return res, true
		} else if strSingleMatch(ms, s, p, ep) {
			s++ /* try with one more repetition */
		} else {
			return 0, false
		}
	}
}

func strStartCapture(ms *strMatchState, s, p int, what int) (int, bool) {
	level := ms.level
	if level >= strMaxCaptures {
		ms.L.Error2("too many captures")
	}
	ms.capture[level].init = s
	ms.capture[level].len = what
	ms.level = level + 1
	res, ok := strMatch(ms, s, p)
	if !ok { /* match failed? */
		ms.level-- /* undo capture */
	}
	return res, ok
}

func strEndCapture(ms *strMatchState, s, p int) (int, bool) {
	l := strCaptureToClose(ms)
	ms.capture[l].len = s - ms.capture[l].init /* close capture */
	res, ok := strMatch(ms, s, p)
	if !ok { /* match failed? */
		ms.capture[l].len = strCapUnfinished /* undo capture */
	}
	return res, ok
}

func strMatchCapture(ms *strMatchState, s, l int) (int, bool) {
	l = strCheckCapture(ms, l)
	l2 := ms.capture[l].len
	if l2 < 0 { /* a position capture never matches as text */
		return 0, false
	}
	if ms.srcEnd-s >= l2 && ms.src[s:s+l2] == ms.src[ms.capture[l].init:ms.capture[l].init+l2] {
		return s + l2, true
	}
	return 0, false
}

// strMatchRet restores the recursion depth and normalizes a result of
// strMatch.
func strMatchRet(ms *strMatchState, s int, ok bool) (int, bool) {
	ms.matchdepth++
	return s, ok
}

// strMatch is the pattern matcher (match).  's' is an offset into the
// subject, 'p' an offset into the pattern.  The C code uses 'goto init'
// for tail recursion; here it is a 'for' loop.
func strMatch(ms *strMatchState, s, p int) (int, bool) {
	if ms.matchdepth == 0 {
		ms.L.Error2("pattern too complex")
	}
	ms.matchdepth--
	for {
		if p == ms.pEnd { /* end of pattern? */
			return strMatchRet(ms, s, true)
		}
		pc := byte(0)
		if p < len(ms.p) {
			pc = ms.p[p]
		}
		switch pc {
		case '(': /* start capture */
			if p+1 < len(ms.p) && ms.p[p+1] == ')' { /* position capture? */
				s2, ok := strStartCapture(ms, s, p+2, strCapPosition)
				return strMatchRet(ms, s2, ok)
			}
			s2, ok := strStartCapture(ms, s, p+1, strCapUnfinished)
			return strMatchRet(ms, s2, ok)
		case ')': /* end capture */
			s2, ok := strEndCapture(ms, s, p+1)
			return strMatchRet(ms, s2, ok)
		case '$':
			if (p + 1) != ms.pEnd { /* is the '$' the last char in pattern? */
				break /* no; go to default */
			}
			if s == ms.srcEnd {
				return strMatchRet(ms, s, true)
			}
			return strMatchRet(ms, 0, false)
		case strEsc: /* escaped sequences not in the format class[*+?-]? */
			ec := byte(0)
			if p+1 < len(ms.p) {
				ec = ms.p[p+1]
			}
			switch ec {
			case 'b': /* balanced string? */
				s3, ok := strMatchBalance(ms, s, p+2)
				if ok {
					s = s3
					p += 4
					continue /* return match(ms, s, p + 4) */
				}
				return strMatchRet(ms, 0, false) /* else fail */
			case 'f': /* frontier? */
				p += 2
				if p >= len(ms.p) || ms.p[p] != '[' {
					ms.L.Error2("missing '[' after '%%f' in pattern")
				}
				ep := strClassEnd(ms, p) /* points to what is next */
				var previous byte
				if s != ms.srcInit {
					previous = ms.src[s-1]
				}
				var current byte
				if s < ms.srcEnd {
					current = ms.src[s]
				}
				if !strMatchBracketClass(previous, p, ep-1, ms.p) &&
					strMatchBracketClass(current, p, ep-1, ms.p) {
					p = ep
					continue /* return match(ms, s, ep) */
				}
				return strMatchRet(ms, 0, false) /* match failed */
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
				/* capture results (%0-%9)? */
				s2, ok := strMatchCapture(ms, s, int(ec))
				if ok {
					p += 2
					s = s2
					continue /* return match(ms, s, p + 2) */
				}
				return strMatchRet(ms, 0, false)
			default:
				/* go to default */
			}
		}
		/* default: pattern class plus optional suffix */
		ep := strClassEnd(ms, p) /* points to optional suffix */
		if !strSingleMatch(ms, s, p, ep) {
			/* does not match at least once? */
			var ec byte
			if ep < len(ms.p) {
				ec = ms.p[ep]
			}
			if ec == '*' || ec == '?' || ec == '-' { /* accept empty? */
				p = ep + 1
				continue /* return match(ms, s, ep + 1) */
			}
			return strMatchRet(ms, 0, false) /* '+' or no suffix: fail */
		}
		/* matched once */
		var ec byte
		if ep < len(ms.p) {
			ec = ms.p[ep]
		}
		switch ec {
		case '?': /* optional */
			if res, ok := strMatch(ms, s+1, ep+1); ok {
				return strMatchRet(ms, res, true)
			}
			p = ep + 1
			continue /* else return match(ms, s, ep + 1) */
		case '+': /* 1 or more repetitions */
			s++ /* 1 match already done */
			s2, ok := strMaxExpand(ms, s, p, ep)
			return strMatchRet(ms, s2, ok)
		case '*': /* 0 or more repetitions */
			s2, ok := strMaxExpand(ms, s, p, ep)
			return strMatchRet(ms, s2, ok)
		case '-': /* 0 or more repetitions (minimum) */
			s2, ok := strMinExpand(ms, s, p, ep)
			return strMatchRet(ms, s2, ok)
		default: /* no suffix */
			s++
			p = ep
			continue /* return match(ms, s + 1, ep) */
		}
	}
}

// strGetOneCapture gets the i-th capture: it returns its start offset
// and its length; for a position capture it pushes the position and
// returns strCapPosition as the length (get_onecapture).
func strGetOneCapture(ms *strMatchState, i, s, e int) (int, int) {
	if i >= ms.level {
		if i != 0 {
			ms.L.Error2("invalid capture index %%%d", i+1)
		}
		return s, e - s
	}
	capl := ms.capture[i].len
	cap := ms.capture[i].init
	if capl == strCapUnfinished {
		ms.L.Error2("unfinished capture")
	} else if capl == strCapPosition {
		ms.L.PushInteger(int64(cap-ms.srcInit) + 1)
	}
	return cap, capl
}

func strPushOneCapture(ms *strMatchState, i, s, e int) {
	cap, l := strGetOneCapture(ms, i, s, e)
	if l != strCapPosition {
		ms.L.PushString(ms.src[cap : cap+l])
	}
	/* else position was already pushed */
}

func strPushCaptures(ms *strMatchState, s, e int) int {
	nlevels := ms.level
	if ms.level == 0 && s >= 0 {
		nlevels = 1
	}
	ms.L.CheckStack2(nlevels, "too many captures")
	for i := 0; i < nlevels; i++ {
		strPushOneCapture(ms, i, s, e)
	}
	return nlevels /* number of strings pushed */
}

// strNoSpecials tells whether the pattern has no special characters
// (nospecials).
func strNoSpecials(p string) bool {
	return strings.IndexAny(p, strSpecials) < 0
}

func strFindAux(L *State, find bool) int {
	s := L.CheckString(1)
	p := L.CheckString(2)
	ls := len(s)
	lp := len(p)
	init := strPosRelatI(L.OptInteger(3, 1), ls) - 1
	if init > ls { /* start after string's end? */
		L.PushNil() /* cannot find anything */
		return 1
	}
	/* explicit request or no special characters? */
	if find && (L.ToBoolean(4) || strNoSpecials(p)) {
		/* do a plain search */
		if i := strings.Index(s[init:], p); i >= 0 {
			L.PushInteger(int64(init+i) + 1)
			L.PushInteger(int64(init + i + lp))
			return 2
		}
	} else {
		s1 := init
		pat := p
		anchor := false
		if len(pat) > 0 && pat[0] == '^' {
			anchor = true
			pat = pat[1:] /* skip anchor character */
		}
		var ms strMatchState
		strPrepState(&ms, L, s, ls, pat, len(pat))
		for {
			strReprepState(&ms)
			if res, ok := strMatch(&ms, s1, 0); ok {
				if find {
					L.PushInteger(int64(s1) + 1) /* start */
					L.PushInteger(int64(res))    /* end */
					return strPushCaptures(&ms, -1, 0) + 2
				}
				return strPushCaptures(&ms, s1, res)
			}
			if s1 >= ms.srcEnd || anchor {
				break
			}
			s1++
		}
	}
	L.PushNil() /* not found */
	return 1
}

func strFind(L *State) int { return strFindAux(L, true) }

// strMatchLib is the 'string.match' function (str_match).
func strMatchLib(L *State) int { return strFindAux(L, false) }

// ---------------------------------------------------------------------
// gmatch
// ---------------------------------------------------------------------

type strGMatchState struct {
	src       int /* current position (offset into the subject) */
	p         int /* pattern offset (always 0 here) */
	lastmatch int /* end of last match */
	ms        strMatchState
}

func strGmatchAux(L *State) int {
	gm, _ := L.ToUserData(UpvalueIndex(3)).(*strGMatchState)
	gm.ms.L = L
	for src := gm.src; src <= gm.ms.srcEnd; src++ {
		strReprepState(&gm.ms)
		if e, ok := strMatch(&gm.ms, src, gm.p); ok && e != gm.lastmatch {
			gm.src = e
			gm.lastmatch = e
			return strPushCaptures(&gm.ms, src, e)
		}
	}
	return 0 /* not found */
}

func strGmatch(L *State) int {
	s := L.CheckString(1)
	p := L.CheckString(2)
	ls := len(s)
	init := strPosRelatI(L.OptInteger(3, 1), ls) - 1
	L.SetTop(2)    /* keep strings on closure to avoid being collected */
	if init > ls { /* start after string's end? */
		init = ls + 1 /* avoid overflows in 's + init' */
	}
	gm := &strGMatchState{}
	strPrepState(&gm.ms, L, s, ls, p, len(p))
	gm.src = init
	gm.p = 0
	gm.lastmatch = -1
	L.NewUserData(gm, 0)
	L.PushGoClosure(strGmatchAux, 3)
	return 1
}

// ---------------------------------------------------------------------
// gsub
// ---------------------------------------------------------------------

// strAddS adds the replacement string (with its %0-%9 escapes) to the
// buffer (add_s).
func strAddS(ms *strMatchState, b *Buffer, s, e int) {
	L := ms.L
	news := L.CheckString(3)
	for {
		i := strings.IndexByte(news, strEsc)
		if i < 0 {
			break
		}
		b.AddString(news[:i])
		p := i + 1 /* skip ESC */
		if p >= len(news) {
			L.Error2("invalid use of '%c' in replacement string", strEsc)
		}
		if news[p] == strEsc { /* '%%' */
			b.AddChar(news[p])
		} else if news[p] == '0' { /* '%0' */
			b.AddString(ms.src[s:e])
		} else if news[p] >= '0' && news[p] <= '9' { /* '%n' */
			cap, resl := strGetOneCapture(ms, int(news[p]-'1'), s, e)
			if resl == strCapPosition {
				b.AddValue() /* add position to accumulated result */
			} else {
				b.AddString(ms.src[cap : cap+resl])
			}
		} else {
			L.Error2("invalid use of '%c' in replacement string", strEsc)
		}
		news = news[p+1:]
	}
	b.AddString(news)
}

// strAddValue adds the replacement value to the buffer; it returns true
// when the original string was changed (add_value).
func strAddValue(ms *strMatchState, b *Buffer, s, e int, tr int) bool {
	L := ms.L
	switch tr {
	case LUA_TFUNCTION: /* call the function */
		L.PushValue(3) /* push the function */
		n := strPushCaptures(ms, s, e)
		L.Call(n, 1) /* call it */
	case LUA_TTABLE: /* index the table */
		strPushOneCapture(ms, 0, s, e) /* first capture is the index */
		L.GetTable(3)
	default: /* LUA_TNUMBER or LUA_TSTRING */
		strAddS(ms, b, s, e) /* add value to the buffer */
		return true          /* something changed */
	}
	if !L.ToBoolean(-1) { /* nil or false? */
		L.Pop(1)                 /* remove value */
		b.AddString(ms.src[s:e]) /* keep original text */
		return false             /* no changes */
	} else if !L.IsString(-1) {
		L.Error2("invalid replacement value (a %s)", TypeName(L.Type(-1)))
		return false
	}
	b.AddValue() /* add result to accumulator */
	return true  /* something changed */
}

func strGsub(L *State) int {
	src := L.CheckString(1) /* subject */
	p := L.CheckString(2)   /* pattern */
	tr := L.Type(3)         /* replacement type */
	maxS := L.OptInteger(4, int64(len(src))+1)
	/* lstrlib.c uses 'luaL_argexpected' here, so the message names the
	   type that came: "string/function/table expected, got boolean". */
	L.ArgExpected(tr == LUA_TNUMBER || tr == LUA_TSTRING ||
		tr == LUA_TFUNCTION || tr == LUA_TTABLE, 3, "string/function/table")
	pat := p
	anchor := false
	if len(pat) > 0 && pat[0] == '^' {
		anchor = true
		pat = pat[1:] /* skip anchor character */
	}
	var ms strMatchState
	strPrepState(&ms, L, src, len(src), pat, len(pat))
	b := L.NewBuffer()
	lastmatch := -1 /* end of last match */
	pos := 0        /* current position in the subject */
	var n int64     /* replacement count */
	changed := false
	for n < maxS {
		strReprepState(&ms) /* (re)prepare state for new match */
		e, ok := strMatch(&ms, pos, 0)
		if ok && e != lastmatch { /* match? */
			n++
			if strAddValue(&ms, b, pos, e, tr) {
				changed = true
			}
			pos = e
			lastmatch = e
		} else if pos < ms.srcEnd { /* otherwise, skip one character */
			b.AddChar(ms.src[pos])
			pos++
		} else { /* end of subject */
			break
		}
		if anchor {
			break
		}
	}
	if !changed { /* no changes? */
		L.PushValue(1) /* return original string */
	} else { /* something changed */
		b.AddString(src[pos:ms.srcEnd])
		b.PushResult() /* create and return new string */
	}
	L.PushInteger(n) /* number of substitutions */
	return 2
}

// ---------------------------------------------------------------------
// STRING FORMAT
// ---------------------------------------------------------------------

const (
	strMaxFormat  = 32
	strMaxItem    = 120
	strFmtFlagsF  = "-+#0 " /* valid flags for a, A, e, E, f, F, g, G */
	strFmtFlagsX  = "-#0"   /* valid flags for o, x, X */
	strFmtFlagsI  = "-+0 "  /* valid flags for d, i */
	strFmtFlagsU  = "-0"    /* valid flags for u */
	strFmtFlagsC  = "-"     /* valid flags for c, p, s */
	strFmtScan    = "123456789."
	strMaxDecimal = ".6" /* default precision of the C 'f'/'e'/'g' */
)

func strIsAlphaChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// strGet2Digits skips up to two digits (get2digits).
func strGet2Digits(s string, i int) int {
	if i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		if i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++ /* (2 digits at most) */
		}
	}
	return i
}

// strCheckFormat checks whether a conversion specification is valid
// (checkformat).
func strCheckFormat(L *State, form, flags string, precision bool) {
	spec := 1 /* skip '%' */
	for spec < len(form) && strings.IndexByte(flags, form[spec]) >= 0 {
		spec++ /* skip flags */
	}
	if spec < len(form) && form[spec] != '0' { /* a width cannot start with '0' */
		spec = strGet2Digits(form, spec) /* skip width */
		if spec < len(form) && form[spec] == '.' && precision {
			spec++
			spec = strGet2Digits(form, spec) /* skip precision */
		}
	}
	if spec >= len(form) || !strIsAlphaChar(form[spec]) {
		/* did not go to the end? */
		L.Error2("invalid conversion specification: '%s'", form)
	}
}

// strFmtSpec is a conversion specification broken into the parts C's
// printf acts on: the flags, the width and the precision.  The port
// needs a view of them of its own, because Go's fmt does not do the
// same things with them that C does.  It counts a width in runes and
// not in bytes, it drops the sign of a zero written with a precision of
// zero, and it pads the digits of a '%#05x' where C pads the field.
type strFmtSpec struct {
	left  bool /* '-': pad on the right */
	plus  bool /* '+': always write a sign */
	space bool /* ' ': write a space where a sign would go */
	zero  bool /* '0': pad with zeros */
	alt   bool /* '#': the alternative form */
	width int
	prec  int /* -1 when no precision was given */
}

// strParseFormat breaks a conversion specification into its parts.
// 'form' is exactly what strCheckFormat has accepted: '%', then flags,
// then a width, then '.' and a precision, then the conversion.
func strParseFormat(form string) (f strFmtSpec) {
	i := 1
	for ; i < len(form)-1 && strings.IndexByte("-+ #0", form[i]) >= 0; i++ {
		switch form[i] {
		case '-':
			f.left = true
		case '+':
			f.plus = true
		case ' ':
			f.space = true
		case '0':
			f.zero = true
		case '#':
			f.alt = true
		}
	}
	for ; i < len(form)-1 && form[i] >= '0' && form[i] <= '9'; i++ {
		f.width = f.width*10 + int(form[i]-'0')
	}
	f.prec = -1
	if i < len(form)-1 && form[i] == '.' {
		f.prec = 0 /* '%.s' names a precision of zero, not "no precision" */
		for i++; i < len(form)-1 && form[i] >= '0' && form[i] <= '9'; i++ {
			f.prec = f.prec*10 + int(form[i]-'0')
		}
	}
	return f
}

// strPadField writes 's' into a field of 'f.width' characters the way
// C's printf does: '-' moves the padding to the right, '0' pads with
// zeros, and a precision overrides '0' and brings the spaces back.
func strPadField(f strFmtSpec, s string) string {
	if pad := f.width - len(s); pad > 0 {
		switch {
		case f.left:
			return s + strings.Repeat(" ", pad)
		case f.zero && f.prec < 0:
			return strings.Repeat("0", pad) + s
		default:
			return strings.Repeat(" ", pad) + s
		}
	}
	return s
}

// strFormatS formats '%s' with a width and a precision, the way C does.
//
// C counts both of them in *bytes* (lstrlib.c str_format hands the
// string to its own formatter, which walks 'strlen' bytes), while Go's
// fmt counts runes.  The two disagree for every string that is not
// ASCII: 'string.format("%6s", "\u{4f60}\u{597d}")' is six bytes and two
// characters, so C pads it with nothing and Go padded it with four
// spaces, and 'string.format("%.3s", ...)' is the first three bytes --
// one character -- where Go took the first three characters.
func strFormatS(form, s string) string {
	f := strParseFormat(form)
	if f.prec >= 0 && len(s) > f.prec {
		s = s[:f.prec] /* the precision is a byte count */
	}
	return strPadField(f, s)
}

// strFormatZeroInt is what C's printf does for a zero integer written
// with a precision of zero: there are no digits, but the sign -- or the
// space that stands in for one -- is written anyway, and the width is
// padded to with spaces even when the '0' flag is given, a flag a
// precision overrides.  Go's fmt writes nothing at all: fmtInteger
// returns before it reaches the sign.  So
// 'string.format("%+.0d", 0)' answered "" where PUC-Lua answers "+",
// 'string.format("% .0d", 0)' answered "" instead of " ", and
// 'string.format("%+5.0d", 0)' answered five spaces instead of "    +".
func strFormatZeroInt(f strFmtSpec) string {
	sign := ""
	if f.plus {
		sign = "+"
	} else if f.space {
		sign = " " /* '+' wins over ' ' when both are given */
	}
	if f.zero && f.prec < 0 {
		/* the sign goes before the zeros, as it does in C */
		if pad := f.width - len(sign); pad > 0 {
			if f.left {
				return sign + strings.Repeat(" ", pad)
			}
			return sign + strings.Repeat("0", pad)
		}
		return sign
	}
	return strPadField(f, sign)
}

// strFormatUnsigned formats '%o', '%x' and '%X' the way C's printf
// does, which is not what Go's fmt does in two places.
//
// The width of a '%#05x' is the width of the whole field, prefix
// included: C answers "0x001" for the value 1, padding the two
// characters of the prefix and the one digit up to five with zeros.
// Go's fmt takes the width as the width of the digits and writes
// "0x00001".  And '#' on a '%o' keeps a zero written with a precision
// of zero from vanishing, because the flag raises the precision until
// the first digit is a zero: C answers "0" where Go answers "".
func strFormatUnsigned(f strFmtSpec, n uint64, spec byte) string {
	base := 16
	if spec == 'o' {
		base = 8
	}
	digits := strconv.FormatUint(n, base)
	if spec == 'X' {
		digits = strings.ToUpper(digits)
	}
	if f.prec == 0 && n == 0 {
		digits = "" /* a precision of zero writes no digits for a zero */
	} else if len(digits) < f.prec {
		digits = strings.Repeat("0", f.prec-len(digits)) + digits
	}
	prefix := ""
	switch {
	case spec == 'o' && f.alt && !strings.HasPrefix(digits, "0"):
		prefix = "0" /* '#' raises the precision to force a leading zero */
	case spec != 'o' && f.alt && n != 0:
		/* '#' prefixes a non-zero hexadecimal value only */
		if spec == 'X' {
			prefix = "0X"
		} else {
			prefix = "0x"
		}
	}
	body := len(prefix) + len(digits)
	if pad := f.width - body; pad > 0 {
		switch {
		case f.left:
			return prefix + digits + strings.Repeat(" ", pad)
		case f.zero && f.prec < 0:
			/* the zeros go between the prefix and the digits */
			return prefix + strings.Repeat("0", pad) + digits
		default:
			return strings.Repeat(" ", pad) + prefix + digits
		}
	}
	return prefix + digits
}

// strGetFormat gets a conversion specification starting at 'i' (just
// after the '%'); it returns the specification ('%...') and the index
// of its last character (getformat).
func strGetFormat(L *State, strfrmt string, i int) (string, int) {
	/* spans flags, width, and precision ('0' is included as a flag) */
	l := 0
	for i+l < len(strfrmt) && strings.IndexByte(strFmtFlagsF+strFmtScan, strfrmt[i+l]) >= 0 {
		l++
	}
	l++ /* adds following character (should be the specifier) */
	/* still needs space for '%', '\0', plus a length modifier */
	if l >= strMaxFormat-10 {
		L.Error2("invalid format (too long)")
	}
	var form string
	if i+l <= len(strfrmt) {
		form = "%" + strfrmt[i:i+l]
	} else {
		form = "%" + strfrmt[i:]
	}
	return form, i + l - 1
}

// strNumber2StrX is the C99 hexadecimal floating-point formatter
// (lua_number2strx).  Unlike the C89 fallback it honours flags, width
// and precision.
func strNumber2StrX(L *State, form string, x float64) string {
	/* parse the conversion specification: '%[flags][width][.prec]a|A' */
	var flags string
	i := 1
	for i < len(form)-1 && strings.IndexByte(strFmtFlagsF, form[i]) >= 0 {
		flags += string(form[i])
		i++
	}
	width := -1
	if i < len(form)-1 && form[i] >= '0' && form[i] <= '9' {
		w := 0
		for i < len(form)-1 && form[i] >= '0' && form[i] <= '9' {
			w = w*10 + int(form[i]-'0')
			i++
		}
		width = w
	}
	prec := -1 /* no precision given: use the shortest round-trip form */
	if i < len(form)-1 && form[i] == '.' {
		i++
		prec = 0 /* '%.a' means precision zero */
		for i < len(form)-1 && form[i] >= '0' && form[i] <= '9' {
			prec = prec*10 + int(form[i]-'0')
			i++
		}
	}
	upper := form[len(form)-1] == 'A'
	alt := strings.IndexByte(flags, '#') >= 0

	var body string
	finite := true
	switch {
	case math.IsNaN(x):
		finite = false
		body = fmtNaN(x) /* see fmtNaN: the sign is the library's choice */
	case math.IsInf(x, 0):
		finite = false
		body = "inf"
	default:
		body = strHexFloat(math.Abs(x), prec, alt)
	}
	/* the sign: '-' wins over '+'/' '; NaN never gets a '+' or a ' ' */
	var sign string
	if math.Signbit(x) && !math.IsNaN(x) {
		/* a NaN carries its '-' (if any) inside the word already */
		sign = "-"
	} else if finite || !math.IsNaN(x) {
		if strings.IndexByte(flags, '+') >= 0 {
			sign = "+"
		} else if strings.IndexByte(flags, ' ') >= 0 {
			sign = " "
		}
	}
	s := sign + body
	if width > len(s) {
		pad := width - len(s)
		switch {
		case strings.IndexByte(flags, '-') >= 0: /* left align */
			s += strings.Repeat(" ", pad)
		case strings.IndexByte(flags, '0') >= 0 && finite:
			/* the padding goes right after the "0x" prefix */
			prefix := len(sign) + 2
			s = s[:prefix] + strings.Repeat("0", pad) + s[prefix:]
		default: /* right align */
			s = strings.Repeat(" ", pad) + s
		}
	}
	if upper {
		s = strings.ToUpper(s)
	}
	return s
}

// strNormalizeHexExp rewrites Go's "p+03" exponent as C's "p+3".
func strNormalizeHexExp(s string) string {
	i := strings.IndexByte(s, 'p')
	if i < 0 {
		return s
	}
	j := i + 1
	var sign string
	if j < len(s) && (s[j] == '+' || s[j] == '-') {
		sign = string(s[j])
		j++
	}
	k := j
	for k < len(s)-1 && s[k] == '0' {
		k++ /* strip leading zeros, but keep at least one digit */
	}
	return s[:i+1] + sign + s[k:]
}

// strHexValue returns the numeric value of a hexadecimal digit.
func strHexValue(c byte) int {
	if c >= '0' && c <= '9' {
		return int(c - '0')
	}
	return int(c-'a') + 10
}

// strRoundsUp tells whether the hexadecimal digits being dropped from a
// %a conversion -- 'tail' is the first of them -- are more than half of
// the last digit kept, which is 'last'.  Half is 8.
//
// A tail that begins with exactly 8 and continues with nothing but
// zeros is a tie, and the C library breaks a tie on the parity of the
// last digit kept: glibc's __printf_fp hands that digit to
// 'round_away', which leaves it alone when it is already even and
// steps it when it is odd -- so the answer is always even.  That is
// also what the C library does for '%f' on every platform ('%.0f' of
// 1.5 is 2), and what this port's decimal conversions do.
//
// Apple's library is the one that differs: its '%a' rounds a tie *down*
// whatever the last digit is, so '%.0A' of 1.5 is 0X1P+0 there and
// 0X2P+0 under glibc.  This port answers 0X2P+0, so that '%a' and '%f'
// agree with each other; formatfuzz_test.go asks the reference which
// library it was built against and leaves those cases out of the
// comparison when it was Apple's.
func strRoundsUp(last byte, tail []byte) bool {
	d := strHexValue(tail[0])
	if d != 8 {
		return d > 8
	}
	for _, c := range tail[1:] {
		if c != '0' {
			return true /* more than half after all */
		}
	}
	return strHexValue(last)%2 == 1 /* a tie: to even */
}

// strHexFloat formats a non-negative finite float as '0x1.xxxp+e'.
// 'prec' is the number of hexadecimal digits after the radix point
// (-1 for the shortest representation) and 'alt' asks for the radix
// point to be always present (the '#' flag).
func strHexFloat(x float64, prec int, alt bool) string {
	if x == 0 { /* can be -0, but 'x' is already the absolute value */
		s := "0x0"
		if prec > 0 {
			s += "." + strings.Repeat("0", prec)
		} else if alt {
			s += "."
		}
		return s + "p+0"
	}
	if prec < 0 { /* no precision: shortest representation that round-trips */
		s := strNormalizeHexExp(strconv.FormatFloat(x, 'x', -1, 64))
		if alt && !strings.Contains(s, ".") {
			s = strings.Replace(s, "p", ".p", 1)
		}
		return s
	}
	m, e := math.Frexp(x) /* x == m * 2^e, with m in [0.5,1) */
	frac := m*2 - 1       /* x == (1 + frac) * 2^(e-1) */
	exp := e - 1
	/* the 52-bit fraction is worth exactly 13 hexadecimal digits */
	digits := make([]byte, 13)
	for i := range digits {
		frac *= 16
		d := int(frac)
		if d > 15 {
			d = 15
		}
		digits[i] = "0123456789abcdef"[d]
		frac -= float64(d)
	}
	intDigit := byte('1')
	/* Round up when what is being dropped is more than half of the
	   last digit kept.  Half is 8 in hexadecimal, so the first
	   dropped digit decides unless it is exactly 8, in which case
	   whatever follows it decides, and if nothing does then the
	   parity of the last digit kept does.  Judging by the first
	   dropped digit alone is what made '%.8a' of 1e-300 answer
	   0x1.56e1fc2fp-997 where C answers 0x1.56e1fc30p-997: the
	   digits are 56e1fc2f|8f359, the first dropped digit is a bare
	   '8', and the rest of the tail is not zero. */
	if prec < 13 {
		/* the digit the rounding lands on: the integer digit when no
		   fraction digit survives, the last one that does otherwise */
		last := intDigit
		if prec > 0 {
			last = digits[prec-1]
		}
		if strRoundsUp(last, digits[prec:]) {
			carry := true
			for i := prec - 1; i >= 0 && carry; i-- {
				v := strHexValue(digits[i])
				if v == 15 {
					digits[i] = '0'
				} else {
					digits[i] = "0123456789abcdef"[v+1]
					carry = false
				}
			}
			if carry {
				/* the integer digit goes from 1 to 2; the C library
				   does not renormalize the result, it just prints
				   "0x2.00..." */
				intDigit = '2'
			}
		}
	}
	var sb strings.Builder
	sb.WriteString("0x")
	sb.WriteByte(intDigit)
	if prec > 0 {
		sb.WriteByte('.')
		for i := 0; i < prec; i++ {
			if i < 13 {
				sb.WriteByte(digits[i])
			} else {
				sb.WriteByte('0')
			}
		}
	} else if alt {
		sb.WriteByte('.')
	}
	sb.WriteString("p")
	if exp >= 0 {
		sb.WriteByte('+')
	} else {
		sb.WriteByte('-')
		exp = -exp
	}
	sb.WriteString(strconv.Itoa(exp))
	return sb.String()
}

// strQuoteFloat serializes a float in a way that Lua can read back
// (quotefloat).
func strQuoteFloat(L *State, n float64) string {
	if math.IsInf(n, 1) {
		return "1e9999"
	} else if math.IsInf(n, -1) {
		return "-1e9999"
	} else if math.IsNaN(n) {
		return "(0/0)"
	}
	return strNumber2StrX(L, "%a", n)
}

// strAddQuoted adds a quoted version of 's' to the buffer (addquoted).
func strAddQuoted(b *Buffer, s string) {
	b.AddChar('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' || c == '\n' {
			b.AddChar('\\')
			b.AddChar(c)
		} else if strIsCntrl(c) {
			if i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9' {
				b.AddString(fmt.Sprintf("\\%03d", int(c)))
			} else {
				b.AddString(fmt.Sprintf("\\%d", int(c)))
			}
		} else {
			b.AddChar(c)
		}
	}
	b.AddChar('"')
}

// strAddLiteral adds the value at 'arg' in its literal form
// (addliteral).
func strAddLiteral(L *State, b *Buffer, arg int) {
	switch L.Type(arg) {
	case LUA_TSTRING:
		s, _ := L.ToString(arg)
		strAddQuoted(b, s)
	case LUA_TNUMBER:
		if !L.IsInteger(arg) { /* float? */
			b.AddString(strQuoteFloat(L, L.CheckNumber(arg)))
		} else { /* integers */
			n := L.CheckInteger(arg)
			if n == LUA_MININTEGER { /* corner case? */
				b.AddString(fmt.Sprintf("0x%x", uint64(n))) /* use hex */
			} else {
				b.AddString(strconv.FormatInt(n, 10))
			}
		}
	case LUA_TNIL, LUA_TBOOLEAN:
		if L.Type(arg) == LUA_TNIL {
			b.AddString("nil")
		} else if L.ToBoolean(arg) {
			b.AddString("true")
		} else {
			b.AddString("false")
		}
	default:
		L.ArgError(arg, "value has no literal form")
	}
}

// strToLString converts the value at 'idx' into a string, pushing the
// result on the stack (luaL_tolstring).  The pushed string is left on
// the stack.
func strToLString(L *State, idx int) string {
	idx = L.AbsIndex(idx)
	if L.CallMeta(idx, "__tostring") { /* metafield? */
		if !L.IsString(-1) {
			L.Error2("'__tostring' must return a string")
		}
	} else {
		switch L.Type(idx) {
		case LUA_TNUMBER:
			/* the C code pushes "%I" or "%f"; both go through
			   luaO_pushvfstring, which formats numbers with
			   'tostringbuff' (i.e. "%.14g"), not with C's printf */
			var num Value
			if L.IsInteger(idx) {
				setivalue(&num, L.CheckInteger(idx))
			} else {
				setfltvalue(&num, L.CheckNumber(idx))
			}
			L.PushString(tostringbuff(&num))
		case LUA_TSTRING:
			L.PushValue(idx)
		case LUA_TBOOLEAN:
			if L.ToBoolean(idx) {
				L.PushString("true")
			} else {
				L.PushString("false")
			}
		case LUA_TNIL:
			L.PushString("nil")
		default:
			kind := ""
			if L.GetMetaField(idx, "__name") == LUA_TSTRING {
				kind = L.ToStringX(-1)
				L.PushString(fmt.Sprintf("%s: 0x%x", kind, uint64(L.ToPointer(idx))))
				L.Remove(-2)
			} else {
				L.PushString(fmt.Sprintf("%s: 0x%x", TypeName(L.Type(idx)), uint64(L.ToPointer(idx))))
			}
		}
	}
	s, _ := L.ToString(-1)
	return s
}

// strFormFlagsWidth takes a format item ('%-10.3f') apart into its flags
// and its width, dropping the precision: a non-finite value is printed as
// a word, so neither the precision nor the '0' flag applies to it.
func strFormFlagsWidth(form string) (flags, width string) {
	end := len(form) - 1 /* the conversion specifier */
	if j := strings.IndexByte(form, '.'); j >= 0 && j < end {
		end = j
	}
	i := 1 /* the flags come first, then the width */
	for i < end && strings.IndexByte(strFmtFlagsF, form[i]) >= 0 {
		if form[i] != '0' { /* "inf" is padded with blanks, not zeroes */
			flags += string(form[i])
		}
		i++
	}
	return flags, form[i:end]
}

// strFormatFloat applies a floating-point conversion of 'form'
// ('f', 'e', 'E', 'g' or 'G') to 'n'.  C's default precision (6) is
// made explicit for Go.
func strFormatFloat(form string, n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		/* C prints a non-finite value as a word: neither the precision
		   nor the '0' flag applies to it, so "%.2f" of inf is "inf"
		   (and not "in") and "%010.2f" of it is "       inf". */
		s := fmtNaN(n) /* see fmtNaN: the sign is the library's choice */
		switch {
		case math.IsInf(n, 1):
			s = "inf"
		case math.IsInf(n, -1):
			s = "-inf"
		}
		/* a NaN never takes the '+' or ' ' flag (only its own sign bit) */
		if !strings.HasPrefix(s, "-") && !math.IsNaN(n) {
			switch {
			case strings.IndexByte(form, '+') >= 0:
				s = "+" + s
			case strings.IndexByte(form, ' ') >= 0:
				s = " " + s
			}
		}
		if form[len(form)-1] == 'E' || form[len(form)-1] == 'G' {
			s = strings.ToUpper(s) /* C spells it in the case of the specifier */
		}
		flags, width := strFormFlagsWidth(form)
		return fmt.Sprintf("%"+flags+width+"s", s)
	}
	f := form
	if !strings.Contains(form, ".") {
		/* no precision given: C uses 6 */
		f = form[:len(form)-1] + strMaxDecimal + form[len(form)-1:]
	}
	/* 'f', 'e' and 'g' use the locale's decimal point, because C's printf
	   does -- "%.3f" of 4.51 is "4,510" under pt_BR. '%a' is not
	   touched: it writes a hexadecimal number, and the reference
	   interpreter writes it with a '.' whatever the locale, which is
	   what lets '%q' round-trip. */
	return num2strLocale(fmt.Sprintf(f, n))
}

func strFormat(L *State) int {
	top := L.GetTop()
	arg := 1
	strfrmt := L.CheckString(arg)
	b := L.NewBuffer()
	i := 0
	for i < len(strfrmt) {
		if strfrmt[i] != strEsc {
			b.AddChar(strfrmt[i])
			i++
			continue
		}
		i++
		if i < len(strfrmt) && strfrmt[i] == strEsc {
			b.AddChar(strEsc) /* %% */
			i++
			continue
		}
		/* format item */
		arg++
		if arg > top {
			return L.ArgError(arg, "no value")
		}
		form, last := strGetFormat(L, strfrmt, i)
		spec := byte(0)
		if last >= 0 && last < len(strfrmt) {
			spec = strfrmt[last]
		}
		i = last + 1
		switch spec {
		case 'c':
			strCheckFormat(L, form, strFmtFlagsC, false)
			b.AddString(fmt.Sprintf(form[:len(form)-1]+"s", string([]byte{byte(L.CheckInteger(arg))})))
		case 'd', 'i':
			/* C fetches the argument first -- 'intcase' is
			   'luaL_checkinteger(L, arg); checkformat(...)' -- so a
			   value that is not an integer is complained about even
			   when the specification is itself invalid: '%#d' with
			   1.5 answers "number has no integer representation",
			   not "invalid conversion specification". */
			n := L.CheckInteger(arg)
			strCheckFormat(L, form, strFmtFlagsI, true)
			if f := strParseFormat(form); n == 0 && f.prec == 0 {
				/* C's printf still writes the sign when there are no
				   digits to write; Go's fmt returns before it gets
				   there.  See strFormatZeroInt. */
				b.AddString(strFormatZeroInt(f))
			} else {
				b.AddString(fmt.Sprintf(form[:len(form)-1]+"d", n))
			}
		case 'u':
			n := L.CheckInteger(arg) /* as in 'intcase': value first */
			strCheckFormat(L, form, strFmtFlagsU, true)
			b.AddString(fmt.Sprintf(form[:len(form)-1]+"d", uint64(n)))
		case 'o', 'x', 'X':
			n := L.CheckInteger(arg) /* as in 'intcase': value first */
			strCheckFormat(L, form, strFmtFlagsX, true)
			b.AddString(strFormatUnsigned(strParseFormat(form),
				uint64(n), spec))
		case 'a', 'A':
			strCheckFormat(L, form, strFmtFlagsF, true)
			b.AddString(strNumber2StrX(L, form, L.CheckNumber(arg)))
		case 'e', 'E', 'f', 'g', 'G':
			/* C reads the number first here too. */
			n := L.CheckNumber(arg)
			strCheckFormat(L, form, strFmtFlagsF, true)
			b.AddString(strFormatFloat(form, n))
		case 'p':
			strCheckFormat(L, form, strFmtFlagsC, false)
			p := L.ToPointer(arg)
			s := "(null)"
			if p != 0 {
				s = fmt.Sprintf("0x%x", uint64(p))
			}
			b.AddString(fmt.Sprintf(form[:len(form)-1]+"s", s))
		case 'q':
			if len(form) > 2 { /* modifiers? */
				return L.Error2("specifier '%%q' cannot have modifiers")
			}
			strAddLiteral(L, b, arg)
		case 's':
			s := strToLString(L, arg)
			if len(form) == 2 { /* no modifiers? */
				b.AddValue() /* keep entire string */
			} else {
				if strings.IndexByte(s, 0) >= 0 {
					L.ArgError(arg, "string contains zeros")
				}
				strCheckFormat(L, form, strFmtFlagsC, true)
				if !strings.Contains(form, ".") && len(s) >= 100 {
					/* no precision and string is too long to be formatted */
					b.AddValue() /* keep entire string */
				} else { /* format the string */
					b.AddString(strFormatS(form, s))
					L.Pop(1) /* remove result from 'strToLString' */
				}
			}
		default: /* also treat cases 'pnLlh' */
			return L.Error2("invalid conversion '%s' to 'format'", form)
		}
	}
	b.PushResult()
	return 1
}

// ---------------------------------------------------------------------
// PACK/UNPACK
// ---------------------------------------------------------------------

const (
	strMaxIntSize    = 16
	strNB            = 8 /* number of bits in a character */
	strMC            = 0xff
	strSzInt         = 8 /* size of a lua_Integer */
	strPackPadByte   = 0x00
	strPackMaxAlign  = 8
	strPackMaxSize   = math.MaxInt32
	strPackAlignSize = 8
)

var strNativeLittle = binary.NativeEndian.Uint16([]byte{1, 0}) == 1

type strKOption int

const (
	strKint strKOption = iota
	strKuint
	strKfloat
	strKnumber
	strKdouble
	strKchar
	strKstring
	strKzstr
	strKpadding
	strKpaddalign
	strKnop
)

type strHeader struct {
	L        *State
	islittle bool
	maxalign int
}

func strInitHeader(L *State, h *strHeader) {
	h.L = L
	h.islittle = strNativeLittle
	h.maxalign = 1
}

func strIsDigitChar(c byte) bool { return c >= '0' && c <= '9' }

// strFmtOpts is a format string as C sees it, which is the bytes up to
// its first zero rather than all of them.
//
// 'str_pack', 'str_packsize' and 'str_unpack' walk a format with
// 'while (*fmt != '\0')', so what they answer is decided by where the
// first zero byte is: 'string.packsize("i4\0i4")' is 4 and not 8, and
// 'string.packsize("\0")' is 0 rather than a complaint about a format
// option that is the zero byte.  ('string.format' is not like this --
// C walks its format with 'strfrmt < strfrmt_end', the end being the
// length -- so that one keeps its zero bytes.)  A Go string is a
// length and some bytes, so the cut C's 'luaL_checkstring' makes for
// free has to be made here.
func strFmtOpts(s string) string {
	if k := strings.IndexByte(s, 0); k >= 0 {
		return s[:k]
	}
	return s
}

// strGetNum reads an integer numeral from the format string (getnum).
func strGetNum(fmt string, i *int, df int) int {
	if *i >= len(fmt) || !strIsDigitChar(fmt[*i]) { /* no number? */
		return df /* return default value */
	}
	a := 0
	for *i < len(fmt) && strIsDigitChar(fmt[*i]) && a <= (strPackMaxSize-9)/10 {
		a = a*10 + int(fmt[*i]-'0')
		*i++
	}
	return a
}

func strGetNumLimit(h *strHeader, fmt string, i *int, df int) int {
	sz := strGetNum(fmt, i, df)
	if sz > strMaxIntSize || sz <= 0 {
		h.L.Error2("integral size (%d) out of limits [1,%d]", sz, strMaxIntSize)
	}
	return sz
}

// strGetOption reads and classifies the next option (getoption).
func strGetOption(h *strHeader, fmt string, i *int, size *int) strKOption {
	opt := byte(0)
	if *i < len(fmt) {
		opt = fmt[*i]
	}
	*i++
	*size = 0 /* default */
	switch opt {
	case 'b':
		*size = 1
		return strKint
	case 'B':
		*size = 1
		return strKuint
	case 'h':
		*size = 2
		return strKint
	case 'H':
		*size = 2
		return strKuint
	case 'l':
		*size = 8
		return strKint
	case 'L':
		*size = 8
		return strKuint
	case 'j':
		*size = strSzInt
		return strKint
	case 'J':
		*size = strSzInt
		return strKuint
	case 'T':
		*size = strPackAlignSize
		return strKuint
	case 'f':
		*size = 4
		return strKfloat
	case 'n':
		*size = 8
		return strKnumber
	case 'd':
		*size = 8
		return strKdouble
	case 'i':
		*size = strGetNumLimit(h, fmt, i, 4)
		return strKint
	case 'I':
		*size = strGetNumLimit(h, fmt, i, 4)
		return strKuint
	case 's':
		*size = strGetNumLimit(h, fmt, i, strPackAlignSize)
		return strKstring
	case 'c':
		*size = strGetNum(fmt, i, -1)
		if *size == -1 {
			h.L.Error2("missing size for format option 'c'")
		}
		return strKchar
	case 'z':
		return strKzstr
	case 'x':
		*size = 1
		return strKpadding
	case 'X':
		return strKpaddalign
	case ' ':
		break
	case '<':
		h.islittle = true
		break
	case '>':
		h.islittle = false
		break
	case '=':
		h.islittle = strNativeLittle
		break
	case '!':
		h.maxalign = strGetNumLimit(h, fmt, i, strPackMaxAlign)
		break
	default:
		h.L.Error2("invalid format option '%c'", opt)
	}
	return strKnop
}

// strGetDetails reads, classifies and fills other details about the
// next option (getdetails).
func strGetDetails(h *strHeader, totalsize int, fmt string, i *int, psize, ntoalign *int) strKOption {
	opt := strGetOption(h, fmt, i, psize)
	align := *psize /* usually, alignment follows size */
	if opt == strKpaddalign {
		/* 'X' gets alignment from following option */
		if *i >= len(fmt) || strGetOption(h, fmt, i, &align) == strKchar || align == 0 {
			h.L.ArgError(1, "invalid next option for option 'X'")
		}
	}
	if align <= 1 || opt == strKchar { /* need no alignment? */
		*ntoalign = 0
	} else {
		if align > h.maxalign { /* enforce maximum alignment */
			align = h.maxalign
		}
		if align&(align-1) != 0 { /* not a power of 2? */
			h.L.ArgError(1, "format asks for alignment not power of 2")
		}
		*ntoalign = (align - (totalsize & (align - 1))) & (align - 1)
	}
	return opt
}

// strPackInt packs the integer 'n' with 'size' bytes and the given
// endianness (packint).
func strPackInt(buf *bytes.Buffer, n uint64, islittle bool, size int, neg bool) {
	var b [8]byte
	if islittle {
		binary.LittleEndian.PutUint64(b[:], n)
	} else {
		binary.BigEndian.PutUint64(b[:], n)
	}
	if size <= 8 {
		if islittle {
			buf.Write(b[:size])
		} else {
			buf.Write(b[8-size:])
		}
		return
	}
	pad := byte(0)
	if neg {
		pad = strMC /* negative number needs sign extension */
	}
	if islittle {
		buf.Write(b[:])
		for i := 8; i < size; i++ {
			buf.WriteByte(pad)
		}
	} else {
		for i := 0; i < size-8; i++ {
			buf.WriteByte(pad)
		}
		buf.Write(b[:])
	}
}

// strUnpackInt unpacks an integer with 'size' bytes (unpackint).
func strUnpackInt(L *State, data string, pos int, islittle bool, size, issigned int) int64 {
	var res uint64
	limit := size
	if size > strSzInt {
		limit = strSzInt
	}
	for i := limit - 1; i >= 0; i-- {
		res <<= strNB
		idx := i
		if !islittle {
			idx = size - 1 - i
		}
		if pos+idx < len(data) {
			res |= uint64(data[pos+idx])
		}
	}
	if size < strSzInt { /* real size smaller than lua_Integer? */
		if issigned != 0 { /* needs sign extension? */
			mask := uint64(1) << uint(size*strNB-1)
			res = (res ^ mask) - mask /* do sign extension */
		}
	} else if size > strSzInt { /* must check unread bytes */
		mask := 0
		if issigned != 0 && int64(res) < 0 {
			mask = strMC
		}
		for i := limit; i < size; i++ {
			idx := i
			if !islittle {
				idx = size - 1 - i
			}
			if int(data[pos+idx]) != mask {
				L.Error2("%d-byte integer does not fit into Lua Integer", size)
			}
		}
	}
	return int64(res)
}

// strPackBits writes the low 'n' bytes of 'bits' with the given
// endianness (copywithendian).
func strPackBits(buf *bytes.Buffer, bits uint64, n int, islittle bool) {
	var b [8]byte
	if islittle {
		binary.LittleEndian.PutUint64(b[:], bits)
		buf.Write(b[:n])
	} else {
		binary.BigEndian.PutUint64(b[:], bits)
		buf.Write(b[8-n:])
	}
}

// strPadBytes appends 'n' padding bytes -- the tail of a 'cN' field.
//
// C pads it one byte at a time (lstrlib.c):
//
//	while (len++ < (size_t)size)
//	  luaL_addchar(&b, LUAL_PACKPADBYTE);
//
// and 'n' can be enormous: 'string.pack("c1000000000000", 1)' asks for a
// gigabyte, and PUC-Lua spends a couple of seconds producing it.  The loop
// is written in chunks, and 'n' bytes are never materialised as one slice,
// because a straight transliteration that grows the string on every step
// would be quadratic and would never finish.
func strPadBytes(buf *bytes.Buffer, n int) {
	const chunk = 64 * 1024
	pad := make([]byte, chunk)
	for n >= chunk {
		buf.Write(pad)
		n -= chunk
	}
	if n > 0 {
		buf.Write(pad[:n])
	}
}

func strUnpackBits(data string, pos, n int, islittle bool) uint64 {
	var b [8]byte
	if islittle {
		copy(b[:n], data[pos:pos+n])
		return binary.LittleEndian.Uint64(b[:])
	}
	copy(b[8-n:], data[pos:pos+n])
	return binary.BigEndian.Uint64(b[:])
}

func strPack(L *State) int {
	fmtstr := strFmtOpts(L.CheckString(1)) /* format string */
	arg := 1                               /* current argument to pack */
	h := strHeader{}
	strInitHeader(L, &h)
	/* C pushes a nil here to separate the arguments from the string
	   buffer (lstrlib.c's str_pack).  The slot is observable: an
	   absent argument sits where that nil is, so 'string.pack("B")'
	   reports "number expected, got nil" instead of "got no value". */
	L.PushNil()
	var buf bytes.Buffer
	totalsize := 0 /* accumulate total size of result */
	for i := 0; i < len(fmtstr); {
		var size, ntoalign int
		opt := strGetDetails(&h, totalsize, fmtstr, &i, &size, &ntoalign)
		totalsize += ntoalign + size
		for j := 0; j < ntoalign; j++ {
			buf.WriteByte(strPackPadByte) /* fill alignment */
		}
		arg++
		switch opt {
		case strKint: /* signed integers */
			n := L.CheckInteger(arg)
			if size < strSzInt { /* need overflow check? */
				lim := int64(1) << uint(size*strNB-1)
				L.ArgCheck(-lim <= n && n < lim, arg, "integer overflow")
			}
			strPackInt(&buf, uint64(n), h.islittle, size, n < 0)
		case strKuint: /* unsigned integers */
			n := L.CheckInteger(arg)
			if size < strSzInt { /* need overflow check? */
				L.ArgCheck(uint64(n) < uint64(1)<<uint(size*strNB), arg, "unsigned overflow")
			}
			strPackInt(&buf, uint64(n), h.islittle, size, false)
		case strKfloat: /* C float */
			f := float32(L.CheckNumber(arg))
			strPackBits(&buf, uint64(math.Float32bits(f)), 4, h.islittle)
		case strKnumber, strKdouble: /* Lua float / C double */
			f := L.CheckNumber(arg)
			strPackBits(&buf, math.Float64bits(f), 8, h.islittle)
		case strKchar: /* fixed-size string */
			s := L.CheckString(arg)
			L.ArgCheck(len(s) <= size, arg, "string longer than given size")
			buf.WriteString(s)             /* add string */
			strPadBytes(&buf, size-len(s)) /* pad extra space */
		case strKstring: /* strings with length count */
			s := L.CheckString(arg)
			L.ArgCheck(size >= strPackAlignSize || len(s) < 1<<uint(size*strNB),
				arg, "string length does not fit in given size")
			strPackInt(&buf, uint64(len(s)), h.islittle, size, false) /* pack length */
			buf.WriteString(s)
			totalsize += len(s)
		case strKzstr: /* zero-terminated string */
			s := L.CheckString(arg)
			L.ArgCheck(strings.IndexByte(s, 0) < 0, arg, "string contains zeros")
			buf.WriteString(s)
			buf.WriteByte(0) /* add zero at the end */
			totalsize += len(s) + 1
		case strKpadding:
			buf.WriteByte(strPackPadByte)
			fallthrough
		case strKpaddalign, strKnop:
			arg-- /* undo increment */
		}
	}
	L.PushString(buf.String())
	return 1
}

func strPackSize(L *State) int {
	fmtstr := strFmtOpts(L.CheckString(1)) /* format string */
	h := strHeader{}
	strInitHeader(L, &h)
	totalsize := 0 /* accumulate total size of result */
	for i := 0; i < len(fmtstr); {
		var size, ntoalign int
		opt := strGetDetails(&h, totalsize, fmtstr, &i, &size, &ntoalign)
		L.ArgCheck(opt != strKstring && opt != strKzstr, 1, "variable-length format")
		size += ntoalign /* total space used by option */
		L.ArgCheck(totalsize <= strPackMaxSize-size, 1, "format result too large")
		totalsize += size
	}
	L.PushInteger(int64(totalsize))
	return 1
}

func strUnpack(L *State) int {
	fmtstr := strFmtOpts(L.CheckString(1))
	data := L.CheckString(2)
	ld := len(data)
	pos := strPosRelatI(L.OptInteger(3, 1), ld) - 1
	h := strHeader{}
	L.ArgCheck(pos <= ld, 3, "initial position out of string")
	strInitHeader(L, &h)
	n := 0 /* number of results */
	for i := 0; i < len(fmtstr); {
		var size, ntoalign int
		opt := strGetDetails(&h, pos, fmtstr, &i, &size, &ntoalign)
		L.ArgCheck(ntoalign+size <= ld-pos, 2, "data string too short")
		pos += ntoalign /* skip alignment */
		/* stack space for item + next position */
		L.CheckStack2(2, "too many results")
		n++
		switch opt {
		case strKint, strKuint:
			L.PushInteger(strUnpackInt(L, data, pos, h.islittle, size, strBoolToInt(opt == strKint)))
		case strKfloat:
			f := math.Float32frombits(uint32(strUnpackBits(data, pos, 4, h.islittle)))
			L.PushNumber(float64(f))
		case strKnumber, strKdouble:
			f := math.Float64frombits(strUnpackBits(data, pos, 8, h.islittle))
			L.PushNumber(f)
		case strKchar:
			L.PushString(data[pos : pos+size])
		case strKstring:
			/* C keeps this length in a 'size_t' and compares it as
			   one: 'size_t len = (size_t)unpackint(...)', and then
			   'len <= ld - pos - size'.  A length read as eight
			   bytes of 0xff is 2^64-1 there and -1 here, and -1 is
			   less than whatever is left of the string -- so the
			   comparison was the wrong way round and the slice
			   below it went out of range.  It is unsigned, which
			   is what C's 'size_t' is. */
			len2 := uint64(strUnpackInt(L, data, pos, h.islittle, size, 0))
			L.ArgCheck(len2 <= uint64(ld-pos-size), 2, "data string too short")
			L.PushString(data[pos+size : pos+size+int(len2)])
			pos += int(len2) /* skip string */
		case strKzstr:
			end := strings.IndexByte(data[pos:], 0)
			if end < 0 {
				end = len(data) - pos
			}
			L.ArgCheck(pos+end < ld, 2, "unfinished string for format 'z'")
			L.PushString(data[pos : pos+end])
			pos += end + 1 /* skip string plus final '\0' */
		case strKpaddalign, strKpadding, strKnop:
			n-- /* undo increment */
		}
		pos += size
	}
	L.PushInteger(int64(pos) + 1) /* next position */
	return n + 1
}

func strBoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

var strLib = []Reg{
	{"byte", strByte},
	{"char", strChar},
	{"dump", strDump},
	{"find", strFind},
	{"format", strFormat},
	{"gmatch", strGmatch},
	{"gsub", strGsub},
	{"len", strLen},
	{"lower", strLower},
	{"match", strMatchLib},
	{"rep", strRep},
	{"reverse", strReverse},
	{"sub", strSub},
	{"upper", strUpper},
	{"pack", strPack},
	{"packsize", strPackSize},
	{"unpack", strUnpack},
}

var strMetaMethods = []Reg{
	{"__add", strArithAdd},
	{"__sub", strArithSub},
	{"__mul", strArithMul},
	{"__mod", strArithMod},
	{"__pow", strArithPow},
	{"__div", strArithDiv},
	{"__idiv", strArithIDiv},
	{"__unm", strArithUnm},
	{"__index", nil}, /* placeholder */
}

// strCreateMetaTable creates the metatable shared by all strings
// (createmetatable).
func strCreateMetaTable(L *State) {
	/* table to be metatable for strings */
	L.CreateTable(0, len(strMetaMethods))
	L.SetFuncs(strMetaMethods, 0)
	L.PushString("")   /* dummy string */
	L.PushValue(-2)    /* copy table */
	L.SetMetaTable(-2) /* set table as metatable for strings */
	L.Pop(1)           /* pop dummy string */
	L.PushValue(-2)    /* get string library */
	L.SetField(-2, "__index")
	L.Pop(1) /* pop metatable */
}

// OpenString opens the string library (luaopen_string).
func OpenString(L *State) int {
	L.NewLib(strLib)
	strCreateMetaTable(L)
	return 1
}
