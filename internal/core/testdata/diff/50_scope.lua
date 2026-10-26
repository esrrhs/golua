-- scoping, the numeric 'for' at the edges of the integer range, goto
-- and labels, and the closures a loop makes.  The loop limits are named
-- inside the chunks: writing them as literals would make them floats,
-- and a float loop a step of 1 cannot advance never ends.
-- scoping, the numeric 'for', goto and labels
local function show(...)
  local n = select('#', ...)
  local t = {}
  for i = 1, n do
    local v = (select(i, ...))
    if type(v) == "string" then t[i] = string.format("%q", v)
    elseif v == nil then t[i] = "nil"
    else
      local ok, s = pcall(tostring, v)
      t[i] = ok and s or ("<" .. type(v) .. ">")
    end
  end
  return table.concat(t, "|")
end
local function rec(f, ...) return show(pcall(f, ...)) end

print("== repeat sees its own body ==")
do
  local out = {}
  local i = 1
  repeat
    local x = i * 10
    i = i + 1
  until x > 20
  print(show(i))
end
do
  -- the condition is inside the scope of the body
  local f = load([[
    local n = 0
    repeat
      local done = n >= 2
      n = n + 1
    until done
    return n
  ]])
  print(show(f()))
end
do
  local f = load("repeat local a = 1 until a return a")
  print(rec(f))
end

print("== numeric for ==")
do
  local out = {}
  for i = 1, 3 do out[#out + 1] = i end
  print(table.concat(out, ","))
  out = {}
  for i = 3, 1, -1 do out[#out + 1] = i end
  print(table.concat(out, ","))
  out = {}
  for i = 1, 0 do out[#out + 1] = i end
  print("[" .. table.concat(out, ",") .. "]")
  out = {}
  for i = 0, 1, 0.5 do out[#out + 1] = i end
  print(table.concat(out, ","))
end
print(rec(load, "for i = 1, 10, 0 do end"))
print(rec(load, "for i = 1, 10, 0.0 do end"))
print(rec(load, "for i = 1, 'x' do end"))
print(rec(load, "for i = 'x', 10 do end"))
print(rec(load, "for i = 1, 10, 'x' do end"))
print(rec(load, "for i = 1.5, 10 do end"))
print(rec(load, "for i = 1, 10.5 do end"))
print(rec(load, "for i = 1, 10, 1.5 do end"))
do
  -- integer loops that reach the very edges of the range.  The limits are
  -- named inside the chunk on purpose: writing "-9223372036854775808" as
  -- a literal makes the lexer read 9223372036854775808, which does not
  -- fit an integer and becomes a float, and a float loop a step of 1 can
  -- never advance is an infinite loop in PUC-Lua too.
  -- Avoid 'for i = ma-k, ma' (5.3 int wrap makes step+overflow an infinite loop).
  local f = load("local mi, ma = math.mininteger, math.maxinteger\
    local out = {}\
    local n = 0 for i = ma - 3, ma - 1 do n = n + 1 end out[1] = n\
    n = 0 for i = mi, mi + 2 do n = n + 1 end out[2] = n\
    n = 0 for i = ma - 4, ma - 1, 2 do n = n + 1 end out[3] = n\
    n = 0 for i = 1, 2, ma do n = n + 1 end out[4] = n\
    n = 0 for i = ma, mi do n = n + 1 end out[5] = n\
    n = 0 for i = mi, mi + 2, -1 do n = n + 1 end out[6] = n\
    n = 0 for i = ma - 1, mi, ma do n = n + 1 end out[7] = n\
    return table.concat(out, ',')")
  print(show(f()))
end
do
  local f = load("local s = 0 for i = 1, 10 do s = s + i; i = 99 end return s")
  print(show(f()))
end

print("== generic for ==")
do
  local out = {}
  for k, v in pairs({a = 1, b = 2}) do out[#out + 1] = k .. v end
  table.sort(out)
  print(table.concat(out, ","))
end
do
  local function iter(s, c)
    if c < 3 then return c + 1, (c + 1) * 10 end
  end
  local out = {}
  for a, b in iter, nil, 0 do out[#out + 1] = tostring(a) .. ":" .. tostring(b) end
  print(table.concat(out, ","))
end
print(rec(load, "for k in 42 do end"))
print(rec(load, "for k in nil do end"))
do
  local f = load("local t = setmetatable({}, {__pairs = function(t) return function() end, t, nil end}) local n = 0 for k in pairs(t) do n = n + 1 end return n")
  print(show(f()))
end

print("== goto and labels ==")
do
  local f = load([[
    local out = {}
    for i = 1, 5 do
      if i == 3 then goto continue end
      out[#out + 1] = i
      ::continue::
    end
    return table.concat(out, ",")
  ]])
  print(show(f()))
end
print(rec(load, "goto nowhere"))
print(rec(load, "::x:: ::x::"))
print(rec(load, "do goto x end ::x::"))
print(rec(load, "for i = 1, 2 do goto x end ::x::"))
print(rec(load, "::x:: do ::x:: end"))
do
  -- jumping out of a scope that has to-be-closed variables is an error
  print(rec(load, "for i = 1, 2 do local x = nil goto out end ::out::"))
end
do
  local f = load([[
    do
      local x = 1
      ::top::
      x = x + 1
      if x < 4 then goto top end
      return x
    end
  ]])
  print(show(f()))
end

print("== closures in loops ==")
do
  local fs = {}
  for i = 1, 3 do fs[i] = function() return i end end
  print(show(fs[1](), fs[2](), fs[3]()))
end
do
  local fs = {}
  for _, v in ipairs{"a", "b"} do fs[#fs + 1] = function() return v end end
  print(show(fs[1](), fs[2]()))
end
do
  local fs = {}
  local i = 1
  while i <= 3 do
    local j = i
    fs[i] = function() return j end
    i = i + 1
  end
  print(show(fs[1](), fs[2](), fs[3]()))
end
print("== local function ==")
do
  local f = load("local function fact(n) if n <= 1 then return 1 end return n * fact(n - 1) end return fact(5)")
  print(show(f()))
  local g = load("local fact = function(n) if n <= 1 then return 1 end return n * fact(n - 1) end return fact(5)")
  print(rec(g))
end
print("done")
