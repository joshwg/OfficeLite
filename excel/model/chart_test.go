// Tests for chart references, live chart data, anchor shifting, cloning
// and theme series colors.

package model

import (
	"image/color"
	"testing"
)

// TestParseRefList checks parsing of qualified, quoted, union, external
// and whole-column references, and rejection of non-references.
func TestParseRefList(t *testing.T) {
	for _, c := range []struct {
		in   string
		want []SheetRange
	}{
		{"Sheet1!$B$2:$B$10", []SheetRange{{"Sheet1", Range{R1: 1, C1: 1, R2: 9, C2: 1}}}},
		{"'My Sheet'!$A$1", []SheetRange{{"My Sheet", Range{}}}},
		{"'It''s'!A1:B2", []SheetRange{{"It's", Range{R2: 1, C2: 1}}}},
		{"(Data!$A$1,Data!$A$3:$A$4)", []SheetRange{{"Data", Range{}}, {"Data", Range{R1: 2, R2: 3}}}},
		{"[Book1.xlsx]Data!$C$5", []SheetRange{{"Data", Range{R1: 4, C1: 2, R2: 4, C2: 2}}}},
		{"B3:B1", []SheetRange{{"Home", Range{R1: 0, C1: 1, R2: 2, C2: 1}}}}, // unqualified: home
		{"Data!$C:$C", []SheetRange{{"Data", Range{C1: 2, R2: MaxRows - 1, C2: 2}}}},
	} {
		got, ok := ParseRefList(c.in, "Home")
		if !ok || len(got) != len(c.want) {
			t.Errorf("%s: got %v %v", c.in, got, ok)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: part %d = %v, want %v", c.in, i, got[i], c.want[i])
			}
		}
	}
	for _, bad := range []string{"#REF!", "Data!#REF!", "SUM(A1)", "Name"} {
		if _, ok := ParseRefList(bad, "Home"); ok {
			t.Errorf("%s: parsed", bad)
		}
	}
}

// chartBook returns a one-sheet workbook with labels A..D in column A,
// 10..40 in column B and the defined name Vals for B1:B4.
func chartBook() *Workbook {
	sh := NewSheet("Data")
	for r, v := range []float64{10, 20, 30, 40} {
		sh.Set(r, 0, Cell{Kind: KindText, Text: string(rune('A' + r))})
		sh.Set(r, 1, Cell{Kind: KindNumber, Num: v, Text: "x"})
	}
	return &Workbook{Sheets: []*Sheet{sh}, Styles: []Style{DefaultStyle()},
		Names: []DefinedName{{Name: "Vals", Sheet: -1, Formula: "Data!$B$1:$B$4"}}}
}

// TestChartValues checks reading series values from cells: hidden rows,
// defined names, multi-level blocks, the cache fallback and live edits.
func TestChartValues(t *testing.T) {
	wb := chartBook()
	sh := wb.Sheets[0]
	vals, _, _, ok := wb.ChartValues("Data!$B$1:$B$4", "", true)
	if !ok || len(vals) != 4 || vals[2].Num != 30 || vals[2].Kind != ValueNumber {
		t.Fatalf("values: %v %v", vals, ok)
	}
	// Hidden rows are left out unless asked for.
	sh.RowHeight[1] = 0
	if vals, _, _, _ = wb.ChartValues("Data!$B$1:$B$4", "", true); len(vals) != 3 || vals[1].Num != 30 {
		t.Errorf("hidden row: %v", vals)
	}
	if vals, _, _, _ = wb.ChartValues("Data!$B$1:$B$4", "", false); len(vals) != 4 {
		t.Errorf("hidden row shown: %v", vals)
	}
	// A defined name; a two-column block (multi-level labels).
	if vals, _, _, ok = wb.ChartValues("Vals", "", false); !ok || len(vals) != 4 {
		t.Errorf("name: %v %v", vals, ok)
	}
	vals, levels, _, _ := wb.ChartValues("Data!$A$1:$B$2", "", false)
	if len(vals) != 2 || vals[0].Num != 10 || len(levels) != 1 || levels[0][1].Text != "B" {
		t.Errorf("block: %v %v", vals, levels)
	}
	// A missing sheet falls back to the cache.
	d := &ChartData{Ref: "Gone!$A$1", Cache: []ChartValue{{Kind: ValueNumber, Num: 7}}}
	if got := wb.SeriesData(d, "", true); len(got) != 1 || got[0].Num != 7 {
		t.Errorf("cache fallback: %v", got)
	}
	// Live: an edited cell shows at once.
	sh.Set(3, 1, Cell{Kind: KindNumber, Num: 99})
	if vals, _, _, _ = wb.ChartValues("Data!$B$4", "", true); vals[0].Num != 99 {
		t.Errorf("after edit: %v", vals)
	}
}

// TestChartAnchorShift checks how two-cell, one-cell and absolute anchors
// move with inserted and deleted rows and columns, and that sheet clones
// don't share charts.
func TestChartAnchorShift(t *testing.T) {
	sh := NewSheet("S")
	two := &Chart{Anchor: Anchor{Kind: AnchorTwoCell, From: CellOffset{Row: 2, Col: 1, DY: 100}, To: CellOffset{Row: 10, Col: 5, DY: 50}}}
	one := &Chart{Anchor: Anchor{Kind: AnchorOneCell, From: CellOffset{Row: 4, Col: 2}, ExtX: 1000, ExtY: 1000}}
	abs := &Chart{Anchor: Anchor{Kind: AnchorAbsolute, PosX: 5, PosY: 5}}
	sh.Charts = []*Chart{two, one, abs}

	sh.InsertRows(0, 2) // above everything: all move but the absolute one
	if two.Anchor.From.Row != 4 || two.Anchor.To.Row != 12 || one.Anchor.From.Row != 6 {
		t.Errorf("insert above: %+v %+v", two.Anchor, one.Anchor)
	}
	sh.InsertRows(5, 3) // through the two-cell chart: it grows
	if two.Anchor.From.Row != 4 || two.Anchor.To.Row != 15 || one.Anchor.From.Row != 9 {
		t.Errorf("insert inside: %+v %+v", two.Anchor, one.Anchor)
	}
	sh.DeleteRows(14, 5) // its end row goes: it ends at the cut
	if two.Anchor.To.Row != 14 || two.Anchor.To.DY != 0 {
		t.Errorf("delete end: %+v", two.Anchor)
	}
	sh.DeleteCols(0, 3) // its first column goes; it shrinks from the left
	if two.Anchor.From.Col != 0 || two.Anchor.To.Col != 2 || two.Anchor.From.DX != 0 {
		t.Errorf("delete cols: %+v", two.Anchor)
	}
	if abs.Anchor.PosX != 5 || abs.Anchor.From != (CellOffset{}) {
		t.Errorf("absolute moved: %+v", abs.Anchor)
	}
	// Clone copies charts deeply.
	c := sh.Clone()
	c.Charts[0].Anchor.From.Row = 99
	c.Charts[0].Plots = append(c.Charts[0].Plots, ChartPlot{})
	if two.Anchor.From.Row == 99 || len(two.Plots) != 0 {
		t.Error("clone shares charts")
	}
}

// TestChartRefsAndClone checks that Refs visits and rewrites every
// reference, RefAt reads one back, and Clone copies deeply.
func TestChartRefsAndClone(t *testing.T) {
	c := &Chart{
		Plots: []ChartPlot{{Series: []ChartSeries{{Name: ChartData{Ref: "S!$B$1"}, Cat: ChartData{Ref: "S!$A$2:$A$4"}, Val: ChartData{Ref: "S!$B$2:$B$4"}}}}},
		Title: &ChartTitle{Ref: ChartData{Ref: "S!$A$1"}},
		Axes:  []ChartAxis{{ID: 1, Title: &ChartTitle{Text: "x"}}},
	}
	var refs []string
	c.Refs(func(s *string) { refs = append(refs, *s); *s += "X" })
	if len(refs) != 4 || c.Plots[0].Series[0].Val.Ref != "S!$B$2:$B$4X" || c.Title.Ref.Ref != "S!$A$1X" {
		t.Errorf("refs %v: %+v", refs, c.Plots[0].Series[0])
	}
	if got, _ := c.RefAt(RawRef{Plot: 0, Series: 0, Field: RefCat}); got != "S!$A$2:$A$4X" {
		t.Errorf("RefAt: %q", got)
	}
	n := c.Clone()
	n.Plots[0].Series[0].Val.Ref = "changed"
	n.Title.Text = "changed"
	if c.Plots[0].Series[0].Val.Ref == "changed" || c.Title.Text == "changed" {
		t.Error("clone shares data")
	}
}

// TestSeriesColors checks the automatic series colors and LumMod against
// Excel's values.
func TestSeriesColors(t *testing.T) {
	th := DefaultTheme()
	if th.SeriesColor(0) != th.Colors[ThemeAccent1] || th.SeriesColor(5) != th.Colors[ThemeAccent1+5] {
		t.Error("first six series aren't the accents")
	}
	// Series 7 is accent 1 darker (lumMod 60%).
	a, d := th.Colors[ThemeAccent1], th.SeriesColor(6)
	if lum(d) >= lum(a) {
		t.Errorf("series 7 %v not darker than %v", d, a)
	}
	// Series 13 is accent 1 lighter (lumMod 80%, lumOff 20%).
	if l := th.SeriesColor(12); lum(l) <= lum(a) {
		t.Errorf("series 13 %v not lighter than %v", l, a)
	}
	// Excel's tx1 lumMod 15% lumOff 85% on black is #D9D9D9.
	if g := LumMod(color.NRGBA{A: 255}, 15000, 85000); g.R != 0xD9 || g.G != 0xD9 {
		t.Errorf("grid grey %v", g)
	}
	if g := LumMod(color.NRGBA{A: 255}, 65000, 35000); g.R != 0x59 {
		t.Errorf("text grey %v", g)
	}
}

// lum is a rough brightness of c: the sum of its channels.
func lum(c color.NRGBA) int { return int(c.R) + int(c.G) + int(c.B) }
