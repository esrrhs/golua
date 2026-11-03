package fastcall

// Call0 calls fn with no arguments and returns the integer result.
func Call0(fn uintptr) uintptr

// Call1 calls fn(a1) and returns the integer result.
func Call1(fn, a1 uintptr) uintptr

// Call2 calls fn(a1, a2) and returns the integer result.
func Call2(fn, a1, a2 uintptr) uintptr
