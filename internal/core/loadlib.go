// Translation of loadlib.c (PUC-Lua 5.4.9): the package library and the
// 'require' mechanism.
//
// Platform / cgo (see NOTES in reports/loadlib.md; companions kept):
//   - Without cgo: lsys_load / lsys_sym stub via pkDynLoadSym==nil
//     (≡ C #else fallback; LIB_FAIL "absent"). Intentional FLAG OK.
//   - With cgo && unix: cdlopen.go sets pkDynLoadSym = cgoLoadSym
//     (dlopen/dlsym). Handle reuse also tracked in cdlHandles there.
//   - CLIBS __gc unload ignored (Go GC policy; FLAG).
package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"
)

// Configuration constants (luaconf.h / lualib.h / loadlib.c).
const (
	pkPathSep   = ";"        // LUA_PATH_SEP
	pkPathMark  = "?"        // LUA_PATH_MARK
	pkExecDir   = "!"        // LUA_EXEC_DIR
	pkIgMark    = "-"        // LUA_IGMARK
	pkVerSuffix = "_5_4"     // LUA_VERSUFFIX
	pkPOF       = "luaopen_" // LUA_POF
	pkOFSEP     = "_"        // LUA_OFSEP

	// pkCLibs is the registry string key for loaded C library handles
	// (5.4.9: static const char *const CLIBS = "_CLIBS").
	pkCLibs        = "_CLIBS"
	pkLoadedTable  = "_LOADED"  // LUA_LOADED_TABLE
	pkPreloadTable = "_PRELOAD" // LUA_PRELOAD_TABLE

	pkPathVar  = "LUA_PATH"  // LUA_PATH_VAR
	pkCPathVar = "LUA_CPATH" // LUA_CPATH_VAR

	// pkLibFail is LIB_FAIL for a build without dynamic loading (C #else).
	pkLibFail = "absent"
	// pkDLMsg is DLMSG (C fallback); with cgo, dlerror() text is used instead.
	pkDLMsg = "dynamic libraries not enabled; check your Lua installation"
)

// pkCLIBSSentinel is a non-nil stand-in for a dlopen handle in registry.CLIBS.
// Go cannot stash a C function pointer as lightuserdata for later lsys_sym;
// the real handles live in cdlopen's cdlHandles. CLIBS still records that
// the library was opened so loadlib(path,"*") short-circuits like C.
var pkCLIBSSentinel int

// pkPathDefault / pkCPathDefault are LUA_PATH_DEFAULT / LUA_CPATH_DEFAULT
// from luaconf.h — Windows vs Unix forms (5.4 version dir).
var (
	pkPathDefault  string
	pkCPathDefault string
)

func init() {
	if runtime.GOOS == "windows" {
		/* luaconf.h _WIN32 branch */
		pkPathDefault = "!\\lua\\?.lua;!\\lua\\?\\init.lua;" +
			"!\\?.lua;!\\?\\init.lua;" +
			"!\\..\\share\\lua\\5.4\\?.lua;!\\..\\share\\lua\\5.4\\?\\init.lua;" +
			".\\?.lua;.\\?\\init.lua"
		pkCPathDefault = "!\\?.dll;" +
			"!\\..\\lib\\lua\\5.4\\?.dll;" +
			"!\\loadall.dll;.\\?.dll"
	} else {
		pkPathDefault = "/usr/local/share/lua/5.4/?.lua;" +
			"/usr/local/share/lua/5.4/?/init.lua;" +
			"/usr/local/lib/lua/5.4/?.lua;" +
			"/usr/local/lib/lua/5.4/?/init.lua;" +
			"./?.lua;./?/init.lua"
		pkCPathDefault = "/usr/local/lib/lua/5.4/?.so;" +
			"/usr/local/lib/lua/5.4/loadall.so;" +
			"./?.so"
	}
}

// pkDirSep is LUA_DIRSEP: "\\" on Windows, "/" elsewhere. It is what
// 'package.config' answers on its first line, and what a module name's
// '.' turns into when 'require' looks for a submodule.
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
// Stack matches C: versioned env name under the path value, SetField(-3)
// into package (CLIBS may sit under package from createclibstable), then
// pop the name. 5.4.9 inserts the default at the first ";;" only (no AUXMARK).
func pkSetPath(L *State, fieldname, envname, dft string) {
	nver := L.PushFString("%s%s", envname, pkVerSuffix)
	path, ok := os.LookupEnv(nver) /* try versioned name */
	if !ok {                       /* no versioned environment variable? */
		path, ok = os.LookupEnv(envname) /* try unversioned name */
	}
	if !ok || pkNoEnv(L) { /* no environment variable? */
		L.PushString(dft) /* use default */
	} else if dftmark := strings.Index(path, pkPathSep+pkPathSep); dftmark < 0 {
		L.PushString(path) /* nothing to change */
	} else { /* path contains a ";;": insert default path in its place */
		b := L.NewBuffer()
		if dftmark > 0 { /* is there a prefix before ';;'? */
			b.AddString(path[:dftmark]) /* add it */
			b.AddChar(pkPathSep[0])
		}
		b.AddString(dft)           /* add default */
		if dftmark < len(path)-2 { /* is there a suffix after ';;'? */
			b.AddChar(pkPathSep[0])
			b.AddString(path[dftmark+2:])
		}
		b.PushResult()
	}
	pkSetProgDir(L)
	L.SetField(-3, fieldname) /* package[fieldname] = path value */
	L.Pop(1)                  /* pop versioned variable name ('nver') */
}

// pkIsWindowsFn is the GOOS check for setprogdir's Windows rewrite.
// Tests may override it temporarily; production always uses runtime.GOOS.
var pkIsWindowsFn = func() bool { return runtime.GOOS == "windows" }

// pkExecutableFn returns the process executable path (os.Executable).
// Tests may override it to force error / empty-dir arms.
var pkExecutableFn = os.Executable

// pkSetProgDir is setprogdir: on Windows replace LUA_EXEC_DIR ("!") with
// the executable's directory; elsewhere a no-op (C #define setprogdir(L) ((void)0)).
func pkSetProgDir(L *State) {
	if !pkIsWindowsFn() {
		return
	}
	exe, err := pkExecutableFn()
	if err != nil {
		L.Error2("unable to get ModuleFileName")
	}
	dir := filepath.Dir(exe)
	if dir == "" || dir == "." {
		L.Error2("unable to get ModuleFileName")
	}
	s := L.ToStringX(-1)
	L.GSub(s, pkExecDir, dir)
	L.Remove(-2) /* remove original string */
}

// ---------------------------------------------------------------------
// dynamic loading (stub without cgo; cdlopen_* with cgo && unix)
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

// pkCheckCLib returns registry.CLIBS[path] (checkclib).
func pkCheckCLib(L *State, path string) unsafe.Pointer {
	L.GetField(LUA_REGISTRYINDEX, pkCLibs)
	L.GetField(-1, path)
	var plib unsafe.Pointer
	if L.IsLightUserData(-1) {
		plib, _ = L.ToUserData(-1).(unsafe.Pointer)
	}
	L.Pop(2) /* pop CLIBS table and 'plib' */
	return plib
}

// pkAddToCLib does registry.CLIBS[path] = plib and appends to the list
// (addtoclib).
func pkAddToCLib(L *State, path string, plib unsafe.Pointer) {
	L.GetField(LUA_REGISTRYINDEX, pkCLibs)
	L.PushLightUserData(plib)
	L.PushValue(-1)
	L.SetField(-3, path)        /* CLIBS[path] = plib */
	L.RawSetI(-2, L.Len2(-2)+1) /* CLIBS[#CLIBS+1] = plib */
	L.Pop(1)                    /* pop CLIBS table */
}

// pkIsStarSym is C's (*sym == '*'): load library only, no function.
func pkIsStarSym(sym string) bool {
	return len(sym) > 0 && sym[0] == '*'
}

// pkLookForFunc is lookforfunc. It returns 0 (and a value on the stack)
// on success, or an error code plus an error message.
//
// Adaptation: C splits lsys_load / lsys_sym around a void* handle.
// Go's pkDynLoadSym is load+sym in one step (a Go func cannot travel as
// void*). CLIBS still records a non-nil sentinel after a successful open
// so a later loadlib(path,"*") short-circuits like C. Symbol re-resolve
// still goes through the backend (cdlHandles may re-dlopen).
// First-char '*' is normalized to "*" for the backend (C seeglb; exact
// "*" makes cgoLoadSym push true like C's post-load pushboolean).
func pkLookForFunc(L *State, path, sym string) int {
	reg := pkCheckCLib(L, path) /* check loaded C libraries */
	if reg == nil {             /* must load library? */
		loadsym := sym
		if pkIsStarSym(sym) {
			loadsym = "*" /* C: seeglb when *sym=='*'; backend wants exact "*" */
		}
		stat := pkLoadSym(L, path, loadsym)
		if stat == pkErrLib {
			return pkErrLib /* unable to load library */
		}
		pkAddToCLib(L, path, unsafe.Pointer(&pkCLIBSSentinel))
		if pkIsStarSym(sym) { /* loading only library (no function)? */
			/* backend already left true for "*"; C pushes boolean here */
			return 0 /* no errors */
		}
		return stat /* 0 or ERRFUNC; value/msg already on stack */
	}
	if pkIsStarSym(sym) { /* already loaded; only library? */
		L.PushBoolean(true) /* return 'true' */
		return 0            /* no errors */
	}
	return pkLoadSym(L, path, sym) /* resolve symbol via backend */
}

// pkLoadFunc is loadfunc: it builds the "luaopen_X" name of a C module.
func pkLoadFunc(L *State, filename, modname string) int {
	/* C: luaL_gsub uses strlen — truncate embedded NUL like checkstring. */
	modname = L.GSub(luaCStr(modname), ".", pkOFSEP)
	mark := strings.Index(modname, pkIgMark)
	if mark >= 0 { /* is there an ignore mark? */
		openfunc := L.PushLString(modname[:mark])
		openfunc = L.PushFString("%s%s", pkPOF, openfunc)
		if stat := pkLookForFunc(L, filename, openfunc); stat != pkErrFunc {
			return stat
		}
		modname = modname[mark+1:] /* else go ahead and try old-style name */
	}
	openfunc := L.PushFString("%s%s", pkPOF, modname)
	return pkLookForFunc(L, filename, openfunc)
}

// pkLoadLib is ll_loadlib: package.loadlib.
func pkLoadLib(L *State) int {
	path := luaCStr(L.CheckString(1)) /* path */
	init := luaCStr(L.CheckString(2)) /* init */
	stat := pkLookForFunc(L, path, init)
	if stat == 0 { /* no errors? */
		return 1 /* return the loaded function */
	}
	/* error; error message is on stack top */
	L.PushNil() /* luaL_pushfail — fail is nil in 5.4 */
	L.Insert(-2)
	/* LIB_FAIL is "open" when the build can dlopen and "absent" when not */
	if stat == pkErrLib {
		if pkHasDynLoad() {
			L.PushString("open")
		} else {
			L.PushString(pkLibFail)
		}
	} else {
		L.PushString("init") /* library opened, no such function */
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
func pkPushErrorNotFound(L *State, path string) {
	b := L.NewBuffer()
	b.AddString("no file '")
	b.AddGSub(path, pkPathSep, "'\n\tno file '")
	b.AddString("'")
	b.PushResult()
}

// pkSearchPath searches for 'name' in 'path' (searchpath). It pushes the
// file name found, or the error message when there is none.
//
// Adaptation: C writes names into a luaL_Buffer and walks with
// getnextfilename; Go strings are immutable, so the list is split with
// strings.Split (keeps empty segments like getnextfilename).
func pkSearchPath(L *State, name, path, sep, dirsep string) bool {
	/* 'luaL_addgsub' inserts the name with 'luaL_addstring', which is
	   'strlen', so a name longer than the bytes before its first zero
	   is not inserted: 'package.searchpath("a\0b", "?.lua")' looks for
	   "a.lua" and says so. */
	name = luaCStr(name)
	/* separator is non-empty and appears in 'name'?
	   C: *sep != '\0' && strchr(name, *sep) — test first byte of sep. */
	if sep != "" && strings.IndexByte(name, sep[0]) >= 0 {
		name = L.GSub(name, sep, dirsep) /* replace it by 'dirsep' */
	}
	/* add path to the buffer, replacing marks ('?') with the file name */
	b := L.NewBuffer()
	b.AddGSub(path, pkPathMark, name)
	pathname := b.String()
	/* Empty pathname: C getnextfilename returns NULL immediately (no
	   readable("") attempt). strings.Split("", ";") yields [""]. */
	if pathname != "" {
		for _, filename := range strings.Split(pathname, pkPathSep) {
			if pkReadable(filename) { /* does file exist and is readable? */
				L.PushString(filename) /* save and return name */
				return true
			}
		}
	}
	b.PushResult()                          /* push path to create error message */
	pkPushErrorNotFound(L, L.ToStringX(-1)) /* create error message */
	return false                            /* not found */
}

// pkSearchPathFunc is ll_searchpath: package.searchpath.
func pkSearchPathFunc(L *State) int {
	if pkSearchPath(L, L.CheckString(1), L.CheckString(2),
		L.OptString(3, "."), L.OptString(4, pkDirSep)) {
		return 1
	}
	/* error message is on top of the stack */
	L.PushNil() /* luaL_pushfail */
	L.Insert(-2)
	return 2 /* return fail + error message */
}

// pkFindFile searches for 'name' in package[pname] (findfile). It
// returns the file name; pkSearchPath leaves it (or the error message)
// on the top of the stack. The path from getfield stays on the stack
// under that result, matching C (caller return-N discards it).
func pkFindFile(L *State, name, pname, dirsep string) (string, bool) {
	L.GetField(UpvalueIndex(1), pname)
	path, ok := L.ToString(-1)
	if !ok {
		L.Error2("'package.%s' must be a string", pname)
	}
	found := pkSearchPath(L, name, path, ".", dirsep)
	return L.ToStringX(-1), found
}

// pkCheckLoad is checkload: 'stat' tells whether the module was loaded,
// and when it was not, the loader's message is on the stack top (where C
// reads it with lua_tostring(L, -1)).
func pkCheckLoad(L *State, stat bool, filename string) int {
	if stat { /* module loaded successfully? */
		L.PushString(filename) /* will be 2nd argument to module */
		return 2               /* return open function and file name */
	}
	return L.Error2("error loading module '%s' from file '%s':\n\t%s",
		L.ToStringX(1), filename, L.ToStringX(-1))
}

// pkSearcherLua is searcher_Lua: it loads a module from a Lua file.
func pkSearcherLua(L *State) int {
	name := luaCStr(L.CheckString(1))
	filename, ok := pkFindFile(L, name, "path", pkLSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	err := L.LoadFile(filename)
	if err != nil {
		L.PushString(err.Error()) /* C: luaL_loadfile leaves msg on stack */
	}
	return pkCheckLoad(L, err == nil, filename)
}

// pkSearcherC is searcher_C: a file found in package.cpath is loaded as
// a C module, and a failure to load it is a real error (C stops there,
// it does not go on to the next searcher).
func pkSearcherC(L *State) int {
	name := luaCStr(L.CheckString(1))
	filename, ok := pkFindFile(L, name, "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	return pkCheckLoad(L, pkLoadFunc(L, filename, name) == 0, filename)
}

// pkSearcherCRoot is searcher_Croot: a C submodule may live inside the
// library named by the part of the module name before the first dot.
func pkSearcherCRoot(L *State) int {
	name := luaCStr(L.CheckString(1))
	dot := strings.IndexByte(name, '.')
	if dot < 0 {
		return 0 /* is root */
	}
	L.PushLString(name[:dot]) /* root name, as C pushlstring */
	filename, ok := pkFindFile(L, L.ToStringX(-1), "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* root not found */
	}
	if stat := pkLoadFunc(L, filename, name); stat != 0 {
		if stat != pkErrFunc {
			return pkCheckLoad(L, false, filename) /* real error */
		}
		/* else: open function not found */
		L.PushFString("no module '%s' in file '%s'", name, filename)
		return 1
	}
	L.PushString(filename) /* will be 2nd argument to module */
	return 2
}

// pkSearcherPreload is searcher_preload: it looks the module up in
// package.preload. 5.4.9 returns loader + ":preload:" (not 5.3's single).
func pkSearcherPreload(L *State) int {
	name := luaCStr(L.CheckString(1))
	L.GetField(LUA_REGISTRYINDEX, pkPreloadTable)
	if L.GetField(-1, name) == LUA_TNIL { /* not found? */
		L.PushFString("no field package.preload['%s']", name)
		return 1
	}
	L.PushString(":preload:")
	return 2
}

// pkFindLoader iterates over package.searchers looking for a loader for
// 'name' (findloader); it leaves the loader and its data on the stack.
func pkFindLoader(L *State, name string) {
	/* push 'package.searchers' to index 3 in the stack */
	if L.GetField(UpvalueIndex(1), "searchers") != LUA_TTABLE {
		L.Error2("'package.searchers' must be a table")
	}
	msg := L.NewBuffer() /* to build error message */
	/*  iterate over available searchers to find a loader */
	for i := 1; ; i++ {
		msg.AddString("\n\t")                   /* error-message prefix */
		if L.RawGetI(3, int64(i)) == LUA_TNIL { /* no more searchers? */
			L.Pop(1)         /* remove nil */
			msg.BuffSub(2)   /* remove prefix */
			msg.PushResult() /* create error message */
			L.Error2("module '%s' not found:%s", name, L.ToStringX(-1))
		}
		L.PushString(name)
		L.Call(1, 2)          /* call it */
		if L.IsFunction(-2) { /* did it find a loader? */
			return /* module loader found */
		} else if L.IsString(-2) { /* searcher returned error message? */
			L.Pop(1)       /* remove extra return */
			msg.AddValue() /* concatenate error message */
		} else { /* no error message */
			L.Pop(2)       /* remove both returns */
			msg.BuffSub(2) /* remove prefix */
		}
	}
}

// pkRequire is ll_require: the global 'require'.
// 5.4.9 returns module result and loader data (2 values).
func pkRequire(L *State) int {
	name := L.CheckString(1)
	/* Every use of the name here but one is a C use, and a C string
	   ends at its first zero byte: 'lua_getfield' and 'lua_setfield'
	   both look the name up with 'luaS_new', which is 'strlen', and
	   'findloader' hands it to the searchers with 'lua_pushstring',
	   which is 'strlen' too.  The one that is not is the name the
	   *loader* is given — 5.4 uses 'lua_pushvalue(L, 1)', a copy of
	   the string as it stands. */
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
	L.Rotate(L.AbsIndex(-2), 1) /* loader data <-> module result */
	return 2                    /* return module result and loader data */
}

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

// pkFuncs are the functions of the 'package' table (pk_funcs). The
// fields 'preload', 'cpath', 'path', 'searchers' and 'loaded' are
// placeholders (NULL funcs → false) and are set by OpenPackage.
var pkFuncs = []Reg{
	{Name: "loadlib", Func: pkLoadLib},
	{Name: "searchpath", Func: pkSearchPathFunc},
	/* placeholders */
	{Name: "preload", Func: nil},
	{Name: "cpath", Func: nil},
	{Name: "path", Func: nil},
	{Name: "searchers", Func: nil},
	{Name: "loaded", Func: nil},
}

// pkLLFuncs are the functions opened into the global table (ll_funcs).
var pkLLFuncs = []Reg{
	{Name: "require", Func: pkRequire},
}

// pkGctm is the __gc metamethod of the CLIBS table (gctm).
// FLAG: does not call lsys_unloadlib — Go GC / finalizer policy.
func pkGctm(L *State) int {
	n := L.Len2(1)
	for ; n >= 1; n-- { /* for each handle, in reverse order */
		L.RawGetI(1, n) /* get handle CLIBS[n] */
		/* C: lsys_unloadlib(lua_touserdata(L, -1)); omitted (Go GC) */
		L.Pop(1) /* pop handle */
	}
	return 0
}

// pkCreateCLibsTable creates the CLIBS table (createclibstable).
// Registry key is the string "_CLIBS" (5.4.9). Leaves CLIBS on the stack
// under 'package' like C (caller return-1 discards it).
func pkCreateCLibsTable(L *State) {
	L.GetSubTable(LUA_REGISTRYINDEX, pkCLibs) /* create CLIBS table */
	L.CreateTable(0, 1)                       /* create metatable for CLIBS */
	L.PushGoFunc(pkGctm)
	L.SetField(-2, "__gc") /* set finalizer for CLIBS table */
	L.SetMetaTable(-2)
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
// Global _G.package and LOADED["package"] are set by RequireF/OpenLibs
// (luaL_requiref), not here — matches C.
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
	L.PushGlobalTable()
	L.PushValue(-2)          /* set 'package' as upvalue for next lib */
	L.SetFuncs(pkLLFuncs, 1) /* open lib into global table */
	L.Pop(1)                 /* pop global table */
	return 1                 /* return 'package' table */
}
