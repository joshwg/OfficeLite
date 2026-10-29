package fileio

import (
	"strconv"
	"testing"

	"officelite/excel/model"
)

// outlineWorkbook has grouped rows (two levels, the inner one collapsed,
// a custom height kept while hidden) and grouped columns with summaries
// at the left, the group collapsed.
func outlineWorkbook() *model.Workbook {
	sh := model.NewSheet("Outline")
	for r := 0; r < 12; r++ {
		sh.Set(r, 0, model.Cell{Kind: model.KindNumber, Num: float64(r), Text: strconv.Itoa(r)})
	}
	for r := 1; r <= 8; r++ {
		sh.SetLevel(true, r, 1)
	}
	for r := 2; r <= 4; r++ {
		sh.SetLevel(true, r, 2)
	}
	sh.RowHeight[3] = 30
	sh.CollapseGroup(model.OutlineGroup{Rows: true, Level: 2, Lo: 2, Hi: 4, Summary: 5})
	sh.Outline.SummaryLeft = true
	sh.ColWidth[3] = 20
	for c := 2; c <= 4; c++ {
		sh.SetLevel(false, c, 1)
	}
	sh.CollapseGroup(model.OutlineGroup{Level: 1, Lo: 2, Hi: 4, Summary: 1})
	sh.SetLevel(false, 7, 3) // a deeper column group, no width of its own
	return &model.Workbook{Name: "outline", Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
}

func TestOutlineRoundTrip(t *testing.T) {
	for _, f := range []Format{XLSX, XLS} {
		t.Run(f.Ext(), func(t *testing.T) {
			wb := outlineWorkbook()
			var got *model.Workbook
			if f == XLSX {
				got = roundTrip(t, wb)
			} else {
				got = writeXLS(t, wb)
			}
			compare(t, wb, got)
			sh := got.Sheets[0]
			if sh.MaxLevel(true) != 2 || sh.MaxLevel(false) != 3 || !sh.IsCollapsed(true, 5) || !sh.IsCollapsed(false, 1) {
				t.Fatalf("levels %d %d, collapsed %v %v", sh.MaxLevel(true), sh.MaxLevel(false), sh.Outline.RowCollapsed, sh.Outline.ColCollapsed)
			}
			if !sh.Outline.SummaryLeft || sh.Outline.SummaryAbove {
				t.Errorf("summary left %v above %v", sh.Outline.SummaryLeft, sh.Outline.SummaryAbove)
			}
			// Expanding gives back the hidden row's height and column's width.
			for _, rows := range []bool{true, false} {
				for _, g := range sh.Groups(rows) {
					if g.Collapsed {
						sh.ExpandGroup(g)
					}
				}
			}
			if sh.RowHeightPx(3) != 30 || sh.RowHeightPx(2) != sh.DefaultRowHeight {
				t.Errorf("row heights %v %v", sh.RowHeightPx(3), sh.RowHeightPx(2))
			}
			if w := sh.ColWidth[3]; w != 20 {
				t.Errorf("column D width %v", w)
			}
			if _, ok := sh.ColWidth[7]; ok {
				t.Errorf("column H got a width: %v", sh.ColWidth[7])
			}
		})
	}
}
