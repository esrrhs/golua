// Translation of lstring.h / lstring.c (PUC-Lua 5.3.4).
//
// Short strings are interned in a strong map[string]*TString (Go stand-in
// for C's stringtable hash + luaS_resize / internshrstr). A weak map made
// the miss path several times slower than building the string (string.format
// hot path). The map is capped (maxInternedShort): past the cap it is
// dropped and later equal shorts become new objects. Equality compares
// content, so a flushed entry still compares equal to a live one. While
// cached, equal shorts stay the same object (what lua_topointer /
// lua_rawsetp see). FLAG: Go intern map OK (BRIEF Go-GC).
//
// GC helpers luaS_resize / luaS_remove / luaS_clearcache and STRCACHE are
// omitted: Go GC + flush() replace them (intentional; see NOTES.md).
//
// KEEP LUAI_HASHLIMIT step-hash (5.3.4). Do NOT replace with 5.4.9
// every-byte hashing.
package core

import (
	"strings"
)

const (
	LUAI_MAXSHORTLEN = 40 /* llimits.h */
	/*
	** Lua will use at most ~(2^LUAI_HASHLIMIT) bytes from a string to
	** compute its hash (lstring.c 5.3.4 default).
	*/
	LUAI_HASHLIMIT   = 5
	MEMERRMSG        = "not enough memory" /* lstring.c */
	maxInternedShort = 1 << 20             /* Go-only intern cap (FLAG) */
)

/*
** stringTable replaces C stringtable { TString **hash; int nuse; int size }.
** Open-hash chains / luaS_resize are not ported; see FLAG in NOTES.md.
*/
type stringTable struct {
	short map[string]*TString
	nuse  int
}

func newStringTable() *stringTable {
	return &stringTable{short: make(map[string]*TString, 1024)}
}

func (tb *stringTable) get(s string) *TString { return tb.short[s] }

/*
** flush drops the intern cache (Go substitute for GC unlink via luaS_remove).
** A full collection must free short strings that nothing else references;
** the cache is an extra root. Reachable strings stay; equality does not
** require the cache (luaS_eqshrstr content fallback).
*/
func (tb *stringTable) flush() {
	if tb == nil || tb.nuse == 0 {
		return
	}
	tb.short = make(map[string]*TString, 1024)
	tb.nuse = 0
}

/*
** put interns ts under s. Past the cap the cache is dropped so unbounded
** short-string invention cannot pin every entry forever.
*/
func (tb *stringTable) put(s string, ts *TString) {
	if tb.nuse >= maxInternedShort {
		tb.short = make(map[string]*TString, 1024)
		tb.nuse = 0
	}
	tb.short[s] = ts
	tb.nuse++
}

/*
** equality for long strings
** lstring.c luaS_eqlngstr
*/
func luaS_eqlngstr(a, b *TString) bool {
	return a == b || /* same instance or... */
		(len(a.val) == len(b.val) && a.val == b.val) /* equal length+contents */
}

/*
** luaS_hash — lstring.c 5.3.4:
**   h = seed ^ l;
**   step = (l >> LUAI_HASHLIMIT) + 1;
**   for (; l >= step; l -= step)
**     h ^= ((h<<5) + (h>>2) + cast_byte(str[l - 1]));
** Go uses a separate index i so len(str) stays intact; same samples.
*/
func luaS_hash(str string, seed uint32) uint32 {
	l := len(str)
	h := seed ^ uint32(l)
	step := (l >> LUAI_HASHLIMIT) + 1
	for i := l; i >= step; i -= step {
		h ^= (h << 5) + (h >> 2) + uint32(str[i-1]) /* cast_byte */
	}
	return h
}

/*
** luaS_hashlongstr — lstring.c (extra==0 → compute and cache hash).
** Seed was stored in ts.hash at createstrobj (G(L)->seed). L is unused
** for the hash itself; kept so call sites (ltable) need no signature change.
*/
func luaS_hashlongstr(L *State, ts *TString) uint32 {
	_ = L
	if ts.extra == 0 { /* no hash? */
		ts.hash = luaS_hash(ts.val, ts.hash) /* seed was stored in hash */
		ts.extra = 1                         /* now it has its hash */
	}
	return ts.hash
}

/*
** luaS_eqshrstr — C eqshrstr (lstring.h) is pointer-only (always interned).
** After a Go flush, equal shorts may be distinct objects; fall back to
** hash+content so Lua equality stays correct. FLAG: companion to intern map.
*/
func luaS_eqshrstr(a, b *TString) bool {
	return a == b || (a.hash == b.hash && a.val == b.val)
}

/*
** omitted (FLAG Go-GC):
**   void luaS_resize (lua_State *L, int newsize);
**   void luaS_clearcache (global_State *g);
**   void luaS_remove (lua_State *L, TString *ts);
** C open-hash rehash / white-strcache scrub / unlink. Go: map + flush().
*/

/*
** luaS_init — lstring.c:
**   luaS_resize(L, MINSTRTABSIZE);
**   g->memerrmsg = luaS_newliteral(L, MEMERRMSG); luaC_fix(...);
**   fill strcache with memerrmsg.
** Go: ensure intern table; create memErrMsg if missing (lstate may have
** already done so). No STRCACHE / no fix().
*/
func luaS_init(L *State) {
	if L.g.strt == nil {
		L.g.strt = newStringTable() /* ≈ luaS_resize(L, MINSTRTABSIZE) */
	}
	if L.g.memErrMsg == nil {
		L.g.memErrMsg = luaS_new(L, MEMERRMSG) /* no luaC_fix: Go GC */
	}
}

/*
** createstrobj (static in C) is inlined at internShort / createlngstrobj:
**   o = luaC_newobj(L, tag, sizelstring(l));
**   ts->hash = h; ts->extra = 0; getstr(ts)[l] = '\0';
** Go: &TString{tt, hash, val}; no trailing NUL (immutable Go string).
*/

/*
** luaS_createlngstrobj — lstring.c:
**   createstrobj(L, l, LUA_TLNGSTR, G(L)->seed); ts->u.lnglen = l;
** Go stores content in val (C memcpy's later); length is len(val).
*/
func luaS_createlngstrobj(L *State, s string) *TString {
	return &TString{tt: LUA_TLNGSTR, hash: L.g.seed, val: s} /* extra==0 */
}

/*
** internShort — lstring.c internshrstr (map lookup; no dead-resurrect /
** open-hash chain / size*2 grow; Go map + put cap replace those).
*/
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

/*
** luaS_newlstr — lstring.c: short → internshrstr; long → createlngstrobj+memcpy.
** C MAX_SIZE toobig check omitted (Go string / allocation limits).
*/
func luaS_newlstr(L *State, s string) *TString {
	if len(s) <= LUAI_MAXSHORTLEN { /* short string? */
		return L.internShort(s)
	}
	return luaS_createlngstrobj(L, s)
}

/*
** luaS_new — lstring.c routes through STRCACHE then strlen+newlstr.
** Go has no STRCACHE; equivalent to luaS_newlstr for a Go string.
*/
func luaS_new(L *State, s string) *TString { return luaS_newlstr(L, s) }

/*
** luaS_newCStr — C luaS_new(const char*): strlen stops at first '\0'.
** Needed for Go strings that may embed NUL (e.g. chunk names from load).
*/
func luaS_newCStr(L *State, s string) *TString {
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return luaS_newlstr(L, s)
}

/*
** luaS_newudata — lstring.c 5.3.4:
**   Udata *luaS_newudata(lua_State *L, size_t s);
** allocates sizeludata(s), sets len=s, metatable=NULL, one nil uservalue.
** Go: val any is the intentional port of the byte blob (and cgo blocks);
** always one uservalue slot (no 5.4 nuvalue / uv[]). Callers may set
** len / cblock elsewhere. MAX_SIZE toobig omitted.
*/
func luaS_newudata(L *State, val any) *Udata {
	_ = L
	u := &Udata{val: val, metatable: nil}
	setnilvalue(&u.user) /* setuservalue(L, u, luaO_nilobject) */
	return u
}

/* Header macros (lstring.h) not separate Go symbols:
**   sizelstring / sizeludata / sizeudata — Go GC / len(val); unused.
**   luaS_newliteral(L, s) — callers use luaS_new / luaS_newlstr.
**   isreserved(s) — llex isreservedToken map (no GC-pinned extra token id).
**   eqshrstr(a,b) — luaS_eqshrstr (with flush-safe content fallback).
*/
