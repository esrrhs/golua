-- The debug library taken apart: every 'what' field of 'getinfo', the
-- numbered and named ways of reaching a local or an upvalue, and the
-- shapes 'traceback' produces.  Chunk names are chosen with '=' so that
-- they do not vary with how the file is named.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end
local function P(...) print(...) end

-- getinfo reduced to comparable text.
local function info(what, ...)
  local t = debug.getinfo(..., what)
  if t == nil then return "nil" end
  local keys = {"source", "short_src", "linedefined", "lastlinedefined",
                "what", "currentline", "name", "namewhat", "nups", "nparams",
                "isvararg", "istailcall", "ntransfer", "ftransfer", "func"}
  local out = {}
  for i = 1, #keys do
    local k = keys[i]
    local v = t[k]
    if v ~= nil then
      if k == "func" then out[#out + 1] = "func=" .. type(v)
      else out[#out + 1] = k .. "=" .. tostring(v) end
    end
  end
  return table.concat(out, " ")
end

print("== getinfo: every field ==")
do
  local function target(a, b) return debug.getinfo(1, "SlunftL") end
  -- Asking about a level, one field at a time so an absent field shows.
  local function probe(w) return debug.getinfo(1, w) end
  local t = probe("n")
  P("n", type(t), t.name, t.namewhat)
  t = probe("S")
  P("S", t.source, t.short_src, t.what, t.linedefined, t.lastlinedefined)
  t = probe("l")
  P("l", type(t.currentline), (t.currentline or 0) > 0)
  t = probe("u")
  P("u", t.nups, t.nparams, t.isvararg)
  t = probe("f")
  P("f", type(t.func))
  t = probe("t")
  P("t", t.istailcall, t.ntransfer, t.ftransfer)
  t = probe("L")
  P("L", type(t.activelines))
  t = probe("")
  P("empty", type(t), (next(t) == nil))
  -- A letter it does not know is an error, not an empty table.
  P("unknown", show(pcall(debug.getinfo, 1, "Z")))
  P("unknown2", show(pcall(debug.getinfo, 1, "SlunZ")))
end
-- A C function, and the main chunk.
do
  local t = debug.getinfo(print, "Sn")
  P("C fn", t.source, t.short_src, t.what, t.linedefined, t.lastlinedefined, t.name, t.namewhat, t.nups)
  P("C fn u", (debug.getinfo(print, "u")).nparams, (debug.getinfo(print, "u")).isvararg)
  local t2 = debug.getinfo(1, "S")
  P("main", t2.what, t2.linedefined, t2.lastlinedefined)
end
-- Level numbers: 0 is the C function doing the asking.
do
  local function f1() return debug.getinfo(1, "n").name, debug.getinfo(2, "n").name end
  P("names", f1())
  P("level0", show(pcall(debug.getinfo, 0, "n")))
  P("level99", show(pcall(debug.getinfo, 99, "n")))
  P("level-1", show(pcall(debug.getinfo, -1, "n")))
  P("level1.0", show(pcall(debug.getinfo, 1.0, "n")))
  P("level1.5", show(pcall(debug.getinfo, 1.5, "n")))
  P("level'1'", show(pcall(debug.getinfo, "1", "n")))
  P("no what", show(pcall(debug.getinfo, 1)))
  P("no args", show(pcall(debug.getinfo)))
  P("bad arg", show(pcall(debug.getinfo, {})))
  P("bad what", show(pcall(debug.getinfo, 1, 1)))
end
-- A function loaded from a string keeps the chunk name it was given.
do
  local f = load("return 1", "=mychunk")
  local t = debug.getinfo(f, "S")
  P("named", t.source, t.short_src, t.what)
  local g = load("return 1", "mychunk")
  t = debug.getinfo(g, "S")
  P("plain", t.source, t.short_src, t.what)
  local h = load("return 1")
  t = debug.getinfo(h, "S")
  P("noname", t.source, t.short_src, t.what)
  local i = load(string.rep("local x\n", 30) .. "return 1", "=long")
  t = debug.getinfo(i, "S")
  P("long", t.source, t.short_src)
end

print("== getlocal / setlocal ==")
do
  local up = "an upvalue"
  local function outer(a, ...)
    local x, y = 1, 2
    local out = {}
    local i = 1
    while true do
      local n, v = debug.getlocal(1, i)
      if not n then break end
      out[#out + 1] = n .. "=" .. tostring(v)
      i = i + 1
    end
    return table.concat(out, ","), up
  end
  P("locals", outer(9, "v1", "v2"))
  -- Writing through a level.
  local function w()
    local x = 1
    debug.setlocal(1, 1, 99)
    return x
  end
  P("setlocal", w())
  P("setlocal oob", show(pcall(debug.setlocal, 1, 100, 1)))
  P("setlocal neg", show(pcall(debug.setlocal, 1, -1, 1)))
  P("setlocal 0", show(pcall(debug.setlocal, 1, 0, 1)))
  -- Writing at a level that is a Lua function really does change that
  -- function's local, so aim at a victim of our own: level 1 is 'poke',
  -- level 2 is 'victim'.
  do
    local function victim()
      local sacrificial = 1
      local function poke() return debug.setlocal(2, 1, 99) end
      local r = poke()
      return r, sacrificial
    end
    P("setlocal Lua", victim())
  end
  -- A level whose frame is a C function ('pcall' here, which is what
  -- called the anonymous function) has nothing to read and nothing to
  -- write, and saying so must not reach past it.
  P("C frame read", show(pcall(function() return debug.getlocal(2, 1) end)))
  P("C frame write", show(pcall(function() return debug.setlocal(2, 1, 1) end)))
  -- Asking about a function instead of a level.
  local function g(a, b) local c = 3 return a + b + c end
  P("fn local", show(pcall(debug.getlocal, g, 1)))
  P("fn not yet", debug.getlocal(g, 1))
  P("fn set", show(pcall(debug.setlocal, g, 1, 1)))
  P(rec(debug.getlocal))
  P(rec(debug.getlocal, 1))
  P(rec(debug.setlocal, 1, 1))
end
print("== upvalues ==")
do
  local a, b = 1, 2
  local function f() return a + b end
  P("getup", debug.getupvalue(f, 1))
  P("getup2", debug.getupvalue(f, 2))
  P("getup3", debug.getupvalue(f, 3))
  P("getup0", debug.getupvalue(f, 0))
  P("getup-1", debug.getupvalue(f, -1))
  P("setup", debug.setupvalue(f, 1, 10))
  P("after", f())
  P("setup oob", debug.setupvalue(f, 9, 1))
  P("id", debug.upvalueid(f, 1) == debug.upvalueid(f, 1))
  P("id diff", debug.upvalueid(f, 1) ~= debug.upvalueid(f, 2))
  local function g() return a end
  P("id shared", debug.upvalueid(f, 1) == debug.upvalueid(g, 1))
  P("join", show(pcall(debug.upvaluejoin, f, 1, g, 1)))
  P("after join", f())
  P("join oob", show(pcall(debug.upvaluejoin, f, 1, g, 9)))
  P("join nonfn", show(pcall(debug.upvaluejoin, 1, 1, g, 1)))
  P("getup C", show(pcall(debug.getupvalue, print, 1)))
  P("getup nonfn", show(pcall(debug.getupvalue, 1, 1)))
  P("getup 1.5", show(pcall(debug.getupvalue, f, 1.5)))
  -- How many upvalues a closure reports.
  local function h() return a, b end
  P("nups", (debug.getinfo(h, "u")).nups, (debug.getinfo(print, "u")).nups)
end

print("== traceback ==")
do
  P("plain", (debug.traceback()):gsub("\n", " / "))
  P("msg", (debug.traceback("hello")):gsub("\n", " / "))
  P("level1", (debug.traceback("m", 1)):gsub("\n", " / "))
  P("level2", (debug.traceback("m", 2)):gsub("\n", " / "))
  P("level0", (debug.traceback("m", 0)):gsub("\n", " / "))
  P("level99", (debug.traceback("m", 99)):gsub("\n", " / "))
  P("level1.5", show(pcall(debug.traceback, "m", 1.5)))
  P("msg num", (debug.traceback(42)):gsub("\n", " / "))
  P("msg table", show(pcall(debug.traceback, {})))
  local co = coroutine.create(function() coroutine.yield() end)
  P("thread", (debug.traceback(co)):gsub("\n", " / "))
  P("thread msg", (debug.traceback(co, "m")):gsub("\n", " / "))
  P("bad thread", show(pcall(debug.traceback, 1, "m")))
  -- A traceback taken in a coroutine only shows that coroutine.
  local co2 = coroutine.create(function()
    return debug.traceback("inner")
  end)
  local ok, r = coroutine.resume(co2)
  P("in coro", ok, (r or ""):gsub("\n", " / "))
end

print("== the rest ==")
P("registry", type(debug.getregistry()))
P("reg main", debug.getregistry()[1] ~= nil)
P("getmetatable", debug.getmetatable(1), debug.getmetatable("x"), debug.getmetatable(nil))
do
  local t = {}
  local mt = {}
  debug.setmetatable(t, mt)
  P("setmetatable", debug.getmetatable(t) == mt, getmetatable(t) == mt)
  debug.setmetatable(t, nil)
  P("clear", debug.getmetatable(t))
  P("setmt bad", show(pcall(debug.setmetatable, 1, {})))
  P("getmt none", show(pcall(debug.getmetatable)))
end
P("iuservalue", show(pcall(debug.getiuservalue, print, 1)))
P("setiuservalue", show(pcall(debug.setiuservalue, print, 1, 1)))
P("setiuservalue ok", show(pcall(debug.setiuservalue, print, 1, nil)))
-- sethook with a bogus mask, and the counts that come back.
do
  P("mask bogus", show(pcall(debug.sethook, function() end, "z")))
  P("mask empty", show(pcall(debug.sethook, function() end, "")))
  P("mask 0", show(pcall(debug.sethook, function() end, "crl", 0)))
  P("mask neg", show(pcall(debug.sethook, function() end, "crl", -1)))
  P("mask 1.5", show(pcall(debug.sethook, function() end, "crl", 1.5)))
  P("no fn", show(pcall(debug.sethook)))
  P("off", debug.sethook())
  P("gethook", debug.gethook())
end
-- 'debug.getinfo' on a tail call: 'istailcall' is only set for the
-- frame that was replaced.
do
  local function tail() return debug.getinfo(2, "tn") end
  local function caller() return tail() end
  local t = caller()
  P("tailcall", t.istailcall, t.name, t.namewhat)
end
