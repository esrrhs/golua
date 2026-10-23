// Translation of loslib.c (PUC-Lua 5.4.9): the standard Operating System
// library.  Round-2 align — control flow / algorithms / messages vs loslib.c.
//
// Platform pieces live outside this file (do not delete):
//   - os_cpu_unix.go / os_cpu_other.go — os.clock CPU time
//   - locale.go + cdlopen_locale.go — os.setlocale / libc strftime (LC_TIME)
//
// os.date's strftime-style conversion specifiers call libc strftime when
// cgo is available (osStrftimeC from cdlopen_locale.go), so LC_TIME from
// os.setlocale is visible the way it is in PUC-Lua.  Without cgo, or when
// the calendar year would make libc's tm_year+1900 overflow an int, each
// specifier is formatted on its own through the Go time package.
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

/*
** {==================================================================
** List of valid conversion specifiers for the 'strftime' function;
** options are grouped by length; group of length 2 start with '||'.
** C99 branch of LUA_STRFTIMEOPTIONS (loslib.c).  Windows '#' / C89 sets
** are compile options — FLAG (this port targets C99).
** ===================================================================
 */
const luaStrftimeOptions = "aAbBcCdDeFgGhHIjmMnprRStTuUVwWxXyYzZ%" +
	"||" + "EcECExEXEyEY" + "OdOeOHOIOmOMOSOuOUOVOwOWOy"

// osStrftimeC is libc strftime when the build has cgo on unix; nil
// otherwise.  Spec is a full conversion including '%', e.g. "%c" or "%Ec".
// epoch is Unix seconds; utc selects gmtime vs localtime.
// Set from cdlopen_locale.go when cdl_strftime is wired.
var osStrftimeC func(epoch int64, utc bool, spec string) (string, bool)

// ---------------------------------------------------------------------
// library functions (C order: execute … clock, then date helpers below)
// ---------------------------------------------------------------------

func osExecute(L *State) int {
	/* C: const char *cmd = luaL_optstring(L, 1, NULL);
	   errno = 0; stat = l_system(cmd);
	   if (cmd != NULL) return luaL_execresult(L, stat);
	   else { lua_pushboolean(L, stat); return 1; } */
	if L.IsNoneOrNil(1) { /* no argument? — l_system(NULL) */
		L.PushBoolean(true) /* true if there is a shell (Go always) */
		return 1
	}
	/* 'system' takes a C string, so the command ends at the first zero. */
	cmd := ioShellCommand(luaCStr(L.CheckString(1)))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	what, code := ioProcessStatus(cmd.Run())
	return ioExecResult(L, what, code)
}

func osRemove(L *State) int {
	/* C: errno = 0; return luaL_fileresult(L, remove(filename) == 0, filename); */
	filename := luaCStr(L.CheckString(1))
	err := os.Remove(filename)
	return ioFileResult(L, err == nil, &filename, err)
}

func osRename(L *State) int {
	/* C: errno = 0; return luaL_fileresult(L, rename(...) == 0, NULL); */
	fromname := luaCStr(L.CheckString(1))
	toname := luaCStr(L.CheckString(2))
	err := ioRename(fromname, toname)
	return ioFileResult(L, err == nil, nil, err)
}

// osTmpName is os_tmpname.  C builds the name with lua_tmpnam (POSIX:
// LUA_TMPNAMTEMPLATE + mkstemp, then close); returns the path, not a handle.
func osTmpName(L *State) int {
	name, err := luaTmpNam()
	if err != nil {
		return L.Error2("unable to generate a unique filename")
	}
	L.PushString(name)
	return 1
}

// luaTmpNam is lua_tmpnam: fills "/tmp/lua_XXXXXX" the way mkstemp does.
func luaTmpNam() (string, error) {
	dir := "/tmp" /* LUA_TMPNAMTEMPLATE */
	if runtime.GOOS == "windows" {
		dir = os.TempDir() /* no '/tmp' there */
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

const tmpNamChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func osGetEnv(L *State) int {
	/* C: lua_pushstring(L, getenv(...));  — NULL → nil */
	v, ok := os.LookupEnv(luaCStr(L.CheckString(1)))
	if !ok {
		L.PushNil()
		return 1
	}
	L.PushString(v)
	return 1
}

// osStartTime is when the program started.  Used only when process CPU
// time is unavailable (see osProcessCPUTime): os.clock then falls back
// to wall time since start — FLAG vs C clock().
var osStartTime = time.Now()

func osClock(L *State) int {
	/* C: ((lua_Number)clock())/(lua_Number)CLOCKS_PER_SEC */
	if sec, ok := osProcessCPUTime(); ok {
		L.PushNumber(sec)
		return 1
	}
	L.PushNumber(time.Since(osStartTime).Seconds())
	return 1
}

/*
** {======================================================
** Time/Date operations
** =======================================================
 */

// osSetField is setfield: push (lua_Integer)value + delta.
// The LUA_NUMTIME overflow check is compile-time only when times are
// doubles and LUA_MAXINTEGER <= INT_MAX; default l_timet is lua_Integer
// so that branch is inactive.
func osSetField(L *State, key string, value, delta int) {
	L.PushInteger(int64(value) + int64(delta))
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
// Field write order matches loslib.c 5.4.9:
//
//	year, month, day, hour, min, sec, yday, wday, isdst
func osSetAllFields(L *State, t time.Time) {
	osSetField(L, "year", t.Year()-1900, 1900)
	osSetField(L, "month", int(t.Month())-1, 1)
	osSetField(L, "day", t.Day(), 0)
	osSetField(L, "hour", t.Hour(), 0)
	osSetField(L, "min", t.Minute(), 0)
	osSetField(L, "sec", t.Second(), 0)
	osSetField(L, "yday", t.YearDay()-1, 1)
	osSetField(L, "wday", int(t.Weekday()), 1)
	isdst := 0
	if tIsDST(t) {
		isdst = 1
	}
	osSetBoolField(L, "isdst", isdst)
}

// tIsDST reports whether t falls in daylight-saving time of its location.
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

// tHasDSTRules reports whether the location ever keeps DST (mktime honour
// of tm_isdst).  Go exposes no transition list; years are sampled.
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

// osGetBoolField is getboolfield: -1 when absent.
func osGetBoolField(L *State, key string) int {
	var res int
	if L.GetField(-1, key) == LUA_TNIL {
		res = -1 /* undefined */
	} else if L.ToBoolean(-1) {
		res = 1
	} else {
		res = 0
	}
	L.Pop(1)
	return res
}

// osGetField is getfield.  Bound check matches 5.4.9:
//
//	if (!(res >= 0 ? res - delta <= INT_MAX : INT_MIN + delta <= res))
//
// Pop is after the whole if/else, as in C.
func osGetField(L *State, key string, d, delta int) int {
	t := L.GetField(-1, key) /* get field and its type */
	res, isnum := L.ToInteger(-1)
	if !isnum { /* field is not an integer? */
		if t != LUA_TNIL { /* some other value? */
			return L.Error2("field '%s' is not an integer", key)
		} else if d < 0 { /* absent field; no default? */
			return L.Error2("field '%s' missing in date table", key)
		}
		res = int64(d)
	} else {
		if !(res >= 0 && res-int64(delta) <= math.MaxInt32 ||
			res < 0 && math.MinInt32+int64(delta) <= res) {
			return L.Error2("field '%s' is out-of-bound", key)
		}
		res -= int64(delta)
	}
	L.Pop(1)
	return int(res)
}

// osCheckOption is checkoption (loslib.c): walk LUA_STRFTIMEOPTIONS with
// '||' length bump and memcmp of oplen against conv[0:convlen].
// Returns the specifier and how many characters it consumed.
func osCheckOption(L *State, conv string, convlen int) (string, int) {
	option := luaStrftimeOptions
	oplen := 1 /* length of options being checked */
	for oi := 0; oi < len(option) && oplen <= convlen; oi += oplen {
		if option[oi] == '|' { /* next block? */
			oplen++ /* will check options with next length (+1) */
		} else if oi+oplen <= len(option) &&
			conv[:oplen] == option[oi:oi+oplen] { /* match? */
			return conv[:oplen], oplen
		}
	}
	/* C: lua_pushfstring(..., conv) — %s stops at first zero in format. */
	L.ArgError(1, fmt.Sprintf("invalid conversion specifier '%%%s'",
		luaCStr(conv)))
	return "", 0
}

// osStrftimeYearSafe reports whether t's calendar year fits in a C int
// (glibc strftime tm_year+1900 in int for %Y/%C).
func osStrftimeYearSafe(t time.Time) bool {
	y := t.Year()
	return y >= math.MinInt32 && y <= math.MaxInt32
}

// osFormatYear4 renders a year the way C's '%Y' does (sign is inside width).
func osFormatYear4(year int) string {
	return fmt.Sprintf("%04d", year)
}

// osFormatYear2 renders a year as two digits the way C's '%y' / '%g' do.
func osFormatYear2(year int) string {
	r := year % 100
	if r < 0 {
		r = -r
	}
	return fmt.Sprintf("%02d", r)
}

// osFormatSpec formats one conversion specifier (hand-roll path; English).
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
		return t.Format("-0700")
	case 'Z':
		/* Under '!': Go UTC names "UTC"; glibc gmtime leaves "GMT".
		   With cgo, libc path matches host; hand-roll keeps Go — FLAG. */
		name, _ := t.Zone()
		if name == "" {
			return t.Format("MST")
		}
		return name
	case '%':
		return "%"
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

// SIZETIMEFMT is the C prep-buffer hint; hand-roll / cgo write into Builder.
const sizeTimeFmt = 250

// osStrftime formats a time according to a strftime-style format string.
// Walk matches C os_date: while s < se, non-'%' char-by-char; on '%' →
// checkoption + strftime/hand-roll.  utc matches leading '!' of os.date.
func osStrftime(L *State, t time.Time, s string, utc bool) string {
	useC := osStrftimeC != nil && osStrftimeYearSafe(t)
	var out strings.Builder
	i := 0
	se := len(s)
	for i < se {
		if s[i] != '%' { /* not a conversion specifier? */
			out.WriteByte(s[i])
			i++
			continue
		}
		i++                                      /* skip '%' */
		spec, n := osCheckOption(L, s[i:], se-i) /* copy specifier */
		i += n
		_ = sizeTimeFmt /* C luaL_prepbuffsize(&b, SIZETIMEFMT) */
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

/*
osCheckTime is l_checktime (loslib.c 5.4.9):

	l_timet t = l_gettime(L, arg);
	luaL_argcheck(L, (time_t)t == t, arg, "time out-of-bounds");

Default build: l_timet = lua_Integer (not LUA_NUMTIME).
Target is Linux LP64 (time_t == int64), so every lua_Integer fits —
the cast check is vacuous (FLAG / LP64).
*/
func osCheckTime(L *State, arg int) int64 {
	t := L.CheckInteger(arg)
	L.ArgCheck(int64(t) == t, arg, "time out-of-bounds")
	return t
}

func osDate(L *State) int {
	s := L.OptString(1, "%c") /* luaL_optlstring — Go string keeps length */
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
		tm = time.Unix(t, 0).UTC() /* l_gmtime */
	} else {
		tm = time.Unix(t, 0) /* l_localtime */
	}
	/* C: if (stm == NULL) return luaL_error(..., "date result cannot…");
	   struct tm keeps the year in an int — emulate that cut-off. */
	if y := tm.Year(); y-1900 > math.MaxInt32 || y-1900 < math.MinInt32 {
		return L.Error2("date result cannot be represented in this installation")
	}
	/* strcmp(s, "*t") — C string; "*t\0junk" is the table. */
	if luaCStr(s) == "*t" {
		L.CreateTable(0, 9) /* 9 = number of fields */
		osSetAllFields(L, tm)
	} else {
		L.PushString(osStrftime(L, tm, s, utc))
	}
	return 1
}

func osTime(L *State) int {
	var t int64
	if L.IsNoneOrNil(1) { /* called without args? */
		t = time.Now().Unix() /* time(NULL) */
	} else {
		L.CheckType(1, LUA_TTABLE)
		L.SetTop(1) /* make sure table is at the top */
		/* loslib.c 5.4.9 read order: year, month, day, hour, min, sec */
		year := osGetField(L, "year", -1, 1900) + 1900
		month := osGetField(L, "month", -1, 1) + 1
		day := osGetField(L, "day", -1, 0)
		hour := osGetField(L, "hour", 12, 0)
		min := osGetField(L, "min", 0, 0)
		sec := osGetField(L, "sec", 0, 0)
		isdst := osGetBoolField(L, "isdst")
		tm := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.Local)
		/* mktime reads tm_isdst as instruction; emulate macOS-style ±1h
		   when the zone has DST rules.  glibc transition-search may
		   differ — FLAG. */
		if isdst >= 0 && tHasDSTRules(tm) {
			switch {
			case isdst > 0 && !tIsDST(tm):
				tm = tm.Add(-time.Hour)
			case isdst == 0 && tIsDST(tm):
				tm = tm.Add(time.Hour)
			}
		}
		osSetAllFields(L, tm) /* update fields with normalized values */
		t = tm.Unix()
	}
	/* C: if (t != (time_t)(l_timet)t || t == (time_t)(-1)) error;
	   cast half vacuous on LP64; -1 refusal applies to both paths. */
	if t == -1 {
		return L.Error2("time result cannot be represented in this installation")
	}
	L.PushInteger(t)
	return 1
}

func osDiffTime(L *State) int {
	t1 := osCheckTime(L, 1)
	t2 := osCheckTime(L, 2)
	L.PushNumber(float64(t1) - float64(t2)) /* C difftime → lua_Number */
	return 1
}

/* }====================================================== */

func osSetLocale(L *State) int {
	/* C: const char *l = luaL_optstring(L, 1, NULL);
	   int op = luaL_checkoption(L, 2, "all", catnames);
	   lua_pushstring(L, setlocale(cat[op], l)); */
	query := L.IsNoneOrNil(1)
	var name string
	if !query {
		name = luaCStr(L.CheckString(1))
	}
	op := L.CheckOption(2, "all", localeCategories)
	if query { /* setlocale(cat, NULL) — query */
		L.PushString(localeCurrent(op))
		return 1
	}
	if got, ok := setLocale(op, name); ok {
		L.PushString(got)
		return 1
	}
	L.PushNil() /* locale not available / setlocale returned NULL */
	return 1
}

func osExit(L *State) int {
	var status int
	if L.IsBoolean(1) {
		if L.ToBoolean(1) {
			status = 0 /* EXIT_SUCCESS */
		} else {
			status = 1 /* EXIT_FAILURE */
		}
	} else {
		status = int(L.OptInteger(1, 0)) /* EXIT_SUCCESS */
	}
	if L.ToBoolean(2) {
		L.Close()
	}
	/* C exit flushes stdio; port flushes io library files. */
	ioFlushAll()
	osExitHook(status) /* C: if (L) exit(status); */
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
	L.SetGlobal("os") /* golua open SPEC; C relies on luaL_requiref(...,1) */
	return 1
}
