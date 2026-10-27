package xlsfile

import (
	"os"
	"strings"
	"testing"

	"officelite/common/colors"
	"officelite/excel/model"
)

func TestReadSample(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.xls")
	if err != nil {
		t.Fatal(err)
	}
	wb, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(wb.Sheets) != 2 || wb.Sheets[0].Name != "Data" || wb.Sheets[1].Name != "Second" {
		t.Fatalf("sheets = %v", wb.Sheets)
	}
	sh := wb.Sheets[0]

	cases := []struct {
		row, col int
		text     string
		kind     model.Kind
	}{
		{0, 0, "Title", model.KindText},
		{1, 0, "1,234.50", model.KindNumber},
		{1, 1, "42", model.KindNumber},
		{1, 2, "0.1", model.KindNumber},
		{2, 0, "2026-10-15", model.KindNumber},
		{2, 1, "TRUE", model.KindBool},
		{4, 0, "Ünïcödé ✓", model.KindText},
		{6, 0, "merged", model.KindText},
	}
	for _, c := range cases {
		got := sh.Get(c.row, c.col)
		if got.Text != c.text || got.Kind != c.kind {
			t.Errorf("%s = %q (kind %d), want %q (kind %d)",
				model.CellName(c.row, c.col), got.Text, got.Kind, c.text, c.kind)
		}
	}
	if long := sh.Get(5, 0).Text; long != strings.Repeat("x", 9000) {
		t.Errorf("long string across CONTINUE: len %d", len(long))
	}

	title := wb.Styles[sh.Get(0, 0).Style]
	if !title.Font.Bold || title.Font.SizePt != 14 || title.Font.Color != colors.RGB(0xFF0000) || title.HAlign != model.HAlignCenter {
		t.Errorf("title style = %+v", title)
	}
	boxed := wb.Styles[sh.Get(3, 0).Style]
	if boxed.Fill != colors.RGB(0xFFFF00) || boxed.Border[model.Bottom].Style != model.BorderMedium || boxed.Border[model.Left].Style != model.BorderThin {
		t.Errorf("boxed style = %+v", boxed)
	}

	if len(sh.Merges) != 1 || sh.Merges[0] != (model.Range{R1: 6, C1: 0, R2: 7, C2: 2}) {
		t.Errorf("merges = %v", sh.Merges)
	}
	if w := sh.ColWidth[0]; w < 19.9 || w > 20.8 { // stored in characters
		t.Errorf("col A width = %v chars, want ~20", w)
	}
	if h := sh.RowHeightPx(8); h != 40 {
		t.Errorf("row 9 height = %v, want 40", h)
	}
	if sh.FreezeRows != 1 || sh.FreezeCols != 1 {
		t.Errorf("freeze = %d,%d", sh.FreezeRows, sh.FreezeCols)
	}
	if got := wb.Sheets[1].Get(299, 0).Text; got != "row 299" {
		t.Errorf("Second!A300 = %q", got)
	}
}

func TestRK(t *testing.T) {
	for _, c := range []struct {
		rk   uint32
		want float64
	}{
		{42<<2 | 0x02, 42},
		{1234<<2 | 0x03, 12.34},
		{0x3FF00000, 1}, // 1.0 as the high 30 bits of a float64
	} {
		if got := rkValue(c.rk); got != c.want {
			t.Errorf("rkValue(%#x) = %v, want %v", c.rk, got, c.want)
		}
	}
}
