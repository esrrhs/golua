-- The io library: every mode of 'open', every format of 'read', 'seek',
-- 'lines' and the messages they fail with.
--
-- What pins the messages is 'luaL_fileresult', which asks whether the
-- file name is NULL and not whether it is empty (lauxlib.c):
--
--   if (fname != NULL) lua_pushfstring(L, "%s: %s", fname, strerror(en));
--   else               lua_pushfstring(L, "%s", strerror(en));
--
-- so 'io.open("")' answers ": No such file or directory", with the colon
-- and the space and nothing in front of them.  This port tested the
-- name for emptiness instead and answered without the prefix.  The same
-- split shows in 'os.remove', which passes a name, and 'os.rename',
-- which does not.
--
-- The files are made in the working directory, which is this directory,
-- and are removed both before and after, so that a run does not depend
-- on what an earlier one left behind.  File handles are asked about
-- with 'io.type', never with 'tostring': that prints an address.
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

local names = {"_io_a.txt", "_io_b.txt", "_io_c.txt", "_io_d.txt", "_io_m.txt"}
local function clean()
  for _, n in ipairs(names) do os.remove(n) end
end
clean()

print("== open with every mode ==")
for _, m in ipairs{"r", "w", "a", "r+", "w+", "a+", "rb", "wb", "ab", "r+b",
                   "w+b", "a+b", "rw", "", "x", "rw+", "rb+", "+", "rb++",
                   "r++"} do
  local ok, f, err = pcall(io.open, "_io_m.txt", m)
  if not ok then f, err = nil, f end
  print(string.format("%-8s %s %s", string.format("%q", m), io.type(f),
        tostring(ok and (f and "open" or err) or err)))
  if f then f:close() end
end
clean()

print("== open a file that is not there ==")
print(rec(io.open, "_io_nope.txt"))
print(rec(io.open, "_io_nope.txt", "w"))
print(rec(io.open, ""))
print(rec(io.open, "."))
print(rec(io.open))
print(rec(io.open, 5))
print(rec(io.open, "_io_x.txt", 5))
clean()

print("== write and read back ==")
local f = assert(io.open("_io_a.txt", "wb"))
print(rec(f.write, f, "hello\n"), rec(f.write, f, 42), rec(f.write, f, 4.5),
      rec(f.write, f, " ", true))
print(rec(f.write, f))
print(rec(f.write, f, {}))
print(rec(f.write, f, nil))
print(rec(f.close, f))
for _, fmt in ipairs{"l", "L", "a", "n", "*l", "*L", "*a", "*n", "*line",
                     "*number", "*all", 5, 0, "x", "*x", 100} do
  local g = assert(io.open("_io_a.txt", "rb"))
  print(string.format("%-8s", tostring(fmt)), rec(g.read, g, fmt))
  g:close()
end
local g = assert(io.open("_io_a.txt", "rb"))
print("all", show(g:read("a")))
g:close()
local g2 = assert(io.open("_io_a.txt", "rb"))
print(rec(g2.read, g2, "l", "l", "l", "a"))
g2:close()

print("== numbers ==")
local h = assert(io.open("_io_c.txt", "wb"))
h:write("3 -4 0x10 1e3 .5 -inf nan 12abc  abc 0x1p4 99999999999999999999\n")
h:close()
for i = 1, 13 do
  local g3 = assert(io.open("_io_c.txt", "rb"))
  print(i, rec(g3.read, g3, "n"))
  g3:close()
end

print("== seek ==")
local s = assert(io.open("_io_a.txt", "rb"))
for _, w in ipairs{"set", "cur", "end"} do
  for _, o in ipairs{0, 3, -3, 1000, -1000} do
    print(w, o, rec(s.seek, s, w, o))
  end
end
print(rec(s.seek, s), rec(s.seek, s, "bad"), rec(s.seek, s, "set", "3"))
print(rec(s.seek, s, "set", 1.5))
s:close()
print(rec(s.seek, s, "set"))
-- Not "io.stdout:seek", which really does move the offset of a
-- redirected stdout and overwrites what was written before it.

print("== lines ==")
local lf = assert(io.open("_io_a.txt", "rb"))
local out = {}
for l in lf:lines() do out[#out + 1] = string.format("%q", l) end
print(table.concat(out, " "))
lf:close()
for _, fmt in ipairs{"l", "L", "n", "*l", 5, "x"} do
  local h4 = assert(io.open("_io_a.txt", "rb"))
  local ok, err = pcall(function()
    local o = {}
    for l in h4:lines(fmt) do o[#o + 1] = tostring(l) end
    print(string.format("%-6s", tostring(fmt)), table.concat(o, " "))
  end)
  if not ok then
    print(string.format("%-6s", tostring(fmt)), "ERR", tostring(err))
  end
  h4:close()
end
print(rec(io.lines, "_io_a.txt"))
print(rec(io.lines, "_io_nope.txt"))
print(rec(io.lines))
local o2 = {}
for l in io.lines("_io_a.txt") do o2[#o2 + 1] = string.format("%q", l) end
print(table.concat(o2, " "))

print("== a file with no newline at the end ==")
local g5 = assert(io.open("_io_b.txt", "wb"))
g5:write("no newline")
g5:close()
local h5 = assert(io.open("_io_b.txt", "rb"))
print(show(h5:read("l")), show(h5:read("l")), show(h5:read("L")),
      show(h5:read("a")))
h5:close()

print("== close and flush ==")
local g6 = assert(io.open("_io_a.txt", "rb"))
print(rec(g6.close, g6), io.type(g6), rec(g6.close, g6), rec(g6.read, g6, "l"))
print(io.type(io.stdout), io.type(io.stderr), io.type(io.stdin), io.type(5))
print(rec(io.stdout.flush, io.stdout))
print(rec(io.stdout.write, io.stdout, ""))
print(rec(io.write, ""))
print(show(io.stdout == io.stdout, io.stdout ~= io.stderr))

print("== the os file functions ==")
print(rec(os.remove, "_io_b.txt"), rec(os.remove, "_io_nope.txt"))
print(rec(os.remove))
print(rec(os.rename, "_io_a.txt", "_io_d.txt"))
print(rec(os.rename, "_io_nope.txt", "_io_b.txt"))
print(rec(os.rename, "", ""))
print(type(os.tmpname()))
print(rec(os.getenv, "NO_SUCH_VAR_XYZ"))
print(rec(os.getenv))
print(rec(os.getenv, 5))
print(type(os.clock()))
print(rec(os.exit, "x"))

clean()
print("done")
