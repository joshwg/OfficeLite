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
