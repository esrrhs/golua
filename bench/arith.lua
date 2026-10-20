-- Numeric loop: float arithmetic in a register-bound loop, the shape of
-- code that measures dispatch and float handling rather than the
-- collector, since nothing here allocates.
--
--     lua bench/arith.lua [iterations]
local iters = tonumber(arg[1] or 20000000)

local s = 0.0
for i = 1, iters do
  s = s + i * 1.5
end
print(string.format("arith %.1f", s))
