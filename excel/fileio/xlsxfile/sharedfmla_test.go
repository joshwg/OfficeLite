// Tests for shared formulas (<f t="shared" si>): each master is scanned
// once and shifted per cell, with the same text formula.Shift gives.

package xlsxfile

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"officelite/excel/formula"
	"officelite/excel/model"
)

// sharedSheet returns a package whose Sheet1 holds the master formula
// master at B3 (si 0) and cells sharing it at every cell of rows×cols
// from A1 after B3 (a shared cell needs its master read first).
func sharedSheet(t *testing.T, master string, rows, cols int) []byte {
	t.Helper()
	sh := model.NewSheet("Sheet1")
	sh.Set(0, 0, model.Cell{Kind: model.KindNumber, Num: 1, Text: "1"})
	wb := &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
	var sd strings.Builder
	sd.WriteString("<sheetData>")
	ref := func(r, c int) string { return model.ColName(c) + fmt.Sprint(r+1) }
	for r := 2; r < rows; r++ {
		fmt.Fprintf(&sd, `<row r="%d">`, r+1)
		if r == 2 {
			fmt.Fprintf(&sd, `<c r="B3"><f t="shared" ref="A1:%s" si="0">%s</f><v>0</v></c>`, ref(rows-1, cols-1), xmlEscape(master))
		}
		for c := range cols {
			if r == 2 && c <= 1 {
				continue
			}
			fmt.Fprintf(&sd, `<c r="%s"><f t="shared" si="0"/><v>0</v></c>`, ref(r, c))
		}
		sd.WriteString("</row>")
	}
	sd.WriteString("</sheetData>")
	data := regexp.MustCompile(`(?s)<sheetData>.*</sheetData>|<sheetData/>`)
	return rewritePart(t, mustWrite(t, wb), "xl/worksheets/sheet1.xml", func(s string) string {
		if !data.MatchString(s) {
			t.Fatal("no sheetData")
		}
		return data.ReplaceAllLiteralString(s, sd.String())
	})
}

// Every cell sharing a master gets exactly the text formula.Shift gives
// for the master moved by the cell's offset (the per-cell path's result),
// including references moved off the sheet.
func TestSharedFormulaCells(t *testing.T) {
	const master = `SUM(A1:B2)+$C$1*D$4+$E5+A:A+3:3+Other!A1+'My Sheet'!B2:C3+LEN("A1")+#REF!`
	const rows, cols = 6, 5
	wb, err := Read(sharedSheet(t, master, rows, cols))
	if err != nil {
		t.Fatal(err)
	}
	sh := wb.Sheets[0]
	for r := 2; r < rows; r++ {
		for c := range cols {
			if r == 2 && c < 1 {
				continue
			}
			want := formula.Shift(master, r-2, c-1)
			if got := sh.Get(r, c).Formula; got != want {
				t.Errorf("%s%d: %q, want %q", model.ColName(c), r+1, got, want)
			}
		}
	}
}

// Expanding a shared formula costs the same per cell however many cells
// share it: 4× the cells take well under 8× the time.
func TestSharedFormulaScales(t *testing.T) {
	master := strings.Repeat("A1+$B$2*C3:D4-Sheet1!E5+", 100) + "1"
	small, large := sharedSheet(t, master, 250, 4), sharedSheet(t, master, 1000, 4)
	timeRead := func(data []byte) time.Duration {
		best := time.Duration(1 << 62)
		for range 3 {
			start := time.Now()
			if _, err := Read(data); err != nil {
				t.Fatal(err)
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	ts, tl := timeRead(small), timeRead(large)
	if tl > 8*ts {
		t.Errorf("1000 rows took %v, 250 rows %v: more than 8×", tl, ts)
	}
}
