//go:build !cgo

package core

// Pure-Go stand-in for POSIX random()/srandom() (TYPE_3, degree 31)
// when cgo is off. Matches glibc's default initstate for small seeds
// well enough for suite files that do not depend on exact sequences;
// with cgo the real libc generators are used.

var (
	nocgoRandState [31]int32
	nocgoRandFptr  int
	nocgoRandRptr  int
)

func lSrand(seed uint) {
	if seed == 0 {
		seed = 1
	}
	nocgoRandState[0] = int32(seed)
	for i := 1; i < 31; i++ {
		/* glibc: long int hi = s / 127773; ... */
		hi := int32(seed) / 127773
		lo := int32(seed) % 127773
		seed = uint(16807*lo - 2836*hi)
		if int32(seed) < 0 {
			seed = uint(int32(seed) + int32(0x7fffffff))
		}
		nocgoRandState[i] = int32(seed)
	}
	nocgoRandFptr = 3
	nocgoRandRptr = 0
	for i := 0; i < 10*31; i++ {
		lRand()
	}
}

func lRand() int32 {
	nocgoRandState[nocgoRandFptr] += nocgoRandState[nocgoRandRptr]
	i := nocgoRandState[nocgoRandFptr]
	/* discard sign bit (glibc TYPE_3) */
	val := i >> 1
	nocgoRandFptr++
	if nocgoRandFptr >= 31 {
		nocgoRandFptr = 0
	}
	nocgoRandRptr++
	if nocgoRandRptr >= 31 {
		nocgoRandRptr = 0
	}
	return val & 0x7fffffff
}

const lRandMax = 2147483647
