// Command luac is the standalone Lua compiler (translation of luac.c,
// PUC-Rio Lua 5.4.9).
//
// C luac.c main (lines 196–210):
//
//	doargs(argc, argv);
//	argc -= i; argv += i;
//	if (argc <= 0) usage("no input files given");
//	L = luaL_newstate();
//	if (L == NULL) fatal("cannot create state: not enough memory");
//	lua_pushcfunction(L, &pmain);
//	lua_pushinteger(L, argc);
//	lua_pushlightuserdata(L, argv);
//	if (lua_pcall(L, 2, 0, 0) != LUA_OK) fatal(lua_tostring(L, -1));
//	lua_close(L);
//	return EXIT_SUCCESS;
//
// Go cannot host that body in this package: -l listing and combine/dump
// walk Proto fields (luac.c includes lobject.h / lopcodes.h / lstate.h).
// Those stay in lua.Luac → core.Luac (internal/core/luac.go). This file
// is only the process entry, matching cmd/lua's thin main → *Main split.
//
// Usage:
//
//	luac [options] [filenames]
//
// Options (luac.c usage()):
//
//	-l       list (use -l -l for full listing)
//	-o name  output to file 'name' (default is "luac.out")
//	-p       parse only
//	-s       strip debug information
//	-v       show version information
//	--       stop handling options
//	-        stop handling options and process stdin
package main

import (
	"os"

	lua "github.com/esrrhs/golua"
)

func main() {
	os.Exit(luacMain(os.Args))
}

// luacMain is main of luac.c.
//
// argv is the process argv (argv[0] included), as in C. Stdout/stderr are
// fixed to os.Stdout / os.Stderr here; injectable writers exist only on
// lua.Luac for tests.
//
// C always returns EXIT_SUCCESS from main (fatal/usage call exit). Go
// returns the status from lua.Luac (0 / 1) so os.Exit matches process
// convention without library-side os.Exit.
func luacMain(argv []string) int {
	return lua.Luac(argv, os.Stdout, os.Stderr)
}
