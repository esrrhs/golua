package fakego

import "sync"

type funcunion struct {
	fb      *func_binary
	haveff  bool
	havebif bool
	havefb  bool
}

type funcmap struct {
	shh sync.Map // variant->*funcunion
}

func (fm *funcmap) size() int {
	return 0
}

func (fm *funcmap) dump() string {
	return "TODO"
}

func (fm *funcmap) add_func(name *variant, fb *func_binary) {
	f := fm.add_func_union(name)
	f.fb = fb
	f.havefb = true
}

func (fm *funcmap) add_func_union(name *variant) *funcunion {
	p, ok := fm.shh.Load(*name)
	if ok {
		return p.(*funcunion)
	}

	tmp := funcunion{}
	fm.shh.Store(*name, &tmp)
	return &tmp
}
