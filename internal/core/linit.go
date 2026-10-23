// Translation of linit.c (PUC-Lua 5.3.4): initialization of standard libraries.
//
// C surface: static loadedlibs[] + luaL_openlibs.
// Adaptation: luaL_requiref (lauxlib.c) is spelled out here as (*State).RequireF
// because the lauxlib translation does not host it; body matches C.
//
// Go-only (not in linit.c): (*State).Open(name) — single-library open helper.
package core

// loadedLibs is C linit.c loadedlibs[]: (name, open function).
// Names are the LUA_*LIBNAME string values from lualib.h.
// bit32 is always registered: matches default LUA_COMPAT_BITLIB in 5.3.4 luaconf.h.
var loadedLibs = []struct {
	name string
	open func(*State) int
}{
	{"_G", OpenBase},             // {"_G", luaopen_base}
	{"package", OpenPackage},     // {LUA_LOADLIBNAME, luaopen_package}
	{"coroutine", OpenCoroutine}, // {LUA_COLIBNAME, luaopen_coroutine}
	{"table", OpenTable},         // {LUA_TABLIBNAME, luaopen_table}
	{"io", OpenIO},               // {LUA_IOLIBNAME, luaopen_io}
	{"os", OpenOS},               // {LUA_OSLIBNAME, luaopen_os}
	{"string", OpenString},       // {LUA_STRLIBNAME, luaopen_string}
	{"math", OpenMath},           // {LUA_MATHLIBNAME, luaopen_math}
	{"utf8", OpenUTF8},           // {LUA_UTF8LIBNAME, luaopen_utf8}
	{"debug", OpenDebug},         // {LUA_DBLIBNAME, luaopen_debug}
	{"bit32", OpenBit32},         // #if LUA_COMPAT_BITLIB {LUA_BITLIBNAME, luaopen_bit32}
	// C sentinel {NULL, NULL} → end of slice
}

// RequireF is luaL_requiref (lauxlib.c:971-988), placed here (see file header).
// Adds module 'modname' to package.loaded, calling 'openf' if not already
// loaded; when 'glb' is set also assigns the module to the global of the
// same name.
func (L *State) RequireF(modname string, openf GoFunc, glb bool) {
	L.GetSubTable(LUA_REGISTRYINDEX, pkLoadedTable) /* LOADED table */
	L.GetField(-1, modname)                         /* LOADED[modname] */
	if !L.ToBoolean(-1) {                           /* package not already loaded? */
		L.Pop(1) /* remove field */
		L.PushGoClosure(openf, 0)
		L.PushString(modname) /* argument to open function */
		L.Call(1, 1)          /* call 'openf' to open module */
		L.PushValue(-1)       /* make copy of module (call result) */
		L.SetField(-3, modname) /* LOADED[modname] = module */
	}
	L.Remove(-2) /* remove LOADED table */
	if glb {
		L.PushValue(-1)      /* copy of module */
		L.SetGlobal(modname) /* _G[modname] = module */
	}
}

// OpenLibs opens all standard libraries (luaL_openlibs).
// "require" each entry of loadedLibs and set results into the global table.
func (L *State) OpenLibs() {
	for _, lib := range loadedLibs {
		L.RequireF(lib.name, lib.open, true)
		L.Pop(1) /* remove lib */
	}
}

// Open opens a single standard library by name, e.g. "string".
// Returns false when the name is not a known library.
//
// FLAG: Go-only API — not present in C linit.c / luaL_openlibs.
// Uses the same RequireF+Pop path as OpenLibs for one entry.
func (L *State) Open(name string) bool {
	for _, lib := range loadedLibs {
		if lib.name == name {
			L.RequireF(name, lib.open, true)
			L.Pop(1)
			return true
		}
	}
	return false
}
