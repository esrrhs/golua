package core

import (
	"strings"
	"testing"
)

func confNewState(t *testing.T) *State {
	t.Helper()
	L := NewState()
	t.Cleanup(L.Close)
	L.OpenLibs()
	return L
}

func confRun(t *testing.T, src string) *State {
	t.Helper()
	L := confNewState(t)
	if err := L.DoString(src); err != nil {
		t.Fatalf("DoString: %v\nsource:\n%s", err, src)
	}
	return L
}

func confRunError(t *testing.T, src string) string {
	t.Helper()
	L := confNewState(t)
	err := L.DoString(src)
	if err == nil {
		t.Fatalf("expected error from:\n%s", src)
	}
	return err.Error()
}

func confGlobalString(t *testing.T, L *State, name string) string {
	t.Helper()
	L.GetGlobal(name)
	defer L.Pop(1)
	if s, ok := L.ToString(-1); ok {
		return s
	}
	if L.IsNil(-1) {
		return "nil"
	}
	if L.Type(-1) == LUA_TBOOLEAN {
		if L.ToBoolean(-1) {
			return "true"
		}
		return "false"
	}
	return "<" + TypeName(L.Type(-1)) + ">"
}

func TestConformanceNumbers(t *testing.T) {
	L := confRun(t, `
		assert(math.maxinteger == 9223372036854775807)
		assert(math.mininteger == -9223372036854775807 - 1)
		assert(math.maxinteger + 1 == math.mininteger)
		assert(math.type(math.maxinteger + 1) == "integer")
		assert(7 // 2 == 3 and 7 / 2 == 3.5)
		assert(-7 // 2 == -4 and -7 / 2 == -3.5)
		assert(3 == 3.0)
		assert(math.type(3) == "integer" and math.type(3.0) == "float")
		assert(1 / 0 == math.huge and 0 / 0 ~= 0 / 0)

		local ok0, e0 = pcall(function() return 0 // 0 end)
		local ok1, e1 = pcall(function() return 1 // 0 end)
		assert(not ok0 and tostring(e0):find("divide by zero", 1, true))
		assert(not ok1 and tostring(e1):find("divide by zero", 1, true))
		assert(0.0 // 0.0 ~= 0.0 // 0.0)
		assert(1.0 // 0.0 == math.huge)

		assert(-5 % 3 == 1 and math.type(-5 % 3) == "integer")
		assert(-5.5 % 3 == 0.5 and math.type(-5.5 % 3) == "float")
		assert(5.5 % 2 == 1.5)
		local okm, em = pcall(function() local one, zero = 1, 0; return one % zero end)
		conf_mod_ok, conf_mod_error = okm, tostring(em)
		local okfm, vfm = pcall(function()
			local one, zero = 1.0, 0.0
			return one % zero
		end)
		conf_float_mod_zero = tostring(okfm) .. ":" .. tostring(vfm)

		assert(string.format("%d", 3) == "3")
		-- lstrlib.c:1308-1315 calls luaL_checkinteger. An exactly integral
		-- float is accepted by Lua 5.4; only a non-integral float is rejected.
		assert(string.format("%d", 3.0) == "3")
		local okf = pcall(string.format, "%d", 3.5)
		assert(not okf)
	`)
	if got := confGlobalString(t, L, "conf_mod_ok"); got != "false" {
		t.Errorf("integer modulo by zero pcall success flag = %s, want false", got)
	}
	if got := confGlobalString(t, L, "conf_mod_error"); !strings.Contains(got, "n%0") {
		t.Errorf("integer modulo by zero error = %q, want a message containing n%%0", got)
	}
	// Float modulo by zero is NaN. The sign bit is the host libm's:
	// glibc's fmod prints "-nan", others print "nan".
	got := confGlobalString(t, L, "conf_float_mod_zero")
	if got != "true:nan" && got != "true:-nan" {
		t.Errorf("1.0 %% 0.0 = %q, want true:nan or true:-nan", got)
	}
}

func TestConformanceStringCoercion(t *testing.T) {
	confRun(t, `
		assert("10" + 5 == 15)
		assert("10" * 2 == 20)
		assert(10 .. "" == "10")
		assert(#"abc" == 3)
		assert(tonumber(" 0x10 ") == 16)
		assert(tonumber("10", 2) == 2)
		assert(tonumber(" -ff ", 16) == -255)
		assert(tonumber("xyz") == nil)
		assert(tostring(1e15) == "1e+15")
		assert(tostring(0.1) == "0.1")
		local ok = pcall(function() return "not-a-number" + 1 end)
		assert(not ok)
	`)
}

func TestConformanceComparisons(t *testing.T) {
	L := confRun(t, `
		assert("a" < "b" and "b" <= "b")
		assert(3 == 3.0 and 3 ~= 3.5)
		-- lvm.c:579-589 converts an exactly representable float to an
		-- integer before comparing mixed numeric variants; lvm.h:75 routes
		-- raw constant equality through that same luaV_equalobj path.
		local large_integer = 9007199254740993
		local rounded_float = 9007199254740993.0
		conf_mixed_large_neq = (large_integer ~= rounded_float)
		conf_mixed_large_literal_neq = (9007199254740993 ~= 9007199254740993.0)
		assert(9007199254740992 == 9007199254740992.0)
		assert(not (3 == "3"))
		local ok1, e1 = pcall(function() return 3 < "4" end)
		local ok2, e2 = pcall(function() return {} < 1 end)
		assert(not ok1 and tostring(e1):find("compare", 1, true))
		assert(not ok2 and tostring(e2):find("compare", 1, true))
	`)
	if got := confGlobalString(t, L, "conf_mixed_large_neq"); got != "true" {
		t.Errorf("mixed integer/float equality lost precision at runtime: got %s, want true", got)
	}
	if got := confGlobalString(t, L, "conf_mixed_large_literal_neq"); got != "true" {
		t.Errorf("mixed integer/float literal equality lost precision: got %s, want true", got)
	}
}

func TestConformanceMetamethods(t *testing.T) {
	L := confRun(t, `
		local fallback = {from_table = 11}
		local t1 = setmetatable({}, {__index = fallback})
		assert(t1.from_table == 11)
		local t2 = setmetatable({}, {__index = function(_, k) return "fn:" .. k end})
		assert(t2.missing == "fn:missing")

		local writes = {}
		local sink = {}
		local n1 = setmetatable({}, {__newindex = function(_, k, v) writes[k] = v end})
		n1.x = 4
		local n2 = setmetatable({}, {__newindex = sink})
		n2.y = 5
		assert(writes.x == 4 and sink.y == 5 and rawget(n1, "x") == nil)

		local callable = setmetatable({base = 7}, {
			__call = function(self, a, b) return self.base + a + b end,
			__tostring = function(self) return "callable:" .. self.base end,
			__len = function() return 99 end,
		})
		assert(callable(2, 3) == 12)
		assert(tostring(callable) == "callable:7")
		assert(#callable == 99 and rawlen(callable) == 0)

		local p = setmetatable({}, {__len = function() return "length" end})
		assert(#p == "length")
		local okn = pcall(function() return #true end)
		assert(not okn)
		assert(#"abc" == 3)

		local eqcalls = 0
		local mta = {__eq = function(a, b) eqcalls = eqcalls + 1; return a.id == b.id end}
		local mtb = {__eq = function() eqcalls = eqcalls + 1; return true end}
		local a = setmetatable({id = 1}, mta)
		local b = setmetatable({id = 1}, mta)
		local c = setmetatable({id = 8}, mtb)
		assert(a == b and eqcalls == 1)
		-- lvm.c:609-624 uses the first table's __eq, falling back to the
		-- second table's; equal metatables are not required in Lua 5.4.
		conf_cross_eq_result = (a ~= c)
		conf_cross_eq_calls = eqcalls
		local calls_before_mixed_type = eqcalls
		assert(not (a == 1) and eqcalls == calls_before_mixed_type)
		assert(rawequal(a, a) and not rawequal(a, b))

		local ord = {__lt = function(x, y) return x.v < y.v end,
		             __le = function(x, y) return x.v <= y.v end}
		local o1, o2 = setmetatable({v = 1}, ord), setmetatable({v = 2}, ord)
		assert(o1 < o2 and o1 <= o2 and not (o2 <= o1))

		local cm = {__concat = function(x, y)
			local function v(z) return type(z) == "table" and z.v or z end
			return "[" .. v(x) .. "+" .. v(y) .. "]"
		end}
		local cc = setmetatable({v = "X"}, cm)
		assert(cc .. "Y" == "[X+Y]")

		local locked = setmetatable({}, {__metatable = "locked"})
		assert(getmetatable(locked) == "locked")
		local oks, es = pcall(setmetatable, locked, {})
		assert(not oks and tostring(es):find("protected metatable", 1, true))

		local raw = setmetatable({1, 2}, {
			__index = function() return 20 end,
			__newindex = function() error("must not run") end,
			__len = function() return 50 end,
			__eq = function() return true end,
		})
		assert(rawget(raw, "absent") == nil)
		rawset(raw, "x", 9)
		assert(rawget(raw, "x") == 9 and rawlen(raw) == 2)
	`)
	if got := confGlobalString(t, L, "conf_cross_eq_result"); got != "true" {
		t.Errorf("a ~= c with the first __eq returning false = %s, want true", got)
	}
	if got := confGlobalString(t, L, "conf_cross_eq_calls"); got != "2" {
		t.Errorf("__eq across different metatables was called %s total time(s), want 2", got)
	}
}

func TestConformanceClosuresAndUpvalues(t *testing.T) {
	confRun(t, `
		local function outer(x)
			local y = x + 1
			return function(z)
				local w = z + 1
				return function() return x + y + z + w end
			end
		end
		assert(outer(1)(3)() == 10)

		local function pair()
			local x = 0
			return function(v) x = x + v; return x end,
			       function() return x end
		end
		local add, get = pair()
		assert(add(2) == 2 and add(5) == 7 and get() == 7)

		local fs = {}
		for i = 1, 3 do fs[i] = function() return i end end
		assert(fs[1]() == 1 and fs[2]() == 2 and fs[3]() == 3)

		local gs = {}
		for k, v in ipairs({10, 20, 30}) do
			gs[k] = function() return k, v end
		end
		local k1, v1 = gs[1]()
		local k3, v3 = gs[3]()
		assert(k1 == 1 and v1 == 10 and k3 == 3 and v3 == 30)
	`)
}

func TestConformanceVarargs(t *testing.T) {
	confRun(t, `
		local function probe(...)
			local n = select("#", ...)
			local second, third = select(2, ...)
			local last = select(-1, ...)
			local all = table.pack(...)
			local literal = {...}
			return n, second, third, last, all, literal
		end
		local n, second, third, last, all, literal = probe("a", nil, "c", nil)
		assert(n == 4 and second == nil and third == "c" and last == nil)
		assert(all.n == 4 and all[1] == "a" and all[2] == nil and all[3] == "c")
		assert(#literal == 3 and rawlen(literal) == 3)
		assert(select("#", nil, nil) == 2)
		assert(select(-2, "a", "b", "c") == "b")
		assert(table.pack(1, nil, 3).n == 3)
		local ok0 = pcall(select, 0, "a")
		local okn = pcall(select, -3, "a")
		local okm = pcall(select)
		assert(not ok0 and not okn and not okm)
	`)
}

func TestConformanceControlFlow(t *testing.T) {
	confRun(t, `
		local sum = 0
		for i = 1, 5 do
			if i == 3 then goto continue end
			sum = sum + i
			::continue::
		end
		assert(sum == 12)

		local seen = ""
		do
			local x = "scope"
			seen = seen .. x
			goto outside
		end
		::outside::
		assert(seen == "scope")

		local ints, floats = {}, {}
		for i = 1, 3 do ints[#ints + 1] = math.type(i) end
		for x = 1.0, 2.0, 0.5 do floats[#floats + 1] = math.type(x) end
		assert(table.concat(ints, ",") == "integer,integer,integer")
		assert(table.concat(floats, ",") == "float,float,float")

		local function iter(state, control)
			control = control + 1
			if control <= state then return control, control * control end
		end
		local total = 0
		for k, v in iter, 3, 0 do total = total + k + v end
		assert(total == 20)

		local count = 0
		repeat
			local visible = count + 1
			count = visible
		until visible == 3
		assert(count == 3)

		local b = 0
		while true do b = b + 1; if b == 4 then break end end
		assert(b == 4)

		local badgoto, egoto = load("goto L; local x = 1; ::L:: return x", "goto-scope")
		assert(badgoto == nil and egoto:find("scope", 1, true))
		local badbreak, ebreak = load("break", "break-scope")
		assert(badbreak == nil and ebreak:find("break", 1, true))
	`)
}

func TestConformanceTables(t *testing.T) {
	confRun(t, `
		assert(#{} == 0 and #{1, 2, 3} == 3)
		local hole = {[1] = "a", [3] = "c"}
		-- Lua only guarantees that #t is a border when a table has holes.
		local border = #hole
		assert((border == 1 or border == 3) and hole[border] ~= nil and hole[border + 1] == nil)
		local mixed = {10, 20, tag = "hash"}
		assert(#mixed == 2)

		local t = {2, 3}
		table.insert(t, 1, 1)
		table.insert(t, 4)
		assert(table.concat(t, ",") == "1,2,3,4")
		assert(table.remove(t, 2) == 2 and table.concat(t, ",") == "1,3,4")
		assert(table.remove(t) == 4 and table.concat(t, ",") == "1,3")

		local a = {1, 2, 3, 4, 5}
		table.move(a, 1, 3, 2)
		assert(table.concat(a, ",") == "1,1,2,3,5")
		local dest = {}
		assert(table.move(a, 2, 4, 1, dest) == dest)
		assert(table.concat(dest, ":") == "1:2:3")

		assert(table.concat({"a", "b", "c"}, "-", 2, 3) == "b-c")
		local u1, u2, u3 = table.unpack({4, 5, 6}, 1, 3)
		assert(u1 == 4 and u2 == 5 and u3 == 6)

		local nums = {5, 1, 4, 2, 3}
		table.sort(nums)
		assert(table.concat(nums, "") == "12345")
		local rows = {{k = 3}, {k = 1}, {k = 2}}
		table.sort(rows, function(x, y) return x.k > y.k end)
		assert(rows[1].k == 3 and rows[2].k == 2 and rows[3].k == 1)
		-- ltablib.c implements an unstable quicksort; equal-key order is
		-- intentionally not asserted here.
	`)
}

func TestConformanceStandardLibraries(t *testing.T) {
	confRun(t, `
		assert(string.format("%s|%d|%.2f|%g|%x|%c", "ok", -12, 3.5, 0.125, 255, 65)
		       == "ok|-12|3.50|0.125|ff|A")
		assert(string.format("%q", 'a"b') == '"a\\"b"')

		local s, e, letters, digits = string.find("xxab12yy", "(%a+)(%d+)")
		assert(s == 1 and e == 6 and letters == "xxab" and digits == "12")
		assert(string.match("x (a(b)c) y", "%b()") == "(a(b)c)")
		local lhs, rhs = string.match("name=lua54", "(%a+)=(%w+)")
		assert(lhs == "name" and rhs == "lua54")
		local p1, p2 = string.match("abc", "()b()")
		assert(p1 == 2 and p2 == 3)
		assert(string.find("a+b", "a%+b") == 1)

		local captures = {}
		for n in string.gmatch("a1 b22 c333", "%a(%d+)") do captures[#captures + 1] = n end
		assert(table.concat(captures, ",") == "1,22,333")
		local replaced, count = string.gsub("hello 123", "(%a+)%s+(%d+)", "%2:%1")
		assert(replaced == "123:hello" and count == 1)
		local mapped = string.gsub("a b c", "%a", {a = "A", b = "B"})
		assert(mapped == "A B c")
		local doubled = string.gsub("a1b2", "%d", function(d) return tonumber(d) * 2 end)
		assert(doubled == "a2b4")

		assert(string.rep("ab", 3, "-") == "ab-ab-ab")
		assert(string.sub("abcdef", -3, -2) == "de")
		local b1, b2 = string.byte("AZ", 1, 2)
		assert(b1 == 65 and b2 == 90)
		assert(string.char(65, 0, 90) == "A\0Z")
		assert(string.reverse("abc") == "cba")

		assert(math.abs(-3.5) == 3.5)
		assert(math.floor(3.9) == 3 and math.type(math.floor(3.9)) == "integer")
		assert(math.ceil(-3.9) == -3 and math.type(math.ceil(-3.9)) == "integer")
		assert(math.sqrt(81) == 9 and math.max(1, 9, 3) == 9 and math.min(1, 9, 3) == 1)
		assert(math.fmod(-7, 3) == -1)
		local mi, mf = math.modf(-3.25)
		assert(mi == -3 and mf == -0.25)
		assert(math.tointeger(3.0) == 3 and math.tointeger(3.5) == nil)
		assert(math.type(3) == "integer" and math.type(3.0) == "float")
		assert(math.abs(math.sin(math.pi / 2) - 1) < 1e-15)
		assert(math.abs(math.cos(math.pi) + 1) < 1e-15)
		assert(math.tan(0) == 0 and math.asin(0) == 0 and math.acos(1) == 0 and math.atan(0) == 0)
		assert(math.deg(math.pi) == 180 and math.rad(180) == math.pi)
		assert(math.abs(math.log(math.exp(2)) - 2) < 1e-15)
		assert(math.log(8, 2) == 3)
		assert(math.ult(1, -1) and not math.ult(-1, 1))

		local seed1, seed2 = math.randomseed(1234, 5678)
		assert(seed1 == 1234 and seed2 == 5678)
		local r1, r2, r3 = math.random(), math.random(10), math.random(-5, 5)
		assert(math.abs(r1 - 0.2804778795727) < 1e-14 and r2 == 2 and r3 == -5)
		math.randomseed(1234, 5678)
		assert(math.random() == r1 and math.random(10) == r2 and math.random(-5, 5) == r3)

		assert(os.date("!%Y-%m-%d %H:%M:%S", 0) == "1970-01-01 00:00:00")
		local timestamp = os.time({year = 2000, month = 1, day = 2, hour = 12, min = 34, sec = 56})
		local parts = os.date("*t", timestamp)
		assert(parts.year == 2000 and parts.month == 1 and parts.day == 2)
		assert(parts.hour == 12 and parts.min == 34 and parts.sec == 56)
	`)
}

func TestConformanceErrorsAndExceptions(t *testing.T) {
	confRun(t, `
		local ok0, e0 = pcall(function() error("plain", 0) end)
		assert(not ok0 and e0 == "plain")
		local ok1, e1 = pcall(function() error("located") end)
		assert(not ok1 and e1:match("^chunk:%d+: located$"))

		local marker = {code = 17}
		local okt, et = pcall(function() error(marker, 0) end)
		assert(not okt and et == marker)

		local okp, a, b, c = pcall(function(x) return x, nil, x + 1 end, 4)
		assert(okp and a == 4 and b == nil and c == 5)
		local okx, ex = xpcall(function() error("bad", 0) end,
			function(e) return "handled:" .. e end)
		assert(not okx and ex == "handled:bad")
		local okxa, xa = xpcall(function(x, y) return x * y end, tostring, 6, 7)
		assert(okxa and xa == 42)

		local aa, ab = assert("value", nil)
		assert(aa == "value" and ab == nil)
		local oka, ea = pcall(assert, false, "assert-message")
		assert(not oka and ea:find("assert%-message"))
		local object = {}
		local oko, eo = pcall(assert, false, object)
		assert(not oko and eo == object)

		local oks = pcall(select)
		assert(not oks)
	`)
}

func TestConformanceCoroutines(t *testing.T) {
	L := confRun(t, `
		local co
		co = coroutine.create(function(a)
			local self, ismain = coroutine.running()
			local x, y = coroutine.yield(a + 1, "first", self == co, ismain, coroutine.isyieldable())
			return x * 2, y
		end)
		assert(coroutine.status(co) == "suspended")
		local ok1, v1, tag, same, coismain, yieldable = coroutine.resume(co, 10)
		conf_co_first = table.concat({tostring(ok1), tostring(v1), tostring(tag),
			tostring(same), tostring(coismain), tostring(yieldable), coroutine.status(co)}, "|")
		if ok1 then
			local ok2, v2, v3 = coroutine.resume(co, 7, "done")
			conf_co_second = table.concat({tostring(ok2), tostring(v2), tostring(v3), coroutine.status(co)}, "|")
			local ok3, e3 = coroutine.resume(co)
			conf_co_dead = tostring(ok3) .. "|" .. tostring(e3)
		end

		local bad = coroutine.create(function() error("inside", 0) end)
		local okb, eb = coroutine.resume(bad)
		assert(not okb and eb == "inside" and coroutine.status(bad) == "dead")

		local wrapped = coroutine.wrap(function(x)
			local y = coroutine.yield(x + 1)
			return y + 2
		end)
		assert(wrapped(3) == 4 and wrapped(5) == 7)
		local wrapped_bad = coroutine.wrap(function() error("wrapped", 0) end)
		local okw, ew = pcall(wrapped_bad)
		assert(not okw and ew:find("wrapped", 1, true))

		local main, ismain = coroutine.running()
		assert(type(main) == "thread" and ismain and not coroutine.isyieldable())

		local close_log = {}
		local close_mt = {__close = function(self, err)
			close_log[#close_log + 1] = self.id .. ":" .. tostring(err)
		end}
		local closing = coroutine.create(function()
			local resource <close> = setmetatable({id = "resource"}, close_mt)
			coroutine.yield("paused")
		end)
		local oky, vy = coroutine.resume(closing)
		assert(oky and vy == "paused")
		local okc, ec = coroutine.close(closing)
		conf_co_close = table.concat({tostring(okc), tostring(ec), coroutine.status(closing),
			table.concat(close_log, ",")}, "|")

		local failing_close = coroutine.create(function()
			local resource <close> = setmetatable({}, {
				__close = function() error("closefail", 0) end,
			})
			coroutine.yield()
		end)
		assert(coroutine.resume(failing_close))
		local okfc, efc = coroutine.close(failing_close)
		conf_co_close_error = table.concat({tostring(okfc), tostring(efc), coroutine.status(failing_close)}, "|")
	`)
	if got := confGlobalString(t, L, "conf_co_first"); got != "true|11|first|true|false|true|suspended" {
		t.Errorf("first coroutine resume/yield tuple = %q, want %q", got, "true|11|first|true|false|true|suspended")
	}
	if got := confGlobalString(t, L, "conf_co_second"); got != "true|14|done|dead" {
		t.Errorf("second coroutine resume tuple = %q, want %q", got, "true|14|done|dead")
	}
	if got := confGlobalString(t, L, "conf_co_dead"); !strings.HasPrefix(got, "false|") || !strings.Contains(got, "dead coroutine") {
		t.Errorf("resuming dead coroutine = %q, want false plus a dead-coroutine error", got)
	}
	if got := confGlobalString(t, L, "conf_co_close"); got != "true|nil|dead|resource:nil" {
		t.Errorf("coroutine.close with <close> = %q, want %q", got, "true|nil|dead|resource:nil")
	}
	if got := confGlobalString(t, L, "conf_co_close_error"); got != "false|closefail|dead" {
		t.Errorf("coroutine.close __close error = %q, want %q", got, "false|closefail|dead")
	}
}

func TestConformanceToBeClosedAndAttributes(t *testing.T) {
	L := confRun(t, `
		local log = {}
		local mt = {__close = function(self, err)
			log[#log + 1] = self.id .. ":" .. tostring(err)
		end}
		do
			local a <close> = setmetatable({id = "a"}, mt)
			local b <close> = setmetatable({id = "b"}, mt)
		end
		conf_close_normal = table.concat(log, ",")

		log = {}
		local ok, err = pcall(function()
			local a <close> = setmetatable({id = "a"}, mt)
			local b <close> = setmetatable({id = "b"}, mt)
			error("boom", 0)
		end)
		conf_close_error_result = tostring(ok) .. ":" .. tostring(err)
		conf_close_error_log = table.concat(log, ",")

		local c <const> = 42
		assert(c == 42)
		local nullclose <close> = nil

		local badassign, eassign = load("local x <const> = 1; x = 2", "const-assign")
		assert(badassign == nil and eassign:find("const variable", 1, true))
		local badattr, eattr = load("local x <unknown> = 1", "bad-attribute")
		assert(badattr == nil and eattr:find("attribute", 1, true))
		local okvalue, evalue = pcall(function()
			local bad <close> = {}
		end)
		assert(not okvalue and tostring(evalue):find("non%-closable"))
	`)
	if got := confGlobalString(t, L, "conf_close_normal"); got != "b:nil,a:nil" {
		t.Errorf("normal <close> order/log = %q, want %q", got, "b:nil,a:nil")
	}
	if got := confGlobalString(t, L, "conf_close_error_result"); got != "false:boom" {
		t.Errorf("protected error with <close> = %q, want false:boom", got)
	}
	if got := confGlobalString(t, L, "conf_close_error_log"); got != "b:boom,a:boom" {
		t.Errorf("error <close> order/log = %q, want %q", got, "b:boom,a:boom")
	}
}

func TestConformanceErrorMessagesGoSide(t *testing.T) {
	msg := confRunError(t, "local function f() error('boom', 2) end\nf()")
	if !strings.Contains(msg, "chunk:2: boom") {
		t.Errorf("error level message = %q, want chunk:2: boom", msg)
	}
}

// TestErrorObjectSurvivesStackReset pins a Go-only hole in protected
// calls. error() panics, and a defer that runs on the way out can pop
// the Lua stack before the protected call reads the error object back.
// The object has to be the one that was thrown — here a boolean — and
// not whatever the defer left on top.
func TestErrorObjectSurvivesStackReset(t *testing.T) {
	L := confNewState(t)
	L.PushGoFunc(func(L *State) int {
		top := L.GetTop()
		defer L.SetTop(top)
		L.PushBoolean(false)
		return L.Error()
	})
	L.SetGlobal("boom")
	if err := L.DoString(`ok, err = pcall(boom)`); err != nil {
		t.Fatal(err)
	}
	if got := confGlobalString(t, L, "ok"); got != "false" {
		t.Fatalf("pcall success = %s, want false", got)
	}
	L.GetGlobal("err")
	defer L.Pop(1)
	if L.Type(-1) != LUA_TBOOLEAN || L.ToBoolean(-1) {
		t.Fatalf("pcall error is %s, want the boolean false that was thrown", TypeName(L.Type(-1)))
	}
}
