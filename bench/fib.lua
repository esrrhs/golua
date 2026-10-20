-- Call overhead: the naive recursive Fibonacci, which is nothing but
-- interpreter dispatch and integer arithmetic, with a call every step.
--
--     lua bench/fib.lua [n]
local function fib(n)
  if n < 2 then return n end
  return fib(n - 1) + fib(n - 2)
end
print("fib", fib(tonumber(arg[1] or 28)))
