package layout

import (
	"officelite/word/model"
)

// Footnotes and endnotes. A footnote goes at the bottom of the text area
// of the page whose line references it, under a separator line; the body
// text ends higher to make room (a line whose footnotes don't fit moves
// to the next page with them). Endnotes follow the last section's text,
// under a separator. The notes' lines are kept in bands (Page.Notes) like
// headers' (their Placed.Para doesn't index the body).

// separatorWidth is the length of Word's note separator line.
const separatorWidth = 144

type noteState struct {
	foot  []*model.Note // footnotes on the page being filled
	footH float64       // the height of their area (0 without)
	sep   *model.Paragraph
	end   *Band // the endnote band of the page being filled
}

// prepareNotes numbers the notes again (paragraphs whose reference marks
// changed are laid out again).
func (e *Engine) prepareNotes() {
	for _, p := range e.Doc.NumberNotes() {
		delete(e.cache, p)
	}
	e.notes = noteState{sep: e.notes.sep}
	if e.notes.sep == nil {
		e.notes.sep = &model.Paragraph{Props: model.ParaProps{SpaceBefore: model.F(0), SpaceAfter: model.F(0), Line: 1, LineRule: model.LineAuto}}
	}
}

// sepHeight is the height of a separator: one empty line.
func (e *Engine) sepHeight() float64 {
	pl := e.para(e.notes.sep, e.areaWidth(), nil)
	return pl.Height
}

// noteFlow lays out a note's text across the page's text area.
func (e *Engine) noteFlow(n *model.Note, width float64) *flow {
	blocks := n.Blocks
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	}
	return e.layoutFlow(blocks, width, nil)
}

// footNeed is the room the footnotes referenced on line l need beyond
// what the page's footnotes take already.
func (e *Engine) footNeed(l *Line) float64 {
	need := 0.0
	for k := range l.Items {
		it := &l.Items[k]
		if it.Kind != ItemObject || it.Obj == nil || it.Obj.Note == nil || it.Obj.Note.Endnote || e.onPage(it.Obj.Note) {
			continue
		}
		if len(e.notes.foot) == 0 && need == 0 {
			need += e.sepHeight()
		}
		need += e.noteFlow(it.Obj.Note, e.areaWidth()).height
	}
	return need
}

func (e *Engine) onPage(n *model.Note) bool {
	for _, f := range e.notes.foot {
		if f == n {
			return true
		}
	}
	return false
}

// addFootnote puts a footnote on the page being filled.
func (e *Engine) addFootnote(n *model.Note) {
	if e.onPage(n) {
		return
	}
	if len(e.notes.foot) == 0 {
		e.notes.footH += e.sepHeight()
	}
	e.notes.foot = append(e.notes.foot, n)
	e.notes.footH += e.noteFlow(n, e.areaWidth()).height
}

// finishPage puts the footnotes of the page being filled at the bottom
// of its text area.
func (e *Engine) finishPage() {
	pg := e.page
	if pg == nil || len(e.notes.foot) == 0 {
		e.notes.foot, e.notes.footH, e.notes.end = nil, 0, nil
		return
	}
	left, w := e.marginLeft(), e.areaWidth()
	y := e.areaBottom() - e.notes.footH
	b := &Band{Top: y}
	e.separator(b, left, y)
	y += e.sepHeight()
	for _, n := range e.notes.foot {
		f := e.noteFlow(n, w)
		addFlow(b, f, left, y)
		y += f.height
	}
	b.Bottom = y
	pg.Notes = append(pg.Notes, b)
	e.notes.foot, e.notes.footH, e.notes.end = nil, 0, nil
}

// marginLeft is the left edge of the page's text area.
func (e *Engine) marginLeft() float64 {
	if e.setup.MarginLeft < 0 {
		return -e.setup.MarginLeft
	}
	return e.setup.MarginLeft
}

// separator adds Word's note separator, a short line in the middle of an
// empty line, at y.
func (e *Engine) separator(b *Band, x, y float64) {
	h := e.sepHeight()
	mid := y + h/2
	b.Rules = append(b.Rules, Rect{x, mid, x + separatorWidth, mid + 0.5})
	sep := e.para(e.notes.sep, e.areaWidth(), nil)
	b.Lines = append(b.Lines, Placed{Line: sep.Lines[0], Box: sep, X: x, Y: y, W: e.areaWidth()})
}

// addFlow adds a flow's content to a band with its top-left corner at
// (x, y).
func addFlow(b *Band, f *flow, x, y float64) {
	for _, pl := range f.lines {
		pl.X += x
		pl.Y += y
		b.Lines = append(b.Lines, pl)
	}
	for _, fl := range f.fills {
		fl.R = Rect{fl.R.X0 + x, fl.R.Y0 + y, fl.R.X1 + x, fl.R.Y1 + y}
		b.Fills = append(b.Fills, fl)
	}
	for _, r := range f.rules {
		b.Rules = append(b.Rules, Rect{r.X0 + x, r.Y0 + y, r.X1 + x, r.Y1 + y})
	}
}

// placeEndnotes places the endnotes after the last section's text: the
// separator, then each note, continuing on new pages as needed.
func (e *Engine) placeEndnotes() {
	var notes []*model.Note
	for _, n := range model.NotesIn(e.Doc.Body) {
		if n.Endnote {
			notes = append(notes, n)
		}
	}
	if len(notes) == 0 {
		return
	}
	band := func() *Band {
		if b := e.notes.end; b != nil && e.lastNotePage() == e.page {
			return b
		}
		b := &Band{Top: e.y}
		e.page.Notes = append(e.page.Notes, b)
		e.notes.end = b
		return b
	}
	if h := e.sepHeight(); e.y+h > e.textBottom() && !e.empty {
		e.nextColumn()
	}
	b := band()
	e.separator(b, e.textLeft(), e.y)
	e.y += e.sepHeight()
	e.empty = false
	for _, n := range notes {
		f := e.noteFlow(n, e.textWidth())
		start := 0.0 // the flow's y placed at the top of the current piece
		base := e.y
		for _, pl := range f.lines {
			if !e.empty && base+pl.Y-start+pl.Line.Height > e.textBottom()+0.01 {
				e.nextColumn()
				start, base = pl.Y, e.y
			}
			b = band()
			pl.X += e.textLeft()
			pl.Y += base - start
			b.Lines = append(b.Lines, pl)
			e.y = pl.Y + pl.Line.Height
			e.empty = false
		}
		for _, fl := range f.fills {
			fl.R = Rect{fl.R.X0 + e.textLeft(), fl.R.Y0 + base - start, fl.R.X1 + e.textLeft(), fl.R.Y1 + base - start}
			b.Fills = append(b.Fills, fl)
		}
		for _, r := range f.rules {
			b.Rules = append(b.Rules, Rect{r.X0 + e.textLeft(), r.Y0 + base - start, r.X1 + e.textLeft(), r.Y1 + base - start})
		}
		e.y = base + f.height - start
	}
	for _, pg := range e.Pages {
		for _, b := range pg.Notes {
			if b.Bottom == 0 {
				for _, pl := range b.Lines {
					b.Bottom = max(b.Bottom, pl.Y+pl.Line.Height)
				}
			}
		}
	}
}

// lastNotePage is the page holding the endnote band being filled.
func (e *Engine) lastNotePage() *Page {
	for _, b := range e.page.Notes {
		if b == e.notes.end {
			return e.page
		}
	}
	return nil
}
