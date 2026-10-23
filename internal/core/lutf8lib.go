// Translation of lutf8lib.c (PUC-Lua 5.4): the UTF-8 library.
//
// The C code works with pointers into the (NUL-terminated) string; here
// we work with indexes into the Go string and treat "past the end" as
// the terminating NUL.
package core

const (
	utf8MaxUnicode = 0x10FFFF
	utf8MaxUTF     = 0x7FFFFFFF
)

const utf8MsgInvalid = "invalid UTF-8 code"

// utf8CharPattern is the pattern matching a single UTF-8 character
// (UTF8PATT). In C it is built with sizeof - 1 because the literal
// starts with an embedded NUL byte.
const utf8CharPattern = "[\x00-\x7F\xC2-\xFD][\x80-\xBF]*"

func utf8IsCont(c byte) bool { return c&0xC0 == 0x80 }

// utf8IsContAt is the equivalent of 'iscontp(s + i)'.
func utf8IsContAt(s string, i int) bool {
	return i < len(s) && utf8IsCont(s[i])
}

// utf8PosRelat translates a relative string position: negative means
// back from end (strlib's u_posrelat).
func utf8PosRelat(pos int64, l int) int64 {
	if pos >= 0 {
		return pos
	}
	if uint64(0)-uint64(pos) > uint64(l) { /* 0u - (size_t)pos */
		return 0
	}
	return int64(l) + pos + 1
}

/*
** Decode one UTF-8 sequence starting at 'i', returning false if the
** byte sequence is invalid.  The array 'limits' stores the minimum
** value for each sequence length, to check for overlong
** representations. Its first entry forces an error for non-ascii bytes
** with no continuation bytes (count == 0).
 */
func utf8libDecode(s string, i int, strict bool) (uint32, int, bool) {
	limits := [6]uint32{^uint32(0), 0x80, 0x800, 0x10000, 0x200000, 0x4000000}
	c := uint32(s[i])
	var res uint32 /* final result */
	next := i + 1
	if c < 0x80 { /* ascii? */
		res = c
	} else if c >= 0xfe { /* c >= 1111 1110b ? */
		return 0, 0, false /* would need six or more continuation bytes */
	} else {
		count := 0        /* to count number of continuation bytes */
		for c&0x40 != 0 { /* while it needs continuation bytes... */
			count++
			if i+count >= len(s) {
				return 0, 0, false /* invalid byte sequence */
			}
			cc := uint32(s[i+count]) /* read next byte */
			if !utf8IsCont(byte(cc)) {
				return 0, 0, false /* invalid byte sequence */
			}
			res = (res << 6) | (cc & 0x3F) /* add lower 6 bits from cont. byte */
			c <<= 1
		}
		res |= (c & 0x7F) << (count * 5) /* add first byte */
		if res > utf8MaxUnicode || res < limits[count] {
			return 0, 0, false /* invalid byte sequence */
		}
		next = i + count + 1 /* skip continuation bytes read */
	}
	if strict {
		/* check for invalid code points; too large or surrogates */
		if res > utf8MaxUnicode || (0xD800 <= res && res <= 0xDFFF) {
			return 0, 0, false
		}
	}
	return res, next, true
}

/*
** utf8len(s [, i [, j [, lax]]]) --> number of characters that
** start in the range [i,j], or nil + current position if 's' is not
** well formed in that interval
 */
func utf8Len(L *State) int {
	n := int64(0) /* counter for the number of characters */
	s := L.CheckString(1)
	l := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), l)
	posj := utf8PosRelat(L.OptInteger(3, -1), l)
	lax := L.ToBoolean(4)
	/* 5.3.4: "out of string" (5.4 says "out of bounds") */
	L.ArgCheck(posi >= 1, 2, "initial position out of string")
	posi--
	L.ArgCheck(posi <= int64(l), 2, "initial position out of string")
	posj--
	L.ArgCheck(posj < int64(l), 3, "final position out of string")
	for posi <= posj {
		_, next, ok := utf8libDecode(s, int(posi), !lax)
		if !ok { /* conversion error? */
			L.PushNil()             /* return fail ... */
			L.PushInteger(posi + 1) /* ... and current position */
			return 2
		}
		posi = int64(next)
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
	l := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), l)
	pose := utf8PosRelat(L.OptInteger(3, posi), l)
	lax := L.ToBoolean(4)
	L.ArgCheck(posi >= 1, 2, "out of range")
	L.ArgCheck(pose <= int64(l), 3, "out of range")
	if posi > pose {
		return 0 /* empty interval; return no values */
	}
	if pose-posi >= int64(int(^uint32(0)>>1)) { /* (int64 -> int) overflow? */
		return L.Error2("string slice too long")
	}
	n := int(pose-posi) + 1 /* upper bound for number of returns */
	L.CheckStack2(n, "string slice too long")
	n = 0           /* count the number of returns */
	se := int(pose) /* string end */
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

func utf8PushChar(L *State, arg int) {
	code := L.CheckInteger(arg)
	/* 5.3.4: MAXUNICODE 0x10FFFF (5.4 allows up to 0x7FFFFFFF). */
	L.ArgCheck(0 <= code && code <= utf8MaxUnicode, arg, "value out of range")
	L.PushString(string(luaO_utf8esc(uint32(code))))
}

/*
** utfchar(n1, n2, ...)  -> char(n1)..char(n2)...
 */
func utf8Char(L *State) int {
	n := L.GetTop() /* number of arguments */
	if n == 1 {     /* optimize common case of single char */
		utf8PushChar(L, 1)
	} else {
		b := L.NewBuffer()
		for i := 1; i <= n; i++ {
			utf8PushChar(L, i)
			b.AddValue()
		}
		b.PushResult()
	}
	return 1
}

/*
** offset(s, n, [i])  -> index where n-th character counting from
**   position 'i' starts; 0 means character at 'i'.
 */
func utf8ByteOffset(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	n := L.CheckInteger(2)
	var posi int64
	if n >= 0 {
		posi = 1
	} else {
		posi = int64(l) + 1
	}
	posi = utf8PosRelat(L.OptInteger(3, posi), l)
	L.ArgCheck(posi >= 1, 3, "position out of range")
	posi--
	L.ArgCheck(posi <= int64(l), 3, "position out of range")
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
			for n > 0 && posi < int64(l) {
				for { /* find beginning of next character */
					posi++
					if !utf8IsContAt(s, int(posi)) { /* (cannot pass final NUL) */
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
		L.PushNil()
	}
	return 1
}

func utf8IterAux(L *State, strict bool) int {
	s := L.CheckString(1)
	n, _ := L.ToInteger(2)
	un := uint64(n)
	for un < uint64(len(s)) && utf8IsContAt(s, int(un)) {
		un++ /* go to next character */
	}
	if un >= uint64(len(s)) { /* (also handles original 'n' being negative) */
		return 0 /* no more codepoints */
	}
	code, next, ok := utf8libDecode(s, int(un), strict)
	if !ok || utf8IsContAt(s, next) {
		return L.Error2(utf8MsgInvalid)
	}
	L.PushInteger(int64(un) + 1)
	L.PushInteger(int64(code))
	return 2
}

func utf8IterStrict(L *State) int { return utf8IterAux(L, true) }

func utf8IterLax(L *State) int { return utf8IterAux(L, false) }

func utf8IterCodes(L *State) int {
	lax := L.ToBoolean(2)
	s := L.CheckString(1)
	L.ArgCheck(!utf8IsContAt(s, 0), 1, utf8MsgInvalid)
	if lax {
		L.PushGoFunc(utf8IterLax)
	} else {
		L.PushGoFunc(utf8IterStrict)
	}
	L.PushValue(1)
	L.PushInteger(0)
	return 3
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

var utf8Funcs = []Reg{
	{Name: "offset", Func: utf8ByteOffset},
	{Name: "codepoint", Func: utf8Codepoint},
	{Name: "char", Func: utf8Char},
	{Name: "len", Func: utf8Len},
	{Name: "codes", Func: utf8IterCodes},
}

// OpenUTF8 opens the UTF-8 library (luaopen_utf8).
func OpenUTF8(L *State) int {
	L.NewLib(utf8Funcs)
	L.PushString(utf8CharPattern)
	L.SetField(-2, "charpattern")
	return 1
}
