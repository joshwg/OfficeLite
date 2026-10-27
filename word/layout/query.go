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
	if p.Para < 0 || p.Para >= len(e.paraLines) || len(e.paraLines[p.Para]) == 0 {
		return nil, false
	}
	refs := e.paraLines[p.Para]
	for _, ref := range refs {
		if pl := e.line(ref); p.Off < pl.Line.End {
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
func (e *Engine) CaretAt(p model.Pos) (Caret, bool) {
	pl, ok := e.lineFor(p)
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

// offAt returns the caret offset nearest to x (relative to pl.X) on a line.
func offAt(pl *Placed, x float64) int {
	l := pl.Line
	best, bestD := l.Start, math.Inf(1)
	try := func(off int, at float64) {
		if d := math.Abs(at - x); d < bestD {
			best, bestD = off, d
		}
	}
	if len(l.Items) == 0 {
		return l.Start
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
	// A caret after the last character of a wrapped line would show at the
	// start of the next line; stay before the trailing space instead.
	if best >= l.End && !l.Last {
		best = max(l.End-1, l.Start)
	}
	return best
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

// HitView hit-tests a point in view coordinates, x relative to the
// pages' left edge.
func (e *Engine) HitView(x, y float64) model.Pos {
	if len(e.Pages) == 0 {
		return model.Pos{}
	}
	p, py := e.PageAt(y)
	return e.HitTest(p, x, py)
}

// LineBounds returns the start and end caret positions of p's line (Home
// and End).
func (e *Engine) LineBounds(p model.Pos) (model.Pos, model.Pos) {
	pl, ok := e.lineFor(p)
	if !ok {
		return p, p
	}
	l := pl.Line
	end := l.End
	if !l.Last {
		end = max(l.End-1, l.Start)
	}
	return model.Pos{Para: p.Para, Off: l.Start}, model.Pos{Para: p.Para, Off: end}
}

// Vertical returns the position on the line dir lines above (-1) or below
// (+1) p nearest to goalX (page coordinates), and false at the document
// edge.
func (e *Engine) Vertical(p model.Pos, dir int, goalX float64) (model.Pos, bool) {
	pl, ok := e.lineFor(p)
	if !ok {
		return p, false
	}
	i := pl.order + dir
	if i < 0 || i >= len(e.order) {
		return p, false
	}
	t := e.line(e.order[i])
	return model.Pos{Para: t.Para, Off: offAt(t, goalX-t.X)}, true
}

// SelectionRects returns the highlighted rectangles of the selection
// [a, b) on page pg.
func (e *Engine) SelectionRects(pg *Page, a, b model.Pos) []Rect {
	if a == b {
		return nil
	}
	var out []Rect
	for i := range pg.Lines {
		pl := &pg.Lines[i]
		l := pl.Line
		ls, le := model.Pos{Para: pl.Para, Off: l.Start}, model.Pos{Para: pl.Para, Off: l.End}
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
