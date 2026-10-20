-- 'luaL_traceback' writes "msg\n" whenever there *is* a message: C tests
-- the pointer ('msg != NULL'), not whether the string is empty. That is
-- why an empty message still starts the traceback with a newline, while
-- no message at all starts it with "stack traceback:".
local function head(s)
	return (s:sub(1, 24):gsub("\n", "\\n"))
end

print(head(debug.traceback("", 1))) -- a newline, then the header
print(head(debug.traceback(nil, 1))) -- no message: no newline
print(head(debug.traceback())) -- 'nil' is the default first argument
print(head(debug.traceback("hi", 1)))
print(head(debug.traceback(42, 1))) -- a number is converted in place
print(type(debug.traceback(true))) -- not convertible: returned untouched
print(debug.traceback(true))

-- the same message, but taken on another thread
local co = coroutine.create(function()
	coroutine.yield(debug.traceback("", 1))
end)
print(head(select(2, coroutine.resume(co))))

-- the handler lua.c puts under every chunk it runs
local ok, err = xpcall(function()
	error("boom")
end, function(m)
	return (debug.traceback(m, 1):gsub("\n", "\\n"))
end)
print(ok, err)
