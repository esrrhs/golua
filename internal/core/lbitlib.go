// Translation of lbitlib.c (PUC-Lua 5.3.4): bit32 library (LUA_COMPAT_BITLIB).
package core

const luaNBits = 32

var allOnes32 = uint64(0xffffffff)

func trim(x uint64) uint64 { return x & allOnes32 }

func maskN(n uint) uint64 {
	if n >= luaNBits {
		return allOnes32
	}
	return (uint64(1) << n) - 1
}

func shift(r uint64, i int) uint64 {
	if i < 0 {
		if i <= -luaNBits {
			return 0
		}
		return trim(r) >> uint(-i)
	}
	if i >= luaNBits {
		return 0
	}
	return trim(r << uint(i))
}

func checkUnsigned(L *State, arg int) uint64 {
	return trim(uint64(L.CheckInteger(arg)))
}

func pushUnsigned(L *State, n uint64) {
	L.PushInteger(int64(trim(n)))
}

func b_and(L *State) int {
	n := L.GetTop()
	r := allOnes32
	for i := 1; i <= n; i++ {
		r &= checkUnsigned(L, i)
	}
	pushUnsigned(L, r)
	return 1
}

func b_or(L *State) int {
	n := L.GetTop()
	var r uint64
	for i := 1; i <= n; i++ {
		r |= checkUnsigned(L, i)
	}
	pushUnsigned(L, r)
	return 1
}

func b_xor(L *State) int {
	n := L.GetTop()
	var r uint64
	for i := 1; i <= n; i++ {
		r ^= checkUnsigned(L, i)
	}
	pushUnsigned(L, r)
	return 1
}

func b_not(L *State) int {
	r := ^checkUnsigned(L, 1)
	pushUnsigned(L, r)
	return 1
}

func b_shift(L *State, r uint64, i int64) int {
	pushUnsigned(L, shift(r, int(i)))
	return 1
}

func b_lshift(L *State) int {
	return b_shift(L, checkUnsigned(L, 1), L.CheckInteger(2))
}

func b_rshift(L *State) int {
	return b_shift(L, checkUnsigned(L, 1), -L.CheckInteger(2))
}

func b_arshift(L *State) int {
	r := checkUnsigned(L, 1)
	i := L.CheckInteger(2)
	if i < 0 || (r&(uint64(1)<<(luaNBits-1))) == 0 {
		return b_shift(L, r, -i)
	}
	// arithmetic shift: fill with 1s
	if i >= luaNBits {
		r = allOnes32
	} else {
		r = trim((r >> uint(i)) | ^(allOnes32 >> uint(i)))
	}
	pushUnsigned(L, r)
	return 1
}

func b_rot(L *State, i int) int {
	r := checkUnsigned(L, 1)
	i %= luaNBits
	if i < 0 {
		i = -i
		r = trim((r >> uint(i)) | (r << uint(luaNBits-i)))
	} else {
		r = trim((r << uint(i)) | (r >> uint(luaNBits-i)))
	}
	pushUnsigned(L, r)
	return 1
}

func b_lrot(L *State) int { return b_rot(L, int(L.CheckInteger(2))) }
func b_rrot(L *State) int { return b_rot(L, -int(L.CheckInteger(2))) }

func fieldArgs(L *State, farg int, width *int) int {
	f := int(L.CheckInteger(farg))
	w := int(L.OptInteger(farg+1, 1))
	L.ArgCheck(f >= 0, farg, "field cannot be negative")
	L.ArgCheck(w > 0, farg+1, "width must be positive")
	L.ArgCheck(f+w <= luaNBits, farg, "trying to access non-existent bits")
	*width = w
	return f
}

func b_extract(L *State) int {
	r := checkUnsigned(L, 1)
	var w int
	f := fieldArgs(L, 2, &w)
	pushUnsigned(L, (r>>uint(f))&maskN(uint(w)))
	return 1
}

func b_replace(L *State) int {
	r := checkUnsigned(L, 1)
	v := checkUnsigned(L, 2)
	var w int
	f := fieldArgs(L, 3, &w)
	m := maskN(uint(w))
	v &= m
	r = (r & ^(m << uint(f))) | (v << uint(f))
	pushUnsigned(L, r)
	return 1
}

var bitlib = []Reg{
	{Name: "arshift", Func: b_arshift},
	{Name: "band", Func: b_and},
	{Name: "bnot", Func: b_not},
	{Name: "bor", Func: b_or},
	{Name: "bxor", Func: b_xor},
	{Name: "btest", Func: nil}, // filled below
	{Name: "extract", Func: b_extract},
	{Name: "lrotate", Func: b_lrot},
	{Name: "lshift", Func: b_lshift},
	{Name: "replace", Func: b_replace},
	{Name: "rrotate", Func: b_rrot},
	{Name: "rshift", Func: b_rshift},
}

func b_test(L *State) int {
	n := L.GetTop()
	r := allOnes32
	for i := 1; i <= n; i++ {
		r &= checkUnsigned(L, i)
	}
	L.PushBoolean(r != 0)
	return 1
}

func OpenBit32(L *State) int {
	for i := range bitlib {
		if bitlib[i].Name == "btest" {
			bitlib[i].Func = b_test
		}
	}
	L.NewLib(bitlib)
	return 1
}
