// Tests of reading and showing dialog lengths.

package gui

import (
	"math"
	"testing"
)

// TestParseLength checks the units, a bare number as inches, and junk.
func TestParseLength(t *testing.T) {
	for in, want := range map[string]float64{
		"1.5": 108, `1.5"`: 108, "1.5 in": 108, "2 inches": 144, " 1 inch ": 72, "2.54 cm": 72,
		"25.4mm": 72, "108 pt": 108, "9 pi": 108, "0": 0, "-0.5": -36, "1.5”": 108,
	} {
		got, ok := ParseLength(in)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("ParseLength(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "abc", "1.5 ft", `"`, "pt"} {
		if v, ok := ParseLength(in); ok {
			t.Errorf("ParseLength(%q) = %v", in, v)
		}
	}
}

// TestFormatInches checks Word's inch display.
func TestFormatInches(t *testing.T) {
	for pt, want := range map[float64]string{108: `1.5"`, 72: `1"`, 0: `0"`, 9.36: `0.13"`, 100: `1.39"`, -36: `-0.5"`} {
		if got := FormatInches(pt); got != want {
			t.Errorf("FormatInches(%v) = %q, want %q", pt, got, want)
		}
	}
}
