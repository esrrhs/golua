package core

// Compatibility stubs for symbols the public golua.go still re-exports
// from the 5.4 API surface. Negative / zero values; not used by the 5.3 VM.

const (
	MAXARG_sC = MAXARG_C >> 1
	MAXARG_sB = MAXARG_B >> 1
	MAXARG_sJ = (1 << 25) - 1

	SHRT_MAX       = 32767

	VDKREG     = 0
	RDKCONST   = 1
	RDKTOCLOSE = 2
	RDKCTC     = 3

	VKSTR     = expkind(100)
	VCONST    = expkind(101)
	VINDEXUP  = expkind(102)
	VINDEXI   = expkind(103)
	VINDEXSTR = expkind(104)
	VRELOC    = VRELOCABLE
)

// SIZE_A already in lopcodes; ensure exported name exists for golua.go
