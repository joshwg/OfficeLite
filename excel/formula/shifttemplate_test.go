// Tests for ShiftTemplate: its Shift is formula.Shift's, offset by offset.

package formula

import "testing"

// TestShiftTemplateMatchesShift checks that a template shifts every formula
// to the same text Shift does, references moved off the sheet, literals,
// sheet names and names included.
func TestShiftTemplateMatchesShift(t *testing.T) {
	formulas := []string{
		"", "1+2", "A1", "$A$1", "A$1+$A1", "SUM(A1:B2)*C3", "A:A+$B:B+1:1+$2:$2",
		`Sheet1!A1+'My Sheet'!B2:C3+LEN("A1 B2")+#REF!`, "XFD1048576+A1", "LOG10(A1)+ABC12X",
		"Data!Name+A1", "A1:A1048576",
	}
	offsets := [][2]int{{0, 0}, {1, 0}, {0, 1}, {-1, -1}, {5, 3}, {-3, 2}, {1048575, 0}, {0, 16383}}
	for _, f := range formulas {
		tm := NewShiftTemplate(f)
		for _, o := range offsets {
			if got, want := tm.Shift(o[0], o[1]), Shift(f, o[0], o[1]); got != want {
				t.Errorf("%q by %v: %q, want %q", f, o, got, want)
			}
		}
	}
}
