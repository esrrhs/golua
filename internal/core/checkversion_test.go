package core

import (
	"strings"
	"testing"
)

func TestCheckVersion_(t *testing.T) {
	L := NewState()
	defer L.Close()

	L.PushGoFunc(func(L *State) int {
		L.CheckVersion_(float64(luaVersionNumGo), luaLNumSizes)
		return 0
	})
	if st := L.PCall(0, 0, 0); st != LUA_OK {
		msg, _ := L.ToString(-1)
		t.Fatalf("matching version/size: status=%d msg=%q", st, msg)
	}

	L.PushGoFunc(func(L *State) int {
		L.CheckVersion_(503, luaLNumSizes)
		return 0
	})
	if st := L.PCall(0, 0, 0); st == LUA_OK {
		t.Fatal("wrong version should raise")
	}
	msg, _ := L.ToString(-1)
	if !strings.Contains(msg, "version mismatch") {
		t.Fatalf("want version mismatch, got %q", msg)
	}
	L.SetTop(0)

	L.PushGoFunc(func(L *State) int {
		L.CheckVersion_(float64(luaVersionNumGo), 0)
		return 0
	})
	if st := L.PCall(0, 0, 0); st == LUA_OK {
		t.Fatal("wrong numeric size should raise")
	}
	msg, _ = L.ToString(-1)
	if !strings.Contains(msg, "incompatible numeric types") {
		t.Fatalf("want incompatible numeric types, got %q", msg)
	}
}
