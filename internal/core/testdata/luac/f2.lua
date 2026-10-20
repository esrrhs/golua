--[[ long
comment ]]
local big = 2^53 + 1
local flt = 0.1
local nan = 0/0
local inf = 1/0
local strs = { "a\nb\ttab\\q\"z", "\65\066", "\u{1F600}", "\x41", "é" }
print(big, flt, nan, inf, #strs)
local n = 300
local t = {}
for i = 1, n do t[i] = i * i end
local function rec(k) if k == 0 then return 0 end return 1 + rec(k - 1) end
print(rec(10))
local cls = {}
function cls.method(self, x) return x end
function cls.static() return "s" end
print(cls:method(1), cls.static())
local ok, err = pcall(function() error("boom", 2) end)
print(ok, err)
local up = 1
local function useup() up = up + 1 return up end
print(useup(), useup())
