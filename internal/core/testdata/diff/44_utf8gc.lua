-- utf8, weak tables, __gc, to-be-closed variables and the coroutine
-- state machine.  No answer depends on when the collector happens to
-- run: every collection is driven by allocating until the step fires.
-- utf8, weak tables, __gc, to-be-closed, the coroutine state machine
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

-- PUC-Lua collects because it allocates; here, 'collectgarbage' being
-- inert, the step an allocation drives is what sweeps a weak table and
-- runs a '__gc' (see gcStep).  Allocating is therefore how a script asks
-- for a cycle in this port.
local function cycle()
  for i = 1, 1000000 do local a = {} end
end

print("== utf8.char ==")
print(show(utf8.char(65, 66, 67)))
print(show(utf8.char()))
print(show(utf8.char(0)))
print(show(utf8.char(0x7F)))
print(show(utf8.char(0x80)))
print(show(utf8.char(0x7FF)))
print(show(utf8.char(0x800)))
print(show(utf8.char(0xFFFF)))
print(show(utf8.char(0x10000)))
print(show(utf8.char(0x10FFFF)))
print(rec(utf8.char, 0x110000))
print(rec(utf8.char, -1))
print(rec(utf8.char, 1.5))
print(rec(utf8.char, "x"))
print(rec(utf8.char, 0x7FFFFFFF))
print(show(#utf8.charpattern))

print("== utf8.codepoint ==")
print(show(utf8.codepoint("ABC")))
print(show(utf8.codepoint("ABC", 2)))
print(show(utf8.codepoint("ABC", 2, 3)))
print(show(utf8.codepoint("ABC", -1)))
print(show(utf8.codepoint("ABC", -3, -1)))
print(rec(utf8.codepoint, "ABC", 0))
print(rec(utf8.codepoint, "ABC", 4))
print(rec(utf8.codepoint, "ABC", 1, 4))
print(rec(utf8.codepoint, "ABC", 3, 2))
print(rec(utf8.codepoint, "\xFF"))
print(rec(utf8.codepoint, ""))
print(rec(utf8.codepoint))
print(rec(utf8.codepoint, 42))
print(show(utf8.codepoint("\u{1F600}")))

print("== utf8.len ==")
print(show(utf8.len("ABC")))
print(show(utf8.len("A\u{1F600}C")))
print(show(utf8.len("")))
print(show(utf8.len("ABC", 2)))
print(show(utf8.len("ABC", 2, 3)))
print(show(utf8.len("ABC", -1)))
print(show(utf8.len("\xFF")))
print(show(utf8.len("\xC3")))
print(show(utf8.len("\xC3\x28")))
print(rec(utf8.len, 42))
print(rec(utf8.len, "ABC", 0))
print(rec(utf8.len, "ABC", 5))
print(rec(utf8.len, "ABC", "2"))

print("== utf8.offset ==")
local s = "A\u{1F600}C"
print(show(utf8.offset(s, 1)))
print(show(utf8.offset(s, 2)))
print(show(utf8.offset(s, 3)))
print(show(utf8.offset(s, 4)))
print(show(utf8.offset(s, -1)))
print(show(utf8.offset(s, -1, #s + 1)))
print(show(utf8.offset(s, -2, #s + 1)))
print(rec(utf8.offset, s, 5))
print(rec(utf8.offset, s, 1, 0))
print(rec(utf8.offset, s, 0))
print(rec(utf8.offset, 42, 1))
print(rec(utf8.offset, s))
print(rec(utf8.offset, s, "x"))

print("== utf8.codes ==")
do
  local out = {}
  for p, c in utf8.codes("A\u{1F600}C") do out[#out + 1] = p .. ":" .. c end
  print(table.concat(out, " "))
end
do
  local out = {}
  for p, c in utf8.codes("") do out[#out + 1] = p .. ":" .. c end
  print("[" .. table.concat(out, " ") .. "]")
end
print(rec(function() for p, c in utf8.codes("\xFF") do end end))
print(rec(utf8.codes))
print(rec(utf8.codes, 42))
do
  local f, s_, i = utf8.codes("AB")
  print(show(f(s_, i)))
  print(show(f(s_, 2)))
  print(show(f(s_, 3)))
end

print("== weak tables ==")
do
  local w = setmetatable({}, {__mode = "k"})
  local k = {}
  w[k] = 1
  print(show(next(w) ~= nil))
  k = nil
  cycle()
  print(show(next(w) == nil))
end
do
  local w = setmetatable({}, {__mode = "v"})
  w[1] = {}
  print(show(next(w) ~= nil))
  cycle()
  print(show(next(w) == nil))
end
do
  local w = setmetatable({}, {__mode = "kv"})
  print(show(w[{}]))
  local a, b = {}, {}
  w[a] = b
  cycle()
  print(show(next(w) == nil))
end
print(rec(setmetatable, {}, {__mode = "x"}))
print(rec(setmetatable, {}, {__mode = 42}))
do
  local w = setmetatable({}, {__mode = "k"})
  cycle()
  print(show(type(w)))
end

print("== __gc ==")
do
  local seen = 0
  for i = 1, 10 do
    setmetatable({}, {__gc = function() seen = seen + 1 end})
  end
  cycle()
  print(show(seen > 0))
end
do
  local order = {}
  setmetatable({}, {__gc = function(o) order[#order + 1] = "a" end})
  setmetatable({}, {__gc = function(o) order[#order + 1] = "b" end})
  cycle()
  print(show(#order))
end
print(rec(setmetatable, {}, {__gc = 42}))

print("== to-be-closed ==")
do
  local log = {}
  do
    local x <close> = setmetatable({}, {__close = function() log[#log + 1] = "c" end})
    log[#log + 1] = "body"
  end
  print(table.concat(log, ","))
end
do
  local log = {}
  local ok, err = pcall(function()
    local x <close> = setmetatable({}, {__close = function() log[#log + 1] = "c" end})
    error("boom")
  end)
  print(ok, err, table.concat(log, ","))
end
do
  local log = {}
  local ok, err = pcall(function()
    local a <close> = setmetatable({}, {__close = function() log[#log + 1] = "a" end})
    local b <close> = setmetatable({}, {__close = function() log[#log + 1] = "b" end})
    error("boom")
  end)
  print(ok, err, table.concat(log, ","))
end
print(rec(load, "local x <const> = 1 local y <close> = 2"))
print(rec(load, "for i = 1, 2 do local x <close> = nil end"))

print("== coroutine state machine ==")
do
  local co = coroutine.create(function() return 1 end)
  print(show(coroutine.status(co)))
  print(rec(coroutine.resume, co))
  print(show(coroutine.status(co)))
  print(rec(coroutine.resume, co))
  print(show(coroutine.status(co)))
end
do
  local co = coroutine.create(function() coroutine.yield(1) error("x") end)
  print(rec(coroutine.resume, co))
  print(show(coroutine.status(co)))
  print(rec(coroutine.resume, co))
  print(show(coroutine.status(co)))
end
do
  local co = coroutine.create(function() return 1 end)
  print(show(coroutine.close(co)))
  print(show(coroutine.status(co)))
  print(rec(coroutine.close, co))
end
print(show(coroutine.isyieldable()))
print(show(coroutine.running()))
print(show(select(2, coroutine.running())))
do
  local main, ismain = coroutine.running()
  print(show(ismain))
end
print(rec(coroutine.close))
print(rec(coroutine.close, 42))
print(rec(coroutine.status, 42))
print(rec(coroutine.create, 42))
do
  local co = coroutine.wrap(function() coroutine.yield(1) end)
  print(show(co()))
  print(rec(co))
end
print("done")
