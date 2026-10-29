// Glyph painting: PaintGlyphs (system rasterizer where available, else Gio
// outlines) and TextLabel, a widget.Label that paints through it.

package gui

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

// PaintGlyphs paints one line of glyphs shaped by sh in color c, with the
// line's origin (the X the glyph positions count from, and the baseline)
// at x, y in the current coordinates.
//
// On Windows, glyphs from a shaper made by a FontLibrary are drawn by
// DirectWrite, hinted and with Windows' gamma and contrast, the way Word
// and Excel draw text; their positions stay exactly where Gio's shaper put
// them, so layout, caret and selection are unchanged. The image is placed
// on whole pixels, so the current transform should be a whole-pixel
// offset. Everywhere else, and for any glyph DirectWrite can't draw, Gio
// fills the glyph outlines.
func PaintGlyphs(ops *op.Ops, sh *text.Shaper, gs []text.Glyph, x, y float32, c color.NRGBA) {
	if len(gs) == 0 {
		return
	}
	if paintSystemGlyphs(ops, sh, gs, x, y, c) {
		return
	}
	paintOutlines(ops, sh, gs, x, y, c)
}

// paintOutlines is PaintGlyphs by Gio alone: it fills the glyph outlines.
func paintOutlines(ops *op.Ops, sh *text.Shaper, gs []text.Glyph, x, y float32, c color.NRGBA) {
	if len(gs) == 0 {
		return
	}
	t := op.Affine(f32.AffineId().Offset(f32.Pt(x+fixedToFloat(gs[0].X), y))).Push(ops)
	cl := clip.Outline{Path: sh.Shape(gs)}.Op().Push(ops)
	paint.ColorOp{Color: c}.Add(ops)
	paint.PaintOp{}.Add(ops)
	cl.Pop()
	sh.Bitmaps(gs).Add(ops)
	t.Pop()
}

// fixedToFloat converts a 26.6 fixed-point value to a float.
func fixedToFloat(v fixed.Int26_6) float32 { return float32(v) / 64 }

// TextLabel lays out and paints text like widget.Label (Alignment and
// wrapping within the constraints, and the same dimensions), but paints
// through PaintGlyphs.
type TextLabel struct {
	Alignment text.Alignment
	// Outlines paints with Gio's outlines even where PaintGlyphs would
	// use the system's rasterizer: for text drawn scaled.
	Outlines bool
}

// Layout lays out txt with p's font and size and paints it in c.
func (l TextLabel) Layout(gtx layout.Context, sh *text.Shaper, p text.Parameters, txt string, c color.NRGBA) layout.Dimensions {
	cs := gtx.Constraints
	p.Alignment = l.Alignment
	p.MaxWidth, p.MinWidth = cs.Max.X, cs.Min.X
	p.Locale = gtx.Locale
	sh.LayoutString(p, txt)

	// As widget.Label: bounds is the logical box of the visible glyphs,
	// pad how far glyph ink overhangs it.
	var bounds, pad image.Rectangle
	first, baseline := true, 0
	view := image.Rectangle{Max: cs.Max}
	var line []text.Glyph
	flush := func() {
		if len(line) > 0 {
			if l.Outlines {
				paintOutlines(gtx.Ops, sh, line, 0, float32(line[0].Y), c)
			} else {
				PaintGlyphs(gtx.Ops, sh, line, 0, float32(line[0].Y), c)
			}
			line = line[:0]
		}
	}
	m := op.Record(gtx.Ops)
	for g, ok := sh.NextGlyph(); ok; g, ok = sh.NextGlyph() {
		pad.Min.X = min(pad.Min.X, g.Bounds.Min.X.Floor())
		pad.Max.X = max(pad.Max.X, (g.Bounds.Max.X - g.Advance).Ceil())
		pad.Min.Y = min(pad.Min.Y, (g.Bounds.Min.Y + g.Ascent).Floor())
		pad.Max.Y = max(pad.Max.Y, (g.Bounds.Max.Y - g.Descent).Ceil())
		lb := image.Rectangle{
			Min: image.Pt(g.X.Floor(), int(g.Y)-g.Ascent.Ceil()),
			Max: image.Pt((g.X + g.Advance).Ceil(), int(g.Y)+g.Descent.Ceil()),
		}
		if first {
			first, baseline, bounds = false, int(g.Y), lb
		}
		if lb.Min.Y > view.Max.Y {
			break
		}
		visible := lb.Max.Y >= view.Min.Y && lb.Max.X >= view.Min.X && lb.Min.X <= view.Max.X
		if visible {
			bounds = bounds.Union(lb)
			line = append(line, g)
		}
		if g.Flags&text.FlagLineBreak != 0 || !visible {
			flush()
		}
	}
	flush()
	call := m.Stop()
	view.Min = view.Min.Add(pad.Min)
	view.Max = view.Max.Add(pad.Max)
	cl := clip.Rect(view).Push(gtx.Ops)
	call.Add(gtx.Ops)
	cl.Pop()
	dims := layout.Dimensions{Size: cs.Constrain(bounds.Size())}
	dims.Baseline = dims.Size.Y - baseline
	return dims
}
