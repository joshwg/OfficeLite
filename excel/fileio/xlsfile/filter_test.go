// Tests for reading and writing .xls AutoFilters.

package xlsfile

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"officelite/excel/model"
)

// filterSheet returns a sheet of 8×5 numbers with a hand-hidden row 11 and
// a taller row 5.
func filterSheet(name string) *model.Sheet {
	sh := model.NewSheet(name)
	for r := 0; r < 8; r++ {
		for c := 0; c < 5; c++ {
			sh.Set(r, c, model.Cell{Kind: model.KindNumber, Num: float64(r*10 + c), Text: "n"})
		}
	}
	sh.Set(10, 0, model.Cell{Kind: model.KindText, Text: "below"})
	sh.RowHeight[10] = 0
	sh.RowHeight[4] = 30
	return sh
}

// TestAutoFilterRoundTrip checks that value, custom and Top 10 criteria,
// filtered rows, row heights and a note beside the filter survive a .xls
// round trip.
func TestAutoFilterRoundTrip(t *testing.T) {
	sh := filterSheet("My Data")
	want := &model.AutoFilter{Range: model.Range{R1: 0, C1: 0, R2: 7, C2: 4}, Columns: []model.FilterColumn{
		{Col: 0, Kind: model.FilterValues, Values: []string{"apple"}, Blanks: true},
		{Col: 1, Kind: model.FilterCustom, And: true, Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "5"}, {Op: model.OpNotEqual, Val: "*x*"}}},
		{Col: 2, Kind: model.FilterTop10, Top10: model.Top10{Percent: true, N: 20, Val: 12.5}},
		{Col: 3, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpLess, Val: "3"}, {Op: model.OpNotEqual}}},
		{Col: 4, Kind: model.FilterTop10, Top10: model.Top10{Top: true, N: 3, Val: 54}},
	}}
	sh.AutoFilter = want.Clone()
	sh.Filtered = map[int]bool{2: true, 4: true}
	sh.SetNote(0, 6, &model.Note{Text: "a note too"})
	wb := &model.Workbook{Sheets: []*model.Sheet{model.NewSheet("First"), sh}, Styles: []model.Style{model.DefaultStyle()}}
	if issues := Fits(wb); len(issues) != 0 {
		t.Errorf("Fits = %q", issues)
	}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	g := got.Sheets[1]
	if !reflect.DeepEqual(g.AutoFilter, want) {
		t.Errorf("AutoFilter\n got %+v\nwant %+v", g.AutoFilter, want)
	}
	if !reflect.DeepEqual(g.Filtered, map[int]bool{2: true, 4: true}) {
		t.Errorf("filtered %v", g.Filtered)
	}
	if h := g.RowHeight[4]; h != 30 {
		t.Errorf("row 5 height %v", h)
	}
	if h, ok := g.RowHeight[10]; !ok || h != 0 {
		t.Errorf("row 11 not hidden by hand")
	}
	if n, ok := g.NoteAt(0, 6); !ok || n.Text != "a note too" {
		t.Errorf("note %+v", n)
	}
	if len(got.Names) != 0 || got.Sheets[0].AutoFilter != nil || len(got.Warnings) != 0 {
		t.Errorf("names %v, first sheet %v, warnings %q", got.Names, got.Sheets[0].AutoFilter, got.Warnings)
	}
}

// TestAutoFilterFits checks that criteria BIFF8 can't hold are reported by
// Fits and saved without criteria, the rows staying hidden.
func TestAutoFilterFits(t *testing.T) {
	sh := filterSheet("S")
	sh.AutoFilter = &model.AutoFilter{Range: model.Range{R2: 7, C2: 4}, Columns: []model.FilterColumn{
		{Col: 0, Kind: model.FilterValues, Values: []string{"a", "b", "c"}},
		{Col: 1, Kind: model.FilterDynamic, Dynamic: "aboveAverage", DynamicVal: 21.5},
		{Col: 2, Kind: model.FilterDynamic, Dynamic: "today"},
	}}
	sh.Filtered = map[int]bool{3: true}
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	issues := Fits(wb)
	if len(issues) != 2 || !strings.Contains(issues[0], "column A") || !strings.Contains(issues[1], "column C") {
		t.Errorf("Fits = %q", issues)
	}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	g := got.Sheets[0]
	want := []model.FilterColumn{{Col: 1, Kind: model.FilterCustom, Conds: []model.FilterCond{{Op: model.OpGreater, Val: "21.5"}}}}
	if g.AutoFilter == nil || !reflect.DeepEqual(g.AutoFilter.Columns, want) || !g.Filtered[3] {
		t.Errorf("filter %+v, filtered %v", g.AutoFilter, g.Filtered)
	}
}
