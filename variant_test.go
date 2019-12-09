package fakego

import (
	"fmt"
	"testing"
)

func Test0001(t *testing.T) {
	m1 := make(map[variant]string)

	v := variant{}
	v.ty = NIL
	fmt.Println(v.String())
	m1[v] = "1"

	v.ty = REAL
	v.data = 0.2
	fmt.Println(v.String())
	m1[v] = "2"

	v.ty = STRING
	v.data = "abc"
	fmt.Println(v.String())
	m1[v] = "3"

	pe := pointerele{}
	pe.ty = "aaa"
	pe.ptr = &pe

	v.ty = POINTER
	v.data = &pe
	fmt.Println(v.String())
	m1[v] = "4"

	v.ty = UUID
	v.data = uint64(214124214)
	fmt.Println(v.String())

	k := variant{}
	k.ty = NIL
	fmt.Println(m1[k])

	k.ty = REAL
	k.data = 0.1
	fmt.Println(m1[k])

	k.ty = REAL
	k.data = 0.2
	fmt.Println(m1[k])

	k.ty = STRING
	k.data = "a"
	fmt.Println(m1[k])

	k.ty = STRING
	k.data = "abc"
	fmt.Println(m1[k])

	k.ty = POINTER
	k.data = &pe
	fmt.Println(m1[k])

	pe1 := pointerele{}
	pe1.ty = "aaa"
	pe1.ptr = &pe

	k.ty = POINTER
	k.data = &pe1
	fmt.Println(m1[k])
}
