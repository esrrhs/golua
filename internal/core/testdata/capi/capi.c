/* capi.c -- exercise the parts of the C API that the other extension
** tests in this directory do not reach.  It is written the way an
** ordinary extension is: compiled against <lua.h>/<lauxlib.h>/<lualib.h>,
** opened by luaopen_capi, found by 'require'.  Everything it returns is
** a number, a boolean or a fixed string, so that PUC-Lua 5.4.9 and this
** port can be compared line for line.
**
** One thing to remember while reading it: luaL_buffinit pushes a "box"
** onto the stack, so every function that uses a buffer keeps its index
** in 'base' and works above it.
*/
#include <stdio.h>
#include <string.h>
#include "lua.h"
#include "lauxlib.h"
#include "lualib.h"

/* ------------------------------------------------------------------ */
/* A. the stack: lua_rotate, lua_absindex, lua_copy, lua_checkstack   */
/* ------------------------------------------------------------------ */

static int l_stack(lua_State *L) {
  int a, b, c, top, bot, after, room;
  const char *s4;
  lua_settop(L, 0);
  lua_pushinteger(L, 1);
  lua_pushinteger(L, 2);
  lua_pushinteger(L, 3);
  lua_rotate(L, 1, 1);                    /* [1,2,3] -> [3,1,2] */
  a = (int)lua_tointeger(L, 1);
  b = (int)lua_tointeger(L, 2);
  c = (int)lua_tointeger(L, 3);
  top = lua_absindex(L, -1);              /* 3 */
  bot = lua_absindex(L, -3);              /* 1 */
  lua_pushliteral(L, "x");
  after = lua_absindex(L, -1);            /* 4 */
  lua_copy(L, 2, 4);                      /* slot 2 (== 1) over "x" */
  room = lua_checkstack(L, 5);
  s4 = lua_tostring(L, 4);
  /* slot 4 stays where it is: 's4' points into it, and the string
     would be collected if it left the stack before the pushfstring */
  lua_pushfstring(L, "%d %d %d | %d %d %d | %s | %d",
                  a, b, c, top, bot, after, s4, room);
  return 1;
}

/* lua_insert / lua_remove / lua_replace are macros over lua_rotate and
** lua_copy, but an extension written against lua.h uses them as such. */
static int l_stackm(lua_State *L) {
  lua_settop(L, 0);
  lua_pushliteral(L, "a");
  lua_pushliteral(L, "b");
  lua_pushliteral(L, "c");
  lua_remove(L, 2);                       /* a c */
  lua_insert(L, -1);                      /* c a */
  lua_pushliteral(L, "z");                /* c a z */
  lua_replace(L, 2);                      /* c z */
  lua_pushfstring(L, "%s %s", lua_tostring(L, 1), lua_tostring(L, 2));
  return 1;
}

/* ------------------------------------------------------------------ */
/* B. lua_arith, lua_compare, lua_concat, lua_len, lua_rawlen          */
/* ------------------------------------------------------------------ */

static int l_arith(lua_State *L) {
  static const int ops[] = {
    LUA_OPADD, LUA_OPSUB, LUA_OPMUL, LUA_OPMOD, LUA_OPPOW,
    LUA_OPDIV, LUA_OPIDIV, LUA_OPBAND, LUA_OPBOR, LUA_OPBXOR,
    LUA_OPSHL, LUA_OPSHR, LUA_OPUNM, LUA_OPBNOT
  };
  luaL_Buffer b;
  int i, base;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  base = lua_gettop(L);
  for (i = 0; i < (int)(sizeof(ops)/sizeof(ops[0])); i++) {
    lua_settop(L, base);
    lua_pushinteger(L, 7);
    lua_pushinteger(L, 2);
    lua_arith(L, ops[i]);
    luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
    luaL_addchar(&b, ' ');
  }
  lua_settop(L, base);
  luaL_pushresult(&b);
  return 1;
}

static int l_cmp(lua_State *L) {
  int r[6];
  lua_settop(L, 0);
  lua_pushinteger(L, 3); lua_pushinteger(L, 3);
  r[0] = lua_compare(L, 1, 2, LUA_OPEQ);
  r[1] = lua_compare(L, 1, 2, LUA_OPLT);
  r[2] = lua_compare(L, 1, 2, LUA_OPLE);
  lua_settop(L, 0);
  lua_pushnumber(L, 2.5); lua_pushinteger(L, 3);
  r[3] = lua_compare(L, 1, 2, LUA_OPLT);
  lua_settop(L, 0);
  lua_pushliteral(L, "abc"); lua_pushliteral(L, "abd");
  r[4] = lua_compare(L, 1, 2, LUA_OPLT);
  r[5] = lua_rawequal(L, 1, 2);
  lua_settop(L, 0);
  lua_pushfstring(L, "%d %d %d %d %d %d", r[0], r[1], r[2], r[3], r[4], r[5]);
  return 1;
}

static int l_catlen(lua_State *L) {
  lua_Integer slen, rlen, tlen;
  lua_settop(L, 0);
  lua_pushliteral(L, "a");
  lua_pushinteger(L, 1);
  lua_pushliteral(L, "b");
  lua_concat(L, 3);                       /* 1: "a1b" */
  lua_pushliteral(L, "abc");              /* 2 */
  lua_len(L, -1);                         /* 3 */
  slen = lua_tointeger(L, -1);
  lua_pop(L, 1);
  rlen = (lua_Integer)lua_rawlen(L, -1);  /* unlike lua_len, it answers */
  lua_pop(L, 1);                          /* "abc" away */
  lua_createtable(L, 0, 0);               /* 2 */
  lua_pushinteger(L, 1); lua_rawseti(L, 2, 1);
  lua_pushinteger(L, 2); lua_rawseti(L, 2, 2);
  tlen = (lua_Integer)lua_rawlen(L, 2);
  lua_settop(L, 1);                       /* only "a1b" is left */
  lua_pushfstring(L, "%s %I %I %I", lua_tostring(L, 1), slen, rlen, tlen);
  return 1;
}

/* ------------------------------------------------------------------ */
/* C. tables: every get/set pair, lua_next and the globals             */
/* ------------------------------------------------------------------ */

static int l_tab(lua_State *L) {
  const char *byfield, *byraw, *byrawi;
  int byi, n = 0;
  lua_settop(L, 0);
  lua_createtable(L, 0, 4);
  lua_pushliteral(L, "v");
  lua_setfield(L, 1, "k");            /* t.k = "v" */
  lua_getfield(L, 1, "k");
  byfield = lua_tostring(L, -1);
  lua_pop(L, 1);

  lua_pushinteger(L, 9);
  lua_seti(L, 1, 1);                  /* t[1] = 9 */
  lua_geti(L, 1, 1);
  byi = (int)lua_tointeger(L, -1);
  lua_pop(L, 1);

  lua_pushliteral(L, "rk");
  lua_pushliteral(L, "rv");
  lua_rawset(L, 1);
  lua_pushliteral(L, "rk");
  lua_rawget(L, 1);
  byraw = lua_tostring(L, -1);
  lua_pop(L, 1);

  lua_pushliteral(L, "ri");
  lua_rawseti(L, 1, 2);
  lua_rawgeti(L, 1, 2);
  byrawi = lua_tostring(L, -1);
  lua_pop(L, 1);

  lua_pushnil(L);
  while (lua_next(L, 1) != 0) {
    n++;
    lua_pop(L, 1); /* the value; the key stays for the next step */
  }
  /* the table stays: it is what keeps the four strings alive */
  lua_pushfstring(L, "%s %d %s %s %d", byfield, byi, byraw, byrawi, n);
  return 1;
}

static int l_glob(lua_State *L) {
  lua_Integer after;
  lua_settop(L, 0);
  lua_getglobal(L, "capi_global");        /* 1: "g" */
  lua_pushinteger(L, 42);
  lua_setglobal(L, "capi_global");
  lua_getglobal(L, "capi_global");        /* 2: 42 */
  after = lua_tointeger(L, -1);
  lua_pushfstring(L, "%s %I", lua_tostring(L, 1), after);
  return 1;
}

/* ------------------------------------------------------------------ */
/* D. the to* family, with the 'isnum' out parameter                   */
/* ------------------------------------------------------------------ */

static int l_conv(lua_State *L) {
  int ok1 = 0, ok2 = 0, ok3 = 0, ok4 = 0, b;
  lua_Integer i, i2;
  lua_Number n, n2;
  lua_settop(L, 0);
  lua_pushliteral(L, "42");
  i = lua_tointegerx(L, -1, &ok1);
  n = lua_tonumberx(L, -1, &ok2);
  lua_settop(L, 0);
  lua_pushliteral(L, "xyz");
  i2 = lua_tointegerx(L, -1, &ok3);
  n2 = lua_tonumberx(L, -1, &ok4);
  lua_settop(L, 0);
  lua_pushboolean(L, 0);
  b = lua_toboolean(L, -1);
  lua_settop(L, 0);
  lua_pushinteger(L, 1);         /* 1: kept on the stack until the end */
  lua_tostring(L, 1);            /* the number, converted in place */
  lua_pushfstring(L, "%I %d %f %d %d %d %I %f | %d %s %s",
                  i, ok1, (double)n, ok2, ok3, ok4, i2, (double)n2,
                  b, lua_tostring(L, 1), lua_tolstring(L, 1, NULL));
  return 1;
}

/* lua_type / lua_typename / every lua_is*, on one value of each kind */
static int l_types(lua_State *L) {
  luaL_Buffer b;
  int i, base;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  base = lua_gettop(L);
  lua_pushnil(L);                            /* +1: nil */
  lua_pushboolean(L, 1);                     /* +2: boolean */
  lua_pushlightuserdata(L, (void *)&i);      /* +3: light userdata */
  lua_pushinteger(L, 1);                     /* +4: number (integer) */
  lua_pushliteral(L, "s");                   /* +5: string */
  lua_createtable(L, 0, 0);                  /* +6: table */
  lua_pushcfunction(L, l_types);             /* +7: function */
  lua_newuserdata(L, 4);                /* +8: userdata */
  lua_newthread(L);                          /* +9: thread */
  for (i = 1; i <= 9; i++) {
    luaL_addstring(&b, lua_typename(L, lua_type(L, base + i)));
    luaL_addchar(&b, ' ');
  }
  luaL_addstring(&b, "| ");
  luaL_addstring(&b, lua_isnil(L, base + 1) ? "nil " : "- ");
  luaL_addstring(&b, lua_isboolean(L, base + 2) ? "bool " : "- ");
  luaL_addstring(&b, lua_islightuserdata(L, base + 3) ? "lud " : "- ");
  luaL_addstring(&b, lua_isnumber(L, base + 4) ? "num " : "- ");
  luaL_addstring(&b, lua_isinteger(L, base + 4) ? "int " : "- ");
  luaL_addstring(&b, lua_isstring(L, base + 5) ? "str " : "- ");
  luaL_addstring(&b, lua_isstring(L, base + 4) ? "strnum " : "- ");
  luaL_addstring(&b, lua_istable(L, base + 6) ? "table " : "- ");
  luaL_addstring(&b, lua_isfunction(L, base + 7) ? "func " : "- ");
  luaL_addstring(&b, lua_iscfunction(L, base + 7) ? "cfunc " : "- ");
  luaL_addstring(&b, lua_isuserdata(L, base + 8) ? "ud " : "- ");
  luaL_addstring(&b, lua_isthread(L, base + 9) ? "thread " : "- ");
  luaL_addstring(&b, lua_isnone(L, base + 10) ? "none " : "- ");
  luaL_addstring(&b, lua_isnoneornil(L, base + 10) ? "noneornil" : "-");
  lua_settop(L, base);
  luaL_pushresult(&b);
  return 1;
}

/* ------------------------------------------------------------------ */
/* E. luaL_ref / luaL_unref                                            */
/* ------------------------------------------------------------------ */

/* Each value is copied into the buffer as soon as it is read: an
** unref'd one is no longer reachable, and a pointer into it would
** dangle by the time the answer is built. */
static int l_refs(lua_State *L) {
  luaL_Buffer b;
  int r1, r2, t;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  t = lua_gettop(L) + 1;                  /* 2: the table */
  lua_createtable(L, 0, 0);
  lua_pushliteral(L, "one");
  r1 = luaL_ref(L, t);
  lua_pushliteral(L, "two");
  r2 = luaL_ref(L, t);
  lua_rawgeti(L, t, r1);
  luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
  lua_pop(L, 2);                          /* the value and its tostring */
  luaL_addchar(&b, ' ');
  lua_rawgeti(L, t, r2);
  luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
  lua_pop(L, 2);
  luaL_addchar(&b, ' ');
  luaL_unref(L, t, r1);                   /* the slot is free again */
  lua_rawgeti(L, t, r1);
  luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
  lua_pop(L, 2);
  luaL_addstring(&b, " | ");
  luaL_addstring(&b, r1 == LUA_REFNIL ? "1 " : "0 ");
  luaL_addstring(&b, r2 == LUA_REFNIL ? "1" : "0");
  lua_settop(L, t - 1);
  luaL_pushresult(&b);
  return 1;
}

/* ------------------------------------------------------------------ */
/* F. the rest of lauxlib                                              */
/* ------------------------------------------------------------------ */

static int l_auxlib(lua_State *L) {
  luaL_Buffer b;
  int fresh, got, same, made, found, st;
  lua_Integer len;
  const char *tb;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);            /* 1: the box */

  /* newmetatable / getmetatable / setmetatable */
  luaL_newmetatable(L, "CapiMT");
  lua_pop(L, 1);
  fresh = luaL_newmetatable(L, "CapiMT");   /* already there: 0 */
  lua_pop(L, 1);
  luaL_addstring(&b, fresh ? "1 " : "0 ");

  lua_createtable(L, 0, 0);            /* 2 */
  luaL_getmetatable(L, "CapiMT");      /* 3 */
  got = (lua_type(L, 3) == LUA_TTABLE);
  lua_pushvalue(L, 3);                 /* 4: a second reference to it */
  lua_setmetatable(L, 2);              /* the table gets CapiMT */
  lua_getmetatable(L, 2);              /* 4: what the table now has */
  same = lua_rawequal(L, 4, 3);
  lua_settop(L, 1);
  luaL_addstring(&b, got ? "T " : "F ");
  luaL_addstring(&b, same ? "T " : "F ");

  /* luaL_len of a string */
  lua_pushliteral(L, "abcd");          /* 2 */
  len = luaL_len(L, 2);
  lua_settop(L, 1);
  luaL_addstring(&b, "len=");
  lua_pushinteger(L, len);             /* 2 */
  luaL_addstring(&b, luaL_tolstring(L, 2, NULL));
  lua_settop(L, 1);
  luaL_addchar(&b, ' ');

  /* luaL_gsub */
  luaL_addstring(&b, "gsub=");
  luaL_addstring(&b, luaL_gsub(L, "hello world", "o", "0"));
  lua_settop(L, 1);
  luaL_addchar(&b, ' ');

  /* luaL_getsubtable: creates it, then finds it */
  lua_createtable(L, 0, 0);                    /* 2 */
  made = luaL_getsubtable(L, 2, "sub");        /* 3 */
  lua_pop(L, 1);
  found = luaL_getsubtable(L, 2, "sub");       /* 3 */
  lua_pop(L, 1);
  lua_settop(L, 1);
  luaL_addstring(&b, made ? "made " : "- ");
  luaL_addstring(&b, found ? "- " : "found ");

  /* luaL_loadbufferx with a mode */
  st = luaL_loadbufferx(L, "return 1+1", 10, "=capichunk", "t");
  luaL_addstring(&b, st == LUA_OK ? "loaded " : "failed ");
  if (st == LUA_OK) {
    lua_call(L, 0, 1);
    luaL_addstring(&b, luaL_tolstring(L, 2, NULL));
  }
  lua_settop(L, 1);
  luaL_addchar(&b, ' ');

  /* luaL_fileresult of a successful operation */
  luaL_fileresult(L, 1, NULL);
  luaL_addstring(&b, luaL_tolstring(L, 2, NULL));
  lua_settop(L, 1);
  luaL_addchar(&b, ' ');

  /* luaL_checkversion: same version, so it must not complain */
  luaL_checkversion(L);
  luaL_addstring(&b, "verOK ");

  /* luaL_traceback: only whether there is one */
  luaL_traceback(L, L, NULL, 0);
  tb = lua_tostring(L, 2);
  luaL_addstring(&b, (tb != NULL && tb[0] != '\0') ? "tb" : "-");
  lua_settop(L, 1);

  luaL_pushresult(&b);
  return 1;
}

/* luaL_check* / luaL_opt* argument checking, from Lua's side */
static int l_checks(lua_State *L) {
  lua_Integer i, oi;
  lua_Number n;
  const char *s;
  lua_settop(L, 4);
  i = luaL_checkinteger(L, 1);
  n = luaL_checknumber(L, 2);
  s = luaL_optstring(L, 3, "dflt");
  oi = luaL_optinteger(L, 4, -1);
  luaL_checkany(L, 1);
  luaL_checktype(L, 1, LUA_TNUMBER);
  luaL_checkstack(L, 3, "no room");
  lua_settop(L, 0);
  lua_pushfstring(L, "%I %f %s %I", i, (double)n, s, oi);
  return 1;
}

static int l_argerr(lua_State *L) {
  luaL_argerror(L, 1, "not what I wanted");
  return 0;
}
static int l_typeerr(lua_State *L) {
  lua_settop(L, 1);
  luaL_typeerror(L, 1, "capi thing");
  return 0;
}
static int l_where(lua_State *L) {
  luaL_where(L, 1);
  return 1;
}

/* ------------------------------------------------------------------ */
/* G. userdata with several user values                                */
/* ------------------------------------------------------------------ */


/* ------------------------------------------------------------------ */
/* H. to-be-closed from C: lua_toclose and lua_closeslot               */
/* ------------------------------------------------------------------ */



/* ------------------------------------------------------------------ */
/* I. warnings                                                         */
/* ------------------------------------------------------------------ */


/* ------------------------------------------------------------------ */
/* J. hooks installed from C, and the debug API                        */
/* ------------------------------------------------------------------ */

static int hookcalls = 0;

static void myhook(lua_State *L, lua_Debug *ar) {
  (void)L;
  hookcalls += ar->event;
}

static int l_hook(lua_State *L) {
  int st;
  hookcalls = 0;
  lua_settop(L, 1);
  luaL_checktype(L, 1, LUA_TFUNCTION);
  lua_pushvalue(L, 1);
  lua_sethook(L, myhook, LUA_MASKCALL | LUA_MASKRET, 0);
  st = lua_pcall(L, 0, 0, 0);
  lua_sethook(L, NULL, 0, 0);
  lua_settop(L, 0);
  lua_pushinteger(L, (lua_Integer)hookcalls);
  lua_pushinteger(L, st);
  return 2;
}

/* lua_getinfo with every field, read through a C lua_Debug */
static int l_info(lua_State *L) {
  lua_Debug ar;
  lua_settop(L, 0);
  if (!lua_getstack(L, 1, &ar)) { lua_pushliteral(L, "nostack"); return 1; }
  lua_getinfo(L, "nSlut", &ar);
  lua_settop(L, 0);
  lua_pushfstring(L, "%s|%s|%s|%d|%d|%d|%d",
                  ar.name ? ar.name : "-",
                  ar.namewhat ? ar.namewhat : "-",
                  ar.what ? ar.what : "-",
                  ar.currentline, ar.linedefined,
                  (int)ar.nups, (int)ar.nparams);
  return 1;
}

/* lua_getlocal / lua_setlocal on the function that called us */
static int l_locals(lua_State *L) {
  lua_Debug ar;
  luaL_Buffer b;
  const char *nm;
  int base;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  base = lua_gettop(L);
  if (lua_getstack(L, 1, &ar)) {
    nm = lua_getlocal(L, &ar, 1);
    luaL_addstring(&b, nm ? nm : "-");
    luaL_addchar(&b, '=');
    luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
    lua_pop(L, 1);
    luaL_addchar(&b, ' ');
    lua_pushinteger(L, 99);
    nm = lua_setlocal(L, &ar, 1);
    luaL_addstring(&b, nm ? nm : "-");
    luaL_addchar(&b, ' ');
    nm = lua_getlocal(L, &ar, 1);
    luaL_addstring(&b, nm ? nm : "-");
    luaL_addchar(&b, '=');
    luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
    lua_pop(L, 1);
  } else {
    luaL_addstring(&b, "nostack");
  }
  lua_settop(L, base);
  luaL_pushresult(&b);
  return 1;
}

/* upvalues: read one, write it, read it again */
static int l_upvalues(lua_State *L) {
  luaL_Buffer b;
  const char *n1, *n2, *n3;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  luaL_loadstring(L, "local x = 5 return function() x = x + 1 return x end");
  lua_call(L, 0, 1);
  n1 = lua_getupvalue(L, -1, 1);
  luaL_addstring(&b, n1 ? n1 : "-");
  luaL_addchar(&b, '=');
  luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
  lua_pop(L, 2);                  /* the tostring and the upvalue itself */
  luaL_addchar(&b, ' ');
  lua_pushinteger(L, 77);
  n2 = lua_setupvalue(L, -2, 1);  /* -2 is the closure again */
  luaL_addstring(&b, n2 ? n2 : "-");
  luaL_addchar(&b, ' ');
  n3 = lua_getupvalue(L, -1, 1);
  luaL_addstring(&b, n3 ? n3 : "-");
  luaL_addchar(&b, '=');
  luaL_addstring(&b, luaL_tolstring(L, -1, NULL));
  lua_pop(L, 2);
  luaL_pushresult(&b);
  return 1;
}

/* lua_upvalueid and lua_upvaluejoin: two closures sharing one variable */
static int l_join(lua_State *L) {
  void *id1, *id2, *id3;
  lua_settop(L, 0);
  luaL_loadstring(L, "local x = 10 return function() x = x + 1 return x end");
  lua_call(L, 0, 1);                     /* closure A at 1 */
  luaL_loadstring(L, "local y = 0 return function() y = y + 100 return y end");
  lua_call(L, 0, 1);                     /* closure B at 2 */
  id1 = lua_upvalueid(L, 1, 1);
  id2 = lua_upvalueid(L, 2, 1);
  lua_upvaluejoin(L, 1, 1, 2, 1);
  id3 = lua_upvalueid(L, 1, 1);
  lua_settop(L, 0);
  lua_pushfstring(L, "%d %d %d", id1 != NULL, id1 != id2, id3 == id2);
  return 1;
}

/* ------------------------------------------------------------------ */
/* K. coroutines driven from C                                         */
/* ------------------------------------------------------------------ */

static int l_coro(lua_State *L) {
  lua_State *co;
  int st;
  luaL_Buffer b;
  lua_settop(L, 0);
  co = lua_newthread(L);                 /* 1: the thread */
  lua_getglobal(co, "capi_cobody");
  luaL_buffinit(L, &b);
  st = lua_resume(co, L, 0);
  luaL_addstring(&b, luaL_tolstring(co, -1, NULL));
  lua_pop(co, 2);
  luaL_addchar(&b, ' ');
  luaL_addstring(&b, st == LUA_YIELD ? "yield" : (st == LUA_OK ? "ok" : "err"));
  luaL_addchar(&b, ' ');
  st = lua_resume(co, L, 0);
  luaL_addstring(&b, luaL_tolstring(co, -1, NULL));
  lua_pop(co, 2);
  luaL_addchar(&b, ' ');
  luaL_addstring(&b, st == LUA_OK ? "ok" : "?");
  luaL_addchar(&b, ' ');
  luaL_addstring(&b, lua_status(co) == LUA_OK ? "finished" : "?");
  luaL_addchar(&b, ' ');
  luaL_addstring(&b, lua_isyieldable(co) ? "y" : "n");
  luaL_pushresult(&b);
  return 1;
}

/* lua_xmove between two threads of one state */
static int l_xmove(lua_State *L) {
  lua_State *co;
  luaL_Buffer b;
  lua_settop(L, 0);
  co = lua_newthread(L);
  lua_pushinteger(L, 7);
  lua_xmove(L, co, 1);
  luaL_buffinit(L, &b);
  luaL_addstring(&b, luaL_tolstring(co, lua_gettop(co), NULL));
  lua_pop(co, 1);
  luaL_pushresult(&b);
  return 1;
}

/* lua_resetthread: a thread that has run can be given a new body */

/* ------------------------------------------------------------------ */
/* L. the buffer API, in full                                          */
/* ------------------------------------------------------------------ */

static int l_buf(lua_State *L) {
  luaL_Buffer b;
  size_t len1, len2;
  const char *addr;
  char *p;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  luaL_addstring(&b, "ab");
  luaL_addchar(&b, 'c');
  luaL_addlstring(&b, "def", 3);
  lua_pushliteral(L, "-val");
  luaL_addvalue(&b);
  p = luaL_prepbuffsize(&b, 4);
  memcpy(p, "WXYZ", 4);
  luaL_addsize(&b, 4);
  len1 = luaL_bufflen(&b);
  luaL_buffsub(&b, 2);
  len2 = luaL_bufflen(&b);
  addr = (const char *)luaL_buffaddr(&b);
  luaL_pushresult(&b);
  lua_pushinteger(L, (lua_Integer)len1);
  lua_pushinteger(L, (lua_Integer)len2);
  lua_pushboolean(L, addr != NULL);
  return 4;
}

static int l_bufsize(lua_State *L) {
  luaL_Buffer b;
  char *p;
  lua_settop(L, 0);
  luaL_buffinitsize(L, &b, 16);
  p = luaL_prepbuffer(&b);
  memcpy(p, "0123456789", 10);
  luaL_pushresultsize(&b, 10);   /* this one adds the size itself */
  return 1;
}

/* ------------------------------------------------------------------ */
/* M. a file handle reached through luaL_Stream                        */
/* ------------------------------------------------------------------ */

static int l_stream(lua_State *L) {
  luaL_Stream *s;
  lua_settop(L, 1);
  s = (luaL_Stream *)luaL_checkudata(L, 1, LUA_FILEHANDLE);
  lua_settop(L, 0);
  lua_pushfstring(L, "%d %d", s != NULL, s != NULL && s->f != NULL);
  return 1;
}

/* ------------------------------------------------------------------ */
/* N. limits and the panic handler                                     */
/* ------------------------------------------------------------------ */

static int mypanic(lua_State *L) {
  (void)L;
  return 0;
}

static int l_limits(lua_State *L) {
  unsigned int old, now;
  lua_CFunction prev, back;
  old = (unsigned int)lua_setcstacklimit(L, 1000);
  now = (unsigned int)lua_setcstacklimit(L, old);
  /* Whether a host installed a panic handler is its business; that
     lua_atpanic hands the previous one back is Lua's. */
  prev = lua_atpanic(L, mypanic);
  back = lua_atpanic(L, prev);
  lua_settop(L, 0);
  lua_pushfstring(L, "%d %d %d", (int)old, (int)now, back == mypanic);
  return 1;
}

/* luaL_addgsub: the same substitution, into a buffer of our own */
static int l_addgsub(lua_State *L) {
  luaL_Buffer b;
  lua_settop(L, 0);
  luaL_buffinit(L, &b);
  luaL_addstring(&b, "[");
  luaL_addgsub(&b, "aXbXc", "X", ".");
  luaL_addstring(&b, "]");
  luaL_pushresult(&b);
  return 1;
}

/* ------------------------------------------------------------------ */
/* O. luaL_requiref: pull in a library the way a host does             */
/* ------------------------------------------------------------------ */

static int l_pullstring(lua_State *L) {
  lua_settop(L, 0);
  luaL_requiref(L, "string", luaopen_string, 1);
  lua_getfield(L, -1, "upper");
  lua_pushliteral(L, "abc");
  lua_call(L, 1, 1);
  return 1; /* the module stays below the result: only the result is returned */
}

static const luaL_Reg funcs[] = {
  /* A */ {"stack", l_stack}, {"stackm", l_stackm},
  /* B */ {"arith", l_arith}, {"cmp", l_cmp}, {"catlen", l_catlen},
  /* C */ {"tab", l_tab}, {"glob", l_glob},
  /* D */ {"conv", l_conv}, {"types", l_types},
  /* E */ {"refs", l_refs},
  /* F */ {"auxlib", l_auxlib}, {"checks", l_checks},
          {"argerr", l_argerr}, {"typeerr", l_typeerr}, {"where", l_where},
  /* J */ {"hook", l_hook}, {"info", l_info}, {"locals", l_locals},
          {"upvalues", l_upvalues}, {"join", l_join},
  /* K */ {"coro", l_coro}, {"xmove", l_xmove},
  /* L */ {"buf", l_buf}, {"bufsize", l_bufsize},
  /* M */ {"stream", l_stream},
  /* N */ {"limits", l_limits},
  /* L */ {"addgsub", l_addgsub},
  /* O */ {"pullstring", l_pullstring},
  {NULL, NULL}
};

int luaopen_capi(lua_State *L) {
  luaL_newlib(L, funcs);
  return 1;
}
