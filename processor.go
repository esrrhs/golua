package fakescript_go

import (
	"time"
)

type processor struct {
	entryroutine *routine
	curroutine   *routine
	routines     []*routine
	genid        int
}

func newProcessor() *processor {
	return &processor{}
}

func (p *processor) start_routine(fun variant, ps *paramstack, retpos []int) *routine {
	r := newRoutine(p.genid)
	p.genid++
	r.inter.pro = p

	var newps paramstack
	if ps != nil {
		newps.vlist = append([]variant(nil), ps.vlist...)
	}

	r.entry(fun, &newps, retpos)

	if p.entryroutine == nil {
		p.entryroutine = r
		r.isMain = true
	}
	if p.curroutine == nil {
		p.curroutine = r
	}

	p.routines = append(p.routines, r)
	return r
}

func (p *processor) all_sleeping() bool {
	for _, r := range p.routines {
		if !r.inter.sleeping {
			return false
		}
	}
	return len(p.routines) > 0
}

func (p *processor) run() {
	cmdLimit := gfs.cfg.PerFrameCmdNum
	if cmdLimit <= 0 {
		cmdLimit = 100
	}

	for len(p.routines) > 0 {
		for i := 0; i < len(p.routines); i++ {
			r := p.routines[i]
			p.curroutine = r
			r.inter.run(cmdLimit)
			if r.is_end() {
				p.routines = append(p.routines[:i], p.routines[i+1:]...)
				i--
			}
		}

		if p.all_sleeping() {
			time.Sleep(1 * time.Millisecond)
		}
	}
}
