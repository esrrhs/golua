package core

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// The eleventh fuzz, over every name a script can hand to the OS.
//
// A C string ends at its first zero byte, and nearly every one of these
// names is a C string by the time it reaches anything: 'fopen', 'open',
// 'remove', 'rename', 'system' and 'popen' are C functions, and the
// strings 'require' and 'package.searchpath' build are put together with
// 'luaL_addgsub' and 'luaL_addstring', both of which are 'strlen'.  So
// 'io.open("a\0b", "w")' makes a file called "a", and 'os.remove("a\0b")'
// removes it and says so -- the name in the message is "a" and not
// "a\0b", because '%s' is 'strlen' too.
//
// A Go string has no such end, and the port used to hand the whole thing
// to the standard library, which does not truncate it: it rejects it.
// Every path through 'syscall' goes through 'BytePtrFromString', which is
// an error -- EINVAL, "Invalid argument" -- for a string with a zero in
// it, so the port answered that for every one of the names below, while
// PUC-Lua went on and did what the truncated name asked for.  A probe of
// 165 cases went red in seven of the eleven functions; 'io.open' and
// 'io.lines' were already right, the io grid having found the same thing
// in the eighth fuzz.
//
// What is left is the grid: a zero at every position of every name, over
// every function that takes one, against a file that is there, one that
// is not, and a directory.  It found two more, and neither of them has a
// zero byte in it -- which is what a grid is for, since neither was what
// it was built to ask about.
//
//  1. 'os.rename' onto a directory.  Go's 'os.Rename' asks whether the
//     name it was given is free before it calls 'rename(2)' at all, so a
//     directory it is told to move onto a directory that is there
//     answers EEXIST -- and, worse, the rename does not happen: the
//     file is still sitting under its old name.  C's 'rename' is asked
//     nothing and answers EISDIR, ENOTDIR or ENOTEMPTY.  'ioRename' in
//     'liolib.go' calls 'syscall.Rename', which is 'rename(2)'.
//
//  2. The spelling of an errno Go groups with another one.  'ioStrError'
//     asked 'errors.Is(err, fs.ErrExist)', and 'Errno.Is' answers that
//     ENOTEMPTY *is* 'fs.ErrExist', which is true of what a Go program
//     is asking and false of what 'strerror' says: a rename of one
//     directory onto another that is not empty, and a 'remove' of a
//     directory with something in it, both answered "File exists" where
//     PUC-Lua answers "Directory not empty".  The same grouping says
//     EPERM is 'fs.ErrPermission', so a permission failure would have
//     answered "Permission denied" where C says "Operation not
//     permitted".  Go spells every errno exactly as C does with the
//     first letter in lower case, so the errno's own text, raised by
//     that one letter, is what is answered now.
//
// Two things it does not do.  It does not ask 'io.popen' or 'os.execute'
// to run a command that says anything on the standard error, because C's
// standard output is a FILE with a buffer of its own and the port writes
// straight to the descriptor, so the two would differ in the interleaving
// alone: every command is run as 'exec 2>/dev/null; ...', which swallows
// the "command not found" that most of these truncated names produce.
// The exit status still tells the truncated names apart -- "" is 0, "t"
// is 127, "tr" is 1, "tru" is 127 and "true" is 0 -- which is the whole
// point of the cases.  And it does not ask 'os.rename' to move a name
// onto one the grid has not made, because a rename that succeeds is one
// the next case would otherwise see.

// nameBases are the names a zero byte is put into.  One letter, a name
// with an extension, a directory, one that is not there, and a module.
var nameBases = []string{"a", "a.txt", "sub", "no", "m.lua"}

// nameModes are the six things 'io.open' can be asked to do.
var nameModes = []string{"r", "w", "a", "r+", "w+", "a+"}

// nameCmds are the commands a zero byte is put into, and the positions
// are what 'sh' makes of them: see the note at the top.
var nameCmds = []string{"true", "false", "exit 7", "echo hi"}

// nameChunk is the harness.  Every case prints one line, and a case that
// raises is a line like any other rather than the end of the run.
var nameChunk = fuzzQuote + `
local n = 0
local function T(...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = q((select(k, ...))) end
  return table.concat(t, "\t")
end

local function P(tag, ...)
  n = n + 1
  print(n .. "\t" .. tag .. "\t" .. T(...))
end

-- the name the OS will be given: the bytes before the first zero
local function cut(s) return (s:gsub("%z.*", "")) end

-- the fixtures.  Each case has a name of its own, so a file one case
-- leaves behind is never another case's.
local function mk(name, data)
  local f = io.open(name, "wb")
  if not f then return end
  f:write(data)
  f:close()
end
-- 'mkdir' says it all over again when the directory is already there,
-- and neither side is asking about that.
local function md(name) os.execute("mkdir " .. name .. " 2>/dev/null") end

local function prep(nm, what)
  if what == "file" then mk(nm, "line1\nline2\n")
  elseif what == "dir" then md(nm)
  elseif what == "good" then mk(nm, "return 1, 'x'\n")
  elseif what == "bad" then mk(nm, "this is not lua\n")
  elseif what == "empty" then mk(nm, "")
  elseif what == "lua" then mk(nm .. ".lua", "return 'm'\n")
  elseif what == "init" then md(nm) mk(nm .. "/init.lua", "return 'i'\n")
  end
end

-- 1. 'io.open': the handle, one line read from it, and the close
local function openCase(full, mode, what)
  prep(cut(full), what)
  local ok, f, e = pcall(io.open, full, mode)
  if not ok then return "raise", f end
  if not f then return "nil", e end
  local ty = tostring(io.type(f))
  local r = T(pcall(f.read, f, "l"))
  local c = T(f.close(f))
  return ty, r, c
end

-- 2. 'io.lines', which opens the file itself
local function linesCase(full, what, fmt)
  prep(cut(full), what)
  local cnt, last = 0, nil
  local ok, e = pcall(function()
    local it
    if fmt then it = io.lines(full, fmt) else it = io.lines(full) end
    for l in it do
      cnt = cnt + 1; last = l
      if cnt > 20 then break end
    end
  end)
  return tostring(ok), q(e), cnt, q(last)
end

-- 3. 'loadfile': whether it loads, and what the chunk answers
local function loadCase(full, what)
  prep(cut(full), what)
  local ok, f, e = pcall(loadfile, full)
  if not ok then return "raise", f end
  if not f then return "no", e end
  return "yes", T(pcall(f))
end

-- 4. 'dofile', which is 'loadfile' and then the chunk
local function doCase(full, what)
  prep(cut(full), what)
  return T(pcall(dofile, full))
end

-- 5. 'os.remove'
local function removeCase(full, what)
  prep(cut(full), what)
  return T(os.remove(full))
end

-- 6. 'os.rename': the zero in the name that is moved, in the name it is
-- moved to, or in both
local function renFrom(full, what)
  prep(cut(full), what)
  return T(os.rename(full, cut(full) .. "Z"))
end
local function renTo(full, what)
  prep(cut(full), what)
  return T(os.rename(cut(full), "g" .. full))
end
local function renBoth(full, what)
  prep(cut(full), what)
  return T(os.rename(full, "g" .. full))
end
-- and a rename of two names neither of which has a zero in it.  The
-- counter keeps every case's pair to itself.
local function renPlain(i, what)
  local a, b = "r" .. i .. "a", "r" .. i .. "b"
  if what == "file" then mk(a, "x")
  elseif what == "f2f" then mk(a, "x") mk(b, "y")
  elseif what == "f2d" then mk(a, "x") md(b)
  elseif what == "d2f" then md(a) mk(b, "y")
  elseif what == "d2e" then md(a) md(b)
  elseif what == "d2n" then md(a) md(b) mk(b .. "/in", "x")
  elseif what == "n2e" then md(a) mk(a .. "/in", "x") md(b)
  elseif what == "d2s" then md(a) mk(b, "y")
  end
  return T(os.rename(a, b)), a .. " is " .. kind(a), b .. " is " .. kind(b)
end
-- what a name is now, which is the other half of what a rename did.
-- 'io.open' cannot say whether one is a directory: it opens a directory
-- for reading, which is what 'fopen' does on its own.  Opening 'name/.'
-- can, and reading it is not asked for.
function md0(name)
  local f = io.open(name .. "/.", "rb")
  if f then f:close() return true end
  return false
end
function kind(name)
  if md0(name) then return "dir" end
  local f = io.open(name, "rb")
  if not f then return "gone" end
  local d = f:read("a")
  f:close()
  if d == nil then return "unreadable" end
  if #d > 0 then return "file" end
  return "empty"
end

-- 7. 'require'
local function reqCase(full, what)
  prep(cut(full), what)
  if what == "preload" then
    package.preload[cut(full)] = function(nm) return nm end
  end
  return T(pcall(require, full))
end
-- and the same module twice, which is 'package.loaded'
local function reqTwice(full, what)
  prep(cut(full), what)
  if what == "preload" then
    package.preload[cut(full)] = function(nm) return "p" end
  end
  local a = T(pcall(require, full))
  local b = T(pcall(require, full))
  return a, b
end

-- 8. 'package.searchpath'
local function spCase(full, pat, what)
  prep(cut(full), what)
  return T(package.searchpath(full, pat))
end
local function spCase4(full, pat, sep, rep, what)
  prep(cut(full), what)
  return T(package.searchpath(full, pat, sep, rep))
end

-- 11. the same names with no zero in them, in the ways the groups above
-- have not asked about: a 'remove' of a directory with something in it,
-- which is ENOTEMPTY and not EEXIST, and a name with a slash on the end
-- of it, which is not a name that can be given to a directory at all
local function rmPlain(i, what)
  local a = "e" .. i .. "a"
  if what == "file" then mk(a, "x")
  elseif what == "dir" then md(a)
  elseif what == "full" then md(a) mk(a .. "/in", "x")
  end
  local r = T(os.remove(a))
  return r, a .. " is " .. (md0(a) and "dir" or "gone")
end
local function slashCase(i, what, op)
  local a = "s" .. i .. "a"
  if what == "file" then mk(a, "x") elseif what == "dir" then md(a) end
  if op == "open-r" then return T(pcall(io.open, a .. "/", "r")) end
  if op == "open-w" then return T(io.open(a .. "/", "w")) end
  if op == "remove" then return T(os.remove(a .. "/")) end
  if op == "loadfile" then
    local ok, f, e = pcall(loadfile, a .. "/")
    if ok and f then return "yes" end
    return tostring(ok), q(e)
  end
  if op == "dofile" then return T(pcall(dofile, a .. "/")) end
  if op == "lines" then
    local ok, e = pcall(function()
      for l in io.lines(a .. "/") do break end
    end)
    return tostring(ok), q(e)
  end
  if op == "rename" then return T(os.rename(a .. "/", a .. "Z")) end
  if op == "searchpath" then
    return T(package.searchpath(a .. "/", "?"))
  end
end

-- 9. 'io.popen' and 10. 'os.execute', over a command with a zero in it
local function popenCase(cmd, mode)
  local ok, f, e = pcall(io.popen, cmd, mode)
  if not ok then return "raise", f end
  if not f then return "nil", e end
  local r = "skip"
  if mode == "r" then r = T(pcall(f.read, f, "a")) end
  return r, T(f.close(f))
end
local function execCase(cmd)
  return T(os.execute(cmd))
end
`

func TestNameFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the name fuzz")
	}

	var b strings.Builder
	b.WriteString(nameChunk)
	n := 0
	seq := 0
	var names []string
	emit := func(tag, call string) {
		n++
		names = append(names, tag)
		b.WriteString("P(" + strconv.Quote(tag) + ", " + call + ")\n")
	}
	/* The name for one case: the counter keeps it to itself, and the
	   zero goes in at position p of the base -- "f12_a\000.txt", which
	   the C side reads as "f12_a".  \0 takes as many digits as it is
	   given, so it is always written \000: "\01" followed by the byte
	   '2' would read back as one character, \012. */
	nm := func(base string, p int) string {
		seq++
		return `"f` + strconv.Itoa(seq) + "_" + base[:p] + `\000` + base[p:] + `"`
	}
	/* A command, which is a name of a kind: the prefix is what keeps a
	   truncated command from saying anything on the standard error. */
	cmd := func(base string, p int) string {
		seq++
		return `"exec 2>/dev/null; ` + base[:p] + `\000` + base[p:] + `"`
	}
	pos := func(base string, p int) string { return base + "+" + strconv.Itoa(p) }

	// 1. 'io.open': every mode, over a file, a directory and nothing
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, m := range nameModes {
				for _, what := range []string{"file", "dir", "none"} {
					emit("open/"+m+"/"+pos(base, p)+"/"+what,
						"openCase("+nm(base, p)+", "+
							strconv.Quote(m)+", "+strconv.Quote(what)+")")
				}
			}
		}
	}

	// 2. 'io.lines', and the same with a format
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, what := range []string{"file", "dir", "none"} {
				emit("lines/"+pos(base, p)+"/"+what,
					"linesCase("+nm(base, p)+", "+
						strconv.Quote(what)+", nil)")
			}
			emit("lines-l/"+pos(base, p), "linesCase("+nm(base, p)+
				`, "file", "l")`)
			emit("lines-a/"+pos(base, p), "linesCase("+nm(base, p)+
				`, "file", "a")`)
		}
	}

	// 3. and 4. 'loadfile' and 'dofile': a chunk that loads, one that
	// does not, an empty one, and a directory
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, what := range []string{"good", "bad", "empty",
				"dir", "none"} {
				emit("loadfile/"+pos(base, p)+"/"+what,
					"loadCase("+nm(base, p)+", "+strconv.Quote(what)+")")
				emit("dofile/"+pos(base, p)+"/"+what,
					"doCase("+nm(base, p)+", "+strconv.Quote(what)+")")
			}
		}
	}

	// 5. 'os.remove'
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, what := range []string{"file", "dir", "none"} {
				emit("remove/"+pos(base, p)+"/"+what,
					"removeCase("+nm(base, p)+", "+strconv.Quote(what)+")")
			}
		}
	}

	// 6. 'os.rename': the zero in the first name, in the second, in
	// both, and then in neither
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, what := range []string{"file", "dir", "none"} {
				emit("rename-from/"+pos(base, p)+"/"+what,
					"renFrom("+nm(base, p)+", "+strconv.Quote(what)+")")
			}
			for _, what := range []string{"file", "none"} {
				emit("rename-to/"+pos(base, p)+"/"+what,
					"renTo("+nm(base, p)+", "+strconv.Quote(what)+")")
			}
			emit("rename-both/"+pos(base, p),
				"renBoth("+nm(base, p)+", "+strconv.Quote("file")+")")
		}
	}
	for i, what := range []string{"none", "file", "f2f", "f2d", "d2f",
		"d2e", "d2n", "n2e", "d2s"} {
		emit("rename-plain/"+what,
			"renPlain("+strconv.Itoa(i)+", "+strconv.Quote(what)+")")
	}
	/* And the four a rename of a directory is, twice: a Go 'os.Rename'
	   answers one of them for itself.  It asks whether the name it was
	   given is free before it calls 'rename' at all, so a directory it
	   is told to move onto a directory that is there answers EEXIST,
	   and the rename does not happen.  C's 'rename' is asked nothing
	   and answers EISDIR, ENOTDIR or ENOTEMPTY. */
	for i, what := range []string{"f2d", "d2f", "d2e", "d2n", "n2e"} {
		emit("rename-plain/again-"+what,
			"renPlain("+strconv.Itoa(100+i)+", "+strconv.Quote(what)+")")
	}

	/* 11b. A 'remove' of a directory with something in it, which is
	   ENOTEMPTY -- "Directory not empty", and not the EEXIST a Go
	   'errors.Is' would group it with. */
	for i, what := range []string{"none", "file", "dir", "full"} {
		emit("remove-plain/"+what,
			"rmPlain("+strconv.Itoa(i)+", "+strconv.Quote(what)+")")
	}
	for i := 0; i < 4; i++ {
		emit("remove-plain/again-"+strconv.Itoa(i),
			"rmPlain("+strconv.Itoa(100+i)+", "+
				strconv.Quote([]string{"none", "file", "dir", "full"}[i])+")")
	}
	// and a name with a slash on the end of it
	{
		i := 0
		for _, what := range []string{"file", "dir", "none"} {
			for _, op := range []string{"open-r", "open-w", "remove",
				"loadfile", "dofile", "lines", "rename", "searchpath"} {
				emit("slash/"+what+"/"+op, "slashCase("+strconv.Itoa(i)+
					", "+strconv.Quote(what)+", "+strconv.Quote(op)+")")
				i++
			}
		}
	}

	// 7. 'require': a module that is there as a file, one that is there
	// as a directory, one that is not there at all, and one that only
	// 'package.preload' has
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, what := range []string{"lua", "init", "none",
				"preload"} {
				emit("require/"+pos(base, p)+"/"+what,
					"reqCase("+nm(base, p)+", "+strconv.Quote(what)+")")
			}
		}
	}
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			emit("require-twice/"+pos(base, p),
				"reqTwice("+nm(base, p)+", "+strconv.Quote("lua")+")")
		}
	}

	// 8. 'package.searchpath', over two paths
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			for _, pat := range []string{"?.lua", "?.lua;?/init.lua"} {
				for _, what := range []string{"lua", "none", "init"} {
					emit("searchpath/"+pos(base, p)+"/"+pat+"/"+what,
						"spCase("+nm(base, p)+", "+strconv.Quote(pat)+
							", "+strconv.Quote(what)+")")
				}
			}
		}
	}
	for _, base := range nameBases {
		for p := 0; p <= len(base); p++ {
			emit("searchpath-abs/"+pos(base, p), "spCase("+nm(base, p)+
				`, "./?.lua", "lua")`)
			for _, a := range []struct{ sep, rep string }{
				{".", "/"}, {".", "\\"}, {"", "/"}, {"-", "_"},
			} {
				emit("searchpath-sep/"+pos(base, p)+"/"+a.sep+"/"+a.rep,
					"spCase4("+nm(base, p)+`, "?.lua", `+
						strconv.Quote(a.sep)+", "+
						strconv.Quote(a.rep)+`, "none")`)
			}
		}
	}

	// 9. 'io.popen' and 10. 'os.execute'.  'echo hi' is left out of the
	// last two: its standard output is this one's, and the two sides
	// would not say it in the same place.
	for _, base := range nameCmds {
		for p := 0; p <= len(base); p++ {
			emit("popen-r/"+pos(base, p),
				"popenCase("+cmd(base, p)+`, "r")`)
			if base != "echo hi" {
				emit("popen-w/"+pos(base, p),
					"popenCase("+cmd(base, p)+`, "w")`)
				emit("execute/"+pos(base, p), "execCase("+cmd(base, p)+")")
			}
		}
	}
	emit("popen-r/empty", `popenCase("exec 2>/dev/null; ", "r")`)
	emit("execute/empty", `execCase("exec 2>/dev/null; ")`)

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	/* Two directories, one for each interpreter: a case is free to
	   create, remove, truncate and leave behind whatever it likes, and
	   the other side still starts from exactly the same empty one. */
	base := t.TempDir()
	dirs := map[string]string{
		"ref":  filepath.Join(base, "ref"),
		"ours": filepath.Join(base, "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "namefuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := fmtRunFile(t, ref, dirs["ref"], "namefuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dirs["ours"], "namefuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the name chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d names of %d bases, %d modes, %d commands: %d cases, "+
			"all of them PUC-Lua's",
			seq, len(nameBases), len(nameModes), len(nameCmds), cases)
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	reported := 0
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var a, c string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			c = gl[i]
		}
		if a == c {
			continue
		}
		reported++
		if reported > 20 {
			t.Logf("... and more")
			break
		}
		what := "?"
		if i < len(names) {
			what = names[i]
		}
		t.Errorf("line %d, %s\n  PUC-Lua: %s\n  this:    %s", i+1, what, a, c)
	}
	if reported == 0 {
		t.Errorf("%d cases: the lines differ but none of them line up", cases)
	}
}

// TestNameInProcess is the eleven functions, driven in this process.
//
// The grid runs them in a subprocess, and a subprocess does not answer
// 'go test -cover' -- which is how 'osRename' and the 'ioRename' behind
// it were found sitting at 0%, along with 'ioStrError' at 54%.  What
// each of them answers is PUC-Lua's and the grid is what says so; this
// is only that they are entered at all, plus the two things the grid
// found, pinned where no reference interpreter is needed to see them.
func TestNameInProcess(t *testing.T) {
	/* What it pins is what a 'rename(2)' answers and what a Go
	   'os.Rename' answers for itself, and neither is the same on
	   Windows: a move there is MoveFileEx, the errnos are not the
	   C ones, and the grid that says which is right runs where the
	   reference interpreter is (§5.13), which is not here. */
	if ioIsWindows() {
		t.Skip("the answer a rename of a directory gets is not the " +
			"same on Windows, and the grid that says which one " +
			"is PUC-Lua's runs on unix")
	}
	dir := t.TempDir()
	/* The fixtures.  A directory needs 'mkdir', which is a shell on
	   some platforms and not on others, so they are made here and not
	   in the chunk -- and the chunk runs with this as its working
	   directory, which is the only place './?.lua' can look. */
	for _, d := range []string{"d1", "d2", "d3", "d6"} {
		if err := os.Mkdir(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	/* 'd3' and 'd6' are the two with something in them: a rename onto
	   a directory that is not empty is ENOTEMPTY, which is the second
	   of the two things the grid found. */
	for _, d := range []string{"d3", "d6"} {
		if err := os.WriteFile(filepath.Join(dir, d, "in"),
			[]byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"a", "c"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"),
			0o644); err != nil {
			t.Fatal(err)
		}
	}
	/* and a module that is there, so that 'require' is asked about one
	   that is as well as about one that is not */
	if err := os.WriteFile(filepath.Join(dir, "k.lua"),
		[]byte("return 7\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	/* The errno the platform spells a thing with, which is what the
	   reference answers too: ENOTEMPTY is 66 on macOS and 39 on
	   Linux.  The message is what is being pinned, and that is
	   written out. */
	eno := func(e syscall.Errno) string { return strconv.Itoa(int(e)) }
	var b strings.Builder
	b.WriteString(`local function T(...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = tostring((select(k, ...))) end
  return table.concat(t, "|")
end
local function mk(n, d)
  local f = assert(io.open(n, "wb")) f:write(d or "x") f:close()
end

-- 1. the two things the grid found, neither of which has a zero byte
--    in it: 'os.rename' onto a directory, and the spelling of an errno
--    that Go groups with another one.  The numbers come from the
--    platform and not from the source -- ENOTEMPTY is 66 on macOS and
--    39 on Linux -- and it is the message that is the answer.
mk("f1")
assert(T(os.rename("f1", "d1")) ==
  "nil|Is a directory|` + eno(syscall.EISDIR) + `")
mk("f2")
assert(T(os.rename("f2", "d2")) ==
  "nil|Is a directory|` + eno(syscall.EISDIR) + `")
assert(T(os.rename("d3", "d1")) == "true")
assert(T(os.rename("d6", "d1")) ==
  "nil|Directory not empty|` + eno(syscall.ENOTEMPTY) + `")
mk("g1")
assert(T(os.rename("d6", "g1")) ==
  "nil|Not a directory|` + eno(syscall.ENOTDIR) + `")
assert(T(os.remove("d2")) == "true")
assert(T(os.remove("d6")) ==
  "nil|d6: Directory not empty|` + eno(syscall.ENOTEMPTY) + `")

-- 2. the family the two belong to: a name with a zero at every kind of
--    position in it, handed to each of the eleven functions
assert(T(os.remove("a\0b")) == "true")
assert(T(io.open("a", "rb")) == "nil|a: No such file or directory|2")
assert(T(os.rename("c\0d", "ce")) == "true")
assert(io.open("ce", "rb") ~= nil)
local f = assert(io.open("q\0r", "wb")) f:write("l1\nl2\n") f:close()
assert(io.open("q", "rb") ~= nil)
for l in io.lines("q\0r") do assert(l == "l1" or l == "l2") end
assert(T(pcall(io.lines, "m\0n")) ==
  "false|cannot open file 'm' (No such file or directory)")
assert(T(loadfile("z\0z")) == "nil|cannot open z: No such file or directory")
assert(T(pcall(dofile, "z\0z")) ==
  "false|cannot open z: No such file or directory")
assert(T(package.searchpath("sp\0q", "?.lua")) == "nil|\n\tno file 'sp.lua'")
-- and the other way ioStrError is reached: EBADF, which a FILE
-- answers on its own and which has no errno behind it to spell
local ro = assert(io.open("q", "rb"))
assert(T(ro.write(ro, "x")) == "nil|Bad file descriptor|9")
local _, e = pcall(require, "zz\0qq")
assert(e:find("module 'zz' not found", 1, true) == 1, e)
assert(e:find("no field package.preload['zz']", 1, true), e)
assert(e:find("no file './zz.lua'", 1, true), e)
assert(T(pcall(require, "k")) == "true|7")
`)
	b.WriteString(`-- 3. the two that are a shell.  'exec 2>/dev/null' is what keeps a
--    truncated command from saying anything on the standard error.
local p = assert(io.popen("exec 2>/dev/null; tru\0e", "r"))
assert(T(p.close(p)) == "nil|exit|127")
assert(T(os.execute("exec 2>/dev/null; tru\0e")) == "nil|exit|127")
assert(T(os.execute("exec 2>/dev/null; true\0e")) == "true|exit|0")
return true
`)
	if err := runLua(t, L, b.String()); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("the name functions do not answer what PUC-Lua answers")
	}
}
