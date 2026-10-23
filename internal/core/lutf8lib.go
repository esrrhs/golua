// Translation of lutf8lib.c (PUC-Lua 5.3.4 v 1.16): the UTF-8 library.
//
// The C code works with pointers into the (NUL-terminated) string; here
// we work with indexes into the Go string and treat "past the end" as
// the terminating NUL. Decoder matches 5.3.4 (four-item limits; no lax).
package core

const utf8MaxUnicode = 0x10FFFF

const utf8MsgInvalid = "invalid UTF-8 code"

// utf8CharPattern is UTF8PATT. C uses sizeof(UTF8PATT)-1 because the
// literal starts with an embedded NUL; Go string contains '\x00'.
const utf8CharPattern = "[\x00-\x7F\xC2-\xF4][\x80-\xBF]*"

func utf8IsCont(c byte) bool { return c&0xC0 == 0x80 }

// utf8IsContAt is iscont(s + i). Bounds-checks: C relies on trailing NUL.
func utf8IsContAt(s string, i int) bool {
	return i < len(s) && utf8IsCont(s[i])
}

// utf8PosRelat is u_posrelat: negative means back from end.
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
** Matches Lua 5.3.4 lutf8lib.c utf8_decode (four-item limits; no lax).
 */
func utf8libDecode(s string, i int) (uint32, int, bool) {
	limits := [4]uint32{0xFF, 0x7F, 0x7FF, 0xFFFF}
	if i >= len(s) {
		return 0, 0, false
	}
	c := uint32(s[i])
	var res uint32 /* final result */
	next := i + 1
	if c < 0x80 { /* ascii? */
		res = c
	} else {
		count := 0        /* to count number of continuation bytes */
		for c&0x40 != 0 { /* still have continuation bytes? */
			count++
			if i+count >= len(s) {
				return 0, 0, false /* past end (C hits NUL → not cont) */
			}
			cc := uint32(s[i+count]) /* read next byte */
			if !utf8IsCont(byte(cc)) {
				return 0, 0, false /* invalid byte sequence */
			}
			res = (res << 6) | (cc & 0x3F) /* add lower 6 bits from cont. byte */
			c <<= 1                        /* to test next bit */
		}
		res |= (c & 0x7F) << (count * 5) /* add first byte */
		if count > 3 || res > utf8MaxUnicode || res <= limits[count] {
			return 0, 0, false /* invalid byte sequence */
		}
		next = i + count + 1 /* skip continuation bytes read */
	}
	return res, next, true
}

/*
** utf8len(s [, i [, j]]) --> number of characters that
** start in the range [i,j], or nil + current position if 's' is not
** well formed in that interval
 */
func utf8Len(L *State) int {
	n := 0 /* C: int n — character counter */
	s := L.CheckString(1)
	l := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), l)
	posj := utf8PosRelat(L.OptInteger(3, -1), l)
	L.ArgCheck(posi >= 1, 2, "initial position out of string")
	posi--
	L.ArgCheck(posi <= int64(l), 2, "initial position out of string")
	posj--
	L.ArgCheck(posj < int64(l), 3, "final position out of string")
	for posi <= posj {
		_, next, ok := utf8libDecode(s, int(posi))
		if !ok { /* conversion error? */
			L.PushNil()             /* return nil ... */
			L.PushInteger(posi + 1) /* ... and current position */
			return 2
		}
		posi = int64(next)
		n++
	}
	L.PushInteger(int64(n))
	return 1
}

/*
** codepoint(s, [i, [j]]) -> returns codepoints for all
** characters that start in the range [i,j]
 */
func utf8Codepoint(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), l)
	pose := utf8PosRelat(L.OptInteger(3, posi), l)
	L.ArgCheck(posi >= 1, 2, "out of range")
	L.ArgCheck(pose <= int64(l), 3, "out of range")
	if posi > pose {
		return 0 /* empty interval; return no values */
	}
	/* INT_MAX via 32-bit mask so overflow guard matches C even if Go int is 64-bit */
	if pose-posi >= int64(int(^uint32(0)>>1)) { /* (lua_Integer -> int) overflow? */
		return L.Error2("string slice too long")
	}
	n := int(pose-posi) + 1
	L.CheckStack2(n, "string slice too long")
	n = 0
	se := int(pose)
	for i := int(posi) - 1; i < se; {
		code, next, ok := utf8libDecode(s, i)
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
	L.ArgCheck(0 <= code && code <= utf8MaxUnicode, arg, "value out of range")
	/* C: lua_pushfstring(L, "%U", (long)code); Go: same encoder family */
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

// utf8IterAux is iter_aux (lutf8lib.c). Aligned to C: n = tointeger(2)-1;
// first iter (n<0) → 0; else skip current byte + continuations.
func utf8IterAux(L *State) int {
	s := L.CheckString(1)
	l := len(s)
	n, _ := L.ToInteger(2)
	n = n - 1
	if n < 0 { /* first iteration? */
		n = 0 /* start from here */
	} else if n < int64(l) {
		n++ /* skip current byte */
		for utf8IsContAt(s, int(n)) { /* and its continuations */
			n++
		}
	}
	if n >= int64(l) {
		return 0 /* no more codepoints */
	}
	code, next, ok := utf8libDecode(s, int(n))
	if !ok || utf8IsContAt(s, next) {
		return L.Error2(utf8MsgInvalid)
	}
	L.PushInteger(n + 1)
	L.PushInteger(int64(code))
	return 2
}

func utf8IterCodes(L *State) int {
	L.CheckString(1)
	L.PushGoFunc(utf8IterAux)
	L.PushValue(1)
	L.PushInteger(0)
	return 3
}

// ---------------------------------------------------------------------
// library registration
// ---------------------------------------------------------------------

// utf8Funcs is funcs[] without the {"charpattern", NULL} placeholder;
// OpenUTF8 SetFields charpattern after NewLib (same table result as C).
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
