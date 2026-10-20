-- 'next' and the promise Lua makes about a traversal that is still in
-- progress, then 'table.sort': what counts as a legal comparator, what
-- happens when one is not, and what the sort does to equal elements.
--
-- Integer keys only, on purpose: the traversal order of the array part
-- and of integer hashes does not depend on the string hash seed (which
-- PUC-Lua draws from time and address), so these answers are stable.
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

print("== next: the contract ==")
-- A traversal of an empty table, and of a table with a hole in it.
do
  local out = {}
  local t = {}
  for k, v in next, t do out[#out + 1] = tostring(k) .. ":" .. tostring(v) end
  P("empty", #out)
  local u = {10, nil, 30, nil, 50}
  out = {}
  local n = 0
  for k, v in next, u do n = n + 1; out[#out + 1] = tostring(k) .. ":" .. tostring(v) end
  P("hole", n, table.concat(out, ","))
end

-- Keys that 'next' refuses.
P(rec(next, {}))
P(rec(next, {}, nil))
P(rec(next, {}, 1))
P(rec(next, {}, "x"))
P(rec(next, {}, 1.0))
P(rec(next, {1}, 2))
P(rec(next, {1}, 1.0))
P(rec(next, {1}, 0))
P(rec(next, {1}, 1.5))
P(rec(next, {1}, "1"))
P(rec(next, {1}, true))
P(rec(next, {1}, {}))
P(rec(next, {1}, print))
P(rec(next, {1, 2, 3}, 4))
P(rec(next, {1, 2, 3}, 3))
P(rec(next, {x = 1}, "y"))
P(rec(next, {x = 1}, "x"))
-- 'next' does not look at __index.
do
  local t = setmetatable({}, {__index = {a = 1}})
  local k, v = next(t)
  P("no __index", k, v)
  t.b = 2
  k, v = next(t)
  P("after set", k, v)
end
-- The state of a traversal is a key, and it survives being handed around.
do
  local t = {1, 2, 3, 4}
  local k, v = next(t)
  P("first", k, v)
  k, v = next(t, k)
  P("second", k, v)
  k, v = next(t, k)
  P("third", k, v)
  k, v = next(t, k)
  P("fourth", k, v)
  k, v = next(t, k)
  P("fifth", k, v)
  -- A key that was reached and then removed: 5.4 says 'next' may answer
  -- it or not, but it must not answer a key it was never given.
  local u = {1, 2, 3, 4}
  k, v = next(u)
  u[2] = nil
  local ok, k2, v2 = pcall(next, u, k)
  P("removed ahead", ok, k2, v2)
end

print("== next: writing while walking ==")
-- Changing a value that is already there is legal and must be seen.
do
  local t = {1, 2, 3}
  local out = {}
  for k, v in next, t do
    out[#out + 1] = k .. ":" .. v
    if k == 1 then t[3] = 33 end
  end
  P("change value", table.concat(out, ","))
end
-- Clearing the entry just visited is legal too.
do
  local t = {1, 2, 3}
  local out = {}
  for k, v in next, t do
    out[#out + 1] = k .. ":" .. v
    t[k] = nil
  end
  P("clear visited", table.concat(out, ","), #t)
  local n = 0
  for k in next, t do n = n + 1 end
  P("after", n)
end
-- Adding a brand new integer key while the array part is being walked:
-- PUC-Lua keeps walking the array part, so the new entry may or may not
-- show up. Both sides run the same ltable.c, so the answer must agree.
do
  local t = {1, 2, 3}
  local out = {}
  for k, v in next, t do
    out[#out + 1] = k .. ":" .. v
    t[10] = 10
  end
  P("add far", table.concat(out, ","))
end
-- Same thing, but the new key lands in the hash part.
do
  local t = {}
  for i = 1, 8 do t[i] = i end
  local out = {}
  for k, v in next, t do
    out[#out + 1] = k .. ":" .. v
    if k == 1 then t[1000] = 1000 end
  end
  P("add hash int", table.concat(out, ","))
end

print("== pairs and ipairs over a changing table ==")
do
  local t = {1, 2, 3, 4, 5}
  local out = {}
  for k, v in pairs(t) do
    out[#out + 1] = k .. ":" .. v
    if k == 2 then t[4] = nil end
  end
  P("pairs clear", table.concat(out, ","))
end
do
  local t = {1, 2, 3}
  local out = {}
  for i, v in ipairs(t) do
    out[#out + 1] = i .. ":" .. v
    t[#t + 1] = #t + 1
    if #out > 6 then break end
  end
  P("ipairs grow", table.concat(out, ","))
end
-- '#t' is a border, not a count; clearing from the right moves it.
do
  local t = {1, 2, 3, 4, 5}
  P("len", #t)
  t[5] = nil
  P("len after", #t)
  t[3] = nil
  P("len hole", #t)
  local t2 = {1, 2, 3}
  t2[5] = 5
  P("len gap", #t2)
  local t3 = {}
  for i = 1, 100 do t3[i] = i end
  for i = 100, 51, -1 do t3[i] = nil end
  P("len back", #t3)
end

print("== table.sort: arguments ==")
P(rec(table.sort, {}))
P(rec(table.sort, {1}))
P(rec(table.sort, {}, nil))
P(rec(table.sort, {}, function() return true end))
P(rec(table.sort, {}, print))
P(rec(table.sort))
P(rec(table.sort, nil))
P(rec(table.sort, "abc"))
P(rec(table.sort, 1))
P(rec(table.sort, {1, 2, 3}, 5))
P(rec(table.sort, {1, 2, 3}, "x"))
P(rec(table.sort, {1, 2, 3}, {}))

print("== table.sort: bad comparators ==")
-- A comparator that never says true: PUC-Lua detects it and complains.
P(rec(table.sort, {3, 2, 1}, function() return false end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return true end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return nil end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return 0 end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return 1 end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return a end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return b end))
P(rec(table.sort, {1, 2, 3}, function(a, b) return "" end))
-- A comparator that answers both ways for the same pair.
P(rec(function()
  local t = {}
  for i = 1, 40 do t[i] = i end
  table.sort(t, function(a, b) return (a % 2) == 0 end)
  return #t
end))
-- A comparator that raises.
P(rec(table.sort, {3, 1, 2}, function(a, b) error("boom") end))
P(rec(table.sort, {3, 1, 2}, function(a, b) error({}) end))
P(rec(table.sort, {3, 1, 2}, function(a, b) if a > 5 then error("x") end; return a < b end))
-- A comparator that yields: not allowed inside the C sort.
P(rec(function()
  return table.sort({3, 1, 2}, function(a, b) coroutine.yield() ; return a < b end)
end))
-- How many arguments the comparator is handed, and in what order.
do
  local seen, ok, e
  seen = {}
  ok, e = pcall(table.sort, {3, 1, 2}, function(...)
    seen[#seen + 1] = select("#", ...) .. ":" .. tostring((select(1, ...))) .. "<" .. tostring((select(2, ...)))
    return (select(1, ...)) < (select(2, ...))
  end)
  P("nargs", ok, e or "ok", table.concat(seen, " "))
end

print("== table.sort: what it produces ==")
do
  local t = {5, 3, 9, 1, 7, 2, 8, 4, 6, 0}
  table.sort(t)
  P("ints", table.concat(t, ","))
  local s = {"pear", "apple", "orange", "banana", "kiwi"}
  table.sort(s)
  P("strs", table.concat(s, ","))
  local m = {3, 1.5, 2, 0.5}
  table.sort(m)
  P("mixed", table.concat(m, ","))
  -- Sorting values that have no order is the comparator's problem.
  P(rec(table.sort, {1, "a"}))
  P(rec(table.sort, {"a", 1}))
  P(rec(table.sort, {true, false}))
  P(rec(table.sort, {{}, {}}))
  P(rec(table.sort, {nil, 1}))
  -- 'n' limits how much of the table is sorted.
  local u = {5, 4, 3, 2, 1}
  table.sort(u, nil)
  P("no n", table.concat(u, ","))
  local v = {5, 4, 3, 2, 1}
  local ok, e = pcall(table.sort, v)
  P("sorted", ok, table.concat(v, ","))
end
-- The sort is not stable; PUC-Lua's answer is a function of the input
-- size, so both sides must land on the same permutation.
do
  local t = {}
  for i = 1, 20 do t[i] = {k = 1, i = i} end
  table.sort(t, function(a, b) return a.k < b.k end)
  local out = {}
  for i = 1, 20 do out[i] = t[i].i end
  P("unstable20", table.concat(out, ","))
  local u = {}
  for i = 1, 30 do u[i] = {k = i % 3, i = i} end
  table.sort(u, function(a, b) return a.k < b.k end)
  out = {}
  for i = 1, 30 do out[i] = u[i].i end
  P("unstable30", table.concat(out, ","))
end
-- Sorting a table that only has a hash part.
do
  local t = {}
  for i = 1, 10 do t["k" .. i] = i end
  P(rec(table.sort, t))
  local n = 0
  for _ in pairs(t) do n = n + 1 end
  P("untouched", n)
end
-- A comparator that mutates the table is undefined; PUC-Lua reads it
-- through, and so must we.
do
  local t = {1, 2, 3, 4, 5, 6, 7, 8}
  local ok = pcall(table.sort, t, function(a, b) t[1] = 100; return a < b end)
  P("mutate", ok, table.concat(t, ","))
end

print("== table.sort: reverse and big ==")
do
  local t = {1, 2, 3, 4, 5}
  table.sort(t, function(a, b) return a > b end)
  P("reverse", table.concat(t, ","))
  local big = {}
  for i = 1, 500 do big[i] = (i * 7919) % 500 + 1 end
  table.sort(big)
  local h = 0
  for i = 1, 500 do h = (h * 31 + big[i]) % 1000003 end
  P("big hash", h, big[1], big[500])
  local sorted = true
  for i = 2, 500 do if big[i - 1] > big[i] then sorted = false end end
  P("big sorted", sorted)
end
