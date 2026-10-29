package xlsxfile

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"officelite/excel/model"
)

// A saved view far outside the sheet (or negative) is clamped on reading;
// an active cell of ZZZZZZZ99999999 used to make the save panic in
// model.ColName.
func TestSheetViewClamped(t *testing.T) {
	sh := model.NewSheet("Sheet1")
	sh.Set(0, 0, model.Cell{Kind: model.KindNumber, Num: 1, Text: "1"})
	sh.Merges = []model.Range{{R1: 1, C1: 1, R2: 2, C2: 2}}
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	views := regexp.MustCompile(`(?s)<sheetViews>.*</sheetViews>`)
	data := rewritePart(t, buf.Bytes(), "xl/worksheets/sheet1.xml", func(s string) string {
		if !views.MatchString(s) {
			t.Fatal("no sheetViews")
		}
		s = views.ReplaceAllString(s, `<sheetViews><sheetView workbookViewId="0" topLeftCell="XFD1048576">`+
			`<pane xSplit="1e300" ySplit="-5" state="frozen"/><selection activeCell="ZZZZZZZ99999999"/></sheetView></sheetViews>`)
		if !strings.Contains(s, `<mergeCell ref="B2:C3"`) {
			t.Fatal("no mergeCell")
		}
		return strings.Replace(s, `<mergeCell ref="B2:C3"`, `<mergeCell ref="C3:B2"`, 1)
	})
	got, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	g := got.Sheets[0]
	if g.TopRow != model.MaxRows-1 || g.LeftCol != model.MaxCols-1 {
		t.Errorf("top-left = %d,%d", g.TopRow, g.LeftCol)
	}
	if g.ActiveRow < 0 || g.ActiveRow >= model.MaxRows || g.ActiveCol < 0 || g.ActiveCol >= model.MaxCols {
		t.Errorf("active = %d,%d", g.ActiveRow, g.ActiveCol)
	}
	if g.FreezeRows != 0 || g.FreezeCols < 0 || g.FreezeCols >= model.MaxCols {
		t.Errorf("freeze = %d×%d", g.FreezeRows, g.FreezeCols)
	}
	if want := (model.Range{R1: 1, C1: 1, R2: 2, C2: 2}); len(g.Merges) != 1 || g.Merges[0] != want {
		t.Errorf("merges = %v, want %v", g.Merges, want)
	}
	if err := Write(got, &buf); err != nil {
		t.Fatal(err)
	}
}

func TestParseCellRefBounds(t *testing.T) {
	for _, s := range []string{"XFE1", "ZZZZZZZ99999999", "A1048577", "A0", "A-1"} {
		if _, _, ok := parseCellRef(s); ok {
			t.Errorf("parseCellRef(%q) accepted", s)
		}
	}
	if r, c, ok := parseCellRef("XFD1048576"); !ok || r != 1048575 || c != 16383 {
		t.Errorf("XFD1048576 = %d,%d,%v", r, c, ok)
	}
}
