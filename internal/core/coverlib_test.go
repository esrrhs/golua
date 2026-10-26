package core

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

// The differential and official suites run cmd/lua in another process, so
// their answers never show up in go test -cover. These tests ask the same
// libraries from inside the test process.

func TestLibrariesInProcess(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data.txt")
	in := filepath.Join(dir, "in.txt")
	out := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(in, []byte("from-in\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COVER_DATA", data)
	t.Setenv("COVER_IN", in)
	t.Setenv("COVER_OUT", out)

	got := runChunk(t, `
	local out = {}
	local function say(s) out[#out + 1] = tostring(s) end

	local u = "a\u{20ac}b"
	say(utf8.char(0x61, 0x20AC, 0x62) == u)
	say(utf8.offset(u, 2))
	say(utf8.offset(u, -1))
	say(utf8.offset(u, 0, 2))
	local cps = {utf8.codepoint(u, 1, -1)}
	say(table.concat(cps, ","))
	local codes = {}
	for p, c in utf8.codes(u) do codes[#codes + 1] = p .. ":" .. c end
	say(table.concat(codes, ","))
	say(utf8.len("\255") == nil)
	local ok = pcall(utf8.codepoint, "\255")
	say(not ok)
	local ok2 = pcall(utf8.char, 0x7FFFFFFF + 1)
	say(not ok2)

	local name = os.getenv("COVER_DATA")
	local f = assert(io.open(name, "w+"))
	f:setvbuf("full", 128)
	assert(f:write("1.5\n-2\nhello\n"))
	f:flush()
	f:setvbuf("line")
	f:seek("set", 0)
	say(f:read("*n"))
	say(f:read("*n"))
	say(f:read("*l") == "")
	say(f:read("*l"))
	say(f:read("*L") == nil)
	f:seek("set", 0)
	say(f:read(3))
	f:seek("end", -1)
	say(f:read(1))
	f:close()
	say(io.type(f))

	local n = 0
	for line in io.lines(name) do n = n + 1 end
	say(n)

	local tmp = assert(io.tmpfile())
	tmp:write("z")
	tmp:seek("set")
	say(tmp:read("*a"))
	tmp:close()

	local oldin, oldout = io.input(), io.output()
	io.input(os.getenv("COVER_IN"))
	io.output(os.getenv("COVER_OUT"))
	io.write("hi\n")
	io.flush()
	io.output():close()
	io.input(oldin)
	io.output(oldout)
	local rf = assert(io.open(os.getenv("COVER_OUT")))
	say(rf:read("*a"))
	rf:close()

	local p = assert(io.popen("echo popen-ok", "r"))
	say((p:read("*l"):gsub("\r$", "")))
	local pc, how, code = p:close()
	say(pc)
	say(how)
	say(code)

	say(os.clock() >= 0)
	say(os.getenv("COVER_DATA") == name)
	say(os.getenv("COVER_MISSING") == nil)
	say(os.difftime(10, 3) == 7)
	say(os.difftime(0, math.mininteger) > 0)
	local tn = os.tmpname()
	say(type(tn))
	os.remove(tn)
	local ra, rb = os.rename(name, name .. ".renamed")
	say(ra)
	os.rename(name .. ".renamed", name)
	local d = os.date("!*t", 0)
	say(d.year)
	say(d.month)
	say(d.day)
	say(os.date("!%Y-%m-%d", 0))
	local t = os.time({year = 1970, month = 1, day = 2, hour = 0})
	say(type(t))

	say("10" - "3")
	say("10" % "3")
	say(("2" ^ "3") == 8)
	say("10" / "4")
	say(-"3")
	say(string.match("xxabyy", "a(b)"))
	local g, gn = string.gsub("abc", "(a)(b)", "%2%1")
	say(g)
	say(gn)
	say(string.match("aaab", "a-b"))
	say(string.format("%5s", "ab"))
	say(string.format("%-5s", "ab"))
	say(string.format("%02d", 7))
	say(string.format("%d", 0))
	say(load("return " .. string.format("%q", 1.5))() == 1.5)
	say(string.format("%a", 1.5):find("0x", 1, true) ~= nil)
	local qmin = string.format("%q", math.mininteger)
	say(qmin:find("0x", 1, true) ~= nil)

	local function boom(f)
		local ok2 = pcall(f)
		return not ok2
	end
	say(boom(function() return "a" + 1 end))
	say(boom(function() return "a" .. {} end))
	say(boom(function() for i = "x", 1 do end end))
	say(boom(function() error("x") end))

	local hooks = 0
	debug.sethook(function() hooks = hooks + 1 end, "c")
	local function one() return 1 end
	one()
	debug.sethook()
	say(hooks > 0)

	local rf = assert(io.open(name))
	say(rf:read(0) == "")
	local c = 0
	for _ in rf:lines() do c = c + 1 end
	say(c)
	say(tostring(rf):find("file", 1, true) ~= nil)
	rf:close()

	local saved = io.input()
	io.input(name)
	say(io.read())
	io.input(saved):close()
	local junk = os.getenv("COVER_OUT") .. ".junk"
	local kept = io.output()
	io.output(junk)
	io.write("x")
	io.close()
	io.output(kept)

	say(string.format("%+.0d", 0))
	say(string.format("% .0d", 0))
	say(string.format("%.3s", "abcdef"))
	say(string.match("aabc", "(a)%1"))
	say(string.match("abc", "(a)%1") == nil)
	say(boom(function() return 1.5 << 1 end))
	say(boom(function()
		local t = setmetatable({}, {__call = "x"})
		return t()
	end))
	say(type(os.time({year = 2020, month = 7, day = 1, hour = 12, isdst = false})))
	say(type(os.time({year = 2020, month = 1, day = 1, hour = 12, isdst = true})))

	return table.concat(out, "\n")
	`)
	want := strings.Join([]string{
		"true",
		"2",
		"5",
		"2",
		"97,8364,98",
		"1:97,2:8364,5:98",
		"true",
		"true",
		"true",
		"1.5",
		"-2",
		"true",
		"hello",
		"true",
		"1.5",
		"\n",
		"closed file",
		"3",
		"z",
		"hi\n",
		"popen-ok",
		"true",
		"exit",
		"0",
		"true",
		"true",
		"true",
		"true",
		"true",
		"string",
		"true",
		"1970",
		"1",
		"1",
		"1970-01-01",
		"number",
		"7.0",
		"1.0",
		"true",
		"2.5",
		"-3.0",
		"b",
		"bac",
		"1",
		"aaab",
		"   ab",
		"ab   ",
		"07",
		"0",
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"true",
		"3",
		"true",
		"1.5",
		"+",
		" ",
		"abc",
		"a",
		"true",
		"true",
		"true",
		"number",
		"number",
	}, "\n")
	if got != want {
		t.Fatalf("libraries:\n%s", sideBySide(want, got))
	}
	body, err := os.ReadFile(out)
	if err != nil || string(body) != "hi\n" {
		t.Fatalf("io.output wrote %q (%v)", body, err)
	}
	ioFlushAll()
	ioPruneOpen()
}

func TestLocaleHelpersAndPureGoPath(t *testing.T) {
	if langOf("pt_BR.UTF-8@euro") != "pt" || langOf("C") != "C" || langOf("") != "" {
		t.Fatalf("langOf: %q %q %q", langOf("pt_BR.UTF-8@euro"), langOf("C"), langOf(""))
	}
	if decPointOf("") != "." || decPointOf("C") != "." || decPointOf("POSIX") != "." ||
		decPointOf("fr_FR") != "," || decPointOf("en_US") != "." {
		t.Fatalf("decPointOf: %q %q %q", decPointOf("fr_FR"), decPointOf("en_US"), decPointOf("C"))
	}
	if indexString("abc", "") != 0 || indexString("abc", "bc") != 1 || indexString("abc", "zz") != -1 {
		t.Fatal("indexString")
	}
	if indexByte("a.b", '.') != 1 || indexByte("ab", '.') != -1 {
		t.Fatal("indexByte")
	}

	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LC_NUMERIC", "")
	t.Setenv("LANG", "C")
	if envLocale() != "de_DE.UTF-8" {
		t.Fatalf("envLocale = %q", envLocale())
	}
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_NUMERIC", "pt_BR")
	if envLocale() != "pt_BR" {
		t.Fatalf("envLocale numeric = %q", envLocale())
	}
	t.Setenv("LC_NUMERIC", "")
	t.Setenv("LANG", "")
	if envLocale() != "C" {
		t.Fatalf("envLocale empty = %q", envLocale())
	}

	oldDec, oldName := localeDecPoint, localeName
	localeDecPoint = ","
	if num2strLocale("1.5e+1") != "1,5e+1" || num2strLocale("inf") != "inf" {
		t.Fatalf("num2strLocale %q", num2strLocale("1.5e+1"))
	}
	if str2numLocale("3,14") != "3.14" || str2numLocale("nan") != "nan" {
		t.Fatal("str2numLocale")
	}
	localeDecPoint, localeName = oldDec, oldName

	wasC := ctypeIsC.Load()
	ctypeIsC.Store(false)
	if !localeIsGraph('A') || localeIsGraph(' ') || !localeIsGraph(0x7e) {
		t.Fatal("localeIsGraph")
	}
	ctypeIsC.Store(wasC)

	if strColl != nil {
		was := collateIsC.Load()
		collateIsC.Store(false)
		if luaV_strcmp("a\x00b", "a\x00c") >= 0 ||
			luaV_strcmp("a\x00b", "a\x00b") != 0 ||
			luaV_strcmp("a", "a\x00b") >= 0 ||
			luaV_strcmp("a\x00b", "a") <= 0 ||
			luaV_strcmp("ab", "ab\x00") >= 0 {
			t.Fatal("luaV_strcmp across zeros")
		}
		collateIsC.Store(was)
	}

	hook, query := setLocaleHook, localeQueryHook
	dec, name := localeDecPoint, localeName
	t.Cleanup(func() {
		setLocaleHook, localeQueryHook = hook, query
		localeDecPoint, localeName = dec, name
		if hook != nil {
			setLocale(0, "C")
		}
	})
	setLocaleHook, localeQueryHook = nil, nil
	got, ok := setLocale(0, "")
	if !ok || got == "" {
		t.Fatalf("pure setlocale empty: %q %v", got, ok)
	}
	got, ok = setLocale(4, "pt_BR.UTF-8")
	if !ok || got != "pt_BR.UTF-8" || localeDecPoint != "," {
		t.Fatalf("pure numeric: %q %v point %q", got, ok, localeDecPoint)
	}
	if localeCurrent(4) != "pt_BR.UTF-8" {
		t.Fatalf("pure query %q", localeCurrent(4))
	}
	if _, ok = setLocale(1, "C"); !ok || localeDecPoint != "," {
		t.Fatal("collate must not change the decimal point")
	}
	got, ok = setLocale(0, "C")
	if !ok || got != "C" || localeDecPoint != "." {
		t.Fatalf("pure restore: %q %q", got, localeDecPoint)
	}
}

func TestMathApproximationsAndSmallAPI(t *testing.T) {
	if math.IsInf(mathExpScaled(709.5, 0), 0) || mathExpScaled(800, 0) != math.Inf(1) {
		t.Fatalf("exp scaled %v %v", mathExpScaled(709.5, 0), mathExpScaled(800, 0))
	}
	if math.IsInf(mathHalfExp(710), 0) {
		t.Fatal("half exp")
	}
	if math.Abs(mathLogScaled(1e-308)-(-709.19620864216608)) > 1e-9 {
		t.Fatalf("log scaled %v", mathLogScaled(1e-308))
	}
	if math.Abs(mathLog10Scaled(1e-308)-(-308)) > 1e-6 {
		t.Fatalf("log10 scaled %v", mathLog10Scaled(1e-308))
	}
	near := mathLog2Scaled(1 + math.Ldexp(1, -52))
	if math.Abs(near-3.2034265038149176e-16) > 1e-30 {
		t.Fatalf("log2 scaled %v", near)
	}
	if math2Pow(2, 10) != 1024 || math2Sqrt(9) != 3 || math2Abs(-1.5) != 1.5 || !math2IsInf(math.Inf(1), 1) {
		t.Fatal("math2")
	}
	if !utf8IsContAt("\x80", 0) || utf8IsContAt("\x80", 1) || utf8IsContAt("A", 0) {
		t.Fatal("utf8IsContAt")
	}
	r, n := utf8Decode("€")
	if r != '€' || n != 3 {
		t.Fatalf("utf8Decode %q %d", r, n)
	}

	L := NewState()
	defer L.Close()
	if !L.Open("string") || L.Open("no-such-lib") {
		t.Fatal("Open")
	}
	L.PushGoClosure(func(L *State) int { return 0 }, 0)
	if L.ToGoFunction(-1) == nil {
		t.Fatal("ToGoFunction")
	}
	L.Pop(1)
	L.CreateTable(0, 0)
	var key int
	p := unsafe.Pointer(&key)
	L.PushInteger(9)
	L.RawSetP(-2, p)
	if L.RawGetP(-1, p) != LUA_TNUMBER {
		t.Fatal("RawGetP")
	}
	n64, ok := L.ToInteger(-1)
	if !ok || n64 != 9 {
		t.Fatal("RawGetP value")
	}
	L.Pop(1)
	L.PushInteger(3)
	L.SetGlobalValue("cover_n", L.stack[L.top-1])
	L.Pop(1)
	L.GetGlobal("cover_n")
	if v, ok := L.ToInteger(-1); !ok || v != 3 {
		t.Fatal("SetGlobalValue")
	}
	L.Pop(1)

	L.CreateTable(0, 0)
	L.CheckType(-1, LUA_TTABLE)
	L.Pop(1)
	if err := L.LoadString("return function() end", "=f"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal("load function")
	}
	L.CheckType(-1, LUA_TFUNCTION)
	L.Pop(1)
	L.NewUserData(nil, 0)
	if !L.IsUserData(-1) {
		t.Fatal("IsUserData")
	}
	L.Pop(1)

	b := L.NewBuffer()
	b.AddString("€")
	if b.Len() != 3 {
		t.Fatalf("buffer len %d", b.Len())
	}
	room := b.room(1)
	if len(room) == 0 {
		t.Fatal("buffer room")
	}
	room[0] = '!'
	b.AddSize(1)
	b.PushResult()
	s, _ := L.ToString(-1)
	if !strings.HasPrefix(s, "€") || !strings.HasSuffix(s, "!") {
		t.Fatalf("buffer %q", s)
	}
	bufFinalize(b)

	if L.HookExternal() {
		t.Fatal("hook")
	}
	_ = L.GetHook()
	_ = L.GetHookMask()
	_ = L.GetHookCount()

	var v Value
	setnilvalue(&v)
	if !ttisstrictnil(&v) || ttistrue(&v) || bvalue(&v) || iscollectable(&v) {
		t.Fatal("nil value")
	}
	setbvalue(&v, true)
	if !ttistrue(&v) || !bvalue(&v) {
		t.Fatal("true value")
	}
	ts := luaS_new(L, strings.Repeat("x", 64))
	var sv Value
	setsvalue(&sv, ts)
	if !ttislngstring(&sv) || !iscollectable(&sv) || tsslen(ts) != 64 {
		t.Fatal("long string")
	}
	setabstkey(&v)
	if !ttisnil(&v) { /* 5.3.4: absent sentinel is nilobject copy / nil tag */
		t.Fatal("abstkey")
	}
	if makevariant(4, 1) != 4|1<<4 {
		t.Fatal("makevariant")
	}
	if toPtrAny(&Table{}) == 0 || toPtrAny(0) != 0 {
		t.Fatal("toPtrAny")
	}
	if ttisclosure(&sv) {
		t.Fatal("string is not a closure")
	}
	short := luaS_new(L, "ab")
	if !short.isShort() {
		t.Fatal("isShort")
	}
	if sBx(0) != -OFFSET_sBx || !numIsNaN(math.NaN()) || numIsNaN(0) {
		t.Fatal("sBx / nan")
	}
	if n, err := ioWriteString(nil, "a"); n != 0 || err == nil {
		t.Fatal("ioWriteString nil")
	}
	var buf strings.Builder
	if n, err := ioWriteString(&buf, "xy"); n != 2 || err != nil || buf.String() != "xy" {
		t.Fatal("ioWriteString")
	}
}

func TestOSExitStatus(t *testing.T) {
	old := osExitHook
	var code int
	var calls int
	osExitHook = func(c int) {
		calls++
		code = c
	}
	t.Cleanup(func() { osExitHook = old })

	if got := runChunk(t, `os.exit(3); return "after"`); got != "after" || calls != 1 || code != 3 {
		t.Fatalf("exit 3: got %q calls %d code %d", got, calls, code)
	}
	if got := runChunk(t, `os.exit(false); return "after"`); got != "after" || code != 1 {
		t.Fatalf("exit false: got %q code %d", got, code)
	}
	if got := runChunk(t, `os.exit(true); return "after"`); got != "after" || code != 0 {
		t.Fatalf("exit true: got %q code %d", got, code)
	}
}
