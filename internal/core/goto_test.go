package core

import "testing"

// TestGotoIfThenElseifLabels covers the 5.3.4 test_then_block path for
// "if cond then goto" / "elseif cond then goto" with an inner label of the
// same name as an outer one (goto.lua testG). A wrong port of that path
// used to corrupt the pending-jump list and panic while compiling.
func TestGotoIfThenElseifLabels(t *testing.T) {
	src := `
local function testG (a)
  if a == 1 then
    goto l1
    error("should never be here!")
  elseif a == 2 then goto l2
  elseif a == 3 then goto l3
  elseif a == 4 then
    goto l1
    error("should never be here!")
    ::l1:: a = a + 1
  else
    goto l4
    ::l4a:: a = a * 2; goto l4b
    error("should never be here!")
    ::l4:: goto l4a
    error("should never be here!")
    ::l4b::
  end
  do return a end
  ::l2:: do return "2" end
  ::l3:: do return "3" end
  ::l1:: return "1"
end
assert(testG(1) == "1")
assert(testG(2) == "2")
assert(testG(3) == "3")
assert(testG(4) == 5)
assert(testG(5) == 10)
`
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadString(src, "@goto_test"); err != nil {
		t.Fatal("load:", err)
	}
	if st := L.PCall(0, 0, 0); st != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal("pcall:", st, msg)
	}
}

func TestGotoOfficialCompiles(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	path := "testdata/official/goto.lua"
	if err := L.LoadFile(path); err != nil {
		t.Fatal("load goto.lua:", err)
	}
}
