package lua

import (
	"strings"
	"testing"
)

// TestExpAtTheEdgeOfTheFloatRange pins the answers at the top of the
// range, where exp overflows but half of it -- which is what 'sinh' and
// 'cosh' tend to -- still does not.
//
// The values are PUC-Lua's.  They are here because Go's math.Exp is not
// one function but a hand-written assembly version per architecture, and
// they disagree: on amd64 math.Exp(709.5) answers +Inf, and 709.5 is
// well inside the range -- exp(709.5) is 1.3549863193146328e+308.  So
// 'math.exp' and 'math.sinh' have to be worked out without asking Go's
// exp about an argument that large, which is what mathExpScaled does.
// On arm64 this test passes either way; on amd64 it is the only thing
// here that would catch the port answering inf.
func TestExpAtTheEdgeOfTheFloatRange(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	local function say(what, x)
		out[#out + 1] = string.format("%s(%s) = %s", what, x,
			string.format("%.17g", math[what](x)))
	end
	for _, x in ipairs{709, 709.5, 709.78, 709.79, 710, 710.5, 800} do
		say("exp", x)
		say("sinh", x)
		say("cosh", x)
	end
	for _, x in ipairs{-709.5, -710, -800} do
		say("sinh", x)
		say("cosh", x)
	end
	return table.concat(out, "\n")
	`)
	want := `exp(709) = 8.2184074615549724e+307
sinh(709) = 4.1092037307774862e+307
cosh(709) = 4.1092037307774862e+307
exp(709.5) = 1.3549863193146328e+308
sinh(709.5) = 6.7749315965731642e+307
cosh(709.5) = 6.7749315965731642e+307
exp(709.78) = 1.7928227943945155e+308
sinh(709.78) = 8.9641139719725777e+307
cosh(709.78) = 8.9641139719725777e+307
exp(709.79) = inf
sinh(709.79) = 9.0542048151523451e+307
cosh(709.79) = 9.0542048151523451e+307
exp(710) = inf
sinh(710) = 1.1169973830808555e+308
cosh(710) = 1.1169973830808555e+308
exp(710.5) = inf
sinh(710.5) = inf
cosh(710.5) = inf
exp(800) = inf
sinh(800) = inf
cosh(800) = inf
sinh(-709.5) = -6.7749315965731642e+307
cosh(-709.5) = 6.7749315965731642e+307
sinh(-710) = -1.1169973830808555e+308
cosh(-710) = 1.1169973830808555e+308
sinh(-800) = -inf
cosh(-800) = inf`
	if got != want {
		t.Errorf("math.exp/sinh/cosh at the edge of the range:\n%s", sideBySide(want, got))
	}
}

// TestLogOfASubnormal pins the answers at the bottom of the range,
// where a float64 has stopped carrying a full significand.
//
// The values are PUC-Lua's, and they are here for the same reason the
// exp ones are: Go's math.Log is an assembly routine on amd64 and s390x,
// and it loses several digits on a subnormal argument -- 1e-308 is one,
// the smallest normal float64 being 2.2250738585072014e-308 -- answering
// -708.71839997088 where log(1e-308) is -709.19620864217.  On arm64 this
// test passes either way; on amd64 it is the only thing here that would
// catch the port answering that.
func TestLogOfASubnormal(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	for _, x in ipairs{1e-308, 1e-320, 5e-324, 2.2250738585072014e-308} do
		out[#out + 1] = string.format("%.14g\t%.14g", math.log(x), math.log10(x))
	end
	for _, x in ipairs{1e-308, 5e-324} do
		out[#out + 1] = string.format("%.14g\t%.14g\t%.14g",
			math.log(x, 2), math.log(x, 10), math.log(x, 3))
	end
	return table.concat(out, "\n")
	`)
	want := `-709.19620864217	-308
-736.82724089097	-320.00000483495
-744.44007192138	-323.30621534312
-708.39641853226	-307.65265556859
-1023.1538532253	-308	-645.53820848113
-1074	-323.30621534312	-677.61855533575`
	if got != want {
		t.Errorf("math.log/math.log10 of a subnormal:\n%s", sideBySide(want, got))
	}
}

// sideBySide shows which lines of a multi-line answer disagree.
func sideBySide(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	out := ""
	for i := 0; i < len(w) || i < len(g); i++ {
		var a, b string
		if i < len(w) {
			a = w[i]
		}
		if i < len(g) {
			b = g[i]
		}
		if a != b {
			out += "  PUC-Lua: " + a + "\n  this:    " + b + "\n"
		}
	}
	return out
}
