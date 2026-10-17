-- An inventory: every name the reference interpreter exposes, so a
-- missing or extra one shows up as a difference.  Printed sorted, so
-- the order of 'pairs' (which PUC-Lua randomises per run) cannot matter.
local function keys(t)
  local out = {}
  for k in pairs(t) do out[#out + 1] = tostring(k) end
  table.sort(out)
  return out
end
local function dump(name, t)
  if type(t) ~= "table" then print(name, "= " .. type(t)); return end
  print("== " .. name .. " (" .. #keys(t) .. ") ==")
  for _, k in ipairs(keys(t)) do
    print("  " .. k .. " : " .. type(t[k]))
  end
end

dump("_G", _G)
for _, lib in ipairs({"string", "table", "math", "io", "os", "debug",
                      "coroutine", "utf8", "package"}) do
  dump(lib, _G[lib])
end

print("== metamethods recognised ==")
local mm = {"__index", "__newindex", "__call", "__tostring", "__len", "__eq",
            "__lt", "__le", "__concat", "__add", "__sub", "__mul", "__div",
            "__mod", "__pow", "__unm", "__idiv", "__band", "__bor", "__bxor",
            "__bnot", "__shl", "__shr", "__close", "__gc", "__mode", "__name",
            "__pairs", "__ipairs", "__metatable"}
for _, m in ipairs(mm) do
  -- A metamethod is recognised if setting it changes behaviour; here we
  -- only check that the name is not rejected by 'setmetatable' and that
  -- 'debug.getmetatable' hands it back.
  local t = setmetatable({}, {[m] = function() end})
  print("  " .. m .. " : " .. tostring(debug.getmetatable(t)[m] ~= nil))
end

print("== version-visible constants ==")
print("  _VERSION", _VERSION)
print("  math.pi", math.pi, "huge", math.huge)
print("  maxinteger", math.maxinteger, "mininteger", math.mininteger)
print("  package.config", string.format("%q", package.config))
print("  utf8.charpattern", string.format("%q", utf8.charpattern))

print("== collectgarbage options ==")
-- Only that the option is accepted: there is no collector here to tune,
-- so what PUC-Lua answers for them cannot be matched.
for _, o in ipairs({"collect", "stop", "restart", "count", "step", "setpause",
                    "setstepmul", "isrunning", "incremental", "generational"}) do
  print("  " .. o .. " : " .. tostring(pcall(collectgarbage, o)))
end

print("== os.exit signature ==")
print("  os.exit", type(os.exit))
local info = debug.getinfo(os.getenv, "Su")
print("  C fn nparams", info.nparams, "isvararg", info.isvararg, "what", info.what)
