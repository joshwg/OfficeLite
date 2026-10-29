package edit

import (
	"errors"
	"testing"

	"officelite/excel/model"
	"officelite/excel/pivot"
)

// pivotBook has a sales table in Data!A1:C7 and a report of Sum of
// Sales by Region on Sheet1 at A3.
func pivotBook(t *testing.T) *Book {
	t.Helper()
	b := newBook()
	rows := [][]string{
		{"Region", "Product", "Sales"},
		{"East", "Pen", "10"},
		{"West", "Pen", "20"},
		{"East", "Ink", "30"},
		{"West", "Pad", "7"},
		{"North", "Ink", "8"},
		{"East", "Pad", "1"},
	}
	for r, row := range rows {
		for c, v := range row {
			b.enter(t, 1, model.CellName(r, c), v)
		}
	}
	pt, err := b.NewPivot(0, b.SourceText(1, rng("A1", "C7")), 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	pt.Rows = []int{0}
	pt.Values = []model.PivotValue{{Field: 2, Name: "Sum of Sales"}}
	if _, err := b.SetPivot(0, -1, pt, false); err != nil {
		t.Fatal(err)
	}
	return b
}

// column lists the texts of column col of sheet from row r1 to r2.
func column(b *Book, sheet, col, r1, r2 int) []string {
	var out []string
	for r := r1; r <= r2; r++ {
		out = append(out, b.WB.Sheets[sheet].Get(r, col).Text)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPivotCreateUndoRefresh(t *testing.T) {
	b := pivotBook(t)
	sh := b.WB.Sheets[0]
	if pt := sh.Pivots[0]; pt.Source != "Data!$A$1:$C$7" || pt.Name != "PivotTable1" || pt.Out.Body != rng("A3", "B7") {
		t.Fatalf("pivot %+v", pt)
	}
	if got := column(b, 0, 0, 2, 6); !eq(got, []string{"Row Labels", "East", "North", "West", "Grand Total"}) {
		t.Errorf("labels %q", got)
	}
	if got := column(b, 0, 1, 2, 6); !eq(got, []string{"Sum of Sales", "41", "8", "27", "76"}) {
		t.Errorf("values %q", got)
	}
	st := b.WB.Styles[b.cell(0, "A3").Style]
	if !st.Font.Bold || st.Fill != pivotBand || st.Border[model.Bottom].Style != model.BorderThin {
		t.Errorf("header style %+v", st)
	}
	if st := b.WB.Styles[b.cell(0, "B7").Style]; !st.Font.Bold || st.Border[model.Top].Style != model.BorderThin {
		t.Errorf("grand total style %+v", st)
	}

	// Typing into the report is refused, except over a value's name.
	if _, err := b.Enter(0, 3, 1, "5"); !errors.Is(err, ErrPivotChange) {
		t.Errorf("typing a value: %v", err)
	}
	b.enter(t, 0, "B3", "Revenue")
	if sh.Pivots[0].Values[0].Name != "Revenue" || b.cell(0, "B3").Text != "Revenue" {
		t.Errorf("renamed: %q", sh.Pivots[0].Values[0].Name)
	}
	b.Undo()
	if sh.Pivots[0].Values[0].Name != "Sum of Sales" || b.cell(0, "B3").Text != "Sum of Sales" {
		t.Errorf("rename undone: %q", b.cell(0, "B3").Text)
	}

	// Add Product as a column field: one step, undone in one.
	p := sh.Pivots[0].Clone()
	p.Cols = []int{1}
	if _, err := b.SetPivot(0, 0, p, false); err != nil {
		t.Fatal(err)
	}
	if got := sh.Pivots[0].Out.Body; got != rng("A3", "E8") || b.cell(0, "B3").Text != "Column Labels" {
		t.Errorf("with columns: %v %q", got, b.cell(0, "B3").Text)
	}
	b.Undo()
	if got := sh.Pivots[0].Out.Body; got != rng("A3", "B7") || b.cell(0, "C4").Text != "" || b.cell(0, "A8").Text != "" {
		t.Errorf("undone: %v %q", got, b.cell(0, "C4").Text)
	}

	// Refresh picks up edited and added source rows (the range is
	// fixed, as in Excel: added rows need a new source).
	b.enter(t, 1, "C2", "100")
	if b.cell(0, "B4").Text != "41" {
		t.Error("refreshed by itself")
	}
	if _, err := b.RefreshPivot(0, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := column(b, 0, 1, 3, 6); !eq(got, []string{"131", "8", "27", "166"}) {
		t.Errorf("after refresh %q", got)
	}
}

func TestPivotLocationAndGuards(t *testing.T) {
	b := pivotBook(t)
	sh := b.WB.Sheets[0]
	// Rows inserted above move the report; rows of the source sheet
	// move its source.
	if _, err := b.InsertRows(0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if pt := sh.Pivots[0]; pt.Out.Body != rng("A5", "B9") || pt.Row != 4 || b.cell(0, "A5").Text != "Row Labels" {
		t.Errorf("after insert: %+v", pt.Out.Body)
	}
	if _, err := b.InsertRows(1, 0, 1); err != nil {
		t.Fatal(err)
	}
	if src := sh.Pivots[0].Source; src != "Data!$A$2:$C$8" {
		t.Errorf("source %q", src)
	}
	if _, err := b.RefreshPivot(0, 0, false); err != nil || b.cell(0, "B9").Text != "76" {
		t.Errorf("refresh after move: %v %q", err, b.cell(0, "B9").Text)
	}
	b.Undo()
	b.Undo()
	if src := sh.Pivots[0].Source; src != "Data!$A$1:$C$7" {
		t.Errorf("source undone %q", src)
	}
	// Rows through the report can't be inserted or deleted.
	if _, err := b.InsertRows(0, 6, 1); !errors.Is(err, ErrPivotChange) {
		t.Errorf("insert through: %v", err)
	}
	if _, err := b.DeleteRows(0, 7, 1); !errors.Is(err, ErrPivotChange) {
		t.Errorf("delete through: %v", err)
	}
	b.Undo()
	if sh.Pivots[0].Out.Body != rng("A3", "B7") {
		t.Errorf("undo insert: %v", sh.Pivots[0].Out.Body)
	}

	// Growing over data asks first.
	b.enter(t, 0, "D5", "x")
	p := sh.Pivots[0].Clone()
	p.Cols = []int{1}
	if _, err := b.SetPivot(0, 0, p, false); !errors.Is(err, ErrPivotOverwrite) || len(sh.Pivots[0].Cols) != 0 {
		t.Errorf("overwrite: %v", err)
	}
	if _, err := b.SetPivot(0, 0, p, true); err != nil || b.cell(0, "D5").Text != "10" {
		t.Errorf("overwritten: %v %q", err, b.cell(0, "D5").Text)
	}
	// Another report can't overlap it.
	q, _ := b.NewPivot(0, sh.Pivots[0].Source, 4, 2)
	if _, err := b.SetPivot(0, -1, q, true); !errors.Is(err, ErrPivotOverlap) {
		t.Errorf("overlap: %v", err)
	}

	b.DeletePivot(0, 0)
	if len(sh.Pivots) != 0 || b.cell(0, "A3").Text != "" || b.cell(0, "A3").Style != 0 {
		t.Errorf("deleted: %d", len(sh.Pivots))
	}
	b.Undo()
	if len(sh.Pivots) != 1 || b.cell(0, "A3").Text != "Sum of Sales" {
		t.Errorf("delete undone: %q", b.cell(0, "A3").Text)
	}
}

func TestPivotSyncFieldsAndDetails(t *testing.T) {
	b := pivotBook(t)
	sh := b.WB.Sheets[0]
	// A renamed source column drops out of the report on refresh.
	b.enter(t, 1, "A1", "Area")
	if _, err := b.RefreshPivot(0, 0, false); err != nil {
		t.Fatal(err)
	}
	if pt := sh.Pivots[0]; len(pt.Rows) != 0 || pt.Fields[0].Name != "Area" || b.cell(0, "A3").Text != "Sum of Sales" || b.cell(0, "A4").Text != "76" {
		t.Errorf("after rename: rows %v, %q %q", pt.Rows, b.cell(0, "A3").Text, b.cell(0, "A4").Text)
	}
	b.Undo()
	b.Undo()

	// Show Details of East's total: a new sheet before the report's.
	si, err := b.ShowDetails(0, 3, 1)
	if err != nil || si != 0 || len(b.WB.Sheets) != 3 {
		t.Fatalf("details: %d %v", si, err)
	}
	if got := column(b, 0, 0, 0, 4); !eq(got, []string{"Region", "East", "East", "East", ""}) {
		t.Errorf("detail rows %q", got)
	}
	if got := column(b, 0, 2, 1, 3); !eq(got, []string{"10", "30", "1"}) {
		t.Errorf("detail sales %q", got)
	}
	b.Undo()
	if len(b.WB.Sheets) != 2 {
		t.Errorf("details undone: %d sheets", len(b.WB.Sheets))
	}

	// GETPIVOTDATA's lookup.
	pt := b.WB.Sheets[0].Pivots[0]
	if r, c, ok := pivot.GetData(pt, "Sales", nil); !ok || r != 6 || c != 1 {
		t.Errorf("grand total at %d,%d %v", r, c, ok)
	}
}

func TestGetPivotData(t *testing.T) {
	b := pivotBook(t)
	b.enter(t, 0, "E1", `=GETPIVOTDATA("Sum of Sales",$A$3,"Region","East")`)
	b.enter(t, 0, "E2", `=GETPIVOTDATA("Sales",A3)`)
	b.enter(t, 0, "E3", `=GETPIVOTDATA("Sales",A3,"Region","South")`)
	if got := []string{b.cell(0, "E1").Text, b.cell(0, "E2").Text, b.cell(0, "E3").Text}; !eq(got, []string{"41", "76", "#REF!"}) {
		t.Errorf("GETPIVOTDATA %q", got)
	}
	// It follows a refresh.
	b.enter(t, 1, "C2", "20")
	if _, err := b.RefreshPivot(0, 0, false); err != nil {
		t.Fatal(err)
	}
	if got := b.cell(0, "E1").Text; got != "51" {
		t.Errorf("after refresh %q", got)
	}
}
