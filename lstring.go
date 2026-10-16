// Translation of lstring.h / lstring.c (PUC-Lua 5.4).
//
// Short strings are interned in a Go map. Lua's collector normally sweeps
// this table; here we let Go's own concurrent collector do it: the map holds
// weak references, so an interned string disappears once nothing else refers
// to it, and a periodic sweep drops the dead entries. This keeps pointer
// identity (which the parser and the table code rely on) while avoiding the
// unbounded growth that a strong map would cause.
package lua

import "weak"

const (
	LUAI_MAXSHORTLEN = 40
	LUAI_HASHLIMIT   = 5
	maxInternedShort = 1 << 16
)

// stringTable replaces Lua's hash table of strings.
type stringTable struct {
	short map[string]weak.Pointer[TString]
	nuse  int
	limit int /* sweep again once 'nuse' passes this */
}

func newStringTable() *stringTable {
	return &stringTable{
		short: make(map[string]weak.Pointer[TString], 1024),
		limit: maxInternedShort,
	}
}

// sweep removes entries whose string is no longer reachable. It is the
// counterpart of the sweep phase of Lua's collector.
//
// It is O(len(short)), so it must not run on every insertion: when every
// interned string is still alive a sweep reclaims nothing, and a per-insert
// sweep would then turn string creation into a quadratic operation. C has no
// such problem, because it sweeps the table as part of a GC cycle. The next
// sweep is therefore deferred until the table has grown again, which makes
// the cost amortised O(1) per insertion.
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

func (L *State) internShort(s string) *TString {
	tb := L.g.strt
	if wp, ok := tb.short[s]; ok {
		if ts := wp.Value(); ts != nil {
			return ts
		}
	}
	ts := &TString{tt: LUA_VSHRSTR, hash: luaS_hash(s, L.g.seed), val: s}
	tb.short[s] = weak.Make(ts)
	tb.nuse++
	if tb.nuse > tb.limit {
		tb.sweep()
	}
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
