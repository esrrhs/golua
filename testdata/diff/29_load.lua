-- 'load' with a reader function: the pieces are pulled one at a time
-- and only as the parser needs them, exactly as C's ZIO pulls them
-- (lzio.c).  That is what lets a chunk that fails to parse stop the
-- reader: C's lexer reaches the malformed token long before a reader
-- that never returns nil runs dry, so 'load' answers with a syntax
-- error instead of reading for ever.

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

-- 1. a reader that keeps returning the same text: the lexer stops it
print(show(pcall(load, function() return "return 1" end)))

-- 2. a reader that returns a number: 'lua_isstring' accepts it, so the
--    chunk becomes "424242..." until the lexer complains
local n = 0
print(show(pcall(load, function()
  if n == 0 then n = 1 return 42 end
  return nil
end)))

-- 3. a reader that ends, and one that ends with an empty string
print(type(load(function() return nil end)))
print(type(load(function() return "" end)))
print(show(pcall(load, function() return "return 1" end)))

-- 4. a reader that raises: the message is the status of the load
print(show(pcall(load, function() error("reader") end)))

-- 5. a reader that returns something that is not a string
print(show(pcall(load, function() return {} end)))
print(show(pcall(load, function() return true end)))

-- 6. a reader called only as far as the chunk needs
local calls = 0
local f = load(function()
  calls = calls + 1
  if calls == 1 then return "return " end
  if calls == 2 then return "1" end
end)
print(type(f), f and f(), calls)

-- 7. a reader whose chunk has a syntax error: it is not drained
calls = 0
local ok, err = pcall(load, function()
  calls = calls + 1
  if calls == 1 then return "@@@" end
  if calls == 2 then return "more" end
  return nil
end)
print(ok, err, calls)

-- 8. the chunk name stops at the first '\0', as a C string does
print(show(pcall(load, "return 1\0garbage")))
print(show(pcall(load, "error('z')", "=a\0b")))
local g = load("error('z')", "=a\0b")
print(show(pcall(g)))
print(show(pcall(load, "\0")))

-- 9. modes: the first byte decides what the chunk is
local chunk = string.dump(function() return 1 end)
print(type(load(chunk)))
print(show(pcall(load, chunk, "=b", "t")))
print(show(pcall(load, "return 1", "=t", "b")))
print(show(pcall(load, "return 1", "=t", "x")))
print(show(pcall(load, "\27Lua garbage", "=b", "b")))

-- 10. string.dump and what survives it
print(type(string.dump(function() return 1 end)))
print(show(pcall(string.dump, print)))
print(show(pcall(string.dump, 42)))
local up = 7
print(load(string.dump(function() return up end))())
print(load(string.dump(function(x) return x * 2 end))(21))
print(load(string.dump(function() return 1 end, true))())
print("done")
