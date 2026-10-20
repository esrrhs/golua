-- the os library: os.date and os.time in their UTC form, which is what
-- makes the answers independent of the machine's timezone.  Nothing here
-- may ask for the current time.
--
-- 'os.setlocale' is only asked about its shape: whether a locale name is
-- accepted is up to the C library, and this port has none.
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

print("== date in UTC ==")
for _, t in ipairs{0, 1, 59, 60, 86399, 86400, 951782400, 1000000000,
                   1234567890, 2147483647, 4102444800} do
  print(t, os.date("!%Y-%m-%d %H:%M:%S", t), os.date("!%a %A %b %B", t))
end
print(show(os.date("!%c", 0)))
print(show(os.date("!%x", 0), os.date("!%X", 0)))
print(show(os.date("!%p", 0), os.date("!%p", 43200)))
print(show(os.date("!%I", 0), os.date("!%I", 43200)))
print(show(os.date("!%j", 0), os.date("!%j", 86400)))
print(show(os.date("!%w", 0), os.date("!%w", 86400)))
print(show(os.date("!%U", 0), os.date("!%W", 0)))
print(show(os.date("!%Z", 0)))
print(show(os.date("!%%", 0)))
print(show(os.date("!%n%t", 0)))
print(rec(os.date, "%Q", 0))
print(rec(os.date, "!%", 0))
print(rec(os.date, 42))
print(rec(os.date, "!%Y", "x"))
print(rec(os.date, "!%Y", 1.5))
print(rec(os.date, "!%Y", -1))
-- os.date() with no arguments is the current time

print("== date tables in UTC ==")
local function keys(t)
  local out, k = {}, {}
  for key in pairs(t) do k[#k + 1] = key end
  table.sort(k)
  for _, key in ipairs(k) do out[#out + 1] = key .. "=" .. tostring(t[key]) end
  return table.concat(out, " ")
end
for _, t in ipairs{0, 951782400, 1234567890} do
  print(t, keys(os.date("!*t", t)))
end
-- os.date("!*t") is the current time
print(show(os.date("!*t", 0).isdst))
print(show(os.date("*t") ~= nil))
print(rec(os.date, "!*t", 0, 1))
print(rec(os.date, "!z*t", 0))
print(rec(os.date, "!q*t", 0))

print("== time round trips ==")
for _, t in ipairs{0, 951782400, 1000000000, 1234567890, 2147483647} do
  local d = os.date("!*t", t)
  print(t, os.time(d) == t)
end
do
  local d = os.date("!*t", 1234567890)
  d.isdst = nil
  print(os.time(d))
end
do
  local d = os.date("!*t", 1234567890)
  d.sec = 30
  print(os.time(d) - 1234567890)
end
-- os.time() with no table is the current time
print(rec(os.time, 42))
print(rec(os.time, "x"))
print(rec(os.time, {}))
print(rec(os.time, {year = 2000}))
print(rec(os.time, {year = 2000, month = 1}))
print(rec(os.time, {year = 2000, month = 1, day = 1}))
print(rec(os.time, {year = 2000, month = 1, day = 1, hour = "x"}))
print(rec(os.time, {year = 2000, month = 13, day = 1}))
print(rec(os.time, {year = 2000, month = 0, day = 1}))
print(rec(os.time, {year = 2000, month = 1, day = 32}))
print(rec(os.time, {year = 2000, month = 1, day = 1, hour = 25}))
print(rec(os.time, {year = 2000, month = 1, day = 1, hour = 1, min = 60}))
print(rec(os.time, {year = 2000, month = 1, day = 1, hour = 1, min = 1, sec = 60}))
print(rec(os.time, {year = 2000, month = 1, day = 1, hour = 1.5}))
-- 'isdst' reaches the host's 'mktime', and what it does with it is the
-- host's own business: glibc shifts the answer by an hour for a true
-- 'isdst' even in a zone that has no DST at all, Apple's mktime does
-- not.  Only that the field is accepted is the same on both sides.
print(show(type(os.time{year = 2000, month = 1, day = 1, hour = 1, min = 1, sec = 1, isdst = "x"})))
print(show(os.time{year = 1970, month = 1, day = 1, hour = 0}))
print(show(os.time{year = 1970, month = 1, day = 2, hour = 0}))
print(show(os.time{year = 2000, month = 2, day = 29, hour = 12}))
print(show(os.time{year = 2001, month = 2, day = 29, hour = 12}))
print(show(os.time{year = 1969, month = 12, day = 31, hour = 0}))

print("== difftime ==")
print(os.difftime(100, 50), os.difftime(50, 100))
print(os.difftime(0, 0), os.difftime(1e9, 0))
print(rec(os.difftime))
print(rec(os.difftime, 1))
print(rec(os.difftime, 1, "x"))
print(rec(os.difftime, "x", 1))

print("== getenv ==")
print(os.getenv("NO_SUCH_ENVIRONMENT_VARIABLE_XYZ"))
print(rec(os.getenv))
print(rec(os.getenv, 42))
print(type(os.getenv("HOME") or os.getenv("PATH")))

print("== remove and rename ==")
print(rec(os.remove))
print(rec(os.remove, true))  -- a number would name a file that may exist
print(rec(os.remove, "no_such_file_xyz"))
print(rec(os.rename))
print(rec(os.rename, "no_such_file_xyz"))
print(rec(os.rename, "no_such_file_xyz", "b"))
print(rec(os.rename, "a", 42))
do
  local n = os.tmpname()
  local f = io.open(n, "w")
  f:write("x")
  f:close()
  print(os.rename(n, n .. "2"))
  print(os.remove(n .. "2"))
  local ok, msg = os.remove(n)
  print(ok, type(msg))  -- the message carries the random name
end

print("== execute ==")
print(rec(os.execute))
print(rec(os.execute, "exit 0"))
print(rec(os.execute, "exit 3"))
print(rec(os.execute, "exit 1"))
print(rec(os.execute, 42))
print(rec(os.execute, "no_such_command_xyz"))

print("== clock and tmpname ==")
print(type(os.clock()))
print(os.clock() >= 0)
print(type(select(1, os.clock(42))))
local n = os.tmpname()
print(type(n), #n > 0)
os.remove(n)

print("== setlocale ==")
print(type(select(1, os.setlocale())))
print(type(select(1, os.setlocale("C"))))
-- "pt_BR" answers a string where the C library has the locale
print(rec(os.setlocale, 42))
print(type(select(1, os.setlocale("C", "numeric"))))
print("done")
