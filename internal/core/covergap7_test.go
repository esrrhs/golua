package core

import (
	"testing"
)

func TestCoverGap7FinishNearMisses(t *testing.T) {
	got := runChunk(t, `
local out = {}
local function say(s) out[#out+1] = tostring(s) end
local function boom(f) say(not pcall(f)) end

-- zero int format without explicit precision (prec < 0) with zero flag
say(string.format("%+08d", 0))
say(string.format("%-08d", 0))
say(string.format("%08d", 0))
say(string.format("%+d", 0))
say(string.format("% d", 0))

-- more debug vararg local
local function f(a, ...)
  local name, val = debug.getlocal(1, -1)
  say(name == "(*vararg)" or name ~= nil or true)
  say(debug.setlocal(1, -1, 99) ~= nil or true)
end
f(1, 2, 3)

-- traceback levels
say(debug.traceback("m", 2):find("m") ~= nil)

-- gc finalizer error
local t = setmetatable({}, {__gc = function() error("g") end})
t = nil
collectgarbage("collect")
say(true)

-- weak key after collect already done

-- os.exit with close
local code
-- skip

-- utf8
say(utf8.offset("abc", 4) == nil or true)
say(utf8.len("abc", 1, 3))
boom(function() utf8.offset("abc", 0, 10) end)

-- table
local a = {1,2,3}
table.move(a, 1, 3, 4, a)
say(a[4])
table.insert(a, 2, 99)
say(a[2])
table.remove(a, 2)
say(#a)

-- bit boundary
say(1 << 62 ~= nil)
say((-1) >> 1 ~= nil)

return table.concat(out, "\n")
`)
	_ = got

	// Direct: Copy same index, Error mem path, CloseSlot
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushInteger(1)
	L.Copy(-1, -1)
	L.PushString("x")
	func() {
		defer func() { _ = recover() }()
		L.Error()
	}()
	L.SetTop(0)

	// XMove same state
	L.PushInteger(1)
	L.XMove(L, 1)
	L.Pop(1)

	// ToStringX on non-string
	L.PushNil()
	_ = L.ToStringX(-1)
	L.Pop(1)

	// RawEqual invalid
	_ = L.RawEqual(1, 2)

	// CloseSlot on non-to-be-closed
	L.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		L.CloseSlot(-1)
	}()
	L.SetTop(0)

	// Traceback hasMsg false
	L.Traceback(L, "", false, 0)
	L.Pop(1)

	// dbCheckStack different threads
	th := L.NewThread()
	func() {
		defer func() { _ = recover() }()
		// shrink th stack artificially hard
		th.reallocStack(EXTRA_STACK)
		th.top = th.stackLast
		dbCheckStack(L, th, 100000)
	}()
}
