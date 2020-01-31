package fakego

type paramstack struct {
	vlist []variant
}

func (ps *paramstack) push(i interface{}) {
	var v variant
	v.from(i)
	ps.vlist = append(ps.vlist, v)
}

func (ps *paramstack) pop() interface{} {
	v := ps.vlist[len(ps.vlist)-1]
	ps.vlist = ps.vlist[0 : len(ps.vlist)-1]
	return v.to()
}
