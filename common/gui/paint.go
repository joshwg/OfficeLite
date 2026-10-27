package gui

import (
	"image"
	"image/color"

	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// FillRect paints r with c.
func FillRect(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	if r.Empty() {
		return
	}
	paint.FillShape(ops, c, clip.Rect(r).Op())
}

// HLine paints a horizontal line of the given thickness whose top edge is y.
func HLine(ops *op.Ops, x0, x1, y, thickness int, c color.NRGBA) {
	FillRect(ops, image.Rect(x0, y, x1, y+thickness), c)
}

// VLine paints a vertical line of the given thickness whose left edge is x.
func VLine(ops *op.Ops, x, y0, y1, thickness int, c color.NRGBA) {
	FillRect(ops, image.Rect(x, y0, x+thickness, y1), c)
}

// DashedLine paints an axis-aligned line from (x0,y0) to (x1,y1) as a run
// of dashes: on pixels drawn, off pixels skipped, repeating.
func DashedLine(ops *op.Ops, x0, y0, x1, y1, thickness, on, off int, c color.NRGBA) {
	if on <= 0 {
		return
	}
	horizontal := y0 == y1
	start, end := y0, y1
	if horizontal {
		start, end = x0, x1
	}
	for p := start; p < end; p += on + off {
		q := min(p+on, end)
		if horizontal {
			FillRect(ops, image.Rect(p, y0, q, y0+thickness), c)
		} else {
			FillRect(ops, image.Rect(x0, p, x0+thickness, q), c)
		}
	}
}

// Text material: a recorded color op for widget.Label.
func Material(ops *op.Ops, c color.NRGBA) op.CallOp {
	m := op.Record(ops)
	paint.ColorOp{Color: c}.Add(ops)
	return m.Stop()
}
