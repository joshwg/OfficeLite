package layout

import (
	"image"
	"math"

	"gioui.org/op"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// Formatting marks (Word's Show/Hide ¶). The marks are only drawn: they
// take no room in line breaking, so a ¶ after a full line reaches into
// the margin, as in Word. Hidden text is the exception: with
// SetShowHidden it is laid out like other text (Word reflows it too) and
// drawn with a dotted underline.

// markColor is the color of the marks: a mid grey, subtler than the text
// (Word draws them in the text color on some versions and a light
// grey-blue on others).
var markColor = colors.RGB(0x8C8C8C)

// MarkKind is a kind of formatting mark.
type MarkKind int8

const (
	MarkPara      MarkKind = iota // ¶ at the end of a paragraph
	MarkCell                      // ¤ at the end of a table cell
	MarkRow                       // ¤ right of a table row
	MarkSpace                     // · centered in a space
	MarkNBSP                      // ° centered in a non-breaking space
	MarkTab                       // → in a tab's space
	MarkBreak                     // ↵ at a line break
	MarkPageBreak                 // "Page Break" across the rest of the line
	MarkHidden                    // dotted underline under hidden text
	MarkSection                   // "Section Break (Next Page)" across the rest of the line
)

// markGlyph is the character drawn for a mark.
var markGlyph = [...]string{
	MarkPara: "¶", MarkCell: "¤", MarkRow: "¤", MarkSpace: "·", MarkNBSP: "°",
	MarkTab: "→", MarkBreak: "↵",
}

// Mark is a formatting mark placed on a page. Spaces, tabs, page breaks
// and hidden text fill the span X..X+W; the others start at X (W = 0).
type Mark struct {
	Kind MarkKind
	X, W float64
	Base float64    // baseline
	St   *TextStyle // the text's style: the mark's font and size
	// Break is the kind of a MarkSection's break (the next section's).
	Break model.SectionBreak
}

// SetShowHidden lays out hidden text like other text (on) or as nothing
// (off, the default and what PDF export uses).
func (e *Engine) SetShowHidden(on bool) {
	if e.showHidden != on {
		e.showHidden = on
		e.all = true
	}
}

// ShowHidden reports whether hidden text is laid out.
func (e *Engine) ShowHidden() bool { return e.showHidden }

// Marks lists the formatting marks of placed lines and of the table rows
// among them (in page coordinates).
func (e *Engine) Marks(lines []Placed, rows []TableRowBox) []Mark {
	var out []Mark
	for i := range lines {
		out = lineMarks(out, &lines[i])
		if pl := &lines[i]; pl.Line.Last && pl.Box != nil && pl.Box.P.Sect != nil && len(out) > 0 && out[len(out)-1].Kind == MarkPara {
			// A section break's mark: Word's double dotted line with the
			// break's kind across the rest of the line.
			m := &out[len(out)-1]
			m.Kind, m.Break, m.W = MarkSection, e.breaks[pl.Box.P], max(pl.X+pl.W-m.X, 0)
		}
	}
	for _, r := range rows {
		if len(r.Edges) == 0 {
			continue
		}
		// The row's mark sits right of its last border, on the baseline
		// of the row's first line.
		var first *Placed
		for i := range lines {
			pl := &lines[i]
			if pl.Box != nil && pl.Box.table == r.Table && pl.Y >= r.Y0-0.01 && pl.Y < r.Y1 && (first == nil || pl.Y < first.Y) {
				first = pl
			}
		}
		if first == nil || first.Box.mark == nil {
			continue
		}
		out = append(out, Mark{Kind: MarkRow, X: r.Edges[len(r.Edges)-1].X + 2, Base: first.Y + first.Line.Baseline, St: first.Box.mark})
	}
	return out
}

// lineMarks appends a line's marks to out.
func lineMarks(out []Mark, pl *Placed) []Mark {
	if pl.collapsed {
		return out
	}
	l := pl.Line
	base := pl.Y + l.Baseline
	end := pl.X + l.X0
	for k := range l.Items {
		it := &l.Items[k]
		if it.St == nil || it.St.Hidden {
			continue
		}
		x := pl.X + it.X
		b := base - it.St.Shift
		end = x + it.W
		switch it.Kind {
		case ItemText:
			if it.St.Revealed && it.W > 0 {
				out = append(out, Mark{Kind: MarkHidden, X: x, W: it.W - it.trail, Base: b, St: it.St})
			}
			cx := x
			for i, r := range []rune(it.Text) {
				if i >= len(it.Adv) {
					break
				}
				w := float64(it.Adv[i])
				switch r {
				case ' ':
					out = append(out, Mark{Kind: MarkSpace, X: cx, W: w, Base: b, St: it.St})
				case ' ':
					out = append(out, Mark{Kind: MarkNBSP, X: cx, W: w, Base: b, St: it.St})
				}
				cx += w
			}
		case ItemTab:
			out = append(out, Mark{Kind: MarkTab, X: x, W: it.W, Base: b, St: it.St})
		case ItemBreak:
			out = append(out, Mark{Kind: MarkBreak, X: x, Base: b, St: it.St})
		case ItemPageBreak:
			out = append(out, Mark{Kind: MarkPageBreak, X: x, W: max(pl.X+pl.W-x, 0), Base: b, St: it.St})
		}
	}
	if l.Last && pl.Box != nil && pl.Box.mark != nil {
		k := MarkPara
		if t := pl.Box.table; t != nil && endsCell(t, pl.Box.P) {
			k = MarkCell
		}
		out = append(out, Mark{Kind: k, X: end, Base: base, St: pl.Box.mark})
	}
	return out
}

// endsCell reports whether p is the last block of one of t's cells.
func endsCell(t *model.Table, p *model.Paragraph) bool {
	for _, r := range t.Rows {
		for _, c := range r.Cells {
			if n := len(c.Blocks); n > 0 && c.Blocks[n-1] == model.Block(p) {
				return true
			}
		}
	}
	return false
}

// drawMarks draws marks whose baseline is within y0..y1 (points; both 0:
// all) at scale s.
func (e *Engine) drawMarks(ops *op.Ops, marks []Mark, y0, y1, s float64) {
	px := func(v float64) int { return int(math.Round(v * s)) }
	for i := range marks {
		m := &marks[i]
		if (y0 != 0 || y1 != 0) && (m.Base < y0 || m.Base-m.St.Size > y1) {
			continue
		}
		st := *m.St
		st.Color, st.Underline, st.Strike = markColor, false, false
		switch m.Kind {
		case MarkHidden:
			// Dots on a pixel grid so neighboring pieces join up.
			d := max(1, px(0.6))
			y := px(m.Base + m.St.Size*0.14)
			for x := (px(m.X)/(2*d) + 1) * 2 * d; x+d <= px(m.X+m.W); x += 2 * d {
				gui.FillRect(ops, image.Rect(x, y, x+d, y+d), m.St.Color)
			}
		case MarkPageBreak, MarkSection:
			e.drawPageBreak(ops, m, &st, s)
		default:
			g := markGlyph[m.Kind]
			x := m.X
			if m.W > 0 {
				x += (m.W - e.m.measure(&st, g).width) / 2
				x = max(x, m.X)
			}
			e.drawText(ops, &st, g, x*s, m.Base*s, s, nil)
		}
	}
}

// drawPageBreak draws Word's "·····Page Break·····" centered in the
// rest of the line.
func (e *Engine) drawPageBreak(ops *op.Ops, m *Mark, st *TextStyle, s float64) {
	px := func(v float64) int { return int(math.Round(v * s)) }
	label := "Page Break"
	if m.Kind == MarkSection {
		label = "Section Break (" + model.BreakLabels[m.Break] + ")"
	}
	st.Size = min(st.Size, 9)
	lw := e.m.measure(st, label).width
	lx := m.X + (m.W-lw)/2
	e.drawText(ops, st, label, lx*s, m.Base*s, s, nil)
	d := max(1, px(0.75))
	y := px(m.Base - st.Size*0.3)
	gap := 4.0
	dots := func(a, b float64) {
		for x := px(a); x+d <= px(b); x += 3 * d {
			gui.FillRect(ops, image.Rect(x, y, x+d, y+d), markColor)
			if m.Kind == MarkSection { // a double line
				gui.FillRect(ops, image.Rect(x, y+2*d, x+d, y+3*d), markColor)
			}
		}
	}
	dots(m.X, lx-gap)
	dots(lx+lw+gap, m.X+m.W)
}
