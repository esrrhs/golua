/*
** lualib.h -- the standard libraries, as an extension sees them.
**
** This is a header for *extensions*, not for the port itself: the port
** is pure Go and never includes it.  It exists because an extension
** written for PUC-Lua includes <lualib.h> as a matter of course --
** LuaFileSystem does, and it would not compile without it -- and what
** the extension needs from it is the set of names the host binary
** exports: luaL_openlibs and the luaopen_* of the libraries that are
** written in C and may be required by hand.
**
** Types and constants follow PUC-Lua 5.3 so that source written for it
** compiles unchanged.  See the notice at the end of this file.
*/
#ifndef lualib_h
#define lualib_h

#include "lua.h"


/* version suffix for environment variable names */
#define LUA_VERSUFFIX          "_" LUA_VERSION_MAJOR "_" LUA_VERSION_MINOR

LUAMOD_API int (luaopen_base) (lua_State *L);

#define LUA_COLIBNAME   "coroutine"
LUAMOD_API int (luaopen_coroutine) (lua_State *L);

#define LUA_TABLIBNAME  "table"
LUAMOD_API int (luaopen_table) (lua_State *L);

#define LUA_IOLIBNAME   "io"
LUAMOD_API int (luaopen_io) (lua_State *L);

#define LUA_OSLIBNAME   "os"
LUAMOD_API int (luaopen_os) (lua_State *L);

#define LUA_STRLIBNAME  "string"
LUAMOD_API int (luaopen_string) (lua_State *L);

#define LUA_UTF8LIBNAME "utf8"
LUAMOD_API int (luaopen_utf8) (lua_State *L);

#define LUA_BITLIBNAME  "bit32"
LUAMOD_API int (luaopen_bit32) (lua_State *L);

#define LUA_MATHLIBNAME "math"
LUAMOD_API int (luaopen_math) (lua_State *L);

#define LUA_DBLIBNAME   "debug"
LUAMOD_API int (luaopen_debug) (lua_State *L);

#define LUA_LOADLIBNAME "package"
LUAMOD_API int (luaopen_package) (lua_State *L);


/* open all previous libraries */
LUALIB_API void (luaL_openlibs) (lua_State *L);


#if !defined(lua_assert)
#define lua_assert(x)	((void)0)
#endif


/*
** Copyright (C) 1994-2017 Lua.org, PUC-Rio.
**
** Permission is hereby granted, free of charge, to any person obtaining
** a copy of this software and associated documentation files (the
** "Software"), to deal in the Software without restriction, including
** without limitation the rights to use, copy, modify, merge, publish,
** distribute, sublicense, and/or sell copies of the Software, and to
** permit persons to whom the Software is furnished to do so, subject to
** the following conditions:
**
** The above copyright notice and this permission notice shall be
** included in all copies or substantial portions of the Software.
**
** THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
** EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
** MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.
** IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY
** CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT,
** TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE
** SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/

#endif
