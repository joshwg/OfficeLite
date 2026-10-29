// Tests for niceScale.

package render

import "testing"

// TestNiceScale checks automatic, fixed, step-limited and log axis scales.
func TestNiceScale(t *testing.T) {
	for _, c := range []struct {
		lo, hi         float64
		min, max, step float64
	}{
		{0, 97, 0, 120, 20},
		{3, 8, 0, 9, 1},
		{0, 10, 0, 12, 2},
		{1, 5, 0, 6, 1},
		{0, 100, 0, 120, 20},
		{0, 50, 0, 60, 10},
		{150, 1200, 0, 1400, 200},
		{2, 10, 0, 12, 2},
		{95, 100, 92, 101, 1},       // close together: the minimum isn't 0
		{-97, 0, -120, 0, 20},       // all negative: mirrored
		{-20, 30, -30, 40, 10},      // both signs
		{0.1, 0.9, 0, 1, 0.1},       // fractions
		{5, 5, 0, 6, 1},             // one value
		{0, 0, 0, 1, 0.1},           // only zeros
		{1000, 5000, 0, 6000, 1000}, // thousands
	} {
		s := niceScale(c.lo, c.hi, scaleOpts{})
		if !near(s.Min, c.min) || !near(s.Max, c.max) || !near(s.Major, c.step) {
			t.Errorf("%v..%v: got %v..%v by %v, want %v..%v by %v", c.lo, c.hi, s.Min, s.Max, s.Major, c.min, c.max, c.step)
		}
	}
	// Fixed bounds keep their values; the unit fits the range.
	s := niceScale(3, 8, scaleOpts{Min: 2, HasMin: true, Max: 10, HasMax: true})
	if s.Min != 2 || s.Max != 10 || s.Major != 1 {
		t.Errorf("fixed 2..10: got %v..%v by %v", s.Min, s.Max, s.Major)
	}
	// Fewer steps allowed (a short chart): a bigger unit.
	s = niceScale(0, 97, scaleOpts{MaxSteps: 4})
	if s.Max != 150 || s.Major != 50 {
		t.Errorf("4 steps: got %v..%v by %v", s.Min, s.Max, s.Major)
	}
	// Log scale: powers of the base.
	s = niceScale(3, 4500, scaleOpts{LogBase: 10})
	if !near(s.Min, 1) || !near(s.Max, 10000) || len(s.ticks()) != 5 {
		t.Errorf("log: got %v..%v ticks %v", s.Min, s.Max, s.ticks())
	}
}

// near reports whether a and b differ by less than 1e-9.
func near(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }
