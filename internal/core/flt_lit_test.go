package core

import (
	"os"
	"testing"
)

func TestFloatLiteralTag(t *testing.T) {
	src := "return 3.0, 3, 3.5\n"
	path := t.TempDir() + "/t.lua"
	os.WriteFile(path, []byte(src), 0o644)

	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := L.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	cl := clLvalue(&L.stack[L.top-1])
	for i := range cl.p.k {
		kv := &cl.p.k[i]
		t.Logf("k[%d] tt=%d isint=%v isflt=%v", i, kv.tt, ttisinteger(kv), ttisfloat(kv))
		if ttisinteger(kv) {
			t.Logf("  int %d", kv.i)
		}
		if ttisfloat(kv) {
			t.Logf("  flt %v", fltvalue(kv))
		}
	}
	if st := L.PCall(0, 3, 0); st != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatal(msg)
	}
	// stack: 3.0, 3, 3.5
	for _, idx := range []int{-3, -2, -1} {
		t.Logf("res[%d] isint=%v type=%s", idx, L.IsInteger(idx), TypeName(L.Type(idx)))
	}
	if !L.IsInteger(-2) {
		t.Fatal("3 should be integer")
	}
	if L.IsInteger(-3) {
		t.Fatal("3.0 should be float, not integer")
	}
	if L.IsInteger(-1) {
		t.Fatal("3.5 should be float")
	}
}
