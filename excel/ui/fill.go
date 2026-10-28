package ui

import (
	"fmt"
	"image"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"officelite/common/gui"
	"officelite/excel/model"
	"officelite/excel/render"
)

// Fill Down / Fill Right and the fill handle.

// fillDown copies the selection's top row into the rest of it (Ctrl+D).
func (a *App) fillDown() { a.fill(true) }

// fillRight copies the selection's left column into the rest (Ctrl+R).
func (a *App) fillRight() { a.fill(false) }

func (a *App) fill(down bool) {
	if !a.commitForCommand() {
		return
	}
	sel := a.state().view.Sel.Range
	if down {
		a.changed(a.book.FillDown(a.active, sel))
	} else {
		a.changed(a.book.FillRight(a.active, sel))
	}
}

// fillDrag is a drag of the fill handle.
type fillDrag struct {
	active bool
	src    model.Range
	to     model.Range
}

// fillHandleRect is where the fill handle is drawn, in grid coordinates.
func (a *App) fillHandleRect(gtx layout.Context, v *render.View) (image.Rectangle, bool) {
	r := v.Sel.Range
	if r.R2 == model.MaxRows-1 && r.C2 == model.MaxCols-1 {
		return image.Rectangle{}, false
	}
	rc, pane, ok := v.CellRect(r.R2, r.C2)
	if !ok || !pane.Overlaps(rc) {
		return image.Rectangle{}, false
	}
	h := max(gtx.Dp(5), 4)
	c := image.Pt(rc.Max.X, rc.Max.Y)
	return image.Rect(c.X-h, c.Y-h, c.X+h, c.Y+h), true
}

// fillPress starts a fill-handle drag when the press is on the handle.
func (a *App) fillPress(v *render.View, pos image.Point) bool {
	if a.ed.active || a.gtx.Ops == nil {
		return false
	}
	hr, ok := a.fillHandleRect(a.gtx, v)
	if !ok || !pos.In(hr) {
		return false
	}
	sel := v.Sel.Range
	// Whole rows or columns fill only their used part.
	if sel.R2 == model.MaxRows-1 {
		sel.R2 = max(v.Sheet.UsedRows()-1, sel.R1)
	}
	if sel.C2 == model.MaxCols-1 {
		sel.C2 = max(v.Sheet.UsedCols()-1, sel.C1)
	}
	a.ins.fill = fillDrag{active: true, src: sel, to: sel}
	return true
}

// fillTarget is the range a drag to hit fills: src extended down, up,
// right or left, whichever the pointer went furthest, or shrunk.
func fillTarget(src model.Range, hit render.Hit) model.Range {
	r, c := hit.Row, hit.Col
	switch hit.Area {
	case render.AreaColHeader:
		r = src.R2
	case render.AreaRowHeader:
		c = src.C2
	}
	down, up := r-src.R2, src.R1-r
	right, left := c-src.C2, src.C1-c
	to := src
	switch m := max(down, up, right, left); {
	case m <= 0:
		// Inside: shrink along the longer side.
		if src.R2-src.R1 >= src.C2-src.C1 {
			to.R2 = max(r, src.R1)
		} else {
			to.C2 = max(c, src.C1)
		}
	case m == down:
		to.R2 = r
	case m == up:
		to.R1 = r
	case m == right:
		to.C2 = c
	default:
		to.C1 = c
	}
	return to
}

// fillMove follows the pointer during a fill-handle drag.
func (a *App) fillMove(v *render.View, pos image.Point) {
	hit := v.HitTest(float32(pos.X), float32(pos.Y))
	to := fillTarget(a.ins.fill.src, hit)
	to.R1, to.C1 = max(to.R1, 0), max(to.C1, 0)
	to.R2, to.C2 = min(to.R2, model.MaxRows-1), min(to.C2, model.MaxCols-1)
	a.ins.fill.to = to
	v.EnsureVisible(min(max(hit.Row, 0), model.MaxRows-1), min(max(hit.Col, 0), model.MaxCols-1))
	a.notice = fmt.Sprintf("Fill %s", rangeName(to))
}

// fillRelease runs the fill when the handle is let go.
func (a *App) fillRelease(ok bool) {
	f := a.ins.fill
	a.ins.fill = fillDrag{}
	a.notice = ""
	if !ok || f.to == f.src {
		return
	}
	res, err := a.book.AutoFill(a.active, f.src, f.to)
	if err != nil {
		a.errMsg = err.Error()
		return
	}
	a.changed(res)
	a.selectRange(a.active, f.to)
}

// drawFillDrag outlines the range a fill-handle drag would fill, and sets
// the thin cross cursor over the handle.
func (a *App) drawFillDrag(gtx layout.Context, v *render.View) {
	if hr, ok := a.fillHandleRect(gtx, v); ok && !a.ed.active {
		area := clip.Rect(hr).Push(gtx.Ops)
		pointer.CursorCrosshair.Add(gtx.Ops)
		area.Pop()
	}
	f := &a.ins.fill
	if !f.active {
		return
	}
	r1, _, ok1 := v.CellRect(f.to.R1, f.to.C1)
	r2, _, ok2 := v.CellRect(f.to.R2, f.to.C2)
	if !ok1 && !ok2 {
		return
	}
	var box image.Rectangle
	switch {
	case ok1 && ok2:
		box = r1.Union(r2)
	case ok1:
		box = image.Rectangle{Min: r1.Min, Max: gtx.Constraints.Max}
	default:
		box = image.Rectangle{Max: r2.Max}
	}
	t := max(gtx.Dp(1), 1)
	on, off := gtx.Dp(3), gtx.Dp(2)
	c := colorsGray
	gui.DashedLine(gtx.Ops, box.Min.X, box.Min.Y, box.Max.X, box.Min.Y, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X, box.Max.Y-1, box.Max.X, box.Max.Y-1, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Min.X, box.Min.Y, box.Min.X, box.Max.Y, t, on, off, c)
	gui.DashedLine(gtx.Ops, box.Max.X-1, box.Min.Y, box.Max.X-1, box.Max.Y, t, on, off, c)
}

var colorsGray = gui.MutedText

// rangeName is A1 or A1:B2.
func rangeName(r model.Range) string {
	s := model.CellName(r.R1, r.C1)
	if r.R1 != r.R2 || r.C1 != r.C2 {
		s += ":" + model.CellName(r.R2, r.C2)
	}
	return s
}
