package fakego

type paramstack struct {
	vlist []variant
}

func (ps *paramstack) push(i interface{}) {
	var v variant
	v.from(i)
	ps.vlist = append(ps.vlist, v)
}

func (ps *paramstack) pushs(a []interface{}) {
	for i, _ := range a {
		ps.push(a[i])
	}
}

func (ps *paramstack) pop() interface{} {
	n := len(ps.vlist)
	v := ps.vlist[n-1]
	ps.vlist = ps.vlist[0 : n-1]
	return v.to()
}

func (ps *paramstack) pops() (a []interface{}) {
	for ps.size() > 0 {
		a = append(a, ps.pop())
	}
	return
}

func (ps *paramstack) size() int {
	return len(ps.vlist)
}

func (ps *paramstack) trans() (a []interface{}) {
	for _, v := range ps.vlist {
		a = append(a, v.to())
	}
	return
}

func (ps *paramstack) clear() {
	ps.vlist = ps.vlist[0:0]
}
