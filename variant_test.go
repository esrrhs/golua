package fakego

import (
	"fmt"
	"testing"
)

type AA struct {
	a int
}

func Test0001(t *testing.T) {
	m1 := make(map[variant]string)

	va := gfs.con.newarray()
	vm := gfs.con.newgmap()

	{
		tt := variant{}
		v := variant{}
		v.V_SET_NIL()
		fmt.Println(v.String())
		m1[v] = "1"
		va.va = append(va.va, &v)

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
	}

	{
		tt := variant{}
		v := variant{}
		v.V_SET_REAL(0.2)
		fmt.Println(v.String())
		m1[v] = "2"
		va.va = append(va.va, &v)

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
	}

	{
		tt := variant{}
		v := variant{}
		v.V_SET_STRING("abc")
		fmt.Println(v.String())
		m1[v] = "3"
		va.va = append(va.va, &v)

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt

	}

	pe := AA{}
	pe.a = 1
	{
		tt := variant{}
		v := variant{}

		v.V_SET_POINTER(&pe)
		fmt.Println(v.String())
		m1[v] = "4"
		va.va = append(va.va, &v)

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
	}

	{
		tt := variant{}
		v := variant{}
		v.V_SET_UUID(214124214)
		fmt.Println(v.String())
		m1[v] = "5"
		va.va = append(va.va, &v)

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
	}

	{
		tt := variant{}
		v := variant{}
		v.V_SET_ARRAY(va)
		fmt.Println(v.String())
		m1[v] = "6"
		va.va = append(va.va, &v)
		v.V_SET_ARRAY(va)
		fmt.Println(v.String())

		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
	}

	{
		tt := variant{}
		v := variant{}
		v.V_SET_MAP(vm)
		fmt.Println(v.String())
		m1[v] = "7"
		tt.V_SET_STRING(m1[v])
		vm.vm[v] = &tt
		v.V_SET_MAP(vm)
		fmt.Println(v.String())
	}

	////////////////////////////////////

	k := variant{}
	k.V_SET_NIL()
	fmt.Println(m1[k])
	if m1[k] != "1" {
		t.Error("fail")
	}

	k.V_SET_REAL(0.1)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	k.V_SET_REAL(0.2)
	fmt.Println(m1[k])
	if m1[k] != "2" {
		t.Error("fail")
	}

	k.V_SET_STRING("a")
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	k.V_SET_STRING("abc")
	fmt.Println(m1[k])
	if m1[k] != "3" {
		t.Error("fail")
	}

	pe1 := AA{}
	pe1.a = 1

	k.V_SET_POINTER(&pe1)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	k.V_SET_POINTER(&pe)
	fmt.Println(m1[k])
	if m1[k] != "4" {
		t.Error("fail")
	}

	k.V_SET_UUID(21412421)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	k.V_SET_UUID(214124214)
	fmt.Println(m1[k])
	if m1[k] != "5" {
		t.Error("fail")
	}

	k.V_SET_ARRAY(va)
	fmt.Println(m1[k])
	if m1[k] != "6" {
		t.Error("fail")
	}

	k.V_SET_MAP(vm)
	fmt.Println(m1[k])
	if m1[k] != "7" {
		t.Error("fail")
	}
}
