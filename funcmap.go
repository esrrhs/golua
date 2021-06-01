package fakego

import "sync"

type bifunc func(inter *interpreter, ps *paramstack)

type funcunion struct {
	fb      *func_binary
	bif     bifunc
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

func (fm *funcmap) add_func(name variant, fb *func_binary) {
	f := fm.add_func_union(name)
	f.fb = fb
	f.havefb = true
}

func (fm *funcmap) add_buildin_func(name variant, bif bifunc) {
	f := fm.add_func_union(name)
	f.bif = bif
	f.havebif = true
}

func (fm *funcmap) add_func_union(name variant) *funcunion {
	p, ok := fm.shh.Load(name)
	if ok {
		return p.(*funcunion)
	}

	tmp := funcunion{}
	fm.shh.Store(name, &tmp)
	return &tmp
}

func (fm *funcmap) get_func(name variant) *funcunion {
	p, ok := fm.shh.Load(name)
	if ok {
		return p.(*funcunion)
	}
	return nil
}
