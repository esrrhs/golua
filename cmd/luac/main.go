// Command luac is the standalone Lua compiler, a Go port of luac.c.
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
//
// The work is done by lua.Luac, which lives in the library because the
// listing has to read prototypes.
package main

import (
	"os"

	lua "github.com/esrrhs/golua"
)

func main() {
	os.Exit(lua.Luac(os.Args, os.Stdout, os.Stderr))
}
