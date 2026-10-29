package layout

import (
	"math"

	"officelite/word/model"
)

// Queries for the page view's mouse features: selecting lines from the
// left margin and resizing tables by their borders.

// TextLeft is the left margin of the page setup the layout ended with, page
// points (not a column's left edge: the layout finishes in some column).
func (e *Engine) TextLeft() float64 { return math.Abs(e.setup.MarginLeft) }

// PageTextLeft is the left margin of page pg, which its section may set
// differently from the last one's.
func (e *Engine) PageTextLeft(pg *Page) float64 { return math.Abs(e.pageSetup(pg).MarginLeft) }

// LineAtY returns the start and end of the line at page y on page pg (the
// leftmost one when several share the height, as table cells do), or the
// nearest line above or below. ok is false on a page without lines.
func (e *Engine) LineAtY(page int, y float64) (a, b model.Pos, ok bool) {
	if page < 0 || page >= len(e.Pages) || len(e.Pages[page].Lines) == 0 {
		return a, b, false
	}
	var best *Placed
	bestD := math.Inf(1)
	for i := range e.Pages[page].Lines {
		pl := &e.Pages[page].Lines[i]
		d := 0.0
		if y < pl.Y {
			d = pl.Y - y
		} else if y >= pl.Y+pl.Line.Height {
			d = y - pl.Y - pl.Line.Height + 0.001
		}
		if d < bestD || d == bestD && pl.X < best.X {
			best, bestD = pl, d
		}
	}
	l := best.Line
	return model.Pos{Para: best.Para, Off: l.Start}, model.Pos{Para: best.Para, Off: l.End}, true
}

// TableBorder is a table border under the pointer: a vertical border
// between grid columns Grid-1 and Grid, or (Row >= 0, Vertical false) the
// bottom border of row Row.
type TableBorder struct {
	Table    *model.Table
	Vertical bool
	Grid     int
	Row      int
	At       float64 // the border's x (vertical) or y, page points
	// From and To are the extent of the guide line drawn while dragging:
	// the table's rows on the page (vertical) or the row's width.
	From, To float64
}

// TableBorderAt finds a table border within tol points of (x, y) on page
// pg. The table's left edge and top border are not offered.
func (e *Engine) TableBorderAt(page int, x, y, tol float64) (TableBorder, bool) {
	if page < 0 || page >= len(e.Pages) {
		return TableBorder{}, false
	}
	rows := e.Pages[page].TableRows
	var best TableBorder
	bestD := tol
	found := false
	for _, rb := range rows {
		if len(rb.Edges) < 2 || y < rb.Y0-tol || y > rb.Y1+tol {
			continue
		}
		left, right := rb.Edges[0].X, rb.Edges[len(rb.Edges)-1].X
		if y >= rb.Y0 && y <= rb.Y1 {
			for _, ed := range rb.Edges[1:] {
				if d := math.Abs(x - ed.X); d <= bestD {
					best, bestD, found = TableBorder{Table: rb.Table, Vertical: true, Grid: ed.Grid, Row: -1, At: ed.X}, d, true
				}
			}
		}
		if x >= left && x <= right {
			if d := math.Abs(y - rb.Y1); d < bestD {
				best, bestD, found = TableBorder{Table: rb.Table, Row: rb.Row, At: rb.Y1, From: left, To: right}, d, true
			}
		}
	}
	if found && best.Vertical {
		best.From, best.To = math.Inf(1), math.Inf(-1)
		for _, rb := range rows {
			if rb.Table == best.Table {
				best.From, best.To = min(best.From, rb.Y0), max(best.To, rb.Y1)
			}
		}
	}
	return best, found
}

// RowTop returns the top of a table row placed on page pg.
func (e *Engine) RowTop(page int, t *model.Table, row int) (float64, bool) {
	if page < 0 || page >= len(e.Pages) {
		return 0, false
	}
	for _, rb := range e.Pages[page].TableRows {
		if rb.Table == t && rb.Row == row {
			return rb.Y0, true
		}
	}
	return 0, false
}
