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
	a.vbar.Vertical, a.vbar.Step, a.vbar.Theme = true, 1, a.ui
	a.vbar.Label = func(pos float64) string { return "Row: " + strconv.Itoa(fr+int(pos+0.5)+1) }
	a.hbar.Step, a.hbar.Theme = 1, a.ui
	a.hbar.Label = func(pos float64) string { return "Column: " + model.ColName(fc+int(pos+0.5)) }
}
