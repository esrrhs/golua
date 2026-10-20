package core

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestExpAtTheEdgeOfTheFloatRange pins the answers at the top of the
// range, where exp overflows but half of it -- which is what 'sinh' and
// 'cosh' tend to -- still does not.
//
// The values below are the Go approximation (mathExpScaled), which is
// what a build without cgo answers.  A build with cgo calls the host
// libm, and glibc's sinh(710) is one ulp above that approximation; both
// are finite, and exp(710) is still +Inf.  The exact bit is the host's,
// so that case only checks the overflow boundary.
//
// They are here because Go's math.Exp is not one function but a
// hand-written assembly version per architecture, and they disagree: on
// amd64 math.Exp(709.5) answers +Inf, and 709.5 is well inside the range
// -- exp(709.5) is 1.3549863193146328e+308.  So without cgo, 'math.exp'
// and 'math.sinh' have to be worked out without asking Go's exp about an
// argument that large, which is what mathExpScaled does.  On arm64 this
// test passes either way; on amd64 it is the only thing here that would
// catch the port answering inf.
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
	if haveLibm {
		got = acceptHostSinh710(t, got)
	}
	if got != want {
		t.Errorf("math.exp/sinh/cosh at the edge of the range:\n%s", sideBySide(want, got))
	}
}

// acceptHostSinh710 checks the four sinh/cosh(±710) lines and then
// rewrites them to the Go-approximation golden.  Host libm and that
// approximation differ by an ulp; both must stay finite and past 1e308.
func acceptHostSinh710(t *testing.T, got string) string {
	t.Helper()
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		var sign string
		switch {
		case strings.HasPrefix(line, "sinh(710) = "), strings.HasPrefix(line, "cosh(710) = "), strings.HasPrefix(line, "cosh(-710) = "):
			sign = "+"
		case strings.HasPrefix(line, "sinh(-710) = "):
			sign = "-"
		default:
			continue
		}
		eq := strings.Index(line, " = ")
		v, err := strconv.ParseFloat(line[eq+3:], 64)
		if err != nil || math.IsInf(v, 0) || math.IsNaN(v) || math.Abs(v) < 1e308 {
			t.Errorf("sinh/cosh(±710) should be a finite value past 1e308, got %q", line)
			continue
		}
		if sign == "+" && v < 0 || sign == "-" && v > 0 {
			t.Errorf("sinh/cosh(±710) has the wrong sign: %q", line)
		}
		lines[i] = line[:eq+3] + "1.1169973830808555e+308"
		if sign == "-" {
			lines[i] = line[:eq+3] + "-1.1169973830808555e+308"
		}
	}
	return strings.Join(lines, "\n")
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

// TestLog2NearOne pins math.log(x, 2) where x is close to 1.
//
// The values are PUC-Lua's.  They are here because Go's math.Log2 is not
// C's log2: it splits the argument with Frexp and adds the exponent back
// afterwards, and both of those round, so an answer that is near zero --
// which is what log2 of a number near 1 is -- comes out of the rounding
// of two numbers a million times bigger than it is: math.Log2 answers
// 3.6816598061451e-16 for log(1.0000000000000002, 2), fifteen per cent
// away from the 3.2034265038149e-16 that C's log2 answers.  Outside this
// band the two agree to an ulp.  mathLog2Scaled is what keeps those
// bits, and this is the only thing here that would catch the port asking
// math.Log2 about an argument that near 1.
//
// The fourteen digits are deliberate.  Every answer here is within an
// ulp or two of the reference on any platform, and %.14g is what keeps
// that from being a failure on a machine whose libm rounds the other
// way; it still shows a fifteen per cent error as plainly as %.17g does.
func TestLog2NearOne(t *testing.T) {
	got := runChunk(t, `
	local out = {}
	for _, x in ipairs{
		1 + 2^-52, 1 - 2^-53, 1 + 1e-15, 1 - 1e-15, 1 + 1e-10, 1 - 1e-10,
		1 + 0.1, 1 - 0.1, 1 + 0.41421356237309503, 0.7071067811865476,
		1.4142135623730951, 2, 4, 8, 1024, 0.5, 2^40, 2^-30,
		3, 10, 100, 1e10, 1e100, 1e-10, 5e-324, 1e-320,
	} do
		out[#out + 1] = string.format("%.14g", math.log(x, 2))
	end
	return table.concat(out, "\n")
	`)
	want := `3.2034265038149e-16
-1.6017132519075e-16
1.6017132519075e-15
-1.4415419267167e-15
1.442695160186e-10
-1.4426951603302e-10
0.13750352374994
-0.15200309344505
0.5
-0.5
0.5
1
2
3
10
-1
40
-30
1.5849625007212
3.3219280948874
6.6438561897747
33.219280948874
332.19280948874
-33.219280948874
-1074
-1063.0170064253`
	if got != want {
		t.Errorf("math.log(x, 2) near x == 1:\n%s", sideBySide(want, got))
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
