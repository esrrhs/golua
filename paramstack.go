package fakego

type paramstack struct {
	vlist []variant
	len   int
}

func (ps *paramstack) push(i interface{}) {
	if ps.len < len(ps.vlist) {
		ps.vlist[ps.len].from(i)
	} else {
		var v variant
		v.from(i)
		ps.vlist = append(ps.vlist, v)
	}
	ps.len++
}

func (ps *paramstack) pushs(a []interface{}) {
	for i, _ := range a {
		ps.push(a[i])
	}
}

func (ps *paramstack) pop() interface{} {
	v := ps.vlist[ps.len-1]
	ps.len--
	return v.to()
}

func (ps *paramstack) size() int {
	return ps.len
}

func (ps *paramstack) clear() {
	ps.len = 0
}
