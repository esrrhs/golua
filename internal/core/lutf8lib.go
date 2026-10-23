// Translation of lutf8lib.c (PUC-Lua 5.3.4 v 1.16): Standard library for
// UTF-8 manipulation. See Copyright Notice in lua.h.
//
// The C code works with pointers into the (NUL-terminated) string; here
// we work with indexes into the Go string and treat "past the end" as
// the terminating NUL. Decoder is 5.3.4: four-item limits, no lax, no
// surrogate ban, UTF8PATT ends at \xF4 (not 5.4 \xFD).
package core

const utf8MaxUnicode = 0x10FFFF /* MAXUNICODE */

const utf8MsgInvalid = "invalid UTF-8 code"

/*
** pattern to match a single UTF-8 character
** C: #define UTF8PATT "[\0-\x7F\xC2-\xF4][\x80-\xBF]*"
** C opens with pushlstring(..., sizeof(UTF8PATT)-1) because of the
** embedded NUL; the Go string includes '\x00' and PushLString uses len.
 */
const utf8CharPattern = "[\x00-\x7F\xC2-\xF4][\x80-\xBF]*"

/* iscont(p) — (*(p) & 0xC0) == 0x80 */
func utf8IsCont(c byte) bool { return c&0xC0 == 0x80 }

/* utf8IsContAt is iscont(s + i). Bounds-check: C relies on trailing NUL. */
func utf8IsContAt(s string, i int) bool {
	return i < len(s) && utf8IsCont(s[i])
}

/* from strlib — translate a relative string position: negative means back from end */
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
** Decode one UTF-8 sequence, returning ok==false if byte sequence is invalid.
** C 5.3.4: static const unsigned int limits[] = {0xFF, 0x7F, 0x7FF, 0xFFFF};
** reject when count > 3 || res > MAXUNICODE || res <= limits[count].
** (Not 5.4: no strict/lax, no six-entry limits, no surrogate ban, no c>=0xfe.)
 */
func utf8libDecode(s string, i int) (val uint32, next int, ok bool) {
	limits := [4]uint32{0xFF, 0x7F, 0x7FF, 0xFFFF}
	if i >= len(s) {
		/* C would read the trailing NUL as ascii U+0000; callers never
		   start a decode at len (interval / n >= len guards). Fail closed. */
		return 0, 0, false
	}
	c := uint32(s[i])
	var res uint32 /* final result */
	next = i + 1
	if c < 0x80 { /* ascii? */
		res = c
	} else {
		count := 0        /* to count number of continuation bytes */
		for c&0x40 != 0 { /* still have continuation bytes? */
			count++
			if i+count >= len(s) {
				/* C: s[++count] hits trailing NUL → (NUL & 0xC0) != 0x80 → NULL */
				return 0, 0, false /* invalid byte sequence */
			}
			cc := uint32(s[i+count]) /* read next byte */
			if (cc & 0xC0) != 0x80 { /* not a continuation byte? */
				return 0, 0, false /* invalid byte sequence */
			}
			res = (res << 6) | (cc & 0x3F) /* add lower 6 bits from cont. byte */
			c <<= 1                        /* to test next bit */
		}
		res |= (c & 0x7F) << (count * 5) /* add first byte */
		if count > 3 || res > utf8MaxUnicode || res <= limits[count] {
			return 0, 0, false /* invalid byte sequence */
		}
		next = i + count + 1 /* skip continuation bytes read (+ first) */
	}
	return res, next, true
}

/*
** utf8len(s [, i [, j]]) --> number of characters that start in the
** range [i,j], or nil + current position if 's' is not well formed in
** that interval
 */
func utf8Len(L *State) int {
	n := 0 /* C: int n */
	s := L.CheckString(1)
	length := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), length)
	posj := utf8PosRelat(L.OptInteger(3, -1), length)
	/* C: luaL_argcheck(L, 1 <= posi && --posi <= (lua_Integer)len, 2, ...) */
	L.ArgCheck(1 <= posi, 2, "initial position out of string")
	posi--
	L.ArgCheck(posi <= int64(length), 2, "initial position out of string")
	/* C: luaL_argcheck(L, --posj < (lua_Integer)len, 3, ...) */
	posj--
	L.ArgCheck(posj < int64(length), 3, "final position out of string")
	for posi <= posj {
		_, s1, ok := utf8libDecode(s, int(posi))
		if !ok { /* conversion error? */
			L.PushNil()             /* return nil ... */
			L.PushInteger(posi + 1) /* ... and current position */
			return 2
		}
		posi = int64(s1)
		n++
	}
	L.PushInteger(int64(n))
	return 1
}

/*
** codepoint(s, [i, [j]])  -> returns codepoints for all characters
** that start in the range [i,j]
 */
func utf8Codepoint(L *State) int {
	s := L.CheckString(1)
	length := len(s)
	posi := utf8PosRelat(L.OptInteger(2, 1), length)
	pose := utf8PosRelat(L.OptInteger(3, posi), length)
	L.ArgCheck(posi >= 1, 2, "out of range")
	L.ArgCheck(pose <= int64(length), 3, "out of range")
	if posi > pose {
		return 0 /* empty interval; return no values */
	}
	/* (lua_Integer -> int) overflow? Keep 32-bit INT_MAX even if Go int is 64-bit. */
	if pose-posi >= int64(int(^uint32(0)>>1)) {
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
	L.PushFString("%U", code) /* lua_pushfstring(L, "%U", (long)code) */
}

/*
** utfchar(n1, n2, ...)  -> char(n1)..char(n2)...
 */
func utf8Char(L *State) int {
	n := L.GetTop() /* number of arguments */
	if n == 1 {     /* optimize common case of single char */
		utf8PushChar(L, 1)
	} else {
		var b Buffer /* luaL_Buffer b; stack-local like C (not heap NewBuffer) */
		b.L = L      /* luaL_buffinit(L, &b) — initb wiring is lauxlib's Buffer shape */
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
	L.ArgCheck(1 <= posi, 3, "position out of range")
	posi--
	L.ArgCheck(posi <= int64(length), 3, "position out of range")
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
		L.PushNil()
	}
	return 1
}

/* iter_aux — C: n = tointeger(2)-1; first iter n<0 → 0; else skip current + conts */
func utf8IterAux(L *State) int {
	s := L.CheckString(1)
	length := len(s)
	n, _ := L.ToInteger(2)
	n = n - 1
	if n < 0 { /* first iteration? */
		n = 0 /* start from here */
	} else if n < int64(length) {
		n++ /* skip current byte */
		for utf8IsContAt(s, int(n)) { /* and its continuations */
			n++
		}
	}
	if n >= int64(length) {
		return 0 /* no more codepoints */
	}
	code, next, ok := utf8libDecode(s, int(n))
	if !ok || utf8IsContAt(s, next) {
		/* C: if (next == NULL || iscont(next)) — 5.3.4 eager-fail quirk */
		return L.Error2(utf8MsgInvalid)
	}
	L.PushInteger(n + 1)
	L.PushInteger(int64(code))
	return 2
}

func utf8IterCodes(L *State) int {
	L.CheckString(1)
	L.PushGoFunc(utf8IterAux) /* lua_pushcfunction(L, iter_aux) */
	L.PushValue(1)
	L.PushInteger(0)
	return 3
}

/* funcs[] — C also lists {"charpattern", NULL} placeholder; Go sets it in OpenUTF8 */
var utf8Funcs = []Reg{
	{Name: "offset", Func: utf8ByteOffset},
	{Name: "codepoint", Func: utf8Codepoint},
	{Name: "char", Func: utf8Char},
	{Name: "len", Func: utf8Len},
	{Name: "codes", Func: utf8IterCodes},
}

/* luaopen_utf8 */
func OpenUTF8(L *State) int {
	L.NewLib(utf8Funcs) /* luaL_newlib */
	L.PushLString(utf8CharPattern) /* lua_pushlstring(L, UTF8PATT, sizeof-1) */
	L.SetField(-2, "charpattern")
	return 1
}
