package edit

import (
	"slices"

	"officelite/excel/calc"
	"officelite/excel/model"
)

// Charts: deleting one (an undoable sheet change), and keeping their
// references in step with structural edits (see rewriteFormulas).

// DeleteChart removes chart idx of sheet. A chart sheet's chart can't
// be deleted (delete the sheet).
func (b *Book) DeleteChart(sheet, idx int) calc.Result {
	sh := b.WB.Sheets[sheet]
	if idx < 0 || idx >= len(sh.Charts) {
		return calc.Result{}
	}
	ch := &sheetChange{sheet: sheet, before: propsOf(sh)}
	ch.after = propsOf(sh)
	ch.after.charts = slices.Delete(ch.after.charts, idx, idx+1)
	a := sh.Charts[idx].Anchor.From
	return b.applySheet(Where{sheet, model.Range{R1: a.Row, C1: a.Col, R2: a.Row, C2: a.Col}}, ch, nil)
}

// sheetCharts is a sheet's charts (and chart sheet's chart) before a
// change rewrote their references.
type sheetCharts struct {
	charts []*model.Chart
	sheet  *model.Chart
}

func chartsOf(sh *model.Sheet) sheetCharts {
	return sheetCharts{model.CloneCharts(sh.Charts), sh.ChartSheet.Clone()}
}

func (c sheetCharts) restore(sh *model.Sheet) {
	sh.Charts = model.CloneCharts(c.charts)
	sh.ChartSheet = c.sheet.Clone()
}

// adjustCharts rewrites the references of sh's charts; it reports
// whether any changed.
func adjustCharts(sh *model.Sheet, fn func(f, home string) string) bool {
	changed := false
	rewrite := func(c *model.Chart) {
		c.Refs(func(ref *string) {
			if g := fn(*ref, sh.Name); g != *ref {
				*ref, changed = g, true
			}
		})
	}
	for _, c := range sh.Charts {
		rewrite(c)
	}
	if sh.ChartSheet != nil {
		rewrite(sh.ChartSheet)
	}
	return changed
}
