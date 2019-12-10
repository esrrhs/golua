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

	tt := variant{}

	v := variant{}
	_V_SET_NIL(&v)
	fmt.Println(v.String())
	m1[v] = "1"
	va.va = append(va.va, v)

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	_V_SET_REAL(&v, 0.2)
	fmt.Println(v.String())
	m1[v] = "2"
	va.va = append(va.va, v)

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	_V_SET_STRING(&v, "abc")
	fmt.Println(v.String())
	m1[v] = "3"
	va.va = append(va.va, v)

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	pe := AA{}
	pe.a = 1

	_V_SET_POINTER(&v, &pe)
	fmt.Println(v.String())
	m1[v] = "4"
	va.va = append(va.va, v)

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	_V_SET_UUID(&v, 214124214)
	fmt.Println(v.String())
	m1[v] = "5"
	va.va = append(va.va, v)

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	_V_SET_ARRAY(&v, va)
	fmt.Println(v.String())
	m1[v] = "6"
	va.va = append(va.va, v)
	_V_SET_ARRAY(&v, va)
	fmt.Println(v.String())

	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt

	_V_SET_MAP(&v, vm)
	fmt.Println(v.String())
	m1[v] = "7"
	_V_SET_STRING(&tt, m1[v])
	vm.vm[v] = tt
	_V_SET_MAP(&v, vm)
	fmt.Println(v.String())

	////////////////////////////////////

	k := variant{}
	_V_SET_NIL(&k)
	fmt.Println(m1[k])
	if m1[k] != "1" {
		t.Error("fail")
	}

	_V_SET_REAL(&k, 0.1)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	_V_SET_REAL(&k, 0.2)
	fmt.Println(m1[k])
	if m1[k] != "2" {
		t.Error("fail")
	}

	_V_SET_STRING(&k, "a")
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	_V_SET_STRING(&k, "abc")
	fmt.Println(m1[k])
	if m1[k] != "3" {
		t.Error("fail")
	}

	pe1 := AA{}
	pe1.a = 1

	_V_SET_POINTER(&k, &pe1)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	_V_SET_POINTER(&k, &pe)
	fmt.Println(m1[k])
	if m1[k] != "4" {
		t.Error("fail")
	}

	_V_SET_UUID(&k, 21412421)
	fmt.Println(m1[k])
	if m1[k] != "" {
		t.Error("fail")
	}

	_V_SET_UUID(&k, 214124214)
	fmt.Println(m1[k])
	if m1[k] != "5" {
		t.Error("fail")
	}

	_V_SET_ARRAY(&k, va)
	fmt.Println(m1[k])
	if m1[k] != "6" {
		t.Error("fail")
	}

	_V_SET_MAP(&k, vm)
	fmt.Println(m1[k])
	if m1[k] != "7" {
		t.Error("fail")
	}
}
