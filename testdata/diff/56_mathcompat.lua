-- The eight functions 'LUA_COMPAT_MATHLIB' puts in the math library.
--
-- luaconf.h defines 'LUA_COMPAT_MATHLIB' and the reference build leaves
-- it on, so 'math.sinh', 'math.cosh', 'math.tanh', 'math.pow',
-- 'math.frexp', 'math.ldexp', 'math.log10' and 'math.atan2' are all
-- there even though Lua 5.4 dropped them from the manual.  This port
-- did not have any of them, so a program that called one could not run
-- here at all.  They are asked over the whole grid, and the grid
-- reaches the special cases: infinities, NaN, signed zeros, exponents
-- that do not fit an 'int', and the band in which exp(|x|) overflows
-- while sinh(x) and cosh(x) do not.
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
local inf, nan = 1 / 0, 0 / 0
local vals = {0, -0.0, 1, -1, 2, -2, 0.5, -0.5, 3.7, -3.7, 10, -10, 100, -100,
              1e308, -1e308, 1e-308, -1e-308, 710, -710, 709, 1e15, 1e-15,
              inf, -inf, nan, math.maxinteger, math.mininteger, 2 ^ 53}

print("== they are all there ==")
for _, k in ipairs{"atan2", "cosh", "sinh", "tanh", "pow", "frexp", "ldexp",
                   "log10"} do
  print(k, type(math[k]))
end

print("== sinh cosh tanh ==")
for _, name in ipairs{"sinh", "cosh", "tanh"} do
  local f = math[name]
  for _, v in ipairs(vals) do
    print(name, show(pcall(f, v)))
  end
end

print("== the band where exp(|x|) overflows but sinh(x) does not ==")
-- Go's math.Sinh answers inf here; sinh(710) is exp(710) / 2, which is
-- 1.1e308 and perfectly representable.
for _, x in ipairs{709, 709.5, 709.78, 709.79, 709.8, 709.9, 710, 710.2,
                   710.4, 710.47, 710.48, 710.5, 711, 800} do
  print(x, show(pcall(math.sinh, x)), show(pcall(math.cosh, x)),
        show(pcall(math.sinh, -x)), show(pcall(math.cosh, -x)))
end

print("== pow ==")
for _, a in ipairs{0, -0.0, 1, -1, 2, -2, 0.5, -0.5, 3.7, -3.7, 10, 1e308,
                   inf, -inf, nan} do
  local out = {}
  for _, b in ipairs{0, -0.0, 1, -1, 2, -2, 0.5, -0.5, 3, 0.3333, 1e308,
                     inf, -inf, nan} do
    local ok, r = pcall(math.pow, a, b)
    out[#out + 1] = ok and tostring(r) or "E"
  end
  print(tostring(a), table.concat(out, " "))
end

print("== frexp ==")
for _, v in ipairs(vals) do
  print(tostring(v), show(pcall(math.frexp, v)))
end
print(show(pcall(math.frexp, 2 ^ -1074)), show(pcall(math.frexp, 2 ^ -1075)))

print("== ldexp ==")
for _, v in ipairs{0, -0.0, 1, -1, 2, 0.5, -0.5, 3.7, 1e308, inf, -inf, nan} do
  local out = {}
  for _, e in ipairs{0, 1, -1, 2, 52, 53, 1023, 1024, -1023, -1074, -1075,
                     10000, -10000, math.maxinteger, math.mininteger,
                     2 ^ 31, 2 ^ 31 + 5, -2 ^ 31} do
    local ok, r = pcall(math.ldexp, v, e)
    out[#out + 1] = ok and tostring(r) or "E"
  end
  print(tostring(v), table.concat(out, " "))
end

print("== log10 ==")
for _, v in ipairs(vals) do
  print(tostring(v), show(pcall(math.log10, v)))
end

print("== atan2 ==")
for _, a in ipairs{0, -0.0, 1, -1, 2, -2, 0.5, inf, -inf, nan, 1e308} do
  local out = {}
  for _, b in ipairs{0, -0.0, 1, -1, 2, -2, 0.5, inf, -inf, nan} do
    local ok, r = pcall(math.atan2, a, b)
    out[#out + 1] = ok and tostring(r) or "E"
  end
  print(tostring(a), table.concat(out, " "))
end
print("== atan2 and atan are the same function ==")
print(show(pcall(math.atan2, 1, 2)), show(pcall(math.atan, 1, 2)),
      show(pcall(math.atan, 1)))

print("== argument checks ==")
for _, name in ipairs{"sinh", "cosh", "tanh", "log10"} do
  local f = math[name]
  print(name, show(pcall(f)), show(pcall(f, "x")), show(pcall(f, nil)),
        show(pcall(f, "1")), show(pcall(f, true)), show(pcall(f, 1, 2)))
end
print(show(pcall(math.pow, 1)), show(pcall(math.pow)),
      show(pcall(math.pow, "x", 1)), show(pcall(math.pow, 1, "y")))
print(show(pcall(math.frexp)), show(pcall(math.frexp, "x")),
      show(pcall(math.frexp, nil)))
print(show(pcall(math.ldexp, 1)), show(pcall(math.ldexp, "x", 1)),
      show(pcall(math.ldexp, 1, "y")))
-- Not the message of the arity error, though: 'math.atan2' and
-- 'math.atan' are the same function, and the name in the message is
-- whichever of the two the search over 'package.loaded' reaches first,
-- which follows the hash seed and so changes from one run of PUC-Lua
-- to the next.
print(show(pcall(math.atan2, 1)), show(pcall(math.atan2, 1, 2, 3)))
print("done")
