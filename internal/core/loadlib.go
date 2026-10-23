// Translation of loadlib.c (PUC-Lua 5.3.4): the package library and the
// 'require' mechanism.
//
// Deviations from the C code (all caused by the fact that a static Go
// binary cannot dlopen a shared object, and all marked with "deviation"
// in the comments below):
//   - lsys_load / lsys_sym always fail: searcher_C and package.loadlib
//     report "dynamic libraries not enabled" instead of loading code.
//   - the CLIBS bookkeeping table is created but always stays empty.
//   - there is no GC, so the __gc metamethod of CLIBS never runs.
package core

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Configuration constants (luaconf.h / lauxlib.h).
const (
	pkPathSep   = ";"        // LUA_PATH_SEP
	pkPathMark  = "?"        // LUA_PATH_MARK
	pkExecDir   = "!"        // LUA_EXEC_DIR
	pkIgMark    = "-"        // LUA_IGMARK
	pkVerSuffix = "_5_3"     // LUA_VERSUFFIX
	pkPOF       = "luaopen_" // LUA_POF
	pkOFSEP     = "_"        // LUA_OFSEP

	pkCLibs        = "_CLIBS"   // registry key for the C-library handles
	pkLoadedTable  = "_LOADED"  // LUA_LOADED_TABLE
	pkPreloadTable = "_PRELOAD" // LUA_PRELOAD_TABLE

	pkPathVar  = "LUA_PATH"  // LUA_PATH_VAR
	pkCPathVar = "LUA_CPATH" // LUA_CPATH_VAR

	// pkPathDefault is LUA_PATH_DEFAULT from luaconf.h (Unix flavour).
	pkPathDefault = "/usr/local/share/lua/5.3/?.lua;" +
		"/usr/local/share/lua/5.3/?/init.lua;" +
		"/usr/local/lib/lua/5.3/?.lua;" +
		"/usr/local/lib/lua/5.3/?/init.lua;" +
		"./?.lua;./?/init.lua"

	// pkCPathDefault is LUA_CPATH_DEFAULT from luaconf.h (Unix flavour).
	pkCPathDefault = "/usr/local/lib/lua/5.3/?.so;" +
		"/usr/local/lib/lua/5.3/loadall.so;" +
		"./?.so"

	// pkLibFail is LIB_FAIL for a build without dynamic loading.
	pkLibFail = "absent"
	// pkDLMsg is the message used in place of dlerror().
	pkDLMsg = "dynamic libraries not enabled"
)

// pkDirSep is LUA_DIRSEP, and it is not a constant because luaconf.h
// does not have one either: it picks "\\" for Windows and "/" for
// everything else.  It is what 'package.config' answers on its first
// line, and what a module name's '.' turns into when 'require' looks
// for a submodule -- 'a.b' becomes 'a\b' there and 'a/b' here.
var pkDirSep = luaDirSep()

// pkLSUBSEP and pkCSUBSEP are LUA_LSUBSEP and LUA_CSUBSEP: both are
// LUA_DIRSEP, so both follow it.
var (
	pkLSUBSEP = pkDirSep
	pkCSUBSEP = pkDirSep
)

func luaDirSep() string {
	if runtime.GOOS == "windows" {
		return "\\"
	}
	return "/"
}

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
		/* 5.3.4 setpath: replace ";;" by ";AUXMARK;" then AUXMARK by
		   the default — so ";;" becomes ";"+default+";" (not bare
		   default). luaL_gsub replaces every occurrence. */
		auxmark := "" /* unlikely in a path */
		path = strings.ReplaceAll(path, pkPathSep+pkPathSep, pkPathSep+auxmark+pkPathSep)
		path = strings.ReplaceAll(path, auxmark, dft)
		L.PushString(path)
	}
	/* deviation: 'setprogdir' is a no-op outside Windows */
	L.SetField(-2, fieldname) /* package[fieldname] = path value */
}

// ---------------------------------------------------------------------
// dynamic loading (always fails in a static Go binary)
// ---------------------------------------------------------------------

// pkDynLoadSym is lsys_load and lsys_sym, as one step: open 'path', find
// 'sym' in it, and leave the function on the stack.
//
// It is nil unless the dynamic-loading backend is in the build: that
// backend needs cgo (cdlopen.go), because the host has to export the
// lua_* and luaL_* names for the extension to resolve.  Without it there
// is no dlopen in a static Go binary, and every load reports why.
var pkDynLoadSym func(L *State, path, sym string) int

// releaseCState gives back whatever the dynamic-loading backend handed
// out for a state. It is nil unless that backend is in the build.
var releaseCState func(L *State)

// pkLoadSym is pkLookForFunc's call into the loader (lsys_load plus
// lsys_sym).  It answers 0, pkErrLib (the library could not be opened)
// or pkErrFunc (the symbol is not in it), and leaves either the function
// or the error message on the stack.
func pkLoadSym(L *State, path, sym string) int {
	if pkDynLoadSym == nil {
		L.PushString(pkDLMsg) /* no backend: say which piece is missing */
		return pkErrLib
	}
	return pkDynLoadSym(L, path, sym)
}

// pkHasDynLoad tells whether the dynamic-loading backend is in this
// build. It is here so callers can tell "there is no dlopen in this
// binary" apart from "dlopen was asked and said no".
func pkHasDynLoad() bool { return pkDynLoadSym != nil }

// pkLookForFunc is lookforfunc. It returns 0 (and a value on the stack)
// on success, or an error code plus an error message.
func pkLookForFunc(L *State, path, sym string) int {
	if stat := pkLoadSym(L, path, sym); stat != 0 {
		return stat /* unable to load the library, or the symbol */
	}
	/* The loader has already left what lookforfunc returns: the function
	   itself, or 'true' when only the library was asked for ("*").  C's
	   lsys_load answers the pointer and lookforfunc pushes it; here the
	   push has to happen where the pointer is, because a Go function
	   value cannot be handed back as a void *. */
	return 0 /* no errors */
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
	path := L.CheckString(1) /* path */
	init := L.CheckString(2) /* init */
	/* With the dynamic-loading backend this is loadlib.c's ll_loadlib
	   exactly: ask for the function, and answer either it or nil plus
	   the message the loader left.  Without it, report why. */
	stat := pkLookForFunc(L, path, init)
	if stat == 0 {
		return 1 /* return the loaded function */
	}
	/* error; the message is on the stack top, under the two answers */
	L.PushNil()  /* first result (failed) */
	L.Insert(-2) /* put before the error message, as C does */
	switch stat {
	case pkErrLib:
		/* LIB_FAIL is "open" when the build can dlopen and "absent"
		   when it cannot */
		if pkHasDynLoad() {
			L.PushString("open") /* could not open the library */
		} else {
			L.PushString(pkLibFail)
		}
	case pkErrFunc:
		L.PushString("init") /* library opened, no such function */
	default:
		L.PushString(pkLibFail)
	}
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
func pkPushErrorNotFound(L *State, filenames []string) {
	/* C: lua_pushfstring(L, "\n\tno file '%s'", filename) per entry */
	var b strings.Builder
	for _, f := range filenames {
		b.WriteString("\n\tno file '")
		b.WriteString(f)
		b.WriteByte('\'')
	}
	L.PushString(b.String())
}

// pkSearchPath searches for 'name' in 'path' (searchpath). It pushes the
// file name found, or the error message when there is none.
func pkSearchPath(L *State, name, path, sep, dirsep string) bool {
	/* 'luaL_addgsub' inserts the name with 'luaL_addstring', which is
	   'strlen', so a name longer than the bytes before its first zero
	   is not inserted: 'package.searchpath("a\0b", "?.lua")' looks for
	   "a.lua" and says so. */
	name = luaCStr(name)
	if sep != "" && strings.Contains(name, sep) {
		name = strings.ReplaceAll(name, sep, dirsep)
	}
	/* pushnexttemplate: skip empty separators; empty path → empty msg. */
	var tried []string
	for _, tmpl := range strings.Split(path, pkPathSep) {
		if tmpl == "" {
			continue
		}
		filename := strings.ReplaceAll(tmpl, pkPathMark, name)
		if pkReadable(filename) {
			L.PushString(filename)
			return true
		}
		tried = append(tried, filename)
	}
	pkPushErrorNotFound(L, tried)
	return false
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

// pkCheckLoadStat is checkload: 'stat' tells whether the module was
// loaded, and when it was not, the loader's message is on the stack top
// (where C reads it with lua_tostring(L, -1)).
func pkCheckLoadStat(L *State, stat bool, filename string) int {
	if stat { /* module loaded successfully? */
		L.PushString(filename) /* will be 2nd argument to module */
		return 2               /* return open function and file name */
	}
	return L.Error2("error loading module '%s' from file '%s':\n\t%s",
		L.ToStringX(1), filename, L.ToStringX(-1))
}

// pkSearcherC is searcher_C: a file found in package.cpath is loaded as
// a C module, and a failure to load it is a real error (C stops there,
// it does not go on to the next searcher).
func pkSearcherC(L *State) int {
	name := L.CheckString(1)
	filename, ok := pkFindFile(L, name, "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	return pkCheckLoadStat(L, pkLoadFunc(L, filename, name) == 0, filename)
}

// pkSearcherCRoot is searcher_Croot: a C submodule may live inside the
// library named by the part of the module name before the first dot.
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
	if stat := pkLoadFunc(L, filename, name); stat != 0 {
		if stat != pkErrFunc {
			return pkCheckLoadStat(L, false, filename) /* real error */
		}
		/* else: open function not found */
		L.PushString(fmt.Sprintf("no module '%s' in file '%s'", name, filename))
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
		L.PushString(fmt.Sprintf("\n\tno field package.preload['%s']", name))
		return 1
	}
	/* 5.3.4: return only the loader; findloader's lua_call(..., 2)
	   supplies nil as the second require argument (not ":preload:"). */
	return 1
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
		if L.RawGetI(3, int64(i)) == LUA_TNIL { /* no more searchers? */
			L.Pop(1) /* remove nil */
			L.PushString(msg.String())
			L.Error2("module '%s' not found:%s", name, L.ToStringX(-1))
		}
		L.PushString(name)
		L.Call(1, 2)          /* call it */
		if L.IsFunction(-2) { /* did it find a loader? */
			return /* module loader found */
		} else if L.IsString(-2) { /* searcher returned error message? */
			L.Pop(1) /* remove extra return */
			msg.WriteString(L.ToStringX(-1)) /* already has "\n\t" */
			L.Pop(1) /* remove the message */
		} else { /* no error message */
			L.Pop(2) /* remove both returns */
		}
	}
}

// pkRequire is ll_require: the global 'require'.
func pkRequire(L *State) int {
	name := L.CheckString(1)
	/* Every use of the name here but one is a C use, and a C string
	   ends at its first zero byte: 'lua_getfield' and 'lua_setfield'
	   both look the name up with 'luaS_new', which is 'strlen', and
	   'findloader' hands it to the searchers with 'lua_pushstring',
	   which is 'strlen' too.  The one that is not is the name the
	   *loader* is given -- that one is 'lua_pushvalue(L, 1)', a copy
	   of the string as it stands, so a loader is handed all three
	   bytes of 'require("a\0b")' while every searcher is handed one.
	   The message names the short one, because 'luaL_error' writes
	   %s with strlen. */
	cname := luaCStr(name)
	L.SetTop(1) /* LOADED table will be at index 2 */
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.GetField(2, cname) /* LOADED[cname] */
	if L.ToBoolean(-1) { /* is it there? */
		return 1 /* package is already loaded */
	}
	/* else must load package */
	L.Pop(1) /* remove 'getfield' result */
	pkFindLoader(L, cname)
	/* stack: name; LOADED; searchers; loader data; loader function */
	L.Rotate(L.AbsIndex(-2), 1) /* function <-> loader data */
	L.PushValue(1)              /* name is 1st argument to module loader */
	L.PushValue(-3)             /* loader data is 2nd argument */
	/* stack: ...; loader data; loader function; mod. name; loader data */
	L.Call(2, 1) /* run loader to load module */
	/* stack: ...; loader data; result from loader */
	if !L.IsNil(-1) { /* non-nil return? */
		L.SetField(2, cname) /* LOADED[cname] = returned value */
	} else {
		L.Pop(1) /* pop nil */
	}
	if L.GetField(2, cname) == LUA_TNIL { /* module set no value? */
		L.PushBoolean(true)  /* use true as result */
		L.Copy(-1, -2)       /* replace loader result */
		L.SetField(2, cname) /* LOADED[cname] = true */
	}
	/* 5.3.4 returns only the module (5.4 also returns loader data). */
	L.Remove(-2) /* drop loader data under the result */
	return 1
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
