// Tests for column width conversion, column names and view clamping.

package model

import "testing"

// TestColumnWidths checks character-to-pixel column widths for several
// digit widths, and the default column width.
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

// TestColNameLimits checks that ColName doesn't panic on the columns a
// damaged file can hold, that ParseColName doesn't overflow on long letter
// runs, and that ClampView brings a wild saved view within the limits.
func TestColNameLimits(t *testing.T) {
	for col, want := range map[int]string{0: "A", 27: "AB", 16383: "XFD", -1: "", 475253: "ZZZZ", 475254: "AAAAA"} {
		if got := ColName(col); got != want {
			t.Errorf("ColName(%d) = %q, want %q", col, got, want)
		}
	}
	ColName(int(^uint(0) >> 1))
	if got := ParseColName("XFD"); got != 16383 {
		t.Errorf("ParseColName(XFD) = %d", got)
	}
	if got := ParseColName("ZZZZZZZZZZZZZZZZZZZZ"); got != -1 {
		t.Errorf("ParseColName(20 Zs) = %d", got)
	}
	sh := NewSheet("S")
	sh.ActiveRow, sh.ActiveCol, sh.TopRow, sh.LeftCol, sh.FreezeRows, sh.FreezeCols = -3, 1<<40, MaxRows, 5, -1, MaxCols
	sh.ClampView()
	if sh.ActiveRow != 0 || sh.ActiveCol != MaxCols-1 || sh.TopRow != MaxRows-1 || sh.LeftCol != 5 || sh.FreezeRows != 0 || sh.FreezeCols != MaxCols-1 {
		t.Errorf("ClampView = %+v", sh)
	}
}
