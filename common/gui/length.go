// Lengths in dialogs as Word shows them (inches: 1.5") and as people type
// them (1.5, 1.5", 1.5 in, 3.8 cm, 38 mm, 108 pt, 9 pi).

package gui

import (
	"strconv"
	"strings"
)

// lengthUnits are the units ParseLength takes, in points each, longest
// spelling first so "inches" isn't read as "in".
var lengthUnits = []struct {
	name string
	pt   float64
}{
	{"inches", 72}, {"inch", 72}, {"in", 72}, {`"`, 72}, {"”", 72},
	{"cm", 72 / 2.54}, {"mm", 72 / 25.4}, {"pt", 1}, {"pi", 12},
}

// FormatInches shows a length in points as inches with at most two
// decimals, as Word's dialogs do: 108 → 1.5".
func FormatInches(pt float64) string {
	return strconv.FormatFloat(roundTo(pt/72, 2), 'f', -1, 64) + `"`
}

// roundTo rounds v to n decimals.
func roundTo(v float64, n int) float64 {
	p := 1.0
	for range n {
		p *= 10
	}
	r := float64(int64(v*p+0.5)) / p
	if v < 0 {
		r = -float64(int64(-v*p+0.5)) / p
	}
	return r
}

// ParseLength reads a typed length into points: a number with a unit
// (", in, cm, mm, pt or pi), or a bare number taken as inches. It reports
// false for anything else.
func ParseLength(s string) (float64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	per := 72.0
	for _, u := range lengthUnits {
		if rest, ok := strings.CutSuffix(s, u.name); ok {
			s, per = strings.TrimSpace(rest), u.pt
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v * per, true
}
