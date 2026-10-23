// Translation of llex.h / llex.c (PUC-Lua 5.3.4).
// Fully aligned to lua-5.3.4/src/llex.c + llex.h (not 5.4 token2str / read_numeral /
// skip_sep return encoding).
//
// Adaptations (same rules as the rest of the package):
//   - no GC: luaC_fix is gone; reserved words are anchored by a package-level
//     map instead of by TString.extra (which would be lost if the intern
//     table is flushed);
//   - the ZIO stream becomes a plain byte slice plus a position;
//   - Mbuffer becomes a []byte, so NUL terminators are unnecessary;
//   - luaX_newstring stores t[str]=str (5.4-shaped / addk-safe) instead of
//     C 5.3.4 t[str]=true + keyfromval; see NOTES.
package core

import (
	"fmt"
)

// EOZ marks the end of the input stream (lzio.h).
const EOZ = -1

const maxInt = int(^uint(0) >> 1)

// FIRST_RESERVED — llex.h 5.3.4 (#define FIRST_RESERVED 257).
const FIRST_RESERVED = 257

const maxSize = int(^uint(0) >> 1) /* llimits.h MAX_SIZE ≈ max int */

// Token codes. WARNING: if you change the order of this enumeration,
// grep "ORDER RESERVED".
const (
	/* terminal symbols denoted by reserved words */
	TK_AND = FIRST_RESERVED + iota
	TK_BREAK
	TK_DO
	TK_ELSE
	TK_ELSEIF
	TK_END
	TK_FALSE
	TK_FOR
	TK_FUNCTION
	TK_GOTO
	TK_IF
	TK_IN
	TK_LOCAL
	TK_NIL
	TK_NOT
	TK_OR
	TK_REPEAT
	TK_RETURN
	TK_THEN
	TK_TRUE
	TK_UNTIL
	TK_WHILE
	/* other terminal symbols */
	TK_IDIV
	TK_CONCAT
	TK_DOTS
	TK_EQ
	TK_GE
	TK_LE
	TK_NE
	TK_SHL
	TK_SHR
	TK_DBCOLON
	TK_EOS
	TK_FLT
	TK_INT
	TK_NAME
	TK_STRING
)

// NUM_RESERVED is the number of reserved words.
const NUM_RESERVED = TK_WHILE - FIRST_RESERVED + 1

// LUA_ENV is the name of the environment upvalue of every chunk.
const LUA_ENV = "_ENV"

/* ORDER RESERVED */
var luaX_tokens = []string{
	"and", "break", "do", "else", "elseif",
	"end", "false", "for", "function", "goto", "if",
	"in", "local", "nil", "not", "or", "repeat",
	"return", "then", "true", "until", "while",
	"//", "..", "...", "==", ">=", "<=", "~=",
	"<<", ">>", "::", "<eof>",
	"<number>", "<integer>", "<name>", "<string>",
}

// reservedTokens maps a reserved word to its token. PUC-Lua stores the
// index in TString.extra and pins the string with luaC_fix; we have no GC
// and the intern table may be flushed, so a global map is the safe
// equivalent.
var reservedTokens = map[string]int{}

// SemInfo is the semantic information of a token (llex.h).
type SemInfo struct {
	r  float64
	i  int64
	ts *TString
}

// Token is a lexical token (llex.h).
type Token struct {
	token   int
	seminfo SemInfo
}

// LexState is the state of the lexer plus the state of the parser shared
// by all functions (llex.h).
type LexState struct {
	current    int   // current character (charint)
	linenumber int   // input line counter
	lastline   int   // line of last token 'consumed'
	t          Token // current token
	lookahead  Token // look ahead token
	fs         *FuncState
	L          *State
	src        []byte // input stream (replaces ZIO)
	pos        int
	// fill pulls the next piece of the stream; it is what C's ZIO does
	// with a reader function (lzio.c).  nil means the whole chunk is
	// already in 'src'.  The pieces are asked for one at a time and only
	// when the lexer runs out, so a chunk that fails to parse stops the
	// reader instead of draining it.
	fill   func() []byte
	buff   []byte // buffer for tokens (replaces Mbuffer)
	h      *Table // to avoid collection/reuse strings
	dyd    *Dyndata
	source *TString // current source name
	envn   *TString // environment variable name
}

// --- input stream (lzio.h) ---

// next reads the next character from the input stream.
func (ls *LexState) next() {
	if ls.pos >= len(ls.src) {
		ls.pull()
	}
	if ls.pos < len(ls.src) {
		ls.current = int(ls.src[ls.pos])
		ls.pos++
	} else {
		ls.current = EOZ
	}
}

// pull asks the reader for the next piece of the stream, dropping what
// has been consumed already (lzio.c's luaZ_fill).  An empty piece, or a
// nil one, is the end of the stream.
func (ls *LexState) pull() {
	if ls.fill == nil {
		return
	}
	piece := ls.fill()
	if len(piece) == 0 {
		ls.fill = nil /* end of stream */
		return
	}
	if ls.pos > 0 {
		ls.src = append(ls.src[ls.pos:], piece...)
		ls.pos = 0
	} else {
		ls.src = append(ls.src, piece...)
	}
}

func currIsNewline(ls *LexState) bool {
	return ls.current == '\n' || ls.current == '\r'
}

// --- token buffer (lzio.h) ---

func (ls *LexState) resetbuffer() { ls.buff = ls.buff[:0] }

func (ls *LexState) buffer() string { return string(ls.buff) }

func (ls *LexState) save(c int) {
	/* llex.c: luaZ_sizebuffer >= MAX_SIZE/2 → "lexical element too long" */
	if len(ls.buff) >= maxSize/2 {
		lexerror(ls, "lexical element too long", 0)
	}
	ls.buff = append(ls.buff, byte(c))
}

// saveAndNext saves the current character and reads the next one.
func (ls *LexState) saveAndNext() { ls.save(ls.current); ls.next() }

func (ls *LexState) buffRemove(n int) {
	if n > len(ls.buff) {
		n = len(ls.buff)
	}
	ls.buff = ls.buff[:len(ls.buff)-n]
}

// --- lctype.h ---

func lislalpha(c int) bool {
	if c == EOZ {
		return false
	}
	r := rune(c)
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func lislalnum(c int) bool {
	if c == EOZ {
		return false
	}
	return lislalpha(c) || lisdigit(c)
}

func lisdigit(c int) bool {
	return c >= '0' && c <= '9'
}

func isxdigit(c int) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// lisprint reports whether c is a printable ASCII character (lctype.h).
// Used by pushfString '%c' (5.3.4 luaO_pushvfstring in lobject.go).
func lisprint(c int) bool {
	return c >= ' ' && c <= '~'
}

func lisspaceByte(c int) bool {
	switch byte(c) {
	case ' ', '\t', '\n', '\v', '\f', '\r':
		return true
	}
	return false
}

// ---------------------------------------------------------------------
// llex.c
// ---------------------------------------------------------------------

// luaX_init creates the reserved words (llex.c).
func luaX_init(L *State) {
	for i := 0; i < NUM_RESERVED; i++ {
		luaS_new(L, luaX_tokens[i])
		reservedTokens[luaX_tokens[i]] = FIRST_RESERVED + i
	}
}

// isreserved reports whether ts is a reserved word, returning its token.
func isreservedToken(ts *TString) (int, bool) {
	tk, ok := reservedTokens[ts.val]
	return tk, ok
}

// luaX_token2str returns a printable representation of a token (llex.c 5.3.4).
// Always "'%c'" for token < FIRST_RESERVED (not 5.4's lisprint / "'<\%d>'" branch).
// Non-printable escaping lives in pushfString '%c' (luaO_pushvfstring), as in C.
func luaX_token2str(ls *LexState, token int) string {
	if token < FIRST_RESERVED { /* single-byte symbols? */
		return pushfString(ls.L, "'%c'", token).val
	}
	s := luaX_tokens[token-FIRST_RESERVED]
	if token < TK_EOS { /* fixed format (symbols and reserved words)? */
		return pushfString(ls.L, "'%s'", s).val
	}
	return s /* names, strings, and numerals */
}

func txtToken(ls *LexState, token int) string {
	switch token {
	case TK_NAME, TK_STRING, TK_FLT, TK_INT:
		return "'" + ls.buffer() + "'"
	default:
		return luaX_token2str(ls, token)
	}
}

// lexerror raises a syntax error (llex.c).
func lexerror(ls *LexState, msg string, token int) {
	msg = luaG_addinfo(ls.L, msg, ls.source, ls.linenumber)
	if token != 0 {
		msg = fmt.Sprintf("%s near %s", msg, txtToken(ls, token))
		ls.L.PushString(msg)
	}
	luaD_throw(ls.L, LUA_ERRSYNTAX)
}

// luaX_syntaxerror raises a syntax error at the current token (llex.c).
func luaX_syntaxerror(ls *LexState, msg string) {
	lexerror(ls, msg, ls.t.token)
}

// luaX_newstring creates a new string and anchors it in the scanner's table
// so that it will not be reused until the end of the compilation (llex.c).
func luaX_newstring(ls *LexState, str string) *TString {
	L := ls.L
	ts := luaS_newlstr(L, str)
	/* PUC-Lua takes the string back out of the *key* of the entry, not
	   out of its value (llex.c: keystrval), and it has to: 'addk' turns
	   the value into the index of a constant the moment the string is
	   used as one (lcode.c), so reading the value here would answer an
	   integer for every string that has been a constant already. Every
	   later occurrence of that literal would then be a new object, and
	   two equal long literals in one chunk would stop being the same
	   string -- which is what 'literals.lua' asserts. */
	if saved := luaH_getstrkey(L, ls.h, ts); saved != nil {
		return saved
	}
	/* not in use yet: t[string] = string, to anchor it for the rest of
	   the compilation */
	var key Value
	setsvalue(&key, ts)
	*luaH_set(L, ls.h, &key) = key
	/* table is not a metatable, so it does not need to invalidate cache */
	return ts
}

// inclinenumber increments the line number and skips a newline sequence
// (any of \n, \r, \n\r, or \r\n) (llex.c).
func (ls *LexState) inclinenumber() {
	old := ls.current
	ls.next() /* skip '\n' or '\r' */
	if currIsNewline(ls) && ls.current != old {
		ls.next() /* skip '\n\r' or '\r\n' */
	}
	ls.linenumber++
	if ls.linenumber >= maxInt {
		lexerror(ls, "chunk has too many lines", 0)
	}
}

// luaX_setinput initializes the lexer for a new chunk (llex.c).
func luaX_setinput(L *State, ls *LexState, src []byte, source *TString, firstchar int, fill func() []byte) {
	ls.t.token = 0
	ls.L = L
	ls.current = firstchar
	ls.lookahead.token = TK_EOS /* no look-ahead token */
	ls.src = src
	ls.pos = 0
	ls.fill = fill
	ls.fs = nil
	ls.linenumber = 1
	ls.lastline = 1
	ls.source = source
	ls.envn = luaS_new(L, LUA_ENV) /* get env name */
	ls.resetbuffer()
}

// ---------------------------------------------------------------------
// LEXICAL ANALYZER
// ---------------------------------------------------------------------

// checkNext1 checks whether the current char is c, consuming it (llex.c).
func (ls *LexState) checkNext1(c int) bool {
	if ls.current == c {
		ls.next()
		return true
	}
	return false
}

// checkNext2 checks whether the current char is in set (two chars) and
// saves it (llex.c).
func (ls *LexState) checkNext2(set string) bool {
	if ls.current == int(set[0]) || ls.current == int(set[1]) {
		ls.saveAndNext()
		return true
	}
	return false
}

// read_numeral reads a numeral (llex.c 5.3.4).
// Quite liberal: luaO_str2num rejects ill-formed numerals.
// Loop shape is 5.3.4 (two separate ifs per iteration), not 5.4 else-if.
// Does not force-save a trailing letter (5.4 does).
func read_numeral(ls *LexState, seminfo *SemInfo) int {
	expo := "Ee"
	first := ls.current
	ls.saveAndNext()
	if first == '0' && ls.checkNext2("xX") { /* hexadecimal? */
		expo = "Pp"
	}
	/* 5.3.4: after optional expo/sign, separate digit/'.' check in same iteration
	   (not 5.4 else-if). Second expo that is not a digit → break without consume. */
	for {
		if ls.checkNext2(expo) { /* exponent part? */
			ls.checkNext2("-+") /* optional exponent sign */
		}
		if isxdigit(ls.current) {
			ls.saveAndNext()
		} else if ls.current == '.' {
			ls.saveAndNext()
		} else {
			break
		}
	}
	/* 5.3.4: no trailing lislalpha force-save (5.4 only). */
	var obj Value
	if !luaO_str2num(ls.buffer(), &obj) { /* format error? */
		lexerror(ls, "malformed number", TK_FLT)
	}
	if ttisinteger(&obj) {
		seminfo.i = ivalue(&obj)
		return TK_INT
	}
	seminfo.r = fltvalue(&obj)
	return TK_FLT
}

// skipSep skips a sequence '[=*[' or ']=*]'. If well formed, return its
// number of '='s; otherwise return a negative number (-1 iff there are no
// '='s after the initial bracket) — llex.c 5.3.4 (not 5.4 count+2/1/0).
func (ls *LexState) skipSep() int {
	count := 0
	s := ls.current
	ls.saveAndNext()
	for ls.current == '=' {
		ls.saveAndNext()
		count++
	}
	if ls.current == s {
		return count
	}
	return (-count) - 1
}

// read_long_string reads a long string or a long comment (llex.c).
func read_long_string(ls *LexState, seminfo *SemInfo, sep int) {
	line := ls.linenumber /* initial line (for error message) */
	ls.saveAndNext()      /* skip 2nd '[' */
	/* string starts with a newline? */
	if currIsNewline(ls) {
		ls.inclinenumber() /* skip it */
	}
endloop:
	for {
		switch ls.current {
		case EOZ: /* error */
			what := "string"
			if seminfo == nil {
				what = "comment"
			}
			msg := fmt.Sprintf("unfinished long %s (starting at line %d)", what, line)
			lexerror(ls, msg, TK_EOS)
		case ']':
			if ls.skipSep() == sep {
				ls.saveAndNext() /* skip 2nd ']' */
				break endloop
			}
		case '\n', '\r':
			ls.save('\n')
			ls.inclinenumber()
			if seminfo == nil {
				ls.resetbuffer() /* avoid wasting space */
			}
		default:
			if seminfo != nil {
				ls.saveAndNext()
			} else {
				ls.next()
			}
		}
	}
	if seminfo != nil {
		/* C: buffer+(2+sep), len-2*(2+sep) */
		seminfo.ts = luaX_newstring(ls, string(ls.buff[(2+sep):len(ls.buff)-(2+sep)]))
	}
}

// esccheck raises an error when c is false (llex.c).
func esccheck(ls *LexState, c bool, msg string) {
	if !c {
		if ls.current != EOZ {
			ls.saveAndNext() /* add current to buffer for error message */
		}
		lexerror(ls, msg, TK_STRING)
	}
}

func gethexa(ls *LexState) int {
	ls.saveAndNext()
	esccheck(ls, isxdigit(ls.current), "hexadecimal digit expected")
	return luaO_hexavalue(byte(ls.current))
}

func readhexaesc(ls *LexState) int {
	r := gethexa(ls)
	r = (r << 4) + gethexa(ls)
	ls.buffRemove(2) /* remove saved chars from buffer */
	return r
}

func readutf8esc(ls *LexState) uint32 {
	var r uint32
	i := 4 /* chars to be removed: '\', 'u', '{', and first digit */
	ls.saveAndNext()
	esccheck(ls, ls.current == '{', "missing '{'")
	r = uint32(gethexa(ls)) /* must have at least one digit */
	for {
		ls.saveAndNext()
		if !isxdigit(ls.current) {
			break
		}
		i++
		r = (r << 4) + uint32(luaO_hexavalue(byte(ls.current)))
		/* 5.3.4: MAXUNICODE is 0x10FFFF (5.4 raised the ceiling). */
		esccheck(ls, r <= 0x10FFFF, "UTF-8 value too large")
	}
	esccheck(ls, ls.current == '}', "missing '}'")
	ls.next()        /* skip '}' */
	ls.buffRemove(i) /* remove saved chars from buffer */
	return r
}

func utf8esc(ls *LexState) {
	buff := luaO_utf8esc(readutf8esc(ls))
	for _, c := range buff { /* add 'buff' to string */
		ls.save(int(c))
	}
}

func readdecesc(ls *LexState) int {
	i := 0
	r := 0 /* result accumulator */
	for i = 0; i < 3 && lisdigit(ls.current); i++ {
		r = 10*r + ls.current - '0'
		ls.saveAndNext()
	}
	esccheck(ls, r <= 255, "decimal escape too large")
	ls.buffRemove(i) /* remove read digits from buffer */
	return r
}

// read_string reads a short literal string (llex.c).
func read_string(ls *LexState, del int, seminfo *SemInfo) {
	ls.saveAndNext() /* keep delimiter (for error messages) */
	for ls.current != del {
		switch ls.current {
		case EOZ:
			lexerror(ls, "unfinished string", TK_EOS)
		case '\n', '\r':
			lexerror(ls, "unfinished string", TK_STRING)
		case '\\': /* escape sequences */
			c := 0 /* final character to be saved */
			/* noSave: nothing to save; readSave: read the escape char then
			   save c; onlySave: the escape char was already consumed */
			const (
				noSave   = 0
				readSave = 1
				onlySave = 2
			)
			mode := noSave
			ls.saveAndNext() /* keep '\\' for error messages */
			switch ls.current {
			case 'a':
				c = '\a'
				mode = readSave
			case 'b':
				c = '\b'
				mode = readSave
			case 'f':
				c = '\f'
				mode = readSave
			case 'n':
				c = '\n'
				mode = readSave
			case 'r':
				c = '\r'
				mode = readSave
			case 't':
				c = '\t'
				mode = readSave
			case 'v':
				c = '\v'
				mode = readSave
			case 'x':
				c = readhexaesc(ls)
				mode = readSave
			case 'u':
				utf8esc(ls)
				mode = noSave
			case '\n', '\r':
				ls.inclinenumber()
				c = '\n'
				mode = onlySave
			case '\\', '"', '\'':
				c = ls.current
				mode = readSave
			case EOZ:
				mode = noSave /* will raise an error next loop */
			case 'z': /* zap following span of spaces */
				ls.buffRemove(1) /* remove '\\' */
				ls.next()        /* skip the 'z' */
				for lisspaceByte(ls.current) {
					if currIsNewline(ls) {
						ls.inclinenumber()
					} else {
						ls.next()
					}
				}
				mode = noSave
			default:
				esccheck(ls, lisdigit(ls.current), "invalid escape sequence")
				c = readdecesc(ls) /* digital escape '\ddd' */
				mode = onlySave
			}
			if mode == readSave {
				ls.next()
			}
			if mode != noSave {
				ls.buffRemove(1) /* remove '\\' */
				ls.save(c)
			}
		default:
			ls.saveAndNext()
		}
	}
	ls.saveAndNext() /* skip delimiter */
	s := ls.buffer()
	seminfo.ts = luaX_newstring(ls, s[1:len(s)-1])
}

// llex reads the next token (llex.c).
func llex(ls *LexState, seminfo *SemInfo) int {
	ls.resetbuffer()
	for {
		switch ls.current {
		case '\n', '\r': /* line breaks */
			ls.inclinenumber()
		case ' ', '\f', '\t', '\v': /* spaces */
			ls.next()
		case '-': /* '-' or '--' (comment) */
			ls.next()
			if ls.current != '-' {
				return '-'
			}
			/* else is a comment */
			ls.next()
			if ls.current == '[' { /* long comment? */
				sep := ls.skipSep()
				ls.resetbuffer() /* 'skipSep' may dirty the buffer */
				if sep >= 0 {
					read_long_string(ls, nil, sep) /* skip long comment */
					ls.resetbuffer()               /* previous call may dirty the buffer */
					continue
				}
			}
			/* else short comment */
			for !currIsNewline(ls) && ls.current != EOZ {
				ls.next() /* skip until end of line (or end of file) */
			}
		case '[': /* long string or simply '[' */
			sep := ls.skipSep()
			if sep >= 0 {
				read_long_string(ls, seminfo, sep)
				return TK_STRING
			} else if sep != -1 { /* '[=...' missing second bracket */
				lexerror(ls, "invalid long string delimiter", TK_STRING)
			}
			return '['
		case '=':
			ls.next()
			if ls.checkNext1('=') {
				return TK_EQ /* '==' */
			}
			return '='
		case '<':
			ls.next()
			if ls.checkNext1('=') {
				return TK_LE /* '<=' */
			} else if ls.checkNext1('<') {
				return TK_SHL /* '<<' */
			}
			return '<'
		case '>':
			ls.next()
			if ls.checkNext1('=') {
				return TK_GE /* '>=' */
			} else if ls.checkNext1('>') {
				return TK_SHR /* '>>' */
			}
			return '>'
		case '/':
			ls.next()
			if ls.checkNext1('/') {
				return TK_IDIV /* '//' */
			}
			return '/'
		case '~':
			ls.next()
			if ls.checkNext1('=') {
				return TK_NE /* '~=' */
			}
			return '~'
		case ':':
			ls.next()
			if ls.checkNext1(':') {
				return TK_DBCOLON /* '::' */
			}
			return ':'
		case '"', '\'': /* short literal strings */
			read_string(ls, ls.current, seminfo)
			return TK_STRING
		case '.': /* '.', '..', '...', or number */
			ls.saveAndNext()
			if ls.checkNext1('.') {
				if ls.checkNext1('.') {
					return TK_DOTS /* '...' */
				}
				return TK_CONCAT /* '..' */
			} else if !lisdigit(ls.current) {
				return '.'
			}
			return read_numeral(ls, seminfo)
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			return read_numeral(ls, seminfo)
		case EOZ:
			return TK_EOS
		default:
			if lislalpha(ls.current) { /* identifier or reserved word? */
				for {
					ls.saveAndNext()
					if !lislalnum(ls.current) {
						break
					}
				}
				ts := luaX_newstring(ls, ls.buffer())
				seminfo.ts = ts
				if tk, ok := isreservedToken(ts); ok { /* reserved word? */
					return tk
				}
				return TK_NAME
			}
			/* single-char tokens ('+', '*', '%', '{', '}', ...) */
			c := ls.current
			ls.next()
			return c
		}
	}
}

// luaX_next reads the next token into ls.t (llex.c).
func luaX_next(ls *LexState) {
	ls.lastline = ls.linenumber
	if ls.lookahead.token != TK_EOS { /* is there a look-ahead token? */
		ls.t = ls.lookahead /* use this one */
		ls.lookahead.token = TK_EOS
	} else {
		ls.t.token = llex(ls, &ls.t.seminfo) /* read next token */
	}
}

// luaX_lookahead reads one token ahead without consuming the current one
// (llex.c).
func luaX_lookahead(ls *LexState) int {
	ls.lookahead.token = llex(ls, &ls.lookahead.seminfo)
	return ls.lookahead.token
}
