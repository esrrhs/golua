-- luaposix, run the way an extension would be used from Lua.
-- Every line is PUC-Lua 5.4.9's, on this host.
local function show(tag, ...)
  local t, n = {}, select('#', ...)
  for i = 1, n do t[i] = tostring((select(i, ...))) end
  print(tag, table.unpack(t, 1, n))
end

-- libgen: pure string work, no host state
local libgen = require "posix.libgen"
show("basename", libgen.basename("/a/b/c.txt"), libgen.basename("c.txt"),
     libgen.basename("/a/b/"), libgen.basename("//"), libgen.basename(""))
show("dirname", libgen.dirname("/a/b/c.txt"), libgen.dirname("c.txt"),
     libgen.dirname("/a/b/"), libgen.dirname("/"), libgen.dirname(""))
show("badargs", pcall(libgen.basename), pcall(libgen.basename, 1))
show("toomany", pcall(libgen.basename, "a", "b"))

-- ctype: one character, one answer
local ctype = require "posix.ctype"
show("ctype.graph", ctype.isgraph("a"), ctype.isgraph(" "), ctype.isgraph("\n"),
     ctype.isgraph("~"))
show("ctype.print", ctype.isprint("a"), ctype.isprint(" "), ctype.isprint("\t"))
show("ctype.bad", pcall(ctype.isgraph), pcall(ctype.isgraph, 1),
     pcall(ctype.isgraph, true))

-- fnmatch: glob-style matching, four flags
local fnmatch = require "posix.fnmatch"
show("fnmatch", fnmatch.fnmatch("*.txt", "a.txt", 0),
     fnmatch.fnmatch("*.txt", "a.lua", 0),
     fnmatch.fnmatch("a?c", "abc", 0),
     fnmatch.fnmatch("A*", "abc", fnmatch.FNM_CASEFOLD),
     fnmatch.fnmatch("a/b", "a/b", fnmatch.FNM_PATHNAME),
     fnmatch.fnmatch("*", "a/b", fnmatch.FNM_PATHNAME),
     fnmatch.fnmatch("a", "a", fnmatch.FNM_PERIOD))
show("fnmatch.bad", pcall(fnmatch.fnmatch), pcall(fnmatch.fnmatch, "a"),
     pcall(fnmatch.fnmatch, "a", 1))

-- errno: the numbers the C library chose
local errno = require "posix.errno"
show("errno", errno.ENOENT, errno.EINVAL, errno.EAGAIN, errno.EACCES)
show("errno.count", (function()
  local n = 0
  for _ in pairs(errno) do n = n + 1 end
  return type(n)
end)())

-- fcntl: mostly constants, plus a lock on a real file
local fcntl = require "posix.fcntl"
show("fcntl.const", fcntl.O_RDONLY, fcntl.O_WRONLY, fcntl.O_RDWR,
     fcntl.O_CREAT, fcntl.O_APPEND, fcntl.F_GETFL, fcntl.F_SETFL)

-- time: gmtime is the same everywhere, and strftime renders it
local time = require "posix.time"
local g = time.gmtime(0)
show("gmtime", g.tm_year, g.tm_mon, g.tm_mday, g.tm_hour, g.tm_min,
     g.tm_sec, g.tm_yday, g.tm_wday)
show("gmtime.more", time.gmtime(1234567890).tm_year, time.gmtime(-1).tm_year,
     time.gmtime(951782400).tm_wday)
show("strftime", time.strftime("%Y-%m-%d %H:%M:%S", time.gmtime(0)),
     time.strftime("%A %j", time.gmtime(951782400)),
     time.strftime("%%", time.gmtime(0)))
show("mktime", type(time.mktime{tm_year=100, tm_mon=0, tm_mday=1,
     tm_hour=12, tm_min=0, tm_sec=0}))
show("time.bad", pcall(time.gmtime), pcall(time.gmtime, "x"),
     pcall(time.strftime), pcall(time.strftime, "%Y", 1))

-- sys.stat: the fields that do not depend on when the file was made
local stat = require "posix.sys.stat"
os.execute("mkdir -p lpfiles && printf 'hello' > lpfiles/f.txt && " ..
           "mkdir -p lpfiles/d && ln -sf f.txt lpfiles/link 2>/dev/null; true")
local function fields(p)
  local s, err = stat.stat(p)
  if not s then return "nil" end
  return (s.st_size or -1) .. ":" .. (s.st_nlink or -1) .. ":" ..
         stat.S_ISDIR(s.st_mode ~ 0 and s.st_mode or 0) .. ":" ..
         stat.S_ISREG(s.st_mode)
end
show("stat.file", fields("lpfiles/f.txt"))
show("stat.dir", fields("lpfiles/d"))
show("stat.link", fields("lpfiles/link"))
show("stat.lstat", (function()
  local s = stat.lstat("lpfiles/link")
  return s and s.st_size or "nil"
end)())
show("stat.missing", tostring(stat.stat("lpfiles/nope")))
show("stat.bad", pcall(stat.stat), pcall(stat.stat, 1))
show("stat.umask", type(stat.umask(0)), type(stat.umask(022)))

-- dirent: a directory listing, sorted so that the order cannot matter
local dirent = require "posix.dirent"
show("dirent.names", (function()
  local t = {}
  for f in dirent.files("lpfiles") do t[#t + 1] = f end
  table.sort(t)
  return table.concat(t, ",")
end)())
show("dirent.bad", pcall(dirent.files), pcall(dirent.files, "lpfiles/nope"))

-- glob: same, sorted
local glob = require "posix.glob"
show("glob", (function()
  local t = glob.glob("lpfiles/*", 0) or {}
  table.sort(t)
  return table.concat(t, ",")
end)())
show("glob.none", type(glob.glob("lpfiles/*.zzz", 0)))

-- utime: set a file's time and read it back through stat
local utime = require "posix.utime"
show("utime", (function()
  local ok = utime.utime("lpfiles/f.txt", 1000000, 2000000)
  local s = stat.stat("lpfiles/f.txt")
  return tostring(ok) .. ":" .. tostring(s and s.st_mtime)
end)())
show("utime.bad", pcall(utime.utime), pcall(utime.utime, "lpfiles/nope"))

os.execute("rm -rf lpfiles")
