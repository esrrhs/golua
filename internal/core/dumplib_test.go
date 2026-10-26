package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

// dumpNew creates a state with the base and the string libraries opened
// ('string.dump' lives in the string library).
func dumpNew(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenBase(L)
	L.Pop(1)
	OpenString(L)
	L.PushValue(-1)
	L.SetGlobal("string")
	L.Pop(1)
	return L
}

// dumpRuns runs a chunk that must fail, returning its message.
func dumpRuns(t *testing.T, src string) string {
	t.Helper()
	L := dumpNew(t)
	err := L.DoString(src)
	if err == nil {
		t.Fatalf("expected an error from:\n%s", src)
	}
	return err.Error()
}

// ---------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------

func TestDumpSignature(t *testing.T) {
	L := dumpNew(t)
	/* the dump starts with the LUA_SIGNATURE ("\x1bLua") */
	baseCheck(t, L, `
		local bin = string.dump(function() return 1 end)
		r = tostring(bin:sub(1, 4) == "\27Lua")
		r2 = tostring(#bin > 0)`, "r", "true")
	baseCheck(t, L, `
		local bin = string.dump(function() return 1 end)
		r2 = tostring(bin:byte(1) == 27 and bin:byte(2) == 76 and
		              bin:byte(3) == 117 and bin:byte(4) == 97)`, "r2", "true")
}

func TestDumpRoundTrip(t *testing.T) {
	L := dumpNew(t)
	/* a plain function */
	baseCheck(t, L, `
		local bin = string.dump(function(a, b) return a * b end)
		r = tostring(load(bin)(6, 7))`, "r", "42")
	/* constants of every kind, in particular long strings */
	baseCheck(t, L, `
		local bin = string.dump(function()
			return 1, 2.5, "short", string.rep("long", 100), true, false, nil
		end)
		local a, b, c, d, e, f = load(bin)()
		r = tostring(a) .. "|" .. tostring(b) .. "|" .. tostring(c) .. "|" ..
		    tostring(#d) .. "|" .. tostring(e) .. "|" .. tostring(f)`,
		"r", "1|2.5|short|400|true|false")
	/* nested prototypes, tables, loops and upvalues local to the chunk */
	baseCheck(t, L, `
		local function mk()
			local t = {"x", "y"}
			return function(n)
				local s = ""
				for i = 1, n do s = s .. t[((i - 1) % 2) + 1] end
				return s
			end
		end
		local mk2 = load(string.dump(mk))
		r = tostring(mk2()(4)) .. "|" .. tostring(mk()(4))`, "r", "xyxy|xyxy")
	/* vararg functions */
	baseCheck(t, L, `
		local bin = string.dump(function(...) return select("#", ...) end)
		r = tostring(load(bin)(1, 2, 3))`, "r", "3")
}

// TestDumpStrip checks the 'strip' argument of string.dump: the result
// is smaller (no debug information) but still loadable.
func TestDumpStrip(t *testing.T) {
	L := dumpNew(t)
	baseCheck(t, L, `
		local f = load("local x = 1 local y = 2 return x + y")
		local b1 = string.dump(f)
		local b2 = string.dump(f, true)
		r = tostring(load(b2)()) .. "|" .. tostring(#b2 < #b1)`,
		"r", "3|true")
}

// TestDumpUpvalueIsEnviron checks the documented (and slightly odd)
// PUC-Lua behaviour: the upvalues of a dumped function are not saved,
// and 'lua_load' installs the globals table as the first one, so an
// upvalue that was a number becomes a table.
func TestDumpUpvalueIsEnviron(t *testing.T) {
	L := dumpNew(t)
	baseCheck(t, L, `
		local x = 100
		local f = function() return x + 1 end
		local g = load(string.dump(f))
		local ok, err = pcall(g)
		r = tostring(ok) .. "|" ..
		    tostring(err:find("arithmetic") ~= nil)`, "r", "false|true")
}

// TestDumpNotALuaFunction checks that only Lua functions can be dumped.
func TestDumpNotALuaFunction(t *testing.T) {
	if msg := dumpRuns(t, `return string.dump(print)`); !strings.Contains(msg, "unable to dump") {
		t.Errorf("string.dump(print) = %q", msg)
	}
	if msg := dumpRuns(t, `return string.dump(42)`); !strings.Contains(msg, "function expected") {
		t.Errorf("string.dump(42) = %q", msg)
	}
	if msg := dumpRuns(t, `return string.dump()`); msg == "" {
		t.Errorf("string.dump() should fail")
	}
}

// TestDumpLoadBinaryAndText checks that 'load' recognizes both kinds of
// chunk, and that the 'mode' argument is enforced for binary chunks.
func TestDumpLoadBinaryAndText(t *testing.T) {
	L := dumpNew(t)
	baseCheck(t, L, `
		local f = load("return 1 + 1")
		r = tostring(f())`, "r", "2")
	baseCheck(t, L, `
		local bin = string.dump(load("return 1 + 1"))
		local f = load(bin)
		r = tostring(f()) .. "|" .. type(f)`, "r", "2|function")
	baseCheck(t, L, `
		local bin = string.dump(load("return 1 + 1"))
		local f, err = load(bin, "=chunk", "t")
		r = tostring(f) .. "|" .. tostring(err)`, "r", "nil|attempt to load a binary chunk (mode is 't')")
	/* a reader function may also return a binary chunk */
	baseCheck(t, L, `
		local bin = string.dump(load("return 7 * 6"))
		local i = 0
		local f = load(function() i = i + 1
			if i == 1 then return bin end end)
		r = tostring(f())`, "r", "42")
}

// TestDumpCorrupted feeds damaged chunks to 'load': it must answer
// 'nil' plus a message, never crash.
func TestDumpCorrupted(t *testing.T) {
	L := dumpNew(t)
	baseCheck(t, L, `
		local bin = string.dump(load("return 1"))
		local bad = {
			bin:sub(1, 10),                    -- truncated header
			"\27Lua\1" .. bin:sub(6),          -- version mismatch
			"\27Lua\84\1" .. bin:sub(7),       -- format mismatch
			bin:sub(1, 6) .. "\1\2\3\4\5\6" .. bin:sub(13), -- corrupted data
			bin:sub(1, 31) .. string.rep("\255", 40),       -- garbage sizes
			bin:sub(1, #bin - 10),                          -- truncated tail
		}
		local ok, n, first = true, 0, ""
		for _, s in ipairs(bad) do
			local f, err = load(s)
			n = n + 1
			if f ~= nil or type(err) ~= "string" then
				ok = false
			elseif first == "" then
				first = err
			end
		end
		r = tostring(ok) .. "|" .. tostring(n)`, "r", "true|6")
	/* the message mentions the binary format */
	L2 := dumpNew(t)
	baseCheck(t, L2, `
		local bin = string.dump(load("return 1"))
		local f, err = load("\27Lua\1" .. bin:sub(6))
		r = tostring(err:find("version mismatch") ~= nil) .. "|" ..
		    tostring(err:find("binary string") ~= nil)`, "r", "true|true")
}

// TestDumpNamedChunk checks the source recorded in the dump: it wins
// over the chunk name given to 'load' (as in PUC-Lua), and it is absent
// when the chunk was stripped.
func TestDumpNamedChunk(t *testing.T) {
	L := dumpNew(t)
	baseCheck(t, L, `
		local bin = string.dump(load("error('boom')"))
		local f = load(bin, "=mychunk")
		local ok, err = pcall(f)
		r = tostring(ok) .. "|" .. tostring(err)`,
		"r", `false|[string "error('boom')"]:1: boom`)
	/* with strip there is neither a source nor line information, so
	   'error' adds no position at all (same as PUC-Lua 5.3+) */
	baseCheck(t, L, `
		local bin = string.dump(load("error('boom')"), true)
		local f = load(bin, "=mychunk")
		local ok, err = pcall(f)
		r2 = tostring(ok) .. "|" .. tostring(err)`, "r2", "false|boom")
}

// TestDumpLoadFileBinary checks that every loading entry point (and not
// only 'load') recognizes a precompiled chunk: loadfile, dofile and the
// Go API LoadBytes. Skipped when PUC-Lua's luac is not installed.
func TestDumpLoadFileBinary(t *testing.T) {
	const luacPath = "/tmp/lua-5.3.4/src/luac"
	if _, err := os.Stat(luacPath); err != nil {
		t.Skipf("no PUC-Lua at %s", luacPath)
	}
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "chunk.lua")
	binPath := filepath.Join(dir, "chunk.luac")
	if err := os.WriteFile(srcPath, []byte("return 6 * 7"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(luacPath, "-o", binPath, srcPath).CombinedOutput(); err != nil {
		t.Fatalf("luac: %v: %s", err, out)
	}
	L := dumpNew(t)
	baseCheck(t, L, fmt.Sprintf(`r = tostring(loadfile(%q)())`, binPath), "r", "42")
	baseCheck(t, L, fmt.Sprintf(`r2 = tostring(dofile(%q))`, binPath), "r2", "42")
	/* a text file still goes through the parser */
	baseCheck(t, L, fmt.Sprintf(`r3 = tostring(loadfile(%q)())`, srcPath), "r3", "42")
	/* the Go entry point accepts binary chunks too */
	L2 := dumpNew(t)
	data, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := L2.LoadBytes(data, "=go"); err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if st := L2.PCall(0, 1, 0); st != LUA_OK {
		t.Fatalf("PCall: %d", st)
	}
	if got, _ := L2.ToInteger(-1); got != 42 {
		t.Errorf("LoadBytes(binary)() = %d, want 42", got)
	}
	/* a damaged .luac must give nil + a message, not a crash */
	badPath := filepath.Join(dir, "bad.luac")
	if err := os.WriteFile(badPath, data[:12], 0o644); err != nil {
		t.Fatal(err)
	}
	baseCheck(t, L, fmt.Sprintf(`
		local f, err = loadfile(%q)
		r4 = tostring(f) .. "|" .. tostring(type(err))`, badPath),
		"r4", "nil|string")
}

// TestDumpOfficialCompat cross-checks the format against PUC-Lua 5.4
// itself, when its 'luac' and 'lua' binaries are available: a chunk
// compiled by 'luac' must run here, and a chunk dumped here must run in
// PUC-Lua. Skipped when the binaries are not installed.
func TestDumpOfficialCompat(t *testing.T) {
	const (
		luacPath = "/tmp/lua-5.3.4/src/luac"
		luaPath  = "/tmp/lua-5.3.4/src/lua"
	)
	if _, err := os.Stat(luacPath); err != nil {
		t.Skipf("no PUC-Lua at %s", luacPath)
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "chunk.lua")
	bin := filepath.Join(dir, "chunk.luac")
	code := `
		local function add(a, b) return a + b end
		local function sum(n)
			local s = 0
			for i = 1, n do s = s + add(i, i) end
			return s
		end
		local t = {"a", "b", "c"}
		return function(x) return sum(x) .. t[2] .. 1.5 .. tostring(true) end
	`
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(luacPath, "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("luac: %v: %s", err, out)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	/* the chunk produced by 'luac' runs here and gives the same answer
	   as PUC-Lua gives */
	L := dumpNew(t)
	L.PushString(string(data))
	L.SetGlobal("bc")
	baseCheck(t, L, `
		local f = load(bc, "=official")
		r = tostring(f()(3))`, "r", "12b1.5true")

	/* ... and a chunk dumped here runs in PUC-Lua */
	baseCheck(t, L, `
		out = string.dump(load("return function(x) return x * 2 + 1 end"))`,
		"r", "12b1.5true")
	L.GetGlobal("out")
	dumped, _ := L.ToString(-1)
	L.Pop(1)
	mine := filepath.Join(dir, "mine.luac")
	if err := os.WriteFile(mine, []byte(dumped), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(luaPath, "-e",
		`local f = load(io.open("`+mine+`", "rb"):read("a"))
		 print(f()(20))`).CombinedOutput()
	if err != nil {
		t.Fatalf("PUC-Lua running our dump: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "41" {
		t.Errorf("PUC-Lua running our dump = %q, want %q", got, "41")
	}
}
