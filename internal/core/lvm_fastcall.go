package core

const (
	fastLeafUnknown = iota
	fastLeafNo
	fastLeafConstant
	fastLeafNumeric
)

// Fast paths disabled for the 5.3.4 port (opcodes changed).
func fastLeafCall(L *State, funcIdx, nargs int) bool { return false }
func constantLeaf(p *Proto) bool                     { return false }
func numericLeaf(p *Proto) bool                      { return false }
