package layout

import (
	"math"
	"strings"

	"gioui.org/font"

	"officelite/common/pdf"
	"officelite/word/model"
)

// ExportPDF adds the laid-out pages to d, as the page view shows them
// (without selection or editing marks): page size and margins, text with
// its fonts, colors, highlighting, underline and strikethrough at the
// positions layout measured, list labels, tab leaders, paragraph and cell
// shading, table borders, pictures (placeholders for those that can't be
// decoded) and the headers and footers. Call Layout first.
func (e *Engine) ExportPDF(d *pdf.Document) {
	if e.showHidden {
		// The PDF shows the document as printed: without hidden text.
		e.SetShowHidden(false)
		e.Layout()
		defer func() {
			e.SetShowHidden(true)
			e.Layout()
		}()
	}
	x := &pdfExport{e: e, d: d, fonts: map[*TextStyle]*pdf.Font{}, pics: map[*model.Picture]*pdf.Image{}}
	for _, pg := range e.Pages {
		p := d.AddPage(pdf.Size{W: pg.Width, H: pg.Height})
		x.p = p
		x.floats(pg, true)
		x.body(pg.Lines, pg.Fills, pg.Rules)
		for _, b := range append([]*Band{pg.Header, pg.Footer}, pg.Notes...) {
			if b != nil {
				x.body(b.Lines, b.Fills, b.Rules)
			}
		}
		x.floats(pg, false)
	}
}

// floats draws the page's floating objects behind the text (behind set)
// or in front of it.
func (x *pdfExport) floats(pg *Page, behind bool) {
	p := x.p
	for _, b := range pg.Floats {
		f := b.F
		if (f.Wrap == model.WrapBehind) != behind {
			continue
		}
		outline, closed := shapeOutline(f, b.R)
		pts := make([]pdf.Point, len(outline))
		for i, q := range outline {
			pts[i] = pdf.Point{X: q.X, Y: q.Y}
		}
		if c, ok := floatColor(f.Fill); ok && closed {
			p.FillPolygon(pts, c)
		}
		if f.Picture != nil {
			im, ok := x.pics[f.Picture]
			if !ok {
				im, _ = x.d.AddImageData(f.Picture.Data)
				x.pics[f.Picture] = im
			}
			if im != nil {
				p.DrawImage(im, b.R.X0, b.R.Y0, b.R.X1-b.R.X0, b.R.Y1-b.R.Y0)
			}
		}
		x.body(b.Lines, b.Fills, b.Rules)
		if c, ok := floatColor(f.Line); ok {
			p.StrokePolyline(pts, closed, max(f.LineWidth, 0.25), c)
		}
	}
}

type pdfExport struct {
	e     *Engine
	d     *pdf.Document
	p     *pdf.Page
	fonts map[*TextStyle]*pdf.Font
	pics  map[*model.Picture]*pdf.Image
}

func (x *pdfExport) font(st *TextStyle) *pdf.Font {
	if f, ok := x.fonts[st]; ok {
		return f
	}
	fam := st.Family
	if fam == "" {
		fam = strings.TrimSpace(strings.Split(string(st.Font.Typeface), ",")[0])
	}
	f := x.d.Font(pdf.FontSpec{Family: fam, Bold: st.Font.Weight >= font.SemiBold, Italic: st.Font.Style == font.Italic})
	x.fonts[st] = f
	return f
}

// body draws placed lines with their shading and rules, in the page
// view's order: shading, highlighting, text, borders.
func (x *pdfExport) body(lines []Placed, fills []Fill, rules []Rect) {
	p := x.p
	for _, f := range fills {
		p.FillRect(f.R.X0, f.R.Y0, f.R.X1-f.R.X0, f.R.Y1-f.R.Y0, f.C)
	}
	for i := range lines {
		pl := &lines[i]
		for k := range pl.Line.Items {
			it := &pl.Line.Items[k]
			if it.St != nil && it.St.Highlight.A > 0 && !it.St.Hidden && it.W > 0 {
				w := it.W
				if k == len(pl.Line.Items)-1 {
					w -= it.trail
				}
				base := pl.Y + pl.Line.Baseline
				top := base - pl.Line.Asc
				p.FillRect(pl.X+it.X, top, w, pl.Line.Asc+pl.Line.Desc, it.St.Highlight)
			}
		}
	}
	for i := range lines {
		x.line(&lines[i])
	}
	for _, r := range rules {
		w, h := r.X1-r.X0, r.Y1-r.Y0
		// Rules are hairline-thin rectangles; give them a printable width.
		if w < h {
			w = max(w, 0.5)
		} else {
			h = max(h, 0.5)
		}
		p.FillRect(r.X0, r.Y0, w, h, ruleColor)
	}
}

func (x *pdfExport) line(pl *Placed) {
	l := pl.Line
	base := pl.Y + l.Baseline
	if lb := l.Label; lb != nil && lb.St != nil && !lb.St.Hidden {
		x.text(lb.St, lb.Text, pl.X+lb.X, base, nil)
	}
	items := l.Items
	for k := 0; k < len(items); k++ {
		it := &items[k]
		if it.St == nil || it.St.Hidden {
			continue
		}
		switch it.Kind {
		case ItemText:
			// Neighboring words of one style become one run, as on screen.
			var b strings.Builder
			b.WriteString(it.Text)
			adv := append([]float32(nil), it.Adv...)
			end := k
			for end+1 < len(items) {
				n, prev := &items[end+1], &items[end]
				if n.Kind != ItemText || n.St != it.St || math.Abs(prev.X+prev.W-n.X) > 0.01 || prev.stretched {
					break
				}
				b.WriteString(n.Text)
				adv = append(adv, n.Adv...)
				end++
			}
			st := it.St
			s := b.String()
			if end == len(items)-1 {
				s = strings.TrimRight(s, " ")
			}
			x.text(st, s, pl.X+it.X, base-st.Shift, adv)
			last := &items[end]
			w := last.X + last.W - it.X
			if end == len(items)-1 {
				w -= last.trail
			}
			x.decorate(st, pl.X+it.X, base-st.Shift, w)
			k = end
		case ItemTab:
			if it.Leader != 0 && it.W > 0 {
				x.leader(it, pl.X+it.X, base)
			}
			if it.St.Underline {
				x.decorate(it.St, pl.X+it.X, base, it.W)
			}
		case ItemObject:
			x.object(it, pl, base)
		}
	}
}

func (x *pdfExport) text(st *TextStyle, s string, xpos, base float64, adv []float32) {
	if s == "" || st.Size <= 0 {
		return
	}
	x.p.DrawText(pdf.Run{Font: x.font(st), Size: st.Size, X: xpos, Y: base, Text: s, Color: st.Color, Adv: adv})
}

// decorate draws underline and strikethrough as the page view does.
func (x *pdfExport) decorate(st *TextStyle, xpos, base, w float64) {
	if (!st.Underline && !st.Strike) || w <= 0 {
		return
	}
	t := max(st.Size/16, 0.5)
	if st.Underline {
		y := base + st.Size*0.12
		x.p.FillRect(xpos, y-t/2, w, t, st.Color)
	}
	if st.Strike {
		y := base - st.Size*0.28
		x.p.FillRect(xpos, y-t/2, w, t, st.Color)
	}
}

func (x *pdfExport) leader(it *Item, xpos, base float64) {
	ch := string(rune(it.Leader))
	switch it.Leader {
	case '.', '-', '_':
	default:
		ch = "."
	}
	f := x.font(it.St)
	cw := f.Measure(ch, it.St.Size)
	if cw <= 0 {
		return
	}
	n := int((it.W - cw) / cw)
	if n <= 0 {
		return
	}
	// Align the dots to a grid so leaders line up between lines.
	start := math.Ceil((xpos+cw/2)/cw) * cw
	x.text(it.St, strings.Repeat(ch, n), start, base, nil)
}

// object draws a picture, or the placeholder the page view shows for
// objects it can't draw.
func (x *pdfExport) object(it *Item, pl *Placed, base float64) {
	p := x.p
	xpos := pl.X + it.X
	if o := it.Obj; o != nil && o.Picture != nil && o.Width > 0 && o.Height > 0 {
		im, ok := x.pics[o.Picture]
		if !ok {
			im, _ = x.d.AddImageData(o.Picture.Data)
			x.pics[o.Picture] = im
		}
		if im != nil {
			r := PictureBox(pl, it)
			p.DrawImage(im, r.X0, r.Y0, r.X1-r.X0, r.Y1-r.Y0)
			return
		}
	}
	if it.Obj != nil && it.Obj.Width > 0 && it.Obj.Height > 0 {
		w, h := it.Obj.Width, it.Obj.Height
		top := base - h
		p.FillRect(xpos, top, w, h, placeholderBG)
		p.StrokeRect(xpos+0.375, top+0.375, w-0.75, h-0.75, 0.75, placeholderLine)
		if it.Text != "" {
			st := *it.St
			st.Color = placeholderText
			p.Save()
			p.ClipRect(xpos, top, w, h)
			x.text(&st, it.Text, xpos+4, top+st.Size+4, nil)
			p.Restore()
		}
		return
	}
	if it.Text == "" {
		return
	}
	st := *it.St
	if it.Obj != nil && it.Obj.Field == "" && it.Obj.Note == nil && !it.Obj.NoteMark {
		st.Color = placeholderText
		p.FillRect(xpos, base-it.Asc, it.W, it.Asc+it.Desc, placeholderBG)
	}
	x.text(&st, it.Text, xpos, base-st.Shift, nil)
}
