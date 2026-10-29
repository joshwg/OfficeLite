// Inserting and deleting cells with a shift (Excel's Insert Cells / Delete
// Cells "shift cells down/right/up/left"): only the cells in the columns
// (rows) of the block move, with the merged areas, spill areas, notes and
// links inside that strip. Formula text is not touched here; see
// formula.AdjustRowsIn.

package model

// ShiftStrip is the part of the sheet that moves when block r is inserted
// or deleted with cells shifted along rows (vertical) or columns.
func ShiftStrip(r Range, vertical bool) Range {
	if vertical {
		return Range{R1: r.R1, C1: r.C1, R2: MaxRows - 1, C2: r.C2}
	}
	return Range{R1: r.R1, C1: r.C1, R2: r.R2, C2: MaxCols - 1}
}

// MergeCrosses reports whether a merged area lies partly inside strip.
func (s *Sheet) MergeCrosses(strip Range) bool {
	for _, m := range s.Merges {
		in := m.R1 >= strip.R1 && m.R2 <= strip.R2 && m.C1 >= strip.C1 && m.C2 <= strip.C2
		out := m.R2 < strip.R1 || m.R1 > strip.R2 || m.C2 < strip.C1 || m.C1 > strip.C2
		if !in && !out {
			return true
		}
	}
	return false
}

// ShiftCells inserts the empty block r, moving the cells below it down
// (vertical) or those to its right right, or with insert false deletes
// r, moving them up or left. Cells pushed past the sheet's end are
// dropped.
func (s *Sheet) ShiftCells(r Range, vertical, insert bool) {
	strip := ShiftStrip(r, vertical)
	n := r.R2 - r.R1 + 1
	at := r.R1
	if !vertical {
		n, at = r.C2-r.C1+1, r.C1
	}
	if !insert {
		n = -n
	}
	put := func(row, col int, c Cell) {
		if c == (Cell{}) && (row >= len(s.rows) || col >= len(s.rows[row])) {
			return
		}
		s.Set(row, col, c)
	}
	s.clearStripSpills(strip, vertical, at, n)
	if vertical {
		used := s.UsedRows()
		for c := r.C1; c <= r.C2; c++ {
			if n > 0 {
				for row := used - 1; row >= at; row-- {
					put(row+n, c, *s.Get(row, c))
				}
				for row := at; row < min(at+n, used); row++ {
					put(row, c, Cell{})
				}
			} else {
				for row := at; row < used; row++ {
					put(row, c, *s.Get(row-n, c))
				}
			}
		}
	} else {
		for row := r.R1; row <= min(r.R2, s.UsedRows()-1); row++ {
			used := s.RowLen(row)
			if n > 0 {
				for c := used - 1; c >= at; c-- {
					put(row, c+n, *s.Get(row, c))
				}
				for c := at; c < min(at+n, used); c++ {
					put(row, c, Cell{})
				}
			} else {
				for c := at; c < used; c++ {
					put(row, c, *s.Get(row, c-n))
				}
			}
		}
	}

	// move shifts a position inside the strip; false when it was deleted
	// or pushed off the sheet.
	move := func(k [2]int) ([2]int, bool) {
		if !strip.Contains(k[0], k[1]) {
			return k, true
		}
		i, limit := &k[0], MaxRows
		if !vertical {
			i, limit = &k[1], MaxCols
		}
		var ok bool
		*i, ok = ShiftIndex(*i, at, n)
		return k, ok && *i < limit
	}
	moveRange := func(m Range) (Range, bool) {
		if !strip.Contains(m.R1, m.C1) || !strip.Contains(m.R2, m.C2) {
			return m, true
		}
		return shiftRange(m, vertical, at, n)
	}
	merges := s.Merges[:0]
	for _, m := range s.Merges {
		if m, ok := moveRange(m); ok && (m.R1 != m.R2 || m.C1 != m.C2) {
			merges = append(merges, m)
		}
	}
	s.Merges = merges
	spills := make(map[[2]int]Range, len(s.Spills))
	for a, rng := range s.Spills {
		if a2, ok := move(a); ok {
			if rng, ok := moveRange(rng); ok {
				spills[a2] = rng
			}
		}
	}
	s.Spills = spills
	s.Notes = shiftCellMap(s.Notes, move)
	s.Links = shiftCellMap(s.Links, move)
	s.shiftStripValidations(strip, vertical, at, n)
}

// clearStripSpills empties the spilled values of formulas in the strip
// that a deletion removes.
func (s *Sheet) clearStripSpills(strip Range, vertical bool, at, n int) {
	if n > 0 {
		return
	}
	for a, rng := range s.Spills {
		i := a[1]
		if vertical {
			i = a[0]
		}
		if !strip.Contains(a[0], a[1]) || i < at || i >= at-n {
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
