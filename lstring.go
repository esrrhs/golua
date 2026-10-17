// Translation of lstring.h / lstring.c (PUC-Lua 5.4).
//
// Short strings are interned in a Go map. C's collector sweeps that table;
// the obvious translation is to let Go's collector do it, by holding the
// strings weakly -- and it is what this file did at first. It costs half the
// time of a string-key table workload: every interned string then carries a
// weak handle, and the handles are exactly what the collector has to walk
// during a cycle, one per special, and a short string is the most allocated
// object in a Lua program.
//
// So the table is now a cache with two generations. New strings go into
// 'cur'; when 'cur' fills up it becomes 'old' and a fresh 'cur' takes over,
// and when 'cur' fills up again 'old' is dropped whole. A string therefore
// survives at least one full generation, the table holds at most two
// generations' worth, and no handle is ever registered with the runtime.
//
// What that gives up is interning being permanent: two live strings can now
// hold the same bytes. Nothing may compare short strings by pointer;
// 'luaS_eqshrstr' compares the hash and then the bytes, and that is what the
// table code and 'luaV_equalobj' use.
package lua

import (
	"strings"
)

const (
	LUAI_MAXSHORTLEN = 40
	LUAI_HASHLIMIT   = 5
	maxInternedShort = 1 << 14
)

// stringTable replaces Lua's hash table of strings. See the comment at the
// top of the file: it is a two-generation cache, not a table the collector
// sweeps.
type stringTable struct {
	cur   map[string]*TString
	old   map[string]*TString
	limit int /* a generation this size is full */
}

func newStringTable() *stringTable {
	return &stringTable{
		cur:   make(map[string]*TString, 1024),
		limit: maxInternedShort,
	}
}

// get returns the interned string with these bytes, or nil.
func (tb *stringTable) get(s string) *TString {
	if ts, ok := tb.cur[s]; ok {
		return ts
	}
	if ts, ok := tb.old[s]; ok {
		tb.cur[s] = ts /* still in use: give it another generation */
		return ts
	}
	return nil
}

// put interns a string, rotating the generations when 'cur' is full.
func (tb *stringTable) put(s string, ts *TString) {
	tb.cur[s] = ts
	if len(tb.cur) >= tb.limit {
		tb.old = tb.cur
		tb.cur = make(map[string]*TString, tb.limit)
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

// luaS_eqshrstr compares two short strings. C's 'eqshrstr' is a pointer
// comparison, which interning makes enough there; here interning is a
// cache (see stringTable), so the hash is compared first and the bytes
// only when the hashes agree.
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
