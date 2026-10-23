// Translation of llex.h / llex.c (PUC-Lua 5.4.9).
// lctype.h / lctype.c are folded here (lexer is the main consumer of
// charint ctype tests; lobject.go keeps separate byte helpers for
// numeral parsing).
//
// Intentional Go diffs (KEEP — do not "fix" to C shapes):
//   - Go GC: luaC_fix / luaC_checkGC omitted. Reserved words are also
//     recorded in reservedTokens so they survive string-table flush()
//     (TString.extra alone would be lost on re-intern).
//   - ZIO → []byte + pos + optional fill (lzio reader stand-in).
//   - Mbuffer → []byte (no NUL terminators; buffRemove/reslice instead).
package core

// EOZ marks the end of the input stream (lzio.h).
const EOZ = -1

const maxInt = int(^uint(0) >> 1)

// maxSize stand-in for llimits.h MAX_SIZE (save: "lexical element too long").
const maxSize = int(^uint(0) >> 1)

/*
** Single-char tokens (terminal symbols) are represented by their own
** numeric code. Other tokens start at the following value.
** C: #define FIRST_RESERVED (UCHAR_MAX + 1)
 */
const FIRST_RESERVED = 256

/*
* WARNING: if you change the order of this enumeration,
* grep "ORDER RESERVED"
 */
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

/* number of reserved words */
const NUM_RESERVED = TK_WHILE - FIRST_RESERVED + 1

/* C: #if !defined(LUA_ENV) #define LUA_ENV "_ENV" */
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
// index in TString.extra and pins the string with luaC_fix; after a Go
// intern-table flush a re-interned short has extra==0, so the map is the
// durable authority (FLAG / intentional Go-GC).
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
	current    int /* current character (charint) */
	linenumber int /* input line counter */
	lastline   int /* line of last token 'consumed' */
	t          Token
	lookahead  Token
	fs         *FuncState
	L          *State
	src        []byte /* input stream (replaces ZIO *z) */
	pos        int
	// fill pulls the next piece of the stream; stand-in for ZIO's reader
	// (lzio.c). nil means the whole chunk is already in 'src'.
	fill   func() []byte
	buff   []byte   /* buffer for tokens (replaces Mbuffer *buff) */
	h      *Table   /* to avoid collection/reuse strings */
	dyd    *Dyndata /* dynamic structures used by the parser */
	source *TString /* current source name */
	envn   *TString /* environment variable name */
}

// ---------------------------------------------------------------------
// input stream (lzio.h next / zgetc)
// ---------------------------------------------------------------------

// next reads the next character from the input stream.
// C: #define next(ls) (ls->current = zgetc(ls->z))
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

// pull asks the reader for the next piece (lzio.c luaZ_fill).
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

// C: #define currIsNewline(ls) (ls->current == '\n' || ls->current == '\r')
func currIsNewline(ls *LexState) bool {
	return ls.current == '\n' || ls.current == '\r'
}

// ---------------------------------------------------------------------
// token buffer (lzio.h Mbuffer helpers)
// ---------------------------------------------------------------------

func (ls *LexState) resetbuffer() { ls.buff = ls.buff[:0] }

func (ls *LexState) buffer() string { return string(ls.buff) }

// save appends c to the token buffer (llex.c).
func (ls *LexState) save(c int) {
	/* C: if (luaZ_sizebuffer(b) >= MAX_SIZE/2) → "lexical element too long" */
	if len(ls.buff) >= maxSize/2 {
		lexerror(ls, "lexical element too long", 0)
	}
	ls.buff = append(ls.buff, byte(c))
}

// C: #define save_and_next(ls) (save(ls, ls->current), next(ls))
func (ls *LexState) saveAndNext() { ls.save(ls.current); ls.next() }

// buffRemove is luaZ_buffremove (lzio.h).
func (ls *LexState) buffRemove(n int) {
	if n > len(ls.buff) {
		n = len(ls.buff)
	}
	ls.buff = ls.buff[:len(ls.buff)-n]
}

// ---------------------------------------------------------------------
// lctype.h / lctype.c (ASCII LUA_USE_CTYPE==0; NONA default 0 — no LUA_UCID)
// ---------------------------------------------------------------------

const (
	ALPHABIT  = 0
	DIGITBIT  = 1
	PRINTBIT  = 2
	SPACEBIT  = 3
	XDIGITBIT = 4
)

func MASK(b int) byte { return 1 << uint(b) }

// testprop: C #define testprop(c,p) (luai_ctype_[(c)+1] & (p))
// Index +1 allows EOZ (-1).
func testprop(c int, p byte) bool {
	if c < EOZ || c > 255 {
		return false
	}
	return luai_ctype_[c+1]&p != 0
}

func lislalpha(c int) bool { return testprop(c, MASK(ALPHABIT)) }
func lislalnum(c int) bool {
	return testprop(c, MASK(ALPHABIT)|MASK(DIGITBIT))
}
func lisdigit(c int) bool { return testprop(c, MASK(DIGITBIT)) }
func lisprint(c int) bool { return testprop(c, MASK(PRINTBIT)) }

// lisspaceChar is lispace for charint (avoids clash with lobject.lisspace(byte)).
func lisspaceChar(c int) bool { return testprop(c, MASK(SPACEBIT)) }

// lisxdigitChar is C lisxdigit for charint (avoids clash with lobject.lisxdigit(byte)).
func lisxdigitChar(c int) bool { return testprop(c, MASK(XDIGITBIT)) }

// ltolower matches lctype.h (ASCII): enough for alphabetic chars and '.'.
func ltolower(c int) int {
	return c | ('A' ^ 'a')
}

// NONA: C lctype.c — 0x01 if LUA_UCID (Unicode identifiers), else 0x00 (default).
const NONA byte = 0x00

// luai_ctype_: one entry for each character and for -1 (EOZ). Matches lctype.c.
var luai_ctype_ = [255 + 2]byte{ /* UCHAR_MAX+2 */
	0x00,                                           /* EOZ */
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, /* 0. */
	0x00, 0x08, 0x08, 0x08, 0x08, 0x08, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, /* 1. */
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x0c, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, /* 2. */
	0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
	0x16, 0x16, 0x16, 0x16, 0x16, 0x16, 0x16, 0x16, /* 3. */
	0x16, 0x16, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
	0x04, 0x15, 0x15, 0x15, 0x15, 0x15, 0x15, 0x05, /* 4. */
	0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
	0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, /* 5. */
	0x05, 0x05, 0x05, 0x04, 0x04, 0x04, 0x04, 0x05,
	0x04, 0x15, 0x15, 0x15, 0x15, 0x15, 0x15, 0x05, /* 6. */
	0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
	0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, /* 7. */
	0x05, 0x05, 0x05, 0x04, 0x04, 0x04, 0x04, 0x00,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* 8. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* 9. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* a. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* b. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	0x00, 0x00, NONA, NONA, NONA, NONA, NONA, NONA, /* c. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* d. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA, /* e. */
	NONA, NONA, NONA, NONA, NONA, NONA, NONA, NONA,
	NONA, NONA, NONA, NONA, NONA, 0x00, 0x00, 0x00, /* f. */
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// ---------------------------------------------------------------------
// llex.c
// ---------------------------------------------------------------------

// luaX_init creates the reserved words (llex.c).
// FLAG: luaC_fix omitted (Go GC); reservedTokens survives intern flush.
func luaX_init(L *State) {
	e := luaS_new(L, LUA_ENV) /* create env name */
	_ = e
	/* C: luaC_fix(L, obj2gco(e)); — never collect this name */
	for i := 0; i < NUM_RESERVED; i++ {
		ts := luaS_new(L, luaX_tokens[i])
		/* C: luaC_fix(L, obj2gco(ts)); — reserved words are never collected */
		ts.extra = byte(i + 1) /* reserved word */
		reservedTokens[luaX_tokens[i]] = FIRST_RESERVED + i
	}
}

// isreserved is C #define isreserved(s) ((s)->tt == LUA_VSHRSTR && (s)->extra > 0)
func isreserved(s *TString) bool {
	return s.tt == LUA_VSHRSTR && s.extra > 0
}

// reservedToken returns the token for a reserved word.
// Prefers TString.extra (C); falls back to reservedTokens after flush.
func reservedToken(ts *TString) (int, bool) {
	if isreserved(ts) {
		return int(ts.extra) - 1 + FIRST_RESERVED, true
	}
	tk, ok := reservedTokens[ts.val]
	return tk, ok
}

// luaX_token2str returns a printable representation of a token (llex.c).
func luaX_token2str(ls *LexState, token int) string {
	if token < FIRST_RESERVED { /* single-byte symbols? */
		if lisprint(token) {
			return pushfString(ls.L, "'%c'", token).val
		}
		/* control character */
		return pushfString(ls.L, "'<\\%d>'", token).val
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
		/* C: save(ls, '\0'); — Go []byte needs no NUL */
		return pushfString(ls.L, "'%s'", ls.buffer()).val
	default:
		return luaX_token2str(ls, token)
	}
}

// lexerror raises a syntax error (llex.c).
func lexerror(ls *LexState, msg string, token int) {
	msg = luaG_addinfo(ls.L, msg, ls.source, ls.linenumber)
	if token != 0 {
		/* C: luaO_pushfstring(ls->L, "%s near %s", msg, txtToken(ls, token)) */
		ts := pushfString(ls.L, "%s near %s", msg, txtToken(ls, token))
		ls.L.pushMessage(ts)
	}
	luaD_throw(ls.L, LUA_ERRSYNTAX)
}

// luaX_syntaxerror raises a syntax error at the current token (llex.c).
func luaX_syntaxerror(ls *LexState, msg string) {
	lexerror(ls, msg, ls.t.token)
}

/*
** Creates a new string and anchors it in scanner's table so that it
** will not be collected until the end of the compilation; by that time
** it should be anchored somewhere. It also internalizes long strings,
** ensuring there is only one copy of each unique string.  The table
** here is used as a set: the string enters as the key, while its value
** is irrelevant. We use the string itself as the value only because it
** is a TValue readily available. Later, the code generation can change
** this value.
**
** FLAG: present-case copy via luaH_getstrkey (≡ keystrval(nodefromval(o)));
** addk may overwrite the value with a constant index, so the value is not
** a reliable handle on the stored string.
 */
func luaX_newstring(ls *LexState, str string) *TString {
	L := ls.L
	ts := luaS_newlstr(L, str) /* create new string */
	o := luaH_getstr(L, ls.h, ts)
	if !ttisnil(o) { /* string already present? */
		ts = luaH_getstrkey(L, ls.h, ts) /* get saved copy */
	} else { /* not in use yet */
		L.checkStack(1)
		stv := &L.stack[L.top]
		L.top++
		setsvalue(stv, ts)                   /* temporarily anchor the string */
		luaH_finishset(L, ls.h, stv, o, stv) /* t[string] = string */
		/* table is not a metatable, so it does not need to invalidate cache */
		/* C: luaC_checkGC(L); — omitted (Go GC) */
		L.top-- /* remove string from stack */
	}
	return ts
}

/*
** increment line number and skips newline sequence (any of
** \n, \r, \n\r, or \r\n)
 */
func (ls *LexState) inclinenumber() {
	old := ls.current
	/* lua_assert(currIsNewline(ls)) */
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
// Go: src/fill replace ZIO; resetbuffer stands in for luaZ_resizebuffer(..., LUA_MINBUFFER).
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
	ls.resetbuffer()               /* C: luaZ_resizebuffer(..., LUA_MINBUFFER) */
}

// ---------------------------------------------------------------------
// LEXICAL ANALYZER
// ---------------------------------------------------------------------

// checkNext1 checks whether the current char is c, consuming it (llex.c check_next1).
func (ls *LexState) checkNext1(c int) bool {
	if ls.current == c {
		ls.next()
		return true
	}
	return false
}

/*
** Check whether current char is in set 'set' (with two chars) and
** saves it (llex.c check_next2).
 */
func (ls *LexState) checkNext2(set string) bool {
	/* lua_assert(set[2] == '\0') */
	if ls.current == int(set[0]) || ls.current == int(set[1]) {
		ls.saveAndNext()
		return true
	}
	return false
}

/* LUA_NUMBER */
/*
** This function is quite liberal in what it accepts, as 'luaO_str2num'
** will reject ill-formed numerals. Roughly, it accepts the following
** pattern:
**
**   %d(%x|%.|([Ee][+-]?))* | 0[Xx](%x|%.|([Pp][+-]?))*
**
** The only tricky part is to accept [+-] only after a valid exponent
** mark, to avoid reading '3-4' or '0xe+1' as a single number.
**
** The caller might have already read an initial dot.
 */
func read_numeral(ls *LexState, seminfo *SemInfo) int {
	var obj Value
	expo := "Ee"
	first := ls.current
	/* lua_assert(lisdigit(ls->current)) */
	ls.saveAndNext()
	if first == '0' && ls.checkNext2("xX") { /* hexadecimal? */
		expo = "Pp"
	}
	for {
		if ls.checkNext2(expo) { /* exponent mark? */
			ls.checkNext2("-+") /* optional exponent sign */
		} else if lisxdigitChar(ls.current) || ls.current == '.' { /* '%x|%.' */
			ls.saveAndNext()
		} else {
			break
		}
	}
	if lislalpha(ls.current) { /* is numeral touching a letter? */
		ls.saveAndNext() /* force an error */
	}
	/* C: save(ls, '\0'); — Go []byte needs no NUL */
	if !luaO_str2num(ls.buffer(), &obj) { /* format error? */
		lexerror(ls, "malformed number", TK_FLT)
	}
	if ttisinteger(&obj) {
		seminfo.i = ivalue(&obj)
		return TK_INT
	}
	/* lua_assert(ttisfloat(&obj)) */
	seminfo.r = fltvalue(&obj)
	return TK_FLT
}

/*
** read a sequence '[=*[' or ']=*]', leaving the last bracket. If
** sequence is well formed, return its number of '='s + 2; otherwise,
** return 1 if it is a single bracket (no '='s and no 2nd bracket);
** otherwise (an unfinished '[==...') return 0.
 */
func (ls *LexState) skipSep() int {
	count := 0
	s := ls.current
	/* lua_assert(s == '[' || s == ']') */
	ls.saveAndNext()
	for ls.current == '=' {
		ls.saveAndNext()
		count++
	}
	if ls.current == s {
		return count + 2
	}
	if count == 0 {
		return 1
	}
	return 0
}

func read_long_string(ls *LexState, seminfo *SemInfo, sep int) {
	line := ls.linenumber  /* initial line (for error message) */
	ls.saveAndNext()       /* skip 2nd '[' */
	if currIsNewline(ls) { /* string starts with a newline? */
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
			/* C: luaO_pushfstring(ls->L, "unfinished long %s (starting at line %d)", ...) */
			msg := pushfString(ls.L, "unfinished long %s (starting at line %d)", what, line).val
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
		seminfo.ts = luaX_newstring(ls, string(ls.buff[sep:len(ls.buff)-sep]))
	}
}

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
	esccheck(ls, lisxdigitChar(ls.current), "hexadecimal digit expected")
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
	i := 4           /* chars to be removed: '\', 'u', '{', and first digit */
	ls.saveAndNext() /* skip 'u' */
	esccheck(ls, ls.current == '{', "missing '{'")
	r = uint32(gethexa(ls)) /* must have at least one digit */
	for {
		ls.saveAndNext()
		if !lisxdigitChar(ls.current) {
			break
		}
		i++
		esccheck(ls, r <= (0x7FFFFFFF>>4), "UTF-8 value too large")
		r = (r << 4) + uint32(luaO_hexavalue(byte(ls.current)))
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
	r := 0                                          /* result accumulator */
	for i = 0; i < 3 && lisdigit(ls.current); i++ { /* read up to 3 digits */
		r = 10*r + ls.current - '0'
		ls.saveAndNext()
	}
	esccheck(ls, r <= 255, "decimal escape too large") /* UCHAR_MAX */
	ls.buffRemove(i)                                   /* remove read digits from buffer */
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
			/* Go has no goto read_save/only_save/no_save; mode mirrors them. */
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
				for lisspaceChar(ls.current) {
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
				ls.resetbuffer() /* 'skip_sep' may dirty the buffer */
				if sep >= 2 {
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
			if sep >= 2 {
				read_long_string(ls, seminfo, sep)
				return TK_STRING
			} else if sep == 0 { /* '[=...' missing second bracket? */
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
				if tk, ok := reservedToken(ts); ok { /* reserved word? */
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
		ls.t = ls.lookahead         /* use this one */
		ls.lookahead.token = TK_EOS /* and discharge it */
	} else {
		ls.t.token = llex(ls, &ls.t.seminfo) /* read next token */
	}
}

// luaX_lookahead reads one token ahead without consuming the current one
// (llex.c).
func luaX_lookahead(ls *LexState) int {
	/* lua_assert(ls->lookahead.token == TK_EOS) */
	ls.lookahead.token = llex(ls, &ls.lookahead.seminfo)
	return ls.lookahead.token
}
