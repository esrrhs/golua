-- The errors that C raises through luaL_argexpected.  Unlike
-- luaL_argcheck, which only appends its own message, luaL_argexpected
-- goes through luaL_typeerror and therefore names the type that was
-- expected *and* the type that came:
--
--     string/function/table expected, got boolean
--
-- Every call site of luaL_argexpected in the 5.4.9 sources is exercised
-- here (lauxlib.c's luaL_checkudata, lbaselib.c's setmetatable and
-- rawlen, lcorolib.c's getco, ldblib.c's debug.setmetatable and
-- lstrlib.c's string.gsub).

local function try(f, ...)
  local ok, err = pcall(f, ...)
  -- strip the "file:line: " prefix so the case does not move when it does
  print(ok, (tostring(err):gsub("^.-:%d+: ", "")))
end

-- string.gsub: the third argument
for _, repl in ipairs{true, false, nil, 42, 42.5, "x", print, {}, coroutine.create(function() end)} do
  try(string.gsub, "hello", "l", repl)
end

-- rawlen: the first argument
for _, v in ipairs{true, 42, 42.5, print, nil, {}, "abc"} do
  try(rawlen, v)
end

-- setmetatable and debug.setmetatable: the second argument
for _, mt in ipairs{42, "x", true, print, {}, nil} do
  try(setmetatable, {}, mt)
  try(debug.setmetatable, {}, mt)
end

-- the coroutine library: everything that takes a thread
for _, v in ipairs{42, "x", true, {}, print} do
  try(coroutine.resume, v)
  try(coroutine.status, v)
  try(coroutine.close, v)
  try(coroutine.isyieldable, v)
end

-- luaL_checkudata: a file method on something that is not a file, with
-- and without a '__name' on the metatable (that is what gets reported)
local mt = setmetatable({}, {__name = "My Type"})
try(io.stdout.close, mt)
try(io.stdout.write, 42)
try(io.stdout.lines, "x")
try(io.stdout.read, true)
try(io.stdout.seek, print)
try(io.stdout.flush, {})

-- and the type name of a plain table, which has no '__name'
try(io.stdout.close, {})

-- a userdata of the right type, for contrast
try(io.stdout.write, io.stdout, "written through a real handle\n")
print("done")
