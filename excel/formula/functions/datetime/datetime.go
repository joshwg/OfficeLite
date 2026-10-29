// Package datetime implements Excel's date and time functions: DATE,
// DATEDIF, DATEVALUE, DAY, DAYS, DAYS360, EDATE, EOMONTH, HOUR, ISOWEEKNUM,
// MINUTE, MONTH, NETWORKDAYS, NETWORKDAYS.INTL, NOW, SECOND, TIME,
// TIMEVALUE, TODAY, WEEKDAY, WEEKNUM, WORKDAY, WORKDAY.INTL, YEAR and
// YEARFRAC.
//
// Dates are serial numbers in the workbook's date system (1900, with the
// fictitious 1900-02-29 as serial 60, or 1904). Calendar fields always go
// through f.SerialToYMD and integer calendar arithmetic so the 1900 leap
// bug behaves as in Excel, and weekdays are computed from serials so that
// Excel's 1900-01-01 is a Sunday.
package datetime

import (
	"math"
	"strings"
	"time"

	f "officelite/excel/formula"
)

const (
	dateFormat     = "m/d/yyyy"
	dateTimeFormat = "m/d/yyyy h:mm"
	timeFormat     = "h:mm AM/PM"
)

// Register adds the date and time functions to r.
func Register(r *f.Registry) {
	r.Add(
		f.Spec{Name: "DATE", Min: 3, Max: 3, Fn: dateFn, Format: dateFormat},
		f.Spec{Name: "DATEDIF", Min: 3, Max: 3, Fn: datedif},
		f.Spec{Name: "DATEVALUE", Min: 1, Max: 1, Fn: datevalue, Format: dateFormat},
		f.Spec{Name: "DAY", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return field(e, a[0], func(y, m, d int) int { return d })
		}},
		f.Spec{Name: "MONTH", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return field(e, a[0], func(y, m, d int) int { return m })
		}},
		f.Spec{Name: "YEAR", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return field(e, a[0], func(y, m, d int) int { return y })
		}},
		f.Spec{Name: "DAYS", Min: 2, Max: 2, Fn: days},
		f.Spec{Name: "DAYS360", Min: 2, Max: 3, Fn: days360},
		f.Spec{Name: "EDATE", Min: 2, Max: 2, Fn: func(e *f.Env, a []f.Value) f.Value {
			return monthShift(e, a, false)
		}, Format: dateFormat},
		f.Spec{Name: "EOMONTH", Min: 2, Max: 2, Fn: func(e *f.Env, a []f.Value) f.Value {
			return monthShift(e, a, true)
		}, Format: dateFormat},
		f.Spec{Name: "HOUR", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return clock(e, a[0], func(s int) int { return s / 3600 })
		}},
		f.Spec{Name: "MINUTE", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return clock(e, a[0], func(s int) int { return s / 60 % 60 })
		}},
		f.Spec{Name: "SECOND", Min: 1, Max: 1, Fn: func(e *f.Env, a []f.Value) f.Value {
			return clock(e, a[0], func(s int) int { return s % 60 })
		}},
		f.Spec{Name: "ISOWEEKNUM", Min: 1, Max: 1, Fn: isoWeekNum},
		f.Spec{Name: "NOW", Min: 0, Max: 0, Volatile: true, Format: dateTimeFormat, Fn: func(e *f.Env, _ []f.Value) f.Value {
			return f.Number(f.TimeToSerial(e.Now(), e.Date1904()))
		}},
		f.Spec{Name: "TODAY", Min: 0, Max: 0, Volatile: true, Format: dateFormat, Fn: func(e *f.Env, _ []f.Value) f.Value {
			return f.Number(math.Floor(f.TimeToSerial(e.Now(), e.Date1904())))
		}},
		f.Spec{Name: "TIME", Min: 3, Max: 3, Fn: timeFn, Format: timeFormat},
		f.Spec{Name: "TIMEVALUE", Min: 1, Max: 1, Fn: timevalue, Format: timeFormat},
		f.Spec{Name: "WEEKDAY", Min: 1, Max: 2, Fn: weekday},
		f.Spec{Name: "WEEKNUM", Min: 1, Max: 2, Fn: weeknum},
		f.Spec{Name: "NETWORKDAYS", Min: 2, Max: 3, Arrays: f.Only(2), Fn: func(e *f.Env, a []f.Value) f.Value {
			return networkdays(e, a[0], a[1], f.Missing{}, opt(a, 2))
		}},
		f.Spec{Name: "NETWORKDAYS.INTL", Min: 2, Max: 4, Arrays: f.Only(3), Fn: func(e *f.Env, a []f.Value) f.Value {
			return networkdays(e, a[0], a[1], opt(a, 2), opt(a, 3))
		}},
		f.Spec{Name: "WORKDAY", Min: 2, Max: 3, Arrays: f.Only(2), Format: dateFormat, Fn: func(e *f.Env, a []f.Value) f.Value {
			return workday(e, a[0], a[1], f.Missing{}, opt(a, 2))
		}},
		f.Spec{Name: "WORKDAY.INTL", Min: 2, Max: 4, Arrays: f.Only(3), Format: dateFormat, Fn: func(e *f.Env, a []f.Value) f.Value {
			return workday(e, a[0], a[1], opt(a, 2), opt(a, 3))
		}},
		f.Spec{Name: "YEARFRAC", Min: 2, Max: 3, Fn: yearfrac},
	)
}

// --- helpers ------------------------------------------------------------

// opt returns argument i, or Missing when it was not given.
func opt(a []f.Value, i int) f.Value {
	if i < len(a) {
		return a[i]
	}
	return f.Missing{}
}

func missing(v f.Value) bool {
	_, ok := v.(f.Missing)
	return ok
}

// maxSerial is 9999-12-31 in the workbook's date system.
func maxSerial(e *f.Env) float64 {
	if e.Date1904() {
		return f.MaxSerial - f.Offset1904
	}
	return f.MaxSerial
}

// serial coerces a date argument to a whole serial number (Excel truncates
// the time of day). Negative or too-large serials are #NUM!.
func serial(e *f.Env, v f.Value) (float64, f.Value) {
	n, err := e.Num(v)
	if err != nil {
		return 0, err
	}
	d := math.Floor(n)
	if d < 0 || d > maxSerial(e) {
		return 0, f.ErrNum
	}
	return d, nil
}

func ymd(e *f.Env, s float64) (y, m, d int) { return f.SerialToYMD(s, e.Date1904()) }

// daysIn is the length of a month in Excel's calendar (February 1900 has
// 29 days in the 1900 date system).
func daysIn(e *f.Env, y, m int) int {
	if y == 1900 && m == 2 && !e.Date1904() {
		return 29
	}
	return time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// mkDate returns the serial for year y, month m (which may be outside
// 1-12 and rolls over) and day d (which may roll over too). Unlike DATE,
// years are taken literally: a result before 1900 or after 9999 fails.
func mkDate(e *f.Env, y, m, d int) (float64, bool) {
	t := y*12 + m - 1
	y, m = f.FloorDiv(t, 12), t-f.FloorDiv(t, 12)*12+1
	if y < 1900 || y > 9999 {
		return 0, false
	}
	// Counting days from the first of the month lands on Excel's serials
	// even around the fictitious 1900-02-29 (DATE(1900,3,0) = 60).
	s := float64(firstOfMonth(e, y, m) + d - 1)
	if s < 0 || s > maxSerial(e) {
		return 0, false
	}
	return s, true
}

// firstOfMonth is the serial of the first day of a month.
func firstOfMonth(e *f.Env, y, m int) int {
	return int(f.TimeToSerial(time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC), e.Date1904()))
}

// dow is the day of the week of a serial, Monday = 0 ... Sunday = 6, as
// Excel computes it (serial 1 in the 1900 system is a Sunday).
func dow(e *f.Env, s float64) int {
	n := int(s)
	if e.Date1904() {
		n += f.Offset1904
	}
	return (n%7 + 7 + 5) % 7
}

// --- DATE, DATEVALUE, DAY/MONTH/YEAR, DAYS ------------------------------

// DATE(year, month, day)
func dateFn(e *f.Env, a []f.Value) f.Value {
	var p [3]int
	for i := range p {
		n, err := f.Int(a[i])
		if err != nil {
			return err
		}
		p[i] = n
	}
	if p[0] < 0 || p[0] > 9999 {
		return f.ErrNum
	}
	if p[0] < 1900 {
		p[0] += 1900 // DATE(108,1,2) is 2008-01-02
	}
	s, ok := mkDate(e, p[0], p[1], p[2])
	if !ok {
		return f.ErrNum
	}
	return f.Number(s)
}

// DATEVALUE(date_text): only text is accepted, and the time is dropped.
func datevalue(e *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case f.Error:
		return v
	case f.String:
		if d, ok := f.ParseDateTime(string(v), e.Date1904()); ok {
			return f.Number(math.Floor(d))
		}
	}
	return f.ErrValue
}

func field(e *f.Env, v f.Value, pick func(y, m, d int) int) f.Value {
	s, err := serial(e, v)
	if err != nil {
		return err
	}
	return f.Number(pick(ymd(e, s)))
}

// DAYS(end_date, start_date)
func days(e *f.Env, a []f.Value) f.Value {
	end, err := serial(e, a[0])
	if err != nil {
		return err
	}
	start, err := serial(e, a[1])
	if err != nil {
		return err
	}
	return f.Number(end - start)
}

// --- DATEDIF ------------------------------------------------------------

// DATEDIF(start_date, end_date, unit)
func datedif(e *f.Env, a []f.Value) f.Value {
	start, err := serial(e, a[0])
	if err != nil {
		return err
	}
	end, err := serial(e, a[1])
	if err != nil {
		return err
	}
	unit, err := f.Str(a[2])
	if err != nil {
		return err
	}
	if start > end {
		return f.ErrNum
	}
	sy, sm, sd := ymd(e, start)
	ey, em, ed := ymd(e, end)
	months := (ey-sy)*12 + em - sm
	if ed < sd {
		months-- // the last month is not complete
	}
	switch strings.ToUpper(unit) {
	case "Y":
		return f.Number(months / 12)
	case "M":
		return f.Number(months)
	case "D":
		return f.Number(end - start)
	case "YM":
		return f.Number(months % 12)
	case "MD":
		if ed >= sd {
			return f.Number(ed - sd)
		}
		// Excel counts from start's day in the month before end's month,
		// letting that day overflow; this gives its documented negative
		// results, e.g. 2015-01-31 to 2015-03-01 is -2.
		py, pm := prevMonth(ey, em)
		return f.Number(ed - sd + daysIn(e, py, pm))
	case "YD":
		s, ok := mkDate(e, ey, sm, sd)
		if !ok || s > end {
			s, ok = mkDate(e, ey-1, sm, sd)
		}
		if !ok {
			return f.ErrNum
		}
		return f.Number(end - s)
	}
	return f.ErrNum
}

func prevMonth(y, m int) (int, int) {
	if m == 1 {
		return y - 1, 12
	}
	return y, m - 1
}

// --- DAYS360 ------------------------------------------------------------

// DAYS360(start_date, end_date, [method])
func days360(e *f.Env, a []f.Value) f.Value {
	start, err := serial(e, a[0])
	if err != nil {
		return err
	}
	end, err := serial(e, a[1])
	if err != nil {
		return err
	}
	european, err := f.Truth(opt(a, 2))
	if err != nil {
		return err
	}
	sy, sm, sd := ymd(e, start)
	ey, em, ed := ymd(e, end)
	if european {
		sd, ed = min(sd, 30), min(ed, 30)
	} else {
		// US (NASD): a start on the last day of the month becomes the
		// 30th; an end on the 31st becomes the 1st of the next month
		// unless the start was on or after the 30th.
		if sd == daysIn(e, sy, sm) {
			sd = 30
		}
		if ed == 31 {
			if sd < 30 {
				ed, em = 1, em+1
			} else {
				ed = 30
			}
		}
	}
	return f.Number((ey-sy)*360 + (em-sm)*30 + ed - sd)
}

// --- EDATE, EOMONTH -----------------------------------------------------

// EDATE(start_date, months) and EOMONTH(start_date, months)
func monthShift(e *f.Env, a []f.Value, eom bool) f.Value {
	start, err := serial(e, a[0])
	if err != nil {
		return err
	}
	n, err := f.Int(a[1])
	if err != nil {
		return err
	}
	y, m, d := ymd(e, start)
	t := y*12 + m - 1 + n
	if t < 1900*12 || t >= 10000*12 {
		return f.ErrNum
	}
	y, m = t/12, t%12+1
	if dim := daysIn(e, y, m); eom || d > dim {
		d = dim // EDATE clamps to the end of a shorter month
	}
	s, ok := mkDate(e, y, m, d)
	if !ok {
		return f.ErrNum
	}
	return f.Number(s)
}

// --- time of day --------------------------------------------------------

// clock implements HOUR, MINUTE and SECOND on the time of day rounded to
// the nearest second.
func clock(e *f.Env, v f.Value, pick func(secs int) int) f.Value {
	n, err := e.Num(v)
	if err != nil {
		return err
	}
	if n < 0 || n >= maxSerial(e)+1 {
		return f.ErrNum
	}
	secs := int(math.Round((n - math.Floor(n)) * 86400))
	return f.Number(pick(secs % 86400))
}

// TIME(hour, minute, second): values roll over, the result is the
// fraction of a day (whole days are dropped).
func timeFn(e *f.Env, a []f.Value) f.Value {
	var p [3]int
	for i := range p {
		n, err := f.Int(a[i])
		if err != nil {
			return err
		}
		if n > 32767 {
			return f.ErrNum
		}
		p[i] = n
	}
	total := p[0]*3600 + p[1]*60 + p[2]
	if total < 0 {
		return f.ErrNum
	}
	return f.Number(float64(total%86400) / 86400)
}

// TIMEVALUE(time_text): only text is accepted, and any date is dropped.
func timevalue(e *f.Env, a []f.Value) f.Value {
	switch v := a[0].(type) {
	case f.Error:
		return v
	case f.String:
		if t, ok := f.ParseDateTime(string(v), e.Date1904()); ok {
			return f.Number(t - math.Floor(t))
		}
	}
	return f.ErrValue
}

// --- weeks --------------------------------------------------------------

// WEEKDAY(serial_number, [return_type])
func weekday(e *f.Env, a []f.Value) f.Value {
	s, err := serial(e, a[0])
	if err != nil {
		return err
	}
	typ := 1
	if !missing(opt(a, 1)) {
		if typ, err = f.Int(a[1]); err != nil {
			return err
		}
	}
	d := dow(e, s)
	switch {
	case typ == 1:
		return f.Number((d+1)%7 + 1)
	case typ == 2:
		return f.Number(d + 1)
	case typ == 3:
		return f.Number(d)
	case typ >= 11 && typ <= 17: // 11 = Monday is 1 ... 17 = Sunday is 1
		return f.Number((d-(typ-11)+7)%7 + 1)
	}
	return f.ErrNum
}

// WEEKNUM(serial_number, [return_type]): week 1 contains January 1st and
// weeks begin on the day given by return_type; 21 is the ISO week.
func weeknum(e *f.Env, a []f.Value) f.Value {
	s, err := serial(e, a[0])
	if err != nil {
		return err
	}
	typ := 1
	if !missing(opt(a, 1)) {
		if typ, err = f.Int(a[1]); err != nil {
			return err
		}
	}
	var first int // first day of the week, Monday = 0
	switch {
	case typ == 1:
		first = 6
	case typ == 2:
		first = 0
	case typ >= 11 && typ <= 17:
		first = typ - 11
	case typ == 21:
		return isoWeek(e, s)
	default:
		return f.ErrNum
	}
	y, _, _ := ymd(e, s)
	jan1, ok := mkDate(e, y, 1, 1)
	if !ok {
		jan1 = 0 // 1904-01-01 or the 1900 system's day 0
	}
	shift := (dow(e, jan1) - first + 7) % 7
	return f.Number((int(s-jan1)+shift)/7 + 1)
}

// ISOWEEKNUM(date)
func isoWeekNum(e *f.Env, a []f.Value) f.Value {
	s, err := serial(e, a[0])
	if err != nil {
		return err
	}
	return isoWeek(e, s)
}

// isoWeek is the ISO 8601 week: weeks start on Monday and week 1 holds the
// year's first Thursday.
func isoWeek(e *f.Env, s float64) f.Value {
	thu := s - float64(dow(e, s)) + 3
	if thu < 0 || (thu < 1 && !e.Date1904()) {
		// Before the first serial: Excel's late December 1899 (week 52)
		// or 1903 (week 53).
		if e.Date1904() {
			return f.Number(53)
		}
		return f.Number(52)
	}
	y, _, _ := ymd(e, thu)
	jan1, ok := mkDate(e, y, 1, 1)
	if !ok {
		jan1 = 0
	}
	return f.Number(int(thu-jan1)/7 + 1)
}
