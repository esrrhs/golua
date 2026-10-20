-- The message 'loadfile' and 'dofile' build when a file cannot be read
-- is lauxlib.c's 'errfile': "cannot open x: strerror(errno)", or "cannot
-- read x: ..." when it is the read that fails.  A Go error would read
-- "open x: no such file or directory", which is neither.
--
-- The name matters too: stdin is what an *absent or nil* argument means,
-- not the empty string -- 'loadfile("")' really does try to open a file
-- with no name.

print(loadfile("nosuchfile.lua"))
print(loadfile(""))
print(pcall(dofile, "nosuchfile.lua"))

-- stdin: the standard input of this case is empty, so both of these
-- compile the empty chunk
print(type(loadfile(nil)))
print(type(loadfile()))

-- a module that is not on the path: 'searchpath' answers with nil and
-- the list of names it tried, which here is only the one pattern given
print(package.searchpath("nope", "./?.lua"))
print(package.searchpath("nope", "./?.lua;./?/init.lua"))
