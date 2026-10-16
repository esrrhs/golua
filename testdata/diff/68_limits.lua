-- The compiler's own limits and refusal messages: how many locals, how
-- many upvalues, how big a constructor, how deep an expression.  Each is
-- reached by building the source and handing it to 'load'.
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
local function rec(f, ...)
  local ok, e = pcall(f, ...)
  if ok then return "ok|" .. show(...) end
  return "err|" .. (tostring(e):gsub("^[^:]*:%d+: ", ""))
end

-- The source is what is being compiled, and in some of these it is tens
-- of kilobytes long, so it is never printed.  Loaded with the chunk name
-- "=c" for the same reason: 'load' uses the source itself as the name
-- when none is given, and the message would then carry all of it.  What
-- is compared is the answer -- the chunk, or the refusal and the line.
local function comp(src, ...)
  local ok, chunk, msg = pcall(load, src, "=c", ...)
  if not ok then return "raise|" .. tostring(chunk) end
  if chunk == nil then return "refuse|" .. tostring(msg) end
  return "ok"
end
local function gensrc(f, n)
  local o = {}
  for i = 1, n do o[i] = f(i) end
  return table.concat(o, " ")
end
-- a name list, which needs commas where 'gensrc' puts spaces
local function names(prefix, n)
  local o = {}
  for i = 1, n do o[i] = prefix .. i end
  return table.concat(o, ", ")
end

print("== locals ==")
print(comp("local " .. names("v", 199) .. " return v199"))
print(comp("local " .. names("v", 200) .. " return v200"))
print(comp("local " .. names("v", 201) .. " return v201"))
print(comp("do local " .. names("v", 201) .. " end"))
print(comp(gensrc(function(i) return "local v" .. i .. " = " .. i end, 200)))
print(comp(gensrc(function(i) return "local v" .. i .. " = " .. i end, 201)))
print(comp("local " .. names("v", 250)))

print("== locals with an initialiser that needs a temporary ==")
print(comp("local a1 = 1 " .. gensrc(function(i) return "local v" .. i .. " = a1" end, 199)))
print(comp("local a1 = 1 " .. gensrc(function(i) return "local v" .. i .. " = a1" end, 200)))
print(comp("local a1 = 1 " .. gensrc(function(i) return "local v" .. i .. " = a1" end, 201)))

print("== upvalues ==")
print(comp("local " .. names("v", 60) .. " return function() return " .. names("v", 60) .. " end"))
print(comp("local " .. names("v", 200) .. " return function() return " .. names("v", 200) .. " end"))
print(comp("local " .. names("v", 255) .. " return function() return " .. names("v", 255) .. " end"))
print(comp("local " .. names("v", 256) .. " return function() return " .. names("v", 256) .. " end"))
print(comp("local a = 1 " .. gensrc(function(i)
  return "local function f" .. i .. "() return a end" end, 300)))

print("== constants ==")
-- the items of a constructor need commas, as the names above did
local items = function(f, n)
  local o = {}
  for i = 1, n do o[i] = f(i) end
  return table.concat(o, ", ")
end
print(comp("local t = {" .. items(function(i) return i end, 1000) .. "} return #t"))
print(comp("local t = {" .. items(function(i) return '["k' .. i .. '"]=' .. i end, 500) .. "} return t.k500"))
print(comp(gensrc(function(i) return "local a" .. i .. " = " .. i .. ".5" end, 300)))
print(comp(gensrc(function(i) return "local s" .. i .. " = 'str" .. i .. "'" end, 300)))

print("== expression and nesting depth ==")
print(comp("return " .. string.rep("(", 190) .. "1" .. string.rep(")", 190)))
print(comp("return " .. string.rep("(", 210) .. "1" .. string.rep(")", 210)))
print(comp("return " .. string.rep("-", 200) .. "1"))
print(comp("return " .. string.rep("not ", 300) .. "1"))
print(comp("return " .. gensrc(function() return "1+" end, 500) .. "1"))
print(comp("return " .. gensrc(function() return "1+" end, 2000) .. "1"))
print(comp("return " .. gensrc(function() return "1*" end, 5000) .. "1"))
print(comp("return " .. string.rep("a", 100) .. " == nil"))
print(comp("local t = " .. string.rep("{", 200) .. string.rep("}", 200) .. " return t"))
print(comp("return " .. gensrc(function() return '"a"..' end, 300) .. '"a"'))

print("== table constructors ==")
print(comp("return {" .. items(function(i) return i end, 10000) .. "}"))
print(comp("return {" .. items(function(i) return '["k' .. i .. '"]=' .. i end, 2000) .. "}"))
print(comp("return {" .. string.rep("1,", 20000) .. "}"))

print("== syntax that has to be refused ==")
print(comp("goto x"))
print(comp("::x:: goto x ::x::"))
print(comp("do goto x ::x:: end"))
print(comp("break"))
print(comp("while true do break end"))
print(comp("for i=1,2 do break end"))
print(comp("repeat break until true"))
print(comp("return 1 return 2"))
print(comp("local a = "))
print(comp("if true then"))
print(comp("return ]]"))
print(comp("return 'unterminated"))
print(comp('return "unterminated'))
print(comp("return [==[x]=]"))
print(comp("return 0x"))
print(comp("return 1e"))
print(comp("return 1e+"))
print(comp("return ."))
print(comp("return 1..2"))
print(comp("return 1..."))
print(comp("return ..1"))
print(comp("return \\z"))
print(comp("return '\\z   x'"))
print(comp("return '\\x'"))
print(comp("return '\\x1'"))
print(comp("return '\\xZZ'"))
print(comp("return '\\u{}'"))
print(comp("return '\\u{110000}'"))
print(comp("return '\\u{fffffffff}'"))
print(comp("return '\\999'"))
print(comp("return '\\400'"))
print(comp("function 1() end"))
print(comp("local function 1() end"))
print(comp("a.b.c = function() end"))
print(comp("(1).x = 1"))
print(comp("1 = 2"))
print(comp("local 1 = 2"))
print(comp("return #1"))
print(comp("return -'a'"))
print(comp("::1::"))
print(comp("local a <bogus> = 1 return a"))
print(comp("local a <const> = 1 a = 2"))
print(comp("local a <close>, b = nil"))
print(comp("for i = 1, 2 do ::c:: end"))
print(comp("return function(...) return ... end"))

print("== string and table size limits ==")
print(rec(string.rep, "a", 2 ^ 31))
print(rec(string.rep, "a", 2 ^ 31, "b"))
print(rec(string.rep, "abc", 2 ^ 30))
print(rec(string.rep, "a", 0))
print(rec(string.rep, "a", -1))
print(rec(string.rep, "", 2 ^ 30))
print(rec(table.concat, {}, ",", 1, 2 ^ 31))
print(rec(table.concat, {}))
print(rec(table.concat, {}, ",", 5, 3))
print(rec(table.concat, {}, ",", 0))
print(rec(table.concat, {}, ",", -1))
print(rec(function() local t = {} for i = 1, 20 do t[i] = "x" end return #table.concat(t) end))
print(rec(string.sub, "abc", 2 ^ 31))
print(rec(string.sub, "abc", 1, 2 ^ 40))
print(rec(function() return ("a"):rep(10):rep(10) end))
print("done")
