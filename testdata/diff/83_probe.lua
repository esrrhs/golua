-- What the other 82 cases each ask about one corner of, asked once
-- about all of them: that every function is there, and that the ones
-- whose answers do not belong to the machine answer what PUC-Lua does.
--
-- 79_inventory is the same idea for the names alone; this one calls
-- them.  Deliberately not asked here, because the answer is the host's:
-- os.clock, os.time, os.date, math.random, anything that prints an
-- address, and os.setlocale with a locale this machine may not have.
--
-- io.popen and os.execute need a shell and 'true'; 42_os and 64_io2
-- already ask for both, so this is not the first case to.

-- every name, and its type
local funcs = {
  "os.getenv", "os.rename", "os.remove", "os.difftime", "os.execute",
  "os.setlocale", "os.tmpname", "os.exit",
  "io.popen", "io.open", "io.lines", "io.read", "io.write", "io.close",
  "io.tmpfile", "io.stdout", "io.stderr", "io.stdin",
  "debug.getlocal", "debug.setlocal", "debug.getupvalue", "debug.setupvalue",
  "debug.getregistry", "debug.getmetatable", "debug.setmetatable",
  "debug.traceback", "debug.getinfo", "debug.sethook", "debug.gethook",
  "debug.getuservalue", "debug.setuservalue", "debug.upvalueid",
  "debug.upvaluejoin",
  "coroutine.close", "coroutine.wrap", "coroutine.isyieldable",
  "coroutine.running", "coroutine.status",
  "string.pack", "string.unpack", "string.packsize", "string.format",
  "string.rep", "string.gmatch", "string.gsub",
  "table.move", "table.concat", "table.insert", "table.remove",
  "table.pack", "table.unpack",
  "math.fmod", "math.ult", "math.tointeger", "math.abs", "math.ceil",
  "math.floor", "math.max", "math.min", "math.sqrt", "math.type",
  "utf8.char", "utf8.codepoint", "utf8.len", "utf8.offset",
  "utf8.charpattern",
  "load", "loadstring", "dofile", "require", "warn", "select", "rawlen",
  "rawequal", "rawget", "rawset", "next", "ipairs", "pairs", "tostring",
  "tonumber", "pcall", "xpcall", "error", "setmetatable", "getmetatable",
  "assert", "collectgarbage", "print", "type", "unpack",
}
local t = {}
for _, n in ipairs(funcs) do
  local f = _G
  for part in n:gmatch("[^.]+") do
    if type(f) == "table" then f = f[part] else f = nil break end
  end
  t[#t + 1] = n .. "=" .. type(f)
end
print("present: " .. table.concat(t, " "))

print("utf8.charpattern", utf8.charpattern)
print("os.getenv", os.getenv("__NO_SUCH_VAR_9X__"))
print("math.type", math.type(1), math.type(1.0), math.type("1"))
print("math.ult", math.ult(1, 2), math.ult(2, 1), math.ult(-1, 2))
print("math.tointeger", math.tointeger(3.0), math.tointeger(3.5))
print("math.fmod", math.fmod(7, 3), math.fmod(-7, 3), math.fmod(7.5, 2))
print("table.move", table.concat(table.move({1, 2, 3, 4}, 2, 3, 1), ","))
print("table.pack", table.pack(1, nil, 3).n)
print("string.rep", #string.rep("ab", 100000))
print("string.format", string.format("%5.2f|%s|%q|%X|%c|%e|%g|%o|%d|%x",
  3.14159, "x", "a\nb", 255, 65, 12345.678, 0.0001, 8, -7, 255))
print("tostring-a-table-is-a-string", type(tostring({})))
print("tonumber", tonumber("0x10"), tonumber("10", 2), tonumber(" 10 "),
  tonumber("1e2"), tonumber("z"))
print("packsize", string.packsize("i4i2b"))
print("pack/unpack", (string.unpack("i4", string.pack("i4", -2))))
print("coroutine.status", coroutine.status(coroutine.running()))
print("isyieldable", coroutine.isyieldable())
print("select", select("#", 1, nil, 3), select(2, "a", "b", "c"))
print("rawlen", rawlen({1, 2, 3}))
print("xpcall-args", xpcall(function(a, b) return a + b end,
  function(e) return "H:" .. e end, 3, 4))
print("error-level-0", select(2, pcall(function() error("x", 0) end)))
print("debug.traceback-is-a-string", type(debug.traceback()))
print("debug.getinfo-is-a-table", type(debug.getinfo(1)))
print("debug.gethook", debug.gethook())
do
  local on = pcall(warn, "@on")
  local wrote = pcall(warn, "msg")
  pcall(warn, "@off")
  print("warn", on, wrote)
end

-- a pipe, and a subprocess: both need a shell
local p = io.popen("echo popen-ok")
if p then print("popen", p:read("l"), p:close()) else print("popen", "nil") end
print("os.execute-is-a-boolean", type(os.execute("true")))
local tn = os.tmpname()
print("tmpname", type(tn), #tn > 0)

-- the debug library: locals, upvalues, the registry, a shared metatable
do
  local function locals(a) local b = a + 1
    return debug.getlocal(1, 1), debug.getlocal(1, 2) end
  print("debug.getlocal", locals(10))
  print("debug.setlocal",
    (function() local x = 1 debug.setlocal(1, 1, 42) return x end)())
  local up = 5
  local function g() return up end
  print("debug.getupvalue", debug.getupvalue(g, 1))
  print("debug.setupvalue",
    (function() debug.setupvalue(g, 1, 9) return g() end)())
  print("debug.getregistry", type(debug.getregistry()))
  debug.setmetatable({}, {__tostring = function() return "MT" end})
  print("debug.setmetatable", type(debug.getregistry()))
end

-- to-be-closed, and the attribute it needs
print("tbc", (function()
  local x <close> = setmetatable({}, {__close = function() print("closed") end})
  return "body"
end)())

-- goto, integer division, and the bitwise operators
do
  local i = 0
  ::top:: i = i + 1
  if i < 3 then goto top end
  print("goto", i)
end
print("ops", 7 // 2, 7 % 2, 5 & 3, 5 | 3, 5 ~ 3, ~5, 1 << 3, 16 >> 2)

-- a binary chunk from this interpreter, loaded by this interpreter
print("dump/load", (load(string.dump(function() return 42 end)))())
print("load-env", (load("return X", "c", "t", {X = 7}))())
print("load-mode", select(2, pcall(load, "return 1", "c", "b")))
print("load-binary", (function()
  local b = string.dump(function() return 3 end)
  return load(b, "c", "b")()
end)())

print("vararg", (function(...) return select("#", ...) end)(1, 2, nil))
print("unpack", table.unpack({1, 2, 3}, 2, 3))
print("strings-index-string", getmetatable("").__index == string)
print("string-methods", ("abc"):upper(), (" abc "):gsub("^%s+", ""),
  ("a,b,c"):gmatch("%a")())
print("alive", _VERSION)
