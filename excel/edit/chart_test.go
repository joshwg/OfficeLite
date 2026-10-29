// chart_test.go tests chart references following structural edits and
// chart deletion.

package edit

import (
	"testing"

	"officelite/excel/model"
)

// chartBook is a book with data on Data (A1:B4), a chart beside it
// reading it, a chart on Sheet1 reading it too, and a chart sheet.
func chartBook(t *testing.T) *Book {
	b := newBook()
	for ref, v := range map[string]string{"A1": "Q", "B1": "Sales", "A2": "Q1", "B2": "10", "A3": "Q2", "B3": "20", "A4": "Q3", "B4": "30"} {
		b.enter(t, 1, ref, v)
	}
	series := func() []model.ChartPlot {
		return []model.ChartPlot{{Type: model.ChartBar, Series: []model.ChartSeries{{
			Name: model.ChartData{Ref: "Data!$B$1"}, Cat: model.ChartData{Ref: "Data!$A$2:$A$4"}, Val: model.ChartData{Ref: "Data!$B$2:$B$4"}}}}}
	}
	data := b.WB.Sheets[1]
	data.Charts = []*model.Chart{{Name: "Chart 1", Plots: series(),
		Anchor: model.Anchor{From: model.CellOffset{Row: 1, Col: 3}, To: model.CellOffset{Row: 10, Col: 8}}}}
	b.WB.Sheets[0].Charts = []*model.Chart{{Name: "Chart 2", Plots: series(),
		Anchor: model.Anchor{From: model.CellOffset{Row: 0, Col: 0}, To: model.CellOffset{Row: 5, Col: 5}}}}
	cs := model.NewSheet("Chart1")
	cs.ChartSheet = &model.Chart{Plots: series()}
	b.WB.Sheets = append(b.WB.Sheets, cs)
	return b
}

// valRef is the values reference of c's first series.
func valRef(c *model.Chart) string { return c.Plots[0].Series[0].Val.Ref }

// TestChartRefsFollowRowsAndCols checks that chart references and anchors
// follow inserted and deleted rows and columns, with undo and redo.
func TestChartRefsFollowRowsAndCols(t *testing.T) {
	b := chartBook(t)
	data, other, cs := b.WB.Sheets[1], b.WB.Sheets[0], b.WB.Sheets[2]
	if _, err := b.InsertRows(1, 0, 2); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*model.Chart{data.Charts[0], other.Charts[0], cs.ChartSheet} {
		if valRef(c) != "Data!$B$4:$B$6" || c.Plots[0].Series[0].Name.Ref != "Data!$B$3" {
			t.Errorf("after insert: %s", valRef(c))
		}
	}
	if a := data.Charts[0].Anchor; a.From.Row != 3 || a.To.Row != 12 {
		t.Errorf("anchor after insert: %+v", a)
	}
	if a := other.Charts[0].Anchor; a.From.Row != 0 {
		t.Errorf("other sheet's chart moved: %+v", a)
	}
	b.Undo()
	if valRef(data.Charts[0]) != "Data!$B$2:$B$4" || valRef(other.Charts[0]) != "Data!$B$2:$B$4" || valRef(cs.ChartSheet) != "Data!$B$2:$B$4" {
		t.Errorf("undo: %s %s %s", valRef(data.Charts[0]), valRef(other.Charts[0]), valRef(cs.ChartSheet))
	}
	if a := data.Charts[0].Anchor; a.From.Row != 1 {
		t.Errorf("anchor after undo: %+v", a)
	}
	b.Redo()
	if valRef(other.Charts[0]) != "Data!$B$4:$B$6" {
		t.Errorf("redo: %s", valRef(other.Charts[0]))
	}
	b.Undo()

	// Deleting a data row shrinks the range; deleting a column moves it.
	if _, err := b.DeleteRows(1, 2, 1); err != nil {
		t.Fatal(err)
	}
	if got := valRef(other.Charts[0]); got != "Data!$B$2:$B$3" {
		t.Errorf("delete row: %s", got)
	}
	if _, err := b.DeleteCols(1, 0, 1); err != nil {
		t.Fatal(err)
	}
	if got := valRef(other.Charts[0]); got != "Data!$A$2:$A$3" {
		t.Errorf("delete col: %s", got)
	}
	if a := data.Charts[0].Anchor; a.From.Col != 2 || a.To.Col != 7 {
		t.Errorf("anchor after delete col: %+v", a)
	}
	// Deleting every cell referred to leaves #REF!.
	if _, err := b.DeleteCols(1, 0, 1); err != nil {
		t.Fatal(err)
	}
	if got := valRef(other.Charts[0]); got != "#REF!" && got != "Data!#REF!" {
		t.Errorf("deleted data: %s", got)
	}
}

// TestChartRefsFollowSheets checks that chart references follow a renamed
// sheet and become #REF! when it is deleted, with undo.
func TestChartRefsFollowSheets(t *testing.T) {
	b := chartBook(t)
	if _, err := b.RenameSheet(1, "Sales Data"); err != nil {
		t.Fatal(err)
	}
	if got := valRef(b.WB.Sheets[0].Charts[0]); got != "'Sales Data'!$B$2:$B$4" {
		t.Errorf("rename: %s", got)
	}
	if got := valRef(b.WB.Sheets[2].ChartSheet); got != "'Sales Data'!$B$2:$B$4" {
		t.Errorf("rename (chart sheet): %s", got)
	}
	b.Undo()
	if got := valRef(b.WB.Sheets[0].Charts[0]); got != "Data!$B$2:$B$4" {
		t.Errorf("undo rename: %s", got)
	}
	if _, err := b.DeleteSheet(1); err != nil {
		t.Fatal(err)
	}
	if got := valRef(b.WB.Sheets[0].Charts[0]); got != "#REF!" {
		t.Errorf("delete sheet: %s", got)
	}
	b.Undo()
	if got := valRef(b.WB.Sheets[0].Charts[0]); got != "Data!$B$2:$B$4" {
		t.Errorf("undo delete sheet: %s", got)
	}
}

// TestDeleteChartUndo checks DeleteChart with undo and redo.
func TestDeleteChartUndo(t *testing.T) {
	b := chartBook(t)
	data := b.WB.Sheets[1]
	b.DeleteChart(1, 0)
	if len(data.Charts) != 0 {
		t.Fatal("not deleted")
	}
	where, _, _ := b.Undo()
	if len(data.Charts) != 1 || data.Charts[0].Name != "Chart 1" || where.Sheet != 1 {
		t.Fatalf("undo: %d charts, where %+v", len(data.Charts), where)
	}
	b.Redo()
	if len(data.Charts) != 0 {
		t.Fatal("redo")
	}
}

// TestChartLiveValues checks that charts draw from the cells: an edit
// shows at once.
func TestChartLiveValues(t *testing.T) {
	b := chartBook(t)
	c := b.WB.Sheets[0].Charts[0]
	b.enter(t, 1, "B3", "=B2*5")
	vals := b.WB.SeriesData(&c.Plots[0].Series[0].Val, "Sheet1", true)
	if len(vals) != 3 || vals[1].Num != 50 {
		t.Fatalf("values %+v", vals)
	}
	b.enter(t, 1, "B2", "4")
	if vals = b.WB.SeriesData(&c.Plots[0].Series[0].Val, "Sheet1", true); vals[1].Num != 20 {
		t.Errorf("recalculated %+v", vals)
	}
}
