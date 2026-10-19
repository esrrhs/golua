// Translation of lstring.h / lstring.c (PUC-Lua 5.4).
//
// Short strings are interned through a Go weak map: an entry disappears
// once nothing else refers to the string, and a sweep drops the dead
// entries. That is exactly what C's string table does under its own
// collector -- a short string lives while something references it and is
// interned afresh otherwise -- and it keeps the guarantee C extensions
// rely on: of the short strings alive at any moment, equal ones are the
// same object, so 'lua_topointer' answers the same address for both and
// a short string can be a 'lua_rawsetp' key.
package lua

import (
	"strings"
	"weak"
)

const (
	LUAI_MAXSHORTLEN = 40
	LUAI_HASHLIMIT   = 5
	maxInternedShort = 1 << 16
)

// stringTable replaces Lua's hash table of strings. The entries are weak:
// Go's collector drops the unreachable ones and 'sweep' takes them out of
// the map, the way C's own sweep takes dead strings out of 'strt'.
type stringTable struct {
	short map[string]weak.Pointer[TString]
	nuse  int
	limit int
}

func newStringTable() *stringTable {
	return &stringTable{
		short: make(map[string]weak.Pointer[TString], 1024),
		limit: maxInternedShort,
	}
}

// sweep removes the entries whose string is no longer reachable. It is
// O(len(short)) and so must not run on every insertion; the next one is
// deferred until the table has grown again.
func (tb *stringTable) sweep() {
	for k, wp := range tb.short {
		if wp.Value() == nil {
			delete(tb.short, k)
		}
	}
	tb.nuse = len(tb.short)
	tb.limit = 2 * tb.nuse
	if tb.limit < maxInternedShort {
		tb.limit = maxInternedShort
	}
}

// get returns the interned string with these bytes, or nil.
func (tb *stringTable) get(s string) *TString {
	if wp, ok := tb.short[s]; ok {
		return wp.Value()
	}
	return nil
}

// put interns a string.
func (tb *stringTable) put(s string, ts *TString) {
	tb.short[s] = weak.Make(ts)
	tb.nuse++
	if tb.nuse > tb.limit {
		tb.sweep()
	}
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

// luaS_eqshrstr compares two short strings. Weak interning still
// guarantees that the strings alive at any moment are one object each --
// a second object is only made when the first is already gone -- so a
// pointer comparison is enough, exactly C's 'eqshrstr'.
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
