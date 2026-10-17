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
void luaL_addlstring(luaL_Buffer *B, const char *s, size_t l);
void luaL_addstring(luaL_Buffer *B, const char *s);
void luaL_addvalue(luaL_Buffer *B);
void luaL_pushresult(luaL_Buffer *B);
void luaL_pushresultsize(luaL_Buffer *B, size_t sz);

/* compatibility with 5.1's luaL_register */
#define luaL_register(L,n,l) (luaL_setfuncs(L, (l), 0))

#endif
