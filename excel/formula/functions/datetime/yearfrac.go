package datetime

import (
	f "officelite/excel/formula"
)

// date is a calendar date in Excel's calendar.
type date struct{ y, m, d int }

// YEARFRAC(start_date, end_date, [basis]): the fraction of a year between
// two dates under a day-count basis (0 US 30/360, 1 actual/actual,
// 2 actual/360, 3 actual/365, 4 European 30/360). The dates are swapped
// when start_date is later.
func yearfrac(e *f.Env, a []f.Value) f.Value {
	start, err := serial(e, a[0])
	if err != nil {
		return err
	}
	end, err := serial(e, a[1])
	if err != nil {
		return err
	}
	basis, err := f.Int(opt(a, 2))
	if err != nil {
		return err
	}
	if basis < 0 || basis > 4 {
		return f.ErrNum
	}
	if start > end {
		start, end = end, start
	}
	var s, t date
	s.y, s.m, s.d = ymd(e, start)
	t.y, t.m, t.d = ymd(e, end)
	days := end - start
	switch basis {
	case 0:
		return f.Number(us30360(e, s, t))
	case 1:
		return f.Number(days / actualYear(e, s, t))
	case 2:
		return f.Number(days / 360)
	case 3:
		return f.Number(days / 365)
	}
	// European 30/360: the 31st counts as the 30th.
	return f.Number(float64((t.y-s.y)*360+(t.m-s.m)*30+min(t.d, 30)-min(s.d, 30)) / 360)
}

func (x date) lastOfMonth(e *f.Env) bool { return x.d == daysIn(e, x.y, x.m) }

func leap(e *f.Env, y int) bool { return daysIn(e, y, 2) == 29 }

// us30360 is YEARFRAC's basis 0, which adjusts month ends slightly
// differently from DAYS360's US method (an end on the 31st stays the 31st
// when the start is before the 30th; February month ends count as the
// 30th).
func us30360(e *f.Env, s, t date) float64 {
	sd, td := s.d, t.d
	switch {
	case sd == 31 && td == 31:
		sd, td = 30, 30
	case sd == 31:
		sd = 30
	case sd == 30 && td == 31:
		td = 30
	case s.m == 2 && s.lastOfMonth(e):
		sd = 30
		if t.m == 2 && t.lastOfMonth(e) {
			td = 30
		}
	}
	return float64((t.y-s.y)*360+(t.m-s.m)*30+td-sd) / 360
}

// actualYear is the year length YEARFRAC's basis 1 divides by: the average
// length of the years spanned when the dates are more than a year apart,
// otherwise 366 if a February 29th falls in the period (or both dates are
// in the same leap year) and 365 if not.
func actualYear(e *f.Env, s, t date) float64 {
	if s.y != t.y && (s.y+1 != t.y || s.m < t.m || (s.m == t.m && s.d < t.d)) {
		total := 0
		for y := s.y; y <= t.y; y++ {
			total += 365
			if leap(e, y) {
				total++
			}
		}
		return float64(total) / float64(t.y-s.y+1)
	}
	switch {
	case leap(e, s.y):
		if s.y == t.y || s.m <= 2 {
			return 366
		}
	case leap(e, t.y):
		if t.m > 2 || (t.m == 2 && t.d == 29) {
			return 366
		}
	}
	return 365
}
