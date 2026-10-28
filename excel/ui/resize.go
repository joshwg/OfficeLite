package ui

import (
	"fmt"
	"image"
	"math"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"officelite/excel/calc"
	"officelite/excel/model"
	"officelite/excel/render"
)

// resizeDrag is a column or row header divider being dragged.
type resizeDrag struct {
	cols    bool
	edge    int   // the column (row) whose divider was pressed
	start   int   // screen position of the dragged column's left (row's top) edge
	targets []int // the columns or rows resized: the dragged one, or every selected one
	orig    []dimSize
	size    float32 // the new width (characters) or height (pixels)
}

type dimSize struct {
	v   float32
	set bool
}

// dividerID identifies a header divider, for double-clicks.
type dividerID struct {
	cols bool
	idx  int
}

// dividerCursors shows the resize cursor over the header dividers.
func (a *App) dividerCursors(gtx layout.Context, v *render.View) {
	hw, hh := v.HeaderSize()
	slop := v.EdgeSlop()
	for _, e := range v.ColEdges() {
		r := clip.Rect{Min: image.Pt(max(e.Pos-slop, hw), 0), Max: image.Pt(e.Pos+slop+1, hh)}.Push(gtx.Ops)
		pointer.CursorColResize.Add(gtx.Ops)
		r.Pop()
	}
	for _, e := range v.RowEdges() {
		r := clip.Rect{Min: image.Pt(0, max(e.Pos-slop, hh)), Max: image.Pt(hw, e.Pos+slop+1)}.Push(gtx.Ops)
		pointer.CursorRowResize.Add(gtx.Ops)
		r.Pop()
	}
}

// beginResize starts dragging a header divider if p is on one. As in
// Excel, dragging a divider inside a selection of whole columns (rows)
// resizes all of them.
func (a *App) beginResize(st *sheetState, p f32.Point) bool {
	v := &st.view
	hw, hh := v.HeaderSize()
	sel := v.Sel.Range
	var d resizeDrag
	switch {
	case p.Y < float32(hh) && p.X >= float32(hw):
		c, ok := v.ColEdgeAt(p.X)
		if !ok {
			return false
		}
		d.cols, d.edge = true, c
		d.start, _, _ = v.ColSpan(c)
		d.targets = []int{c}
		if sel.R1 == 0 && sel.R2 == model.MaxRows-1 && c >= sel.C1 && c <= sel.C2 {
			d.targets = span(sel.C1, sel.C2)
		}
	case p.X < float32(hw) && p.Y >= float32(hh):
		r, ok := v.RowEdgeAt(p.Y)
		if !ok {
			return false
		}
		d.edge = r
		d.start, _, _ = v.RowSpan(r)
		d.targets = []int{r}
		if sel.C1 == 0 && sel.C2 == model.MaxCols-1 && r >= sel.R1 && r <= sel.R2 {
			d.targets = span(sel.R1, sel.R2)
		}
	default:
		return false
	}
	sizes := a.dimMap(v.Sheet, d.cols)
	d.orig = make([]dimSize, len(d.targets))
	for i, t := range d.targets {
		s, ok := sizes[t]
		d.orig[i] = dimSize{s, ok}
	}
	d.size = -1
	a.grid.resize = &d
	return true
}

func span(lo, hi int) []int {
	out := make([]int, 0, hi-lo+1)
	for i := lo; i <= hi; i++ {
		out = append(out, i)
	}
	return out
}

func (a *App) dimMap(sh *model.Sheet, cols bool) map[int]float32 {
	if cols {
		return sh.ColWidth
	}
	return sh.RowHeight
}

// dragResize previews the new size and shows it in the status bar, as
// Excel's tooltip does.
func (a *App) dragResize(gtx layout.Context, st *sheetState, p f32.Point) {
	d := a.grid.resize
	v := &st.view
	pos := p.Y
	if d.cols {
		pos = p.X
	}
	px := math.Max(0, float64(pos-float32(d.start))/float64(gtx.Metric.PxPerDp*a.zoom)) // sheet pixels
	if d.cols {
		d.size = model.PxToColWidth(px, v.DigitWidth)
		px = float64(model.ColWidthToPx(float64(d.size), v.DigitWidth))
		if d.size == 0 {
			px = 0
		}
		shown := math.Trunc(math.Max(px-5, 0)/v.DigitWidth*100+0.5) / 100
		a.notice = fmt.Sprintf("Width: %.2f (%d pixels)", shown, int(px))
	} else {
		d.size = float32(math.Round(px))
		a.notice = fmt.Sprintf("Height: %.2f (%d pixels)", float64(d.size)*0.75, int(d.size))
	}
	sizes := a.dimMap(v.Sheet, d.cols)
	for _, t := range d.targets {
		sizes[t] = d.size
	}
	v.Geo = render.NewGeometry(v.Sheet, v.DigitWidth)
}

// endResize restores the original sizes and, for a completed drag, makes
// the change through the undo stack.
func (a *App) endResize(st *sheetState, commit bool) {
	d := a.grid.resize
	a.grid.resize = nil
	v := &st.view
	sizes := a.dimMap(v.Sheet, d.cols)
	for i, t := range d.targets {
		if d.orig[i].set {
			sizes[t] = d.orig[i].v
		} else {
			delete(sizes, t)
		}
	}
	a.notice = ""
	if commit && d.size >= 0 {
		if d.cols {
			a.book.ResizeCols(a.active, d.targets, d.size)
		} else {
			a.book.ResizeRows(a.active, d.targets, d.size)
		}
		a.changed(calc.Result{})
	}
	v.Geo = render.NewGeometry(v.Sheet, v.DigitWidth)
}

// autoFit sizes the columns (rows) of a divider press to their contents:
// each column as wide as its widest text, each row as tall as its tallest.
// Columns and rows without text keep their size.
func (a *App) autoFit(gtx layout.Context, st *sheetState, d *resizeDrag) {
	v := &st.view
	var idx []int
	var sizes []float32
	for _, t := range d.targets {
		if d.cols {
			if px, ok := v.FitColWidth(gtx, t); ok {
				idx = append(idx, t)
				w := model.PxToColWidth(px, v.DigitWidth)
				if float64(model.ColWidthToPx(float64(w), v.DigitWidth)) < px {
					w = model.PxToColWidth(px+1, v.DigitWidth) // rounded down
				}
				sizes = append(sizes, w)
			}
		} else if px, ok := v.FitRowHeight(gtx, t); ok {
			idx = append(idx, t)
			sizes = append(sizes, float32(px))
		}
	}
	if len(idx) == 0 {
		return
	}
	if d.cols {
		a.book.SetColWidths(a.active, idx, sizes)
	} else {
		a.book.SetRowHeights(a.active, idx, sizes)
	}
	a.changed(calc.Result{})
	v.Geo = render.NewGeometry(v.Sheet, v.DigitWidth)
}
