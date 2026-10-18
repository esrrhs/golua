package lua

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The twelfth fuzz, over 'os.date', 'os.time' and the 'strftime' they
// are a binding for.
//
// A conversion specifier is a question about an instant, and a good many
// of them are not answered by reading the instant: '%U' is the week of
// the year, which is counted from the day of the week the first of
// January was and from a day-of-the-year that C counts from zero and Go
// from one; '%V' and '%G' are the ISO week and the year that goes with
// it, which is not always the year of the instant; '%C' is the century,
// which is the year divided by a hundred and not the hundredth part of
// anything; '%j' is the day of the year in three digits; and '%z' is
// not about the instant at all (see below).  A probe of 1 643 cases
// answered 0 lines red, and by the rule of this project that is not yet
// an answer: the same corpus was asked again with the day-of-the-year
// shift in '%U' taken out and it did not notice.  It could not have:
// the fourteen instants it asked about are all ones where
// 'yday + 7 - wday' lands in the same seventh whether 'yday' counts from
// zero or from one.  The first of January 2001 is a Monday, and there
// the two answers are 00 and 01.  "Nobody asked" is not "it is right".
//
// So the grid asks every day of two years, at two times of day, in four
// zones, and then the hours and the minutes, the 19 'E' and 'O'
// modifiers, the tables 'os.time' takes, and the ends of what a
// 'struct tm' holds.  It found one thing, and the finding is the reason
// the grid no longer asks the question that found it:
//
//   Two digits that carried a minus sign.  '%y' and '%g' are the year
//   without its century, which is the remainder of a signed division by
//   a hundred, and C takes the absolute value of it before it writes
//   it two digits wide.  This port wrote what Go writes, which keeps
//   the sign, so the year -2147481748 was -48 here and 48 there.
//
//   That was the grid's first run, which asked the ends of a 'struct
//   tm' all the way down to INT_MIN.  By the rule of this project a
//   finding is not finished until the fix is taken out and the grid
//   goes red, and for this one that cannot be done: with the fix out,
//   the 2 531 cases that saw it differ are cases the two C libraries
//   themselves disagree about, so they are no longer in the grid at
//   all (see below).  The one thing a mutation test needs -- a case
//   both sides agree on the answer to -- does not exist for it.  It is
//   right by the other evidence: it is what the C source says and what
//   the reference prints, and the wrong answer was wrong against both
//   C libraries.
//
// Four things it does not ask, and they are all things the C library
// decides rather than PUC-Lua, which is why they are left out and not
// fixed (§6).  Each one is a case where the two C libraries this
// project is measured against answer differently, so no port could
// agree with both of them:
//
//   '%z' and '%Z' under '!'.  Under '!' PUC-Lua breaks the time down
//   with 'gmtime'.  glibc's 'gmtime' leaves 'tm_gmtoff' at zero and
//   'tm_zone' at "GMT", so PUC-Lua prints +0000 and GMT; Apple's '%z'
//   is taken from the C library's 'timezone' variable and not from the
//   broken-down time, so the same format prints the zone's nominal
//   offset (-0500 in New York) and UTC.  This port prints +0000 and
//   UTC.
//
//   '%z' at an instant the timezone database does not cover.  Before
//   its first transition the offset is a local mean time, and for a
//   zone with negative daylight saving, like Dublin, it is also an
//   instant whose historical standard time is not the one the current
//   rules describe: New York before 1883 is -0456 to Go and -0500 to
//   Apple's C library, and Dublin in 1901 is -0025 and +0000.  '%Z'
//   is asked there, because both C libraries name the type "LMT" or
//   "DMT" alike; only the offset is each one's own.
//
//   The year, when it is below a thousand.  A year is written with a
//   width, and the two C libraries pick different ones: for the year
//   1, '%Y' is "0001" to Apple's and "1" to glibc's, and for the year
//   -27, '%y' is "27" to one and "73" to the other.  Every instant in
//   this grid is in a year of four digits or more.
//
//   A year before 1900 in a table for 'os.time'.  macOS's 'mktime'
//   refuses one and answers -1, and glibc's does not.  The reference
//   is asked that one question first and the cases that turn on it are
//   left out of the comparison on both sides.

// dateSpecs are the conversion specifiers of C99's 'strftime', all 37
// of them counting the two 'strftime' spells of one of them.
var dateSpecs = []string{"a", "A", "b", "B", "c", "C", "d", "D", "e", "F",
	"g", "G", "h", "H", "I", "j", "m", "M", "n", "p", "r", "R", "S", "t",
	"T", "u", "U", "V", "w", "W", "x", "X", "y", "Y", "z", "Z", "%"}

// dateMods are the two-character conversion specifiers: the 'E' and 'O'
// modifiers of C99.
var dateMods = []string{"Ec", "EC", "Ex", "EX", "Ey", "EY", "Od", "Oe",
	"OH", "OI", "Om", "OM", "OS", "Ou", "OU", "OV", "Ow", "OW", "Oy"}

// dateZones are the zones the grid is run in.  One of them has no
// summer time at all, one is west of Greenwich, one east, and Dublin is
// the one whose standard time is the smaller of its two.
var dateZones = []string{"UTC", "America/New_York", "Europe/Dublin",
	"Australia/Sydney"}

// dateEnds are the instants at and past the ends of what a 'struct tm'
// holds.  The top is exactly the 'int' boundary, and the bottom is the
// 'int' boundary of a 32-bit 'time_t' -- which is the year 1901, and
// is the oldest instant every C library of this grid agrees about (see
// the note at the top).
var dateEnds = []int64{67768036191676799, 67768036191676800, 2147483647,
	2147483648, 253402300799, 253402300800, 0, 1, -1, -2147483648,
	-2147483649, 1 << 40}

// dateChunk is the harness.  Every case prints one line, and a case
// that raises is a line like any other rather than the end of the run.
var dateChunk = `
local n = 0
local function T(...)
  local t = {}
  for k = 1, select("#", ...) do t[k] = q((select(k, ...))) end
  return table.concat(t, "\t")
end
local function P(tag, ...)
  n = n + 1
  print(n .. "\t" .. tag .. "\t" .. T(...))
end
-- one instant asked every question in a list, in one line
local function one(t, utc, list)
  local pre = utc and "!" or ""
  local p = {}
  for i, s in ipairs(list) do
    local ok, v = pcall(os.date, pre .. "%" .. s, t)
    if ok then p[i] = tostring(v) else p[i] = "<raise>" end
  end
  return table.concat(p, "|")
end
local function ALL(t) return one(t, false, SPECS) end
local function ALLE(t) return one(t, false, SPECS_EDGE) end
local function UT(t) return one(t, true, SPECS_UTC) end
local function MOD(t) return one(t, false, MODS) end
local function UTOD(t) return one(t, true, TIMES) end
local function TOD(t) return one(t, false, TIMES) end

-- the fields of '*t', which is the same instant asked a different way
local function star(t, utc)
  local ok, d = pcall(os.date, (utc and "!" or "") .. "*t", t)
  if not ok then return "<raise>" end
  local ks = {"year", "month", "day", "hour", "min", "sec", "wday",
              "yday", "isdst"}
  local p = {}
  for i, k in ipairs(ks) do p[i] = k .. "=" .. tostring(d[k]) end
  return table.concat(p, ",")
end

-- a format that is not a conversion specifier at all.  The message of
-- a refused one is part of the answer: it is where the '%s' of C's
-- 'lua_pushfstring' is 'strlen' and not the whole format.
local function fmt(t, s)
  local ok, v = pcall(os.date, s, t)
  if ok then return v end
  return "raise: " .. tostring(v)
end

-- a missing format, which is the default one.  'os.date()' with no
-- argument asks for the time now, which the two interpreters do not
-- start at the same second for, so the nil is handed over with an
-- instant instead.
local function def(t)
  local ok, v = pcall(os.date, nil, t)
  if ok then return v end
  return "raise: " .. tostring(v)
end

-- 'os.time', which is the same table going the other way
local function tm(tab)
  local ok, v = pcall(os.time, tab)
  if not ok then return "<raise>" end
  return tostring(v)
end
`

// dateSubList is a copy of a specifier list with one or two of them
// taken out.  Which ones are taken out, and why, is written at the top
// of this file.
func dateSubList(ss []string, drop ...string) []string {
	out := []string{}
	for _, s := range ss {
		kept := true
		for _, d := range drop {
			if s == d {
				kept = false
			}
		}
		if kept {
			out = append(out, s)
		}
	}
	return out
}

func TestDateFuzzDifferential(t *testing.T) {
	ref := os.Getenv("LUA_REF")
	if ref == "" {
		t.Skip("no LUA_REF: point it at a reference interpreter to run the date fuzz")
	}
	/* The one question that has to be asked before the comparison can
	   be made: see the note at the top. */
	oldYears := dateRefRefusesOldYears(t, ref)

	var b strings.Builder
	/* The lists go in front of the functions, because a 'local' after
	   them is a different thing: 'ALL' would see a global by that
	   name and not this one, and it is nil. */
	b.WriteString(fuzzQuote)
	b.WriteString("SPECS = {" + dateList(dateSpecs) + "}\n")
	b.WriteString("SPECS_EDGE = {" + dateList(dateSubList(dateSpecs, "z")) + "}\n")
	b.WriteString("SPECS_UTC = {" +
		dateList(dateSubList(dateSpecs, "z", "Z")) + "}\n")
	b.WriteString("MODS = {" + dateList(dateMods) + "}\n")
	b.WriteString(`TIMES = {"H", "I", "M", "S", "p", "r", "R", "T",
  "X", "s", "c"}` + "\n")
	b.WriteString(dateChunk)
	n := 0
	emit := func(tag, call string) {
		n++
		b.WriteString("P(" + strconv.Quote(tag) + ", " + call + ")\n")
	}

	// 1. every day of two years, at two times of day.  2000 is a leap
	// year and 2001 is one whose first of January is a Monday, which
	// is the day '%U' and '%W' disagree about.
	noon := int64(12 * 60 * 60)
	for _, y := range []int{2000, 2001} {
		base := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC).Unix()
		days := 365
		if y%4 == 0 && (y%100 != 0 || y%400 == 0) {
			days = 366
		}
		for d := 0; d < days; d++ {
			t0 := base + int64(d)*86400
			when := fmt.Sprintf("%04d-%02d-%02d", y,
				time.Unix(t0, 0).UTC().Month(),
				time.Unix(t0, 0).UTC().Day())
			emit("all/"+when, "ALL("+strconv.FormatInt(t0, 10)+")")
			emit("all!/"+when, "UT("+strconv.FormatInt(t0, 10)+")")
			if y == 2000 {
				emit("all-noon/"+when, "ALL("+
					strconv.FormatInt(t0+noon, 10)+")")
				emit("all-noon!/"+when, "UT("+
					strconv.FormatInt(t0+noon, 10)+")")
			}
		}
	}

	// 2. every hour of a day, and every five minutes of an hour: the
	// specifiers that are about the time of day and not the date
	day0 := time.Date(2001, time.June, 15, 0, 0, 0, 0, time.UTC).Unix()
	for h := 0; h < 24; h++ {
		t0 := day0 + int64(h)*3600
		emit(fmt.Sprintf("tod/%02d", h), "TOD("+
			strconv.FormatInt(t0, 10)+")")
		emit(fmt.Sprintf("tod!/%02d", h), "UTOD("+
			strconv.FormatInt(t0, 10)+")")
	}
	for m := 0; m < 60; m += 5 {
		t0 := day0 + int64(m)*60
		emit(fmt.Sprintf("tod-min/%02d", m), "TOD("+
			strconv.FormatInt(t0, 10)+")")
	}

	// 3. the 'E' and 'O' modifiers, at four instants
	for _, t0 := range []int64{0, 951782400, 1234567890, 1735689599} {
		emit("mod/"+strconv.FormatInt(t0, 10), "MOD("+
			strconv.FormatInt(t0, 10)+")")
		emit("mod!/"+strconv.FormatInt(t0, 10), "UT("+
			strconv.FormatInt(t0, 10)+")")
	}

	// 4. the ends of what a 'struct tm' holds, and one past each of
	// them.  The top is exactly the 'int' boundary.
	for _, t0 := range dateEnds {
		emit("edge/"+strconv.FormatInt(t0, 10), "ALLE("+
			strconv.FormatInt(t0, 10)+")")
		emit("edge!/"+strconv.FormatInt(t0, 10), "UT("+
			strconv.FormatInt(t0, 10)+")")
		emit("edge-t/"+strconv.FormatInt(t0, 10), "star("+
			strconv.FormatInt(t0, 10)+", false)")
		emit("edge-t!/"+strconv.FormatInt(t0, 10), "star("+
			strconv.FormatInt(t0, 10)+", true)")
	}

	// 5. '*t' at a spread of instants, and the fields it answers
	for _, t0 := range []int64{0, 951782400, 1234567890, 1735689599,
		2147483647, -1, 86400 * 400, 978307200} {
		emit("star/"+strconv.FormatInt(t0, 10), "star("+
			strconv.FormatInt(t0, 10)+", false)")
		emit("star!/"+strconv.FormatInt(t0, 10), "star("+
			strconv.FormatInt(t0, 10)+", true)")
	}

	// 6. the formats that are not conversion specifiers, and the ones
	// that are not two letters long
	bad := []string{"%", "%%", "%q", "%Q", "%1", "%-", "%_", "%0",
		"%E", "%O", "%Ea", "%Oa", "%Ez", "%Oz", "%Ec", "%Od", "a%",
		"%a%", "x%qx", "%Y-%m-%d", "%H:%M:%S", "%I:%M:%S %p",
		"%A %B %e %Y", "%%Y", "%c", "%x %X", "%r", "%T", "%D", "%F",
		"%s", "%j", "%U %W %V", "%G %g", "%u %w", "%n%t", "%C", "%h",
		"abc", "", "!", "!x", "%%%", "%z%Z", "%Ey%OY",
		/* and the ones with a zero byte in them.  A format is walked by
		   its length and not by its first zero, but '*t' is recognised
		   with 'strcmp', which is the other way round. */
		"\x00", "%\x00", "\x00%Y", "%Y\x00%m",
		"!\x00%Y", "!%Y\x00%m", "%c\x00", "\x00*c", "*t\x00", "\x00*t",
		"*t\x00junk", "!*t\x00junk", "*\x00t", "!\x00*t", "%\x00Y",
		"%\x00c", "%Y\x00junk", "\x00!*t"}
	for _, s := range bad {
		emit("fmt "+s, "fmt(1234567890, "+strconv.Quote(s)+")")
	}
	// and the format that is not there at all
	for _, t0 := range []int64{0, 1234567890, 2147483647, -1} {
		emit("default/"+strconv.FormatInt(t0, 10), "def("+
			strconv.FormatInt(t0, 10)+")")
	}

	// 7. 'os.time', over a spread of tables and over the ones that are
	// not a date.  A year before 1900 is left out on a host that
	// refuses it: see the note at the top.
	years := []int{1970, 1999, 2000, 2001, 2024, 2038, 1900, 1899, 1969}
	for _, y := range years {
		if oldYears && y < 1900 {
			continue
		}
		for _, mo := range []int{1, 2, 12} {
			for _, d := range []int{1, 28, 29, 31} {
				emit(fmt.Sprintf("time/%04d-%02d-%02d", y, mo, d),
					"tm({year="+strconv.Itoa(y)+",month="+
						strconv.Itoa(mo)+",day="+strconv.Itoa(d)+"})")
			}
		}
	}
	for _, s := range []string{
		"{year=2000,month=2,day=29,hour=12,min=30,sec=30}",
		"{year=1970,month=1,day=1,hour=0,min=0,sec=0}",
		"{year=1970,month=1,day=1,hour=0,min=0,sec=0,isdst=true}",
		"{year=1970,month=1,day=1,hour=0,min=0,sec=0,isdst=false}",
		"{year=2000,month=13,day=1}", "{year=2000,month=0,day=1}",
		"{year=2000,month=1,day=32}", "{year=2000,month=1,day=0}",
		"{year=2000,month=1,day=-1}", "{year=2000,month=1,day=1,hour=25}",
		"{year=2000,month=1,day=1,hour=-1}",
		"{year=2000,month=1,day=1,min=60}",
		"{year=2000,month=1,day=1,sec=61}",
		"{year=2000,month=1,day=1,sec=1.5}",
		"{year=2000.5,month=1,day=1}", "{year=2000,month=1.5,day=1}",
		"{}", "{month=1}", "{year=2000}", "{year=2000,month=1}",
		"{year=2000,month=1,day=1,min=30}",
		"{year=2000,month=1,day=1,hour=12}",
		"{year=2000,month=1,day=1,sec=30}",
		"{year=2000,month=1,day=1,isdst=true}",
		"{year=9999,month=12,day=31}", "{year=10000,month=1,day=1}",
		"{year=2000,month=1,day=1,yday=100}",
		"{year=2000,month=1,day=1,wday=3}",
	} {
		emit("time "+s, "tm("+s+")")
	}

	cases := n
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
		if err := os.WriteFile(filepath.Join(d, "datefuzz.lua"),
			[]byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	/* Every zone is a separate comparison: a specifier that is about
	   the zone is a different question in each of them, and a run in
	   one says nothing about the other three. */
	ours := diffBuild(t)
	for _, tz := range dateZones {
		want := dateRun(t, ref, dirs["ref"], tz)
		got := dateRun(t, ours, dirs["ours"], tz)
		for _, side := range []struct{ name, out string }{
			{"PUC-Lua in " + tz, want}, {"this in " + tz, got},
		} {
			if !strings.Contains(side.out, "count\t"+strconv.Itoa(cases)) {
				t.Fatalf("the date chunk did not run to the end in %s:\n%s",
					side.name, side.out)
			}
		}
		w, g := diffNormalise(want), diffNormalise(got)
		if w == g {
			continue
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
				t.Logf("... and more, in %s", tz)
				break
			}
			t.Errorf("%s, line %d\n  PUC-Lua: %s\n  this:    %s",
				tz, i+1, a, c)
		}
		if reported == 0 {
			t.Errorf("in %s: the lines differ but none of them line up", tz)
		}
	}
	t.Logf("every day of 2000 and 2001 plus noon, %d hours and %d "+
		"minutes, the %d modifiers, %d tables for 'os.time', the ends "+
		"of a 'struct tm' and the formats that are not specifiers: "+
		"%d cases in each of %d zones, all of them PUC-Lua's",
		24, 12, len(dateMods), 9*12+25, cases, len(dateZones))
}

func dateList(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = strconv.Quote(s)
	}
	return strings.Join(quoted, ",")
}

// dateRun runs the chunk with one zone in the environment, which the
// two interpreters are handed alike.
func dateRun(t *testing.T, bin, dir, tz string) string {
	t.Helper()
	cmd := &exec.Cmd{
		Path: bin,
		Args: []string{"lua", "datefuzz.lua"},
		Dir:  dir,
		Env:  append(os.Environ(), "TZ="+tz),
	}
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// dateRefRefusesOldYears asks the reference whether its 'mktime' refuses
// a date in a year before 1900, which is the one question that has to be
// answered before 'os.time' can be compared.  macOS's 'mktime' does and
// glibc's does not, and which of them a runner has is not something a
// port can be right or wrong about (§6).
func dateRefRefusesOldYears(t *testing.T, ref string) bool {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "oldprobe.lua")
	if err := os.WriteFile(path,
		[]byte("print(os.time({year=1899,month=1,day=1}))\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &exec.Cmd{Path: ref, Args: []string{"lua", "oldprobe.lua"},
		Dir: dir, Env: append(os.Environ(), "TZ=UTC")}
	out, _ := cmd.CombinedOutput()
	switch {
	case strings.Contains(string(out), "cannot be represented"):
		return true
	case strings.HasPrefix(strings.TrimSpace(string(out)), "-22"):
		return false
	default:
		t.Fatalf("the reference answered %q to 'os.time' of a date in "+
			"1899, which is neither a refusal nor a number; whether its "+
			"'mktime' takes a year before 1900 has to be known before "+
			"the grid can be compared", string(out))
		return false
	}
}

// TestDateInProcess is the thirty-seven specifiers, the 19 'E' and 'O'
// modifiers, the table, the refusals and 'os.time', asked in this
// process and not in a subprocess.
//
// A subprocess does not answer 'go test -cover', which is how
// 'osFormatSpec' was found sitting at 18.4% and 'osFormatYear2' at 0%:
// every case that entered them was a case in another process.  What
// they answer is PUC-Lua's and the grid above is what says so; this is
// only that they are entered, and that the answers are pinned where no
// reference interpreter is needed to see them.
//
// Every format is asked in its UTC form, which is what makes the
// answers the same in every zone and on every machine.  The two
// letters in front of a format are the ones the C library decides
// about, and this test runs on every platform the package claims.
func TestDateInProcess(t *testing.T) {
	L := NewState()
	defer L.Close()
	L.OpenLibs()
	if err := runLua(t, L, `
		-- 1. Every specifier, at twelve instants.  The answers are
		--    the reference's, read off it in the UTC form.
		local F = "!%a|%A|%b|%B|%c|%C|%d|%D|%e|%F|%g|%G|%h|%H|%I|%j|"..
		  "%m|%M|%n|%p|%r|%R|%S|%t|%T|%u|%U|%V|%w|%W|%x|%X|%y|%Y|%z|%Z|%%"
		assert(os.date(F, 0) ==
		  "Thu|Thursday|Jan|January|Thu Jan  1 00:00:00 1970|19|01|01/01/70| 1|1970-01-01|70|1970|Jan|00|12|001|01|00|\n|AM|12:00:00 AM|00:00|00|\t|00:00:00|4|00|01|4|00|01/01/70|00:00:00|70|1970|+0000|UTC|%")
		assert(os.date(F, 1) ==
		  "Thu|Thursday|Jan|January|Thu Jan  1 00:00:01 1970|19|01|01/01/70| 1|1970-01-01|70|1970|Jan|00|12|001|01|00|\n|AM|12:00:01 AM|00:00|01|\t|00:00:01|4|00|01|4|00|01/01/70|00:00:01|70|1970|+0000|UTC|%")
		assert(os.date(F, -1) ==
		  "Wed|Wednesday|Dec|December|Wed Dec 31 23:59:59 1969|19|31|12/31/69|31|1969-12-31|70|1970|Dec|23|11|365|12|59|\n|PM|11:59:59 PM|23:59|59|\t|23:59:59|3|52|01|3|52|12/31/69|23:59:59|69|1969|+0000|UTC|%")
		assert(os.date(F, 951782400) ==
		  "Tue|Tuesday|Feb|February|Tue Feb 29 00:00:00 2000|20|29|02/29/00|29|2000-02-29|00|2000|Feb|00|12|060|02|00|\n|AM|12:00:00 AM|00:00|00|\t|00:00:00|2|09|09|2|09|02/29/00|00:00:00|00|2000|+0000|UTC|%")
		assert(os.date(F, 1234567890) ==
		  "Fri|Friday|Feb|February|Fri Feb 13 23:31:30 2009|20|13|02/13/09|13|2009-02-13|09|2009|Feb|23|11|044|02|31|\n|PM|11:31:30 PM|23:31|30|\t|23:31:30|5|06|07|5|06|02/13/09|23:31:30|09|2009|+0000|UTC|%")
		assert(os.date(F, 1735689599) ==
		  "Tue|Tuesday|Dec|December|Tue Dec 31 23:59:59 2024|20|31|12/31/24|31|2024-12-31|25|2025|Dec|23|11|366|12|59|\n|PM|11:59:59 PM|23:59|59|\t|23:59:59|2|52|01|2|53|12/31/24|23:59:59|24|2024|+0000|UTC|%")
		assert(os.date(F, 2147483647) ==
		  "Tue|Tuesday|Jan|January|Tue Jan 19 03:14:07 2038|20|19|01/19/38|19|2038-01-19|38|2038|Jan|03|03|019|01|14|\n|AM|03:14:07 AM|03:14|07|\t|03:14:07|2|03|03|2|03|01/19/38|03:14:07|38|2038|+0000|UTC|%")
		assert(os.date(F, 2147483648) ==
		  "Tue|Tuesday|Jan|January|Tue Jan 19 03:14:08 2038|20|19|01/19/38|19|2038-01-19|38|2038|Jan|03|03|019|01|14|\n|AM|03:14:08 AM|03:14|08|\t|03:14:08|2|03|03|2|03|01/19/38|03:14:08|38|2038|+0000|UTC|%")
		assert(os.date(F, 253402300800) ==
		  "Sat|Saturday|Jan|January|Sat Jan  1 00:00:00 10000|100|01|01/01/00| 1|10000-01-01|99|9999|Jan|00|12|001|01|00|\n|AM|12:00:00 AM|00:00|00|\t|00:00:00|6|00|52|6|00|01/01/00|00:00:00|00|10000|+0000|UTC|%")
		assert(os.date(F, -2147483648) ==
		  "Fri|Friday|Dec|December|Fri Dec 13 20:45:52 1901|19|13|12/13/01|13|1901-12-13|01|1901|Dec|20|08|347|12|45|\n|PM|08:45:52 PM|20:45|52|\t|20:45:52|5|49|50|5|49|12/13/01|20:45:52|01|1901|+0000|UTC|%")
		assert(os.date(F, 1099511627776) ==
		  "Mon|Monday|Feb|February|Mon Feb 20 00:36:16 36812|368|20|02/20/12|20|36812-02-20|12|36812|Feb|00|12|051|02|36|\n|AM|12:36:16 AM|00:36|16|\t|00:36:16|1|08|08|1|08|02/20/12|00:36:16|12|36812|+0000|UTC|%")
		assert(os.date(F, 67768036191676799) ==
		  "Wed|Wednesday|Dec|December|Wed Dec 31 23:59:59 2147485547|21474855|31|12/31/47|31|2147485547-12-31|48|2147485548|Dec|23|11|365|12|59|\n|PM|11:59:59 PM|23:59|59|\t|23:59:59|3|52|01|3|52|12/31/47|23:59:59|47|2147485547|+0000|UTC|%")
		local M = "!%Ec|%EC|%Ex|%EX|%Ey|%EY|%Od|%Oe|%OH|%OI|%Om|%OM|"..
		  "%OS|%Ou|%OU|%OV|%Ow|%OW|%Oy"
		assert(os.date(M, 1234567890) ==
		  "Fri Feb 13 23:31:30 2009|20|02/13/09|23:31:30|09|2009|13|13|23|11|02|31|30|5|06|07|5|06|09")
		assert(os.date(M, 0) ==
		  "Thu Jan  1 00:00:00 1970|19|01/01/70|00:00:00|70|1970|01| 1|00|12|01|00|00|4|00|01|4|00|70")
		--    A year before the Common Era, which the grid cannot ask
		--    about: it is the one thing the two C libraries spell
		--    differently.  They do agree on a year that divides by
		--    a hundred, and there the two digits of '%y' and '%g'
		--    are no digits at all and the four characters of '%Y'
		--    are three of them and a sign.
		local N = "!%Y|%C|%y|%g|%G|%j|%V|%u|%w|%a|%b|%e|%d|%m"
		assert(os.date(N, -65307340800) ==
		  "-100|-1|00|00|-100|181|26|6|6|Sat|Jun|30|30|06")
		assert(os.date(N, -68463014400) ==
		  "-200|-2|00|00|-200|181|27|1|1|Mon|Jun|30|30|06")
		assert(os.date(N, -100019923200) ==
		  "-1200|-12|00|00|-1200|182|26|5|5|Fri|Jun|30|30|06")
		assert(os.date(N, -125265484800) ==
		  "-2000|-20|00|00|-2000|182|26|5|5|Fri|Jun|30|30|06")
		--    Seven consecutive days, which is every day of the week
		--    and the Sunday '%u' calls 7.  The grid asks them 731
		--    times over; this is the seven of them.
		local W = {"Thu|Thursday|4|4", "Fri|Friday|5|5",
		  "Sat|Saturday|6|6", "Sun|Sunday|7|0", "Mon|Monday|1|1",
		  "Tue|Tuesday|2|2", "Wed|Wednesday|3|3"}
		for i = 0, 6 do
		  assert(os.date("!%a|%A|%u|%w", i * 86400) == W[i + 1],
		    tostring(i))
		end
		-- 2. The table, and its nine fields
		for _, c in ipairs{
		    {0, 1970, 1, 1, 0, 0, 0, 5, 1},
		    {1234567890, 2009, 2, 13, 23, 31, 30, 6, 44},
		    {2147483647, 2038, 1, 19, 3, 14, 7, 3, 19}} do
		  local d = os.date("!*t", c[1])
		  assert(d.year == c[2] and d.month == c[3] and d.day == c[4] and
		    d.hour == c[5] and d.min == c[6] and d.sec == c[7] and
		    d.wday == c[8] and d.yday == c[9] and d.isdst == false,
		    tostring(c[1]))
		end
		-- 3. The refusals, with the messages: the one a specifier
		--    that is not one gets, and the one a zero byte makes
		--    shorter than the format behind it
		for _, s in ipairs{"%q", "%Q", "%1", "%E", "%O", "%Ea", "%Oa",
		    "%Ez", "%Oz", "%s"} do
		  local ok, e = pcall(os.date, s, 0)
		  assert(not ok and e == "bad argument #1 to 'os.date' "..
		    "(invalid conversion specifier '" .. s .. "')", tostring(e))
		end
		local ok, e = pcall(os.date, "%\0Y", 0)
		assert(not ok and e == "bad argument #1 to 'os.date' "..
		  "(invalid conversion specifier '%')", tostring(e))
		-- 4. 'strcmp' and not a comparison of the whole format:
		--    the table is the answer behind a zero byte, and a
		--    zero byte in the middle of a format that is not one
		--    is only text
		assert(type(os.date("*t\0junk", 0)) == "table")
		assert(type(os.date("!*t\0junk", 0)) == "table")
		assert(os.date("\0*t", 0) == "\0*t")
		assert(os.date("!", 0) == "")
		-- 5. 'os.time', whose answers belong to the zone the
		--    machine is in and not to Lua: what is asked here is
		--    only what kind of thing it answers, and what it says
		--    to something that is not a table
		assert(math.type(os.time{year = 2000, month = 1, day = 1}) ==
		  "integer")
		for _, v in ipairs{0, "x", true} do
		  local ok, e = pcall(os.time, v)
		  assert(not ok, tostring(v))
		  assert(e:find("table expected", 1, true), e)
		end
	`); err != nil {
		t.Fatal(err)
	}
}
