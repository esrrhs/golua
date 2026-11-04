package core

import "testing"

func TestModDivFloorSemantics(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	src := `
local function check(a,b, want)
  local g = a % b
  if g ~= want then error(string.format("%s %% %s => %s want %s", tostring(a),tostring(b),tostring(g),tostring(want))) end
end
check(10, 4, 2)
check(10, 3, 1)
check(7, 7, 0)
check(1, 7, 1)
check(-10, 4, 2)
check(-7, 3, 2)
check(-1, 7, 6)
check(10, -4, -2)
check(7, -3, -2)
check(-10, -4, -2)
check(-7, -3, -1)
assert(10 // 3 == 3)
assert(-10 // 3 == -4)
assert(10 // -3 == -4)
assert(-10 // -3 == 3)
assert((-1) // 1 == -1)
assert(0 % 5 == 0)
assert(-5 % 5 == 0)
local ok, err = pcall(function() return 1 % 0 end)
assert(not ok)
ok, err = pcall(function() return 1 // 0 end)
assert(not ok)
`
	if err := L.DoString(src); err != nil {
		t.Fatal(err)
	}
}
