package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverGap18VMMore(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
say(~5)
say(~(-1))
-- LOADKX: many constants
local a={"c1","c2","c3","c4","c5","c6","c7","c8","c9","c10",
"c11","c12","c13","c14","c15","c16","c17","c18","c19","c20",
"c21","c22","c23","c24","c25","c26","c27","c28","c29","c30",
"c31","c32","c33","c34","c35","c36","c37","c38","c39","c40",
"c41","c42","c43","c44","c45","c46","c47","c48","c49","c50",
"c51","c52","c53","c54","c55","c56","c57","c58","c59","c60",
"c61","c62","c63","c64","c65","c66","c67","c68","c69","c70",
"c71","c72","c73","c74","c75","c76","c77","c78","c79","c80",
"c81","c82","c83","c84","c85","c86","c87","c88","c89","c90",
"c91","c92","c93","c94","c95","c96","c97","c98","c99","c100",
"c101","c102","c103","c104","c105","c106","c107","c108","c109","c110",
"c111","c112","c113","c114","c115","c116","c117","c118","c119","c120",
"c121","c122","c123","c124","c125","c126","c127","c128","c129","c130",
"c131","c132","c133","c134","c135","c136","c137","c138","c139","c140",
"c141","c142","c143","c144","c145","c146","c147","c148","c149","c150",
"c151","c152","c153","c154","c155","c156","c157","c158","c159","c160",
"c161","c162","c163","c164","c165","c166","c167","c168","c169","c170",
"c171","c172","c173","c174","c175","c176","c177","c178","c179","c180",
"c181","c182","c183","c184","c185","c186","c187","c188","c189","c190",
"c191","c192","c193","c194","c195","c196","c197","c198","c199","c200",
"c201","c202","c203","c204","c205","c206","c207","c208","c209","c210",
"c211","c212","c213","c214","c215","c216","c217","c218","c219","c220",
"c221","c222","c223","c224","c225","c226","c227","c228","c229","c230",
"c231","c232","c233","c234","c235","c236","c237","c238","c239","c240",
"c241","c242","c243","c244","c245","c246","c247","c248","c249","c250",
"c251","c252","c253","c254","c255","c256","c257","c258","c259","c260"}
say(a[260])
-- huge constructor for SETLIST EXTRAARG
local t={1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40,41,42,43,44,45,46,47,48,49,50,51,52,53,54,55,56,57,58,59,60,61,62,63,64,65,66,67,68,69,70,71,72,73,74,75,76,77,78,79,80,81,82,83,84,85,86,87,88,89,90,91,92,93,94,95,96,97,98,99,100,101,102,103,104,105,106,107,108,109,110,111,112,113,114,115,116,117,118,119,120,121,122,123,124,125,126,127,128,129,130,131,132,133,134,135,136,137,138,139,140,141,142,143,144,145,146,147,148,149,150,151,152,153,154,155,156,157,158,159,160,161,162,163,164,165,166,167,168,169,170,171,172,173,174,175,176,177,178,179,180,181,182,183,184,185,186,187,188,189,190,191,192,193,194,195,196,197,198,199,200,201,202,203,204,205,206,207,208,209,210,211,212,213,214,215,216,217,218,219,220,221,222,223,224,225,226,227,228,229,230,231,232,233,234,235,236,237,238,239,240,241,242,243,244,245,246,247,248,249,250,251,252,253,254,255,256,257,258,259,260,261,262,263,264,265,266,267,268,269,270,271,272,273,274,275,276,277,278,279,280,281,282,283,284,285,286,287,288,289,290,291,292,293,294,295,296,297,298,299,300,301,302,303,304,305,306,307,308,309,310,311,312,313,314,315,316,317,318,319,320,321,322,323,324,325,326,327,328,329,330,331,332,333,334,335,336,337,338,339,340,341,342,343,344,345,346,347,348,349,350,351,352,353,354,355,356,357,358,359,360,361,362,363,364,365,366,367,368,369,370,371,372,373,374,375,376,377,378,379,380,381,382,383,384,385,386,387,388,389,390,391,392,393,394,395,396,397,398,399,400,401,402,403,404,405,406,407,408,409,410,411,412,413,414,415,416,417,418,419,420,421,422,423,424,425,426,427,428,429,430,431,432,433,434,435,436,437,438,439,440,441,442,443,444,445,446,447,448,449,450,451,452,453,454,455,456,457,458,459,460,461,462,463,464,465,466,467,468,469,470,471,472,473,474,475,476,477,478,479,480,481,482,483,484,485,486,487,488,489,490,491,492,493,494,495,496,497,498,499,500,501,502,503,504,505,506,507,508,509,510,511,512}
say(#t)
-- float idiv
say(7.0//2.0)
say((-7.0)//2.0)
-- mul registers
local a,b=12345,678
say(a*b)
say(a//b)
-- forlimit float non-number already; float limit error via userdata
say(not pcall(function() for i=1,io.stdin,1 do end end))
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap18LAPIDirect(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushInteger(1)
	L.PushInteger(2)
	L.PushInteger(3)
	_ = stackIndexOf(L, -1)
	_ = stackIndexOf(L, -2)
	_ = L.indexToStack(-1)
	// SetMetaTable non-table mt
	L.NewTable()
	L.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		L.SetMetaTable(-2)
	}()
	// C closure upvalue OOB via Call
	L.PushInteger(7)
	L.PushGoClosure(func(L *State) int {
		_ = index2value(L, UpvalueIndex(1))
		_ = index2value(L, UpvalueIndex(50)) // line 62
		_ = L.ToGoFunction(UpvalueIndex(1))
		return 0
	}, 1)
	L.Call(0, 0)
	// Error with non-string? push number then Error
	L.PushInteger(1)
	func() {
		defer func() { _ = recover() }()
		_ = L.Error()
	}()
	// Compare invalid
	L.PushInteger(1)
	L.PushInteger(2)
	func() {
		defer func() { _ = recover() }()
		_ = L.Compare(-1, -2, -1)
	}()
	// UpvalueJoin invalid
	func() {
		defer func() { _ = recover() }()
		L.PushGoFunc(func(L *State) int { return 0 })
		L.UpvalueJoin(-1, 1, -1, 1)
	}()
	// LoadFileMode nonexistent
	err := L.LoadFileMode("/no/golua18/missing.lua", "bt")
	_ = err
	// Where deep
	_ = L.Where(10)
	// rawGetRegistryGlobals when registry missing globals? just call
	_ = L.rawGetRegistryGlobals()
	// pcallK yieldable re-panic yieldAbort path — call with k=true that yields via panic
	co := L.NewThread()
	co.PushGoFunc(func(L *State) int {
		L.PushGoFunc(func(L *State) int {
			panic(&yieldAbort{})
		})
		func() {
			defer func() { _ = recover() }()
			_ = L.pcallK(0, 0, 0, true)
		}()
		return 0
	})
	_ = co.Resume(L, 0)
	// aux_upvalue Lua closure name nil — strip debug?
	got := runChunk(t, `
local function f(a)
  local function g() return a end
  return g
end
local g=f(1)
-- getupvalue
local n,v=debug.getupvalue(g,1)
return n,v
`)
	_ = got
}

func TestCoverGap18ConcatGC(t *testing.T) {
	// Try to get pending finalizers then concat so gcAfterAlloc returns true
	got := runChunk(t, `
collectgarbage("stop")
local bag={}
for i=1,200 do
  bag[i]=setmetatable({}, {__gc=function() end})
end
bag=nil
collectgarbage("restart")
collectgarbage("collect")
-- concat allocates
local s="a"
for i=1,50 do s=s..tostring(i) end
return #s
`)
	_ = got
}

func TestCoverGap18IORemain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "n.txt")
	_ = os.WriteFile(path, []byte("   1.25e+2\n"), 0o644)
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	L.PushString(path)
	L.SetGlobal("P")
	src := `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
local f=assert(io.open(P,"r"))
say(f:read("*n"))
f:close()
-- tmpfile fail is hard; popen write
local p=io.popen("cat >/dev/null","w")
if p then say(p:write("hi")); say(p:close()) end
-- flush closed
f=assert(io.open(P,"r")); f:close()
boom(function() f:flush() end)
-- seek whence bad
f=assert(io.open(P,"r"))
boom(function() f:seek("bogus") end)
f:close()
return table.concat(out,"|")
`
	if err := L.LoadString(src, "=io"); err != nil {
		t.Fatal(err)
	}
	if L.PCall(0, 1, 0) != LUA_OK {
		t.Fatal(L.ToString(-1))
	}
	// num reader overflow break at digits
	s := &ioStream{canR: true, rd: nopCloser{strings.NewReader(strings.Repeat("9", 250) + "\n")}}
	L2 := NewState()
	defer L2.Close()
	L2.checkStack(4)
	_ = ioReadNumber(L2, s)
}

type nopCloser struct{ *strings.Reader }

func (nopCloser) Close() error { return nil }

func TestCoverGap18OsDateDST(t *testing.T) {
	got := runChunk(t, `
local out={}
local function say(s) out[#out+1]=tostring(s) end
local function boom(f) say(not pcall(f)) end
say(os.date("%Y-%m-%d", 0))
say(os.date("!*t", 0).year)
say(os.date("*t", os.time{year=2020,month=7,day=1,hour=12}).isdst ~= nil)
say(os.time{year=2020,month=1,day=1})
boom(function() os.time{year=2020} end) -- missing fields?
boom(function() os.date("%", 0) end)
say(os.getenv("PATH") ~= nil)
say(os.getenv("GOLUA_NO_SUCH_ENV_18") == nil)
return table.concat(out,"|")
`)
	_ = got
}

func TestCoverGap18HookYieldMark(t *testing.T) {
	// line hook that yields to hit CIST_HOOKYIELD path is very hard in sync;
	// at least count+line mix
	got := runChunk(t, `
local n=0
debug.sethook(function(e)
  n=n+1
  if n>30 then debug.sethook() end
end, "l", 1)
local function f(x)
  if x>0 then return f(x-1) end
  return x
end
f(5)
debug.sethook()
return n
`)
	_ = got
}
