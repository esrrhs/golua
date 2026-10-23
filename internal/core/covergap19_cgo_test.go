//go:build cgo && unix

package core

import "testing"

func TestCoverGap19CMallocNilAndClosethread(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("cgo")
	}
	L := NewState()
	defer L.Close()

	testCMallocNilStatePtr(L)
	testCMallocNilPtrOf()

	st := testLuaClosethread(L, nil)
	_ = st

	co := L.NewThread()
	_ = co.LoadString(`error("cg19")`, "=e")
	_ = co.Resume(L, 0)
	_ = testLuaClosethread(co, L)

	testCFuncMigrate(L)
}

func TestCoverGap19CdlNilDebugAPI(t *testing.T) {
	if !capiHasChecks() {
		t.Skip("cgo")
	}
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	testLuaGetstackNil(L)
	testLuaGetstackMiss(L)
	testLuaGetinfoNil(L)
	testLuaLocalNil(L)
	testLuaUpvalueMiss(L)
	testLuaLNilBuf(L)
	testCdlPushEmpty(L)
}
