-- Four pure-Lua libraries, run unmodified.
--
-- Nothing here is written to test this port: these are the libraries a
-- Lua program actually uses -- a JSON encoder and decoder, an
-- object-orientation library, a functional-iterator library and a
-- pretty-printer -- and the only thing asked of them is that they run
-- and print. What comes out is compared against PUC-Lua line for line,
-- which makes every line of these five files a question about the
-- virtual machine, the string and table libraries, the metamethods and
-- the coroutines underneath them.
--
-- Two things keep the output comparable:
--
--   - a table is printed with its keys sorted (see 'dump'). The order
--     'pairs' hands them out is the hash table's, and PUC-Lua seeds
--     its hashes from the time and an address, so that order is
--     different on every run -- of the reference as much as of this
--     port. Nothing here may depend on it.
--   - every case is wrapped, so that one library refusing a value says
--     so and the next case is asked anyway.

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
    parts[#parts + 1] = "[" .. dump(k, seen) .. "]=" .. dump(v[k], seen)
  end
  seen[v] = nil
  return "{" .. table.concat(parts, ", ") .. "}"
end

local n = 0
local function P(tag, ...)
  n = n + 1
  local t = {tostring(n), tag}
  for k = 1, select("#", ...) do
    local v = (select(k, ...))
    t[#t + 1] = type(v) == "string" and string.format("%q", v) or tostring(v)
  end
  print(table.concat(t, "\t"))
end

-- one case: the answer, or the complaint, but never the end of the run
local function C(tag, f, ...)
  local ok, a, b, c = pcall(f, ...)
  if not ok then
    a, b, c = nil, a, nil
  end
  P(tag, a, b, c)
end

-- ------------------------------------------------------------------
-- dkjson: a JSON encoder and decoder in one file
-- ------------------------------------------------------------------
local dkjson = require("dkjson")

local json = {
  '[1,2,3]',
  '{"a":1,"b":2}',
  '{"a":[1,2,{"b":null}],"c":true,"d":false}',
  '{"a":1.5e10,"b":-0.0,"c":1e-10}',
  '"a\\u00e9\\/b\\\\c\\"d"',
  '{"a":"\\ud83d\\ude00"}',
  '[ ]',
  '{}',
  'null',
  '  \n\t[1,\n 2 ]  ',
  '{"a":1} trailing',
  '[1,2,]',
  '{"a":}',
  '{"a" 1}',
  'nope',
  '',
  '[1,2',
  '{"a":1,"a":2}',
  '[[[[1]]]]',
  '{"":1}',
  '{"\\u00e9":1}',
  '"unterminated',
  '{"a":01}',
  '[1e400]',
  '-',
  '+1',
}
for i, s in ipairs(json) do
  C("dkjson/decode " .. string.format("%q", s), function()
    local v, pos, err = dkjson.decode(s, 1, nil)
    return dump(v), pos, err
  end)
  C("dkjson/decode-null " .. string.format("%q", s), function()
    local v, pos, err = dkjson.decode(s, 1, dkjson.null)
    return dump(v), pos, err
  end)
end
C("dkjson/decode/pos", function()
  local v, pos = dkjson.decode("[1,2,3] rest", 1, nil)
  return dump(v), pos
end)
C("dkjson/decode/from", function()
  local v, pos = dkjson.decode("....[7]", 5, nil)
  return dump(v), pos
end)
C("dkjson/depth", function()
  local s = string.rep("[", 40) .. "1" .. string.rep("]", 40)
  return dump(dkjson.decode(s, 1, nil))
end)

local encodable = {
  {1, 2, 3},
  {"a", "b"},
  {1, 1.5, -0.0, 1e300, -1e-300},
  {true, false},
  {{1, 2}, {3, {4, 5}}},
  {a = 1},
  {[""] = 1},
  {a = {b = {c = 1}}},
  "plain",
  "a\0b",
  "a\"b\\c",
  "a\nb\tc",
  "é中😀",
  "\r\f\b",
  42,
  0,
  -0.0,
  1 / 2,
  true,
  false,
  -- the long ones: a JSON encoder prints a number with '%.14g' or with
  -- 'tostring', so these ask for the fourteenth significant digit
  1 / 3,
  math.pi,
  0.1,
  1 / 7,
  2 ^ 53,
  2 ^ 53 + 1,
  123.456,
  1e15,
  1e16,
  1e-15,
  12345678901234,
  -1.7976931348623157e308,
}
for i, v in ipairs(encodable) do
  C("dkjson/encode " .. dump(v), function() return dkjson.encode(v) end)
  C("dkjson/encode-indent " .. dump(v), function()
    return dkjson.encode(v, {indent = true})
  end)
end
C("dkjson/encode/nan", function() return dkjson.encode(0 / 0) end)
C("dkjson/encode/inf", function() return dkjson.encode(1 / 0) end)
C("dkjson/encode/fn", function() return dkjson.encode({print}) end)
C("dkjson/encode/mixed", function() return dkjson.encode({1, a = 2}) end)
C("dkjson/encode/cycle", function()
  local t = {}
  t.self = t
  return dkjson.encode(t)
end)
C("dkjson/encode/keyorder", function()
  return dkjson.encode({a = 1, b = 2}, {keyorder = {"b", "a"}})
end)
C("dkjson/encode/null", function() return dkjson.encode({dkjson.null}) end)
C("dkjson/pack", function()
  local v = dkjson.decode('{"__jsontype":"array"}', 1, nil)
  return dkjson.encode(v)
end)
C("dkjson/use_lpeg", function()
  -- the type, not the value: 'use_lpeg' is a function, and the address
  -- a function prints with is the host's
  return type(dkjson.use_lpeg), tostring(dkjson.using_lpeg)
end)
C("dkjson/big", function()
  local t = {}
  for i = 1, 40 do
    t[i] = {i = i, name = "n" .. i, tags = {"a", "b"}, ok = i % 2 == 0}
  end
  local s = dkjson.encode(t)
  local v, pos, err = dkjson.decode(s, 1, nil)
  return #s, pos, err, dump(v[1]), dump(v[40])
end)
C("dkjson/deep", function()
  local s = string.rep('{"a":', 60) .. "1" .. string.rep("}", 60)
  local v, pos, err = dkjson.decode(s, 1, nil)
  return pos, err, type(v)
end)
C("dkjson/numbers", function()
  return dump(dkjson.decode(
    '[0,-0,1,-1,1e308,-1e308,5e-324,9007199254740993,1e400,-1e400]', 1, nil))
end)
C("dkjson/uescape", function()
  return dump(dkjson.decode('"\\u0000\\u001f\\u007f\\uffff\\ud83d\\ude00"', 1, nil)),
    dump(dkjson.decode('"\\ud800"', 1, nil)),
    dump(dkjson.decode('"\\uZZZZ"', 1, nil))
end)
C("dkjson/encode-limits", function()
  return dkjson.encode({2 ^ 53, -2 ^ 53, 2 ^ 63, math.maxinteger,
    math.mininteger, 5e-324, 1.7976931348623157e308})
end)
C("dkjson/encode-utf8", function()
  return dkjson.encode("é中😀"), dkjson.encode("\127\128"),
    dkjson.encode("a\226b")
end)
C("dkjson/decode-pos-past", function()
  return dump(dkjson.decode("[1]", 10, nil))
end)
C("dkjson/decode-pos-zero", function()
  return dump(dkjson.decode("[1]", 0, nil))
end)

-- ------------------------------------------------------------------
-- json.lua (rxi): a smaller JSON library, asked the same questions
-- ------------------------------------------------------------------
local rxi = require("json")
for i, s in ipairs(json) do
  C("rxi/decode " .. string.format("%q", s), function()
    return dump(rxi.decode(s))
  end)
end
for i, v in ipairs(encodable) do
  C("rxi/encode " .. dump(v), function() return rxi.encode(v) end)
end
C("rxi/encode/nan", function() return rxi.encode(0 / 0) end)
C("rxi/encode/inf", function() return rxi.encode(1 / 0) end)
C("rxi/encode/fn", function() return rxi.encode({print}) end)
C("rxi/encode/cycle", function()
  local t = {}
  t.self = t
  return rxi.encode(t)
end)
C("rxi/encode/empty", function() return rxi.encode({}) end)
C("rxi/roundtrip", function()
  return dump(rxi.decode(rxi.encode({1, "two", {three = 3}})))
end)

-- ------------------------------------------------------------------
-- middleclass: object orientation, which is metatables all the way down
-- ------------------------------------------------------------------
local class = require("middleclass")
C("mc/class", function()
  local A = class("A")
  function A:initialize(x) self.x = x end
  function A:show() return "A" .. tostring(self.x) end
  local a = A(1)
  return a:show(), a.x, tostring(a):match("^instance of A") ~= nil,
    A.name, tostring(A.super)
end)
C("mc/inherit", function()
  local A = class("A")
  function A:who() return "A" end
  local B = class("B", A)
  function B:who() return "B" .. A.who(self) end
  local C = class("C", B)
  local c = C()
  return c:who(), C:isSubclassOf(A), A:isSubclassOf(C), C:isSubclassOf(C),
    c:isInstanceOf(A), c:isInstanceOf(C), A:isInstanceOf(A)
end)
C("mc/tostring", function()
  local A = class("A")
  function A:__tostring() return "I am A" end
  local B = class("B", A)
  return tostring(A()), tostring(B()), tostring(A), tostring(B)
end)
C("mc/metamethods", function()
  local V = class("V")
  function V:initialize(x, y) self.x, self.y = x, y end
  function V:__add(o) return V(self.x + o.x, self.y + o.y) end
  function V:__eq(o) return self.x == o.x and self.y == o.y end
  function V:__lt(o) return self.x < o.x end
  function V:__len() return 2 end
  function V:__concat(o) return "V+" .. o end
  function V:__call(a) return self.x + a end
  function V:__unm() return V(-self.x, -self.y) end
  local a, b = V(1, 2), V(3, 4)
  local s = a + b
  return dump({s.x, s.y}), a == b, a < b, #a, a .. "x", a(10),
    dump({(-a).x, (-a).y})
end)
C("mc/include", function()
  local Mixin = {shared = 1, hello = function(self) return "hi" end}
  local A = class("A")
  A:include(Mixin)
  local a = A()
  return a.hello(a), a.shared, A.shared
end)
C("mc/static", function()
  local A = class("A")
  A.static.make = function() return A() end
  return A.make():isInstanceOf(A), A.static.make ~= nil
end)
C("mc/nilmethod", function()
  local A = class("A")
  return A():nope()
end)
C("mc/nilfield", function()
  local A = class("A")
  return A().nope
end)
C("mc/subclass-root", function()
  local A = class("A")
  return A.super == nil, A:isSubclassOf(Object), Object.name
end)
C("mc/allocate", function()
  local A = class("A")
  local a = A:allocate()
  return a.class == A, a:isInstanceOf(A), type(rawget(a, "initialize"))
end)
C("mc/anonymous", function()
  local A = class()
  return tostring(A):match("^class") ~= nil, A():isInstanceOf(A)
end)
C("mc/gc", function()
  local A = class("A")
  local out = {}
  for i = 1, 3 do
    local a = A()
    a.i = i
    out[i] = a.i
  end
  return table.concat(out, ",")
end)
C("mc/super-method", function()
  local A = class("A")
  function A:greet() return "hi " .. self.name end
  function A:initialize(n) self.name = n end
  local B = class("B", A)
  function B:greet() return A.greet(self) .. "!" end
  function B:initialize(n) A.initialize(self, n .. "?") end
  return B("x"):greet()
end)
C("mc/included-hook", function()
  local M = {
    included = function(self, klass) self.hooked = klass.name end,
    speak = function(self) return "speak" end,
  }
  local A = class("A")
  A:include(M)
  A:include(M)
  return A().speak(A()), A().hooked
end)
C("mc/index-override", function()
  local A = class("A")
  function A:__index(k) return "index:" .. k end
  local a = A()
  return a.anything, a.x
end)
C("mc/subclass-many", function()
  local A = class("A")
  local cur = A
  for i = 1, 20 do cur = class("C" .. i, cur) end
  local o = cur()
  return o:isInstanceOf(A), cur:isSubclassOf(A), A:isSubclassOf(cur),
    #cur.name
end)
C("mc/error-new", function()
  local A = class("A")
  return A.new(A, 1, 2, 3).x
end)
C("mc/tostring-err", function()
  local A = class("A")
  function A:__tostring() error("no") end
  return tostring(A())
end)
C("mc/classtype", function()
  local A = class("A")
  return type(A), type(A()), tostring(A.__instanceDict):match("^table"),
    A.__index == A.__instanceDict
end)

-- ------------------------------------------------------------------
-- Lua Fun: iterators, and every one of them a coroutine
-- ------------------------------------------------------------------
local fun = require("fun")
local function nums(k)
  local t = {}
  for i = 1, k do t[i] = i end
  return t
end
C("fun/range", function() return dump(fun.range(1, 6):totable()) end)
C("fun/range-step", function() return dump(fun.range(1, 10, 3):totable()) end)
C("fun/range-down", function() return dump(fun.range(5, 1, -2):totable()) end)
C("fun/iter", function() return dump(fun.iter(nums(4)):totable()) end)
C("fun/map", function()
  return dump(fun.map(function(x) return x * 2 end, nums(5)):totable())
end)
C("fun/map2", function()
  return dump(fun.map(function(x, y) return x + y end, nums(3), nums(3)):totable())
end)
C("fun/filter", function()
  return dump(fun.filter(function(x) return x % 2 == 0 end, nums(10)):totable())
end)
C("fun/reduce", function()
  return fun.reduce(function(a, b) return a + b end, 0, nums(10))
end)
C("fun/sum", function() return fun.sum(nums(10)), fun.sum(fun.range(1, 0)) end)
C("fun/minmax", function()
  return fun.min(nums(7)), fun.max(nums(7)), fun.min(fun.range(1, 0))
end)
C("fun/take-skip", function()
  return dump(fun.take(3, nums(9)):totable()),
    dump(fun.skip(7, nums(9)):totable())
end)
C("fun/zip", function()
  return dump(fun.zip(nums(3), {"a", "b", "c"}):totable())
end)
C("fun/chain", function()
  return dump(fun.chain(nums(2), nums(2)):totable())
end)
C("fun/enumerate", function()
  return dump(fun.enumerate(fun.iter({"a", "b"})):totable())
end)
C("fun/all-any", function()
  return fun.all(function(x) return x > 0 end, nums(4)),
    fun.any(function(x) return x > 3 end, nums(4)),
    fun.all(function(x) return x > 3 end, nums(4))
end)
C("fun/length", function() return fun.length(nums(5)), fun.length(fun.range(1, 0)) end)
C("fun/nth", function() return fun.nth(3, nums(9)), fun.nth(99, nums(9)) end)
C("fun/partition", function()
  local a, b = fun.partition(function(x) return x % 2 == 0 end, nums(6))
  return dump(a:totable()), dump(b:totable())
end)
C("fun/span", function()
  local a, b = fun.span(function(x) return x < 4 end, nums(8))
  return dump(a:totable()), dump(b:totable())
end)
C("fun/take_while", function()
  return dump(fun.take_while(function(x) return x < 4 end, nums(8)):totable())
end)
C("fun/drop_while", function()
  return dump(fun.drop_while(function(x) return x < 4 end, nums(8)):totable())
end)
C("fun/flatten", function()
  return dump(fun.flatten({{1, 2}, {3}, {}}):totable())
end)
C("fun/intersperse", function()
  return dump(fun.intersperse(0, nums(4)):totable())
end)
C("fun/chunks", function()
  return dump(fun.chunks(2, nums(7)):totable())
end)
C("fun/cycle", function()
  return dump(fun.take(5, fun.cycle(nums(2))):totable())
end)
C("fun/duplicate", function()
  return dump(fun.take(3, fun.duplicate("x")):totable())
end)
C("fun/tabulate", function()
  return dump(fun.take(4, fun.tabulate(function(i) return i * i end)):totable())
end)
C("fun/foreach", function()
  local out = {}
  fun.each(function(x) out[#out + 1] = x end, nums(3))
  return table.concat(out, ",")
end)
C("fun/method-chain", function()
  return dump(fun.range(1, 20):filter(function(x) return x % 3 == 0 end)
    :map(function(x) return x * x end):take(3):totable())
end)
C("fun/stateful-twice", function()
  local it = fun.iter(nums(3))
  return dump(it:totable()), dump(it:totable())
end)
C("fun/empty", function()
  return dump(fun.iter({}):totable()), fun.length(fun.iter({}))
end)
C("fun/strings", function()
  return dump(fun.iter({"b", "a", "c"}):totable()),
    fun.totable(fun.map(string.upper, fun.iter({"a", "b"})))[1]
end)
C("fun/sort", function()
  return dump(fun.sort(fun.iter({3, 1, 2})):totable())
end)

-- ------------------------------------------------------------------
-- inspect.lua: a pretty-printer, over the values whose printing does
-- not depend on the order 'pairs' hands out keys
-- ------------------------------------------------------------------
local inspect = require("inspect")
local flat = {1, "two", 3.5, true, nil, -0.0, 1e300, "a\0b", "é"}
for i, v in ipairs(flat) do
  C("inspect " .. tostring(v), function() return inspect(v) end)
end
C("inspect/flat-table", function() return inspect({x = 1, y = "two"}) end)
C("inspect/array", function() return inspect({1, 2, 3}) end)
C("inspect/nested", function() return inspect({a = {b = {c = 1}}}) end)
C("inspect/empty", function() return inspect({}) end)
C("inspect/fn", function() return inspect(print) end)
C("inspect/options", function()
  return inspect(1.0, {indent = "  ", newline = "\n"}),
    inspect({x = 1}, {depth = 0}),
    inspect({x = 1}, {depth = 1})
end)
C("inspect/metatable", function()
  return inspect(setmetatable({}, {__tostring = function() return "MT" end}))
end)
C("inspect/keysorted", function()
  -- one table, so that no number is handed out in an order that is not
  -- this one: see the note at the top
  return inspect({["1"] = 1, [1] = 2, [true] = 3, [2.5] = 4})
end)

print(n, "cases")
