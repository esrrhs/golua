-- numbers, literals, escapes, comments and string quoting.
print(0x0, 0x1, 0x9, 0xA, 0xF, 0x10, 0xFF, 0x7fffffff, 0x7FFFFFFFFFFFFFFF)
print(0x8000000000000000)
print(0X10, 0xaBcDeF)
print(1, 1., .1, 1e2, 1E2, 1e+2, 1e-2, .5e1)
print(0xA.Bp1, 0x.8p1, 0xA8p-2, 0x1p4, 0x1P-1)
print(math.type(1), math.type(1.0), math.type(0x10), math.type(1e2))
print(1e400 == math.huge, -1e400 == -math.huge)
print(1 / 0 == math.huge, -1 / 0 == -math.huge, 0 / 0 ~= 0 / 0)
print(7 // 2, -7 // 2, 7 // -2, 7.0 // 2, 7 % 2, -7 % 2, 7.5 % 2)
print(3 & 5, 3 | 5, 3 ~ 5, ~0, 1 << 3, 1 << 63, 1 << 64, -1 >> 1)
print(2 ^ 10, 2 ^ 0.5, (-8) ^ (1 / 3) ~= (-8) ^ (1 / 3))
print(math.maxinteger, math.mininteger, math.maxinteger + 1 == math.mininteger)
print(math.tointeger(3.0), math.tointeger(3.5), math.tointeger(2 ^ 31))
print(math.ult(1, 2), math.ult(-1, 2))
print(0.1 + 0.2 == 0.3, 1.0 == 1, 1 == 1.0)
print(math.type(1 // 1), math.type(1 / 1))
print(string.format("%.17g", 0.1))
print(1e15, 1e16, 1e17)
print(9007199254740993, 9007199254740993.0, 9007199254740993 == 9007199254740993.0)
print(-0.0, 1 / -0.0 == -math.huge)
print(tostring(1e300 * 10))

-- escapes
print("\a\b\f\n\r\t\v\\\"\'")
print("\65\065\x41\x041\u{41}\z
      A" == "AAAAA")
print(#"\0", ("\0"):byte())
print("\xc3\xa9" == "é")
print(("\u{1F600}"):len())
print(pcall(load, 'return "\\x"'))
print(pcall(load, 'return "\\u{}"'))
print(pcall(load, 'return "\\u{110000}"'))
print(pcall(load, 'return "\\z"'))

-- long strings and comments
print([[abc]], [==[a]]b]==], [[
line1
line2]])
--[[ block
comment ]]
print(--[[inline]] 42)
print(1 --[[x]] + --[[y]] 2)

-- decimal escapes and separators
print(1 .. 2 .. 3)
print(0xff == 255, 0XFF == 255)
print(pcall(load, "return 0x"))
print(pcall(load, "return 1e"))
print(pcall(load, "return 1..2"))
print(pcall(load, "return 3-4"))
print(3 - 4)
print(pcall(load, "return 0xg"))
