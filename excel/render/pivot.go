package render

import (
	"image"

	"gioui.org/op"
)

// The drop-down buttons of PivotTable reports: on the Row Labels and
// Column Labels (or field name) cells and the report filters' values.
// They look like the AutoFilter's.

// drawPivotButtons draws the report buttons in pane p.
func (v *View) drawPivotButtons(ops *op.Ops, p *pane) {
	for _, pt := range v.Sheet.Pivots {
		for _, b := range pt.Out.Buttons {
			if b.Row < p.r0 || b.Row >= p.r1 || b.Col < p.c0 || b.Col >= p.c1 || v.Geo.Rows.Hidden(b.Row) || v.Geo.Cols.Hidden(b.Col) {
				continue
			}
			v.drawFilterButton(ops, v.filterButton(v.cellRect(p, b.Row, b.Col)), b.Filtered)
		}
	}
}

// PivotButtonRect is the on-screen rectangle (grid coordinates, from the
// last Draw) of the report button on cell row, col; ok is false when it
// isn't showing.
func (v *View) PivotButtonRect(row, col int) (image.Rectangle, bool) {
	if v.scale == 0 || v.Geo.Rows.Hidden(row) || v.Geo.Cols.Hidden(col) {
		return image.Rectangle{}, false
	}
	if _, _, ok := v.Sheet.PivotButtonAt(row, col); !ok {
		return image.Rectangle{}, false
	}
	for _, p := range v.panes() {
		if row < p.r0 || row >= p.r1 || col < p.c0 || col >= p.c1 {
			continue
		}
		b := v.filterButton(v.cellRect(&p, row, col))
		if b = b.Intersect(p.clip); !b.Empty() {
			return b, true
		}
	}
	return image.Rectangle{}, false
}

// PivotButtonAt reports the cell of the report button at x, y (grid
// coordinates, from the last Draw).
func (v *View) PivotButtonAt(x, y float32) (row, col int, ok bool) {
	pt := image.Pt(int(x), int(y))
	for _, p := range v.Sheet.Pivots {
		for _, b := range p.Out.Buttons {
			if r, ok := v.PivotButtonRect(b.Row, b.Col); ok && pt.In(r) {
				return b.Row, b.Col, true
			}
		}
	}
	return 0, 0, false
}
