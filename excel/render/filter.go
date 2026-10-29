// The AutoFilter's drop-down buttons at the right edge of its header
// cells, and the blue row numbers of a filtered range.

package render

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"officelite/common/colors"
	"officelite/common/gui"
)

var (
	filterBtnBG     = colors.RGB(0xFFFFFF)
	filterBtnBorder = colors.RGB(0xA6A6A6)
	filterBtnArrow  = colors.RGB(0x404040)
	filterRowText   = colors.RGB(0x0070C0)
)

// filterButtonSize is a drop-down button's side, unzoomed.
const filterButtonSize = 17

// filterButton is the drop-down button of header cell rc.
func (v *View) filterButton(rc image.Rectangle) image.Rectangle {
	s := min(v.px(filterButtonSize), rc.Dx()-1, rc.Dy()-1)
	return image.Rect(rc.Max.X-s-1, rc.Max.Y-s-1, rc.Max.X-1, rc.Max.Y-1)
}

// drawFilterButtons draws the buttons of the header cells in pane p.
func (v *View) drawFilterButtons(ops *op.Ops, p *pane) {
	f := v.Sheet.AutoFilter
	if f == nil {
		return
	}
	r := f.Range.R1
	if r < p.r0 || r >= p.r1 || v.Geo.Rows.Hidden(r) {
		return
	}
	for c := max(f.Range.C1, p.c0); c <= min(f.Range.C2, p.c1-1); c++ {
		fc, filtered := f.Column(c)
		if v.Geo.Cols.Hidden(c) || filtered && fc.HideButton {
			continue
		}
		v.drawFilterButton(ops, v.filterButton(v.cellRect(p, r, c)), filtered)
	}
}

// drawFilterButton draws one button: a small square with a down arrow,
// or with a funnel and a smaller arrow when the column is filtered.
func (v *View) drawFilterButton(ops *op.Ops, b image.Rectangle, filtered bool) {
	gui.FillRect(ops, b, filterBtnBorder)
	gui.FillRect(ops, b.Inset(1), filterBtnBG)
	s := float32(b.Dx())
	o := f32.Pt(float32(b.Min.X), float32(b.Min.Y))
	at := func(x, y float32) f32.Point { return o.Add(f32.Pt(x*s, y*s)) }
	if !filtered {
		fillPoly(ops, filterBtnArrow, at(0.27, 0.40), at(0.73, 0.40), at(0.5, 0.66))
		return
	}
	// The funnel on the left, a small arrow at the bottom right.
	fillPoly(ops, filterBtnArrow, at(0.16, 0.22), at(0.62, 0.22), at(0.44, 0.46), at(0.44, 0.80), at(0.34, 0.73), at(0.34, 0.46))
	fillPoly(ops, filterBtnArrow, at(0.60, 0.62), at(0.88, 0.62), at(0.74, 0.80))
}

// fillPoly fills a polygon.
func fillPoly(ops *op.Ops, c color.NRGBA, pts ...f32.Point) {
	var p clip.Path
	p.Begin(ops)
	p.MoveTo(pts[0])
	for _, q := range pts[1:] {
		p.LineTo(q)
	}
	p.Close()
	defer clip.Outline{Path: p.End()}.Op().Push(ops).Pop()
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
}

// FilterButtonAt reports the column of the AutoFilter drop-down button
// at x, y (grid coordinates, from the last Draw).
func (v *View) FilterButtonAt(x, y float32) (col int, ok bool) {
	f := v.Sheet.AutoFilter
	if f == nil || v.scale == 0 {
		return 0, false
	}
	pt := image.Pt(int(x), int(y))
	for c := f.Range.C1; c <= f.Range.C2; c++ {
		if b, ok := v.FilterButtonRect(c); ok && pt.In(b) {
			return c, true
		}
	}
	return 0, false
}

// FilterButtonRect is the on-screen rectangle (grid coordinates, from
// the last Draw) of column col's drop-down button; ok is false when it
// isn't showing.
func (v *View) FilterButtonRect(col int) (image.Rectangle, bool) {
	f := v.Sheet.AutoFilter
	if f == nil || v.scale == 0 || col < f.Range.C1 || col > f.Range.C2 {
		return image.Rectangle{}, false
	}
	if fc, ok := f.Column(col); ok && fc.HideButton {
		return image.Rectangle{}, false
	}
	r := f.Range.R1
	if v.Geo.Rows.Hidden(r) || v.Geo.Cols.Hidden(col) {
		return image.Rectangle{}, false
	}
	for _, p := range v.panes() {
		if r < p.r0 || r >= p.r1 || col < p.c0 || col >= p.c1 {
			continue
		}
		b := v.filterButton(v.cellRect(&p, r, col))
		if b = b.Intersect(p.clip); !b.Empty() {
			return b, true
		}
	}
	return image.Rectangle{}, false
}

// filteredRow reports whether row r's number is drawn blue: a data row of
// an AutoFilter with criteria.
func (v *View) filteredRow(r int) bool {
	f := v.Sheet.AutoFilter
	return f.Active() && r > f.Range.R1 && r <= f.Range.R2
}
