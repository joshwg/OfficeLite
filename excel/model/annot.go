// Cell notes and hyperlinks: their types, accessors, and how they move
// when rows or columns are inserted or deleted.

package model

// Note is a cell note (what Excel 365 calls a Note and older versions a
// Comment): plain text with its author, shown when the cell is pointed at.
type Note struct {
	Author string
	Text   string
}

// Hyperlink is a cell's link. The cell's own text is what Excel calls the
// text to display.
type Hyperlink struct {
	// Target is an external address: a URL, a mailto: address or a file
	// path. Empty for a link to a place in the workbook.
	Target string
	// Location is a place in the workbook (Sheet2!A1, a defined name), or
	// for an external target the part after '#'.
	Location string
	Tooltip  string
}

// Address is the link as typed in the Insert Hyperlink dialog.
func (h Hyperlink) Address() string {
	switch {
	case h.Target == "":
		return "#" + h.Location
	case h.Location != "":
		return h.Target + "#" + h.Location
	}
	return h.Target
}

// NoteAt returns the note of row, col.
func (s *Sheet) NoteAt(row, col int) (Note, bool) {
	n, ok := s.Notes[[2]int{row, col}]
	return n, ok
}

// SetNote sets (or with nil, removes) the note of row, col.
func (s *Sheet) SetNote(row, col int, n *Note) {
	k := [2]int{row, col}
	if n == nil {
		delete(s.Notes, k)
		return
	}
	if s.Notes == nil {
		s.Notes = map[[2]int]Note{}
	}
	s.Notes[k] = *n
}

// LinkAt returns the hyperlink of row, col.
func (s *Sheet) LinkAt(row, col int) (Hyperlink, bool) {
	h, ok := s.Links[[2]int{row, col}]
	return h, ok
}

// SetLink sets (or with nil, removes) the hyperlink of row, col.
func (s *Sheet) SetLink(row, col int, h *Hyperlink) {
	k := [2]int{row, col}
	if h == nil {
		delete(s.Links, k)
		return
	}
	if s.Links == nil {
		s.Links = map[[2]int]Hyperlink{}
	}
	s.Links[k] = *h
}

// shiftAnnots moves notes and links for inserted or deleted rows
// (columns); those on deleted cells go.
func (s *Sheet) shiftAnnots(rows bool, at, n int) {
	s.Notes = shiftCellMap(s.Notes, func(k [2]int) ([2]int, bool) {
		i := &k[1]
		if rows {
			i = &k[0]
		}
		var ok bool
		*i, ok = ShiftIndex(*i, at, n)
		return k, ok
	})
	s.Links = shiftCellMap(s.Links, func(k [2]int) ([2]int, bool) {
		i := &k[1]
		if rows {
			i = &k[0]
		}
		var ok bool
		*i, ok = ShiftIndex(*i, at, n)
		return k, ok
	})
}

// shiftCellMap re-keys m by move, dropping entries it reports false for.
func shiftCellMap[V any](m map[[2]int]V, move func([2]int) ([2]int, bool)) map[[2]int]V {
	if len(m) == 0 {
		return m
	}
	out := make(map[[2]int]V, len(m))
	for k, v := range m {
		if k2, ok := move(k); ok {
			out[k2] = v
		}
	}
	return out
}
