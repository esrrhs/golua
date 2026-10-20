-- 'arg' the way lua.c's 'createargtable' builds it: index 0 is the
-- script, the negative indices hold the interpreter and the options.
-- (The runner always passes "lua" as argv[0], so arg[-1] is "lua".)
print(arg[0])
print(arg[-1])
print(#arg, arg[1], arg[2])
print(type(arg), type(arg[0]), type(arg[1]))
