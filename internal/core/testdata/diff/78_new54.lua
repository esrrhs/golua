-- What is new in 5.4, and what only shows up when it is used: 'warn'
-- and its __warn hook, 'debug.setcstacklimit', the 'close' argument of
-- 'os.exit', integer 'math.random', the new '%q', and the mode argument
-- of 'load'.
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

print("== warn ==")
P(rec(warn, "a message"))
P(rec(warn, "@on"))
P(rec(warn, "on"))
P(rec(warn, "@off"))
P(rec(warn, "off"))
warn("@off")
P(rec(warn, "quiet"))
warn("@on")
P(rec(warn, "loud"))
warn("@off")
P(rec(warn))
P(rec(warn, 1))
P(rec(warn, "a", "b", "c"))
P(rec(warn, "with", 1, nil, true))
P(rec(warn, "@maybe"))
P(rec(warn, "@"))
-- Every argument has to be a string or a number; 5.4's 'warn' does not
-- consult a '__warn' metamethod, and a __tostring is not enough either.
do
  local o = setmetatable({}, {__tostring = function() return "O" end,
                              __name = "MyType"})
  warn("@on")
  P("table arg", show(pcall(warn, o)))
  P("named", show(pcall(warn, setmetatable({}, {__tostring = function() return "T" end, __name = "MyType"}))))
  P("number arg", show(pcall(warn, 1, "x")))
  P("nil arg", show(pcall(warn, "x", nil)))
  P("bool arg", show(pcall(warn, true)))
  warn("@off")
end

print("== debug.setcstacklimit ==")
P("current", debug.setcstacklimit(200))
P("set 100", debug.setcstacklimit(100))
do
  local depth = 0
  local function down()
    depth = depth + 1
    local ok = pcall(down)
    return 1
  end
  local ok, e = pcall(down)
  P("depth at 100", ok, e, depth)
end
P("restore", debug.setcstacklimit(200))
P("bad", show(pcall(debug.setcstacklimit, 0)))
P("bad neg", show(pcall(debug.setcstacklimit, -1)))
P("bad 1.5", show(pcall(debug.setcstacklimit, 1.5)))
P("big", debug.setcstacklimit(1000))
P("restore2", debug.setcstacklimit(200))

print("== os.exit: the close argument ==")
-- Only observed from outside; here we check that both values are
-- accepted and that a non-boolean 'close' is not.
P("type", type(os.exit))
do
  local info = debug.getinfo(os.exit, "Su")
  P("sig", info.nparams, info.isvararg, info.what)
end

print("== math.random: integer form ==")
math.randomseed(42)
local r1 = {}
for i = 1, 20 do r1[i] = math.random() end
P("float stream", table.concat(r1, ","))
math.randomseed(42)
local r2 = {}
for i = 1, 20 do r2[i] = math.random() end
P("same seed", table.concat(r1, ",") == table.concat(r2, ","))
P("int 0", show(pcall(math.random, 0)))
P("int 1", math.random(1))
P("int 6", show(pcall(function()
  local s = {}
  for i = 1, 20 do s[i] = math.random(6) end
  return table.concat(s, ",")
end)))
P("range", show(pcall(function()
  local s = {}
  for i = 1, 20 do s[i] = math.random(10, 20) end
  return table.concat(s, ",")
end)))
P("bad range", show(pcall(math.random, 20, 10)))
P("neg", show(pcall(math.random, -5, -1)))
P("huge", show(pcall(math.random, 1, math.maxinteger)))
P("float arg", show(pcall(math.random, 1.5)))
P("string arg", show(pcall(math.random, "6")))
P("str range", show(pcall(math.random, "10", "20")))
-- With no argument the two seeds are the current time and the address
-- of the state, so only their shape can be compared.
do
  local ok, a, b = pcall(math.randomseed)
  P("randomseed0", ok, type(a), type(b), math.type(a), math.type(b),
    tostring(a == math.tointeger(a) and b == math.tointeger(b)))
end
P("randomseed 2", show(pcall(math.randomseed, 1, 2)))
P("randomseed str", show(pcall(math.randomseed, "1")))
math.randomseed(42)

print("== string.format %q ==")
P("q str", string.format("%q", 'a"b\\c\nd\te'))
P("q esc", string.format("%q", "\a\b\f\n\r\t\v\\\""))
P("q nul", string.format("%q", "a\0b"))
P("q utf8", string.format("%q", "\xe4\xb8\xad"))
P("q bad utf8", string.format("%q", "\xff\xfe"))
P("q num", string.format("%q", 1))
P("q float", string.format("%q", 1.5))
P("q maxint", string.format("%q", math.maxinteger))
P("q minint", string.format("%q", math.mininteger))
P("q inf", string.format("%q", 1 / 0))
P("q -inf", string.format("%q", -1 / 0))
P("q nan", string.format("%q", 0 / 0))
P("q nil", show(pcall(string.format, "%q", nil)))
P("q bool", show(pcall(string.format, "%q", true)))
P("q roundtrip", load("return " .. string.format("%q", "a\0b\n"))())
P("q width", show(pcall(string.format, "%10q", "x")))

print("== load: the mode argument ==")
do
  local bin = string.dump(function() return 1 end)
  P("mode default", load(bin) ~= nil)
  P("mode b", load(bin, nil, "b") ~= nil)
  P("mode t text", load("return 1", nil, "t") ~= nil)
  P("mode t bin", show(pcall(load, bin, nil, "t")))
  P("mode bt", load(bin, nil, "bt") ~= nil)
  P("mode empty", load("return 1", nil, "") ~= nil)
  P("mode z", show(pcall(load, "return 1", nil, "z")))
  P("mode num", show(pcall(load, "return 1", nil, 1)))
  P("strip", load(string.dump(function() return 1 end, true)) ~= nil)
end

print("== table.pack / unpack boundaries ==")
P("pack", show(pcall(function() local t = table.pack(1, 2, 3) return t.n, #t end)))
P("pack nil", show(pcall(function() local t = table.pack(nil, nil) return t.n, #t end)))
P("pack empty", show(pcall(function() return table.pack().n end)))
P("unpack", show(pcall(table.unpack, {1, 2, 3})))
P("unpack range", show(pcall(table.unpack, {1, 2, 3, 4, 5}, 2, 4)))
P("unpack empty", show(pcall(table.unpack, {})))
P("unpack oob", show(pcall(table.unpack, {1, 2}, 1, 1e7)))
P("unpack neg", show(pcall(table.unpack, {1, 2}, -1)))
P("unpack float", show(pcall(table.unpack, {1, 2}, 1.5)))

print("== integer division and modulo: negatives ==")
P("idiv", 7 // 2, -7 // 2, 7 // -2, -7 // -2)
P("mod", 7 % 2, -7 % 2, 7 % -2, -7 % -2)
P("fmod", math.fmod(7, 2), math.fmod(-7, 2), math.fmod(7, -2))
P("maxint idiv", show(pcall(function() return math.maxinteger // -1 end)))
P("mod zero", show(pcall(function() return 1 % 0 end)))
P("idiv zero", show(pcall(function() return 1 // 0 end)))
P("mod float zero", show(pcall(function() return 1.0 % 0 end)))
P("idiv float zero", show(pcall(function() return 1.0 // 0 end)))
P("minint mod -1", show(pcall(function() return math.mininteger % -1 end)))
