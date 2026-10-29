package layout

import (
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"officelite/common/colors"
	"officelite/word/model"
)

// Floating objects (model.Floating): text boxes, shapes and pictures
// anchored in a paragraph and positioned on the page. Each is placed
// when the line holding its anchor is, from the references the object
// names (page, margins, column, paragraph, line); text then stays clear
// of objects that wrap it top-and-bottom or square (WordLite doesn't
// flow text beside an object: square and tight wrapping keep the text
// above and below, too). A text box's text is laid out inside its box.

// FloatBox is a floating object placed on a page: its box and, for a
// text box, its text (lines, fills and rules in page coordinates).
type FloatBox struct {
	F     *model.Floating
	R     Rect
	Lines []Placed
	Fills []Fill
	Rules []Rect
}

// addFloat places a floating object anchored on line pl (at x itemX of
// the line) on the page being filled.
func (e *Engine) addFloat(f *model.Floating, pl *Placed, itemX float64) {
	pg := e.page
	for _, b := range pg.Floats {
		if b.F == f {
			return
		}
	}
	setup := e.setup
	ml, mt := math.Abs(setup.MarginLeft), math.Abs(setup.MarginTop)
	var x0, w float64
	switch f.RelX {
	case model.RelPage:
		x0, w = 0, pg.Width
	case model.RelMargin:
		x0, w = ml, e.areaWidth()
	case model.RelCharacter:
		x0, w = pl.X+itemX, 0
	default: // column, paragraph
		x0, w = pl.X-pl.Box.Left, pl.W
		if pl.Box.table == nil {
			x0, w = e.textLeft(), e.textWidth()
		}
	}
	x := x0 + f.X
	switch f.AlignX {
	case "left", "inside":
		x = x0
	case "center":
		x = x0 + (w-f.W)/2
	case "right", "outside":
		x = x0 + w - f.W
	}
	var y0, h float64
	switch f.RelY {
	case model.RelPage:
		y0, h = 0, pg.Height
	case model.RelMargin:
		y0, h = mt, e.areaBottom()-mt
	case model.RelLine:
		y0, h = pl.Y, pl.Line.Height
	default: // paragraph
		y0, h = e.paraTop, 0
	}
	y := y0 + f.Y
	switch f.AlignY {
	case "top":
		y = y0
	case "center":
		y = y0 + (h-f.H)/2
	case "bottom":
		y = y0 + h - f.H
	}
	b := &FloatBox{F: f, R: Rect{x, y, x + f.W, y + f.H}}
	if f.IsTextBox() {
		in := f.Inset
		fl := e.layoutFlow(f.Blocks, max(f.W-in[0]-in[2], 6), nil)
		for _, l := range fl.lines {
			l.X += x + in[0]
			l.Y += y + in[1]
			b.Lines = append(b.Lines, l)
		}
		for _, fi := range fl.fills {
			fi.R = Rect{fi.R.X0 + x + in[0], fi.R.Y0 + y + in[1], fi.R.X1 + x + in[0], fi.R.Y1 + y + in[1]}
			b.Fills = append(b.Fills, fi)
		}
		for _, r := range fl.rules {
			b.Rules = append(b.Rules, Rect{r.X0 + x + in[0], r.Y0 + y + in[1], r.X1 + x + in[0], r.Y1 + y + in[1]})
		}
	}
	pg.Floats = append(pg.Floats, b)
}

// wraps reports whether text keeps clear of an object.
func wraps(f *model.Floating) bool {
	switch f.Wrap {
	case model.WrapSquare, model.WrapTight, model.WrapTopBottom:
		return true
	}
	return false
}

// clearFloats returns where a line h high at y goes so it doesn't run
// into an object wrapped top-and-bottom on the page: below the object.
func (e *Engine) clearFloats(y, h float64) float64 {
	if e.page == nil {
		return y
	}
	x0, x1 := e.textLeft(), e.textLeft()+e.textWidth()
	for moved := true; moved; {
		moved = false
		for _, b := range e.page.Floats {
			r := b.R
			d := b.F.Distance
			if !wraps(b.F) || r.X1+d[2] <= x0 || r.X0-d[0] >= x1 {
				continue
			}
			if y+h > r.Y0-d[1] && y < r.Y1+d[3] {
				y, moved = r.Y1+d[3], true
			}
		}
	}
	return y
}

// pt is a point in page coordinates.
type pt struct{ X, Y float64 }

// shapeOutline returns the outline of a floating object's shape in its
// box, and whether it is closed (a line or an arc isn't).
func shapeOutline(f *model.Floating, r Rect) ([]pt, bool) {
	w, h := r.X1-r.X0, r.Y1-r.Y0
	at := func(u, v float64) pt {
		if f.FlipH {
			u = 1 - u
		}
		if f.FlipV {
			v = 1 - v
		}
		return pt{r.X0 + u*w, r.Y0 + v*h}
	}
	switch f.Shape {
	case "line", "straightConnector1", "connector":
		return []pt{at(0, 0), at(1, 1)}, false
	case "ellipse", "oval":
		var out []pt
		for i := range 64 {
			a := float64(i) / 64 * 2 * math.Pi
			out = append(out, pt{r.X0 + w/2 + w/2*math.Cos(a), r.Y0 + h/2 + h/2*math.Sin(a)})
		}
		return out, true
	case "roundRect":
		rad := min(w, h) * 0.1667
		var out []pt
		corner := func(cx, cy, a0 float64) {
			for i := 0; i <= 8; i++ {
				a := a0 + float64(i)/8*math.Pi/2
				out = append(out, pt{cx + rad*math.Cos(a), cy + rad*math.Sin(a)})
			}
		}
		corner(r.X1-rad, r.Y0+rad, -math.Pi/2)
		corner(r.X1-rad, r.Y1-rad, 0)
		corner(r.X0+rad, r.Y1-rad, math.Pi/2)
		corner(r.X0+rad, r.Y0+rad, math.Pi)
		return out, true
	case "rightArrow", "leftArrow":
		pts := []pt{at(0, 0.25), at(0.7, 0.25), at(0.7, 0), at(1, 0.5), at(0.7, 1), at(0.7, 0.75), at(0, 0.75)}
		if f.Shape == "leftArrow" {
			for i := range pts {
				pts[i].X = r.X0 + r.X1 - pts[i].X
			}
		}
		return pts, true
	case "upArrow", "downArrow":
		pts := []pt{at(0.25, 1), at(0.25, 0.3), at(0, 0.3), at(0.5, 0), at(1, 0.3), at(0.75, 0.3), at(0.75, 1)}
		if f.Shape == "downArrow" {
			for i := range pts {
				pts[i].Y = r.Y0 + r.Y1 - pts[i].Y
			}
		}
		return pts, true
	case "triangle":
		return []pt{at(0.5, 0), at(1, 1), at(0, 1)}, true
	case "diamond":
		return []pt{at(0.5, 0), at(1, 0.5), at(0.5, 1), at(0, 0.5)}, true
	}
	return []pt{{r.X0, r.Y0}, {r.X1, r.Y0}, {r.X1, r.Y1}, {r.X0, r.Y1}}, true
}

// floatColor parses an object's fill or line color; ok is false for none.
func floatColor(hex string) (color.NRGBA, bool) {
	if hex == "" {
		return color.NRGBA{}, false
	}
	return colors.ParseHex(hex)
}

// drawFloats draws the page's floating objects behind the text (behind
// set) or in front of it.
func (e *Engine) drawFloats(ops *op.Ops, pg *Page, behind bool, s float64, marks bool) {
	for _, b := range pg.Floats {
		if (b.F.Wrap == model.WrapBehind) != behind {
			continue
		}
		e.drawFloat(ops, b, s)
		if marks {
			e.drawMarks(ops, e.Marks(b.Lines, nil), 0, 0, s)
		}
	}
}

func (e *Engine) drawFloat(ops *op.Ops, b *FloatBox, s float64) {
	f := b.F
	pts, closed := shapeOutline(f, b.R)
	path := func() clip.PathSpec {
		var p clip.Path
		p.Begin(ops)
		for i, q := range pts {
			v := f32.Pt(float32(q.X*s), float32(q.Y*s))
			if i == 0 {
				p.MoveTo(v)
			} else {
				p.LineTo(v)
			}
		}
		if closed {
			p.Close()
		}
		return p.End()
	}
	if c, ok := floatColor(f.Fill); ok && closed {
		paint.FillShape(ops, c, clip.Outline{Path: path()}.Op())
	}
	if f.Picture != nil {
		r := b.R
		it := Item{Kind: ItemObject, Obj: &model.Opaque{Picture: f.Picture, Width: r.X1 - r.X0, Height: r.Y1 - r.Y0}}
		if !e.drawPicture(ops, &it, r.X0, r.Y1, s) {
			e.drawObject(ops, &Item{Kind: ItemObject, Obj: it.Obj, St: e.style(e.Doc.ResolveRun(&model.Paragraph{}, &model.Run{})), Text: "[picture]"}, r.X0, r.Y1, s)
		}
	}
	for _, fl := range b.Fills {
		fillRect(ops, fl.R, fl.C, s)
	}
	for i := range b.Lines {
		e.drawLine(ops, &b.Lines[i], s)
	}
	for _, r := range b.Rules {
		fillRect(ops, Rect{r.X0, r.Y0, max(r.X1, r.X0+0.75), max(r.Y1, r.Y0+0.75)}, ruleColor, s)
	}
	if c, ok := floatColor(f.Line); ok && f.LineWidth >= 0 {
		lw := max(f.LineWidth, 0.25)
		paint.FillShape(ops, c, clip.Stroke{Path: path(), Width: float32(max(lw*s, 1))}.Op())
	}
}

func fillRect(ops *op.Ops, r Rect, c color.NRGBA, s float64) {
	rr := clip.Rect{Min: f32Pt(r.X0, r.Y0, s).Round(), Max: f32Pt(r.X1, r.Y1, s).Round()}
	paint.FillShape(ops, c, rr.Op())
}

func f32Pt(x, y, s float64) f32.Point { return f32.Pt(float32(x*s), float32(y*s)) }
