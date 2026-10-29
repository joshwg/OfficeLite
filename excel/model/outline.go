// The outline (Data > Group): every row and column has an outline level,
// 0 to MaxOutlineLevel. A run of rows whose level is at least L is a
// group of level L; its summary row is the row right below it (or above,
// with SummaryAbove), which carries the group's collapsed flag. A
// collapsed group's rows are hidden rows (height 0), as Excel keeps them;
// the size they had is kept in RowShowSize (ColShowSize) to give back
// when they are shown again. Columns work the same way, with the summary
// column at the right (or left, with SummaryLeft).

package model

import (
	"maps"
	"slices"
)

// MaxOutlineLevel is the deepest outline level (Excel's 8 levels of
// buttons: 1 for the top, 7 of grouping).
const MaxOutlineLevel = 7

// Outline is a sheet's outline.
type Outline struct {
	RowLevel, ColLevel         map[int]uint8   // 1..7; absent = 0
	RowCollapsed, ColCollapsed map[int]bool    // summary rows (columns) of collapsed groups
	RowShowSize, ColShowSize   map[int]float32 // sizes of rows (columns) the outline hid
	// SummaryAbove puts summary rows above their detail and SummaryLeft
	// summary columns at the left (Excel's default is below and right).
	SummaryAbove, SummaryLeft bool
}

// OutlineGroup is one group of the outline.
type OutlineGroup struct {
	Rows   bool // a group of rows, else of columns
	Level  int  // 1..7
	Lo, Hi int  // its detail rows (columns)
	// Summary is the summary row (column): Hi+1, or Lo-1 when summaries
	// are above (left); out of the sheet when there is none.
	Summary   int
	Collapsed bool
}

// HasSummary reports whether the group's summary row (column) is on the
// sheet, which is where its +/- button goes.
func (g OutlineGroup) HasSummary() bool {
	n := MaxCols
	if g.Rows {
		n = MaxRows
	}
	return g.Summary >= 0 && g.Summary < n
}

// Clone copies the outline.
func (o Outline) Clone() Outline {
	o.RowLevel, o.ColLevel = maps.Clone(o.RowLevel), maps.Clone(o.ColLevel)
	o.RowCollapsed, o.ColCollapsed = maps.Clone(o.RowCollapsed), maps.Clone(o.ColCollapsed)
	o.RowShowSize, o.ColShowSize = maps.Clone(o.RowShowSize), maps.Clone(o.ColShowSize)
	return o
}

// outlineMaps returns pointers to the level, collapsed and show-size maps
// of the rows (rows) or columns, so callers can create them on demand.
func (s *Sheet) outlineMaps(rows bool) (lv *map[int]uint8, coll *map[int]bool, show *map[int]float32) {
	o := &s.Outline
	if rows {
		return &o.RowLevel, &o.RowCollapsed, &o.RowShowSize
	}
	return &o.ColLevel, &o.ColCollapsed, &o.ColShowSize
}

// Level is the outline level of row (rows) or column i.
func (s *Sheet) Level(rows bool, i int) int {
	lv, _, _ := s.outlineMaps(rows)
	return int((*lv)[i])
}

// SetLevel sets the outline level of row (column) i, 0..MaxOutlineLevel.
func (s *Sheet) SetLevel(rows bool, i, level int) {
	lv, _, _ := s.outlineMaps(rows)
	level = min(max(level, 0), MaxOutlineLevel)
	if level == 0 {
		delete(*lv, i)
		return
	}
	if *lv == nil {
		*lv = map[int]uint8{}
	}
	(*lv)[i] = uint8(level)
}

// MaxLevel is the deepest outline level of the rows (columns): 0 when
// they have no outline.
func (s *Sheet) MaxLevel(rows bool) int {
	lv, _, _ := s.outlineMaps(rows)
	m := 0
	for _, l := range *lv {
		m = max(m, int(l))
	}
	return m
}

// IsCollapsed reports the collapsed flag of summary row (column) i.
func (s *Sheet) IsCollapsed(rows bool, i int) bool {
	_, coll, _ := s.outlineMaps(rows)
	return (*coll)[i]
}

// SetCollapsedFlag sets the collapsed flag of summary row (column) i.
func (s *Sheet) SetCollapsedFlag(rows bool, i int, on bool) {
	_, coll, _ := s.outlineMaps(rows)
	if !on {
		delete(*coll, i)
		return
	}
	if *coll == nil {
		*coll = map[int]bool{}
	}
	(*coll)[i] = true
}

// summaryAfter reports whether summaries follow their detail (below,
// right).
func (s *Sheet) summaryAfter(rows bool) bool {
	if rows {
		return !s.Outline.SummaryAbove
	}
	return !s.Outline.SummaryLeft
}

// Groups lists the groups of the rows (columns) by level, then position.
func (s *Sheet) Groups(rows bool) []OutlineGroup {
	lv, _, _ := s.outlineMaps(rows)
	if len(*lv) == 0 {
		return nil
	}
	idx := slices.Sorted(maps.Keys(*lv))
	after := s.summaryAfter(rows)
	var out []OutlineGroup
	for level := 1; level <= MaxOutlineLevel; level++ {
		lo, prev := -1, -2
		flush := func() {
			if lo < 0 {
				return
			}
			g := OutlineGroup{Rows: rows, Level: level, Lo: lo, Hi: prev, Summary: prev + 1}
			if !after {
				g.Summary = lo - 1
			}
			g.Collapsed = g.HasSummary() && s.IsCollapsed(rows, g.Summary)
			out = append(out, g)
			lo = -1
		}
		for _, i := range idx {
			if int((*lv)[i]) < level {
				continue
			}
			if i != prev+1 {
				flush()
			}
			if lo < 0 {
				lo = i
			}
			prev = i
		}
		flush()
	}
	return out
}

// IndexHidden reports whether row (column) i is hidden by its size (the
// AutoFilter's rows aside).
func (s *Sheet) IndexHidden(rows bool, i int) bool {
	if rows {
		h, ok := s.RowHeight[i]
		return ok && h == 0
	}
	w, ok := s.ColWidth[i]
	return ok && w == 0
}

// SetIndexHidden hides or shows row (column) i. A hidden row keeps its
// size in RowShowSize, which it gets back when shown; a row that had none
// gets the default.
func (s *Sheet) SetIndexHidden(rows bool, i int, hidden bool) {
	sizes := s.ColWidth
	if rows {
		sizes = s.RowHeight
	}
	_, _, show := s.outlineMaps(rows)
	cur, ok := sizes[i]
	if hidden {
		if ok && cur == 0 {
			return
		}
		if ok {
			if *show == nil {
				*show = map[int]float32{}
			}
			(*show)[i] = cur
		}
		sizes[i] = 0
		return
	}
	if !ok || cur != 0 {
		return
	}
	if v, ok := (*show)[i]; ok && v > 0 {
		sizes[i] = v
	} else {
		delete(sizes, i)
	}
	delete(*show, i)
}

// CollapseGroup hides g's detail and marks its summary collapsed.
func (s *Sheet) CollapseGroup(g OutlineGroup) {
	for i := g.Lo; i <= g.Hi; i++ {
		s.SetIndexHidden(g.Rows, i, true)
	}
	if g.HasSummary() {
		s.SetCollapsedFlag(g.Rows, g.Summary, true)
	}
}

// ExpandGroup shows g's detail, apart from the detail of collapsed groups
// inside it, and clears its summary's collapsed flag.
func (s *Sheet) ExpandGroup(g OutlineGroup) {
	if g.HasSummary() {
		s.SetCollapsedFlag(g.Rows, g.Summary, false)
	}
	keep := map[int]bool{}
	for _, sub := range s.Groups(g.Rows) {
		if sub.Level > g.Level && sub.Collapsed && sub.Lo >= g.Lo && sub.Hi <= g.Hi {
			for i := sub.Lo; i <= sub.Hi; i++ {
				keep[i] = true
			}
		}
	}
	for i := g.Lo; i <= g.Hi; i++ {
		if !keep[i] {
			s.SetIndexHidden(g.Rows, i, false)
		}
	}
}

// ShowLevels shows the outline down to level n (the level buttons): rows
// (columns) at level n or deeper are hidden, the others shown; groups of
// level n or deeper are collapsed, the others expanded.
func (s *Sheet) ShowLevels(rows bool, n int) {
	lv, _, _ := s.outlineMaps(rows)
	for i, l := range *lv {
		s.SetIndexHidden(rows, i, int(l) >= n)
	}
	for _, g := range s.Groups(rows) {
		if g.HasSummary() {
			s.SetCollapsedFlag(rows, g.Summary, g.Level >= n)
		}
	}
}

// TidyOutline drops collapsed flags no group uses any more.
func (s *Sheet) TidyOutline() {
	for _, rows := range []bool{true, false} {
		_, coll, _ := s.outlineMaps(rows)
		if len(*coll) == 0 {
			continue
		}
		used := map[int]bool{}
		for _, g := range s.Groups(rows) {
			if g.HasSummary() {
				used[g.Summary] = true
			}
		}
		maps.DeleteFunc(*coll, func(i int, _ bool) bool { return !used[i] })
	}
}

// shiftOutline moves the outline with inserted or deleted rows (columns).
// Inserted rows join a group when both their neighbors are in it.
func (s *Sheet) shiftOutline(rows bool, at, n int) {
	lv, coll, show := s.outlineMaps(rows)
	if len(*lv) == 0 && len(*coll) == 0 && len(*show) == 0 {
		return
	}
	ins := 0
	if n > 0 && at > 0 {
		ins = min(int((*lv)[at-1]), int((*lv)[at]))
	}
	if len(*lv) > 0 {
		out := make(map[int]uint8, len(*lv)+max(n, 0))
		for k, v := range *lv {
			if k2, ok := ShiftIndex(k, at, n); ok {
				out[k2] = v
			}
		}
		for i := at; i < at+n && ins > 0; i++ {
			out[i] = uint8(ins)
		}
		*lv = out
	}
	if len(*coll) > 0 {
		out := make(map[int]bool, len(*coll))
		for k := range *coll {
			if k2, ok := ShiftIndex(k, at, n); ok {
				out[k2] = true
			}
		}
		*coll = out
	}
	if len(*show) > 0 {
		*show = shiftKeys(*show, at, n)
	}
}
