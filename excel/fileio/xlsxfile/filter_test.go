package xlsxfile

import (
	"reflect"
	"strings"
	"testing"

	"officelite/excel/model"
)

// filterModel is a sheet with an AutoFilter on A1:D7 using each kind of
// criteria, rows 3 and 5 hidden by it, row 9 hidden by hand (outside the
// filter) and row 6 (filtered) with a height of its own.
func filterModel() *model.Workbook {
	wb := &model.Workbook{Styles: []model.Style{model.DefaultStyle()}}
	sh := model.NewSheet("My Data")
	for r := 0; r < 7; r++ {
		for c := 0; c < 4; c++ {
			sh.Set(r, c, model.Cell{Kind: model.KindNumber, Num: float64(r*10 + c), Text: "x"})
		}
	}
	sh.Set(8, 0, model.Cell{Kind: model.KindText, Text: "below"})
	sh.RowHeight[8] = 0
	sh.RowHeight[5] = 30
	sh.AutoFilter = &model.AutoFilter{Range: model.Range{R2: 6, C2: 3}, Columns: []model.FilterColumn{
		{Col: 0, Kind: model.FilterValues, Values: []string{"a & b", "10"}, Blanks: true,
			Dates: []model.DateGroup{{Year: 2026, Month: 1, Grouping: "month"}}},
		{Col: 1, Kind: model.FilterCustom, And: true, Conds: []model.FilterCond{{Op: model.OpGreaterEqual, Val: "5"}, {Op: model.OpNotEqual, Val: "*x*"}}},
		{Col: 2, Kind: model.FilterTop10, Top10: model.Top10{Percent: true, N: 20, Val: 12.5}},
		{Col: 3, Kind: model.FilterDynamic, Dynamic: "aboveAverage", DynamicVal: 33.5, HideButton: true},
	}}
	sh.Filtered = map[int]bool{2: true, 5: true}
	wb.Sheets = []*model.Sheet{model.NewSheet("First"), sh}
	return wb
}

func TestAutoFilterRoundTrip(t *testing.T) {
	want := filterModel()
	got, data := roundTrip(t, want)
	sh := got.Sheets[1]
	if !reflect.DeepEqual(sh.AutoFilter, want.Sheets[1].AutoFilter) {
		t.Errorf("AutoFilter\n got %+v\nwant %+v", sh.AutoFilter, want.Sheets[1].AutoFilter)
	}
	if !reflect.DeepEqual(sh.Filtered, map[int]bool{2: true, 5: true}) {
		t.Errorf("filtered rows %v", sh.Filtered)
	}
	if h, ok := sh.RowHeight[5]; !ok || h != 30 {
		t.Errorf("row 6 height %v %v", h, ok)
	}
	if h, ok := sh.RowHeight[8]; !ok || h != 0 || sh.Filtered[8] {
		t.Errorf("row 9: height %v %v, filtered %v", h, ok, sh.Filtered[8])
	}
	if _, ok := sh.RowHeight[2]; ok {
		t.Errorf("row 3 got a height")
	}
	if got.Sheets[0].AutoFilter != nil || len(got.Names) != 0 {
		t.Errorf("first sheet filter %v, names %v", got.Sheets[0].AutoFilter, got.Names)
	}
	parts := zipParts(t, data)
	if w := strings.ReplaceAll(parts["xl/workbook.xml"], "&apos;", "'"); !strings.Contains(w, `<definedName name="_xlnm._FilterDatabase" localSheetId="1" hidden="1">'My Data'!$A$1:$D$7</definedName>`) {
		t.Errorf("workbook.xml: %s", w)
	}
	ws := parts["xl/worksheets/sheet2.xml"]
	if i, j := strings.Index(ws, "</sheetData><autoFilter"), strings.Index(ws, "<autoFilter"); i < 0 || j != i+len("</sheetData>") {
		t.Errorf("autoFilter not after sheetData: %s", ws)
	}
}

// A file whose filter excelize can't model: a color filter is dropped
// with a warning, its rows stay hidden (and count as filtered).
func TestAutoFilterColorDropped(t *testing.T) {
	wb := filterModel()
	data := mustWrite(t, wb)
	data = patchZip(t, data, "xl/worksheets/sheet2.xml", `<filterColumn colId="2"><top10 top="0" percent="1" val="20" filterVal="12.5"/></filterColumn>`,
		`<filterColumn colId="2"><colorFilter dxfId="0"/></filterColumn>`)
	got, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	af := got.Sheets[1].AutoFilter
	if af == nil || len(af.Columns) != 3 {
		t.Fatalf("filter %+v", af)
	}
	if _, ok := af.Column(2); ok {
		t.Error("color filter kept")
	}
	if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], "color") {
		t.Errorf("warnings %q", got.Warnings)
	}
	if !got.Sheets[1].Filtered[2] {
		t.Error("row 3 not filtered")
	}
}
