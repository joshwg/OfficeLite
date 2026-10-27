package formula

import "testing"

func TestShift(t *testing.T) {
	for _, c := range []struct {
		in         string
		dRow, dCol int
		want       string
	}{
		{"A1*2", 1, 0, "A2*2"},
		{"SUM(A1:B2)", 2, 1, "SUM(B3:C4)"},
		{"$A$1+A$1+$A1", 3, 3, "$A$1+D$1+$A4"},
		{`"A1"&A1`, 1, 0, `"A1"&A2`},
		{"'My A1'!B2", 1, 1, "'My A1'!C3"},
		{"Sheet2!A1", 1, 0, "Sheet2!A2"},
		{"LOG10(A1)", 1, 0, "LOG10(A2)"},
		{"A1", -1, 0, "#REF!"},
		{"Z1", 0, 1, "AA1"},
	} {
		if got := Shift(c.in, c.dRow, c.dCol); got != c.want {
			t.Errorf("Shift(%q, %d, %d) = %q, want %q", c.in, c.dRow, c.dCol, got, c.want)
		}
	}
}
