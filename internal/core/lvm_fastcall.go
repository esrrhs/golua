package core

const (
	fastLeafUnknown = iota
	fastLeafNo
	fastLeafConstant
	fastLeafNumeric
)

// Fast paths disabled for the 5.3.4 port: leaf-call recognition was
// written against the 5.4 opcode set (MMBIN, LOADI, …). Re-enable only
// after rewriting constantLeaf/numericLeaf for 5.3 RK/ISA and the
// official suite is green under LUA_REF.
func fastLeafCall(L *State, funcIdx, nargs int) bool { return false }
func constantLeaf(p *Proto) bool                     { return false }
func numericLeaf(p *Proto) bool                      { return false }
