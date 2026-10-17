-- String identity inside one chunk. PUC-Lua anchors every literal of a
-- chunk in the scanner's table, so two occurrences of the same text are
-- the same object -- and that has to keep being true after the string
-- has been used as a constant, because 'addk' overwrites the table
-- entry with the index of the constant (lcode.c). llex.c therefore
-- reads the entry's *key*, not its value, and this is what breaks when
-- it does not.
local function add(s) return string.format("%p", s) end
local function same(a, b) return add(a) == add(b) end
local function P(name, v) print(name, v) end

-- long strings (not interned globally, only anchored per chunk)
local l1 <const> = "01234567890123456789012345678901234567890123456789"
local l2 <const> = "01234567890123456789012345678901234567890123456789"
local l3 = l1
P("long length", #l1)
P("long same literal", same(l1, l2))
P("long via local", same(l1, l3))
P("long via function", same(l1, (function() return l1 end)()))
P("long via literal again", same(l1, (function()
    return "01234567890123456789012345678901234567890123456789"
  end)()))

-- ...but a string built at run time is a different object
local l4 = "0123456789" .. "0123456789012345678901234567890123456789"
P("concat equal", l4 == l1)
P("concat distinct", add(l4) ~= add(l1))

-- short strings, after each has been used as a constant
local function use(s) return s end
local s1 = "hello"
P("short first use", use(s1))
local s2 = "hello"
P("short later literal", same(s1, s2))
local s3 = "hello"
P("short third", same(s1, s3))
P("short in table", same(s1, ({"hello"})[1]))
P("short via concat", ("he" .. "llo") == s1)
print("done")
