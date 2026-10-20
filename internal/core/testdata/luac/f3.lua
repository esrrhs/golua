return function(a)
  local t = {}
  for i = 1, a do t[#t+1] = i end
  local sum = 0
  for _, v in ipairs(t) do sum = sum + v end
  return sum, #t
end
