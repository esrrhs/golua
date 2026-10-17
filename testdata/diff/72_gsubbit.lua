-- 'string.gsub' is the one string function with a real protocol behind it
-- (string / table / function replacements, %-captures, and a second return
-- value that can stop the walk), and the bitwise library is the one place
-- where a float that happens to be integral is not the same as an integer.
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

print("== gsub: string replacements ==")
P(rec(string.gsub, "hello world", "o", "0"))
P(rec(string.gsub, "hello world", "o", "0", 1))
P(rec(string.gsub, "hello world", "o", "0", 0))
P(rec(string.gsub, "hello world", "o", "0", -1))
P(rec(string.gsub, "hello", "", "-"))
P(rec(string.gsub, "hello", "", "-", 3))
P(rec(string.gsub, "hello", "l+", "L"))
P(rec(string.gsub, "abc", "x", "y"))
P(rec(string.gsub, "abc", "", ""))
P(rec(string.gsub, "", "", "x"))
P(rec(string.gsub, "", "a", "b"))
print("== gsub: captures ==")
P(rec(string.gsub, "hello world", "(o)", "[%1]"))
P(rec(string.gsub, "hello world", "(o)(.)", "%2%1"))
P(rec(string.gsub, "hello world", "(o)(.)", "%0"))
P(rec(string.gsub, "hello", "(h)(e)(l)(l)(o)", "%5%4%3%2%1"))
P(rec(string.gsub, "hello", "(l)", "%1%1"))
P(rec(string.gsub, "hello", "()", "%1"))
P(rec(string.gsub, "hello", "()l()", "[%1,%2]"))
P(rec(string.gsub, "hello", "(l)(l)", "%3"))
P(rec(string.gsub, "hello", "(l+)", "%1%0"))
-- '%' followed by something that is not a capture.
P(rec(string.gsub, "hello", "l", "%"))
P(rec(string.gsub, "hello", "l", "%x"))
P(rec(string.gsub, "hello", "l", "%%"))
P(rec(string.gsub, "hello", "l", "%%1"))
P(rec(string.gsub, "hello", "(l)", "%%1"))
P(rec(string.gsub, "hello", "(l)", "%a"))
P(rec(string.gsub, "hello", "(l)", "%9"))
P(rec(string.gsub, "hello", "l", "100%"))
P(rec(string.gsub, "hello", "l", "%2"))
-- Position captures count as captures for '%2'.
P(rec(string.gsub, "hello", "(l)()", "%2"))

print("== gsub: table replacements ==")
P(rec(string.gsub, "hello", ".", {}))
P(rec(string.gsub, "hello", ".", {h = "H", o = "0"}))
P(rec(string.gsub, "hello", "(l)", {l = "L"}))
P(rec(string.gsub, "hello", "()", {}))
P(rec(string.gsub, "hello", ".", setmetatable({}, {__index = function(t, k) return "<" .. k .. ">" end})))
P(rec(string.gsub, "hello", ".", {h = false}))
P(rec(string.gsub, "hello", ".", {h = 0}))
P(rec(string.gsub, "hello", ".", {h = 1}))
P(rec(string.gsub, "hello", ".", {h = true}))
P(rec(string.gsub, "hello", ".", {h = 1.5}))
P(rec(string.gsub, "hello", ".", {h = {}}))
-- The key looked up is the whole match, or capture 1 if there is one.
P(rec(string.gsub, "a1b2", "(%a)(%d)", {a = "A", b = "B"}))
P(rec(string.gsub, "a1b2", "(%a)(%d)", {a1 = "X"}))

print("== gsub: function replacements ==")
P(rec(string.gsub, "hello", ".", function(c) return c:upper() end))
P(rec(string.gsub, "hello", ".", function(c) return nil end))
P(rec(string.gsub, "hello", ".", function(c) return false end))
P(rec(string.gsub, "hello", ".", function(c) return 0 end))
P(rec(string.gsub, "hello", ".", function(c) return 1 end))
P(rec(string.gsub, "hello", ".", function(c) return 1.5 end))
P(rec(string.gsub, "hello", ".", function(c) return true end))
P(rec(string.gsub, "hello", ".", function(c) return {} end))
P(rec(string.gsub, "hello", ".", function(c) error("no") end))
P(rec(string.gsub, "hello", ".", function(c) return c:upper(), 1 end))
-- The second return value stops the walk.
P(rec(string.gsub, "hello", ".", function(c) return c:upper(), 0 end))
P(rec(string.gsub, "hello", ".", function(c) return c:upper(), "x" end))
P(rec(string.gsub, "hello", ".", function(c) return nil, 1 end))
P(rec(string.gsub, "hello", ".", function(c) return nil, 0 end))
-- How many arguments the function gets.
do
  local seen = {}
  string.gsub("a1b2", "(%a)(%d)", function(...) seen[#seen + 1] = show(...) end)
  P("args", table.concat(seen, " / "))
end
-- A function that yields, and one that clears the pattern's subject.
P(rec(function() return (string.gsub("ab", ".", function(c) coroutine.yield() return c end)) end))
P(rec(string.gsub, "hello", ".", function(c) return print and c end))

print("== gsub: bad arguments ==")
P(rec(string.gsub))
P(rec(string.gsub, "a"))
P(rec(string.gsub, "a", "a"))
P(rec(string.gsub, 1, "a", "b"))
P(rec(string.gsub, "a", 1, "b"))
P(rec(string.gsub, "a", "a", 1))
P(rec(string.gsub, "a", "a", {}, 1))
P(rec(string.gsub, "a", "a", "b", "1"))
P(rec(string.gsub, "a", "a", "b", 1.0))
P(rec(string.gsub, "a", "a", "b", 1.5))
P(rec(string.gsub, "a", "a", "b", math.maxinteger))
P(rec(string.gsub, "a", "[", "b"))
P(rec(string.gsub, "a", "a", "b", 0))

print("== gmatch / find / match ==")
do
  local out = {}
  for a, b in ("a1 b2 c3"):gmatch("(%a)(%d)") do out[#out + 1] = a .. b end
  P("gmatch", table.concat(out, ","))
  out = {}
  for w in ("a1 b2 c3"):gmatch("%a%d") do out[#out + 1] = w end
  P("gmatch1", table.concat(out, ","))
  out = {}
  for w in ("abc"):gmatch("()") do out[#out + 1] = tostring(w) end
  P("gmatch pos", table.concat(out, ","))
  out = {}
  for w in ("abc"):gmatch("") do out[#out + 1] = "<" .. w .. ">" end
  P("gmatch empty", table.concat(out, ","))
  P(rec(string.gmatch))
  P(rec(string.gmatch, "abc", "["))
  P(rec(string.gmatch, "abc", 1))
  -- The iterator is independent of the string it came from.
  local it = ("abc"):gmatch("%a")
  P("it", it(), it(), it(), it())
end
P(rec(string.find, "hello", "l"))
P(rec(string.find, "hello", "l", 4))
P(rec(string.find, "hello", "l", 5))
P(rec(string.find, "hello", "l", 6))
P(rec(string.find, "hello", "l", -1))
P(rec(string.find, "hello", "l", -3))
P(rec(string.find, "hello", "l", -10))
P(rec(string.find, "hello", "l", 1, true))
P(rec(string.find, "hello", "l", 2, true))
P(rec(string.find, "hello", "", 1, true))
P(rec(string.find, "hello", "", 6, true))
P(rec(string.find, "hello", "", 7, true))
P(rec(string.find, "hello", "l", 0))
P(rec(string.find, "hello", "l", 1, 1))
P(rec(string.find, "hello", "l", 1, 0))
P(rec(string.find, "hello", "l", 1, false))
P(rec(string.find, "hello", "l", 1, "x"))
P(rec(string.match, "hello", "(l)(o)"))
P(rec(string.match, "hello", "()l()"))
P(rec(string.match, "hello", "%d"))
P(rec(string.match, "hello", "()"))
P(rec(string.match, "hello"))
P(rec(string.match, "hello", "l", 5))
P(rec(string.match, "hello", "l", -2))

print("== bitwise ==")
local b = bit32 or nil
P("band", string.format("%x", 0xff & 0x0f), string.format("%x", -1 & 0xff))
P("bor", string.format("%x", 0xf0 | 0x0f))
P("bxor", string.format("%x", 0xff ~ 0x0f))
P("bnot", string.format("%x", ~0), string.format("%x", ~(-1)))
P("shl", string.format("%x", 1 << 62), string.format("%x", 1 << 63))
P("shr", string.format("%x", -1 >> 60), string.format("%x", -1 >> 64))
P("shr0", string.format("%x", -1 >> 0))
P("shl0", string.format("%x", 1 << 0))
P("shl neg", string.format("%x", 1 << -1))
P("shr neg", string.format("%x", 8 >> -1))
P("shl big", string.format("%x", 1 << 100))
P("shr big", string.format("%x", -1 >> 100))
P("chain", string.format("%x", 1 | 2 | 4 | 8))
P("args0", select("#", (function() return 0xff & 0x0f end)()))
do
  -- Every argument has to have an exact integer representation.
  local function t(f, ...) return show(pcall(f, ...)) end
  P(t(function(a, b) return a & b end, 1, 1.0))
  P(t(function(a, b) return a & b end, 1, 1.5))
  P(t(function(a, b) return a & b end, 1.5, 1))
  P(t(function(a, b) return a & b end, 1, "2"))
  P(t(function(a, b) return a & b end, "2", 1))
  P(t(function(a, b) return a & b end, 1, "x"))
  P(t(function(a, b) return a & b end, 1))
  P(t(function(a) return a & 0 end))
  P(t(function(a) return ~nil end))
  P(t(function(a) return ~"x" end))
  P(t(function(a) return ~2^63 end))
  P(t(function(a) return ~(-2^63) end))
  P(t(function(a) return ~(2^63 - 1) end))
  P(t(function(a) return a >> 1 end, 2^63))
  P(t(function(a) return a >> 1 end, -2^63))
  P(t(function(a) return a << 1 end, 2^62))
  P(t(function(a) return a << 1 end, 2^63))
  P("no args", t(function() return 1 & 2 end))
end
-- Bit ops coerce strings, as arithmetic does.
P("coerce", ("10" & 12), tostring(("0xff" | 0) == 255))
P("float int", (3.0 & 1), math.type(3.0 & 1))
P("huge", show(pcall(function() return (1 / 0) & 1 end)))
P("nan", show(pcall(function() return (0 / 0) & 1 end)))
-- __band / __bor metamethods.
do
  local mt = {__band = function(a, b) return "band" end,
              __bor = function(a, b) return "bor" end,
              __bxor = function(a, b) return "bxor" end,
              __bnot = function(a) return "bnot" end,
              __shl = function(a, b) return "shl" end,
              __shr = function(a, b) return "shr" end}
  local o = setmetatable({}, mt)
  P("mm", o & 1, 1 & o, o | 1, 1 | o, o ~ 1, 1 ~ o, ~o, o << 1, 1 << o, o >> 1, 1 >> o)
  local p = setmetatable({}, mt)
  P("mm both", o & p, o | p, o ~ p)
  -- One side has no metamethod: it is an error, not a coercion.
  P(rec(function() return o & 1.5 end))
  P(rec(function() return o & "x" end))
  P(rec(function() return ~setmetatable({}, {}) end))
end
