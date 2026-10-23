// Translation of loslib.c (PUC-Lua 5.3.4): the standard Operating System
// library.  align-wip deliverable — fully aligned to PUC 5.3.4 loslib.c.
//
// Platform pieces live outside this file (see NOTES.md):
//   - os_cpu_unix.go / os_cpu_other.go — os.clock CPU time
//   - locale.go + cdlopen_locale.go — os.setlocale / libc strftime (LC_TIME)
//
// os.date's strftime-style conversion specifiers call libc strftime when
// cgo is available (osStrftimeC from cdlopen_locale.go), so LC_TIME from
// os.setlocale is visible the way it is in PUC-Lua.  Without cgo, or when
// the calendar year would make libc's tm_year+1900 overflow an int, each
// specifier is formatted on its own through the Go time package so literal
// text is never mistaken for a layout element.
package core

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------------
// Time/Date operations
// ---------------------------------------------------------------------

// osStrftimeOptions are the valid conversion specifiers (C99 set).
const osStrftimeOptions = "aAbBcCdDeFgGhHIjmMnprRStTuUVwWxXyYzZ%"

// osModifierOptions are the two-character conversion specifiers: the
// 'E' and 'O' modifiers of C99.
var osModifierOptions = map[byte]string{
	'E': "cCxXyY",
	'O': "deHImMSuUVwWy",
}

// osStrftimeC is libc strftime when the build has cgo on unix; nil
// otherwise.  Spec is a full conversion including '%', e.g. "%c" or "%Ec".
// epoch is Unix seconds; utc selects gmtime vs localtime.
var osStrftimeC func(epoch int64, utc bool, spec string) (string, bool)

// osStrftimeYearSafe reports whether t's calendar year fits in a C int.
// glibc's strftime computes tm_year+1900 in int for %Y/%C; at the top of
// the struct-tm range that overflows (see datefuzz_test.go §6).  The
// hand-rolled formatter is used there instead.
func osStrftimeYearSafe(t time.Time) bool {
	y := t.Year()
	return y >= math.MinInt32 && y <= math.MaxInt32
}

// osSetField sets a numeric field in the table below the top (setfield).
func osSetField(L *State, key string, value int) {
	L.PushInteger(int64(value))
	L.SetField(-2, key)
}

// osSetBoolField is setboolfield: skip the field when value < 0 (undefined).
func osSetBoolField(L *State, key string, value int) {
	if value < 0 { /* undefined? */
		return /* does not set field */
	}
	L.PushBoolean(value != 0)
	L.SetField(-2, key)
}

// osSetAllFields fills the table on top of the stack (setallfields).
// Field write order matches loslib.c 5.3.4: sec,min,hour,day,month,year,wday,yday,isdst.
func osSetAllFields(L *State, t time.Time) {
	osSetField(L, "sec", t.Second())
	osSetField(L, "min", t.Minute())
	osSetField(L, "hour", t.Hour())
	osSetField(L, "day", t.Day())
	osSetField(L, "month", int(t.Month()))
	osSetField(L, "year", t.Year())
	osSetField(L, "wday", int(t.Weekday())+1)
	osSetField(L, "yday", t.YearDay())
	isdst := 0
	if tIsDST(t) {
		isdst = 1
	}
	osSetBoolField(L, "isdst", isdst)
}

// tIsDST reports whether t falls in the daylight-saving time of its
// location.
func tIsDST(t time.Time) bool {
	_, offset := t.Zone()
	_, janOffset := time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location()).Zone()
	_, julOffset := time.Date(t.Year(), 7, 1, 0, 0, 0, 0, t.Location()).Zone()
	if janOffset == julOffset {
		return false
	}
	if janOffset > julOffset {
		return offset == janOffset
	}
	return offset == julOffset
}

// tHasDSTRules reports whether the location of t ever keeps daylight-
// saving time.  mktime ignores 'tm_isdst' in a zone that has no such
// rule, and honours it in one that has, even in a year the rule does not
// touch -- so the answer is not "is t in DST" but "does this zone know
// about DST at all".  The scan is what the timezone database gives us:
// Go exposes no list of transitions, so the years are sampled.
var dstRules sync.Map /* *time.Location -> bool */

func tHasDSTRules(t time.Time) bool {
	loc := t.Location()
	if v, ok := dstRules.Load(loc); ok {
		return v.(bool)
	}
	has := false
	for year := 1900; year <= 2100 && !has; year++ {
		_, jan := time.Date(year, time.January, 1, 0, 0, 0, 0, loc).Zone()
		_, jul := time.Date(year, time.July, 1, 0, 0, 0, 0, loc).Zone()
		has = jan != jul
	}
	dstRules.Store(loc, has)
	return has
}

// osGetBoolField reads a boolean field of the table at the top; it
// returns -1 when the field is absent (getboolfield).
func osGetBoolField(L *State, key string) int {
	if L.GetField(-1, key) == LUA_TNIL {
		L.Pop(1)
		return -1 /* undefined */
	}
	res := 0
	if L.ToBoolean(-1) {
		res = 1
	}
	L.Pop(1)
	return res
}

// osGetField reads an integer field of the table at the top, with a
// default (getfield).
func osGetField(L *State, key string, d, delta int) int {
	tp := L.GetField(-1, key) /* get field and its type */
	v, isnum := L.ToInteger(-1)
	L.Pop(1)
	if !isnum { /* field is not an integer? */
		if tp != LUA_TNIL { /* some other value? */
			return L.Error2("field '%s' is not an integer", key)
		}
		if d < 0 { /* absent field; no default? */
			return L.Error2("field '%s' missing in date table", key)
		}
		return d
	}
	/* 5.3.4: L_MAXDATEFIELD = INT_MAX/2 */
	const maxDateField = int64(math.MaxInt32 / 2)
	if !(-maxDateField <= v && v <= maxDateField) {
		return L.Error2("field '%s' is out-of-bound", key)
	}
	return int(v) - delta
}

// osCheckOption validates one conversion specifier; it returns the
// specifier and how many characters it uses (checkoption).
func osCheckOption(L *State, conv string) (string, int) {
	if len(conv) >= 2 {
		if opts, ok := osModifierOptions[conv[0]]; ok {
			if strings.IndexByte(opts, conv[1]) >= 0 {
				return conv[:2], 2
			}
		}
	}
	if len(conv) >= 1 && strings.IndexByte(osStrftimeOptions, conv[0]) >= 0 {
		return conv[:1], 1
	}
	/* C's message is built with 'lua_pushfstring', whose '%s' is
	   'strlen', and 'conv' is a pointer into the middle of the format:
	   what is written out is the bytes up to the first zero and not
	   the rest of the format behind them.  'os.date("%\0Y")' is
	   "invalid conversion specifier '%'" in PUC-Lua and was
	   "invalid conversion specifier '%\0Y'" here. */
	L.ArgError(1, fmt.Sprintf("invalid conversion specifier '%%%s'",
		luaCStr(conv)))
	return "", 0
}

// osFormatYear4 renders a year the way C's '%Y' does, which is four
// characters wide and not four digits wide: the sign is a character of
// the width, so the year -101 is "-101" and the year 1 is "0001".  Go's
// layout pads the digits and leaves the sign outside them, which is
// "-0101" for the first of those.
func osFormatYear4(year int) string {
	return fmt.Sprintf("%04d", year)
}

// osFormatYear2 renders a year as two digits the way C's '%y' and '%g'
// do, which is not the way its '%Y' and '%C' do.  Those two keep the
// sign of a year before the Common Era: C writes -21474817 for the
// century of -2147481748.  But '%y' is the remainder of a signed
// division by a hundred and it is written two digits wide, and C takes
// the absolute value of the remainder before it writes it, so the same
// year is 48 and not -48.  Go's '%y' is the year modulo a hundred with
// the sign left on it.  (Which two digits a year before the Common Era
// gets is then the C library's to say as well -- see §6 -- and this is
// the one Apple's spells.  It is the one the differential tests are run
// against.)
func osFormatYear2(year int) string {
	r := year % 100
	if r < 0 {
		r = -r
	}
	return fmt.Sprintf("%02d", r)
}

// osFormatSpec formats one conversion specifier of a strftime string.
func osFormatSpec(t time.Time, spec string) string {
	if len(spec) == 0 {
		return ""
	}
	c := spec[len(spec)-1] /* skip a possible 'E'/'O' modifier */
	switch c {
	case 'a':
		return t.Format("Mon")
	case 'A':
		return t.Format("Monday")
	case 'b', 'h':
		return t.Format("Jan")
	case 'B':
		return t.Format("January")
	case 'c':
		return t.Format("Mon Jan _2 15:04:05 2006")
	case 'd':
		return t.Format("02")
	case 'D':
		return t.Format("01/02/06")
	case 'e':
		return t.Format("_2")
	case 'F':
		return t.Format("2006-01-02")
	case 'H':
		return t.Format("15")
	case 'I':
		return t.Format("03")
	case 'm':
		return t.Format("01")
	case 'M':
		return t.Format("04")
	case 'n':
		return "\n"
	case 'p':
		return t.Format("PM")
	case 'r':
		return t.Format("03:04:05 PM")
	case 'R':
		return t.Format("15:04")
	case 'S':
		return t.Format("05")
	case 't':
		return "\t"
	case 'T':
		return t.Format("15:04:05")
	case 'x':
		return t.Format("01/02/06")
	case 'X':
		return t.Format("15:04:05")
	case 'y':
		return osFormatYear2(t.Year())
	case 'Y':
		return osFormatYear4(t.Year())
	case 'z':
		/* The offset of the instant, which is what Go's layout asks
		   for.  Under '!' the instant has been moved to UTC, so this
		   is +0000: the same answer glibc's '%z' gives, because
		   its 'gmtime' leaves 'tm_gmtoff' at zero.  What the other
		   C libraries answer instead, and why it is not asked
		   about, is written up in §6. */
		return t.Format("-0700")
	case 'Z':
		/* Zone abbreviation.  Under '!' Go's UTC location names it
		   "UTC"; glibc's gmtime leaves tm_zone as "GMT".  With cgo,
		   osStrftime uses libc and matches the host; this hand-roll
		   path (no cgo / unsafe year) keeps Go's name — SPEC. */
		name, _ := t.Zone()
		if name == "" {
			return t.Format("MST")
		}
		return name
	case '%':
		return "%"
	/* specifiers with no direct counterpart in a Go layout */
	case 'C':
		return fmt.Sprintf("%02d", t.Year()/100)
	case 'j':
		return fmt.Sprintf("%03d", t.YearDay())
	case 'u':
		w := int(t.Weekday())
		if w == 0 {
			w = 7
		}
		return fmt.Sprintf("%d", w)
	case 'w':
		return fmt.Sprintf("%d", int(t.Weekday()))
	case 'U':
		/* C's 'tm_yday' counts from zero and Go's YearDay from one, so
		   the day of the year has to be shifted back before the week
		   number is taken: strftime counts a week that has started as
		   soon as 'yday + 7 - wday' reaches 7, and starting the count
		   at one makes every week start a day early.  A first of
		   January that is a Monday is week 00 in C and was week 01
		   here. */
		yday := t.YearDay() - 1
		return fmt.Sprintf("%02d", (yday+7-int(t.Weekday()))/7)
	case 'W':
		yday := t.YearDay() - 1
		return fmt.Sprintf("%02d", (yday+7-(int(t.Weekday())+6)%7)/7)
	case 'V':
		_, week := t.ISOWeek()
		return fmt.Sprintf("%02d", week)
	case 'G':
		year, _ := t.ISOWeek()
		return fmt.Sprintf("%04d", year)
	case 'g':
		year, _ := t.ISOWeek()
		return osFormatYear2(year)
	}
	return ""
}

// osStrftime formats a time according to a strftime-style format string.
// utc matches the leading '!' of os.date so libc gmtime/localtime agree
// with the time.Time already chosen by the caller.
func osStrftime(L *State, t time.Time, s string, utc bool) string {
	useC := osStrftimeC != nil && osStrftimeYearSafe(t)
	var out strings.Builder
	for len(s) > 0 {
		i := strings.IndexByte(s, '%')
		if i < 0 { /* no more conversion specifiers */
			out.WriteString(s)
			break
		}
		out.WriteString(s[:i])
		s = s[i+1:] /* skip '%' */
		spec, n := osCheckOption(L, s)
		s = s[n:]
		if useC {
			if got, ok := osStrftimeC(t.Unix(), utc, "%"+spec); ok {
				out.WriteString(got)
				continue
			}
		}
		out.WriteString(osFormatSpec(t, spec))
	}
	return out.String()
}

// ---------------------------------------------------------------------
// library functions
// ---------------------------------------------------------------------

func osExecute(L *State) int {
	if L.IsNoneOrNil(1) { /* no argument? */
		L.PushBoolean(true) /* true if there is a shell */
		return 1
	}
	/* 'system' takes a C string, so the command ends where its first
	   zero byte is: 'os.execute("true\0x")' runs "true". */
	cmd := ioShellCommand(luaCStr(L.CheckString(1)))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	what, code := ioProcessStatus(cmd.Run())
	return ioExecResult(L, what, code)
}

func osRemove(L *State) int {
	/* 'remove' is a C function, so it is the bytes before the first
	   zero that are removed -- and it is those that the message names,
	   because 'luaL_fileresult' writes the name with a %s. */
	filename := luaCStr(L.CheckString(1))
	err := os.Remove(filename)
	return ioFileResult(L, err == nil, &filename, err)
}

func osRename(L *State) int {
	/* C asks no name in the message here: 'fileresult' is called with
	   NULL, so a failing 'os.rename' answers 'strerror' alone, which is
	   "No such file or directory" and not "rename a b: no such file or
	   directory". */
	fromname := luaCStr(L.CheckString(1))
	toname := luaCStr(L.CheckString(2))
	err := ioRename(fromname, toname)
	return ioFileResult(L, err == nil, nil, err)
}

// osTmpName is os_tmpname (loslib.c).  The C library builds the name with
// 'lua_tmpnam', which under POSIX copies LUA_TMPNAMTEMPLATE and hands it
// to mkstemp; the file is created and closed, and the name -- not an open
// handle -- is what the function returns.  The path therefore has to be
// the one scripts expect, which is why the template is reproduced here
// instead of using os.CreateTemp (that would use $TMPDIR).
func osTmpName(L *State) int {
	name, err := luaTmpNam()
	if err != nil {
		return L.Error2("unable to generate a unique filename")
	}
	L.PushString(name)
	return 1
}

// luaTmpNam is lua_tmpnam: it fills the template "/tmp/lua_XXXXXX" the
// way mkstemp does and creates the file.
func luaTmpNam() (string, error) {
	dir := "/tmp" /* LUA_TMPNAMTEMPLATE */
	if runtime.GOOS == "windows" {
		dir = os.TempDir() /* no '/tmp' there: use whatever the OS says */
	}
	var b [6]byte
	for i := 0; i < 128; i++ {
		for j := range b {
			b[j] = tmpNamChars[rand.Intn(len(tmpNamChars))]
		}
		name := dir + "/lua_" + string(b[:])
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			f.Close() /* mkstemp returns a descriptor; C closes it at once */
			return name, nil
		}
	}
	return "", errors.New("unable to generate a unique filename")
}

// tmpNamChars is the alphabet mkstemp picks the six letters from.
const tmpNamChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func osGetEnv(L *State) int {
	/* getenv takes a C string, so the name ends at the first zero:
	   os.getenv("FOO\0BAR") looks up "FOO", matching PUC-Lua. */
	v, ok := os.LookupEnv(luaCStr(L.CheckString(1)))
	if !ok {
		L.PushNil() /* if not found push nil */
		return 1
	}
	L.PushString(v)
	return 1
}

// osStartTime is when the program started.  Used only when process CPU
// time is unavailable (see osProcessCPUTime): os.clock then falls back
// to wall time since start — a documented SPEC vs C clock().
var osStartTime = time.Now()

func osClock(L *State) int {
	/* C: ((lua_Number)clock())/(lua_Number)CLOCKS_PER_SEC — CPU time.
	   On Unix, Getrusage(RUSAGE_SELF) user+system matches that without
	   cgo.  Elsewhere there is no portable CPU timer; wall-since-start
	   is the best effort (SPEC). */
	if sec, ok := osProcessCPUTime(); ok {
		L.PushNumber(sec)
		return 1
	}
	L.PushNumber(time.Since(osStartTime).Seconds())
	return 1
}

/*
osCheckTime is l_checktime (loslib.c): (time_t)t == t.

	Target is Linux LP64 (time_t == int64), so every lua_Integer fits.
*/
func osCheckTime(L *State, arg int) int64 {
	t := L.CheckInteger(arg)
	L.ArgCheck(int64(t) == t, arg, "time out-of-bounds")
	return t
}

func osDate(L *State) int {
	s := L.OptString(1, "%c")
	t := time.Now().Unix()
	if !L.IsNoneOrNil(2) {
		t = osCheckTime(L, 2) /* luaL_opt(L, l_checktime, 2, time(NULL)) */
	}
	utc := false
	if len(s) > 0 && s[0] == '!' { /* UTC? */
		utc = true
		s = s[1:] /* skip '!' */
	}
	var tm time.Time
	if utc {
		tm = time.Unix(t, 0).UTC()
	} else {
		tm = time.Unix(t, 0)
	}
	/* 'struct tm' keeps the year in an 'int', and the C functions
	   answer NULL when it does not fit (loslib.c):
	      if (stm == NULL)
	        return luaL_error(L,
	          "time result cannot be represented in this installation");
	   Go has no such limit -- time.Unix answers with the year
	   146138514283 for 2^62 -- so the cut-off has to be asked for
	   here.  On this host the top of the range is exactly the int
	   boundary: 67768036191676799 is the last representable second
	   (year 2147485547, 'tm_year' INT_MAX) and one more is refused.
	   The bottom of the range stops a little short of INT_MIN, so
	   the two ends are not quite symmetric. */
	if y := tm.Year(); y-1900 > math.MaxInt32 || y-1900 < math.MinInt32 {
		return L.Error2("time result cannot be represented in this installation")
	}
	/* 'strcmp(s, "*t")', which is a C string comparison and not a
	   comparison of the whole format: 'os.date("*t\0junk", t)' is the
	   table in PUC-Lua, because what 'strcmp' sees is "*t".  The
	   format itself is walked by length and not by a zero, so a zero
	   that is not there is kept -- the two are separate rules and
	   both have to be right. */
	if luaCStr(s) == "*t" {
		L.CreateTable(0, 9) /* 9 = number of fields */
		osSetAllFields(L, tm)
	} else {
		L.PushString(osStrftime(L, tm, s, utc))
	}
	return 1
}

func osTime(L *State) int {
	if L.IsNoneOrNil(1) { /* called without args? */
		L.PushInteger(time.Now().Unix()) /* get current time */
		return 1
	}
	L.CheckType(1, LUA_TTABLE)
	L.SetTop(1) /* make sure table is at the top */
	/* getfield subtracts 'delta' (as in C, where the fields of 'struct
	   tm' are relative); add it back for time.Date */
	/* loslib.c 5.3.4 order: sec, min, hour, day, month, year */
	sec := osGetField(L, "sec", 0, 0)
	min := osGetField(L, "min", 0, 0)
	hour := osGetField(L, "hour", 12, 0)
	day := osGetField(L, "day", -1, 0)
	month := osGetField(L, "month", -1, 1) + 1
	year := osGetField(L, "year", -1, 1900) + 1900
	isdst := osGetBoolField(L, "isdst") /* -1: work it out, 0: no, 1: yes */
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.Local)
	/* mktime reads 'tm_isdst' as an instruction rather than as a
	   question: 1 means "read the wall clock as daylight-saving time",
	   0 means "read it as standard time", and -1 (the field absent)
	   means "work it out".  When the instruction disagrees with the
	   zone, the absolute time moves by the daylight-saving step while
	   the wall clock stays where it was -- that is why a winter
	   'isdst = true' in New York answers with 11 o'clock and not 12,
	   and why the fields written back into the table say 11 too.  Every
	   zone in general use steps by an hour, and that is the step taken
	   here.  A zone with no daylight-saving rule at all has nothing to
	   disagree about and the field is ignored -- which is why 'isdst =
	   true' changes nothing in UTC but shifts by an hour in Shanghai,
	   whose rules still hold the daylight-saving time China kept in the
	   late 1980s.
	   That is what macOS's 'mktime' does.  glibc's goes about it the
	   other way: it looks through the timezone database for a nearby
	   instant whose 'tm_isdst' is the one it was told and borrows its
	   offset, and only when it finds none anywhere near does it assume
	   an hour.  For 1970 in Dublin it borrows an offset from 1972,
	   which is the offset Dublin kept the whole year round in 1970, so
	   the instant does not move at all; in UTC, where nothing has a
	   summer time, it assumes the hour and the instant moves.  The two
	   libraries answer -7200 and -3600 to one question and 0 and -3600
	   to the other, and a port can follow only one of them: it follows
	   the macOS one, and the two questions are asked only where the
	   two agree (§6.16). */
	if isdst >= 0 && tHasDSTRules(t) {
		switch {
		case isdst > 0 && !tIsDST(t):
			t = t.Add(-time.Hour)
		case isdst == 0 && tIsDST(t):
			t = t.Add(time.Hour)
		}
	}
	osSetAllFields(L, t) /* update fields with normalized values */
	/* C refuses a result of exactly -1, the value 'mktime' answers when
	   it cannot represent the time (loslib.c):
	      if (t != (time_t)(l_timet)t || t == (time_t)(-1))
	        return luaL_error(L,
	          "time result cannot be represented in this installation"); */
	if t.Unix() == -1 {
		return L.Error2("time result cannot be represented in this installation")
	}
	L.PushInteger(t.Unix())
	return 1
}

// osDiffTime is os.difftime (loslib.c).  The subtraction is done in
// floating point, because that is what C's 'difftime' does: 'os.difftime'
// hands its two 'time_t' to the C library and the difference comes back
// as a double.  Subtracting the integers first and converting after
// overflows instead -- 'os.difftime(0, math.mininteger)' answered
// -9.2233720368548e+18 where PUC-Lua answers +9.2233720368548e+18, and
// 'os.difftime(math.maxinteger, math.mininteger)' answered -1.0 instead
// of 1.844674407371e+19.
func osDiffTime(L *State) int {
	t1 := osCheckTime(L, 1)
	t2 := osCheckTime(L, 2)
	L.PushNumber(float64(t1) - float64(t2))
	return 1
}

// osSetLocale is 'os.setlocale' (loslib.c). With cgo it is the C
// library's setlocale, including the per-category query. Without cgo
// there is no C library to ask; see locale.go.
func osSetLocale(L *State) int {
	cat := L.CheckOption(2, "all", localeCategories)
	if L.IsNoneOrNil(1) { /* no locale: query the current one */
		L.PushString(localeCurrent(cat))
		return 1
	}
	name, _ := L.ToString(1)
	if got, ok := setLocale(cat, name); ok {
		L.PushString(got)
		return 1
	}
	L.PushNil() /* locale not available */
	return 1
}

func osExit(L *State) int {
	var status int
	if L.IsBoolean(1) {
		if L.ToBoolean(1) {
			status = 0
		} else {
			status = 1
		}
	} else {
		status = int(L.OptInteger(1, 0))
	}
	if L.ToBoolean(2) {
		L.Close()
	}
	/* 'exit' flushes the buffers of the streams that are still open; it
	   does not run '__gc', so a file that was never closed keeps what
	   was written to it. */
	ioFlushAll()
	osExitHook(status)
	return 0
}

// osExitHook is os.Exit. Tests replace it so they can see the status
// without ending the process.
var osExitHook = os.Exit

// ---------------------------------------------------------------------
// registration
// ---------------------------------------------------------------------

var osLib = []Reg{
	{"clock", osClock},
	{"date", osDate},
	{"difftime", osDiffTime},
	{"execute", osExecute},
	{"exit", osExit},
	{"getenv", osGetEnv},
	{"remove", osRemove},
	{"rename", osRename},
	{"setlocale", osSetLocale},
	{"time", osTime},
	{"tmpname", osTmpName},
}

// OpenOS opens the operating system library (luaopen_os).
func OpenOS(L *State) int {
	L.NewLib(osLib)
	L.PushValue(-1)
	L.SetGlobal("os")
	return 1
}
