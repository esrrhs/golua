-- evaluation order, varargs in constructors, string case and the math
-- library.  'math.random' is only asked about a seeded sequence; the
-- default seed is built from the clock and cannot be pinned.
-- evaluation order, varargs in constructors, string case (locale), math
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

print("== evaluation order ==")
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local t = {}
  t[f("i")] = f("v")
  print(table.concat(log, ","))
end
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local t = {}
  local a, b = f("A"), f("B")
  print(table.concat(log, ","), a, b)
end
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local t = {f("1"), f("2"), f("3")}
  print(table.concat(log, ","))
end
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local t = {[f("k")] = f("v"), f("2")}
  print(table.concat(log, ","))
end
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local t = {}
  t[f("a")], t[f("b")] = f("c"), f("d")
  print(table.concat(log, ","))
end
do
  local log = {}
  local function f(x) log[#log + 1] = x; return x end
  local s = f("1") .. f("2") .. f("3")
  print(table.concat(log, ","), s)
end
do
  local log = {}
  local obj = {}
  function obj:m(x) log[#log + 1] = "m" .. x; return obj end
  function obj.f(self, x) log[#log + 1] = "f" .. x; return function() log[#log + 1] = "inner" end end
  obj:m(1):m(2)
  obj:f(3)()
  print(table.concat(log, ","))
end
do
  local log = {}
  local function f(...) log[#log + 1] = select('#', ...); return ... end
  print(show(f(f(1, 2, 3))))
  print(show((f(1, 2, 3))))
  print(show(#{f(1, 2, 3)}))
  local t = {f(1, 2, 3)}
  print(#t)
  print(table.concat(log, ","))
end

print("== vararg in constructors ==")
do
  local function f(...) return {...} end
  print(show(#f(1, 2, 3), f(1, 2, 3)[2]))
end
do
  local function f(n, ...) return n, {...} end
  print(show(f(1, 2, 3)))
end
do
  local t = {}
  for i = 1, 100 do t[i] = i end
  local function g(...) return #{...} end
  print(g(table.unpack(t)))
end
do
  local big = {}
  for i = 1, 60 do big[i] = i end
  local function g(...) return select('#', ...), select(60, ...) end
  print(show(g(table.unpack(big))))
end
do
  -- a constructor long enough to flush the set list more than once
  local src = {}
  for i = 1, 200 do src[#src + 1] = "return {" .. string.rep("i,", 200) .. "}" end
  local f = load(table.concat(src, "\n"))
  print(rec(f))
end
do
  local a, b, c = 1, 2, 3
  local function f() return a, b, c end
  print(show(f()))
  print(show({f()}))
  print(show({(f())}))
  print(show(select('#', f())))
  print(show(select(2, f())))
  print(show(select(-1, f())))
  print(rec(select, 0, 1, 2))
  print(rec(select, "x", 1))
end

print("== string case ==")
print(show(string.upper("abcXYZ123")))
print(show(string.lower("ABCxyz123")))
print(show(string.upper("\xe4\xbd\xa0")))  -- locale dependent in C
print(show(string.lower("\xc3\x84")))
print(show(string.rep("ab", 3)))
print(show(string.rep("ab", 3, "-")))
print(show(string.rep("ab", 0)))
print(show(string.rep("", 3)))
print(rec(string.rep, "ab", -1))
print(rec(string.rep, "ab", 1, 42))
print(show(string.reverse("abc")))
print(show(string.reverse("\xe4\xbd\xa0")))
print(show(string.sub("hello", 2, -2)))
print(show(string.sub("hello", -3)))
print(show(string.sub("hello", 0)))
print(show(string.sub("hello", 10)))
print(show(string.sub("hello", 1, 100)))
print(rec(string.sub, "x", "1"))
print(rec(string.sub, "x", 1, "2"))
print(show(string.byte("A"), string.byte("AB", 2)))
print(show(string.byte("A", 2)))
print(rec(string.byte))
print(rec(string.byte, 42))
print(show(string.char(65, 66)))
print(rec(string.char, 300))
print(rec(string.char, -1))

print("== math ==")
print(show(math.floor(3.7), math.floor(-3.7), math.ceil(3.2), math.ceil(-3.2)))
print(show(math.floor(3), math.type(math.floor(3.0))))
print(rec(math.floor))
print(rec(math.floor, "x"))
print(show(math.fmod(7, 3), math.fmod(-7, 3), math.fmod(7.5, 2)))
print(rec(math.fmod, 1, 0))
print(show(math.abs(-1), math.abs(-1.5), math.abs(math.mininteger)))
print(rec(math.abs))
print(show(math.max(1, 2, 3), math.min(1, 2, 3)))
print(show(math.max(1), math.min(1)))
print(rec(math.max))
print(rec(math.max, 1, "x"))
print(show(math.tointeger(3.0)))
print(show(math.tointeger(3.5)))
print(show(math.tointeger("3")))
print(show(math.tointeger(3)))
print(show(math.tointeger(2 ^ 70)))
print(rec(math.tointeger))
print(show(math.type(1), math.type(1.0), math.type("x"), math.type(nil)))
print(rec(math.type))
print(show(math.sqrt(4), math.sqrt(2) > 1.41))
print(rec(math.sqrt, -1))
print(rec(math.sqrt, "x"))
print(show(math.ult(1, 2), math.ult(2, 1), math.ult(-1, 1)))
print(rec(math.ult, 1))
print(rec(math.ult, 1, "x"))
print(show(math.pi > 3.14, math.huge == math.huge, math.maxinteger + 1 == math.mininteger))
-- math.random(0) is a full-range random integer, so nothing about the
-- default seed can be asserted: only the seeded sequence below is stable
print(rec(math.random, 5, 1))
print(rec(math.random, 1, 2, 3))
print(rec(math.random, "x"))
print(rec(math.random, 1, "x"))
print(rec(math.random, 1.5))
do
  math.randomseed(42)
  local a = math.random(1, 1000)
  math.randomseed(42)
  local b = math.random(1, 1000)
  print(a == b)
end
print("done")
