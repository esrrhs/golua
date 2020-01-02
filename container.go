package fakego

import "sync"

type variant_container_base struct {
	recurflag int32
	isconst   bool
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
