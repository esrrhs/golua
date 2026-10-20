-- What mktime does with 'tm_isdst'.  It is an instruction rather than a
-- question: 1 means "read the wall clock as daylight-saving time", 0
-- means "read it as standard time", and an absent field means "work it
-- out".  When the instruction disagrees with the zone the absolute time
-- moves by the daylight-saving step while the wall clock stays put --
-- which is why 'isdst = true' on a winter date in New York answers with
-- 11 o'clock and not 12, and why the fields written back into the table
-- say 11 too.  A zone with no daylight-saving rule at all (UTC) has
-- nothing to disagree about and ignores the field.
--
-- Every line here holds in any zone, so the golden file does not depend
-- on the machine's TZ.

local function timet(day, isdst)
  local t = {year = 2023, month = 7, day = day, hour = 12, isdst = isdst}
  return os.time(t), t
end

local a, ta = timet(1, true)
local b, tb = timet(1, false)
local c = timet(1, nil)

-- a zone that knows about daylight saving answers differently for the
-- two instructions; one that does not answers the same for both
-- not printed: whether the zone knows about DST is its own business
local knowsDST = a ~= b
print("gap is an hour or nothing", a == b or math.abs(a - b) == 3600)
print("isdst true is never later", a <= b)
print("isdst absent agrees with one of them", c == a or c == b)
if knowsDST then
  print("hours written back", (tb.hour - ta.hour) % 24 == 1)
else
  print("hours written back", ta.hour == tb.hour)
end

-- the field written back says whether the *answer* is in DST
print("field is consistent", ta.isdst == (os.date("*t", a).isdst),
                            tb.isdst == (os.date("*t", b).isdst))

-- the same invariants on a second date
local d, td = timet(15, true)
local e, te = timet(15, false)
print("mid-month", d == e or math.abs(d - e) == 3600, d <= e,
      td.isdst == (os.date("*t", d).isdst))

-- an absent 'isdst' must round-trip through os.date
local t0 = os.time{year = 2023, month = 11, day = 14, hour = 22, min = 13, sec = 20}
print("round trip", os.time(os.date("*t", t0)) == t0)

-- two days are 86400 seconds apart whatever the zone
local jan1 = os.time{year = 1970, month = 1, day = 1, hour = 0, min = 0, sec = 0}
local jan2 = os.time{year = 1970, month = 1, day = 2, hour = 0, min = 0, sec = 0}
print("a day is a day", jan2 - jan1 == 86400)
print("epoch is a number", math.type(jan1) == "integer" or type(jan1) == "number")

-- out-of-range fields are normalized, not rejected
local n = {year = 2023, month = 13, day = 1, hour = 12}
os.time(n)
print("month 13", n.year, n.month, n.day)
local h = {year = 2023, month = 1, day = 1, hour = 25}
os.time(h)
print("hour 25", h.year, h.month, h.day, h.hour)

-- missing fields
print(pcall(os.time, {}))
print(pcall(os.time, {year = 2023}))
print(pcall(os.time, {year = 2023, month = 1}))
print("done")
