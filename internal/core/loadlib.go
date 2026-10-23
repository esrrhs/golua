// Translation of loadlib.c (PUC-Lua 5.3.4): the package library and the
// 'require' mechanism.
//
// Platform / cgo (see NOTES.md; not rewritten in this unit — FLAG KEEP):
//   - Without cgo: lsys_load / lsys_sym stub via pkDynLoadSym==nil
//     (≡ C #else fallback; LIB_FAIL "absent"). Intentional.
//   - With cgo && unix: cdlopen.go sets pkDynLoadSym = cgoLoadSym
//     (dlopen/dlsym). Handle reuse also tracked in cdlHandles there.
//   - CLIBS __gc unload omitted (Go GC policy; see FLAG in NOTES).
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
	pkVerSuffix = "_5_3"     // LUA_VERSUFFIX
	pkPOF       = "luaopen_" // LUA_POF
	pkOFSEP     = "_"        // LUA_OFSEP

	pkLoadedTable  = "_LOADED"  // LUA_LOADED_TABLE
	pkPreloadTable = "_PRELOAD" // LUA_PRELOAD_TABLE

	pkPathVar  = "LUA_PATH"  // LUA_PATH_VAR
	pkCPathVar = "LUA_CPATH" // LUA_CPATH_VAR

	// pkLibFail is LIB_FAIL for a build without dynamic loading (C #else).
	pkLibFail = "absent"
	// pkDLMsg is DLMSG (C fallback); with cgo, dlerror() text is used instead.
	pkDLMsg = "dynamic libraries not enabled; check your Lua installation"

	pkAuxMark = "\x01" // AUXMARK — single SOH, as in C "\1"
)

// pkCLIBS is C's static const int CLIBS: unique registry key via its address
// (lua_rawgetp / lua_rawsetp). Not a string key.
var pkCLIBS int

// pkCLIBSSentinel is a non-nil stand-in for a dlopen handle in registry.CLIBS.
// Go cannot stash a C function pointer as lightuserdata for later lsys_sym;
// the real handles live in cdlopen's cdlHandles. CLIBS still records that
// the library was opened so loadlib(path,"*") short-circuits like C.
var pkCLIBSSentinel int

// pkPathDefault / pkCPathDefault are LUA_PATH_DEFAULT / LUA_CPATH_DEFAULT
// from luaconf.h — Windows vs Unix forms.
var (
	pkPathDefault  string
	pkCPathDefault string
)

func init() {
	if runtime.GOOS == "windows" {
		/* luaconf.h _WIN32 branch */
		pkPathDefault = "!\\lua\\?.lua;!\\lua\\?\\init.lua;" +
			"!\\?.lua;!\\?\\init.lua;" +
			"!\\..\\share\\lua\\5.3\\?.lua;!\\..\\share\\lua\\5.3\\?\\init.lua;" +
			".\\?.lua;.\\?\\init.lua"
		pkCPathDefault = "!\\?.dll;" +
			"!\\..\\lib\\lua\\5.3\\?.dll;" +
			"!\\loadall.dll;.\\?.dll"
	} else {
		pkPathDefault = "/usr/local/share/lua/5.3/?.lua;" +
			"/usr/local/share/lua/5.3/?/init.lua;" +
			"/usr/local/lib/lua/5.3/?.lua;" +
			"/usr/local/lib/lua/5.3/?/init.lua;" +
			"./?.lua;./?/init.lua"
		pkCPathDefault = "/usr/local/lib/lua/5.3/?.so;" +
			"/usr/local/lib/lua/5.3/loadall.so;" +
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
// Stack shape matches C: versioned env name stays under the path value,
// SetField(-3) writes into package, then the name is popped.
func pkSetPath(L *State, fieldname, envname, dft string) {
	nver := L.PushFString("%s%s", envname, pkVerSuffix)
	path, ok := os.LookupEnv(nver) /* use versioned name */
	if !ok {                       /* no environment variable? */
		path, ok = os.LookupEnv(envname) /* try unversioned name */
	}
	if !ok || pkNoEnv(L) { /* no environment variable? */
		L.PushString(dft) /* use default */
	} else {
		/* replace ";;" by ";AUXMARK;" and then AUXMARK by default path */
		path = L.GSub(path, pkPathSep+pkPathSep, pkPathSep+pkAuxMark+pkPathSep)
		L.GSub(path, pkAuxMark, dft)
		L.Remove(-2) /* remove result from 1st 'gsub' */
	}
	pkSetProgDir(L)
	L.SetField(-3, fieldname) /* package[fieldname] = path value */
	L.Pop(1)                  /* pop versioned variable name */
}

// pkSetProgDir is setprogdir: on Windows replace LUA_EXEC_DIR ("!") with
// the executable's directory; elsewhere a no-op (C #define setprogdir(L) ((void)0)).
func pkSetProgDir(L *State) {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
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
	L.RawGetP(LUA_REGISTRYINDEX, unsafe.Pointer(&pkCLIBS))
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
	L.RawGetP(LUA_REGISTRYINDEX, unsafe.Pointer(&pkCLIBS))
	L.PushLightUserData(plib)
	L.PushValue(-1)
	L.SetField(-3, path)        /* CLIBS[path] = plib */
	L.RawSetI(-2, L.Len2(-2)+1) /* CLIBS[#CLIBS+1] = plib */
	L.Pop(1)                    /* pop CLIBS table */
}

// pkLookForFunc is lookforfunc. It returns 0 (and a value on the stack)
// on success, or an error code plus an error message.
//
// Adaptation (FLAG): C splits lsys_load / lsys_sym around a void* handle.
// Go's pkDynLoadSym is load+sym in one step (a Go func cannot travel as
// void*). CLIBS records a non-nil sentinel after a successful open so a
// later loadlib(path,"*") short-circuits like C. Symbol re-resolve still
// goes through the backend (cdlHandles may re-dlopen — NOTES).
func pkLookForFunc(L *State, path, sym string) int {
	reg := pkCheckCLib(L, path) /* check loaded C libraries */
	if reg == nil {             /* must load library? */
		stat := pkLoadSym(L, path, sym)
		if stat == pkErrLib {
			return pkErrLib /* unable to load library */
		}
		pkAddToCLib(L, path, unsafe.Pointer(&pkCLIBSSentinel))
		if len(sym) > 0 && sym[0] == '*' { /* loading only library? */
			/* cgoLoadSym already pushed true for "*". */
			if !(L.GetTop() > 0 && L.IsBoolean(-1)) {
				L.PushBoolean(true)
			}
			return 0
		}
		return stat /* 0 or ERRFUNC; value/msg already on stack */
	}
	if len(sym) > 0 && sym[0] == '*' { /* already loaded; only library? */
		L.PushBoolean(true) /* return 'true' */
		return 0
	}
	return pkLoadSym(L, path, sym) /* resolve symbol via backend */
}

// pkLoadFunc is loadfunc: builds the "luaopen_X" name of a C module.
func pkLoadFunc(L *State, filename, modname string) int {
	/* C: luaL_gsub uses strlen — truncate embedded NUL like checkstring. */
	modname = L.GSub(luaCStr(modname), ".", pkOFSEP)
	mark := strings.Index(modname, pkIgMark)
	if mark >= 0 {
		openfunc := L.PushLString(modname[:mark])
		openfunc = L.PushFString("%s%s", pkPOF, openfunc)
		stat := pkLookForFunc(L, filename, openfunc)
		if stat != pkErrFunc {
			return stat
		}
		modname = modname[mark+1:] /* else go ahead and try old-style name */
	}
	openfunc := L.PushFString("%s%s", pkPOF, modname)
	return pkLookForFunc(L, filename, openfunc)
}

// pkLoadLib is ll_loadlib: package.loadlib.
func pkLoadLib(L *State) int {
	path := luaCStr(L.CheckString(1))
	init := luaCStr(L.CheckString(2))
	stat := pkLookForFunc(L, path, init)
	if stat == 0 { /* no errors? */
		return 1 /* return the loaded function */
	}
	/* error; error message is on stack top */
	L.PushNil()
	L.Insert(-2)
	if stat == pkErrLib {
		/* LIB_FAIL is "open" when the build can dlopen and "absent" when not */
		if pkHasDynLoad() {
			L.PushString("open")
		} else {
			L.PushString(pkLibFail)
		}
	} else {
		L.PushString("init")
	}
	return 3 /* return nil, error message, and where */
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

// pkPushNextTemplate pushes the next path template and returns the
// remainder of 'path' (pushnexttemplate). ok is false when there are no
// more templates.
func pkPushNextTemplate(L *State, path string) (rest string, ok bool) {
	for strings.HasPrefix(path, pkPathSep) { /* skip separators */
		path = path[len(pkPathSep):]
	}
	if path == "" {
		return "", false /* no more templates */
	}
	i := strings.Index(path, pkPathSep) /* find next separator */
	var tmpl string
	if i < 0 {
		tmpl = path
		rest = ""
	} else {
		tmpl = path[:i]
		rest = path[i:] /* points at separator; next call skips it */
	}
	L.PushString(tmpl) /* template */
	return rest, true
}

// pkSearchPath searches for 'name' in 'path' (searchpath). On success it
// leaves the file name on the stack and returns it; on failure it leaves
// the aggregated error message and returns ("", false).
func pkSearchPath(L *State, name, path, sep, dirsep string) (string, bool) {
	msg := L.NewBuffer() /* to build error message */
	/* C: luaL_addgsub / strlen — truncate name at first NUL. */
	name = luaCStr(name)
	if sep != "" { /* non-empty separator? */
		name = L.GSub(name, sep, dirsep) /* replace it by 'dirsep' */
	}
	for {
		var ok bool
		path, ok = pkPushNextTemplate(L, path)
		if !ok {
			break
		}
		filename := L.GSub(L.ToStringX(-1), pkPathMark, name)
		L.Remove(-2)          /* remove path template */
		if pkReadable(filename) { /* does file exist and is readable? */
			return filename, true /* return that file name (on stack) */
		}
		L.PushFString("\n\tno file '%s'", filename)
		L.Remove(-2)   /* remove file name */
		msg.AddValue() /* concatenate error msg. entry */
	}
	msg.PushResult() /* create error message */
	return "", false /* not found */
}

// pkSearchPathFunc is ll_searchpath: package.searchpath.
func pkSearchPathFunc(L *State) int {
	_, ok := pkSearchPath(L, L.CheckString(1), L.CheckString(2),
		L.OptString(3, "."), L.OptString(4, pkDirSep))
	if ok {
		return 1
	}
	/* error message is on top of the stack */
	L.PushNil()
	L.Insert(-2)
	return 2 /* return nil + error message */
}

// pkFindFile searches for 'name' in package[pname] (findfile).
// Leaves package[pname] on the stack under the searchpath result, as C does.
func pkFindFile(L *State, name, pname, dirsep string) (string, bool) {
	L.GetField(UpvalueIndex(1), pname)
	path, ok := L.ToString(-1)
	if !ok {
		L.Error2("'package.%s' must be a string", pname)
	}
	return pkSearchPath(L, name, path, ".", dirsep)
}

// pkCheckLoad is checkload when the loader reports via a Go error
// (searcher_Lua / LoadFile). Format string matches C.
func pkCheckLoad(L *State, err error, filename string) int {
	if err == nil { /* module loaded successfully? */
		L.PushString(filename) /* will be 2nd argument to module */
		return 2               /* return open function and file name */
	}
	return L.Error2("error loading module '%s' from file '%s':\n\t%s",
		L.ToStringX(1), filename, err.Error())
}

// pkCheckLoadStat is checkload when the error message is already on the
// stack top (searcher_C / lookforfunc), as C reads with lua_tostring(L, -1).
func pkCheckLoadStat(L *State, stat bool, filename string) int {
	if stat { /* module loaded successfully? */
		L.PushString(filename) /* will be 2nd argument to module */
		return 2               /* return open function and file name */
	}
	return L.Error2("error loading module '%s' from file '%s':\n\t%s",
		L.ToStringX(1), filename, L.ToStringX(-1))
}

// pkSearcherLua is searcher_Lua: loads a module from a Lua file.
// Uses LoadFile (≡ luaL_loadfile) so BOM / shebang are skipped.
func pkSearcherLua(L *State) int {
	name := L.CheckString(1)
	filename, ok := pkFindFile(L, name, "path", pkLSUBSEP)
	if !ok {
		return 1 /* module not found in this path */
	}
	return pkCheckLoad(L, L.LoadFile(filename), filename)
}

// pkSearcherC is searcher_C: a file found in package.cpath is loaded as
// a C module; failure to load it is a real error (C stops there).
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
	p := strings.IndexByte(name, '.')
	if p < 0 {
		return 0 /* is root */
	}
	L.PushString(name[:p]) /* C: lua_pushlstring(L, name, p - name) */
	filename, ok := pkFindFile(L, L.ToStringX(-1), "cpath", pkCSUBSEP)
	if !ok {
		return 1 /* root not found */
	}
	if stat := pkLoadFunc(L, filename, name); stat != 0 {
		if stat != pkErrFunc {
			return pkCheckLoadStat(L, false, filename) /* real error */
		}
		/* open function not found */
		L.PushFString("\n\tno module '%s' in file '%s'", name, filename)
		return 1
	}
	L.PushString(filename) /* will be 2nd argument to module */
	return 2
}

// pkSearcherPreload is searcher_preload: looks the module up in
// package.preload. 5.3.4 returns only the loader (findloader's
// lua_call(..., 2) supplies nil as the second require argument).
func pkSearcherPreload(L *State) int {
	name := L.CheckString(1)
	L.GetField(LUA_REGISTRYINDEX, pkPreloadTable)
	if L.GetField(-1, name) == LUA_TNIL { /* not found? */
		L.PushFString("\n\tno field package.preload['%s']", name)
	}
	return 1
}

// pkFindLoader iterates over package.searchers looking for a loader for
// 'name' (findloader); leaves the loader and its data on the stack.
func pkFindLoader(L *State, name string) {
	msg := L.NewBuffer() /* to build error message */
	/* push 'package.searchers' to index 3 in the stack */
	if L.GetField(UpvalueIndex(1), "searchers") != LUA_TTABLE {
		L.Error2("'package.searchers' must be a table")
	}
	/* iterate over available searchers to find a loader */
	for i := 1; ; i++ {
		if L.RawGetI(3, int64(i)) == LUA_TNIL { /* no more searchers? */
			L.Pop(1) /* remove nil */
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
		} else {
			L.Pop(2) /* remove both returns */
		}
	}
}

// pkRequire is ll_require: the global 'require'.
func pkRequire(L *State) int {
	name := L.CheckString(1)
	/* C: luaL_checkstring → const char*; every pushstring/getfield uses
	   strlen. Go CheckString keeps embedded NUL; truncate like C. */
	cname := luaCStr(name)
	L.SetTop(1) /* LOADED table will be at index 2 */
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	L.GetField(2, cname) /* LOADED[name] */
	if L.ToBoolean(-1) { /* is it there? */
		return 1 /* package is already loaded */
	}
	/* else must load package */
	L.Pop(1) /* remove 'getfield' result */
	pkFindLoader(L, cname)
	/* C: lua_pushstring(L, name) — NUL-truncates; not lua_pushvalue(1). */
	L.PushString(cname) /* pass name as argument to module loader */
	L.Insert(-2)        /* name is 1st argument (before search data) */
	L.Call(2, 1)        /* run loader to load module */
	if !L.IsNil(-1) {   /* non-nil return? */
		L.SetField(2, cname) /* LOADED[name] = returned value */
	}
	if L.GetField(2, cname) == LUA_TNIL { /* module set no value? */
		L.PushBoolean(true)  /* use true as result */
		L.PushValue(-1)      /* extra copy to be returned */
		L.SetField(2, cname) /* LOADED[name] = true */
	}
	/* 5.3.4 returns only the module (not 5.4 loader-data). */
	return 1
}

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

// pkFuncs are the functions of the 'package' table (pk_funcs). The
// fields 'preload', 'cpath', 'path', 'searchers' and 'loaded' are
// placeholders in C and are set by OpenPackage instead.
// LUA_COMPAT_MODULE (module/seeall) and LUA_COMPAT_LOADERS are gated on
// LUA_COMPAT_5_1; default 5.3.4 Makefile is -DLUA_COMPAT_5_2 only → omitted.
var pkFuncs = []Reg{
	{Name: "loadlib", Func: pkLoadLib},
	{Name: "searchpath", Func: pkSearchPathFunc},
}

// pkLLFuncs are the functions opened into the global table (ll_funcs).
var pkLLFuncs = []Reg{
	{Name: "require", Func: pkRequire},
}

// pkGctm is the __gc metamethod of the CLIBS table (gctm).
// FLAG: does not call lsys_unloadlib — Go GC / finalizer policy (NOTES).
func pkGctm(L *State) int {
	n := L.Len2(1)
	for ; n >= 1; n-- { /* for each handle, in reverse order */
		L.RawGetI(1, n) /* get handle CLIBS[n] */
		/* C: lsys_unloadlib(lua_touserdata(L, -1)); omitted (FLAG GC) */
		L.Pop(1) /* pop handle */
	}
	return 0
}

// pkCreateCLibsTable creates the CLIBS table (createclibstable).
// Registry key is lightuserdata &pkCLIBS (C: &CLIBS), not a string.
func pkCreateCLibsTable(L *State) {
	L.NewTable()        /* create CLIBS table */
	L.CreateTable(0, 1) /* create metatable for CLIBS */
	L.PushGoFunc(pkGctm)
	L.SetField(-2, "__gc") /* set finalizer for CLIBS table */
	L.SetMetaTable(-2)
	L.RawSetP(LUA_REGISTRYINDEX, unsafe.Pointer(&pkCLIBS)) /* set CLIBS in registry */
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
	/* LUA_COMPAT_LOADERS (LUA_COMPAT_5_1 only): omitted — default Makefile */
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
	L.PushValue(-2) /* set 'package' as upvalue for next lib */
	L.SetFuncs(pkLLFuncs, 1) /* open lib into global table */
	L.Pop(1)                 /* pop global table */
	return 1                 /* return 'package' table */
}
