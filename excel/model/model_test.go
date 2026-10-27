package model

import "testing"

func TestColumnWidths(t *testing.T) {
	for _, c := range []struct {
		chars, mdw float64
		want       float32
	}{
		{9.140625, 7, 64}, // Excel's default 8.43 columns, Calibri 11
		{18.125, 7, 127},  // a 4640/256 BIFF width
		{18.125, 10, 181}, // the same width with a Verdana 12 default font
		{0, 7, 0},         // hidden
	} {
		if got := ColWidthToPx(c.chars, c.mdw); got != c.want {
			t.Errorf("ColWidthToPx(%v, %v) = %v, want %v", c.chars, c.mdw, got, c.want)
		}
	}
	sh := NewSheet("s")
	if got := sh.DefaultColWidthPx(7); got != 64 {
		t.Errorf("default width = %v, want 64", got)
	}
	if got := sh.DefaultColWidthPx(10); got != 88 {
		t.Errorf("default width at mdw 10 = %v, want 88", got)
	}
}
