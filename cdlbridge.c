/*
** cdlbridge.c -- the four C API functions that Go cannot express.
**
** lua_pushfstring, lua_pushvfstring, luaL_error and lua_gc take '...',
** and neither direction of the cgo boundary can: a //export-ed Go
** function has a fixed signature, and cgo will not call a variadic C
** function from Go.  So these four are written in C, walk their own
** va_list, and hand the result to the Go side through the callbacks
** declared in cdlbridge.h.
**
** The formatting is *not* printf's.  luaO_pushvfstring (lobject.c) has
** its own set of conversions and rejects everything else:
**
**   %%  a percent sign          %s  a string (NULL prints "(null)")
**   %c  an int as a character   %d  an int, printed as a Lua integer
**   %I  a lua_Integer           %f  a lua_Number, printed "%.14g"
**   %p  a pointer, as "0x%p"    %U  a long, as a UTF-8 sequence
**
** anything else is a Lua error, "invalid option '%X' to
** 'lua_pushfstring'".  printf would both accept more ('%.2f', '%lld')
** and print %d as C's int rather than Lua's, so it cannot be used.
*/

#include <stdlib.h>
#include <locale.h>

#include "cdlbridge.h"

#define CDL_NKEEP 4  /* how many formatted results stay valid at once */

static char *cdl_keep_buf[CDL_NKEEP];
static size_t cdl_keep_size[CDL_NKEEP];
static int cdl_keep_next;

/* cdl_keep answers a copy of s/n that stays valid until CDL_NKEEP more
   calls have been made.  PUC-Lua answers the string on the Lua stack;
   that lives in Go memory, which C may not hold, so what C gets is a
   copy that is good for the "format it and use it at once" idiom. */
static char *cdl_keep(const char *s, size_t n) {
  int i = cdl_keep_next;
  cdl_keep_next = (cdl_keep_next + 1) % CDL_NKEEP;
  if (cdl_keep_size[i] < n + 1) {
    char *p = (char *)realloc(cdl_keep_buf[i], n + 1);
    if (p == NULL) return NULL;
    cdl_keep_buf[i] = p;
    cdl_keep_size[i] = n + 1;
  }
  memcpy(cdl_keep_buf[i], s, n + 1);
  return cdl_keep_buf[i];
}

/* ---- the growable buffer the formatter writes into ---- */

typedef struct {
  char *p;
  size_t len;
  size_t cap;
} cdl_SB;

static int cdl_sb_reserve(cdl_SB *b, size_t extra) {
  if (b->len + extra + 1 <= b->cap) return 1;
  size_t cap = b->cap ? b->cap : 128;
  while (cap < b->len + extra + 1) cap *= 2;
  char *p = (char *)realloc(b->p, cap);
  if (p == NULL) return 0;
  b->p = p;
  b->cap = cap;
  return 1;
}

static int cdl_sb_put(cdl_SB *b, const char *s, size_t n) {
  if (!cdl_sb_reserve(b, n)) return 0;
  memcpy(b->p + b->len, s, n);
  b->len += n;
  b->p[b->len] = 0;
  return 1;
}

static int cdl_sb_printf(cdl_SB *b, const char *fmt, ...) {
  va_list ap;
  int n;
  va_start(ap, fmt);
  if (!cdl_sb_reserve(b, 64)) { va_end(ap); return 0; }
  n = vsnprintf(b->p + b->len, b->cap - b->len, fmt, ap);
  va_end(ap);
  if (n < 0) return 0;
  if ((size_t)n >= b->cap - b->len) { /* did not fit: retry with room */
    if (!cdl_sb_reserve(b, (size_t)n)) return 0;
    va_start(ap, fmt);
    vsnprintf(b->p + b->len, b->cap - b->len, fmt, ap);
    va_end(ap);
  }
  b->len += (size_t)n;
  b->p[b->len] = 0;
  return 1;
}

/* ---- luaO_utf8esc (lobject.c), for %U ---- */
#define CDL_UTF8BUFFSZ 8
static int cdl_utf8esc(char *buff, unsigned long x) {
  int n = 1; /* bytes put in the buffer, backwards */
  if (x < 0x80) { /* ascii? */
    buff[CDL_UTF8BUFFSZ - 1] = (char)x;
  } else { /* needs continuation bytes */
    unsigned int mfb = 0x3f; /* maximum that fits in the first byte */
    do {
      buff[CDL_UTF8BUFFSZ - (n++)] = (char)(0x80 | (x & 0x3f));
      x >>= 6;         /* remove the bits just added */
      mfb >>= 1;       /* one less bit available in the first byte */
    } while (x > mfb); /* still needs continuation bytes? */
    buff[CDL_UTF8BUFFSZ - n] = (char)((~mfb << 1) | x); /* the first byte */
  }
  return n;
}

/*
** cdl_vformat renders fmt/argp the way luaO_pushvfstring does.  It
** answers 0 on success (with the text in *out, malloc'd), 1 when the
** format names an option Lua does not have, and -1 when it ran out of
** memory.  *opt is then the offending character.
*/
static int cdl_vformat(lua_State *L, const char *fmt, va_list argp, char **out,
                       int *opt) {
  cdl_SB b;
  b.p = NULL;
  b.len = b.cap = 0;
  while (*fmt != 0) {
    const char *e = strchr(fmt, '%');
    if (e == NULL) { /* no more '%': the rest is literal */
      if (!cdl_sb_put(&b, fmt, strlen(fmt))) return -1;
      break;
    }
    if (e > fmt && !cdl_sb_put(&b, fmt, (size_t)(e - fmt))) return -1;
    if (e[1] == 0) { /* a trailing '%' with nothing after it */
      if (!cdl_sb_put(&b, "%", 1)) return -1;
      break;
    }
    switch (e[1]) {
      case 's': {
        const char *s = va_arg(argp, char *);
        if (s == NULL) s = "(null)";
        if (!cdl_sb_put(&b, s, strlen(s))) return -1;
        break;
      }
      case 'c': {
        char c = (char)(unsigned char)va_arg(argp, int);
        if (!cdl_sb_put(&b, &c, 1)) return -1;
        break;
      }
      case 'd': {
        /* an int, printed the way Lua prints an integer */
        if (!cdl_sb_printf(&b, "%lld", (long long)va_arg(argp, int))) return -1;
        break;
      }
      case 'I': {
        if (!cdl_sb_printf(&b, "%lld", (long long)va_arg(argp, lua_Integer)))
          return -1;
        break;
      }
      case 'f': {
        /* LUAI_NUMFFORMAT, plus the ".0" C adds when the result looks
           like an integer (tostringbuff in lobject.c) */
        double d = (double)va_arg(argp, lua_Number);
        size_t at = b.len;
        if (!cdl_sb_printf(&b, "%.14g", d)) return -1;
        if (strspn(b.p + at, "-0123456789") == b.len - at) {
          if (!cdl_sb_put(&b, ".0", 2)) return -1;
        }
        break;
      }
      case 'p': {
        /* lua_pointer2str is a bare "%p", not "%p" with "0x" in front */
        void *p = va_arg(argp, void *);
        if (!cdl_sb_printf(&b, "%p", p)) return -1;
        break;
      }
      case 'U': {
        char bf[CDL_UTF8BUFFSZ];
        int n = cdl_utf8esc(bf, (unsigned long)va_arg(argp, long));
        if (!cdl_sb_put(&b, bf + CDL_UTF8BUFFSZ - n, (size_t)n)) return -1;
        break;
      }
      case '%': {
        if (!cdl_sb_put(&b, "%", 1)) return -1;
        break;
      }
      default: {
        *opt = (unsigned char)e[1];
        free(b.p);
        *out = NULL;
        return 1;
      }
    }
    fmt = e + 2; /* the '%' and the specifier */
  }
  if (b.p == NULL) { /* nothing at all was written */
    b.p = (char *)malloc(1);
    if (b.p == NULL) return -1;
    b.p[0] = 0;
  }
  *out = b.p;
  return 0;
}

const char *lua_pushvfstring(lua_State *L, const char *fmt, va_list ap) {
  char *out = NULL;
  int opt = 0;
  int st = cdl_vformat(L, fmt, ap, &out, &opt);
  if (st == 1) { /* an option Lua does not have: raise, as C does */
    char msg[64];
    snprintf(msg, sizeof msg, "invalid option '%%%c' to 'lua_pushfstring'", opt);
    cdl_runerror_cb(L, msg);
    return NULL;
  }
  if (st != 0 || out == NULL) return NULL;
  cdl_pushlstring_cb(L, out, strlen(out));
  return cdl_keep(out, strlen(out));
}

const char *lua_pushfstring(lua_State *L, const char *fmt, ...) {
  va_list ap;
  const char *r;
  va_start(ap, fmt);
  r = lua_pushvfstring(L, fmt, ap);
  va_end(ap);
  return r;
}

int luaL_error(lua_State *L, const char *fmt, ...) {
  va_list ap;
  char *out = NULL;
  int opt = 0;
  int st;
  va_start(ap, fmt);
  st = cdl_vformat(L, fmt, ap, &out, &opt);
  va_end(ap);
  if (st == 1) {
    char msg[64];
    snprintf(msg, sizeof msg, "invalid option '%%%c' to 'lua_pushfstring'", opt);
    return cdl_error_cb(L, msg);
  }
  if (st != 0 || out == NULL) return cdl_error_cb(L, (char *)fmt);
  st = cdl_error_cb(L, out);
  free(out);
  return st;
}

int lua_gc(lua_State *L, int what, ...) {
  /* LUA_GC* from lua.h; repeated here so this file stands alone */
  int a1 = 0, a2 = 0, a3 = 0;
  va_list ap;
  va_start(ap, what);
  switch (what) {
    case 5:  /* LUA_GCSTEP */
    case 6:  /* LUA_GCSETPAUSE */
    case 7:  /* LUA_GCSETSTEPMUL */
    case 9:  /* LUA_GCISRUNNING */
      a1 = va_arg(ap, int);
      break;
    case 10: /* LUA_GCGEN */
      a1 = va_arg(ap, int);
      a2 = va_arg(ap, int);
      break;
    case 11: /* LUA_GCINC */
      a1 = va_arg(ap, int);
      a2 = va_arg(ap, int);
      a3 = va_arg(ap, int);
      break;
    default:
      break;
  }
  va_end(ap);
  return cdl_gc_cb(L, what, a1, a2, a3);
}

/* The allocator an extension gets from lua_getallocf: PUC-Lua's own
   'l_alloc' (lauxlib.c). It has to be C, because the extension calls
   the pointer it is given -- lpeg compiles a pattern through it, and
   would otherwise jump to nothing at all. */
void *cdl_alloc (void *ud, void *ptr, size_t osize, size_t nsize) {
  (void)ud; (void)osize;  /* not used */
  if (nsize == 0) {
    free(ptr);
    return NULL;
  }
  else
    return realloc(ptr, nsize);
}

void *cdl_allocfn (void) {
  return (void *)cdl_alloc;
}

/* The locale, for os.setlocale. Go has no setlocale of its own, and the
   C library's is the one that decides how a number is written -- so it
   has to be C's, or os.setlocale does nothing that can be seen. */
const char *cdl_setlocale (int cat, const char *name) {
  return setlocale(cat, name);
}

const char *cdl_decpoint (void) {
  struct lconv *lc = localeconv();
  return lc->decimal_point;
}

int cdl_lc_cat (int which) {
  switch (which) {
    case 1: return LC_COLLATE;
    case 2: return LC_CTYPE;
    case 3: return LC_MONETARY;
    case 4: return LC_NUMERIC;
    case 5: return LC_TIME;
    default: return LC_ALL;
  }
}
