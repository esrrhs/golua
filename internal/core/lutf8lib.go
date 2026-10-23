// Translation of lutf8lib.c (PUC-Lua 5.4.9): Standard library for UTF-8
// manipulation. See Copyright Notice in lua.h.
//
// The C code works with pointers into the (NUL-terminated) string; here
// we work with indexes into the Go string and treat "past the end" as
// the terminating NUL (utf8IsContAt returns false at/after len).
package core

const (
	utf8MaxUnicode = 0x10FFFF   // MAXUNICODE
	utf8MaxUTF     = 0x7FFFFFFF // MAXUTF (needs 31 bits; utfint → uint32)
)

const utf8MsgInvalid = "invalid UTF-8 code" // MSGInvalid

// utf8CharPattern is UTF8PATT. C uses sizeof(UTF8PATT)/sizeof(char) - 1
// because the literal starts with an embedded NUL byte.
const utf8CharPattern = "[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"

// iscont / iscontp
func utf8IsCont(c byte) bool { return c&0xC0 == 0x80 }

// utf8IsContAt is iscontp(s + i). Bounds-checks: C relies on trailing NUL.
func utf8IsContAt(s string, i int) bool {
	return i < len(s) && utf8IsCont(s[i])
}

// utf8PosRelat is u_posrelat (from strlib): negative means back from end.
func utf8PosRelat(pos int64, length int) int64 {
	if pos >= 0 {
		return pos
	} else if uint64(0)-uint64(pos) > uint64(length) { /* 0u - (size_t)pos */
		return 0
	} else {
		return int64(length) + pos + 1
	}
}

/*
** Decode one UTF-8 sequence starting at 'i', returning ok==false if the
** byte sequence is invalid.  The array 'limits' stores the minimum value
** for each sequence length, to check for overlong representations. Its
** first entry forces an error for non-ascii bytes with no continuation
** bytes (count == 0).
** C: utf8_decode(const char *s, utfint *val, int strict)
 */
func utf8libDecode(s string, i int, strict bool) (val uint32, next int, ok bool) {
	limits := [6]uint32{^uint32(0), 0x80, 0x800, 0x10000, 0x200000, 0x4000000}
	c := uint32(s[i])
	var res uint32 /* final result */
	pos := i       /* C: s cursor; advanced by count for non-ASCII */
	if c < 0x80 {  /* ascii? */
		res = c
	} else if c >= 0xfe { /* c >= 1111 1110b ? */
		return 0, 0, false /* would need six or more continuation bytes */
	} else {
		count := 0                   /* to count number of continuation bytes */
		for ; c&0x40 != 0; c <<= 1 { /* while it needs continuation bytes... */
			count++
			if i+count >= len(s) {
				/* C reads s[++count] into trailing NUL → !iscont → NULL */
				return 0, 0, false /* invalid byte sequence */
			}
			cc := uint32(s[i+count])   /* read next byte */
			if !utf8IsCont(byte(cc)) { /* not a continuation byte? */
				return 0, 0, false /* invalid byte sequence */
			}
			res = (res << 6) | (cc & 0x3F) /* add lower 6 bits from cont. byte */
		}
		res |= (c & 0x7F) << (count * 5) /* add first byte */
		if res > utf8MaxUTF || res < limits[count] {
			return 0, 0, false /* invalid byte sequence */
		}
		pos = i + count /* s += count — skip continuation bytes read */
	}
	if strict {
		/* check for invalid code points; too large or surrogates */
		if res > utf8MaxUnicode || (0xD800 <= res && res <= 0xDFFF) {
			return 0, 0, false
		}
	}
	return res, pos + 1, true /* +1 to include first byte */
}

/*
** utf8len(s [, i [, j [, lax]]]) --> number of characters that
** start in the range [i,j], or nil + current position if 's' is not
** well formed in that interval
 */
func utf8Len(L *State) int {
	n := int64(0) /* counter for the number of characters */
	s := L.CheckString(1)
	length := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), length)
	posj := utf8PosRelat(L.OptInteger(3, -1), length)
	lax := L.ToBoolean(4)
	/* C: luaL_argcheck(L, 1 <= posi && --posi <= (lua_Integer)len, 2, ...) */
	L.ArgCheck(1 <= posi, 2, "initial position out of bounds")
	posi--
	L.ArgCheck(posi <= int64(length), 2, "initial position out of bounds")
	/* C: luaL_argcheck(L, --posj < (lua_Integer)len, 3, ...) */
	posj--
	L.ArgCheck(posj < int64(length), 3, "final position out of bounds")
	for posi <= posj {
		_, s1, ok := utf8libDecode(s, int(posi), !lax)
		if !ok { /* conversion error? */
			L.PushFail()            /* luaL_pushfail — return fail ... */
			L.PushInteger(posi + 1) /* ... and current position */
			return 2
		}
		posi = int64(s1)
		n++
	}
	L.PushInteger(n)
	return 1
}

/*
** codepoint(s, [i, [j [, lax]]]) -> returns codepoints for all
** characters that start in the range [i,j]
 */
func utf8Codepoint(L *State) int {
	s := L.CheckString(1)
	length := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), length)
	pose := utf8PosRelat(L.OptInteger(3, posi), length)
	lax := L.ToBoolean(4)
	L.ArgCheck(posi >= 1, 2, "out of bounds")
	L.ArgCheck(pose <= int64(length), 3, "out of bounds")
	if posi > pose {
		return 0 /* empty interval; return no values */
	}
	/* (lua_Integer -> int) overflow? C: pose - posi >= INT_MAX */
	const intMax = int64(^uint32(0) >> 1) /* INT_MAX on typical Lua hosts */
	if pose-posi >= intMax {
		return L.Error2("string slice too long")
	}
	n := int(pose-posi) + 1 /* upper bound for number of returns */
	L.CheckStack2(n, "string slice too long")
	n = 0           /* count the number of returns */
	se := int(pose) /* string end (exclusive index; C: se = s + pose) */
	for i := int(posi) - 1; i < se; {
		code, next, ok := utf8libDecode(s, i, !lax)
		if !ok {
			return L.Error2(utf8MsgInvalid)
		}
		L.PushInteger(int64(code))
		n++
		i = next
	}
	return n
}

// pushutfchar
func utf8PushChar(L *State, arg int) {
	code := uint64(L.CheckInteger(arg)) /* lua_Unsigned */
	L.ArgCheck(code <= utf8MaxUTF, arg, "value out of range")
	/* C: lua_pushfstring(L, "%U", (long)code) */
	L.PushFString("%U", int64(code))
}

/*
** utfchar(n1, n2, ...)  -> char(n1)..char(n2)...
 */
func utf8Char(L *State) int {
	n := L.GetTop() /* number of arguments */
	if n == 1 {     /* optimize common case of single char */
		utf8PushChar(L, 1)
	} else {
		b := L.NewBuffer() /* luaL_Buffer; luaL_buffinit */
		for i := 1; i <= n; i++ {
			utf8PushChar(L, i)
			b.AddValue() /* luaL_addvalue */
		}
		b.PushResult() /* luaL_pushresult */
	}
	return 1
}

/*
** offset(s, n, [i])  -> index where n-th character counting from
**   position 'i' starts; 0 means character at 'i'.
 */
func utf8ByteOffset(L *State) int {
	s := L.CheckString(1)
	length := len(s)
	n := L.CheckInteger(2)
	var posi int64
	if n >= 0 {
		posi = 1
	} else {
		posi = int64(length) + 1
	}
	posi = utf8PosRelat(L.OptInteger(3, posi), length)
	/* C: luaL_argcheck(L, 1 <= posi && --posi <= (lua_Integer)len, 3, ...) */
	L.ArgCheck(1 <= posi, 3, "position out of bounds")
	posi--
	L.ArgCheck(posi <= int64(length), 3, "position out of bounds")
	if n == 0 {
		/* find beginning of current byte sequence */
		for posi > 0 && utf8IsContAt(s, int(posi)) {
			posi--
		}
	} else {
		if utf8IsContAt(s, int(posi)) {
			return L.Error2("initial position is a continuation byte")
		}
		if n < 0 {
			for n < 0 && posi > 0 { /* move back */
				for { /* find beginning of previous character */
					posi--
					if !(posi > 0 && utf8IsContAt(s, int(posi))) {
						break
					}
				}
				n++
			}
		} else {
			n-- /* do not move for 1st character */
			for n > 0 && posi < int64(length) {
				for { /* find beginning of next character */
					posi++
					if !utf8IsContAt(s, int(posi)) { /* (cannot pass final '\0') */
						break
					}
				}
				n--
			}
		}
	}
	if n == 0 { /* did it find given character? */
		L.PushInteger(posi + 1)
	} else { /* no such character */
		L.PushFail() /* luaL_pushfail */
	}
	return 1
}

// iter_aux
func utf8IterAux(L *State, strict bool) int {
	s := L.CheckString(1)
	length := len(s)
	n, _ := L.ToInteger(2)
	un := uint64(n) /* lua_Unsigned n = (lua_Unsigned)lua_tointeger(L, 2) */
	if un < uint64(length) {
		for utf8IsContAt(s, int(un)) {
			un++ /* go to next character */
		}
	}
	if un >= uint64(length) { /* (also handles original 'n' being negative) */
		return 0 /* no more codepoints */
	} else {
		code, next, ok := utf8libDecode(s, int(un), strict)
		if !ok || utf8IsContAt(s, next) {
			return L.Error2(utf8MsgInvalid)
		}
		L.PushInteger(int64(un) + 1)
		L.PushInteger(int64(code))
		return 2
	}
}

func utf8IterAuxStrict(L *State) int { return utf8IterAux(L, true) }  // iter_auxstrict
func utf8IterAuxLax(L *State) int    { return utf8IterAux(L, false) } // iter_auxlax

// iter_codes
func utf8IterCodes(L *State) int {
	lax := L.ToBoolean(2)
	s := L.CheckString(1)
	L.ArgCheck(!utf8IsContAt(s, 0), 1, utf8MsgInvalid)
	if lax {
		L.PushGoFunc(utf8IterAuxLax) /* lua_pushcfunction */
	} else {
		L.PushGoFunc(utf8IterAuxStrict)
	}
	L.PushValue(1)
	L.PushInteger(0)
	return 3
}

/* funcs[] — charpattern is a placeholder in C (NULL), set in luaopen_utf8 */
var utf8Funcs = []Reg{
	{Name: "offset", Func: utf8ByteOffset},
	{Name: "codepoint", Func: utf8Codepoint},
	{Name: "char", Func: utf8Char},
	{Name: "len", Func: utf8Len},
	{Name: "codes", Func: utf8IterCodes},
	/* placeholders */
	{Name: "charpattern", Func: nil},
}

// OpenUTF8 opens the UTF-8 library (luaopen_utf8).
func OpenUTF8(L *State) int {
	L.NewLib(utf8Funcs) /* luaL_newlib */
	/* C: lua_pushlstring(L, UTF8PATT, sizeof(UTF8PATT)/sizeof(char) - 1) */
	L.PushLString(utf8CharPattern)
	L.SetField(-2, "charpattern")
	return 1
}
