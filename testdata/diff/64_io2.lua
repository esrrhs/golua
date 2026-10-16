-- io.popen, io.tmpfile and io.lines with a file name: the parts of the
-- io library that hand a stream to another process, or open a file the
-- caller never named.
--
-- The 'popen' section is guarded: the commands it runs are POSIX ones
-- and the reference interpreter runs them through 'sh', so on a host
-- without a POSIX shell the section prints a marker instead.  These
-- lines are the ones that found the bug -- 'pclose' has to flush the
-- buffer before it closes the pipe, or 'io.popen("wc -c", "w")' counts
-- nothing at all.
local unix = package.config:sub(1, 1) == "/"
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

if unix then
  print("== io.popen, reading ==")
  local p = io.popen("echo hello")
  print(type(p), io.type(p))
  print(p:read("l"), p:read("l"), p:read("a"))
  print(p:close())
  local p2 = io.popen("printf 'a\\nb\\nc\\n'")
  print(p2:read("a"), p2:close())
  local p3 = io.popen("printf 'x'")
  print(p3:read(1), p3:read(1), p3:close())
  local p4 = io.popen("exit 3")
  print(p4:read("a"), p4:close())
  local p5 = io.popen("kill -TERM $$")
  print(p5:close())

  print("== io.popen, writing ==")
  local q = io.popen("cat", "w")
  print(type(q), io.type(q))
  print(q:write("xyz"), q:write("123"))
  print(q:close())
  local q2 = io.popen("wc -c", "w")
  q2:write("abcdef")
  print(q2:close())
  local q3 = io.popen("cat > /tmp/p39cat.txt", "w")
  q3:write("stored\n")
  print(q3:close())
  local rf = io.open("/tmp/p39cat.txt")
  print(rf and rf:read("l"))
  rf:close()
  os.remove("/tmp/p39cat.txt")

  print("== io.popen, refused ==")
  print(rec(io.popen, "no_such_command_xyz_123 2>/dev/null"))
  print(rec(io.popen))
  -- The child's stderr is inherited, so a command that fails would write
  -- to this process's stderr -- and it does so whenever the kernel gets
  -- round to it, which is a race no golden file can hold.  The shell's
  -- own stderr is pointed at nothing first; what is being asked here is
  -- that a number is accepted as a command.
  print(rec(io.popen, "exec 2>/dev/null; 5"))
  print(rec(io.popen, "echo hi", "x"))
  print(rec(io.popen, "echo hi", 5))
  print(rec(io.popen, "echo hi", "r", "extra"))
else
  print("(popen needs a POSIX shell)")
end
print("== io.tmpfile ==")
local t = io.tmpfile()
print(type(t), io.type(t))
print(t:write("abc"), t:seek("set", 0), t:read("a"))
print(t:seek("end"), t:seek("cur"))
print(t:flush(), t:close())
print(rec(function() return io.tmpfile():close() end))
print(rec(io.tmpfile, "x"))

print("== io.lines with a file name ==")
local fn = "/tmp/p39data.txt"
local w = assert(io.open(fn, "w"))
w:write("a\nb\nc\n")
w:close()
local acc = {}
for l in io.lines(fn) do acc[#acc + 1] = l end
print(table.concat(acc, ","))
local acc2 = {}
for l in io.lines(fn, "L") do acc2[#acc2 + 1] = l end
print(table.concat(acc2, ","))
local acc3 = {}
for a, b in io.lines(fn, "l", "l") do acc3[#acc3 + 1] = tostring(a) .. tostring(b) end
print(table.concat(acc3, ","))
print(rec(function()
  local n = 0
  for x in io.lines(fn, "n") do n = n + 1 end
  return n
end))
print(rec(function()
  local out = {}
  for n in io.lines(fn, "n") do out[#out + 1] = n end
  return table.concat(out, ",")
end))
print(rec(function()
  local n = 0
  for x in io.lines(fn, "*n") do n = n + 1 end
  return n
end))
print(rec(io.lines, "/no/such/file/xyz"))
print(rec(io.lines, "/tmp"))
print(rec(io.lines))
print(rec(io.lines, 5))
print(rec(io.lines, fn, "z"))
print(os.remove(fn))

print("== file methods on a real file ==")
local g = assert(io.open(fn, "w"))
print(io.type(g), g:seek("set"), g:seek("cur"))
print(g:setvbuf("no"), g:setvbuf("full"), g:setvbuf("line"), g:setvbuf("full", 1024))
print(rec(g.setvbuf, g, "x"))
print(rec(g.setvbuf, g))
print(g:write("hello\n"))
print(g:flush())
print(g:seek("end"), g:seek("set", 0), g:read("a"))
print(rec(g.read, g, "z"))
print(g:close(), io.type(g))
print(rec(g.read, g, "l"))
print(rec(g.close, g))
print(os.remove(fn))
print("done")
