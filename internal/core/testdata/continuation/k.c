/*
** k.c -- a C extension built to exercise continuations: the three C API
** functions that take a lua_KFunction, which are the one part of Lua a
** C extension can reach that an ordinary call cannot.
**
** Written for this port; MIT, like the rest of the testdata.
*/

#include "lua.h"
#include "lauxlib.h"

/* cont leaves the context it was given plus 100, and the status it was
   called with: that is the only way to see from Lua whether the
   continuation ran, and what it was told. */
static int cont(lua_State *L, int status, lua_KContext ctx) {
  lua_pushinteger(L, (int)ctx + 100);
  lua_pushinteger(L, status);
  return 2;
}

/* a C function that yields, with a continuation */
static int yielder(lua_State *L) {
  lua_pushinteger(L, 42);
  return lua_yieldk(L, 1, 7, cont);
}

/* the same without one: the values given to resume come back instead */
static int plain(lua_State *L) {
  lua_pushinteger(L, 5);
  return lua_yieldk(L, 1, 0, NULL);
}

/* a yield of no values at all, with no continuation */
static int nyield(lua_State *L) {
  return lua_yieldk(L, 0, 0, NULL);
}

/* a C function that calls a Lua function which yields */
static int caller(lua_State *L) {
  lua_callk(L, 0, 0, 0, cont);
  return cont(L, 0, 0);
}

/* lua_callk with no continuation at all: the call is not yieldable */
static int nocall(lua_State *L) {
  lua_callk(L, 0, 0, 0, NULL);
  lua_pushstring(L, "no k");
  return 1;
}

/* lua_pcallk whose callee yields */
static int pcally(lua_State *L) {
  int st = lua_pcallk(L, 0, 1, 0, 3, cont);
  return cont(L, st, 3);
}

/* lua_pcallk whose callee raises */
static int pcallerr(lua_State *L) {
  int st = lua_pcallk(L, 0, 1, 0, 4, cont);
  return cont(L, st, 4);
}

/* contmsg also reports the error object: finishpcallk leaves it where
   the callee's first result would have been, which for pcallmsg is
   index 2. */
static int contmsg(lua_State *L, int status, lua_KContext ctx) {
  lua_pushinteger(L, (int)ctx + 100);
  lua_pushinteger(L, status);
  if (lua_type(L, 2) == LUA_TSTRING)
    lua_pushstring(L, lua_tostring(L, 2));
  else
    lua_pushstring(L, "-");
  return 3;
}

/* lua_pcallk with a message handler: called as pcallmsg(handler, f) */
static int pcallmsg(lua_State *L) {
  int st = lua_pcallk(L, 0, 1, 1, 6, contmsg);
  return contmsg(L, st, 6);
}

/* PUC-Lua's own dofilecont is 'return lua_gettop(L) - 1', so the stack a
   continuation sees is part of the contract.  This one answers how many
   values are on it: for ytop, below, the two that were yielded plus the
   one the resume was given, because C leaves the yielded values there. */
static int contall(lua_State *L, int status, lua_KContext ctx) {
  int n = lua_gettop(L);
  (void)status;
  (void)ctx;
  lua_settop(L, 0);
  lua_pushinteger(L, n);
  return 1;
}

static int ytop(lua_State *L) {
  lua_pushinteger(L, 8);
  lua_pushinteger(L, 9);
  return lua_yieldk(L, 2, 0, contall);
}

static int pall(lua_State *L) {
  int st = lua_pcallk(L, 0, LUA_MULTRET, 0, 0, contall);
  return contall(L, st, 0);
}

/* cont2 yields again, so one C function suspends several times */
static int cont2(lua_State *L, int status, lua_KContext ctx) {
  if ((int)ctx > 3) {
    lua_pushstring(L, "done");
    return 1;
  }
  lua_pushinteger(L, (int)ctx);
  return lua_yieldk(L, 1, (lua_KContext)((int)ctx + 1), cont2);
}

static int two(lua_State *L) {
  return cont2(L, 0, 1);
}

/* can this C function yield? */
static int yable(lua_State *L) {
  lua_pushboolean(L, lua_isyieldable(L));
  return 1;
}

static const luaL_Reg funcs[] = {
    {"yielder", yielder},   {"plain", plain},       {"nyield", nyield},
    {"caller", caller},     {"nocall", nocall},     {"pcally", pcally},
    {"pcallerr", pcallerr}, {"pcallmsg", pcallmsg}, {"pall", pall},
    {"ytop", ytop},         {"two", two},           {"yable", yable},
    {NULL, NULL}};

int luaopen_k(lua_State *L) {
  luaL_newlib(L, funcs);
  return 1;
}
