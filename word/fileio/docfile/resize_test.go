package docfile

import (
	"math"
	"testing"

	"officelite/word/model"
)

// TestResizedTableRoundTrip checks a .doc keeps column widths and a row
// height set by dragging table borders.
func TestResizedTableRoundTrip(t *testing.T) {
	doc := model.New("t")
	e := model.NewEditor(doc)
	e.InsertTable(2, 3)
	tb := doc.Body[0].(*model.Table)
	w := tb.Columns[0]
	e.MoveColumnBorder(tb, 2, -40)
	e.SetRowHeight(tb, 1, 28)
	back := roundTrip(t, doc)
	var got *model.Table
	for _, b := range back.Body {
		if x, ok := b.(*model.Table); ok {
			got = x
		}
	}
	if got == nil {
		t.Fatal("no table")
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.1 }
	if len(got.Columns) != 3 || !near(got.Columns[0], w) || !near(got.Columns[1], w-40) || !near(got.Columns[2], w+40) {
		t.Errorf("columns %v", got.Columns)
	}
	if !near(got.Rows[1].Height, 28) {
		t.Errorf("row height %v", got.Rows[1].Height)
	}
}
