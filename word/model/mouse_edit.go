package model

// Edits made with the mouse in the page view: dragging selected text to
// move or copy it, and dragging table borders to resize columns and rows.
// Each is one undo step.

// MinColumnWidth is the narrowest a column gets by dragging its border,
// points.
const MinColumnWidth = 2*cellPad + 4

// MinRowHeight is the lowest row height set by dragging, points.
const MinRowHeight = 4

// CanMoveSelection reports whether the selection can be dragged to
// another place: it is not empty and lies in one container (the body or
// one table cell) with no table inside it.
func (e *Editor) CanMoveSelection() bool {
	s, t := e.Sel.Ordered()
	if s == t {
		return false
	}
	for i := s.Para + 1; i <= t.Para; i++ {
		if e.locs[i].Blocks != e.locs[s.Para].Blocks || e.locs[i].Index != e.locs[i-1].Index+1 {
			return false
		}
	}
	return true
}

// MoveText moves the selection to dst (copies it when copy is set),
// keeping its formatting, and selects it there. Dropping inside the
// selection does nothing; it reports whether anything changed.
func (e *Editor) MoveText(dst Pos, copy bool) bool {
	if !e.CanMoveSelection() {
		return false
	}
	s, t := e.Sel.Ordered()
	dst = e.clamp(dst)
	if !dst.Less(s) && !t.Less(dst) {
		return false
	}
	f := e.Copy()
	from := min(e.locs[s.Para].Top, e.locs[dst.Para].Top)
	to := max(e.locs[t.Para].Top, e.locs[dst.Para].Top)
	kind := "move"
	if copy {
		kind = "copy"
	}
	e.edit(kind, from, to, false, func() {
		if !copy {
			n := len(e.locs)
			e.deleteSelection()
			if t.Less(dst) {
				// The text before dst is gone: paragraphs after the
				// selection move up, and on its last line dst moves left.
				np := dst.Para - (n - len(e.locs))
				off := dst.Off
				if dst.Para == t.Para {
					off -= t.Off
					if np == s.Para {
						off += s.Off
					}
				}
				dst = Pos{np, off}
			}
		}
		end := e.pasteAt(dst, f)
		e.Sel = Selection{dst, end}
	})
	return true
}

// ColumnBorderRange returns how far the border at grid boundary g (the
// right edge of column g-1) can move left (lo, negative) and right (hi):
// the columns on both sides keep MinColumnWidth; at the table's right
// edge only the last column changes.
func ColumnBorderRange(t *Table, g int) (lo, hi float64) {
	cols := gridWidths(t)
	if g < 1 || g > len(cols) {
		return 0, 0
	}
	lo = min(MinColumnWidth-cols[g-1], 0)
	hi = 1e6
	if g < len(cols) {
		hi = max(cols[g]-MinColumnWidth, 0)
	}
	return lo, hi
}

// MoveColumnBorder moves the border at grid boundary g by dx points, as
// dragging it in Word does: column g-1 grows (or shrinks) and column g
// gives up (or takes) the same width; the table's right edge changes only
// the last column. It reports whether the table changed.
func (e *Editor) MoveColumnBorder(t *Table, g int, dx float64) bool {
	lo, hi := ColumnBorderRange(t, g)
	dx = min(max(dx, lo), hi)
	if dx > -0.05 && dx < 0.05 || len(t.Rows) == 0 {
		return false
	}
	top := e.tableTop(t)
	e.edit("table", top, top, false, func() {
		cols := gridWidths(t)
		cols[g-1] += dx
		if g < len(cols) {
			cols[g] -= dx
		}
		t.Columns = cols
		fixCellWidths(t)
	})
	return true
}

// SetRowHeight sets row r's height (at least h points, as Word sets it
// when its border is dragged).
func (e *Editor) SetRowHeight(t *Table, r int, h float64) bool {
	if r < 0 || r >= len(t.Rows) {
		return false
	}
	h = max(h, MinRowHeight)
	if d := h - t.Rows[r].Height; d > -0.05 && d < 0.05 {
		return false
	}
	top := e.tableTop(t)
	e.edit("table", top, top, false, func() { t.Rows[r].Height = h })
	return true
}

// tableTop is the index of the body block that is or holds table t.
func (e *Editor) tableTop(t *Table) int {
	var holds func(b Block) bool
	holds = func(b Block) bool {
		tb, ok := b.(*Table)
		if !ok {
			return false
		}
		if tb == t {
			return true
		}
		for _, row := range tb.Rows {
			for _, c := range row.Cells {
				for _, cb := range c.Blocks {
					if holds(cb) {
						return true
					}
				}
			}
		}
		return false
	}
	for i, b := range e.Doc.Body {
		if holds(b) {
			return i
		}
	}
	return 0
}
