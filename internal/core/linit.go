// Translation of linit.c (PUC-Lua 5.3.4): opening the standard libraries.
//
// The only adaptation is that 'luaL_requiref' is spelled out here as
// (*State).RequireF, because lauxlib.c is translated without it.
package core

// loadedLibs is the 'loadedlibs' array of linit.c: (name, open function).
var loadedLibs = []struct {
	name string
	open func(*State) int
}{
	{"_G", OpenBase},
	{"package", OpenPackage},
	{"coroutine", OpenCoroutine},
	{"table", OpenTable},
	{"io", OpenIO},
	{"os", OpenOS},
	{"string", OpenString},
	{"math", OpenMath},
	{"utf8", OpenUTF8},
	{"debug", OpenDebug},
	{"bit32", OpenBit32},
}

// RequireF adds the module 'modname' to package.loaded, calling 'openf'
// to build it if it is not there yet; when 'glb' is set it also assigns
// the module to the global of the same name (luaL_requiref).
func (L *State) RequireF(modname string, openf GoFunc, glb bool) {
	L.GetSubTable(LUA_REGISTRYINDEX, pkLoadedTable) /* LOADED table */
	L.GetField(-1, modname)                         /* LOADED[modname] */
	if !L.ToBoolean(-1) {                           /* not already loaded? */
		L.Pop(1) /* remove field */
		L.PushGoClosure(openf, 0)
		L.PushString(modname) /* argument to open function */
		L.Call(1, 1)          /* call 'openf' to open module */
		L.PushValue(-1)       /* make copy of module (call result) */
		L.SetField(-3, modname)
	}
	L.Remove(-2) /* remove LOADED table */
	if glb {
		L.PushValue(-1)      /* copy of module */
		L.SetGlobal(modname) /* _G[modname] = module */
	}
}

// OpenLibs opens all standard libraries (luaL_openlibs).
func (L *State) OpenLibs() {
	for _, lib := range loadedLibs {
		L.RequireF(lib.name, lib.open, true)
		L.Pop(1) /* remove lib */
	}
}

// Open opens a single standard library by name, e.g. "string". It
// returns false when the name is not a known library.
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
