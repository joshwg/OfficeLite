package ui

import (
	"strconv"

	"officelite/excel/model"
)

// scrollHints sets the scrollbars' arrow steps (a row, a column) and the
// hint Excel shows beside a dragged thumb: "Row: 120", "Column: AB".
func (a *App) scrollHints() {
	st := a.state()
	fr, fc := st.view.Sheet.FreezeRows, st.view.Sheet.FreezeCols
	a.vbar.Vertical, a.vbar.Step, a.vbar.Theme, a.vbar.OpenEnded = true, 1, a.ui, true
	a.vbar.Label = func(pos float64) string { return "Row: " + strconv.Itoa(fr+int(pos+0.5)+1) }
	a.hbar.Step, a.hbar.Theme, a.hbar.OpenEnded = 1, a.ui, true
	a.hbar.Label = func(pos float64) string { return "Column: " + model.ColName(fc+int(pos+0.5)) }
}

// extent is the part of a sheet holding values or formulas, cached until
// the next change (formatting alone doesn't count: .xls files often format
// rows out to column IV).
type extent struct {
	rows, cols int
	ok         bool
}

// dataExtent returns the number of rows and columns up to the last cell
// with content. Excel's scrollbars reach that far (plus one) and no
// further, unless the view or selection is already beyond it.
func (a *App) dataExtent(st *sheetState) (rows, cols int) {
	if !st.extent.ok {
		sh := st.view.Sheet
		var e extent
		for r := range sh.UsedRows() {
			for c := sh.RowLen(r) - 1; c >= 0; c-- {
				if cell := sh.Get(r, c); cell.Kind != model.KindEmpty || cell.Formula != "" {
					e.rows, e.cols = r+1, max(e.cols, c+1)
					break
				}
			}
		}
		e.ok = true
		st.extent = e
	}
	return st.extent.rows, st.extent.cols
}
