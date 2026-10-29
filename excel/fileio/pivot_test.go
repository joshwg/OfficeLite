package fileio

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"officelite/excel/edit"
	"officelite/excel/model"
)

// areaTexts is every cell text of a report's area.
func areaTexts(sh *model.Sheet, a model.Range) [][]string {
	var out [][]string
	for r := a.R1; r <= a.R2; r++ {
		var row []string
		for c := a.C1; c <= a.C2; c++ {
			row = append(row, sh.Get(r, c).Text)
		}
		out = append(out, row)
	}
	return out
}

func sameTexts(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if len(a[i]) != len(b[i]) {
			return false
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}
	}
	return true
}

// TestExcelPivots opens a workbook Excel made with two PivotTables:
// they are recognised, and refreshing them reproduces what Excel saved.
func TestExcelPivots(t *testing.T) {
	wb, err := Open(filepath.Join("..", "..", "Samples", "Pivot-Tables-and-Charts.xlsx"))
	if err != nil {
		t.Fatal(err)
	}
	b := edit.New(wb)
	defer b.Close()
	p1 := wb.Sheets[1].Pivots
	p2 := wb.Sheets[2].Pivots
	if len(p1) != 1 || len(p2) != 1 {
		t.Fatalf("pivots: %d, %d", len(p1), len(p2))
	}
	pt := p1[0]
	if pt.Source != "'Sales Data'!$A$1:$D$6" || len(pt.Rows) != 1 || pt.Rows[0] != 0 || len(pt.Cols) != 1 || pt.Cols[0] != 1 ||
		len(pt.Pages) != 1 || pt.Pages[0] != 2 || len(pt.Values) != 1 || pt.Values[0].Name != "Sum of Quantity" || pt.Layout != model.LayoutCompact {
		t.Errorf("PivotTable1: %+v", pt)
	}
	if pt.Out.Area != (model.Range{R2: 9, C2: 6}) || len(pt.Out.Buttons) != 3 {
		t.Errorf("area %+v, buttons %+v", pt.Out.Area, pt.Out.Buttons)
	}
	if p := p2[0]; len(p.Values) != 2 || len(p.Cols) != 1 || p.Cols[0] != model.DataField {
		t.Errorf("PivotTable2: %+v", p)
	}
	for si := 1; si <= 2; si++ {
		sh := wb.Sheets[si]
		before := areaTexts(sh, sh.Pivots[0].Out.Area)
		if _, err := b.RefreshPivot(si, 0, false); err != nil {
			t.Fatal(err)
		}
		if after := areaTexts(sh, sh.Pivots[0].Out.Area); !sameTexts(before, after) {
			t.Errorf("sheet %d refreshed:\n%q\nsaved:\n%q", si, after, before)
		}
	}

	// Written and read back, the reports are the same.
	var buf bytes.Buffer
	if err := Write(wb, &buf, XLSX, 0); err != nil {
		t.Fatal(err)
	}
	wb2, err := Read("x.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	b2 := edit.New(wb2)
	defer b2.Close()
	for si := 1; si <= 2; si++ {
		q, p := wb2.Sheets[si].Pivots, wb.Sheets[si].Pivots[0]
		if len(q) != 1 || q[0].Source != p.Source || q[0].Out.Body != p.Out.Body || len(q[0].Values) != len(p.Values) {
			t.Fatalf("sheet %d read back: %+v", si, q)
		}
		sh := wb2.Sheets[si]
		before := areaTexts(sh, q[0].Out.Area)
		if _, err := b2.RefreshPivot(si, 0, false); err != nil {
			t.Fatal(err)
		}
		if after := areaTexts(sh, q[0].Out.Area); !sameTexts(before, after) {
			t.Errorf("sheet %d round trip refreshed:\n%q\nsaved:\n%q", si, after, before)
		}
	}
}

// pivotSample is a sales table on "Sales" and three reports on "Reports":
// compact (region by quarter), tabular (product within region with a
// report filter and subtotals) and outline (two value fields, one as %
// of the grand total).
func pivotSample(t *testing.T) (*model.Workbook, *edit.Book) {
	t.Helper()
	wb := &model.Workbook{
		Name:   "PivotTables.xlsx",
		Sheets: []*model.Sheet{model.NewSheet("Sales"), model.NewSheet("Reports")},
		Styles: []model.Style{model.DefaultStyle()},
	}
	b := edit.New(wb)
	head := []string{"Date", "Region", "Rep", "Product", "Units", "Revenue"}
	regions := []string{"East", "West", "North", "South"}
	reps := []string{"Avery", "Blake", "Casey", "Drew", "Emery"}
	products := []string{"Laptop", "Monitor", "Keyboard", "Mouse"}
	price := []float64{899, 249, 49, 19}
	for c, h := range head {
		b.Enter(0, 0, c, h)
	}
	for i := 1; i <= 48; i++ {
		p := (i/4 + i*7) % 4
		units := float64(1 + (i*13)%9)
		row := []string{
			fmt.Sprintf("%d/%d/2026", 1+(i*5)%12, 1+(i*3)%28),
			regions[(i*3)%4], reps[(i*2)%5], products[p],
			fmt.Sprint(units), fmt.Sprint(units * price[p]),
		}
		for c, v := range row {
			if _, err := b.Enter(0, i, c, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	src := b.SourceText(0, model.Range{R2: 48, C2: 5})
	add := func(row, col int, f func(pt *model.PivotTable)) {
		pt, err := b.NewPivot(1, src, row, col)
		if err != nil {
			t.Fatal(err)
		}
		f(pt)
		if _, err := b.SetPivot(1, -1, pt, false); err != nil {
			t.Fatal(err)
		}
	}
	add(2, 0, func(pt *model.PivotTable) {
		pt.Rows, pt.Cols = []int{1}, []int{3}
		pt.Values = []model.PivotValue{{Field: 5, Name: "Sum of Revenue", NumFmtID: 3}}
	})
	add(13, 0, func(pt *model.PivotTable) {
		pt.Layout = model.LayoutTabular
		pt.Pages = []int{2}
		pt.Rows = []int{1, 3}
		pt.Values = []model.PivotValue{{Field: 4, Name: "Sum of Units"}}
		pt.Fields[2].Hidden = []string{"s:avery"}
	})
	add(2, 9, func(pt *model.PivotTable) {
		pt.Layout = model.LayoutOutline
		pt.Rows = []int{3}
		pt.Values = []model.PivotValue{
			{Field: 5, Name: "Average of Revenue", Func: model.FuncAverage, NumFmtCode: "#,##0.00"},
			{Field: 5, Name: "% of Revenue", ShowAs: model.ShowPercentOfTotal},
		}
		pt.Fields[3].Sort = model.SortDescending
	})
	// Fitted as ExcelLite (and Excel) fit a report's columns.
	b.SetColWidths(0, []int{0, 1, 2, 3, 4, 5}, []float32{11, 9, 9, 11, 8, 10})
	b.SetColWidths(1, []int{0, 1, 2, 3, 4, 5, 9, 10, 11}, []float32{14, 16, 14, 11, 11, 13, 13, 20, 15})
	return wb, b
}

func TestWritePivots(t *testing.T) {
	wb, b := pivotSample(t)
	defer b.Close()
	var buf bytes.Buffer
	if err := Write(wb, &buf, XLSX, 0); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OFFICELITE_SAMPLES") != "" {
		if err := os.WriteFile(filepath.Join("..", "..", "Samples", "PivotTables.xlsx"), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wb2, err := Read("PivotTables.xlsx", buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	b2 := edit.New(wb2)
	defer b2.Close()
	sh, sh2 := wb.Sheets[1], wb2.Sheets[1]
	if len(sh2.Pivots) != 3 {
		t.Fatalf("%d pivots read back", len(sh2.Pivots))
	}
	for i, p := range sh.Pivots {
		q := sh2.Pivots[i]
		if q.Layout != p.Layout || q.Out.Area != p.Out.Area || len(q.Values) != len(p.Values) || q.Values[len(q.Values)-1] != p.Values[len(p.Values)-1] ||
			len(q.Fields[2].Hidden) != len(p.Fields[2].Hidden) || q.Fields[3].Sort != p.Fields[3].Sort {
			t.Errorf("pivot %d read back:\n%+v\nwrote\n%+v", i, q, p)
		}
		before := areaTexts(sh2, q.Out.Area)
		if !sameTexts(before, areaTexts(sh, p.Out.Area)) {
			t.Errorf("pivot %d cells differ", i)
		}
		if _, err := b2.RefreshPivot(1, i, false); err != nil {
			t.Fatal(err)
		}
		if after := areaTexts(sh2, q.Out.Area); !sameTexts(before, after) {
			t.Errorf("pivot %d refreshed:\n%q\nsaved:\n%q", i, after, before)
		}
	}
}
