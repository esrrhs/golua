package fakescript_go

type routine struct {
	id     int
	inter  *interpreter
	isMain bool
}

func newRoutine(id int) *routine {
	return &routine{
		id:    id,
		inter: &interpreter{},
	}
}

func (r *routine) entry(fun variant, ps *paramstack, retpos []int) {
	r.inter.call(fun, ps, retpos)
}

func (r *routine) is_end() bool {
	return r.inter.isend
}
