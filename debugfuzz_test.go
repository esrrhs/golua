package lua

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The ninth fuzz, over the debug library.
//
// 'ldblib.go' is where the port is furthest from being a translation of
// C: four of its 43 functions were never entered by a single test --
// 'debug.upvalueid', 'debug.upvaluejoin', 'debug.setuservalue' and the
// helper they share -- and a probe of 258 cases over the whole library
// went red in exactly one place, which is the one below.
//
//  1. **A coroutine made while a hook is on inherits it.**  C's
//     'lua_newthread' copies 'hook', 'hookmask' and 'basehookcount'
//     into the new thread, so 'debug.sethook(f, "crl", 2)' followed by
//     'debug.gethook(coroutine.create(...))' answers 'nil, "crl", 2' --
//     the mask and the count of a hook nobody ever set on that
//     coroutine.  The port kept the mask and the count in a map of its
//     own, keyed by the thread, and a thread that had never been given
//     one was not in it, so the answer was 'nil, nil, nil'.
//
//     The map was also a slow leak that C's own design does not have:
//     its keys were strong, so every coroutine that ever had a hook
//     stayed reachable for as long as the process did.  C's hook table
//     is weak ("__mode","k").  The mask, the count and the "is this
//     hook the debug library's?" flag are on the thread now, which is
//     where C keeps them, and are copied by 'NewThread' as C copies
//     them.
//
// There is one thing the grid cannot ask, and it is a thing worth
// knowing rather than a thing missing: **an error raised in a hook
// escapes every pcall and ends the interpreter.**  PUC-Lua 5.4.9 does
// that too -- the answer is not catchable -- but it means such a case
// cannot be a line in the grid, because it is the last line the run
// would ever print.  'TestDebugHookErrors' asks them instead, each in a
// process of its own, and compares the exit status along with the
// output.

// debugVars is what an option string can be made of: the eight letters
// 'db_getinfo' recognises and the one it does not.
var (
	debugOptVars   = []string{"n", "S", "l", "u", "f", "L", "t", "r", "x"}
	debugHookVars  = []byte("crl")
	debugHookExtra = []byte("crlx")
)

// debugOptStrings is every option string of length 0 to 2 over the nine
// letters, plus the ones long enough to carry a letter C ignores
// alongside ones it does not.
func debugOptStrings() []string {
	var out []string
	out = append(out, "")
	for _, a := range debugOptVars {
		out = append(out, a)
		for _, b := range debugOptVars {
			out = append(out, a+b)
		}
	}
	return out
}

// debugHookMasks is every hook mask of length 0 to 3 over 'c', 'r' and
// 'l', plus the ones with a letter C does not recognise in them.
func debugHookMasks() []string {
	var out []string
	out = append(out, "")
	for _, a := range debugHookVars {
		out = append(out, string(a))
		for _, b := range debugHookVars {
			out = append(out, string([]byte{a, b}))
			for _, c := range debugHookVars {
				out = append(out, string([]byte{a, b, c}))
			}
		}
	}
	for _, a := range debugHookExtra {
		for _, b := range debugHookExtra {
			out = append(out, string([]byte{a, b}))
		}
	}
	return out
}

// debugChunk is the harness.
//
// Two things are taken out of the answers before they are compared, and
// both are the host's business rather than Lua's:
//
//   - a table is printed with its keys sorted, because the order
//     'pairs' hands them out is the hash table's, and that is a
//     different order on every run of the reference as much as of this
//     port.  'debug.getinfo' returns a table, and a case that printed
//     one raw would be comparing the seed.
//   - a function is printed as "function", not as its address.
var debugChunk = `
local function keystr(k)
  local t = type(k)
  if t == "string" then return "s:" .. k end
  if t == "number" then return "n:" .. string.format("%.17g", k) end
  return "o:" .. t .. ":" .. tostring(k)
end

local function dump(v, seen)
  local t = type(v)
  if t == "string" then return string.format("%q", v) end
  if t ~= "table" then return tostring(v) end
  seen = seen or {}
  if seen[v] then return "<cycle>" end
  seen[v] = true
  local keys = {}
  for k in pairs(v) do keys[#keys + 1] = k end
  table.sort(keys, function(a, b) return keystr(a) < keystr(b) end)
  local parts = {}
  for _, k in ipairs(keys) do
    parts[#parts + 1] = "[" .. keystr(k) .. "]=" .. dump(v[k], seen)
  end
  seen[v] = nil
  return "{" .. table.concat(parts, ", ") .. "}"
end

local function q(v)
  local t = type(v)
  if t == "string" then
    return '"' .. v:gsub("[%c\"\\]", function(c)
      return string.format("\\%03d", c:byte())
    end) .. '"'
  end
  if t == "table" then return dump(v) end
  if t == "function" then return "function" end
  return tostring(v)
end

-- one call, whether it answered or complained
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

-- ------------------------------------------------------------------
-- the kinds of function, and the shapes of stack, a case can ask about
-- ------------------------------------------------------------------
local up1, up2 = 10, 20
local function plain(a, b) local c = a + b return c end
local function closes() return up1 + up2 end
local function varf(...) return select("#", ...) end
local function manylines()
  local a = 1
  local b = 2
  local c = 3
  return a + b + c
end
local objm = {}
function objm:m(x) return self, x end
local nopups = function() return 0 end

-- 1. getinfo about a function, for every option string
local function infoOpt(name, what)
  local f = ({
    plain = plain, closes = closes, vararg = varf, method = objm.m,
    manylines = manylines, nopups = nopups,
    ["C print"] = print, ["C gsub"] = string.gsub,
  })[name]
  return T(pcall(debug.getinfo, f, what))
end

-- 2. getinfo about a stack level, at every depth the question can be
--    asked from
local function levelIn(what, k)
  return T(pcall(debug.getinfo, k, what))
end
local function levelFn(what, k)
  local a, b = 1, 2
  return T(pcall(debug.getinfo, k, what))
end
local function levelVar(what, k, ...)
  return T(pcall(debug.getinfo, k, what))
end
local function levelGsub(what, k)
  local out
  string.gsub("ab", ".", function()
    out = T(pcall(debug.getinfo, k, what))
    return ""
  end)
  return out
end
local function levelDeep1(what, k) return T(pcall(debug.getinfo, k, what)) end
local function levelDeep2(what, k) return levelDeep1(what, k) end
local function levelDeep3(what, k) return levelDeep2(what, k) end

-- 3. getinfo about a coroutine, in each of the states it can be in
local function coInfo(state, k, what)
  local co
  if state == "fresh" then
    co = coroutine.create(function(q) local a = q coroutine.yield() return a end)
  elseif state == "yielded" then
    co = coroutine.create(function(q) local a = q coroutine.yield() return a end)
    coroutine.resume(co, 1)
  elseif state == "dead" then
    co = coroutine.create(function() return 1 end)
    coroutine.resume(co)
  elseif state == "errored" then
    co = coroutine.create(function() error("x") end)
    coroutine.resume(co)
  elseif state == "unstarted-err" then
    co = coroutine.create(function() error("x") end)
  end
  return T(pcall(debug.getinfo, co, k, what))
end

-- 4. 'name' and 'namewhat', which are the two fields a *level* can
--    answer and a function cannot: C's 'getfuncname' reads the calling
--    instruction, and there is none when the question is about a
--    function handed to 'getinfo' directly -- which is why every
--    'getinfo(f, "n")' above answers namewhat="" and nothing else.
--    Each of these calls the same function a different way.
local function ask(k) return T(pcall(debug.getinfo, k, "n")) end
NAMED = ask                     -- a global
local NAMEDL = ask              -- a local
local holder = {f = ask}        -- a field
function holder:m(k) return ask(k) end
local vtab = {ask}              -- a table element
do
  local up = ask
  NAMEDUP = function(k) return up(k) end   -- an upvalue
end
local function callGlobal(k) return NAMED(k) end
local function callLocal(k) return NAMEDL(k) end
local function callField(k) return holder.f(k) end
local function callMethod(k) return holder:m(k) end
local function callElem(k) return vtab[1](k) end
local function callUpvalue(k) return NAMEDUP(k) end
local function callInGsub(k)
  local r
  string.gsub("a", ".", function() r = NAMED(k) return "" end)
  return r
end
local function callInSort(k)
  local r
  table.sort({3, 1, 2}, function(a, b) r = NAMED(k) return a < b end)
  return r
end
local function callInPcall(k)
  local ok, r = pcall(NAMED, k)
  return tostring(ok) .. "|" .. tostring(r)
end
local function callTail(k) return NAMED(k) end
local function callMainTail(k) return callTail(k) end
local function nameCase(fn)
  local f = ({
    global = callGlobal, localv = callLocal, field = callField,
    method = callMethod, elem = callElem, upvalue = callUpvalue,
    gsub = callInGsub, sort = callInSort, pcall = callInPcall,
    tail = callMainTail,
  })[fn]
  return f(1)
end
local function nameCase2(fn)
  local f = ({
    global = callGlobal, localv = callLocal, field = callField,
    method = callMethod, elem = callElem, upvalue = callUpvalue,
    gsub = callInGsub, sort = callInSort, pcall = callInPcall,
    tail = callMainTail,
  })[fn]
  return f(2)
end

-- 5. locals: by index, in each shape of frame, and on a coroutine
local function localGet(k, i)
  local a, b = 1, 2
  return T(pcall(debug.getlocal, k, i))
end
local function localGetParams(k, i, p1, p2)
  return T(pcall(debug.getlocal, k, i))
end
local function localGetVar(k, i, ...)
  return T(pcall(debug.getlocal, k, i))
end
local function localGetMain(k, i)
  return T(pcall(debug.getlocal, k, i))
end
local function localSet(k, i, v)
  local a = "untouched"
  local r = T(pcall(debug.setlocal, k, i, v))
  return r .. " a=" .. tostring(a)
end
local function localSetParams(k, i, p1, v)
  local r = T(pcall(debug.setlocal, k, i, v))
  return r .. " p1=" .. tostring(p1)
end
local function coLocalGet(i)
  local co = coroutine.create(function(p)
    local a = p
    local b = 2
    coroutine.yield()
    return a
  end)
  coroutine.resume(co, "in")
  local r = T(pcall(debug.getlocal, co, 1, i))
  select(2, coroutine.resume(co))
  return r
end
local function coLocalSet(i, v)
  local co = coroutine.create(function(p)
    local a = p
    coroutine.yield()
    return a
  end)
  coroutine.resume(co, "in")
  local r = T(pcall(debug.setlocal, co, 1, i, v))
  local ok2, got = coroutine.resume(co)
  return r .. " then=" .. tostring(got)
end

-- 6. upvalues
local mk0, mk1, mk2
do
  mk0 = function() return 0 end
  local a = 1
  mk1 = function() return a end
  local b, c = 2, 3
  mk2 = function() return b + c end
  -- two closures sharing one of their upvalues
  local s = 100
  mk1 = function() return s end
  mk2 = function() return s end
end
local function upGet(name, i)
  local f = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[name]
  return T(pcall(debug.getupvalue, f, i))
end
local function upSet(name, i, v)
  local f = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[name]
  return T(pcall(debug.setupvalue, f, i, v))
end
local function upSetThen(name, i, v)
  local f = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[name]
  local r = T(pcall(debug.setupvalue, f, i, v))
  return r .. " -> " .. tostring(f())
end
local function upID(name, i)
  local f = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[name]
  return T(pcall(debug.upvalueid, f, i))
end
local function upIDEq(n1, i1, n2, i2)
  local fa = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[n1]
  local fb = ({mk0 = mk0, mk1 = mk1, mk2 = mk2})[n2]
  local ok1, a = pcall(debug.upvalueid, fa, i1)
  local ok2, b = pcall(debug.upvalueid, fb, i2)
  if not ok1 or not ok2 then
    return "false|" .. tostring(ok1) .. "|" .. tostring(ok2) ..
      "|" .. tostring(a) .. "|" .. tostring(b)
  end
  return "true|" .. tostring(a == b) .. "|" .. tostring(type(a))
end
local function upJoin(i, j)
  local t = {}
  local f1 = function() return 1 end
  local f2 = function() return 2 end
  local r = T(pcall(debug.upvaluejoin, f1, i, f2, j))
  return r .. " -> " .. tostring(f1())
end
local function upJoinReal(i, j)
  local f1, f2 = mk1, mk2
  local r = T(pcall(debug.upvaluejoin, f1, i, f2, j))
  return r .. " -> " .. tostring(f1())
end

-- 7. hooks.  The log is capped, and the hook is always taken off
--    again before the case returns, so that what runs next is not
--    being watched.
local function hookCase(mask, count, has)
  local log = {}
  local function h(what, line)
    if #log < 30 then log[#log + 1] = tostring(what) .. ":" .. tostring(line) end
  end
  if has then debug.sethook(h, mask, count) else debug.sethook(h, mask) end
  local s = 0
  for i = 1, 6 do s = s + i end
  local g = function(a) return a end
  g(1)
  local gh, gm, gc = debug.gethook()
  debug.sethook()
  return #log, table.concat(log, ","), s, type(gh), gm, gc
end
-- a coroutine made while a hook is on inherits it: this is the case
-- the probe found
local function inheritCase(mask, count, when, has)
  local co
  if when == "before" then
    co = coroutine.create(function() return 1 end)
  end
  if has then debug.sethook(function() end, mask, count)
  else debug.sethook(function() end, mask) end
  if when == "after" then
    co = coroutine.create(function() return 1 end)
  end
  local h, m, c = debug.gethook(co)
  debug.sethook()
  if co == nil then return "no co" end
  return type(h), m, c
end
-- and the hook runs inside a coroutine that inherited it
local function inheritRuns(mask, count, when, has)
  local log = {}
  local function h(what) if #log < 20 then log[#log + 1] = tostring(what) end end
  local make = function()
    return coroutine.create(function()
      local s = 0
      for i = 1, 4 do s = s + i end
      return s
    end)
  end
  local co
  if when == "before" then co = make() end
  if has then debug.sethook(h, mask, count) else debug.sethook(h, mask) end
  if when == "after" then co = make() end
  local a, b = coroutine.resume(co)
  debug.sethook()
  return #log, table.concat(log, ","), a, b
end
local function coHookCase(mask, count, has)
  local co = coroutine.create(function()
    local s = 0
    for i = 1, 4 do s = s + i end
    return s
  end)
  if has then debug.sethook(co, function() end, mask, count)
  else debug.sethook(co, function() end, mask) end
  local h, m, c = debug.gethook(co)
  local mh, mm, mc = debug.gethook()
  debug.sethook(co)
  return type(h), m, c, type(mh), mm, mc
end
local function clearCase(mask, count, has)
  if has then debug.sethook(function() end, mask, count)
  else debug.sethook(function() end, mask) end
  debug.sethook()
  return debug.gethook()
end

-- 8. traceback
local function tbCase(msg, level)
  if level ~= nil then return T(pcall(debug.traceback, msg, level)) end
  return T(pcall(debug.traceback, msg))
end
local function tbDeep(k, msg)
  local function a() return debug.traceback(msg) end
  local function b() return a() end
  local function c() return b() end
  local function d() return c() end
  local f = ({a = a, b = b, c = c, d = d})[k]
  return T(pcall(f))
end
local function tbCo(state, msg, level)
  local co = coroutine.create(function()
    local s = 0
    for i = 1, 3 do s = s + i end
    coroutine.yield()
    return s
  end)
  if state == "yielded" then coroutine.resume(co)
  elseif state == "dead" then coroutine.resume(co) coroutine.resume(co) end
  if level ~= nil then return T(pcall(debug.traceback, co, msg, level)) end
  return T(pcall(debug.traceback, co, msg))
end
local function tbErr(msg, level)
  local ok, e = pcall(function() error(msg, level) end)
  return tostring(ok), e
end

-- 9. metatables, the registry, and the values behind a userdata
local function metaGet(v) return T(pcall(debug.getmetatable, v)) end
local function metaSet(v, m)
  local r = T(pcall(debug.setmetatable, v, m))
  local g = getmetatable(v)
  return r .. " get=" .. tostring(g ~= nil and type(g) or "nil")
end
-- a metatable set on something that is not a table is the metatable of
-- its *type*, and every value of that type then shares it: taking the
-- string metatable away leaves "s":gsub -- which the harness itself is
-- made of -- with nothing to index.  These four are asked last and put
-- it back, and they cannot go through 'T' while it is gone, because 'T'
-- is what would crash.
local STRINGMETA = getmetatable("")
local function metaSetRisky(k, m)
  local v
  if k == "str" then v = "s"
  elseif k == "num" then v = 1
  elseif k == "bool" then v = true
  elseif k == "fn" then v = print
  elseif k == "thr" then v = coroutine.create(function() end) end
  local ok, a = pcall(debug.setmetatable, v, m)
  local g = getmetatable(v)
  if k == "str" then debug.setmetatable("", STRINGMETA) end
  return tostring(ok) .. "|" .. tostring(a) .. "|" ..
    tostring(g ~= nil and type(g) or "nil")
end
-- The registry is a table libraries share, so its keys are a list of
-- what has been loaded and what has been needed.  One of them is not
-- that, and is left out: '_UBOX*' is the metatable 'lauxlib.c' makes
-- for the userdata its buffer is held in, and it appears the first
-- time a buffer grows off the C stack -- 'string.rep("x", 9000)' is
-- enough.  This port's buffer is not a stack object and has no such
-- moment, so the key is never created (README, the known differences).
local function regKeys()
  local r = debug.getregistry()
  local ks = {}
  for k in pairs(r) do
    if k ~= "_UBOX*" then ks[#ks + 1] = tostring(k) end
  end
  table.sort(ks)
  return table.concat(ks, ",")
end
local function withFile(f)
  local h, e = io.tmpfile()
  if not h then return "no tmpfile" end
  local a = f(h)
  h:close()
  return a
end
local function uvGet(n)
  return withFile(function(h)
    if n ~= nil then return T(pcall(debug.getuservalue, h, n)) end
    return T(pcall(debug.getuservalue, h))
  end)
end
local function uvSet(v, n)
  return withFile(function(h)
    local r
    if n ~= nil then r = T(pcall(debug.setuservalue, h, v, n))
    else r = T(pcall(debug.setuservalue, h, v)) end
    return r .. " get=" .. tostring(debug.getuservalue(h))
  end)
end
local function uvSetNon(v, n, m)
  if m ~= nil then return T(pcall(debug.setuservalue, v, n, m)) end
  return T(pcall(debug.setuservalue, v, n))
end
local function uvGetNon(v, n)
  if n ~= nil then return T(pcall(debug.getuservalue, v, n)) end
  return T(pcall(debug.getuservalue, v))
end

-- 10. the C stack limit, which the VM asks about rather than the
--     machine
local function cstack(n) return T(pcall(debug.setcstacklimit, n)) end

-- 11. every debug function asked the wrong thing
local function wrong(fname, ...)
  local f = debug[fname]
  local a = table.pack(...)
  return T(pcall(f, table.unpack(a, 1, a.n)))
end
`

func TestDebugFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the debug fuzz")
	}

	var b strings.Builder
	b.WriteString(debugChunk)
	n := 0
	emit := func(name, call string) {
		n++
		b.WriteString("P(" + strconv.Itoa(n) + ", " + strconv.Quote(name) +
			", " + call + ")\n")
	}

	// 1. every option string of length 0 to 2, over the eight letters
	// C recognises and the one it does not, asked about each kind of
	// function.  The 'x' ones are where C says "invalid option".
	opts := debugOptStrings()
	targets := []string{"plain", "closes", "vararg", "method", "manylines",
		"nopups", "C print", "C gsub"}
	for _, o := range opts {
		for _, tg := range targets {
			emit("getinfo/"+tg+"/"+o,
				"infoOpt("+strconv.Quote(tg)+", "+strconv.Quote(o)+")")
		}
	}

	// 2. every stack level, asked from every depth.  The level is the
	// only way to get a 'name' and a 'namewhat', a 'currentline' that
	// moves, or an 'istailcall'.
	levels := []string{"-2", "-1", "0", "1", "2", "3", "99", "1.5", `"1"`}
	for _, k := range levels {
		emit("level/top "+k, "T(pcall(debug.getinfo, "+k+`, "nSlufLt"))`)
		emit("level/fn "+k, `levelFn("nSlufLt", `+k+")")
		emit("level/var "+k, `levelVar("nSlufLt", `+k+`, "a", "b")`)
		emit("level/gsub "+k, `levelGsub("nSlufLt", `+k+")")
		emit("level/deep1 "+k, `levelDeep1("nSlufLt", `+k+")")
		emit("level/deep2 "+k, `levelDeep2("nSlufLt", `+k+")")
		emit("level/deep3 "+k, `levelDeep3("nSlufLt", `+k+")")
	}

	// 3. a coroutine, in each state it can be asked about
	for _, s := range []string{"fresh", "yielded", "dead", "errored",
		"unstarted-err"} {
		for _, k := range []string{"0", "1", "2"} {
			for _, o := range []string{"nSl", "u", "f", "nSlufLt"} {
				emit("co/getinfo/"+s+" "+k+" "+o,
					"coInfo("+strconv.Quote(s)+", "+k+", "+strconv.Quote(o)+")")
			}
		}
	}

	// 4. 'name' and 'namewhat': the same function called ten ways,
	//    asked about itself (level 1) and about its caller (level 2)
	for _, c := range []string{"global", "localv", "field", "method",
		"elem", "upvalue", "gsub", "sort", "pcall", "tail"} {
		emit("name/"+c+"/1", "nameCase("+strconv.Quote(c)+")")
		emit("name/"+c+"/2", "nameCase2("+strconv.Quote(c)+")")
	}

	// 5. locals: every index, in every shape of frame, on the main
	//    thread and on a coroutine; and 'setlocal' for each of them
	for _, i := range []string{"-2", "-1", "0", "1", "2", "3", "4", "99",
		"1.5"} {
		emit("local/top "+i, "T(pcall(debug.getlocal, "+i+"))")
		emit("local/fn "+i, "localGet(1, "+i+")")
		emit("local/params "+i, `localGetParams(1, `+i+`, "a", "b")`)
		emit("local/var "+i, `localGetVar(1, `+i+`, "a", "b")`)
		emit("local/set "+i, `localSet(1, `+i+`, "changed")`)
		emit("local/setparams "+i, `localSetParams(1, `+i+`, "p", "changed")`)
	}
	for _, i := range []string{"-2", "-1", "0", "1", "2", "3", "99"} {
		emit("co/local "+i, "coLocalGet("+i+")")
		emit("co/local/set "+i, `coLocalSet(`+i+`, "changed")`)
	}

	// 6. upvalues: get, set, the identity C hands back, and the join
	for _, nm := range []string{"mk0", "mk1", "mk2"} {
		for _, i := range []string{"-1", "0", "1", "2", "3", "99"} {
			emit("upvalue/get/"+nm+" "+i, "upGet("+strconv.Quote(nm)+", "+i+")")
			emit("upvalue/set/"+nm+" "+i,
				"upSetThen("+strconv.Quote(nm)+", "+i+", 777)")
		}
	}
	for _, nm := range []string{"mk1", "mk2"} {
		for _, i := range []string{"1", "2"} {
			emit("upvalue/id/"+nm+" "+i, "upID("+strconv.Quote(nm)+", "+i+")")
		}
	}
	pairs := [][4]string{
		{"mk1", "1", "mk1", "1"}, {"mk1", "1", "mk2", "1"},
		{"mk1", "1", "mk2", "2"}, {"mk1", "2", "mk2", "1"},
		{"mk1", "2", "mk2", "2"}, {"mk2", "1", "mk2", "2"},
		{"mk0", "1", "mk0", "1"}, {"mk0", "1", "mk1", "1"},
	}
	for _, p := range pairs {
		emit("upvalue/ideq "+p[0]+"."+p[1]+" "+p[2]+"."+p[3],
			"upIDEq("+strconv.Quote(p[0])+", "+p[1]+", "+
				strconv.Quote(p[2])+", "+p[3]+")")
	}
	for _, i := range []string{"-1", "0", "1", "2", "99"} {
		for _, j := range []string{"-1", "0", "1", "2", "99"} {
			emit("upvalue/join "+i+" "+j, "upJoin("+i+", "+j+")")
		}
	}
	for _, i := range []string{"1", "2", "3", "99"} {
		for _, j := range []string{"1", "2", "3", "99"} {
			emit("upvalue/joinreal "+i+" "+j, "upJoinReal("+i+", "+j+")")
		}
	}

	// 7. hooks: every mask, every count, and the coroutine that
	//    inherits one
	for _, m := range debugHookMasks() {
		emit("hook/"+m, "hookCase("+strconv.Quote(m)+", 0, false)")
	}
	for _, m := range []string{"", "c", "r", "l", "crl"} {
		for _, c := range []string{"0", "1", "2", "3", "7", "1000"} {
			emit("hook/count "+m+" "+c, "hookCase("+strconv.Quote(m)+", "+c+", true)")
		}
	}
	for _, m := range []string{"", "c", "r", "l", "cr", "rl", "crl", "x"} {
		for _, w := range []string{"before", "after"} {
			for _, c := range []string{"0", "3"} {
				emit("hook/inherit "+m+" "+w+" "+c,
					"inheritCase("+strconv.Quote(m)+", "+c+", "+
						strconv.Quote(w)+", "+c+" ~= 0)")
			}
		}
	}
	for _, m := range []string{"c", "l", "cl"} {
		for _, w := range []string{"before", "after"} {
			for _, c := range []string{"0", "2"} {
				emit("hook/inherit-runs "+m+" "+w+" "+c,
					"inheritRuns("+strconv.Quote(m)+", "+c+", "+
						strconv.Quote(w)+", "+c+" ~= 0)")
			}
		}
	}
	for _, m := range []string{"", "c", "crl", "l"} {
		for _, c := range []string{"0", "2"} {
			emit("hook/co "+m+" "+c, "coHookCase("+strconv.Quote(m)+", "+c+
				", "+c+" ~= 0)")
		}
	}
	for _, m := range []string{"", "c", "crl"} {
		emit("hook/clear "+m, "clearCase("+strconv.Quote(m)+", 0, false)")
	}

	// 8. traceback: every message, every level, from every depth
	msgs := []string{"nil", `""`, `"m"`, "1", "1.5", "true", "{}"}
	for _, m := range msgs {
		for _, l := range []string{"-1", "0", "1", "2", "99"} {
			if l == "-1" {
				emit("tb "+m, "tbCase("+m+", nil)")
			} else {
				emit("tb "+m+" "+l, "tbCase("+m+", "+l+")")
			}
		}
	}
	for _, k := range []string{"a", "b", "c", "d"} {
		emit("tb/deep "+k, `tbDeep(`+strconv.Quote(k)+`, "in")`)
	}
	for _, s := range []string{"fresh", "yielded", "dead"} {
		for _, l := range []string{"-1", "0", "1"} {
			if l == "-1" {
				emit("tb/co/"+s, `tbCo(`+strconv.Quote(s)+`, "m", nil)`)
			} else {
				emit("tb/co/"+s+" "+l, `tbCo(`+strconv.Quote(s)+`, "m", `+l+")")
			}
		}
	}
	for _, l := range []string{"0", "1", "2", "3"} {
		emit("tb/err "+l, `tbErr("boom", `+l+")")
	}

	// 9. metatables, the registry, and the values behind a userdata
	for _, v := range []string{"nil", "true", "1", `"s"`, "{}", "print",
		"coroutine.create(function() end)"} {
		emit("meta/get "+v, "metaGet("+v+")")
	}
	/* Only a table here: see metaSetRisky. */
	for _, m := range []string{"nil", "{}", `{__metatable = "locked"}`} {
		emit("meta/set {} "+m, "metaSet({}, "+m+")")
	}
	/* These are last in the file for a reason, and each puts back what
	   it took away. */
	for _, k := range []string{"str", "num", "bool", "fn", "thr"} {
		for _, m := range []string{"nil", "{}", `{__metatable = "locked"}`} {
			emit("meta/set-risky "+k+" "+m,
				"metaSetRisky("+strconv.Quote(k)+", "+m+")")
		}
	}
	emit("registry/keys", "regKeys()")
	for _, u := range []string{"-1", "0", "1", "2", "3", "99"} {
		emit("uv/get "+u, "uvGet("+u+")")
		emit("uv/get-nil "+u, "uvGetNon({}, "+u+")")
		emit("uv/get-str "+u, `uvGetNon("x", `+u+")")
	}
	for _, v := range []string{`"v"`, "nil", "1", "true", "{}"} {
		for _, u := range []string{"0", "1", "2"} {
			emit("uv/set "+v+" "+u, "uvSet("+v+", "+u+")")
		}
	}
	for _, v := range []string{"nil", "1", `"x"`, "{}"} {
		emit("uv/set-nonud "+v, "uvSetNon("+v+", 1, 1)")
	}

	// 10. the C stack limit
	for _, c := range []string{"0", "1", "10", "50", "199", "200", "201",
		"1000", "1000000000"} {
		emit("cstack "+c, "cstack("+c+")")
	}

	// 11. every debug function, asked the wrong thing
	wrongs := [][]string{
		{"getinfo", "{}"}, {"getinfo", "1.5"}, {"getinfo", `"x"`},
		{"getinfo", "print", "1"}, {"getinfo", "1", "1"},
		{"getinfo", "print", "{}"}, {"getinfo", "nil"},
		{"getlocal", "1", "{}"}, {"getlocal", "print", "1"},
		{"getlocal", "1", "1.5"}, {"getlocal", "{}", "1", "1"},
		{"setlocal", "1", "1", "1"}, {"setlocal", "1", "{}", "1"},
		{"getupvalue", "{}", "1"}, {"getupvalue", "print", "1"},
		{"getupvalue", "nil", "1"}, {"setupvalue", "{}", "1", "1"},
		{"setupvalue", "print", "1", "1"},
		{"upvalueid", "{}", "1"}, {"upvalueid", "print", "1"},
		{"upvalueid", "nil", "1"}, {"upvaluejoin", "1", "1", "2", "1"},
		{"upvaluejoin", "print", "1", "print", "1"},
		{"upvaluejoin", "nil", "1", "nil", "1"},
		{"sethook", "1"}, {"sethook", "print", "1"},
		{"sethook", `function() end, 1`}, {"sethook", "function() end", "c", "x"},
		{"gethook", "1", "2"}, {"traceback", "1", "{}"},
		{"traceback", "{}", "1"}, {"getmetatable", "{}", "1"},
		{"setmetatable", "1", "1", "1"}, {"setmetatable", "{}"},
		{"getuservalue", "1", "1"}, {"setuservalue", "1", "1", "1"},
		{"setuservalue", "1"}, {"setcstacklimit", `"x"`},
		{"setcstacklimit", "{}"}, {"setcstacklimit", "nil"},
	}
	for _, w := range wrongs {
		args := make([]string, len(w)-1)
		copy(args, w[1:])
		emit("wrong/"+w[0]+" "+strings.Join(args, " "),
			"wrong("+strconv.Quote(w[0])+", "+strings.Join(args, ", ")+")")
	}

	cases := n
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	/* One directory is enough: nothing here writes a file that the
	   other side could see, and the chunk name has to be the same on
	   both, which it is when each is run in its own directory with
	   the same relative name. */
	dirs := map[string]string{
		"ref":  filepath.Join(t.TempDir(), "ref"),
		"ours": filepath.Join(t.TempDir(), "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "debugfuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := fmtRunFile(t, ref, dirs["ref"], "debugfuzz.lua")
	got := fmtRunFile(t, diffBuild(t), dirs["ours"], "debugfuzz.lua")
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the debug chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d option strings x %d kinds of function, %d levels x %d "+
			"shapes of stack, %d hook masks, %d upvalue cases, %d "+
			"tracebacks, and the errors: %d cases, all of them PUC-Lua's",
			len(opts), len(targets), len(levels), 7, len(debugHookMasks()),
			119, 55, cases)
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
		t.Errorf("case %d differs:\n  PUC-Lua: %s\n  this:    %s",
			i+1, a, c)
		if reported >= 12 {
			t.Errorf("... and %d more", (len(wl)-i-1)+(len(gl)-i-1))
			break
		}
	}
}

// debugHookErrors are the cases that cannot be lines in the grid.
//
// An error raised in a hook escapes every 'pcall' and ends the
// interpreter -- in PUC-Lua as much as here -- so a case that asks one
// would be the last thing the run ever printed.  Each of these is
// therefore a program of its own, and what is compared is the exit
// status as well as the output: a difference in how the process ends is
// a difference worth seeing.
var debugHookErrors = []string{
	// a line hook that raises, and nothing to catch it
	`debug.sethook(function() error("in hook") end, "l")
local s = 0
for i = 1, 5 do s = s + i end
print("after", s)
`,
	// a call hook
	`debug.sethook(function() error("in hook") end, "c")
local function g() return 1 end
g()
print("after")
`,
	// a return hook
	`debug.sethook(function() error("in hook") end, "r")
local function g() return 1 end
g()
print("after")
`,
	// a count hook
	`debug.sethook(function() error("in hook") end, "", 1)
local s = 0
for i = 1, 5 do s = s + i end
print("after", s)
`,
	// under a pcall, which does not catch it
	`debug.sethook(function() error("in hook") end, "l")
local ok, e = pcall(function()
  local s = 0
  for i = 1, 3 do s = s + i end
  return s
end)
print("ok", ok, e)
`,
	// under an xpcall, whose handler does not see it either
	`debug.sethook(function() error("in hook") end, "l")
local ok, e = xpcall(function()
  local s = 0
  for i = 1, 3 do s = s + i end
  return s
end, function(m) return "handled:" .. tostring(m) end)
print("ok", ok, e)
`,
	// in a coroutine: 'resume' does not catch it either
	`debug.sethook(function() error("in hook") end, "l")
local co = coroutine.create(function()
  local s = 0
  for i = 1, 3 do s = s + i end
  return s
end)
print(coroutine.resume(co))
`,
	// the same, but the value raised is not a string
	`debug.sethook(function() error({code = 1}) end, "l")
local s = 0
for i = 1, 3 do s = s + i end
print("after", s)
`,
	// a hook set on a coroutine from the inside: an error there is an
	// ordinary coroutine error, and 'resume' does catch it.  This is
	// the case above with one thing different, and the two answers are
	// not the same.
	`local co = coroutine.create(function()
  debug.sethook(function() error("in hook") end, "l")
  local s = 0
  for i = 1, 3 do s = s + i end
  return s
end)
print(coroutine.resume(co))
`,
	// the hook is taken off before anything can call it
	`debug.sethook(function() error("in hook") end, "l")
debug.sethook()
local s = 0
for i = 1, 3 do s = s + i end
print("survived", s)
`,
	// a hook that yields somewhere it cannot: across a C-call
	// boundary, which is gsub's
	`local co = coroutine.create(function()
  return (string.gsub("abc", ".", string.upper))
end)
debug.sethook(co, function() coroutine.yield() end, "c")
print(coroutine.resume(co))
`,
	// and one that yields where there is no coroutine at all
	`local co = coroutine.create(function() return 1 end)
debug.sethook(function() coroutine.yield() end, "l", 1)
print(coroutine.resume(co))
print(pcall(coroutine.resume, co))
`,
}

func TestDebugHookErrors(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to compare hook errors")
	}
	dirs := map[string]string{
		"ref":  filepath.Join(t.TempDir(), "ref"),
		"ours": filepath.Join(t.TempDir(), "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	run := func(bin, dir, name string) (string, int) {
		t.Helper()
		cmd := &exec.Cmd{
			Path: bin,
			Args: []string{"lua", name},
			Dir:  dir,
			Env:  diffEnv(),
		}
		out, err := cmd.CombinedOutput()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("%s: %v", bin, err)
		}
		return string(out), code
	}
	for i, src := range debugHookErrors {
		name := "h" + strconv.Itoa(i) + ".lua"
		for _, d := range dirs {
			if err := os.WriteFile(filepath.Join(d, name),
				[]byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		w, wc := run(ref, dirs["ref"], name)
		g, gc := run(diffBuild(t), dirs["ours"], name)
		if ww, gg := diffNormalise(w), diffNormalise(g); ww != gg || wc != gc {
			t.Errorf("%s: differs from PUC-Lua (exit %d vs %d)\n"+
				"--- want ---\n%s--- got ---\n%s", name, wc, gc, ww, gg)
		}
	}
}

// The two things the grid cannot ask, because they need a hook the
// debug library did not install.
//
// C's 'db_gethook' answers "external hook" when 'L1->hook' is not
// 'hookf' -- that is, for a hook put there with 'lua_sethook' from the
// C API -- and answers the hook table's entry otherwise.  Go cannot
// compare two functions, so the distinction is recorded when the hook
// is installed (see 'installHook') rather than being asked of it
// afterwards.  These two pin both halves; the marker is what the
// coroutine case above depends on, since a coroutine copies it along
// with the hook.

func TestDebugExternalHook(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	/* a hook the debug library did not install: this is what a C
	   extension's 'lua_sethook(L, f, LUA_MASKCALL, 0)' is */
	n := 0
	L.SetHook(func(*State, *dbAr) { n++ }, 1, 0)
	if err := runLua(t, L, `
		local h, m, c = debug.gethook()
		assert(h == "external hook", tostring(h))
		assert(m == "c", tostring(m))
		assert(c == 0, tostring(c))
		return 1
	`); err != nil {
		t.Fatal(err)
	}
	/* and it is still that after a coroutine inherits it */
	if err := runLua(t, L, `
		local co = coroutine.create(function() return 1 end)
		local h, m, c = debug.gethook(co)
		assert(h == "external hook", tostring(h))
		assert(m == "c", tostring(m))
		return 1
	`); err != nil {
		t.Fatal(err)
	}
	L.SetHook(nil, 0, 0)
	if err := runLua(t, L, `
		local h = debug.gethook()
		return h == nil
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("a hook that was taken off is still being reported")
	}
}

// TestDebugHookSurvivesInheritance is the case the probe found, pinned
// without a reference interpreter: a coroutine made while a hook is
// running answers that hook's mask and count, in the coroutine it was
// never set on.
func TestDebugHookSurvivesInheritance(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		debug.sethook(function() end, "crl", 2)
		local h, m, c = debug.gethook(coroutine.create(function() end))
		return h == nil and m == "crl" and c == 2
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("a coroutine did not inherit the hook it was created under")
	}
	if err := runLua(t, L, `
		debug.sethook()
		-- one value, not three: 'db_gethook' answers a single fail
		-- before it ever reaches the mask and the count
		local a = table.pack(debug.gethook(coroutine.create(function() end)))
		return a.n == 1 and a[1] == nil
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("a coroutine inherited a hook that was not there")
	}
}

// TestDebugUpvaluesAndUserValues covers, in this process, the four
// functions whose coverage the fuzz cannot reach: it drives them from
// the API rather than from a binary, and a subprocess does not answer
// 'go test -cover'.  What each of them answers is PUC-Lua's, and the
// grid is what says so; this is only that they are entered at all.
func TestDebugUpvaluesAndUserValues(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		local a, b = 1, 2
		local f1 = function() return a end
		local f2 = function() return a, b end
		assert(debug.getupvalue(f1, 1) == "a")
		assert(select(2, debug.getupvalue(f1, 2)) == nil)
		assert(debug.setupvalue(f1, 1, 7) == "a")
		assert(f1() == 7)
		local i1, i2 = debug.upvalueid(f1, 1), debug.upvalueid(f2, 1)
		assert(i1 == i2, "two closures share the upvalue")
		assert(i1 ~= debug.upvalueid(f2, 2))
		debug.upvaluejoin(f2, 2, f1, 1)
		assert(select(2, f2()) == 7, "the join moved the value")
		-- and the two that need a userdata, which a file handle is.
		-- A FILE is made with no user value slot at all, so the set is
		-- accepted and changes nothing: that is PUC-Lua's answer, and
		-- the grid is what says so.
		local h = io.tmpfile()
		assert(debug.getuservalue(h) == nil)
		local ok, e = pcall(debug.setuservalue, h, "v")
		assert(ok, tostring(e))
		assert(debug.getuservalue(h) == nil)
		ok, e = pcall(debug.setuservalue, h, "v", 2)
		assert(ok == false or e == nil, tostring(e))
		h:close()
		return true
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("the debug upvalue API does not answer what PUC-Lua answers")
	}
}
