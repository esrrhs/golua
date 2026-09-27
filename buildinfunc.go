package fakescript_go

import (
	"fmt"
	"strings"
)

type buildinfunc struct {
}

// ─── print ──────────────────────────────────────────────────────────────────

func buildin_print(inter *interpreter, ps *paramstack) {
	var strs []string
	for _, v := range ps.vlist {
		strs = append(strs, vartostring(v))
	}
	str := strings.Join(strs, " ")
	if gfs.cfg.FakePrint != nil {
		gfs.cfg.FakePrint(str)
	} else {
		fmt.Printf("%s\n", str)
	}
	ps.clear()
	// ret
	ps.push(str)
}

// ─── array() ────────────────────────────────────────────────────────────────

func buildin_array(inter *interpreter, ps *paramstack) {
	va := gfs.con.newarray()
	var v variant
	v.V_SET_ARRAY(va)
	ps.clear()
	ps.pushVariant(v)
}

// ─── map() ──────────────────────────────────────────────────────────────────

func buildin_map(inter *interpreter, ps *paramstack) {
	vm := gfs.con.newmap()
	var v variant
	v.V_SET_MAP(vm)
	ps.clear()
	ps.pushVariant(v)
}

// ─── gmap() ─────────────────────────────────────────────────────────────────

func buildin_gmap(inter *interpreter, ps *paramstack) {
	vm := gfs.con.newgmap()
	var v variant
	v.V_SET_MAP(vm)
	ps.clear()
	ps.pushVariant(v)
}

// ─── size(container) ────────────────────────────────────────────────────────

func buildin_size(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "size() requires 1 argument")
	}
	c := ps.vlist[0]
	var sz float64
	switch c.ty {
	case ARRAY:
		arr := c.data.(*variant_array)
		arr.lock.Lock()
		sz = float64(len(arr.va))
		arr.lock.Unlock()
	case MAP:
		m := c.data.(*variant_map)
		m.lock.Lock()
		sz = float64(len(m.vm))
		m.lock.Unlock()
	case STRING:
		sz = float64(len(c.data.(string)))
	default:
		sz = 0
	}
	ps.clear()
	ps.push(sz)
}

// ─── range(container, pos) → value, nextpos, ok ─────────────────────────────
// Returns: value at pos, next pos (or -1 when done), bool(1=ok 0=end)
// For arrays: pos is integer index
// For maps:   pos is not meaningful; iteration order not guaranteed

func buildin_range(inter *interpreter, ps *paramstack) {
	if ps.size() < 2 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "range() requires 2 arguments (container, pos)")
	}
	c := ps.vlist[0]
	pos := int(ps.vlist[1].V_GET_REAL())
	ps.clear()

	switch c.ty {
	case ARRAY:
		arr := c.data.(*variant_array)
		arr.lock.Lock()
		items := make([]*variant, len(arr.va))
		copy(items, arr.va)
		arr.lock.Unlock()
		if pos < 0 || pos >= len(items) {
			ps.push(nil_variant)
			ps.push(float64(-1))
			ps.push(float64(0))
		} else {
			v := items[pos]
			if v == nil {
				ps.pushVariant(nil_variant)
			} else {
				ps.pushVariant(*v)
			}
			ps.push(float64(pos + 1))
			ps.push(float64(1))
		}
	default:
		ps.push(nil_variant)
		ps.push(float64(-1))
		ps.push(float64(0))
	}
}

// ─── tonumber(v) ────────────────────────────────────────────────────────────

func buildin_tonumber(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "tonumber() requires 1 argument")
	}
	v := ps.vlist[0]
	ps.clear()
	switch v.ty {
	case REAL:
		ps.pushVariant(v)
	case STRING:
		var f float64
		fmt.Sscanf(v.data.(string), "%g", &f)
		ps.push(f)
	default:
		ps.push(float64(0))
	}
}

// ─── tolong(v) ──────────────────────────────────────────────────────────────

func buildin_tolong(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "tolong() requires 1 argument")
	}
	v := ps.vlist[0]
	ps.clear()
	switch v.ty {
	case REAL:
		ps.push(float64(int64(v.data.(float64))))
	case STRING:
		var i int64
		fmt.Sscanf(v.data.(string), "%d", &i)
		ps.push(float64(i))
	default:
		ps.push(float64(0))
	}
}

// ─── tostring(v) ────────────────────────────────────────────────────────────

func buildin_tostring(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "tostring() requires 1 argument")
	}
	v := ps.vlist[0]
	ps.clear()
	ps.push(vartostring(v))
}

// ─── typeof(v) → string ─────────────────────────────────────────────────────

func buildin_typeof(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "typeof() requires 1 argument")
	}
	v := ps.vlist[0]
	ps.clear()
	var t string
	switch v.ty {
	case NIL:
		t = "nil"
	case REAL:
		t = "number"
	case STRING:
		t = "string"
	case POINTER:
		t = "pointer"
	case UUID:
		t = "uuid"
	case ARRAY:
		t = "array"
	case MAP:
		t = "map"
	default:
		t = "unknown"
	}
	ps.push(t)
}

// ─── isfunc(name) → bool ─────────────────────────────────────────────────────

func buildin_isfunc(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "isfunc() requires 1 argument")
	}
	v := ps.vlist[0]
	ps.clear()
	f := gfs.fm.get_func(v)
	if f != nil {
		ps.push(float64(1))
	} else {
		ps.push(float64(0))
	}
}

// ─── format(fmt, ...) → string ───────────────────────────────────────────────

func buildin_format(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "format() requires at least 1 argument")
	}
	fmtstr := vartostring(ps.vlist[0])
	args := make([]interface{}, 0, ps.size()-1)
	for i := 1; i < ps.size(); i++ {
		v := ps.vlist[i]
		switch v.ty {
		case REAL:
			d := v.data.(float64)
			if isInt(d) {
				args = append(args, int64(d))
			} else {
				args = append(args, d)
			}
		case STRING:
			args = append(args, v.data.(string))
		default:
			args = append(args, vartostring(v))
		}
	}
	result := fmt.Sprintf(fmtstr, args...)
	ps.clear()
	ps.push(result)
}

// ─── dofile(filename) ────────────────────────────────────────────────────────

func buildin_dofile(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "dofile() requires 1 argument")
	}
	file := vartostring(ps.vlist[0])
	ps.clear()
	err := Parse(file)
	if err != nil {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "dofile %s fail: %v", file, err)
	}
}

// ─── dostring(src) ───────────────────────────────────────────────────────────

func buildin_dostring(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "dostring() requires 1 argument")
	}
	src := vartostring(ps.vlist[0])
	ps.clear()
	err := ParseString(src)
	if err != nil {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "dostring fail: %v", err)
	}
}

// ─── dumpfunc(name) → string ─────────────────────────────────────────────────

func buildin_dumpfunc(inter *interpreter, ps *paramstack) {
	if ps.size() < 1 {
		seterror(inter.getcurfile(), inter.getcurline(), inter.getcurfunc(), "dumpfunc() requires 1 argument")
	}
	name := ps.vlist[0]
	ps.clear()
	f := gfs.fm.get_func(name)
	if f == nil || !f.havefb {
		ps.push("")
		return
	}
	ps.push(f.fb.dump(0))
}

// ─── dumpallfunc() → string ──────────────────────────────────────────────────

func buildin_dumpallfunc(inter *interpreter, ps *paramstack) {
	ps.clear()
	ps.push(gfs.fm.dump())
}

// ─── getcurfile() → string ───────────────────────────────────────────────────

func buildin_getcurfile(inter *interpreter, ps *paramstack) {
	ps.clear()
	ps.push(inter.getcurfile())
}

// ─── getcurline() → number ───────────────────────────────────────────────────

func buildin_getcurline(inter *interpreter, ps *paramstack) {
	ps.clear()
	ps.push(float64(inter.getcurline()))
}

// ─── getcurfunc() → string ───────────────────────────────────────────────────

func buildin_getcurfunc(inter *interpreter, ps *paramstack) {
	ps.clear()
	ps.push(inter.getcurfunc())
}

// ─── getcurcallstack() → string ──────────────────────────────────────────────

func buildin_getcurcallstack(inter *interpreter, ps *paramstack) {
	ps.clear()
	// Build a simple call-stack string from bp chain
	var sb strings.Builder
	bp := inter.bp
	for bp > 0 {
		fb := inter.BP_GET_FB(bp)
		if fb != nil {
			sb.WriteString(fb.name)
			sb.WriteString("\n")
		}
		callbp := inter.BP_GET_BP(bp)
		if callbp >= bp {
			break // guard against loops
		}
		bp = callbp
	}
	ps.push(sb.String())
}

// ─── register ────────────────────────────────────────────────────────────────

func (bi *buildinfunc) reg(name string, bif bifunc) {
	var kv variant
	kv.V_SET_STRING(name)
	gfs.fm.add_buildin_func(kv, bif)
}

func (bi *buildinfunc) openbasefunc() {
	bi.reg("print", buildin_print)
	bi.reg("array", buildin_array)
	bi.reg("map", buildin_map)
	bi.reg("gmap", buildin_gmap)
	bi.reg("size", buildin_size)
	bi.reg("range", buildin_range)
	bi.reg("tonumber", buildin_tonumber)
	bi.reg("tolong", buildin_tolong)
	bi.reg("tostring", buildin_tostring)
	bi.reg("typeof", buildin_typeof)
	bi.reg("isfunc", buildin_isfunc)
	bi.reg("format", buildin_format)
	bi.reg("dofile", buildin_dofile)
	bi.reg("dostring", buildin_dostring)
	bi.reg("dumpfunc", buildin_dumpfunc)
	bi.reg("dumpallfunc", buildin_dumpallfunc)
	bi.reg("getcurfile", buildin_getcurfile)
	bi.reg("getcurline", buildin_getcurline)
	bi.reg("getcurfunc", buildin_getcurfunc)
	bi.reg("getcurcallstack", buildin_getcurcallstack)
}
