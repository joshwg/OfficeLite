// Cell notes and hyperlinks: their types, accessors, and how they move
// when rows or columns are inserted or deleted.

package model

import (
	"strings"
	"time"
)

// Note is a cell note (what Excel 365 calls a Note and older versions a
// Comment): plain text with its author, shown when the cell is pointed at.
// With Threaded set it is instead an Excel 365 comment: a discussion
// whose first post is Author, Text and Time, followed by Replies.
//
// Notes are stored by value and Replies is shared between copies: change
// it only by building a new slice (see WithReply).
type Note struct {
	Author string
	Text   string

	Threaded bool
	Time     time.Time // when the first post was made (threaded only); zero if unknown
	Replies  []Post
	Resolved bool // the thread is marked done
}

// Post is a reply in a comment thread.
type Post struct {
	Author string
	Text   string
	Time   time.Time
}

// Posts returns a thread's posts in order, the first post included.
func (n Note) Posts() []Post {
	return append([]Post{{Author: n.Author, Text: n.Text, Time: n.Time}}, n.Replies...)
}

// WithReply returns the thread with p added as its last reply.
func (n Note) WithReply(p Post) Note {
	n.Replies = append(n.Replies[:len(n.Replies):len(n.Replies)], p)
	return n
}

// PlainText is the note's text, or a thread's posts written out one after
// another ("Author: text"), for formats that only hold notes.
func (n Note) PlainText() string {
	if !n.Threaded {
		return n.Text
	}
	var b strings.Builder
	for i, p := range n.Posts() {
		if i > 0 {
			b.WriteString("\n")
		}
		if p.Author != "" {
			b.WriteString(p.Author + ": ")
		}
		b.WriteString(p.Text)
	}
	return b.String()
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
