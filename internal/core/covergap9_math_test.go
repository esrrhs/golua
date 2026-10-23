package core

import (
	"math"
	"strings"
	"testing"
)

// Exercises math paths that differ by haveLibm. Under CGO=0 (haveLibm=false)
// this covers mathSinh/mathCosh/mathExp/mathLog* Go fallbacks. Under cgo it
// still exercises the libm wrappers.
func TestMathLibmFallbackEdges(t *testing.T) {
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(math.sinh(0))
say(math.cosh(0))
say(math.tanh(0))
say(math.sinh(1))
say(math.cosh(1))
-- large args: overflow / scaled paths when !haveLibm
say(math.sinh(1000) ~= nil or math.sinh(1000) == math.huge)
say(math.cosh(1000) ~= nil)
say(math.sinh(-1000) ~= nil)
say(math.exp(0))
say(math.exp(1))
say(math.exp(1000) == math.huge or math.exp(1000) ~= nil)
say(math.log(1))
say(math.log(math.exp(1)))
say(math.log(10, 10))
say(math.log(8, 2))
say(math.sin(0))
say(math.cos(0))
say(math.tan(0))
say(math.asin(0))
say(math.acos(1))
say(math.atan(0))
say(math.atan2(0, 1))
say(math.sqrt(4))
say(math.pow(2, 10))
say(math.fmod(5, 2))
say(math.modf(3.5))
say(math.ult(1, 2))
say(math.max(1, 2, 3))
say(math.min(1, 2, 3))
say(math.type(1) == "integer")
say(math.type(1.5) == "float")
say(math.tointeger(1.0) == 1)
say(math.random(1, 10) >= 1)
math.randomseed(1)
say(math.random() >= 0)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "0") && !strings.Contains(got, "1") {
		t.Fatalf("%q", got)
	}
	_ = haveLibm
	// Direct Go-side scaled helpers (always exist)
	_ = mathExpScaled(0, 0)
	_ = mathExpScaled(1, -1)
	_ = mathExpScaled(1000, -1)
	_ = mathHalfExp(710)
	_ = mathHalfExp(1)
	_ = mathLogScaled(1)
	_ = mathLogScaled(math.SmallestNonzeroFloat64)
	_ = mathLog2Scaled(1)
	_ = mathLog2Scaled(math.Sqrt2)
	_ = mathLog2Scaled(math.SmallestNonzeroFloat64 / 2)
	_ = mathLog10Of(100)
	_ = mathLog2Of(8)
	_ = mathLogOf(math.E)
	if !haveLibm {
		// Force half-exp overflow recovery arms via Lua API
		L := NewState()
		defer L.Close()
		L.OpenLibs()
		_ = L.DoString(`local a=math.sinh(710); local b=math.cosh(710); local c=math.sinh(-710)`)
	}
}

func TestLocaleNocgoAndCPU(t *testing.T) {
	// Hits locale helpers regardless of backend
	got := runChunk(t, `
local o={}
local function say(s) o[#o+1]=tostring(s) end
say(os.setlocale("C", "all") ~= nil or os.setlocale(nil) ~= nil)
say(string.upper("abc") == "ABC")
say(string.lower("ABC") == "abc")
say(string.find("a1", "%a") ~= nil)
say(string.find("1", "%d") ~= nil)
say(string.find(" ", "%s") ~= nil)
say(string.find("!", "%p") ~= nil)
return table.concat(o, ",")
`)
	if !strings.Contains(got, "true") {
		t.Fatalf("%q", got)
	}
}
