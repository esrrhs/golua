-- the limits llex.c, lparser.c and lcode.c enforce, and the edges of the
-- lexer: number literals, escapes and long strings.
--
-- The recursion guard is the C-call count, not a special parser limit:
-- 5.4.9's 'enterlevel' is just 'luaE_incCstack', so a chunk nested too
-- deeply reports "C stack overflow" (and the host's message handler
-- appends a traceback) rather than a syntax error.
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else t[i] = tostring(v) end
  end
  return table.concat(t, "|")
end

local function try(src)
  local f, e = load(src, "=g")
  if not f then return "nil|" .. string.format("%q", e) end
  local ok, r = pcall(f)
  return (ok and "true|" or "false|") .. (ok and show(r) or string.format("%q", r))
end

-- 1. local variables: MAXVARS is 200
for _, n in ipairs{199, 200, 201, 300} do
  local names = {}
  for i = 1, n do names[i] = "v" .. i end
  print("locals", n, try("local " .. table.concat(names, ",") .. " return v1"))
end

-- 2. upvalues: 255 of them
for _, n in ipairs{254, 255, 256} do
  local names = {}
  for i = 1, n do names[i] = "u" .. i end
  local src = "local " .. table.concat(names, ",") .. "\n"
  local use = {}
  for i = 1, n do use[i] = "u" .. i end
  print("upvals", n, try(src .. "return function() return " .. table.concat(use, "+") .. " end"))
end

-- 3. registers: an expression needs one per operand
for _, n in ipairs{100, 200, 250, 300} do
  local t = {}
  for i = 1, n do t[i] = tostring(i) end
  print("regs", n, try("return " .. table.concat(t, "+")))
end
for _, n in ipairs{100, 250, 300} do
  local t = {}
  for i = 1, n do t[i] = "x" .. i .. "=1" end
  print("ctor", n, try("return {" .. table.concat(t, ",") .. "}"))
end

-- 4. syntax levels: nested parentheses and constructors
for _, n in ipairs{10, 199, 200, 201} do
  print("paren", n, try("return " .. string.rep("(", n) .. "1" .. string.rep(")", n)))
end
for _, n in ipairs{10, 199, 200, 201} do
  print("table", n, try("return " .. string.rep("{", n) .. string.rep("}", n)))
end
for _, n in ipairs{10, 199, 200, 201} do
  print("unary", n, try("return " .. string.rep("-", n) .. "1"))
end

-- 5. constants
for _, n in ipairs{1000, 5000} do
  local t = {}
  for i = 1, n do t[i] = "return " .. i .. ";" end
  print("consts", n, try(table.concat(t)))
end
do
  local t = {}
  for i = 1, 3000 do t[i] = string.format("f(%d)", i) end
  print("calls", #t, try(table.concat(t, " ")))
end

-- 6. arguments to a call
for _, n in ipairs{10, 100, 1000} do
  local t = {}
  for i = 1, n do t[i] = tostring(i) end
  print("args", n, try("local function f(...) return select('#', ...) end return f(" .. table.concat(t, ",") .. ")"))
end

-- 7. number literals
local nums = {"0", "1", "0x10", "0X10", "0x1p4", "0x1p-2", "0x.8p1", "0x", "0x1p",
              ".5", "5.", "1e2", "1E2", "1e+2", "1e-2", "1e", ".5e2", "5.e2",
              "0x7fffffffffffffff", "0x8000000000000000", "0xffffffffffffffff",
              "9223372036854775807", "9223372036854775808", "1e400", "-1e400",
              "3", "3.0", "0.0", "-0.0", "1e308", "1e309", "2^63"}
for _, s in ipairs(nums) do
  print(string.format("%-22s", s), try("return " .. s))
end
print(try("return 0x1p1000"))
print(try("return 1e999"))

-- 8. escapes and long strings
local strs = {'"\\n"', '"\\t"', '"\\\\"', '"\\""', '"\\\'"', '"\\a"', '"\\b"',
              '"\\f"', '"\\v"', '"\\r"', '"\\0"', '"\\1"', '"\\255"', '"\\x41"',
              '"\\x4"', '"\\x"', '"\\u{41}"', '"\\u{1F600}"', '"\\u{}"',
              '"\\u{110000}"', '"\\z\n  x"', '"\\65"', '"\\256"', '"\\9"'}
for _, s in ipairs(strs) do
  print(string.format("%-16s", s), try("return " .. s))
end
print(try("return [[abc]]"), try("return [==[a]]b]==]"))
print(try("return [[]]"), try("return [[a\n\nb]]"))
print(try("return [[\nleading newline]]"))
print(try("return 'a\\"))
print(try("return \"unterminated"))

-- 9. names, comments, and whitespace
print(try("--just a comment"))
print(try("--[[]]"))
print(try("--[==[x]==]"))
print(try("#!/usr/bin/lua\nreturn 1"))
print(try("return 1 -- trailing"))
print(try("return--[[c]]1"))
print(try("return 1;"))
print(try(";;;"))
print(try("return _VERSION"))
print(try("local \226\152\131 = 1 return \226\152\131"))
print(try("return 1 2"))
print(try("return a b"))

-- 10. things that are too big
print(try("return " .. string.rep("1+", 5000) .. "1"))
print(try("local a\n" .. string.rep("do local b = a\n", 200) .. string.rep("end\n", 200) .. "return a"))
print(try("return " .. string.rep("{", 200) .. string.rep("}", 200)))
print(try("local t = {}\n" .. string.rep("t.x = 1\n", 2000) .. "return t.x"))
print("done")
