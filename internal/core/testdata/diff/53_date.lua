-- Every conversion specifier os.date accepts, in its UTC form --
-- which is what makes the answers independent of the machine's
-- timezone -- over a spread of timestamps, plus the two-character
-- 'E' and 'O' modifiers and the specifiers that are rejected.
--
-- '%z' is left out on purpose.  With '!' the time is UTC, but the
-- host's gmtime leaves its offset at the local value, so PUC-Lua
-- prints the local offset -- '+0800' here -- where a UTC offset
-- would be '+0000'.  Which of the two gets that right is up to
-- the C library, so it is not asked about.
-- Every conversion specifier os.date accepts, in its UTC form (which is
-- what makes the answers independent of the machine's timezone), over a
-- spread of timestamps.  The two-character 'E' and 'O' modifiers are
-- asked about too.
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

local times = {0, 1, 59, 86400, 951782400, 1000000000, 1234567890,
               2147483647, 4102444800, -1, -86400, -2208988800}
local specs = {"a", "A", "b", "B", "c", "C", "d", "D", "e", "F", "g", "G",
               "h", "H", "I", "j", "m", "M", "n", "p", "r", "R", "S", "t",
               "T", "u", "U", "V", "w", "W", "x", "X", "y", "Y", "Z",
               "%"}

print("== single specifiers ==")
for _, sp in ipairs(specs) do
  local out = {}
  for _, t in ipairs(times) do
    out[#out + 1] = tostring(os.date("!%" .. sp, t))
  end
  print(sp, show(table.concat(out, " ")))
end

print("== combined ==")
for _, f in ipairs{"!%Y-%m-%d", "!%H:%M:%S", "!%A %d %B %Y", "!%c", "!%x",
                   "!%X", "!%I:%M %p", "!%j %U %W", "!%s", "!%z %Z",
                   "!%%%Y%%", "!a%nb%tc", "!%Y-%m-%dT%H:%M:%S%z"} do
  print(string.format("%q", f), show(os.date(f, 1234567890)))
end

print("== E and O modifiers ==")
for _, sp in ipairs{"Ec", "EC", "Ex", "EX", "Ey", "EY", "Od", "Oe", "OH",
                    "OI", "Om", "OM", "OS", "Ou", "OU", "OV", "Ow", "OW",
                    "Oy"} do
  print(sp, show(os.date("!%" .. sp, 1234567890)))
end

print("== rejected ==")
for _, sp in ipairs{"q", "Q", "N", "1", "E", "O", "Ez", "OZ", " ", "-", "l",
                    "k", "s", "P", "f", "L"} do
  print(string.format("%q", sp), rec(os.date, "!%" .. sp, 0))
end

print("== the second argument ==")
print(rec(os.date, "!%Y", "x"))
print(rec(os.date, "!%Y", 1.5))
print(rec(os.date, "!%Y", 1.0))
print(rec(os.date, "!%Y", 2 ^ 31))
print(rec(os.date, "!%Y", 2 ^ 40))
print(rec(os.date, "!%Y", -2 ^ 40))
print(rec(os.date, "!%Y", math.mininteger))
print(rec(os.date, "!%Y", math.maxinteger))
print(rec(os.date, "!%Y", 0 / 0))
print(rec(os.date, "!%Y", 1 / 0))
print(rec(os.date, 42))
print(rec(os.date, true))
print(rec(os.date, {}))

print("== the table form ==")
local function keys(t)
  local out, k = {}, {}
  for key in pairs(t) do k[#k + 1] = key end
  table.sort(k)
  for _, key in ipairs(k) do out[#out + 1] = key .. "=" .. tostring(t[key]) end
  return table.concat(out, " ")
end
for _, t in ipairs{0, 951782400, 1234567890, 2147483647, -1, -2208988800} do
  print(t, keys(os.date("!*t", t)))
end
print(rec(os.date, "!*t", "x"))
print(rec(os.date, "!*t", 0, 1))
print(rec(os.date, "!z*t", 0))
print(rec(os.date, "!q*t", 0))
print(rec(os.date, "!*tx", 0))
print(rec(os.date, "*t", 1234567890) ~= nil)
print("done")
