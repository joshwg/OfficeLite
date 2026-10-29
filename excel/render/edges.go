package render

import "math"

// Edge is the right edge of a column, or the bottom edge of a row, on
// screen: where a header divider can be dragged to resize it.
type Edge struct {
	Index int // column or row
	Pos   int // screen x (columns) or y (rows)
}

// ColSpan is column c's screen x range, or ok false when it's scrolled
// out of view.
func (v *View) ColSpan(c int) (x0, x1 int, ok bool) {
	g := v.Geo.Cols
	switch {
	case v.scale == 0:
		return 0, 0, false
	case c < v.fc:
		x0, x1 = v.hdrW+v.px(g.Start(c)), v.hdrW+v.px(g.End(c))
	case c >= v.LeftCol:
		s := g.Start(v.LeftCol)
		x0, x1 = v.frozenX+v.px(g.Start(c)-s), v.frozenX+v.px(g.End(c)-s)
	default:
		return 0, 0, false
	}
	return x0, x1, x0 < v.size.X
}

// RowSpan is row r's screen y range, or ok false when it's scrolled out
// of view.
func (v *View) RowSpan(r int) (y0, y1 int, ok bool) {
	g := v.Geo.Rows
	switch {
	case v.scale == 0:
		return 0, 0, false
	case r < v.fr:
		y0, y1 = v.hdrH+v.px(g.Start(r)), v.hdrH+v.px(g.End(r))
	case r >= v.TopRow:
		s := g.Start(v.TopRow)
		y0, y1 = v.frozenY+v.px(g.Start(r)-s), v.frozenY+v.px(g.End(r)-s)
	default:
		return 0, 0, false
	}
	return y0, y1, y0 < v.size.Y
}

// ColEdges lists the visible columns' right edges. Hidden columns have
// no edge of their own.
func (v *View) ColEdges() []Edge {
	return v.edges(v.fc, v.LeftCol, v.Geo.Cols, v.ColSpan)
}

// RowEdges lists the visible rows' bottom edges.
func (v *View) RowEdges() []Edge {
	return v.edges(v.fr, v.TopRow, v.Geo.Rows, v.RowSpan)
}

func (v *View) edges(frozen, first int, ax *Axis, span func(int) (int, int, bool)) []Edge {
	var out []Edge
	add := func(i int) bool {
		if ax.Hidden(i) {
			return true
		}
		_, end, ok := span(i)
		if !ok {
			return false
		}
		out = append(out, Edge{i, end})
		return true
	}
	for i := 0; i < frozen; i++ {
		add(i)
	}
	for i := first; i < ax.Len() && add(i); i++ {
	}
	return out
}

// EdgeSlop is how far, in screen pixels, from a divider the resize cursor
// still applies.
func (v *View) EdgeSlop() int {
	z := float64(v.Zoom)
	if z == 0 {
		z = 1
	}
	return max(2, int(math.Round(3*v.scale/z)))
}

// ColEdgeAt returns the column whose right edge is within EdgeSlop of x.
func (v *View) ColEdgeAt(x float32) (int, bool) { return edgeAt(v.ColEdges(), x, v.EdgeSlop()) }

// RowEdgeAt returns the row whose bottom edge is within EdgeSlop of y.
func (v *View) RowEdgeAt(y float32) (int, bool) { return edgeAt(v.RowEdges(), y, v.EdgeSlop()) }

// HeaderSize is where the cells start on screen: the right edge of the
// row headers and the bottom of the column headers (the outline's
// gutters included; see Gutters).
func (v *View) HeaderSize() (w, h int) { return v.hdrW, v.hdrH }

// Gutters is the width of the row outline's gutter left of the row
// headers and the height of the column outline's gutter above the column
// headers (0 without an outline); the headers start there.
func (v *View) Gutters() (w, h int) { return v.gutW, v.gutH }

func edgeAt(edges []Edge, p float32, slop int) (int, bool) {
	best, dist := -1, slop+1
	for _, e := range edges {
		if d := int(math.Abs(float64(p) - float64(e.Pos))); d < dist {
			best, dist = e.Index, d
		}
	}
	return best, best >= 0
}
