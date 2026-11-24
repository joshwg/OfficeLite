package layout

import (
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word"
	"officelite/word/model"
)

var (
	placeholderBG   = colors.RGB(0xF2F2F2)
	placeholderLine = colors.RGB(0xBFBFBF)
	placeholderText = colors.RGB(0x7F7F7F)
	ruleColor       = colors.RGB(0x000000)
	fieldShade      = colors.RGB(0xD9D9D9)
)

// DrawOptions control DrawPage.
type DrawOptions struct {
	Scale     float64 // pixels per point
	Sel       model.Selection
	SelColor  color.NRGBA
	Y0, Y1    float64 // visible part of the page (points); both 0 = all
	NoPageBox bool    // don't paint the white sheet
	// HFDim dims the headers and footers (the page view while the body is
	// edited); BodyDim dims the body instead (while they are edited).
	HFDim, BodyDim bool
	// ShowMarks draws formatting marks (Show/Hide ¶; see marks.go).
	ShowMarks bool
}

// DrawPage draws a page with its top-left corner at the current origin.
func (e *Engine) DrawPage(ops *op.Ops, pg *Page, o DrawOptions) {
	s := o.Scale
	px := func(v float64) int { return int(math.Round(v * s)) }
	rect := func(r Rect) image.Rectangle {
		return image.Rect(px(r.X0), px(r.Y0), max(px(r.X1), px(r.X0)+1), max(px(r.Y1), px(r.Y0)+1))
	}
	if !o.NoPageBox {
		gui.FillRect(ops, image.Rect(0, 0, px(pg.Width), px(pg.Height)), colors.White)
	}
	y0, y1 := o.Y0, o.Y1
	if y0 == 0 && y1 == 0 {
		y1 = pg.Height
	}
	visible := func(pl *Placed) bool { return pl.Y+pl.Line.Height >= y0 && pl.Y <= y1 }
	e.drawFloats(ops, pg, true, s, o.ShowMarks)
	for _, f := range pg.Fills {
		gui.FillRect(ops, rect(f.R), f.C)
	}
	// Highlighting behind text.
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		if !visible(pl) {
			continue
		}
		for k := range pl.Line.Items {
			it := &pl.Line.Items[k]
			if it.St != nil && it.St.Highlight.A > 0 && !it.St.Hidden && it.W > 0 {
				w := it.W
				if k == len(pl.Line.Items)-1 {
					w -= it.trail
				}
				base := pl.Y + pl.Line.Baseline
				gui.FillRect(ops, rect(Rect{pl.X + it.X, base - pl.Line.Asc, pl.X + it.X + w, base + pl.Line.Desc}), it.St.Highlight)
			}
		}
	}
	// Word shades a field's result while the caret is in it.
	for _, f := range e.fields {
		if c := o.Sel.Caret; !c.Less(f.From) && c.Less(f.To) {
			for _, r := range e.SelectionRects(pg, f.From, f.To) {
				gui.FillRect(ops, rect(r), fieldShade)
			}
		}
	}
	if !o.Sel.Empty() {
		a, b := o.Sel.Ordered()
		for _, r := range e.SelectionRects(pg, a, b) {
			gui.FillRect(ops, rect(r), o.SelColor)
		}
	}
	for i := range pg.Lines {
		if pl := &pg.Lines[i]; visible(pl) {
			e.drawLine(ops, pl, s)
		}
	}
	if o.ShowMarks {
		e.drawMarks(ops, e.Marks(pg.Lines, pg.TableRows), y0, y1, s)
	}
	for _, r := range pg.Rules {
		rr := rect(r)
		if rr.Dx() < rr.Dy() {
			rr.Max.X = rr.Min.X + max(1, px(0.75))
		} else {
			rr.Max.Y = rr.Min.Y + max(1, px(0.75))
		}
		gui.FillRect(ops, rr, ruleColor)
	}
	for _, b := range pg.Notes {
		e.DrawBand(ops, b, s)
		if o.ShowMarks {
			e.drawMarks(ops, e.Marks(b.Lines, nil), 0, 0, s)
		}
	}
	e.drawFloats(ops, pg, false, s, o.ShowMarks)
	e.drawPictureFrame(ops, pg, o.Sel, s)
	e.drawBands(ops, pg, o)
}

// drawLine draws a line's label, text, objects and decorations.
func (e *Engine) drawLine(ops *op.Ops, pl *Placed, s float64) {
	l := pl.Line
	base := pl.Y + l.Baseline
	if lb := l.Label; lb != nil {
		e.drawText(ops, lb.St, lb.Text, (pl.X+lb.X)*s, base*s, s, nil)
	}
	items := l.Items
	for k := 0; k < len(items); k++ {
		it := &items[k]
		if it.St == nil || it.St.Hidden {
			continue
		}
		switch it.Kind {
		case ItemText:
			// Join neighboring words of one style into one shaping call.
			sText, adv, w, end := coalesceTextItems(items, k, e.adv[:0])
			x := pl.X + it.X
			st := it.St
			e.adv = adv
			e.drawText(ops, st, strings.TrimRight(sText, " "), x*s, (base-st.Shift)*s, s, adv)
			e.decorate(ops, st, x, base-st.Shift, w, s)
			k = end
		case ItemTab:
			if it.Leader != 0 && it.W > 0 {
				e.drawLeader(ops, it, pl.X+it.X, base, s)
			}
			if it.St.Underline {
				e.decorate(ops, it.St, pl.X+it.X, base, it.W, s)
			}
		case ItemObject:
			e.drawObject(ops, it, pl.X+it.X, base, s)
		}
	}
}

// decorate draws underline and strikethrough for text from x to x+w with
// its baseline at base (points).
func (e *Engine) decorate(ops *op.Ops, st *TextStyle, x, base, w float64, s float64) {
	if (!st.Underline && !st.Strike) || w <= 0 {
		return
	}
	t := max(1, int(math.Round(st.Size/16*s)))
	x0, x1 := int(math.Round(x*s)), int(math.Round((x+w)*s))
	if st.Underline {
		y := int(math.Round((base + st.Size*0.12) * s))
		gui.HLine(ops, x0, x1, y, t, st.Color)
	}
	if st.Strike {
		y := int(math.Round((base - st.Size*0.28) * s))
		gui.HLine(ops, x0, x1, y, t, st.Color)
	}
}

// coalesceTextItems merges adjacent ItemText runs starting at start that share
// a style and can be shaped as one run. It returns the merged text, advances,
// decoration width (excluding trailing space at line end), and last index.
func coalesceTextItems(items []Item, start int, advBuf []float32) (text string, adv []float32, w float64, end int) {
	it := &items[start]
	var b strings.Builder
	b.WriteString(it.Text)
	adv = append(advBuf, it.Adv...)
	end = start
	for end+1 < len(items) {
		n, prev := &items[end+1], &items[end]
		if n.Kind != ItemText || n.St != it.St || math.Abs(prev.X+prev.W-n.X) > 0.01 || prev.stretched {
			break
		}
		b.WriteString(n.Text)
		adv = append(adv, n.Adv...)
		end++
	}
	last := &items[end]
	w = last.X + last.W - it.X
	if end == len(items)-1 {
		w -= last.trail
	}
	return b.String(), adv, w, end
}

// leaderChar returns the single-character string used for a tab leader.
func leaderChar(leader byte) string {
	switch leader {
	case '.', '-', '_':
		return string(rune(leader))
	default:
		return "."
	}
}

// leaderRun returns the repeated leader string and its grid-aligned starting X
// for a tab of width w starting at x with single-glyph width cw.
func leaderRun(ch string, x, w, cw float64) (string, float64, bool) {
	if cw <= 0 {
		return "", 0, false
	}
	n := int((w - cw) / cw)
	if n <= 0 {
		return "", 0, false
	}
	// Align the dots to a grid so leaders line up between lines.
	return strings.Repeat(ch, n), math.Ceil((x+cw/2)/cw) * cw, true
}

// drawLeader draws tab it's leader (dots, dashes or underscores) across the
// tab at x on baseline base, at scale s.
func (e *Engine) drawLeader(ops *op.Ops, it *Item, x, base, s float64) {
	ch := leaderChar(it.Leader)
	if run, start, ok := leaderRun(ch, x, it.W, e.m.measure(it.St, ch).width); ok {
		e.drawText(ops, it.St, run, start*s, base*s, s, nil)
	}
}

// drawObject draws object item it at x on baseline base, at scale s: the
// picture if it decodes, else a framed placeholder box, else the item's text:
// plain for a field result or note mark, on a shaded background otherwise.
func (e *Engine) drawObject(ops *op.Ops, it *Item, x, base, s float64) {
	px := func(v float64) int { return int(math.Round(v * s)) }
	if isPicture(it) && e.drawPicture(ops, it, x, base, s) {
		return
	}
	if it.Obj != nil && it.Obj.Width > 0 && it.Obj.Height > 0 {
		r := image.Rect(px(x), px(base-it.Obj.Height), px(x+it.Obj.Width), px(base))
		gui.FillRect(ops, r, placeholderBG)
		gui.HLine(ops, r.Min.X, r.Max.X, r.Min.Y, 1, placeholderLine)
		gui.HLine(ops, r.Min.X, r.Max.X, r.Max.Y-1, 1, placeholderLine)
		gui.VLine(ops, r.Min.X, r.Min.Y, r.Max.Y, 1, placeholderLine)
		gui.VLine(ops, r.Max.X-1, r.Min.Y, r.Max.Y, 1, placeholderLine)
		if it.Text != "" {
			st := *it.St
			st.Color = placeholderText
			cl := clip.Rect(r).Push(ops)
			e.drawText(ops, &st, it.Text, float64(r.Min.X)+4*s, float64(r.Min.Y)+(st.Size+4)*s, s, nil)
			cl.Pop()
		}
		return
	}
	if it.Text == "" {
		return
	}
	st := *it.St
	if it.Obj != nil && it.Obj.Field == "" && it.Obj.Note == nil && !it.Obj.NoteMark {
		st.Color = placeholderText
		r := image.Rect(px(x), px(base-it.Asc), px(x+it.W), px(base+it.Desc))
		gui.FillRect(ops, r, placeholderBG)
	}
	e.drawText(ops, &st, it.Text, x*s, (base-st.Shift)*s, s, nil)
}

// drawText shapes and paints s with its left end at x and baseline at y
// (pixels). With adv (the measured advance of each rune, points) the
// glyphs are placed where layout measured them: Gio rounds advances to
// whole pixels at small sizes, which would otherwise make text drift
// against the caret and the next word.
func (e *Engine) drawText(ops *op.Ops, st *TextStyle, str string, x, y, scale float64, adv []float32) {
	if str == "" || !(st.Size > 0) {
		return
	}
	if sc := shapeScale(st.Size, scale); sc < scale {
		// Too big for Gio to shape (see word.LayoutMaxShapePx): shape it
		// smaller and scale the drawing up.
		k := scale / sc
		t := op.Affine(f32.AffineId().Scale(f32.Point{}, f32.Pt(float32(k), float32(k)))).Push(ops)
		defer t.Pop()
		x, y, scale = x/k, y/k, sc
	}
	sh := e.m.shaper
	sh.LayoutString(text.Parameters{
		Font:             st.Font,
		PxPerEm:          fixed.Int26_6(math.Round(st.Size * scale * 64)),
		MaxWidth:         word.LayoutUnwrappedWidth,
		MaxLines:         1,
		DisableSpaceTrim: true,
	}, str)
	gs := e.glyphs[:0]
	for {
		g, ok := sh.NextGlyph()
		if !ok {
			break
		}
		gs = append(gs, g)
	}
	e.glyphs = gs
	if len(gs) == 0 {
		return
	}
	if adv != nil {
		r := 0
		cum := 0.0
		var origStart fixed.Int26_6
		newStart := fixed.Int26_6(0)
		clusterOpen := false
		for i := range gs {
			g := &gs[i]
			if !clusterOpen {
				origStart, newStart = g.X, fixed.Int26_6(math.Round(cum*scale*64))
				clusterOpen = true
			}
			g.X = newStart + (g.X - origStart)
			if g.Flags&text.FlagClusterBreak != 0 {
				for k := 0; k < int(g.Runes) && r < len(adv); k++ {
					cum += float64(adv[r])
					r++
				}
				clusterOpen = false
			}
		}
	}
	gui.PaintGlyphs(ops, sh, gs, float32(x), float32(y), st.Color)
}
