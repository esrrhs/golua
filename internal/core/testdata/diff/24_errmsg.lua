-- Error messages, byte for byte: the wording, the type names (including
-- the '__name' a metatable can give) and the argument numbers.  A
-- drop-in Lua is judged on these as much as on what it computes, and the
-- position prefix is stripped so that the case is independent of the
-- file it lives in.

-- exhaustive differential: error messages.  A drop-in Lua has to get the
-- wording, the type names and the positions byte for byte.
local function try(f, ...)
  local ok, err = pcall(f, ...)
  print(ok, tostring(err):gsub("^.-:%d+: ", ""))
end

local vals = {
  nil, true, false, 0, 1.5, "s", {}, print, coroutine.create(function () end),
  io.stdout, setmetatable({}, {__name = "Named"}),
}

local ops = {
  arith_add = function (a) return a + 1 end,
  arith_sub = function (a) return 1 - a end,
  arith_mul = function (a) return a * 2 end,
  arith_div = function (a) return a / 2 end,
  arith_idiv = function (a) return a // 2 end,
  arith_mod = function (a) return a % 2 end,
  arith_pow = function (a) return a ^ 2 end,
  arith_unm = function (a) return -a end,
  concat = function (a) return a .. "x" end,
  concat2 = function (a) return "x" .. a end,
  len = function (a) return #a end,
  index = function (a) return a.x end,
  index2 = function (a) return a[1] end,
  newindex = function (a) a.x = 1 end,
  call = function (a) return a() end,
  method = function (a) return a:m() end,
  band = function (a) return a & 1 end,
  bor = function (a) return a | 1 end,
  bxor = function (a) return a ~ 1 end,
  bnot = function (a) return ~a end,
  shl = function (a) return a << 1 end,
  shr = function (a) return a >> 1 end,
  lt = function (a) return a < 1 end,
  le = function (a) return a <= 1 end,
  lt_str = function (a) return a < "x" end,
  eq = function (a) return a == {} end,
  forloop = function (a) for i = a, 10 do end end,
  forloop2 = function (a) for i = 1, a do end end,
  forloop3 = function (a) for i = 1, 10, a do end end,
}

local names = {}
for k in pairs(ops) do names[#names + 1] = k end
table.sort(names)
for _, name in ipairs(names) do
  for i, v in ipairs(vals) do
    try(ops[name], v)
  end
end

-- library argument errors
local libcalls = {
  function () return math.floor("x") end,
  function () return math.floor(nil) end,
  function () return math.floor() end,
  function () return math.abs(true) end,
  function () return math.max() end,
  function () return math.tointeger(1.5) end,
  function () return string.rep("a", -1) end,
  function () return string.rep("a", 1, "x") end,
  function () return string.sub("abc", "x") end,
  function () return string.char(-1) end,
  function () return string.char(256) end,
  function () return string.format("%d", "x") end,
  function () return string.format("%y", 1) end,
  function () return string.format("%d") end,
  function () return table.insert({}, 1, 2, 3) end,
  function () return table.concat({{}}) end,
  function () return table.concat({}, {}) end,
  function () return table.move({}, 1, 2, 3, 4, 5, 6) end,
  function () return table.sort({}, 1) end,
  function () return table.sort({1, 2}, function () return true end) end,
  function () return table.unpack({}, 1, 1e10) end,
  function () return os.date("%Q") end,
  function () return os.time({}) end,
  function () return utf8.char(-1) end,
  function () return utf8.codepoint("x") end,
  function () return utf8.len("\xff") end,
  function () return ("x"):gsub("x", {}) end,
  function () return select(0, 1) end,
  function () return select("x", 1) end,
  function () return rawget(1, 1) end,
  function () return rawset(1, 1, 1) end,
  function () return rawequal() end,
  function () return setmetatable(1, {}) end,
  function () return setmetatable({}, 1) end,
  function () return getmetatable() end,
  function () return tostring() end,
  function () return tonumber() end,
  function () return tonumber("10", 99) end,
  function () return ipairs(1) end,
  function () return next(1) end,
  function () return next({}, "nokey") end,
  function () return error() end,
  function () return assert(false) end,
  function () return pcall(1) end,
  function () return load(1) end,
  function () return load("x", 1) end,
  function () return coroutine.resume(1) end,
  function () return coroutine.close(1) end,
  function () return io.open() end,
  function () return io.lines(1) end,
  function () return debug.getinfo(1, "Z") end,
  function () return debug.setlocal(1, 0, 1) end,
  function () return require(1) end,
  function () return os.setlocale(1) end,
  function () return os.rename(1) end,
}
for _, f in ipairs(libcalls) do
  try(f)
end

-- error positions and levels
local function lvl(n) error("msg", n) end
try(function () lvl(0) end)
try(function () lvl(1) end)
try(function () lvl(2) end)
try(function () lvl(3) end)
try(function () error(setmetatable({}, {__tostring = function () return "obj" end})) end)
try(function () error({}) end)
try(function () error(nil) end)
try(function () local t = setmetatable({}, {__index = function () error("in index") end}); return t.x end)
try(function () local t = setmetatable({}, {__add = function () error("in add") end}); return t + 1 end)
try(function () local t = setmetatable({}, {__lt = function () error("in lt") end}); return t < t end)
try(function () local t = setmetatable({}, {__call = function () error("in call") end}); return t() end)
try(function () local t = setmetatable({}, {__gc = 1}); return t end)
try(function () local t = setmetatable({}, {__close = 1}); local x <close> = t end)

print("done")
