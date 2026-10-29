// NETWORKDAYS, WORKDAY and their .INTL forms: weekend masks, holiday sets
// and working-day counting.

package datetime

import (
	f "officelite/excel/formula"
)

// weekend marks the non-working days of the week, Monday = 0.
type weekend [7]bool

// parseWeekend reads the weekend argument of NETWORKDAYS.INTL and
// WORKDAY.INTL: a number (1-7 for two-day weekends starting Saturday,
// Sunday ... Friday; 11-17 for a single day, Sunday ... Saturday) or a
// seven-character mask such as "0000011" (Monday first, 1 = weekend).
func parseWeekend(v f.Value) (weekend, f.Value) {
	var w weekend
	switch x := v.(type) {
	case f.Missing:
		w[5], w[6] = true, true
		return w, nil
	case f.String:
		if len(x) != 7 {
			return w, f.ErrValue
		}
		for i, c := range string(x) {
			switch c {
			case '1':
				w[i] = true
			case '0':
			default:
				return w, f.ErrValue
			}
		}
		return w, nil
	}
	n, err := f.Int(v)
	if err != nil {
		return w, err
	}
	switch {
	case n >= 1 && n <= 7: // 1 = Saturday+Sunday, 2 = Sunday+Monday ...
		w[(n+4)%7], w[(n+5)%7] = true, true
	case n >= 11 && n <= 17: // 11 = Sunday only, 12 = Monday only ...
		w[(n-5)%7] = true
	default:
		return w, f.ErrNum
	}
	return w, nil
}

// holidaySet collects the holiday dates from a range, array or scalar.
// Blank cells are skipped; date text is accepted ({"2006/1/2"}).
func holidaySet(e *f.Env, v f.Value) (map[float64]bool, f.Value) {
	set := map[float64]bool{}
	if missing(v) {
		return set, nil
	}
	var bad f.Value
	e.Each(v, func(x f.Value, _ bool) bool {
		switch x.(type) {
		case f.Blank, f.Missing:
			return true
		case f.Bool:
			bad = f.ErrValue
			return false
		}
		d, err := serial(e, x)
		if err != nil {
			bad = err
			return false
		}
		set[d] = true
		return true
	})
	return set, bad
}

// networkdays implements NETWORKDAYS(start_date, end_date, [holidays]) and
// NETWORKDAYS.INTL(start_date, end_date, [weekend], [holidays]): working
// days between the dates inclusive, negative when start_date is later.
func networkdays(e *f.Env, startV, endV, weekendV, holV f.Value) f.Value {
	start, err := serial(e, startV)
	if err != nil {
		return err
	}
	end, err := serial(e, endV)
	if err != nil {
		return err
	}
	w, err := parseWeekend(weekendV)
	if err != nil {
		return err
	}
	hol, err := holidaySet(e, holV)
	if err != nil {
		return err
	}
	sign := 1
	if start > end {
		start, end, sign = end, start, -1
	}
	perWeek := 0
	for _, off := range w {
		if !off {
			perWeek++
		}
	}
	n := int(end-start) + 1
	count := n / 7 * perWeek
	for d := start + float64(n/7*7); d <= end; d++ {
		if !w[dow(e, d)] {
			count++
		}
	}
	for h := range hol {
		if h >= start && h <= end && !w[dow(e, h)] {
			count--
		}
	}
	return f.Number(sign * count)
}

// workday implements WORKDAY(start_date, days, [holidays]) and
// WORKDAY.INTL(start_date, days, [weekend], [holidays]): the date that is
// the given number of working days before or after start_date.
func workday(e *f.Env, startV, daysV, weekendV, holV f.Value) f.Value {
	start, err := serial(e, startV)
	if err != nil {
		return err
	}
	n, err := f.Int(daysV)
	if err != nil {
		return err
	}
	w, err := parseWeekend(weekendV)
	if err != nil {
		return err
	}
	if w == (weekend{true, true, true, true, true, true, true}) {
		return f.ErrValue
	}
	hol, err := holidaySet(e, holV)
	if err != nil {
		return err
	}
	step := 1.0
	if n < 0 {
		step, n = -1, -n
	}
	limit := maxSerial(e)
	if float64(n) > limit {
		return f.ErrNum
	}
	d := start
	for n > 0 {
		d += step
		if d < 0 || d > limit {
			return f.ErrNum
		}
		if !w[dow(e, d)] && !hol[d] {
			n--
		}
	}
	return f.Number(d)
}
