-- coroutine semantics: resume/yield/wrap/status/close, nested yields,
-- yieldable pcall, yields through metamethods and iterators.
local function pr(...) print(...) end

-- basic
local co = coroutine.create(function(a, b)
	pr("start", a, b)
	local x = coroutine.yield(a + b)
	pr("got", x)
	return "done", x
end)
pr(coroutine.resume(co, 1, 2))
pr(coroutine.status(co))
pr(coroutine.resume(co, 10))
pr(coroutine.status(co))
pr(coroutine.resume(co, 10))

-- wrap
local w = coroutine.wrap(function(a)
	for i = 1, a do coroutine.yield(i * i) end
	return "end"
end)
pr(w(3))
pr(w())
pr(w())
pr(w())
pr(pcall(w))

-- yieldable pcall (5.4)
local co2 = coroutine.create(function()
	pr(pcall(function() coroutine.yield("from-pcall") return 1 end))
	return 2
end)
pr(coroutine.resume(co2))
pr(coroutine.resume(co2))

-- yield across xpcall + metamethods
local mt = { __index = function(t, k) return coroutine.yield("mm:" .. k) end }
local t = setmetatable({}, mt)
local co3 = coroutine.create(function()
	local v = t.foo
	pr("v", v)
	return v
end)
pr(coroutine.resume(co3))
pr(coroutine.resume(co3, "val"))

-- yield in a for iterator
local function iter(s, c)
	coroutine.yield("iter:" .. c)
	if c < 3 then return c + 1, c * 10 end
end
local co4 = coroutine.create(function()
	for i, v in iter, nil, 1 do pr("loop", i, v) end
	return "iterdone"
end)
pr(coroutine.resume(co4))
pr(coroutine.resume(co4))
pr(coroutine.resume(co4))
pr(coroutine.resume(co4))

-- errors inside coroutines
local co5 = coroutine.create(function()
	error("boom")
end)
pr(coroutine.resume(co5))
pr(coroutine.status(co5))
pr(coroutine.close(co5))
pr(coroutine.close(co5))
pr(coroutine.resume(co5))

-- non-string error object
local co6 = coroutine.create(function() error({code = 42}) end)
local ok, e = coroutine.resume(co6)
pr(ok, type(e), e.code)

-- running / isyieldable
pr(coroutine.running())
pr(coroutine.isyieldable())
local co7 = coroutine.create(function()
	pr("in-co running", coroutine.running())
	pr("in-co yieldable", coroutine.isyieldable())
	pr("main yieldable", coroutine.isyieldable(coroutine.running()))
end)
coroutine.resume(co7)

-- close a suspended coroutine
local co8 = coroutine.create(function() coroutine.yield(1) end)
coroutine.resume(co8)
pr(coroutine.status(co8))
pr(coroutine.close(co8))
pr(coroutine.status(co8))

-- close the running one / the main one
pr(coroutine.close(coroutine.running()))
local co9 = coroutine.create(function() return coroutine.close(coroutine.running()) end)
pr(coroutine.resume(co9))
