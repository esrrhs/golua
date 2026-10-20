print(rawequal(1, 1.0), rawequal(1.0, 1.0), rawequal(1, 1))
print(rawequal("a", "a"), rawequal({}, {}), rawequal(print, print))
print(rawequal(nil, false), rawequal(true, true))
local t = {}
t[1] = "int"
print(t[1], t[1.0], t[1.5])
t[2.0] = "flt"
print(t[2], t[2.0])
local big = {}
big[9007199254740993] = "a"
big[9007199254740993.0] = "b"
print(big[9007199254740993], big[9007199254740992.0])
print(9007199254740993 == 9007199254740993.0)
print(9007199254740992 == 9007199254740992.0)
print(1 == 1.0, 1.0 == 1, 0 == 0.0, -0.0 == 0.0)
local s = {}
s["1"] = "str"
print(s[1], s["1"])
print(next({[1] = "x"}))
