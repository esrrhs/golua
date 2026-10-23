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

#include <ctype.h>
#include <stdlib.h>
#include <locale.h>
#include <time.h>
#include <string.h>

#ifdef _WIN32
#include <io.h>
#else
#include <unistd.h>
#endif

#include "cdlbridge.h"

/* Everything in this file exists to serve a host that can load a
   shared object, and the four variadic functions at the heart of it
   are only reachable from one: there is no dlopen on Windows, and
   every Go file that asks for any of this is built on unix only, so
   the callbacks they call would not be defined there either.  The
   file is still compiled -- see cdlbridge_nowin.go -- so it compiles
   to nothing. */
#if !defined(_WIN32)

/* formatted scratch buffers not yet freed. Tests watch this: a leak in
   lua_pushfstring shows up as it climbing. */
static int cdl_out_live;

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

static int cdl_fmt_fail(cdl_SB *b) {
  free(b->p);
  b->p = NULL;
  return 0 - 1;
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
      if (!cdl_sb_put(&b, fmt, strlen(fmt))) return cdl_fmt_fail(&b);
      break;
    }
    if (e > fmt && !cdl_sb_put(&b, fmt, (size_t)(e - fmt))) return cdl_fmt_fail(&b);
    if (e[1] == 0) { /* a trailing '%' with nothing after it */
      if (!cdl_sb_put(&b, "%", 1)) return cdl_fmt_fail(&b);
      break;
    }
    switch (e[1]) {
      case 's': {
        const char *s = va_arg(argp, char *);
        if (s == NULL) s = "(null)";
        if (!cdl_sb_put(&b, s, strlen(s))) return cdl_fmt_fail(&b);
        break;
      }
      case 'c': {
        char c = (char)(unsigned char)va_arg(argp, int);
        if (!cdl_sb_put(&b, &c, 1)) return cdl_fmt_fail(&b);
        break;
      }
      case 'd': {
        /* an int, printed the way Lua prints an integer */
        if (!cdl_sb_printf(&b, "%lld", (long long)va_arg(argp, int))) return cdl_fmt_fail(&b);
        break;
      }
      case 'I': {
        if (!cdl_sb_printf(&b, "%lld", (long long)va_arg(argp, lua_Integer)))
          return cdl_fmt_fail(&b);
        break;
      }
      case 'f': {
        /* LUAI_NUMFFORMAT, plus the ".0" C adds when the result looks
           like an integer (tostringbuff in lobject.c) */
        double d = (double)va_arg(argp, lua_Number);
        size_t at = b.len;
        if (!cdl_sb_printf(&b, "%.14g", d)) return cdl_fmt_fail(&b);
        if (strspn(b.p + at, "-0123456789") == b.len - at) {
          if (!cdl_sb_put(&b, ".0", 2)) return cdl_fmt_fail(&b);
        }
        break;
      }
      case 'p': {
        /* lua_pointer2str is a bare "%p", not "%p" with "0x" in front */
        void *p = va_arg(argp, void *);
        if (!cdl_sb_printf(&b, "%p", p)) return cdl_fmt_fail(&b);
        break;
      }
      case 'U': {
        char bf[CDL_UTF8BUFFSZ];
        int n = cdl_utf8esc(bf, (unsigned long)va_arg(argp, long));
        if (!cdl_sb_put(&b, bf + CDL_UTF8BUFFSZ - n, (size_t)n)) return cdl_fmt_fail(&b);
        break;
      }
      case '%': {
        if (!cdl_sb_put(&b, "%", 1)) return cdl_fmt_fail(&b);
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
    if (b.p == NULL) return cdl_fmt_fail(&b);
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
  if (st != 0 || out == NULL) {
    free(out);
    return NULL;
  }
  /* out is a scratch copy. C is given the Lua string's own pointer,
     which stays valid while that value does, and out is freed here
     instead of being leaked or parked in a ring. */
  cdl_out_live++;
  char *ret = cdl_pushlstring_cb(L, out, strlen(out));
  free(out);
  cdl_out_live--;
  return ret;
}

int cdl_out_live_count(void) { return cdl_out_live; }

const char *cdl_test_one_fstring(lua_State *L, int i) {
  return lua_pushfstring(L, "i=%d", i);
}

const char *lua_pushfstring(lua_State *L, const char *fmt, ...) {
  va_list ap;
  const char *r;
  va_start(ap, fmt);
  r = lua_pushvfstring(L, fmt, ap);
  va_end(ap);
  return r;
}

/* formatted luaL_error scratch not yet freed. The throw panics out of
   Go and never returns here, so the free has to happen before it. */
static int cdl_err_live;

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
    cdl_error_prepare_cb(L, msg);
    return cdl_error_throw_cb(L);
  }
  if (st != 0 || out == NULL) {
    cdl_error_prepare_cb(L, (char *)fmt);
    return cdl_error_throw_cb(L);
  }
  /* Copy into the Lua stack first, free the scratch, then throw. A
     single callback that both pushed and threw would skip this free. */
  cdl_err_live++;
  cdl_error_prepare_cb(L, out);
  free(out);
  cdl_err_live--;
  return cdl_error_throw_cb(L);
}

int cdl_err_live_count(void) { return cdl_err_live; }

/* The symbols lua_pcall and friends are //export-ed from Go. */
extern void lua_pushcclosure(lua_State *L, lua_CFunction fn, int n);
extern void lua_pushinteger(lua_State *L, lua_Integer n);
extern int lua_pcall(lua_State *L, int nargs, int nresults, int msgh);
extern lua_Integer lua_tointeger(lua_State *L, int idx);

static int cdl_test_boom(lua_State *L) {
  return luaL_error(L, "boom %d (%s)", (int)lua_tointeger(L, 1), "x");
}

int cdl_test_error_round(lua_State *L, int n) {
  int i;
  for (i = 0; i < n; i++) {
    lua_pushcclosure(L, cdl_test_boom, 0);
    lua_pushinteger(L, (lua_Integer)i);
    (void)lua_pcall(L, 1, 0, 0);
  }
  return cdl_err_live;
}


/* 5.3.4: no lua_setwarnf / lua_warning */
void cdl_test_install_warnf(lua_State *L, void *ud) { (void)L; (void)ud; }
void *cdl_test_warn_ud_seen(void) { return NULL; }

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

/* The argument of every ctype function has to be EOF or an unsigned
   char.  A bare 'char' that is negative is undefined, so the cast
   happens here and Go passes the byte as an int. */
int cdl_strcoll (const char *a, const char *b) { return strcoll(a, b); }
int cdl_tolower (int c) { return tolower((unsigned char)c); }
int cdl_toupper (int c) { return toupper((unsigned char)c); }
int cdl_isalpha (int c) { return isalpha((unsigned char)c); }
int cdl_iscntrl (int c) { return iscntrl((unsigned char)c); }
int cdl_isdigit (int c) { return isdigit((unsigned char)c); }
int cdl_isgraph (int c) { return isgraph((unsigned char)c); }
int cdl_islower (int c) { return islower((unsigned char)c); }
int cdl_ispunct (int c) { return ispunct((unsigned char)c); }
int cdl_isspace (int c) { return isspace((unsigned char)c); }
int cdl_isupper (int c) { return isupper((unsigned char)c); }
int cdl_isalnum (int c) { return isalnum((unsigned char)c); }
int cdl_isxdigit (int c) { return isxdigit((unsigned char)c); }

/* os.date's per-specifier strftime.  Broken-down time comes from the C
   library so %z/%Z see tm_gmtoff/tm_zone the way PUC-Lua does. */
size_t cdl_strftime (char *buf, size_t maxsize, const char *fmt,
                     long long epoch, int utc) {
  time_t t = (time_t)epoch;
  struct tm tmr, *stm;
  if (utc)
    stm = gmtime_r(&t, &tmr);
  else
    stm = localtime_r(&t, &tmr);
  if (stm == NULL)
    return (size_t)-1;
  return strftime(buf, maxsize, fmt, stm);
}


/* ---- a file handle as C sees it: see cdlbridge.h ---- */

/* One that never closes anything: it only has to be an address that is
   not NULL, because nothing here ever calls it. */
static int cdl_stream_closef (lua_State *L) { (void)L; return 0; }

FILE *cdl_fdopen (int fd) {
  /* The descriptor is duplicated: the FILE is closed here, and closing
     it must not close the file underneath Go.  Two owners of one
     descriptor would mean that whichever closed first closed the file
     out from under the other, and that the number is free to be handed
     out again before the second one is through with it. */
  int fd2 = dup(fd);
  FILE *f;
  if (fd2 < 0) return NULL;
  f = fdopen(fd2, "r+");
  if (f == NULL) f = fdopen(fd2, "r");
  if (f == NULL) f = fdopen(fd2, "w");
  if (f == NULL) { close(fd2); return NULL; }
  /* Unbuffered: Go does the buffering, and a second buffer on the same
     descriptor would put the two of them out of step. */
  setvbuf(f, NULL, _IONBF, 0);
  return f;
}

void cdl_stream_set (void *block, FILE *f) {
  void **p = (void **)block;
  p[0] = (void *)f;
  p[1] = (void *)&cdl_stream_closef;
}

void cdl_stream_clear (void *block) {
  void **p = (void **)block;
  p[0] = NULL;
  p[1] = NULL;
}

void cdl_stream_close (FILE *f) {
  if (f != NULL) fclose(f);
}

#endif /* !defined(_WIN32) */
