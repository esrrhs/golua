package fakego

import (
	"fmt"
	"sync"
)

type bifunc func(inter *interpreter, ps *paramstack)

type funcunion struct {
	fb      *func_binary
	bif     bifunc
	ff      fkfunctor
	haveff  bool
	havebif bool
	havefb  bool
}

type funcmap struct {
	shh sync.Map // variant->*funcunion
}

func (fm *funcmap) size() int {
	len := 0
	fm.shh.Range(func(key, value interface{}) bool {
		len++
		return true
	})
	return len
}

func (fm *funcmap) dump() string {
	str := ""
	fm.shh.Range(func(key, value interface{}) bool {
		k := key.(variant)
		f := value.(*funcunion)
		str += fmt.Sprintf("%v\tfb(%v)\tbif(%v)\tff(%v)\n", vartostring(k), f.havefb, f.havebif, f.haveff)
		return true
	})
	return str
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

func (fm *funcmap) add_bind_func(name variant, ff fkfunctor) {
	f := fm.add_func_union(name)
	f.ff = ff
	f.haveff = true
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
