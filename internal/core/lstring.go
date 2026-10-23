// Translation of lstring.h / lstring.c (PUC-Lua 5.4.9).
//
// Short strings are interned in a strong map[string]*TString (Go stand-in
// for C's stringtable hash + luaS_resize / internshrstr). A weak map made
// the miss path several times slower than building the string (string.format
// hot path). The map is capped (maxInternedShort): past the cap it is
// dropped and later equal shorts become new objects. Equality compares
// content, so a flushed entry still compares equal to a live one. While
// cached, equal shorts stay the same object (what lua_topointer /
// lua_rawsetp see). FLAG: Go intern map OK (BRIEF).
//
// GC helpers luaS_resize / luaS_remove / luaS_clearcache, tablerehash,
// growstrtab, and STRCACHE are omitted: Go GC + flush() replace them
// (intentional; BRIEF Skip GC). 5.3 LUAI_HASHLIMIT step-hash purged —
// 5.4.9 hashes every byte.
package core

import (
	"strings"
)

// ---------------------------------------------------------------------
// Header (lstring.h) + limits used here
// ---------------------------------------------------------------------

const (
	LUAI_MAXSHORTLEN = 40 /* llimits.h */
	MEMERRMSG        = "not enough memory"
	MINSTRTABSIZE    = 128     /* llimits.h; C initial stringtable size */
	maxInternedShort = 1 << 20 /* Go-only intern map cap */
)

// sizelstring — N/A under Go string heap (C: offsetof + (l+1)*sizeof(char)).
// isreserved — in llex.go (C macro: tt==LUA_VSHRSTR && extra>0).
// eqshrstr → luaS_eqshrstr below (content fallback after flush).

// luaS_newliteral is C #define luaS_newliteral(L,s) luaS_newlstr(L,"" s, sizeof(s)-1).
func luaS_newliteral(L *State, s string) *TString {
	return luaS_newlstr(L, s)
}

// stringTable replaces C stringtable { hash[], nuse, size }.
type stringTable struct {
	short map[string]*TString
	nuse  int
}

func newStringTable() *stringTable {
	/* C starts at MINSTRTABSIZE buckets; Go map hint is larger (perf FLAG). */
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

// ---------------------------------------------------------------------
// lstring.c
// ---------------------------------------------------------------------

/*
** equality for long strings
 */
func luaS_eqlngstr(a, b *TString) bool {
	lena := len(a.val) /* C: a->u.lnglen */
	return a == b ||   /* same instance or... */
		(lena == len(b.val) && /* equal length and ... */
			a.val == b.val) /* equal contents (C: memcmp) */
}

/*
** luaS_hash — lstring.c 5.4.9: seed ^ l, then every byte (no LUAI_HASHLIMIT).
** C: unsigned int luaS_hash(const char *str, size_t l, unsigned int seed)
 */
func luaS_hash(str string, l int, seed uint32) uint32 {
	h := seed ^ uint32(l)
	for ; l > 0; l-- {
		h ^= (h << 5) + (h >> 2) + uint32(str[l-1]) /* cast_byte */
	}
	return h
}

/*
** luaS_hashlongstr — lstring.c (extra==0 → compute and cache hash).
** Seed was stored in ts.hash at createstrobj (G(L)->seed). L is unused
** for the hash itself; kept so call sites (ltable) need no signature change.
** C signature is luaS_hashlongstr(TString *ts) only.
 */
func luaS_hashlongstr(L *State, ts *TString) uint32 {
	_ = L
	if ts.extra == 0 { /* no hash? */
		ts.hash = luaS_hash(ts.val, len(ts.val), ts.hash) /* seed was stored in hash */
		ts.extra = 1                                      /* now it has its hash */
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
** luaS_resize / luaS_clearcache / luaS_remove / tablerehash / growstrtab:
** omitted (C string-table GC). Go map + flush stand in; see file header.
 */

/*
** Initialize the string table and the string cache
** C also allocates MINSTRTABSIZE, fixes memerrmsg, fills strcache.
** Go: ensure intern table; pre-create memerrmsg (luaC_fix / STRCACHE N/A).
 */
func luaS_init(L *State) {
	g := L.g
	if g.strt == nil {
		g.strt = newStringTable()
	}
	/* pre-create memory-error message (C: luaS_newliteral + luaC_fix) */
	g.memErrMsg = luaS_newliteral(L, MEMERRMSG)
	/* STRCACHE fill omitted — no API strcache under Go GC */
}

/*
** creates a new string object (C createstrobj; no luaC_newobj / trailing NUL)
 */
func createstrobj(L *State, s string, tag int, h uint32) *TString {
	_ = L
	return &TString{tt: tag, hash: h, extra: 0, val: s}
}

/*
** luaS_createlngstrobj — C: createstrobj(..., LUA_VLNGSTR, G(L)->seed);
** lnglen = l; shrlen = 0xFF. Go stores content in val (C memcpy's later);
** length is len(val); no separate shrlen/lnglen fields on TString.
 */
func luaS_createlngstrobj(L *State, s string) *TString {
	ts := createstrobj(L, s, LUA_VLNGSTR, L.g.seed)
	/* C: ts->u.lnglen = l; ts->shrlen = 0xFF — tag + len(val) suffice */
	return ts
}

/*
** Checks whether short string exists and reuses it or creates a new one.
** C: open-hash chain, may resurrect dead, growstrtab.
** Go: map lookup; no dead-resurrect / open-hash; put cap replaces grow.
 */
func internshrstr(L *State, str string) *TString {
	g := L.g
	tb := g.strt
	h := luaS_hash(str, len(str), g.seed)
	if ts := tb.get(str); ts != nil {
		/* found! (C may changewhite if dead — Go GC, no resurrect) */
		return ts
	}
	/* else must create a new string */
	ts := createstrobj(L, str, LUA_VSHRSTR, h)
	tb.put(str, ts)
	return ts
}

/*
** new string (with explicit length)
** C MAX_SIZE toobig check omitted (Go string / allocation limits).
 */
func luaS_newlstr(L *State, s string) *TString {
	if len(s) <= LUAI_MAXSHORTLEN { /* short string? */
		return internshrstr(L, s)
	}
	/* long string: C createlngstrobj + memcpy; Go passes content in s */
	return luaS_createlngstrobj(L, s)
}

/*
** Create or reuse a zero-terminated string.
** C: STRCACHE by pointer address, then strlen + newlstr.
** Go has no STRCACHE; equivalent to luaS_newlstr for a Go string
** (use luaS_newCStr when C strlen / first-NUL semantics are required).
 */
func luaS_new(L *State, s string) *TString {
	return luaS_newlstr(L, s)
}

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
** luaS_newudata — lstring.c 5.4.9:
**   Udata *luaS_newudata(lua_State *L, size_t s, int nuvalue);
** allocates sizeudata(nuvalue,s), sets len/nuvalue, metatable=NULL, nil uvs.
** Go: val any is the intentional port of the byte blob (and cgo blocks);
** callers may set len / cblock elsewhere. nuvalue drives uv[].
** Arg order kept as (L, nuvalue, val) — existing call sites.
 */
func luaS_newudata(L *State, nuvalue int, val any) *Udata {
	_ = L
	/* C: luaM_toobig if s too large — omitted (Go heap) */
	u := &Udata{nuvalue: uint16(nuvalue), val: val, metatable: nil}
	if nuvalue > 0 {
		u.uv = make([]Value, nuvalue)
		for i := 0; i < nuvalue; i++ {
			setnilvalue(&u.uv[i])
		}
	}
	return u
}
