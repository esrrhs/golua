-- shebang lines, 'loadfile' and 'dofile' with their 'mode', the C
-- stack limit, yielding across a C boundary, and finalizers.  The only
-- file used is a temporary one, and it is removed again.
-- shebang, dofile/loadfile, 'os.exit', finalizers at close, C stack limits
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

print("== shebang ==")
print(rec(load, "#!/usr/bin/lua\nreturn 1"))
print(rec(load, "#comment\nreturn 1"))
print(rec(load, "\nreturn 1"))
print(rec(load, "  return 1"))
print(rec(load, "\xef\xbb\xbfreturn 1"))
do
  -- only 'loadfile' skips a first line starting with '#'; 'load' does not
  local f, err = load("#!/x\nreturn 7")
  print(show(type(f), err))
end
print(rec(load, "#"))

print("== loadfile and dofile ==")
do
  local n = os.tmpname()
  local f = io.open(n, "w")
  f:write("return 1 + 1\n")
  f:close()
  print(rec(loadfile, n))
  print(show(dofile(n)))
  print(rec(loadfile, n, "b"))
  print(rec(loadfile, n, "t", {}))
  os.remove(n)
end
print(rec(loadfile, "no_such_file_xyz"))
print(rec(loadfile))
print(rec(loadfile, 42))
print(rec(dofile, "no_such_file_xyz"))
print(rec(dofile))

print("== C stack overflow ==")
do
  local mt
  mt = {__index = function(t, k) return t[k] end}
  print(rec(function() local t = setmetatable({}, mt); return t.x end))
end
do
  local n = 0
  local function f() n = n + 1; return f() + 1 end
  print(rec(f))
  print(n > 100)
end
do
  local mt = {__add = function(a, b) return a + b end}
  print(rec(function() return setmetatable({}, mt) + 1 end))
end
do
  local mt = {__tostring = function(t) return tostring(t) end}
  print(rec(function() return tostring(setmetatable({}, mt)) end))
end
do
  local mt = {__eq = function(a, b) return a == b end}
  print(rec(function() return setmetatable({}, mt) == setmetatable({}, mt) end))
end
do
  local mt = {__lt = function() return true end}
  local a = setmetatable({}, mt)
  print(rec(function() return a < a end))
end

print("== yield across a C boundary ==")
do
  local co = coroutine.create(function()
    return (table.sort({2, 1}, function(a, b) coroutine.yield() return a < b end))
  end)
  print(rec(coroutine.resume, co))
end
do
  local co = coroutine.create(function()
    return string.gsub("abc", "%a", function() coroutine.yield() return "x" end)
  end)
  print(rec(coroutine.resume, co))
end
do
  local co = coroutine.create(function()
    for k in next, {1} do coroutine.yield() end
  end)
  print(rec(coroutine.resume, co))
end
do
  local co = coroutine.create(function() return (load(function() coroutine.yield() return "return 1" end)) end)
  print(rec(coroutine.resume, co))
end
do
  local co = coroutine.create(function() return require("nope") end)
  print(rec(coroutine.resume, co))
end

print("== finalizers ==")
do
  local log = {}
  do
    local t <close> = setmetatable({}, {__close = function() log[#log + 1] = "close" end})
  end
  print(table.concat(log, ","))
end
do
  local n = 0
  setmetatable({}, {__gc = function() n = n + 1 end})
  collectgarbage("collect")
  print(show(n == 1))
  collectgarbage("collect")
  print(show(n == 1))
end
do
  -- a finalizer that runs when the state is closed answers nothing that
  -- can be compared, so only the count after a full collection is asked
  local n = 0
  for i = 1, 5 do setmetatable({}, {__gc = function() n = n + 1 end}) end
  collectgarbage("collect")
  print(show(n))
end
print(rec(setmetatable, {}, {__gc = print}))
print("done")
