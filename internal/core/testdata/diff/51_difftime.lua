-- os.difftime and the integer edges of the two-argument arithmetic.
--
-- What pins 'os.difftime' is the *order* of the subtraction and the
-- conversion: C hands two 'time_t' to the C library's 'difftime', which
-- answers a double, so the difference is taken in floating point.  Taking
-- it in integers first and converting after overflows instead, which is
-- what 'os.difftime(0, math.mininteger)' is here to catch -- it answers
-- -9.2233720368548e+18 if the subtraction happens before the
-- conversion, and +9.2233720368548e+18 if it happens after.
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

local mi, ma = math.mininteger, math.maxinteger

print("== difftime ==")
print(show(os.difftime(0, 0)))
print(show(os.difftime(100, 50)))
print(show(os.difftime(50, 100)))
print(show(os.difftime(0, 1)))
print(show(os.difftime(1, 0)))
print(show(os.difftime(0, mi)))
print(show(os.difftime(mi, 0)))
print(show(os.difftime(0, ma)))
print(show(os.difftime(ma, 0)))
print(show(os.difftime(mi, ma)))
print(show(os.difftime(ma, mi)))
print(show(os.difftime(mi, mi)))
print(show(os.difftime(ma, ma)))
print(show(os.difftime(mi, -1)))
print(show(os.difftime(ma, -1)))
print(show(os.difftime(-1, mi)))
print(show(os.difftime(-1, ma)))
print(show(os.difftime(mi, 1)))
print(show(os.difftime(ma, 1)))
print(show(os.difftime(1, mi)))
print(show(os.difftime(1, ma)))
print(show(os.difftime(0.5, 0)))
print(show(os.difftime(0, 0.5)))
print(rec(os.difftime, 1))
print(rec(os.difftime))
print(rec(os.difftime, "x", 1))
print(rec(os.difftime, 1, "x"))
print(rec(os.difftime, 1.5, 1))

print("== two-argument arithmetic at the edges ==")
for _, p in ipairs{{0, mi}, {mi, 0}, {0, ma}, {ma, 0}, {mi, ma}, {ma, mi},
                   {mi, mi}, {ma, ma}, {mi, -1}, {ma, -1}, {-1, mi}, {-1, ma},
                   {mi, 1}, {ma, 1}, {1, mi}, {1, ma}} do
  local a, b = p[1], p[2]
  print(show(rec(math.max, a, b), rec(math.min, a, b), rec(math.fmod, a, b),
             rec(math.ult, a, b)))
end
print("== mixed integer and float ==")
print(show(rec(math.max, mi, 0.0), rec(math.min, mi, 0.0)))
print(show(rec(math.max, 0.0, mi), rec(math.min, 0.0, mi)))
print(show(rec(math.max, ma, 0.0), rec(math.min, ma, 0.0)))
print(show(rec(math.max, mi, "x")))
print(show(rec(math.fmod, ma, 1.0)))
print(show(rec(math.fmod, mi, 1.0)))
print("done")
