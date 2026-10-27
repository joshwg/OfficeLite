package formula

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Excel stores dates as serial day numbers. In the 1900 system day 1 is
// 1900-01-01 and day 60 is the nonexistent 1900-02-29 (kept for Lotus 1-2-3
// compatibility), so from day 61 on serial = days since 1899-12-30. In the
// 1904 system day 0 is 1904-01-01. The fraction is the time of day.

var (
	epoch1900 = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	epoch1904 = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
)

// MaxSerial is 9999-12-31 in the 1900 system; Excel's last valid date.
const MaxSerial = 2958465

// SerialToTime converts a serial number to a UTC time. Serial 60 in the
// 1900 system (the fictitious 1900-02-29) maps to 1900-03-01; use
// SerialToYMD for Excel's exact calendar fields.
func SerialToTime(serial float64, date1904 bool) time.Time {
	days := math.Floor(serial)
	frac := serial - days
	var t time.Time
	if date1904 {
		t = epoch1904.AddDate(0, 0, int(days))
	} else {
		if days < 61 {
			days++ // before the fake leap day the offset is one less
		}
		t = epoch1900.AddDate(0, 0, int(days))
	}
	ms := math.Round(frac * 86400000)
	return t.Add(time.Duration(ms) * time.Millisecond)
}

// SerialToYMD returns Excel's calendar fields for a serial number,
// including 1900-02-29 for serial 60 and 1900-01-00 for serial 0.
func SerialToYMD(serial float64, date1904 bool) (year, month, day int) {
	d := math.Floor(serial)
	if !date1904 {
		switch d {
		case 60:
			return 1900, 2, 29
		case 0:
			return 1900, 1, 0
		}
	}
	t := SerialToTime(d, date1904)
	return t.Year(), int(t.Month()), t.Day()
}

// TimeToSerial converts a time (its wall-clock fields) to a serial number.
func TimeToSerial(t time.Time, date1904 bool) float64 {
	// Whole days via Unix seconds: time.Duration overflows after ~292 years.
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	epoch := epoch1900
	if date1904 {
		epoch = epoch1904
	}
	days := float64((day.Unix() - epoch.Unix()) / 86400)
	frac := (float64(t.Hour()*3600+t.Minute()*60+t.Second()) + float64(t.Nanosecond())/1e9) / 86400
	if !date1904 && days < 61 {
		days--
	}
	return days + frac
}

// DateSerial implements DATE(year, month, day): years 0-1899 are offset by
// 1900, and months/days outside their range roll over. It reports false for
// results outside Excel's date range.
func DateSerial(year, month, day int, date1904 bool) (float64, bool) {
	if year < 0 || year > 9999 {
		return 0, false
	}
	if year < 1900 {
		year += 1900
	}
	// Normalize the month first so that DATE(2026,14,1) is 2027-02-01.
	y := year + floorDiv(month-1, 12)
	m := (month-1)%12 + 1
	if m <= 0 {
		m += 12
	}
	first := time.Date(y, time.Month(m), 1, 0, 0, 0, 0, time.UTC)
	// Counting days from the first of the month lands on Excel's serials
	// even around the fictitious 1900-02-29 (DATE(1900,2,29) = 60).
	s := TimeToSerial(first, date1904) + float64(day-1)
	if s < 0 { // the upper end is limited by year <= 9999 above
		return 0, false
	}
	return s, true
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

var monthNames = map[string]int{
	"jan": 1, "january": 1, "feb": 2, "february": 2, "mar": 3, "march": 3,
	"apr": 4, "april": 4, "may": 5, "jun": 6, "june": 6, "jul": 7, "july": 7,
	"aug": 8, "august": 8, "sep": 9, "sept": 9, "september": 9, "oct": 10,
	"october": 10, "nov": 11, "november": 11, "dec": 12, "december": 12,
}

var (
	reTime  = regexp.MustCompile(`(?i)^(\d{1,2})(?::(\d{1,2}))?(?::(\d{1,2}(?:\.\d+)?))?\s*(am|pm|a|p)?$`)
	reMDY   = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})(?:[/-](\d{2}|\d{4}))?$`)
	reYMD   = regexp.MustCompile(`^(\d{4})[/-](\d{1,2})[/-](\d{1,2})$`)
	reDMonY = regexp.MustCompile(`(?i)^(\d{1,2})[-\s/]([a-z]+)\.?(?:[-\s/,]+(\d{2}|\d{4}))?$`)
	reMonDY = regexp.MustCompile(`(?i)^([a-z]+)\.?[-\s/]+(\d{1,2})(?:[-\s/,]+(\d{2}|\d{4}))?$`)
	reMonY  = regexp.MustCompile(`(?i)^([a-z]+)\.?[-\s/]+(\d{4})$`)
)

// ParseDateTime parses date and/or time text as Excel's DATEVALUE,
// TIMEVALUE and text-to-number coercion do (en-US conventions). Date-only
// text gives a whole serial, time-only text a fraction of a day.
func ParseDateTime(s string, date1904 bool) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if t, ok := parseTimeOfDay(s); ok {
		return t, true
	}
	// Split off a trailing time: "1/15/2026 3:45 PM".
	if i := strings.IndexByte(s, ':'); i > 0 {
		if sp := strings.LastIndexAny(s[:i], " T"); sp > 0 {
			d, ok1 := parseDate(strings.TrimSpace(s[:sp]), date1904)
			t, ok2 := parseTimeOfDay(strings.TrimSpace(s[sp+1:]))
			if ok1 && ok2 {
				return d + t, true
			}
			return 0, false
		}
	}
	return parseDate(s, date1904)
}

func parseTimeOfDay(s string) (float64, bool) {
	m := reTime.FindStringSubmatch(s)
	if m == nil || (m[2] == "" && m[4] == "") {
		return 0, false // a bare number is not a time
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	sec, _ := strconv.ParseFloat(m[3], 64)
	if ap := strings.ToLower(m[4]); ap != "" {
		if h == 0 || h > 12 {
			return 0, false
		}
		if h == 12 {
			h = 0
		}
		if ap[0] == 'p' {
			h += 12
		}
	}
	if mi > 59 || sec >= 60 || h > 9999 {
		return 0, false
	}
	return (float64(h)*3600 + float64(mi)*60 + sec) / 86400, true
}

func parseDate(s string, date1904 bool) (float64, bool) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ","))
	year := func(y string) int {
		if y == "" {
			return time.Now().Year()
		}
		n, _ := strconv.Atoi(y)
		if len(y) == 2 {
			if n < 30 {
				n += 2000
			} else {
				n += 1900
			}
		}
		return n
	}
	valid := func(y, m, d int) (float64, bool) {
		if m < 1 || m > 12 || d < 1 || y < 1900 || y > 9999 {
			return 0, false
		}
		dim := time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()
		if y == 1900 && m == 2 {
			dim = 29
		}
		if d > dim {
			return 0, false
		}
		return DateSerial(y, m, d, date1904)
	}
	if m := reYMD.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return valid(y, mo, d)
	}
	if m := reMDY.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		d, _ := strconv.Atoi(m[2])
		return valid(year(m[3]), mo, d)
	}
	if m := reDMonY.FindStringSubmatch(s); m != nil {
		if mo, ok := monthNames[strings.ToLower(m[2])]; ok {
			d, _ := strconv.Atoi(m[1])
			return valid(year(m[3]), mo, d)
		}
	}
	if m := reMonDY.FindStringSubmatch(s); m != nil {
		if mo, ok := monthNames[strings.ToLower(m[1])]; ok {
			d, _ := strconv.Atoi(m[2])
			if m[3] == "" && d > 31 {
				return 0, false
			}
			return valid(year(m[3]), mo, d)
		}
	}
	if m := reMonY.FindStringSubmatch(s); m != nil {
		if mo, ok := monthNames[strings.ToLower(m[1])]; ok {
			return valid(year(m[2]), mo, 1)
		}
	}
	return 0, false
}
