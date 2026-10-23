// Command luac is the standalone Lua compiler (translation of luac.c main).
//
// C main: doargs → luaL_newstate → pcall(pmain) → exit.
// Go: os.Exit(lua.Luac(...)) — Luac lives in the library because -l listing
// walks Proto internals (not exposed through the public API).
//
// Usage:
//
//	luac [options] [filenames]
//
// Options:
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
	os.Exit(lua.Luac(os.Args, os.Stdout, os.Stderr))
}
