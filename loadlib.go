// Translation of loadlib.c (PUC-Lua 5.4): the package library and the
// 'require' mechanism.
//
// Deviations from the C code (all caused by the fact that a static Go
// binary cannot dlopen a shared object, and all marked with "deviation"
// in the comments below):
//   - lsys_load / lsys_sym always fail: searcher_C and package.loadlib
//     report "dynamic libraries not enabled" instead of loading code.
//   - the CLIBS bookkeeping table is created but always stays empty.
//   - there is no GC, so the __gc metamethod of CLIBS never runs.
package lua

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// Configuration constants (luaconf.h / lauxlib.h).
const (
	pkDirSep    = "/"        // LUA_DIRSEP
	pkPathSep   = ";"        // LUA_PATH_SEP
	pkPathMark  = "?"        // LUA_PATH_MARK
	pkExecDir   = "!"        // LUA_EXEC_DIR
	pkIgMark    = "-"        // LUA_IGMARK
	pkVerSuffix = "_5_4"     // LUA_VERSUFFIX
	pkLSUBSEP   = pkDirSep   // LUA_LSUBSEP
	pkCSUBSEP   = pkDirSep   // LUA_CSUBSEP
	pkPOF       = "luaopen_" // LUA_POF
	pkOFSEP     = "_"        // LUA_OFSEP

	pkCLibs        = "_CLIBS"   // registry key for the C-library handles
	pkLoadedTable  = "_LOADED"  // LUA_LOADED_TABLE
	pkPreloadTable = "_PRELOAD" // LUA_PRELOAD_TABLE

	pkPathVar  = "LUA_PATH"  // LUA_PATH_VAR
	pkCPathVar = "LUA_CPATH" // LUA_CPATH_VAR

	// pkPathDefault is LUA_PATH_DEFAULT from luaconf.h (Unix flavour).
	pkPathDefault = "/usr/local/share/lua/5.4/?.lua;" +
		"/usr/local/share/lua/5.4/?/init.lua;" +
		"/usr/local/lib/lua/5.4/?.lua;" +
		"/usr/local/lib/lua/5.4/?/init.lua;" +
		"./?.lua;./?/init.lua"

	// pkCPathDefault is LUA_CPATH_DEFAULT from luaconf.h (Unix flavour).
	pkCPathDefault = "/usr/local/lib/lua/5.4/?.so;" +
		"/usr/local/lib/lua/5.4/loadall.so;" +
		"./?.so"

	// pkLibFail is LIB_FAIL for a build without dynamic loading.
	pkLibFail = "absent"
	// pkDLMsg is the message used in place of dlerror().
	pkDLMsg = "dynamic libraries not enabled"
)

// error codes for 'lookforfunc'
const (
	pkErrLib  = 1 // ERRLIB
	pkErrFunc = 2 // ERRFUNC
)

// ---------------------------------------------------------------------
// Set Paths (setpath)
// ---------------------------------------------------------------------

// pkNoEnv returns registry.LUA_NOENV as a boolean (noenv).
func pkNoEnv(L *State) bool {
	L.GetField(LUA_REGISTRYINDEX, "LUA_NOENV")
	b := L.ToBoolean(-1)
	L.Pop(1) /* remove value */
	return b
}

// pkSetPath sets package[fieldname] from the environment (setpath).
//
// deviation: C pushes the versioned variable name on the stack (and pops
// it at the end); there is nothing to push here, so the field is set
// relative to the top of the stack instead.
func pkSetPath(L *State, fieldname, envname, dft string) {
	path, ok := os.LookupEnv(envname + pkVerSuffix) /* try versioned name */
	if !ok {
		path, ok = os.LookupEnv(envname) /* try unversioned name */
	}
	switch {
	case !ok || pkNoEnv(L): /* no environment variable? */
		L.PushString(dft) /* use default */
	default:
		if !strings.Contains(path, pkPathSep+pkPathSep) {
			L.PushString(path) /* nothing to change */
			break
		}
		/* path contains a ";;": insert default path in its place */
		var sb strings.Builder
		dftmark := strings.Index(path, pkPathSep+pkPathSep)
		if dftmark > 0 { /* is there a prefix before ';;'? */
			sb.WriteString(path[:dftmark]) /* add it */
			sb.WriteString(pkPathSep)
		}
		sb.WriteString(dft)        /* add default */
		if dftmark < len(path)-2 { /* is there a suffix after ';;'? */
			sb.WriteString(pkPathSep)
			sb.WriteString(path[dftmark+2:])
		}
		L.PushString(sb.String())
	}
	/* deviation: 'setprogdir' is a no-op outside Windows */
	L.SetField(-2, fieldname) /* package[fieldname] = path value */
}

// ---------------------------------------------------------------------
// dynamic loading (always fails in a static Go binary)
// ---------------------------------------------------------------------

// pkLoadSym is lsys_load: there is no dlopen in Go, so it always fails,
// pushing an error message (lsys_sym would do the same).
func pkLoadSym(L *State, path, sym string) error {
	L.PushString(pkDLMsg) /* deviation: no dynamic libraries */
	return errors.New(pkDLMsg)
}

// pkLookForFunc is lookforfunc. It returns 0 (and a value on the stack)
// on success, or an error code plus an error message.
func pkLookForFunc(L *State, path, sym string) int {
	if pkLoadSym(L, path, sym) != nil { /* must (and cannot) load library */
		return pkErrLib /* unable to load library */
	}
	if sym == "*" { /* loading only library (no function)? */
		L.PushBoolean(true) /* return 'true' */
		return 0            /* no errors */
	}
	/* unreachable without dlopen: lsys_sym would push the function */
	L.PushNil()
	return 0
}

// pkLoadFunc is loadfunc: it builds the "luaopen_X" name of a C module.
// It always fails here, as there is nothing to load.
func pkLoadFunc(L *State, filename, modname string) int {
	modname = strings.ReplaceAll(modname, ".", pkOFSEP)
	mark := strings.Index(modname, pkIgMark)
	if mark >= 0 { /* is there an ignore mark? */
		openfunc := fmt.Sprintf("%s%s", pkPOF, modname[:mark])
		if stat := pkLookForFunc(L, filename, openfunc); stat != pkErrFunc {
			return stat
		}
		modname = modname[mark+1:] /* else go ahead and try old-style name */
	}
	openfunc := fmt.Sprintf("%s%s", pkPOF, modname)
	return pkLookForFunc(L, filename, openfunc)
}

// pkLoadLib is ll_loadlib: package.loadlib.
func pkLoadLib(L *State) int {
	L.CheckString(1) /* path */
	L.CheckString(2) /* init */
	/* deviation: the library is never loaded; report why. Note that C
	   uses lua_insert here; the message is captured and re-pushed
	   instead, which gives the same three results. */
	L.PushNil()
	L.PushString(pkDLMsg)
	L.PushString(pkLibFail)
	return 3 /* return fail, error message, and where */
}

// ---------------------------------------------------------------------
// 'require' (searchers and loader)
// ---------------------------------------------------------------------

// pkReadable tells whether a file exists and can be read (readable).
func pkReadable(filename string) bool {
	f, err := os.Open(filename)
	if err != nil {
		return false /* open failed */
	}
	f.Close()
	return true
}

// pkPushErrorNotFound pushes
//
//	no file 'x'
//		no file 'y'
//
// for each name in path (pusherrornotfound).
func pkPushErrorNotFound(L *State, path string) {
	L.PushString("no file '" +
		strings.ReplaceAll(path, pkPathSep, "'\n\tno file '") + "'")
}

// pkSearchPath searches for 'name' in 'path' (searchpath). It pushes the
// file name found, or the error message when there is none.
//
// deviation: the C code writes the list of names into a luaL_Buffer and
// walks it with getnextfilename; Go strings are immutable, so the list
// is split with strings.Split, which yields the same names.
func pkSearchPath(L *State, name, path, sep, dirsep string) bool {
	/* separator is non-empty and appears in 'name'? */
	if sep != "" && strings.Contains(name, sep) {
		name = strings.ReplaceAll(name, sep, dirsep) /* replace it by 'dirsep' */
	}
	/* add path to the buffer, replacing marks ('?') with the file name */
	pathname := strings.ReplaceAll(path, pkPathMark, name)
	for _, filename := range strings.Split(pathname, pkPathSep) {
		if pkReadable(filename) { /* does file exist and is readable? */
			L.PushString(filename) /* save and return name */
			return true
		}
	}
	L.PushString(pathname)
	pkPushErrorNotFound(L, pathname) /* create error message */
	return false                     /* not found */
}

// pkSearchPathFunc is ll_searchpath: package.searchpath.
func pkSearchPathFunc(L *State) int {
	if pkSearchPath(L, L.CheckString(1), L.CheckString(2),
		L.OptString(3, "."), L.OptString(4, pkDirSep)) {
		return 1
	}
	/* error message is on top of the stack */
	msg := L.ToStringX(-1)
	L.Pop(1)
	L.PushNil()
	L.PushString(msg)
	return 2 /* return fail + error message */
}

// pkFindFile searches for 'name' in package[pname] (findfile). It
// returns the file name; pkSearchPath leaves it (or the error message)
// on the top of the stack.
func pkFindFile(L *State, name, pname, dirsep string) (string, bool) {
	L.GetField(UpvalueIndex(1), pname)
	path, ok := L.ToString(-1)
	L.Pop(1) /* remove the path from the stack */
	if !ok {
		L.Error2("'package.%s' must be a string", pname)
	}
	found := pkSearchPath(L, name, path, ".", dirsep)
	return L.ToStringX(-1), found
}

// pkCheckLoad checks whether a module could be loaded (checkload).
//
// deviation: the C version reads the error message from the stack (put
// there by luaL_loadfile); here the Go error carries it.
func pkCheckLoad(L *State, err error, filename string) int {
	if err == nil { /* module loaded successfully? */
		L.PushString(filename) /* will be 2nd argument to module */
		return 2               /* return open function and file name */
	}
	return L.Error2("error loading module '%s' from file '%s':\n\t%s",
		L.ToStringX(1), filename, err.Error())
}

// pkSearcherLua is searcher_Lua: it loads a module from a Lua file.
func pkSearcherLua(L *State) int {
	name := L.CheckString(1)
	filename, ok := pkFindFile(L, name, "path", pkLSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	data, err := os.ReadFile(filename)
	if err == nil {
		err = L.LoadBytes(data, "@"+filename)
	}
	return pkCheckLoad(L, err, filename)
}

// pkSearcherC is searcher_C.
//
// deviation: Go cannot dlopen. C raises an error when the file is found
// but cannot be loaded; here the message is returned instead, so that
// 'require' keeps trying the remaining searchers and reports the usual
// "module not found" error.
func pkSearcherC(L *State) int {
	name := L.CheckString(1)
	filename, ok := pkFindFile(L, name, "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	if pkLoadFunc(L, filename, name) != 0 {
		L.Pop(1) /* remove the message pushed by lsys_load/lsys_sym */
		L.PushString(pkDLMsg)
		return 1
	}
	return pkCheckLoad(L, nil, filename)
}

// pkSearcherCRoot is searcher_Croot: a C submodule may live inside the
// library named by the part of the module name before the first dot.
//
// deviation: Go cannot dlopen, so nothing can ever be loaded from the
// root.  The search for it still happens, though, because that is what
// contributes its "no file" lines to the message 'require' builds:
// 'require("a.b.c")' lists "./a.so" as well as "./a/b/c.so".
func pkSearcherCRoot(L *State) int {
	name := L.CheckString(1)
	dot := strings.IndexByte(name, '.')
	if dot < 0 {
		return 0 /* is root: no error */
	}
	root := name[:dot]
	filename, ok := pkFindFile(L, root, "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* root not found */
	}
	if pkLoadFunc(L, filename, name) != 0 {
		L.Pop(1) /* remove the message pushed by lsys_load/lsys_sym */
		L.PushString(pkDLMsg)
		return 1
	}
	L.PushString(filename) /* will be 2nd argument to module */
	return 2
}

// pkSearcherPreload is searcher_preload: it looks the module up in
// package.preload.
func pkSearcherPreload(L *State) int {
	name := L.CheckString(1)
	L.GetField(LUA_REGISTRYINDEX, pkPreloadTable)
	if L.GetField(-1, name) == LUA_TNIL { /* not found? */
		L.PushString(fmt.Sprintf("no field package.preload['%s']", name))
		return 1
	}
	L.PushString(":preload:")
	return 2
}

// pkBuffSub removes the last n characters of a message buffer
// (luaL_buffsub).
func pkBuffSub(sb *strings.Builder, n int) {
	s := sb.String()
	sb.Reset()
	if len(s) >= n {
		sb.WriteString(s[:len(s)-n])
	}
}

// pkFindLoader iterates over package.searchers looking for a loader for
// 'name' (findloader); it leaves the loader and its data on the stack.
func pkFindLoader(L *State, name string) {
	var msg strings.Builder /* to build error message */
	/* push 'package.searchers' to index 3 in the stack */
	if L.GetField(UpvalueIndex(1), "searchers") != LUA_TTABLE {
		L.Error2("'package.searchers' must be a table")
	}
	/*  iterate over available searchers to find a loader */
	for i := 1; ; i++ {
		msg.WriteString("\n\t")                 /* error-message prefix */
		if L.RawGetI(3, int64(i)) == LUA_TNIL { /* no more searchers? */
			L.Pop(1)           /* remove nil */
			pkBuffSub(&msg, 2) /* remove prefix */
			L.PushString(msg.String())
			L.Error2("module '%s' not found:%s", name, L.ToStringX(-1))
		}
		L.PushString(name)
		L.Call(1, 2)          /* call it */
		if L.IsFunction(-2) { /* did it find a loader? */
			return /* module loader found */
		} else if L.IsString(-2) { /* searcher returned error message? */
			L.Pop(1) /* remove extra return */
			msg.WriteString(L.ToStringX(-1))
			L.Pop(1) /* remove the message */
		} else { /* no error message */
			L.Pop(2)           /* remove both returns */
			pkBuffSub(&msg, 2) /* remove prefix */
		}
	}
}

// pkRequire is ll_require: the global 'require'.
func pkRequire(L *State) int {
	name := L.CheckString(1)
	L.SetTop(1) /* LOADED table will be at index 2 */
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.GetField(2, name)  /* LOADED[name] */
	if L.ToBoolean(-1) { /* is it there? */
		return 1 /* package is already loaded */
	}
	/* else must load package */
	L.Pop(1) /* remove 'getfield' result */
	pkFindLoader(L, name)
	/* stack: name; LOADED; searchers; loader data; loader function */
	L.Rotate(L.AbsIndex(-2), 1) /* function <-> loader data */
	L.PushValue(1)              /* name is 1st argument to module loader */
	L.PushValue(-3)             /* loader data is 2nd argument */
	/* stack: ...; loader data; loader function; mod. name; loader data */
	L.Call(2, 1) /* run loader to load module */
	/* stack: ...; loader data; result from loader */
	if !L.IsNil(-1) { /* non-nil return? */
		L.SetField(2, name) /* LOADED[name] = returned value */
	} else {
		L.Pop(1) /* pop nil */
	}
	if L.GetField(2, name) == LUA_TNIL { /* module set no value? */
		L.PushBoolean(true) /* use true as result */
		L.Copy(-1, -2)      /* replace loader result */
		L.SetField(2, name) /* LOADED[name] = true */
	}
	L.Rotate(L.AbsIndex(-2), 1) /* loader data <-> module result */
	return 2                    /* return module result and loader data */
}

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

// pkFuncs are the functions of the 'package' table (pk_funcs). The
// fields 'preload', 'cpath', 'path', 'searchers' and 'loaded' are
// placeholders in C and are set by OpenPackage instead.
var pkFuncs = []Reg{
	{Name: "loadlib", Func: pkLoadLib},
	{Name: "searchpath", Func: pkSearchPathFunc},
}

// pkLLFuncs are the functions opened into the global table (ll_funcs).
var pkLLFuncs = []Reg{
	{Name: "require", Func: pkRequire},
}

// pkGctm is the __gc metamethod of the CLIBS table; with no dynamic
// libraries the table is always empty.
func pkGctm(L *State) int {
	n := L.Len2(1)
	for ; n >= 1; n-- { /* for each handle, in reverse order */
		L.RawGetI(1, n) /* get handle CLIBS[n] */
		L.Pop(1)        /* pop handle */
	}
	return 0
}

// pkCreateCLibsTable creates the CLIBS table (createclibstable).
func pkCreateCLibsTable(L *State) {
	L.GetSubTable(LUA_REGISTRYINDEX, pkCLibs) /* create CLIBS table */
	L.CreateTable(0, 1)                       /* create metatable for CLIBS */
	L.PushGoFunc(pkGctm)
	L.SetField(-2, "__gc") /* set finalizer for CLIBS table */
	L.SetMetaTable(-2)
	L.Pop(1) /* pop CLIBS table */
}

// pkCreateSearchersTable fills package.searchers (createsearcherstable).
func pkCreateSearchersTable(L *State) {
	searchers := []GoFunc{
		pkSearcherPreload,
		pkSearcherLua,
		pkSearcherC,
		pkSearcherCRoot,
	}
	/* create 'searchers' table */
	L.CreateTable(len(searchers), 0)
	/* fill it with predefined searchers */
	for i := 0; i < len(searchers); i++ {
		L.PushValue(-2) /* set 'package' as upvalue for all searchers */
		L.PushGoClosure(searchers[i], 1)
		L.RawSetI(-2, int64(i+1))
	}
	L.SetField(-2, "searchers") /* put it in field 'searchers' */
}

// OpenPackage opens the package library (luaopen_package).
//
// deviation: C calls luaL_requiref here to open the base, coroutine,
// string, ... libraries. In this port every library is registered by
// linit.go (which does not exist yet), so OpenPackage only builds the
// 'package' table, the searchers and the global 'require'.
func OpenPackage(L *State) int {
	pkCreateCLibsTable(L)
	L.NewLib(pkFuncs) /* create 'package' table */
	pkCreateSearchersTable(L)
	/* set paths */
	pkSetPath(L, "path", pkPathVar, pkPathDefault)
	pkSetPath(L, "cpath", pkCPathVar, pkCPathDefault)
	/* store config information */
	L.PushString(pkDirSep + "\n" + pkPathSep + "\n" + pkPathMark + "\n" +
		pkExecDir + "\n" + pkIgMark + "\n")
	L.SetField(-2, "config")
	/* set field 'loaded' */
	L.GetSubTable(LUA_REGISTRYINDEX, pkLoadedTable)
	L.SetField(-2, "loaded")
	/* set field 'preload' */
	L.GetSubTable(LUA_REGISTRYINDEX, pkPreloadTable)
	L.SetField(-2, "preload")
	/* add 'require' to the global table, with 'package' as upvalue */
	L.PushGlobalTable()
	L.PushValue(-2) /* set 'package' as upvalue for next lib */
	L.SetFuncs(pkLLFuncs, 1)
	L.Pop(1) /* pop global table */
	/* make the library reachable when it is opened on its own: linit.go
	   (luaL_requiref) finds it already loaded and just re-exports it */
	L.PushValue(-1)
	L.SetGlobal("package")
	L.GetSubTable(LUA_REGISTRYINDEX, pkLoadedTable)
	L.PushValue(-2) /* the 'package' table */
	L.SetField(-2, "package")
	L.Pop(1) /* pop LOADED table */
	return 1 /* return 'package' table */
}
