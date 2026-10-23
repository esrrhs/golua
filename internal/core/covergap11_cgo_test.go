//go:build cgo

package core

import (
	"testing"
	"unsafe"
)

func TestCoverGap11CdlMemHelpers(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("cgo")
	}
	L := NewState()
	defer L.Close()
	_ = cgoGoid()
	_ = internCStr("")
	_ = internCStr("x")
	id := registerState(L)
	_ = stateByID(id)
	_ = stateByID(0)
	_ = cstateTrackedCount(L.g)
	_ = cGoString(nil)
	s := internCStr("abc")
	_ = cGoString(unsafe.Pointer(s))
}
