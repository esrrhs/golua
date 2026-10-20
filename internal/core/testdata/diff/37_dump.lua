-- string.dump: the bytes our code generator and serializer produce have to
-- be the ones PUC-Lua produces, since a dumped chunk travels to other
-- interpreters.  Every chunk is built by 'load' with an explicit name so
-- that the bytes do not depend on where this file happens to live.
local function hex(s)
  return (s:gsub(".", function(c) return string.format("%02x", string.byte(c)) end))
end

local srcs = {
  "return 1",
  "return",
  "local a, b = 1, 2 return a + b",
  "return ...",
  "return function() return 2 end",
  "local t = {} for i = 1, 10 do t[i] = i * i end return t",
  "return 'hello', 1.5, 0x10, nil, true, 2^40, 1e300, 0.1",
  "if true then return 'a' else return 'b' end",
  "for i = 1, 3 do end while false do end repeat until true",
  "local function inner() return 1 end return inner()",
  "goto done ::done:: return 3",
  "local x <const> = 5 return x",
  "local x <close> = nil return x",
  "return 'a' .. 'b', #'abc', ('x'):rep(3)",
  "local a = 1; a = a + 1; a = a * 2; return a",
  "return setmetatable({}, {__index = function() return 1 end})",
  "return 1, 2, 3, 4, 5, 6, 7, 8, 9, 10",
  "return -1, -0.0, 0/0, 1/0, -1/0",
  "local s = [[a long string]] return s",
  "return (...)",
}
for i, s in ipairs(srcs) do
  local f, e = load(s, "=c" .. i)
  if not f then print(i, "load failed", e)
  else
    local ok, d = pcall(string.dump, f)
    print(i, ok, ok and #d, ok and hex(d))
  end
end

-- dumping with and without stripping is the same in 5.4 (only one form)
print(pcall(string.dump, load("return 1", "=x"), true))
print(pcall(string.dump, load("return 1", "=x"), false))
print(pcall(string.dump, print))
print(pcall(string.dump, 42))
print(pcall(string.dump))
print(select('#', string.dump(load("return 1", "=x"))))

-- round trip: dump, load the bytes, run them again, dump again
local f = load("return (...) * 2", "=rt")
local d = string.dump(f)
print(#d, type(load(d)))
print(load(d)(21))
print(string.dump(load(d)) == d)
print(string.dump(load(string.dump(load(d)))) == d)
print(load(d, "=other")(3))

-- loading broken bytes
print(pcall(load, d .. "x"))
print(pcall(load, d:sub(1, 10)))
print(pcall(load, ""))
print(pcall(load, "\27Lua"))
print(load(""))
print(load(d)(21))

-- a chunk with many constants and many upvalues
local big = {}
for i = 1, 200 do big[i] = "return " .. i end
local dumps = {}
for i = 1, 200 do
  local fn = load(big[i], "=b")
  dumps[i] = #string.dump(fn)
end
print(#dumps, dumps[1], dumps[200])
do
  local ups = {}
  local src = "local a1, a2, a3, a4, a5 = 1, 2, 3, 4, 5 "
  src = src .. "return function() return a1+a2+a3+a4+a5 end"
  print(#string.dump(load(src, "=u")))
end
print("done")
