-- pattern matching: the magic characters, the classes, the sets, the
-- anchors, %b, %f, captures and every way 'gsub' can be called.

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
local function rec(f, ...) return show(pcall(f, ...)) end

local function findall(s, pat)
  local out = {}
  local i = 1
  while true do
    local a, b, c = string.find(s, pat, i)
    if not a then break end
    out[#out + 1] = a .. "-" .. b .. (c and ":" .. tostring(c) or "")
    if b < a then i = i + 1 else i = b + 1 end
    if #out > 30 then break end
  end
  return table.concat(out, ",")
end

-- 1. magic characters, plain and escaped
for _, c in ipairs{"^", "$", "*", "+", "?", ".", "(", ")", "[", "]", "%", "-"} do
  print(string.format("%q", c), show(string.find("a" .. c .. "b", c, 1, true)),
        show(string.find("a" .. c .. "b", "%" .. c)))
end
print(findall("aaa", "a*"), findall("aaa", "a-"), findall("aaa", "a+"))
print(show(string.match("hello", "(h(e(l(l(o)))))")))
print(show(string.match("hello", "()h()e()l()l()o()")))
print(show(string.match("abc", "()(b)()")))

-- 2. classes and their complements
for _, c in ipairs{"a", "c", "d", "g", "l", "p", "s", "u", "w", "x"} do
  local up = "%" .. c:upper()
  print(c, show(string.match("abc123 \t!", "%" .. c)),
        show(string.match("abc123 \t!", up)),
        show(string.match("", "%" .. c)), show(string.match("", up)))
end
print(findall("ab_12 cd", "%w+"), findall("ab_12 cd", "%a+"))
print(findall("ab_12 cd", "%s+"), findall("ab_12 cd", "%S+"))
print(findall("0x1F zz", "%x+"))
print(findall("a\0b\1c", "%c+"), findall("a\0b\1c", "%z+"))

-- 3. sets
print(findall("abcxyz", "[abc]+"), findall("abcxyz", "[^abc]+"))
print(findall("a-z", "[a%-z]+"), findall("a-z", "[a-z]+"))
print(findall("]^[", "[]]+"), findall("]^[", "[^]]+"))
print(show(string.match("a]", "[]]")), show(string.match("^a", "[%^]")))
print(show(string.match("-a", "[-a]+")), show(string.match("a-", "[a-]+")))
print(findall("Aa1_ ", "[%a%d]+"), findall("Aa1_ ", "[^%a]+"))
print(rec(string.match, "abc", "[a"), rec(string.match, "abc", "[]"))
print(rec(string.match, "abc", "%"), rec(string.match, "abc", "("))

-- 4. anchors
print(show(string.find("abcabc", "^abc")), show(string.find("abcabc", "abc$")))
print(show(string.find("abcabc", "^abc", 4)), show(string.find("abcabc", "^b", 2)))
print(show(string.find("abc", "^$")), show(string.find("", "^$")))
print(show(string.find("", "^")), show(string.find("", "$")))

-- 5. %b and %f
print(show(string.match("(a(b)c)", "%b()")))
print(show(string.match("(a(b)c", "%b()")))
print(show(string.match("[x[y]z]", "%b[]")))
print(show(string.match("()", "%b()")))
print(rec(string.match, "abc", "%b"))
print(show(string.match("hello world", "%f[%w]%w+")))
print(show(string.match("hello world", "%f[%w]%w+", 2)))
print(show(string.find("abc THE-x", "%f[%a]%u+")))
print(rec(string.match, "abc", "%f"), rec(string.match, "abc", "%f[a]"))

-- 6. captures and back references
print(show(string.match("abcabc", "(abc)%1")))
print(show(string.match("abab", "(a)(b)%1%2")))
print(show(string.match("2024-01-02", "(%d+)-(%d+)-(%d+)")))
print(show(string.gsub("hello world", "(%w+) (%w+)", "%2 %1")))
print(show(string.gsub("abc", "(a)(b)(c)", "%3%2%1")))
print(show(string.gsub("abc", "a", "%0%0")))
print(rec(string.gsub, "abc", "a", "%"))
print(rec(string.gsub, "abc", "a", "%2"), rec(string.gsub, "abc", "a", "%9"))

-- 7. gsub: counts, limits, and the three kinds of replacement
print(show(string.gsub("hello", "l", "L", 1)))
print(show(string.gsub("hello", "l", "L", 0)))
print(show(string.gsub("hello", "", "-")))
print(show(string.gsub("", "a", "b")))
print(show(string.gsub("hello", "l", {})))
print(show(string.gsub("abc", "%w", function(c) return c:upper() end)))
print(show(string.gsub("abc", "%w", function(c) return nil end)))
print(show(string.gsub("abc", "%w", function(c) return c, c:upper() end)))
print(show(string.gsub("aaa", "a", function() return "b" end, 2)))
print(show(string.gsub("abc", "b", setmetatable({}, {__index = function() return "T" end}))))
print(rec(string.gsub, "abc", "b", 42), rec(string.gsub, "abc"))
print(rec(string.gsub, 42, "b", "x"), rec(string.gsub, "abc", 42, "x"))

-- 8. gmatch, including the empty match
do
  local out = {}
  for w in ("hello world"):gmatch("%a+") do out[#out + 1] = w end
  print(table.concat(out, ","))
end
do
  local out = {}
  for k, v in ("a=1,b=2"):gmatch("(%w+)=(%w+)") do out[#out + 1] = k .. v end
  print(table.concat(out, ","))
end
do
  local out = {}
  for w in ("abc"):gmatch("") do out[#out + 1] = "<" .. w .. ">" end
  print(#out, table.concat(out, ""))
end
do
  local out = {}
  for w in ("abc"):gmatch("x*") do out[#out + 1] = "<" .. w .. ">" end
  print(#out, table.concat(out, ""))
end
print(rec(string.gmatch, "abc", "("), rec(string.gmatch, 42, "a"))

-- 9. find with init, plain searches and out-of-range positions
print(show(string.find("abcabc", "b", 3)), show(string.find("abcabc", "b", -3)))
print(show(string.find("abcabc", "b", -10)), show(string.find("abcabc", "b", 100)))
print(show(string.find("abc", "", 4)), show(string.find("abc", "", 0)))
print(show(string.find("a.c", ".", 1, true)), show(string.find("abc", "b", 2, true)))
print(rec(string.find, "abc", nil), show(string.find("abc", "b", "2")))

-- 10. sub and rep at the edges
print(show(("hello"):sub(1, 2), ("hello"):sub(-2), ("hello"):sub(0), ("hello"):sub(10)))
print(show(("hello"):sub(2, -2), ("hello"):sub(-10, 10), ("hello"):sub(3, 2)))
print(show(#("hello"):rep(0), #("hello"):rep(3), ("ab"):rep(2, "-")))
print(rec(string.rep, "x", -1), rec(string.rep, "x", 1e10))
print(rec(string.sub, "abc", "1"), rec(string.sub, 42, 1))

-- 11. utf8
print(show(utf8.charpattern), show(string.match("héllo", utf8.charpattern)))
do
  local out = {}
  for c in ("héllo"):gmatch(utf8.charpattern) do out[#out + 1] = c end
  print(#out, table.concat(out, ","))
end
print(show(utf8.len("héllo"), utf8.len("a\255b")))
