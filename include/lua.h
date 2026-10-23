/*
** lua.h -- the C API of this port, so that an extension written against
** Lua 5.3 can be compiled against it.
**
** This is a header for *extensions*, not for the port itself: the port
** is pure Go and never includes it.  Every function declared here is
** exported by the host binary (see cdlopen.go), so an extension built
** the usual way -- without linking liblua -- resolves these names at
** load time, which is what dlopen does when the host exports them.
**
** Types and constants follow PUC-Lua 5.3 so that source written for it
** compiles unchanged.
*/
#ifndef lua_h
#define lua_h

#include <stdarg.h>
#include <stddef.h>

#define LUA_VERSION_MAJOR   "5"
#define LUA_VERSION_MINOR   "3"
#define LUA_VERSION_RELEASE "4"
#define LUA_VERSION         "Lua 5.3"
#define LUA_VERSION_NUM     503
#define LUA_RELEASE         "Lua 5.3.4"

/* The rest of the version block, which PUC-Lua spells in lua.h. */
#define LUA_VERSION_RELEASE_NUM		(LUA_VERSION_NUM * 100 + 4)
#define LUA_COPYRIGHT	LUA_RELEASE "  Copyright (C) 1994-2017 Lua.org, PUC-Rio"
#define LUA_AUTHORS	"R. Ierusalimschy, L. H. de Figueiredo, W. Celes"

/* The four bytes at the start of a precompiled chunk (lundump.h). An
   extension that looks at a binary chunk asks for these. */
#define LUA_SIGNATURE	"\x1bLua"

/*
** The decoration PUC-Lua puts on every declaration in its headers.
** luaconf.h defines these, and an extension that declares its own
** 'luaopen_' with them -- or includes a header of ours that does --
** has to be able to use them, so they are here: the port's lua.h stands
** for lua.h and luaconf.h together.  They are empty because the names
** are resolved when the module is loaded, not when it is linked, so
** there is nothing for a declaration to export.
*/
#define LUA_API
#define LUALIB_API  LUA_API
#define LUAMOD_API  LUALIB_API

/* option for multiple returns in 'lua_pcall' and 'lua_call' */
#define LUA_MULTRET (-1)

/*
** Pseudo-indices.  LUA_REGISTRYINDEX is the value the port's
** LUA_REGISTRYINDEX constant has; the two below are the pre-defined
** registry entries of lauxlib/linit.
*/
#define LUA_REGISTRYINDEX   (-1000000 - 1000)
#define lua_upvalueindex(i) (LUA_REGISTRYINDEX - (i))
#define LUA_RIDX_MAINTHREAD 1
#define LUA_RIDX_GLOBALS    2
#define LUA_RIDX_LAST       LUA_RIDX_GLOBALS

/* basic types */
#define LUA_TNONE          (-1)
#define LUA_TNIL           0
#define LUA_TBOOLEAN       1
#define LUA_TLIGHTUSERDATA 2
#define LUA_TNUMBER        3
#define LUA_TSTRING        4
#define LUA_TTABLE         5
#define LUA_TFUNCTION      6
#define LUA_TUSERDATA      7
#define LUA_TTHREAD        8
#define LUA_NUMTYPES       9
#define LUA_NUMTAGS        LUA_NUMTYPES

/* minimum stack available to a C function */
#define LUA_MINSTACK 20

/*
** The bytes PUC-Lua keeps in front of every lua_State for the host to
** use as it likes (luaconf.h).  'lua_getextraspace' reaches behind the
** pointer, and the port really does keep them there -- they belong to
** the state, are zeroed when it is first handed to C, and live as long
** as it does.
*/
#define LUA_EXTRASPACE		(sizeof(void *))
#define lua_getextraspace(L)	((void *)((char *)(L) - LUA_EXTRASPACE))

/* predefined values in the registry */
#define LUA_REFNIL (-1)
#define LUA_NOREF  (-2)

/* status codes */
#define LUA_OK        0
#define LUA_YIELD     1
#define LUA_ERRRUN    2
#define LUA_ERRSYNTAX 3
#define LUA_ERRMEM    4
#define LUA_ERRERR    5

/* comparison operators for lua_compare / lua_arith */
#define LUA_OPADD  0
#define LUA_OPSUB  1
#define LUA_OPMUL  2
#define LUA_OPMOD  3
#define LUA_OPPOW  4
#define LUA_OPDIV  5
#define LUA_OPIDIV 6
#define LUA_OPBAND 7
#define LUA_OPBOR  8
#define LUA_OPBXOR 9
#define LUA_OPSHL  10
#define LUA_OPSHR  11
#define LUA_OPUNM  12
#define LUA_OPBNOT 13

#define LUA_OPEQ 0
#define LUA_OPLT 1
#define LUA_OPLE 2

/* size of the 'source' and 'short_src' buffers of lua_Debug */
#define LUA_IDSIZE 60

/* debug hook event codes and masks */
#define LUA_HOOKCALL     0
#define LUA_HOOKRET      1
#define LUA_HOOKLINE     2
#define LUA_HOOKCOUNT    3
#define LUA_HOOKTAILCALL 4

#define LUA_MASKCALL  (1 << LUA_HOOKCALL)
#define LUA_MASKRET   (1 << LUA_HOOKRET)
#define LUA_MASKLINE  (1 << LUA_HOOKLINE)
#define LUA_MASKCOUNT (1 << LUA_HOOKCOUNT)

/* garbage-collection options (the collector is Go's; see README §6) */
#define LUA_GCSTOP       0
#define LUA_GCRESTART    1
#define LUA_GCCOLLECT    2
#define LUA_GCCOUNT      3
#define LUA_GCCOUNTB     4
#define LUA_GCSTEP       5
#define LUA_GCSETPAUSE   6
#define LUA_GCSETSTEPMUL 7
#define LUA_GCISRUNNING  9
#define LUA_GCGEN        10
#define LUA_GCINC        11

typedef struct lua_State lua_State;

/* number types */
typedef double      lua_Number;
typedef long long   lua_Integer;
typedef unsigned long long lua_Unsigned;
typedef ptrdiff_t   lua_KContext;

typedef int (*lua_CFunction)(lua_State *L);
typedef int (*lua_KFunction)(lua_State *L, int status, lua_KContext ctx);
typedef const char *(*lua_Reader)(lua_State *L, void *data, size_t *size);
typedef int (*lua_Writer)(lua_State *L, const void *p, size_t sz, void *ud);
typedef void *(*lua_Alloc)(void *ud, void *ptr, size_t osize, size_t nsize);
typedef void (*lua_WarnFunction)(void *ud, const char *msg, int tocont);

struct lua_Debug; /* defined below: lua_Hook only ever points at one */
typedef void (*lua_Hook)(lua_State *L, struct lua_Debug *ar);

/*
** The debug record.  The fields up to short_src are the ones an
** extension reads; i_ci is PUC-Lua's private pointer to its own call
** record, which an extension must not touch -- it is declared void *
** here because what it names lives on the Go side.
*/
struct lua_Debug {
  int event;
  const char *name;      /* (n) */
  const char *namewhat;  /* (n) */
  const char *what;      /* (S) */
  const char *source;    /* (S) */
  size_t srclen;         /* (S) */
  int currentline;       /* (l) */
  int linedefined;       /* (S) */
  int lastlinedefined;   /* (S) */
  unsigned char nups;    /* (u) */
  unsigned char nparams; /* (u) */
  char isvararg;         /* (u) */
  char istailcall;       /* (t) */
  unsigned short ftransfer; /* (r) */
  unsigned short ntransfer; /* (r) */
  char short_src[LUA_IDSIZE]; /* (S) */
  void *i_ci;            /* private */
};

/* lua_Hook above names struct lua_Debug, which is this */
typedef struct lua_Debug lua_Debug;

/* ---- state ---- */
lua_State *lua_newstate(lua_Alloc f, void *ud);
void       lua_close(lua_State *L);
lua_State *lua_newthread(lua_State *L);
int        lua_resetthread(lua_State *L);
int        lua_closethread(lua_State *L, lua_State *from);
lua_CFunction lua_atpanic(lua_State *L, lua_CFunction panicf);
lua_Number lua_version(lua_State *L);
lua_Alloc  lua_getallocf(lua_State *L, void **ud);
void       lua_setallocf(lua_State *L, lua_Alloc f, void *ud);
int        lua_setcstacklimit(lua_State *L, unsigned int limit);
void       lua_toclose(lua_State *L, int idx);

/* ---- stack ---- */
int  lua_absindex(lua_State *L, int idx);
int  lua_gettop(lua_State *L);
void lua_settop(lua_State *L, int idx);
void lua_pop(lua_State *L, int n);          /* macro in PUC-Lua */
int  lua_checkstack(lua_State *L, int n);
void lua_rotate(lua_State *L, int idx, int n);
void lua_copy(lua_State *L, int fromidx, int toidx);
void lua_insert(lua_State *L, int idx);     /* macro in PUC-Lua */
void lua_remove(lua_State *L, int idx);     /* macro in PUC-Lua */
void lua_closeslot(lua_State *L, int idx);

/* ---- access ---- */
int  lua_type(lua_State *L, int idx);
const char *lua_typename(lua_State *L, int tp);
int  lua_isnil(lua_State *L, int idx);
int  lua_isboolean(lua_State *L, int idx);
int  lua_isnumber(lua_State *L, int idx);
int  lua_isinteger(lua_State *L, int idx);
int  lua_isstring(lua_State *L, int idx);
int  lua_istable(lua_State *L, int idx);
int  lua_isfunction(lua_State *L, int idx);
int  lua_iscfunction(lua_State *L, int idx);
int  lua_isuserdata(lua_State *L, int idx);
int  lua_islightuserdata(lua_State *L, int idx);
int  lua_isthread(lua_State *L, int idx);
int  lua_isnone(lua_State *L, int idx);
int  lua_isnoneornil(lua_State *L, int idx);

lua_Number  lua_tonumberx(lua_State *L, int idx, int *isnum);
lua_Integer lua_tointegerx(lua_State *L, int idx, int *isnum);
int         lua_toboolean(lua_State *L, int idx);
const char *lua_tolstring(lua_State *L, int idx, size_t *len);
const char *lua_tostring(lua_State *L, int idx);   /* macro in PUC-Lua */
lua_Unsigned lua_rawlen(lua_State *L, int idx);
lua_CFunction lua_tocfunction(lua_State *L, int idx);
void        *lua_touserdata(lua_State *L, int idx);
lua_State   *lua_tothread(lua_State *L, int idx);
void        *lua_topointer(lua_State *L, int idx);
const char  *lua_getupvalue(lua_State *L, int funcindex, int n);
const char  *lua_setupvalue(lua_State *L, int funcindex, int n);
lua_Number  lua_tonumber(lua_State *L, int idx);   /* macro in PUC-Lua */
lua_Integer lua_tointeger(lua_State *L, int idx);  /* macro in PUC-Lua */

/* ---- arithmetic and comparison ---- */
void lua_arith(lua_State *L, int op);
int  lua_rawequal(lua_State *L, int idx1, int idx2);
int  lua_compare(lua_State *L, int idx1, int idx2, int op);
size_t lua_stringtonumber(lua_State *L, const char *s);

/* ---- push ---- */
void lua_pushnil(lua_State *L);
void lua_pushnumber(lua_State *L, lua_Number n);
void lua_pushinteger(lua_State *L, lua_Integer n);
const char *lua_pushlstring(lua_State *L, const char *s, size_t len);
const char *lua_pushstring(lua_State *L, const char *s);
const char *lua_pushvfstring(lua_State *L, const char *fmt, va_list argp);
const char *lua_pushfstring(lua_State *L, const char *fmt, ...);
void lua_pushcclosure(lua_State *L, lua_CFunction fn, int n);
void lua_pushboolean(lua_State *L, int b);
void lua_pushlightuserdata(lua_State *L, void *p);
int  lua_pushthread(lua_State *L);
void lua_pushglobaltable(lua_State *L);
void lua_pushvalue(lua_State *L, int idx);

/* ---- get ---- */
int lua_getglobal(lua_State *L, const char *name);
int lua_gettable(lua_State *L, int idx);
int lua_getfield(lua_State *L, int idx, const char *k);
int lua_geti(lua_State *L, int idx, lua_Integer n);
int lua_rawget(lua_State *L, int idx);
int lua_rawgeti(lua_State *L, int idx, lua_Integer n);
int lua_rawgetp(lua_State *L, int idx, const void *p);
int lua_getmetatable(lua_State *L, int idx);
void lua_createtable(lua_State *L, int narr, int nrec);
void lua_newtable(lua_State *L);            /* macro in PUC-Lua */
void *lua_newuserdatauv(lua_State *L, size_t sz, int nuvalue);
void *lua_newuserdata(lua_State *L, size_t sz);  /* macro in PUC-Lua */
int  lua_getiuservalue(lua_State *L, int idx, int n);

/* ---- set ---- */
void lua_setglobal(lua_State *L, const char *name);
void lua_settable(lua_State *L, int idx);
void lua_setfield(lua_State *L, int idx, const char *k);
void lua_seti(lua_State *L, int idx, lua_Integer n);
void lua_rawset(lua_State *L, int idx);
void lua_rawseti(lua_State *L, int idx, lua_Integer n);
void lua_rawsetp(lua_State *L, int idx, const void *p);
int  lua_setmetatable(lua_State *L, int idx);
int  lua_setiuservalue(lua_State *L, int idx, int n);

/* ---- load and call ---- */
void lua_callk(lua_State *L, int nargs, int nresults, lua_KContext ctx, lua_KFunction k);
void lua_call(lua_State *L, int nargs, int nresults);
int  lua_pcallk(lua_State *L, int nargs, int nresults, int msgh,
                lua_KContext ctx, lua_KFunction k);
int  lua_pcall(lua_State *L, int nargs, int nresults, int msgh);
int  lua_load(lua_State *L, lua_Reader reader, void *data,
              const char *chunkname, const char *mode);
int  lua_dump(lua_State *L, lua_Writer writer, void *data, int strip);
int  lua_error(lua_State *L);
int  lua_next(lua_State *L, int idx);
void lua_concat(lua_State *L, int n);
void lua_len(lua_State *L, int idx);
int  lua_resume(lua_State *L, lua_State *from, int nargs, int *nresults);
int  lua_status(lua_State *L);
int  lua_isyieldable(lua_State *L);
int  lua_yieldk(lua_State *L, int nresults, lua_KContext ctx, lua_KFunction k);
int  lua_yield(lua_State *L, int nresults);

/* ---- warnings ---- */
void lua_warning(lua_State *L, const char *msg, int tocont);
void lua_setwarnf(lua_State *L, lua_WarnFunction f, void *ud);

/* ---- garbage collection, hooks, misc ---- */
int lua_gc(lua_State *L, int what, ...);
void lua_sethook(lua_State *L, lua_Hook f, int mask, int count);
lua_Hook lua_gethook(lua_State *L);
int lua_gethookmask(lua_State *L);
int lua_gethookcount(lua_State *L);
int lua_getstack(lua_State *L, int level, lua_Debug *ar);
int lua_getinfo(lua_State *L, const char *what, lua_Debug *ar);
const char *lua_getlocal(lua_State *L, const lua_Debug *ar, int n);
const char *lua_setlocal(lua_State *L, const lua_Debug *ar, int n);
void *lua_upvalueid(lua_State *L, int fidx, int n);
void  lua_upvaluejoin(lua_State *L, int fidx1, int n1, int fidx2, int n2);
void  lua_xmove(lua_State *from, lua_State *to, int n);

#define lua_getuservalue(L,idx)   lua_getiuservalue(L, idx, 1)
#define lua_setuservalue(L,idx)   lua_setiuservalue(L, idx, 1)
#define lua_pushliteral(L,s)      lua_pushstring(L, "" s)
#define lua_replace(L,idx)        (lua_copy(L, -1, (idx)), lua_pop(L, 1))
#define lua_pushunsigned(L,n)     lua_pushinteger(L, (lua_Integer)(n))
#define lua_tounsignedx(L,i,is)   ((lua_Unsigned)lua_tointegerx(L, i, is))
#define lua_tounsigned(L,i)       lua_tounsignedx(L, (i), NULL)

#define lua_pop(L,n)          lua_settop(L, -(n)-1)
#define lua_newtable(L)       lua_createtable(L, 0, 0)
#define lua_tostring(L,i)     lua_tolstring(L, (i), NULL)
#define lua_tonumber(L,i)     lua_tonumberx(L,i,NULL)
#define lua_tointeger(L,i)    lua_tointegerx(L,(i),NULL)
#define lua_newuserdata(L,s)  lua_newuserdatauv(L, s, 1)
#define lua_pushcfunction(L,f) lua_pushcclosure(L, (f), 0)
#define lua_register(L,n,f)   (lua_pushcfunction(L, (f)), lua_setglobal(L, (n)))
#define lua_insert(L,idx)     lua_rotate(L, (idx), 1)
#define lua_remove(L,idx)     (lua_rotate(L, (idx), -1), lua_pop(L, 1))

#endif
