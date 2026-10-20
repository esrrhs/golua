-- the base library's remaining corners, math.random's own PRNG, the %q
-- escapes, and the table functions that raise.
--
-- 'rawget', 'rawset' and 'rawequal' are the interesting ones: they check
-- their arguments with 'luaL_checkany', which asks only whether a value
-- is there (never what it is), so the message names no type at all.

local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else t[i] = tostring(v) end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end

-- 1. luaL_checkany: absent, but no type is named
print(rec(rawget, {}))
print(rec(rawset, {}))
print(rec(rawset, {}, 1))
print(rec(rawequal, 1))
print(rec(rawequal))
print(rec(rawlen))
print(rec(next, {}))

-- 2. select
print(show(pcall(select, "#")), show(pcall(select, "#", 1, 2, 3)))
print(show(select(1, "a", "b", "c")), show(select(3, "a", "b", "c")))
print(show(select(4, "a", "b", "c")), show(pcall(select, 0, "a", "b")))
print(show(select(-1, "a", "b", "c")), show(pcall(select, -4, "a", "b", "c")))
print(show(pcall(select, 1.5, "a")), show(pcall(select, "x", "a")))
print(show(pcall(select)), show(pcall(select, "2", "a", "b")))

-- 3. type, rawequal, rawlen
for _, v in ipairs{nil, true, 42, 42.5, "x", {}, print} do
  print(type(v), rawequal(v, v), rec(rawlen, v), rec(rawget, v, 1))
end
print(rawequal(1, 1.0), rawequal("a", "a"), rawequal({}, {}), rawequal(nil, false))
print(rec(rawset, 42, 1, 1), rec(rawset, "x", 1, 1))
do
  local t = {}
  print(rawset(t, 1, "a") == t, rawget(t, 1))
end

-- 4. next, ipairs, pairs
do
  local n = {a = 1, b = 2}
  local seen = {}
  for k, v in next, n do seen[#seen + 1] = tostring(k) .. "=" .. tostring(v) end
  table.sort(seen)
  print(table.concat(seen, ","))
  print(next({}, nil), rec(next, 42), rec(next, n, "nope"))
end
do
  local out = {}
  for i, v in ipairs({"a", "b"}) do out[#out + 1] = i .. v end
  print(table.concat(out, ","))
  out = {}
  for i, v in ipairs({[1] = "a", [3] = "c"}) do out[#out + 1] = i .. v end
  print(table.concat(out, ","))
  print(rec(ipairs, 42), rec(ipairs, nil), rec(ipairs, "x"))
end
do
  local proxy = setmetatable({}, {__index = function(_, i) return i * 10 end})
  local out = {}
  for i, v in ipairs(proxy) do out[#out + 1] = i .. v if i > 3 then break end end
  print(table.concat(out, ","))
end
print(rec(pairs, 42), rec(pairs, nil))
print(rec(pairs, setmetatable({}, {__pairs = 42})))

-- 5. assert, error and its levels
print(show(pcall(assert, 1)), show(pcall(assert, 1, 2, 3)))
print(show(pcall(assert, false)), show(pcall(assert, nil, "message")))
print(show(pcall(assert)), show(pcall(assert, nil, nil)))
print(show(pcall(error, "plain")), show(pcall(error, "l0", 0)))
print(show(pcall(error, "l1", 1)), show(pcall(error, "l2", 2)))
print(show(pcall(error, nil)), show(pcall(error)))
print(show(pcall(error, "x", 1.5)), show(pcall(error, "x", "2")))

-- 6. pcall and xpcall
print(show(pcall(pcall)), show(pcall(pcall, error, "in pcall")))
print(show(pcall(pcall, coroutine.yield)))
print(show(pcall(xpcall, function() error("bad") end, function(m) return "H:" .. m end)))
print(show(pcall(xpcall, function() error("bad") end, function(m) return "H:" .. m end, 1, 2)))
print(show(pcall(xpcall, function() error({}) end, function(m) return type(m) end)))
print(show(pcall(xpcall, print, 42)), show(pcall(xpcall)), show(pcall(xpcall, print)))

-- 7. math.random: Lua runs xoshiro256** on its own state, so the
--    sequence is the same everywhere
math.randomseed(42)
local out = {}
for i = 1, 20 do out[#out + 1] = string.format("%.17g", math.random()) end
print(table.concat(out, " "))
math.randomseed(42)
out = {}
for i = 1, 20 do out[#out + 1] = math.random(1, 100) end
print(table.concat(out, " "))
math.randomseed(123456789)
out = {}
for i = 1, 10 do out[#out + 1] = string.format("%.17g", math.random()) end
print(table.concat(out, " "))
math.randomseed(42)
print(math.random(0), math.random(1), math.random(1, 1))
print(show(math.random(0, 0)), show(math.random(-5, 5)))
print(math.random(math.maxinteger - 1, math.maxinteger) >= math.maxinteger - 1)
print(rec(math.random, 0), rec(math.random, 3, 1), rec(math.random, 1, 2, 3))
print(rec(math.random, "x"), rec(math.random, 1, "x"), rec(math.random, 1.5))
print(math.type(math.random(1, 2)))
print(show(math.randomseed(5)), show(math.randomseed(1, 2)))
print(rec(math.randomseed, "x"), rec(math.randomseed, 1.5))

-- 8. %q: the escaping is what 'load' has to read back
for _, s in ipairs{"", "a", "a\"b", "a\\b", "a\nb", "a\rb", "a\tb", "a\0b",
                   "\1\2\255", "\226\152\131", "line1\nline2\n", "tab\there",
                   "'", '"', "\n", "\r\n", "a\13\10b"} do
  print(string.format("%q", s))
end
print(show(string.format("%q", 1), string.format("%q", 1.0)))
print(show(string.format("%q", true), string.format("%q", nil)))
print(rec(string.format, "%q"), rec(string.format, "%q", {}))
print(load("return " .. string.format("%q", "a\nb"))() == "a\nb")

-- 9. table.sort, including a comparator that lies about the order
local function sorted(t, cmp)
  local a = {table.unpack(t)}
  local ok, err = pcall(table.sort, a, cmp)
  if not ok then return "ERR:" .. tostring(err) end
  local out = {}
  for i = 1, #a do out[i] = tostring(a[i]) end
  return table.concat(out, ",")
end
print(sorted({3, 1, 2}), sorted({5, 4, 3, 2, 1}), sorted({"b", "a", "c"}))
print(sorted({}, nil), sorted({1}, nil))
print(sorted({1, 2}, function(a, b) return a > b end))
print(sorted({1, 2, 3}, function(a, b) return true end))
print(sorted({1, 2, 3}, function(a, b) return false end))
print(show(pcall(table.sort, {1, 2}, 42)), show(pcall(table.sort, 42)))
print(show(pcall(table.sort)), show(pcall(table.sort, {1, {}})))
print(show(pcall(table.sort, {1, 2, 3}, function(a, b) error("cmp") end)))
do
  local t = {}
  for i = 1, 200 do t[i] = (i * 7919) % 200 end
  table.sort(t)
  local okp = true
  for i = 2, #t do if t[i - 1] > t[i] then okp = false end end
  print("sorted200", okp, t[1], t[200])
end

-- 10. unpack, pack, concat, insert, remove, move
print(show(pcall(table.unpack, {1, 2, 3}, 2)))
print(show(pcall(table.unpack, {1, 2, 3}, 0)))
print(show(pcall(table.unpack, {1, 2, 3}, 1, 0)))
print(show(pcall(table.unpack, {1, 2, 3}, 1, 100)))
print(show(pcall(table.unpack, {1, 2, 3}, 4, 3)))
print(rec(table.unpack, 42), rec(table.unpack), rec(table.unpack, {1, 2}, "1"))
do
  local p = table.pack(1, 2, 3)
  print(p.n, p[1], p[3], table.pack().n, table.pack(nil, nil).n)
end
print(show(pcall(table.concat, {1, 2, 3}, ",", 2, 3)))
print(show(pcall(table.concat, {}, ",")), show(pcall(table.concat, {1, 2}, ",", 3, 1)))
print(rec(table.concat, 42), rec(table.concat, {1, {}}), rec(table.concat, {1, 2}, 42))
do
  local t = {1, 2, 3}
  print(show(pcall(table.insert, t, 4)), t[4])
  table.insert(t, 1, 0)
  print(t[1], #t)
  print(show(pcall(table.insert, t, 100, 9)), show(pcall(table.insert, t)))
  print(show(pcall(table.insert, t, 1, 2, 3)), show(pcall(table.insert, 42, 1)))
end
do
  local t = {1, 2, 3}
  print(table.remove(t), table.remove(t, 1), #t)
  print(show(pcall(table.remove, t, 100)), show(pcall(table.remove, t, 0)))
  print(show(pcall(table.remove, {})), rec(table.remove), rec(table.remove, 42))
end
do
  local t = {1, 2, 3, 4, 5}
  print(show(pcall(table.move, t, 1, 3, 3)), table.concat(t, ","))
  t = {1, 2, 3}
  print(show(pcall(table.move, t, 1, 3, 1, {})), #({}))
  print(show(pcall(table.move, {1, 2}, 1, 2, 1, 42)))
  print(show(pcall(table.move, {1, 2}, 0, 2, 1)), rec(table.move, 42, 1, 2, 1))
end

-- 11. stack limits
do
  local depth = 0
  local function f()
    depth = depth + 1
    if depth > 1000000 then return depth end
    return f() + 1
  end
  print(rec(f) == "false|\"stack overflow\"")
end
do
  local function f(n) if n == 0 then return 0 end return 1 + f(n - 1) end
  print(rec(f, 100), rec(f, 10000), rec(f, 1000000))
end
do
  local t = {}
  setmetatable(t, {__index = t})
  print(rec(function() return t.x end))
end
