package docxfile

import (
	"bytes"
	"math"
	"testing"

	"officelite/word/model"
)

// TestResizedTableRoundTrip resizes a read table's columns and a row as
// dragging their borders does and checks a re-save keeps the sizes.
func TestResizedTableRoundTrip(t *testing.T) {
	doc := model.New("t")
	e := model.NewEditor(doc)
	e.InsertTable(2, 3)
	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	back, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	e = model.NewEditor(back)
	tb := back.Body[0].(*model.Table)
	w := tb.Columns[0]
	e.MoveColumnBorder(tb, 1, 36)
	e.SetRowHeight(tb, 0, 30)
	buf.Reset()
	if err := Write(back, &buf); err != nil {
		t.Fatal(err)
	}
	again, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	tb = again.Body[0].(*model.Table)
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.1 }
	if !near(tb.Columns[0], w+36) || !near(tb.Columns[1], w-36) || !near(tb.Columns[2], w) {
		t.Errorf("columns %v", tb.Columns)
	}
	if c := tb.Rows[1].Cells[0]; !near(c.Width, w+36) {
		t.Errorf("cell width %v", c.Width)
	}
	if !near(tb.Rows[0].Height, 30) {
		t.Errorf("row height %v", tb.Rows[0].Height)
	}
}
