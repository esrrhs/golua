// Translation of lstring.h / lstring.c (PUC-Lua 5.4).
//
// Short strings are interned in a Go map that holds them for the whole
// life of the state, exactly as C's string table does: two short strings
// that compare equal are the same object, so 'lua_topointer' answers the
// same address for both and a short string can be used as a key with
// 'lua_rawsetp'. C's collector sweeps the table; here Go's collector
// does not, because the map holds a strong reference to every interned
// string -- which is the price of pointer identity, and the same price
// PUC-Lua pays.
package lua

import (
	"strings"
)

const (
	LUAI_MAXSHORTLEN = 40
	LUAI_HASHLIMIT   = 5
)

// stringTable replaces Lua's hash table of strings. It holds every short
// string the state has interned, for the whole life of the state: a
// lookup always answers the same *TString for the same bytes.
type stringTable struct {
	entries map[string]*TString
}

func newStringTable() *stringTable {
	return &stringTable{
		entries: make(map[string]*TString, 1024),
	}
}

// get returns the interned string with these bytes, or nil.
func (tb *stringTable) get(s string) *TString {
	return tb.entries[s]
}

// put interns a string.
func (tb *stringTable) put(s string, ts *TString) {
	tb.entries[s] = ts
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

// luaS_eqshrstr compares two short strings. Short strings are interned
// for the life of the state, so equal short strings are the same object
// and a pointer comparison is enough -- exactly C's 'eqshrstr'.
func luaS_eqshrstr(a, b *TString) bool {
	return a == b
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
