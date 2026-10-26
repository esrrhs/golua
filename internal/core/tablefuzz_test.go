package core

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The thirteenth fuzz, over the table library.
//
// Six functions, and every one of them is small enough to hold in your
// head -- which is exactly why a port can get one wrong in a way no
// test notices: 'table.move' copies a range in one of two directions
// and only one of them is right when the range overlaps itself;
// 'table.remove' and 'table.insert' each keep their own rule about
// which positions are in bounds and the two are not the same rule;
// 'table.concat' names the type of the value it refuses; and
// 'table.sort' is a quicksort whose pivot and whose order of
// comparisons a caller can see, through a comparison function that
// keeps a tally or one that raises.
//
// A probe of 209 hand-picked cases found one thing:
//
//   A default argument that C does not evaluate.  'tunpack' reads its
//   last argument with the macro 'luaL_opt(L, luaL_checkinteger, 3,
//   luaL_len(L, 1))', whose default is an expression: it is evaluated
//   only in the branch that needs it.  Go evaluates every argument
//   before it is entered, so this port asked for the length even when
//   the third argument was there, and 'table.unpack(nil, 1, 3)' was
//   "attempt to get length of a nil value" where PUC-Lua, which never
//   gets as far as the length, answers "attempt to index a nil value".
//
//   By the rule of this project a finding is not finished until the
//   fix is taken out and it goes red, and it does: it is one of the
//   209 above.
//
// Three things the fuzz does not ask, and one of them it cannot:
//
//   A 'table.move' that would move a huge number of elements.
//   'table.move({}, 1, math.maxinteger, 1)' passes both of its checks
//   and then spends the rest of time copying: it hangs PUC-Lua itself,
//   on every machine, so there is no answer to compare against.  What
//   the two checks say is asked instead -- "too many elements to move"
//   and "destination wrap around" are both refused before the copy
//   begins.
//
//   A 'table.sort' of more than 128 elements with a comparison
//   function whose answer depends on the order it is called in.  PUC-Lua
//   starts with a randomization of 0, which its 'choosePivot' is not
//   asked to use, and reaches for a real one -- 'l_randomizePivot',
//   which reads 'clock()' and 'time(NULL)' -- only after a partition
//   lands badly, which is what '(up - lo) / 128 > n' says, where 'n' is
//   the smaller of the two intervals the partition left.  Since 'up -
//   lo' is one less than the size of the interval, an interval of 128
//   or fewer elements can never set it: 127 / 128 is 0, and 0 is not
//   more than anything.  So an array of 128 or fewer elements always
//   picks the middle element, and the pairs it compares are fixed; a
//   bigger one can pick differently from one second to the next, and
//   although the array it ends up with is the same either way, the
//   pairs are not.  The tally is asked of the first and not of the
//   second.
//
//   A 'table.sort' of an array of more than INT_MAX elements, which is
//   what "array too big" is for.  Building one is not a thing a test
//   pays for.
//
// The same six functions are asked a second time in this process, by
// 'TestTableInProcess' below, which is the only way they are ever
// entered as far as 'go test -cover' is concerned (§5.10).  It is not
// a copy of the grid: it asks what the grid cannot hold in a
// subprocess -- a userdata, the two places 'partition' refuses an
// order that is not one, and the messages, which name the function as
// C found it and not as the source spells it.

// tableMeta is one of the three metamethods 'checktab' can ask for.
type tableMeta struct {
	name               string
	index, newindex, l bool
}

// tableMetaCombos are all eight of them.  Which of the three a function
// needs is in the name of the case, and which of them it gets is the
// difference between being let through and being told it is not a
// table.
var tableMetaCombos = []tableMeta{
	{"none", false, false, false},
	{"index", true, false, false},
	{"newindex", false, true, false},
	{"len", false, false, true},
	{"index+len", true, false, true},
	{"newindex+len", false, true, true},
	{"index+newindex", true, true, false},
	{"all", true, true, true},
}

// mtBody is the metatable for one of them, as a Lua table literal.
// With 'shadow' the two that move values around are written to go
// through a table called 'sh', so that a case can ask afterwards
// which keys were touched.
func mtBody(c tableMeta, shadow bool) string {
	var f []string
	if c.index {
		if shadow {
			f = append(f, "__index = function(_, k) return sh[k] end")
		} else {
			f = append(f, "__index = function(_, k) return 42 end")
		}
	}
	if c.newindex {
		if shadow {
			f = append(f, "__newindex = function(_, k, v) sh[k] = v end")
		} else {
			f = append(f, "__newindex = function() end")
		}
	}
	if c.l {
		f = append(f, "__len = function() return 2 end")
	}
	return "{" + strings.Join(f, ", ") + "}"
}

// tableKinds are the eight kinds of value as Lua expressions, the ones
// the library can be handed in place of a table or a string.
var tableKinds = []string{"nil", "true", "false", "0", "1.5", `"s"`,
	"{}", "print"}

// tableKindNames name them, for the tag of a case.
var tableKindNames = []string{"nil", "true", "false", "int", "float",
	"str", "table", "func"}

var tableChunk = `
local n = 0
local function P(tag, ...)
  n = n + 1
  local p = {}
  for k = 1, select("#", ...) do p[k] = q((select(k, ...))) end
  print(n .. "\t" .. tag .. "\t" .. table.concat(p, "\t"))
end
-- a userdata with a metatable on it: the only kind of value a script
-- can make that is not a table and can still be asked the three
-- metamethods, which is what 'checktab' is about.  It is a file handle,
-- because there is no other way to make one in portable Lua.
local function U(mt)
  local u = io.tmpfile()
  debug.setmetatable(u, mt)
  return u
end
-- a case that raises prints the message, which is part of the answer
local function C(fn, ...)
  local ok, v = pcall(fn, ...)
  if ok then return v end
  return "raise: " .. tostring(v)
end
-- a table as its array part and its length, with the keys that are not
-- in it behind.  A sorted table and a shuffled one have the same keys
-- in different places, and '#' is what says which is which.
local function dump(v)
  if type(v) ~= "table" then return q(v) end
  local len = #v
  local p = {}
  for i = 1, len do p[i] = q(v[i]) end
  local ks = {}
  for k, vv in pairs(v) do
    if type(k) ~= "number" or k < 1 or k > len or k ~= math.floor(k) then
      ks[#ks + 1] = tostring(k) .. "=" .. q(vv)
    end
  end
  table.sort(ks)
  return "#" .. len .. "[" .. table.concat(p, ",") .. "]" ..
         (#ks > 0 and "{" .. table.concat(ks, ",") .. "}" or "")
end
`

// tableMutation wraps a call that mutates a table: it builds the table,
// calls the operation, and answers with the table as it is afterwards
// or with the message of the refusal.  It is a string of Lua rather
// than a function because the case is not a value: it is the whole
// sentence, and the two interpreters each read it in their own process.
func tableMutation(build, call string) string {
	return "(function() " + build + " local ok, e = pcall(" + call +
		"); if not ok then return \"raise: \" .. tostring(e) end " +
		"return dump(t) end)()"
}

func TestTableFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the table fuzz")
	}
	var b strings.Builder
	b.WriteString(fuzzQuote)
	b.WriteString(tableChunk)
	n := 0
	emit := func(tag, call string) {
		n++
		b.WriteString("P(" + strconv.Quote(tag) + ", " + call + ")\n")
	}
	/* Each group counts itself, so the note the test writes says what
	   the corpus is made of without anybody having to add it up. */
	at := 0
	took := func() int {
		d := n - at
		at = n
		return d
	}

	// 1. 'table.move': every f, e and t over a table of four, in both
	//    directions and to another table.  Where the destination is
	//    inside the range that is being copied, the order the elements
	//    are copied in is the whole difference.
	const four = "{10,20,30,40}"
	for f := 0; f <= 5; f++ {
		for e := 0; e <= 5; e++ {
			for tt := 0; tt <= 5; tt++ {
				mv := fmt.Sprintf("table.move, t, %d, %d, %d", f, e, tt)
				emit(fmt.Sprintf("move/%d,%d,%d", f, e, tt),
					tableMutation("local t = "+four+";", mv))
				emit(fmt.Sprintf("move2/%d,%d,%d", f, e, tt),
					"(function() local t = "+four+
						"; local o = {}; local ok, e = pcall(table.move, t, "+
						fmt.Sprintf("%d, %d, %d, o)", f, e, tt)+
						"; if not ok then return \"raise: \" .. tostring(e) end "+
						"return dump(t) .. \" -> \" .. dump(o) end)()")
			}
		}
	}
	// and the two checks that refuse before anything is copied
	emit("move/too-many", "C(table.move, {}, 0, math.maxinteger, 1)")
	emit("move/too-many-neg", "C(table.move, {}, -1, math.maxinteger, 1)")
	emit("move/wrap", "C(table.move, {1,2,3}, 1, 3, math.maxinteger-1)")
	emit("move/wrap-2", "C(table.move, {1,2,3}, 1, 3, math.maxinteger)")
	emit("move/1arg", "C(table.move, {1,2,3})")
	emit("move/2arg", "C(table.move, {1,2,3}, 1)")
	emit("move/3arg", "C(table.move, {1,2,3}, 1, 2)")
	emit("move/dst-nil", "C(table.move, {1,2,3}, 1, 2, 1, nil)")
	emit("move/dst-other", "C(table.move, {1,2,3}, 1, 2, 1, {})")
	nMove := took()

	// 2. 'table.remove' and 'table.insert', at every position of a
	//    table of every size up to four.  'remove' passes a position
	//    of 'size' without checking it at all, and accepts 'size + 1';
	//    'insert' accepts one position more than that, because its
	//    range is measured after the new element has a slot.
	for s := 0; s <= 4; s++ {
		arr := "{"
		for i := 1; i <= s; i++ {
			if i > 1 {
				arr += ","
			}
			arr += strconv.Itoa(i * 10)
		}
		arr += "}"
		emit(fmt.Sprintf("remove/%d/default", s),
			tableMutation("local t = "+arr+";", "table.remove, t"))
		emit(fmt.Sprintf("insert/%d/append", s),
			tableMutation("local t = "+arr+";", "table.insert, t, 99"))
		for p := -1; p <= s+2; p++ {
			emit(fmt.Sprintf("remove/%d/%d", s, p),
				tableMutation("local t = "+arr+";",
					fmt.Sprintf("table.remove, t, %d", p)))
			emit(fmt.Sprintf("insert/%d/%d", s, p),
				tableMutation("local t = "+arr+";",
					fmt.Sprintf("table.insert, t, %d, 99", p)))
		}
	}
	emit("insert/4args", "C(table.insert, {}, 1, 1, 1)")
	emit("insert/1arg", "C(table.insert, {})")
	emit("insert/0arg", "C(table.insert)")
	nEnds := took()

	// 3. 'table.concat': every kind of value in every position, every
	//    kind of separator, and every pair of ends.
	for k, kind := range tableKinds {
		for p := 1; p <= 3; p++ {
			arr := "{"
			for i := 1; i <= 3; i++ {
				if i > 1 {
					arr += ","
				}
				if i == p {
					arr += kind
				} else {
					arr += `"x"`
				}
			}
			arr += "}"
			emit(fmt.Sprintf("concat/%s/%d", tableKindNames[k], p),
				"C(table.concat, "+arr+", \",\")")
		}
		emit("concat-sep/"+tableKindNames[k],
			"C(table.concat, {\"a\",\"b\"}, "+kind+")")
	}
	for i := 0; i <= 4; i++ {
		for j := 0; j <= 4; j++ {
			emit(fmt.Sprintf("concat-ij/%d,%d", i, j),
				"C(table.concat, {\"a\",\"b\",\"c\"}, \"-\", "+itoa(i)+", "+itoa(j)+")")
		}
	}
	emit("concat/empty", "C(table.concat, {}, \",\")")
	emit("concat/long-sep", "C(table.concat, {\"a\",\"b\"}, string.rep(\"-\", 100))")
	/* A separator with a zero byte in it: a C string ends at the zero,
	   so what 'strcat' copies is the part before it. */
	emit("concat/zero-sep", "C(table.concat, {\"a\",\"b\"}, \"\\0\")")
	emit("concat/hole", "C(table.concat, {[1]=\"a\",[3]=\"c\"}, \",\")")
	emit("concat/neg", "C(table.concat, {[-1]=\"a\"}, \",\", -1, -1)")
	emit("concat/no-sep", "C(table.concat, {\"a\",\"b\"})")
	nConcat := took()

	// 4. 'table.unpack' and 'table.pack'.
	for i := -1; i <= 4; i++ {
		for j := -1; j <= 4; j++ {
			emit(fmt.Sprintf("unpack/%d,%d", i, j),
				"C(table.unpack, {10,20,30}, "+itoa(i)+", "+itoa(j)+")")
		}
	}
	for _, e := range []string{"1000000000", "2^31", "math.maxinteger",
		"math.mininteger"} {
		emit("unpack/huge/"+e, "C(table.unpack, {}, 1, "+e+")")
		emit("unpack/huge0/"+e, "C(table.unpack, {}, 0, "+e+")")
	}
	for k, kind := range tableKinds {
		emit("unpack-of/"+tableKindNames[k],
			"C(table.unpack, "+kind+", 1, 1)")
		emit("pack-of/"+tableKindNames[k],
			"dump(C(table.pack, "+kind+"))")
	}
	emit("unpack/holes", "C(table.unpack, {[1]=\"a\",[3]=\"c\"}, 1, 3)")
	emit("unpack/norange", "C(table.unpack, {10,20,30})")
	emit("unpack/nil-3rd", "C(table.unpack, {1,2,3}, 1, nil)")
	emit("unpack/100", "select(\"#\", C(table.unpack, {1,2,3}, 1, 3))")
	emit("pack/3", "dump(C(table.pack, 1, 2, 3))")
	emit("pack/nils", "dump(C(table.pack, nil, nil))")
	emit("pack/1", "dump(C(table.pack, nil))")
	emit("pack/0", "dump(C(table.pack))")
	nUnpack := took()

	// 5. 'table.sort'.  The comparison function is asked about every
	//    pair, in the order it is asked about them, and it writes them
	//    all down: a quicksort that picks a different pivot compares
	//    different pairs, and the two tallies would not be alike.
	for _, size := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 16, 32} {
		for _, shape := range []string{"asc", "desc", "same", "shuf"} {
			emit(fmt.Sprintf("sortlog/%d/%s", size, shape),
				tableSortLog(size, shape))
		}
	}
	/* The pivot, one way at a time.  A comparison function that says
	   "a is the pivot" -- whatever the pivot turns out to be -- makes
	   the two "invalid order function for sorting" checks reachable,
	   and they are two different checks: the one in the loop that
	   walks up from lo fires when every element it meets says it is
	   less than the pivot, and the one in the loop that walks down
	   from up fires when the pivot says it is less than every element
	   it meets, which needs the whole array to be on the far side of
	   it.  Only one value in the array is the pivot, so the rest of
	   these are the cases where the claim is false, and a sort that
	   walks the pairs in a different order reaches a different check
	   or none. */
	for _, size := range []int{5, 8, 10} {
		for _, shape := range []string{"asc", "desc", "shuf"} {
			for v := 1; v <= size; v++ {
				emit(fmt.Sprintf("sortpivot/%d/%s/%d", size, shape, v),
					tableSortWith(size, "return a == "+itoa(v)))
			}
		}
	}
	/* And the same claim about a pivot, on an array that holds the
	   value twice.  The claim "a is the pivot" is a constant inside
	   each of the two loops -- it does not depend on the other
	   element at all -- so the loop that walks down from up sees it
	   as true for every element it meets and walks off the end,
	   which is the other of the two checks.  It only holds when the
	   pivot really is that value, and the pivot is the element the
	   algorithm lands on, which is why the value has to be in two
	   places at once: the one it is picked from, and the one it is
	   swapped into. */
	for _, size := range []int{5, 8, 10} {
		mid := (1 + size) / 2
		for _, v := range []int{2, 4, 6} {
			for _, d := range []struct {
				name   string
				p1, p2 int
			}{
				{"1+mid", 1, mid}, {"mid+up", mid, size}, {"1+up", 1, size},
				{"2+mid", 2, mid}, {"1+2", 1, 2},
			} {
				emit(fmt.Sprintf("sortdup/%d/%d/%s", size, v, d.name),
					tableSortDup(size, v, d.p1, d.p2))
			}
		}
	}
	// and the comparison functions that are not orders at all: they
	// are the ones that turn into "invalid order function for
	// sorting", and when they do is up to the pair that was compared.
	for _, cmp := range []string{"true", "false", "nil", "1", `"x"`, "0"} {
		for _, size := range []int{3, 5, 10} {
			emit(fmt.Sprintf("sortcmp/%s/%d", cmp, size),
				tableSortWith(size, "return "+cmp))
		}
	}
	for _, size := range []int{3, 5, 10} {
		emit(fmt.Sprintf("sortcmp/sub/%d", size),
			tableSortWith(size, "return a - b"))
		emit(fmt.Sprintf("sortcmp/err/%d", size),
			tableSortWith(size, "error(\"boom\")"))
	}
	// one that raises after a while, which leaves the array sorted as
	// far as it got
	for k := 1; k <= 6; k++ {
		emit(fmt.Sprintf("sorterr/%d", k), tableSortErrAfter(10, k))
	}
	// and one that moves the array about while it is being sorted
	emit("sortmut/10", `C(function()
  local t = {}
  for i = 1, 10 do t[i] = i end
  table.sort(t, function(a, b) t[11] = a; return a < b end)
  return dump(t)
end)`)
	emit("sort/nan", `C(function()
  local t = {1, 0/0, 2}
  table.sort(t)
  return dump(t)
end)`)
	emit("sort/mixed", `C(function()
  local t = {1, "a", 2}
  table.sort(t)
  return dump(t)
end)`)
	emit("sort/comp-not-func", "C(table.sort, {1,2,3}, 5)")
	emit("sort/comp-nil", `C(function()
  local t = {1,2,3}
  table.sort(t, nil)
  return dump(t)
end)`)
	emit("sort/bool", `C(function()
  local t = {true, false, true}
  table.sort(t)
  return dump(t)
end)`)
	emit("sort/100", `C(function()
  local t = {}
  for i = 1, 100 do t[i] = (i * 7919) % 100 + 1 end
  table.sort(t)
  return dump(t)
end)`)
	/* A length that is not the length of anything.  'sort' asks for
	   the length first and refuses it there, before it looks at a
	   single element, and a '__len' can answer with a number that
	   belongs to no array -- which is the only way to reach "array
	   too big": the array itself cannot be built, but the number can
	   be claimed.  What is asked is the refusal and, for the lengths
	   that are small enough to be let through, whatever the sort
	   reaches when it looks for elements that are not there.  The
	   length of INT_MAX is refused too, because the check is '<'. */
	for _, n := range []string{"math.maxinteger", "2147483647", "2147483648",
		"100000000", "200000", "100000", "-1", "0", "1", "2", "3",
		"2^70", "1.5"} {
		emit(fmt.Sprintf("sortlen/%s", n), `C(function()
  local t = setmetatable({}, {__len = function() return `+n+` end})
  table.sort(t)
  return dump(t)
end)`)
	}
	// and one on a userdata, which is the kind of value 'checktab'
	// has to be asked about before the length is even looked at
	for _, n := range []string{"math.maxinteger", "3"} {
		emit(fmt.Sprintf("sortlen/userdata/%s", n), `C(function()
  local u = io.tmpfile()
  debug.setmetatable(u, {__index = function(_, k) return k end,
                         __newindex = function() end,
                         __len = function() return `+n+` end})
  table.sort(u)
  return "ok"
end)`)
	}
	nSort := took()

	// 6. things that are not tables but answer the three metamethods,
	//    which is what makes 'checktab' let them through -- and which
	//    three it asks for is each function's own business: 'concat'
	//    asks for '__index' and '__len', 'move' asks for '__index' on
	//    the source and '__newindex' on the destination, and 'unpack'
	//    asks for none of them and then fails at whatever it reaches
	//    first.  They have to be userdata and not tables: a table with
	//    a metatable is a table, and 'checktab' asks nothing of it.
	//    The userdata is a file handle with a metatable put on it,
	//    which is the only kind of value a script can make that is
	//    not a table and can still be given one.
	for _, c := range tableMetaCombos {
		u := "U(" + mtBody(c, false) + ")"
		emit("uobj-concat/"+c.name, "C(table.concat, "+u+", \",\")")
		emit("uobj-insert/"+c.name, "C(table.insert, "+u+", 99)")
		emit("uobj-remove/"+c.name, "C(table.remove, "+u+", 1)")
		emit("uobj-move/"+c.name, "C(table.move, "+u+", 1, 1, 1)")
		emit("uobj-unpack/"+c.name, "C(table.unpack, "+u+", 1, 1)")
		emit("uobj-sort/"+c.name, "C(table.sort, "+u+")")
		/* and the same object as the destination of a 'move', with a
		   real table behind the two metamethods: that is the only
		   way to see from outside which keys it wrote and in what
		   order. */
		emit("uobj-move-to/"+c.name,
			"(function() local sh = {}; local u = io.tmpfile(); "+
				"debug.setmetatable(u, "+mtBody(c, true)+"); "+
				"local ok, e = pcall(table.move, {1,2,3}, 1, 3, 1, u); "+
				"if not ok then return \"raise: \" .. tostring(e) end; "+
				"return dump(sh) end)()")
		emit("uobj-move-from/"+c.name,
			"(function() local u = "+u+"; local o = {}; "+
				"local ok, e = pcall(table.move, u, 1, 3, 1, o); "+
				"if not ok then return \"raise: \" .. tostring(e) end; "+
				"return dump(o) end)()")
	}
	nUser := took()
	// and the same three metamethods on a thing that is a table
	// already, which is the other question: 'checktab' lets it through
	// without asking anything, and what it is asked afterwards is the
	// metamethods themselves.
	for _, mt := range []struct{ name, body string }{
		{"index", "{__index = {}}"},
		{"len", "{__len = function() return 2 end}"},
		{"index+len", "{__index = {[1]=\"a\",[2]=\"b\"}, __len = function() return 2 end}"},
		{"newindex", "{__newindex = function() end}"},
		{"all", "{__index = {[1]=\"a\",[2]=\"b\"}, __newindex = function() end, __len = function() return 2 end}"},
	} {
		o := "setmetatable({}, " + mt.body + ")"
		emit("mtab-concat/"+mt.name, "C(table.concat, "+o+", \",\")")
		emit("mtab-insert/"+mt.name, "C(table.insert, "+o+", 99)")
		emit("mtab-remove/"+mt.name, "C(table.remove, "+o+", 1)")
		emit("mtab-move/"+mt.name, "C(table.move, "+o+", 1, 1, 1)")
		emit("mtab-unpack/"+mt.name, "C(table.unpack, "+o+", 1, 1)")
		emit("mtab-sort/"+mt.name, "C(table.sort, "+o+")")
	}
	nTab := took()

	// 7. and every function asked about something that is not a table
	//    and does not pretend to be one
	for k, kind := range tableKinds {
		emit("wrong-concat/"+tableKindNames[k], "C(table.concat, "+kind+", \",\")")
		emit("wrong-insert/"+tableKindNames[k], "C(table.insert, "+kind+", 1)")
		emit("wrong-remove/"+tableKindNames[k], "C(table.remove, "+kind+")")
		emit("wrong-move/"+tableKindNames[k], "C(table.move, "+kind+", 1, 1, 1)")
		emit("wrong-unpack/"+tableKindNames[k], "C(table.unpack, "+kind+", 1, 1)")
		emit("wrong-sort/"+tableKindNames[k], "C(table.sort, "+kind+")")
	}
	nWrong := took()

	cases := n
	if os.Getenv("TABLEFUZZ_DUMP") != "" {
		os.WriteFile(os.Getenv("TABLEFUZZ_DUMP"), []byte(b.String()), 0o644)
	}
	b.WriteString(`print("count", ` + strconv.Itoa(cases) + ")\n")

	base := t.TempDir()
	dirs := map[string]string{
		"ref":  filepath.Join(base, "ref"),
		"ours": filepath.Join(base, "ours"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "tablefuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ours := diffBuild(t)
	want := tableRun(t, ref, dirs["ref"])
	got := tableRun(t, ours, dirs["ours"])
	for _, side := range []struct{ name, out string }{
		{"PUC-Lua", want}, {"this", got},
	} {
		if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
			t.Fatalf("the table chunk did not run to the end in %s:\n%s",
				side.name, side.out)
		}
	}
	w, g := diffNormalise(want), diffNormalise(got)
	if w == g {
		t.Logf("%d move, %d of the ends a position can be, %d concat, "+
			"%d unpack and pack, %d sort, %d userdata and %d tables "+
			"that behave like one and %d that do not: %d cases, all of "+
			"them PUC-Lua's", nMove, nEnds, nConcat, nUnpack, nSort,
			nUser, nTab, nWrong, cases)
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	reported := 0
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var a, c string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			c = gl[i]
		}
		if a == c {
			continue
		}
		reported++
		if reported > 20 {
			t.Logf("... and more")
			break
		}
		t.Errorf("line %d\n  PUC-Lua: %s\n  this:    %s", i+1, a, c)
	}
	if reported == 0 {
		t.Errorf("the lines differ but none of them line up")
	}
}

// tableSortLog sorts an array with a comparison function that writes
// down every pair it is asked about, and answers with the tally and
// with the array afterwards.  Two quicksorts that agree on the result
// can still disagree on the tally, and that is what this is for.
func tableSortLog(size int, shape string) string {
	build := "local t = {}; for i = 1, " + itoa(size) + " do t[i] = "
	switch shape {
	case "asc":
		build += "i"
	case "desc":
		build += itoa(size) + " + 1 - i"
	case "same":
		build += "7"
	default:
		build += "(i * 7919) % " + itoa(size) + " + 1"
	}
	build += " end"
	return "(function() " + build + "; local log = {}; " +
		"local ok, e = pcall(table.sort, t, function(a, b) " +
		"log[#log+1] = a .. \"<\" .. b; return a < b end); " +
		"if not ok then return \"raise: \" .. tostring(e) end; " +
		"return table.concat(log, \" \") .. \"|\" .. dump(t) end)()"
}

// tableSortDup sorts an array that holds one value twice with a
// comparison function that claims its first argument is that value,
// which is a claim that is equally true or false whatever it is
// compared against.
func tableSortDup(size, v, p1, p2 int) string {
	build := "local t = {}; for i = 1, " + itoa(size) +
		" do t[i] = i * 2 end; t[" + itoa(p1) + "] = " + itoa(v) +
		"; t[" + itoa(p2) + "] = " + itoa(v)
	return "(function() " + build + "; local ok, e = pcall(table.sort, t, " +
		"function(a, b) return a == " + itoa(v) + " end); " +
		"if not ok then return \"raise: \" .. tostring(e) end; " +
		"return dump(t) end)()"
}

// tableSortWith sorts an array with a comparison function that is not
// an order, and answers with the array and with whatever it raised.
func tableSortWith(size int, body string) string {
	build := "local t = {}; for i = 1, " + itoa(size) +
		" do t[i] = (i * 7919) % " + itoa(size) + " + 1 end"
	return "(function() " + build + "; local ok, e = pcall(table.sort, t, " +
		"function(a, b) " + body + " end); " +
		"if not ok then return \"raise: \" .. tostring(e) end; " +
		"return dump(t) end)()"
}

// tableSortErrAfter sorts an array with a comparison function that
// raises once it has been asked a number of times, which leaves the
// array sorted as far as the sort had got.
func tableSortErrAfter(size, after int) string {
	build := "local t = {}; for i = 1, " + itoa(size) + " do t[i] = i end"
	return "(function() " + build + "; local k = 0; " +
		"local ok, e = pcall(table.sort, t, function(a, b) " +
		"k = k + 1; if k >= " + itoa(after) + " then error(\"boom\") end; " +
		"return a < b end); " +
		"if not ok then return \"raise: \" .. tostring(e) end; " +
		"return dump(t) end)()"
}

// TestTableInProcess is the six functions asked in this process and not
// in a subprocess.
//
// A subprocess does not answer 'go test -cover', which is how
// 'tableCheckField', 'tableIncr', 'tableRandomizePivot' and
// 'tableChoosePivot' were found sitting at 0% and 'tableCheckTab' at
// 20%: every case that entered them was a case in another process.
// What they answer is PUC-Lua's and the grid above is what says so;
// this is only that they are entered, and that the answers are pinned
// where no reference interpreter is needed to see them.
//
// Why the four were at 0% is two reasons, not one: the two metamethod
// ones need a userdata, which the grid makes but only in a subprocess,
// and the two quicksort ones need an array big enough to have a badly
// balanced partition.  What is asserted about that array is only that
// it comes out sorted, because the pairs the sort compared are the
// clock's (§6.18) and no test can hold them.  Every function of
// 'ltablib.go' is at 100% after this.
//
// And two things it asks that the grid cannot ask outside a
// subprocess: the two places 'partition' says a comparison function is
// not an order.  Both of them say the same thing, so what holds them
// apart is the number of pairs the comparison function was asked
// about before it gave up — ten and twenty-six — and that too is
// PUC-Lua's.
func TestTableInProcess(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		-- 1. What the probe found: a default argument that C does
		--    not evaluate.  With the third argument given, PUC-Lua
		--    never gets as far as the length.
		local ok, e = pcall(table.unpack, nil, 1, 3)
		assert(not ok and e == "attempt to index a nil value", e)
		ok, e = pcall(table.unpack, nil)
		assert(not ok and e == "attempt to get length of a nil value", e)

		-- 2. 'table.move' copies backwards where the range it is
		--    copying overlaps the place it is going, and forwards
		--    where it does not.
		local a = {10,20,30,40}
		table.move(a, 1, 3, 2)
		assert(a[1]==10 and a[2]==10 and a[3]==20 and a[4]==30, "backwards")
		local b = {10,20,30,40}
		table.move(b, 2, 4, 1)
		assert(b[1]==20 and b[2]==30 and b[3]==40 and b[4]==40, "forwards")
		-- its two refusals, which are both before the copy begins
		ok, e = pcall(table.move, {}, 0, math.maxinteger, 1)
		assert(not ok and e ==
		  "bad argument #3 to 'table.move' (too many elements to move)", e)
		ok, e = pcall(table.move, {1,2,3}, 1, 3, math.maxinteger-1)
		assert(not ok and e ==
		  "bad argument #4 to 'table.move' (destination wrap around)", e)

		-- 3. 'remove' and 'insert' accept the same positions --
		--    one to the size plus one, and not one further.  What
		--    they do with an argument they were not asked for is
		--    where they part: 'remove' ignores it and 'insert'
		--    refuses it, and the refusal names the function as
		--    the C source spells it, not as C found it.
		local t = {10,20,30}
		assert(table.remove(t) == 30 and #t == 2)
		assert(table.remove(t, 2) == 20 and #t == 1)
		assert(table.remove(t, 2) == nil and #t == 1)
		ok, e = pcall(table.remove, t, 3)
		assert(not ok and e ==
		  "bad argument #1 to 'table.remove' (position out of bounds)", e)
		t = {10,20,30}
		assert(table.remove(t, 1, 99) == 10 and #t == 2)
		assert(t[1] == 20 and t[2] == 30, "what a third argument leaves")
		t = {10,20,30}
		table.insert(t, 99)
		assert(#t == 4 and t[4] == 99)
		table.insert(t, 2, 98)
		assert(#t == 5 and t[2] == 98 and t[3] == 20)
		local s = {1,2,3,4,5}
		ok, e = pcall(table.insert, s, 6, 6)
		assert(ok and #s == 6 and s[6] == 6, "the size plus one is in")
		ok, e = pcall(table.insert, s, 8, 8)
		assert(not ok and e ==
		  "bad argument #2 to 'table.insert' (position out of bounds)", e)
		ok, e = pcall(table.remove, {1,2,3,4,5}, 6)
		assert(ok and e == nil, "and so it is for 'remove'")
		ok, e = pcall(table.remove, {1,2,3,4,5}, 7)
		assert(not ok and e ==
		  "bad argument #1 to 'table.remove' (position out of bounds)", e)
		ok, e = pcall(table.insert, {}, 1, 1, 1)
		assert(not ok and e == "wrong number of arguments to 'insert'", e)

		-- 4. 'concat' names the kind of the value it refuses, and
		--    the index it found it at.
		ok, e = pcall(table.concat, {true}, ",")
		assert(not ok and e == "invalid value (boolean) at index 1 in table for 'concat'", e)
		ok, e = pcall(table.concat, {"a", {}}, ",")
		assert(not ok and e == "invalid value (table) at index 2 in table for 'concat'", e)
		assert(table.concat({1,2}, ",") == "1,2")
		assert(table.concat({"a","b"}, 1) == "a1b")
		ok, e = pcall(table.concat, {"a","b"}, print)
		assert(not ok and e ==
		  "bad argument #2 to 'table.concat' (string expected, got function)", e)

		-- 5. 'unpack' and 'pack'
		assert(select("#", table.unpack({1,2,3}, 1, 3)) == 3)
		assert(select("#", table.unpack({1,2,3}, 2, 3)) == 2)
		assert(select("#", table.unpack({1,2,3}, 3, 2)) == 0)
		ok, e = pcall(table.unpack, {}, 1, 1000000000)
		assert(not ok and e == "too many results to unpack", e)
		assert(table.pack(1,2,3).n == 3)
		assert(table.pack().n == 0)

		-- 6. 'table.sort' in an array of 128 or fewer elements
		--    compares a fixed sequence of pairs, so it can be held.
		--    The pairs below are PUC-Lua's own, and they include
		--    '9<9': the nine is the pivot and it is compared with
		--    itself, which is what the first loop of 'partition'
		--    does when every element before the nine is less than
		--    it -- it stops at the pivot, not before it.
		local t = {}
		for i = 1, 10 do t[i] = (i * 7919) % 10 + 1 end
		local log = {}
		table.sort(t, function(a, b)
			log[#log+1] = a .. "<" .. b
			return a < b
		end)
		assert(table.concat(log, " ") ==
		  "1<10 6<1 10<6 9<6 6<3 8<6 6<4 7<6 6<5 2<6 7<6 6<2 10<8 9<8 "..
		  "10<9 7<9 9<9 9<7 7<8 2<1 4<1 2<4 3<2 2<5 2<3 2<1 4<5 3<4",
		  table.concat(log, " "))
		assert(#log == 28, #log)
		assert(table.concat(t, ",") == "1,2,3,4,5,6,7,8,9,10", "sorted")

		-- 7. An array big enough to have a badly balanced partition,
		--    which is the only way in to the two functions that are
		--    about the randomization.  What is asserted is only that
		--    it comes out sorted: the pairs it compared are the
		--    clock's (§6.18).
		for _, shape in ipairs{"asc", "desc", "same", "shuf"} do
		  local big = {}
		  for i = 1, 3000 do
		    if shape == "asc" then big[i] = i
		    elseif shape == "desc" then big[i] = 3001 - i
		    elseif shape == "same" then big[i] = 7
		    else big[i] = (i * 7919) % 3000 + 1 end
		  end
		  table.sort(big)
		  for i = 2, 3000 do assert(big[i-1] <= big[i], shape) end
		end

		-- 8. The two places 'partition' says that the comparison
		--    function is not an order, and they are two different
		--    places.  The first is what a function that says
		--    everything is less than everything walks into, at the
		--    top of its way up.  The second is harder to reach: it
		--    needs a function whose answer does not depend on the
		--    element it is given -- 'a == 4' -- and an array that
		--    holds four in two places at once, because the pivot has
		--    been moved out of the position it was picked from.  In
		--    this process the two are 0% until something asks them:
		--    the grid asks them 45 times and in another process.
		--    Both of them say the same thing, so the message alone
		--    cannot tell the two places apart: how many pairs the
		--    comparison function was asked about before it gave up
		--    can, and it is PUC-Lua's too.
		local k = 0
		ok, e = pcall(table.sort, {1,2,3,4,5,6,7,8,9,10},
		              function() k = k + 1; return true end)
		assert(not ok and e == "invalid order function for sorting" and
		  k == 10, tostring(e) .. " in " .. k .. " calls")
		local d = {}
		for i = 1, 10 do d[i] = i * 2 end
		d[5] = 4
		d[6] = 4 -- there are two of them now
		k = 0
		ok, e = pcall(table.sort, d, function(a) k = k + 1; return a == 4 end)
		assert(not ok and e == "invalid order function for sorting" and
		  k == 26, tostring(e) .. " in " .. k .. " calls")

		-- 9. And a userdata, which is the only kind of value that
		--    is not a table and can be asked the three metamethods:
		--    that is what 'checktab' is for.
		local u = io.tmpfile()
		debug.setmetatable(u, {__index = function() return 42 end,
		                       __len = function() return 2 end})
		assert(table.concat(u, ",") == "42,42")
		local v = io.tmpfile()
		debug.setmetatable(v, {__len = function() return 2 end})
		ok, e = pcall(table.concat, v, ",")
		assert(not ok and e ==
		  "bad argument #1 to 'table.concat' (table expected, got userdata)", e)
		local w = io.tmpfile()
		debug.setmetatable(w, {__newindex = function() end})
		ok, e = pcall(table.move, {1,2,3}, 1, 3, 1, w)
		assert(ok, e)
		local x = io.tmpfile()
		debug.setmetatable(x, {})
		ok, e = pcall(table.move, {1,2,3}, 1, 3, 1, x)
		assert(not ok and e ==
		  "bad argument #5 to 'table.move' (table expected, got userdata)", e)

		-- 10. A length that is not the length of anything.  'sort'
		--     asks for the length before it looks at a single element,
		--     and a '__len' can answer it with a number that belongs
		--     to no array -- which is the only road to "array too
		--     big": the array it names cannot be built, but the
		--     number can be claimed, and the refusal is at the '<'
		--     that stops it, so INT_MAX itself is refused too.
		local function len(n)
		  return setmetatable({}, {__len = function() return n end})
		end
		ok, e = pcall(table.sort, len(math.maxinteger))
		assert(not ok and e ==
		  "bad argument #1 to 'table.sort' (array too big)", e)
		ok, e = pcall(table.sort, len(2147483647))
		assert(not ok and e ==
		  "bad argument #1 to 'table.sort' (array too big)", e)
		-- the lengths that are let through reach whatever is there,
		-- which is nothing.  One of them is a hundred thousand past
		-- the smallest length a mutation of that refusal could draw
		-- the line at, so that moving the line is seen.
		ok, e = pcall(table.sort, len(2))
		assert(not ok and e == "attempt to compare two nil values", e)
		ok, e = pcall(table.sort, len(200000))
		assert(not ok and e == "attempt to compare two nil values", e)
		-- and one that is not a number of elements at all
		ok, e = pcall(table.sort, len(1.5))
		assert(not ok and e == "object length is not an integer", e)
		-- the same length on a userdata, which 'checktab' has to let
		-- through before the length is even looked at
		local y = io.tmpfile()
		debug.setmetatable(y, {__index = function(_, k) return k end,
		                       __newindex = function() end,
		                       __len = function() return math.maxinteger end})
		ok, e = pcall(table.sort, y)
		assert(not ok and e ==
		  "bad argument #1 to 'table.sort' (array too big)", e)
		return true
	`); err != nil {
		t.Fatal(err)
	}
	if !L.ToBoolean(-1) {
		t.Fatal("the table library does not behave like PUC-Lua's")
	}
}

// tableRun runs the chunk, the way the reference was invoked.
func tableRun(t *testing.T, bin, dir string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "tablefuzz.lua"},
		Dir:  dir,
		Env:  diffEnv(),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func itoa(i int) string { return strconv.Itoa(i) }
