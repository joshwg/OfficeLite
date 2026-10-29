package edit

import (
	"testing"

	"officelite/excel/model"
)

func TestResizeUndo(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	sh.ColWidth[2] = 20
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	b := New(wb)
	defer b.Close()

	b.ResizeCols(0, []int{1, 2}, 5)
	if sh.ColWidth[1] != 5 || sh.ColWidth[2] != 5 {
		t.Fatalf("widths = %v", sh.ColWidth)
	}
	b.ResizeRows(0, []int{3}, 0)
	if h, ok := sh.RowHeight[3]; !ok || h != 0 {
		t.Fatalf("heights = %v", sh.RowHeight)
	}
	b.Undo()
	if _, ok := sh.RowHeight[3]; ok {
		t.Errorf("row height not restored: %v", sh.RowHeight)
	}
	where, _, _ := b.Undo()
	if _, ok := sh.ColWidth[1]; ok || sh.ColWidth[2] != 20 {
		t.Errorf("widths not restored: %v", sh.ColWidth)
	}
	if where.Range.C1 != 1 || where.Range.C2 != 2 {
		t.Errorf("where = %+v", where)
	}
	b.Redo()
	if sh.ColWidth[1] != 5 || sh.ColWidth[2] != 5 {
		t.Errorf("redo widths = %v", sh.ColWidth)
	}
}

// Hide keeps rows' and columns' sizes, Unhide gives them back, also far
// past the used area, and both undo.
func TestHideUnhideKeepsSizes(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	sh.RowHeight[2] = 40
	sh.ColWidth[700] = 22
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	b := New(wb)
	defer b.Close()

	if !b.Hide(0, true, []int{2, 3, 5000}) || !b.Hide(0, false, []int{700}) {
		t.Fatal("nothing hidden")
	}
	if !sh.RowHidden(2) || !sh.RowHidden(5000) || !sh.IndexHidden(false, 700) {
		t.Fatalf("heights %v widths %v", sh.RowHeight, sh.ColWidth)
	}
	if b.Hide(0, true, []int{2}) {
		t.Error("hiding a hidden row changed something")
	}
	// The whole sheet: rows and columns far below and right of the data.
	if !b.Unhide(0, true, 0, model.MaxRows-1) || !b.Unhide(0, false, 0, model.MaxCols-1) {
		t.Fatal("nothing shown")
	}
	if sh.RowHeight[2] != 40 || sh.ColWidth[700] != 22 || sh.RowHidden(3) || sh.RowHidden(5000) {
		t.Errorf("unhide: heights %v widths %v", sh.RowHeight, sh.ColWidth)
	}
	if _, ok := sh.RowHeight[3]; ok {
		t.Errorf("row 4 got a height: %v", sh.RowHeight)
	}
	if b.Unhide(0, true, 0, model.MaxRows-1) {
		t.Error("unhide with nothing hidden changed something")
	}
	b.Undo()
	if !sh.IndexHidden(false, 700) {
		t.Errorf("undo unhide cols: %v", sh.ColWidth)
	}
	b.Undo()
	b.Undo()
	b.Undo()
	if sh.RowHeight[2] != 40 || sh.ColWidth[700] != 22 || sh.RowHidden(5000) {
		t.Errorf("undo all: heights %v widths %v", sh.RowHeight, sh.ColWidth)
	}
	b.Redo()
	if !sh.RowHidden(2) || !sh.RowHidden(3) {
		t.Errorf("redo: %v", sh.RowHeight)
	}
}
