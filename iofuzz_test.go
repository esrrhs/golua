package lua

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The eighth fuzz, over the io library.
//
// 'liolib.go' was the one file whose coverage stayed under 55% -- 79
// functions, 28 of them never entered -- and a probe of 411 cases went
// red in three places and then, as the grid was being built, in three
// more.  Every one of them is a place where C asks the FILE a question
// and the port asked the descriptor behind it:
//
//  1. 'io.open' with a mode C's grammar refuses.  'l_checkmode' is
//     '[rwa]%+?b*' -- 'L_MODEEXT' is "b" and nothing else -- so "rt",
//     "wt", "at" and "r+t" are "invalid mode" in PUC-Lua however
//     willing every C library is to take them.  The port accepted 't'.
//
//  2. Writing to a handle that was not opened for writing.  C's
//     'cantwrite' looks at the flags 'fopen' set in the FILE and fails
//     with EBADF before a byte moves, so 'io.open(f, "rb"):write("x")'
//     answers nil, "Bad file descriptor", 9.  A Go file only fails at
//     the system call, and the write buffer swallows a short piece
//     before it ever gets there -- the port answered the handle, as if
//     the write had worked.  The same the other way round, for
//     'cantread': 'io.popen(cmd, "w"):read("a")' is not the empty
//     string, and 'io.stdin:write("z")' is not a successful write.
//
//  3. A failed write reported the wrong errno.  'g_write' threw the
//     real error away and handed 'luaL_fileresult' a made-up one, so
//     every failure answered "(no extra info)" with an errno of 0.
//
//  4. A name with a zero byte in it.  'fopen' is handed a C string, so
//     'io.open("a\0b", "w")' makes a file called "a"; the port passed
//     the whole name to Go, which answers "Invalid argument".
//
//  5. A handle that was never closed.  Nothing closed it: '__gc' never
//     ran for a file, so a handle dropped with bytes still in its
//     buffer lost them.  'runtime.SetFinalizer' does not fire for an
//     object in a cycle, and the stream pointing back at its userdata
//     made the pair one (checked on go1.27: two objects that point at
//     each other, one with a finalizer, are never collected).  The
//     pointer back is weak now, which makes the cycle a tree.
//
// All five are fixed.  What is left is the grid, which is every mode
// 'io.open' can be given, every format 'read' takes, every way a handle
// can be used the wrong way round, and every answer that is an errno.
//
// One thing it does not ask: 'f:setvbuf(mode, size)' with a negative
// size.  C hands 'size' to 'setvbuf' as a 'size_t', so -1 is SIZE_MAX
// there and what comes of it is the library's business: Apple's fails
// without setting errno -- nil, "(no extra info)", 0 -- while glibc
// does not ask for a buffer that large at all and answers true.  No
// portable answer exists, so the case is not in the grid, and what the
// port answers is success (README, the known differences).

// ioVars is what a mode can be made of: the three ways to open a file,
// the '+' that makes it both ways round, the one extension C accepts,
// the one it does not, and a letter that is none of them.
var ioVars = []byte("rwa+btx")

// ioFiles are the fixtures, and both interpreters get an identical copy
// of them in a directory of their own: a case that writes is free to
// leave whatever it likes behind, because the other side never sees it.
var ioFiles = map[string]string{
	"text.txt":  "line1\nline2\nline3\n",
	"empty.txt": "",
	"nonl.txt":  "abc",
	"crlf.txt":  "a\r\nb\r\n",
	"nul.txt":   "a\000b\n",
	"utf8.txt":  "\344\275\240\345\245\275\n",
	"nums.txt":  "1 2.5 -3 0x10 1e2 abc\n",
	"long.txt":  strings.Repeat("x", 5000) + "\n",
	"bin.dat":   ioAllBytes(),
	"mixed.txt": "12\nhello\n34\nworld\n",
}

func ioAllBytes() string {
	var b strings.Builder
	for i := 0; i < 256; i++ {
		b.WriteByte(byte(i))
	}
	return b.String()
}

// ioFileNames is the order the fixtures are walked in: the map above
// has no order of its own, and a case number has to mean one file.
var ioFileNames = []string{
	"text.txt", "empty.txt", "nonl.txt", "crlf.txt", "nul.txt",
	"utf8.txt", "nums.txt", "long.txt", "bin.dat", "mixed.txt",
}

// ioChunk is the harness.  Every case prints one line, and a case that
// raises is a line like any other rather than the end of the run.
var ioChunk = fuzzQuote + `
local function hex(s)
  if type(s) ~= "string" then return tostring(s) end
  local t = {}
  for k = 1, #s do t[k] = string.format("%02x", s:byte(k)) end
  return table.concat(t)
end

-- one call, however many values it answers
local function T(ok, ...)
  local t = {tostring(ok)}
  for k = 1, select("#", ...) do t[#t + 1] = q((select(k, ...))) end
  return table.concat(t, "|")
end

local function P(i, name, ...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = q((select(k, ...))) end
  print(i .. "\t" .. name .. "\t" .. table.concat(t, "\t"))
end

local function mk(name, data)
  local f, e = io.open(name, "wb")
  if not f then return e end
  f:write(data)
  f:close()
  return true
end

local function slurp(name)
  local f, e = io.open(name, "rb")
  if not f then return nil end
  local d = f:read("a")
  f:close()
  return d
end

-- 1. a mode, a name, and what the handle that comes of it can do
local function openCase(name, mode, prepare)
  if prepare then mk(name, "line1\nline2\n") end
  local ok, f, e = pcall(io.open, name, mode)
  if not ok then return "raise", f end -- an invalid mode: C raises
  if not f then return "err", e end
  local ty = io.type(f)
  local r = T(pcall(f.read, f, "a"))
  local w = T(pcall(f.write, f, "Z"))
  local s = T(pcall(f.seek, f, "set", 0))
  local c = T(pcall(f.close, f))
  return ty, r, w, s, c
end

-- 2. one format, or several, against each of the fixtures
local function readCase(file, ...)
  local f, e = io.open(file, "rb")
  if not f then return "err", e end
  local r = T(pcall(f.read, f, ...))
  f:close()
  return r
end

-- 3. what 'write' puts in the file, byte for byte
local function writeCase(name, ...)
  local f, e = io.open(name, "wb")
  if not f then return "err", e end
  local r = T(pcall(f.write, f, ...))
  f:close()
  return r, hex(slurp(name))
end

-- 4. 'seek': from where, by how much, and what is left to read
local function seekCase(file, whence, off, pre)
  local f, e = io.open(file, "rb")
  if not f then return "err", e end
  if pre then f:read("l") end
  local s = T(pcall(f.seek, f, whence, off))
  local a = T(pcall(f.read, f, "a"))
  f:close()
  return s, a
end

-- 5. 'lines', which opens the file itself and closes it again
-- '...' does not reach into a closure: the formats are packed first, so
-- that what is iterated over is the arguments this case was given
local function linesCase(file, ...)
  local a, n, last, ok, e = table.pack(...), 0, nil
  ok, e = pcall(function()
    for l in io.lines(file, table.unpack(a, 1, a.n)) do
      n = n + 1; last = l
      if n >= 40 then break end
    end
  end)
  return tostring(ok), e, n, q(last)
end
local function flinesCase(file, ...)
  local f, e = io.open(file, "rb")
  if not f then return "err", e end
  local a, n, last, ok, e2 = table.pack(...), 0, nil
  ok, e2 = pcall(function()
    for l in f:lines(table.unpack(a, 1, a.n)) do
      n = n + 1; last = l
      if n >= 40 then break end
    end
  end)
  f:close()
  return tostring(ok), e2, n, q(last)
end

-- 6. 'setvbuf': only sizes C can be asked for (see the note above)
local function vbufCase(mode, size, has)
  local f = assert(io.open("vb.tmp", "wb"))
  local r
  if has then r = T(pcall(f.setvbuf, f, mode, size))
  else r = T(pcall(f.setvbuf, f, mode)) end
  f:write("x")
  f:close()
  return r
end

-- 7. every kind of handle, used every way it can be
local function handle(kind)
  if kind == "stdin" then return io.stdin end
  if kind == "stdout" then return io.stdout end
  if kind == "stderr" then return io.stderr end
  if kind == "r" then return assert(io.open("text.txt", "rb")) end
  if kind == "w" then return assert(io.open("h.tmp", "wb")) end
  if kind == "a" then return assert(io.open("h.tmp", "ab")) end
  if kind == "r+" then mk("h.tmp", "line1\nline2\n")
    return assert(io.open("h.tmp", "r+b")) end
  if kind == "w+" then return assert(io.open("h.tmp", "w+b")) end
  if kind == "a+" then return assert(io.open("h.tmp", "a+b")) end
  if kind == "closed" then
    local f = assert(io.open("text.txt", "rb")) f:close() return f end
  if kind == "tmp" then return assert(io.tmpfile()) end
  if kind == "popenr" then return assert(io.popen("echo hi", "r")) end
  if kind == "popenw" then return assert(io.popen("true", "w")) end
end
local function handleCase(kind, op)
  local f = handle(kind)
  local r = "?"
  if op == "read" then r = T(pcall(f.read, f))
  elseif op == "reada" then r = T(pcall(f.read, f, "a"))
  elseif op == "readl" then r = T(pcall(f.read, f, "l"))
  elseif op == "readL" then r = T(pcall(f.read, f, "L"))
  elseif op == "readn" then r = T(pcall(f.read, f, "n"))
  elseif op == "read0" then r = T(pcall(f.read, f, 0))
  elseif op == "read3" then r = T(pcall(f.read, f, 3))
  elseif op == "write" then r = T(pcall(f.write, f, "x"))
  elseif op == "seek" then r = T(pcall(f.seek, f, "set", 0))
  elseif op == "seekn" then r = T(pcall(f.seek, f))
  elseif op == "lines" then r = T(pcall(f.lines, f))
  elseif op == "flush" then r = T(pcall(f.flush, f))
  elseif op == "setvbuf" then r = T(pcall(f.setvbuf, f, "no"))
  elseif op == "close" then r = T(pcall(f.close, f))
  elseif op == "close2" then pcall(f.close, f) r = T(pcall(f.close, f))
  elseif op == "type" then r = tostring(io.type(f))
  elseif op == "tostring" then r = (tostring(f):gsub("0x%x+", "0xADDR"))
  end
  pcall(f.close, f)
  return r
end
`

func TestIOFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the io fuzz")
	}

	var b strings.Builder
	b.WriteString(ioChunk)
	var names []string
	n := 0
	emit := func(name, call string) {
		n++
		names = append(names, name)
		b.WriteString("P(" + strconv.Itoa(n) + ", " + strconv.Quote(name) +
			", " + call + ")\n")
	}

	// 1. every mode of length 0 to 3 over the seven letters, against a
	// file that exists, one that does not, and a directory, plus the
	// modes with a zero byte in them, where a C string ends.
	var modes []string
	modes = append(modes, "")
	for _, a := range ioVars {
		modes = append(modes, string(a))
	}
	for _, a := range ioVars {
		for _, c := range ioVars {
			modes = append(modes, string([]byte{a, c}))
		}
	}
	for _, a := range ioVars {
		for _, c := range ioVars {
			for _, d := range ioVars {
				modes = append(modes, string([]byte{a, c, d}))
			}
		}
	}
	for i, m := range modes {
		name := fmt.Sprintf("m%d.dat", i)
		emit("open/exists "+m, "openCase("+strconv.Quote(name)+", "+
			strconv.Quote(m)+", true)")
		emit("open/missing "+m, "openCase("+strconv.Quote(name+"x")+", "+
			strconv.Quote(m)+", false)")
		emit("open/dir "+m, `openCase("sub", `+strconv.Quote(m)+", false)")
	}
	for _, m := range []string{`r\0`, `r\0b`, `w\0`, `a\0x`, `\0`, `rb\0`,
		`r+\0`, `r\0\0`, `x\0`} {
		emit("open/nul "+m, `openCase("n1.dat", "`+m+`", true)`)
		emit("open/nul-missing "+m, `openCase("n2.dat", "`+m+`", false)`)
	}

	// 2. every format 'read' takes: each letter of the alphabet in both
	// cases, the four C asks for, their '*' spellings, and the counts
	// and the values that are not formats at all.
	formats := []string{"0", "1", "2", "3", "5", "10", "100", "5000",
		"5001", "2147483648", "1.5", `"5"`, "nil", "true", "{}", "print"}
	for c := byte('a'); c <= 'z'; c++ {
		formats = append(formats, `"`+string(c)+`"`)
	}
	for c := byte('A'); c <= 'Z'; c++ {
		formats = append(formats, `"`+string(c)+`"`)
	}
	for _, f := range []string{`"l"`, `"L"`, `"n"`, `"a"`, `"*l"`, `"*L"`,
		`"*n"`, `"*a"`, `"*x"`, `""`} {
		formats = append(formats, f)
	}
	for _, file := range ioFileNames {
		for _, f := range formats {
			emit("read/"+file+" "+f,
				"readCase("+strconv.Quote(file)+", "+f+")")
		}
	}

	// 3. the same, three formats at a time, which is where a read that
	// fails part of the way through shows what it leaves behind.
	combo := []string{`"l"`, `"L"`, `"a"`, `"n"`, `"*l"`, "0", "3"}
	for _, a := range combo {
		for _, c := range combo {
			emit("read2/mixed.txt "+a+" "+c, `readCase("mixed.txt", `+a+", "+c+")")
			for _, d := range combo {
				emit("read3/nums.txt "+a+" "+c+" "+d,
					`readCase("nums.txt", `+a+", "+c+", "+d+")")
			}
		}
	}

	// 4. what 'write' does with each kind of value, and with two of them
	writes := []string{`""`, `"x"`, `"a\0b"`, `"a\r\nb"`, "0", "-0.0",
		"1", "-1", "1.5", "1e300", "math.maxinteger", "math.mininteger",
		"2.0^53", "0/0", "1/0", "-1/0", "12", "1e15",
		`"\228\189\160\229\165\189"`, `string.rep("y", 5000)`,
		"nil", "true", "{}", "print"}
	for i, v := range writes {
		emit("write "+v, fmt.Sprintf(`writeCase("w%d.dat", %s)`, i, v))
	}
	for i, a := range []string{`"a"`, "1", "1.5", `"\0"`, `""`, "12"} {
		for j, c := range []string{`"b"`, "2", "-0.0", "nil", "true", "{}"} {
			emit("write2 "+a+" "+c,
				fmt.Sprintf(`writeCase("w%d_%d.dat", %s, %s)`, i, j, a, c))
		}
	}

	// 5. 'seek': from where, by how much, before and after a read
	whences := []string{`"set"`, `"cur"`, `"end"`, `"bad"`, "nil", "0",
		"true", "{}"}
	offs := []string{"0", "1", "-1", "5", "-5", "5000", "-5000",
		"2147483648", "-2147483648", "math.maxinteger", "math.mininteger",
		"1.5", "-1.5", "nil", `"3"`, "true", "{}"}
	for _, w := range whences {
		for _, o := range offs {
			emit("seek/text.txt "+w+" "+o, `seekCase("text.txt", `+w+", "+o+", false)")
			emit("seek-after/text.txt "+w+" "+o,
				`seekCase("text.txt", `+w+", "+o+", true)")
		}
	}

	// 6. 'setvbuf': the three modes, with and without a size
	for _, m := range []string{`"no"`, `"full"`, `"line"`} {
		emit("setvbuf "+m, "vbufCase("+m+", 0, false)")
		for _, s := range []string{"0", "1", "8", "4096", "65536"} {
			emit("setvbuf "+m+" "+s, "vbufCase("+m+", "+s+", true)")
		}
	}
	for _, m := range []string{"nil", `"full2"`, `""`, "1.5", "true", "{}"} {
		emit("setvbuf/bad "+m, "vbufCase("+m+", 0, true)")
	}

	// 7. 'lines', over the fixtures and over a file that is not there
	lineFmts := []string{"nil", `"l"`, `"L"`, `"*l"`, `"*L"`, `"*a"`,
		`"*n"`, "0", "3", "1.5", `"bad"`, "true", "{}"}
	for _, file := range []string{"text.txt", "empty.txt", "nonl.txt",
		"crlf.txt", "nums.txt", "mixed.txt", "missing.txt", "sub"} {
		emit("lines/"+file, "linesCase("+strconv.Quote(file)+")")
		for _, f := range lineFmts {
			emit("lines/"+file+" "+f, "linesCase("+strconv.Quote(file)+", "+f+")")
		}
	}
	for _, file := range []string{"text.txt", "empty.txt", "crlf.txt",
		"missing.txt"} {
		emit("flines/"+file, "flinesCase("+strconv.Quote(file)+")")
		for _, f := range lineFmts {
			emit("flines/"+file+" "+f, "flinesCase("+strconv.Quote(file)+", "+f+")")
		}
	}

	// 8. every kind of handle, used every way it can be
	kinds := []string{"stdin", "stdout", "stderr", "r", "w", "a", "r+",
		"w+", "a+", "closed", "tmp", "popenr", "popenw"}
	ops := []string{"read", "reada", "readl", "readL", "readn", "read0",
		"read3", "write", "seek", "seekn", "lines", "flush", "setvbuf",
		"close", "close2", "type", "tostring"}
	for _, k := range kinds {
		for _, o := range ops {
			/* Writing to a standard file puts the text in this very
			   output, and the two sides would not put it in the same
			   place: C's stdout is a FILE with a buffer of its own,
			   and what has been printed goes out when that buffer is
			   emptied, which is not when the port writes it. */
			if (k == "stdout" || k == "stderr") &&
				(o == "write" || o == "flush") {
				continue
			}
			emit("handle/"+k+"/"+o, "handleCase("+strconv.Quote(k)+", "+
				strconv.Quote(o)+")")
		}
	}

	// 9. the default files.  Nothing here is ever allowed to write to
	// the standard output: C's is a FILE with a buffer of its own, so
	// what has been printed leaves when that buffer is emptied, and a
	// byte written straight to the descriptor would land somewhere
	// else -- the two sides would differ in the interleaving alone.
	for _, a := range []string{"nil", `"text.txt"`, `"missing.txt"`,
		"12", "true", "{}", "print", "io.stdin", "io.stdout"} {
		emit("input "+a, `pcall(function()
      local ok, e = pcall(io.input, `+a+`)
      local r = "?"
      if ok then r = T(pcall(io.read, "l")) end
      pcall(io.input, io.stdin)
      return ok, e, r
    end)`)
	}
	for _, a := range []string{"nil", `"o2.tmp"`, `"missing.txt"`,
		"12", "true", "{}", "print", "io.stdin", "io.stdout"} {
		emit("output "+a, `pcall(function()
      mk("o.tmp", "")
      io.output("o.tmp")
      local ok, e = pcall(io.output, `+a+`)
      local r = "?"
      if ok and io.output() ~= io.stdout then
        pcall(io.write, "a", 1, "\n")
        r = hex(slurp("o.tmp"))
      end
      pcall(io.output, io.stdout)
      return ok, e, r
    end)`)
	}

	// 10. the errors, and the two ways a handle closes by itself
	emit("open/noargs", `pcall(io.open)`)
	emit("open/nil", `pcall(io.open, nil, "r")`)
	emit("open/table", `pcall(io.open, {}, "r")`)
	emit("type/noargs", `pcall(io.type)`)
	emit("type/12", `tostring(io.type(12))`)
	emit("type/closed", `pcall(function()
    local f = assert(io.open("text.txt", "rb")) f:close() return io.type(f)
  end)`)
	emit("close/12", `pcall(io.close, 12)`)
	emit("close/none", `pcall(io.close)`)
	/* 251 formats, which is one more than 'io.lines' takes.  They are
	   unpacked rather than written out: three hundred arguments are
	   more registers than the compiler will give one call, so the
	   limit would be the compiler's and not the library's. */
	emit("lines/toomany", `pcall(function()
    local t = {}
    for i = 1, 251 do t[i] = "l" end
    return io.lines("text.txt", table.unpack(t))
  end)`)
	emit("write/middle-bad", `pcall(function()
    local f = assert(io.open("e.tmp", "wb"))
    local r = T(pcall(f.write, f, "a", {}, "b"))
    f:close()
    return r, hex(slurp("e.tmp"))
  end)`)
	emit("read/closed-default", `pcall(function()
    io.input("text.txt") io.input():close()
    local r = T(pcall(io.read, "l"))
    io.input(io.stdin)
    return r
  end)`)
	emit("toclose/write", `pcall(function()
    do local f <close> = assert(io.open("tc.tmp", "wb")) f:write("hi") end
    return hex(slurp("tc.tmp"))
  end)`)
	emit("toclose/closed", `pcall(function()
    local f = assert(io.open("tc.tmp", "rb")) f:close()
    do local g <close> = f end
  end)`)
	emit("toclose/error", `pcall(function()
    do local f <close> = assert(io.open("tc.tmp", "wb"))
      f:write("hi") error("boom") end
  end)`)
	emit("toclose/notfile", `pcall(function() do local f <close> = 12 end end)`)
	emit("gcclose/write", `pcall(function()
    do local f = assert(io.open("gc.tmp", "wb")) f:write("hi") end
    collectgarbage() collectgarbage()
    return hex(slurp("gc.tmp"))
  end)`)
	emit("gcclose/read", `pcall(function()
    do local f = assert(io.open("text.txt", "rb")) f:read("l") end
    collectgarbage() collectgarbage()
    return "ok"
  end)`)

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	/* Two directories, one for each interpreter: a case is free to
	   create, truncate and leave behind whatever it likes, and the
	   other side still starts from exactly the same files. */
	base := t.TempDir()
	dirs := map[string]string{
		"ref":  filepath.Join(base, "ref"),
		"ours": filepath.Join(base, "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(d, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range ioFileNames {
			if err := os.WriteFile(filepath.Join(d, name),
				[]byte(ioFiles[name]), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(d, "iofuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := fmtRunFile(t, ref, dirs["ref"], "iofuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dirs["ours"], "iofuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the io chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d modes x 3 names, %d formats x %d files, %d kinds of "+
			"handle x %d ways to use one, and the errors: %d cases, all "+
			"of them PUC-Lua's", len(modes), len(formats), len(ioFileNames),
			len(kinds), len(ops), cases)
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
