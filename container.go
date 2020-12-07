package fakego

import (
	"errors"
	"fmt"
	"sync"
)

type variant_container_base struct {
	recurflag int32
	isconst   bool
	lock      sync.Mutex
}

type variant_array struct {
	variant_container_base
	va []*variant
}

type variant_map struct {
	variant_container_base
	vm map[variant]*variant
}

type container struct {
	gm   *variant_map
	once sync.Once
}

func (c *container) newarray() *variant_array {
	va := &variant_array{}
	return va
}

func (c *container) newmap() *variant_map {
	vm := &variant_map{}
	vm.vm = make(map[variant]*variant)
	return vm
}

func (c *container) newgmap() *variant_map {
	c.once.Do(func() {
		c.gm = c.newmap()
	})
	return c.gm
}

func (c *container) newconstarray() *variant_array {
	va := &variant_array{}
	va.isconst = true
	return va
}

func (c *container) newconstmap() *variant_map {
	vm := &variant_map{}
	vm.vm = make(map[variant]*variant)
	vm.isconst = true
	return vm
}

func (vm *variant_map) con_map_get(k variant) *variant {
	vm.lock.Lock()
	defer vm.lock.Unlock()
	var ret *variant
	ret, ok := vm.vm[k]
	if ok {
		return ret
	}
	ret = &variant{}
	vm.vm[k] = ret
	return ret
}

func (vm *variant_map) con_map_set(k variant, v *variant) {
	vm.lock.Lock()
	defer vm.lock.Unlock()
	vm.vm[k] = v
}

func (vm *variant_map) con_map_to() map[variant]*variant {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	ret := make(map[variant]*variant)
	for k, v := range vm.vm {
		ret[k] = v
	}
	return ret
}

func (va *variant_array) con_array_get(k variant) *variant {
	va.lock.Lock()
	defer va.lock.Unlock()

	index := int(k.V_GET_REAL())
	if index < 0 {
		panic(errors.New(fmt.Sprintf("interpreter get array fail, index %d", index)))
	}

	if index >= len(va.va) {
		oldsize := len(va.va)
		newsize := index + 1 + oldsize*gfs.cfg.ArrayGrowSpeed/100
		va.va = append(va.va, make([]*variant, newsize-oldsize)...)
	}

	if va.va[index] == nil {
		va.va[index] = &variant{}
	}

	return va.va[index]
}

func (va *variant_array) con_array_set(k variant, v *variant) {
	va.lock.Lock()
	defer va.lock.Unlock()

	index := int(k.V_GET_REAL())
	if index < 0 {
		panic(errors.New(fmt.Sprintf("interpreter set array fail, index %d", index)))
	}

	if index >= len(va.va) {
		oldsize := len(va.va)
		newsize := index + 1 + oldsize*gfs.cfg.ArrayGrowSpeed/100
		va.va = append(va.va, make([]*variant, newsize-oldsize)...)
	}

	va.va[index] = v
}

func (va *variant_array) con_array_to() []*variant {
	va.lock.Lock()
	defer va.lock.Unlock()

	ret := make([]*variant, len(va.va))
	copy(ret, va.va)
	return ret
}
