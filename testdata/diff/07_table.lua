local t = {3, 1, 2}
table.sort(t)
print(table.concat(t, ","))
table.sort(t, function(a, b) return a > b end)
print(table.concat(t, ","))
table.insert(t, 99)
table.insert(t, 1, 0)
print(table.concat(t, ","))
print(table.remove(t), table.remove(t, 1), table.concat(t, ","))
print(table.unpack({1, 2, 3}))
print(select("#", table.unpack({1, 2, 3})))
print(select("#", table.unpack({}, 1, 100)))
local p = table.pack(1, nil, 3)
print(p.n, p[1], p[2], p[3])
local a = {1, 2, 3, 4, 5}
print(table.concat(table.move(a, 2, 4, 1), ","))
print(table.concat(table.move({1, 2}, 1, 2, 3, {}), ","))
local m = {}
for k, v in pairs({a = 1, b = 2}) do m[#m + 1] = k end
table.sort(m)
print(table.concat(m, ","))
print(pcall(table.concat, {1, {}, 3}))
print(pcall(table.insert, {}, 1, 2, 3))
print(pcall(table.remove, {}, 1, 2))
local big = {}
for i = 1, 100 do big[i] = i * i end
print(#big, big[100])
print(next({}))
print(rawlen({1, 2, 3}), rawlen("abc"))
local mt = setmetatable({}, {__len = function() return 42 end})
print(#mt, rawlen(mt))
print(table.concat({tostring(1 / 3)}))
