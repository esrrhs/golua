// Translation of lstring.h / lstring.c (PUC-Lua 5.4).
//
// Short strings are interned in a strong map. A Go weak map made the miss
// path several times slower than building the string, which is what
// string.format spends its time on. The map is capped: past the cap it is
// dropped and later duplicates are new objects. Equality compares bytes,
// so a flushed entry still compares equal to a live one. While an entry
// is cached, equal short strings stay the same object, which is what
// lua_topointer and lua_rawsetp see.
package core

import (
	"strings"
)

const (
	LUAI_MAXSHORTLEN = 40
	LUAI_HASHLIMIT   = 5
	maxInternedShort = 1 << 20
)

// stringTable replaces Lua's hash table of strings.
type stringTable struct {
	short map[string]*TString
	nuse  int
}

func newStringTable() *stringTable {
	return &stringTable{short: make(map[string]*TString, 1024)}
}

// get returns the interned string with these bytes, or nil.
func (tb *stringTable) get(s string) *TString { return tb.short[s] }

// flush drops the cache. A full collection has to be able to free a
// short string that nothing else references; the cache is an extra
// root, and a C caller waiting on the string's finalizer (the copy
// lua_tolstring handed out) would otherwise wait forever. Strings
// still reachable from the stack or a table stay, and equality does
// not require the cache.
func (tb *stringTable) flush() {
	if tb == nil || tb.nuse == 0 {
		return
	}
	tb.short = make(map[string]*TString, 1024)
	tb.nuse = 0
}

// put interns a string. Past the cap the cache is dropped so a process
// that invents short strings forever does not keep all of them.
func (tb *stringTable) put(s string, ts *TString) {
	if tb.nuse >= maxInternedShort {
		tb.short = make(map[string]*TString, 1024)
		tb.nuse = 0
	}
	tb.short[s] = ts
	tb.nuse++
}

// luaS_hash computes the hash of a string (lstring.c).
func luaS_hash(str string, seed uint32) uint32 {
	l := len(str)
	h := seed ^ uint32(l)
	step := (l >> LUAI_HASHLIMIT) + 1
	for i := l; i >= step; i -= step {
		h ^= (h << 5) + (h >> 2) + uint32(str[i-1])
	}
	return h
}

// luaS_hashlongstr computes (and caches) the hash of a long string.
func luaS_hashlongstr(L *State, ts *TString) uint32 {
	if ts.extra == 0 { /* no hash? */
		ts.hash = luaS_hash(ts.val, L.g.seed)
		ts.extra = 1
	}
	return ts.hash
}

// luaS_eqlngstr compares two strings by content (lstring.c).
func luaS_eqlngstr(a, b *TString) bool {
	return a == b || (len(a.val) == len(b.val) && a.val == b.val)
}

// luaS_eqshrstr compares two short strings. Cached strings are unique, so
// the pointer test is the usual answer. After the cache is flushed a
// live string and a newly interned copy still have to compare equal.
func luaS_eqshrstr(a, b *TString) bool {
	return a == b || (a.hash == b.hash && a.val == b.val)
}

func (L *State) internShort(s string) *TString {
	tb := L.g.strt
	if ts := tb.get(s); ts != nil {
		return ts
	}
	ts := &TString{tt: LUA_VSHRSTR, hash: luaS_hash(s, L.g.seed), val: s}
	tb.put(s, ts)
	return ts
}

// luaS_createlngstrobj creates a long string (lstring.c).
func luaS_createlngstrobj(L *State, s string) *TString {
	return &TString{tt: LUA_VLNGSTR, val: s}
}

// luaS_newlstr returns the interned (short) or fresh (long) string for s.
func luaS_newlstr(L *State, s string) *TString {
	if len(s) <= LUAI_MAXSHORTLEN {
		return L.internShort(s)
	}
	return luaS_createlngstrobj(L, s)
}

// luaS_new is luaS_newlstr for a Go string.
func luaS_new(L *State, s string) *TString { return luaS_newlstr(L, s) }

// luaS_newCStr is what C does when it hands a 'const char *' to
// luaS_new: the string stops at the first '\0' (lstring.c's luaS_new
// measures the name with strlen).  A chunk name reaches the parser that
// way, so 'load("return 1", "=a\0b")' reports its errors as "a:1: ..."
// and not "a\0b:1: ...".
func luaS_newCStr(L *State, s string) *TString {
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return luaS_newlstr(L, s)
}

// luaS_init initialises the string table (lstring.c).
func luaS_init(L *State) {
	if L.g.strt == nil {
		L.g.strt = newStringTable()
	}
}

// luaS_newudata creates a full userdata wrapping a Go value.
func luaS_newudata(L *State, nuvalue int, val any) *Udata {
	u := &Udata{nuvalue: uint16(nuvalue), val: val}
	if nuvalue > 0 {
		u.uv = make([]Value, nuvalue)
		for i := 0; i < nuvalue; i++ {
			setnilvalue(&u.uv[i])
		}
	}
	return u
}
