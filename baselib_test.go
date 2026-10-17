package lua

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

// baseNew creates a state with the base library opened.
func baseNew(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	OpenBase(L)
	L.Pop(1) /* remove the global table pushed by OpenBase */
	return L
}

// baseRun runs a chunk, failing the test on any error.
func baseRun(t *testing.T, L *State, src string) {
	t.Helper()
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v\nsource:\n%s", err, src)
	}
}

// baseRunErr runs a chunk that must fail, returning its message.
func baseRunErr(t *testing.T, src string) string {
	t.Helper()
	L := baseNew(t)
	err := L.DoString(src)
	if err == nil {
		t.Fatalf("expected an error from:\n%s", src)
	}
	return err.Error()
}

// baseGlobal returns a global as a string (the way 'tostring' would
// show it), so that results can be compared with literals.
func baseGlobal(t *testing.T, L *State, name string) string {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	switch L.Type(-1) {
	case LUA_TNIL:
		return "nil"
	case LUA_TBOOLEAN:
		if L.ToBoolean(-1) {
			return "true"
		}
		return "false"
	}
	if s, ok := L.ToString(-1); ok {
		return s
	}
	return "<" + TypeName(L.Type(-1)) + ">"
}

// baseCheck runs a chunk and compares the (stringified) global 'name'.
func baseCheck(t *testing.T, L *State, src, name, want string) {
	t.Helper()
	baseRun(t, L, src)
	if got := baseGlobal(t, L, name); got != want {
		t.Errorf("%s = %v, want %q\nsource:\n%s", name, got, want, src)
	}
}

// ---------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------

func TestBasePrint(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	L := baseNew(t)
	baseRun(t, L, `print("a", 1, 3.5, true, nil)`)
	baseRun(t, L, `print()`)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	want := "a\t1\t3.5\ttrue\tnil\n\n"
	if string(out) != want {
		t.Errorf("print output = %q, want %q", out, want)
	}
}

func TestBaseWarn(t *testing.T) {
	L := baseNew(t)
	baseRun(t, L, `warn("hello", " world")`)
	baseRun(t, L, `warn("one")`)
	if msg := baseRunErr(t, `warn({})`); !strings.Contains(msg, "table") {
		t.Errorf("warn(1) error = %q", msg)
	}
}

func TestBaseAssert(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `a, b = assert(42, "msg")`, "a", "42")
	baseCheck(t, L, `a, b = assert(42, "msg")`, "b", "msg")
	baseCheck(t, L, `r = assert(1 < 2)`, "r", "true")
	if msg := baseRunErr(t, `assert(false, "boom")`); !strings.Contains(msg, "boom") {
		t.Errorf("assert(false,'boom') error = %q", msg)
	}
	if msg := baseRunErr(t, `assert(nil)`); !strings.Contains(msg, "assertion failed!") {
		t.Errorf("assert(nil) error = %q", msg)
	}
	if msg := baseRunErr(t, `assert(false, 42)`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseErrorLevel(t *testing.T) {
	msg := baseRunErr(t, "error('boom')")
	if !strings.Contains(msg, "boom") {
		t.Errorf("error('boom') = %q", msg)
	}
	if !strings.Contains(msg, ":1:") {
		t.Errorf("error('boom') has no position: %q", msg)
	}
	msg2 := baseRunErr(t, "local function f() error('deep', 2) end\nf()")
	if !strings.Contains(msg2, "deep") || !strings.Contains(msg2, ":2:") {
		t.Errorf("error('deep', 2) = %q", msg2)
	}
	/* level 0: no position added */
	msg3 := baseRunErr(t, "error('plain', 0)")
	if msg3 != "plain" {
		t.Errorf("error('plain', 0) = %q, want \"plain\"", msg3)
	}
	/* non-string error objects are raised as they are */
	msg4 := baseRunErr(t, "error({})")
	if msg4 == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseType(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = type(nil) .. "," .. type(true) .. "," .. type(1) ..
		"," .. type(1.5) .. "," .. type("s") .. "," .. type({}) .. "," ..
		type(print)`, "r", "nil,boolean,number,number,string,table,function")
	if msg := baseRunErr(t, `type()`); !strings.Contains(msg, "value expected") {
		t.Errorf("type() error = %q", msg)
	}
}

func TestBaseToNumber(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = tostring(tonumber("10") + tonumber("ff", 16) + tonumber("101", 2))`, "r", "270")
	baseCheck(t, L, `r = tostring(tonumber("  42  "))`, "r", "42")
	baseCheck(t, L, `r = tostring(tonumber("0x10"))`, "r", "16")
	baseCheck(t, L, `r = tostring(tonumber("1e2"))`, "r", "100.0")
	baseCheck(t, L, `r = tostring(tonumber("abc"))`, "r", "nil")
	baseCheck(t, L, `r = tostring(tonumber(7))`, "r", "7")
	/* 'b_str2int' does not understand the "0x" prefix */
	baseCheck(t, L, `r = tostring(tonumber("0x10", 16))`, "r", "nil")
	baseCheck(t, L, `r = tostring(tonumber(" -ff ", 16))`, "r", "-255")
	baseCheck(t, L, `r = tostring(tonumber("12", 8))`, "r", "10")
	if msg := baseRunErr(t, `tonumber("10", 40)`); !strings.Contains(msg, "base out of range") {
		t.Errorf("tonumber base out of range = %q", msg)
	}
	if msg := baseRunErr(t, `tonumber(10, 10)`); msg == "" {
		t.Errorf("expected an error")
	}
	if msg := baseRunErr(t, `tonumber()`); !strings.Contains(msg, "value expected") {
		t.Errorf("tonumber() = %q", msg)
	}
}

func TestBaseToString(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = tostring(10) .. "|" .. tostring(1.5) .. "|" ..
		tostring("s") .. "|" .. tostring(true) .. "|" .. tostring(nil)`,
		"r", "10|1.5|s|true|nil")
	/* __tostring metamethod */
	baseCheck(t, L, `
		local mt = {__tostring = function(t) return "OBJ(" .. t.n .. ")" end}
		r = tostring(setmetatable({n = 7}, mt))`, "r", "OBJ(7)")
	/* __name */
	baseRun(t, L, `
		local mt = {__name = "MyType"}
		r = tostring(setmetatable({}, mt))`)
	if got := baseGlobal(t, L, "r"); !strings.HasPrefix(got, "MyType: 0x") {
		t.Errorf("tostring with __name = %q", got)
	}
}

func TestBaseSelect(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = tostring(select('#', 1, 2, 3))`, "r", "3")
	baseCheck(t, L, `r = tostring(select('#'))`, "r", "0")
	baseCheck(t, L, `r = select(2, "a", "b", "c")`, "r", "b")
	baseCheck(t, L, `r = select(-1, "a", "b", "c")`, "r", "c")
	baseCheck(t, L, `r = select(-3, "a", "b", "c")`, "r", "a")
	baseCheck(t, L, `r = tostring(#{select(2, "a", "b", "c")})`, "r", "2")
	if msg := baseRunErr(t, `select(0, "a")`); !strings.Contains(msg, "index out of range") {
		t.Errorf("select(0) = %q", msg)
	}
	if msg := baseRunErr(t, `select(-4, "a")`); !strings.Contains(msg, "index out of range") {
		t.Errorf("select(-4) = %q", msg)
	}
}

func TestBaseNextPairsIpairs(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local t = {10, 20, 30}
		local s, n = 0, 0
		for k, v in ipairs(t) do s = s + k * v; n = n + 1 end
		r = s .. "/" .. n`, "r", "140/3")
	baseCheck(t, L, `
		local sum = 0
		for k, v in pairs({a = 1, b = 2, c = 3}) do sum = sum + v end
		r = tostring(sum)`, "r", "6")
	baseCheck(t, L, `
		local k, v = next({x = 5})
		r = tostring(k) .. "=" .. tostring(v)`, "r", "x=5")
	baseCheck(t, L, `r = tostring(next({}) == nil)`, "r", "true")
	baseCheck(t, L, `
		local t = {"a"}
		local k1, v1 = next(t, nil)
		local k2, v2 = next(t, k1)
		r = tostring(k1) .. tostring(v1) .. tostring(k2) .. tostring(v2)`, "r", "1anilnil")
	/* ipairs returns 3 values: iterator, state and 0 */
	baseCheck(t, L, `
		local f, s, i = ipairs({})
		r = tostring(i) .. tostring(type(f))`, "r", "0function")
	if msg := baseRunErr(t, `next(1)`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBasePairsMetamethod(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local obj = setmetatable({}, {__pairs = function(t)
			local i = 0
			return function(_, k)
				i = i + 1
				if i <= 2 then return i, "v" .. i end
			end, t, nil
		end})
		local acc = ""
		for k, v in pairs(obj) do acc = acc .. k .. ":" .. v .. "," end
		r = acc`, "r", "1:v1,2:v2,")
}

func TestBasePCallXPCall(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		ok, err = pcall(function() error("oops") end)
		r = tostring(ok) .. "|" .. tostring(err)`, "r", "false|chunk:2: oops")
	baseCheck(t, L, `
		ok, v = pcall(function(a, b) return a + b end, 3, 4)
		r = tostring(ok) .. "|" .. tostring(v)`, "r", "true|7")
	baseCheck(t, L, `
		local a, b, c = pcall(function() return 1, 2 end)
		r = tostring(a) .. tostring(b) .. tostring(c)`, "r", "true12")
	/* pcall also catches non-string error objects */
	baseCheck(t, L, `
		ok, err = pcall(function() error({code = 42}) end)
		r = tostring(ok) .. "|" .. type(err)`, "r", "false|table")
	/* xpcall with a message handler */
	baseCheck(t, L, `
		ok, err = xpcall(function() error("bad") end,
		                 function(e) return "H:" .. e end)
		r = tostring(ok) .. "|" .. tostring(err)`, "r", "false|H:chunk:2: bad")
	/* xpcall passing extra arguments (5.4) */
	baseCheck(t, L, `
		ok, v = xpcall(function(a, b) return a * b end, print, 6, 7)
		r = tostring(ok) .. "|" .. tostring(v)`, "r", "true|42")
	if msg := baseRunErr(t, `pcall()`); msg == "" {
		t.Errorf("expected an error")
	}
	if msg := baseRunErr(t, `xpcall(print, 5)`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseMetatables(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local t = {}
		r = tostring(getmetatable(t))
		local mt = {__index = function() return 99 end}
		setmetatable(t, mt)
		r2 = tostring(getmetatable(t) == mt)
		r3 = tostring(t.anything)
		setmetatable(t, nil)
		r4 = tostring(getmetatable(t))`, "r2", "true")
	baseCheck(t, L, `
		local t = {}
		r = tostring(getmetatable(t))
		local mt = {__index = function() return 99 end}
		setmetatable(t, mt)
		r3 = tostring(t.anything)`, "r3", "99")
	baseCheck(t, L, `
		local t = {}
		r = tostring(getmetatable(t))
		local mt = {__index = function() return 99 end}
		setmetatable(t, mt)
		r3 = tostring(t.anything)
		setmetatable(t, nil)
		r4 = tostring(getmetatable(t))`, "r4", "nil")
	baseCheck(t, L, `
		local t = {}
		r = tostring(getmetatable(t))`, "r", "nil")
	/* __metatable protects the metatable */
	baseCheck(t, L, `
		local t = setmetatable({}, {__metatable = "locked"})
		r = tostring(getmetatable(t))`, "r", "locked")
	if msg := baseRunErr(t, `
		local t = setmetatable({}, {__metatable = "locked"})
		setmetatable(t, {})`); !strings.Contains(msg, "protected") {
		t.Errorf("protected metatable error = %q", msg)
	}
	if msg := baseRunErr(t, `setmetatable({}, 5)`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseRawFunctions(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local t = setmetatable({}, {__newindex = function() error("no") end,
		                            __index = function() return "meta" end,
		                            __len = function() return 99 end,
		                            __eq = function() return true end})
		rawset(t, "k", "v")
		r1 = rawget(t, "k")
		r2 = rawlen(t)
		r3 = tostring(rawlen("abcd"))
		r4 = tostring(rawequal(t, t))
		r5 = tostring(rawequal(t, {}))
		r6 = tostring(t.other)`, "r1", "v")
	baseCheck(t, L, `
		local t = setmetatable({}, {__newindex = function() error("no") end,
		                            __index = function() return "meta" end,
		                            __len = function() return 99 end})
		rawset(t, "k", "v")
		r2 = rawlen(t)`, "r2", "0")
	baseCheck(t, L, `
		local t = {}
		rawset(t, "k", "v")
		r3 = tostring(rawlen("abcd"))`, "r3", "4")
	baseCheck(t, L, `
		local t = {}
		r4 = tostring(rawequal(t, t))
		r5 = tostring(rawequal(t, {}))`, "r4", "true")
	baseCheck(t, L, `
		local t = {}
		r5 = tostring(rawequal(t, {}))`, "r5", "false")
	baseCheck(t, L, `
		local t = setmetatable({}, {__index = function() return "meta" end})
		r6 = tostring(t.other)`, "r6", "meta")
	if msg := baseRunErr(t, `rawlen(1)`); msg == "" {
		t.Errorf("expected an error")
	}
	if msg := baseRunErr(t, `rawget(1, 2)`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseLoad(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local f = load("return 1 + 2")
		r = tostring(f())`, "r", "3")
	baseCheck(t, L, `
		local f = load("return ...", "=mychunk")
		r = tostring(f(10))`, "r", "10")
	baseCheck(t, L, `
		local f, err = load("return +")
		r = tostring(f) .. "|" .. tostring(err ~= nil)`, "r", "nil|true")
	/* reader function */
	baseCheck(t, L, `
		local parts = {"return ", "1 +", " 2"}
		local i = 0
		local f = load(function() i = i + 1; return parts[i] end)
		r = tostring(f())`, "r", "3")
	baseCheck(t, L, `
		local f, err = load(function() error("reader") end)
		r = tostring(f)`, "r", "nil")
	/* 'env' argument sets the _ENV upvalue */
	baseCheck(t, L, `
		local env = {x = "from env"}
		local f = load("return x", "=c", "t", env)
		r = tostring(f())`, "r", "from env")
	baseCheck(t, L, `
		local env = {}
		local f = load("y = 5 return y", "=c", "t", env)
		r = tostring(f()) .. "|" .. tostring(env.y) .. "|" .. tostring(y)`,
		"r", "5|5|nil")
	/* a number is converted to a string ('lua_tolstring' accepts
	   numbers), so load(1) fails with a syntax error, but does not
	   raise: it returns nil plus the message */
	baseCheck(t, L, `
		local f, err = load(1)
		r = tostring(f) .. "|" .. tostring(err ~= nil)`, "r", "nil|true")
	/* a reader that returns a non-string makes 'load' fail, not raise */
	baseCheck(t, L, `
		local f, err = load(function() return {} end)
		r = tostring(f) .. "|" .. tostring(err ~= nil)`, "r", "nil|true")
	/* no argument at all is a type error, which is raised */
	if msg := baseRunErr(t, `load()`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseDoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chunk.lua")
	if err := os.WriteFile(path, []byte("return 7"), 0o644); err != nil {
		t.Fatal(err)
	}
	L := baseNew(t)
	baseCheck(t, L, fmt.Sprintf("r = tostring(dofile(%q))", path), "r", "7")
	baseCheck(t, L, fmt.Sprintf(`
		local f = loadfile(%q)
		r = tostring(f())`, path), "r", "7")
	if msg := baseRunErr(t, `dofile("/no/such/file.lua")`); msg == "" {
		t.Errorf("expected an error")
	}
}

func TestBaseCollectGarbage(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = tostring(collectgarbage())`, "r", "0")
	/* "count" is the size of the Go heap in Kbytes: an actual number,
	   not 0, so that scripts that watch memory see something move. */
	baseCheck(t, L, `r = type(collectgarbage("count"))`, "r", "number")
	baseCheck(t, L, `r = tostring(collectgarbage("count") > 0)`, "r", "true")
	/* And the rest is what lbaselib.c's 'luaB_collectgarbage' answers,
	   checked against PUC-Lua: 'step' and 'isrunning' are booleans,
	   'setpause' and 'setstepmul' answer the value they replaced
	   (200 and 100, LUAI_GCPAUSE and LUAI_GCMUL), and the two modes
	   answer the name of the mode they replaced -- which for a fresh
	   state is "generational", because that is what 5.4 opens in. */
	baseCheck(t, L, `r = tostring(collectgarbage("isrunning"))`, "r", "true")
	/* A basic step -- the one with no size -- answers false in C and
	   answers false here.  A step that was given work to do answers
	   true, because here any step is a whole cycle; see the comment
	   in baseCollectGarbage for why it cannot be false to both. */
	baseCheck(t, L, `r = tostring(collectgarbage("step"))`, "r", "false")
	baseCheck(t, L, `r = tostring(collectgarbage("step", 20000))`, "r", "true")
	baseCheck(t, L, `r = tostring(collectgarbage("setpause", 100))`, "r", "200")
	baseCheck(t, L, `r = tostring(collectgarbage("setstepmul", 50))`, "r", "100")
	baseCheck(t, L, `r = collectgarbage("generational")`, "r", "generational")
	baseCheck(t, L, `r = collectgarbage("incremental")`, "r", "generational")
	baseCheck(t, L, `r = collectgarbage("generational")`, "r", "incremental")
	/* A 0 is a value like any other: 'setpause' with no argument sets
	   the pause to 0, which the next call answers. */
	baseCheck(t, L, `r = tostring(collectgarbage("setpause"))`, "r", "100")
	baseCheck(t, L, `r = tostring(collectgarbage("setpause", 200))`, "r", "0")
	baseCheck(t, L, `r = tostring(collectgarbage("collect"))`, "r", "0")
	baseCheck(t, L, `r = tostring(collectgarbage("stop"))`, "r", "0")
	baseCheck(t, L, `r = tostring(collectgarbage("isrunning"))`, "r", "false")
	baseCheck(t, L, `r = tostring(collectgarbage("restart"))`, "r", "0")
	baseCheck(t, L, `r = tostring(collectgarbage("isrunning"))`, "r", "true")
	/* A second argument that is not a number is an error, because C
	   reads it: 'luaL_optinteger' rejects it. */
	if msg := baseRunErr(t, `collectgarbage("step", "x")`); !strings.Contains(msg, "number expected") {
		t.Errorf(`collectgarbage("step", "x") = %q`, msg)
	}
	if msg := baseRunErr(t, `collectgarbage("foo")`); !strings.Contains(msg, "invalid option") {
		t.Errorf("collectgarbage('foo') = %q", msg)
	}
}

// TestBaseCollectGarbageCollects is what 'collectgarbage("collect")' is
// for: after it, an entry whose key nothing refers to any more has
// left the weak table, and the '__gc' of what has become unreachable
// has run.  coroutine.lua, gengc.lua and locals.lua of PUC-Lua's own
// suite are what assert it; this is the same assertion in three lines.
func TestBaseCollectGarbageCollects(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `
		local t = setmetatable({}, {__mode = "k"})
		do local k = {} t[k] = 1 end
		r = tostring(next(t) ~= nil)
		collectgarbage()
		r = r .. " " .. tostring(next(t) == nil)`, "r", "true true")
	baseCheck(t, L, `
		local ran = false
		do
			local t = setmetatable({}, {__gc = function() ran = true end})
		end
		r = tostring(ran)
		collectgarbage()
		r = r .. " " .. tostring(ran)`, "r", "false true")
}

func TestBaseGlobals(t *testing.T) {
	L := baseNew(t)
	baseCheck(t, L, `r = tostring(_G._G == _G)`, "r", "true")
	baseCheck(t, L, `r = _VERSION`, "r", "Lua 5.4")
	baseCheck(t, L, `r = tostring(type(_G))`, "r", "table")
	/* the library functions are visible as globals */
	baseCheck(t, L, `r = tostring(type(print)) .. tostring(type(pairs)) ..
		tostring(type(require) == "nil")`, "r", "functionfunctiontrue")
}

// A small end-to-end program using most of the base library.
func TestBaseProgram(t *testing.T) {
	L := baseNew(t)
	baseRun(t, L, `
		local function add(a, b) return a + b end
		assert(add(1, 2) == 3)
		local t = {"a", "b", "c"}
		local acc = ""
		for i, v in ipairs(t) do acc = acc .. i .. v end
		assert(acc == "1a2b3c", acc)
		assert(select("#", "x", "y") == 2)
		assert(tonumber("0x10") == 16)
		local ok, err = pcall(function() error("caught") end)
		assert(not ok and type(err) == "string", tostring(err))
		local mt = {__tostring = function() return "MT" end}
		assert(tostring(setmetatable({}, mt)) == "MT")
		r = "ok"
	`)
	baseCheck(t, L, `r2 = r`, "r2", "ok")
}
