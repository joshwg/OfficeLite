package model

import "slices"

// Structural edits: inserting and deleting rows and columns moves the
// cells and everything positioned by row or column (sizes, merged cells,
// frozen panes, spill areas). Formula text is not touched here; see
// formula.AdjustRows.

// InsertRows inserts n empty rows before row at. Cells pushed past
// MaxRows are dropped.
func (s *Sheet) InsertRows(at, n int) {
	if n <= 0 || at >= MaxRows {
		return
	}
	if at < len(s.rows) {
		s.rows = slices.Insert(s.rows, at, make([][]Cell, n)...)
		if len(s.rows) > MaxRows {
			s.rows = s.rows[:MaxRows]
		}
	}
	s.RowHeight = shiftKeys(s.RowHeight, at, n)
	s.shift(true, at, n)
}

// DeleteRows deletes rows at..at+n-1, moving the rows below up.
func (s *Sheet) DeleteRows(at, n int) {
	if n <= 0 {
		return
	}
	s.clearOrphanSpills(true, at, n)
	if at < len(s.rows) {
		s.rows = slices.Delete(s.rows, at, min(at+n, len(s.rows)))
	}
	s.RowHeight = shiftKeys(s.RowHeight, at, -n)
	s.shift(true, at, -n)
}

// InsertCols inserts n empty columns before column at. Cells pushed past
// MaxCols are dropped.
func (s *Sheet) InsertCols(at, n int) {
	if n <= 0 || at >= MaxCols {
		return
	}
	for i, r := range s.rows {
		if at < len(r) {
			r = slices.Insert(r, at, make([]Cell, n)...)
			s.rows[i] = r[:min(len(r), MaxCols)]
		}
	}
	s.ColWidth = shiftKeys(s.ColWidth, at, n)
	s.shift(false, at, n)
}

// DeleteCols deletes columns at..at+n-1, moving the columns to the right
// left.
func (s *Sheet) DeleteCols(at, n int) {
	if n <= 0 {
		return
	}
	s.clearOrphanSpills(false, at, n)
	for i, r := range s.rows {
		if at < len(r) {
			s.rows[i] = slices.Delete(r, at, min(at+n, len(r)))
		}
	}
	s.ColWidth = shiftKeys(s.ColWidth, at, -n)
	s.shift(false, at, -n)
}

// ShiftIndex moves a row or column index for an insertion (n > 0) of n
// entries before at, or a deletion (n < 0) of entries at..at-n-1. It
// reports false for a deleted index.
func ShiftIndex(i, at, n int) (int, bool) {
	switch {
	case i < at:
		return i, true
	case n > 0:
		return i + n, true
	case i < at-n:
		return at, false
	default:
		return i + n, true
	}
}

// ShiftSpan moves the span lo..hi like ShiftIndex, clipping it when part
// of it is deleted. It reports false when all of it is.
func ShiftSpan(lo, hi, at, n int) (int, int, bool) {
	l, lok := ShiftIndex(lo, at, n)
	h, hok := ShiftIndex(hi, at, n)
	if !hok {
		h = at - 1
	}
	if !lok {
		l = at
	}
	return l, h, l <= h
}

func shiftKeys(m map[int]float32, at, n int) map[int]float32 {
	out := make(map[int]float32, len(m))
	for k, v := range m {
		if k2, ok := ShiftIndex(k, at, n); ok {
			out[k2] = v
		}
	}
	return out
}

// shiftRange moves r along rows or columns; false if it was deleted.
func shiftRange(r Range, rows bool, at, n int) (Range, bool) {
	var ok bool
	if rows {
		r.R1, r.R2, ok = ShiftSpan(r.R1, r.R2, at, n)
	} else {
		r.C1, r.C2, ok = ShiftSpan(r.C1, r.C2, at, n)
	}
	return r, ok
}

// shift moves merges, frozen panes and spill areas.
func (s *Sheet) shift(rows bool, at, n int) {
	merges := s.Merges[:0]
	for _, m := range s.Merges {
		if m, ok := shiftRange(m, rows, at, n); ok && (m.R1 != m.R2 || m.C1 != m.C2) {
			merges = append(merges, m)
		}
	}
	s.Merges = merges

	spills := make(map[[2]int]Range, len(s.Spills))
	for a, rng := range s.Spills {
		i := a[1]
		if rows {
			i = a[0]
		}
		i2, ok := ShiftIndex(i, at, n)
		if !ok {
			continue
		}
		if rows {
			a[0] = i2
		} else {
			a[1] = i2
		}
		if rng, ok := shiftRange(rng, rows, at, n); ok {
			spills[a] = rng
		}
	}
	s.Spills = spills

	freeze := &s.FreezeCols
	if rows {
		freeze = &s.FreezeRows
	}
	if *freeze > at {
		if n > 0 {
			*freeze += n
		} else {
			*freeze -= min(-n, *freeze-at)
		}
	}
}

// clearOrphanSpills empties the spilled values of formulas about to be
// deleted.
func (s *Sheet) clearOrphanSpills(rows bool, at, n int) {
	for a, rng := range s.Spills {
		i := a[1]
		if rows {
			i = a[0]
		}
		if i < at || i >= at+n {
			continue
		}
		for r := rng.R1; r <= rng.R2; r++ {
			for c := rng.C1; c <= rng.C2; c++ {
				if cell := s.Get(r, c); cell.Formula == "" && cell.Kind != KindEmpty {
					s.Set(r, c, Cell{Style: cell.Style})
				}
			}
		}
		delete(s.Spills, a)
	}
}

// SpillAnchor returns the dynamic-array formula cell whose result spilled
// into row, col (not counting the formula's own cell).
func (s *Sheet) SpillAnchor(row, col int) (r, c int, ok bool) {
	for a, rng := range s.Spills {
		if rng.Contains(row, col) && a != [2]int{row, col} {
			return a[0], a[1], true
		}
	}
	return 0, 0, false
}

// Clone returns a deep copy of the sheet.
func (s *Sheet) Clone() *Sheet {
	c := *s
	c.rows = make([][]Cell, len(s.rows))
	for i, r := range s.rows {
		c.rows[i] = slices.Clone(r)
	}
	c.ColWidth = cloneMap(s.ColWidth)
	c.RowHeight = cloneMap(s.RowHeight)
	c.Merges = slices.Clone(s.Merges)
	c.Spills = make(map[[2]int]Range, len(s.Spills))
	for k, v := range s.Spills {
		c.Spills[k] = v
	}
	return &c
}

func cloneMap(m map[int]float32) map[int]float32 {
	out := make(map[int]float32, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
