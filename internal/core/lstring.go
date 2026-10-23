// Translation of lstring.h / lstring.c (PUC-Lua 5.3.4).
//
// Short strings are interned in a strong map[string]*TString (Go stand-in
// for C's stringtable hash + luaS_resize). A weak map made the miss path
// several times slower than building the string (string.format hot path).
// The map is capped (maxInternedShort): past the cap it is dropped and
// later equal shorts become new objects. Equality compares content, so a
// flushed entry still compares equal to a live one. While cached, equal
// shorts stay the same object (what lua_topointer / lua_rawsetp see).
//
// GC helpers luaS_resize / luaS_remove / luaS_clearcache are omitted:
// Go GC + flush() replace them (intentional; see NOTES.md).
package core

import (
	"strings"
)

const (
	LUAI_MAXSHORTLEN = 40 /* llimits.h */
	LUAI_HASHLIMIT   = 5  /* lstring.c default */
	maxInternedShort = 1 << 20
)

/* MEMERRMSG is created in lstate (C does it in luaS_init). */

// stringTable replaces C stringtable { hash[], nuse, size }.
type stringTable struct {
	short map[string]*TString
	nuse  int
}

func newStringTable() *stringTable {
	return &stringTable{short: make(map[string]*TString, 1024)}
}

func (tb *stringTable) get(s string) *TString { return tb.short[s] }

// flush drops the intern cache (Go substitute for GC unlink via luaS_remove).
// A full collection must free short strings that nothing else references;
// the cache is an extra root. Reachable strings stay; equality does not
// require the cache (luaS_eqshrstr content fallback).
func (tb *stringTable) flush() {
	if tb == nil || tb.nuse == 0 {
		return
	}
	tb.short = make(map[string]*TString, 1024)
	tb.nuse = 0
}

// put interns ts under s. Past the cap the cache is dropped so unbounded
// short-string invention cannot pin every entry forever.
func (tb *stringTable) put(s string, ts *TString) {
	if tb.nuse >= maxInternedShort {
		tb.short = make(map[string]*TString, 1024)
		tb.nuse = 0
	}
	tb.short[s] = ts
	tb.nuse++
}

// luaS_hash — lstring.c: seed ^ l, step = (l >> LUAI_HASHLIMIT) + 1.
func luaS_hash(str string, seed uint32) uint32 {
	l := len(str)
	h := seed ^ uint32(l)
	step := (l >> LUAI_HASHLIMIT) + 1
	for i := l; i >= step; i -= step {
		h ^= (h << 5) + (h >> 2) + uint32(str[i-1])
	}
	return h
}

// luaS_hashlongstr — lstring.c (extra==0 → compute and cache hash).
// L is unused (kept for call-site convenience; C has no L).
func luaS_hashlongstr(L *State, ts *TString) uint32 {
	_ = L
	if ts.extra == 0 { /* no hash? */
		ts.hash = luaS_hash(ts.val, ts.hash) /* seed was stored in hash */
		ts.extra = 1                         /* now it has its hash */
	}
	return ts.hash
}

// luaS_eqlngstr — lstring.c: same instance or equal length+contents.
func luaS_eqlngstr(a, b *TString) bool {
	return a == b || (len(a.val) == len(b.val) && a.val == b.val)
}

// luaS_eqshrstr — C eqshrstr is pointer-only (always interned).
// After a Go flush, equal shorts may be distinct objects; fall back to
// hash+content so Lua equality stays correct.
func luaS_eqshrstr(a, b *TString) bool {
	return a == b || (a.hash == b.hash && a.val == b.val)
}

// internShort — lstring.c internshrstr (map lookup; no dead-resurrect).
func (L *State) internShort(s string) *TString {
	tb := L.g.strt
	if ts := tb.get(s); ts != nil {
		return ts
	}
	/* createstrobj(L, l, LUA_TSHRSTR, h): extra zeroed by struct literal */
	ts := &TString{tt: LUA_TSHRSTR, hash: luaS_hash(s, L.g.seed), val: s}
	tb.put(s, ts)
	return ts
}

// luaS_createlngstrobj — lstring.c: createstrobj(..., G(L)->seed); lnglen = l.
// Go takes the byte content as string (C memcpy's into the object later).
func luaS_createlngstrobj(L *State, s string) *TString {
	return &TString{tt: LUA_TLNGSTR, hash: L.g.seed, val: s}
}

// luaS_newlstr — lstring.c: short → internshrstr; long → createlngstrobj+memcpy.
func luaS_newlstr(L *State, s string) *TString {
	if len(s) <= LUAI_MAXSHORTLEN {
		return L.internShort(s)
	}
	return luaS_createlngstrobj(L, s)
}

// luaS_new — lstring.c routes through STRCACHE then strlen+newlstr.
// Go has no STRCACHE; equivalent to luaS_newlstr for a Go string.
func luaS_new(L *State, s string) *TString { return luaS_newlstr(L, s) }

// luaS_newCStr — C luaS_new(const char*): strlen stops at first '\0'.
// Needed for Go strings that may embed NUL (e.g. chunk names).
func luaS_newCStr(L *State, s string) *TString {
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return luaS_newlstr(L, s)
}

// luaS_init — lstring.c also resizes to MINSTRTABSIZE, fixes memerrmsg,
// and fills strcache. Go: ensure intern table; memerrmsg lives in lstate.
func luaS_init(L *State) {
	if L.g.strt == nil {
		L.g.strt = newStringTable()
	}
}

// luaS_newudata — lstring.c 5.3.4:
//   Udata *luaS_newudata(lua_State *L, size_t s);
// allocates sizeludata(s), sets len=s, metatable=NULL, one nil uservalue.
// Go: val any is the intentional port of the byte blob (and cgo blocks);
// always one uservalue slot (no 5.4 nuvalue / uv[]).
func luaS_newudata(L *State, val any) *Udata {
	_ = L
	u := &Udata{val: val, metatable: nil}
	setnilvalue(&u.user) /* setuservalue(L, u, luaO_nilobject) */
	return u
}
