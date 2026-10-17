/*
** lauxlib.h -- the auxiliary library, as an extension sees it.
**
** As with lua.h, this is a header for extensions.  The buffer API is
** declared here as functions rather than macros so that the port has a
** single implementation of each.
*/
#ifndef lauxlib_h
#define lauxlib_h

#include <stddef.h>
#include <stdio.h>

#include "lua.h"

/* extra error code for 'luaL_loadfilex' */
#define LUA_ERRFILE (LUA_ERRERR + 1)

/* the name of the global table (lauxlib.h) */
#define LUA_GNAME "_G"

typedef struct luaL_Buffer luaL_Buffer;

typedef struct luaL_Reg {
  const char *name;
  lua_CFunction func;
} luaL_Reg;

#define LUAL_NUMSIZES (sizeof(lua_Integer) * 16 + sizeof(lua_Number))

/* key, in the registry, for the metatables of userdata types */
#define LUA_LOADED_TABLE "_LOADED"
#define LUA_PRELOAD_TABLE "_PRELOAD"

/* ---- version and metatable helpers ---- */
void luaL_checkversion_(lua_State *L, lua_Number ver, size_t sz);
#define luaL_checkversion(L) \
  luaL_checkversion_(L, LUA_VERSION_NUM, LUAL_NUMSIZES)

#define luaL_argcheck(L, cond,arg,extramsg) \
  ((void)((cond) || luaL_argerror(L, (arg), (extramsg))))
#define luaL_argexpected(L,cond,arg,tname) \
  ((void)((cond) || luaL_typeerror(L, (arg), (tname))))
#define luaL_checkstring(L,n)     (luaL_checklstring(L, (n), NULL))
#define luaL_optstring(L,n,d)     (luaL_optlstring(L, (n), (d), NULL))
#define luaL_typename(L,i)        lua_typename(L, lua_type(L, (i)))
#define luaL_pushfail(L)          lua_pushnil(L)
#define luaL_getmetatable(L,n)    (lua_getfield(L, LUA_REGISTRYINDEX, (n)))
#define luaL_loadbuffer(L,s,sz,n) luaL_loadbufferx(L, s, sz, n, NULL)
#define luaL_dofile(L, fn) \
  (luaL_loadfile(L, fn) || lua_pcall(L, 0, LUA_MULTRET, 0))
#define luaL_dostring(L, s) \
  (luaL_loadstring(L, s) || lua_pcall(L, 0, LUA_MULTRET, 0))

int  luaL_getmetafield(lua_State *L, int obj, const char *e);
int  luaL_callmeta(lua_State *L, int obj, const char *e);
const char *luaL_tolstring(lua_State *L, int idx, size_t *len);
int  luaL_argerror(lua_State *L, int arg, const char *extramsg);
int  luaL_typeerror(lua_State *L, int arg, const char *tname);
const char *luaL_checklstring(lua_State *L, int arg, size_t *len);
const char *luaL_optlstring(lua_State *L, int arg, const char *def, size_t *len);
lua_Number  luaL_checknumber(lua_State *L, int arg);
lua_Number  luaL_optnumber(lua_State *L, int arg, lua_Number def);
lua_Integer luaL_checkinteger(lua_State *L, int arg);
lua_Integer luaL_optinteger(lua_State *L, int arg, lua_Integer def);

void luaL_checkstack(lua_State *L, int sz, const char *msg);
void luaL_checktype(lua_State *L, int arg, int t);
void luaL_checkany(lua_State *L, int arg);

int   luaL_newmetatable(lua_State *L, const char *tname);
void  luaL_setmetatable(lua_State *L, const char *tname);
void *luaL_testudata(lua_State *L, int arg, const char *tname);
void *luaL_checkudata(lua_State *L, int arg, const char *tname);

void luaL_where(lua_State *L, int lvl);
lua_Integer luaL_len(lua_State *L, int idx);
int  luaL_error(lua_State *L, const char *fmt, ...);

int  luaL_checkoption(lua_State *L, int arg, const char *def,
                      const char *const lst[]);

int  luaL_fileresult(lua_State *L, int stat, const char *fname);
int  luaL_execresult(lua_State *L, int stat);

/* predefined references */
int luaL_ref(lua_State *L, int t);
void luaL_unref(lua_State *L, int t, int ref);

int luaL_loadfilex(lua_State *L, const char *filename, const char *mode);
#define luaL_loadfile(L,f) luaL_loadfilex(L, f, NULL)

int luaL_loadbufferx(lua_State *L, const char *buff, size_t sz,
                     const char *name, const char *mode);
int luaL_loadstring(lua_State *L, const char *s);

lua_State *luaL_newstate(void);

const char *luaL_gsub(lua_State *L, const char *s, const char *p,
                      const char *r);

void luaL_setfuncs(lua_State *L, const luaL_Reg *l, int nup);
int  luaL_getsubtable(lua_State *L, int idx, const char *fname);
void luaL_traceback(lua_State *L, lua_State *L1, const char *msg, int level);
void luaL_requiref(lua_State *L, const char *modname, lua_CFunction openf,
                   int glb);

#define luaL_newlibtable(L,l) \
  lua_createtable(L, 0, sizeof(l)/sizeof((l)[0]) - 1)
#define luaL_newlib(L,l) (luaL_newlibtable(L,l), luaL_setfuncs(L,(l),0))

/* ---- the buffer API ---- */
#define LUAL_BUFFERSIZE 8192

struct luaL_Buffer {
  char   *b;      /* not used by the host; kept for layout compatibility */
  size_t  size;
  size_t  n;
  lua_State *L;
  int     ref;    /* slot in the host's per-state buffer table */
};

#define luaL_addchar(B,c) \
  ((void)((B)->n < (B)->size || luaL_prepbuffsize(B, 1)), luaL_addchar_(B, (char)(c)))

void luaL_addchar_(luaL_Buffer *B, char c);
void luaL_buffinit(lua_State *L, luaL_Buffer *B);
char *luaL_prepbuffsize(luaL_Buffer *B, size_t sz);
void luaL_addgsub(luaL_Buffer *B, const char *s, const char *p,
                  const char *r);
char *luaL_buffinitsize(lua_State *L, luaL_Buffer *B, size_t sz);
void luaL_addlstring(luaL_Buffer *B, const char *s, size_t l);
void luaL_addstring(luaL_Buffer *B, const char *s);
void luaL_addvalue(luaL_Buffer *B);
void luaL_pushresult(luaL_Buffer *B);
void luaL_pushresultsize(luaL_Buffer *B, size_t sz);

/* an arithmetic operation on lua_Integers, done in lua_Unsigned so that
   the overflow is defined (lauxlib.h) */
#define luaL_intop(op,v1,v2)  \
	((lua_Integer)((lua_Unsigned)(v1) op (lua_Unsigned)(v2)))

/* luaL_opt: 'f' applied to argument 'n', or 'd' when there is none */
#define luaL_opt(L,f,n,d)	(lua_isnoneornil(L,(n)) ? (d) : f(L,(n)))

/* the buffer's own bookkeeping, which the macros read straight out of
   the struct -- see luaL_addsize and friends */
#define luaL_bufflen(bf)     ((bf)->n)
#define luaL_buffaddr(bf)    ((bf)->b)
#define luaL_addsize(B,s)    ((B)->n += (s))
#define luaL_buffsub(B,s)    ((B)->n -= (s))
#define luaL_prepbuffer(B)   luaL_prepbuffsize(B, LUAL_BUFFERSIZE)

/*
** The standard FILE* handle, as lualib.h declares it.  An extension
** that reaches through a file handle uses this; the port's own io
** library keeps its file in the userdata the same way.
*/
typedef struct luaL_Stream {
  void *f;              /* the host's file */
  lua_CFunction closef; /* how to close it */
} luaL_Stream;

/*
** A file handle is a userdata with metatable 'LUA_FILEHANDLE' (lua.h of
** PUC-Lua says so, and liolib.c registers it under that name).  An
** extension testing whether a value is one -- or making one of its own
** that io can use -- needs the name.
*/
#define LUA_FILEHANDLE "FILE*"

/*
** How the interpreter writes to stdout and stderr, which lua.c defines
** and an extension printing the same way may use (lauxlib.h).
*/
#define lua_writestring(s, l) fwrite((s), sizeof(char), (l), stdout)
#define lua_writeline() (lua_writestring("\n", 1), fflush(stdout))
#define lua_writestringerror(s, p) (fprintf(stderr, (s), (p)), fflush(stderr))

/* internal assertions: off, as PUC-Lua has them without LUAI_ASSERT */
#define lua_assert(c) ((void)0)

/* compatibility with 5.1's luaL_register */
#define luaL_register(L,n,l) (luaL_setfuncs(L, (l), 0))

/*
** {============================================================
** Compatibility with deprecated conversions
**
** PUC-Lua's luaconf.h only turns these on inside its LUA_COMPAT_5_3
** block, which is off by default, so they are conditional here too:
** an extension that wants them defines LUA_COMPAT_5_3 (or the macro
** itself) exactly as it would against PUC-Lua.
** =============================================================
*/
#if defined(LUA_COMPAT_APIINTCASTS)

#define luaL_opt(L,f,n,d)	(lua_isnoneornil(L,(n)) ? (d) : f(L,(n)))

#define luaL_checkunsigned(L,a)	((lua_Unsigned)luaL_checkinteger(L,a))
#define luaL_optunsigned(L,a,d)	\
	((lua_Unsigned)luaL_optinteger(L,a,(lua_Integer)(d)))

#define luaL_checkint(L,n)	((int)luaL_checkinteger(L, (n)))
#define luaL_optint(L,n,d)	((int)luaL_optinteger(L, (n), (d)))

#define luaL_checklong(L,n)	((long)luaL_checkinteger(L, (n)))
#define luaL_optlong(L,n,d)	((long)luaL_optinteger(L, (n), (d)))

#endif
/* }============================================================ */

#endif
