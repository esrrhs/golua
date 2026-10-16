-- the io library against real files: every read format, seek, lines,
-- closed handles, and the errors.  The files are made with os.tmpname and
-- removed again, so nothing is left behind.
--
-- Nothing may depend on whether stdout is seekable: the reference output
-- is captured through a pipe, as 'go test' runs it.
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

print("== stdout/stderr ==")
print(io.type(io.stdout), io.type(io.stderr), io.type(io.stdin))
print(io.type(42), io.type(nil), io.type("x"))
-- io.stdout:seek() depends on whether stdout is seekable
io.stdout:flush()
print(io.stdout:flush() ~= nil)

print("== open modes ==")
local name = os.tmpname()
local f = io.open(name, "w")
print(f ~= nil, io.type(f))
f:write("line1\nline2\nline3\n")
f:write("10 20.5 0x10 abc\n")
f:close()

for _, mode in ipairs{"r", "rb", "r+", "rw", "a", "ab", "a+"} do
  local ok, h, msg = pcall(io.open, name, mode)
  if not ok then print(mode, "ERR:" .. tostring(h))
  elseif h == nil then print(mode, false, tostring(msg))
  else print(mode, true, io.type(h)) h:close() end
end
print(rec(io.open, name, "x"))
print(rec(io.open, name, "z"))
print(rec(io.open, name, "r", 42))
print(rec(io.open))
print(rec(io.open, 42))
print(rec(io.open, nil, "r"))
print(rec(io.open, "no_such_file_xyz"))
do  -- "w" would leave the file behind and poison later cases
  local w = os.tmpname()
  local h = io.open(w, "w")
  print(h ~= nil, io.type(h))
  h:close()
  os.remove(w)
end

print("== read formats ==")
local function reads(mode, ...)
  local h = io.open(name, "r")
  local r = {h:read(mode, ...)}
  h:close()
  return show(table.unpack(r))
end
print("a", reads("a"))
print("l", reads("l"))
print("ll", reads("l", "l"))
print("L", reads("L"))
print("LL", reads("L", "L"))
print("n", reads("n"))
print("nn", reads("n", "n"))
print("nnnn", reads("n", "n", "n", "n"))
print("4", reads(4))
print("0", reads(0))
print("100", reads(100))
print("la", reads("l", "a"))
print("al", reads("a", "l"))
print("*l", reads("*l"))
print("*a", reads("*a"))
print("*n", reads("*n"))
print("*L", reads("*L"))

local function readbad(mode)
  local h = io.open(name, "r")
  local ok, err = pcall(h.read, h, mode)
  h:close()
  return ok and "true|" .. show(err) or "false|" .. show(err)
end
print("bad1", readbad(42))
print("bad2", readbad("q"))
print("bad3", readbad(true))
print("bad4", readbad({}))
print("bad5", readbad(-1))

print("== lines ==")
do
  local out = {}
  for l in io.lines(name) do out[#out + 1] = l end
  print(#out, table.concat(out, "/"))
end
do
  local out = {}
  for l in io.lines(name, "L") do out[#out + 1] = show(l) end
  print(#out, table.concat(out, "/"))
end
do
  local out = {}
  for a, b in io.lines(name, "n", "n") do out[#out + 1] = tostring(a) .. "," .. tostring(b) end
  print(#out, table.concat(out, "/"))
end
do
  local h = io.open(name, "r")
  local out = {}
  for l in h:lines() do out[#out + 1] = l end
  h:close()
  print(#out)
end
do
  local h = io.open(name, "r")
  local out = {}
  for l in h:lines("L") do out[#out + 1] = show(l) end
  h:close()
  print(#out)
end
print(rec(io.lines, 42))
print(rec(io.lines))
print(rec(io.lines, "no_such_file_xyz"))
print(rec(io.lines, name, "q"))
print(rec(io.lines, name, 42))

print("== write ==")
local wn = os.tmpname()
local w = io.open(wn, "w")
print(show(pcall(w.write, w, "a", 1, 2.5, "b")))
print(show(pcall(w.write, w)))
print(show(pcall(w.write, w, nil)))
print(show(pcall(w.write, w, true)))
print(show(pcall(w.write, w, {})))
print(show(pcall(w.write, w, 1, nil, 2)))
w:close()
local r = io.open(wn, "r")
print(show(r:read("a")))
r:close()
print(os.remove(wn))

print("== seek ==")
local h = io.open(name, "r")
print(show(pcall(h.seek, h)))
print(h:seek("set", 0), h:read("l"))
print(h:seek("set", 6), h:read("l"))
print(h:seek("cur", 0))
print(h:seek("end", 0), h:read("l"))
print(h:seek("set"), h:read("l"))
print(rec(h.seek, h, "nowhere"))
print(rec(h.seek, h, "set", -1))
print(rec(h.seek, h, "set", 1e20))
print(rec(h.seek, h, "end", 100), h:read("a"))
h:close()
print(rec(h.seek, h))
print(rec(h.read, h))
print(rec(h.write, h, "x"))
print(rec(h.lines, h))
print(rec(h.flush, h))

print("== closing twice ==")
h = io.open(name, "r")
print(h:close())
print(show(pcall(h.close, h)))
print(show(pcall(h.read, h)))
print(io.type(h))
h = io.open(name, "r")
print(show(pcall(h.close, h)))
print(show(pcall(h.close, h)))

print("== output/input redirection ==")
local on = os.tmpname()
local old = io.output()
print(io.output(on))
io.write("redirected\n")
io.output(old)
local rr = io.open(on, "r")
print(rr:read("l"))
rr:close()
os.remove(on)
do  -- io.output(42) creates a file called "42" and takes over stdout
  local prev = io.output()
  print(rec(io.output, 42))
  io.output(prev)
  os.remove("42")
  print(rec(io.output))
  print(rec(io.input, 42))
  print(rec(io.input))
end

print("== file methods ==")
h = io.open(name, "r")
print(type(getmetatable(h)))
print(getmetatable(h).__index ~= nil)
print(getmetatable(h).__name)
print(rec(function() return h.nope() end))
print(rec(function() return h:nope() end))
print(rec(function() h.nope = 1 end))
print(show(pcall(function() return h.read(h, "l") end)))
h:close()
print(show(pcall(function() return h:read("l") end)))
print(os.remove(name))
print(io.open(name) == nil)
print("done")
