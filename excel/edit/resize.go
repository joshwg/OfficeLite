package edit

import (
	"slices"

	"officelite/excel/model"
)

// dimChange is a change of column widths or row heights.
type dimChange struct {
	sheet  int
	cols   bool
	idx    []int
	before []dimValue
	after  []float32 // per index
}

// dimValue is a width or height before a change; set false means the
// default.
type dimValue struct {
	v   float32
	set bool
}

func (d *dimChange) sizes(wb *model.Workbook) map[int]float32 {
	sh := wb.Sheets[d.sheet]
	if d.cols {
		return sh.ColWidth
	}
	return sh.RowHeight
}

func (d *dimChange) set(wb *model.Workbook, after bool) {
	m := d.sizes(wb)
	for i, x := range d.idx {
		switch {
		case after:
			m[x] = d.after[i]
		case d.before[i].set:
			m[x] = d.before[i].v
		default:
			delete(m, x)
		}
	}
}

// ResizeCols sets the width of columns idx on sheet, in characters
// (model.Sheet.ColWidth units; 0 hides them).
func (b *Book) ResizeCols(sheet int, idx []int, width float32) {
	defer b.Op("Column Width")()
	b.SetColWidths(sheet, idx, repeat(width, len(idx)))
}

// ResizeRows sets the height of rows idx on sheet, in pixels (0 hides
// them).
func (b *Book) ResizeRows(sheet int, idx []int, height float32) {
	defer b.Op("Row Height")()
	b.SetRowHeights(sheet, idx, repeat(height, len(idx)))
}

// SetColWidths gives each column idx[i] its own width widths[i], as one
// change (AutoFit of several columns).
func (b *Book) SetColWidths(sheet int, idx []int, widths []float32) {
	defer b.Op("Column Width")()
	if len(idx) == 0 {
		return
	}
	b.resize(&dimChange{sheet: sheet, cols: true, idx: idx, after: widths},
		Where{sheet, model.Range{C1: minOf(idx), C2: maxOf(idx), R2: model.MaxRows - 1}})
}

// SetRowHeights gives each row idx[i] its own height heights[i].
func (b *Book) SetRowHeights(sheet int, idx []int, heights []float32) {
	defer b.Op("Row Height")()
	if len(idx) == 0 {
		return
	}
	b.resize(&dimChange{sheet: sheet, idx: idx, after: heights},
		Where{sheet, model.Range{R1: minOf(idx), R2: maxOf(idx), C2: model.MaxCols - 1}})
}

// Hide hides rows (columns) idx (Format > Hide Rows, a divider dragged
// to zero). Each keeps its size for when it is shown again; one undo step
// (an outline change, which restores the sizes and the outline).
func (b *Book) Hide(sheet int, rows bool, idx []int) bool {
	defer b.Op(axisLabel("Hide", rows))()
	if len(idx) == 0 {
		return false
	}
	return b.changeOutline(sheet, axisWhere(rows, minOf(idx), maxOf(idx)), func(sh *model.Sheet) bool {
		changed := false
		for _, i := range idx {
			if !sh.IndexHidden(rows, i) {
				sh.SetIndexHidden(rows, i, true)
				changed = true
			}
		}
		return changed
	})
}

// Unhide shows the hidden rows (columns) of lo..hi, wherever they are
// (a whole sheet selected reaches past the used area), with the sizes
// they had before they were hidden. It reports false when none was
// hidden.
func (b *Book) Unhide(sheet int, rows bool, lo, hi int) bool {
	defer b.Op(axisLabel("Unhide", rows))()
	sh := b.WB.Sheets[sheet]
	sizes := sh.ColWidth
	if rows {
		sizes = sh.RowHeight
	}
	var idx []int
	for i, v := range sizes {
		if v == 0 && i >= lo && i <= hi {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return false
	}
	slices.Sort(idx)
	return b.changeOutline(sheet, axisWhere(rows, lo, hi), func(sh *model.Sheet) bool {
		for _, i := range idx {
			sh.SetIndexHidden(rows, i, false)
		}
		return true
	})
}

func repeat(v float32, n int) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = v
	}
	return out
}

func (b *Book) resize(d *dimChange, where Where) {
	if len(d.idx) == 0 {
		return
	}
	m := d.sizes(b.WB)
	d.before = make([]dimValue, len(d.idx))
	for i, x := range d.idx {
		v, ok := m[x]
		d.before[i] = dimValue{v, ok}
	}
	d.set(b.WB, true)
	b.visibilityChanged()
	b.record(&change{where: where, dims: d})
}

func minOf(xs []int) int {
	m := xs[0]
	for _, x := range xs {
		m = min(m, x)
	}
	return m
}

func maxOf(xs []int) int {
	m := xs[0]
	for _, x := range xs {
		m = max(m, x)
	}
	return m
}

// axisLabel is verb's Undo list name for rows or columns.
func axisLabel(verb string, rows bool) string {
	if rows {
		return verb + " Rows"
	}
	return verb + " Columns"
}
