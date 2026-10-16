-- os.* and package/require -- the two libraries that reach outside the
-- process, plus the module loader's path search.
--
-- Three things are deliberately left out:
--
--   - 'os.time()' and 'os.date()' with no argument.  They answer with
--     the wall clock, so no golden file can hold them.  'os.clock',
--     'os.tmpname' and every call that passes an explicit time are
--     here, but only through 'type' or an explicit stamp.
--
--   - 'package.loadlib'.  It is the one entry point that can only be
--     answered by loading a shared object, which a Go program cannot
--     do; the reference build on this host answers with a 'dlopen'
--     message and "open", this one with "dynamic libraries not
--     enabled" and "absent" -- PUC-Lua's own answer when it is built
--     without 'LUA_USE_DLOPEN'.
--
--   - 'os.getenv'.  Only the type of the answer is asked for: 'go test'
--     puts the Go toolchain at the front of PATH, so the value of that
--     variable is not the same inside the test as it was when the
--     golden file was captured.
--
--   - 'os.date' beyond the year 2147485547.  'struct tm' keeps the year
--     in an 'int' and the C library refuses to fill it in, so the
--     reference answers "date result cannot be represented in this
--     installation"; the cut-off at the bottom of the range is the
--     host C library's and is not reproduced to the second.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    elseif type(v) == "number" then t[i] = string.format("%.17g", v)
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end
local function keys(t)
  local o = {}
  for k in pairs(t) do o[#o + 1] = tostring(k) end
  table.sort(o)
  return table.concat(o, ",")
end

print("== os.setlocale ==")
print(rec(os.setlocale))
print(rec(os.setlocale, "C"))
print(rec(os.setlocale, "C", "all"))
print(rec(os.setlocale, nil, "all"))
print(rec(os.setlocale, "C", "ctype"))
print(rec(os.setlocale, "C", "numeric"))
print(rec(os.setlocale, "C", "collate"))
print(rec(os.setlocale, "C", "time"))
print(rec(os.setlocale, "C", "monetary"))
print(rec(os.setlocale, "C", "bogus"))
print(rec(os.setlocale, "xx_XX"))
print(rec(os.setlocale, "C", 5))
print(rec(os.setlocale, 5))
print(rec(os.setlocale, "C", "all", "extra"))

print("== os.time ==")
print(rec(os.time, {year = 1970, month = 1, day = 1, hour = 0}))
print(rec(os.time, {year = 1970, month = 1, day = 1, hour = 0, min = 0, sec = 0}))
print(rec(os.time, {year = 2000, month = 2, day = 29, hour = 12}))
print(rec(os.time, {year = 2000, month = 2, day = 29, hour = 12, isdst = false}))
print(rec(os.time, {year = 2000, month = 2, day = 29, hour = 12, isdst = true}))
print(rec(os.time, {year = 2038, month = 1, day = 19, hour = 3, min = 14, sec = 7}))
print(rec(os.time, {year = 1969, month = 12, day = 31, hour = 0}))
print(rec(os.time, {}))
print(rec(os.time, {year = 1970, month = 1, day = 1, hour = 0, sec = 61}))
print(rec(os.time, {year = 1970, month = 13, day = 1}))
print(rec(os.time, {year = 1970, month = 1, day = 32}))
print(rec(os.time, {year = 1970, month = 0, day = 1}))
print(rec(os.time, {year = 1970, month = 1, day = 0}))
print(rec(os.time, {year = 1970, month = 1, day = 1, hour = 25}))
print(rec(os.time, {year = 1970, month = 1, day = 1, hour = 0, min = 0, sec = 0, x = 1}))
print(rec(os.time, 0))
print(rec(os.time, "x"))

print("== os.date ==")
print(rec(os.date, "!%Y-%m-%d %H:%M:%S", 0))
print(rec(os.date, "!%Y-%m-%d %H:%M:%S", 2000000000))
print(rec(os.date, "!%Y-%m-%d %H:%M:%S", -1))
print(rec(os.date, "!%a %A %b %B", 0))
print(rec(os.date, "!%c", 0))
print(rec(os.date, "!%x %X", 0))
print(rec(os.date, "!%p %I", 0))
print(rec(os.date, "!%j %U %W %w", 0))
print(rec(os.date, "!%% %n %t", 0))
print(rec(os.date, "!%C %y %G %g %u %V", 0))
print(rec(os.date, "!%e %k %l %s", 0))
print(rec(os.date, "!%Z", 0))
print(rec(os.date, "%Y", 0))
print(rec(os.date, "!*t", 0))
print(rec(os.date, "!*t", 2000000000))
print(rec(os.date, "*t", 0))
print(rec(os.date, "!*t", 2 ^ 62))
print(rec(os.date, "!%Y", 2 ^ 62))
print(rec(os.date, "!"))
print(rec(os.date, "!", "x"))
print(rec(os.date, "!", 0, 1))
print(rec(os.date, 5, 0))
print(rec(os.date, "!%Y", 0 / 0))
print(rec(os.date, "!%Y", 1 / 0))
print(rec(os.date, "%Q", 0))

print("== os.difftime / clock / tmpname / getenv ==")
print(rec(os.difftime, 100, 50))
print(rec(os.difftime, 50, 100))
print(rec(os.difftime, 100))
print(rec(os.difftime, "100", 50))
print(rec(os.difftime))
print(type(os.clock()))
print(type(os.tmpname()), #os.tmpname() > 0)
print(type(os.getenv("PATH")), #(os.getenv("PATH") or "") > 0)
print(rec(os.getenv, "NO_SUCH_VAR_XYZ_123"))
print(rec(os.getenv, ""))
print(rec(os.getenv))
print(rec(os.getenv, 5))

print("== package ==")
print(show(package.config))
print(show(package.path))
print(show(package.cpath))
print(type(package.loaded), type(package.preload), type(package.searchers))
print(#package.searchers, type(package.searchers[1]), type(package.searchers[2]),
      type(package.searchers[3]), type(package.searchers[4]))
print(type(package.loadlib), type(package.searchpath))
print(keys(package.loaded))
print(rec(package.searchpath, "no.such.module", package.path))
print(rec(package.searchpath, "a", "./?.lua;?"))
print(rec(package.searchpath, "a", "./?.lua"))
print(rec(package.searchpath, "a.b", "./?.lua", "."))
print(rec(package.searchpath, "a.b", "./?.lua", ".", ":"))
print(rec(package.searchpath, "a", "", ""))
print(rec(package.searchpath, "a", "?"))
print(rec(package.searchpath, "a", 5))
print(rec(package.searchpath, "a"))
print(rec(package.searchpath))
print(rec(package.searchpath, 5, "?"))

print("== require ==")
print(rec(require, "no_such_module_xyz"))
print(rec(require, "string"), rec(require, "math"))
print(package.loaded.no_such_module_xyz)
package.path = "/tmp/p36dir/?.lua;/tmp/p36dir/?/init.lua"
print(rec(require, "m1"), rec(require, "m1"))
print(rec(require, "m2"), rec(require, "m2"))
print(rec(require, "m3"))
print(rec(require, "sub/m4"))
print(rec(require, "sub.m4"))
print(rec(require, "bad"))
print(rec(require, "err"))
print(rec(require, "argm", "hello"))
print(package.loaded.m1, package.loaded.m2, package.loaded.m3)
print(rec(require, "circ1"))
print(rec(require))
print(rec(require, 5))
package.preload["pre1"] = function(name) return "pre:" .. name end
print(rec(require, "pre1"), rec(require, "pre1"))
package.loaded["pre2"] = "already"
print(rec(require, "pre2"))
package.loaded["pre3"] = false
print(rec(require, "pre3"))
print(rec(require, "pre3"))
print("done")
