// Translation of linit.c (PUC-Lua 5.4.9): initialization of standard libraries.
//
// C surface: static loadedlibs[] + luaL_openlibs (lualib.h).
// Adaptation: luaL_requiref (lauxlib.c) is spelled out here as (*State).RequireF
// because the lauxlib translation does not host it; body matches C 5.4.9.
//
// Go-only (not in linit.c): (*State).Open(name) — single-library open helper.
// No bit32: removed in Lua 5.4 (no LUA_COMPAT_BITLIB in 5.4.9 linit.c).
package core

// Library name macros from lualib.h (used by loadedlibs[] in linit.c).
// LUA_GNAME lives in lauxlib.go (lauxlib.h).
const (
	LUA_COLIBNAME   = "coroutine" // LUA_COLIBNAME
	LUA_TABLIBNAME  = "table"     // LUA_TABLIBNAME
	LUA_IOLIBNAME   = "io"        // LUA_IOLIBNAME
	LUA_OSLIBNAME   = "os"        // LUA_OSLIBNAME
	LUA_STRLIBNAME  = "string"    // LUA_STRLIBNAME
	LUA_UTF8LIBNAME = "utf8"      // LUA_UTF8LIBNAME
	LUA_MATHLIBNAME = "math"      // LUA_MATHLIBNAME
	LUA_DBLIBNAME   = "debug"     // LUA_DBLIBNAME
	LUA_LOADLIBNAME = "package"   // LUA_LOADLIBNAME
)

/*
** these libs are loaded by lua.c and are readily available to any Lua
** program
 */
// loadedLibs is C linit.c loadedlibs[] (luaL_Reg).
var loadedLibs = []Reg{
	{LUA_GNAME, OpenBase},          // {LUA_GNAME, luaopen_base}
	{LUA_LOADLIBNAME, OpenPackage}, // {LUA_LOADLIBNAME, luaopen_package}
	{LUA_COLIBNAME, OpenCoroutine}, // {LUA_COLIBNAME, luaopen_coroutine}
	{LUA_TABLIBNAME, OpenTable},    // {LUA_TABLIBNAME, luaopen_table}
	{LUA_IOLIBNAME, OpenIO},        // {LUA_IOLIBNAME, luaopen_io}
	{LUA_OSLIBNAME, OpenOS},        // {LUA_OSLIBNAME, luaopen_os}
	{LUA_STRLIBNAME, OpenString},   // {LUA_STRLIBNAME, luaopen_string}
	{LUA_MATHLIBNAME, OpenMath},    // {LUA_MATHLIBNAME, luaopen_math}
	{LUA_UTF8LIBNAME, OpenUTF8},    // {LUA_UTF8LIBNAME, luaopen_utf8}
	{LUA_DBLIBNAME, OpenDebug},     // {LUA_DBLIBNAME, luaopen_debug}
	// C sentinel {NULL, NULL} → end of slice
}

// RequireF is luaL_requiref (lauxlib.c), placed here (see file header).
// Adds module 'modname' to package.loaded, calling 'openf' if not already
// loaded; when 'glb' is set also assigns the module to the global of the
// same name. Leaves resulting module on the top.
func (L *State) RequireF(modname string, openf GoFunc, glb bool) {
	L.GetSubTable(LUA_REGISTRYINDEX, pkLoadedTable) /* LOADED table */
	L.GetField(-1, modname)                         /* LOADED[modname] */
	if !L.ToBoolean(-1) {                           /* package not already loaded? */
		L.Pop(1) /* remove field */
		L.PushGoFunc(openf)
		L.PushString(modname)   /* argument to open function */
		L.Call(1, 1)            /* call 'openf' to open module */
		L.PushValue(-1)         /* make copy of module (call result) */
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
	/* "require" functions from 'loadedlibs' and set results to global table */
	for _, lib := range loadedLibs {
		L.RequireF(lib.Name, lib.Func, true) /* glb = 1 */
		L.Pop(1)                             /* remove lib */
	}
}

// Open opens a single standard library by name, e.g. "string".
// Returns false when the name is not a known library.
//
// FLAG: Go-only API — not present in C linit.c / luaL_openlibs.
// Uses the same RequireF+Pop path as OpenLibs for one entry.
func (L *State) Open(name string) bool {
	for _, lib := range loadedLibs {
		if lib.Name == name {
			L.RequireF(name, lib.Func, true)
			L.Pop(1)
			return true
		}
	}
	return false
}
