// Translation of loslib.c (PUC-Lua 5.4): the standard Operating System
// library.
//
// The strftime-style conversion specifiers of os.date are implemented on
// top of the Go time package; every specifier is formatted on its own so
// that literal text is never mistaken for a layout element.
package lua

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

// osSetField sets a numeric field in the table below the top (setfield).
func osSetField(L *State, key string, value int) {
	L.PushInteger(int64(value))
	L.SetField(-2, key)
}

// osSetBoolField sets a boolean field, skipping it when undefined
// (setboolfield).
func osSetBoolField(L *State, key string, value bool) {
	L.PushBoolean(value)
	L.SetField(-2, key)
}

// osSetAllFields fills the table on top of the stack (setallfields).
func osSetAllFields(L *State, t time.Time) {
	osSetField(L, "year", t.Year())
	osSetField(L, "month", int(t.Month()))
	osSetField(L, "day", t.Day())
	osSetField(L, "hour", t.Hour())
	osSetField(L, "min", t.Minute())
	osSetField(L, "sec", t.Second())
	osSetField(L, "yday", t.YearDay())
	osSetField(L, "wday", int(t.Weekday())+1)
	osSetBoolField(L, "isdst", tIsDST(t))
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
	/* The field has to survive both the subtraction of 'delta' and a
	   trip through 'struct tm', whose members are 'int' (loslib.c):
	      if (!(res >= 0 ? res - delta <= INT_MAX : INT_MIN + delta <= res))
	        return luaL_error(L, "field '%s' is out-of-bound", key);
	   'os.time{year = -2147483648}' is out of bound for that reason --
	   'tm_year' would be -2147485548 -- while '2147483647' is not. */
	if !(v >= 0 && v-int64(delta) <= math.MaxInt32 ||
		v < 0 && math.MinInt32+int64(delta) <= v) {
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
	L.ArgError(1, fmt.Sprintf("invalid conversion specifier '%%%s'", conv))
	return "", 0
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
		return t.Format("06")
	case 'Y':
		return t.Format("2006")
	case 'z':
		return t.Format("-0700")
	case 'Z':
		return t.Format("MST")
	case '%':
		return "%"
	/* specifiers with no direct counterpart in a Go layout */
	case 'C':
		return fmt.Sprintf("%02d", t.Year()/100)
	case 'j':
		return fmt.Sprintf("%03d", t.YearDay())
	case 's':
		return fmt.Sprintf("%d", t.Unix())
	case 'u':
		w := int(t.Weekday())
		if w == 0 {
			w = 7
		}
		return fmt.Sprintf("%d", w)
	case 'w':
		return fmt.Sprintf("%d", int(t.Weekday()))
	case 'U':
		return fmt.Sprintf("%02d", (t.YearDay()+7-int(t.Weekday()))/7)
	case 'W':
		return fmt.Sprintf("%02d", (t.YearDay()+7-(int(t.Weekday())+6)%7)/7)
	case 'V':
		_, week := t.ISOWeek()
		return fmt.Sprintf("%02d", week)
	case 'G':
		year, _ := t.ISOWeek()
		return fmt.Sprintf("%04d", year)
	case 'g':
		year, _ := t.ISOWeek()
		return fmt.Sprintf("%02d", year%100)
	}
	return ""
}

// osStrftime formats a time according to a strftime-style format string.
func osStrftime(L *State, t time.Time, s string) string {
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
	cmd := ioShellCommand(L.CheckString(1))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	what, code := ioProcessStatus(cmd.Run())
	return ioExecResult(L, what, code)
}

func osRemove(L *State) int {
	filename := L.CheckString(1)
	err := os.Remove(filename)
	return ioFileResult(L, err == nil, filename, err)
}

func osRename(L *State) int {
	fromname := L.CheckString(1)
	toname := L.CheckString(2)
	err := os.Rename(fromname, toname)
	return ioFileResult(L, err == nil, "", err)
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
	v, ok := os.LookupEnv(L.CheckString(1))
	if !ok {
		L.PushNil() /* if not found push nil */
		return 1
	}
	L.PushString(v)
	return 1
}

// osStartTime is when the program started; os.clock measures from there
// (there is no portable CPU-time clock in Go).
var osStartTime = time.Now()

func osClock(L *State) int {
	L.PushNumber(time.Since(osStartTime).Seconds())
	return 1
}

func osDate(L *State) int {
	s := L.OptString(1, "%c")
	t := L.OptInteger(2, time.Now().Unix())
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
	if s == "*t" {
		L.CreateTable(0, 9) /* 9 = number of fields */
		osSetAllFields(L, tm)
	} else {
		L.PushString(osStrftime(L, tm, s))
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
	year := osGetField(L, "year", -1, 1900) + 1900
	month := osGetField(L, "month", -1, 1) + 1
	day := osGetField(L, "day", -1, 0)
	hour := osGetField(L, "hour", 12, 0)
	min := osGetField(L, "min", 0, 0)
	sec := osGetField(L, "sec", 0, 0)
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
	   zone in general use steps by an hour.  A zone with no
	   daylight-saving rule at all has nothing to disagree about and
	   ignores the field -- which is why 'isdst = true' changes nothing
	   in UTC but shifts by an hour in Shanghai, whose rules still hold
	   the daylight-saving time China kept in the late 1980s. */
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
	t1 := L.CheckInteger(1)
	t2 := L.CheckInteger(2)
	L.PushNumber(float64(t1) - float64(t2))
	return 1
}

// localeName is the only locale this port can report. Go has no setlocale
// and this port has no cgo, so "C" -- which is how the Go runtime behaves
// by default anyway -- is the one locale that can be set or queried; every
// other name is reported as unavailable, exactly as a C build does when the
// locale is not installed.
var localeName = "C"

func osSetLocale(L *State) int {
	catnames := []string{"all", "collate", "ctype", "monetary", "numeric", "time"}
	L.CheckOption(2, "all", catnames)
	if L.IsNoneOrNil(1) { /* no locale: query the current one */
		L.PushString(localeName)
		return 1
	}
	if s, ok := L.ToString(1); ok && s == "C" {
		localeName = "C"
		L.PushString(localeName)
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
	os.Exit(status)
	return 0
}

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
