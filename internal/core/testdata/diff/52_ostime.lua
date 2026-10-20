-- The fields of the table 'os.time' takes, and the bounds 'getfield'
-- puts on them (loslib.c).  Only the errors are asked about: whether a
-- given date answers a particular number depends on the machine's
-- timezone, and nothing here may.
--
-- What pins the bounds is that the field has to survive both the
-- subtraction of 'delta' and a trip through 'struct tm', whose members
-- are 'int':
--
--   if (!(res >= 0 ? res - delta <= INT_MAX : INT_MIN + delta <= res))
--     return luaL_error(L, "field '%s' is out-of-bound", key);
--
-- so 'year' -- whose delta is 1900 -- is out of bound at -2147483648,
-- because 'tm_year' would be -2147485548, while 'month', whose delta is
-- 1, is still in bound there.  Converting first and checking after
-- answers nothing at all for the first of those.
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

local MAXI, MINI = 2147483647, -2147483648

print("== year: delta 1900 ==")
print(rec(os.time, {year = MAXI, month = 6, day = 15, hour = 12}) ~= nil)
print(rec(os.time, {year = MAXI + 1, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = MAXI + 1900, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = MAXI + 1901, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = MINI, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = MINI + 1899, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = MINI - 1, month = 6, day = 15, hour = 12}))
-- A field that is *in* bound can still ask for a date the C library's
-- 'mktime' will not represent -- any of these normalises to a year
-- before 1900 -- and whether it does is up to the host.  Those are left
-- out on purpose; see the README on 'os.time'.

print("== month: delta 1 ==")
print(rec(os.time, {year = 2000, month = MINI, day = 1, hour = 12}))
print(rec(os.time, {year = 2000, month = MAXI, day = 1, hour = 12}))
print(rec(os.time, {year = 2000, month = MAXI + 1, day = 1, hour = 12}))

print("== day, hour, min, sec: delta 0 ==")
print(rec(os.time, {year = 2000, month = 6, day = MAXI, hour = 12}))
print(rec(os.time, {year = 2000, month = 6, day = MAXI + 1, hour = 12}))
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = MAXI}))
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = MAXI + 1}))
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = 12, min = MAXI + 1}))
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = 12, sec = MAXI + 1}))

print("== missing and not an integer ==")
print(rec(os.time, {}))
print(rec(os.time, {year = 2000}))
print(rec(os.time, {year = 2000, month = 6}))
print(rec(os.time, {year = 2000, month = 6, day = 15}))
print(rec(os.time, {year = "x", month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = 1.5, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = 2000.0, month = 6, day = 15, hour = 12}) ~= nil)
print(rec(os.time, {year = true, month = 6, day = 15, hour = 12}))
print(rec(os.time, {year = {}, month = 6, day = 15, hour = 12}))
-- 'isdst' is the host's to interpret: see 42_os.lua.  What is compared
-- here is that the field is accepted, not the number it produces.
print(show(type(os.time{year = 2000, month = 6, day = 15, hour = 12, isdst = "x"})))
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = 12, isdst = 0}) ~= nil)
print(rec(os.time, {year = 2000, month = 6, day = 15, hour = 12, isdst = 1}) ~= nil)

print("== the argument itself ==")
print(rec(os.time, 42))
print(rec(os.time, "x"))
-- os.time() with no table is the current time
print(show(type(select(1, os.time()))))
print("done")
