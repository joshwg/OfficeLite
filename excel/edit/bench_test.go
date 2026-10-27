package edit

import (
	"fmt"
	"testing"
	"time"

	"officelite/excel/model"
)

// bigBook has 50k rows of a value, a formula and a running total (100k
// formulas).
func bigBook() *Book {
	sh := model.NewSheet("Sheet1")
	for r := 0; r < 50000; r++ {
		sh.Set(r, 0, model.Cell{Kind: model.KindNumber, Num: float64(r), Text: fmt.Sprint(r)})
		sh.Set(r, 1, model.Cell{Formula: fmt.Sprintf("A%d*2+1", r+1)})
		if r == 0 {
			sh.Set(r, 2, model.Cell{Formula: "B1"})
		} else {
			sh.Set(r, 2, model.Cell{Formula: fmt.Sprintf("C%d+B%d", r, r+1)})
		}
	}
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	b := New(wb)
	b.Now = func() time.Time { return now }
	return b
}

// TestEditTiming logs how long edits take on a 100k-formula workbook
// (go test -run Timing -v).
func TestEditTiming(t *testing.T) {
	if !testing.Verbose() {
		t.Skip("timing only; run with -v")
	}
	b := bigBook()
	b.Recalculate()
	timeIt := func(what string, f func()) {
		start := time.Now()
		f()
		t.Logf("%-32s %v", what, time.Since(start).Round(time.Millisecond))
	}
	timeIt("first edit (builds dependencies)", func() { b.Enter(0, 49000, 0, "5") })
	timeIt("edit near the end", func() { b.Enter(0, 49001, 0, "6") })
	timeIt("edit A1 (every total changes)", func() { b.Enter(0, 0, 0, "7") })
	timeIt("undo", func() { b.Undo() })
	timeIt("insert row 10", func() { b.InsertRows(0, 9, 1) })
	timeIt("delete row 10", func() { b.DeleteRows(0, 9, 1) })
	timeIt("undo delete", func() { b.Undo() })
	timeIt("F9", func() { b.Recalculate() })
}
