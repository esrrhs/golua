/*
** cdlbridge.h -- the C side of the dynamic-loading bridge.
**
** This header is for the *host*, not for extensions: include/lua.h and
** include/lauxlib.h are what an extension sees, and this is what the
** host's cgo files see.  The two have to agree on every layout that
** crosses the boundary, and the comments below say which ones those
** are.
**
** Two rules shape it:
**
**   - Nothing here is const-qualified.  cgo declares a //export-ed Go
**     function taking 'char *', and a 'const char *' declaration of
**     the same name is a *different* type in C, so the two would not
**     link.  (This is a host-internal detail; the extension's own
**     header keeps the consts that PUC-Lua has.)
**
**   - Only declarations and 'static inline' helpers live here, because
**     every cgo file in the package includes it: a real definition
**     would be compiled once per file.
**
** The variadic functions -- lua_pushfstring, lua_pushvfstring,
** luaL_error and lua_gc -- cannot be written in Go at all (neither
** //export nor a call from Go can express '...'), so they are written
** in C in cdlbridge.c and hand the finished text to the Go side
** through the callbacks declared at the bottom.
*/
#ifndef cdlbridge_h
#define cdlbridge_h

/* dlfcn is the one thing here the platform decides: there is no dlopen
   on Windows, and this file is compiled as part of the package whenever
   cgo is on at all -- so what it needs has to be asked for
   conditionally, not assumed.  See cdlbridge_nowin.go for why the file
   is compiled there at all. */
#if !defined(_WIN32)
#include <dlfcn.h>
#endif

#include <stdarg.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- types an extension's lua.h must match ---- */

typedef struct lua_State lua_State;
struct lua_Debug;
typedef long long lua_Integer;
typedef double lua_Number;
typedef unsigned long long lua_Unsigned;
typedef ptrdiff_t lua_KContext;

typedef int (*lua_CFunction)(lua_State *L);
typedef int (*lua_KFunction)(lua_State *L, int status, lua_KContext ctx);
typedef const char *(*lua_Reader)(lua_State *L, void *data, size_t *size);
typedef int (*lua_Writer)(lua_State *L, const void *p, size_t sz, void *ud);
typedef void *(*lua_Alloc)(void *ud, void *ptr, size_t osize, size_t nsize);
typedef void (*lua_WarnFunction)(void *ud, const char *msg, int tocont);
typedef void (*lua_Hook)(lua_State *L, struct lua_Debug *ar);

/* The allocator lua_getallocf hands out. An extension that asks for the
   allocator calls it directly -- lpeg compiles a pattern through it --
   so it has to be a real C function, not something in Go. */
void *cdl_alloc(void *ud, void *ptr, size_t osize, size_t nsize);
void *cdl_allocfn(void); /* the address of cdl_alloc, as a void * */

/* The C library's own locale, which os.setlocale sets: there is no way
   to reach it from Go, and a number has to be written the way the
   locale says (1.5 is "1,5" under pt_BR).

   cdl_lc_cat turns the category os.setlocale was given -- by its place
   in Lua's list: "all", "collate", "ctype", "monetary", "numeric",
   "time" -- into the LC_* it means. It is a function because the LC_*
   are not plain numbers on every platform, and cgo cannot read a macro
   it cannot evaluate. */
const char *cdl_setlocale(int cat, const char *name);
const char *cdl_decpoint(void);
int cdl_lc_cat(int which);

/* strcoll and the ctype predicates.  Lua compares strings with strcoll,
   one segment at a time, and string.upper / string.lower / '%a' ask
   toupper and tolower.  A NULL name to cdl_setlocale queries. */
int cdl_strcoll(const char *a, const char *b);
int cdl_tolower(int c);
int cdl_toupper(int c);
int cdl_isalpha(int c);
int cdl_iscntrl(int c);
int cdl_isdigit(int c);
int cdl_isgraph(int c);
int cdl_islower(int c);
int cdl_ispunct(int c);
int cdl_isspace(int c);
int cdl_isupper(int c);
int cdl_isalnum(int c);
int cdl_isxdigit(int c);

#include <stdlib.h>

/* LUA_IDSIZE is 60 in PUC-Lua; short_src below must be the same size */
#define CDL_IDSIZE 60

/* The initial size of the block behind a luaL_Buffer: LUAL_BUFFERSIZE in
   include/lauxlib.h, and the two have to agree. */
#define CDL_BUFFERSIZE 8192

struct lua_Debug {
  int event;
  char *name;
  char *namewhat;
  char *what;
  char *source;
  size_t srclen;
  int currentline;
  int linedefined;
  int lastlinedefined;
  unsigned char nups;
  unsigned char nparams;
  char isvararg;
  char istailcall;
  unsigned short ftransfer;
  unsigned short ntransfer;
  char short_src[CDL_IDSIZE];
  void *i_ci; /* private: what the port's own activation record is */
};
typedef struct lua_Debug lua_Debug;

typedef struct luaL_Reg {
  char *name;
  lua_CFunction func;
} luaL_Reg;

/*
** The fields up to 'L' are the ones include/lauxlib.h declares and the
** luaL_addchar macro reads, so the layout has to be identical.  'ref'
** is this port's own slot number in a table kept on the Go side.
*/
typedef struct luaL_Buffer {
  char *b;
  size_t size;
  size_t n;
  lua_State *L;
  int ref;
} luaL_Buffer;

static inline int *cdl_buf_refp(luaL_Buffer *B) { return &B->ref; }
static inline void cdl_buf_init(luaL_Buffer *B, lua_State *L, char *b,
                                size_t size) {
  B->b = b;
  B->size = size;
  B->n = 0;
  B->L = L;
  B->ref = 0;
}

/* Go cannot call a C function pointer, but C can. */
static inline int cdl_call_cfunction(lua_CFunction f, lua_State *L) { return f(L); }
static inline int cdl_call_kfunction(lua_KFunction f, lua_State *L, int status,
                                     lua_KContext ctx) {
  return f(L, status, ctx);
}
static inline int cdl_call_writer(lua_Writer f, lua_State *L, void *p, size_t sz,
                                  void *ud) {
  return f(L, p, sz, ud);
}
static inline void cdl_call_hook(lua_Hook f, lua_State *L, lua_Debug *ar) { f(L, ar); }
static inline void cdl_call_warnf(lua_WarnFunction f, char *msg, int tocont,
                                  void *ud) {
  f(ud, msg, tocont);
}

/* The pointer arithmetic Go may not do itself: reaching the nth entry
   of a NULL-terminated list, and filling short_src, which is a fixed
   array inside lua_Debug rather than a pointer. */
static inline char **cdl_lst_at(char **lst, int i) { return &lst[i]; }
static inline void cdl_copy_str(char *dst, char *src, size_t n, size_t cap) {
  size_t m = (n < cap - 1) ? n : cap - 1;
  if (src != NULL && m > 0) memcpy(dst, src, m);
  dst[m] = 0;
}

static inline const char *cdl_call_reader(lua_Reader f, lua_State *L, void *d,
                                          size_t *sz) {
  return f(L, d, sz);
}

/* luaL_Reg's second field is called 'func', which Go cannot spell, so
   it is read at its offset instead: one pointer past 'name'. */
static inline luaL_Reg *cdl_reg_at(luaL_Reg *l, int i) { return &l[i]; }
static inline lua_CFunction cdl_reg_func(luaL_Reg *r) {
  return *(lua_CFunction *)((char *)r + sizeof(r->name));
}

/* PUC-Lua opens with RTLD_NOW|RTLD_LOCAL, and the flags show up in
   dlerror's message, so they have to match.  On Windows there is no
   dlopen to call: the two functions answer NULL, and nothing here
   asks, because nothing here loads. */
#if !defined(_WIN32)

static inline void *cdl_dlopen(const char *path) { return dlopen(path, RTLD_NOW | RTLD_LOCAL); }

/* Except when the symbol asked for is "*" -- 'load only this library'
   -- where loadlib.c passes RTLD_GLOBAL instead, so that a library
   loaded that way lends its symbols to the ones opened after it.  Lua's
   own suite needs exactly this: attrib.lua opens lib1 with "*" and then
   loads lib11, whose luaopen_lib11 calls lib1's lib1_export. */
static inline void *cdl_dlopen_global(const char *path) {
  return dlopen(path, RTLD_NOW | RTLD_GLOBAL);
}

#else /* _WIN32 */

static inline void *cdl_dlopen(const char *path) { return NULL; }
static inline void *cdl_dlopen_global(const char *path) { return NULL; }

#endif

/* ---- the callbacks cdlbridge.c uses to reach the Go side ----
   These are //export-ed from Go, so cgo declares them too; the
   declarations must match cgo's exactly, which is why no const. */
extern char *cdl_pushlstring_cb(lua_State *L, char *s, size_t n);
/* prepare pushes the error object; throw raises it. Split so luaL_error
   can free a heap-formatted message before the Go panic unwinds. */
extern void cdl_error_prepare_cb(lua_State *L, char *s);
extern int cdl_error_throw_cb(lua_State *L);
extern int cdl_gc_cb(lua_State *L, int what, int a1, int a2, int a3);
/* the "invalid option" error of lua_pushfstring */
extern int cdl_runerror_cb(lua_State *L, char *s);

/* ---- the variadic functions, defined in cdlbridge.c ---- */
const char *lua_pushvfstring(lua_State *L, const char *fmt, va_list ap);
const char *lua_pushfstring(lua_State *L, const char *fmt, ...);
int luaL_error(lua_State *L, const char *fmt, ...);
int lua_gc(lua_State *L, int what, ...);

/* test hooks: how many formatted buffers lua_pushfstring / luaL_error
   have not freed, and one call of each. */
int cdl_out_live_count(void);
const char *cdl_test_one_fstring(lua_State *L, int i);
int cdl_err_live_count(void);
int cdl_test_error_round(lua_State *L, int n);

/* the buffer keeps the state it was made with, which luaL_addgsub needs */
static inline lua_State *cdl_buf_L(luaL_Buffer *B) { return B->L; }

#endif

/* ---- a file handle as C sees it ----
   PUC-Lua's file handle is a luaL_Stream: a FILE* and a close function,
   in that order, and an extension -- LuaFileSystem's setmode is one --
   casts the userdata it is handed to one and reads straight out of it.
   The port's handles are Go's, so the block behind one is filled in as
   if it were C's: a real FILE* on the same descriptor (which is what
   makes fileno and friends work) and a close function that is nothing
   but an address, since C never calls it here.  Both are cleared when
   the file is closed, which is how an extension sees a closed handle
   the way it sees one in PUC-Lua. */
FILE *cdl_fdopen (int fd);
void cdl_stream_set (void *block, FILE *f);
void cdl_stream_clear (void *block);
void cdl_stream_close (FILE *f);
