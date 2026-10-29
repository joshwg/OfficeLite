package layout

import (
	"math"

	"officelite/word/model"
)

// Caret is where the caret goes: a page, and the top and height of a
// vertical bar at X, in page coordinates (points).
type Caret struct {
	Page    int
	X, Y, H float64
}

// line returns the placed line ref points at.
func (e *Engine) line(ref LineRef) *Placed { return &e.Pages[ref.Page].Lines[ref.Line] }

// lineFor finds the placed line holding the caret position p: the first
// line of the paragraph that ends after it, else the last one.
func (e *Engine) lineFor(p model.Pos) (*Placed, bool) {
	return e.lineForEOL(p, false)
}

// lineForEOL is lineFor with the caret's end-of-line affinity: with eol
// set, a position at a wrapped line's end is on that line, not at the
// start of the next one.
func (e *Engine) lineForEOL(p model.Pos, eol bool) (*Placed, bool) {
	if p.Para < 0 || p.Para >= len(e.paraLines) || len(e.paraLines[p.Para]) == 0 {
		return nil, false
	}
	refs := e.paraLines[p.Para]
	for _, ref := range refs {
		pl := e.line(ref)
		if p.Off < pl.Line.End || eol && p.Off == pl.Line.End && p.Off > pl.Line.Start {
			return pl, true
		}
	}
	return e.line(refs[len(refs)-1]), true
}

// xAt is the caret x (relative to pl.X) of offset off in a line.
func xAt(pl *Placed, off int) float64 {
	l := pl.Line
	if len(l.Items) == 0 || off < l.Items[0].Off {
		return l.X0
	}
	for i := range l.Items {
		it := &l.Items[i]
		if off >= it.Off && off < it.Off+it.N {
			x := it.X
			for _, a := range it.Adv[:off-it.Off] {
				x += float64(a)
			}
			return x
		}
	}
	last := &l.Items[len(l.Items)-1]
	if last.Kind == ItemBreak || last.Kind == ItemPageBreak {
		return last.X
	}
	return last.X + last.W
}

// CaretAt returns where to draw the caret for p.
func (e *Engine) CaretAt(p model.Pos) (Caret, bool) { return e.CaretAtEOL(p, false) }

// CaretAtEOL is CaretAt with end-of-line affinity (see LineBounds): with
// eol set, a caret at a wrapped line's end shows at the end of that line.
func (e *Engine) CaretAtEOL(p model.Pos, eol bool) (Caret, bool) {
	pl, ok := e.lineForEOL(p, eol)
	if !ok {
		return Caret{}, false
	}
	l := pl.Line
	top := pl.Y + l.Baseline - l.Asc
	return Caret{Page: e.pageOf(pl), X: pl.X + xAt(pl, p.Off), Y: max(top, pl.Y), H: min(l.Asc+l.Desc, l.Height)}, true
}

func (e *Engine) pageOf(pl *Placed) int {
	return e.order[pl.order].Page
}

// offAt returns the caret offset nearest to x (relative to pl.X) on a
// line (see offAtEOL), never its end on a wrapped line.
func offAt(pl *Placed, x float64) int {
	off, eol := offAtEOL(pl, x)
	if eol {
		off = max(off-1, pl.Line.Start)
	}
	return off
}

// endsInSpace reports whether a line ends in a space or a break, before
// which the caret stops at the line's end.
func endsInSpace(l *Line) bool {
	if l.HardBreak || l.PageBreak || len(l.Items) == 0 {
		return true
	}
	last := &l.Items[len(l.Items)-1]
	return last.spaces > 0 || last.Kind == ItemBreak || last.Kind == ItemPageBreak
}

// offAtEOL returns the caret offset nearest to x (relative to pl.X) on a
// line. A wrapped line's end is the next line's start, so the caret stays
// before a trailing space instead; a line wrapped after a hyphen or
// inside a long word has none, and its end is returned with eol set (the
// caret shows at the end of this line, see CaretAtEOL).
func offAtEOL(pl *Placed, x float64) (int, bool) {
	l := pl.Line
	best, bestD := l.Start, math.Inf(1)
	try := func(off int, at float64) {
		if d := math.Abs(at - x); d < bestD {
			best, bestD = off, d
		}
	}
	if len(l.Items) == 0 {
		return l.Start, false
	}
	for i := range l.Items {
		it := &l.Items[i]
		at := it.X
		for k := 0; k < it.N; k++ {
			try(it.Off+k, at)
			at += float64(it.Adv[k])
		}
		if i == len(l.Items)-1 && it.Kind != ItemBreak && it.Kind != ItemPageBreak {
			try(it.Off+it.N, at)
		}
	}
	if best >= l.End && !l.Last {
		if !endsInSpace(l) {
			return l.End, true
		}
		best = max(l.End-1, l.Start)
	}
	return best, false
}

// HitTest returns the caret position nearest to (x, y) on page pg (page
// coordinates). Lines in the column under x are preferred, so a click in
// a table cell stays in that cell.
func (e *Engine) HitTest(page int, x, y float64) model.Pos {
	if page < 0 || page >= len(e.Pages) || len(e.Pages[page].Lines) == 0 {
		return e.nearestPageLine(page)
	}
	pg := e.Pages[page]
	var best *Placed
	bestScore := math.Inf(1)
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		dx := 0.0
		if x < pl.X {
			dx = pl.X - x
		} else if x > pl.X+pl.W {
			dx = x - pl.X - pl.W
		}
		dy := 0.0
		if y < pl.Y {
			dy = pl.Y - y
		} else if y >= pl.Y+pl.Line.Height {
			dy = y - pl.Y - pl.Line.Height + 0.001
		}
		if s := dx*1000 + dy; s < bestScore {
			best, bestScore = pl, s
		}
	}
	return model.Pos{Para: best.Para, Off: offAt(best, x-best.X)}
}

func (e *Engine) nearestPageLine(page int) model.Pos {
	for p := min(page, len(e.Pages)-1); p >= 0; p-- {
		if ls := e.Pages[p].Lines; len(ls) > 0 {
			l := ls[len(ls)-1]
			return model.Pos{Para: l.Para, Off: l.Line.End}
		}
	}
	return model.Pos{}
}

// PageAt returns the page under view coordinate y (points from the top of
// the page view) and y relative to that page.
func (e *Engine) PageAt(y float64) (int, float64) {
	for i, pg := range e.Pages {
		if y < pg.Top+pg.Height+PageGap/2 || i == len(e.Pages)-1 {
			return i, y - pg.Top
		}
	}
	return 0, y
}

// PageNumberOf returns the number (as PAGE fields show it) of the page
// where body paragraph p starts, or 0.
func (e *Engine) PageNumberOf(p *model.Paragraph) int {
	i, ok := e.index[p]
	if !ok || i >= len(e.paraLines) || len(e.paraLines[i]) == 0 {
		return 0
	}
	return e.Pages[e.paraLines[i][0].Page].Number
}

// HitView hit-tests a point in view coordinates, x relative to the
// pages' left edge.
func (e *Engine) HitView(x, y float64) model.Pos {
	if len(e.Pages) == 0 {
		return model.Pos{}
	}
	p, py := e.PageAt(y)
	return e.HitTest(p, x-e.Pages[p].X, py)
}

// LineBounds returns the start and end caret positions of p's line (Home
// and End); eol is p's end-of-line affinity. On a wrapped line the end is
// before the trailing space, or, when the line wraps after a hyphen or
// inside a word, the line's end with endEOL set: the caret must then be
// drawn with CaretAtEOL to show at the end of this line.
func (e *Engine) LineBounds(p model.Pos, eol bool) (start, end model.Pos, endEOL bool) {
	pl, ok := e.lineForEOL(p, eol)
	if !ok {
		return p, p, false
	}
	l := pl.Line
	off := l.End
	if !l.Last {
		if !endsInSpace(l) {
			endEOL = true
		} else {
			off = max(l.End-1, l.Start)
		}
	}
	return model.Pos{Para: p.Para, Off: l.Start}, model.Pos{Para: p.Para, Off: off}, endEOL
}

// Vertical returns the position on the nearest line above (dir -1) or
// below (+1) p's line (eol is p's affinity) closest to goalX (page
// coordinates), preferring lines whose column holds goalX, so Down in a
// table cell goes to the cell below; toEOL is the result's affinity (see
// offAtEOL). ok is false at the document edge.
func (e *Engine) Vertical(p model.Pos, dir int, goalX float64, eol bool) (to model.Pos, toEOL, ok bool) {
	pl, ok := e.lineForEOL(p, eol)
	if !ok {
		return p, false, false
	}
	page := e.pageOf(pl)
	var best *Placed
	bestScore := math.Inf(1)
	for pg := page; pg >= 0 && pg < len(e.Pages) && best == nil; pg += dir {
		for i := range e.Pages[pg].Lines {
			t := &e.Pages[pg].Lines[i]
			// dy is how far the line is past p's line; lines on later
			// pages are all past those on this one.
			dy := 0.0
			switch {
			case pg != page && dir > 0:
				dy = t.Y
			case pg != page:
				dy = e.Pages[pg].Height - t.Y - t.Line.Height
			case dir > 0:
				dy = t.Y - (pl.Y + pl.Line.Height)
			default:
				dy = pl.Y - (t.Y + t.Line.Height)
			}
			if dy < -0.01 {
				continue // not wholly above (below) p's line
			}
			dx := 0.0
			if goalX < t.X {
				dx = t.X - goalX
			} else if goalX > t.X+t.W {
				dx = goalX - t.X - t.W
			}
			if s := dx*1000 + dy; s < bestScore {
				best, bestScore = t, s
			}
		}
	}
	if best == nil {
		return p, false, false
	}
	off, toEOL := offAtEOL(best, goalX-best.X)
	return model.Pos{Para: best.Para, Off: off}, toEOL, true
}

// SelectionRects returns the highlighted rectangles of the selection
// [a, b) on page pg.
func (e *Engine) SelectionRects(pg *Page, a, b model.Pos) []Rect {
	if a == b {
		return nil
	}
	var out []Rect
	cross := e.cellOf(a.Para) != e.cellOf(b.Para)
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		l := pl.Line
		ls, le := model.Pos{Para: pl.Para, Off: l.Start}, model.Pos{Para: pl.Para, Off: l.End}
		if cross && e.cellSelected(pl.Para, a, b) {
			// A selection across cells highlights whole cells.
			out = append(out, Rect{pl.X - cellMargin, pl.Y, pl.X + pl.W + cellMargin, pl.Y + l.Height})
			continue
		}
		if !ls.Less(b) || !(a.Less(le) || l.Last && a == le && b.Para > pl.Para) {
			continue
		}
		x0 := l.X0
		if a.Para == pl.Para && a.Off > l.Start {
			x0 = xAt(pl, a.Off)
		}
		x1 := xAt(pl, l.End)
		if b.Para == pl.Para && b.Off < l.End {
			x1 = xAt(pl, b.Off)
		}
		// The paragraph mark shows as a space-wide block when selected.
		if l.Last && b.Para > pl.Para {
			x1 += 4
		}
		if x1 > x0 {
			out = append(out, Rect{pl.X + x0, pl.Y, pl.X + x1, pl.Y + l.Height})
		}
	}
	return out
}

// ParaCount is the number of paragraphs laid out.
func (e *Engine) ParaCount() int { return len(e.locs) }

// PageOfPos returns the page holding p's caret.
func (e *Engine) PageOfPos(p model.Pos) int {
	if c, ok := e.CaretAt(p); ok {
		return c.Page
	}
	return 0
}

// cellOf returns the table cell paragraph i is in, or nil.
func (e *Engine) cellOf(i int) *model.TableCell {
	if i < 0 || i >= len(e.locs) {
		return nil
	}
	return e.locs[i].Cell
}

// cellSelected reports whether the selection [a, b) covers all of the
// cell paragraph i is in.
func (e *Engine) cellSelected(i int, a, b model.Pos) bool {
	c := e.cellOf(i)
	if c == nil {
		return false
	}
	first, last := i, i
	for first > 0 && e.locs[first-1].Cell == c {
		first--
	}
	for last+1 < len(e.locs) && e.locs[last+1].Cell == c {
		last++
	}
	return !(model.Pos{Para: first}).Less(a) && !b.Less(model.Pos{Para: last, Off: e.locs[last].P.Len()})
}
