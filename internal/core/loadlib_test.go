package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newPkgState returns a state with only the package library opened. The
// base library is not part of this port yet, so the chunks below must
// not use 'assert', 'print', 'type' and friends.
func newPkgState(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenPackage(L)
	return L
}

// globalString returns the value of a global as a string.
func globalString(t *testing.T, L *State, name string) string {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	s, _ := L.ToString(-1)
	return s
}

func TestPackageFields(t *testing.T) {
	L := newPkgState(t)

	/* the 'package' table and all its fields must be there */
	L.GetGlobal("package")
	defer L.Pop(1)
	if !L.IsTable(-1) {
		t.Fatal("global 'package' is not a table")
	}
	for _, field := range []string{"loaded", "preload", "path", "cpath", "searchers", "config"} {
		L.GetField(-1, field)
		if L.IsNil(-1) {
			t.Fatalf("package.%s is missing", field)
		}
		L.Pop(1)
	}
	/* package.config and package.path have the documented contents */
	L.GetField(-1, "config")
	cfg, _ := L.ToString(-1)
	L.Pop(1)
	/* the first line is LUA_DIRSEP, which is "\\" on Windows */
	if cfg != pkDirSep+"\n;\n?\n!\n-\n" {
		t.Fatalf("package.config = %q", cfg)
	}
	L.GetField(-1, "path")
	path, _ := L.ToString(-1)
	L.Pop(1)
	if !strings.Contains(path, "./?.lua") {
		t.Fatalf("package.path = %q", path)
	}
	/* 'require' is a global function */
	L.GetGlobal("require")
	if !L.IsFunction(-1) {
		t.Fatal("global 'require' is not a function")
	}
	L.Pop(1)
	/* package.loaded is the table in the registry */
	L.GetField(-1, "loaded")
	L.GetField(LUA_REGISTRYINDEX, pkLoadedTable)
	if !L.RawEqual(-1, -2) {
		t.Fatal("package.loaded is not registry._LOADED")
	}
	L.Pop(2)
}

func TestPackageSearchPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "found.lua"), []byte("return {}"), 0666); err != nil {
		t.Fatal(err)
	}
	L := newPkgState(t)

	/* a file that exists */
	src := "p = package.searchpath('found', " + quote(dir+"/?.lua") + ")"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	/* the path handed to searchpath is used as it stands: it is the
	   '.' in a module name that becomes LUA_DIRSEP, never the '/' that
	   is already there.  So the answer keeps the separator the caller
	   wrote, whatever the platform's is. */
	if got, want := globalString(t, L, "p"), dir+"/found.lua"; got != want {
		t.Fatalf("package.searchpath = %q, want %q", got, want)
	}

	/* a file that does not exist: nil + message */
	src = "p, msg = package.searchpath('missing', " + quote(dir+"/?.lua;") + ")"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("p")
	if !L.IsNil(-1) {
		t.Fatal("package.searchpath of a missing file is not nil")
	}
	L.Pop(1)
	if got := globalString(t, L, "msg"); !strings.Contains(got, "no file '") {
		t.Fatalf("error message = %q", got)
	}
}

func TestPackageRequire(t *testing.T) {
	dir := t.TempDir()
	/* a module that returns a table */
	if err := os.WriteFile(filepath.Join(dir, "mymod.lua"),
		[]byte("local M = {}\nM.v = 42\nreturn M\n"), 0666); err != nil {
		t.Fatal(err)
	}
	/* a submodule: the dot becomes the directory separator */
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0777); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "mod.lua"),
		[]byte("return 7\n"), 0666); err != nil {
		t.Fatal(err)
	}
	/* a module that returns nothing */
	if err := os.WriteFile(filepath.Join(dir, "novalue.lua"),
		[]byte("local x = 1\n"), 0666); err != nil {
		t.Fatal(err)
	}

	L := newPkgState(t)
	src := "package.path = " + quote(dir+"/?.lua;"+dir+"/?/init.lua") + "\n" +
		"a = require('mymod')\n" +
		"av = a.v\n" +
		"b = require('mymod')\n" +
		"same = a == b\n" +
		"c = require('sub.mod')\n" +
		"d = require('novalue')\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := globalString(t, L, "av"); got != "42" {
		t.Fatalf("require('mymod').v = %q, want 42", got)
	}
	L.GetGlobal("same")
	if !L.ToBoolean(-1) {
		t.Fatal("require returned a different value the second time")
	}
	L.Pop(1)
	if got := globalString(t, L, "c"); got != "7" {
		t.Fatalf("require('sub.mod') = %q, want 7", got)
	}
	L.GetGlobal("d")
	if !L.ToBoolean(-1) {
		t.Fatal("require of a module without a return value is not true")
	}
	L.Pop(1)

	/* the module is cached in package.loaded */
	L.GetGlobal("package")
	L.GetField(-1, "loaded")
	L.GetField(-1, "mymod")
	if L.IsNil(-1) {
		t.Fatal("package.loaded.mymod is nil")
	}
	L.Pop(3)
}

func TestPackageRequireNotFound(t *testing.T) {
	dir := t.TempDir()
	L := newPkgState(t)
	src := "package.path = " + quote(dir+"/?.lua") + "\nrequire('nosuchmodule')"
	err := L.DoString(src)
	if err == nil {
		t.Fatal("require of a missing module did not raise an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "module 'nosuchmodule' not found") {
		t.Fatalf("error message = %q", msg)
	}
	/* the messages of the searchers are part of the report: the
	   preload searcher and the Lua searcher (the C searcher only
	   reports pkDLMsg when a matching file exists) */
	if !strings.Contains(msg, "no field package.preload['nosuchmodule']") ||
		!strings.Contains(msg, "no file '"+dir+"/nosuchmodule.lua'") {
		t.Fatalf("error message = %q", msg)
	}
}

func TestPackageRequirePreload(t *testing.T) {
	L := newPkgState(t)
	/* package.preload is looked up in the registry */
	src := "package.preload['pre'] = function (name) return {n = name} end\n" +
		"m = require('pre')\n" +
		"mn = m.n\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	if got := globalString(t, L, "mn"); got != "pre" {
		t.Fatalf("preloaded module got name %q, want pre", got)
	}
}

func TestPackageLoadLib(t *testing.T) {
	L := newPkgState(t)
	src := "a, b, c = package.loadlib('nosuch.so', 'luaopen_nosuch')"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("a")
	if !L.IsNil(-1) {
		t.Fatal("package.loadlib did not return nil")
	}
	L.Pop(1)
	msg := globalString(t, L, "b")
	where := globalString(t, L, "c")
	/* Two builds, two answers.  With the cgo backend the message is
	   dlerror()'s own text and LIB_FAIL is "open", exactly as in a
	   PUC-Lua built with LUA_USE_DLOPEN; without it there is no dlopen
	   at all and both answers say so. */
	if pkHasDynLoad() {
		if !strings.Contains(msg, "nosuch.so") {
			t.Fatalf("package.loadlib message = %q, want dlerror text naming the library", msg)
		}
		if where != "open" {
			t.Fatalf("package.loadlib 'where' = %q, want %q", where, "open")
		}
	} else {
		if !strings.Contains(msg, pkDLMsg) {
			t.Fatalf("package.loadlib message = %q, want it to name %q", msg, pkDLMsg)
		}
		if where != pkLibFail {
			t.Fatalf("package.loadlib 'where' = %q, want %q", where, pkLibFail)
		}
	}
}

func TestPackageRequirePackage(t *testing.T) {
	L := newPkgState(t)
	/* the package library registers itself in package.loaded */
	src := "p = require('package')\nq = p == package\n"
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v", err)
	}
	L.GetGlobal("q")
	if !L.ToBoolean(-1) {
		t.Fatal("require('package') is not the package table")
	}
	L.Pop(1)
}

func TestPackageSetPath(t *testing.T) {
	/* ';;' in the environment means "put the default path here" */
	t.Setenv(pkPathVar+pkVerSuffix, "a"+pkPathSep+pkPathSep+"b")
	L := newPkgState(t)
	if got := pkgFieldString(t, L, "path"); got != "a"+pkPathSep+pkPathDefault+pkPathSep+"b" {
		t.Fatalf("package.path = %q", got)
	}

	/* a plain path is used as is */
	t.Setenv(pkPathVar+pkVerSuffix, "a"+pkPathSep+"b")
	L = newPkgState(t)
	if got := pkgFieldString(t, L, "path"); got != "a"+pkPathSep+"b" {
		t.Fatalf("package.path = %q", got)
	}

	/* the unversioned name is used when the versioned one is absent */
	t.Setenv(pkPathVar+pkVerSuffix, "")
	t.Setenv(pkCPathVar, "onlyc")
	L = newPkgState(t)
	if got := pkgFieldString(t, L, "cpath"); got != "onlyc" {
		t.Fatalf("package.cpath = %q", got)
	}
}

// pkgFieldString returns package[name] as a string.
func pkgFieldString(t *testing.T, L *State, name string) string {
	t.Helper()
	L.GetGlobal("package")
	defer L.Pop(1)
	L.GetField(-1, name)
	defer L.Pop(1)
	s, _ := L.ToString(-1)
	return s
}

// quote returns s as a Lua long string, so that any character (including
// a backslash) is safe inside a chunk.
func quote(s string) string {
	return "[[" + s + "]]"
}
